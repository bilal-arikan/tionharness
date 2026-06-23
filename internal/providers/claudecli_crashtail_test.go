package providers

import (
	"strings"
	"testing"
)

func TestStdoutCrashTail(t *testing.T) {
	// Prefers plain (non-JSON) lines — those carry the real error text when the
	// CLI dies with empty stderr.
	mixed := []string{`{"type":"system"}`, "panic: boom", `{"type":"assistant"}`}
	if got := stdoutCrashTail(mixed); !strings.Contains(got, "panic: boom") || strings.Contains(got, `"type"`) {
		t.Fatalf("should surface the plain error line only, got: %s", got)
	}
	// Falls back to the raw tail when only JSON lines exist.
	if got := stdoutCrashTail([]string{`{"type":"result"}`}); !strings.Contains(got, "result") {
		t.Fatalf("should fall back to raw tail, got: %s", got)
	}
	// Empty input still yields an explicit note, never an empty string.
	if got := stdoutCrashTail(nil); got == "" {
		t.Fatal("empty tail should still return a note")
	}
}
