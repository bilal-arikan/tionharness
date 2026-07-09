package providers

import "testing"

// TestCLIParserParallelBatchGrouping: the CLI splits ONE API assistant message
// (carrying several parallel tool_use blocks) into several stream events sharing
// the same message id. The parser must stamp all of that message's tool steps
// with one shared Batch id — including retroactively the first, whose result has
// not arrived yet — while lone calls stay Batch=0 and a second parallel message
// gets the NEXT id.
func TestCLIParserParallelBatchGrouping(t *testing.T) {
	p := newCLIParser("claude-opus-4-8", nil)

	// Message msg_1: two parallel Writes, split across two assistant events.
	p.feed(`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"tool_use","id":"t1","name":"Write","input":{}}]}}`)
	p.feed(`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"tool_use","id":"t2","name":"Write","input":{}}]}}`)
	p.feed(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`)
	p.feed(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"ok"}]}}`)
	// Message msg_2: a lone call → no batch.
	p.feed(`{"type":"assistant","message":{"id":"msg_2","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{}}]}}`)
	p.feed(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":"ok"}]}}`)
	// Message msg_3: three parallel calls in ONE event → next batch id.
	p.feed(`{"type":"assistant","message":{"id":"msg_3","content":[` +
		`{"type":"tool_use","id":"t4","name":"Write","input":{}},` +
		`{"type":"tool_use","id":"t5","name":"Write","input":{}},` +
		`{"type":"tool_use","id":"t6","name":"Write","input":{}}]}}`)

	var batches []int
	for _, ts := range p.resp.Trace {
		if ts.Kind == "tool" {
			batches = append(batches, ts.Batch)
		}
	}
	want := []int{1, 1, 0, 2, 2, 2}
	if len(batches) != len(want) {
		t.Fatalf("got %d tool steps, want %d (batches=%v)", len(batches), len(want), batches)
	}
	for i := range want {
		if batches[i] != want[i] {
			t.Fatalf("batch ids = %v, want %v", batches, want)
		}
	}
}
