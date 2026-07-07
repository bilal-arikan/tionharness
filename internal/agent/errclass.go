package agent

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"time"
)

// errClass is the coarse taxonomy of provider-call failures. It exists so the
// recovery loop can pick the correct action per class (bounded retry, compact,
// terminate) instead of treating every error as terminal. Modeled after the
// classifier-driven failover pattern (external-context-agent error_classifier): a
// centralized, conservative classification replaces scattered string checks.
type errClass string

const (
	errRateLimit       errClass = "rate_limit"       // 429 / throttled — backoff then retry
	errOverloaded      errClass = "overloaded"       // 503/529 — provider overloaded, backoff then retry
	errServer          errClass = "server_error"     // 500/502/504 — transient upstream fault, retry
	errTimeout         errClass = "timeout"          // connection/read timeout — retry
	errContextOverflow errClass = "context_overflow" // prompt exceeds the window — compact, never retry as-is
	errAuth            errClass = "auth"             // 401/403 — terminal (key problem, retry reproduces it)
	errBilling         errClass = "billing"          // 402 / credit exhaustion — terminal
	errCancelled       errClass = "cancelled"        // caller context cancelled — terminal
	errUnknown         errClass = "unknown"          // unclassified — terminal (never retried blindly)
)

// retryable reports whether a class is safe to retry with the SAME request:
// transient server-side or transport faults. Everything else either needs a
// different request (context_overflow → compaction) or is deterministic
// (auth/billing/unknown), where a retry would only burn budget.
func (c errClass) retryable() bool {
	switch c {
	case errRateLimit, errOverloaded, errServer, errTimeout:
		return true
	default:
		return false
	}
}

// classifyProviderError maps a provider-call error onto the taxonomy. Matching
// is deliberately conservative — provider errors are plain wrapped strings
// (e.g. "anthropic HTTP 429: …"), so only unambiguous markers classify; any
// doubt lands in errUnknown, which is terminal, never silently retried.
func classifyProviderError(err error) errClass {
	if err == nil {
		return errUnknown
	}
	if errors.Is(err, context.Canceled) {
		return errCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errTimeout
	}
	msg := strings.ToLower(err.Error())
	switch {
	case isContextOverflow(err):
		return errContextOverflow
	case strings.Contains(msg, "http 429"), strings.Contains(msg, "rate limit"), strings.Contains(msg, "rate_limit"):
		return errRateLimit
	case strings.Contains(msg, "http 529"), strings.Contains(msg, "http 503"), strings.Contains(msg, "overloaded"):
		return errOverloaded
	case strings.Contains(msg, "http 500"), strings.Contains(msg, "http 502"), strings.Contains(msg, "http 504"),
		strings.Contains(msg, "internal server error"):
		return errServer
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "timed out"), strings.Contains(msg, "deadline exceeded"):
		return errTimeout
	case strings.Contains(msg, "http 401"), strings.Contains(msg, "http 403"),
		strings.Contains(msg, "authentication_error"), strings.Contains(msg, "invalid x-api-key"):
		return errAuth
	case strings.Contains(msg, "http 402"), strings.Contains(msg, "credit balance"),
		strings.Contains(msg, "insufficient credit"), strings.Contains(msg, "quota exceeded"):
		return errBilling
	default:
		return errUnknown
	}
}

// retryBackoff returns the wait before provider-retry number attempt (0-based):
// exponential from 1s with ±25% jitter, capped at 30s. Jitter avoids thundering
// re-requests when several sessions hit the same 429/529 window.
func retryBackoff(attempt int) time.Duration {
	base := time.Second << uint(min(attempt, 5)) // 1s, 2s, 4s, 8s, 16s, 32s→cap
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := time.Duration(rand.Int63n(int64(base) / 2)) // [0, base/2)
	return base*3/4 + jitter                              // base ± 25%
}

// sleepCtx waits d unless ctx is cancelled first; reports whether the full wait
// completed. Used for retry backoff so a user stop is never delayed by a sleep.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
