package providers

import (
	"errors"
	"fmt"
	"time"
)

// CLIInterruption preserves partial work without converting a transport failure
// into a successful completion. Direct callers (including compactors) still see
// an error; the runtime may persist Partial with an explicit terminal marker.
type CLIInterruption struct {
	Partial *Response
	Reason  string
	Window  time.Duration
	Cause   error
}

func (e *CLIInterruption) Error() string {
	return fmt.Sprintf("CLI turn interrupted (%s): %v", e.Reason, e.Cause)
}
func (e *CLIInterruption) Unwrap() error { return e.Cause }

func InterruptedResponse(err error) (*Response, *CLIInterruption) {
	var interrupted *CLIInterruption
	if errors.As(err, &interrupted) && interrupted.Partial != nil {
		return interrupted.Partial, interrupted
	}
	return nil, nil
}
