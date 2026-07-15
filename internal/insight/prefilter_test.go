package insight

import "testing"

func sig(steps, debug map[string]int, tokens int) SessionSignals {
	if steps == nil {
		steps = map[string]int{}
	}
	if debug == nil {
		debug = map[string]int{}
	}
	return SessionSignals{StepKinds: steps, DebugEvents: debug, TokenTotal: tokens}
}

func TestPrefilterEmptyMatchesAll(t *testing.T) {
	if !(Prefilter{}).Match(sig(nil, nil, 0)) {
		t.Fatal("empty prefilter should match everything")
	}
}

func TestPrefilterRequiresAny(t *testing.T) {
	p := Prefilter{RequiresAny: []string{"error"}}
	if p.Match(sig(map[string]int{"tool": 3}, nil, 0)) {
		t.Fatal("requiresAny should reject a session without the signal")
	}
	if !p.Match(sig(map[string]int{"error": 1}, nil, 0)) {
		t.Fatal("requiresAny should accept when the signal is present in steps")
	}
	if !p.Match(sig(nil, map[string]int{"error": 2}, 0)) {
		t.Fatal("requiresAny should also match on debug events")
	}
}

func TestPrefilterRequiresAllAndExcludes(t *testing.T) {
	p := Prefilter{RequiresAll: []string{"worker", "idle"}, Excludes: []string{"handoff"}}
	if p.Match(sig(map[string]int{"worker": 1}, nil, 0)) {
		t.Fatal("requiresAll should reject when not all signals present")
	}
	if !p.Match(sig(map[string]int{"worker": 1, "idle": 1}, nil, 0)) {
		t.Fatal("requiresAll should accept when all present")
	}
	if p.Match(sig(map[string]int{"worker": 1, "idle": 1, "handoff": 1}, nil, 0)) {
		t.Fatal("excludes should reject when an excluded signal is present")
	}
}

func TestPrefilterMatchesToolName(t *testing.T) {
	p := Prefilter{RequiresAny: []string{"use_skill"}}
	s := SessionSignals{
		StepKinds:   map[string]int{"tool": 4},
		DebugEvents: map[string]int{},
		Tools:       map[string]int{"use_skill": 2, "Bash": 4},
	}
	if !p.Match(s) {
		t.Fatal("requiresAny should match on tool name (use_skill)")
	}
	if (Prefilter{RequiresAny: []string{"read_lessons"}}).Match(s) {
		t.Fatal("should not match a tool that was not used")
	}
}

func TestPrefilterMinCountAndTokens(t *testing.T) {
	p := Prefilter{MinCount: map[string]int{"cache_break": 3}, MinTokens: 50000}
	if p.Match(sig(nil, map[string]int{"cache_break": 2}, 60000)) {
		t.Fatal("minCount should reject below threshold")
	}
	if p.Match(sig(nil, map[string]int{"cache_break": 3}, 40000)) {
		t.Fatal("minTokens should reject below threshold")
	}
	if !p.Match(sig(nil, map[string]int{"cache_break": 3}, 50000)) {
		t.Fatal("should accept at both thresholds")
	}
}
