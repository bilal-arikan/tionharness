package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMinimax_CompleteSuccess(t *testing.T) {
	var gotPath, gotAuth string
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello world"},"finish_reason":"stop"}],"model":"MiniMax-M2.1","usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer srv.Close()

	m := NewMinimax("secret-key", srv.URL)
	resp, err := m.Complete(context.Background(), Request{
		Model:  "MiniMax-M2.1",
		System: "be terse",
		Messages: []Message{
			{Role: RoleUser, Text: "hi"},
			{Role: RoleSystem, Text: "ignored"}, // system-role messages are skipped
			{Role: RoleAssistant, Text: ""},     // empty text skipped
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello world")
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Errorf("Usage = %+v, want {3 2}", resp.Usage)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("auth = %q, want Bearer secret-key", gotAuth)
	}
	// System prompt becomes the first message; in-band system/empty are dropped.
	if len(gotReq.Messages) != 2 || gotReq.Messages[0].Role != "system" || gotReq.Messages[1].Content != "hi" {
		t.Errorf("messages = %+v, want [system, user:hi]", gotReq.Messages)
	}
}

func TestOAICompat_CacheUsageParsing(t *testing.T) {
	// OpenAI/OpenRouter report cache reads under prompt_tokens_details; MiniMax at
	// the top level. Both are a subset of prompt_tokens, so InputTokens must come
	// out as the fresh remainder and CacheReadTokens as the cached count.
	cases := []struct {
		name           string
		usageJSON      string
		wantInput      int
		wantCacheRead  int
		wantCacheWrite int
	}{
		{
			name:          "openai details",
			usageJSON:     `{"prompt_tokens":100,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":80}}`,
			wantInput:     20,
			wantCacheRead: 80,
		},
		{
			name:          "minimax top level",
			usageJSON:     `{"prompt_tokens":100,"completion_tokens":10,"prompt_cache_hit_tokens":60}`,
			wantInput:     40,
			wantCacheRead: 60,
		},
		{
			name:           "openrouter write",
			usageJSON:      `{"prompt_tokens":50,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":0},"cache_creation_input_tokens":50}`,
			wantInput:      50,
			wantCacheRead:  0,
			wantCacheWrite: 50,
		},
		{
			name:          "no cache",
			usageJSON:     `{"prompt_tokens":7,"completion_tokens":3}`,
			wantInput:     7,
			wantCacheRead: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m","usage":` + tc.usageJSON + `}`))
			}))
			defer srv.Close()

			m := NewMinimax("k", srv.URL)
			resp, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "x"}}})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Usage.InputTokens != tc.wantInput {
				t.Errorf("InputTokens = %d, want %d", resp.Usage.InputTokens, tc.wantInput)
			}
			if resp.Usage.CacheReadTokens != tc.wantCacheRead {
				t.Errorf("CacheReadTokens = %d, want %d", resp.Usage.CacheReadTokens, tc.wantCacheRead)
			}
			if resp.Usage.CacheWriteTokens != tc.wantCacheWrite {
				t.Errorf("CacheWriteTokens = %d, want %d", resp.Usage.CacheWriteTokens, tc.wantCacheWrite)
			}
		})
	}
}

