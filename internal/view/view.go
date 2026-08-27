// Package view projects large runtime state — long sessions, many-node flow
// runs, crowded boards, whole workspaces — into a compact, deterministic,
// budget-aware representation that is cheap enough to hand an agent AND is shown
// verbatim to the user in the UI.
//
// The design contract (see _Docs/66-VIEW-KATMANI.md):
//
//   - Deterministic first. L0 (counting) and L1 (rule-based signals) are plain
//     Go and never call an LLM, so numbers can never be hallucinated. An optional
//     L2 narrative layer may be folded in later; it must never produce numbers.
//   - No silent truncation. Every View reports how many items it hid (Elided),
//     and the renderer prints it. A summary that quietly drops 173 cards
//     systematically misleads the agent reading it.
//   - Same bytes for agent and human. The UI renders View.Body as-is rather than
//     prettifying it, so a wrong or stale projection is visible to the user.
//
// The output format is a line-oriented compact DSL rather than JSON: JSON's
// repeated keys are pure token waste at this scale.
package view

import (
	"fmt"
	"strings"
	"time"
)

// Kind identifies which entity a view projects.
type Kind string

const (
	KindFlowRun  Kind = "flowrun"
	KindSession  Kind = "session"
	KindBoard    Kind = "board"
	KindSpace    Kind = "workspace"
	KindSchedule Kind = "schedule"
	// KindAgent projects one agent: identity + today's token/cost + how many of
	// its sessions are open + last activity. Its structural children are that
	// agent's sessions.
	KindAgent Kind = "agent"
	// KindBudget wraps billing.RollupOf into a view: today's spend broken down by
	// model. It re-renders the rollup, it never re-prices — the numbers come from
	// the same computation every budget surface uses.
	KindBudget Kind = "budget"
	// KindTools projects the workspace tool surface: the MCP server pool and how
	// many tools are switched off workspace-wide.
	KindTools Kind = "tools"
	// KindCategory is a group node in the Explorer map — a structural bucket
	// (sessions, flows, agents, a board column) that counts its members and lists
	// the top-N as handles, reporting the remainder as Elided. Its ID names the
	// bucket (see the Category* / categoryColumnPrefix constants in category.go).
	KindCategory Kind = "category"
	// KindArtifact projects one saved artifact's METADATA (identity, origin,
	// kind, group, age) — the map leaf for the Artifacts category. The artifact's
	// content itself stays in the Artifacts screen; a drill-down needs to know
	// whether it is worth opening, not its body.
	KindArtifact Kind = "artifact"
	// KindAutomation projects one automation rule: trigger, target, enabled state
	// and fire bookkeeping. Map leaf for the Otomasyonlar category.
	KindAutomation Kind = "automation"
	// KindSkill projects one skill catalog entry: slug, description, access tier
	// and group. Map leaf for the Skill'ler category. The body (the skill's
	// instructions) stays behind use_skill — the map only advertises it.
	KindSkill Kind = "skill"
	// KindInsight projects one insight finding (a retrospective scan result). Map
	// leaf for the İçgörüler category. The projection uses a view-local finding
	// shape (Sources.FindingsSource) because the insight package imports view — a
	// reverse edge would cycle.
	KindInsight Kind = "insight"
	// KindLogs projects the recent process log stream (the global ring buffer) as
	// a leaf node: the tail renders inline, exactly like budget/tools.
	KindLogs Kind = "logs"
	// KindWorkers is a coordinator's live fleet. Unlike the others it is not
	// resolvable through Projector: its input is runtime state, not store state,
	// so the caller builds WorkersInput and calls ProjectWorkers directly. It is
	// also not offered by get_view — coordinators already receive it in their
	// prompt every turn, and list_workers covers the on-demand case.
	KindWorkers Kind = "workers"
)

// Ref addresses one projection target. Sub is an optional drill-down selector
// (a node id for a flow run, a turn id for a session) — empty means the whole
// entity.
type Ref struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
	Sub  string `json:"sub,omitempty"`
}

// String renders a Ref the way handles and the get_view tool spell it:
// "flowrun:RUN7f2" or "flowrun:RUN7f2#fetch-b".
func (r Ref) String() string {
	s := string(r.Kind) + ":" + r.ID
	if r.Sub != "" {
		s += "#" + r.Sub
	}
	return s
}

