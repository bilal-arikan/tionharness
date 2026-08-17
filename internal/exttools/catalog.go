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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/proc"
	"github.com/bilal-arikan/tionswarm/internal/stt"
	"github.com/bilal-arikan/tionswarm/internal/tts"
)

// ClaudeToolName is the catalog key for the Claude Code CLI. Named after the
// executable (that is what PATH resolution and the version probe use), not after
// the provider id "claude-cli" that wraps it.
const ClaudeToolName = "claude"

// OpenPencilToolName is the catalog key for OpenPencil. Deliberately NOT the
// executable name: the binary is `op`, which is ALSO the 1Password CLI. Keying
// the catalog on "openpencil" keeps Find/Detect/the update endpoint unambiguous,
// and openPencilExe() below refuses to claim a bare PATH `op` that is not
// OpenPencil's — reporting 1Password's version against OpenPencil's release feed
// would produce a confident, wrong "outdated" verdict.
const OpenPencilToolName = "openpencil"

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
//   - "provider" → drives an LLM provider (UI shows a badge → Settings ▸ Providers)
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
	// PreRelease opts this tool into LatestPreRelease: the project marks every
	// release as a GitHub prerelease, so releases/latest 404s and the version
	// check would report "no published release" forever. Leave false unless the
	// repo was checked — see LatestPreRelease for why this is not a global rule.
	PreRelease bool
}

// wingetSpec builds a winget one-click update for the given package id — but ONLY
// on Windows. TionSwarm also runs on Linux servers, where winget does not exist.
//
// Getting this wrong is not merely a dead button. RunUpdate would fail with a
// clear error, but the panel ALSO renders a copy-to-clipboard chip for every
// `command` spec, so a Ubuntu user would be handed an authoritative-looking
// `winget upgrade --id …` line that cannot work on their machine. A wrong
// instruction is worse than no instruction, so off-Windows these become `manual`
// with a note pointing at the distro's own package manager.
//
// goos is a parameter rather than a direct runtime.GOOS read so both branches are
// testable from either platform.
func wingetSpec(goos, id, manualNote string) UpdateSpec {
	if goos == "windows" {
		return UpdateSpec{
			Kind:    UpdateCommand,
			Command: "winget",
			Args:    []string{"upgrade", "--id", id, "--accept-source-agreements", "--accept-package-agreements"},
		}
	}
	return UpdateSpec{Kind: UpdateManual, Note: manualNote}
}

// bunUpdateSpec: winget owns the install on Windows (that is how it is installed
// there), but everywhere else bun ships its own updater, which is the supported
// path and needs no package manager at all.
func bunUpdateSpec(goos string) UpdateSpec {
	if goos == "windows" {
		return wingetSpec(goos, "Oven-sh.Bun", "")
	}
	return UpdateSpec{Kind: UpdateCommand, Command: "bun", Args: []string{"upgrade"}}
}

// nodeUpdateSpec / pythonUpdateSpec stay `manual` on every platform — TionSwarm
// cannot know which tool owns the install (nvm, a distro package, pyenv, brew,
// conda, an installer) and picking wrong fights the real owner. Only the NOTE is
// platform-specific, because a note is an instruction the user will actually
// follow: telling a Ubuntu admin to run winget is how a panel loses its
// credibility.
func nodeUpdateSpec(goos string) UpdateSpec {
	note := "Node'u hangi aracın kurduğunu TionSwarm bilemez, o yüzden karışmaz: nvm kullanıyorsan `nvm install --lts && nvm alias default lts/*`, aksi halde dağıtımının paketi yerine **NodeSource** deposu önerilir (`apt`'taki node genelde çok eskidir). Sunucuda **LTS** hattında kal."
	switch goos {
	case "windows":
		note = "Node'u hangi aracın kurduğunu TionSwarm bilemez, o yüzden karışmaz: nvm kullanıyorsan `nvm install --lts && nvm alias default lts/*` (winget/installer nvm'in kurulumuyla çakışır), aksi halde nodejs.org installer'ı veya `winget upgrade --id OpenJS.NodeJS.LTS`. Sunucuda **LTS** hattında kal."
	case "darwin":
		note = "Node'u hangi aracın kurduğunu TionSwarm bilemez, o yüzden karışmaz: nvm kullanıyorsan `nvm install --lts && nvm alias default lts/*`, Homebrew ile kurduysan `brew upgrade node`. **LTS** hattında kal."
	}
	return UpdateSpec{Kind: UpdateManual, Note: note}
}

