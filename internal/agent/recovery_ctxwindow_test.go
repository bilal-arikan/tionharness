package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestDecideRecovery_ContextWindowStop: Claude 4.5+ reports a context-window
// hit as a STOP REASON — same one-shot compact-and-retry policy as the
// error-shaped overflow, and a repeat is context_window_exhausted (never
// "completed", the reply is truncated).
func TestDecideRecovery_ContextWindowStop(t *testing.T) {
	cfg := recoveryConfig{maxTokenLimit: DefaultMaxTokenRetries, reactiveCompact: true}
	resp := &providers.Response{StopReason: providers.StopContextWindow}

	d := decideRecovery(resp, nil, loopState{}, cfg)
	if !d.compact || d.reason != contCompactRetry {
		t.Errorf("first hit should compact+retry: %+v", d)
	}
	d = decideRecovery(resp, nil, loopState{compacted: true}, cfg)
	if d.compact || d.term != termContextExhausted {
		t.Errorf("post-compaction hit should be terminal context_window_exhausted: %+v", d)
	}
	// Reactive compaction disabled → terminal straight away.
	d = decideRecovery(resp, nil, loopState{}, recoveryConfig{reactiveCompact: false})
	if d.compact || d.term != termContextExhausted {
		t.Errorf("compaction-off hit should be terminal: %+v", d)
	}
}
