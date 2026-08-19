package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/claudeauth"
)

// Loopback (paste-less) OAuth: instead of the user copying a code from the callback
// page, the browser redirects straight to a short-lived local HTTP listener we spin
// up on 127.0.0.1:<ephemeral>. The callback handler exchanges the code and writes
// the credential; the popup just polls status until it flips to ok/error. Works when
// the browser runs on the same machine as the backend (the desktop / local case).

type loopbackFlow struct {
	mu        sync.Mutex
	status    string // "pending" | "ok" | "error"
	detail    string
	home      string
	createdAt time.Time
	srv       *http.Server
}

var loopbackFlows = struct {
	mu sync.Mutex
	m  map[string]*loopbackFlow
}{m: map[string]*loopbackFlow{}}

// pruneLoopback closes + drops flows older than the TTL (called under the lock).
func pruneLoopback(now time.Time) {
	for id, fl := range loopbackFlows.m {
		if now.Sub(fl.createdAt) > oauthLoginTTL {
			if fl.srv != nil {
				_ = fl.srv.Close()
			}
			delete(loopbackFlows.m, id)
		}
	}
}

// handleClaudeOAuthLoopbackStart begins the paste-less flow: bind a local listener,
// build a loopback authorization URL pointing at it, and return the URL for the popup
// to open. The credential is written by the local /callback handler when the browser
// redirects back; the popup polls handleClaudeOAuthLoopbackStatus.
func (s *Server) handleClaudeOAuthLoopbackStart(w http.ResponseWriter, r *http.Request) {
	home, ok := s.resolveAppCLIHome(w, "claude-cli")
	if !ok {
		return
	}
	s.handleClaudeOAuthLoopbackStartFor(w, r, home, s.providers.ClaudeCLIPath())
}

func (s *Server) handleClaudeOAuthLoopbackStartFor(w http.ResponseWriter, r *http.Request, home, binPath string) {
	_ = binPath
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not bind local callback port: "+err.Error())
		return
	}
	port := ln.Addr().(*net.TCPAddr).Port
	authURL, pending, err := claudeauth.BeginWith(claudeauth.LoopbackConfig(port))
	if err != nil {
		_ = ln.Close()
		writeError(w, http.StatusInternalServerError, "oauth begin failed: "+err.Error())
		return
	}
	flowID := uuid.NewString()
	fl := &loopbackFlow{status: "pending", home: home, createdAt: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(cw http.ResponseWriter, cr *http.Request) {
		q := cr.URL.Query()
		code, state := q.Get("code"), q.Get("state")
		if errParam := q.Get("error"); errParam != "" {
			fl.set("error", firstNonEmpty(q.Get("error_description"), errParam))
			writeLoopbackPage(cw, false, fl.detailOf())
			fl.shutdownSoon()
			return
		}
		paste := code
		if state != "" {
			paste = code + "#" + state
		}
		cred, exErr := claudeauth.Exchange(nil, pending, paste)
		if exErr != nil {
			fl.set("error", exErr.Error())
		} else if wErr := claudeauth.WriteCredentials(home, cred); wErr != nil {
			fl.set("error", wErr.Error())
		} else {
			fl.set("ok", "")
		}
		writeLoopbackPage(cw, fl.statusOf() == "ok", fl.detailOf())
		fl.shutdownSoon()
	})
	srv := &http.Server{Handler: mux}
	fl.srv = srv

	loopbackFlows.mu.Lock()
	pruneLoopback(time.Now())
	loopbackFlows.m[flowID] = fl
	loopbackFlows.mu.Unlock()

	go func() { _ = srv.Serve(ln) }() // returns when Shutdown/Close is called
	// Safety: never leave a listener open forever if the user abandons the flow.
	go func() {
		time.Sleep(oauthLoginTTL)
		if fl.statusOf() == "pending" {
			fl.set("error", "zaman aşımı — giriş tamamlanmadı")
		}
		_ = srv.Close()
	}()

	writeJSON(w, http.StatusOK, map[string]any{"flowId": flowID, "authUrl": authURL, "port": port})
}

// handleClaudeOAuthLoopbackStatus reports the current state of a loopback flow so
// the popup can poll until the browser callback completes it.
func (s *Server) handleClaudeOAuthLoopbackStatus(w http.ResponseWriter, r *http.Request) {
	home, ok := s.resolveAppCLIHome(w, "claude-cli")
	if !ok {
		return
	}
	s.handleClaudeOAuthLoopbackStatusFor(w, r, home, s.providers.ClaudeCLIPath())
}

func (s *Server) handleClaudeOAuthLoopbackStatusFor(w http.ResponseWriter, r *http.Request, home, binPath string) {
	_, _ = home, binPath
	id := r.URL.Query().Get("flowId")
	loopbackFlows.mu.Lock()
	fl := loopbackFlows.m[id]
	loopbackFlows.mu.Unlock()
	if fl == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unknown"})
		return
	}
	if fl.home != home {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unknown"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":        fl.statusOf(),
		"detail":        fl.detailOf(),
		"claudeHomeDir": fl.home,
	})
}

// --- loopbackFlow helpers (thread-safe) ---

func (fl *loopbackFlow) set(status, detail string) {
	fl.mu.Lock()
	fl.status, fl.detail = status, detail
	fl.mu.Unlock()
}

func (fl *loopbackFlow) statusOf() string {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return fl.status
}

func (fl *loopbackFlow) detailOf() string {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return fl.detail
}

// shutdownSoon closes the local server shortly after the callback response flushes.
func (fl *loopbackFlow) shutdownSoon() {
	srv := fl.srv
	go func() {
		time.Sleep(500 * time.Millisecond)
		if srv != nil {
			_ = srv.Shutdown(context.Background())
		}
	}()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// writeLoopbackPage renders the tiny HTML the browser shows after the redirect, so
// the user knows to return to TionSwarm.
func writeLoopbackPage(w http.ResponseWriter, ok bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, body, color := "Giriş başarılı ✓", "TionSwarm'a geri dönebilirsin — bu sekmeyi kapat.", "#16a34a"
	if !ok {
		title, body, color = "Giriş başarısız", escHTML(detail), "#dc2626"
	}
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:system-ui,sans-serif;background:#0b0b0c;color:#e5e5e5;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center;max-width:420px;padding:24px">
<h2 style="color:%s;margin:0 0 8px">%s</h2>
<p style="color:#a3a3a3;font-size:14px">%s</p>
</div></body></html>`, title, color, title, body)
}

// escHTML escapes a detail string for safe inclusion in the loopback HTML.
func escHTML(s string) string {
	r := ""
	for _, ch := range s {
		switch ch {
		case '<':
			r += "&lt;"
		case '>':
			r += "&gt;"
		case '&':
			r += "&amp;"
		default:
			r += string(ch)
		}
	}
	return r
}
