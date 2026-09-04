package providers

import (
	"fmt"
	"strings"
)

// lmstudioDefaultBaseURL is LM Studio's local OpenAI-compatible server. The
// desktop app's "Local Server" tab serves Chat Completions on port 1234 under
// a "/v1" prefix; the client appends "/chat/completions" to this base.
const lmstudioDefaultBaseURL = "http://localhost:1234/v1"

// lmstudioKind is the LM Studio transport: models running on the user's own
// machine, reached over LM Studio's OpenAI-compatible local server. It reuses
// the shared OpenAICompat client (tool-use + streaming) so a local model drives
// the native agentic loop exactly like a hosted OpenAI-compatible provider.
//
// It differs from the generic "openai-compat" kind in the three ways that make
// a local endpoint local:
//
//  1. No API key. A localhost server has nothing to authenticate, so NeedsKey
//     is false and the key field is optional — supplied only for the case where
//     the user fronts LM Studio with a reverse proxy that does check a bearer
//     token. OpenAICompat omits the Authorization header entirely when the key
//     is empty.
//  2. A long request budget. Local inference on consumer hardware is far slower
//     than a hosted endpoint — a large prompt against a 30B model can spend
//     minutes in prompt processing alone — so the per-request wall clock is
//     raised to 30 minutes instead of the hosted default of 120s.
//  3. Zero cost. Local tokens are not billed, so pricing reports 0 rather than
//     "unknown" (see PriceFor).
//
// Like the other OpenAI-compatible kinds this is a self-registering plugin
// file: the catalog, registry resolve() and settings form all pick it up from
// its Manifest alone.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "lmstudio",
			AppliesToolHooks: true,
			Label:            "LM Studio (yerel)",
			NeedsKey:         false,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            11,
			Transport:        TransportAPI,
			Multi:            true,
			// Local inference is slow enough that the hosted 120s default would abort
			// ordinary turns. 30 minutes covers prompt processing plus generation for
			// a large model on CPU/consumer GPU.
			RequestTimeoutSecs: 1800,
			Fields: []FieldSpec{
				{
					Key:         FieldKeyBaseURL,
					Label:       "Taban URL",
					Type:        "text",
					Placeholder: lmstudioDefaultBaseURL,
					Help:        "LM Studio > Developer sekmesindeki yerel sunucu adresi. Bos birakilirsa " + lmstudioDefaultBaseURL + " kullanilir.",
				},
				{
					Key:    FieldKeyAPIKey,
					Label:  "API Anahtari (istege bagli)",
					Type:   "password",
					Secret: true,
					Help:   "LM Studio anahtar istemez. Yalnizca sunucuyu anahtar dogrulayan bir ara sunucu arkasina aldiysaniz doldurun.",
				},
			},
			// LM Studio serves whatever the user has downloaded, so there is no fixed
			// model list to curate. These are suggestions for the common local
			// tool-calling families; AllowCustomModel lets the user type any id the
			// server reports (LM Studio uses the model's own slug as the id).
			Models: []ModelInfo{
				{ID: "qwen3-coder-30b-a3b-instruct", Label: "Qwen3 Coder 30B A3B — yerel, kod", Description: "256K baglam, MoE (3B aktif); yerelde guvenilir tool-calling icin onerilen sinif. Ucretsiz."},
				{ID: "qwen3-32b", Label: "Qwen3 32B — yerel, genel", Description: "128K baglam, yogun model; genel ajan islerinde guclu. Ucretsiz."},
				{ID: "qwen3-8b", Label: "Qwen3 8B — yerel, hafif", Description: "128K baglam; hizli ve az bellek ister, ancak tool-calling guvenilirligi dusuktur — ozetleyici/alt-ajan rolu icin uygundur. Ucretsiz."},
			},
		},
		// A local endpoint needs no credential, so an instance is available as soon
		// as it exists — unlike the key-gated hosted kinds.
		func(cfg ResolvedConfig) bool { return true },
		func(cfg ResolvedConfig) (Provider, error) {
			base := strings.TrimSpace(cfg.BaseURL)
			if base == "" {
				base = lmstudioDefaultBaseURL
			}
			name := cfg.InstanceID
			if name == "" {
				return nil, fmt.Errorf("lmstudio provider not configured (missing instance id)")
			}
			return NewOpenAICompat(name, cfg.Key, base, cfg.DefaultModel).
				WithCaps(cfg.Reasoning, cfg.PromptCache), nil
		},
	))
}
