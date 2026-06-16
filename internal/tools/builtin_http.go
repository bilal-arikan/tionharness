package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bilal/swarmgo/internal/providers"
)

const httpGetMaxBytes = 64 * 1024 // cap the body fed back to the model

// HTTPGetTool fetches a URL with GET and returns the (truncated) body. It is a
// read-only network tool; no headers/auth are supported by design.
type HTTPGetTool struct {
	client *http.Client
}

// NewHTTPGetTool builds the tool with a bounded-timeout client.
func NewHTTPGetTool() HTTPGetTool {
	return HTTPGetTool{client: &http.Client{Timeout: 20 * time.Second}}
}

func (HTTPGetTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "http_get",
		Description: "Fetch a URL over HTTP GET and return the response body (truncated to 64KB). Read-only.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"url":{"type":"string","description":"Absolute http(s) URL to fetch"}},
			"required":["url"],
			"additionalProperties":false
		}`),
	}
}

func (t HTTPGetTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "swarmgo/0.0.1")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(body)), nil
}
