package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Anthropic beta feature flags (sent via the anthropic-beta header).
// The 1M-context beta (context-1m-2025-08-07) was retired: Anthropic made the
// 1M window GA at standard pricing on 2026-03-13 (no header needed) and turned
// the beta header off on 2026-04-30, so it is no longer sent.
const (
	betaExtendedCacheTTL  = "extended-cache-ttl-2025-04-11"
	betaContextManagement = "context-management-2025-06-27"
)

// Context-editing defaults (P3): the server clears old tool_use/tool_result blocks
// from the cached prefix in place (cache_edits) once the prompt grows past the
// trigger, keeping the most recent tool uses — Claude Code's microcompact analogue.
// Conservative floors so short turns are untouched and recent context is preserved.
const (
	contextClearTriggerTokens = 100000 // start clearing past this input size
	contextClearKeepToolUses  = 3      // always keep the newest N tool uses
	contextClearAtLeastTokens = 5000   // minimum tokens to reclaim per clear
)

const (
	anthropicURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion   = "2023-06-01"
	DefaultModel       = "claude-sonnet-5"
	defaultMaxTokens   = 4096
	requestTimeoutSecs = 120
)

// cacheTTL is the SINGLE ttl used by EVERY prompt-cache breakpoint (tools, static
// system, rolling history). Anthropic requires that a breakpoint's TTL never be
// shorter than one appearing later in the tools → system → messages prefix order,
// so a mixed TTL would silently break caching. Defining it once (P5 hardening)
// makes a mid-request TTL drift impossible: change the policy here, everywhere.
const cacheTTL = "1h"

// Anthropic is a thin client for the Anthropic Messages API.
// It avoids the official SDK to stay dependency-light and version-stable.
//
// The endpoint, default model and provider name are configurable (via
// WithEndpoint) so the same client can drive any Anthropic-compatible host —
// e.g. MiniMax's /anthropic/v1, which accepts the same request shape (tool-use,
// thinking, cache_control, x-api-key auth) and returns native Messages format.
type Anthropic struct {
	apiKey       string
	client       *http.Client
	baseURL      string // full /messages endpoint
	defaultModel string // model applied when a request omits one
	name         string // provider identity reported by Name()

	extendedCache  bool // 1h extended prompt cache TTL beta
	contextEditing bool // API-native context editing (clear_tool_uses) beta
}

// NewAnthropic creates a client with the given API key, defaulting to the
// official Anthropic endpoint and model.
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey:       apiKey,
		client:       &http.Client{Timeout: requestTimeoutSecs * time.Second},
		baseURL:      anthropicURL,
		defaultModel: DefaultModel,
		name:         "anthropic",
	}
}

// WithEndpoint points the client at an Anthropic-compatible host. Empty
// arguments keep the current value, so callers can override only what differs.
// messagesURL is the full /messages endpoint (e.g.
// "https://api.minimax.io/anthropic/v1/messages"). Returns the client for
// chaining.
func (a *Anthropic) WithEndpoint(name, messagesURL, defaultModel string) *Anthropic {
	if name != "" {
		a.name = name
	}
	if messagesURL != "" {
		a.baseURL = messagesURL
	}
	if defaultModel != "" {
		a.defaultModel = defaultModel
	}
	return a
}

// WithBetas enables optional Anthropic beta capabilities and returns the client
// for chaining. extendedCache = 1h prompt-cache TTL; contextEditing = API-native
// context editing (server-side clear_tool_uses, the microcompact analogue).
func (a *Anthropic) WithBetas(extendedCache, contextEditing bool) *Anthropic {
	a.extendedCache = extendedCache
	a.contextEditing = contextEditing
	return a
}

// Name implements Provider.
func (a *Anthropic) Name() string { return a.name }

// anthropicReq mirrors the Messages API request body.
type anthropicReq struct {
	Model             string             `json:"model"`
	MaxTokens         int                `json:"max_tokens"`
	System            any                `json:"system,omitempty"` // string, or []systemBlock when caching
	Messages          []anthropicMessage `json:"messages"`
	Tools             []anthropicTool    `json:"tools,omitempty"`
	Thinking          *thinkingParam     `json:"thinking,omitempty"`
	OutputConfig      *outputConfig      `json:"output_config,omitempty"`
	ContextManagement *contextManagement `json:"context_management,omitempty"`
	Stream            bool               `json:"stream,omitempty"`
}

