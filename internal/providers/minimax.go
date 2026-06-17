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

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	// Assistant tool-call requests (role "assistant").
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
	// Tool result link (role "tool").
	ToolCallID string `json:"tool_call_id,omitempty"`
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
	Model         string         `json:"model"`
	Messages      []oaiMessage   `json:"messages"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Tools         []oaiTool      `json:"tools,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *oaiStreamOpts `json:"stream_options,omitempty"`
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
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type oaiResp struct {
	Choices []struct {
		Message struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
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
		Model:     model,
		Messages:  toOAIMessages(req),
		MaxTokens: req.MaxTokens,
		Tools:     toOAITools(req.Tools),
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

	return &Response{
		Text:       choice.Message.Content,
		ToolCalls:  calls,
		StopReason: oaiStopReason(choice.FinishReason),
		Model:      usedModel,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
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
		Model:         model,
		Messages:      toOAIMessages(req),
		MaxTokens:     req.MaxTokens,
		Stream:        true,
		StreamOptions: &oaiStreamOpts{IncludeUsage: true},
	}
	headers := map[string]string{"Authorization": "Bearer " + m.apiKey}

	var sb strings.Builder
	out := &Response{Model: model, StopReason: StopEndTurn}

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
				sb.WriteString(c)
				onDelta(StreamDelta{Kind: DeltaText, Text: c})
			}
			if fr := ch.Choices[0].FinishReason; fr != "" {
				out.StopReason = oaiStopReason(fr)
			}
		}
		if ch.Usage != nil {
			out.Usage.InputTokens = ch.Usage.PromptTokens
			out.Usage.OutputTokens = ch.Usage.CompletionTokens
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	out.Text = sb.String()
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

// toOAIMessages converts a provider Request into OpenAI-style messages: the
// static + dynamic system prompt becomes the leading system message (OpenAI has
// no cache breakpoint, so the two parts are concatenated). Assistant turns with
// tool calls carry an OpenAI tool_calls array; user turns answering them are
// expanded into one "tool" role message per result. In-band system-role turns
// and empty text-only turns are dropped.
func toOAIMessages(req Request) []oaiMessage {
	msgs := make([]oaiMessage, 0, len(req.Messages)+1)
	if sys := strings.TrimSpace(strings.TrimSpace(req.System) + "\n\n" + strings.TrimSpace(req.SystemDynamic)); sys != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: sys})
	}
	for _, mm := range req.Messages {
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
	return msgs
}
