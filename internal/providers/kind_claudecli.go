package providers

import "fmt"

// claudecliKind is the local `claude` (Claude Code) CLI transport: keyless
// (OAuth/subscription login), runs its own agentic loop, delegates tools via
// --mcp-config. Model is an optional alias ("opus"/"sonnet"/"haiku").
type claudecliKind struct{}

func (claudecliKind) Manifest() Manifest {
	return Manifest{
		Kind:             "claude-cli",
		Label:            "Claude CLI (abonelik · anahtarsız)",
		NeedsKey:         false,
		NeedsBaseURL:     false,
		AllowCustomModel: true,
		Order:            0,
		Models: []ModelInfo{
			{ID: "", Label: "Varsayılan (oturum modeli)", Description: "claude oturumunun aktif modelini kullanır"},
			{ID: "opus", Label: "Opus — en güçlü", Description: "En yetenekli; en yavaş/pahalı"},
			{ID: "sonnet", Label: "Sonnet — dengeli", Description: "Hız/kalite dengesi (günlük kullanım)"},
			{ID: "haiku", Label: "Haiku — hızlı", Description: "En hızlı/ucuz; basit görevler"},
		},
	}
}

func (claudecliKind) Available(cfg ResolvedConfig) bool { return cfg.CLIPath != "" }

func (claudecliKind) Build(cfg ResolvedConfig) (Provider, error) {
	if cfg.CLIPath == "" {
		return nil, fmt.Errorf("claude CLI not found on PATH (install Claude Code)")
	}
	return NewClaudeCLI(cfg.CLIPath, cfg.Model, cfg.CLIConfigDir, cfg.CLIAuthKind, cfg.CLIAuthToken), nil
}

func init() { RegisterKind(claudecliKind{}) }
