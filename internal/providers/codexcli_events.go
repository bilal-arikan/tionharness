package providers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// --- codex exec --json event shapes ---
//
// `codex exec --json` writes pure JSONL on stdout: one JSON object per line,
// discriminated by "type". The schema mirrors codex-rs/exec/src/exec_events.rs:
//
//	thread.started                              → {"thread_id": "<uuid>"}
//	turn.started                                → {}
//	turn.completed                              → {"usage": Usage}
//	turn.failed                                 → {"error": {"message": "..."}}
//	item.started | item.updated | item.completed → {"item": ThreadItem}
//	error                                       → {"message": "..."}
//
// Codex serialises a ThreadItem as an internally-tagged enum with the payload
// flattened (serde `#[serde(tag = "type")]` + `#[serde(flatten)]`), so every
// kind-specific field lives at the SAME level as "id" and "type" rather than
// nested under a per-kind object. We therefore model the item as one struct
// holding the union of all kinds' fields, each optional. That is deliberate:
// a per-kind struct set would need a two-pass decode (peek "type", re-unmarshal)
// for no gain, since the field names do not collide across kinds.

// codexUsage is the token accounting reported on turn.completed. All counters
// are i64 on the wire.
type codexUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	CacheWriteInputTokens int `json:"cache_write_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	// ReasoningOutputTokens is MEASURED by Codex (not derived like the Anthropic
	// path), so it maps straight onto Usage.ThinkingTokens.
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

// codexError is the error payload of a turn.failed event and of a failed
// mcp_tool_call item.
type codexError struct {
	Message string `json:"message"`
}

// codexFileChange is one entry of a file_change item's "changes" array.
type codexFileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // add | delete | update
}

// codexTodoItem is one entry of a todo_list item's "items" array.
type codexTodoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// codexMCPContent is one block of an mcp_tool_call result's content array. Only
// text blocks carry displayable text; other kinds (image, resource) are skipped
// by flattenMCPContent.
type codexMCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// codexMCPResult is the successful payload of an mcp_tool_call item.
type codexMCPResult struct {
	Content           []codexMCPContent `json:"content"`
	StructuredContent json.RawMessage   `json:"structured_content"`
}

// codexItem is a ThreadItem: the "id"/"type" envelope plus the flattened union
// of every kind's fields (see the package comment above for why this is one
// struct). Fields are only meaningful for the kinds that emit them.
type codexItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	// agent_message, reasoning, error
	Text    string `json:"text"`
	Message string `json:"message"`

	// command_execution
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code"` // nullable while in_progress

	// file_change
	Changes []codexFileChange `json:"changes"`

	// mcp_tool_call
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    *codexMCPResult `json:"result"`
	Error     *codexError     `json:"error"`

	// collab_tool_call
	SenderThreadID    string   `json:"sender_thread_id"`
	ReceiverThreadIDs []string `json:"receiver_thread_ids"`
	Prompt            string   `json:"prompt"`

	// web_search
	Query  string `json:"query"`
	Action string `json:"action"`

	// todo_list
	Items []codexTodoItem `json:"items"`

	// every kind: in_progress | completed | failed | declined
	Status string `json:"status"`
}

// codexEvent is one line of the JSONL stream.
type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"` // thread.started
	Usage    *codexUsage `json:"usage"`     // turn.completed
	Error    *codexError `json:"error"`     // turn.failed
	Message  string      `json:"message"`   // error
	Item     *codexItem  `json:"item"`      // item.started/updated/completed
}

// codexStreamParser incrementally consumes the codex JSONL stream, building a
// Response.Trace and (when onEvent is set) emitting each step the moment it is
// final. It is the codex dialect of cliStreamParser and carries the same
// responsibilities: incremental emit, tool tracking by id, salvage of a
// truncated stream, and a ranTool signal for retry safety.
type codexStreamParser struct {
	resp    *Response
	onEvent func(TraceStep)
	// itemIdx maps a ThreadItem id → index in resp.Trace. Items arrive as
	// item.started then (optionally item.updated then) item.completed sharing the
	// same id, exactly like claude-cli's tool_use/tool_result pairing.
	itemIdx   map[string]int
	itemStart map[string]time.Time
	emitted   map[int]bool

	finalText string // last agent_message wins — that is the turn's answer
	// sawTurn records whether any turn.*/item.* event was seen at all. The caller
	// uses it to tell "the process died before producing anything" (a clean crash,
	// retryable) from a real, reported failure.
	sawTurn     bool
	sawComplete bool
	hadError    bool
	errText     string
}

func newCodexParser(model string, onEvent func(TraceStep)) *codexStreamParser {
	return &codexStreamParser{
		resp:      &Response{Model: model},
		onEvent:   onEvent,
		itemIdx:   map[string]int{},
		itemStart: map[string]time.Time{},
		emitted:   map[int]bool{},
	}
}

func (p *codexStreamParser) emit(i int) {
	if p.onEvent == nil || p.emitted[i] || i < 0 || i >= len(p.resp.Trace) {
		return
	}
	p.emitted[i] = true
	p.onEvent(p.resp.Trace[i])
}

