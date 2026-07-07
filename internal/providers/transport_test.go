package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withRetry swaps the package retry policy for the duration of a test and
// restores it afterward, so retry tests run without real backoff waits and
// non-retry tests don't incur backoff delays.
func withRetry(t *testing.T, p retryPolicy) {
	t.Helper()
	prev := httpRetry
	httpRetry = p
	t.Cleanup(func() { httpRetry = prev })
}

func TestPostJSON_SuccessSendsHeadersAndBody(t *testing.T) {
	var gotContentType, gotCustom, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotCustom = r.Header.Get("X-Test")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"n":7}`))
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
		N  int  `json:"n"`
	}
	status, raw, _, err := postJSON(context.Background(), srv.Client(), "test", srv.URL,
		map[string]string{"X-Test": "1"}, map[string]any{"hi": "there"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if !out.OK || out.N != 7 {
		t.Errorf("decoded = %+v, want {OK:true N:7}", out)
	}
	if len(raw) == 0 {
		t.Error("raw body should not be empty")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotCustom != "1" {
		t.Errorf("X-Test = %q, want 1", gotCustom)
	}
	if gotBody != `{"hi":"there"}` {
		t.Errorf("body = %q, want {\"hi\":\"there\"}", gotBody)
	}
}

func TestPostJSON_Non200ReturnsStatusAndRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	var out map[string]any
	status, raw, _, err := postJSON(context.Background(), srv.Client(), "test", srv.URL, nil, map[string]any{}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusTeapot {
		t.Errorf("status = %d, want 418", status)
	}
	if string(raw) != `{"error":"nope"}` {
		t.Errorf("raw = %q", string(raw))
	}
}

func TestPostJSON_DecodeFailureWrapsPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	var out map[string]any
	_, _, _, err := postJSON(context.Background(), srv.Client(), "myprov", srv.URL, nil, map[string]any{}, &out)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if want := "myprov decode"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestPostJSON_TransportError(t *testing.T) {
	withRetry(t, retryPolicy{maxAttempts: 1}) // no retry: fail fast on connect error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	srv.Close() // close immediately so the request fails to connect

	var out map[string]any
	_, _, _, err := postJSON(context.Background(), client, "myprov", srv.URL, nil, map[string]any{}, &out)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if want := "myprov request"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestPostJSON_RetriesThenSucceeds(t *testing.T) {
	withRetry(t, retryPolicy{maxAttempts: 4, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond})
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503 → retryable
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	status, _, _, err := postJSON(context.Background(), srv.Client(), "test", srv.URL, nil, map[string]any{}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK || !out.OK {
		t.Errorf("status=%d ok=%v, want 200 true", status, out.OK)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server saw %d calls, want 3 (2 retries)", got)
	}
}

func TestPostJSON_RetryExhaustionReturnsLastResponse(t *testing.T) {
	withRetry(t, retryPolicy{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond})
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer srv.Close()

	var out map[string]any
	status, raw, _, err := postJSON(context.Background(), srv.Client(), "test", srv.URL, nil, map[string]any{}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err) // exhausted retries surface the body, not a transport error
	}
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
	if string(raw) != `{"error":"down"}` {
		t.Errorf("raw = %q", string(raw))
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server saw %d calls, want 2", got)
	}
}

func TestPostJSON_NonRetryableStatusNotRetried(t *testing.T) {
	withRetry(t, retryPolicy{maxAttempts: 4, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond})
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest) // 400 → not retryable
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var out map[string]any
	status, _, _, err := postJSON(context.Background(), srv.Client(), "test", srv.URL, nil, map[string]any{}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server saw %d calls, want 1 (no retry)", got)
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, code := range []int{408, 429, 500, 502, 503, 504, 529} {
		if !retryableStatus(code) {
			t.Errorf("status %d should be retryable", code)
		}
	}
	for _, code := range []int{200, 400, 401, 403, 404, 418, 422} {
		if retryableStatus(code) {
			t.Errorf("status %d should NOT be retryable", code)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("3"); got != 3*time.Second {
		t.Errorf("parseRetryAfter(3) = %v, want 3s", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("parseRetryAfter(empty) = %v, want 0", got)
	}
	if got := parseRetryAfter("Wed, 21 Oct 2026 07:28:00 GMT"); got != 0 {
		t.Errorf("parseRetryAfter(http-date) = %v, want 0 (unsupported form)", got)
	}
}
