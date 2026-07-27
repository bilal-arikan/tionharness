package api

import (
	"net/http"
	"os/exec"

	"github.com/bilal-arikan/tionswarm/internal/stt"
	"github.com/bilal-arikan/tionswarm/internal/tts"
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
	{"mmdc", "Mermaid CLI — mermaid diyagramlarını yerelde SVG/PNG'ye render eder (gömülü tarayıcı render'ının yanında dosya çıktısı için)", "https://github.com/mermaid-js/mermaid-cli", "render", "cli"},
	{"codebase-memory-mcp", "Codebase Memory — kod tabanını kalıcı bilgi grafiğine indeksler (158 dil, sub-ms sorgu, ~%99 daha az token); search_graph/query_graph/trace_path/get_architecture araçları. Market'te 'Codebase Memory MCP' paketi ile kurulur", "https://github.com/DeusData/codebase-memory-mcp", "dev", "mcp"},
	{"piper", "Piper — yerel/offline nöral TTS motoru (35+ dil, Türkçe dahil). TionSwarm sunucu-tarafı sesli okuma (TTS) için OTOMATİK kullanır → telefon dahil her cihazda aynı ses. Progs\\piper altına kurulur veya PATH'te bulunur; bir de .onnx ses modeli gerekir.", "https://github.com/OHF-Voice/piper1-gpl", "voice", ""},
	{"whisper-cli", "whisper.cpp — yerel/offline STT (ses→metin, 100+ dil, Türkçe dahil). TionSwarm sunucu-tarafı sesle yazma (dikte) için kullanır; ffmpeg + bir ggml-*.bin model gerekir. Progs\\whisper altına kurulur veya PATH'te bulunur.", "https://github.com/ggml-org/whisper.cpp", "voice", ""},
	{"ffmpeg", "FFmpeg — ses/video dönüştürücü. Sunucu-tarafı STT'de tarayıcı ses kaydını (webm/opus) whisper'ın istediği 16 kHz WAV'a çevirmek için gerekir.", "https://ffmpeg.org", "voice", ""},
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
		if found, p := detectExternalTool(t.Name); found {
			st.Found = true
			st.Path = p
		}
		out = append(out, st)
	}
	writeJSON(w, http.StatusOK, out)
}

// detectExternalTool resolves a known tool's presence + path. Most tools are
// found via PATH (exec.LookPath), but Piper usually lives OUTSIDE PATH (a Progs
// install), so it uses the tts package's richer resolver (env / Progs / PATH).
func detectExternalTool(name string) (bool, string) {
	// Piper + whisper-cli usually live OUTSIDE PATH (a Progs install), so use the
	// tts/stt resolvers (env / Progs / PATH) instead of plain LookPath.
	switch name {
	case "piper":
		if p := tts.BinaryPath(); p != "" {
			return true, p
		}
		return false, ""
	case "whisper-cli":
		if p := stt.WhisperPath(); p != "" {
			return true, p
		}
		return false, ""
	}
	if p, err := exec.LookPath(name); err == nil {
		return true, p
	}
	return false, ""
}
