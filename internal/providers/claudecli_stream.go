package providers

// claude-cli stream parsing: the incremental consumer of the CLI's stream-json
// event log. It turns the raw event lines into a Response (trace steps, usage,
// native-compaction lifecycle, rate-limit/auth classification) and is driven by
// runAttempt in claudecli.go, which owns the subprocess itself.

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// cliStreamParser incrementally consumes the stream-json event log, building a
// Response.Trace and (when onEvent is set) emitting each step the moment it is
// ready: thinking immediately, intermediate text on flush, a tool step once its
// result arrives. The trailing text is the final answer (not emitted as a step).
type cliStreamParser struct {
	resp           *Response
	onEvent        func(TraceStep)
	toolIdx        map[string]int // tool_use id → index in resp.Trace
	emitted        map[int]bool   // trace index → already delivered via onEvent
	pending        strings.Builder
	finalText      string
	sawResult      bool
	hadError       bool
	errText        string
	notedMCP       bool                 // the unusable-MCP-server note was already emitted for this turn
	notedParseDrop bool                 // malformed stream JSON was already reported for this turn
	notedBlock     map[string]bool      // assistant content block types already reported as unknown
	sawModelTurn   bool                 // any assistant/tool/result content seen (vs. only system/init noise)
	rateLimited    bool                 // the turn was rejected by a subscription usage / rate limit
	rateLimitMsg   string               // human-readable detail for the rate-limit failure
	notLoggedIn    bool                 // the turn was rejected because this claude-home is not authenticated
	authMsg        string               // human-readable detail for the auth failure ("Not logged in · ...")
	toolStart      map[string]time.Time // tool_use id → time the event was seen (for per-tool latency)
	// Parallel-batch grouping: the CLI splits ONE API assistant message (which may
	// carry several parallel tool_use blocks) into several stream events sharing the
	// same message id. Track the current message's tool trace indices so the 2nd+
	// tool_use of a message allocates a batch id and stamps the earlier ones too.
	curMsgID                    string // assistant message id currently being accumulated
	curMsgTools                 []int  // trace indices of that message's tool steps
	batchSeq                    int    // 1-based batch id allocator (unique within the turn)
	nativeCompactionID          string
	nativeCompactionHasStart    bool
	nativeCompactionDone        bool
	nativeCompactionSignal      string
	nativeCompactionHookID      string
	nativeCompactionFailed      bool
	nativeCompactionStatusStart bool
	nativeCompactionSeq         int
	nativeCompactionMu          sync.Mutex
	nativeCompactionTimer       *time.Timer
	nativeCompactionTimeout     time.Duration
}

// primaryModelUsage returns the model key that consumed the most tokens in a
// result envelope's modelUsage map, chosen deterministically (alphabetical
// tie-break). A turn may invoke several models — e.g. an auxiliary tool-search
// haiku call next to the primary answer — so the map has multiple keys; the one
// that processed the full context (largest total tokens) is the answer's model.
// Output tokens alone are misleading (the tiny auxiliary call can emit more), so
// score by input + output + cache.
func primaryModelUsage(mu map[string]json.RawMessage) string {
	best := ""
	bestScore := -1
	for k, raw := range mu {
		var u struct {
			InputTokens              int `json:"inputTokens"`
			OutputTokens             int `json:"outputTokens"`
			CacheReadInputTokens     int `json:"cacheReadInputTokens"`
			CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
		}
		_ = json.Unmarshal(raw, &u)
		score := u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		if score > bestScore || (score == bestScore && (best == "" || k < best)) {
			best, bestScore = k, score
		}
	}
	return best
}

func newCLIParser(model string, onEvent func(TraceStep)) *cliStreamParser {
	return &cliStreamParser{
		resp:       &Response{Model: model},
		onEvent:    onEvent,
		toolIdx:    map[string]int{},
		emitted:    map[int]bool{},
		toolStart:  map[string]time.Time{},
		notedBlock: map[string]bool{},
	}
}

func (p *cliStreamParser) emit(i int) {
	if p.onEvent == nil || p.emitted[i] || i < 0 || i >= len(p.resp.Trace) {
		return
	}
	p.emitted[i] = true
	p.onEvent(p.resp.Trace[i])
}

