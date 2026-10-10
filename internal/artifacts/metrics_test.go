package artifacts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/observability"
)

type metricsTestStore struct{ err error }

func (s metricsTestStore) PutIfAbsent(context.Context, string, []byte, string) (ObjectInfo, error) {
	return ObjectInfo{}, s.err
}
func (s metricsTestStore) Get(context.Context, string) ([]byte, ObjectInfo, error) {
	return nil, ObjectInfo{}, s.err
}
func (s metricsTestStore) Head(context.Context, string) (ObjectInfo, error) {
	return ObjectInfo{}, s.err
}
func (s metricsTestStore) Delete(context.Context, string) error { return s.err }

func TestMetricsStoreCountsOnlyUnexpectedErrorsWithoutLabels(t *testing.T) {
	var metrics observability.RuntimeMetrics
	ctx := context.Background()
	_, _, err := WithMetrics(metricsTestStore{err: ErrNotFound}, &metrics).Get(ctx, "sensitive/object-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error=%v, want ErrNotFound", err)
	}
	_, _, err = WithMetrics(metricsTestStore{err: errors.New("credential=secret")}, &metrics).Get(ctx, "sensitive/object-key")
	if err == nil {
		t.Fatal("Get unexpectedly succeeded")
	}
	var out strings.Builder
	metrics.WritePrometheus(&out)
	if !strings.Contains(out.String(), "tasklab_artifact_error_total 1") {
		t.Fatalf("unexpected artifact failure not counted: %s", out.String())
	}
	for _, secret := range []string{"sensitive/object-key", "credential=secret"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("metric leaked %q", secret)
		}
	}
}