// appendStep appends a trace step and remembers it under the item id so a later
// item.updated / item.completed can fill it in place.
func (p *codexStreamParser) appendStep(id string, step TraceStep) int {
	p.resp.Trace = append(p.resp.Trace, step)
	idx := len(p.resp.Trace) - 1
	if id != "" {
		p.itemIdx[id] = idx
		p.itemStart[id] = time.Now()
	}
	return idx
}

// feed processes one line of the stream. Blank and non-JSON lines are skipped,
// as are unknown event types and unknown item kinds — Codex adds event kinds
// between releases and an unknown one must never fail the turn.
func (p *codexStreamParser) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ev codexEvent
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}

	switch ev.Type {
	case "thread.started":
		// The thread id is STABLE across resumes (unlike claude-cli's rotating
		// session id), so the caller can store it once and reuse it every turn.
		if ev.ThreadID != "" {
			p.resp.SessionID = ev.ThreadID
		}
	case "turn.started":
		p.sawTurn = true
	case "turn.completed":
		p.sawTurn = true
		p.sawComplete = true
		if u := ev.Usage; u != nil {
			p.resp.Usage.InputTokens = u.InputTokens
			p.resp.Usage.CacheReadTokens = u.CachedInputTokens
			p.resp.Usage.CacheWriteTokens = u.CacheWriteInputTokens
			p.resp.Usage.OutputTokens = u.OutputTokens
			// Measured, not estimated — the codex path must never overwrite this
			// with deriveThinkingTokens' guess.
			p.resp.Usage.ThinkingTokens = u.ReasoningOutputTokens
		}
	case "turn.failed":
		p.sawTurn = true
		p.hadError = true
		if ev.Error != nil && ev.Error.Message != "" {
			p.errText = ev.Error.Message
		}
	case "error":
		p.hadError = true
		if ev.Message != "" {
			p.errText = ev.Message
		}
	case "item.started", "item.updated", "item.completed":
		p.sawTurn = true
		if ev.Item != nil {
			p.feedItem(ev.Type, ev.Item)
		}
	}
}

// feedItem routes one ThreadItem to its kind handler. The step is created on
// item.started, refreshed in place on item.updated, and finalised + emitted on
// item.completed. An item that only ever appears as item.completed (the CLI
// skips "started" for instantaneous items — see the agent_message fixture) is
// created and emitted in that single call.
func (p *codexStreamParser) feedItem(evType string, it *codexItem) {
	final := evType == "item.completed"

	switch it.Type {
	case "agent_message":
		p.feedAgentMessage(it, final)
	case "reasoning":
		p.setStep(it, final, TraceStep{Kind: "thinking", Text: strings.TrimSpace(it.Text)})
	case "command_execution":
		p.setStep(it, final, TraceStep{
			Kind:    "tool",
			Tool:    "shell",
			Input:   jsonString(it.Command),
			Output:  it.AggregatedOutput,
			IsError: commandFailed(it),
		})
	case "file_change":
		p.setStep(it, final, TraceStep{
			Kind:    "tool",
			Tool:    "apply_patch",
			Output:  summarizeFileChanges(it.Changes),
			IsError: it.Status == "failed",
		})
	case "mcp_tool_call":
		p.setStep(it, final, TraceStep{
			// Codex namespaces MCP tools exactly like claude-cli, so the existing
			// mcp.SplitNamespaced / trace-stripping helpers work unchanged.
			Kind:    "tool",
			Tool:    "mcp__" + it.Server + "__" + it.Tool,
			Input:   it.Arguments,
			Output:  mcpToolOutput(it),
			IsError: it.Error != nil || it.Status == "failed",
		})
	case "web_search":
		p.setStep(it, final, TraceStep{Kind: "tool", Tool: "web_search", Output: it.Query})
	case "todo_list":
		p.setStep(it, final, TraceStep{Kind: "tool", Tool: "todo_list", Output: summarizeTodoList(it.Items)})
	case "collab_tool_call":
		p.setStep(it, final, TraceStep{
			Kind:    "tool",
			Tool:    "collab_tool_call",
			Output:  summarizeCollab(it),
			IsError: it.Status == "failed",
		})
	case "error":
		// A non-fatal item error (observed: "Model metadata not found"). Surface it
		// in the trace so it is visible, but do NOT fail the turn — the turn can and
		// does complete normally afterwards.
		p.setStep(it, final, TraceStep{Kind: "text", Text: "[codex error] " + strings.TrimSpace(it.Message)})
	default:
		// Unknown item kind: skip silently. The parser has no logger, and a future
		// Codex release adding a kind must not break an otherwise healthy turn.
	}
}

// setStep creates or updates the trace step backing this item id, and emits it
// once the item is final. TraceStep.Batch is deliberately left at 0 everywhere:
// Codex reports no assistant-message id to group parallel tool calls by, so the
// UI's parallel-batch grouping is unavailable on this transport. Known,
// accepted gap.
func (p *codexStreamParser) setStep(it *codexItem, final bool, step TraceStep) {
	idx, ok := p.itemIdx[it.ID]
	if !ok || it.ID == "" {
		idx = p.appendStep(it.ID, step)
	} else {
		// Preserve the arrival time; replace the payload in place.
		p.resp.Trace[idx] = step
	}
	if !final {
		return
	}
	if start, ok := p.itemStart[it.ID]; ok {
		p.resp.Trace[idx].DurMs = time.Since(start).Milliseconds()
		delete(p.itemStart, it.ID)
	}
	p.emit(idx)
}