// note appends an out-of-band parser note to the trace as a plain "text" step.
// TraceStep has no dedicated warning kind (kinds are text|thinking|tool), so
// this follows the existing convention of a bracket-prefixed text step (see the
// codex parser's "[codex error] ..." steps) — visible in the activity view
// without pretending to be a tool call.
func (p *cliStreamParser) note(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	p.flushText()
	p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: text})
	p.emit(len(p.resp.Trace) - 1)
}

// noteUnknownBlock reports an assistant content block this parser does not model
// (once per type per turn). Without it a new block type — server_tool_use was
// the real case — vanishes from the trace with no trace of the omission.
func (p *cliStreamParser) noteUnknownBlock(blockType string) {
	t := strings.TrimSpace(blockType)
	if t == "" {
		t = "(missing type)"
	}
	if p.notedBlock[t] {
		return
	}
	p.notedBlock[t] = true
	p.note("[claude-cli] unhandled content block skipped: " + t)
}

// noteParseDrop reports the first malformed JSON event in a turn. The bounded
// payload keeps a broken stream from flooding the activity trace while retaining
// the line length and parser error needed to diagnose a lost tool call.
func (p *cliStreamParser) noteParseDrop(line string, err error) {
	if p.notedParseDrop {
		return
	}
	p.notedParseDrop = true
	const maxNoteBytes = 500
	note := fmt.Sprintf("[claude-cli parse drop] line bytes=%d: %v; payload=%s", len(line), err, line)
	if len(note) > maxNoteBytes {
		note = note[:maxNoteBytes]
		for !utf8.ValidString(note) {
			note = note[:len(note)-1]
		}
	}
	p.note(note)
}

// describePermissionDenials renders the result envelope's permission_denials
// array as a one-line note, or "" when nothing was denied.
func describePermissionDenials(ds []cliPermissionDenial) string {
	if len(ds) == 0 {
		return ""
	}
	names := make([]string, 0, len(ds))
	for _, d := range ds {
		if n := strings.TrimSpace(d.ToolName); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		names = append(names, "(unnamed tool)")
	}
	return fmt.Sprintf("[permission] %d permission denial(s): %s", len(ds), strings.Join(names, ", "))
}

// describeUnusableMCPServers renders the system/init MCP inventory as a
// one-line note naming every server whose tools are NOT available this turn,
// with the reason the CLI gave; "" when every server is usable. "connected"
// and "pending" (a cached client still warming up) are usable; every other
// status is not.
func describeUnusableMCPServers(ev cliEvent) string {
	var parts []string
	for _, s := range ev.MCPServers {
		status := strings.ToLower(strings.TrimSpace(s.Status))
		if status == "connected" || status == "pending" || status == "" {
			continue
		}
		parts = append(parts, mcpServerLabel(s.Name)+" ("+status+")")
	}
	for _, f := range ev.FailedMCPServers {
		reason := strings.TrimSpace(f.Error)
		if reason == "" {
			reason = strings.TrimSpace(f.ErrorCode)
		}
		if reason == "" {
			reason = "failed to start"
		}
		parts = append(parts, mcpServerLabel(f.Name)+" ("+trimOneLine(reason, 160)+")")
	}
	if len(parts) == 0 {
		return ""
	}
	return "[mcp] unavailable MCP server(s) in this turn: " + strings.Join(parts, ", ") +
		" — their tools are missing from the tool catalog until the server is back up."
}

// mcpServerLabel keeps an unnamed server from rendering as an empty label.
func mcpServerLabel(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "(unnamed server)"
}

// trimOneLine collapses s to a single line bounded by limit runes-ish bytes.
func trimOneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if limit > 0 && len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

// hookWarningText renders a failed hook (non-zero exit, or an outcome that says
// it failed/blocked) as a one-line note with a trimmed stderr; "" when the hook
// succeeded, so successful hooks add no trace noise.
func hookWarningText(ev cliEvent) string {
	outcome := strings.ToLower(strings.TrimSpace(ev.Outcome))
	failed := ev.ExitCode != 0 ||
		strings.Contains(outcome, "fail") ||
		strings.Contains(outcome, "error") ||
		strings.Contains(outcome, "block") ||
		strings.Contains(outcome, "deny")
	if !failed {
		return ""
	}
	name := strings.TrimSpace(ev.HookName)
	if name == "" {
		name = strings.TrimSpace(ev.HookEvent)
	}
	if name == "" {
		name = "(unnamed hook)"
	}
	msg := fmt.Sprintf("[hook] %s failed (exit %d)", name, ev.ExitCode)
	if outcome != "" {
		msg += ", outcome " + strings.TrimSpace(ev.Outcome)
	}
	if errOut := strings.TrimSpace(ev.Stderr); errOut != "" {
		const maxStderr = 500
		if len(errOut) > maxStderr {
			errOut = errOut[:maxStderr] + "…"
		}
		msg += ": " + errOut
	}
	return msg
}

