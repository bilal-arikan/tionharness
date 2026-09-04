package providers

import "strings"

// Context-window and output caps for the open-weight model families people run
// locally (LM Studio, Ollama, llama.cpp, vLLM). They live in their own file
// because their sizing rule differs from the hosted families in one important
// way: the advertised window is an upper bound the user's own hardware usually
// does NOT grant. LM Studio loads a model with an explicit context length
// chosen at load time — commonly far below the model's maximum, because KV
// cache memory scales with it — and a request over that length is rejected or
// silently truncated by the server.
//
// So these values are deliberately the conservative LOADED size a user is
// likely to have configured, not the family maximum. Over-estimating here is
// the dangerous direction: compaction would fire too late and the turn would
// die at the server instead of folding cleanly.
const (
	// Qwen3 advertises 32K natively and 128K–256K with YaRN rope scaling, but LM
	// Studio's default load for these files is 32K and raising it costs GBs of KV
	// cache. 32K is what an unmodified local setup actually serves.
	windowLocalQwen = 32_768
	// Llama 3.x advertises 128K; the same default-load argument applies.
	windowLocalLlama = 32_768
	// Mistral / Mixtral / Devstral: 32K is the common native and loaded size.
	windowLocalMistral = 32_768
	// Gemma 2/3 ship an 8K native window (Gemma 3 extends to 128K, but the 8K
	// files remain the common local download), so it keeps the smallest value.
	windowLocalGemma = 8_192
	// Phi-3/4 advertise 128K in the "-128k" variants and 4K otherwise; the plain
	// files are the common case.
	windowLocalPhi = 16_384
)

const (
	// Local generation caps. These sit far below the context window because a
	// local server must fit prompt AND generation in the one loaded context: an
	// output cap near the window size would leave no room for the prompt. They
	// also stay well above the providers' 4096 fallback so ordinary answers
	// finish in a single call.
	maxOutLocalStandard = 8_192
	maxOutLocalSmall    = 4_096 // Gemma's 8K window cannot spare more
)

// localModelFamily reports whether the slug names an open-weight family that is
// typically run locally, and returns its conservative window/output pair. The
// match is on the model slug alone, so it holds whether the model is served by
// LM Studio, Ollama or a hosted endpoint offering the same weights.
//
// Order matters: "qwen3-coder" and "deepseek-r1-distill-qwen" both contain
// "qwen", and the distills carry Qwen's window rather than the hosted DeepSeek
// V4 window — so this function is consulted for the local kinds BEFORE the
// hosted family table, which would otherwise claim 1M for any slug containing
// "deepseek".
func localModelFamily(m string) (window, maxOut int, ok bool) {
	switch {
	case strings.Contains(m, "qwen"):
		return windowLocalQwen, maxOutLocalStandard, true
	case strings.Contains(m, "llama"):
		return windowLocalLlama, maxOutLocalStandard, true
	case strings.Contains(m, "mistral"), strings.Contains(m, "mixtral"), strings.Contains(m, "devstral"), strings.Contains(m, "magistral"):
		return windowLocalMistral, maxOutLocalStandard, true
	case strings.Contains(m, "gemma"):
		return windowLocalGemma, maxOutLocalSmall, true
	case strings.Contains(m, "phi-"), strings.Contains(m, "phi3"), strings.Contains(m, "phi4"):
		return windowLocalPhi, maxOutLocalStandard, true
	}
	return 0, 0, false
}

// localProvider reports whether the provider kind serves models from the user's
// own machine. Only these kinds consult localModelFamily first — a hosted
// endpoint serving the same open weights (OpenRouter routing to Qwen, say) is
// not constrained by a local KV-cache budget and keeps the hosted sizing.
func localProvider(provider string) bool {
	return provider == "lmstudio"
}

// localWindowFor returns the local window/output pair when the provider is a
// local one AND the slug is a recognised open-weight family. It is the hook
// ContextWindowFor / MaxOutputFor / AdaptiveBudgetFraction consult first.
func localWindowFor(provider, model string) (window, maxOut int, ok bool) {
	if !localProvider(provider) {
		return 0, 0, false
	}
	return localModelFamily(strings.ToLower(strings.TrimSpace(model)))
}
