package httputil

import (
	"math"
	"testing"
	"time"
)

func TestBackoff_RetryAfterHeader(t *testing.T) {
	r := &RateLimiter{BaseDelay: 2 * time.Second, MaxDelay: 30 * time.Second, MaxRetries: 5}
	got := r.Backoff(0, "10")
	if got != 10*time.Second {
		t.Errorf("expected 10s from Retry-After header, got %v", got)
	}
}

func TestBackoff_RetryAfterInvalid(t *testing.T) {
	r := Default()
	// Falls back to exponential when Retry-After is not a plain integer.
	got := r.Backoff(0, "not-a-number")
	if got <= 0 || got > r.MaxDelay {
		t.Errorf("expected positive delay <= MaxDelay, got %v", got)
	}
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	r := &RateLimiter{BaseDelay: 1 * time.Second, MaxDelay: 60 * time.Second, MaxRetries: 5}
	// With ±30% jitter the actual value sits in [0.7*base*2^n, 1.3*base*2^n].
	for attempt := 0; attempt <= 3; attempt++ {
		mid := float64(r.BaseDelay) * math.Pow(2, float64(attempt))
		lo := time.Duration(mid * 0.7)
		hi := time.Duration(mid * 1.3)
		// Sample several times to guard against lucky jitter values.
		for i := 0; i < 50; i++ {
			d := r.Backoff(attempt, "")
			if d < lo || d > hi {
				t.Errorf("attempt %d: got %v, want [%v, %v]", attempt, d, lo, hi)
				break
			}
		}
	}
}

func TestBackoff_CappedAtMaxDelay(t *testing.T) {
	r := &RateLimiter{BaseDelay: 2 * time.Second, MaxDelay: 5 * time.Second, MaxRetries: 10}
	for i := 0; i < 20; i++ {
		if d := r.Backoff(10, ""); d > r.MaxDelay {
			t.Errorf("delay %v exceeds MaxDelay %v", d, r.MaxDelay)
		}
	}
}

func TestDefault(t *testing.T) {
	r := Default()
	if r.BaseDelay != 2*time.Second || r.MaxDelay != 30*time.Second || r.MaxRetries != 5 {
		t.Errorf("unexpected Default values: %+v", r)
	}
}
