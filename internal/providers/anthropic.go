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
const (
	betaOneMillionContext = "context-1m-2025-08-07"
	betaExtendedCacheTTL  = "extended-cache-ttl-2025-04-11"
)

const (
	anthropicURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion   = "2023-06-01"
	DefaultModel       = "claude-sonnet-4-6"
	defaultMaxTokens   = 4096
	requestTimeoutSecs = 120
)

// Anthropic is a thin client for the Anthropic Messages API.
// It avoids the official SDK to stay dependency-light and version-stable.
type Anthropic struct {
	apiKey string
	client *http.Client

	oneMContext   bool // 1M-token context window beta
	extendedCache bool // 1h extended prompt cache TTL beta
}

// NewAnthropic creates a client with the given API key.
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey: apiKey,
		client: &http.Client{Timeout: requestTimeoutSecs * time.Second},
	}
}

// WithBetas enables optional Anthropic beta capabilities and returns the client
// for chaining.
func (a *Anthropic) WithBetas(oneMContext, extendedCache bool) *Anthropic {
	a.oneMContext = oneMContext
	a.extendedCache = extendedCache
	return a
}

// Name implements Provider.
func (a *Anthropic) Name() string { return "anthropic" }

// anthropicReq mirrors the Messages API request body.
type anthropicReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    any                `json:"system,omitempty"` // string, or []systemBlock when caching
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Thinking  *thinkingParam     `json:"thinking,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

// thinkingParam enables extended reasoning. The model emits thinking blocks
// (not shown by SwarmGo) before its answer; max_tokens must exceed budget.
type thinkingParam struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

// thinkingFor returns the thinking parameter (or nil) and the max_tokens to use:
// when thinking is on, max_tokens must be strictly greater than the budget, so
// it is bumped to leave room for the visible answer.
func thinkingFor(budget, maxTokens int) (*thinkingParam, int) {
	if budget <= 0 {
		return nil, maxTokens
	}
	if maxTokens <= budget {
		maxTokens = budget + defaultMaxTokens
	}
	return &thinkingParam{Type: "enabled", BudgetTokens: budget}, maxTokens
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
	Role    string        `json:"role"`
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
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicResp mirrors the relevant parts of the response body.
type anthropicResp struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Model      string `json:"model"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
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
		model = DefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, maxTokens := thinkingFor(req.ThinkingBudget, maxTokens)

	body := anthropicReq{
		Model:     model,
		MaxTokens: maxTokens,
		System:    a.systemField(req.System, req.SystemDynamic),
		Messages:  toAnthropicMessages(req.Messages),
		Tools:     toAnthropicTools(req.Tools),
		Thinking:  thinking,
	}

	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if beta := a.betaHeader(); beta != "" {
		headers["anthropic-beta"] = beta
	}

	var parsed anthropicResp
	status, raw, err := postJSON(ctx, a.client, "anthropic", anthropicURL, headers, body, &parsed)
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
	for _, c := range parsed.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "tool_use":
			calls = append(calls, ToolCall{ID: c.ID, Name: c.Name, Input: c.Input})
		}
	}

	return &Response{
		Text:       text,
		ToolCalls:  calls,
		StopReason: parsed.StopReason,
		Model:      parsed.Model,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
	}, nil
}

// Stream implements Streamer via the Messages API with "stream": true. It
// parses the SSE event sequence (message_start → content_block_delta(text) →
// message_delta → message_stop), forwarding each text chunk to onDelta and
// accumulating the full text/usage/stop reason for the returned Response. Tools
// are not used on the streaming path (text-only turns).
func (a *Anthropic) Stream(ctx context.Context, req Request, onDelta func(string)) (*Response, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}
	model := req.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, maxTokens := thinkingFor(req.ThinkingBudget, maxTokens)

	body := anthropicReq{
		Model:     model,
		MaxTokens: maxTokens,
		System:    a.systemField(req.System, req.SystemDynamic),
		Messages:  toAnthropicMessages(req.Messages),
		Thinking:  thinking,
		Stream:    true,
	}
	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if beta := a.betaHeader(); beta != "" {
		headers["anthropic-beta"] = beta
	}

	var sb strings.Builder
	out := &Response{Model: model, StopReason: StopEndTurn}
	parseErr := error(nil)

	err := postSSE(ctx, a.client, "anthropic", anthropicURL, headers, body, func(event string, data []byte) bool {
		switch event {
		case "message_start":
			var ev struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(data, &ev) == nil {
				out.Usage.InputTokens = ev.Message.Usage.InputTokens
			}
		case "content_block_delta":
			var ev struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal(data, &ev) == nil && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				sb.WriteString(ev.Delta.Text)
				onDelta(ev.Delta.Text)
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
	return out, nil
}

// betaHeader builds the comma-separated anthropic-beta header from the enabled
// beta flags ("" when none).
func (a *Anthropic) betaHeader() string {
	var betas []string
	if a.oneMContext {
		betas = append(betas, betaOneMillionContext)
	}
	if a.extendedCache {
		betas = append(betas, betaExtendedCacheTTL)
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
func (a *Anthropic) systemField(static, dynamic string) any {
	static = strings.TrimSpace(static)
	dynamic = strings.TrimSpace(dynamic)
	if static == "" && dynamic == "" {
		return nil
	}
	if !a.extendedCache {
		return strings.TrimSpace(static + "\n\n" + dynamic)
	}

	// The cache breakpoint goes on the static prefix when present; otherwise it
	// falls to the dynamic block so a static-less prompt is still cached (matching
	// the prior single-block behavior).
	cache := &cacheControl{Type: "ephemeral", TTL: "1h"}
	var blocks []systemBlock
	if static != "" {
		blocks = append(blocks, systemBlock{Type: "text", Text: static, CacheControl: cache})
		cache = nil
	}
	if dynamic != "" {
		blocks = append(blocks, systemBlock{Type: "text", Text: dynamic, CacheControl: cache})
	}
	return blocks
}

// toAnthropicMessages converts provider messages to content-block form,
// skipping the system role (passed separately in the Anthropic API).
func toAnthropicMessages(msgs []Message) []anthropicMessage {
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
	return out
}

func toAnthropicTools(tools []ToolDef) []anthropicTool {
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
	return out
}
