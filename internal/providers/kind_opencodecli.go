package providers

import "fmt"

// opencodecliKind is the local `opencode` CLI transport (anomalyco/opencode):
// one subprocess per turn (`opencode run --format json`), its own agentic loop,
// TionHarness tools over the MCP bridge. It is the third CLI transport next to
// claude-cli and codex-cli and the one that reaches every OTHER model vendor:
// opencode speaks to 75+ providers through its own provider layer, so an
// OpenRouter, DeepSeek, local Ollama/LM Studio or OpenAI API-key account is
// configured ONCE inside opencode (`opencode auth login` in this instance's
// home) instead of as a TionHarness-native HTTP kind.
//
// Model ids are opencode's `provider/model` form ("anthropic/claude-sonnet-5-5",
// "openai/gpt-5.5", "opencode/big-pickle"). The empty id defers to the
// opencode home's own default model.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind: "opencode-cli",
			// `opencode run` runs its own loop with no hook passthrough: neither
			// its native tools nor the bridged TionHarness tools fire
			// PreToolUse/PostToolUse hooks.
			AppliesToolHooks: false,
			Label:            "OpenCode CLI (çoklu sağlayıcı · anahtar opencode'da)",
			NeedsKey:         false,
			AllowCustomModel: true,
			Order:            12,
			Transport:        TransportCLI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyCLIPath, Label: "CLI Yolu", Type: "path", Placeholder: "otomatik (PATH'te ara)", Help: "Bos birakilirsa PATH'teki opencode ikili dosyasi kullanilir."},
				{Key: FieldKeyConfigDir, Label: "Config Dizini", Type: "dir", Help: "Bos birakilirsa uygulamanin opencode-home dizini kullanilir; doldurulursa bu ornege ozel, izole bir opencode evi (auth + oturum veritabani) kullanilir."},
				{Key: FieldKeyEffortVariant, Label: "Efor Varyantı", Type: "select", Options: []string{"", "auto"}, Help: "auto: ajanin dusunme seviyesi `--variant` olarak gecirilir (yalniz anthropic/ ve openai/ modelleri). Bos: gecirilmez."},
			},
			Models: []ModelInfo{
				{ID: "", Label: "opencode varsayılan modeli", Description: "opencode evinin kendi varsayılan modelini kullanır (opencode.json `model`)"},
				{ID: "anthropic/claude-sonnet-5-5", Label: "Anthropic · Claude Sonnet 5.5", Description: "Anthropic API anahtarı opencode evinde (`opencode auth login`). Abonelik (Pro/Max) girişi opencode'da YASAK; bunun için claude-cli kullanın."},
				{ID: "anthropic/claude-opus-5-5", Label: "Anthropic · Claude Opus 5.5", Description: "Anthropic API anahtarı ile."},
				{ID: "openai/gpt-5.5", Label: "OpenAI · GPT-5.5", Description: "OpenAI API anahtarı veya ChatGPT girişi ile."},
				{ID: "openrouter/anthropic/claude-sonnet-5-5", Label: "OpenRouter · Claude Sonnet 5.5", Description: "OpenRouter anahtarı opencode evinde."},
				{ID: "deepseek/deepseek-chat", Label: "DeepSeek · chat", Description: "DeepSeek anahtarı opencode evinde."},
				{ID: "opencode/big-pickle", Label: "OpenCode Zen · big-pickle (ücretsiz)", Description: "Anahtarsız ücretsiz deneme modeli; kalite düşük, bağlantı testi için."},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.OpenCodePath != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.OpenCodePath == "" {
				return nil, fmt.Errorf("opencode CLI not found on PATH (install opencode: npm i -g opencode-ai)")
			}
			p := NewOpenCodeCLI(cfg.OpenCodePath, cfg.DefaultModel, cfg.OpenCodeConfigDir)
			p.effortVariant = cfg.Values[FieldKeyEffortVariant] == "auto"
			return p, nil
		},
	))
}
