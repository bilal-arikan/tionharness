package skills

import "testing"

// TestCoordinatorWorkflowFrontmatter checks that a coordinator-workflow recipe's
// kind/pattern/worker_targets/stop_condition/max_turns fields parse into the Skill.
func TestCoordinatorWorkflowFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "wf-loop", "---\n"+
		"name: Loop\n"+
		"kind: coordinator-workflow\n"+
		"pattern: loop\n"+
		"worker_targets: [explore, reviewer]\n"+
		"stop_condition: no new findings for 2 rounds\n"+
		"max_turns: 25\n"+
		"access: shared\n"+
		"auto_summary: false\n"+
		"---\nbody")
	s := New("", dir)
	sk, ok := s.Get("wf-loop")
	if !ok {
		t.Fatal("recipe not found")
	}
	if !sk.IsCoordinatorWorkflow() {
		t.Errorf("IsCoordinatorWorkflow = false, want true (kind=%q)", sk.Kind)
	}
	if sk.Pattern != "loop" {
		t.Errorf("Pattern = %q, want loop", sk.Pattern)
	}
	if len(sk.WorkerTargets) != 2 || sk.WorkerTargets[0] != "explore" {
		t.Errorf("WorkerTargets = %v, want [explore reviewer]", sk.WorkerTargets)
	}
	if sk.StopCondition == "" {
		t.Error("StopCondition empty, want parsed value")
	}
	if sk.MaxTurns != 25 {
		t.Errorf("MaxTurns = %d, want 25", sk.MaxTurns)
	}
}

// TestKnownPattern guards the pattern allow-list (case-insensitive) used to reject
// a recipe with a bogus pattern at apply time.
func TestKnownPattern(t *testing.T) {
	for _, p := range PatternValues {
		if !KnownPattern(p) {
			t.Errorf("KnownPattern(%q) = false, want true", p)
		}
	}
	if !KnownPattern("FanOut") {
		t.Error("KnownPattern should be case-insensitive")
	}
	if KnownPattern("bogus") {
		t.Error("KnownPattern(bogus) = true, want false")
	}
}

// TestMalformedMaxTurnsIsUnset verifies a non-numeric max_turns degrades to 0
// (use-the-default) rather than failing the scan.
func TestMalformedMaxTurnsIsUnset(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "wf-bad", "---\nname: Bad\nkind: coordinator-workflow\npattern: fanout\nmax_turns: soon\n---\nbody")
	s := New("", dir)
	sk, ok := s.Get("wf-bad")
	if !ok {
		t.Fatal("recipe not found")
	}
	if sk.MaxTurns != 0 {
		t.Errorf("MaxTurns = %d, want 0 for malformed value", sk.MaxTurns)
	}
}

// TestDefaultRecipesValid asserts every shipped coordinator-workflow default parses
// with a known pattern — a guard so a new recipe with a typo'd pattern is caught.
func TestDefaultRecipesValid(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	s := New("", dir)
	found := 0
	for _, sk := range s.List() {
		if !sk.IsCoordinatorWorkflow() {
			continue
		}
		found++
		if sk.Pattern == "" || !KnownPattern(sk.Pattern) {
			t.Errorf("recipe %q has invalid pattern %q", sk.Slug, sk.Pattern)
		}
	}
	if found < 6 {
		t.Errorf("found %d coordinator-workflow defaults, want >= 6", found)
	}
}