// feedAgentMessage handles the assistant's visible text. Codex emits SEVERAL
// agent_message items over a turn — progress notes followed by the real answer —
// so the LAST completed one is the final text and the earlier ones are demoted
// to plain "text" trace steps (mirroring how the claude parser flushes
// intermediate text before a tool call).
func (p *codexStreamParser) feedAgentMessage(it *codexItem, final bool) {
	if !final {
		return // nothing to show until the text is complete
	}
	text := strings.TrimSpace(it.Text)
	if text == "" {
		return
	}
	if p.finalText != "" {
		// The previously-final message turned out to be an intermediate one.
		p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: p.finalText})
		p.emit(len(p.resp.Trace) - 1)
	}
	p.finalText = text
}

// finish resolves the final answer, or reports the failure the stream carried.
func (p *codexStreamParser) finish() (*Response, error) {
	if p.hadError {
		msg := strings.TrimSpace(p.errText)
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("codex CLI error: %s", msg)
	}
	if !p.sawComplete {
		return nil, fmt.Errorf("codex CLI: no turn.completed in stream")
	}
	// Emit any tool step whose completion never arrived, so the UI still sees it.
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			p.emit(i)
		}
	}
	p.resp.Text = p.finalText
	return p.resp, nil
}

// salvage recovers whatever the parser accumulated when the stream was cut off
// before turn.completed (the subprocess died mid-turn). Returns nil when the
// stream carried a genuine error — that must propagate, not be masked as
// success — or when there is nothing usable.
func (p *codexStreamParser) salvage() *Response {
	if p.hadError {
		return nil
	}
	text := strings.TrimSpace(p.finalText)
	if text == "" {
		for i := len(p.resp.Trace) - 1; i >= 0; i-- {
			if p.resp.Trace[i].Kind == "text" && strings.TrimSpace(p.resp.Trace[i].Text) != "" {
				text = strings.TrimSpace(p.resp.Trace[i].Text)
				break
			}
		}
	}
	if text == "" {
		return nil
	}
	p.resp.Text = text
	return p.resp
}

// ranTool reports whether any tool executed during the turn — a crashed turn
// that reached a tool may have side effects and is NOT safe to retry.
func (p *codexStreamParser) ranTool() bool {
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			return true
		}
	}
	return false
}

// commandFailed reports whether a command_execution item ended badly: an
// explicit "failed" status, or a non-zero exit code. A nil exit code means the
// command is still running and is not a failure by itself.
func commandFailed(it *codexItem) bool {
	if it.Status == "failed" {
		return true
	}
	return it.ExitCode != nil && *it.ExitCode != 0
}

// mcpToolOutput renders the displayable result of an mcp_tool_call: the error
// message when the call failed, otherwise the flattened text blocks (mirroring
// toolResultText on the claude path), falling back to the structured content
// when the server returned no text blocks.
func mcpToolOutput(it *codexItem) string {
	if it.Error != nil {
		return it.Error.Message
	}
	if it.Result == nil {
		return ""
	}
	if s := flattenMCPContent(it.Result.Content); s != "" {
		return s
	}
	if len(it.Result.StructuredContent) > 0 && string(it.Result.StructuredContent) != "null" {
		return string(it.Result.StructuredContent)
	}
	return ""
}

// flattenMCPContent concatenates the text of an MCP result's content blocks.
func flattenMCPContent(blocks []codexMCPContent) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Text != "" {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// summarizeFileChanges renders a file_change item's paths and change kinds as a
// compact one-line-per-file summary.
func summarizeFileChanges(changes []codexFileChange) string {
	if len(changes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(changes))
	for _, c := range changes {
		parts = append(parts, c.Kind+" "+c.Path)
	}
	return strings.Join(parts, "\n")
}

// summarizeTodoList renders a todo_list item as checkbox lines.
func summarizeTodoList(items []codexTodoItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, t := range items {
		mark := "[ ] "
		if t.Completed {
			mark = "[x] "
		}
		parts = append(parts, mark+t.Text)
	}
	return strings.Join(parts, "\n")
}

// summarizeCollab renders a collab_tool_call item: which sub-tool ran, against
// how many receiver threads, and the prompt it carried.
func summarizeCollab(it *codexItem) string {
	parts := []string{}
	if it.Tool != "" {
		parts = append(parts, it.Tool)
	}
	if n := len(it.ReceiverThreadIDs); n > 0 {
		parts = append(parts, strconv.Itoa(n)+" receiver(s)")
	}
	if it.Prompt != "" {
		parts = append(parts, it.Prompt)
	}
	return strings.Join(parts, " · ")
}

// jsonString encodes s as a JSON string for a TraceStep.Input, which is raw
// JSON rather than plain text.
func jsonString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}
