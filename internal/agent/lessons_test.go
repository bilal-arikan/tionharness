package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
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

	if got, _ := rt.db.ListLessons(0); len(got) != 0 {
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

	lessons, err := rt.db.ListLessons(0)
	if err != nil {
		t.Fatalf("list lessons: %v", err)
	}
	if len(lessons) != 0 {
		t.Fatalf("system-agent session produced lessons: %+v", lessons)
	}
}

func TestLessonsContextBlock(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	tun.SetLessonReflect(true)
	base := time.Now().Unix()

	if b := rt.LessonsContextBlock(ctx, ""); b != "" {
		t.Fatalf("empty store must yield empty block, got %q", b)
	}
	_, _ = rt.db.AddLesson(db.Lesson{Time: base - 300, Tool: "Read", Signature: "s1", Text: "verify the path exists before reading"})
	_, _ = rt.db.AddLesson(db.Lesson{Time: base - 200, Tool: "Read", Signature: "s1", Text: "verify the path exists before reading"}) // count → 2
	_, _ = rt.db.AddLesson(db.Lesson{Time: base - 100, Signature: "s2", Text: "turn-level lesson"})

	b := rt.LessonsContextBlock(ctx, "")
	if !strings.Contains(b, "Lessons from past failures") ||
		!strings.Contains(b, "[Read] verify the path exists before reading (seen 2 times)") ||
		!strings.Contains(b, "- turn-level lesson") {
		t.Fatalf("block malformed:\n%s", b)
	}

	tun.SetLessonReflect(false)
	if rt.LessonsContextBlock(ctx, "") != "" {
		t.Errorf("gate off must suppress the block")
	}
}

func TestLessonsContextBlock_AgentPriority(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	tun.SetLessonReflect(true)
	base := time.Now().Unix()

	// Six newer lessons from other agents + one OLDER lesson from AGT1: without
	// prioritization AGT1's would fall outside the newest-5 window.
	for i := 0; i < 6; i++ {
		_, _ = rt.db.AddLesson(db.Lesson{Time: base - int64(10*i), AgentID: "OTHER", Signature: sig("o", i), Text: "other lesson"})
	}
	_, _ = rt.db.AddLesson(db.Lesson{Time: base - 900, AgentID: "AGT1", Signature: "mine", Text: "my own hard-won lesson"})

	b := rt.LessonsContextBlock(ctx, "AGT1")
	if !strings.Contains(b, "my own hard-won lesson") {
		t.Fatalf("own-agent lesson not promoted into the injected set:\n%s", b)
	}
	// Without an agent id the same lesson is outside the newest-5 window.
	if strings.Contains(rt.LessonsContextBlock(ctx, ""), "my own hard-won lesson") {
		t.Errorf("unprioritized block unexpectedly contains the old lesson")
	}
}

func TestLessonsContextBlock_InsightCountIsTrustNeutral(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetLessonReflect(true)
	base := time.Now().Unix()
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, Signature: "lesson:repeated-finding", Text: "insight lesson", Count: 9})
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, Signature: "Bash:repeated-error", Text: "tool lesson", Count: 9})

	block := rt.LessonsContextBlock(context.Background(), "")
	assertLessonBefore(t, block, "insight lesson", "tool lesson")
}

func TestLessonsContextBlock_RecurringToolLessonRanksLater(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetLessonReflect(true)
	base := time.Now().Unix()
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, Signature: "Bash:6f1c9a03b2d84e57", Text: "high count lesson", Count: 8})
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, Signature: "Bash:2ad7e0416b93cf85", Text: "low count lesson", Count: 1})

	block := rt.LessonsContextBlock(context.Background(), "")
	assertLessonBefore(t, block, "low count lesson", "high count lesson")
}

func TestLessonsContextBlock_LowTrustLessonsAreNotFiltered(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetLessonReflect(true)
	base := time.Now().Unix()
	for i := 0; i < lessonsInjectCount; i++ {
		addLessonForTest(t, rt, db.Lesson{
			Time:      base - int64(i),
			Signature: sig("low", i),
			Text:      "low trust lesson " + string(rune('a'+i)),
			Count:     20,
		})
	}

	block := rt.LessonsContextBlock(context.Background(), "")
	if got := strings.Count(block, "\n- "); got != lessonsInjectCount {
		t.Fatalf("injected lesson count = %d, want %d:\n%s", got, lessonsInjectCount, block)
	}
}

func TestLessonsContextBlock_AgentPriorityOutranksTrust(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetLessonReflect(true)
	base := time.Now().Unix()
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, AgentID: "AGT1", Signature: "Bash:c30fb95a17e2d648", Text: "lower trust matching lesson", Count: 20})
	addLessonForTest(t, rt, db.Lesson{Time: base - 60, AgentID: "OTHER", Signature: "Bash:80e4a7f2159cbd36", Text: "higher trust other lesson", Count: 1})

	block := rt.LessonsContextBlock(context.Background(), "AGT1")
	assertLessonBefore(t, block, "lower trust matching lesson", "higher trust other lesson")
}

func addLessonForTest(t *testing.T, rt *Runtime, lesson db.Lesson) {
	t.Helper()
	if _, err := rt.db.AddLesson(lesson); err != nil {
		t.Fatalf("add lesson: %v", err)
	}
}

func assertLessonBefore(t *testing.T, block, first, second string) {
	t.Helper()
	firstAt, secondAt := strings.Index(block, first), strings.Index(block, second)
	if firstAt < 0 || secondAt < 0 || firstAt >= secondAt {
		t.Fatalf("%q must appear before %q:\n%s", first, second, block)
	}
}

func sig(p string, i int) string { return p + string(rune('a'+i)) }
