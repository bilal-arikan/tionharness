package api

import (
	"encoding/json"
	"testing"
)

// appendChanges must keep exactly what the chat renders as a diff card and drop
// everything else — the popup's file count is compared against those cards.
func TestAppendChangesFilter(t *testing.T) {
	cases := []struct {
		name string
		step string
		want bool
	}{
		{"native diff step", `{"kind":"diff","tool":"Write","path":"/a.go","patch":"+x"}`, true},
		{"cli edit tool step", `{"kind":"tool","tool":"Edit","input":{"file_path":"/a.go"}}`, true},
		{"namespaced edit tool", `{"kind":"tool","tool":"mcp__srv__write_file","input":{}}`, true},
		{"errored edit changed nothing", `{"kind":"tool","tool":"Edit","isError":true}`, false},
		{"errored diff", `{"kind":"diff","tool":"Write","isError":true}`, false},
		{"read is not a mutation", `{"kind":"tool","tool":"Read","input":{}}`, false},
		{"thinking", `{"kind":"thinking","text":"hm"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appendChanges(nil, json.RawMessage(tc.step), "MSG1", "AGT1", 42, false)
			if (len(got) == 1) != tc.want {
				t.Fatalf("kept=%d, want kept=%v", len(got), tc.want)
			}
			if tc.want && (got[0].MsgID != "MSG1" || got[0].CreatedAt != 42 || got[0].Nested) {
				t.Fatalf("origin not tagged: %+v", got[0])
			}
		})
	}
}

// A subagent's edits hit the same disk as the parent agent's, so they belong in
// the list — flagged, so the UI does not attribute them to the main agent.
func TestAppendChangesRecursesSubSteps(t *testing.T) {
	step := `{"kind":"subagent","tool":"run_subagent","subSteps":[
		{"kind":"diff","tool":"Write","path":"/nested.go","patch":"+x"},
		{"kind":"tool","tool":"Read","input":{}}
	]}`
	got := appendChanges(nil, json.RawMessage(step), "MSG1", "AGT1", 7, false)
	if len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
	if !got[0].Nested {
		t.Error("subagent change not flagged as nested")
	}
}

// A subagent run that ended in error can still have applied real edits before it
// failed; those files really did change.
func TestAppendChangesKeepsEditsUnderErroredParent(t *testing.T) {
	step := `{"kind":"subagent","tool":"run_subagent","isError":true,"subSteps":[
		{"kind":"diff","tool":"Write","path":"/nested.go","patch":"+x"}
	]}`
	if got := appendChanges(nil, json.RawMessage(step), "M", "A", 1, false); len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
}

// The step is passed through verbatim: the frontend synthesizes the patch for
// the claude-cli path from the very fields a re-marshal could reorder or drop.
func TestAppendChangesPreservesRawStep(t *testing.T) {
	raw := `{"kind":"tool","tool":"Edit","input":{"file_path":"/a.go","old_string":"a","new_string":"b"}}`
	got := appendChanges(nil, json.RawMessage(raw), "M", "A", 1, false)
	if len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
	if string(got[0].Step) != raw {
		t.Errorf("step rewritten:\n got %s\nwant %s", got[0].Step, raw)
	}
}

func TestToolBaseName(t *testing.T) {
	for in, want := range map[string]string{
		"Edit":                 "edit",
		"mcp__server__Write":   "write",
		"tionswarm__multiedit": "multiedit",
		"":                     "",
		"WebFetch":             "webfetch",
	} {
		if got := toolBaseName(in); got != want {
			t.Errorf("toolBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}
