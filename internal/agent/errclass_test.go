package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClassifyProviderError(t *testing.T) {
	tests := []struct {
		msg  string
		want errClass
	}{
		{"anthropic HTTP 429: rate limited", errRateLimit},
		{"minimax HTTP 429: {\"error\":\"too many requests\"}", errRateLimit},
		{"openrouter: rate limit exceeded for model", errRateLimit},
		{"anthropic HTTP 529: overloaded_error", errOverloaded},
		{"anthropic HTTP 503: service unavailable", errOverloaded},
		{"anthropic HTTP 500: internal server error", errServer},
		{"minimax HTTP 502: bad gateway", errServer},
		{"request timed out after 120s", errTimeout},
		{"context deadline exceeded", errTimeout},
		{"anthropic HTTP 401: authentication_error", errAuth},
		{"anthropic HTTP 403: forbidden", errAuth},
		{"anthropic HTTP 402: insufficient credit", errBilling},
		{"your credit balance is too low", errBilling},
		{"prompt is too long: 250000 tokens > 200000 maximum", errContextOverflow},
		{"context_length_exceeded", errContextOverflow},
		{"connection refused", errUnknown},
		{"invalid request: unknown field", errUnknown},
	}
	for _, tc := range tests {
		if got := classifyProviderError(errors.New(tc.msg)); got != tc.want {
			t.Errorf("classifyProviderError(%q) = %q, want %q", tc.msg, got, tc.want)
		}
	}
}

func TestClassifyProviderError_ContextSentinels(t *testing.T) {
	if got := classifyProviderError(context.Canceled); got != errCancelled {
		t.Errorf("context.Canceled = %q, want %q", got, errCancelled)
	}
	if got := classifyProviderError(context.DeadlineExceeded); got != errTimeout {
		t.Errorf("context.DeadlineExceeded = %q, want %q", got, errTimeout)
	}
	if got := classifyProviderError(nil); got != errUnknown {
		t.Errorf("nil = %q, want %q", got, errUnknown)
	}
}

func TestErrClassRetryable(t *testing.T) {
	retryable := []errClass{errRateLimit, errOverloaded, errServer, errTimeout}
	terminal := []errClass{errContextOverflow, errAuth, errBilling, errCancelled, errUnknown}
	for _, c := range retryable {
		if !c.retryable() {
			t.Errorf("%q.retryable() = false, want true", c)
		}
	}
	for _, c := range terminal {
		if c.retryable() {
			t.Errorf("%q.retryable() = true, want false", c)
		}
	}
}

func TestLimitErrorText(t *testing.T) {
	// Only usage/rate-limit, overload and billing classes get a friendly, retry-
	// oriented message; every other class must fall through to "" so the caller
	// keeps the raw provider string (a normal crash is never dressed up as a
	// recoverable limit).
	withText := []errClass{errRateLimit, errOverloaded, errBilling}
	for _, c := range withText {
		if limitErrorText(c) == "" {
			t.Errorf("limitErrorText(%q) = empty, want a message", c)
		}
	}
	withoutText := []errClass{errServer, errTimeout, errContextOverflow, errAuth, errCancelled, errUnknown}
	for _, c := range withoutText {
		if got := limitErrorText(c); got != "" {
			t.Errorf("limitErrorText(%q) = %q, want empty", c, got)
		}
	}
}

func TestRetryBackoff_BoundedAndGrowing(t *testing.T) {
	for attempt := 0; attempt < 8; attempt++ {
		d := retryBackoff(attempt)
		if d <= 0 {
			t.Errorf("retryBackoff(%d) = %v, want > 0", attempt, d)
		}
		if d > 40*time.Second {
			t.Errorf("retryBackoff(%d) = %v, want ≤ 40s (30s cap + jitter)", attempt, d)
		}
	}
}

func TestSleepCtx_CancelledReturnsFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, time.Minute) {
		t.Errorf("sleepCtx on a cancelled ctx = true, want false")
	}
	if !sleepCtx(context.Background(), 0) {
		t.Errorf("sleepCtx(0) = false, want true (no wait)")
	}
}

// Provider-retry decisions: transient faults retry within the budget, then
// terminate; deterministic classes never retry.
func TestDecideRecovery_ProviderRetry(t *testing.T) {
	cfg := recoveryConfig{maxTokenLimit: DefaultMaxTokenRetries, reactiveCompact: true, maxProviderRetries: 2}
	transient := errors.New("anthropic HTTP 529: overloaded_error")

	d := decideRecovery(nil, transient, loopState{}, cfg)
	if !d.cont || d.reason != contProviderRetry {
		t.Fatalf("first transient fault: cont=%v reason=%q, want retry", d.cont, d.reason)
	}
	if d.backoff <= 0 {
		t.Errorf("retry decision carries no backoff")
	}

	d = decideRecovery(nil, transient, loopState{providerRetries: 2}, cfg)
	if d.cont || d.term != termProviderErr {
		t.Errorf("budget exhausted: cont=%v term=%q, want terminal provider_error", d.cont, d.term)
	}

	// Budget 0 (disabled): terminal on the first fault.
	d = decideRecovery(nil, transient, loopState{}, recoveryConfig{maxProviderRetries: 0})
	if d.cont {
		t.Errorf("retry disabled: cont=true, want terminal")
	}

	// Deterministic auth error: never retried even with budget.
	d = decideRecovery(nil, errors.New("anthropic HTTP 401: authentication_error"), loopState{}, cfg)
	if d.cont || d.term != termProviderErr {
		t.Errorf("auth error: cont=%v term=%q, want terminal", d.cont, d.term)
	}

	// User cancellation: terminal with the cancelled tag, never retried.
	d = decideRecovery(nil, context.Canceled, loopState{}, cfg)
	if d.cont || d.term != termCancelled {
		t.Errorf("cancelled: cont=%v term=%q, want %q", d.cont, d.term, termCancelled)
	}

	// A rate-limited call still prefers compaction when the error is overflow
	// (overflow is checked first and is not retryable).
	d = decideRecovery(nil, errors.New("prompt is too long: 300000 tokens > 200000 maximum"), loopState{}, cfg)
	if !d.compact {
		t.Errorf("overflow with retry budget: compact=false, want compact path")
	}
}

// The server's Retry-After hint (threaded through the error text by the
// provider) overrides the computed backoff, capped at maxRetryAfterWait.
func TestDecideRecovery_RetryAfterHint(t *testing.T) {
	cfg := recoveryConfig{maxProviderRetries: 2}

	d := decideRecovery(nil, errors.New("anthropic HTTP 429: rate limited (retry-after: 7s)"), loopState{}, cfg)
	if !d.cont || d.backoff != 7*time.Second {
		t.Errorf("backoff = %v, want the server's 7s hint", d.backoff)
	}

	d = decideRecovery(nil, errors.New("anthropic HTTP 429: rate limited (retry-after: 3600s)"), loopState{}, cfg)
	if d.backoff != maxRetryAfterWait {
		t.Errorf("backoff = %v, want capped at %v", d.backoff, maxRetryAfterWait)
	}

	// No hint: the computed jittered backoff applies (nonzero).
	d = decideRecovery(nil, errors.New("anthropic HTTP 529: overloaded_error"), loopState{}, cfg)
	if d.backoff <= 0 {
		t.Errorf("no hint: computed backoff missing")
	}
}
