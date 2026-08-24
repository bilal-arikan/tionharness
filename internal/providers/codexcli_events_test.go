package providers

import (
	"strings"
	"testing"
)

// feedAll pushes every line through the parser, as the subprocess reader does.
func feedAll(p *codexStreamParser, lines ...string) {
	for _, l := range lines {
		p.feed(l)
	}
}

// The fixtures below are REAL lines captured from `codex exec --json`
// (codex-cli 0.147.0); see the implementation contract §1.4. Keep them verbatim.
const (
	fxThreadStarted = `{"type":"thread.started","thread_id":"01a01457-0c11-71d0-b495-615d96b8b913"}`
	fxTurnStarted   = `{"type":"turn.started"}`
	fxAgentMessage  = `{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"OK"}}`
	fxMCPStarted    = `{"type":"item.started","item":{"id":"item_1","type":"mcp_tool_call","server":"tionprobe","tool":"tion_ping","arguments":{"who":"tionharness"},"result":null,"error":null,"status":"in_progress"}}`
	fxMCPCompleted  = `{"type":"item.completed","item":{"id":"item_1","type":"mcp_tool_call","server":"tionprobe","tool":"tion_ping","arguments":{"who":"tionharness"},"result":{"content":[{"type":"text","text":"SECRET-IS-BANANA-FOR-tionharness"}],"structured_content":null},"error":null,"status":"completed"}}`
	fxMCPCancelled  = `{"type":"item.completed","item":{"id":"item_1","type":"mcp_tool_call","server":"tionprobe","tool":"tion_ping","arguments":{"who":"tionharness"},"result":null,"error":{"message":"user cancelled MCP tool call"},"status":"failed"}}`
	fxCmdStarted    = `{"type":"item.started","item":{"id":"item_6","type":"command_execution","command":"\"C:\\\\Windows\\\\System32\\\\WindowsPowerShell\\\\v1.0\\\\powershell.exe\" -Command 'echo hello-from-shell'","aggregated_output":"","exit_code":null,"status":"in_progress"}}`
	fxCmdCompleted  = `{"type":"item.completed","item":{"id":"item_6","type":"command_execution","command":"...","aggregated_output":"hello-from-shell\r\n","exit_code":0,"status":"completed"}}`
	fxTurnCompleted = `{"type":"turn.completed","usage":{"input_tokens":21060,"cached_input_tokens":14592,"cache_write_input_tokens":0,"output_tokens":52,"reasoning_output_tokens":33}}`
	fxTurnFailed    = `{"type":"turn.failed","error":{"message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, ..."}}`
	fxError         = `{"type":"error","message":"Selected model is at capacity. Please try a different model."}`
)

