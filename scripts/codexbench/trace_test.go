package main

import (
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestNativeTraceChunkingAndFailures(t *testing.T) {
	s := &nativeStream{trace: &toolTrace{start: time.Now()}, starts: map[string]int64{}, seen: map[string]bool{}}
	lines := []string{
		`{"type":"item.started","item":{"id":"a","type":"command_execution"}}`,
		`{"type":"item.completed","item":{"id":"a","type":"command_execution","command":"python check.py","exit_code":1,"aggregated_output":"AssertionError","status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"a","type":"command_execution"}}`,
		`{"type":"item.completed","item":{"id":"b","type":"file_change","status":"completed","changes":[{"path":"solution.py","diff":"+pass"}]}}`,
		`{"type":"item.completed","item":{"id":"c","type":"mcp_tool_call","result":{"is_error":true}}}`,
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":9}}`,
	}
	for _, line := range lines {
		for _, b := range []byte(line + "\n") {
			_, _ = s.Write([]byte{b})
		}
	}
	s.flush()
	if s.err != nil || !s.completed || len(s.trace.records) != 3 {
		t.Fatalf("unexpected stream: %+v", s)
	}
	if !s.trace.records[0].IsError || s.trace.records[0].DurationMs == nil {
		t.Fatal("failed command or measured duration lost")
	}
	if s.trace.records[1].DurationMs != nil {
		t.Fatal("missing start event must not become zero duration")
	}
	if !s.trace.records[2].IsError {
		t.Fatal("MCP protocol failure lost")
	}
	if s.usage.InputTokens != 60 || s.usage.CacheReadTokens != 40 {
		t.Fatal("cache counted twice")
	}
}

func TestProviderTraceUnknownDuration(t *testing.T) {
	trace := &toolTrace{start: time.Now()}
	trace.providerEvent(providers.TraceStep{Kind: "tool", Running: true})
	trace.providerEvent(providers.TraceStep{Kind: "text"})
	trace.providerEvent(providers.TraceStep{Kind: "tool", Tool: "shell", IsError: true})
	trace.providerEvent(providers.TraceStep{Kind: "tool", Tool: "apply_patch", DurMs: 12})
	if len(trace.records) != 2 || !trace.records[0].IsError || trace.records[0].DurationMs != nil || *trace.records[1].DurationMs != 12 {
		t.Fatalf("unexpected trace: %+v", trace.records)
	}
}
