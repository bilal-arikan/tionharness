package api

import (
	"net/http"
	"os/exec"
)

// knownExternalTools are optional third-party helpers TionSwarm can detect on the
// host. They are NOT bundled, installed, or run by TionSwarm — detection is
// presence-only so the user can see whether a tool they may want to wire up
// exists on this machine.
//
// Category groups tools in the Settings panel; Wire tells the UI how the tool is
// used once present:
//   - "hook" → wired via a PreToolUse/PostToolUse hook (UI offers a one-click toggle)
//   - "mcp"  → wired as an MCP server (UI shows an info badge → Settings ▸ MCP)
//   - "cli"  → a plain CLI the agent calls directly via Bash (UI shows a "CLI" badge)
//
// Adding a tool = one entry here (+ a frontend hook template only when Wire="hook").
var knownExternalTools = []struct {
	Name     string
	Desc     string
	URL      string
	Category string
	Wire     string
}{
	{"rtk", "Rust Token Killer — komut çıktısı sıkıştırma CLI proxy'si", "https://github.com/rtk-ai/rtk", "token", "hook"},
	{"sqz", "LLM bağlam sıkıştırma (PreToolUse hook)", "https://github.com/ojuschugh1/sqz", "token", "hook"},
	{"crabbox", "Uzak yürütme/test control-plane'i — kutuyu ısıt, diff'i senkronla, komutu uzakta koştur (lease+sync+run); geliştirmede Bash ile çağrılır", "https://github.com/openclaw/crabbox", "dev", "cli"},
	{"mmdc", "Mermaid CLI — mermaid diyagramlarını yerelde SVG/PNG'ye render eder (gömülü tarayıcı render'ının yanında dosya çıktısı için)", "https://github.com/mermaid-js/mermaid-cli", "render", "cli"},
	{"codebase-memory-mcp", "Codebase Memory — kod tabanını kalıcı bilgi grafiğine indeksler (158 dil, sub-ms sorgu, ~%99 daha az token); search_graph/query_graph/trace_path/get_architecture araçları. Market'te 'Codebase Memory MCP' paketi ile kurulur", "https://github.com/DeusData/codebase-memory-mcp", "dev", "mcp"},
}

// externalToolStatus is one tool's detection result for the Settings panel.
type externalToolStatus struct {
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	URL      string `json:"url"`
	Category string `json:"category"`
	Wire     string `json:"wire"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
}

// handleExternalTools reports whether each known external tool is present on the
// host PATH. Detection uses exec.LookPath ONLY: it searches the PATH directories
// (honouring PATHEXT on Windows) for the executable and never installs, executes,
// or modifies anything — satisfying "check without running".
func (s *Server) handleExternalTools(w http.ResponseWriter, _ *http.Request) {
	out := make([]externalToolStatus, 0, len(knownExternalTools))
	for _, t := range knownExternalTools {
		st := externalToolStatus{Name: t.Name, Desc: t.Desc, URL: t.URL, Category: t.Category, Wire: t.Wire}
		if p, err := exec.LookPath(t.Name); err == nil {
			st.Found = true
			st.Path = p
		}
		out = append(out, st)
	}
	writeJSON(w, http.StatusOK, out)
}
