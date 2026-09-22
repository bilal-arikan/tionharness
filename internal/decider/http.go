package decider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// sharedClient carries no client-wide timeout: every attempt is bounded by its
// own context deadline instead, so one slow call cannot outlive its caller.
var sharedClient = &http.Client{}

// Retry shape for a decision call. A decision sits in the caller's critical path
// (a permission check, a flow branch), so the budget is one quick retry — never
// the multi-second exponential backoff chat transports use.
var (
	retryPause    = 200 * time.Millisecond
	maxRetryAfter = 2 * time.Second
)

// maxResponseBytes caps how much of a response is read. A decision answer is a
// few hundred bytes; anything near this size is not a decision answer.
const maxResponseBytes = 1 << 20

// postJSON POSTs body as JSON to url and decodes a 2xx answer into out. A
// transient failure (network error, attempt timeout, 408/429/5xx) is retried
// once after a short pause. Each attempt gets its own timeout, and the whole
// call never outlives ctx. prefix names the backend in errors.
func postJSON(ctx context.Context, client *http.Client, prefix, url string, authorize func(http.Header), timeout time.Duration, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", prefix, err)
	}
	if client == nil {
		client = sharedClient
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		retry, wait, err := postOnce(ctx, client, prefix, url, authorize, timeout, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt == 2 || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// postOnce is one attempt. It reports whether a failure is worth retrying and
// how long to wait first.
func postOnce(ctx context.Context, client *http.Client, prefix, url string, authorize func(http.Header), timeout time.Duration, payload []byte, out any) (retry bool, wait time.Duration, err error) {
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(actx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, 0, fmt.Errorf("%s: build request: %w", prefix, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if authorize != nil {
		authorize(req.Header)
	}
	resp, err := client.Do(req)
	if err != nil {
		// The parent context ending is the caller's decision, not a transient
		// fault: report it as is and never retry it.
		if ctx.Err() != nil {
			return false, 0, ctx.Err()
		}
		return true, retryPause, fmt.Errorf("%s request: %w", prefix, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, ctx.Err()
		}
		return true, retryPause, fmt.Errorf("%s: read response: %w", prefix, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		he := &HTTPError{Backend: prefix, Status: resp.StatusCode, Message: errorMessage(raw), RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
		wait := retryPause
		if he.RetryAfter > 0 {
			wait = min(he.RetryAfter, maxRetryAfter)
		}
		return retryableStatus(resp.StatusCode), wait, he
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false, 0, fmt.Errorf("%s: decode response (status %d): %w", prefix, resp.StatusCode, err)
	}
	return false, 0, nil
}

// retryableStatus reports whether an HTTP status is a transient fault.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout,
		524, 529: // edge timeout, provider overloaded
		return true
	}
	return false
}

// errorMessage extracts a readable message from an error body: the standard
// {"error":{"message":…}} envelope when present, else the raw text. Whitespace
// is collapsed and the result capped, because a validation error arrives as a
// pretty-printed JSON dump several hundred bytes long.
func errorMessage(raw []byte) string {
	var env struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := string(raw)
	if json.Unmarshal(raw, &env) == nil && env.Error != nil && env.Error.Message != "" {
		msg = env.Error.Message
	}
	msg = strings.Join(strings.Fields(msg), " ")
	const maxLen = 300
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "…"
	}
	if msg == "" {
		msg = "(empty response)"
	}
	return msg
}

// parseRetryAfter parses the delta-seconds form of Retry-After (0 when absent).
func parseRetryAfter(h string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// isTimeout reports whether err is a deadline or network timeout.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
