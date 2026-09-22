package announce

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPostDiscordSendsContent pins the Discord payload shape: a JSON object
// with a "content" field, which is what a webhook URL accepts.
func TestPostDiscordSendsContent(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := Post(context.Background(), srv.Client(), TargetDiscord, srv.URL, "", "hello"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got["content"] != "hello" {
		t.Fatalf("discord payload is %+v", got)
	}
}

// TestPostTelegramUsesSendMessage pins the Bot API call: the endpoint is the
// token base, sendMessage is appended, and the chat id rides in the body.
func TestPostTelegramUsesSendMessage(t *testing.T) {
	var path string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Post(context.Background(), srv.Client(), TargetTelegram, srv.URL, "-100123", "hello"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !strings.HasSuffix(path, "/sendMessage") {
		t.Fatalf("telegram path is %q", path)
	}
	if got["chat_id"] != "-100123" || got["text"] != "hello" {
		t.Fatalf("telegram payload is %+v", got)
	}
}

// TestPostTelegramNeedsAChatID: without it the Bot API has nowhere to post, and
// failing now beats a 400 the agent has to interpret.
func TestPostTelegramNeedsAChatID(t *testing.T) {
	err := Post(context.Background(), http.DefaultClient, TargetTelegram, "https://api.telegram.org/botX", "", "hi")
	if err == nil {
		t.Fatal("a telegram post with no chat id was accepted")
	}
}

// TestPostRejectsAnEmptyEndpoint: an unset secret must be an error, not a
// silent no-op that reports the release as announced.
func TestPostRejectsAnEmptyEndpoint(t *testing.T) {
	if err := Post(context.Background(), http.DefaultClient, TargetDiscord, "", "", "hi"); err == nil {
		t.Fatal("an empty endpoint was accepted")
	}
}

// TestPostSurfacesProviderErrors: a rejected post must fail loudly and carry
// the provider's own message, or a release goes unannounced while the agent
// reports success.
func TestPostSurfacesProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"Invalid Webhook Token"}`))
	}))
	defer srv.Close()

	err := Post(context.Background(), srv.Client(), TargetDiscord, srv.URL, "", "hi")
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	if !strings.Contains(err.Error(), "Invalid Webhook Token") {
		t.Fatalf("the provider message was swallowed: %v", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("the status code was swallowed: %v", err)
	}
}
