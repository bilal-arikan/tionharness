package providers

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type CLITraceError struct {
	Cause error
	Trace []TraceStep
}

func (e *CLITraceError) Error() string { return e.Cause.Error() }
func (e *CLITraceError) Unwrap() error { return e.Cause }
func CLIErrorTrace(err error) []TraceStep {
	var traced *CLITraceError
	if errors.As(err, &traced) {
		return traced.Trace
	}
	return nil
}

var diagnosticCredential = regexp.MustCompile(`(?i)(authorization|bearer|api[_-]?key|access[_-]?token|token|password|secret)([\s"'=:\\]+)[^\s,;"']+`)
var diagnosticURL = regexp.MustCompile(`https?://[^\s"'<>]+`)
var diagnosticBearer = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;"']+`)

// Startup diagnostics are bounded and never retain URL credentials or tokens.
func safeCLIStartupDetail(detail string) string {
	detail = diagnosticURL.ReplaceAllStringFunc(detail, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return "[redacted URL]"
		}
		u.User, u.RawQuery, u.Fragment = nil, "", ""
		return u.String()
	})
	detail = diagnosticBearer.ReplaceAllString(detail, "Bearer [redacted]")
	detail = diagnosticCredential.ReplaceAllString(detail, "$1$2[redacted]")
	runes := []rune(strings.TrimSpace(detail))
	if len(runes) > 1600 {
		runes = runes[len(runes)-1600:]
	}
	return string(runes)
}
