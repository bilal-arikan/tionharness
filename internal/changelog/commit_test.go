package changelog

import "testing"

// TestParseCommitConventionalForms pins the subject grammar: type, optional
// scope, optional "!", description.
func TestParseCommitConventionalForms(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		wantType string
		scope    string
		breaking bool
		subject  string
	}{
		{"plain", "feat: add a thing", "feat", "", false, "add a thing"},
		{"scoped", "fix(api): stop the leak", "fix", "api", false, "stop the leak"},
		{"bang", "feat!: drop the old flag", "feat", "", true, "drop the old flag"},
		{"scoped bang", "refactor(db)!: rename the column", "refactor", "db", true, "rename the column"},
		{"uppercase type is normalised", "Feat: shout", "feat", "", false, "shout"},
		{"colon in the description survives", "docs: explain a:b", "docs", "", false, "explain a:b"},
		{"scope with a slash", "ci(build/win): pin the runner", "ci", "build/win", false, "pin the runner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := parseCommit("abc1234", tc.message)
			if c.Type != tc.wantType || c.Scope != tc.scope || c.Breaking != tc.breaking || c.Subject != tc.subject {
				t.Fatalf("got type=%q scope=%q breaking=%v subject=%q",
					c.Type, c.Scope, c.Breaking, c.Subject)
			}
		})
	}
}

// TestParseCommitKeepsUnconventionalSubjects: this repository predates the
// convention in places. An unparseable subject must keep its text and fall to
// the Other section, never vanish from the release.
func TestParseCommitKeepsUnconventionalSubjects(t *testing.T) {
	for _, msg := range []string{"snapshot: land the in-flight work", "rename things", "WIP"} {
		c := parseCommit("abc1234", msg)
		if c.Subject == "" {
			t.Fatalf("subject %q was dropped", msg)
		}
	}
	// "snapshot:" parses as a type — an unrecognised one, which is exactly what
	// the Other bucket is for.
	c := parseCommit("abc1234", "rename things")
	if c.Type != "" {
		t.Fatalf("a subject with no colon got type %q", c.Type)
	}
	if got := group([]Commit{c}); len(got) != 1 || got[0].Title != otherTitle {
		t.Fatalf("an unconventional commit did not land in %q: %+v", otherTitle, got)
	}
}

// TestParseCommitBreakingFooter covers the spec's alternative to "!".
func TestParseCommitBreakingFooter(t *testing.T) {
	for _, body := range []string{"BREAKING CHANGE: the flag is gone", "BREAKING-CHANGE: the flag is gone"} {
		c := parseCommit("abc1234", "feat: something\n\n"+body)
		if !c.Breaking {
			t.Fatalf("footer %q was not recognised as breaking", body)
		}
	}
	// The words must be a FOOTER, not a mention in prose.
	c := parseCommit("abc1234", "feat: something\n\nthis is not a BREAKING CHANGE at all")
	if c.Breaking {
		t.Fatal("a prose mention was treated as a breaking change")
	}
}

// TestGroupOrdersSectionsAndKeepsBreakingInBoth: the breaking block answers
// "what will break", the type section answers "what changed". A reader
// browsing by category must still see the commit.
func TestGroupOrdersSectionsAndKeepsBreakingInBoth(t *testing.T) {
	commits := []Commit{
		parseCommit("1", "chore: tidy"),
		parseCommit("2", "feat!: big change"),
		parseCommit("3", "fix: small fix"),
	}
	got := group(commits)
	if len(got) != 4 {
		t.Fatalf("expected breaking+feat+fix+chore, got %d: %+v", len(got), got)
	}
	want := []string{breakingTitle, "Features", "Bug Fixes", "Chores"}
	for i, w := range want {
		if got[i].Title != w {
			t.Fatalf("section %d is %q, want %q", i, got[i].Title, w)
		}
	}
	if len(got[1].Commits) != 1 || got[1].Commits[0].Hash != "2" {
		t.Fatal("the breaking commit was removed from its type section")
	}
}

// TestPrependIsIdempotent: a retried release job must not duplicate a section.
func TestPrependIsIdempotent(t *testing.T) {
	rendered := "## v1.2.3 — 2026-01-01\n\n### Features\n\n- a thing (abc)\n\n"
	first := Prepend("", rendered, "v1.2.3")
	second := Prepend(first, rendered, "v1.2.3")
	if first != second {
		t.Fatalf("re-running duplicated the section:\n%s", second)
	}
	if got := countOccurrences(second, "## v1.2.3 "); got != 1 {
		t.Fatalf("section appears %d times", got)
	}
}

// TestPrependPutsNewestFirst: the newest release must be read first.
func TestPrependPutsNewestFirst(t *testing.T) {
	old := Prepend("", "## v1.0.0 — 2026-01-01\n\n- old (aaa)\n\n", "v1.0.0")
	both := Prepend(old, "## v1.1.0 — 2026-02-01\n\n- new (bbb)\n\n", "v1.1.0")
	iNew, iOld := indexOf(both, "v1.1.0"), indexOf(both, "v1.0.0")
	if iNew == -1 || iOld == -1 || iNew > iOld {
		t.Fatalf("the newer release is not on top:\n%s", both)
	}
	if got := countOccurrences(both, "# Changelog"); got != 1 {
		t.Fatalf("the title appears %d times", got)
	}
}

// TestIsVersionTag: a marker tag such as before-rename is not a release
// boundary and must never start a changelog range.
func TestIsVersionTag(t *testing.T) {
	good := []string{"v1.2.3", "v0.0.1", "v1.2.3-rc1", "v10.20.30", "v1.2.3+build"}
	bad := []string{"before-rename", "v1.2", "1.2.3", "v1.2.3.4", "vX.Y.Z", "v", "va.b.c"}
	for _, g := range good {
		if !isVersionTag(g) {
			t.Fatalf("%q was rejected", g)
		}
	}
	for _, b := range bad {
		if isVersionTag(b) {
			t.Fatalf("%q was accepted", b)
		}
	}
}

// RenderMarkdown must show the scope and the hash: without the hash a
// changelog line cannot be traced back to a commit.
func TestRenderMarkdownShowsScopeAndHash(t *testing.T) {
	r := Release{Tag: "v1.2.3", Date: "2026-01-01", Count: 1,
		Sections: group([]Commit{parseCommit("deadbee", "fix(api): stop the leak")})}
	out := RenderMarkdown(r)
	for _, want := range []string{"## v1.2.3 — 2026-01-01", "### Bug Fixes", "**api:**", "stop the leak", "(deadbee)"} {
		if indexOf(out, want) == -1 {
			t.Fatalf("output is missing %q:\n%s", want, out)
		}
	}
}

// TestRenderMarkdownEmptyRelease: a tag with no commits since the last one must
// say so rather than render an empty, confusing section list.
func TestRenderMarkdownEmptyRelease(t *testing.T) {
	out := RenderMarkdown(Release{Tag: "v1.2.3", Date: "2026-01-01"})
	if indexOf(out, "No changes") == -1 {
		t.Fatalf("an empty release did not say so:\n%s", out)
	}
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func countOccurrences(haystack, needle string) int {
	n, i := 0, 0
	for i+len(needle) <= len(haystack) {
		if haystack[i:i+len(needle)] == needle {
			n++
			i += len(needle)
			continue
		}
		i++
	}
	return n
}
