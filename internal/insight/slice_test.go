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

	out := s.buildSlice(db.Session{ID: "SES1", Title: "t"}, msgs, nil)

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

	out := s.buildSlice(db.Session{ID: "SES1", Title: "t"}, msgs, nil)

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

	out := s.buildSlice(db.Session{ID: "SES1", Title: "auth"}, msgs, events)

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

	out := s.buildSlice(db.Session{ID: "SES1", Title: "t"}, msgs, events)

	steps := strings.Count(out, "- [error] reason=")
	if steps == 0 {
		t.Fatalf("budget starved the primary evidence entirely:\n%s", out)
	}
	// The events section still announces its omission rather than vanishing.
	if !strings.Contains(out, "## Debug events") {
		t.Errorf("events section header missing:\n%s", out)
	}
}
