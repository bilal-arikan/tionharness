package db

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"time"
)

// debugFile is the per-session debug journal that lives next to session.jsonl.
// Where session.jsonl holds the user-facing conversation, debug.jsonl is a
// parallel, append-only stream of STRUCTURED OBSERVABILITY events: turn timings,
// per-call token spend, per-tool latency/size, hook decisions, errors,
// compaction and recovery. It is written O(1) (O_APPEND) on a separate file, so
// it never touches the conversation hot path or the store lock, and it is read
// on demand (never loaded into memory at boot). Its purpose is debugging,
// token/latency optimisation and agent self-improvement: an agent can read its
// own debug stream to see where time and tokens went and adjust its behaviour.
const debugFile = "debug.jsonl"

// DefaultDebugJournalCap is the number of newest events kept per session when no
// explicit cap is configured. The file is pruned (oldest events dropped) once it
// grows past cap + cap/4, so debug data stays bounded on long-lived sessions.
const DefaultDebugJournalCap = 5000

// Debug event type tags (the "type" field of a DebugEvent).
const (
	DebugTurn       = "turn"        // one assistant turn finished (durMs, stop, err)
	DebugLLMCall    = "llm_call"    // one provider completion (model, in/out/cache tokens)
	DebugTool       = "tool"        // one tool execution (name, durMs, outBytes, err)
	DebugHook       = "hook"        // one PreToolUse/PostToolUse hook ran (name, detail)
	DebugError      = "error"       // a turn-level / permission / budget error
	DebugCompaction = "compaction"  // in-flight history was compacted (savedBytes)
	DebugRecovery   = "recovery"    // a turn recovery fired (output resume / compact)
	DebugCacheBreak = "cache_break" // the prompt-cache warm prefix was lost (attributed reason in Name/Detail)
	DebugRepair     = "repair"      // message-sequence repair healed the in-flight history (rule in Name)
	DebugGuardrail  = "guardrail"   // tool-loop guardrail decision (warn/block/halt in Name, tool in Detail)
	DebugLesson     = "lesson"      // a failure lesson was distilled and stored (tool in Name, lesson in Detail)
	DebugEpoch      = "epoch"       // prompt-epoch lifecycle: created/adopted/stale/refreshed (reason in Name/Detail)
)

// DebugEvent is one structured observability record. Fields are sparse
// (omitempty) so each event only carries what is relevant to its type; the db
// layer treats the file as opaque JSONL and never interprets the payload beyond
// the aggregation helpers below.
type DebugEvent struct {
	Time       int64  `json:"ts"`   // unix milliseconds (stamped on append when 0)
	Type       string `json:"type"` // one of the Debug* tags above
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"` // the assistant reply message id this event belongs to (per-message debug)
	AgentID    string `json:"agentId,omitempty"`
	Kind       string `json:"kind,omitempty"`  // call origin (chat/task/schedule/flow/…)
	Name       string `json:"name,omitempty"`   // tool name / hook event name
	HookID     string `json:"hookId,omitempty"` // for type=hook: the firing hook's id (attributes rtk/sqz/… activity)
	Model      string `json:"model,omitempty"`  // provider model for llm_call
	PromptKey  string `json:"promptKey,omitempty"`  // registry prompt key that drove an auxiliary llm_call
	PromptHash string `json:"promptHash,omitempty"` // 8-hex hash of the RESOLVED prompt text (edited ≠ default)
	DurMs      int64  `json:"durMs,omitempty"`
	In         int    `json:"in,omitempty"`
	Out        int    `json:"out,omitempty"`
	CacheRead  int    `json:"cacheRead,omitempty"`
	CacheWrite int    `json:"cacheWrite,omitempty"`
	// Calls is the number of underlying provider API round-trips this llm_call
	// represents. 0/1 for native single-call providers; for claude-cli it is the
	// CLI's own internal tool-loop turn count (result event num_turns), since the
	// CLI bills in/out/cache CUMULATIVELY across those steps — dividing by Calls
	// recovers the per-call (single-pass) context. Only meaningful for llm_call.
	Calls      int    `json:"calls,omitempty"`
	OutBytes   int    `json:"outBytes,omitempty"`   // tool result size
	SavedBytes int    `json:"savedBytes,omitempty"` // compaction bytes trimmed
	Stop       string `json:"stop,omitempty"`       // turn stop reason
	Err        bool   `json:"err,omitempty"`        // tool/turn failed
	Detail     string `json:"detail,omitempty"`     // free-form (error msg, reason, decision)
}

// debugPath returns the debug journal path for a session.
func (d *DB) debugPath(sessionID string) string {
	return d.dir(dirSessions, sessionID, debugFile)
}

