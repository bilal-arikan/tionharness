package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoopbackGuard verifies the external-gateway loopback default: with no auth token
// (gatewayRequireLoopback=true) a non-loopback client is rejected 403 while a loopback
// client passes; with a token (guard off) everything passes through to the handler's
// own bearer check.
func TestLoopbackGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	serve := func(requireLoopback bool, remote string) int {
		s := &Server{gatewayRequireLoopback: requireLoopback}
		h := s.loopbackGuard(ok)
		req := httptest.NewRequest(http.MethodPost, "/mcp/gateway", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := serve(true, "203.0.113.7:5555"); code != http.StatusForbidden {
		t.Errorf("loopback-only: remote client must be 403, got %d", code)
	}
	if code := serve(true, "127.0.0.1:5555"); code != http.StatusOK {
		t.Errorf("loopback-only: loopback client must pass, got %d", code)
	}
	if code := serve(true, "[::1]:5555"); code != http.StatusOK {
		t.Errorf("loopback-only: IPv6 loopback must pass, got %d", code)
	}
	if code := serve(false, "203.0.113.7:5555"); code != http.StatusOK {
		t.Errorf("token mode: remote client must pass the guard (bearer checked downstream), got %d", code)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8090": true,
		"[::1]:8090":     true,
		"203.0.113.7:80": false,
		"10.0.0.5:1234":  false,
		"garbage":        false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestEnvTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "on", "YES", " On "} {
		if !envTruthy(v) {
			t.Errorf("envTruthy(%q) should be true", v)
		}
	}
	for _, v := range []string{"", "0", "false", "off", "no", "x"} {
		if envTruthy(v) {
			t.Errorf("envTruthy(%q) should be false", v)
		}
	}
}
