package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// Per-workspace, editable configuration lives in <workspace>/config/, a sibling
// of the store/ (database) and workspace/ (agent sandbox) directories:
//
//	<workspace>/
//	├── config/
//	│   ├── prompts/<key>.md   overrides for central-registry runtime prompts
//	│   │                      (keys: internal/prompts — summary/title/compact/
//	│   │                      handoff/continuation/btw-*/lesson/…)
//	│   ├── instructions.md    workspace-wide agent guidance
//	│   └── README.md          human notes (free-form)
//	├── store/
//	└── workspace/
//
// Both the user (directly on disk) and the app (Settings UI / runtime) edit
// these files. The runtime reads each prompt with the compiled-in default as a
// fallback, so a missing or blank file never breaks a turn.

// PromptKeys lists the editable runtime prompt keys in display order. The set
// is the central registry (internal/prompts); this alias keeps existing
// callers (api DTOs, market publish, templates) on one source of truth.
var PromptKeys = prompts.Keys()

// PromptDefault returns the compiled-in default text for a prompt key ("" if the
// key is unknown).
func PromptDefault(key string) string { return prompts.Default(key) }

// CompactPromptTemplate returns this workspace's editable compaction prompt (the
// config/prompts/compact.md override, or the registry default when missing/
// blank/invalid). Callers inject it onto the turn context via conversation.WithCompactPrompt
// so the shared Manager honors the workspace's edit. Exported (vs readPrompt) so
// the api package can reach it without the local `agent` variable shadowing the
// package name at those call sites.
func (r *Runtime) CompactPromptTemplate() string {
	compactor, _, err := r.ResolveSystemAgent("compaction")
	if err != nil {
		return r.readPrompt("compact")
	}
	if prompts.Validate("compact", compactor.Soul) != nil {
		return prompts.Default("compact")
	}
	return compactor.Soul
}

// WorkspaceConfigDir returns the config directory for a workspace data dir.
func WorkspaceConfigDir(wsDir string) string { return filepath.Join(wsDir, "config") }

// PromptFilePath returns the on-disk path of a workspace prompt file.
func PromptFilePath(wsDir, key string) string {
	return filepath.Join(WorkspaceConfigDir(wsDir), "prompts", key+".md")
}

// InstructionsFilePath returns the workspace instructions file path.
func InstructionsFilePath(wsDir string) string {
	return filepath.Join(WorkspaceConfigDir(wsDir), "instructions.md")
}

// ReadmeFilePath returns the workspace README file path.
func ReadmeFilePath(wsDir string) string {
	return filepath.Join(WorkspaceConfigDir(wsDir), "README.md")
}

// configDir returns this runtime's workspace config directory, or "" when the
// work dir is unknown (e.g. a bare runtime in tests) — callers then use the
// compiled-in defaults.
func (r *Runtime) configDir() string {
	if r.workDir == "" {
		return ""
	}
	return WorkspaceConfigDir(filepath.Dir(r.workDir))
}

// readPrompt returns the workspace-local prompt for key, falling back to the
// registry default when the work dir is unknown or the override file is
// missing, blank, or fails placeholder validation (see prompts.ResolveFile).
func (r *Runtime) readPrompt(key string) string {
	dir := r.configDir()
	if dir == "" {
		return prompts.Default(key)
	}
	return prompts.ResolveFile(filepath.Join(dir, "prompts", key+".md"), key)
}

// WorkspacePrompt resolves a registry prompt for a workspace data dir — the
// same override→default resolution readPrompt applies, exported for callers
// outside a Runtime (the api layer's coordinator prefix, for one).
func WorkspacePrompt(wsDir, key string) string {
	if wsDir == "" {
		return prompts.Default(key)
	}
	return prompts.ResolveFile(PromptFilePath(wsDir, key), key)
}

// readmeTemplate is seeded into a fresh workspace as config/README.md so the
// folder is self-documenting. Docs are Turkish per project convention.
const readmeTemplate = `# Workspace Yapılandırması

Bu klasör bu workspace'e özel, **düzenlenebilir** dosyaları içerir. Hem sen
(doğrudan diskten) hem de uygulama (Ayarlar ekranı / ajanlar) bu dosyaları
değiştirebilir.

- ` + "`prompts/<anahtar>.md`" + ` — merkezi prompt kayıt defterindeki (registry) bir
  runtime promptunu bu workspace için geçersiz kılar. Anahtar listesi ve her
  promptun açıklaması Ayarlar → Promptlar & Dosyalar ekranındadır (summary,
  title, compact, handoff, continuation, btw-*, lesson, insight-analyzer,
  auto-continue, coordinator, subagent-*).
- ` + "`instructions.md`" + ` — bu workspace'teki tüm ajanlara eklenen yönergeler

Bir prompt dosyası yoksa/boşsa uygulama **gömülü varsayılanı** kullanır. Bazı
promptlar ` + "`{{yerTutucu}}`" + ` alanları içerir — bunlar korunmalıdır; eksikse dosya
yok sayılıp varsayılana düşülür.
`

// SeedWorkspaceConfig creates the config/ tree for a workspace if absent,
// writing the instructions file (from the given current instructions) and a
// README. Prompt defaults are deliberately NOT seeded as files anymore: a file
// identical to the shipped default would freeze the workspace on that text and
// silently mask future default improvements (the drift the skills seeder
// versions its way around). Resolution falls back to the embedded default when
// no file exists, so only a REAL user edit lives on disk. Files an older build
// seeded verbatim are cleaned up here for the same reason; edited files are
// never touched.
func SeedWorkspaceConfig(wsDir, instructions string) error {
	if wsDir == "" {
		return nil
	}
	promptsDir := filepath.Join(WorkspaceConfigDir(wsDir), "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return err
	}
	for _, key := range PromptKeys {
		removeIfDefault(PromptFilePath(wsDir, key), key)
	}
	if err := writeIfAbsent(InstructionsFilePath(wsDir), instructions); err != nil {
		return err
	}
	return writeIfAbsent(ReadmeFilePath(wsDir), readmeTemplate)
}

// removeIfDefault deletes a prompt override file whose content matches the
// current embedded default (an old-build seed artifact, not a user edit). The
// comparison runs after legacy normalization, so a pre-registry compact.md
// whose only difference is the %s→{{...}} placeholder form is cleaned up too.
// Best-effort: any error just leaves the file in place, which is harmless.
func removeIfDefault(path, key string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	got := prompts.NormalizeLegacy(key, strings.TrimSpace(string(data)))
	if got == strings.TrimSpace(prompts.Default(key)) {
		_ = os.Remove(path)
	}
}

// writeIfAbsent writes content to path only when the file does not yet exist.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already present — keep user edits
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
