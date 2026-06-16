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

// Minimax is a thin client for MiniMax's OpenAI-compatible Chat Completions API.
// The same implementation works for any OpenAI-compatible endpoint (base URL +
// Bearer key), so it doubles as a generic OpenAI-style provider.
type Minimax struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewMinimax creates a client. An empty baseURL falls back to MiniMax's public
// endpoint.
func NewMinimax(apiKey, baseURL string) *Minimax {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = minimaxDefaultBaseURL
	}
	return &Minimax{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: requestTimeoutSecs * time.Second},
	}
}

// Name implements Provider.
func (m *Minimax) Name() string { return "minimax" }

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiReq struct {
	Model         string         `json:"model"`
	Messages      []oaiMessage   `json:"messages"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
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
			Content string `json:"content"`
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

// Complete implements Provider via the OpenAI-compatible chat endpoint. Tools
// are not offered (text completion only); tool definitions in the request are
// ignored.
func (m *Minimax) Complete(ctx context.Context, req Request) (*Response, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("minimax: missing API key")
	}

	model := req.Model
	if model == "" {
		model = minimaxDefaultModel
	}

	msgs := toOAIMessages(req)
	headers := map[string]string{"Authorization": "Bearer " + m.apiKey}

	var parsed oaiResp
	status, raw, err := postJSON(ctx, m.client, "minimax", m.baseURL+"/chat/completions", headers, oaiReq{Model: model, Messages: msgs, MaxTokens: req.MaxTokens}, &parsed)
	if err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("minimax API error (%s): %s", parsed.Error.Type, parsed.Error.Message)
	}
	if parsed.BaseResp != nil && parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax API error (%d): %s", parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("minimax HTTP %d: %s", status, string(raw))
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("minimax: empty response")
	}

	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}
	return &Response{
		Text:       parsed.Choices[0].Message.Content,
		StopReason: StopEndTurn,
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
func (m *Minimax) Stream(ctx context.Context, req Request, onDelta func(string)) (*Response, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("minimax: missing API key")
	}
	model := req.Model
	if model == "" {
		model = minimaxDefaultModel
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

	err := postSSE(ctx, m.client, "minimax", m.baseURL+"/chat/completions", headers, body, func(_ string, data []byte) bool {
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
				onDelta(c)
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

// toOAIMessages converts a provider Request into OpenAI-style messages: the
// static + dynamic system prompt becomes the leading system message (OpenAI has
// no cache breakpoint, so the two parts are concatenated); in-band system-role
// and empty-text turns are dropped.
func toOAIMessages(req Request) []oaiMessage {
	msgs := make([]oaiMessage, 0, len(req.Messages)+1)
	if sys := strings.TrimSpace(strings.TrimSpace(req.System) + "\n\n" + strings.TrimSpace(req.SystemDynamic)); sys != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: sys})
	}
	for _, mm := range req.Messages {
		if mm.Role == RoleSystem || mm.Text == "" {
			continue
		}
		msgs = append(msgs, oaiMessage{Role: mm.Role, Content: mm.Text})
	}
	return msgs
}
