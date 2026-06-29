package providers

import "fmt"

// antigravityKind is the local `agy` (Google Antigravity CLI) transport: it
// mirrors the keyless claude-cli plugin, driving the CLI in headless --print
// mode and capturing its plain-text answer. Auth is delegated to the user's agy
// configuration (interactive Google Sign-In on first run, or ANTIGRAVITY_API_KEY
// when supported). Availability = the agy binary on PATH. Model is optional —
// agy auto-selects a tier (Flash by default) when none is given. Phase 1 is chat
// only (no MCP tool delegation — agy's mcp_config.json model differs from claude's).
//
// EXPERIMENTAL — blocked on upstream bug google-antigravity/antigravity-cli#76:
// `agy --print` silently drops its answer (zero bytes, exit 0, hangs ignoring
// --print-timeout) whenever stdout is NOT a TTY — which is exactly how SwarmGo
// spawns it (exec + piped stdout). Confirmed on Windows 11; there is no env-var
// workaround and `--output-format json` is rejected in agy v1.x. The provider
// code is correct and ready: it works the moment agy ships non-TTY stdout (or a
// structured --output-format), or if it is run through a pseudo-terminal
// (Unix `script -qec`, Windows ConPTY). Until then this kind is labelled DENEYSEL
// and selecting it will fail fast with an actionable error (see Complete).
type antigravityKind struct{}

func (antigravityKind) Manifest() Manifest {
	return Manifest{
		Kind:             "antigravity-cli",
		Label:            "Antigravity CLI (agy · DENEYSEL — upstream #76)",
		NeedsKey:         false,
		NeedsBaseURL:     false,
		AllowCustomModel: true,
		Order:            5,
		// agy auto-selects when no model is given; these are the tiers agy exposes
		// (via `agy models` / the in-TUI /model switcher). IDs may evolve —
		// AllowCustomModel lets the user type any. "" = let agy auto-select.
		Models: []ModelInfo{
			{ID: "", Label: "Varsayılan (agy auto)", Description: "agy uygun katmanı seçer (varsayılan Flash)"},
			{ID: "gemini-3.5-flash", Label: "Gemini 3.5 Flash — hızlı", Description: "Düşük gecikme varsayılan katman"},
			{ID: "gemini-3.1-pro", Label: "Gemini 3.1 Pro — güçlü", Description: "Daha yetenekli Gemini katmanı"},
			{ID: "claude-sonnet", Label: "Claude Sonnet — dengeli", Description: "agy üzerinden Anthropic Sonnet"},
			{ID: "claude-opus", Label: "Claude Opus — en güçlü", Description: "agy üzerinden Anthropic Opus"},
			{ID: "gpt-oss-120b", Label: "GPT-OSS 120B", Description: "agy üzerinden açık-ağırlık GPT-OSS"},
		},
	}
}

func (antigravityKind) Available(cfg ResolvedConfig) bool { return cfg.AntigravityCLIPath != "" }

func (antigravityKind) Build(cfg ResolvedConfig) (Provider, error) {
	if cfg.AntigravityCLIPath == "" {
		return nil, fmt.Errorf("antigravity CLI (agy) not found on PATH (install from https://antigravity.google/docs/cli-install)")
	}
	// agy auto-selects a model when none is given, so an empty/foreign global
	// default is simply passed through as "" (no --model) rather than forced.
	model := cfg.Model
	if !looksLikeAntigravityModel(model) {
		model = ""
	}
	return NewAntigravityCLI(cfg.AntigravityCLIPath, model, cfg.AntigravityKey), nil
}

// looksLikeAntigravityModel reports whether a model id belongs to a tier agy
// exposes, so a foreign global default (e.g. a claude-cli alias) is dropped to ""
// (let agy auto-select) instead of being forwarded as a bad --model value.
func looksLikeAntigravityModel(id string) bool {
	switch id {
	case "gemini-3.5-flash", "gemini-3.1-pro", "claude-sonnet", "claude-opus", "gpt-oss-120b":
		return true
	}
	return false
}

func init() { RegisterKind(antigravityKind{}) }