// contextManagement carries the API-native context-editing directives (P3). The
// server applies them to the CACHED prefix in place — clearing old tool_use/
// tool_result blocks past a size trigger while keeping the newest N — so the warm
// prefix shrinks without a full rewrite (cache_edits), the microcompact analogue.
type contextManagement struct {
	Edits []contextEdit `json:"edits"`
}

type contextEdit struct {
	Type         string            `json:"type"`
	Trigger      *contextThreshold `json:"trigger,omitempty"`
	Keep         *contextThreshold `json:"keep,omitempty"`
	ClearAtLeast *contextThreshold `json:"clear_at_least,omitempty"`
	ExcludeTools []string          `json:"exclude_tools,omitempty"`
}

type contextThreshold struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

// contextMgmt builds the context-editing directive when the beta is enabled, else
// nil (the field is omitted). One strategy: clear_tool_uses_20250919 with
// conservative defaults (see the context-editing constants).
func (a *Anthropic) contextMgmt() *contextManagement {
	if !a.contextEditing {
		return nil
	}
	return &contextManagement{
		Edits: []contextEdit{{
			Type:         "clear_tool_uses_20250919",
			Trigger:      &contextThreshold{Type: "input_tokens", Value: contextClearTriggerTokens},
			Keep:         &contextThreshold{Type: "tool_uses", Value: contextClearKeepToolUses},
			ClearAtLeast: &contextThreshold{Type: "input_tokens", Value: contextClearAtLeastTokens},
		}},
	}
}

// thinkingParam enables extended reasoning. Adaptive-class models take
// {type:"adaptive"} (budget_tokens is rejected there); legacy models take
// {type:"enabled", budget_tokens:N}.
type thinkingParam struct {
	Type         string `json:"type"`                    // "adaptive" | "disabled" | "enabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // legacy enabled shape only
	Display      string `json:"display,omitempty"`       // "summarized" — adaptive class defaults to "omitted" (empty traces)
}

// outputConfig carries response-level controls; effort steers thinking depth on
// adaptive-class models (the replacement for budget_tokens).
type outputConfig struct {
	Effort string `json:"effort,omitempty"` // "low" | "medium" | "high"
}

// thinkingFor returns the thinking parameter, the optional output_config, and
// the max_tokens to use for the given model.
//
// Adaptive class (UsesAdaptiveThinking — Fable/Mythos 5, Opus 4.7/4.8,
// Sonnet 5): the legacy enabled+budget shape 400s, so the budget is translated
// to {type:"adaptive"} + output_config.effort, with display:"summarized" so the
// thinking trace carries text (these models default to "omitted"). Budget 0 →
// explicit {type:"disabled"}, except always-on models (Fable/Mythos) where
// disabled also 400s and the field is omitted entirely.
//
// Legacy models keep enabled+budget_tokens; max_tokens must be strictly greater
// than the budget, so it is bumped to leave room for the visible answer.
func thinkingFor(model string, budget, maxTokens int) (*thinkingParam, *outputConfig, int) {
	if UsesAdaptiveThinking(model) {
		if budget <= 0 {
			if AlwaysOnThinking(model) {
				return nil, nil, maxTokens
			}
			return &thinkingParam{Type: "disabled"}, nil, maxTokens
		}
		var cfg *outputConfig
		if effort := EffortForThinkingBudget(budget); effort != "" {
			cfg = &outputConfig{Effort: effort}
		}
		return &thinkingParam{Type: "adaptive", Display: "summarized"}, cfg, maxTokens
	}
	if budget <= 0 {
		return nil, nil, maxTokens
	}
	if maxTokens <= budget {
		maxTokens = budget + defaultMaxTokens
	}
	return &thinkingParam{Type: "enabled", BudgetTokens: budget}, nil, maxTokens
}

