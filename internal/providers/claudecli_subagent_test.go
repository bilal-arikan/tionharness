package providers

import (
	"strings"
	"testing"
)

// A CLI-native subagent's events (parent_tool_use_id set) fold into the Agent
// step that launched it: its text never reaches the main reply, its tool calls
// land in SubSteps with their results, each addition republishes the parent as
// a running card, and the final tool_result closes the card with the same id.
func TestCLIStreamFoldsSubagentEventsIntoLauncher(t *testing.T) {
	var live []TraceStep
	p := newCLIParser("", func(s TraceStep) { live = append(live, s) })
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Let me look."}]}}`,
		`{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"toolu_agent","name":"Agent","input":{"subagent_type":"Explore","description":"find callers","prompt":"Find callers of foo"}}]}}`,
		`{"type":"assistant","parent_tool_use_id":"toolu_agent","message":{"id":"m3","content":[{"type":"thinking","thinking":"scanning"},{"type":"tool_use","id":"toolu_sub1","name":"Grep","input":{"pattern":"foo"}}]}}`,
		`{"type":"user","parent_tool_use_id":"toolu_agent","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_sub1","content":"a.go:1"}]}}`,
		`{"type":"assistant","parent_tool_use_id":"toolu_agent","message":{"id":"m4","content":[{"type":"text","text":"Two callers in a.go."}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_agent","content":"Two callers in a.go."}]}}`,
		`{"type":"assistant","message":{"id":"m5","content":[{"type":"text","text":"foo is called twice."}]}}`,
		`{"type":"result","subtype":"success","result":"foo is called twice.","session_id":"s1","num_turns":3}`,
	}
	for _, l := range lines {
		p.feed(l)
	}
	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.Text != "foo is called twice." {
		t.Fatalf("reply = %q; subagent text must not leak into the main reply", resp.Text)
	}
	// Main trace: intermediate text + the Agent step; nothing from the child at top level.
	var agent *TraceStep
	for i := range resp.Trace {
		st := &resp.Trace[i]
		if st.Tool == "Grep" || (st.Kind == "text" && strings.Contains(st.Text, "Two callers")) {
			t.Fatalf("subagent event leaked into the main trace: %+v", st)
		}
		if st.Tool == "Agent" {
			agent = st
		}
	}
	if agent == nil {
		t.Fatalf("no Agent step in trace: %+v", resp.Trace)
	}
	if agent.ID != "toolu_agent" || agent.Output != "Two callers in a.go." || agent.IsError {
		t.Fatalf("Agent step = %+v", *agent)
	}
	if len(agent.SubSteps) != 3 {
		t.Fatalf("sub-steps = %+v, want thinking + Grep + text", agent.SubSteps)
	}
	if agent.SubSteps[0].Kind != "thinking" || agent.SubSteps[1].Tool != "Grep" || agent.SubSteps[1].Output != "a.go:1" || agent.SubSteps[2].Text != "Two callers in a.go." {
		t.Fatalf("sub-steps = %+v", agent.SubSteps)
	}
	if agent.SubSteps[1].DurMs < 0 {
		t.Fatalf("nested tool latency must be measured, got %d", agent.SubSteps[1].DurMs)
	}

	// Live: three running republishes (thinking+tool_use, tool_result, text) then the
	// final closed card, all sharing the launcher's id.
	var running, final int
	for _, s := range live {
		if s.Tool != "Agent" {
			continue
		}
		if s.ID != "toolu_agent" {
			t.Fatalf("live Agent card without the launcher id: %+v", s)
		}
		if s.Running {
			running++
		} else {
			final++
		}
	}
	if running != 3 || final != 1 {
		t.Fatalf("live Agent cards: running=%d final=%d, want 3/1 (%+v)", running, final, live)
	}
	if last := live[len(live)-1]; last.Running || last.Tool != "Agent" || len(last.SubSteps) != 3 {
		// The final emission is the tool_result of the launcher, which comes before
		// the closing main text; find it explicitly.
		var closed *TraceStep
		for i := range live {
			if live[i].Tool == "Agent" && !live[i].Running {
				closed = &live[i]
			}
		}
		if closed == nil || len(closed.SubSteps) != 3 {
			t.Fatalf("closed Agent card must carry the full nested trace: %+v", closed)
		}
	}
}

// An event tagged with a parent the parser never saw (a nested spawn deeper
// than the launcher it knows, or a stale id) falls back to the main trace
// instead of being dropped.
func TestCLIStreamUnknownParentFallsBackToMainTrace(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"assistant","parent_tool_use_id":"toolu_missing","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_x","name":"Read","input":{"file_path":"x"}}]}}`)
	p.feed(`{"type":"user","parent_tool_use_id":"toolu_missing","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_x","content":"body"}]}}`)
	p.feed(`{"type":"result","subtype":"success","result":"ok","session_id":"s1"}`)
	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 || resp.Trace[0].Tool != "Read" || resp.Trace[0].Output != "body" {
		t.Fatalf("trace = %+v, want the orphan tool at top level", resp.Trace)
	}
}

// The launch env and the system note follow the request flag together: with
// native subagents on, the depth cap + text forwarding travel and the note names
// the Explore/Plan menu; off, neither does.
func TestNativeSubagentEnvAndNote(t *testing.T) {
	if env := nativeSubagentEnv(Request{}); env != nil {
		t.Fatalf("env off = %v, want nil", env)
	}
	env := nativeSubagentEnv(Request{CLINativeSubagents: true})
	if len(env) != 2 || env[0] != "CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1" || env[1] != "CLAUDE_CODE_FORWARD_SUBAGENT_TEXT=1" {
		t.Fatalf("env on = %v", env)
	}
	if note := interactionNote(true); !strings.Contains(note, "subagent_type Explore or Plan") || strings.Contains(note, "never use the built-in Task or Agent") {
		t.Fatalf("native note = %q", note)
	}
	if note := interactionNote(false); !strings.Contains(note, "never use the built-in Task or Agent") {
		t.Fatalf("default note = %q", note)
	}
	// Both wordings allow the mirrored native TodoWrite and still forbid the
	// unmirrored TaskCreate family.
	for _, note := range []string{interactionNote(true), interactionNote(false)} {
		if !strings.Contains(note, "built-in TodoWrite") || !strings.Contains(note, "TaskCreate/TaskUpdate/TaskList/TaskGet") {
			t.Fatalf("note checklist wording = %q", note)
		}
	}
}
