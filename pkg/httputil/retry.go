// Package httputil provides shared HTTP utilities for API clients.
package httputil

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// HTTPDoer is the interface satisfied by *http.Client; use it in API client
// structs so tests can inject a mock without a real network.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// RateLimiter configures exponential backoff with jitter for rate-limited APIs.
type RateLimiter struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
}

// Default returns a RateLimiter with the standard configuration used across
// all API clients (2 s base, 30 s max, 5 retries).
func Default() *RateLimiter {
	return &RateLimiter{
		BaseDelay:  2 * time.Second,
		MaxDelay:   30 * time.Second,
		MaxRetries: 5,
	}
}

// Backoff returns the delay before the next retry attempt.
// If the Retry-After response header is a valid integer, that value is used
// directly. Otherwise exponential backoff with ±30 % jitter is applied,
// capped at MaxDelay.
func (r *RateLimiter) Backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	delay := float64(r.BaseDelay) * math.Pow(2, float64(attempt))
	delay *= 0.7 + rand.Float64()*0.6 // jitter: 0.7–1.3×
	if delay > float64(r.MaxDelay) {
		delay = float64(r.MaxDelay)
	}
	return time.Duration(delay)
}
