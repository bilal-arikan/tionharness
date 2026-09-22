// Package announce renders a release into the short messages the announcement
// fan-out posts to chat webhooks, and delivers them.
//
// The fan-out is deliberately NOT a CI bot: TionHarness's own schedule + agent
// drive it (see _Docs/86), so the project dogfoods its automation instead of
// maintaining a second, parallel one in the release workflow.
package announce

import (
	"fmt"
	"strings"
)

// maxBodyRunes bounds a rendered announcement. Discord's content limit is 2000
// characters and Telegram's is 4096; staying under the lower one keeps a single
// renderer valid for both targets. A release with a long changelog is truncated
// with a pointer to the notes rather than rejected.
const maxBodyRunes = 1800

// Release is the part of _Docs/release.json the announcement needs. It is
// declared here rather than imported from internal/changelog so the fan-out
// stays decoupled from how the changelog is produced: the contract between them
// is the JSON file, not a Go type.
type Release struct {
	Version  string    `json:"version"`
	Tag      string    `json:"tag"`
	Date     string    `json:"date"`
	Previous string    `json:"previous"`
	Sections []Section `json:"sections"`
	Count    int       `json:"count"`
}

// Section is one heading of the changelog with its entries.
type Section struct {
	Title   string  `json:"title"`
	Entries []Entry `json:"entries"`
}

// Entry is one changelog line.
type Entry struct {
	Hash     string `json:"hash"`
	Scope    string `json:"scope,omitempty"`
	Subject  string `json:"subject"`
	Breaking bool   `json:"breaking,omitempty"`
}

// highlightSections are the only sections carried into an announcement, in this
// order. An announcement is a headline, not the changelog: chores, style and
// test commits belong in the full notes, and including them would push the
// interesting lines past the length cap.
var highlightSections = []string{"BREAKING CHANGES", "Features", "Bug Fixes"}

// maxEntriesPerSection bounds one section in the announcement. Beyond this the
// remainder is counted rather than listed.
const maxEntriesPerSection = 5

// Render turns a release into the announcement body. notesURL, when non-empty,
// is appended so a reader can reach the full notes.
//
// The output is plain text with light Markdown: both Discord and Telegram
// (parse_mode=Markdown) render "**bold**" and "- " lists, so one rendering
// serves both and there is no per-target formatter to keep in step.
func Render(r Release, notesURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s %s** released", projectName, r.Tag)
	if r.Date != "" {
		fmt.Fprintf(&b, " — %s", r.Date)
	}
	b.WriteString("\n")

	for _, want := range highlightSections {
		s, ok := findSection(r, want)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n**%s**\n", s.Title)
		shown := s.Entries
		if len(shown) > maxEntriesPerSection {
			shown = shown[:maxEntriesPerSection]
		}
		for _, e := range shown {
			b.WriteString("- ")
			if e.Scope != "" {
				fmt.Fprintf(&b, "%s: ", e.Scope)
			}
			b.WriteString(e.Subject)
			b.WriteString("\n")
		}
		if rest := len(s.Entries) - len(shown); rest > 0 {
			fmt.Fprintf(&b, "- …and %d more\n", rest)
		}
	}

	if notesURL != "" {
		fmt.Fprintf(&b, "\nFull notes: %s", notesURL)
	}
	return truncate(strings.TrimRight(b.String(), "\n"), notesURL)
}

// projectName is the name the announcement leads with.
const projectName = "TionHarness"

// findSection returns the named section.
func findSection(r Release, title string) (Section, bool) {
	for _, s := range r.Sections {
		if s.Title == title {
			return s, true
		}
	}
	return Section{}, false
}

// truncate caps the body at maxBodyRunes, cutting on a line boundary and
// keeping the notes link — the link is the one line a reader must not lose,
// because it is what the truncated text points them to.
func truncate(body, notesURL string) string {
	if len([]rune(body)) <= maxBodyRunes {
		return body
	}
	tail := ""
	if notesURL != "" {
		tail = "\n…\nFull notes: " + notesURL
	} else {
		tail = "\n…"
	}
	budget := max(maxBodyRunes-len([]rune(tail)), 0)
	cut := string([]rune(body)[:budget])
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return cut + tail
}
