package agent

import (
	"regexp"
	"strings"
)

// Context change diff — what drifted between a session's frozen static prefix
// (prompt epoch snapshot) and current live state.
//
// The prompt epoch (promptepoch.go) freezes the cacheable prefix at session
// start and keeps shipping the same bytes so mid-session edits never bust the
// prompt cache. That protects cost but leaves the agent (and the user) blind to
// WHAT changed until the next refresh. This layer computes a cheap, self-
// labelling diff of that drift and surfaces it two ways, both on the VOLATILE
// side so neither breaks the cache the epoch exists to protect:
//
//   - to the agent, as a compact note appended to the dynamic suffix, so it can
//     partially adapt before the frozen prefix is adopted;
//   - to the user, as a context_change TurnStep in the chat trace (emitted once
//     per drift episode).
//
// The static prefix is assembled by joining labelled blocks with a blank line
// (buildStaticPrefix / autonomousSystemPrompt), so a paragraph-level diff is
// naturally block-aligned: each changed paragraph's first line is a meaningful
// label (a header like "# Workspace Instructions" or "<user_context>", the
// skills catalog header, etc.) without any hard-coded marker registry.

// ContextChangeKind classifies how one area of the static context changed.
type ContextChangeKind string

const (
	// ContextAdded marks a paragraph/tool present live but not in the snapshot.
	ContextAdded ContextChangeKind = "added"
	// ContextRemoved marks a paragraph/tool present in the snapshot but not live.
	ContextRemoved ContextChangeKind = "removed"
	// ContextModified marks a paragraph present in both, in two different forms:
	// its Lines carry a unified line diff instead of a plain body.
	ContextModified ContextChangeKind = "modified"
)

// ContextArea is one changed region of the frozen prefix (or tool set), self-
// labelled by the first line of the changed paragraph.
type ContextArea struct {
	Label string            `json:"label"`
	Kind  ContextChangeKind `json:"kind"`
	// Lines is the changed paragraph's body (capped), so the UI can show the
	// actual text on expand. Empty for a tool-name area. For a "modified" area
	// it is a unified diff instead: each line is prefixed with " ", "-" or "+",
	// and elided runs are a lone "…".
	Lines []string `json:"lines,omitempty"`
}

// ContextChange is the drift between the frozen snapshot and live state.
type ContextChange struct {
	Areas []ContextArea `json:"areas"`
	// Added/Removed are the headline counts. A wholly new or wholly deleted
	// paragraph (or tool) counts as one; a MODIFIED paragraph contributes the
	// real number of added/removed LINES in its diff, so a block that only lost
	// lines reports no additions.
	Added   int `json:"added"`
	Removed int `json:"removed"`
	// Truncated reports that some changed areas were dropped from Areas to keep
	// the payload bounded (surfaced as "+N more" in the note/UI).
	Truncated int `json:"truncated,omitempty"`
}

// Caps keep the diff bounded: the note rides every stale turn's suffix, so an
// unbounded diff would inflate token cost until the refresh.
const (
	maxContextAreas     = 12  // areas kept in a single change
	maxAreaLines        = 24  // body lines kept per area
	maxAreaLineLen      = 400 // per-line character cap
	maxNoteAreasListed  = 8   // areas spelled out in the agent-facing suffix note
	contextLabelMaxRune = 96  // label truncation
)

var blankLineSplit = regexp.MustCompile(`\n[ \t]*\n`)