// systemBlock is the structured form of the system prompt, used when extended
// prompt caching is on so a cache_control breakpoint can be attached.
type systemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`          // "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "1h" with the extended-cache beta
}

// anthropicMessage carries an array of content blocks (text / tool_use /
// tool_result), which is the form required once tools are involved.
type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock is a tagged union over the block types we use.
type contentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	// CacheControl marks a rolling cache breakpoint on the conversation history
	// (attached to the last block of the last message). See toAnthropicMessages.
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	// CacheControl, when set on the LAST tool, marks a cache breakpoint after the
	// whole tools block. Anthropic caches by prefix in tools → system → messages
	// order, so this caches the tool schemas INDEPENDENTLY of the (possibly
	// changing) system block — a system-prompt edit no longer invalidates the
	// tool-definition cache. Only the last tool carries it (one breakpoint covers
	// the entire preceding block).
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// anthropicResp mirrors the relevant parts of the response body.
type anthropicResp struct {
	Content []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		Thinking string          `json:"thinking"`
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Model      string `json:"model"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete implements Provider.
func (a *Anthropic) Complete(ctx context.Context, req Request) (*Response, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}

	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, outCfg, maxTokens := thinkingFor(model, req.ThinkingBudget, maxTokens)

	sysField, msgs := a.buildSystemAndMessages(req)
	body := anthropicReq{
		Model:             model,
		MaxTokens:         maxTokens,
		System:            sysField,
		Messages:          msgs,
		Tools:             toAnthropicTools(req.Tools, a.extendedCache),
		Thinking:          thinking,
		OutputConfig:      outCfg,
		ContextManagement: a.contextMgmt(),
	}

	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if beta := a.betaHeader(); beta != "" {
		headers["anthropic-beta"] = beta
	}

	var parsed anthropicResp
	status, raw, err := postJSON(ctx, a.client, a.name, a.baseURL, headers, body, &parsed)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		if parsed.Error != nil {
			return nil, fmt.Errorf("anthropic API error (%s): %s", parsed.Error.Type, parsed.Error.Message)
		}
		return nil, fmt.Errorf("anthropic HTTP %d: %s", status, string(raw))
	}

	var text string
	var calls []ToolCall
	var trace []TraceStep
	for _, c := range parsed.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "thinking":
			// Extended-reasoning block (precedes the answer); surface it as a
			// thinking trace step, mirroring the keyless claude-cli path.
			if c.Thinking != "" {
				trace = append(trace, TraceStep{Kind: "thinking", Text: c.Thinking})
			}
		case "tool_use":
			calls = append(calls, ToolCall{ID: c.ID, Name: c.Name, Input: c.Input})
		}
	}

	return &Response{
		Text:       text,
		ToolCalls:  calls,
		StopReason: parsed.StopReason,
		Model:      parsed.Model,
		Trace:      trace,
		Usage: Usage{
			InputTokens:      parsed.Usage.InputTokens,
			OutputTokens:     parsed.Usage.OutputTokens,
			CacheWriteTokens: parsed.Usage.CacheCreationInputTokens,
			CacheReadTokens:  parsed.Usage.CacheReadInputTokens,
		},
	}, nil
}

