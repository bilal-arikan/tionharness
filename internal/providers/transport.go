package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// postJSON marshals body to JSON, POSTs it to url with the given headers, reads
// the full response, and decodes it into out. It returns the HTTP status code
// and the raw body so callers can apply provider-specific error handling (e.g.
// vendor error envelopes that arrive with a 200, or non-200 fallbacks).
//
// A decode failure is reported with the status code attached so the caller can
// surface a useful message. The prefix names the provider for error wrapping.
func postJSON(ctx context.Context, client *http.Client, prefix, url string, headers map[string]string, body, out any) (int, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s request: %w", prefix, err)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", prefix, err)
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
