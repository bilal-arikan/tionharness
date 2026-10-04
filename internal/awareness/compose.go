// Package awareness is the one place a session's situational context is
// assembled: what an agent is told at session start (the brief), what rides
// every turn (the volatile suffix with its pulse), and what is recorded when a
// turn ends (the digest). Three moments, one budget discipline, one meter.
//
// Before this layer the per-turn context was a dozen independent string
// builders appended to one another with no shared budget: nothing could tell
// how much context a turn cost, nothing degraded gracefully, and the pieces
// disagreed on what "the workspace" looked like. Every producer now emits a
// Section with a priority and an optional pointer form, Compose fits them to a
// byte budget by degrading the cheapest-to-lose sections first (never the
// pinned ones), and the last line of every composition is a meter that names
// what was degraded or dropped — a silent loss misleads; a named loss is a
// signal. See _Docs/94-FARKINDALIK-VE-NOTLAR.md.
//
// The package imports view (projections), notes (memory), progress and db
// (models) and nothing else from the application; the agent runtime and the
// api layer wire it.
package awareness

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// Moment is when a composition is delivered.
type Moment string

const (
	// MomentBrief is the session-start briefing. It is composed once per
	// session (and again after a compaction) and lives in the cacheable static
	// prefix, so it must not change between turns.
	MomentBrief Moment = "brief"
	// MomentTurn is the volatile per-turn suffix: clock, session state, tool
	// recap, checklist, pulse. It changes every turn by nature and never touches
	// the cached prefix.
	MomentTurn Moment = "turn"
	// MomentWrap is the end-of-turn digest: what this session did, recorded for
	// the next session's brief and for the user.
	MomentWrap Moment = "wrap"
)

// PriorityPinned marks a section that is never degraded or dropped (identity,
// pending questions, the open checklist). Everything else is a positive number;
// the HIGHER the number, the sooner the section degrades under pressure.
const PriorityPinned = 0

// Section is one producer's contribution.
type Section struct {
	// Key names the section in the meter and in tests.
	Key string
	// Text is the full rendering. An empty Text means "nothing to say" and the
	// section is skipped entirely (not counted, not reported).
	Text string
	// Pointer is the degraded form: a one-liner that says what was omitted and
	// how to get it ("12 artifacts in this session; list_artifacts"). A section
	// without a pointer is dropped outright when it must go.
	Pointer string
	// Priority orders degradation: PriorityPinned never degrades; among the rest
	// the highest number goes first, ties broken by size (largest first).
	Priority int
	// Volatile marks content that changes turn to turn. Informational: the
	// brief refuses volatile sections, since it is frozen into the prefix.
	Volatile bool
	// Urgent is set by the pulse producer when the rule-based urgency fires; the
	// service pins an urgent pulse and adds the attention line. Other sections
	// leave it false.
	Urgent bool
}

// SectionState is what happened to a section during composition.
type SectionState string

const (
	StateFull    SectionState = "full"
	StatePointer SectionState = "pointer"
	StateDropped SectionState = "dropped"
	StateCut     SectionState = "cut"
)

// SectionStat is the per-section row of a composition's accounting.
type SectionStat struct {
	Key      string       `json:"key"`
	Bytes    int          `json:"bytes"`
	State    SectionState `json:"state"`
	Priority int          `json:"priority"`
}

// Composition is the result of fitting sections to a budget.
type Composition struct {
	Moment Moment `json:"moment"`
	// Text is the delivered block: sections joined by blank lines, the meter last.
	Text string `json:"text"`
	// Meter is the last line of Text, kept separately for the UI.
	Meter string `json:"meter"`
	// Bytes is len(Text); Budget the ceiling it was fitted to.
	Bytes    int           `json:"bytes"`
	Budget   int           `json:"budget"`
	Sections []SectionStat `json:"sections"`
	Degraded []string      `json:"degraded,omitempty"`
	Dropped  []string      `json:"dropped,omitempty"`
	Cut      bool          `json:"cut,omitempty"`
	// Hash fingerprints the delivered section texts (not the meter), so two
	// compositions with the same content compare equal even if their meters
	// differ in timing.
	Hash string `json:"hash"`
	// At is the unix time the composition was made (set by the service).
	At int64 `json:"at,omitempty"`
}

// Empty reports whether nothing was composed (no section had text).
func (c Composition) Empty() bool { return len(c.Sections) == 0 }

// meterReserve is the room kept for the meter line itself when fitting.
const meterReserve = 160

