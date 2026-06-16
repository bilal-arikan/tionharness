package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	status, raw, err := postJSON(context.Background(), srv.Client(), "test", srv.URL,
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
	status, raw, err := postJSON(context.Background(), srv.Client(), "test", srv.URL, nil, map[string]any{}, &out)
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
	_, _, err := postJSON(context.Background(), srv.Client(), "myprov", srv.URL, nil, map[string]any{}, &out)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if want := "myprov decode"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestPostJSON_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	srv.Close() // close immediately so the request fails to connect

	var out map[string]any
	_, _, err := postJSON(context.Background(), client, "myprov", srv.URL, nil, map[string]any{}, &out)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if want := "myprov request"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}
