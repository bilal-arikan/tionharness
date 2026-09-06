package trajectory

import "testing"

func TestBoutsSplitsOnIdleGaps(t *testing.T) {
	// Two sittings 16 hours apart, each a handful of turns minutes apart.
	base := int64(1_700_000_000)
	times := []int64{
		base, base + 120, base + 300,
		base + 16*3600, base + 16*3600 + 60,
	}
	got := Bouts(times, DefaultBoutGapSec)
	want := []Span{
		{Start: base, End: base + 300},
		{Start: base + 16*3600, End: base + 16*3600 + 60},
	}
	if len(got) != len(want) {
		t.Fatalf("bouts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bout %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBoutsGapThresholdIsInclusive(t *testing.T) {
	base := int64(1_700_000_000)
	// Exactly the gap stays in one bout; a second past it cuts a new one.
	if got := Bouts([]int64{base, base + 600}, 600); len(got) != 1 {
		t.Fatalf("gap == threshold split into %d bouts, want 1", len(got))
	}
	if got := Bouts([]int64{base, base + 601}, 600); len(got) != 2 {
		t.Fatalf("gap > threshold gave %d bouts, want 2", len(got))
	}
}

func TestBoutsSortsInputWithoutMutatingIt(t *testing.T) {
	in := []int64{300, 100, 200}
	got := Bouts(in, 600)
	if len(got) != 1 || got[0].Start != 100 || got[0].End != 300 {
		t.Fatalf("bouts = %v, want one 100..300 span", got)
	}
	if in[0] != 300 || in[1] != 100 || in[2] != 200 {
		t.Fatalf("input was mutated: %v", in)
	}
}

func TestBoutsEmptyAndSingle(t *testing.T) {
	if got := Bouts(nil, 600); got != nil {
		t.Fatalf("no timestamps gave %v, want nil", got)
	}
	got := Bouts([]int64{42}, 600)
	if len(got) != 1 || got[0] != (Span{Start: 42, End: 42}) {
		t.Fatalf("single timestamp gave %v, want one zero-width span", got)
	}
}

func TestBoutsDefaultsGap(t *testing.T) {
	base := int64(1_700_000_000)
	// gapSec <= 0 must behave like DefaultBoutGapSec, not like "never split".
	if got := Bouts([]int64{base, base + DefaultBoutGapSec + 1}, 0); len(got) != 2 {
		t.Fatalf("zero gap gave %d bouts, want 2 (default applied)", len(got))
	}
}

func TestBoutsCapsCountByFoldingTheOldest(t *testing.T) {
	base := int64(1_700_000_000)
	// One timestamp per hour: every one of them is its own bout.
	var times []int64
	for i := int64(0); i < MaxBouts+10; i++ {
		times = append(times, base+i*3600)
	}
	got := Bouts(times, DefaultBoutGapSec)
	if len(got) != MaxBouts {
		t.Fatalf("bouts = %d, want the cap %d", len(got), MaxBouts)
	}
	if got[0].Start != base {
		t.Fatalf("folded head starts at %d, want the true first timestamp %d", got[0].Start, base)
	}
	last := got[len(got)-1]
	if last.End != times[len(times)-1] {
		t.Fatalf("last bout ends at %d, want %d", last.End, times[len(times)-1])
	}
}
