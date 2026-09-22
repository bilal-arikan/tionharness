package decider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrDisabled: the decider is switched off in settings.
	ErrDisabled = errors.New("decider is disabled")
	// ErrSiteOff: the calling authority is set to off.
	ErrSiteOff = errors.New("decider is off for this authority")
	// ErrNoModel: no decision model is configured (or the one named is gone).
	ErrNoModel = errors.New("no decision model is configured")
	// ErrModelDisabled: the decision model is switched off in its settings.
	ErrModelDisabled = errors.New("decision model is disabled")
	// ErrNoEndpoint: no enabled provider instance can lend the decision model
	// its credentials.
	ErrNoEndpoint = errors.New("no provider instance can reach the decision backend")
	// ErrBackoff: the model failed recently and is being rested; callers fall
	// back to their own logic instead of paying another timeout.
	ErrBackoff = errors.New("decision model is backing off after recent failures")
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
// with the same configuration cannot succeed, so the Hub quarantines the model
// until its settings (or the provider settings it borrows from) change or the
// quarantine expires.
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

// fallbackWorthy reports whether another model might answer where this one
// failed: anything but a malformed request (which no model can fix) and the
// caller giving up.
func fallbackWorthy(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() == nil && !errors.Is(err, ErrInvalidRequest)
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
	case errors.Is(err, ErrNoModel):
		return "no_model"
	case errors.Is(err, ErrModelDisabled):
		return "model_disabled"
	case errors.Is(err, ErrBackoff):
		return "backoff"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrSiteOff):
		return "off"
	case isTimeout(err):
		return "timeout"
	default:
		return "error"
	}
}
