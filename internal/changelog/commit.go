package changelog

import (
	"regexp"
	"strings"
)

// Commit is one parsed conventional commit.
type Commit struct {
	Hash     string // short hash
	Type     string // feat, fix, docs, … ("" when the subject is not conventional)
	Scope    string // the optional (scope), without parentheses
	Breaking bool   // a "!" before the colon, or a BREAKING CHANGE footer
	Subject  string // the description after the colon (or the whole subject when unconventional)
	Body     string // everything after the subject line
}

// conventionalRe matches the conventional-commit subject line:
//
//	type(scope)!: description
//
// The type is letters only, the scope is anything but ")", and both the scope
// and the "!" are optional. A subject that does not match is NOT an error: the
// repository predates the convention in places, and those commits still belong
// in the changelog under "other".
var conventionalRe = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]*)\))?(!)?:[ \t]*(.+)$`)

// breakingFooterRe matches the footer form of a breaking change, which is the
// spec's alternative to the "!" marker.
var breakingFooterRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE[:(]`)

// parseCommit turns one hash + raw message into a Commit. An unconventional
// subject yields an empty Type and keeps the whole subject as the description,
// so nothing is silently dropped from the changelog.
func parseCommit(hash, message string) Commit {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	subject, body, _ := strings.Cut(message, "\n")
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)

	c := Commit{Hash: hash, Subject: subject, Body: body}
	if m := conventionalRe.FindStringSubmatch(subject); m != nil {
		c.Type = strings.ToLower(m[1])
		c.Scope = m[2]
		c.Breaking = m[3] == "!"
		c.Subject = strings.TrimSpace(m[4])
	}
	if breakingFooterRe.MatchString(body) {
		c.Breaking = true
	}
	return c
}

// Section is one rendered group of commits: a heading plus its entries.
type Section struct {
	Title   string
	Commits []Commit
}

// typeTitles maps a conventional type to its changelog heading. A type missing
// from this map is grouped under "other" rather than given a heading of its
// own — an invented heading per typo would fragment the changelog.
var typeTitles = map[string]string{
	"feat":     "Features",
	"fix":      "Bug Fixes",
	"perf":     "Performance",
	"refactor": "Refactoring",
	"docs":     "Documentation",
	"test":     "Tests",
	"build":    "Build",
	"ci":       "CI",
	"style":    "Style",
	"chore":    "Chores",
	"revert":   "Reverts",
}

// sectionOrder fixes the order the headings appear in. What a reader wants
// first is what changed for THEM, so features and fixes lead and housekeeping
// trails.
var sectionOrder = []string{"feat", "fix", "perf", "refactor", "docs", "test", "build", "ci", "style", "chore", "revert"}

// otherTitle collects commits with no recognised type, so an unconventional or
// mistyped subject is still visible instead of vanishing from the release.
const otherTitle = "Other"

// breakingTitle leads every release that has one: a breaking change is the one
// thing a reader must not miss.
const breakingTitle = "BREAKING CHANGES"

// group buckets commits into ordered sections. Breaking changes are listed
// FIRST, and also remain in their own type section: the breaking block answers
// "what will break", the type section answers "what changed" — dropping the
// duplicate would hide the change from whoever reads by category.
func group(commits []Commit) []Section {
	var sections []Section

	var breaking []Commit
	for _, c := range commits {
		if c.Breaking {
			breaking = append(breaking, c)
		}
	}
	if len(breaking) > 0 {
		sections = append(sections, Section{Title: breakingTitle, Commits: breaking})
	}

	byType := map[string][]Commit{}
	var other []Commit
	for _, c := range commits {
		if _, known := typeTitles[c.Type]; known {
			byType[c.Type] = append(byType[c.Type], c)
			continue
		}
		other = append(other, c)
	}
	for _, t := range sectionOrder {
		if cs := byType[t]; len(cs) > 0 {
			sections = append(sections, Section{Title: typeTitles[t], Commits: cs})
		}
	}
	if len(other) > 0 {
		sections = append(sections, Section{Title: otherTitle, Commits: other})
	}
	return sections
}
