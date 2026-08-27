package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Pool manages a bounded set of claimed tasks and a fixed-size executor pool.
// PostgreSQL performs cluster-wide queue admission; this process only limits
// how much leased work it can hold locally.
type Pool struct {
	q            queue.Queue
	workerID     string
	concurrency  int
	pollInterval time.Duration
	leaseTimeout time.Duration
	batchSize    int
	logger       *slog.Logger
	handler      Handler

	runCtx       context.Context
	cancelRun    context.CancelFunc
	claimCtx     context.Context
	cancelClaims context.CancelFunc
	stopOnce     sync.Once
	claimDone    chan struct{}
	heartbeatWG  sync.WaitGroup
	executorWG   sync.WaitGroup
	renewWG      sync.WaitGroup
	jobs         chan taskJob
	slots        chan struct{}
}

type Handler interface {
	Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (*TaskExecResult, error)
}

type TaskExecResult struct {
	Summary   string
	Artifacts map[string]string
	Metrics   map[string]float64
}

func NewPool(q queue.Queue, workerID string, concurrency int, pollInterval, leaseTimeout time.Duration, batchSize int, handler Handler, logger *slog.Logger) *Pool {
	if concurrency <= 0 {
		concurrency = 1
	}
	if batchSize <= 0 {
		batchSize = 1
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	if leaseTimeout <= 0 {
		leaseTimeout = 90 * time.Second
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	claimCtx, cancelClaims := context.WithCancel(runCtx)
	prefetch := batchSize
	if prefetch > concurrency {
		prefetch = concurrency
	}
	ownedLimit := concurrency + prefetch
	return &Pool{
		q: q, workerID: workerID, concurrency: concurrency,
		pollInterval: pollInterval, leaseTimeout: leaseTimeout, batchSize: batchSize,
		handler: handler, logger: logger,
		runCtx: runCtx, cancelRun: cancelRun, claimCtx: claimCtx, cancelClaims: cancelClaims,
		claimDone: make(chan struct{}), jobs: make(chan taskJob, ownedLimit), slots: make(chan struct{}, ownedLimit),
	}
}

func (p *Pool) Start() {
	p.logger.Info("worker pool starting", "worker_id", p.workerID, "concurrency", p.concurrency,
		"poll_interval", p.pollInterval, "lease_timeout", p.leaseTimeout, "max_local_claims", cap(p.slots))
	p.heartbeatWG.Add(1)
	go p.heartbeatLoop()
	go p.claimLoop()
	for i := 0; i < p.concurrency; i++ {
		p.executorWG.Add(1)
		go p.executeLoop(i)
	}
}

func (p *Pool) Stop() { _ = p.StopContext(context.Background()) }

// StopContext stops new claims, drains and renews already-owned tasks, then
// force-cancels remaining executions when the caller deadline expires.
func (p *Pool) StopContext(ctx context.Context) error {
	p.stopOnce.Do(p.cancelClaims)
	select {
	case <-p.claimDone:
	case <-ctx.Done():
		p.cancelRun()
		return ctx.Err()
	}

	drained := make(chan struct{})
	go func() { p.executorWG.Wait(); close(drained) }()
	var stopErr error
	select {
	case <-drained:
	case <-ctx.Done():
		stopErr = ctx.Err()
		p.cancelRun()
		// A Go process cannot forcibly terminate a handler goroutine that ignores
		// context cancellation. Return to main so process shutdown can complete;
		// the lease will expire and another worker will recover the task.
		return stopErr
	}
	p.cancelRun()
	p.renewWG.Wait()
	p.heartbeatWG.Wait()
	p.logger.Info("worker pool stopped", "worker_id", p.workerID, "forced", stopErr != nil)
	return stopErr
}

type taskJob struct {
	ID             string
	Type           string
	Queue          string
	Payload        map[string]any
	TimeoutSeconds int
	Attempt        int
	LeaseVersion   int
	ClaimReason    string
	leaseCtx       context.Context
	stopRenewal    context.CancelFunc
	renewDone      <-chan struct{}
	renewLost      <-chan error
}

func (p *Pool) claimLoop() {
	defer close(p.claimDone)
	defer close(p.jobs)
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	p.claimAndDispatch()
	for {
		select {
		case <-p.claimCtx.Done():
			return
		case <-ticker.C:
			p.claimAndDispatch()
		}
	}
}

func (p *Pool) heartbeatLoop() {
	defer p.heartbeatWG.Done()
	p.heartbeat()
	interval := p.leaseTimeout / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.runCtx.Done():
			return
		case <-ticker.C:
			p.heartbeat()
		}
	}
}