// AppendDebugEvent appends one structured event to a session's debug.jsonl. It
// stamps the time when unset, writes a single compact line (O_APPEND), and prunes
// the file to the newest cap events once it grows past cap + cap/4. A blank
// sessionID is a no-op. cap <= 0 selects DefaultDebugJournalCap.
//
// Best-effort and self-contained: it takes only its own debugMu (never the store
// lock), so emitting a debug event can never stall an agent turn.
func (d *DB) AppendDebugEvent(sessionID string, ev DebugEvent, cap int) error {
	if sessionID == "" {
		return nil
	}
	if ev.Time == 0 {
		ev.Time = time.Now().UnixMilli()
	}
	ev.SessionID = sessionID
	if cap <= 0 {
		cap = DefaultDebugJournalCap
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ev); err != nil {
		return err
	}

	d.debugMu.Lock()
	defer d.debugMu.Unlock()

	path := d.debugPath(sessionID)
	// First write for this session in this process: learn the current line count
	// so the cap is enforced even across restarts.
	if _, known := d.debugCount[sessionID]; !known {
		d.debugCount[sessionID] = countFileLines(path)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	d.debugCount[sessionID]++

	// Prune lazily: rewrite keeping only the newest cap events once we drift past
	// cap + cap/4, so the rewrite cost is amortised over many appends.
	if d.debugCount[sessionID] > cap+cap/4 {
		if n, err := truncateDebugTail(path, cap); err == nil {
			d.debugCount[sessionID] = n
		}
	}
	return nil
}

// ReadDebugEvents returns a session's debug events oldest→newest, optionally
// filtered to a single type. limit <= 0 returns all retained events; a positive
// limit returns the newest `limit` (still oldest→newest within that window). A
// missing file yields an empty slice (not an error).
func (d *DB) ReadDebugEvents(ctx context.Context, sessionID, typ string, limit int) ([]DebugEvent, error) {
	if sessionID == "" {
		return nil, nil
	}
	evs, err := readDebugFile(d.debugPath(sessionID))
	if err != nil {
		return nil, err
	}
	if typ != "" {
		filtered := evs[:0]
		for _, e := range evs {
			if e.Type == typ {
				filtered = append(filtered, e)
			}
		}
		evs = filtered
	}
	if limit > 0 && len(evs) > limit {
		evs = evs[len(evs)-limit:]
	}
	return evs, nil
}

// DebugToolStat is the per-tool rollup inside a DebugSummary.
type DebugToolStat struct {
	Calls    int   `json:"calls"`
	Errors   int   `json:"errors"`
	DurMs    int64 `json:"durMs"`
	OutBytes int   `json:"outBytes"`
}

// DebugAnomaly is one heuristic finding the summary flags for attention — the
// raw material for the UI warnings, the read_session_debug tool, and the
// reflector's self-improvement notes. Severity is "warn" | "info"; Code is a
// stable machine tag; Message is human-readable (Turkish, UI-facing).
type DebugAnomaly struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// debugSeriesCap bounds the per-turn time series carried in a summary so a
// long-lived session's sparkline stays small (newest points kept).
const debugSeriesCap = 40

// DebugSummary is an aggregate view of a session's debug journal — the data an
// agent (or the UI Debug tab) reads to spot where tokens, time and errors went.
type DebugSummary struct {
	SessionID    string                   `json:"sessionId"`
	Events       int                      `json:"events"`
	Turns        int                      `json:"turns"`
	LLMCalls     int                      `json:"llmCalls"`
	InputTokens  int                      `json:"inputTokens"`
	OutputTokens int                      `json:"outputTokens"`
	CacheRead    int                      `json:"cacheReadTokens"`
	CacheWrite   int                      `json:"cacheWriteTokens"`
	ToolCalls    int                      `json:"toolCalls"`
	Errors       int                      `json:"errors"`
	Compactions  int                      `json:"compactions"`
	Recoveries   int                      `json:"recoveries"`
	CacheBreaks  int                      `json:"cacheBreaks"`
	SavedBytes   int                      `json:"savedBytes"`
	TurnDurMs    int64                    `json:"turnDurMs"`
	ByTool       map[string]DebugToolStat `json:"byTool,omitempty"`
	ByModel      map[string]int           `json:"byModel,omitempty"` // model → total tokens
	TopTools     []string                 `json:"topTools,omitempty"`
	LastError    string                   `json:"lastError,omitempty"`
	FirstTs      int64                    `json:"firstTs,omitempty"`
	LastTs       int64                    `json:"lastTs,omitempty"`
	// LastCacheBreak is the human-readable reason of the most recent prompt-cache
	// break (empty when none) — surfaced in the Debug card + the cache_break anomaly.
	LastCacheBreak string `json:"lastCacheBreak,omitempty"`
	// Time series for sparklines (newest debugSeriesCap points, oldest→newest):
	// per-turn duration (ms) and per-llm-call total tokens (in+out).
	TurnDurSeries []int64 `json:"turnDurSeries,omitempty"`
	TokenSeries   []int   `json:"tokenSeries,omitempty"`
	// Anomalies are heuristic findings (slow/failing tools, error bursts, frequent
	// compaction) surfaced to the UI, the agent tool, and the reflector.
	Anomalies []DebugAnomaly `json:"anomalies,omitempty"`
}

// GetDebugSummary computes an aggregate of a session's debug journal. A missing
// file yields a zero-valued summary (not an error).
func (d *DB) GetDebugSummary(ctx context.Context, sessionID string) (DebugSummary, error) {
	sum := DebugSummary{SessionID: sessionID, ByTool: map[string]DebugToolStat{}, ByModel: map[string]int{}}
	evs, err := readDebugFile(d.debugPath(sessionID))
	if err != nil {
		return sum, err
	}
	for _, e := range evs {
		sum.Events++
		if sum.FirstTs == 0 || e.Time < sum.FirstTs {
			sum.FirstTs = e.Time
		}
		if e.Time > sum.LastTs {
			sum.LastTs = e.Time
		}
		switch e.Type {
		case DebugTurn:
			sum.Turns++
			sum.TurnDurMs += e.DurMs
			sum.TurnDurSeries = appendCapped(sum.TurnDurSeries, e.DurMs)
		case DebugLLMCall:
			sum.LLMCalls++
			sum.InputTokens += e.In
			sum.OutputTokens += e.Out
			sum.CacheRead += e.CacheRead
			sum.CacheWrite += e.CacheWrite
			sum.TokenSeries = appendCappedInt(sum.TokenSeries, e.In+e.Out)
			if e.Model != "" {
				sum.ByModel[e.Model] += e.In + e.Out
			}
		case DebugTool:
			sum.ToolCalls++
			st := sum.ByTool[e.Name]
			st.Calls++
			st.DurMs += e.DurMs
			st.OutBytes += e.OutBytes
			if e.Err {
				st.Errors++
			}
			sum.ByTool[e.Name] = st
		case DebugError:
			sum.Errors++
			if e.Detail != "" {
				sum.LastError = e.Detail
			}
		case DebugCompaction:
			sum.Compactions++
			sum.SavedBytes += e.SavedBytes
		case DebugRecovery:
			sum.Recoveries++
		case DebugCacheBreak:
			sum.CacheBreaks++
			if e.Detail != "" {
				sum.LastCacheBreak = e.Detail
			}
		}
	}
	// TopTools: tool names ordered by total duration (the optimisation hot list).
	if len(sum.ByTool) > 0 {
		names := make([]string, 0, len(sum.ByTool))
		for n := range sum.ByTool {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			return sum.ByTool[names[i]].DurMs > sum.ByTool[names[j]].DurMs
		})
		if len(names) > 5 {
			names = names[:5]
		}
		sum.TopTools = names
	}
	sum.Anomalies = computeDebugAnomalies(sum)
	if len(sum.ByTool) == 0 {
		sum.ByTool = nil
	}
	if len(sum.ByModel) == 0 {
		sum.ByModel = nil
	}
	return sum, nil
}

