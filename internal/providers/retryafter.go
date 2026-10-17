package providers

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// RetryAfterSuffix renders the machine-readable wait-hint suffix a provider
// appends to a transient-failure error message, e.g. " (retry-after: 30s)".
// "" when there is no hint. Parsed back by ParseRetryAfterHint.
func RetryAfterSuffix(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return fmt.Sprintf(" (retry-after: %ds)", int(d.Seconds()))
}

// retryAfterHintRe matches the suffix produced by RetryAfterSuffix.
var retryAfterHintRe = regexp.MustCompile(`\(retry-after: (\d+)s\)`)

// ParseRetryAfterHint extracts the server's wait hint from a provider error
// message (the RetryAfterSuffix marker). 0 when absent — callers fall back to
// their own computed backoff.
func ParseRetryAfterHint(err error) time.Duration {
	if err == nil {
		return 0
	}
	m := retryAfterHintRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	secs, aerr := strconv.Atoi(m[1])
	if aerr != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