func (p *cliStreamParser) flushText() {
	t := strings.TrimSpace(p.pending.String())
	p.pending.Reset()
	if t != "" {
		p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: t})
		p.emit(len(p.resp.Trace) - 1)
	}
}

// feed processes one event line from the stream.
func (p *cliStreamParser) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ev cliEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		p.noteParseDrop(line, err)
		return
	}
	// Capture the CLI session id wherever it appears (system/init first, result
	// last). The result event's id is the one to resume from next turn, so letting
	// later events overwrite is correct.
	if ev.SessionID != "" {
		p.resp.SessionID = ev.SessionID
	}
	// A login lapse surfaces first as a standalone {"error":"authentication_failed"}
	// line (before the result envelope). Catch it here so the failure is classified
	// as auth regardless of which event carried the signal.
	if isAuthErrorText(ev.Error) {
		p.notLoggedIn = true
		if p.authMsg == "" {
			p.authMsg = strings.TrimSpace(ev.Error)
		}
	}

	switch ev.Type {
	case "system":
		if ev.Subtype == "status" {
			if ev.Status != nil && *ev.Status == "compacting" {
				p.startNativeCompaction("")
			}
			switch ev.CompactResult {
			case "success":
				p.completeNativeCompaction(ev, "status")
			case "failed":
				p.resp.NativeCompactionError = strings.TrimSpace(ev.CompactError)
				p.failNativeCompaction()
			}
		}
		if ev.Subtype == "hook_started" && ev.HookEvent == "PreCompact" {
			p.startNativeCompaction(ev.HookID)
		}
		if ev.Subtype == "compact_boundary" {
			p.completeNativeCompaction(ev, "boundary")
		} else if ev.Subtype == "hook_response" && ev.HookEvent == "PostCompact" {
			p.completeNativeCompaction(ev, "post")
		}
		// hook_response reports how each configured hook ran. A hook that exits
		// non-zero can silently strip a tool call or block an edit, and the turn
		// still ends "successfully" — so make the failure visible. Successful
		// hooks stay silent (they fire on every step; noting them would drown
		// the trace).
		if ev.Subtype == "hook_response" {
			p.note(hookWarningText(ev))
		}
		// system/init lists every MCP server with its connection status. A server
		// that is not connected costs the turn its tools without failing the turn,
		// so it must be said once — same visibility contract as the native and
		// codex paths.
		if ev.Subtype == "init" && !p.notedMCP {
			if t := describeUnusableMCPServers(ev); t != "" {
				p.notedMCP = true
				p.note(t)
			}
		}
	case "rate_limit_event":
		// The CLI reports the subscription rate-limit window on every turn. The
		// "allowed" family lets the request proceed: "allowed" is the normal case and
		// "allowed_warning" only signals the window is filling up (observed event:
		// status=allowed_warning, utilization 0.64, isUsingOverage=false — 36% of quota
		// still free). Only a status OUTSIDE that family (e.g. "rejected"/"blocked")
		// means the window is exhausted and, with overage disabled, the request is
		// refused before any assistant output. Matching just "allowed" mis-flagged the
		// warning as a hard limit, reported a bogus "usage/rate limit reached" and
		// masked the real exit cause (letting the !sawModelTurn branch classify it,
		// which is also retryable when no tool ran).
		if rl := ev.RateLimit; rl != nil && rl.Status != "" && !strings.HasPrefix(strings.ToLower(rl.Status), "allowed") {
			p.rateLimited = true
			p.rateLimitMsg = describeRateLimit(rl)
		}
	case "assistant":
		p.sawModelTurn = true
		if ev.Message == nil {
			return
		}
		// New API message → reset the parallel-batch accumulator (see curMsgID).
		// An empty id (older CLI) degrades to per-event grouping, which is still
		// correct when one event carries all of a message's blocks.
		if ev.Message.ID != p.curMsgID {
			p.curMsgID = ev.Message.ID
			p.curMsgTools = p.curMsgTools[:0]
		}
		if ev.Message.Model != "" {
			p.resp.Model = ev.Message.Model
		}
		if ev.Message.Usage != nil {
			p.resp.Usage.OutputTokens += ev.Message.Usage.OutputTokens
			if ev.Message.Usage.InputTokens > p.resp.Usage.InputTokens {
				p.resp.Usage.InputTokens = ev.Message.Usage.InputTokens
			}
			if v := ev.Message.Usage.CacheReadInputTokens; v > p.resp.Usage.CacheReadTokens {
				p.resp.Usage.CacheReadTokens = v
			}
			if v := ev.Message.Usage.CacheCreationInputTokens; v > p.resp.Usage.CacheWriteTokens {
				p.resp.Usage.CacheWriteTokens = v
			}
		}
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "text":
				p.pending.WriteString(b.Text)
			case "thinking":
				p.flushText()
				if t := strings.TrimSpace(b.Thinking); t != "" {
					p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "thinking", Text: t})
					p.emit(len(p.resp.Trace) - 1)
				}
			case "tool_use":
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input})
				idx := len(p.resp.Trace) - 1
				if b.ID != "" {
					p.toolIdx[b.ID] = idx
					p.toolStart[b.ID] = time.Now() // start the latency clock for this tool
				}
				// Parallel-batch grouping: the 2nd tool_use of the SAME API message
				// allocates a batch id and stamps every tool of that message (incl.
				// retroactively the 1st, whose result has not arrived yet — tool steps
				// emit only on tool_result, so the stamp lands before delivery).
				p.curMsgTools = append(p.curMsgTools, idx)
				if len(p.curMsgTools) == 2 {
					p.batchSeq++
				}
				if len(p.curMsgTools) >= 2 {
					for _, ti := range p.curMsgTools {
						p.resp.Trace[ti].Batch = p.batchSeq
					}
				}
				// Not emitted yet — wait for its tool_result to fill the output.
			case "server_tool_use":
				// Native server-side tool (WebSearch/WebFetch running inside the API,
				// not through our loop): nothing to execute here, but the step must be
				// visible — same handling as the anthropic provider.
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input, Output: "(executed server-side)"})
				p.emit(len(p.resp.Trace) - 1)
			case "web_search_tool_result":
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: webSearchName, Output: renderWebToolResult(b.Content, "result")})
				p.emit(len(p.resp.Trace) - 1)
			case "web_fetch_tool_result":
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: webFetchName, Output: renderWebToolResult(b.Content, "document")})
				p.emit(len(p.resp.Trace) - 1)
			default:
				// Never drop a block silently: an unrecognised type means the CLI
				// emitted something this parser does not model yet, and swallowing it
				// is exactly how native web search stayed invisible. Reported once per
				// type per turn so a repeated block cannot flood the trace.
				p.noteUnknownBlock(b.Type)
			}
		}
	case "user":
		if ev.Message == nil {
			return
		}
		for _, b := range ev.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			if i, ok := p.toolIdx[b.ToolUseID]; ok {
				p.resp.Trace[i].Output = CapToolOutput(toolResultText(b.Content))
				p.resp.Trace[i].IsError = b.IsError
				if start, ok := p.toolStart[b.ToolUseID]; ok {
					p.resp.Trace[i].DurMs = time.Since(start).Milliseconds()
					delete(p.toolStart, b.ToolUseID)
				}
				p.emit(i)
			}
		}
	case "result":
		p.sawResult = true
		p.sawModelTurn = true
		// Blocked tool calls mean the turn ran with less capability than it asked
		// for. Surface them on both the success and the error path — a silently
		// degraded answer is the worst outcome.
		if denials := describePermissionDenials(ev.PermissionDenials); denials != "" {
			p.note(denials)
		}
		if ev.IsError {
			p.hadError = true
			p.errText = ev.Result
			isRate := isRateLimitText(ev.APIErrorStatus) || isRateLimitText(ev.Result)
			isAuth := isAuthErrorText(ev.APIErrorStatus) || isAuthErrorText(ev.Result)
			// A usage/rate-limit rejection often surfaces here as the result error
			// (api_error_status == "rate_limit" or wording in the result text) rather
			// than a separate rate_limit_event — classify it either way.
			if isRate {
				p.rateLimited = true
				if p.rateLimitMsg == "" {
					p.rateLimitMsg = strings.TrimSpace(ev.APIErrorStatus + " " + ev.Result)
				}
			}
			// A login lapse commonly surfaces here as result "Not logged in · Please
			// run /login". Classify it so the caller fails fast with an actionable
			// message instead of a bare "exit status 1" that gets retried in vain.
			if isAuth {
				p.notLoggedIn = true
				if p.authMsg == "" {
					p.authMsg = strings.TrimSpace(ev.Result)
				}
			}
			// Neither auth nor rate limit: the result text is often a bare sentence
			// (or empty) and terminal_reason/stop_reason carry the only machine
			// readable cause. Fold them in so the caller sees e.g. "api_error"
			// instead of an unattributable message.
			if !isRate && !isAuth {
				if reason := strings.TrimSpace(ev.TerminalReason); reason != "" {
					p.errText = strings.TrimSpace(strings.TrimSpace(p.errText) + " (terminal_reason: " + reason + ")")
				} else if reason := strings.TrimSpace(ev.StopReason); reason != "" {
					p.errText = strings.TrimSpace(strings.TrimSpace(p.errText) + " (stop_reason: " + reason + ")")
				}
			}
			return
		}
		p.finalText = ev.Result
		if ev.Usage != nil {
			if ev.Usage.InputTokens > 0 {
				p.resp.Usage.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens > 0 {
				p.resp.Usage.OutputTokens = ev.Usage.OutputTokens
			}
			// The result envelope carries the authoritative aggregate; let it win.
			if ev.Usage.CacheReadInputTokens > 0 {
				p.resp.Usage.CacheReadTokens = ev.Usage.CacheReadInputTokens
			}
			if ev.Usage.CacheCreationInputTokens > 0 {
				p.resp.Usage.CacheWriteTokens = ev.Usage.CacheCreationInputTokens
			}
		}
		// A single turn can touch more than one model: ENABLE_TOOL_SEARCH runs an
		// auxiliary haiku call alongside the primary (opus) answer, so the result
		// envelope's modelUsage holds several keys. Iterating the map and taking
		// "any" key picked one at RANDOM (Go map order is unspecified), which made
		// resp.Model flap between the primary and the auxiliary model turn-to-turn —
		// a phantom "model downgraded to haiku" that never actually happened. Pick
		// the model that did the real work: the one with the most tokens.
		if m := primaryModelUsage(ev.ModelUsage); m != "" {
			p.resp.Model = m
		}
		// num_turns = how many internal model API round-trips the CLI made this turn.
		// The Usage above is the SUM across those round-trips (cache_read especially is
		// cumulative — verified: result cacheRead == Σ per-assistant cacheRead), so the
		// caller divides Usage by ProviderCalls to recover the per-call context size.
		if ev.NumTurns > 0 {
			p.resp.ProviderCalls = ev.NumTurns
		}
	}
}