// Level is the token budget tier the caller asks for. The projection fits the
// tier; it does not merely get truncated to it.
type Level string

const (
	// LevelTiny is a single dense line (~50 tokens). Small and stable enough to
	// be pushed into a volatile prompt suffix without churning the prompt cache.
	LevelTiny Level = "tiny"
	// LevelCard is the default: header plus the signals that matter (~300 tokens).
	LevelCard Level = "card"
	// LevelFull adds per-item detail for drill-down (~1500 tokens).
	LevelFull Level = "full"
)

// ParseLevel resolves a request string, defaulting to LevelCard.
func ParseLevel(s string) Level {
	switch Level(strings.TrimSpace(strings.ToLower(s))) {
	case LevelTiny:
		return LevelTiny
	case LevelFull:
		return LevelFull
	default:
		return LevelCard
	}
}

// Handle is a drill-down pointer emitted alongside a view: the projection stayed
// small, and this is how to get the part that was left out. Label is what the
// renderer prints; Ref is what get_view takes.
type Handle struct {
	Label string `json:"label"`
	Ref   Ref    `json:"ref"`
	// Level is the tier the handle should be opened at ("" = card).
	Level Level `json:"level,omitempty"`
}

// View is one projection result.
type View struct {
	Ref     Ref       `json:"ref"`
	Level   Level     `json:"level"`
	Header  string    `json:"header"`  // always produced, deterministic
	Body    string    `json:"body"`    // graded by Level; may be empty at tiny
	Handles []Handle  `json:"handles"` // drill-down pointers
	AsOf    time.Time `json:"asOf"`    // when the underlying data was read
	// Source identifies the exact revision projected (a seq, a hash, an
	// UpdatedAt). It is both the cache key and the user's way to tell whether
	// two views describe the same state.
	Source string `json:"source"`
	// Elided is how many items the projection deliberately left out. Rendered
	// unconditionally — see the package doc.
	Elided int `json:"elided"`
	// ElidedUnit names what was left out ("kart", "mesaj", "node"). A generic
	// "item" count is ambiguous — 174 hidden messages and 174 hidden cards mean
	// very different things to whoever reads the view. Empty renders as "öğe".
	ElidedUnit string `json:"elidedUnit,omitempty"`
	// ElidedNote overrides the rendered elision sentence. It exists for the one
	// projection that is not a UI surface: the coordinator worker block is an
	// English prompt fragment, and the default Turkish sentence would drop a
	// stray language switch into the middle of it. The structured Elided count
	// is still set either way, so "no silent truncation" holds regardless.
	ElidedNote string `json:"elidedNote,omitempty"`
	// Tokens is an approximate cost of Header+Body (chars/4). Approximate on
	// purpose: it exists so the UI can show which views are expensive, not for
	// billing.
	Tokens int `json:"tokens"`
}

// Text is the full projection as an agent receives it: header, body, then the
// elision notice. This is also exactly what the UI shows.
func (v View) Text() string {
	var b strings.Builder
	b.WriteString(v.Header)
	if v.Body != "" {
		if !strings.HasSuffix(v.Header, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(v.Body)
	}
	if v.Elided > 0 {
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		if v.ElidedNote != "" {
			b.WriteString(v.ElidedNote)
		} else {
			unit := v.ElidedUnit
			if unit == "" {
				unit = "öğe"
			}
			b.WriteString(fmt.Sprintf("…%d %s gizlendi", v.Elided, unit))
			// Name the handle rather than its ref: the label says what opening it
			// gets you, which is what a reader needs to decide. The exact call
			// syntax is appended separately by the get_view tool.
			if len(v.Handles) > 0 {
				b.WriteString("  ↳ " + v.Handles[0].Label)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// finalize fills the derived fields every projection shares.
func (v *View) finalize() {
	v.Tokens = estimateTokens(v.Header + v.Body)
	if v.AsOf.IsZero() {
		v.AsOf = time.Now()
	}
}

// estimateTokens is the usual chars/4 heuristic. Good enough to rank views by
// cost, and honest about being an estimate.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + 3) / 4
}
