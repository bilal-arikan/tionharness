package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
)

// auth.go — optional bearer-token authentication for the REST surface.
//
// The gate is OPT-IN: setting TIONHARNESS_API_AUTH_TOKEN turns it on, and
// nothing else does. Without the variable the API behaves exactly as it always
// has, on every bind address.
//
// It deliberately does NOT auto-engage on a non-loopback bind, even though that
// is the risky configuration. scripts/dev.ps1 binds 0.0.0.0 BY DEFAULT so the UI
// and API are reachable from a phone or second laptop on the LAN — a documented,
// everyday workflow — and the bundled frontend sends no Authorization header at
// all (frontend/src/api/client.ts). Refusing unauthenticated non-loopback
// requests therefore breaks the normal dev loop with a 401 the UI cannot
// satisfy: workspaces simply stop loading. Auth that fires on a condition the
// only client cannot meet is a bug, not a safeguard.
//
// So the honest state of things: the API is unauthenticated unless an operator
// opts in, and a 0.0.0.0 bind on an untrusted network is exposed. dev.ps1 says
// this in its own header. Closing that properly needs the frontend to carry a
// token first; until then this provides the mechanism for anyone deploying
// beyond a trusted LAN, and logs a warning when the risky shape is detected
// (see internal/app/app.go).
//
// Behavior:
//
//	TIONHARNESS_API_AUTH_TOKEN=<secret>  → bearer required on every /api/* call,
//	                                       from loopback and network alike.
//	(unset)                              → open, on any bind address.

// AuthTokenEnv names the environment variable holding the bearer token. Exported
// so the boot path can name it in the warning it logs when a non-loopback bind
// has no token set.
const AuthTokenEnv = "TIONHARNESS_API_AUTH_TOKEN"

// authExemptPath reports whether a path is reachable without a token even when
// one is configured. Deliberately tiny: only unauthenticated liveness probing.
// /api/version is NOT exempt — it reports the build, which is reconnaissance.
func authExemptPath(path string) bool {
	return path == "/health"
}

// apiAuth wraps the REST surface with the token policy described above. token is
// the configured secret; when it is empty the gate is off and next is returned
// unwrapped, so the default deployment pays nothing per request.
func apiAuth(token string, next http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight carries no Authorization header by design; withCORS
		// answers it before this runs, but a mount-order change must not turn
		// every preflight into a 401.
		if r.Method == http.MethodOptions || authExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !bearerMatches(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tionharness"`)
			writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerMatches reports whether an Authorization header carries the expected
// bearer token. The comparison is constant-time: a token is a shared secret and
// byte-wise early exit leaks its prefix to a caller who can time the response.
func bearerMatches(header, want string) bool {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	got := strings.TrimSpace(header[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// addrIsLoopbackOnly reports whether a listen address binds exclusively to the
// loopback interface. An empty host (":8080") or a wildcard ("0.0.0.0", "::")
// binds every interface and is NOT loopback-only.
func addrIsLoopbackOnly(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		// An unset address means "not configured", not "exposed": the caller has
		// not bound anything yet, so demanding a token here would be noise.
		return true
	}
	// SplitHostPort handles the bracketed-IPv6 form ("[::1]:8080") that hand
	// splitting on the last colon gets wrong. A bare host with no port ("::1")
	// fails it, in which case the whole string IS the host.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	switch host {
	case "":
		return false // ":8080" — every interface.
	case "localhost":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AuthPolicyFromEnv reports the configured token and whether addr is an exposed
// (non-loopback) bind. The second value does NOT gate anything — it drives the
// boot warning only, so an operator who exposes the API without a token is told
// rather than silently locked out (see apiAuth's header for why).
func AuthPolicyFromEnv(addr string) (token string, exposedBind bool) {
	return strings.TrimSpace(os.Getenv(AuthTokenEnv)), !addrIsLoopbackOnly(addr)
}
