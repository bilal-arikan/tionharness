package providers

import "fmt"

// claudecliKind is the local `claude` (Claude Code) CLI transport: keyless
// (OAuth/subscription login), runs its own agentic loop, delegates tools via
// --mcp-config. Model is an optional alias ("opus"/"sonnet"/"haiku").
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "claude-cli",
			AppliesToolHooks: true,
			Label:            "Claude CLI (abonelik · anahtarsız)",
			NeedsKey:         false,
			NeedsBaseURL:     false,
			AllowCustomModel: true,
			Order:            0,
			Transport:        TransportCLI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyCLIPath, Label: "CLI Yolu", Type: "path", Placeholder: "otomatik (PATH'te ara)", Help: "Bos birakilirsa PATH'teki claude ikili dosyasi kullanilir."},
				{Key: FieldKeyConfigDir, Label: "Config Dizini", Type: "dir", Help: "Bos birakilirsa workspace'in claude-home dizini kullanilir (mevcut davranis); doldurulursa bu ornege ozel, izole bir login evi kullanilir."},
				{Key: FieldKeyAuthKind, Label: "Kimlik Dogrulama Turu", Type: "select", Options: []string{"", "oauth", "apikey"}, Help: "Bos = login'siz/varsayilan."},
				{Key: FieldKeyAuthToken, Label: "Kimlik Dogrulama Bilgisi", Type: "password", Secret: true, Help: "authKind secildiyse ilgili token/anahtar."},
			},
			Models: []ModelInfo{
				{ID: "", Label: "claude oturum modeli", Description: "claude oturumunun aktif modelini kullanır"},
				{ID: "fable", Label: "Fable — öncü", Description: "CLI'nin güncel Fable'ı (5.1); 1M bağlam, adaptif düşünme (daima açık)"},
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
