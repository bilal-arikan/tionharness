package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
)

// Per-workspace, editable configuration lives in <workspace>/config/, a sibling
// of the store/ (database) and workspace/ (agent sandbox) directories:
//
//	<workspace>/
//	├── config/
//	│   ├── prompts/{summary,reflect,title}.md   runtime utility prompts
//	│   ├── instructions.md                      workspace-wide agent guidance
//	│   └── README.md                            human notes (free-form)
//	├── store/
//	└── workspace/
//
// Both the user (directly on disk) and the app (Settings UI / runtime) edit
// these files. The runtime reads each prompt with the compiled-in default as a
// fallback, so a missing or blank file never breaks a turn.

// PromptKeys lists the editable runtime prompt keys in display order.
var PromptKeys = []string{"summary", "reflect", "title", "compact"}

// promptDefaults maps a prompt key to its compiled-in default text, used both to
// seed a fresh workspace and as the fallback when a file is missing or blank.
// "compact" is the conversation-compaction template; its default carries two %s
// slots (existing summary, new messages) that an edit MUST preserve — the
// compaction core validates and falls back to the default if they are broken.
var promptDefaults = map[string]string{
	"summary": summarySystemPrompt,
	"title":   titleSystemPrompt,
	"compact": conversation.CompactPromptDefault(),
}

// PromptDefault returns the compiled-in default text for a prompt key ("" if the
// key is unknown).
func PromptDefault(key string) string { return promptDefaults[key] }

// CompactPromptTemplate returns this workspace's editable compaction prompt (the
// config/prompts/compact.md override, or the compiled-in default when missing/
// blank). Callers inject it onto the turn context via conversation.WithCompactPrompt
// so the shared Manager honors the workspace's edit. Exported (vs readPrompt) so
// the api package can reach it without the local `agent` variable shadowing the
// package name at those call sites.
func (r *Runtime) CompactPromptTemplate() string { return r.readPrompt("compact") }

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
// compiled-in default when the work dir is unknown or the file is missing/blank.
func (r *Runtime) readPrompt(key string) string {
	def := promptDefaults[key]
	dir := r.configDir()
	if dir == "" {
		return def
	}
	data, err := os.ReadFile(filepath.Join(dir, "prompts", key+".md"))
	if err != nil {
		return def
	}
	if s := strings.TrimSpace(string(data)); s != "" {
		return s
	}
	return def
}

// readmeTemplate is seeded into a fresh workspace as config/README.md so the
// folder is self-documenting. Docs are Turkish per project convention.
const readmeTemplate = `# Workspace Yapılandırması

Bu klasör bu workspace'e özel, **düzenlenebilir** dosyaları içerir. Hem sen
(doğrudan diskten) hem de uygulama (Ayarlar ekranı / ajanlar) bu dosyaları
değiştirebilir.

- ` + "`prompts/summary.md`" + ` — ` + "`/memory` · `/board` · `/flows`" + ` özet komutlarının sistem promptu
- ` + "`prompts/reflect.md`" + ` — ` + "`/reflect`" + ` (dream cycle) yansıma promptu
- ` + "`prompts/title.md`" + ` — otomatik başlık üretimi sistem promptu
- ` + "`prompts/compact.md`" + ` — bağlam sıkıştırma (compaction) promptu — **iki ` + "`%s`" + ` yer tutucusu** (mevcut özet, yeni mesajlar) korunmalı; bozuksa gömülü varsayılana düşer
- ` + "`instructions.md`" + ` — bu workspace'teki tüm ajanlara eklenen yönergeler

Bir prompt dosyasını boş bırakırsan uygulama **gömülü varsayılanı** kullanır.
`

// SeedWorkspaceConfig creates the config/ tree for a workspace if absent,
// writing each prompt default, the instructions file (from the given current
// instructions) and a README. Existing files are never overwritten, so user
// edits survive restarts.
func SeedWorkspaceConfig(wsDir, instructions string) error {
	if wsDir == "" {
		return nil
	}
	promptsDir := filepath.Join(WorkspaceConfigDir(wsDir), "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return err
	}
	for _, key := range PromptKeys {
		if err := writeIfAbsent(PromptFilePath(wsDir, key), promptDefaults[key]); err != nil {
			return err
		}
	}
	if err := writeIfAbsent(InstructionsFilePath(wsDir), instructions); err != nil {
		return err
	}
	return writeIfAbsent(ReadmeFilePath(wsDir), readmeTemplate)
}

// writeIfAbsent writes content to path only when the file does not yet exist.
func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already present — keep user edits
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
