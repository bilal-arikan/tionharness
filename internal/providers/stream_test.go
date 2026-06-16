package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer returns a test server that writes the given raw SSE body and a flush.
func sseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

func TestAnthropic_StreamAccumulatesDeltas(t *testing.T) {
	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":11}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	srv := sseServer(t, body)
	defer srv.Close()

	a := NewAnthropic("key")
	a.client = srv.Client()
	// redirect the fixed endpoint to the test server via a custom transport.
	a.client.Transport = rewriteHost(srv.URL)

	var got []string
	resp, err := a.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "hi"}}},
		func(d string) { got = append(got, d) })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if resp.Text != "Hello" {
		t.Errorf("Text = %q, want Hello", resp.Text)
	}
	if strings.Join(got, "|") != "Hel|lo" {
		t.Errorf("deltas = %v, want [Hel lo]", got)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {11 5}", resp.Usage)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("stop = %q, want end_turn", resp.StopReason)
	}
}

func TestMinimax_StreamAccumulatesDeltas(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Mer"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"haba"}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	srv := sseServer(t, body)
	defer srv.Close()

	m := NewMinimax("key", srv.URL)
	var got []string
	resp, err := m.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "selam"}}},
		func(d string) { got = append(got, d) })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if resp.Text != "Merhaba" {
		t.Errorf("Text = %q, want Merhaba", resp.Text)
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v, want {7 3}", resp.Usage)
	}
}

func TestCanStream(t *testing.T) {
	if !CanStream(NewAnthropic("k")) {
		t.Error("Anthropic should implement Streamer")
	}
	if !CanStream(NewMinimax("k", "")) {
		t.Error("Minimax should implement Streamer")
	}
}

// rewriteHost is an http.RoundTripper that redirects every request to the given
// base URL's host/scheme, so a provider with a hard-coded endpoint can be aimed
// at a test server.
type hostRewriter struct {
	base      string
	transport http.RoundTripper
}

func rewriteHost(base string) http.RoundTripper {
	return &hostRewriter{base: base, transport: http.DefaultTransport}
}

func (h *hostRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := req.URL.Parse(h.base)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.Host = u.Host
	return h.transport.RoundTrip(req)
}