func pythonUpdateSpec(goos string) UpdateSpec {
	// The Linux warning is the important one: on Debian/Ubuntu the system
	// interpreter is what apt's own tooling runs, so "upgrading python3" in place
	// is a known way to brick a server. pyenv/venv is the safe answer there.
	note := "Python'u hangi aracın kurduğunu TionSwarm bilemez, o yüzden karışmaz. **Dikkat:** Debian/Ubuntu'da sistem `python3`'ü apt'ın kendi araçları tarafından kullanılır — yerinde yükseltmek sunucuyu bozabilir. Yeni sürüm gerekiyorsa `deadsnakes` PPA'sından yan yana kur veya **pyenv** kullan; proje bağımlılıklarını `venv` içinde tut."
	switch goos {
	case "windows":
		note = "Python'u hangi aracın kurduğunu TionSwarm bilemez (python.org installer'ı, winget, pyenv-win, conda…), o yüzden karışmaz. python.org'dan yeni sürümü kurabilir veya `winget upgrade --id Python.Python.3.13` diyebilirsin. Minör sürüm atlarken (3.13 → 3.14) `pip` paketlerinin yeniden kurulması gerekir."
	case "darwin":
		note = "Python'u hangi aracın kurduğunu TionSwarm bilemez (Homebrew, pyenv, conda, python.org installer'ı…), o yüzden karışmaz. Homebrew ile kurduysan `brew upgrade python@3.13`; macOS'un kendi sistem python'una dokunma. Minör sürüm atlarken `pip` paketleri yeniden kurulmalıdır."
	}
	return UpdateSpec{Kind: UpdateManual, Note: note}
}

// gitProjectURL picks the release feed git is compared against.
//
// git/git on GitHub is a read-only mirror that publishes TAGS but no RELEASES,
// so it 404s on releases/latest and would report "sürüm karşılaştırılamadı"
// forever. git-for-windows/git does publish releases, and its tags name the
// upstream version they build (v2.55.0.windows.3 → 2.55.0), so the comparison is
// meaningful. It is still a Windows distribution, so elsewhere the entry falls
// back to the project site: no GitHub slug → no release check, which is the
// honest answer rather than a Windows build number shown to a Linux user.
var gitProjectURL = func() string {
	if runtime.GOOS == "windows" {
		return "https://github.com/git-for-windows/git"
	}
	return "https://git-scm.com"
}()

