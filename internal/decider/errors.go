package decider

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrDisabled: the decider is switched off in settings.
	ErrDisabled = errors.New("decider is disabled")
	// ErrSiteOff: the calling site is set to off.
	ErrSiteOff = errors.New("decider is off for this site")
	// ErrNoEndpoint: no enabled provider instance can reach the configured backend.
	ErrNoEndpoint = errors.New("no provider instance can reach the decision backend")
	// ErrBackoff: the endpoint failed recently and is being rested; callers fall
	// back to their own logic instead of paying another timeout.
	ErrBackoff = errors.New("decision endpoint is backing off after recent failures")
)

// HTTPError is a non-2xx answer from a decision service. Its message follows
// the "<name> HTTP <code>: <detail>" shape the rest of the app classifies.
type HTTPError struct {
	Backend    string
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s HTTP %d: %s", e.Backend, e.Status, e.Message)
}

// IsAuthError reports a credential or billing rejection (401/402/403). Retrying
// with the same configuration cannot succeed, so the Hub quarantines the
// endpoint until the provider settings change or the quarantine expires.
func IsAuthError(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	switch he.Status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	}
	return false
}

// errorClass is the short, stable label a ledger record stores instead of the
// raw error text (which may echo request content).
func errorClass(err error) string {
	if err == nil {
		return ""
	}
	var he *HTTPError
	switch {
	case errors.As(err, &he):
		return fmt.Sprintf("http_%d", he.Status)
	case errors.Is(err, ErrNoEndpoint):
		return "no_endpoint"
	case errors.Is(err, ErrBackoff):
		return "backoff"
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrSiteOff):
		return "off"
	case isTimeout(err):
		return "timeout"
	default:
		return "error"
	}
}
