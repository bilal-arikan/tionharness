package providers

import "fmt"

// claudecliKind is the local `claude` (Claude Code) CLI transport: keyless
// (OAuth/subscription login), runs its own agentic loop, delegates tools via
// --mcp-config. Model is an optional alias ("opus"/"sonnet"/"haiku").
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "claude-cli",
			Label:            "Claude CLI (abonelik · anahtarsız)",
			NeedsKey:         false,
			NeedsBaseURL:     false,
			AllowCustomModel: true,
			Order:            0,
			Models: []ModelInfo{
				{ID: "", Label: "Varsayılan", Description: "claude oturumunun aktif modelini kullanır"},
				{ID: "fable", Label: "Fable 5 — öncü", Description: "En yeni nesil; 1M bağlam, adaptif düşünme (daima açık)"},
				{ID: "opus", Label: "Opus — en güçlü", Description: "En yetenekli; en yavaş/pahalı"},
				{ID: "sonnet", Label: "Sonnet — dengeli", Description: "Hız/kalite dengesi (günlük kullanım)"},
				{ID: "haiku", Label: "Haiku — hızlı", Description: "En hızlı/ucuz; basit görevler"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.CLIPath != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.CLIPath == "" {
				return nil, fmt.Errorf("claude CLI not found on PATH (install Claude Code)")
			}
			// Model is not injected at the registry level any more (no app-global
			// default model); an empty model lets the CLI use the agent's request
			// model, falling back to its own session default.
			return NewClaudeCLI(cfg.CLIPath, "", cfg.CLIConfigDir, cfg.CLIAuthKind, cfg.CLIAuthToken), nil
		},
	))
}
