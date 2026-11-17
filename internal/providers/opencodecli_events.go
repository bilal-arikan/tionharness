package providers

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// --- opencode run --format json event shapes ---
//
// `opencode run --format json` writes one JSON object per line on stdout,
// discriminated by "type". Shapes verified live on opencode 1.19.0:
//
//	step_start   → {"sessionID": "...", "part": {"type":"step-start", ...}}
//	text         → {"part": {"id":"prt_…", "type":"text", "text":"…"}}
//	reasoning    → {"part": {"id":"prt_…", "type":"reasoning", "text":"…"}}
//	tool_use     → {"part": {"type":"tool", "tool":"read", "callID":"…",
//	                 "state": {"status":"completed"|"error"|"running"|"pending",
//	                           "input":{…}, "output":"…", "error":"…"}}}
//	step_finish  → {"part": {"type":"step-finish", "reason":"stop"|"tool-calls",
//	                 "tokens": {"input":N,"output":N,"reasoning":N,
//	                            "cache":{"read":N,"write":N}}, "cost":F}}
//	error        → {"error": {"name":"UnknownError", "data":{"message":"…"}}}
//
// 2.x (verified on 2.2.5) keeps these event types with three differences the
// parser absorbs: a tool part is streamed ONCE, when it completed or failed (no
// pending/running line, so a long tool is a silent open step); its "id" is the
// provider call id ("callID" is gone, "partID" holds the part id); and bridged
// MCP tools run inside Code Mode as the native "execute" tool, whose
// state.metadata.metadata.toolCalls names the real call ("<server>.<tool>").
//
// Every event carries the top-level "sessionID" (the opencode session, stable
// across `-s` resumes). Token counters are DISJOINT: "input" excludes the
// cached prefix (measured: input 152 + cache.read 6659 on a warm step), so the
// mapping onto Usage is direct, unlike codex's nested counters.

type opencodeTokens struct {
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	Cache     struct {
		Read  int `json:"read"`
		Write int `json:"write"`
	} `json:"cache"`
}

type opencodeToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
	Error  string          `json:"error"`
	Title  string          `json:"title"`
	// Metadata is tool-specific; kept raw so an unexpected shape can never
	// drop the whole line (see opencodeCodeModeCalls).
	Metadata json.RawMessage `json:"metadata"`
}

// opencodeCodeModeCall is one tool call made from inside a 2.x Code Mode
// `execute` run.
type opencodeCodeModeCall struct {
	Tool   string          `json:"tool"`
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
}

// opencodeCodeModeCalls reads state.metadata.metadata.toolCalls of a 2.x
// `execute` tool part; nil when absent or malformed.
func opencodeCodeModeCalls(meta json.RawMessage) []opencodeCodeModeCall {
	if len(meta) == 0 {
		return nil
	}
	var m struct {
		Metadata struct {
			ToolCalls []opencodeCodeModeCall `json:"toolCalls"`
		} `json:"metadata"`
	}
	if json.Unmarshal(meta, &m) != nil {
		return nil
	}
	return m.Metadata.ToolCalls
}

type opencodePart struct {
	ID     string             `json:"id"`
	Type   string             `json:"type"`
	Text   string             `json:"text"`
	Tool   string             `json:"tool"`
	CallID string             `json:"callID"`
	State  *opencodeToolState `json:"state"`
	Reason string             `json:"reason"`
	Tokens *opencodeTokens    `json:"tokens"`
}

type opencodeErrorData struct {
	Message string `json:"message"`
}

// opencodeError covers both error shapes: 1.x {"name", "data":{"message"}} and
// 2.x {"type":"provider.free-limit", "message", "status":429, ...} (verified
// on 2.2.5).
type opencodeError struct {
	Name    string            `json:"name"`
	Data    opencodeErrorData `json:"data"`
	Type    string            `json:"type"`
	Message string            `json:"message"`
	Status  int               `json:"status"`
}

type opencodeEvent struct {
	Type      string         `json:"type"`
	SessionID string         `json:"sessionID"`
	Part      *opencodePart  `json:"part"`
	Error     *opencodeError `json:"error"`
}

