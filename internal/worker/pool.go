package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

// Pool manages a fixed-size goroutine pool for task execution.
type Pool struct {
	q            queue.Queue
	workerID     string
	concurrency  int
	pollInterval time.Duration
	leaseTimeout time.Duration
	batchSize    int
	logger       *slog.Logger
	handler      Handler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	jobs   chan taskJob

	limitersMu sync.Mutex
	limiters   map[string]*rate.Limiter
}

// Handler defines the interface for task-type-specific execution logic.
// attempt is the current attempt number (1-based, as tracked by the queue).
type Handler interface {
	Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (result *TaskExecResult, err error)
}

// TaskExecResult is the outcome of executing a task.
type TaskExecResult struct {
	Summary   string
	Artifacts map[string]string
	Metrics   map[string]float64
}

// NewPool creates a new worker pool.
func NewPool(q queue.Queue, workerID string, concurrency int, pollInterval, leaseTimeout time.Duration, batchSize int, handler Handler, logger *slog.Logger) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		q:            q,
		workerID:     workerID,
		concurrency:  concurrency,
		pollInterval: pollInterval,
		leaseTimeout: leaseTimeout,
		batchSize:    batchSize,
		handler:      handler,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		jobs:         make(chan taskJob, 100),
	}
}

// Start begins the worker pool's claim-execute loop.
func (p *Pool) Start() {
	p.logger.Info("worker pool starting",
		"worker_id", p.workerID,
		"concurrency", p.concurrency,
		"poll_interval", p.pollInterval,
		"lease_timeout", p.leaseTimeout,
	)

	// Launch heartbeat goroutine.
	p.wg.Add(1)
	go p.heartbeatLoop()

	// Launch the claim loop.
	p.wg.Add(1)
	go p.claimLoop()

	// Launch task executors.
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.executeLoop(i)
	}
}

// Stop gracefully shuts down the worker pool.
func (p *Pool) Stop() {
	p.logger.Info("worker pool stopping", "worker_id", p.workerID)
	p.cancel()
	p.wg.Wait()
	p.logger.Info("worker pool stopped", "worker_id", p.workerID)
}

// taskJob represents a claimed task ready for execution.
type taskJob struct {
	ID             string
	Type           string
	Queue          string
	Payload        map[string]any
	TimeoutSeconds int
	Attempt        int
	LeaseVersion   int
}

// claimLoop periodically claims tasks from the queue.
func (p *Pool) claimLoop() {
	defer p.wg.Done()
	defer close(p.jobs)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	// Claim immediately on start.
	p.claimAndDispatch()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.claimAndDispatch()
		}
	}
}

