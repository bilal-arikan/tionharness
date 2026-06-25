package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	minimaxDefaultBaseURL = "https://api.minimax.io/v1"
	minimaxDefaultModel   = "MiniMax-M2.1"
)

// OpenAICompat is a thin client for any OpenAI-compatible Chat Completions API
// (MiniMax, OpenRouter, Gemini's OpenAI-compat endpoint, Kimi/Moonshot, Groq,
// Ollama, ...). It is configured by base URL + Bearer key, so a single
// implementation serves every such provider.
//
// Complete supports tool-use (OpenAI tools / tool_calls), so models behind these
// endpoints can drive the native agentic loop. Stream is text-only (tool-call
// streaming deltas are not assembled); the agent loop uses Complete whenever a
// turn carries tools, so this does not limit agent behaviour.
type OpenAICompat struct {
	name         string
	apiKey       string
	baseURL      string
	defaultModel string
	client       *http.Client
	// reasoning, when true, sends a `reasoning_effort` field (mapped from the
	// request's ThinkingBudget) so reasoning-capable models honour the agent's
	// thinking level. Off by default: many endpoints 400 on an unknown param.
	reasoning bool
	// cacheMode is "native" | "auto" | "none" | "". "native" attaches an
	// Anthropic-style cache_control breakpoint to the system prefix (forwarded by
	// proxies like OpenRouter to Anthropic/Gemini backends).
	cacheMode string
}

// WithCaps sets the optional capability flags (reasoning-effort passthrough and
// prompt-cache mode) and returns the client for chaining. Used by buildCustom to
// apply a market provider pack's declared capabilities.
func (m *OpenAICompat) WithCaps(reasoning bool, promptCache string) *OpenAICompat {
	m.reasoning = reasoning
	m.cacheMode = strings.TrimSpace(promptCache)
	return m
}

// NewOpenAICompat creates a client. An empty baseURL falls back to MiniMax's
// public endpoint; an empty name defaults to "openai". defaultModel is applied
// when a request omits one (may be "").
func NewOpenAICompat(name, apiKey, baseURL, defaultModel string) *OpenAICompat {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = minimaxDefaultBaseURL
	}
	if name == "" {
		name = "openai"
	}
	return &OpenAICompat{
		name:         name,
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: requestTimeoutSecs * time.Second},
	}
}

// NewMinimax creates an OpenAI-compatible client identified as "minimax" with
// the MiniMax default model. Retained for the minimax kind and existing tests.
func NewMinimax(apiKey, baseURL string) *OpenAICompat {
	return NewOpenAICompat("minimax", apiKey, baseURL, minimaxDefaultModel)
}

// Name implements Provider.
func (m *OpenAICompat) Name() string { return m.name }

// cachesSystem reports whether this endpoint should attach a cache_control
// breakpoint to the system prefix. Only OpenRouter forwards Anthropic-style
// cache_control to its (Anthropic/Gemini) backends; other OpenAI-compatible
// endpoints (MiniMax, Groq, Ollama, …) cache automatically or not at all and may
// reject array-form content, so they keep plain string content. OpenAI/DeepSeek
// models via OpenRouter cache implicitly — the extra breakpoint is harmless there.
func (m *OpenAICompat) cachesSystem() bool {
	return m.name == "openrouter" || m.cacheMode == "native"
}

// reasoningEffortFor maps a thinking-token budget (the agent's ThinkingLevel,
// already resolved to 0/2048/8192/16384 by the tool loop) to an OpenAI-style
// reasoning_effort label. 0 → "" (omit the field entirely).
func reasoningEffortFor(budget int) string {
	switch {
	case budget <= 0:
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	default:
		return "high"
	}
}

// effortFor returns the reasoning_effort to send for this request: "" unless the
// provider declared reasoning support AND the request carries a thinking budget.
func (m *OpenAICompat) effortFor(req Request) string {
	if !m.reasoning {
		return ""
	}
	return reasoningEffortFor(req.ThinkingBudget)
}