// Compose fits sections to budget bytes. The algorithm:
//
//  1. Sections with empty Text are skipped.
//  2. While over budget, the most droppable full section (highest Priority,
//     then largest) degrades to its Pointer, or is dropped when it has none.
//     Pinned sections are never touched here.
//  3. If only pinned sections remain and the total is still over budget, the
//     LAST section's text is cut on a line boundary and marked, so the model
//     sees the cut instead of silently reading a truncated block.
//  4. The meter is appended as the last line and names every degraded or
//     dropped section.
//
// A budget <= 0 means unlimited: everything ships full, the meter still reports.
func Compose(moment Moment, sections []Section, budget int) Composition {
	type slot struct {
		sec   Section
		state SectionState
		text  string
	}
	var slots []*slot
	for _, s := range sections {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		slots = append(slots, &slot{sec: s, state: StateFull, text: strings.TrimSpace(s.Text)})
	}
	comp := Composition{Moment: moment, Budget: budget}
	if len(slots) == 0 {
		comp.Meter = meterLine(moment, 0, budget, 0, nil, nil, false)
		comp.Text = comp.Meter
		comp.Bytes = len(comp.Text)
		comp.Hash = hashTexts(nil)
		return comp
	}
	total := func() int {
		n := 0
		for i, s := range slots {
			if s.state == StateDropped {
				continue
			}
			if i > 0 {
				n += 2
			}
			n += len(s.text)
		}
		return n + meterReserve
	}
	if budget > 0 {
		for total() > budget {
			var pick *slot
			for _, s := range slots {
				if s.sec.Priority == PriorityPinned || s.state != StateFull {
					continue
				}
				if pick == nil || s.sec.Priority > pick.sec.Priority ||
					(s.sec.Priority == pick.sec.Priority && len(s.text) > len(pick.text)) {
					pick = s
				}
			}
			if pick == nil {
				break
			}
			if p := strings.TrimSpace(pick.sec.Pointer); p != "" {
				pick.state = StatePointer
				pick.text = p
				comp.Degraded = append(comp.Degraded, pick.sec.Key)
			} else {
				pick.state = StateDropped
				pick.text = ""
				comp.Dropped = append(comp.Dropped, pick.sec.Key)
			}
		}
		// A pointer can itself be over budget in a pathological config; the
		// next pass drops pointers too, pinned still untouched.
		for total() > budget {
			var pick *slot
			for _, s := range slots {
				if s.sec.Priority == PriorityPinned || s.state != StatePointer {
					continue
				}
				if pick == nil || s.sec.Priority > pick.sec.Priority {
					pick = s
				}
			}
			if pick == nil {
				break
			}
			pick.state = StateDropped
			pick.text = ""
			comp.Dropped = append(comp.Dropped, pick.sec.Key)
		}
		if total() > budget {
			// Only pinned content left and still over: cut the last live
			// section on a line boundary. Never cut the first one to nothing.
			over := total() - budget
			for i := len(slots) - 1; i >= 0 && over > 0; i-- {
				s := slots[i]
				if s.state == StateDropped {
					continue
				}
				const marker = "\n[…cut to fit the context budget; the rest is available through the tools]"
				keep := len(s.text) - over - len(marker)
				if i == 0 && keep < len(s.text)/2 {
					keep = len(s.text) / 2
				}
				if keep <= 0 {
					if i == 0 {
						break
					}
					over -= len(s.text) + 2
					s.state = StateDropped
					s.text = ""
					comp.Dropped = append(comp.Dropped, s.sec.Key)
					continue
				}
				cutAt := strings.LastIndex(s.text[:keep], "\n")
				if cutAt < keep/2 {
					cutAt = keep
				}
				removed := len(s.text) - cutAt
				s.text = strings.TrimRight(s.text[:cutAt], " \n") + marker
				s.state = StateCut
				comp.Cut = true
				over -= removed - len(marker)
			}
		}
	}
	var parts []string
	var hashParts []string
	for _, s := range slots {
		comp.Sections = append(comp.Sections, SectionStat{Key: s.sec.Key, Bytes: len(s.text), State: s.state, Priority: s.sec.Priority})
		if s.state == StateDropped {
			continue
		}
		parts = append(parts, s.text)
		hashParts = append(hashParts, s.sec.Key+"\x00"+s.text)
	}
	body := strings.Join(parts, "\n\n")
	comp.Meter = meterLine(moment, len(body), budget, len(parts), comp.Degraded, comp.Dropped, comp.Cut)
	if body == "" {
		comp.Text = comp.Meter
	} else {
		comp.Text = body + "\n" + comp.Meter
	}
	comp.Bytes = len(comp.Text)
	comp.Hash = hashTexts(hashParts)
	return comp
}

// meterLine renders the accounting line. It is read by the model and by the
// user, so it is terse and names sections by key.
func meterLine(moment Moment, bytes, budget, sections int, degraded, dropped []string, cut bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[context meter · %s · %s", moment, kb(bytes))
	if budget > 0 {
		fmt.Fprintf(&b, " of %s", kb(budget))
	}
	fmt.Fprintf(&b, " · %d sections", sections)
	if len(degraded) > 0 {
		b.WriteString(" · pointer: " + strings.Join(sortedCopy(degraded), ","))
	}
	if len(dropped) > 0 {
		b.WriteString(" · dropped: " + strings.Join(sortedCopy(dropped), ","))
	}
	if cut {
		b.WriteString(" · cut")
	}
	b.WriteString("]")
	return b.String()
}

func kb(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1fKB", float64(n)/1024)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func hashTexts(parts []string) string {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// HashText fingerprints one text the same way Compose fingerprints sections.
func HashText(s string) string { return hashTexts([]string{s}) }