func (p *cliStreamParser) startNativeCompaction(id string) {
	p.nativeCompactionMu.Lock()
	defer p.nativeCompactionMu.Unlock()
	statusStart := id == ""
	if !p.nativeCompactionDone && !p.nativeCompactionFailed && p.nativeCompactionID != "" && (statusStart || p.nativeCompactionStatusStart) {
		return
	}
	if p.nativeCompactionTimer != nil {
		p.nativeCompactionTimer.Stop()
		p.nativeCompactionTimer = nil
	}
	if !p.nativeCompactionDone && p.nativeCompactionID != "" && p.onEvent != nil {
		p.onEvent(TraceStep{Kind: "tombstone", Ref: p.nativeCompactionID})
	}
	if id == "" {
		p.nativeCompactionSeq++
		id = fmt.Sprintf("claude-compact-%d", p.nativeCompactionSeq)
	}
	p.nativeCompactionID = id
	p.nativeCompactionHasStart = true
	p.nativeCompactionDone = false
	p.nativeCompactionFailed = false
	p.nativeCompactionStatusStart = statusStart
	p.nativeCompactionSignal = ""
	p.nativeCompactionHookID = ""
	if p.onEvent != nil {
		p.onEvent(TraceStep{ID: id, Running: true, Kind: "compaction", Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact"})
		timeout := p.nativeCompactionTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		p.nativeCompactionTimer = time.AfterFunc(timeout, func() {
			p.nativeCompactionMu.Lock()
			defer p.nativeCompactionMu.Unlock()
			if !p.nativeCompactionDone && p.nativeCompactionID == id {
				p.onEvent(TraceStep{Kind: "tombstone", Ref: id})
				p.nativeCompactionID = ""
				p.nativeCompactionHasStart = false
				p.nativeCompactionTimer = nil
			}
		})
	}
}

func (p *cliStreamParser) failNativeCompaction() {
	p.nativeCompactionMu.Lock()
	defer p.nativeCompactionMu.Unlock()
	if p.nativeCompactionTimer != nil {
		p.nativeCompactionTimer.Stop()
		p.nativeCompactionTimer = nil
	}
	if !p.nativeCompactionDone && p.nativeCompactionID != "" && p.onEvent != nil {
		p.onEvent(TraceStep{Kind: "tombstone", Ref: p.nativeCompactionID})
	}
	p.nativeCompactionFailed = true
	p.nativeCompactionDone = false
}

func (p *cliStreamParser) completeNativeCompaction(ev cliEvent, signal string) {
	p.nativeCompactionMu.Lock()
	defer p.nativeCompactionMu.Unlock()
	if p.nativeCompactionFailed {
		return
	}
	if p.nativeCompactionDone {
		// With a PreCompact start, every completion signal belongs to that one
		// correlated cycle. Without hooks, consecutive boundary events are distinct
		// compactions; a boundary+PostCompact pair is duplicate evidence for one.
		if p.nativeCompactionHasStart || signal != p.nativeCompactionSignal || (signal == "post" && ev.HookID == p.nativeCompactionHookID) {
			return
		}
		p.nativeCompactionID = ""
		p.nativeCompactionDone = false
	}
	p.nativeCompactionDone = true
	p.nativeCompactionSignal = signal
	p.nativeCompactionHookID = ev.HookID
	if p.nativeCompactionTimer != nil {
		p.nativeCompactionTimer.Stop()
		p.nativeCompactionTimer = nil
	}
	id := p.nativeCompactionID
	if id == "" {
		id = ev.HookID
	}
	step := TraceStep{ID: id, Kind: "compaction", Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact"}
	if ev.CompactMetadata != nil {
		step.Trigger = ev.CompactMetadata.Trigger
	}
	p.resp.Trace = append(p.resp.Trace, step)
	p.emit(len(p.resp.Trace) - 1)
}

// finish resolves the final answer and emits any tool steps whose result never
// arrived (so the UI still sees them).
func (p *cliStreamParser) finish() (*Response, error) {
	p.nativeCompactionMu.Lock()
	if p.nativeCompactionTimer != nil {
		p.nativeCompactionTimer.Stop()
		p.nativeCompactionTimer = nil
		if !p.nativeCompactionDone && p.nativeCompactionID != "" && p.onEvent != nil {
			p.onEvent(TraceStep{Kind: "tombstone", Ref: p.nativeCompactionID})
		}
	}
	p.nativeCompactionMu.Unlock()
	if p.hadError {
		return nil, fmt.Errorf("claude CLI error: %s", p.errText)
	}
	if !p.sawResult {
		return nil, fmt.Errorf("claude CLI: no result in stream")
	}
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			p.emit(i)
		}
	}
	if p.finalText == "" {
		p.finalText = strings.TrimSpace(p.pending.String())
	}
	// Output guard: if the CLI echoed a harness repair reminder instead of an
	// answer (a malformed replayed turn can trigger this), don't surface it as
	// the assistant's reply. Strip the artifact; if nothing genuine remains,
	// fail the turn so the caller can retry rather than persist the reminder.
	if isRepairArtifact(p.finalText) {
		if cleaned := sanitizeTranscriptText(p.finalText); cleaned != "" {
			p.finalText = cleaned
		} else {
			return nil, fmt.Errorf("claude CLI returned only a repair reminder, not an answer")
		}
	}
	p.resp.Text = p.finalText
	return p.resp, nil
}

// ranTool reports whether any tool was invoked during the turn — used to decide
// if a crashed turn is safe to retry (a tool may have side effects, so a turn
// that reached one is NOT retried).
func (p *cliStreamParser) ranTool() bool {
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			return true
		}
	}
	return false
}