// Stream implements Streamer via the Messages API with "stream": true. It
// parses the SSE event sequence (message_start → content_block_delta →
// message_delta → message_stop), forwarding each text/thinking chunk to onDelta
// (tagged via StreamDelta.Kind) and accumulating the full text/usage/stop
// reason for the returned Response. When extended reasoning is on, the full
// thinking text is also returned as a thinking TraceStep so it can be persisted.
// Tools are not used on the streaming path (text-only turns).
func (a *Anthropic) Stream(ctx context.Context, req Request, onDelta func(StreamDelta)) (*Response, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, outCfg, maxTokens := thinkingFor(model, req.ThinkingBudget, maxTokens)

	sysField, msgs := a.buildSystemAndMessages(req)
	body := anthropicReq{
		Model:        model,
		MaxTokens:    maxTokens,
		System:       sysField,
		Messages:     msgs,
		Thinking:     thinking,
		OutputConfig: outCfg,
		Stream:       true,
	}
	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if beta := a.betaHeader(); beta != "" {
		headers["anthropic-beta"] = beta
	}

	var sb strings.Builder // visible answer text
	var tb strings.Builder // extended-reasoning (thinking) text
	out := &Response{Model: model, StopReason: StopEndTurn}
	parseErr := error(nil)

	err := postSSE(ctx, a.client, a.name, a.baseURL, headers, body, func(event string, data []byte) bool {
		switch event {
		case "message_start":
			var ev struct {
				Message struct {
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(data, &ev) == nil {
				out.Usage.InputTokens = ev.Message.Usage.InputTokens
				out.Usage.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens
				out.Usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			}
		case "content_block_delta":
			var ev struct {
				Delta struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					Thinking string `json:"thinking"`
				} `json:"delta"`
			}
			if json.Unmarshal(data, &ev) != nil {
				return true
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					sb.WriteString(ev.Delta.Text)
					onDelta(StreamDelta{Kind: DeltaText, Text: ev.Delta.Text})
				}
			case "thinking_delta":
				if ev.Delta.Thinking != "" {
					tb.WriteString(ev.Delta.Thinking)
					onDelta(StreamDelta{Kind: DeltaThinking, Text: ev.Delta.Thinking})
				}
			}
		case "message_delta":
			var ev struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(data, &ev) == nil {
				if ev.Delta.StopReason != "" {
					out.StopReason = ev.Delta.StopReason
				}
				out.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "error":
			var ev struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(data, &ev)
			parseErr = fmt.Errorf("anthropic stream error (%s): %s", ev.Error.Type, ev.Error.Message)
			return false
		case "message_stop":
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	out.Text = sb.String()
	if tb.Len() > 0 {
		out.Trace = []TraceStep{{Kind: "thinking", Text: tb.String()}}
	}
	return out, nil
}

// betaHeader builds the comma-separated anthropic-beta header from the enabled
// beta flags ("" when none).
func (a *Anthropic) betaHeader() string {
	var betas []string
	if a.extendedCache {
		betas = append(betas, betaExtendedCacheTTL)
	}
	if a.contextEditing {
		betas = append(betas, betaContextManagement)
	}
	return strings.Join(betas, ",")
}

// systemField builds the system prompt from a stable static prefix and a
// volatile dynamic suffix. When extended caching is on, the cache_control
// breakpoint (1h TTL) is placed on the static block, so the static prefix — plus
// the tool definitions that precede it in the request — is cached across calls,
// while the dynamic suffix (recalled memory + running summary) that changes every
// turn stays outside the cached prefix and never invalidates it. Without caching
// the two parts are concatenated into a plain string.
// buildSystemAndMessages assembles the system field and message list for one
// request, placing the rolling summary (req.Summary) for maximum cache reuse.
// With caching ON the summary rides a synthetic head user message INSIDE the
// cached prefix (before the rolling history breakpoint) so it is a cache READ
// between folds, while the volatile dynamic trails AFTER the breakpoint. With
// caching OFF there is no cached prefix to protect, so both the summary and the
// dynamic fold back into the system prompt (byte-parity with the pre-cache path).
func (a *Anthropic) buildSystemAndMessages(req Request) (any, []anthropicMessage) {
	if a.extendedCache {
		msgs := prependSummaryMessage(req.Messages, req.Summary)
		return a.systemField(req.System, req.SystemDynamic), toAnthropicMessages(msgs, true, req.SystemDynamic)
	}
	dyn := joinNonEmpty(req.SystemDynamic, req.Summary)
	return a.systemField(req.System, dyn), toAnthropicMessages(req.Messages, false, dyn)
}

func (a *Anthropic) systemField(static, dynamic string) any {
	static = strings.TrimSpace(static)
	dynamic = strings.TrimSpace(dynamic)
	if static == "" && dynamic == "" {
		return nil
	}
	if !a.extendedCache {
		// Caching off: placement is irrelevant, keep the historical single-block
		// concatenation (static + dynamic) so the non-cached path is unchanged.
		return strings.TrimSpace(static + "\n\n" + dynamic)
	}

	// Caching ON: the system field is STATIC-ONLY so it is fully cacheable. The
	// volatile dynamic suffix is NOT placed here — it moves to a trailing block on
	// the last message (see toAnthropicMessages), i.e. AFTER the rolling history
	// breakpoint, so it never invalidates the cached prefix. This lets tools +
	// system + the whole message history all become cache READS turn-to-turn
	// (previously the dynamic sat upstream of tools/messages and busted both every
	// turn — only the static system prefix ever hit). Mirrors the claude-cli path,
	// which already weaves the dynamic into the last user message. When there is no
	// static prefix, system is nil (dynamic still rides the messages).
	if static == "" {
		return nil
	}
	return []systemBlock{{Type: "text", Text: static, CacheControl: &cacheControl{Type: "ephemeral", TTL: cacheTTL}}}
}

// toAnthropicMessages converts provider messages to content-block form,
// skipping the system role (passed separately in the Anthropic API). When
// extendedCache is on, a rolling cache breakpoint (1h TTL) is attached to the
// last block of the last message so the ENTIRE conversation prefix up to the
// current turn is cached — the biggest lever on a long session, where the raw
// transcript (not the static system/tools) dominates input tokens. Anthropic
// caches by prefix in tools → system → messages order and allows up to 4
// breakpoints; with tools(1) + system-static(1) this history breakpoint is the
// 3rd, safely within the limit. On turn N the breakpoint marks the prefix as a
// cache write; on turn N+1 that same prefix is a cache read (0.10×) and the new
// breakpoint moves forward to the newest message — the standard "sliding
// breakpoint" pattern. Gated on the same extendedCache flag as the system/tool
// breakpoints so the caching on/off policy stays unified.
func toAnthropicMessages(msgs []Message, extendedCache bool, dynamic string) []anthropicMessage {
	// Anthropic requires strictly alternating roles; merge any back-to-back
	// same-role plain-text turns (e.g. two agents' replies in a shared thread)
	// into one so the request is valid.
	msgs = coalescePlainSameRole(msgs)
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleSystem {
			continue
		}
		var blocks []contentBlock
		if m.Text != "" {
			blocks = append(blocks, contentBlock{Type: "text", Text: m.Text})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, contentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
		}
		for _, tr := range m.ToolResults {
			blocks = append(blocks, contentBlock{
				Type:      "tool_result",
				ToolUseID: tr.CallID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, contentBlock{Type: "text", Text: ""})
		}
		out = append(out, anthropicMessage{Role: m.Role, Content: blocks})
	}
	if extendedCache {
		// Rolling history breakpoint: mark the last block of the last (persisted)
		// message so tools + system + the whole conversation prefix is cached. On
		// turn N this prefix is a cache write; on N+1 the same prefix is a cache
		// read and the breakpoint slides forward to the newest turn.
		if len(out) > 0 {
			last := &out[len(out)-1]
			if n := len(last.Content); n > 0 {
				last.Content[n-1].CacheControl = &cacheControl{Type: "ephemeral", TTL: cacheTTL}
			}
		}
		// Volatile dynamic (date/time, recalled memory, running summary, …) rides as
		// a trailing text block on the last message — AFTER the breakpoint above, so
		// it stays OUTSIDE the cached prefix. It is request-time only (never
		// persisted), so the cached prefix is byte-identical turn-to-turn and hits;
		// the dynamic block's absence in the next turn's rebuilt history is
		// irrelevant because it was never part of the cached prefix. When there are
		// no messages yet, seed one so the dynamic is not dropped.
		if d := strings.TrimSpace(dynamic); d != "" {
			if len(out) == 0 {
				out = append(out, anthropicMessage{Role: RoleUser, Content: []contentBlock{{Type: "text", Text: d}}})
			} else {
				last := &out[len(out)-1]
				last.Content = append(last.Content, contentBlock{Type: "text", Text: d})
			}
		}
	}
	return out
}

// toAnthropicTools converts the tool defs and, when caching is on, attaches a
// cache breakpoint to the LAST tool so the whole tools block is cached on its own
// prefix (independent of the system block). The breakpoint uses the same 1h TTL as
// the system block — valid because tools precede system in Anthropic's ordering,
// so a 1h tool breakpoint never lands after a shorter-TTL one. Gated on the same
// extendedCache flag as the system breakpoint, so the caching on/off policy is
// unchanged; only its granularity improves.
func toAnthropicTools(tools []ToolDef, extendedCache bool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	if extendedCache {
		out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral", TTL: cacheTTL}
	}
	return out
}
