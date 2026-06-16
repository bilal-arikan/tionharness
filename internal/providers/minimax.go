package providers

import (
	"context"
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
	Model     string       `json:"model"`
	Messages  []oaiMessage `json:"messages"`
	MaxTokens int          `json:"max_tokens,omitempty"`
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

	msgs := make([]oaiMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: req.System})
	}
	for _, mm := range req.Messages {
		if mm.Role == RoleSystem || mm.Text == "" {
			continue
		}
		msgs = append(msgs, oaiMessage{Role: mm.Role, Content: mm.Text})
	}

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
