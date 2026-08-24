package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/secrets"
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

	backends, err := t.selectBackends()
	if err != nil {
		return "", err
	}

	return searchWithBackends(ctx, t.client, backends, query, count)
}

// searchWithBackends tries each backend in order and returns the first success.
// A backend that errors (its service is down, rate-limited, misconfigured) must
// not sink the call when another one is configured — the previous single-backend
// behaviour turned a stopped SearXNG container into "the agent has no web search
// at all". When every backend fails, the returned error names each one and what
// it failed with.
func searchWithBackends(ctx context.Context, client *http.Client, backends []searchBackend, query string, count int) (string, error) {
	var failures []string
	for _, backend := range backends {
		results, err := backend.search(ctx, client, query, count)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", backend.name(), searchFailureHint(err)))
			continue
		}
		return formatSearchResults(query, backend.name(), results), nil
	}
	return "", fmt.Errorf("web search failed on every configured backend — %s",
		strings.Join(failures, "; "))
}

// searchFailureHint annotates a backend error with the operator action it implies.
// Connection-level failures mean the service itself is not answering, which reads
// very differently from an HTTP error the service produced deliberately.
func searchFailureHint(err error) string {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "connection refused"),
		strings.Contains(low, "no such host"),
		strings.Contains(low, "actively refused"),
		strings.Contains(low, "timeout"),
		strings.Contains(low, "deadline exceeded"):
		return msg + " (the backend is unreachable — is the service running at that address?)"
	default:
		return msg
	}
}

// selectBackends lists the search backends configured in the workspace vault, in
// preference order: a self-hosted SearXNG (a deliberate, keyless infra choice)
// first, a Tavily key second. Both are returned when both are configured so the
// caller can fall back. With neither configured it returns an explicit error
// rather than silently degrading — the operator must add one credential via
// secret_set.
func (t WebSearchTool) selectBackends() ([]searchBackend, error) {
	if t.vault == nil {
		return nil, fmt.Errorf("no secret vault is available, so no search backend is configured " +
			"(add SEARXNG_URL or TAVILY_API_KEY)")
	}
	var backends []searchBackend
	if base, ok := t.vault.Get("SEARXNG_URL"); ok && strings.TrimSpace(base) != "" {
		backends = append(backends, searxngBackend{baseURL: strings.TrimSpace(base)})
	}
	if key, ok := t.vault.Get("TAVILY_API_KEY"); ok && strings.TrimSpace(key) != "" {
		backends = append(backends, tavilyBackend{apiKey: strings.TrimSpace(key)})
	}
	if len(backends) == 0 {
		return nil, fmt.Errorf("no web-search backend configured: add a secret named SEARXNG_URL " +
			"(self-hosted SearXNG base URL) or TAVILY_API_KEY (Tavily API key) via secret_set")
	}
	return backends, nil
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