// Catalog is the ordered set of known external tools.
var Catalog = []Tool{
	// The Claude Code CLI is the only catalog entry TionSwarm depends on for a
	// CORE feature rather than an optional nicety: the keyless `claude-cli`
	// provider is this binary. It is listed here anyway (and not only in the
	// provider settings) because the questions this panel answers — where is it,
	// which version, is it current — are exactly the ones asked when a claude-cli
	// agent misbehaves, and the answer used to be split across two screens.
	{
		Name:        ClaudeToolName,
		Desc:        "Claude Code CLI — anahtarsız `claude-cli` sağlayıcısının çalıştırdığı ikili (Max/Pro aboneliğiyle). TionSwarm bunu OTOMATİK kullanır; yolu Ayarlar ▸ Sağlayıcılar'dan geçersiz kılınabilir, boşsa PATH'ten bulunur.",
		URL:         "https://github.com/anthropics/claude-code",
		Category:    "provider",
		Wire:        "provider",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Claude Code kendini arka planda günceller — çoğu zaman bir şey yapman gerekmez. Elle güncellemek için terminalde `claude update` (native kurulum) veya `npm install -g @anthropic-ai/claude-code` (npm kurulumu) çalıştır. TionSwarm bunu kendisi koşturmaz: çalışan bir claude-cli turu ikiliyi kilitler ve yarım kalan güncelleme tüm claude-cli ajanlarını durdurur.",
		},
	},
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
		Name:        "git",
		Desc:        "Git — TionSwarm oturum bağlamına çalışma dizininin branch'ini enjekte eder, `scripts\\worktree.ps1` yardımcısı ve ajanın kendi shell komutları buna dayanır. Alt-süreçler non-interactive git env alır (`GIT_EDITOR=true` → editör/pinentry asılması yok).",
		URL:         gitProjectURL,
		Category:    "dev",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		Update: wingetSpec(runtime.GOOS, "Git.Git",
			"Dağıtımının paket yöneticisiyle güncelle (ör. `sudo apt update && sudo apt install --only-upgrade git`, ya da güncel sürüm için `ppa:git-core/ppa`)."),
	},
	// node/npm carry NO GitHub release feed, and that is a measured decision rather
	// than an oversight (both were tried, 2026-08-02):
	//
	//   nodejs/node  releases/latest → v26.5.1 "(Current)". The endpoint returns the
	//     newest release by date, which is the Current line, NOT the LTS a server
	//     tool should sit on. Wiring it would flag an LTS user as "outdated" and
	//     push them off LTS — worse than saying nothing. Node's LTS state lives in
	//     nodejs.org/dist/index.json (an `lts` field), which is not a GitHub
	//     release feed and would need a second fetcher.
	//   npm/cli      releases/latest → "libnpmpack-v10.0.2" — a workspace package of
	//     the monorepo, not the npm CLI. semverRe would happily read "10.0.2" out of
	//     it and compare that against npm's real version, producing a confident and
	//     meaningless verdict.
	//
	// A non-GitHub URL makes Repo() return "" so the check reports "no release feed"
	// instead of inventing an answer. Presence + version + path is still the point:
	// on this machine they resolve inside an nvm directory, which is exactly what
	// someone debugging "why did mmdc break" needs to see.
	{
		Name:        "node",
		Desc:        "Node.js — npm tabanlı araçların (ör. mmdc) çalışma zamanı. TionSwarm doğrudan kullanmaz; ajan geliştirmede Bash ile çağırır. Sürüm karşılaştırması bilerek yapılmaz: GitHub'ın `releases/latest`'i LTS'i değil Current'ı verir.",
		URL:         "https://nodejs.org",
		Category:    "dev",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		Update:      nodeUpdateSpec(runtime.GOOS),
	},
	// bun is the third interpreter transform_data accepts (python3/node/bun). Unlike
	// node/npm it DOES have a usable release feed: oven-sh/bun publishes releases and
	// tags them "bun-v1.3.14" — semverRe reads 1.3.14 out of that, so the comparison
	// is real. Verified 2026-08-03.
	{
		Name:        "bun",
		Desc:        "Bun — `transform_data` aracının kabul ettiği üçüncü çalışma zamanı (python3/node/bun); node'dan hızlı başlar, tek dosyalık script'lerde tercih edilir.",
		URL:         "https://github.com/oven-sh/bun",
		Category:    "dev",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		Update:      bunUpdateSpec(runtime.GOOS),
	},
	{
		Name:        "npm",
		Desc:        "npm — Node paket yöneticisi. TionSwarm bunu `mmdc` güncellemesini çalıştırmak için arar (Ayarlar ▸ Harici Araçlar ▸ Güncelle); yoksa o güncelleme başarısız olur.",
		URL:         "https://www.npmjs.com",
		Category:    "dev",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		Update: UpdateSpec{
			Kind:    UpdateCommand,
			Command: "npm",
			Args:    []string{"install", "-g", "npm@latest"},
		},
	},
	// python is a REAL dependency, not an optional nicety: run_code and
	// transform_data shell out to it, and code-mode generates Python bindings.
	//
	// No release feed — python/cpython answers releases/latest with 404 (it tags
	// but does not publish releases), same as the git/git mirror. Verified rather
	// than assumed, 2026-08-02.
	{
		Name:        "python",
		Desc:        "Python — `run_code` ve `transform_data` araçlarını çalıştıran yorumlayıcı, code-mode'un ürettiği binding'ler de buna koşar. TionSwarm OTOMATİK kullanır. Windows'ta `python3.exe` genelde Microsoft Store kısayolu olduğu için önce `python` denenir.",
		URL:         "https://www.python.org",
		Category:    "dev",
		VersionArgs: []string{"--version"},
		Update:      pythonUpdateSpec(runtime.GOOS),
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
		Name:        OpenPencilToolName,
		Desc:        "OpenPencil — açık kaynak, ajan-yerlisi vektör tasarım aracı (Rust + GPU-Skia). Ajan `op` CLI'ı ile UI tasarlar, PNG/deck export eder ve tasarımı React/Vue/Svelte/Flutter/SwiftUI koduna çevirir; belgeler git dostu `.op` JSON'udur. `openpencil-design` skill'i bunu sürer. İkilinin adı `op`, ama 1Password CLI de aynı adı kullandığı için PATH'ten körlemesine alınmaz (bkz. Detect).",
		URL:         "https://github.com/ZSeven-W/openpencil",
		Category:    "design",
		Wire:        "cli",
		VersionArgs: []string{"--version"},
		// Every OpenPencil release is tagged prerelease (v0.8.0…v0.8.4, checked
		// 2026-08-15) even though they are the shipped downloads.
		PreRelease: true,
		Update: UpdateSpec{
			Kind: UpdateManual,
			Note: "Release iki ayrı arşiv taşır (`op-cli-*` + `openpencil-desktop-*`) ve ikisi de aynı sürümde olmalı; TionSwarm bunu kendisi yapmaz. Önce **`op stop`** ile çalışan sunucuyu kapat — açık MCP sunucusu `op.exe`'yi kilitler. Sonra release'ten yeni zip'leri indirip mevcut `openpencil` klasörünün üzerine aç (checksum'lar `SHA256SUMS.txt`'te). Paket yöneticisiyle kurduysan `scoop update openpencil` (Windows) ya da `brew upgrade --cask openpencil` (macOS).",
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
		Update: wingetSpec(runtime.GOOS, "Gyan.FFmpeg",
			"Dağıtımının paket yöneticisiyle güncelle (ör. `sudo apt update && sudo apt install --only-upgrade ffmpeg`). Sunucuda genelde dağıtım paketi yeterlidir; daha yeni sürüm gerekiyorsa statik build indirilir."),
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

// pathOverrides holds user-configured absolute paths, keyed by catalog name.
// Set from the settings layer (see SetPathOverride); empty by default.
var pathOverrides = struct {
	sync.RWMutex
	m map[string]string
}{m: map[string]string{}}

// SetPathOverride records the path the rest of TionSwarm will actually run for a
// tool, so this package reports on the same binary. An empty path clears the
// override and restores normal resolution.
//
// Without this, a user who points Settings ▸ Providers at a specific claude
// binary would see "bulunamadı" here (or worse, the version of a DIFFERENT
// claude on PATH) while their agents ran happily — a panel that contradicts the
// running system is worse than no panel.
func SetPathOverride(name, path string) {
	path = strings.TrimSpace(path)
	pathOverrides.Lock()
	defer pathOverrides.Unlock()
	if path == "" {
		delete(pathOverrides.m, name)
		return
	}
	pathOverrides.m[name] = path
}

// pathOverride returns the configured path for name, or "".
func pathOverride(name string) string {
	pathOverrides.RLock()
	defer pathOverrides.RUnlock()
	return pathOverrides.m[name]
}

// Detect resolves a known tool's presence + absolute path. Most tools are found
// on PATH (exec.LookPath, honouring PATHEXT on Windows), but piper and
// whisper-cli usually live OUTSIDE PATH (a Progs install), so they use the
// tts/stt resolvers (env / Progs / PATH) that the voice subsystems already rely
// on. Detection never runs the tool.
func Detect(name string) (bool, string) {
	// An explicit override wins and does NOT fall back to PATH: the override is
	// what TionSwarm executes, so if it points at nothing the honest answer is
	// "not installed", not the version of some other binary that happens to be
	// on PATH and will never be used.
	if p := pathOverride(name); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true, p
		}
		return false, ""
	}
	switch name {
	case "python":
		// Same resolver run_code/transform_data use: candidate order plus skipping
		// Microsoft Store alias stubs. A plain lookPath("python3") on Windows would
		// "find" the WindowsApps stub, and the version probe would then report
		// "Python was not found" for a machine with a perfectly good interpreter.
		if p, ok := proc.LookInterpreter(proc.PythonCandidates()...); ok {
			return true, p
		}
		return false, ""
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
	case OpenPencilToolName:
		if p := openPencilExe(); p != "" {
			return true, p
		}
		return false, ""
	}
	if p, err := lookPath(name); err == nil {
		return true, p
	}
	return false, ""
}

