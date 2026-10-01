package tools

import (
	"context"
	"encoding/json"
)

type ClarificationCheck func(context.Context, json.RawMessage) (string, bool)
type clarificationCheckKey struct{}

func WithClarificationCheck(ctx context.Context, check ClarificationCheck) context.Context {
	return context.WithValue(ctx, clarificationCheckKey{}, check)
}

func CheckClarification(ctx context.Context, input json.RawMessage) (string, bool) {
	if check, _ := ctx.Value(clarificationCheckKey{}).(ClarificationCheck); check != nil {
		return check(ctx, input)
	}
	return "", false
}

// Missing required_input preserves the original ask behavior.
func OptionalClarification(input json.RawMessage) bool {
	var in struct {
		Required *bool `json:"required_input"`
	}
	return json.Unmarshal(input, &in) == nil && in.Required != nil && !*in.Required
}
