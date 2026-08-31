package providers

import (
	"testing"
	"time"
)

func TestClaudeParserNativeCompactionLifecycle(t *testing.T) {
	var events []TraceStep
	p := newCLIParser("", func(step TraceStep) { events = append(events, step) })
	p.feed(`{"type":"system","subtype":"hook_started","hook_id":"hook-1","hook_event":"PreCompact","session_id":"session-1"}`)
	p.feed(`{"type":"system","subtype":"compact_boundary","session_id":"session-1"}`)
	p.feed(`{"type":"system","subtype":"hook_response","hook_id":"hook-2","hook_event":"PostCompact","session_id":"session-1"}`)

	if len(events) != 2 || !events[0].Running || events[1].Running {
		t.Fatalf("events = %+v, want running then completed", events)
	}
	if events[0].ID != "hook-1" || events[1].ID != "hook-1" {
		t.Fatalf("event IDs = %q/%q, want correlated hook-1", events[0].ID, events[1].ID)
	}
	if events[1].Kind != "compaction" || events[1].Source != "cli-native" || events[1].Provider != "claude-cli" {
		t.Fatalf("completed event = %+v", events[1])
	}
	if len(p.resp.Trace) != 1 {
		t.Fatalf("durable trace = %+v, want one deduplicated completion", p.resp.Trace)
	}
}

func TestClaudeParserNativeCompactionMultipleCycles(t *testing.T) {
	var events []TraceStep
	p := newCLIParser("", func(step TraceStep) { events = append(events, step) })
	for _, id := range []string{"hook-1", "hook-2"} {
		p.feed(`{"type":"system","subtype":"hook_started","hook_id":"` + id + `","hook_event":"PreCompact"}`)
		p.feed(`{"type":"system","subtype":"compact_boundary"}`)
	}
	if len(events) != 4 || events[2].ID != "hook-2" || events[3].ID != "hook-2" {
		t.Fatalf("events = %+v, want two complete lifecycles", events)
	}
	if len(p.resp.Trace) != 2 {
		t.Fatalf("durable trace = %+v, want two completions", p.resp.Trace)
	}
}

func TestClaudeParserNativeCompactionCompletionSignals(t *testing.T) {
	for name, line := range map[string]string{
		"boundary":  `{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"auto"}}`,
		"post-hook": `{"type":"system","subtype":"hook_response","hook_id":"post-1","hook_event":"PostCompact"}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := newCLIParser("", nil)
			p.feed(line)
			if len(p.resp.Trace) != 1 || p.resp.Trace[0].Kind != "compaction" {
				t.Fatalf("trace = %+v", p.resp.Trace)
			}
		})
	}
}

func TestClaudeParserConsecutiveBoundaryOnlyCompactions(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"compact_boundary"}`)
	p.feed(`{"type":"system","subtype":"compact_boundary"}`)
	if len(p.resp.Trace) != 2 {
		t.Fatalf("trace = %+v, want two boundary-only completions", p.resp.Trace)
	}
}

