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
			{Role: RoleAssistant, Text: ""},      // empty text skipped
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
