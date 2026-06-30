package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// tavilyEndpoint is the Tavily search API. The key is sent as a Bearer token
// (the current scheme); the body carries the query and result count.
const tavilyEndpoint = "https://api.tavily.com/search"

// tavilyBackend queries the Tavily LLM-search API. Tavily returns clean,
// snippet-ready content per hit, which maps directly onto searchResult.
type tavilyBackend struct {
	apiKey string
}

func (tavilyBackend) name() string { return "tavily" }

func (b tavilyBackend) search(ctx context.Context, client *http.Client, query string, count int) ([]searchResult, error) {
	reqBody, err := json.Marshal(map[string]any{
		"query":        query,
		"max_results":  count,
		"search_depth": "basic",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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
		return nil, fmt.Errorf("decode response: %w", err)
	}

	out := make([]searchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}
