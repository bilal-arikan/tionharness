package agent

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
)

func TestReactiveCompactionStepCarriesFoldFigures(t *testing.T) {
	fold := conversation.ReactiveFold{
		Compaction: conversation.Compaction{
			FoldedMsgs:   7,
			BeforeTokens: 12000,
			AfterTokens:  3400,
			Trigger:      conversation.TriggerReactive,
		},
		SavedBytes:   4096,
		SummaryBytes: 512,
	}

	step := reactiveCompactionStep(fold, nil)

	if step.Kind != StepCompaction {
		t.Fatalf("Kind = %q, want %q", step.Kind, StepCompaction)
	}
	if step.FoldedMsgs != 7 {
		t.Errorf("FoldedMsgs = %d, want 7", step.FoldedMsgs)
	}
	if step.BeforeTokens != 12000 {
		t.Errorf("BeforeTokens = %d, want 12000", step.BeforeTokens)
	}
	if step.AfterTokens != 3400 {
		t.Errorf("AfterTokens = %d, want 3400", step.AfterTokens)
	}
	if step.Trigger != conversation.TriggerReactive {
		t.Errorf("Trigger = %q, want %q", step.Trigger, conversation.TriggerReactive)
	}
	// Text stays populated for clients that predate the structural fields.
	if !strings.Contains(step.Text, "7") || !strings.Contains(step.Text, "12000") || !strings.Contains(step.Text, "3400") {
		t.Errorf("Text does not carry the fold figures: %q", step.Text)
	}
}

// TestReactiveCompactionStepJSONKeys pins the wire contract the chat UI reads
// (frontend/src/types/message.ts). Renaming a JSON tag must fail here.
func TestReactiveCompactionStepJSONKeys(t *testing.T) {
	step := reactiveCompactionStep(conversation.ReactiveFold{
		Compaction: conversation.Compaction{
			FoldedMsgs:   2,
			BeforeTokens: 900,
			AfterTokens:  120,
			Trigger:      conversation.TriggerReactive,
		},
	}, nil)

	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]string{
		"kind":         `"compaction"`,
		"foldedMsgs":   `2`,
		"beforeTokens": `900`,
		"afterTokens":  `120`,
		"trigger":      `"reactive"`,
		"source":       `"tionharness"`,
	}
	for key, wantVal := range want {
		val, ok := got[key]
		if !ok {
			t.Errorf("missing key %q in %s", key, raw)
			continue
		}
		if string(val) != wantVal {
			t.Errorf("%q = %s, want %s", key, val, wantVal)
		}
	}
	if _, ok := got["text"]; !ok {
		t.Errorf("missing key %q in %s", "text", raw)
	}
	if extra := keysBeyond(got, "kind", "text", "foldedMsgs", "beforeTokens", "afterTokens", "trigger", "source"); len(extra) > 0 {
		t.Errorf("unexpected keys %v in %s", extra, raw)
	}
}

// TestReactiveCompactionStepOmitsZeroFields guards the omitempty tags: a zero
// fold must not ship 0-valued counters the UI would render as a real fold.
func TestReactiveCompactionStepOmitsZeroFields(t *testing.T) {
	raw, err := json.Marshal(reactiveCompactionStep(conversation.ReactiveFold{}, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"foldedMsgs", "beforeTokens", "afterTokens", "trigger"} {
		if _, ok := got[key]; ok {
			t.Errorf("key %q present on a zero fold: %s", key, raw)
		}
	}
}

func keysBeyond(m map[string]json.RawMessage, allowed ...string) []string {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	var extra []string
	for k := range m {
		if !set[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return extra
}
