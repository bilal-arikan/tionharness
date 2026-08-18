package providers

import (
	"fmt"
	"strings"
)

// anthropicCompatKind is the generic Anthropic-compatible Messages transport
// for user-added providers (Faz 2, _Docs/71 §3): the kind-ified successor to
// the former ad hoc CustomSpec/buildCustom("anthropic") path. Every instance
// of this kind supplies its own base URL, key and default model via its
// ProviderInstance config/secrets.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "anthropic-compat",
			AppliesToolHooks: true,
			Label:            "Anthropic-uyumlu (özel)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            10,
			Transport:        TransportAPI,
			Multi:            true,
			TemplateOnly:     true,
			Fields: []FieldSpec{
				{Key: FieldKeyAPIKey, Label: "API Anahtari", Type: "password", Required: true, Secret: true, Help: "Uc noktanin API anahtari."},
				{Key: FieldKeyBaseURL, Label: "Taban URL", Type: "text", Required: true, Help: "Anthropic-uyumlu Messages uc noktasi (/messages eklenir)."},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("provider %q not configured (set an API key in Settings)", cfg.InstanceID)
			}
			endpoint := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/") + "/messages"
			return NewAnthropic(cfg.Key).WithEndpoint(cfg.InstanceID, endpoint, cfg.DefaultModel), nil
		},
	))
}
