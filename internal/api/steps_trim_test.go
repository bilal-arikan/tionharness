package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// decodeSteps is a test helper: the trimmer's contract is JSON-in / JSON-out.
func decodeSteps(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("trimmed steps are not valid JSON: %v", err)
	}
	return out
}

func TestTrimStepsSmallTracePassesThroughVerbatim(t *testing.T) {
	raw := `[{"kind":"tool","tool":"Read","output":"short"}]`
	if got := trimStepsJSON(raw); got != raw {
		t.Fatalf("small trace was rewritten:\n got %q\nwant %q", got, raw)
	}
}

func TestTrimStepsCutsOversizedOutput(t *testing.T) {
	big := strings.Repeat("x", stepFieldCap*3)
	raw, err := json.Marshal([]map[string]any{
		{"kind": "tool", "tool": "Bash", "output": big},
	})
	if err != nil {
		t.Fatal(err)
	}
	steps := decodeSteps(t, trimStepsJSON(string(raw)))
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	out, _ := steps[0]["output"].(string)
	if len(out) != stepFieldCap {
		t.Fatalf("output not cut to cap: got %d bytes, want %d", len(out), stepFieldCap)
	}
	if steps[0]["outputTruncated"] != true {
		t.Fatal("outputTruncated flag missing — the UI cannot offer the full trace")
	}
	if n, _ := steps[0]["outputLen"].(float64); int(n) != len(big) {
		t.Fatalf("outputLen = %v, want %d", steps[0]["outputLen"], len(big))
	}
}

// The input object's KEYS drive the tool label, the program tag and the
// synthesized Edit/Write diff, so trimming must reach only the long leaves.
func TestTrimStepsPreservesInputStructure(t *testing.T) {
	raw, err := json.Marshal([]map[string]any{{
		"kind":  "tool",
		"tool":  "Write",
		"input": map[string]any{"file_path": "a.go", "content": strings.Repeat("y", stepFieldCap*2)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	steps := decodeSteps(t, trimStepsJSON(string(raw)))
	in, ok := steps[0]["input"].(map[string]any)
	if !ok {
		t.Fatalf("input is no longer an object: %T", steps[0]["input"])
	}
	if in["file_path"] != "a.go" {
		t.Fatalf("short leaf was altered: %v", in["file_path"])
	}
	if got, _ := in["content"].(string); len(got) != stepFieldCap {
		t.Fatalf("long leaf not cut: %d bytes", len(got))
	}
	if steps[0]["inputTruncated"] != true {
		t.Fatal("inputTruncated flag missing")
	}
}

func TestTrimStepsRecursesIntoSubSteps(t *testing.T) {
	raw, err := json.Marshal([]map[string]any{{
		"kind": "subagent",
		"subSteps": []map[string]any{
			{"kind": "tool", "tool": "Grep", "output": strings.Repeat("z", stepFieldCap*2)},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	steps := decodeSteps(t, trimStepsJSON(string(raw)))
	subs, ok := steps[0]["subSteps"].([]any)
	if !ok || len(subs) != 1 {
		t.Fatalf("subSteps lost: %v", steps[0]["subSteps"])
	}
	sub := subs[0].(map[string]any)
	if sub["outputTruncated"] != true {
		t.Fatal("nested subagent output was not trimmed")
	}
}

// A trace we cannot parse must be served as-is rather than dropped.
func TestTrimStepsUnparseableIsServedVerbatim(t *testing.T) {
	raw := "[{" + strings.Repeat("!", stepFieldCap)
	if got := trimStepsJSON(raw); got != raw {
		t.Fatal("unparseable trace was not passed through unchanged")
	}
}

// Cutting mid-rune would serialize as U+FFFD and corrupt the preview.
func TestTrimStepsCutsOnRuneBoundary(t *testing.T) {
	// "ç" is 2 bytes, so a naive cut at stepFieldCap lands inside a rune.
	big := strings.Repeat("ç", stepFieldCap)
	raw, err := json.Marshal([]map[string]any{{"kind": "tool", "output": "a" + big}})
	if err != nil {
		t.Fatal(err)
	}
	steps := decodeSteps(t, trimStepsJSON(string(raw)))
	out, _ := steps[0]["output"].(string)
	if strings.ContainsRune(out, '�') {
		t.Fatal("truncation split a rune")
	}
	if len(out) > stepFieldCap {
		t.Fatalf("cut overshot the cap: %d bytes", len(out))
	}
}
