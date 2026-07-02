package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// webSearchMaxOutBytes caps the result list fed back to the model (context).
// Each result is title + url + a trimmed snippet, so this comfortably holds the
// default handful of hits without risking context bloat.
const webSearchMaxOutBytes = 32 * 1024

// webSearchDefaultCount is how many results we ask the backend for when the
// caller does not specify, and the ceiling we clamp larger requests to.
const (
	webSearchDefaultCount = 5
	webSearchMaxCount     = 10
)

// searchResult is one backend-agnostic hit: a title, the page URL and a short
// textual snippet. Backends normalise their own payloads into this shape.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// searchBackend performs a query against a concrete search service (Tavily,
// SearXNG, …) and returns normalised results. name() identifies the backend in
// the tool's output header so the model (and logs) know which one answered.
type searchBackend interface {
	name() string
	search(ctx context.Context, client *http.Client, query string, count int) ([]searchResult, error)
}

// WebSearchTool runs a web search and returns ranked results (title, URL,
// snippet) as Markdown. It is the native counterpart of the claude-cli WebSearch
// tool, provided to every provider that does NOT ship its own (anthropic API,
// minimax, openrouter). The claude-cli path keeps its built-in
// WebSearch, so this tool is deliberately NOT registered there.
//
// The backend is chosen at call time from the workspace vault, so no key is
// baked in and the operator can switch providers without a rebuild:
//   - SEARXNG_URL    → a self-hosted SearXNG instance (keyless, private)
//   - TAVILY_API_KEY → the Tavily LLM-search API (1k free queries/month)
//
// Read-only. Unlike WebFetch, the request URL is operator-configured (a trusted
// backend), not model-supplied — only the query string comes from the model —
// so there is no SSRF surface and the client uses a plain dialer (a self-hosted
// SearXNG on a private/loopback address must remain reachable).
type WebSearchTool struct {
	client *http.Client
	vault  *secrets.Vault
}

// NewWebSearchTool binds the tool to the workspace vault (where the backend
// credentials live) with a bounded-timeout HTTP client.
func NewWebSearchTool(vault *secrets.Vault) WebSearchTool {
	return WebSearchTool{
		client: &http.Client{Timeout: 30 * time.Second},
		vault:  vault,
	}
}

func (WebSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "WebSearch",
		Description: "Search the web and return ranked results (title, URL, snippet) as Markdown. " +
			"Use it to find current information or to discover pages, then call WebFetch on a " +
			"result's URL to read its full content. Read-only. Requires a search backend configured " +
			"in the workspace vault (SEARXNG_URL or TAVILY_API_KEY).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"query":{"type":"string","description":"The search query"},
				"count":{"type":"integer","description":"Max results to return (default 5, max 10)"}
			},
			"required":["query"],
			"additionalProperties":false
		}`),
	}
}

func (t WebSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	count := args.Count
	if count <= 0 {
		count = webSearchDefaultCount
	}
	if count > webSearchMaxCount {
		count = webSearchMaxCount
	}

	backend, err := t.selectBackend()
	if err != nil {
		return "", err
	}

	results, err := backend.search(ctx, t.client, query, count)
	if err != nil {
		return "", fmt.Errorf("%s search failed: %w", backend.name(), err)
	}
	return formatSearchResults(query, backend.name(), results), nil
}

// selectBackend picks the search backend from the workspace vault. A self-hosted
// SearXNG URL wins when present (a deliberate, keyless infra choice); otherwise a
// Tavily key is used. With neither configured it returns an explicit error rather
// than silently degrading — the operator must add one credential via secret_set.
func (t WebSearchTool) selectBackend() (searchBackend, error) {
	if t.vault == nil {
		return nil, fmt.Errorf("no secret vault is available, so no search backend is configured " +
			"(add SEARXNG_URL or TAVILY_API_KEY)")
	}
	if base, ok := t.vault.Get("SEARXNG_URL"); ok && strings.TrimSpace(base) != "" {
		return searxngBackend{baseURL: strings.TrimSpace(base)}, nil
	}
	if key, ok := t.vault.Get("TAVILY_API_KEY"); ok && strings.TrimSpace(key) != "" {
		return tavilyBackend{apiKey: strings.TrimSpace(key)}, nil
	}
	return nil, fmt.Errorf("no web-search backend configured: add a secret named SEARXNG_URL " +
		"(self-hosted SearXNG base URL) or TAVILY_API_KEY (Tavily API key) via secret_set")
}

// formatSearchResults renders hits as a numbered Markdown list with a header
// naming the query and backend, capped at webSearchMaxOutBytes.
func formatSearchResults(query, backend string, results []searchResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("WebSearch: %q (via %s) — no results.", query, backend)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "WebSearch: %q (via %s) — %d result(s)\n\n", query, backend, len(results))
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, firstNonEmpty(strings.TrimSpace(r.Title), "(untitled)"), r.URL)
		if s := strings.TrimSpace(r.Snippet); s != "" {
			fmt.Fprintf(&b, "   %s\n", s)
		}
		b.WriteString("\n")
	}
	out := strings.TrimRight(b.String(), "\n")
	if len(out) > webSearchMaxOutBytes {
		out = out[:webSearchMaxOutBytes] + fmt.Sprintf("\n\n[truncated at %dKB]", webSearchMaxOutBytes/1024)
	}
	return out
}
