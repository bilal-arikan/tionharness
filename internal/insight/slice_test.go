package insight

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// buildSlice used to assemble the whole transcript and then slice it to a byte
// budget. That had two failure modes the analyzer could not detect: the last
// record arrived cut mid-line (still looking like a complete record), and
// nothing said HOW MUCH was missing. These tests pin the replacement.

func errStepsMsg(n int, pad string) db.Message {
	var steps []string
	for i := 0; i < n; i++ {
		steps = append(steps, fmt.Sprintf(
			`{"kind":"error","reason":"r%d","text":"%s%d"}`, i, pad, i))
	}
	return db.Message{Steps: "[" + strings.Join(steps, ",") + "]"}
}

func TestBuildSliceReportsWhatItDropped(t *testing.T) {
	s := &Scanner{sliceCap: 400}
	msgs := []db.Message{errStepsMsg(40, strings.Repeat("x", 40))}

	out := s.buildSlice(Lens{}, db.Session{ID: "SES1", Title: "t"}, msgs, nil)

	if !strings.Contains(out, "more error/recovery step(s) omitted for size") {
		t.Fatalf("dropped steps were not reported:\n%s", out)
	}
	// Every rendered record must be whole: a line that starts a record must also
	// contain the text that record carried.
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "- [error]") && !strings.Contains(ln, "reason=") {
			t.Errorf("record rendered without its fields (cut mid-line?): %q", ln)
		}
	}
}

func TestBuildSliceKeepsRecordsWholeAtTheBoundary(t *testing.T) {
	s := &Scanner{sliceCap: 400}
	msgs := []db.Message{errStepsMsg(40, strings.Repeat("y", 40))}

	out := s.buildSlice(Lens{}, db.Session{ID: "SES1", Title: "t"}, msgs, nil)

	// The old byte-slice would end the payload mid-token. Now the last rendered
	// step line ends with the step's own text, never a partial word boundary
	// followed by nothing.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var lastStep string
	for _, ln := range lines {
		if strings.HasPrefix(ln, "- [error]") {
			lastStep = ln
		}
	}
	if lastStep == "" {
		t.Fatalf("no step rendered at all:\n%s", out)
	}
	if !strings.Contains(lastStep, "yyy") {
		t.Errorf("last step lost its payload: %q", lastStep)
	}
}

func TestBuildSliceEmitsBothSectionsWhenEverythingFits(t *testing.T) {
	s := &Scanner{sliceCap: 8000}
	msgs := []db.Message{errStepsMsg(2, "short")}
	events := []db.DebugEvent{
		{Type: db.DebugGuardrail, Name: "loop_detected", Detail: "aynı araç 5 kez"},
	}

	out := s.buildSlice(Lens{}, db.Session{ID: "SES1", Title: "auth"}, msgs, events)

	for _, want := range []string{
		`SESSION SES1 — "auth"`,
		"## Error / recovery steps",
		"- [error] reason=r0",
		"## Debug events (errors/repairs/guardrails)",
		"- [guardrail] loop_detected: aynı araç 5 kez",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Nothing was dropped, so nothing must claim it was.
	if strings.Contains(out, "omitted for size") {
		t.Errorf("clean slice wrongly reported an omission:\n%s", out)
	}
}

// TestBuildSliceStepsOutrankEvents: steps are the primary evidence and the debug
// events largely restate them, so a tight budget must spend itself on steps
// rather than starving them to fit events.
func TestBuildSliceStepsOutrankEvents(t *testing.T) {
	s := &Scanner{sliceCap: 400}
	msgs := []db.Message{errStepsMsg(20, strings.Repeat("z", 30))}
	var events []db.DebugEvent
	for i := 0; i < 20; i++ {
		events = append(events, db.DebugEvent{
			Type: db.DebugError, Name: fmt.Sprintf("e%d", i), Detail: strings.Repeat("q", 30),
		})
	}

	out := s.buildSlice(Lens{}, db.Session{ID: "SES1", Title: "t"}, msgs, events)

	steps := strings.Count(out, "- [error] reason=")
	if steps == 0 {
		t.Fatalf("budget starved the primary evidence entirely:\n%s", out)
	}
	// The events section still announces its omission rather than vanishing.
	if !strings.Contains(out, "## Debug events") {
		t.Errorf("events section header missing:\n%s", out)
	}
}

// The prompt-cache surface is OPT-IN: a lens that did not ask for it must not be
// charged tokens for cache events, and a lens that DID ask must receive the
// attributed cause, the cold prefix size, the measured waste and the interleaved
// epoch events (the adopt-vs-unexplained distinction depends on them).
func TestBuildSliceCacheScope(t *testing.T) {
	s := &Scanner{sliceCap: 4000}
	events := []db.DebugEvent{
		{Type: db.DebugEpoch, Name: "created", Time: 1_700_000_000_000},
		{Type: db.DebugCacheBreak, Name: "ttl-or-server-eviction", Detail: "önek soğudu",
			CacheWrite: 9000, In: 120, WasteUSD: 0.0213, WasteEstimated: true, Time: 1_700_003_600_000},
	}

	// No cache scope → the section is absent entirely.
	plain := s.buildSlice(Lens{}, db.Session{ID: "SES1", Title: "t"}, nil, events)
	if strings.Contains(plain, "cache_break") || strings.Contains(plain, "Prompt-cache events") {
		t.Fatalf("a lens without scope:[cache] must not receive cache events:\n%s", plain)
	}

	// With cache scope → cause, cold size, waste and the epoch event all present.
	out := s.buildSlice(Lens{Scope: []string{"debug", ScopeCache}}, db.Session{ID: "SES1", Title: "t"}, nil, events)
	for _, want := range []string{
		"Prompt-cache events",
		"cause=ttl-or-server-eviction",
		"coldTokens=9120",
		"wasteUsd=0.0213(est)",
		"[epoch] at=",
		"created",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cache slice missing %q:\n%s", want, out)
		}
	}
	// Timestamps must be present: the cadence lens reasons about the GAP between
	// events, so a slice without them cannot answer its own question.
	if !strings.Contains(out, "2023-11-14") {
		t.Errorf("cache lines must carry a readable timestamp:\n%s", out)
	}
}

// extractSignals must index a cache break by its attributed CAUSE, so the two
// cache lenses (ordering vs cooling) can prefilter on opposite causes of the same
// event type — and flag measured waste separately.
func TestExtractSignalsCacheCause(t *testing.T) {
	sig := extractSignals(nil, []db.DebugEvent{
		{Type: db.DebugCacheBreak, Name: "ttl-or-server-eviction", WasteUSD: 0.01},
		{Type: db.DebugCacheBreak, Name: "ttl-or-server-eviction"},
		{Type: db.DebugCacheBreak, Name: "model-changed"},
	})
	if got := sig.DebugEvents["cache_break"]; got != 3 {
		t.Errorf("bare cache_break count = %d, want 3", got)
	}
	if got := sig.DebugEvents["cache_break:ttl-or-server-eviction"]; got != 2 {
		t.Errorf("ttl cause count = %d, want 2", got)
	}
	if got := sig.DebugEvents["cache_break:model-changed"]; got != 1 {
		t.Errorf("model cause count = %d, want 1", got)
	}
	if got := sig.DebugEvents["cooling_waste"]; got != 1 {
		t.Errorf("cooling_waste = %d, want 1 (only the event carrying WasteUSD)", got)
	}
}
