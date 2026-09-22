package announce

import (
	"strings"
	"testing"
)

// sampleRelease builds a release with every section kind, so the tests can
// assert what is carried into an announcement and what is left out.
func sampleRelease() Release {
	return Release{
		Version: "1.2.3", Tag: "v1.2.3", Date: "2026-09-22", Count: 6,
		Sections: []Section{
			{Title: "BREAKING CHANGES", Entries: []Entry{{Hash: "a1", Scope: "api", Subject: "drop the old flag", Breaking: true}}},
			{Title: "Features", Entries: []Entry{{Hash: "b1", Scope: "monitor", Subject: "watch files"}}},
			{Title: "Bug Fixes", Entries: []Entry{{Hash: "c1", Subject: "stop the leak"}}},
			{Title: "Chores", Entries: []Entry{{Hash: "d1", Subject: "bump a dep"}}},
			{Title: "Tests", Entries: []Entry{{Hash: "e1", Subject: "add a case"}}},
		},
	}
}

// TestRenderCarriesHighlightsOnly: an announcement is a headline, not the
// changelog. Chores and tests belong in the full notes.
func TestRenderCarriesHighlightsOnly(t *testing.T) {
	out := Render(sampleRelease(), "")
	for _, want := range []string{"v1.2.3", "BREAKING CHANGES", "drop the old flag", "Features", "watch files", "Bug Fixes", "stop the leak"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"bump a dep", "add a case", "Chores", "Tests"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("housekeeping section %q leaked into the announcement:\n%s", unwanted, out)
		}
	}
}

// TestRenderOrdersBreakingFirst: the one thing a reader must not miss leads.
func TestRenderOrdersBreakingFirst(t *testing.T) {
	out := Render(sampleRelease(), "")
	iBreak := strings.Index(out, "BREAKING CHANGES")
	iFeat := strings.Index(out, "Features")
	if iBreak == -1 || iFeat == -1 || iBreak > iFeat {
		t.Fatalf("breaking changes are not first:\n%s", out)
	}
}

// TestRenderAppendsNotesURL: the link is how a reader reaches what the headline
// left out.
func TestRenderAppendsNotesURL(t *testing.T) {
	const url = "https://tionharness.com/releases/v1.2.3"
	out := Render(sampleRelease(), url)
	if !strings.Contains(out, url) {
		t.Fatalf("notes url is missing:\n%s", out)
	}
	if out := Render(sampleRelease(), ""); strings.Contains(out, "Full notes") {
		t.Fatalf("an empty url produced a dangling link line:\n%s", out)
	}
}

// TestRenderCapsEntriesPerSection: a release with 40 features must not push the
// rest of the message past the length cap.
func TestRenderCapsEntriesPerSection(t *testing.T) {
	r := sampleRelease()
	var many []Entry
	for i := 0; i < 12; i++ {
		many = append(many, Entry{Hash: "x", Subject: "feature number " + string(rune('a'+i))})
	}
	r.Sections = []Section{{Title: "Features", Entries: many}}

	out := Render(r, "")
	if strings.Count(out, "feature number ") != maxEntriesPerSection {
		t.Fatalf("expected %d listed entries:\n%s", maxEntriesPerSection, out)
	}
	if !strings.Contains(out, "and 7 more") {
		t.Fatalf("the remainder was not counted:\n%s", out)
	}
}

// TestRenderTruncatesButKeepsTheLink: when the body is cut, the link is the one
// line that must survive — it is what the truncated text points at.
func TestRenderTruncatesButKeepsTheLink(t *testing.T) {
	const url = "https://tionharness.com/releases/v1.2.3"
	r := sampleRelease()
	long := strings.Repeat("a very long subject line that goes on and on ", 40)
	var entries []Entry
	for i := 0; i < maxEntriesPerSection; i++ {
		entries = append(entries, Entry{Hash: "x", Subject: long})
	}
	r.Sections = []Section{{Title: "Features", Entries: entries}}

	out := Render(r, url)
	if n := len([]rune(out)); n > maxBodyRunes {
		t.Fatalf("body is %d runes, over the %d cap", n, maxBodyRunes)
	}
	if !strings.Contains(out, url) {
		t.Fatalf("truncation dropped the notes link:\n%s", out)
	}
}

// TestRenderHandlesAnEmptyRelease: a release with no highlighted sections still
// announces the version rather than emitting a blank message.
func TestRenderHandlesAnEmptyRelease(t *testing.T) {
	out := Render(Release{Tag: "v9.9.9", Date: "2026-01-01"}, "")
	if !strings.Contains(out, "v9.9.9") {
		t.Fatalf("version is missing from an empty release:\n%s", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("an empty release rendered a blank message")
	}
}

// TestParseTarget keeps the target set closed.
func TestParseTarget(t *testing.T) {
	for _, good := range []string{"discord", "Discord", " telegram "} {
		if _, err := ParseTarget(good); err != nil {
			t.Fatalf("%q was rejected: %v", good, err)
		}
	}
	for _, bad := range []string{"", "slack", "email"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}