// TurnToolCall is one tool execution within a single turn — the per-call rows the
// message-level debug panel lists (name + latency + output size + error).
type TurnToolCall struct {
	Name     string `json:"name"`
	DurMs    int64  `json:"durMs"`
	OutBytes int    `json:"outBytes"`
	Err      bool   `json:"err,omitempty"`
}

// TurnDebug is the per-MESSAGE (per-turn) debug rollup behind the chat message
// debug panel: the token spend, latency, model and the exact tool calls that ran
// to produce ONE assistant reply. Events are correlated by DebugEvent.TurnID (the
// reply message id). ByModel feeds the shared cost helper in the API layer and is
// not serialized directly.
type TurnDebug struct {
	SessionID    string              `json:"sessionId"`
	TurnID       string              `json:"turnId"`
	Found        bool                `json:"found"`
	Model        string              `json:"model,omitempty"`
	DurMs        int64               `json:"durMs"`
	Stop         string              `json:"stop,omitempty"`
	LLMCalls     int                 `json:"llmCalls"`
	InputTokens  int                 `json:"inputTokens"`
	OutputTokens int                 `json:"outputTokens"`
	CacheRead    int                 `json:"cacheReadTokens"`
	CacheWrite   int                 `json:"cacheWriteTokens"`
	ToolCalls    int                 `json:"toolCalls"`
	Tools        []TurnToolCall      `json:"tools,omitempty"`
	Errors       int                 `json:"errors"`
	Recoveries   int                 `json:"recoveries"`
	Compactions  int                 `json:"compactions"`
	LastError    string              `json:"lastError,omitempty"`
	FirstTs      int64               `json:"firstTs,omitempty"`
	LastTs       int64               `json:"lastTs,omitempty"`
	ByModel      map[string]KindStat `json:"-"` // cost calc input (API layer); not serialized
}

