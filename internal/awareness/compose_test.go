package awareness

import (
	"strings"
	"testing"
)

func sec(key string, size, prio int, pointer string) Section {
	return Section{Key: key, Text: "## " + key + "\n" + strings.Repeat("x", size), Pointer: pointer, Priority: prio}
}

func TestComposeShipsEverythingUnderBudgetAndMeters(t *testing.T) {
	c := Compose(MomentBrief, []Section{sec("a", 100, 0, ""), sec("b", 100, 3, "b pointer"), {Key: "empty"}}, 10_000)
	if len(c.Sections) != 2 || c.Degraded != nil || c.Dropped != nil || c.Cut {
		t.Fatalf("nothing should degrade: %+v", c)
	}
	if !strings.HasSuffix(c.Text, c.Meter) || !strings.Contains(c.Meter, "brief") || !strings.Contains(c.Meter, "2 sections") {
		t.Fatalf("meter: %q", c.Meter)
	}
	if !strings.Contains(c.Text, "## a") || !strings.Contains(c.Text, "## b") {
		t.Fatalf("text lacks sections: %q", c.Text)
	}
	if c.Bytes != len(c.Text) || c.Hash == "" {
		t.Fatalf("accounting: %+v", c)
	}
}

func TestComposeDegradesHighestPriorityToPointerFirstThenDrops(t *testing.T) {
	sections := []Section{
		sec("identity", 200, PriorityPinned, ""),
		sec("sessions", 1000, 5, "sessions pointer"),
		sec("notes", 1000, 3, "notes pointer"),
		sec("recap", 800, 7, ""), // no pointer → dropped outright
	}
	// Budget fits identity + both pointers + meter only.
	c := Compose(MomentTurn, sections, 200+40+40+meterReserve+60)
	if len(c.Dropped) != 1 || c.Dropped[0] != "recap" {
		t.Fatalf("recap (highest priority, no pointer) must drop first: %+v", c)
	}
	if len(c.Degraded) != 2 {
		t.Fatalf("both pointer-bearing sections degrade: %+v", c)
	}
	if strings.Contains(c.Text, strings.Repeat("x", 1000)) {
		t.Fatal("degraded text must not ship")
	}
	if !strings.Contains(c.Text, "sessions pointer") || !strings.Contains(c.Text, "notes pointer") {
		t.Fatalf("pointers must ship: %q", c.Text)
	}
	if !strings.Contains(c.Meter, "pointer: notes,sessions") || !strings.Contains(c.Meter, "dropped: recap") {
		t.Fatalf("meter must name losses: %q", c.Meter)
	}
	var identity SectionStat
	for _, s := range c.Sections {
		if s.Key == "identity" {
			identity = s
		}
	}
	if identity.State != StateFull {
		t.Fatalf("pinned section must stay full: %+v", identity)
	}
	// Order of degradation: the higher priority number goes first.
	c2 := Compose(MomentTurn, sections[:3], 200+1000+40+meterReserve+10)
	if len(c2.Degraded) != 1 || c2.Degraded[0] != "sessions" {
		t.Fatalf("priority 5 must degrade before priority 3: %+v", c2.Degraded)
	}
}

func TestComposeCutsPinnedContentOnlyAsLastResortAndSaysSo(t *testing.T) {
	long := Section{Key: "todo", Priority: PriorityPinned, Text: strings.Repeat("- item line\n", 100)}
	c := Compose(MomentTurn, []Section{sec("id", 50, 0, ""), long}, 600)
	if !c.Cut {
		t.Fatalf("expected a cut: %+v", c)
	}
	if !strings.Contains(c.Text, "cut to fit") || !strings.Contains(c.Meter, "cut") {
		t.Fatalf("a cut must be visible: %q", c.Text)
	}
	if c.Bytes > 600+40 {
		t.Fatalf("still far over budget after cut: %d", c.Bytes)
	}
	// The first pinned section is never cut below half.
	only := Compose(MomentBrief, []Section{{Key: "only", Priority: PriorityPinned, Text: strings.Repeat("y\n", 500)}}, 300)
	if !strings.Contains(only.Text, "y\ny") {
		t.Fatalf("first pinned section must keep content: %q", only.Text)
	}
}

func TestComposeHashIgnoresMeterAndTracksContent(t *testing.T) {
	a := Compose(MomentTurn, []Section{sec("a", 10, 1, "")}, 1000)
	b := Compose(MomentTurn, []Section{sec("a", 10, 1, "")}, 2000) // different budget → different meter
	if a.Hash != b.Hash {
		t.Fatal("hash must not depend on the meter")
	}
	c := Compose(MomentTurn, []Section{sec("a", 11, 1, "")}, 1000)
	if a.Hash == c.Hash {
		t.Fatal("hash must follow content")
	}
	empty := Compose(MomentBrief, nil, 100)
	if !empty.Empty() || empty.Text != empty.Meter {
		t.Fatalf("empty composition is meter-only: %+v", empty)
	}
}

func TestUnlimitedBudgetNeverDegrades(t *testing.T) {
	c := Compose(MomentTurn, []Section{sec("a", 100000, 9, "p")}, 0)
	if len(c.Degraded) != 0 || c.Cut {
		t.Fatalf("budget 0 is unlimited: %+v", c)
	}
	if strings.Contains(c.Meter, " of ") {
		t.Fatalf("unlimited meter shows no ceiling: %q", c.Meter)
	}
}
