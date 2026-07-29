package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A persisted assistant turn carries its whole activity trace (agent.TurnStep
// records) as a JSON string on the message. Tool outputs dominate that payload —
// a single Read/Grep/Bash step can be tens of kilobytes — so serving a long
// transcript ships megabytes the chat never paints: the collapsed activity card
// shows a one-line summary, and even expanded it renders at most stepFieldCap
// characters.
//
// The trimming below runs ONLY on the HTTP read path (transcript listing + hub
// replay). It never touches what is persisted to session.jsonl nor what is fed
// back to the model — those keep the full text. A trimmed field is marked with
// its `*Truncated` flag so the UI can offer "fetch the full trace"
// (GET /api/sessions/{id}/messages/{msgId}/steps).
//
// 2 KB ≈ 25 lines — enough that the expanded card almost always shows the whole
// payload. Measured against a real 11-session workspace it cuts the transcript's
// trace payload by ~45%; 1 KB would reach ~60% but starts clipping ordinary tool
// output, and 4 KB only reaches ~27%.
const stepFieldCap = 2048

// stepsTrimFloor is a cheap bail-out: a trace smaller than this cannot contain
// an over-cap field, so it is served verbatim without a parse/re-marshal round.
const stepsTrimFloor = stepFieldCap

// trimStepsJSON returns the steps JSON with every oversized string field cut to
// stepFieldCap. On ANY problem (unparseable trace, marshal failure) it returns
// the input unchanged — a transcript is never dropped just because it could not
// be shrunk.
func trimStepsJSON(raw string) string {
	if len(raw) <= stepsTrimFloor {
		return raw
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	// UseNumber keeps added/removed/batch as their exact literals instead of
	// round-tripping small ints through float64.
	dec.UseNumber()
	var steps []any
	if err := dec.Decode(&steps); err != nil {
		return raw
	}
	changed := false
	for _, s := range steps {
		if trimStep(s) {
			changed = true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(steps)
	if err != nil {
		return raw
	}
	return string(out)
}

// trimStep cuts one step's heavy fields in place, recursing into a subagent
// step's nested trace. Returns whether anything was cut.
func trimStep(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	// output / text / patch are plain strings on the step itself.
	for _, f := range []struct{ key, flag, lenKey string }{
		{"output", "outputTruncated", "outputLen"},
		{"text", "textTruncated", "textLen"},
		{"patch", "patchTruncated", "patchLen"},
	} {
		s, ok := m[f.key].(string)
		if !ok || len(s) <= stepFieldCap {
			continue
		}
		m[f.key] = truncUTF8(s, stepFieldCap)
		m[f.flag] = true
		m[f.lenKey] = json.Number(strconv.Itoa(len(s)))
		changed = true
	}
	// input is arbitrary JSON. Its KEYS are load-bearing (the UI resolves the
	// tool label, the command program tag and the synthesized Edit/Write diff
	// from them), so the structure is preserved and only the long leaf strings
	// inside it are cut.
	if in, ok := m["input"]; ok {
		if trimmed, did := trimLeafStrings(in); did {
			m["input"] = trimmed
			m["inputTruncated"] = true
			changed = true
		}
	}
	if subs, ok := m["subSteps"].([]any); ok {
		for _, sub := range subs {
			if trimStep(sub) {
				changed = true
			}
		}
	}
	return changed
}

// trimLeafStrings walks an arbitrary decoded JSON value and cuts every string
// leaf longer than stepFieldCap, returning the (possibly rebuilt) value and
// whether anything was cut.
func trimLeafStrings(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		if len(t) <= stepFieldCap {
			return t, false
		}
		return truncUTF8(t, stepFieldCap), true
	case map[string]any:
		changed := false
		for k, val := range t {
			nv, did := trimLeafStrings(val)
			if did {
				t[k] = nv
				changed = true
			}
		}
		return t, changed
	case []any:
		changed := false
		for i, val := range t {
			nv, did := trimLeafStrings(val)
			if did {
				t[i] = nv
				changed = true
			}
		}
		return t, changed
	default:
		return v, false
	}
}

// truncUTF8 cuts s to at most n bytes without splitting a rune — a half rune
// would serialize as U+FFFD and corrupt the preview.
func truncUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