func TestClaudeParserNativeCompactionTimeoutRetractsRunning(t *testing.T) {
	events := make(chan TraceStep, 2)
	p := newCLIParser("", func(step TraceStep) { events <- step })
	p.nativeCompactionTimeout = 5 * time.Millisecond
	p.feed(`{"type":"system","subtype":"hook_started","hook_id":"hook-timeout","hook_event":"PreCompact"}`)

	first := <-events
	if !first.Running || first.ID != "hook-timeout" {
		t.Fatalf("first event = %+v", first)
	}
	select {
	case got := <-events:
		if got.Kind != "tombstone" || got.Ref != "hook-timeout" {
			t.Fatalf("timeout event = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("missing compaction timeout tombstone")
	}
}

func TestClaudeParserOverlappingPreCompactRetractsPreviousRunning(t *testing.T) {
	var events []TraceStep
	p := newCLIParser("", func(step TraceStep) { events = append(events, step) })
	p.feed(`{"type":"system","subtype":"hook_started","hook_id":"old","hook_event":"PreCompact"}`)
	p.feed(`{"type":"system","subtype":"hook_started","hook_id":"new","hook_event":"PreCompact"}`)
	p.feed(`{"type":"system","subtype":"compact_boundary"}`)

	if len(events) != 4 || events[1].Kind != "tombstone" || events[1].Ref != "old" {
		t.Fatalf("events = %+v, want old running/tombstone then new lifecycle", events)
	}
	if events[2].ID != "new" || !events[2].Running || events[3].ID != "new" || events[3].Running {
		t.Fatalf("new lifecycle = %+v", events[2:])
	}
}

func TestClaudeParserStatusCompactionFailedRetractsRunning(t *testing.T) {
	var events []TraceStep
	p := newCLIParser("", func(step TraceStep) { events = append(events, step) })
	p.feed(`{"type":"system","subtype":"status","status":"compacting"}`)
	p.feed(`{"type":"system","subtype":"status","status":null,"compact_result":"failed","compact_error":"Not enough messages to compact."}`)
	p.feed(`{"type":"system","subtype":"compact_boundary"}`)

	if len(events) != 2 || !events[0].Running || events[1].Kind != "tombstone" || events[1].Ref != events[0].ID {
		t.Fatalf("events = %+v, want running then correlated tombstone", events)
	}
	if len(p.resp.Trace) != 0 {
		t.Fatalf("trace = %+v, failed compaction must not complete", p.resp.Trace)
	}
	if got := p.resp.NativeCompactionError; got != "Not enough messages to compact." {
		t.Fatalf("NativeCompactionError = %q", got)
	}
}

func TestClaudeParserStatusCompactionSuccess(t *testing.T) {
	var events []TraceStep
	p := newCLIParser("", func(step TraceStep) { events = append(events, step) })
	p.feed(`{"type":"system","subtype":"status","status":"compacting"}`)
	p.feed(`{"type":"system","subtype":"status","status":null,"compact_result":"success"}`)

	if len(events) != 2 || !events[0].Running || events[1].Running || events[0].ID != events[1].ID {
		t.Fatalf("events = %+v, want correlated running and completion", events)
	}
	if got := p.resp.Trace; len(got) != 1 || got[0].Source != "cli-native" || got[0].SessionAction != "native-compact" {
		t.Fatalf("trace = %+v", got)
	}
}

func TestClaudeParserStatusCompactionDeduplicatesOtherSignals(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"status","status":"compacting"}`)
	p.feed(`{"type":"system","subtype":"hook_started","hook_id":"pre-1","hook_event":"PreCompact"}`)
	p.feed(`{"type":"system","subtype":"status","status":null,"compact_result":"success"}`)
	p.feed(`{"type":"system","subtype":"compact_boundary"}`)
	p.feed(`{"type":"system","subtype":"hook_response","hook_id":"post-1","hook_event":"PostCompact"}`)

	if len(p.resp.Trace) != 1 {
		t.Fatalf("trace = %+v, want one completion", p.resp.Trace)
	}
}

func TestClaudeParserStatusCompactionMultipleCycles(t *testing.T) {
	p := newCLIParser("", nil)
	for range 2 {
		p.feed(`{"type":"system","subtype":"status","status":"compacting"}`)
		p.feed(`{"type":"system","subtype":"status","status":null,"compact_result":"success"}`)
	}
	if len(p.resp.Trace) != 2 || p.resp.Trace[0].ID == p.resp.Trace[1].ID {
		t.Fatalf("trace = %+v, want two distinct cycles", p.resp.Trace)
	}
}

func TestClaudeParserStatusOnlyCompactionTimeout(t *testing.T) {
	events := make(chan TraceStep, 2)
	p := newCLIParser("", func(step TraceStep) { events <- step })
	p.nativeCompactionTimeout = 5 * time.Millisecond
	p.feed(`{"type":"system","subtype":"status","status":"compacting"}`)

	first := <-events
	if !first.Running || first.ID == "" {
		t.Fatalf("first event = %+v", first)
	}
	select {
	case got := <-events:
		if got.Kind != "tombstone" || got.Ref != first.ID {
			t.Fatalf("timeout event = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("missing status-only compaction timeout tombstone")
	}
}
