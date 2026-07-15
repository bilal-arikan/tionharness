package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// Prompt trace: which registry prompt (and which VERSION of its text) drove an
// auxiliary LLM call. Call sites that resolve a prompt through readPrompt stamp
// the context with the key + a short content hash; RecordUsage copies both onto
// the llm_call debug event. That makes "this bad turn ran with an EDITED
// summary prompt" answerable from debug.jsonl — an edited prompt hashes
// differently from the shipped default, so prompt edits become measurable.

// promptTrace identifies one resolved prompt: registry key + 8-hex content hash.
type promptTrace struct {
	key  string
	hash string
}

type promptTraceCtxKey struct{}

// WithPromptTrace stamps ctx with the registry prompt key and a short hash of
// the RESOLVED text the call is about to use. Empty key or text is a no-op.
func WithPromptTrace(ctx context.Context, key, text string) context.Context {
	if key == "" || text == "" {
		return ctx
	}
	sum := sha256.Sum256([]byte(text))
	return context.WithValue(ctx, promptTraceCtxKey{}, promptTrace{key: key, hash: hex.EncodeToString(sum[:4])})
}

// promptTraceFrom returns the stamped trace, or zero values when absent.
func promptTraceFrom(ctx context.Context) (key, hash string) {
	if t, ok := ctx.Value(promptTraceCtxKey{}).(promptTrace); ok {
		return t.key, t.hash
	}
	return "", ""
}