// heartbeatLoop periodically sends heartbeats to register worker liveness.
func (p *Pool) heartbeatLoop() {
	defer p.wg.Done()

	// Send initial heartbeat immediately.
	if err := p.q.Heartbeat(p.ctx, p.workerID); err != nil {
		p.logger.Warn("heartbeat failed", "error", err)
	}

	ticker := time.NewTicker(p.leaseTimeout / 3) // heartbeat every 1/3 of lease
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if err := p.q.Heartbeat(p.ctx, p.workerID); err != nil {
				p.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

func (p *Pool) claimAndDispatch() {
	p.refreshLimiters()

	tasks, err := p.q.ClaimTasks(p.ctx, p.workerID, p.batchSize, p.leaseTimeout)
	if err != nil {
		p.logger.Error("claim tasks failed", "error", err, "worker_id", p.workerID)
		return
	}

	if len(tasks) == 0 {
		return
	}

	p.logger.Info("claimed tasks", "count", len(tasks), "worker_id", p.workerID)

	for _, task := range tasks {
		job := taskJob{
			ID:             task.ID,
			Type:           string(task.Type),
			Queue:          task.Queue,
			Payload:        task.Payload.Data,
			TimeoutSeconds: task.TimeoutSeconds,
			Attempt:        task.Attempt,
			LeaseVersion:   task.LeaseVersion,
		}

		// Enforce the queue's rate limit before dispatching. Note: for very
		// large backlogs this dispatch-time throttle should be paired with
		// lease renewal; demo volumes are modest so the lease comfortably
		// outlives the spacing between dispatches.
		if err := p.throttle(p.ctx, task.Queue); err != nil {
			return
		}

		select {
		case <-p.ctx.Done():
			return
		case p.jobs <- job:
		}
	}
}

// refreshLimiters reloads per-queue token buckets from the queue governance config.
func (p *Pool) refreshLimiters() {
	queues, err := p.q.ListQueues(p.ctx)
	if err != nil {
		p.logger.Warn("list queues failed (rate limiting skipped this cycle)", "error", err)
		return
	}

	p.limitersMu.Lock()
	defer p.limitersMu.Unlock()
	if p.limiters == nil {
		p.limiters = make(map[string]*rate.Limiter)
	}
	for _, qc := range queues {
		if qc.RateLimitPerSecond > 0 {
			burst := qc.Burst
			if burst <= 0 {
				burst = 1
			}
			p.limiters[qc.Name] = rate.NewLimiter(rate.Limit(qc.RateLimitPerSecond), burst)
		} else {
			delete(p.limiters, qc.Name)
		}
	}
}

// throttle waits until the given queue's token bucket admits one task.
func (p *Pool) throttle(ctx context.Context, queueName string) error {
	p.limitersMu.Lock()
	limiter := p.limiters[queueName]
	p.limitersMu.Unlock()
	if limiter == nil {
		return nil
	}
	return limiter.Wait(ctx)
}

// executeLoop processes claimed tasks.
func (p *Pool) executeLoop(workerNum int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
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
	p.logger.Info("executing task",
		"task_id", job.ID,
		"task_type", job.Type,
		"worker_id", p.workerID,
		"goroutine", workerNum,
		"timeout_seconds", job.TimeoutSeconds,
	)

	// Determine task timeout: use task.TimeoutSeconds if set, else fall back to lease-based bound.
	taskTimeout := time.Duration(job.TimeoutSeconds) * time.Second
	if taskTimeout <= 0 || taskTimeout > p.leaseTimeout {
		taskTimeout = p.leaseTimeout - time.Second
	}

	ctx, cancel := context.WithTimeout(p.ctx, taskTimeout)
	defer cancel()

	// writeCtx is decoupled from the execution deadline so a task that times
	// out can still persist its failure/result. It has its own short timeout
	// and is only cancelled when the pool itself shuts down.
	writeCtx, writeCancel := context.WithTimeout(context.WithoutCancel(p.ctx), 5*time.Second)
	defer writeCancel()

	// Execute the handler.
	result, execErr := p.handler.Handle(ctx, job.Type, job.Payload, job.Attempt)

	if execErr != nil {
		errType := domain.ErrorTypeExecution
		retryable := true
		var execErr2 *ExecutionError
		if errors.As(execErr, &execErr2) {
			if execErr2.ErrorType != "" {
				errType = execErr2.ErrorType
			}
			retryable = execErr2.Retryable
		}
		if ctx.Err() == context.DeadlineExceeded {
			errType = domain.ErrorTypeTimeout
			retryable = true
		}
		p.logger.Warn("task execution failed",
			"task_id", job.ID,
			"error", execErr,
			"error_type", errType,
			"retryable", retryable,
			"worker_id", p.workerID,
		)
		failure := domain.TaskFailure{
			ErrorType:  errType,
			Message:    execErr.Error(),
			Retryable:  retryable,
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.q.MarkFailure(writeCtx, job.ID, p.workerID, job.LeaseVersion, failure); err != nil {
			p.logger.Error("mark failure failed", "task_id", job.ID, "error", err)
		}
		return
	}

	// Task succeeded.
	domResult := domain.TaskResult{Summary: "completed"}
	if result != nil {
		domResult = domain.TaskResult{
			Summary:   result.Summary,
			Artifacts: result.Artifacts,
			Metrics:   result.Metrics,
		}
	}
	if err := p.q.MarkSuccess(writeCtx, job.ID, p.workerID, job.LeaseVersion, domResult); err != nil {
		p.logger.Error("mark success failed", "task_id", job.ID, "error", err)
		return
	}

	p.logger.Info("task completed successfully",
		"task_id", job.ID,
		"worker_id", p.workerID,
	)
}