func TestCodexParserHappyTurn(t *testing.T) {
	p := newCodexParser("gpt-5.5", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.Text != "OK" {
		t.Errorf("Text = %q, want %q", resp.Text, "OK")
	}
	if resp.SessionID != "01a01457-0c11-71d0-b495-615d96b8b913" {
		t.Errorf("SessionID = %q", resp.SessionID)
	}
	if resp.Model != "gpt-5.5" {
		t.Errorf("Model = %q", resp.Model)
	}
	u := resp.Usage
	// Codex nests the cache counters inside input_tokens; Usage is disjoint, so
	// the fresh input is 21060 - 14592 cached - 0 written. Reporting the raw
	// 21060 alongside CacheReadTokens would bill the cached prefix twice.
	if u.InputTokens != 6468 {
		t.Errorf("InputTokens = %d, want 6468", u.InputTokens)
	}
	if u.CacheReadTokens != 14592 {
		t.Errorf("CacheReadTokens = %d, want 14592", u.CacheReadTokens)
	}
	if u.CacheWriteTokens != 0 {
		t.Errorf("CacheWriteTokens = %d, want 0", u.CacheWriteTokens)
	}
	if u.OutputTokens != 52 {
		t.Errorf("OutputTokens = %d, want 52", u.OutputTokens)
	}
	// Measured by Codex — must be carried through, not left for the agent layer.
	if u.ThinkingTokens != 33 {
		t.Errorf("ThinkingTokens = %d, want 33", u.ThinkingTokens)
	}
}

// A turn that also WRITES cache: cache_write_input_tokens is a subset of
// input_tokens too, so both cache counters come out of the fresh input.
func TestCodexParserUsageCacheWriteIsSubsetOfInput(t *testing.T) {
	const line = `{"type":"turn.completed","usage":{"input_tokens":30000,"cached_input_tokens":12000,"cache_write_input_tokens":8000,"output_tokens":100,"reasoning_output_tokens":10}}`
	p := newCodexParser("gpt-5.5", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, fxAgentMessage, line)
	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	u := resp.Usage
	if u.InputTokens != 10000 || u.CacheReadTokens != 12000 || u.CacheWriteTokens != 8000 {
		t.Errorf("in=%d read=%d write=%d, want 10000/12000/8000",
			u.InputTokens, u.CacheReadTokens, u.CacheWriteTokens)
	}
	if got := u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens; got != 30000 {
		t.Errorf("disjoint parts sum to %d, want the reported total 30000", got)
	}
}

func TestCodexParserLastAgentMessageWins(t *testing.T) {
	first := `{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"working on it"}}`
	last := `{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"final answer"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, first, last, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.Text != "final answer" {
		t.Errorf("Text = %q, want %q", resp.Text, "final answer")
	}
	// The earlier progress message survives as an intermediate text step.
	if len(resp.Trace) != 1 || resp.Trace[0].Kind != "text" || resp.Trace[0].Text != "working on it" {
		t.Fatalf("Trace = %+v, want one text step %q", resp.Trace, "working on it")
	}
}

func TestCodexParserMCPToolCall(t *testing.T) {
	var events []TraceStep
	p := newCodexParser("", func(s TraceStep) { events = append(events, s) })
	feedAll(p, fxThreadStarted, fxTurnStarted, fxMCPStarted, fxMCPCompleted, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 {
		t.Fatalf("Trace = %+v, want exactly one step", resp.Trace)
	}
	step := resp.Trace[0]
	if step.Kind != "tool" {
		t.Errorf("Kind = %q, want tool", step.Kind)
	}
	if step.Tool != "mcp__tionprobe__tion_ping" {
		t.Errorf("Tool = %q, want mcp__tionprobe__tion_ping", step.Tool)
	}
	if step.Output != "SECRET-IS-BANANA-FOR-tionharness" {
		t.Errorf("Output = %q", step.Output)
	}
	if step.IsError {
		t.Error("IsError = true, want false")
	}
	if !strings.Contains(string(step.Input), "tionharness") {
		t.Errorf("Input = %q, want the arguments object", step.Input)
	}
	// Batch grouping is unavailable on this transport (no assistant-message id).
	if step.Batch != 0 {
		t.Errorf("Batch = %d, want 0", step.Batch)
	}
	// Emitted exactly once, on item.completed.
	if len(events) != 1 || events[0].Tool != "mcp__tionprobe__tion_ping" {
		t.Fatalf("onEvent got %+v, want one tool step", events)
	}
	if !p.ranTool() {
		t.Error("ranTool = false, want true")
	}
}

func TestCodexParserMCPToolCallCancelled(t *testing.T) {
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, fxMCPStarted, fxMCPCancelled, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 {
		t.Fatalf("Trace = %+v, want exactly one step", resp.Trace)
	}
	step := resp.Trace[0]
	if !step.IsError {
		t.Error("IsError = false, want true for a cancelled MCP call")
	}
	if step.Output != "user cancelled MCP tool call" {
		t.Errorf("Output = %q, want the error message", step.Output)
	}
}

func TestCodexParserCommandExecution(t *testing.T) {
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, fxCmdStarted, fxCmdCompleted, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 {
		t.Fatalf("Trace = %+v, want exactly one step", resp.Trace)
	}
	step := resp.Trace[0]
	if step.Tool != "shell" {
		t.Errorf("Tool = %q, want shell", step.Tool)
	}
	if step.Output != "hello-from-shell\r\n" {
		t.Errorf("Output = %q", step.Output)
	}
	if step.IsError {
		t.Error("IsError = true, want false for exit_code 0")
	}
}

func TestCodexParserCommandExecutionNonZeroExit(t *testing.T) {
	failed := `{"type":"item.completed","item":{"id":"item_7","type":"command_execution","command":"false","aggregated_output":"boom\n","exit_code":1,"status":"completed"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, failed, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 || !resp.Trace[0].IsError {
		t.Fatalf("Trace = %+v, want one failing step", resp.Trace)
	}
}

func TestCodexParserTurnFailed(t *testing.T) {
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, fxTurnFailed)

	if _, err := p.finish(); err == nil {
		t.Fatal("finish returned nil error for turn.failed")
	} else if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("error = %v, want the reported message", err)
	}
	if !p.sawTurn {
		t.Error("sawTurn = false, want true — the turn reported a real failure")
	}
	if p.salvage() != nil {
		t.Error("salvage returned a response for a failed turn")
	}
}

func TestCodexParserStandaloneError(t *testing.T) {
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxError)

	if _, err := p.finish(); err == nil {
		t.Fatal("finish returned nil error for an error event")
	} else if !strings.Contains(err.Error(), "at capacity") {
		t.Errorf("error = %v, want the reported message", err)
	}
	// No turn.*/item.* was ever seen: the caller may treat this as a clean crash.
	if p.sawTurn {
		t.Error("sawTurn = true, want false — nothing was produced")
	}
}