// GetTurnDebug aggregates a session's debug journal down to the events tagged with
// one turn id (one assistant reply) — the data behind the per-message debug panel.
// A missing file or unknown turn id yields Found=false (not an error).
func (d *DB) GetTurnDebug(ctx context.Context, sessionID, turnID string) (TurnDebug, error) {
	td := TurnDebug{SessionID: sessionID, TurnID: turnID, ByModel: map[string]KindStat{}}
	if sessionID == "" || turnID == "" {
		return td, nil
	}
	evs, err := readDebugFile(d.debugPath(sessionID))
	if err != nil {
		return td, err
	}
	for _, e := range evs {
		if e.TurnID != turnID {
			continue
		}
		td.Found = true
		if td.FirstTs == 0 || e.Time < td.FirstTs {
			td.FirstTs = e.Time
		}
		if e.Time > td.LastTs {
			td.LastTs = e.Time
		}
		switch e.Type {
		case DebugTurn:
			td.DurMs += e.DurMs
			if e.Stop != "" {
				td.Stop = e.Stop
			}
		case DebugLLMCall:
			td.LLMCalls++
			td.InputTokens += e.In
			td.OutputTokens += e.Out
			td.CacheRead += e.CacheRead
			td.CacheWrite += e.CacheWrite
			if e.Model != "" {
				td.Model = e.Model
				st := td.ByModel[e.Model]
				st.Calls++
				st.InputTokens += e.In
				st.OutputTokens += e.Out
				st.CacheReadTokens += e.CacheRead
				st.CacheWriteTokens += e.CacheWrite
				td.ByModel[e.Model] = st
			}
		case DebugTool:
			td.ToolCalls++
			td.Tools = append(td.Tools, TurnToolCall{Name: e.Name, DurMs: e.DurMs, OutBytes: e.OutBytes, Err: e.Err})
		case DebugError:
			td.Errors++
			if e.Detail != "" {
				td.LastError = e.Detail
			}
		case DebugRecovery:
			td.Recoveries++
		case DebugCompaction:
			td.Compactions++
		}
	}
	return td, nil
}

