package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/lib/pq"
)

const sharedIngressBucketCapacity = 10000

// SharedIngressLimiter is a disabled-by-default candidate for a shared API
// token bucket. Principal identifiers are stored only as keyed HMAC digests.
type SharedIngressLimiter struct {
	db     *sql.DB
	key    []byte
	rate   float64
	burst  float64
	initMu sync.Mutex
	ready  bool
}

func NewSharedIngressLimiter(db *sql.DB, hashKey []byte, rate float64, burst int) (*SharedIngressLimiter, error) {
	if db == nil || len(hashKey) < 32 || rate <= 0 || burst < 1 {
		return nil, fmt.Errorf("shared ingress limiter requires database, 32-byte hash key, positive rate, and positive burst")
	}
	return &SharedIngressLimiter{db: db, key: append([]byte(nil), hashKey...), rate: rate, burst: float64(burst)}, nil
}

// Allow uses PostgreSQL statement time as the clock authority. The slot table
// bounds cardinality; an occupied bucket can be evicted only after its tokens
// would have fully refilled under the active policy.
func (l *SharedIngressLimiter) Allow(ctx context.Context, subject string) (bool, int, error) {
	if subject == "" {
		return true, 0, nil
	}
	if err := l.initialize(ctx); err != nil {
		return false, 0, err
	}
	digest := l.digest(subject)
	for attempt := 0; attempt < 3; attempt++ {
		tx, err := l.db.BeginTx(ctx, nil)
		if err != nil {
			return false, 0, fmt.Errorf("begin shared ingress admission: %w", err)
		}
		tokens, admitted, err := l.updateExisting(ctx, tx, digest)
		if err == nil {
			if err := tx.Commit(); err != nil {
				return false, 0, fmt.Errorf("commit shared ingress admission: %w", err)
			}
			return admitted, l.retryAfter(tokens), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			return false, 0, err
		}
		tokens, err = l.claimOrReclaimSlot(ctx, tx, digest)
		if err != nil {
			_ = tx.Rollback()
			var postgresErr *pq.Error
			if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
				// Another replica claimed the same new subject after our first
				// lookup. Retry through the row-locked existing-bucket path.
				continue
			}
			if errors.Is(err, sql.ErrNoRows) {
				return false, 1, nil // all bounded slots are still partially depleted
			}
			return false, 0, err
		}
		if err := tx.Commit(); err != nil {
			return false, 0, fmt.Errorf("commit shared ingress bucket claim: %w", err)
		}
		return true, l.retryAfter(tokens), nil
	}
	return false, 1, nil
}

func (l *SharedIngressLimiter) initialize(ctx context.Context) error {
	l.initMu.Lock()
	defer l.initMu.Unlock()
	if l.ready {
		return nil
	}
	if _, err := l.db.ExecContext(ctx, `
		INSERT INTO api_ingress_rate_limit_buckets (slot)
		SELECT generate_series(1, $1)
		ON CONFLICT (slot) DO NOTHING
	`, sharedIngressBucketCapacity); err != nil {
		return fmt.Errorf("initialize shared ingress bucket slots: %w", err)
	}
	l.ready = true
	return nil
}

func (l *SharedIngressLimiter) updateExisting(ctx context.Context, tx *sql.Tx, digest []byte) (float64, bool, error) {
	var tokens float64
	var admitted bool
	err := tx.QueryRowContext(ctx, `
		UPDATE api_ingress_rate_limit_buckets
		SET tokens = CASE
			WHEN LEAST($2, tokens + GREATEST(0, EXTRACT(EPOCH FROM (statement_timestamp() - updated_at))) * $3) >= 1
			THEN LEAST($2, tokens + GREATEST(0, EXTRACT(EPOCH FROM (statement_timestamp() - updated_at))) * $3) - 1
			ELSE LEAST($2, tokens + GREATEST(0, EXTRACT(EPOCH FROM (statement_timestamp() - updated_at))) * $3)
		END,
		last_admitted = LEAST($2, tokens + GREATEST(0, EXTRACT(EPOCH FROM (statement_timestamp() - updated_at))) * $3) >= 1,
		updated_at = statement_timestamp()
		WHERE principal_hash = $1
		RETURNING tokens, last_admitted
	`, digest, l.burst, l.rate).Scan(&tokens, &admitted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("update shared ingress bucket: %w", err)
	}
	return tokens, admitted, err
}

func (l *SharedIngressLimiter) claimOrReclaimSlot(ctx context.Context, tx *sql.Tx, digest []byte) (float64, error) {
	var tokens float64
	err := tx.QueryRowContext(ctx, `
		UPDATE api_ingress_rate_limit_buckets
		SET principal_hash = $1, tokens = $2 - 1, updated_at = statement_timestamp(), last_admitted = TRUE
		WHERE slot = (
			SELECT slot FROM api_ingress_rate_limit_buckets
			WHERE principal_hash IS NULL OR
				LEAST($2, tokens + GREATEST(0, EXTRACT(EPOCH FROM (statement_timestamp() - updated_at))) * $3) >= $2
			ORDER BY (principal_hash IS NOT NULL), updated_at ASC, slot ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		) AND NOT EXISTS (
			SELECT 1 FROM api_ingress_rate_limit_buckets WHERE principal_hash = $1
		)
		RETURNING tokens
	`, digest, l.burst, l.rate).Scan(&tokens)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("claim shared ingress bucket slot: %w", err)
	}
	return tokens, err
}

func (l *SharedIngressLimiter) digest(subject string) []byte {
	mac := hmac.New(sha256.New, l.key)
	_, _ = mac.Write([]byte(subject))
	return mac.Sum(nil)
}

func (l *SharedIngressLimiter) retryAfter(tokens float64) int {
	if tokens >= 1 {
		return 0
	}
	return max(1, int(math.Ceil((1-tokens)/l.rate)))
}
