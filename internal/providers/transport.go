package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