// describeRateLimit renders a compact, human-readable summary of a rate-limit
// window for the failure message (type, status, overage state, reset time).
func describeRateLimit(rl *cliRateLimit) string {
	parts := []string{}
	if rl.RateLimitType != "" {
		parts = append(parts, rl.RateLimitType+" window")
	}
	if rl.Status != "" {
		parts = append(parts, "status="+rl.Status)
	}
	if rl.OverageStatus != "" {
		parts = append(parts, "overage="+rl.OverageStatus)
	}
	if rl.OverageDisabledReason != "" {
		parts = append(parts, rl.OverageDisabledReason)
	}
	if rl.ResetsAt > 0 {
		parts = append(parts, "resets "+time.Unix(rl.ResetsAt, 0).Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return "rate limit reached"
	}
	return strings.Join(parts, ", ")
}

// isRateLimitText reports whether a result/api-error string signals a usage or
// rate-limit rejection (used to classify a result-error envelope).
func isRateLimitText(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "rate_limit") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "usage limit") ||
		strings.Contains(s, "usage_limit") ||
		strings.Contains(s, "quota")
}

// isAuthErrorText reports whether a result/api-error/error string signals an
// authentication failure — this claude-home has no valid login (never ran
// /login, or the OAuth token / API key expired or was revoked). Such failures
// are NOT retryable: a second attempt hits the same wall in milliseconds.
func isAuthErrorText(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "authentication_failed") ||
		strings.Contains(s, "not logged in") ||
		strings.Contains(s, "please run /login") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "invalid x-api-key") ||
		strings.Contains(s, "oauth token has expired") ||
		// CLI 2.1.238 wording, observed live: the result envelope reports
		// subtype "success" with is_error=true and the text below. Without
		// these two matches the turn is classified as a generic (retryable)
		// failure and retried in vain against the same dead session.
		strings.Contains(s, "failed to authenticate") ||
		strings.Contains(s, "oauth session expired") ||
		strings.Contains(s, "oauth authentication is currently not supported") ||
		strings.Contains(s, "invalid bearer token")
}

