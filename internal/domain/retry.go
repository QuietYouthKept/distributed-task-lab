package domain

import (
	"math/rand"
	"time"
)

// ExponentialBackoff returns the deterministic (jitter-free) backoff delay for a
// retry that occurs after the given 1-based attempt. The first retry (attempt==1)
// yields base; each subsequent retry multiplies by `multiplier` until `max` is
// reached. If max <= 0 there is no cap.
func ExponentialBackoff(base, max time.Duration, multiplier float64, attempt int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if multiplier <= 0 {
		multiplier = 2.0
	}
	if attempt < 1 {
		attempt = 1
	}

	delay := base
	for i := 1; i < attempt; i++ {
		next := time.Duration(float64(delay) * multiplier)
		if next <= delay { // overflow guard
			break
		}
		delay = next
		if max > 0 && delay >= max {
			return max
		}
	}
	if max > 0 && delay > max {
		return max
	}
	return delay
}

// FullJitter returns a random delay uniformly distributed in [0, delay]. It uses
// the package-level math/rand source, which is safe for concurrent use.
func FullJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(delay) + 1))
}
