package insight

// SessionSignals are the cheap, LLM-free signals extracted from one session that
// a lens Prefilter evaluates before deciding whether the session is worth an LLM
// look. StepKinds counts TurnStep kinds (error/tool/thinking/...) from the
// session transcript; DebugEvents counts debug.jsonl event types
// (error/repair/guardrail/cache_break/...); Tools counts invocations by tool NAME
// (use_skill/Bash/...), so a lens can target a specific tool; TokenTotal is the
// session's token usage. Extraction lives in the scanner; this struct is the seam
// the Prefilter is tested against.
type SessionSignals struct {
	StepKinds   map[string]int
	DebugEvents map[string]int
	Tools       map[string]int
	TokenTotal  int
}

// count returns how many times a signal name appears across step kinds, debug
// event types and tool names (a lens can match on any of the three with one
// name). The three namespaces rarely collide; when they do the counts add.
func (s SessionSignals) count(name string) int {
	return s.StepKinds[name] + s.DebugEvents[name] + s.Tools[name]
}

// Match reports whether a session with the given signals passes this prefilter.
// An empty prefilter matches everything. Evaluation order (all must hold):
// excludes (NOT) -> requiresAll (AND) -> requiresAny (OR) -> minCount (thresholds)
// -> minTokens. Purely declarative, no expression parser (_Docs/60 §2).
func (p Prefilter) Match(sig SessionSignals) bool {
	for _, e := range p.Excludes {
		if sig.count(e) > 0 {
			return false
		}
	}
	for _, r := range p.RequiresAll {
		if sig.count(r) == 0 {
			return false
		}
	}
	if len(p.RequiresAny) > 0 {
		hit := false
		for _, r := range p.RequiresAny {
			if sig.count(r) > 0 {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	for name, min := range p.MinCount {
		if sig.count(name) < min {
			return false
		}
	}
	if p.MinTokens > 0 && sig.TokenTotal < p.MinTokens {
		return false
	}
	return true
}
