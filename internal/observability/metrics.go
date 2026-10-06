package observability

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// RuntimeMetrics is a dependency-free process-local metrics core. It keeps the
// execution kernel exporter-neutral while exposing Prometheus-compatible data.
type RuntimeMetrics struct {
	claims              atomic.Int64
	reclaims            atomic.Int64
	claimLatencyNS      atomic.Int64
	claimLatencyCount   atomic.Int64
	activeHandlers      atomic.Int64
	claimedNotStarted   atomic.Int64
	jobsDepth           atomic.Int64
	handlerTotal        atomic.Int64
	handlerFailures     atomic.Int64
	handlerLatencyNS    atomic.Int64
	handlerLatencyCount atomic.Int64
	renewTotal          atomic.Int64
	renewFailures       atomic.Int64
	fencingRejections   atomic.Int64
	effectUnknown       atomic.Int64
}

var runtimeMetrics RuntimeMetrics

func Runtime() *RuntimeMetrics { return &runtimeMetrics }

func (m *RuntimeMetrics) RecordClaim(count, reclaims int, latency time.Duration) {
	m.claims.Add(int64(count))
	m.reclaims.Add(int64(reclaims))
	m.claimLatencyNS.Add(latency.Nanoseconds())
	m.claimLatencyCount.Add(1)
}
func (m *RuntimeMetrics) SetLocalQueue(claimedNotStarted, jobsDepth int) {
	m.claimedNotStarted.Store(int64(claimedNotStarted))
	m.jobsDepth.Store(int64(jobsDepth))
}
func (m *RuntimeMetrics) HandlerStarted() { m.activeHandlers.Add(1); m.handlerTotal.Add(1) }
func (m *RuntimeMetrics) HandlerFinished(d time.Duration, failed bool) {
	m.activeHandlers.Add(-1)
	m.handlerLatencyNS.Add(d.Nanoseconds())
	m.handlerLatencyCount.Add(1)
	if failed {
		m.handlerFailures.Add(1)
	}
}
func (m *RuntimeMetrics) LeaseRenewed(ok bool) {
	m.renewTotal.Add(1)
	if !ok {
		m.renewFailures.Add(1)
	}
}
func (m *RuntimeMetrics) FencingRejected() { m.fencingRejections.Add(1) }
func (m *RuntimeMetrics) EffectUnknown()   { m.effectUnknown.Add(1) }

func (m *RuntimeMetrics) WritePrometheus(w io.Writer) {
	values := []struct {
		name, help, kind string
		value            int64
	}{
		{"tasklab_claim_total", "Tasks claimed by this process.", "counter", m.claims.Load()},
		{"tasklab_reclaim_total", "Expired task generations reclaimed by this process.", "counter", m.reclaims.Load()},
		{"tasklab_active_handlers", "Currently executing handlers.", "gauge", m.activeHandlers.Load()},
		{"tasklab_claimed_not_started", "Leased tasks waiting for a local executor.", "gauge", m.claimedNotStarted.Load()},
		{"tasklab_jobs_channel_depth", "Current bounded jobs channel depth.", "gauge", m.jobsDepth.Load()},
		{"tasklab_handler_total", "Handlers started by this process.", "counter", m.handlerTotal.Load()},
		{"tasklab_handler_failure_total", "Handlers that returned an error.", "counter", m.handlerFailures.Load()},
		{"tasklab_lease_renew_total", "Lease renewal attempts.", "counter", m.renewTotal.Load()},
		{"tasklab_lease_renew_failure_total", "Failed lease renewals.", "counter", m.renewFailures.Load()},
		{"tasklab_fencing_rejection_total", "Stale ownership writes or renewals rejected.", "counter", m.fencingRejections.Load()},
		{"tasklab_effect_unknown_total", "External effects with uncertain provider outcome.", "counter", m.effectUnknown.Load()},
	}
	for _, v := range values {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n", v.name, v.help, v.name, v.kind, v.name, v.value)
	}
	claimCount := m.claimLatencyCount.Load()
	if claimCount > 0 {
		fmt.Fprint(w, "# TYPE tasklab_claim_latency_seconds histogram\n")
		fmt.Fprintf(w, "tasklab_claim_latency_seconds_sum %f\ntasklab_claim_latency_seconds_count %d\n", float64(m.claimLatencyNS.Load())/float64(time.Second), claimCount)
	}
	handlerCount := m.handlerLatencyCount.Load()
	if handlerCount > 0 {
		fmt.Fprint(w, "# TYPE tasklab_handler_latency_seconds histogram\n")
		fmt.Fprintf(w, "tasklab_handler_latency_seconds_sum %f\ntasklab_handler_latency_seconds_count %d\n", float64(m.handlerLatencyNS.Load())/float64(time.Second), handlerCount)
	}
}
