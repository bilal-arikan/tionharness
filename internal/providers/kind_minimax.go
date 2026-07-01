package providers

import "fmt"

// minimaxKind is the MiniMax (OpenAI-compatible Chat Completions) transport.
// The underlying OpenAICompat client works against any OpenAI-compatible
// endpoint via a custom base URL and now supports tool-use (OpenAI
// tools/tool_calls), so MiniMax models can drive the native agentic loop.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "minimax",
			Label:            "MiniMax (OpenAI-uyumlu)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            2,
			Models: []ModelInfo{
				{ID: "MiniMax-M3", Label: "MiniMax M3 - guncel amiral", Description: "En yeni akil yurutme/kodlama modeli (varsayilan)"},
				{ID: "MiniMax-M2.7", Label: "MiniMax M2.7 - onceki nesil", Description: "Bir onceki hosted akil-yurutme modeli"},
				{ID: "MiniMax-M2.7-highspeed", Label: "MiniMax M2.7 Highspeed - hizli", Description: "M2.7'nin hizli katmani"},
				{ID: "MiniMax-M2.5", Label: "MiniMax M2.5", Description: "Dengeli akil yurutme katmani"},
				{ID: "MiniMax-M2.5-highspeed", Label: "MiniMax M2.5 Highspeed - hizli", Description: "M2.5'in hizli katmani"},
				{ID: "MiniMax-M2.1", Label: "MiniMax M2.1 — çok dilli kodlama", Description: "Güçlü kodlama, çok dil (~60 tps)"},
				{ID: "MiniMax-M2.1-lightning", Label: "MiniMax M2.1 Lightning — hızlı", Description: "M2.1'in hızlı varyantı (~100 tps)"},
				{ID: "MiniMax-M2", Label: "MiniMax M2 — ajan/akıl yürütme", Description: "Ajan yetenekleri + gelişmiş akıl yürütme"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("minimax provider not configured (set an API key in Settings)")
			}
			return NewMinimax(cfg.Key, cfg.BaseURL), nil
		},
	))
}
