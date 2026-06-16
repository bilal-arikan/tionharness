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

	body := anthropicReq{
		Model:     model,
		MaxTokens: maxTokens,
		System:    a.systemField(req.System),
		Messages:  toAnthropicMessages(req.Messages),
		Tools:     toAnthropicTools(req.Tools),
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

// systemField returns the system prompt either as a plain string or, when
// extended caching is enabled, as a single cache-controlled block (1h TTL) so
// the large persona/context prefix is cached across calls.
func (a *Anthropic) systemField(system string) any {
	if system == "" {
		return nil
	}
	if !a.extendedCache {
		return system
	}
	return []systemBlock{{
		Type:         "text",
		Text:         system,
		CacheControl: &cacheControl{Type: "ephemeral", TTL: "1h"},
	}}
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
