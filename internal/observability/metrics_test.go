package observability

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRuntimeMetricsPrometheusTypesAndCompletedLatency(t *testing.T) {
	var metrics RuntimeMetrics
	metrics.RecordClaim(2, 1, 4*time.Millisecond)
	metrics.HandlerStarted()
	metrics.HandlerFinished(7*time.Millisecond, true)
	metrics.LeaseRenewed(false)
	metrics.EffectUnknown()
	metrics.TaskRetried()
	metrics.ArtifactError()
	metrics.APIResponse(503)
	var out bytes.Buffer
	metrics.WritePrometheus(&out)
	text := out.String()
	for _, want := range []string{
		"# TYPE tasklab_claim_total counter", "# TYPE tasklab_active_handlers gauge",
		"# TYPE tasklab_claim_latency_seconds histogram", "# TYPE tasklab_handler_latency_seconds histogram",
		"tasklab_handler_latency_seconds_count 1", "tasklab_effect_unknown_total 1",
		"tasklab_retry_total 1", "tasklab_artifact_error_total 1",
		"tasklab_api_requests_total{status_class=\"5xx\"} 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("scrape missing %q:\n%s", want, text)
		}
	}
}

func TestRuntimeMetricsDoesNotPublishUnfinishedHandlerLatency(t *testing.T) {
	var metrics RuntimeMetrics
	metrics.HandlerStarted()
	var out bytes.Buffer
	metrics.WritePrometheus(&out)
	if strings.Contains(out.String(), "tasklab_handler_latency_seconds_count") {
		t.Fatalf("unfinished handlers must not contribute a latency sample: %s", out.String())
	}
}
