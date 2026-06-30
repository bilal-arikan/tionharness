package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// searxngBackend queries a self-hosted SearXNG instance via its JSON API
// (GET {base}/search?q=...&format=json). The instance must have the `json`
// output format enabled in its settings.yml. Keyless and private — the operator
// runs it themselves — so a private/loopback base URL is expected and allowed.
type searxngBackend struct {
	baseURL string
}

func (searxngBackend) name() string { return "searxng" }

func (b searxngBackend) search(ctx context.Context, client *http.Client, query string, count int) ([]searchResult, error) {
	endpoint, err := url.Parse(strings.TrimRight(b.baseURL, "/") + "/search")
	if err != nil {
		return nil, fmt.Errorf("invalid SEARXNG_URL: %w", err)
	}
	q := endpoint.Query()
	q.Set("q", query)
	q.Set("format", "json")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "swarmgo/0.0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (is the SearXNG `json` format enabled?): %w", err)
	}

	// SearXNG ignores a result count in the request, so we trim client-side.
	results := parsed.Results
	if len(results) > count {
		results = results[:count]
	}
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}
