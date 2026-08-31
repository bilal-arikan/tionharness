package providers

import "testing"

func TestHasCompletedNativeCompactionRejectsRunningOnly(t *testing.T) {
	running := &Response{Trace: []TraceStep{{Kind: "compaction", Source: "cli-native", Running: true}}}
	if hasCompletedNativeCompaction(running) {
		t.Fatal("running-only lifecycle must not count as completed")
	}
	completed := &Response{Trace: []TraceStep{{Kind: "compaction", Source: "cli-native"}}}
	if !hasCompletedNativeCompaction(completed) {
		t.Fatal("completed lifecycle was not recognized")
	}
}

func TestCodexCompactCompletedRequiresMatchingLifecycle(t *testing.T) {
	var envelope codexRPCEnvelope
	envelope.Method = "item/completed"
	envelope.Params.ThreadID = "thread-1"
	envelope.Params.Item.Type = "contextCompaction"
	if !codexCompactCompleted(envelope, "thread-1") {
		t.Fatal("matching contextCompaction completion was not recognized")
	}
	if codexCompactCompleted(envelope, "thread-2") {
		t.Fatal("completion from another thread was accepted")
	}
	envelope.Params.Item.Type = "agentMessage"
	if codexCompactCompleted(envelope, "thread-1") {
		t.Fatal("unrelated completed item was accepted")
	}
}
