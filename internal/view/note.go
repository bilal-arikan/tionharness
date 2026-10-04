package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/notes"
)

// NotesSource lists and resolves the workspace's memory notes (internal/notes).
// Optional like the other sources: without it the Notlar category and the note
// leaves report the source as unavailable instead of failing the map.
type NotesSource interface {
	ListNotes() []notes.Note
	GetNote(id string) (notes.Note, bool)
	ExpandNote(id string) (notes.Expansion, error)
}

// NoteInput is one note plus the clock.
type NoteInput struct {
	Note notes.Note
	// Expansion is optional: when set, the card lists link/backlink counts and
	// the correction chain.
	Expansion *notes.Expansion
	Now       time.Time
}

// ProjectNote renders one memory note: what it claims, how sure it is, who it
// reaches, and whether it has been corrected. The body is clipped by level; the
// full text stays behind note_expand / the Notes screen.
func ProjectNote(in NoteInput, level Level) (View, error) {
	n := in.Note
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if n.ID == "" {
		return View{}, fmt.Errorf("view: note has no id")
	}
	v := View{
		Ref:    Ref{Kind: KindNote, ID: n.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%d/%s", n.Updated, n.Occurrences, n.SupersededBy),
	}
	status := ""
	switch {
	case n.Archived:
		status = " · archived"
	case n.Retired():
		status = " · superseded by " + n.SupersededBy
	case n.Private:
		status = " · private"
	}
	v.Header = fmt.Sprintf("NOTE · %s · [%s·%s] %s%s · asOf %s", n.ID, n.Kind, n.Confidence, clip(n.Title, 60), status, hhmmss(now))
	if level == LevelTiny {
		v.finalize()
		return v, nil
	}
	var l lines
	reach := string(n.Scope)
	switch n.Scope {
	case notes.ScopeAgent:
		reach += ": " + strings.Join(n.Agents, ", ")
	case notes.ScopeProject:
		reach += ": " + strings.Join(n.Projects, ", ")
	}
	l.add("reach: %s", reach)
	meta := []string{}
	if n.Source != "" {
		meta = append(meta, "source "+n.Source)
	}
	if n.SourceAgent != "" {
		meta = append(meta, "by "+n.SourceAgent)
	}
	if n.SourceSession != "" {
		meta = append(meta, "in "+n.SourceSession)
	}
	if n.Occurrences > 1 {
		meta = append(meta, fmt.Sprintf("seen %d×", n.Occurrences))
	}
	if n.Updated > 0 {
		meta = append(meta, "updated "+age(time.Unix(n.Updated, 0), now))
	}
	if len(meta) > 0 {
		l.add("%s", strings.Join(meta, " · "))
	}
	if n.Verification != "" {
		l.add("verified by: %s", clip(n.Verification, 120))
	}
	if n.Supersedes != "" {
		l.add("corrects: %s", n.Supersedes)
	}
	if len(n.Tags) > 0 {
		l.add("tags: %s", strings.Join(n.Tags, ", "))
	}
	if in.Expansion != nil {
		ex := in.Expansion
		if len(ex.Links)+len(ex.Backlinks)+len(ex.Unresolved) > 0 {
			l.add("links: %d out · %d in · %d unresolved", len(ex.Links), len(ex.Backlinks), len(ex.Unresolved))
		}
	} else if len(n.Links) > 0 {
		l.add("links: %d out", len(n.Links))
	}
	body := strings.TrimSpace(n.Body)
	max := 400
	if level == LevelFull {
		max = 2000
	}
	if body != "" {
		shown := clip(body, max)
		l.add("--")
		l.add("%s", shown)
		if len([]rune(body)) > max {
			v.Elided = 1
			v.ElidedUnit = "gövde parçası"
			v.Handles = append(v.Handles, Handle{Label: "tam metin: note_expand " + n.ID, Ref: Ref{Kind: KindNote, ID: n.ID}, Level: LevelFull})
		}
	}
	v.Body = l.String()
	v.finalize()
	return v, nil
}

// noteHandleList renders notes as category member handles, newest first (the
// source already orders them so).
func noteHandleList(ns []notes.Note) []Handle {
	hs := make([]Handle, 0, len(ns))
	for _, n := range ns {
		hs = append(hs, Handle{
			Label: "note:" + n.ID + " [" + string(n.Kind) + "] " + clip(n.Title, 40),
			Ref:   Ref{Kind: KindNote, ID: n.ID},
			Level: LevelCard,
		})
	}
	return hs
}

// noteChildren are a note's structural edges: the notes it links to and the
// correction that replaced it, so the map walks the memory graph the same way
// it walks sessions and workers.
func (p *Projector) noteChildren(id string) ([]Handle, error) {
	if p.sources.Notes == nil {
		return nil, fmt.Errorf("view: note %s: notes store unavailable", id)
	}
	ex, err := p.sources.Notes.ExpandNote(id)
	if err != nil {
		return nil, fmt.Errorf("view: note %s: %w", id, err)
	}
	var hs []Handle
	for _, s := range ex.Successors {
		hs = append(hs, Handle{Label: "correction: " + s.ID + " " + clip(s.Title, 40), Ref: Ref{Kind: KindNote, ID: s.ID}, Level: LevelCard})
		break // the immediate successor; it drills into its own
	}
	hs = append(hs, noteHandleList(ex.Links)...)
	return hs, nil
}

// loadNote resolves one note for Project.
func (p *Projector) loadNote(id string) (NoteInput, error) {
	if p.sources.Notes == nil {
		return NoteInput{}, fmt.Errorf("view: note %s: notes store unavailable", id)
	}
	n, ok := p.sources.Notes.GetNote(id)
	if !ok {
		return NoteInput{}, fmt.Errorf("view: note %s not found", id)
	}
	in := NoteInput{Note: n}
	if ex, err := p.sources.Notes.ExpandNote(id); err == nil {
		in.Expansion = &ex
	}
	return in, nil
}