// salvage recovers whatever assistant content the parser accumulated when the
// stream was cut off before a final "result" event (the CLI crashed/exited at the
// end of the turn). It lets a long research turn that died on a trailing fault
// still return its work instead of failing the whole turn / flow node. Returns
// nil when there is nothing usable, or when the stream carried a genuine error
// result (hadError) — those must propagate, not be masked as success.
func (p *cliStreamParser) salvage() *Response {
	if p.hadError {
		return nil
	}
	text := strings.TrimSpace(p.finalText)
	if text == "" {
		text = strings.TrimSpace(p.pending.String())
	}
	if text == "" {
		// Fall back to the last non-empty assistant text step in the trace.
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

// toolResultText extracts displayable text from a tool_result content field,
// which the CLI encodes either as a JSON string or an array of content blocks.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []cliBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Text != "" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return string(raw)
}

// serializeTranscript turns a multi-turn history into a single prompt.
// For a single user turn it returns the text directly; otherwise it builds
// a labelled transcript so the CLI has prior context.
func serializeTranscript(msgs []Message) string {
	// Filter to user/assistant turns.
	turns := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleUser || m.Role == RoleAssistant {
			turns = append(turns, m)
		}
	}
	if len(turns) == 0 {
		return ""
	}
	if len(turns) == 1 {
		return turns[0].Text
	}

	var b strings.Builder
	b.WriteString("Continue this conversation. Reply only as the assistant to the final user message.\n\n")
	for _, m := range turns {
		label := "User"
		text := m.Text
		if m.Role == RoleAssistant {
			label = "Assistant"
			// Strip any leaked tool-call / harness markup from prior assistant
			// turns so the CLI never sees a malformed message and injects its own
			// repair <system-reminder> (which the model would then echo back).
			text = sanitizeTranscriptText(text)
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