type oaiMessage struct {
	Role string `json:"role"`
	// Content is a plain string for ordinary messages, or a []oaiContentPart when
	// a cache_control breakpoint must be attached (OpenRouter prompt caching for
	// Anthropic/Gemini models). interface{} lets one field carry both shapes.
	Content any `json:"content,omitempty"`
	// Assistant tool-call requests (role "assistant").
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
	// Tool result link (role "tool").
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// oaiContentPart is one block of a structured (array) message content. Used only
// when a cache_control breakpoint is needed; ordinary messages keep string content.
type oaiContentPart struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *oaiCacheCtrl `json:"cache_control,omitempty"`
}

// oaiCacheCtrl is the Anthropic-style cache breakpoint OpenRouter forwards to
// Anthropic/Gemini backends ("ephemeral" = standard prompt cache).
type oaiCacheCtrl struct {
	Type string `json:"type"` // "ephemeral"
}

// oaiToolCall is a function call the model requested / we echo back. Arguments
// is a JSON-encoded string (OpenAI convention), not a JSON object.
type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// oaiTool is a tool offered to the model in OpenAI's tools format.
type oaiTool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaiReq struct {
	Model           string         `json:"model"`
	Messages        []oaiMessage   `json:"messages"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Tools           []oaiTool      `json:"tools,omitempty"`
	Stream          bool           `json:"stream,omitempty"`
	StreamOptions   *oaiStreamOpts `json:"stream_options,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
}

type oaiStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

// oaiStreamChunk is one SSE chunk of an OpenAI-compatible streaming response.
type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage"`
}

// oaiUsage is the OpenAI-compatible usage object, extended with the prompt-cache
// fields the various backends report. cached prompt tokens are a SUBSET of
// prompt_tokens (already counted there), so toUsage subtracts them out to get the
// fresh input count — matching the Anthropic convention the rest of the codebase
// and the pricing table assume (input + cacheRead are billed separately).
type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	// OpenAI / OpenRouter: cache reads live under prompt_tokens_details.
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	// MiniMax reports the cache hit count at the top level instead.
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	// OpenRouter (Anthropic/Gemini models): tokens WRITTEN to cache this turn — a
	// separate count, not part of prompt_tokens, so it is not subtracted.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// toUsage maps the OpenAI-compatible usage object to the provider Usage, pulling
// cache reads out of the prompt-token total so cost/savings are computed right.
func (u oaiUsage) toUsage() Usage {
	cached := u.PromptCacheHitTokens
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > cached {
		cached = u.PromptTokensDetails.CachedTokens
	}
	input := u.PromptTokens - cached // cached is a subset of prompt_tokens
	if input < 0 {
		input = 0
	}
	return Usage{
		InputTokens:      input,
		OutputTokens:     u.CompletionTokens,
		CacheReadTokens:  cached,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

type oaiResp struct {
	Choices []struct {
		Message struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Model string   `json:"model"`
	Usage oaiUsage `json:"usage"`
	// MiniMax wraps errors in base_resp (status_code 0 = success).
	BaseResp *struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	// Standard OpenAI-style error envelope.
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete implements Provider via the OpenAI-compatible chat endpoint, with
// tool-use: req.Tools are offered in OpenAI tools format and any tool_calls the
// model returns are surfaced as Response.ToolCalls (StopReason StopToolUse).
func (m *OpenAICompat) Complete(ctx context.Context, req Request) (*Response, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("%s: missing API key", m.name)
	}

	model := req.Model
	if model == "" {
		model = m.defaultModel
	}

	body := oaiReq{
		Model:           model,
		Messages:        toOAIMessages(req, m.cachesSystem()),
		MaxTokens:       req.MaxTokens,
		Tools:           toOAITools(req.Tools),
		ReasoningEffort: m.effortFor(req),
	}
	headers := map[string]string{"Authorization": "Bearer " + m.apiKey}

	var parsed oaiResp
	status, raw, err := postJSON(ctx, m.client, m.name, m.baseURL+"/chat/completions", headers, body, &parsed)
	if err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%s API error (%s): %s", m.name, parsed.Error.Type, parsed.Error.Message)
	}
	if parsed.BaseResp != nil && parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%s API error (%d): %s", m.name, parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s HTTP %d: %s", m.name, status, string(raw))
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty response", m.name)
	}

	choice := parsed.Choices[0]
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}

	var calls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}

	// Strip any <think>…</think> reasoning out of the visible text into a
	// thinking trace step (MiniMax and some other OpenAI-compat models embed it).
	text, think := splitThink(choice.Message.Content)
	var trace []TraceStep
	if think != "" {
		trace = []TraceStep{{Kind: "thinking", Text: think}}
	}

	return &Response{
		Text:       text,
		ToolCalls:  calls,
		StopReason: oaiStopReason(choice.FinishReason),
		Model:      usedModel,
		Trace:      trace,
		Usage:      parsed.Usage.toUsage(),
	}, nil
}

