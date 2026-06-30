package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/secrets"
)

// stubCipher is a no-op cipher so tests can build a real vault without keys.
type stubCipher struct{}

func (stubCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (stubCipher) Decrypt(c string) (string, error) { return strings.TrimPrefix(c, "enc:"), nil }

func newTestVault(t *testing.T) *secrets.Vault {
	t.Helper()
	v, err := secrets.Open(t.TempDir(), stubCipher{})
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	return v
}

func TestWebSearchRequiresQuery(t *testing.T) {
	tool := NewWebSearchTool(newTestVault(t))
	if _, err := tool.Call(t.Context(), []byte(`{"query":"  "}`)); err == nil {
		t.Fatal("expected error for blank query")
	}
}

func TestWebSearchNoBackendConfigured(t *testing.T) {
	// Vault present but neither SEARXNG_URL nor TAVILY_API_KEY set: must error
	// loudly rather than silently returning nothing.
	tool := NewWebSearchTool(newTestVault(t))
	if _, err := tool.Call(t.Context(), []byte(`{"query":"go generics"}`)); err == nil {
		t.Fatal("expected error when no search backend is configured")
	}
}

func TestWebSearchNilVault(t *testing.T) {
	tool := NewWebSearchTool(nil)
	if _, err := tool.Call(t.Context(), []byte(`{"query":"x"}`)); err == nil {
		t.Fatal("expected error when no vault is available")
	}
}

func TestWebSearchSearxngHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("format"); got != "json" {
			t.Errorf("format = %q, want json", got)
		}
		if got := r.URL.Query().Get("q"); got != "go generics" {
			t.Errorf("q = %q, want 'go generics'", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"Generics in Go","url":"https://go.dev/doc/generics","content":"Type parameters explained."},
			{"title":"Tutorial","url":"https://example.com/t","content":"Hands-on guide."}
		]}`))
	}))
	defer srv.Close()

	vault := newTestVault(t)
	if _, err := vault.Set("SEARXNG_URL", srv.URL, ""); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	tool := NewWebSearchTool(vault)
	out, err := tool.Call(t.Context(), []byte(`{"query":"go generics","count":5}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	for _, want := range []string{"via searxng", "Generics in Go", "https://go.dev/doc/generics", "2 result"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestWebSearchSearxngTrimsToCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"title":"a","url":"https://a","content":"x"},
			{"title":"b","url":"https://b","content":"y"},
			{"title":"c","url":"https://c","content":"z"}
		]}`))
	}))
	defer srv.Close()

	vault := newTestVault(t)
	_, _ = vault.Set("SEARXNG_URL", srv.URL, "")
	tool := NewWebSearchTool(vault)

	out, err := tool.Call(t.Context(), []byte(`{"query":"q","count":2}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "2 result") || strings.Contains(out, "https://c") {
		t.Errorf("expected exactly 2 results (no third), got:\n%s", out)
	}
}

func TestWebSearchSearxngPreferredOverTavily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"hit","url":"https://h","content":"c"}]}`))
	}))
	defer srv.Close()

	vault := newTestVault(t)
	_, _ = vault.Set("SEARXNG_URL", srv.URL, "")
	_, _ = vault.Set("TAVILY_API_KEY", "tvly-should-not-be-used", "")

	out, err := NewWebSearchTool(vault).Call(t.Context(), []byte(`{"query":"q"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "via searxng") {
		t.Errorf("expected SearXNG to win over Tavily, got:\n%s", out)
	}
}

func TestFormatSearchResultsEmpty(t *testing.T) {
	out := formatSearchResults("nothing", "tavily", nil)
	if !strings.Contains(out, "no results") {
		t.Errorf("expected 'no results', got %q", out)
	}
}