func TestCodexParserUnknownEventsSurvive(t *testing.T) {
	unknownEvent := `{"type":"turn.throttled","detail":{"wait_ms":250}}`
	unknownItem := `{"type":"item.completed","item":{"id":"item_9","type":"future_kind","whatever":true}}`
	itemError := `{"type":"item.completed","item":{"id":"item_2","type":"error","message":"Model metadata not found"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, unknownEvent, unknownItem, itemError, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.Text != "OK" {
		t.Errorf("Text = %q, want OK — unknown events must not derail the turn", resp.Text)
	}
	// The item error is visible but non-fatal; the unknown kinds left no trace.
	if len(resp.Trace) != 1 || resp.Trace[0].Kind != "text" {
		t.Fatalf("Trace = %+v, want one text step for the item error", resp.Trace)
	}
	if !strings.Contains(resp.Trace[0].Text, "Model metadata not found") {
		t.Errorf("Trace text = %q", resp.Trace[0].Text)
	}
}

func TestCodexParserSkipsGarbageLines(t *testing.T) {
	p := newCodexParser("", nil)
	feedAll(p, "", "   ", "Reading additional input from stdin...", "{not json}", fxThreadStarted, fxTurnStarted, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if resp.Text != "OK" {
		t.Errorf("Text = %q, want OK", resp.Text)
	}
}

func TestCodexParserSalvageAfterCutoff(t *testing.T) {
	p := newCodexParser("", nil)
	// Stream dies right after the answer — no turn.completed ever arrives.
	feedAll(p, fxThreadStarted, fxTurnStarted, fxAgentMessage)

	if _, err := p.finish(); err == nil {
		t.Fatal("finish returned nil error without turn.completed")
	}
	got := p.salvage()
	if got == nil {
		t.Fatal("salvage returned nil, want the accumulated answer")
	}
	if got.Text != "OK" {
		t.Errorf("salvaged Text = %q, want OK", got.Text)
	}
	if got.SessionID != "01a01457-0c11-71d0-b495-615d96b8b913" {
		t.Errorf("salvaged SessionID = %q", got.SessionID)
	}
}

func TestCodexParserItemUpdatedDoesNotReemit(t *testing.T) {
	updated := `{"type":"item.updated","item":{"id":"item_6","type":"command_execution","command":"...","aggregated_output":"partial","exit_code":null,"status":"in_progress"}}`
	var events []TraceStep
	p := newCodexParser("", func(s TraceStep) { events = append(events, s) })
	feedAll(p, fxThreadStarted, fxTurnStarted, fxCmdStarted, updated, fxCmdCompleted, fxTurnCompleted)

	if len(events) != 1 {
		t.Fatalf("onEvent fired %d times, want 1 (only on item.completed)", len(events))
	}
	if events[0].Output != "hello-from-shell\r\n" {
		t.Errorf("emitted Output = %q, want the completed output", events[0].Output)
	}
	if _, err := p.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestCodexParserReasoningAndFileChange(t *testing.T) {
	reasoning := `{"type":"item.completed","item":{"id":"item_4","type":"reasoning","text":"Checking the config"}}`
	fileChange := `{"type":"item.completed","item":{"id":"item_5","type":"file_change","changes":[{"path":"a.go","kind":"update"},{"path":"b.go","kind":"add"}],"status":"completed"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, reasoning, fileChange, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 2 {
		t.Fatalf("Trace = %+v, want two steps", resp.Trace)
	}
	if resp.Trace[0].Kind != "thinking" || resp.Trace[0].Text != "Checking the config" {
		t.Errorf("step 0 = %+v, want a thinking step", resp.Trace[0])
	}
	if resp.Trace[1].Tool != "apply_patch" {
		t.Errorf("step 1 Tool = %q, want apply_patch", resp.Trace[1].Tool)
	}
	if resp.Trace[1].Output != "update a.go\nadd b.go" {
		t.Errorf("step 1 Output = %q", resp.Trace[1].Output)
	}
}

func TestCodexParserWebSearchAndTodoList(t *testing.T) {
	search := `{"type":"item.completed","item":{"id":"item_8","type":"web_search","query":"golang json flatten","action":"search"}}`
	todo := `{"type":"item.completed","item":{"id":"item_9","type":"todo_list","items":[{"text":"read contract","completed":true},{"text":"write parser","completed":false}]}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, search, todo, fxAgentMessage, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 2 {
		t.Fatalf("Trace = %+v, want two steps", resp.Trace)
	}
	if resp.Trace[0].Tool != "web_search" || resp.Trace[0].Output != "golang json flatten" {
		t.Errorf("step 0 = %+v", resp.Trace[0])
	}
	if resp.Trace[1].Tool != "todo_list" || resp.Trace[1].Output != "[x] read contract\n[ ] write parser" {
		t.Errorf("step 1 = %+v", resp.Trace[1])
	}
}

// An MCP tool that answers with the protocol's own failure flag is reported by
// codex as a transport success (status "completed", error null); only
// result.is_error marks it as a failure.
func TestCodexParserMCPToolCallResultIsError(t *testing.T) {
	failed := `{"type":"item.completed","item":{"id":"item_9","type":"mcp_tool_call","server":"tionprobe","tool":"tion_ping","arguments":"{}","result":{"content":[{"type":"text","text":"missing required argument: name"}],"is_error":true},"status":"completed"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, failed, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 {
		t.Fatalf("Trace = %+v, want exactly one step", resp.Trace)
	}
	step := resp.Trace[0]
	if !step.IsError {
		t.Error("IsError = false, want true for an MCP result carrying is_error")
	}
	if step.Output != "missing required argument: name" {
		t.Errorf("Output = %q, want the tool's error text", step.Output)
	}
}

// A kind the parser does not know is skipped while healthy, but a failed one
// still reaches the trace so a new tool kind cannot fail invisibly.
func TestCodexParserUnknownItemKind(t *testing.T) {
	ok := `{"type":"item.completed","item":{"id":"item_10","type":"future_tool","status":"completed"}}`
	bad := `{"type":"item.completed","item":{"id":"item_11","type":"future_tool","message":"future_tool exploded","status":"failed"}}`
	p := newCodexParser("", nil)
	feedAll(p, fxThreadStarted, fxTurnStarted, ok, bad, fxTurnCompleted)

	resp, err := p.finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if len(resp.Trace) != 1 {
		t.Fatalf("Trace = %+v, want only the failed item", resp.Trace)
	}
	step := resp.Trace[0]
	if step.Tool != "future_tool" || !step.IsError || step.Output != "future_tool exploded" {
		t.Errorf("step = %+v", step)
	}
}
