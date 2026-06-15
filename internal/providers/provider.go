// Package providers abstracts LLM endpoints behind a common interface so
// agents can switch between Anthropic, OpenAI, Ollama, etc.
package providers

import "context"

// Role identifies the author of a message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Message is a provider-agnostic chat turn.
type Message struct {
	Role string
	Text string
}

// Request is a completion request.
type Request struct {
	Model     string
	System    string
	Messages  []Message
	MaxTokens int
}

// Usage reports token consumption.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Response is a completion result.
type Response struct {
	Text  string
	Usage Usage
	Model string
}

// Provider is implemented by every LLM backend.
type Provider interface {
	// Name returns the provider identifier, e.g. "anthropic".
	Name() string
	// Complete runs a non-streaming completion.
	Complete(ctx context.Context, req Request) (*Response, error)
}