func TestOpenRouter_SystemCacheControl(t *testing.T) {
	// OpenRouter: the system message is STATIC-ONLY with a cache_control breakpoint;
	// the volatile dynamic moves to the message tail (a trailing part on the last
	// message, AFTER the history breakpoint) so it never busts the cached prefix.
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"anthropic/claude-sonnet-4.6","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewOpenAICompat("openrouter", "k", srv.URL, "anthropic/claude-sonnet-4.6")
	_, err := m.Complete(context.Background(), Request{
		System:        "STATIC PREFIX",
		SystemDynamic: "DYNAMIC SUFFIX",
		Messages:      []Message{{Role: RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotReq.Messages) == 0 || gotReq.Messages[0].Role != "system" {
		t.Fatalf("expected leading system message, got %+v", gotReq.Messages)
	}
	// System is static-only: a single content part carrying cache_control.
	sysParts, ok := gotReq.Messages[0].Content.([]any)
	if !ok || len(sysParts) != 1 {
		t.Fatalf("system content = %#v, want 1 static part", gotReq.Messages[0].Content)
	}
	if first, _ := sysParts[0].(map[string]any); first["cache_control"] == nil {
		t.Errorf("static part missing cache_control: %#v", sysParts[0])
	}
	// The dynamic rides the LAST message: [ "hi" (breakpoint), "DYNAMIC SUFFIX" (no bp) ].
	last := gotReq.Messages[len(gotReq.Messages)-1]
	msgParts, ok := last.Content.([]any)
	if !ok || len(msgParts) != 2 {
		t.Fatalf("last message content = %#v, want persisted+dynamic parts", last.Content)
	}
	if bp, _ := msgParts[0].(map[string]any); bp["cache_control"] == nil {
		t.Errorf("breakpoint must sit on the persisted message part: %#v", msgParts[0])
	}
	dyn, _ := msgParts[1].(map[string]any)
	if dyn["text"] != "DYNAMIC SUFFIX" || dyn["cache_control"] != nil {
		t.Errorf("dynamic must trail after the breakpoint, uncached: %#v", dyn)
	}
}

func TestOpenRouter_HistoryBreakpoint(t *testing.T) {
	// OpenRouter gets a SECOND breakpoint on the tail of the transcript so the
	// conversation prefix is cached. The last plain-text message becomes an array
	// part with cache_control; assistant tool-call messages are skipped.
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewOpenAICompat("openrouter", "k", srv.URL, "anthropic/claude-sonnet-4.6")
	_, err := m.Complete(context.Background(), Request{
		System: "SYS",
		Messages: []Message{
			{Role: RoleUser, Text: "first"},
			{Role: RoleAssistant, Text: "reply"},
			{Role: RoleUser, Text: "second"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// messages: [system, user:first, assistant:reply, user:second]
	last := gotReq.Messages[len(gotReq.Messages)-1]
	parts, ok := last.Content.([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("last message content = %#v, want 1 part with breakpoint", last.Content)
	}
	if p, _ := parts[0].(map[string]any); p["cache_control"] == nil {
		t.Errorf("last message missing cache_control: %#v", parts[0])
	}
	// An earlier message stays a plain string (only the tail carries the 2nd bp).
	if _, isStr := gotReq.Messages[1].Content.(string); !isStr {
		t.Errorf("non-tail message should stay a string, got %#v", gotReq.Messages[1].Content)
	}
}

// P2 (OpenRouter, caching on): the rolling summary rides a synthetic head user
// message right after the system message — inside the cached prefix — while the
// volatile dynamic still trails on the last message. So the summary is a cache
// READ between folds, not re-sent every turn.
func TestOpenRouter_SummaryHeadMessage(t *testing.T) {
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewOpenAICompat("openrouter", "k", srv.URL, "anthropic/claude-sonnet-4.6")
	_, err := m.Complete(context.Background(), Request{
		System:        "SYS",
		SystemDynamic: "DYN",
		Summary:       "PRIOR SUMMARY",
		Messages:      []Message{{Role: RoleAssistant, Text: "hello"}, {Role: RoleUser, Text: "again"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// messages: [system, user:PRIOR SUMMARY, assistant:hello, user:again(+DYN)]
	if len(gotReq.Messages) != 4 {
		t.Fatalf("got %d messages, want 4 (system + summary head + 2), %+v", len(gotReq.Messages), gotReq.Messages)
	}
	head := gotReq.Messages[1]
	if head.Role != RoleUser {
		t.Fatalf("summary head must be a user message, got role %q", head.Role)
	}
	if s, _ := head.Content.(string); s != "PRIOR SUMMARY" {
		t.Errorf("summary head content = %#v, want \"PRIOR SUMMARY\"", head.Content)
	}
}

// P2 (non-OpenRouter, caching off): the summary folds back into the plain-string
// system message; there is no head summary message.
func TestMinimax_SummaryFoldsIntoSystem(t *testing.T) {
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewMinimax("k", srv.URL)
	_, err := m.Complete(context.Background(), Request{
		System:        "STATIC",
		SystemDynamic: "DYN",
		Summary:       "PRIOR SUMMARY",
		Messages:      []Message{{Role: RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := gotReq.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("system content should be a plain string, got %#v", gotReq.Messages[0].Content)
	}
	if s != "STATIC\n\nDYN\n\nPRIOR SUMMARY" {
		t.Errorf("summary must fold into system when caching is off, got %q", s)
	}
	// Only [system, user:hi] — no head summary message.
	if len(gotReq.Messages) != 2 {
		t.Errorf("got %d messages, want 2 (no head summary), %+v", len(gotReq.Messages), gotReq.Messages)
	}
}

func TestMinimax_NoCacheControl(t *testing.T) {
	// Non-OpenRouter endpoints keep plain string system content (no breakpoint).
	var gotReq oaiReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewMinimax("k", srv.URL)
	_, err := m.Complete(context.Background(), Request{
		System:        "STATIC",
		SystemDynamic: "DYN",
		Messages:      []Message{{Role: RoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, isStr := gotReq.Messages[0].Content.(string); !isStr {
		t.Errorf("minimax system content should be a plain string, got %#v", gotReq.Messages[0].Content)
	}
}

func TestMinimax_MissingKey(t *testing.T) {
	m := NewMinimax("", "")
	if _, err := m.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("expected missing-key error, got nil")
	}
}

func TestMinimax_BaseRespError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":1004,"status_msg":"auth failed"}}`))
	}))
	defer srv.Close()

	m := NewMinimax("k", srv.URL)
	_, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "x"}}})
	if err == nil {
		t.Fatal("expected base_resp error, got nil")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("error %q should mention status_msg", err.Error())
	}
}

func TestMinimax_DefaultBaseURL(t *testing.T) {
	m := NewMinimax("k", "")
	if m.baseURL != minimaxDefaultBaseURL {
		t.Errorf("baseURL = %q, want default %q", m.baseURL, minimaxDefaultBaseURL)
	}
	// trailing slash is trimmed
	m2 := NewMinimax("k", "https://x.example/v1/")
	if m2.baseURL != "https://x.example/v1" {
		t.Errorf("baseURL = %q, want trimmed", m2.baseURL)
	}
}
