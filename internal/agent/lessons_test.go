package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func TestCollectLessonEvidence(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepTool, Tool: "Read", IsError: true, Input: json.RawMessage(`{"path":"x"}`), Output: "no such file or directory"},
		{Kind: StepTool, Tool: "Bash", IsError: true, Output: "Claude requested permissions to use Bash, but you haven't granted it yet."}, // policy denial → excluded
		{Kind: StepTool, Tool: "Grep", IsError: false, Output: "ok"},
		{Kind: StepTool, Tool: "Write", IsError: true, Output: "disk full\n\n[loop guardrail] This exact Write call has failed 2 times this turn."},
		{Kind: StepError, Reason: "provider_error", Text: "boom", IsError: true}, // not a tool step
	}
	ev, turnLevel := collectLessonEvidence(steps, "provider_error: anthropic HTTP 500")
	if len(ev) != 2 {
		t.Fatalf("evidence = %+v, want 2 (Read + Write)", ev)
	}
	if ev[0].tool != "Read" || ev[1].tool != "Write" {
		t.Errorf("tools = %s, %s", ev[0].tool, ev[1].tool)
	}
	if strings.Contains(ev[1].errs, "loop guardrail") {
		t.Errorf("guardrail coaching leaked into evidence: %q", ev[1].errs)
	}
	if turnLevel == "" {
		t.Errorf("turn-level error dropped")
	}

	// A clean turn yields nothing.
	ev, turnLevel = collectLessonEvidence([]TurnStep{{Kind: StepTool, Tool: "Read", Output: "fine"}}, "")
	if len(ev) != 0 || turnLevel != "" {
		t.Errorf("clean turn produced evidence: %+v %q", ev, turnLevel)
	}
}

func TestLessonSignature_StableAndToolScoped(t *testing.T) {
	ev := []lessonEvidence{{tool: "Read", errs: "No Such File or Directory: /x"}}
	a := lessonSignature("Read", ev, "")
	b := lessonSignature("Read", []lessonEvidence{{tool: "Read", errs: "no such file or directory: /x"}}, "")
	if a != b {
		t.Errorf("signature not case-normalized: %q vs %q", a, b)
	}
	c := lessonSignature("Write", ev, "")
	if a == c {
		t.Errorf("different tools must not collide")
	}
}

func TestMaybeReflectLessons_GatesAndSkips(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: "AGT1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	badSteps := []TurnStep{{Kind: StepTool, Tool: "Read", IsError: true, Output: "boom"}}

	// Gate off → no goroutine, no lesson (provider would fail anyway; the point
	// is it returns without panicking and writes nothing).
	tun.SetLessonReflect(false)
	rt.maybeReflectLessons(ctx, sess.ID, badSteps, "")
	// Stuck-gate refusals and user stops teach nothing even when on.
	tun.SetLessonReflect(true)
	rt.maybeReflectLessons(ctx, sess.ID, nil, "stopped")
	rt.maybeReflectLessons(ctx, sess.ID, nil, "session X "+stuckGuardMarker)
	// Clean turn → nothing to reflect.
	rt.maybeReflectLessons(ctx, sess.ID, []TurnStep{{Kind: StepTool, Tool: "Read", Output: "fine"}}, "")

	if got, _ := rt.db.ListLessons(0); len(got) != 0 {
		t.Fatalf("lessons written without a reflectable failure: %+v", got)
	}
}

func TestLessonsContextBlock(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	tun.SetLessonReflect(true)

	if b := rt.LessonsContextBlock(ctx); b != "" {
		t.Fatalf("empty store must yield empty block, got %q", b)
	}
	_, _ = rt.db.AddLesson(db.Lesson{Time: 100, Tool: "Read", Signature: "s1", Text: "verify the path exists before reading"})
	_, _ = rt.db.AddLesson(db.Lesson{Time: 200, Tool: "Read", Signature: "s1", Text: "verify the path exists before reading"}) // count → 2
	_, _ = rt.db.AddLesson(db.Lesson{Time: 300, Signature: "s2", Text: "turn-level lesson"})

	b := rt.LessonsContextBlock(ctx)
	if !strings.Contains(b, "Lessons from past failures") ||
		!strings.Contains(b, "[Read] verify the path exists before reading (seen 2 times)") ||
		!strings.Contains(b, "- turn-level lesson") {
		t.Fatalf("block malformed:\n%s", b)
	}

	tun.SetLessonReflect(false)
	if rt.LessonsContextBlock(ctx) != "" {
		t.Errorf("gate off must suppress the block")
	}
}