// splitParagraphs cuts a composed prefix into blank-line-separated paragraphs,
// trimmed, dropping empties. This mirrors how buildStaticPrefix joins blocks.
func splitParagraphs(s string) []string {
	parts := blankLineSplit.Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// diffParagraphs returns the paragraphs added and removed between old and new
// via a standard LCS, preserving order. Unchanged paragraphs are skipped; a
// modified paragraph surfaces as one removal (old form) plus one addition (new
// form), which reads correctly in the UI.
func diffParagraphs(oldP, newP []string) (added, removed []string) {
	m, n := len(oldP), len(newP)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			switch {
			case oldP[i] == newP[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case oldP[i] == newP[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			removed = append(removed, oldP[i])
			i++
		default:
			added = append(added, newP[j])
			j++
		}
	}
	for ; i < m; i++ {
		removed = append(removed, oldP[i])
	}
	for ; j < n; j++ {
		added = append(added, newP[j])
	}
	return added, removed
}

// diffSystemPrefix computes the paragraph-level change between the frozen and
// live static system prefixes. Returns nil when they are byte-identical.
func diffSystemPrefix(frozen, live string) *ContextChange {
	if frozen == live {
		return nil
	}
	added, removed := diffParagraphs(splitParagraphs(frozen), splitParagraphs(live))
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	c := &ContextChange{}
	// Pair each removal with the addition that is the same block in a new form;
	// those become one "modified" area carrying a line diff, so the user sees the
	// changed lines instead of two near-identical walls of text.
	pairTo := pairParagraphs(removed, added)
	pairedAdd := make([]bool, len(added))
	for i, p := range removed {
		if j := pairTo[i]; j >= 0 {
			pairedAdd[j] = true
			// Counters follow the actual edit: a paired block contributes its real
			// added/removed LINE counts, not a blanket "+1 -1". A block that only lost
			// lines must not advertise an addition the user then hunts for in the diff.
			area, addLines, delLines := modifiedArea(p, added[j])
			c.Added += addLines
			c.Removed += delLines
			c.appendArea(area)
			continue
		}
		// An unpaired paragraph is a whole block gained or lost: counted as one.
		c.Removed++
		c.appendArea(paragraphArea(p, ContextRemoved))
	}
	for j, p := range added {
		if pairedAdd[j] {
			continue
		}
		c.Added++
		c.appendArea(paragraphArea(p, ContextAdded))
	}
	return c
}

// diffToolNames computes the added/removed tool names between the frozen and
// live eager tool sets. Returns nil when the name sets match (a pure schema-
// body change is reported by the epoch's toolsStale flag but not itemised here,
// since a tool schema is not human-readable in a chat chip).
func diffToolNames(frozen, live []string) *ContextChange {
	fs := make(map[string]bool, len(frozen))
	for _, n := range frozen {
		fs[n] = true
	}
	ls := make(map[string]bool, len(live))
	for _, n := range live {
		ls[n] = true
	}
	var c ContextChange
	for _, n := range live {
		if !fs[n] {
			c.Added++
			c.appendArea(ContextArea{Label: "Tool: " + n, Kind: ContextAdded})
		}
	}
	for _, n := range frozen {
		if !ls[n] {
			c.Removed++
			c.appendArea(ContextArea{Label: "Tool: " + n, Kind: ContextRemoved})
		}
	}
	if len(c.Areas) == 0 && c.Truncated == 0 {
		return nil
	}
	return &c
}

// paragraphArea builds a ContextArea from a changed paragraph: the first non-
// empty line becomes the label, the whole (capped) body the detail.
func paragraphArea(p string, kind ContextChangeKind) ContextArea {
	lines := strings.Split(p, "\n")
	label := paragraphLabel(p)
	body := make([]string, 0, len(lines))
	for _, l := range lines {
		// Guard by RUNE count, not byte length: len(l) is bytes, but the slice below
		// indexes []rune. A line with multi-byte UTF-8 (Turkish ç/ğ/ı/ö/ş/ü, emoji,
		// CJK) can exceed maxAreaLineLen BYTES while holding fewer RUNES, so the old
		// len(l)>max guard let string([]rune(l)[:max]) slice past the rune slice's
		// capacity and panic ("slice bounds out of range [:400] with capacity 384").
		if r := []rune(l); len(r) > maxAreaLineLen {
			l = string(r[:maxAreaLineLen]) + "…"
		}
		body = append(body, l)
		if len(body) >= maxAreaLines {
			body = append(body, "…")
			break
		}
	}
	return ContextArea{Label: label, Kind: kind, Lines: body}
}

// appendArea adds an area under the cap, counting overflow into Truncated.
func (c *ContextChange) appendArea(a ContextArea) {
	if len(c.Areas) >= maxContextAreas {
		c.Truncated++
		return
	}
	c.Areas = append(c.Areas, a)
}

// merge folds another change into c (system + tools into one step/note). nil
// operands are ignored.
func mergeContextChanges(a, b *ContextChange) *ContextChange {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	a.Added += b.Added
	a.Removed += b.Removed
	a.Truncated += b.Truncated
	for _, area := range b.Areas {
		a.appendArea(area)
	}
	return a
}

// Empty reports whether the change carries nothing worth surfacing.
func (c *ContextChange) Empty() bool {
	return c == nil || (len(c.Areas) == 0 && c.Truncated == 0)
}

// Summary is the one-line headline for the chat step and logs.
func (c *ContextChange) Summary() string {
	if c == nil {
		return ""
	}
	b := strings.Builder{}
	b.WriteString("Static context changed")
	if c.Added > 0 || c.Removed > 0 {
		b.WriteString(": ")
		if c.Added > 0 {
			b.WriteString("+" + itoa(c.Added))
		}
		if c.Added > 0 && c.Removed > 0 {
			b.WriteString(" ")
		}
		if c.Removed > 0 {
			b.WriteString("-" + itoa(c.Removed))
		}
	}
	return b.String()
}

// suffixNoteMarker wraps both the generic stale note and this rich note, so the
// tool loop's double-append guard (strings.Contains) treats them the same.
const suffixNoteMarker = "<context_snapshot_note>"

// SuffixNote renders the agent-facing note appended to the dynamic suffix while
// the change is pending. Wrapped in the shared marker tag so it never double-
// appends with the generic note and can never bust the cached prefix.
func (c *ContextChange) SuffixNote() string {
	if c.Empty() {
		return PromptEpochStaleNote
	}
	b := strings.Builder{}
	b.WriteString("<context_snapshot_note>Parts of your static context (persona, instructions, ")
	b.WriteString("skills or tool catalog) changed mid-session; this prompt still shows the session-start ")
	b.WriteString("snapshot (kept byte-stable to preserve the prompt cache). Pending changes apply on the ")
	b.WriteString("next context refresh — automatic after a compaction or an idle gap, or on demand via the ")
	b.WriteString("/refresh-context chat command (or update_session with refresh_context). Behaviour is safe ")
	b.WriteString("meanwhile: disabled tools fail closed at execution, newly added tools become callable after ")
	b.WriteString("the refresh.\nWhat changed since the snapshot:")
	listed := 0
	for _, a := range c.Areas {
		if listed >= maxNoteAreasListed {
			break
		}
		sign := "+"
		switch a.Kind {
		case ContextRemoved:
			sign = "-"
		case ContextModified:
			sign = "~"
		}
		b.WriteString("\n" + sign + " " + a.Label)
		listed++
	}
	if extra := (len(c.Areas) - listed) + c.Truncated; extra > 0 {
		b.WriteString("\n… and " + itoa(extra) + " more")
	}
	b.WriteString("</context_snapshot_note>")
	return b.String()
}

// itoa is a tiny non-allocating-path integer formatter (avoids importing strconv
// just for two call sites in this file).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