// openPencilExeName is OpenPencil's CLI binary. Shared by every candidate below.
func openPencilExeName() string {
	if runtime.GOOS == "windows" {
		return "op.exe"
	}
	return "op"
}

// openPencilExe resolves OpenPencil's `op` CLI: an env override, the Progs
// install layout, then PATH — but the PATH result is ACCEPTED ONLY when its
// resolved path names openpencil.
//
// That last clause is the whole point. `op` is also the 1Password CLI, which is
// widely installed and answers `--version` with a plain semver. Accepting it
// would make the panel report 1Password's version, compare it against
// ZSeven-W/openpencil's releases, and tell the user their design tool is years
// out of date — the same "confident and meaningless verdict" the node/npm entries
// above refuse to produce. Package-manager installs (scoop apps\openpencil\…,
// brew Cellar/openpencil/…) still match; anything else needs TIONSWARM_OPENPENCIL.
func openPencilExe() string {
	if p := strings.TrimSpace(os.Getenv("TIONSWARM_OPENPENCIL")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		// An override that points at nothing means "not installed", exactly as in
		// Detect: silently falling through would run a different binary than the
		// one the user configured.
		return ""
	}
	exe := openPencilExeName()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, c := range []string{
			filepath.Join(home, "Desktop", "Progs", "openpencil", "cli", exe),
			filepath.Join(home, "Desktop", "Progs", "openpencil", exe),
		} {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
		}
	}
	if p, err := lookPath("op"); err == nil && strings.Contains(strings.ToLower(p), "openpencil") {
		return p
	}
	return ""
}
