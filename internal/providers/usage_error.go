package providers

import "errors"

// UsageError is a provider failure that still consumed billable tokens.
//
// A turn can fail AFTER the request was sent — a rate-limit or auth rejection at
// the result envelope, a CLI crash on the last internal round-trip — and the
// input tokens (plus every completed internal round-trip) are paid for either
// way. The provider contract returns (nil, err) on failure, so a plain error
// discards that usage and the most expensive turns bill as zero.
//
// Wrapping instead of returning a partial *Response is deliberate: every caller
// of Complete/Turn already treats a non-nil error as "there is no response", and
// handing them a non-nil response next to a non-nil error would make a failed
// turn look successful to whichever call site forgets the err check. The error
// stays an error; only a caller that explicitly asks for the usage (via
// UsageFromError) sees it.
type UsageError struct {
	Err           error
	Model         string
	Usage         Usage
	ProviderCalls int
}

func (e *UsageError) Error() string { return e.Err.Error() }

func (e *UsageError) Unwrap() error { return e.Err }

// WithUsage attaches billable usage to err. It returns err unchanged when there
// is nothing to bill (zero usage) or when the chain already carries a
// UsageError — the innermost wrap is closest to the parser that measured it.
func WithUsage(err error, model string, u Usage, providerCalls int) error {
	if err == nil || u == (Usage{}) {
		return err
	}
	var existing *UsageError
	if errors.As(err, &existing) {
		return err
	}
	return &UsageError{Err: err, Model: model, Usage: u, ProviderCalls: providerCalls}
}

// UsageFromError reports the billable usage carried by a failed turn, if any.
func UsageFromError(err error) (*UsageError, bool) {
	var ue *UsageError
	if errors.As(err, &ue) {
		return ue, true
	}
	return nil, false
}