// Stream implements Streamer via the OpenAI-compatible chat endpoint with
// "stream": true. Each chunk's delta.content is forwarded to onDelta; the final
// usage-only chunk (requested via stream_options) populates the Response usage.
// Tool-call deltas are not assembled here — the agent loop routes tool turns
// through Complete — so every chunk is treated as a text delta.
func (m *OpenAICompat) Stream(ctx context.Context, req Request, onDelta func(StreamDelta)) (*Response, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("%s: missing API key", m.name)
	}
	model := req.Model
	if model == "" {
		model = m.defaultModel
	}

	body := oaiReq{
		Model:           model,
		Messages:        toOAIMessages(req, m.cachesSystem()),
		MaxTokens:       req.MaxTokens,
		Stream:          true,
		StreamOptions:   &oaiStreamOpts{IncludeUsage: true},
		ReasoningEffort: m.effortFor(req),
	}
	headers := map[string]string{"Authorization": "Bearer " + m.apiKey}

	var sb, thinkSB strings.Builder
	filt := &thinkFilter{}
	out := &Response{Model: model, StopReason: StopEndTurn}

	emit := func(text, think string) {
		if text != "" {
			sb.WriteString(text)
			onDelta(StreamDelta{Kind: DeltaText, Text: text})
		}
		if think != "" {
			thinkSB.WriteString(think)
			onDelta(StreamDelta{Kind: DeltaThinking, Text: think})
		}
	}

	err := postSSE(ctx, m.client, m.name, m.baseURL+"/chat/completions", headers, body, func(_ string, data []byte) bool {
		if string(data) == "[DONE]" {
			return false
		}
		var ch oaiStreamChunk
		if json.Unmarshal(data, &ch) != nil {
			return true // skip an unparseable chunk rather than abort
		}
		if len(ch.Choices) > 0 {
			if c := ch.Choices[0].Delta.Content; c != "" {
				// Route <think>…</think> reasoning to a live thinking block.
				emit(filt.feed(c))
			}
			if fr := ch.Choices[0].FinishReason; fr != "" {
				out.StopReason = oaiStopReason(fr)
			}
		}
		if ch.Usage != nil {
			out.Usage = ch.Usage.toUsage()
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	emit(filt.flush())
	out.Text = sb.String()
	if thinkSB.Len() > 0 {
		out.Trace = []TraceStep{{Kind: "thinking", Text: thinkSB.String()}}
	}
	return out, nil
}

// oaiStopReason maps an OpenAI-compatible finish_reason to a provider stop
// reason. "tool_calls" -> StopToolUse (the model wants tools run), "length"
// (hit max_tokens) -> StopMaxTok so A1 recovery can resume; everything else is
// a normal end of turn.
func oaiStopReason(finishReason string) string {
	switch finishReason {
	case "tool_calls":
		return StopToolUse
	case "length":
		return StopMaxTok
	default:
		return StopEndTurn
	}
}

// toOAITools converts provider tool defs into OpenAI's tools format.
func toOAITools(tools []ToolDef) []oaiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		var ot oaiTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.InputSchema
		out = append(out, ot)
	}
	return out
}

