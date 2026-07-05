package agent

import (
	"path/filepath"
	"runtime"
)

// PromptInfo describes one built-in prompt used by the runtime's utility
// operations (on-demand summaries, reflection, titling), for read-only display
// in the settings UI. The prompts themselves are compiled-in constants — this
// is purely a viewer payload.
type PromptInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	File   string `json:"file"`   // source file basename where the prompt lives
	System string `json:"system"` // system prompt (empty when none)
	User   string `json:"user"`   // user-turn template (placeholders shown literally)
	Note   string `json:"note"`   // short usage note
}

// PromptsDir returns the absolute directory holding the prompt source files, as
// recorded at build time via runtime.Caller. On a locally-built binary this
// resolves to the repository's internal/agent folder, so the UI's "open folder"
// button lands on the actual sources.
func PromptsDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Dir(file)
	}
	return ""
}

// Prompts returns the built-in prompt set for read-only display.
func Prompts() []PromptInfo {
	return []PromptInfo{
		{
			Key:    "summary",
			Label:  "Özet komutları — /board · /flows",
			File:   "summarizer.go",
			System: summarySystemPrompt,
			User:   "Summarize the following <etiket> for the user:\n\n<toplanan veri>",
			Note:   "Veri sunucuda toplanır (<etiket> = görev / akış), en fazla 40 öğe. Ucuz title-modeli varsa o, yoksa ajanın modeli kullanılır.",
		},
		{
			Key:    "title",
			Label:  "Otomatik başlık (auto-title)",
			File:   "titler.go",
			System: titleSystemPrompt,
			Note:   "Sohbetin ilk turunda ve görev oluşturmada başlık üretir; Ayarlar'daki başlık modeli (ucuz) kullanılabilir.",
		},
	}
}
