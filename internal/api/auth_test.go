package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddrIsLoopbackOnly(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.1:0", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{"::1", true},
		// Every-interface binds. These drive the boot WARNING only — the gate
		// itself is opt-in, so nothing here refuses a request (see apiAuth).
		{":8080", false},
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{"192.168.1.10:8080", false},
	} {
		if got := addrIsLoopbackOnly(tc.addr); got != tc.want {
			t.Errorf("addrIsLoopbackOnly(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestBearerMatches(t *testing.T) {
	for _, tc := range []struct {
		header, want string
		ok           bool
	}{
		{"Bearer s3cret", "s3cret", true},
		{"bearer s3cret", "s3cret", true}, // scheme is case-insensitive per RFC 7235
		{"Bearer  s3cret ", "s3cret", true},
		{"Bearer wrong", "s3cret", false},
		{"Bearer", "s3cret", false},
		{"", "s3cret", false},
		{"Basic s3cret", "s3cret", false},
		{"Bearer s3cretX", "s3cret", false},
	} {
		if got := bearerMatches(tc.header, tc.want); got != tc.ok {
			t.Errorf("bearerMatches(%q, %q) = %v, want %v", tc.header, tc.want, got, tc.ok)
		}
	}
}

// okHandler is the protected resource standing in for the real mux.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served"))
	})
}

// TestAPIAuthNoTokenIsOpen: the gate is opt-in. With no token the API behaves
// exactly as it did before auth existed — on ANY bind address, not just
// loopback. scripts/dev.ps1 binds 0.0.0.0 by default for phone/LAN testing and
// the bundled frontend sends no Authorization header, so a gate that engaged on
// a non-loopback bind would 401 the whole UI (workspaces stop loading).
func TestAPIAuthNoTokenIsOpen(t *testing.T) {
	h := apiAuth("", okHandler())
	for _, path := range []string{"/api/sessions", "/api/workspaces", "/api/secrets"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("no-token GET %s = %d, want 200 (gate must stay opt-in)", path, rr.Code)
		}
	}
}

func TestAPIAuthTokenGate(t *testing.T) {
	h := apiAuth("s3cret", okHandler())

	t.Run("valid token passes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/sessions", nil)
		r.Header.Set("Authorization", "Bearer s3cret")
		h.ServeHTTP(rr, r)
		if rr.Code != http.StatusOK {
			t.Fatalf("valid token = %d, want 200", rr.Code)
		}
	})

	t.Run("missing token refused even from loopback", func(t *testing.T) {
		// A configured token applies everywhere: loopback is not a bypass.
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/sessions", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("missing token = %d, want 401", rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got == "" {
			t.Error("401 should carry WWW-Authenticate so clients can negotiate")
		}
	})

	t.Run("wrong token refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/sessions", nil)
		r.Header.Set("Authorization", "Bearer nope")
		h.ServeHTTP(rr, r)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("wrong token = %d, want 401", rr.Code)
		}
	})

	t.Run("health stays open for liveness probes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/health", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("/health = %d, want 200 (probes carry no token)", rr.Code)
		}
	})

	t.Run("version is NOT exempt", func(t *testing.T) {
		// Build info is reconnaissance; it must sit behind the gate.
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/version", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("/api/version = %d, want 401", rr.Code)
		}
	})

	t.Run("CORS preflight passes without a token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("OPTIONS", "/api/sessions", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("preflight = %d, want 200", rr.Code)
		}
	})
}