// opencodeStreamParser incrementally consumes the opencode JSONL stream,
// building a Response.Trace and (when onEvent is set) emitting each step the
// moment it is final. It is the opencode dialect of codexStreamParser with the
// same responsibilities: incremental emit, tool tracking by id, salvage of a
// truncated stream and a ranTool signal for retry safety.
type opencodeStreamParser struct {
	resp    *Response
	onEvent func(TraceStep)
	// serverKeys are the MCP server keys configured for this turn; opencode
	// names a bridged tool "<key>_<tool>" and the trace wants "mcp__<key>__<tool>"
	// so the existing prefix-stripping helpers render it like the other CLIs.
	serverKeys []string

	partIdx   map[string]int // part id / callID → index in resp.Trace
	partStart map[string]time.Time
	emitted   map[int]bool

	finalText string
	finalID   string
	sawStep   bool
	sawFinish bool
	hadError  bool
	errName   string
	errText   string
	steps     int

	notedParseDrop bool
	parseDropCount int
	// toolsActive holds the callIDs of tool parts seen pending/running and not
	// yet completed or failed: opencode is silent while a tool runs, so the
	// stdout watchdog asks toolInFlight before reading that as a hang (same
	// contract as the codex parser).
	toolsActive map[string]bool
	// silentTools marks a 2.x stream, which never reports a tool as running:
	// there an open step (step_start without its step_finish) counts as busy,
	// since the silence is either the model generating or a tool executing.
	silentTools bool
	stepOpen    bool
}

func newOpenCodeParser(model string, serverKeys []string, onEvent func(TraceStep)) *opencodeStreamParser {
	return &opencodeStreamParser{
		resp:        &Response{Model: model},
		onEvent:     onEvent,
		serverKeys:  serverKeys,
		partIdx:     map[string]int{},
		partStart:   map[string]time.Time{},
		emitted:     map[int]bool{},
		toolsActive: map[string]bool{},
	}
}

// toolInFlight reports whether a tool part has started and not yet finished,
// or — on a 2.x stream, which cannot tell — whether a step is still open.
func (p *opencodeStreamParser) toolInFlight() bool {
	return len(p.toolsActive) > 0 || (p.silentTools && p.stepOpen)
}

func (p *opencodeStreamParser) emit(i int) {
	if p.onEvent == nil || p.emitted[i] || i < 0 || i >= len(p.resp.Trace) {
		return
	}
	p.emitted[i] = true
	p.onEvent(p.resp.Trace[i])
}

func (p *opencodeStreamParser) noteParseDrop(line string, err error) {
	p.parseDropCount++
	if p.notedParseDrop {
		return
	}
	p.notedParseDrop = true
	note := boundedNote(fmt.Sprintf("[opencode parse drop] line bytes=%d: %v; payload=%s", len(line), err, line))
	p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: note})
	p.emit(len(p.resp.Trace) - 1)
}

func (p *opencodeStreamParser) summarizeParseDrops() {
	n := p.parseDropCount - 1
	if n <= 0 {
		return
	}
	p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: fmt.Sprintf("[opencode parse drop] +%d more line(s) dropped this turn", n)})
	p.emit(len(p.resp.Trace) - 1)
}

// feed processes one line of the stream. Blank and non-JSON lines are skipped,
// as are unknown event and part types — opencode adds kinds between releases
// and an unknown one must never fail the turn.
func (p *opencodeStreamParser) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ev opencodeEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		p.noteParseDrop(line, err)
		return
	}
	if ev.SessionID != "" && p.resp.SessionID == "" {
		p.resp.SessionID = ev.SessionID
	}
	switch ev.Type {
	case "step_start":
		p.sawStep = true
		p.stepOpen = true
	case "step_finish":
		p.sawStep = true
		p.stepOpen = false
		p.steps++
		if ev.Part != nil && ev.Part.Tokens != nil {
			t := ev.Part.Tokens
			prompt := t.Input + t.Cache.Read + t.Cache.Write
			if p.steps == 1 {
				p.resp.FirstCallPromptTokens = prompt
			}
			p.resp.Usage.InputTokens += t.Input
			p.resp.Usage.OutputTokens += t.Output
			p.resp.Usage.CacheReadTokens += t.Cache.Read
			p.resp.Usage.CacheWriteTokens += t.Cache.Write
			// Measured by opencode; never overwrite with an estimate.
			p.resp.Usage.ThinkingTokens += t.Reasoning
			p.resp.Usage.ThinkingTokensMeasured = true
		}
		p.resp.ProviderCalls = p.steps
		// "stop" ends the turn; "tool-calls" means another step follows.
		if ev.Part != nil && ev.Part.Reason != "tool-calls" {
			p.sawFinish = true
		}
	case "text":
		p.sawStep = true
		if ev.Part != nil {
			p.feedText(ev.Part)
		}
	case "reasoning":
		p.sawStep = true
		if ev.Part != nil && strings.TrimSpace(ev.Part.Text) != "" {
			p.setPart(ev.Part.ID, true, TraceStep{Kind: "thinking", Text: strings.TrimSpace(ev.Part.Text)})
		}
	case "tool_use":
		p.sawStep = true
		if ev.Part != nil {
			p.feedTool(ev.Part)
		}
	case "error":
		p.hadError = true
		p.stepOpen = false
		if e := ev.Error; e != nil {
			p.errName = e.Name
			if p.errName == "" {
				p.errName = e.Type
			}
			if e.Status > 0 {
				// The HTTP status feeds classification ("429", "401").
				p.errName = strings.TrimSpace(fmt.Sprintf("%s %d", p.errName, e.Status))
			}
			p.errText = e.Data.Message
			if p.errText == "" {
				p.errText = e.Message
			}
			if p.errText == "" {
				p.errText = p.errName
			}
		}
	}
}