// computeDebugAnomalies derives heuristic findings from a populated summary —
// pure (no I/O) so it is trivially testable and reusable by the reflector. Each
// threshold is intentionally conservative to avoid noise on healthy sessions.
func computeDebugAnomalies(sum DebugSummary) []DebugAnomaly {
	var out []DebugAnomaly

	// 1) A single tool dominating total tool time (the optimisation hot spot).
	var totalToolDur int64
	for _, st := range sum.ByTool {
		totalToolDur += st.DurMs
	}
	if totalToolDur > 2000 {
		for name, st := range sum.ByTool {
			if share := float64(st.DurMs) / float64(totalToolDur); share >= 0.6 {
				out = append(out, DebugAnomaly{
					Severity: "warn",
					Code:     "tool_time_dominant",
					Message:  name + " araç süresinin " + pct(share) + "'ini tüketiyor — çağrıları topla veya girdiyi daralt.",
				})
			}
		}
	}

	// 2) A tool that fails often (calls>=3, error ratio > 30%).
	for name, st := range sum.ByTool {
		if st.Calls >= 3 && float64(st.Errors)/float64(st.Calls) > 0.3 {
			out = append(out, DebugAnomaly{
				Severity: "warn",
				Code:     "tool_failing",
				Message:  name + " sık başarısız oluyor (" + itoa(st.Errors) + "/" + itoa(st.Calls) + ") — kullanımını gözden geçir.",
			})
		}
	}

	// 3) A tool returning very large outputs on average (>64KB, calls>=2) —
	// bloats context and forces compaction.
	for name, st := range sum.ByTool {
		if st.Calls >= 2 && st.OutBytes/st.Calls > 64*1024 {
			out = append(out, DebugAnomaly{
				Severity: "info",
				Code:     "tool_large_output",
				Message:  name + " büyük çıktı döndürüyor (ort. " + humanBytes(st.OutBytes/st.Calls) + ") — filtre/limit ekle.",
			})
		}
	}

	// 4) Frequent context compaction → running hot on context.
	if sum.Compactions >= 3 {
		out = append(out, DebugAnomaly{
			Severity: "warn",
			Code:     "frequent_compaction",
			Message:  "Bağlam " + itoa(sum.Compactions) + " kez sıkıştırıldı — önemli bilgileri core memory'ye yaz veya handoff yap.",
		})
	}

	// 4b) Prompt-cache breaks: the warm prefix was lost and re-written cold. A single
	// break can be normal (first turn, a genuine model/prompt change); repeated breaks
	// mean the cache rarely holds — the token bill is paying full price turn after turn.
	if sum.CacheBreaks >= 2 {
		out = append(out, DebugAnomaly{
			Severity: "warn",
			Code:     "cache_breaks",
			Message:  "Prompt-cache " + itoa(sum.CacheBreaks) + " kez kırıldı (sıcak önek yeniden yazıldı) — son sebep: " + clip(sum.LastCacheBreak, 80),
		})
	} else if sum.CacheBreaks == 1 && sum.LastCacheBreak != "" {
		out = append(out, DebugAnomaly{
			Severity: "info",
			Code:     "cache_break",
			Message:  "Prompt-cache bir kez kırıldı — " + clip(sum.LastCacheBreak, 80),
		})
	}

	// 5) Error burst across the session.
	if sum.Errors >= 3 {
		out = append(out, DebugAnomaly{
			Severity: "warn",
			Code:     "error_burst",
			Message:  itoa(sum.Errors) + " hata kaydı — son hata: " + clip(sum.LastError, 80),
		})
	}

	// 6) Slow turns on average (>45s, turns>=2) — latency worth investigating.
	if sum.Turns >= 2 {
		if avg := sum.TurnDurMs / int64(sum.Turns); avg > 45000 {
			out = append(out, DebugAnomaly{
				Severity: "info",
				Code:     "slow_turns",
				Message:  "Ortalama tur süresi " + humanDur(avg) + " — gecikme kaynağını araç sürelerinden incele.",
			})
		}
	}

	return out
}

// ---- small formatting + series helpers (pure) ----

func appendCapped(s []int64, v int64) []int64 {
	s = append(s, v)
	if len(s) > debugSeriesCap {
		s = s[len(s)-debugSeriesCap:]
	}
	return s
}

func appendCappedInt(s []int, v int) []int {
	s = append(s, v)
	if len(s) > debugSeriesCap {
		s = s[len(s)-debugSeriesCap:]
	}
	return s
}

func itoa(n int) string { return strconv.Itoa(n) }

func pct(f float64) string { return strconv.Itoa(int(f*100+0.5)) + "%" }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func humanBytes(b int) string {
	switch {
	case b >= 1_000_000:
		return strconv.FormatFloat(float64(b)/1_000_000, 'f', 1, 64) + "MB"
	case b >= 1000:
		return strconv.FormatFloat(float64(b)/1000, 'f', 1, 64) + "KB"
	default:
		return strconv.Itoa(b) + "B"
	}
}

func humanDur(ms int64) string {
	if ms >= 1000 {
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "s"
	}
	return strconv.FormatInt(ms, 10) + "ms"
}

// ---- file helpers (no store lock; debug.jsonl is independent) ----

// countFileLines returns the number of newline-terminated lines in a file, or 0
// when the file is absent/unreadable.
func countFileLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) > 0 {
			n++
		}
	}
	return n
}

// readDebugFile parses every line of a debug journal into DebugEvents. A missing
// file yields an empty slice. A single unparsable line is skipped (best-effort),
// mirroring the session.jsonl tolerance for a torn trailing write.
func readDebugFile(path string) ([]DebugEvent, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []DebugEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev DebugEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // tolerate a torn/corrupt line
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}

// truncateDebugTail rewrites a debug journal keeping only its newest `keep`
// lines, returning the new line count. Atomic (tmp→rename) so a crash never
// leaves a half-written file.
func truncateDebugTail(path string, keep int) (int, error) {
	evs, err := readDebugFile(path)
	if err != nil {
		return 0, err
	}
	if len(evs) <= keep {
		return len(evs), nil
	}
	evs = evs[len(evs)-keep:]
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, e := range evs {
		if err := enc.Encode(e); err != nil {
			return 0, err
		}
	}
	if err := atomicWriteBytes(path, buf.Bytes()); err != nil {
		return 0, err
	}
	return len(evs), nil
}
