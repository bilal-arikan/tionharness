package providers

import "fmt"

// codexcliKind is the local `codex` CLI transport: keyless (ChatGPT/Codex
// subscription login inside CODEX_HOME), runs its own agentic loop, delegates
// tools via [mcp_servers] blocks in its config.toml. Model is an optional slug.
//
// Not every catalog slug is usable on every account — a ChatGPT-account login
// rejects the API-billed models — so AllowCustomModel is on and the empty
// "use the CLI default" entry stays first.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind: "codex-cli",
			// codex exec runs its own agentic loop in a subprocess with no hook
			// passthrough — neither its native shell/apply_patch tools nor
			// TionSwarm tools called over the MCP bridge run PreToolUse/PostToolUse
			// hooks (so sqz/token-optimizer PostToolUse compression is inactive too).
			AppliesToolHooks: false,
			Label:            "Codex CLI (abonelik · anahtarsız)",
			NeedsKey:         false,
			NeedsBaseURL:     false,
			AllowCustomModel: true,
			// The contract asks for Order 1 (directly after claude-cli), but that
			// slot is taken by anthropic and a duplicate Order makes the catalog
			// order depend on map iteration — i.e. nondeterministic. Parked at the
			// end until the shift of anthropic..deepseek-anthropic by one can be
			// done in a single commit; only the picker position is affected.
			Order:     8,
			Transport: TransportCLI,
			Multi:     true,
			Fields: []FieldSpec{
				{Key: FieldKeyCLIPath, Label: "CLI Yolu", Type: "path", Placeholder: "otomatik (PATH'te ara)", Help: "Bos birakilirsa PATH'teki codex ikili dosyasi kullanilir."},
				{Key: FieldKeyConfigDir, Label: "Config Dizini", Type: "dir", Help: "Bos birakilirsa workspace'in codex-home dizini kullanilir (mevcut davranis); doldurulursa bu ornege ozel, izole bir login evi kullanilir."},
			},
			Models: []ModelInfo{
				{ID: "", Label: "Varsayılan", Description: "codex oturumunun aktif modelini kullanır"},
				{ID: "gpt-5.6-sol", Label: "GPT-5.6 Sol", Description: "En yeni nesil; ~1.05M bağlam (272k üzeri uzun-bağlam fiyatlandırması)"},
				{ID: "gpt-5.6-terra", Label: "GPT-5.6 Terra", Description: "En yeni nesil; ~1.05M bağlam (272k üzeri uzun-bağlam fiyatlandırması)"},
				{ID: "gpt-5.6-luna", Label: "GPT-5.6 Luna", Description: "En yeni nesil; 400k bağlam"},
				{ID: "gpt-5.5", Label: "GPT-5.5 — varsayılan", Description: "codex-cli varsayılanı"},
				{ID: "gpt-5.4", Label: "GPT-5.4", Description: "ChatGPT hesabıyla kullanılamaz (API faturalı)"},
				{ID: "gpt-5.4-mini", Label: "GPT-5.4 Mini — hızlı", Description: "En hızlı/ucuz; basit görevler"},
				{ID: "gpt-5.2", Label: "GPT-5.2", Description: "ChatGPT hesabıyla kullanılamaz (API faturalı)"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.CodexPath != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.CodexPath == "" {
				return nil, fmt.Errorf("codex CLI not found on PATH (install the Codex CLI)")
			}
			// Model is not injected at the registry level (no app-global default
			// model); an empty model lets the agent's request model decide, falling
			// back to the CLI's own default.
			return NewCodexCLI(cfg.CodexPath, "", cfg.CodexConfigDir), nil
		},
	))
}
