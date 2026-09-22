package changelog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Release is one version's worth of grouped changes — the value both renderers
// and the announcement fan-out consume.
type Release struct {
	Version  string    `json:"version"`  // without the leading v
	Tag      string    `json:"tag"`      // as it exists in git (v1.2.3)
	Date     string    `json:"date"`     // YYYY-MM-DD
	Previous string    `json:"previous"` // the tag this release is diffed against ("" for the first)
	Sections []Section `json:"sections"` // ordered, breaking first
	Count    int       `json:"count"`    // commits in the range
}

// Build assembles a Release from a tag by resolving its predecessor and
// grouping the commits between them.
func Build(dir, tag string) (Release, error) {
	prev, err := PreviousTag(dir, tag)
	if err != nil {
		return Release{}, err
	}
	commits, err := Commits(dir, prev, tag)
	if err != nil {
		return Release{}, err
	}
	date, err := TagDate(dir, tag)
	if err != nil {
		return Release{}, err
	}
	return Release{
		Version:  strings.TrimPrefix(tag, "v"),
		Tag:      tag,
		Date:     date,
		Previous: prev,
		Sections: group(commits),
		Count:    len(commits),
	}, nil
}

// MarshalJSON is defined on Section so release.json carries a readable shape
// (title + entries) rather than Go field names.
func (s Section) MarshalJSON() ([]byte, error) {
	type entry struct {
		Hash     string `json:"hash"`
		Scope    string `json:"scope,omitempty"`
		Subject  string `json:"subject"`
		Breaking bool   `json:"breaking,omitempty"`
	}
	entries := make([]entry, 0, len(s.Commits))
	for _, c := range s.Commits {
		entries = append(entries, entry{Hash: c.Hash, Scope: c.Scope, Subject: c.Subject, Breaking: c.Breaking})
	}
	return json.Marshal(struct {
		Title   string  `json:"title"`
		Entries []entry `json:"entries"`
	}{Title: s.Title, Entries: entries})
}

// RenderMarkdown renders one release as a CHANGELOG.md section.
//
// Entries are "scope: subject (hash)". The scope is kept because it is the
// fastest way for a reader to tell whether a line concerns them, and the hash
// because a changelog line is useless if you cannot find the commit behind it.
func RenderMarkdown(r Release) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s\n\n", r.Tag, r.Date)
	if r.Count == 0 {
		b.WriteString("_No changes._\n")
		return b.String()
	}
	for _, s := range r.Sections {
		fmt.Fprintf(&b, "### %s\n\n", s.Title)
		for _, c := range s.Commits {
			b.WriteString("- ")
			if c.Scope != "" {
				fmt.Fprintf(&b, "**%s:** ", c.Scope)
			}
			b.WriteString(c.Subject)
			fmt.Fprintf(&b, " (%s)\n", c.Hash)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Prepend inserts a rendered release at the top of an existing CHANGELOG body,
// below its title, so the newest release is read first.
//
// A release already present in the body is NOT inserted again: re-running the
// generator for the same tag must be idempotent, or a retried release job would
// duplicate the section.
func Prepend(existing, rendered, tag string) string {
	if strings.Contains(existing, "## "+tag+" ") {
		return existing
	}
	const title = "# Changelog"
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return title + "\n\n" + rendered
	}
	if rest, ok := strings.CutPrefix(existing, title); ok {
		return title + "\n\n" + rendered + strings.TrimLeft(rest, "\n") + "\n"
	}
	return title + "\n\n" + rendered + existing + "\n"
}

// SortedTypes returns the recognised conventional types, for the tool's help
// output and the docs. Sorted so the list is stable.
func SortedTypes() []string {
	out := make([]string, 0, len(typeTitles))
	for t := range typeTitles {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