// toOAIMessages converts a provider Request into OpenAI-style messages. The
// static + dynamic system prompt becomes the leading system message. When
// cacheSystem is true (OpenRouter), the static prefix carries a cache_control
// breakpoint so it is cached across turns while the volatile dynamic suffix stays
// outside the cached prefix — mirroring the native Anthropic provider. Otherwise
// the two parts are concatenated into a plain string (no breakpoint). Assistant
// turns with tool calls carry an OpenAI tool_calls array; user turns answering
// them are expanded into one "tool" role message per result. In-band system-role
// turns and empty text-only turns are dropped.
func toOAIMessages(req Request, cacheSystem bool) []oaiMessage {
	msgs := make([]oaiMessage, 0, len(req.Messages)+1)
	if sysMsg, ok := buildSystemMessage(req.System, req.SystemDynamic, cacheSystem); ok {
		msgs = append(msgs, sysMsg)
	}
	// Merge back-to-back same-role plain-text turns (e.g. several agents replying
	// in one shared thread) so the role sequence stays clean for stricter
	// OpenAI-compatible backends.
	for _, mm := range coalescePlainSameRole(req.Messages) {
		if mm.Role == RoleSystem {
			continue
		}
		// Tool results (user side) become one tool-role message each.
		if len(mm.ToolResults) > 0 {
			for _, tr := range mm.ToolResults {
				content := tr.Content
				if tr.IsError && content == "" {
					content = "error"
				}
				msgs = append(msgs, oaiMessage{Role: "tool", ToolCallID: tr.CallID, Content: content})
			}
			continue
		}
		// Assistant tool-call request.
		if len(mm.ToolCalls) > 0 {
			calls := make([]oaiToolCall, 0, len(mm.ToolCalls))
			for _, tc := range mm.ToolCalls {
				var oc oaiToolCall
				oc.ID = tc.ID
				oc.Type = "function"
				oc.Function.Name = tc.Name
				oc.Function.Arguments = string(tc.Input)
				calls = append(calls, oc)
			}
			msgs = append(msgs, oaiMessage{Role: mm.Role, Content: mm.Text, ToolCalls: calls})
			continue
		}
		if mm.Text == "" {
			continue
		}
		msgs = append(msgs, oaiMessage{Role: mm.Role, Content: mm.Text})
	}
	if cacheSystem {
		// Second breakpoint on the tail of the transcript so the whole conversation
		// prefix (system + history) is cached and reused next turn — not just the
		// system prefix. OpenRouter/Anthropic allow up to 4 breakpoints; we use 2.
		attachHistoryBreakpoint(msgs)
	}
	return msgs
}

// attachHistoryBreakpoint places a cache_control breakpoint on the last plain-text
// message (user/assistant text or a tool result) by converting its string content
// to a single text part. Messages carrying tool_calls and the system message
// (index 0, which already has its own breakpoint) are skipped. No-op when there is
// no eligible message.
func attachHistoryBreakpoint(msgs []oaiMessage) {
	for i := len(msgs) - 1; i >= 1; i-- {
		if len(msgs[i].ToolCalls) > 0 {
			continue
		}
		s, ok := msgs[i].Content.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		msgs[i].Content = []oaiContentPart{{Type: "text", Text: s, CacheControl: &oaiCacheCtrl{Type: "ephemeral"}}}
		return
	}
}

// buildSystemMessage assembles the leading system message from a static prefix
// and a volatile dynamic suffix. Without caching the two are concatenated into a
// plain string. With caching (OpenRouter) they become two text parts and a
// cache_control breakpoint is placed on the static prefix (or, if there is no
// static prefix, on the dynamic block) so the cached prefix is reused next turn.
// ok is false when both parts are empty (no system message at all).
func buildSystemMessage(static, dynamic string, cache bool) (oaiMessage, bool) {
	static = strings.TrimSpace(static)
	dynamic = strings.TrimSpace(dynamic)
	if static == "" && dynamic == "" {
		return oaiMessage{}, false
	}
	if !cache {
		return oaiMessage{Role: "system", Content: strings.TrimSpace(static + "\n\n" + dynamic)}, true
	}

	bp := &oaiCacheCtrl{Type: "ephemeral"}
	var parts []oaiContentPart
	if static != "" {
		parts = append(parts, oaiContentPart{Type: "text", Text: static, CacheControl: bp})
		bp = nil // breakpoint already placed on the static prefix
	}
	if dynamic != "" {
		parts = append(parts, oaiContentPart{Type: "text", Text: dynamic, CacheControl: bp})
	}
	return oaiMessage{Role: "system", Content: parts}, true
}