// feedText records an assistant text part. opencode emits several text parts
// over a turn (narration between tool calls, then the answer): the LAST one is
// the final text and earlier ones are demoted to plain "text" trace steps,
// mirroring the codex parser. A part re-sent under the same id replaces the
// earlier payload.
func (p *opencodeStreamParser) feedText(part *opencodePart) {
	text := strings.TrimSpace(part.Text)
	if text == "" {
		return
	}
	if p.finalText != "" && p.finalID != part.ID {
		p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: p.finalText})
		p.emit(len(p.resp.Trace) - 1)
	}
	p.finalText = text
	p.finalID = part.ID
}

// feedTool records a tool part keyed by its callID. The step is created on the
// first sighting (pending/running) and finalised + emitted on completed/error.
func (p *opencodeStreamParser) feedTool(part *opencodePart) {
	key := part.CallID
	if key == "" {
		key = part.ID
	}
	step := TraceStep{Kind: "tool", Tool: p.toolName(part.Tool)}
	final := false
	if st := part.State; st != nil {
		step.Input = st.Input
		if part.Tool == "execute" {
			p.unwrapCodeMode(&step, st)
		}
		switch st.Status {
		case "completed":
			final = true
			step.Output = st.Output
		case "error":
			final = true
			step.IsError = true
			step.Output = st.Error
			if step.Output == "" {
				step.Output = st.Output
			}
		}
	}
	if step.Tool == "todo_list" {
		step.Output = summarizeOpenCodeTodos(step.Input)
	}
	if key != "" {
		if final {
			delete(p.toolsActive, key)
		} else {
			if p.toolsActive == nil {
				p.toolsActive = map[string]bool{}
			}
			p.toolsActive[key] = true
		}
	}
	p.setPart(key, final, step)
}

// unwrapCodeMode renders a 2.x Code Mode `execute` part that made exactly one
// call to a configured MCP server as that bridged tool, so the trace shows
// "mcp__<server>__<tool>" with the tool's own input instead of a JavaScript
// snippet. Multi-call or opencode-internal runs stay "execute".
func (p *opencodeStreamParser) unwrapCodeMode(step *TraceStep, st *opencodeToolState) {
	calls := opencodeCodeModeCalls(st.Metadata)
	if len(calls) != 1 {
		return
	}
	ns, name, ok := strings.Cut(calls[0].Tool, ".")
	if !ok || name == "" {
		return
	}
	for _, key := range p.serverKeys {
		if opencodeSanitizeID(key) != ns {
			continue
		}
		step.Tool = mcp.NamespaceTool("mcp", mcp.NamespaceTool(key, name))
		if len(calls[0].Input) > 0 {
			step.Input = calls[0].Input
		}
		if calls[0].Status == "error" {
			step.IsError = true
		}
		return
	}
}

// toolName maps an opencode tool id onto the trace's naming conventions:
// bridged MCP tools regain the mcp__<server>__ prefix, opencode's checklist tool
// takes the shared "todo_list" name so the agent layer promotes it to a
// checklist card and mirrors it into the progress sink.
func (p *opencodeStreamParser) toolName(name string) string {
	for _, key := range p.serverKeys {
		if rest, ok := strings.CutPrefix(name, opencodeSanitizeID(key)+"_"); ok && rest != "" {
			return mcp.NamespaceTool("mcp", mcp.NamespaceTool(key, rest))
		}
	}
	switch name {
	case "todowrite":
		return "todo_list"
	}
	return name
}

// summarizeOpenCodeTodos renders a todowrite input ({"todos":[{content,status}]},
// the same shape as the bridged todo_write) as checkbox lines.
func summarizeOpenCodeTodos(input json.RawMessage) string {
	var in struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	if len(input) == 0 || json.Unmarshal(input, &in) != nil || len(in.Todos) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in.Todos))
	for _, t := range in.Todos {
		mark := "[ ] "
		if t.Status == "completed" {
			mark = "[x] "
		}
		parts = append(parts, mark+t.Content)
	}
	return strings.Join(parts, "\n")
}

