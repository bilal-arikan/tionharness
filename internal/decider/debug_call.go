package decider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
)

type debugCall struct {
	hub  *Hub
	base DebugEvent
}

type debugCallKey struct{}

// WithLocation correlates a decision with session debug independently of its
// flow/trajectory Ref. A background challenger inherits the same location.
func WithLocation(sessionID, turnID string) CallOption {
	return func(o *callOptions) { o.sessionID, o.turnID = sessionID, turnID }
}

func WithWorkspace(id string) CallOption {
	return func(o *callOptions) { o.workspaceID = debugToken(id) }
}

func debugHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (h *Hub) startDebug(authority string, cfg Config, o callOptions, role string) *debugCall {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return &debugCall{hub: h, base: DebugEvent{
		TraceID: hex.EncodeToString(id[:]), Authority: authority, Mode: cfg.Mode(authority),
		Ref: o.ref, SessionID: o.sessionID, WorkspaceID: o.workspaceID, TurnID: o.turnID, Role: role,
		Threshold: cfg.Threshold(authority), ConfigHash: debugHash(cfg),
	}}
}

func (d *debugCall) forModel(id, role string) *debugCall {
	if d == nil {
		return nil
	}
	copy := *d
	copy.base.Instance, copy.base.Role = id, role
	return &copy
}

func (d *debugCall) emit(e DebugEvent) {
	if d == nil {
		return
	}
	e.At = d.hub.now().UnixMilli()
	e.TraceID, e.Authority, e.Mode = d.base.TraceID, d.base.Authority, d.base.Mode
	e.Ref, e.SessionID, e.TurnID = d.base.Ref, d.base.SessionID, d.base.TurnID
	e.WorkspaceID = d.base.WorkspaceID
	e.ConfigHash, e.Threshold = d.base.ConfigHash, d.base.Threshold
	e.Role, e.Instance = d.base.Role, d.base.Instance
	if e.QuestionID == "" {
		e.QuestionID = d.base.QuestionID
	}
	d.hub.debug.append(e)
}

func (d *debugCall) finish(resp *Response, err error, start time.Time) {
	if d == nil {
		return
	}
	e := DebugEvent{Stage: "completed", Error: errorClass(err), LatencyMs: time.Since(start).Milliseconds()}
	if resp != nil {
		resp.DebugID = d.base.TraceID
		d = d.forModel(resp.Instance, d.base.Role)
	} else {
		var me *ModelError
		if errors.As(err, &me) {
			d = d.forModel(me.Instance, d.base.Role)
		}
	}
	d.emit(e)
}

// debugRequest fingerprints the request but keeps no input or question text.
func debugRequest(req Request) DebugEvent {
	b, _ := json.Marshal(req.State)
	types := map[string]int{}
	for _, q := range req.Questions {
		types[string(q.Type)]++
	}
	return DebugEvent{RequestHash: debugHash(req), StateBytes: len(b), QuestionTypes: types}
}

func debugStateBytes(state any) int {
	data, err := json.Marshal(state)
	if err != nil {
		return 0
	}
	var generic any
	if json.Unmarshal(data, &generic) != nil {
		return 0
	}
	redacted, _ := json.Marshal(redactValue(generic))
	return len(redacted)
}

func debugResponse(resp *Response, e *DebugEvent) {
	if resp == nil {
		return
	}
	e.Model, e.ServedModel, e.Backend = debugToken(resp.Model), debugToken(resp.ServedModel), debugToken(resp.Backend)
	e.InputTokens, e.OutputTokens, e.CostUSD = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CostUSD
	e.Answers = map[string]Answer{}
	for _, key := range sortedKeys(resp.Answers) {
		if len(e.Answers) >= maxQuestions {
			break
		}
		a := resp.Answers[key]
		a.Choice = debugToken(a.Choice)
		probs := map[string]float64{}
		for _, option := range sortedKeys(a.Probabilities) {
			if len(probs) >= 255 {
				break
			}
			probs[debugToken(option)] = a.Probabilities[option]
		}
		a.Probabilities = probs
		e.Answers[debugToken(key)] = a
	}
	// Backend warnings can contain provider-generated content. Preserve stable
	// classes only; the response itself still returns the full warning to callers.
	for _, warning := range resp.Warnings {
		code := "backend_warning"
		if strings.Contains(strings.ToLower(warning), "logprob") {
			code = "missing_logprobs"
		}
		if !slices.Contains(e.Warnings, code) {
			e.Warnings = append(e.Warnings, code)
		}
	}
}

func debugToken(s string) string {
	s = Redact(s)
	if len(s) > 128 {
		return "fingerprint:" + debugHash(s)
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.:/~", c)) {
			return "fingerprint:" + debugHash(s)
		}
	}
	return s
}

// debugError preserves errors.Is/As while correlating failures with the trace.
type debugError struct {
	id  string
	err error
}

func (e *debugError) Error() string { return e.err.Error() }
func (e *debugError) Unwrap() error { return e.err }

func debugID(resp *Response, err error) string {
	if resp != nil {
		return resp.DebugID
	}
	var de *debugError
	if errors.As(err, &de) {
		return de.id
	}
	return ""
}

func (h *Hub) debugOutcome(rec Record) {
	if rec.DebugID == "" {
		return
	}
	// The outcome arrives after Decide returns. Copy correlation metadata from
	// its start event so custom flow refs and thresholds remain consistent.
	if e, ok := h.debug.lastForTrace(rec.DebugID); ok {
		e.Stage, e.Role, e.Instance = "outcome", "primary", rec.Instance
		if rec.Role == RoleChallenger {
			e.Role = "challenger"
		}
		e.Answers, e.Warnings, e.QuestionTypes = nil, nil, nil
		e.RequestHash, e.Model, e.ServedModel, e.Backend = "", "", "", ""
		e.HTTPAttempt, e.LatencyMs, e.CostUSD, e.InputTokens, e.OutputTokens = 0, 0, 0, 0, 0
		e.StateBytes, e.PreparedBytes, e.RetryWaitMs, e.StateTrimmed = 0, 0, 0, false
		e.ModelHash, e.QuestionID, e.ContextTokens, e.TimeoutMs = "", "", 0, 0
		e.At, e.Outcome, e.Baseline, e.Strength, e.Applied, e.Error = h.now().UnixMilli(), debugToken(rec.Outcome), debugToken(rec.Baseline), rec.Strength, rec.Applied, rec.Error
		h.debug.append(e)
	}
}

// copyQuestionMetadata keeps normalization and map ownership separate from
// callers. Question text is used only by the backend, never the debug journal.
func copyQuestionMetadata(req Request) Request {
	req = req.Normalized()
	req.Questions = maps.Clone(req.Questions)
	return req
}
