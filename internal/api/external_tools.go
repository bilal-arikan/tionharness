package api

import (
	"net/http"
	"os/exec"
)

// knownExternalTools are optional third-party token-optimization helpers SwarmGo
// can detect on the host. They are NOT bundled, installed, or run by SwarmGo —
// detection is presence-only so the user can see whether a tool they may want to
// wire up exists on this machine.
var knownExternalTools = []struct {
	Name string
	Desc string
	URL  string
}{
	{"rtk", "Rust Token Killer — komut çıktısı sıkıştırma CLI proxy'si", "https://github.com/rtk-ai/rtk"},
	{"sqz", "LLM bağlam sıkıştırma (PreToolUse hook)", "https://github.com/ojuschugh1/sqz"},
	{"context-mode", "Bağlam penceresi optimizasyonu — tool çıktısını sandbox'layıp ~%98 küçültür + SQLite oturum belleği (MCP + hook)", "https://github.com/mksglu/context-mode"},
}

// externalToolStatus is one tool's detection result for the Settings panel.
type externalToolStatus struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	URL   string `json:"url"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

// handleExternalTools reports whether each known external token-optimization tool
// is present on the host PATH. Detection uses exec.LookPath ONLY: it searches the
// PATH directories (honouring PATHEXT on Windows) for the executable and never
// installs, executes, or modifies anything — satisfying "check without running".
func (s *Server) handleExternalTools(w http.ResponseWriter, _ *http.Request) {
	out := make([]externalToolStatus, 0, len(knownExternalTools))
	for _, t := range knownExternalTools {
		st := externalToolStatus{Name: t.Name, Desc: t.Desc, URL: t.URL}
		if p, err := exec.LookPath(t.Name); err == nil {
			st.Found = true
			st.Path = p
		}
		out = append(out, st)
	}
	writeJSON(w, http.StatusOK, out)
}
