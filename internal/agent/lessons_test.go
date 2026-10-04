package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
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
	// Variable parts (paths, numbers) collapse: same failure shape on a
	// different file / line count hashes identically.
	d := lessonSignature("Read", []lessonEvidence{{tool: "Read", errs: "no such file or directory: C:\\other\\place.txt"}}, "")
	if a != d {
		t.Errorf("path variance must not split the signature: %q vs %q", a, d)
	}
	e1 := lessonSignature("", nil, "prompt is too long: 250000 tokens > 200000 maximum")
	e2 := lessonSignature("", nil, "prompt is too long: 310007 tokens > 200000 maximum")
	if e1 != e2 {
		t.Errorf("digit variance must not split the signature")
	}
}

func TestCleanLessonText(t *testing.T) {
	if got := cleanLessonText("NONE"); got != "" {
		t.Errorf("bare NONE = %q, want empty", got)
	}
	if got := cleanLessonText("NONE\n\nWait — this is generalizable.\nUse a writable path."); got != "Wait — this is generalizable.\nUse a writable path." {
		t.Errorf("leading NONE not stripped: %q", got)
	}
	if got := cleanLessonText("  a plain lesson  "); got != "a plain lesson" {
		t.Errorf("plain lesson mangled: %q", got)
	}
	if got := cleanLessonText(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestNormalizeErrSig(t *testing.T) {
	got := normalizeErrSig("No such file: C:\\Users\\x\\a.txt (line 42)")
	want := "no such file: <path> (line #)"
	if got != want {
		t.Errorf("normalizeErrSig = %q, want %q", got, want)
	}
	if normalizeErrSig("exit status 1") != "exit status #" {
		t.Errorf("digit run not collapsed: %q", normalizeErrSig("exit status 1"))
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

	if got := rt.notes.List(notes.Filter{}); len(got) != 0 {
		t.Fatalf("lessons written without a reflectable failure: %+v", got)
	}
}

func TestReflectLessonsSkipsSystemAgentSession(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "lesson extractor", System: true, SystemKey: "lesson-extractor"})
	if err != nil {
		t.Fatalf("create system agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agent.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rt.reflectLessons(ctx, sess.ID, []lessonEvidence{{tool: "Read", errs: "boom"}}, "")

	if lessons := rt.notes.List(notes.Filter{}); len(lessons) != 0 {
		t.Fatalf("system-agent session produced lessons: %+v", lessons)
	}
}

func TestLessonTitleCutsAtAWordWithoutEllipsis(t *testing.T) {
	long := "When calling tools that require specific identifiers such as note ids or task ids, query the listing tool first instead of guessing. Second sentence."
	got := lessonTitle(long)
	if len([]rune(got)) > 90 || strings.Contains(got, "…") || strings.HasSuffix(got, " ") {
		t.Fatalf("title %q", got)
	}
	if !strings.HasPrefix(got, "When calling tools that require specific identifiers") {
		t.Fatalf("title must start with the sentence: %q", got)
	}
	if got := lessonTitle("[tool] Quote paths."); got != "(tool) Quote paths" {
		t.Fatalf("brackets: %q", got)
	}
	if got := lessonTitle("   "); got != "Lesson from a failed turn" {
		t.Fatalf("empty: %q", got)
	}
}
