package providers

import "fmt"

// openaiCompatKind is the generic OpenAI-compatible Chat Completions transport
// for user-added providers (Faz 2, _Docs/71 §3): the kind-ified successor to
// the former ad hoc CustomSpec/buildCustom("openai") path. Every instance of
// this kind supplies its own base URL, key, default model and capability
// flags via its ProviderInstance config/secrets — there is exactly one
// registered kind regardless of how many "openai-compat" instances exist.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "openai-compat",
			AppliesToolHooks: true,
			Label:            "OpenAI-uyumlu (özel)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            9,
			Transport:        TransportAPI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyAPIKey, Label: "API Anahtari", Type: "password", Required: true, Secret: true, Help: "Uc noktanin API anahtari."},
				{Key: FieldKeyBaseURL, Label: "Taban URL", Type: "text", Required: true, Help: "OpenAI-uyumlu Chat Completions uc noktasi."},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("provider %q not configured (set an API key in Settings)", cfg.InstanceID)
			}
			return NewOpenAICompat(cfg.InstanceID, cfg.Key, cfg.BaseURL, cfg.DefaultModel).
				WithCaps(cfg.Reasoning, cfg.PromptCache), nil
		},
	))
}
