package artifacts

import (
	"context"
	"errors"

	"github.com/fantasticyi/distributed-task-lab/internal/observability"
)

// WithMetrics counts unexpected object-store failures without adding object
// keys, payloads, or error text as labels. Expected misses/conflicts and caller
// cancellation are excluded from the backend-error counter.
func WithMetrics(store Store, metrics *observability.RuntimeMetrics) Store {
	if store == nil || metrics == nil {
		return store
	}
	return &metricsStore{Store: store, metrics: metrics}
}

type metricsStore struct {
	Store
	metrics *observability.RuntimeMetrics
}

func (s *metricsStore) record(err error) {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidKey) || errors.Is(err, ErrTooLarge) || errors.Is(err, context.Canceled) {
		return
	}
	s.metrics.ArtifactError()
}

func (s *metricsStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	info, err := s.Store.PutIfAbsent(ctx, key, data, contentType)
	s.record(err)
	return info, err
}

func (s *metricsStore) Get(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
	data, info, err := s.Store.Get(ctx, key)
	s.record(err)
	return data, info, err
}

func (s *metricsStore) Head(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.Store.Head(ctx, key)
	s.record(err)
	return info, err
}

func (s *metricsStore) Delete(ctx context.Context, key string) error {
	err := s.Store.Delete(ctx, key)
	s.record(err)
	return err
}
