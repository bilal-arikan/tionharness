package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryPolicy controls transient-failure retries for provider HTTP calls.
// Retries cover network errors and a small set of retryable status codes (429 +
// the transient 5xx family, including Anthropic's 529 "overloaded"), with
// exponential backoff and jitter. A Retry-After header, when present, overrides
// the computed backoff.
type retryPolicy struct {
	maxAttempts int           // total attempts including the first (1 = no retry)
	baseDelay   time.Duration // backoff before the first retry
	maxDelay    time.Duration // backoff cap
}

// httpRetry is the package-wide retry policy for provider transport. It is a var
// (not a const) so tests can swap in a no-wait policy.
var httpRetry = retryPolicy{maxAttempts: 4, baseDelay: 500 * time.Millisecond, maxDelay: 8 * time.Second}

// retryableStatus reports whether an HTTP status warrants a retry: request
// timeout, rate limiting, and the transient 5xx family (529 = Anthropic
// "overloaded").
func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
		529:                            // overloaded (Anthropic)
		return true
	}
	return false
}

// backoff returns the delay before the given 1-based retry attempt: exponential
// (base * 2^(attempt-1)) capped at maxDelay, with up to ±25% jitter to avoid
// synchronized retries across concurrent agents.
func (p retryPolicy) backoff(attempt int) time.Duration {
	d := float64(p.baseDelay) * math.Pow(2, float64(attempt-1))
	if d > float64(p.maxDelay) {
		d = float64(p.maxDelay)
	}
	jitter := (rand.Float64()*0.5 - 0.25) * d // ±25%
	wait := time.Duration(d + jitter)
	if wait < 0 {
		wait = 0
	}
	return wait
}

// parseRetryAfter parses a Retry-After header's delta-seconds form (the
// HTTP-date form is ignored). Returns 0 when absent or unparseable.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// doWithRetry sends the request produced by build, retrying transient failures
// per the package retry policy. build is invoked once per attempt because an
// HTTP request body is consumed on send. On the final attempt the response (even
// with a retryable status) or the wrapped network error is returned unmodified
// so callers can surface the provider's own message/body. Context cancellation
// is honored both during the wait and on a failed send.
func doWithRetry(ctx context.Context, client *http.Client, prefix string, build func() (*http.Request, error)) (*http.Response, error) {
	policy := httpRetry
	if policy.maxAttempts < 1 {
		policy.maxAttempts = 1
	}
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil && !retryableStatus(resp.StatusCode) {
			return resp, nil // success or non-retryable status
		}
		if attempt == policy.maxAttempts {
			if err != nil {
				return nil, fmt.Errorf("%s request: %w", prefix, err)
			}
			return resp, nil // out of retries → let the caller read the body
		}

		// Transient failure with attempts remaining: compute the wait, drain any
		// response body so the connection can be reused, then back off.
		wait := policy.backoff(attempt)
		if err == nil {
			if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				wait = ra
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	// Unreachable: the loop returns on the final attempt.
	return nil, fmt.Errorf("%s: retry loop exhausted", prefix)
}

// postJSON marshals body to JSON, POSTs it to url with the given headers, reads
// the full response, and decodes it into out. It returns the HTTP status code
// and the raw body so callers can apply provider-specific error handling (e.g.
// vendor error envelopes that arrive with a 200, or non-200 fallbacks).
//
// Transient failures are retried (see doWithRetry). A decode failure is reported
// with the status code attached so the caller can surface a useful message. The
// prefix names the provider for error wrapping.
func postJSON(ctx context.Context, client *http.Client, prefix, url string, headers map[string]string, body, out any) (int, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}

	resp, err := doWithRetry(ctx, client, prefix, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	})
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return resp.StatusCode, raw, fmt.Errorf("%s decode (status %d): %w", prefix, resp.StatusCode, err)
	}
	return resp.StatusCode, raw, nil
}

// postSSE POSTs body and streams the Server-Sent-Events response, invoking
// onEvent(eventName, data) for each `data:` line (eventName is the most recent
// `event:` line, "" if none). It stops when onEvent returns false, the stream
// ends, or the context is cancelled. A non-2xx status returns an error with the
// body so callers can surface the provider's message. The prefix names the
// provider for error wrapping.
func postSSE(ctx context.Context, client *http.Client, prefix, url string, headers map[string]string, body any, onEvent func(event string, data []byte) bool) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	// Retry only the initial connection: doWithRetry returns before any stream
	// bytes are consumed, so a retried attempt never replays partial output.
	resp, err := doWithRetry(ctx, client, prefix, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s HTTP %d: %s", prefix, resp.StatusCode, string(raw))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tolerate large event lines
	var event string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "":
			event = "" // blank line ends an event block
		case strings.HasPrefix(line, ":"):
			// comment / heartbeat — ignore
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			data := []byte(strings.TrimSpace(line[len("data:"):]))
			if !onEvent(event, data) {
				return nil
			}
		}
	}
	return sc.Err()
}