func (p *Pool) heartbeat() {
	ctx, cancel := context.WithTimeout(p.runCtx, 5*time.Second)
	defer cancel()
	if err := p.q.Heartbeat(ctx, p.workerID); err != nil && p.runCtx.Err() == nil {
		p.logger.Warn("heartbeat failed", "error", err)
	}
}

func (p *Pool) claimAndDispatch() {
	available := cap(p.slots) - len(p.slots)
	if available <= 0 {
		return
	}
	if available > p.batchSize {
		available = p.batchSize
	}
	ctx, cancel := context.WithTimeout(p.claimCtx, 10*time.Second)
	defer cancel()
	claimStart := time.Now()
	tasks, err := p.q.ClaimTasks(ctx, p.workerID, available, p.leaseTimeout)
	if err != nil {
		if p.claimCtx.Err() == nil {
			p.logger.Error("claim tasks failed", "error", err, "worker_id", p.workerID)
		}
		return
	}
	reclaims := 0
	for i := range tasks {
		if tasks[i].ClaimReason == "expired_lease" {
			reclaims++
		}
	}
	observability.Runtime().RecordClaim(len(tasks), reclaims, time.Since(claimStart))
	for _, task := range tasks {
		p.slots <- struct{}{}
		job := taskJob{ID: task.ID, Type: string(task.Type), Queue: task.Queue,
			Payload: task.Payload.Data, TimeoutSeconds: task.TimeoutSeconds,
			Attempt: task.Attempt, LeaseVersion: task.LeaseVersion, ClaimReason: task.ClaimReason}
		job = p.withLeaseRenewal(job)
		select {
		case <-p.runCtx.Done():
			job.stopRenewal()
			<-p.slots
			return
		case p.jobs <- job:
			observability.Runtime().SetLocalQueue(len(p.jobs), len(p.jobs))
		}
	}
}

// withLeaseRenewal starts renewal as soon as a task is claimed, including while
// it waits in the bounded local queue. This closes the claim-to-handler gap.
func (p *Pool) withLeaseRenewal(job taskJob) taskJob {
	leaseCtx, cancelLease := context.WithCancel(p.runCtx)
	renewCtx, stopRenewal := context.WithCancel(leaseCtx)
	done := make(chan struct{})
	lost := make(chan error, 1)
	job.leaseCtx, job.stopRenewal, job.renewDone, job.renewLost = leaseCtx, stopRenewal, done, lost
	p.renewWG.Add(1)
	go func() {
		defer p.renewWG.Done()
		defer close(done)
		interval := p.leaseTimeout / 3
		if interval < 10*time.Millisecond {
			interval = 10 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				opCtx, cancel := context.WithTimeout(renewCtx, minDuration(5*time.Second, interval))
				err := p.q.RenewLease(opCtx, job.ID, p.workerID, job.LeaseVersion, p.leaseTimeout)
				cancel()
				observability.Runtime().LeaseRenewed(err == nil)
				if err != nil {
					// The handler completion path deliberately stops renewal before
					// writing its result. If that cancellation races an in-flight SQL
					// call, it is local lifecycle completion—not ownership loss.
					if renewCtx.Err() != nil {
						return
					}
					if errors.Is(err, domain.ErrTaskNotClaimed) {
						observability.Runtime().FencingRejected()
					}
					select {
					case lost <- err:
					default:
					}
					cancelLease()
					return
				}
			}
		}
	}()
	return job
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (p *Pool) executeLoop(workerNum int) {
	defer p.executorWG.Done()
	for {
		select {
		case <-p.runCtx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.executeTask(job, workerNum)
		}
	}
}