// setPart creates or updates the trace step backing id and emits it once final.
func (p *opencodeStreamParser) setPart(id string, final bool, step TraceStep) {
	step.Output = CapToolOutput(step.Output)
	idx, ok := p.partIdx[id]
	if !ok || id == "" {
		p.resp.Trace = append(p.resp.Trace, step)
		idx = len(p.resp.Trace) - 1
		if id != "" {
			p.partIdx[id] = idx
			p.partStart[id] = time.Now()
		}
	} else {
		p.resp.Trace[idx] = step
	}
	if !final {
		return
	}
	if start, ok := p.partStart[id]; ok {
		p.resp.Trace[idx].DurMs = time.Since(start).Milliseconds()
		p.resp.Trace[idx].DurationMs = p.resp.Trace[idx].DurMs
		delete(p.partStart, id)
	}
	p.emit(idx)
}

// finish resolves the final answer, or reports the failure the stream carried.
func (p *opencodeStreamParser) finish() (*Response, error) {
	p.summarizeParseDrops()
	if p.hadError {
		msg := strings.TrimSpace(p.errText)
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("opencode CLI error: %s", msg)
	}
	if !p.sawFinish {
		return nil, fmt.Errorf("opencode CLI: no final step_finish in stream")
	}
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			if !p.emitted[i] && p.resp.Trace[i].Output == "" {
				p.resp.Trace[i].IsError = true
				p.resp.Trace[i].Output = "opencode CLI ended before the tool call completed; no tool result was received"
			}
			p.emit(i)
		}
	}
	p.resp.Text = p.finalText
	return p.resp, nil
}

func (p *opencodeStreamParser) usageError(err error) error {
	return WithUsage(err, p.resp.Model, p.resp.Usage, p.resp.ProviderCalls)
}

// salvage recovers whatever the parser accumulated when the stream was cut off
// before the final step_finish. nil when the stream carried a genuine error or
// there is nothing usable.
func (p *opencodeStreamParser) salvage() *Response {
	if p.hadError {
		return nil
	}
	text := strings.TrimSpace(p.finalText)
	if text == "" {
		for _, v := range slices.Backward(p.resp.Trace) {
			if v.Kind == "text" && strings.TrimSpace(v.Text) != "" {
				text = strings.TrimSpace(v.Text)
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

func (p *opencodeStreamParser) ranTool() bool {
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			return true
		}
	}
	return false
}

// opencodeFailureClass names why an opencode turn failed terminally.
type opencodeFailureClass int

const (
	opencodeFailureNone opencodeFailureClass = iota
	opencodeFailureAuth
	opencodeFailureQuota
	opencodeFailureModel
)

// classifyOpenCodeError maps an error event (name + message) onto a failure
// class by case-insensitive signature matching. Unrecognised text yields
// opencodeFailureNone so the caller falls back to stream-shape retry rules.
func classifyOpenCodeError(name, msg string) opencodeFailureClass {
	s := strings.ToLower(name + " " + msg)
	switch {
	case strings.Contains(s, "autherror"), strings.Contains(s, "unauthorized"), strings.Contains(s, "401"),
		strings.Contains(s, "invalid api key"), strings.Contains(s, "api key"), strings.Contains(s, "not logged in"),
		strings.Contains(s, "authentication"):
		return opencodeFailureAuth
	case strings.Contains(s, "rate limit"), strings.Contains(s, "429"), strings.Contains(s, "quota"),
		strings.Contains(s, "usage limit"), strings.Contains(s, "overloaded"):
		return opencodeFailureQuota
	case strings.Contains(s, "model not found"), strings.Contains(s, "unknown model"), strings.Contains(s, "modelnotfound"),
		strings.Contains(s, "no such model"):
		return opencodeFailureModel
	}
	return opencodeFailureNone
}

func newOpenCodeFailureError(class opencodeFailureClass, msg, home string) error {
	msg = strings.TrimSpace(msg)
	var described string
	switch class {
	case opencodeFailureAuth:
		described = fmt.Sprintf("opencode CLI: provider authentication failed (%s) — run `opencode auth login` in this instance's home (%s)", msg, home)
	case opencodeFailureQuota:
		described = "opencode CLI: provider rate/usage limit (" + msg + ")"
	case opencodeFailureModel:
		described = "opencode CLI: model unavailable for this account (" + msg + ")"
	default:
		described = "opencode CLI error: " + msg
	}
	if class == opencodeFailureAuth || class == opencodeFailureModel {
		return &permanentProviderError{message: described}
	}
	return fmt.Errorf("%s", described)
}
