package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// TestChatTurnFailureMapping pins what the user sees for each way a chat turn can
// end without a clean reply. The idle case is the one that matters most: a turn cut
// by the inactivity watchdog must NOT read like a generic provider error, because
// the provider did not fail — it went silent.
func TestChatTurnFailureMapping(t *testing.T) {
	boom := errors.New("boom")

	if detail, reason := chatTurnFailure(agent.ErrTurnIdleTimeout, boom); reason != reasonTurnIdleTimeout ||
		!strings.Contains(detail, "Sağlayıcı akışı takıldı") {
		t.Fatalf("idle timeout = (%q, %q)", detail, reason)
	}
	if _, reason := chatTurnFailure(agent.ErrTurnHardTimeout, boom); reason != reasonTurnHardTimeout {
		t.Fatalf("hard timeout reason = %q", reason)
	}
	if _, reason := chatTurnFailure(context.Canceled, boom); reason != reasonStopped {
		t.Fatalf("user stop reason = %q", reason)
	}
	// A genuine provider failure carries the underlying error text through.
	detail, reason := chatTurnFailure(nil, boom)
	if reason != reasonProviderError || !strings.Contains(detail, "boom") {
		t.Fatalf("provider error = (%q, %q)", detail, reason)
	}
}