func (p *Pool) executeTask(job taskJob, workerNum int) {
	defer func() { <-p.slots; observability.Runtime().SetLocalQueue(len(p.jobs), len(p.jobs)) }()
	defer job.stopRenewal()
	p.logger.Info("executing task", "task_id", job.ID, "task_type", job.Type,
		"queue", job.Queue, "attempt", job.Attempt, "lease_version", job.LeaseVersion,
		"worker_id", p.workerID, "goroutine", workerNum)

	taskTimeout := time.Duration(job.TimeoutSeconds) * time.Second
	if taskTimeout <= 0 {
		taskTimeout = p.leaseTimeout
	}
	execCtx, cancelExec := context.WithTimeout(job.leaseCtx, taskTimeout)
	carrier := propagation.MapCarrier{}
	if traceparent, ok := job.Payload["_traceparent"].(string); ok {
		carrier.Set("traceparent", traceparent)
	}
	if tracestate, ok := job.Payload["_tracestate"].(string); ok {
		carrier.Set("tracestate", tracestate)
	}
	execCtx = otel.GetTextMapPropagator().Extract(execCtx, carrier)
	handlerPayload := make(map[string]any, len(job.Payload))
	for key, value := range job.Payload {
		if key != "_traceparent" && key != "_tracestate" {
			handlerPayload[key] = value
		}
	}
	execCtx, workerSpan := otel.Tracer("distributed-task-lab/worker").Start(
		execCtx,
		"task.execute "+job.Type,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("task.id", job.ID),
			attribute.String("task.type", job.Type),
			attribute.String("task.queue", job.Queue),
			attribute.Int("task.attempt", job.Attempt),
			attribute.Int("task.lease_version", job.LeaseVersion),
			attribute.String("task.claim_reason", job.ClaimReason),
			attribute.String("worker.id", p.workerID),
		),
	)
	defer workerSpan.End()
	handlerStart := time.Now()
	observability.Runtime().HandlerStarted()
	result, execErr := p.handler.Handle(execCtx, job.Type, handlerPayload, job.Attempt)
	observability.Runtime().HandlerFinished(time.Since(handlerStart), execErr != nil)
	cancelExec()
	job.stopRenewal()
	<-job.renewDone
	select {
	case renewErr := <-job.renewLost:
		p.logger.Warn("task lease ownership lost; result discarded", "task_id", job.ID,
			"lease_version", job.LeaseVersion, "error", renewErr)
		return
	default:
	}

	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	if execErr != nil {
		workerSpan.RecordError(execErr)
		workerSpan.SetStatus(codes.Error, "handler failed")
		errType, retryable := domain.ErrorTypeExecution, true
		var classified *ExecutionError
		if errors.As(execErr, &classified) {
			if classified.ErrorType != "" {
				errType = classified.ErrorType
			}
			retryable = classified.Retryable
		}
		if errors.Is(execErr, context.DeadlineExceeded) {
			errType, retryable = domain.ErrorTypeTimeout, true
		}
		failure := domain.TaskFailure{ErrorType: errType, Message: execErr.Error(), Retryable: retryable,
			OccurredAt: time.Now().UTC().Format(time.RFC3339)}
		if err := p.q.MarkFailure(writeCtx, job.ID, p.workerID, job.LeaseVersion, failure); err != nil {
			p.logger.Error("mark failure failed", "task_id", job.ID, "error", err)
		}
		return
	}

	domResult := domain.TaskResult{Summary: "completed"}
	if result != nil {
		domResult = domain.TaskResult{Summary: result.Summary, Artifacts: result.Artifacts, Metrics: result.Metrics}
	}
	if err := p.q.MarkSuccess(writeCtx, job.ID, p.workerID, job.LeaseVersion, domResult); err != nil {
		workerSpan.RecordError(err)
		workerSpan.SetStatus(codes.Error, "completion write failed")
		if errors.Is(err, domain.ErrTaskNotClaimed) {
			observability.Runtime().FencingRejected()
		}
		p.logger.Error("mark success failed", "task_id", job.ID, "error", err)
		return
	}
	p.logger.Info("task completed successfully", "task_id", job.ID, "worker_id", p.workerID)
}
