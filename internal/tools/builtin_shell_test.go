package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestShellCallStreamEmitsChunks verifies the shell tool implements StreamingTool
// and forwards output to onChunk while still returning the full result.
func TestShellCallStreamEmitsChunks(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)

	// Sanity: it is recognised as a streaming tool by the registry.
	reg := NewRegistry(tool)
	if !reg.CanStream("shell") {
		t.Fatal("shell should be a StreamingTool")
	}

	var chunks []string
	out, err := tool.CallStream(
		context.Background(),
		json.RawMessage(`{"command":"echo streamtest"}`),
		func(c string) { chunks = append(chunks, c) },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "streamtest") {
		t.Fatalf("full output missing token: %q", out)
	}
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "streamtest") {
		t.Fatalf("streamed chunks missing token: %q", joined)
	}
}
