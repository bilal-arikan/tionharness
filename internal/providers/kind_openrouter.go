package providers

import "fmt"

// openrouterBaseURL is OpenRouter's OpenAI-compatible Chat Completions base.
const openrouterBaseURL = "https://openrouter.ai/api/v1"

// openrouterDefaultModel is applied when a request omits a model. A widely
// available, capable default.
const openrouterDefaultModel = "anthropic/claude-sonnet-5"

// openrouterKind is the OpenRouter transport: a single OpenAI-compatible
// endpoint that proxies hundreds of models from every major lab behind one key
// (author/model-slug ids). It reuses the shared OpenAICompat client (tool-use +
// streaming), so OpenRouter models drive the native agentic loop like MiniMax.
//
// Like the MiniMax kinds, this is a self-registering plugin file — adding it
// touches no other provider code; the catalog, registry resolve() and settings
// wiring pick it up via its Manifest and a dedicated key field.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "openrouter",
			Label:            "OpenRouter (OpenAI-uyumlu)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            4,
			// Curated suggestions: ~25 popular current models (June 2026), validated
			// against OpenRouter's live /models catalog. IDs evolve and OpenRouter
			// exposes hundreds more, so AllowCustomModel lets the user type any slug.
			Models: []ModelInfo{
				{ID: "anthropic/claude-fable-5", Label: "Claude Fable 5 — öncü", Description: "En yeni Anthropic nesli; 1M bağlam, adaptif düşünme (daima açık)"},
				{ID: "anthropic/claude-opus-5", Label: "Claude Opus 5 — en yetenekli", Description: "En yeni Anthropic amiral; 1M bağlam, öncü ajan/kodlama + computer-use"},
					{ID: "anthropic/claude-opus-4.8", Label: "Claude Opus 4.8 — önceki nesil", Description: "Önceki Anthropic amiral; karmaşık akıl yürütme + ajan"},
				{ID: "anthropic/claude-opus-4.8-fast", Label: "Claude Opus 4.8 (Fast)", Description: "Opus 4.8'in hızlı varyantı"},
				{ID: "anthropic/claude-opus-4.7", Label: "Claude Opus 4.7", Description: "Önceki Opus nesli"},
				{ID: "anthropic/claude-sonnet-5", Label: "Claude Sonnet 5 — dengeli", Description: "En yeni Anthropic dengeli nesli; 1M bağlam, güçlü ajan/kodlama"},
					{ID: "anthropic/claude-sonnet-4.6", Label: "Claude Sonnet 4.6 — önceki dengeli", Description: "Güçlü ve hızlı; önceki nesil dengeli model"},
				{ID: "anthropic/claude-haiku-4.5", Label: "Claude Haiku 4.5 — hızlı/ucuz", Description: "Düşük gecikme, yüksek hacim"},
				{ID: "openai/gpt-5.5", Label: "GPT-5.5", Description: "OpenAI amiral genel-amaçlı"},
				{ID: "openai/gpt-5.5-pro", Label: "GPT-5.5 Pro", Description: "GPT-5.5'in en güçlü katmanı"},
				{ID: "google/gemini-3.5-flash", Label: "Gemini 3.5 Flash — hızlı", Description: "Google hızlı çok-kipli model"},
				{ID: "google/gemini-3.1-flash-lite", Label: "Gemini 3.1 Flash Lite", Description: "Çok hızlı/ucuz Google katmanı"},
				{ID: "deepseek/deepseek-v4-flash", Label: "DeepSeek V4 Flash", Description: "1M bağlam, yüksek hacim lideri"},
				{ID: "deepseek/deepseek-v4-pro", Label: "DeepSeek V4 Pro", Description: "DeepSeek'in güçlü katmanı"},
				{ID: "x-ai/grok-4.3", Label: "Grok 4.3", Description: "xAI amiral model"},
				{ID: "minimax/minimax-m3", Label: "MiniMax M3", Description: "MiniMax'in güncel akıl-yürütme/kodlama modeli"},
				{ID: "moonshotai/kimi-k2.6", Label: "Kimi K2.6", Description: "Moonshot; geliştiriciler arası saygın"},
				{ID: "qwen/qwen3.7-max", Label: "Qwen3.7 Max", Description: "Alibaba en güçlü Qwen katmanı"},
				{ID: "qwen/qwen3.7-plus", Label: "Qwen3.7 Plus", Description: "Dengeli Qwen katmanı"},
				{ID: "nvidia/nemotron-3-ultra-550b-a3b", Label: "Nemotron 3 Ultra", Description: "NVIDIA 550B hibrit MoE"},
				{ID: "stepfun/step-3.7-flash", Label: "Step 3.7 Flash", Description: "StepFun hızlı kodlama modeli"},
				{ID: "z-ai/glm-5.2", Label: "GLM 5.2", Description: "Z.ai amiral model"},
				{ID: "mistralai/mistral-medium-3.5", Label: "Mistral Medium 3.5", Description: "Mistral dengeli model"},
				{ID: "tencent/hy3-preview", Label: "Tencent Hy3 (preview)", Description: "Tencent Hunyuan 3 önizleme"},
				{ID: "xiaomi/mimo-v2.5", Label: "MiMo V2.5", Description: "Xiaomi popüler kodlama modeli"},
				{ID: "openrouter/owl-alpha", Label: "Owl Alpha", Description: "OpenRouter topluluk modeli"},
				{ID: "anthropic/claude-opus-4.7-fast", Label: "Claude Opus 4.7 (Fast)", Description: "Opus 4.7'nin hızlı varyantı"},
				{ID: "google/gemini-3-pro-image", Label: "Gemini 3 Pro (çok-kipli)", Description: "Google Gemini 3 Pro — görsel + metin"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("openrouter provider not configured (set the OpenRouter API key in Settings)")
			}
			base := cfg.BaseURL
			if base == "" {
				base = openrouterBaseURL
			}
			return NewOpenAICompat("openrouter", cfg.Key, base, openrouterDefaultModel), nil
		},
	))
}
