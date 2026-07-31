// Package exttools knows about the OPTIONAL third-party CLIs TionSwarm can work
// alongside: which ones exist, where they live on this host, what version is
// installed, what the latest published version is, and — where it is safe — how
// to update them.
//
// Nothing here is bundled with TionSwarm. Detection resolves a path; a version
// probe runs the tool with its version flag (side-effect free); an update runs a
// package manager the user already has. Tools whose upgrade means replacing a
// binary or unpacking a multi-file archive are deliberately declared "manual" —
// see UpdateSpec.
package exttools

import (
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/stt"
	"github.com/bilal-arikan/tionswarm/internal/tts"
)

// UpdateKind classifies how a tool is upgraded.
const (
	// UpdateCommand: a single idempotent package-manager command TionSwarm may run
	// for the user (npm/winget/…). Safe because the manager owns the install dir
	// and handles a running binary itself.
	UpdateCommand = "command"
	// UpdateManual: the upgrade replaces a binary or unpacks an archive in place.
	// TionSwarm refuses to do this: on Windows a running child (an MCP stdio server
	// holding its own .exe, a piper synth in flight) locks the file, and a half-
	// applied update leaves the tool broken. The UI shows Note + the release link.
	UpdateManual = "manual"
)

// UpdateSpec declares how a tool is upgraded. Kind is one of the UpdateKind
// constants; Command/Args apply to UpdateCommand, Note to UpdateManual.
type UpdateSpec struct {
	Kind    string   `json:"kind"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// Tool is one catalog entry: presentation metadata plus the probes that make
// version reporting and updating possible.
//
// Category groups tools in the Settings panel; Wire tells the UI how the tool is
// used once present:
//   - "hook"    → wired via a PreToolUse/PostToolUse hook (UI offers a one-click toggle)
//   - "setting" → wired by a workspace setting, not a hook (UI offers that toggle)
//   - "mcp"     → wired as an MCP server (UI shows an info badge → Settings ▸ MCP)
//   - "cli"     → a plain CLI the agent calls directly via Bash (UI shows a "CLI" badge)
//   - ""        → used automatically by a TionSwarm subsystem (voice)
//
// VersionArgs is the flag that makes the tool print its version; empty means the
// tool cannot report one and the UI shows no version chip.
//
// Adding a tool = one entry here (+ a frontend hook template only when Wire="hook").
type Tool struct {
	Name        string
	Desc        string
	URL         string
	Category    string
	Wire        string
	VersionArgs []string
	Update      UpdateSpec
}

// Catalog is the ordered set of known external tools.
var Catalog = []Tool{
	// rtk is wired by the ShellCommandRewrite SETTING, not a hook. It used to ship a
	// PreToolUse hook that prefixed `rtk ` onto every command; that template was
	// removed after it bricked the shell (2026-07-31, WS10/SES63): the hook body was
	// a PowerShell one-liner, but claude-cli runs hooks through bash, so it died with
	// `syntax error near unexpected token '|'` — and a failing PreToolUse hook BLOCKS
	// the tool call, so every Bash call in that workspace failed. The in-process
	// rewriter replaces it and is strictly better: it covers both the native and
	// bridged paths, and it only rewrites commands measured to benefit (the hook
	// wrapped everything, including `git diff` and `cat`, where rtk loses).
	{
		Name:        "rtk",
		Desc:        "Rust Token Killer — test/build komutlarını yeniden yazan CLI proxy'si (Ayarlar ▸ Workspace ▸ \"Shell komutu yeniden yazma\" ile açılır)",
		URL:         "https://github.com/rtk-ai/rtk",
		Category:    "token",
		Wire:        "setting",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Release sayfasından yeni binary'yi indirip mevcut rtk.exe'nin üzerine kopyala. TionSwarm bunu kendisi yapmaz: çalışan bir shell turu ikiliyi kilitleyebilir ve yarım kalan kopya aracı bozar.",
		},
	},
	{
		Name:        "sqz",
		Desc:        "LLM bağlam sıkıştırma (PreToolUse hook)",
		URL:         "https://github.com/ojuschugh1/sqz",
		Category:    "token",
		Wire:        "hook",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Release sayfasından yeni binary'yi indirip mevcut sqz.exe'nin üzerine kopyala. Güncelledikten sonra bağlı PostToolUse hook'u yeniden çalışacaktır.",
		},
	},
	{
		Name:        "mmdc",
		Desc:        "Mermaid CLI — mermaid diyagramlarını yerelde SVG/PNG'ye render eder (gömülü tarayıcı render'ının yanında dosya çıktısı için)",
		URL:         "https://github.com/mermaid-js/mermaid-cli",
		Category:    "render",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind:    UpdateCommand,
			Command: "npm",
			Args:    []string{"install", "-g", "@mermaid-js/mermaid-cli"},
		},
	},
	{
		Name:        "codebase-memory-mcp",
		Desc:        "Codebase Memory — kod tabanını kalıcı bilgi grafiğine indeksler (158 dil, sub-ms sorgu, ~%99 daha az token); search_graph/query_graph/trace_path/get_architecture araçları. Market'te 'Codebase Memory MCP' paketi ile kurulur",
		URL:         "https://github.com/DeusData/codebase-memory-mcp",
		Category:    "dev",
		Wire:        "mcp",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Önce bu workspace'te MCP sunucusunu kaldır (aşağıdaki \"MCP'yi kaldır\" düğmesi) — çalışan stdio alt-süreci .exe dosyasını kilitler. Sonra release'ten yeni exe'yi kopyalayıp MCP'yi tekrar ekle.",
		},
	},
	{
		Name:        "piper",
		Desc:        "Piper — yerel/offline nöral TTS motoru (35+ dil, Türkçe dahil). TionSwarm sunucu-tarafı sesli okuma (TTS) için OTOMATİK kullanır → telefon dahil her cihazda aynı ses. Progs\\piper altına kurulur veya PATH'te bulunur; bir de .onnx ses modeli gerekir.",
		URL:         "https://github.com/OHF-Voice/piper1-gpl",
		Category:    "voice",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Release arşivi binary + dll + espeak-ng verisi taşır; klasörün tamamı değişir. Yeni arşivi indirip mevcut piper klasörünün üzerine aç. Ses modelleri (.onnx) ayrıdır, yeniden indirmen gerekmez.",
		},
	},
	{
		Name:        "whisper-cli",
		Desc:        "whisper.cpp — yerel/offline STT (ses→metin, 100+ dil, Türkçe dahil). TionSwarm sunucu-tarafı sesle yazma (dikte) için kullanır; ffmpeg + bir ggml-*.bin model gerekir. Progs\\whisper altına kurulur veya PATH'te bulunur.",
		URL:         "https://github.com/ggml-org/whisper.cpp",
		Category:    "voice",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Release arşivi birden çok binary + dll taşır; klasörün tamamı değişir. Yeni arşivi mevcut whisper klasörünün üzerine aç. Model dosyaları (ggml-*.bin) ayrıdır, korunur.",
		},
	},
	{
		Name:        "ffmpeg",
		Desc:        "FFmpeg — ses/video dönüştürücü. Sunucu-tarafı STT'de tarayıcı ses kaydını (webm/opus) whisper'ın istediği 16 kHz WAV'a çevirmek için gerekir.",
		URL:         "https://ffmpeg.org",
		Category:    "voice",
		VersionArgs: []string{"-version"},
		Update: UpdateSpec{
			Kind:    UpdateCommand,
			Command: "winget",
			Args:    []string{"upgrade", "--id", "Gyan.FFmpeg", "--accept-source-agreements", "--accept-package-agreements"},
		},
	},
}

// Find returns the catalog entry for name, or nil when unknown.
func Find(name string) *Tool {
	for i := range Catalog {
		if Catalog[i].Name == name {
			return &Catalog[i]
		}
	}
	return nil
}

// githubPrefix is the only release feed exttools understands. A tool whose URL
// points elsewhere (ffmpeg.org) simply has no upstream version to compare with.
const githubPrefix = "https://github.com/"

// Repo derives the "owner/repo" GitHub slug from the tool's URL, or "" when the
// URL is not a GitHub project. Derived rather than stored as a second field so
// the two can never drift apart.
func (t Tool) Repo() string {
	if !strings.HasPrefix(t.URL, githubPrefix) {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(t.URL, githubPrefix), "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// Detect resolves a known tool's presence + absolute path. Most tools are found
// on PATH (exec.LookPath, honouring PATHEXT on Windows), but piper and
// whisper-cli usually live OUTSIDE PATH (a Progs install), so they use the
// tts/stt resolvers (env / Progs / PATH) that the voice subsystems already rely
// on. Detection never runs the tool.
func Detect(name string) (bool, string) {
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
	if p, err := lookPath(name); err == nil {
		return true, p
	}
	return false, ""
}
