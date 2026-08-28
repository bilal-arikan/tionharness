package view

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// AgentInput is everything the agent projection reads: the agent record, the
// sessions bound to it, and its usage rollup for today. All three are in-memory
// store reads, so an agent view — like the workspace roll-up — costs no disk I/O.
type AgentInput struct {
	Agent    db.Agent
	Sessions []db.Session
	// Usage is the agent's usage row for the current day (zero-valued when the
	// agent has run nothing today). It is not rendered — spend belongs to the
	// budget view — but its call count fingerprints the projection's Source.
	Usage db.Usage
	// IsDefault reports whether this agent is the workspace's default agent (the
	// one pre-selected for new sessions). It is NOT on db.Agent — it lives in the
	// workspace settings — so the loader wires it in; a caller that cannot know it
	// leaves it false and the projection simply omits the marker.
	IsDefault bool
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

const (
	// agentSessionHandles is how many session handles an agent view lists before
	// the rest are reported as Elided.
	agentSessionHandles = 20
	// agentBlockedToolsShown is how many denied tool names are spelled out before
	// the rest collapse into a "+N" tail. The total count is always rendered.
	agentBlockedToolsShown = 5
	// agentPersonaWidth is the rune budget for each persona line (soul, identity)
	// at LevelFull — enough to recognise the agent, not enough to reproduce it.
	agentPersonaWidth = 120
)

// ProjectAgent renders one agent: who it is, how many of its sessions are open,
// and when it last did anything. It re-uses the same L0/L1 discipline as every
// other projection — every number is counted in Go, nothing is narrated. Spend
// is not part of it: the budget view is the one place that reports money.
func ProjectAgent(in AgentInput, level Level) (View, error) {
	if in.Agent.ID == "" {
		return View{}, fmt.Errorf("agent input has no agent")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindAgent, ID: in.Agent.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d@%d", len(in.Sessions), in.Usage.Calls),
	}

	st := computeAgentStats(in, now)
	// Today's tokens and cost are deliberately absent: the budget view owns spend,
	// and an agent read should not put a money figure in front of the reader.
	v.Header = fmt.Sprintf("AGENT:%s %q · %s · %d oturum (%d aktif) · son etkinlik %s",
		in.Agent.ID, clip(agentName(in.Agent), 60), modelLabel(in.Agent),
		st.Total, st.Active, agentLastActivity(st.LastActivity, now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	for _, s := range agentSignals(st) {
		l.add("%s", s)
	}
	for _, s := range agentConfigLines(in, level) {
		l.add("%s", s)
	}

	v.Handles, v.Elided = agentSessionHandleList(in.Sessions, now)
	v.ElidedUnit = "oturum"
	v.Body = l.String()
	v.finalize()
	return v, nil
}

// agentStats is the L0 counting pass over an agent's sessions.
type agentStats struct {
	Total        int
	Active       int
	Coordinators int
	Stuck        []db.Session
	LastActivity int64
}

func computeAgentStats(in AgentInput, now time.Time) agentStats {
	var st agentStats
	activeCutoff := now.Add(-wsActiveWindow).Unix()
	for _, s := range in.Sessions {
		if s.State == "archived" {
			continue
		}
		st.Total++
		if s.UpdatedAt >= activeCutoff {
			st.Active++
		}
		if s.StuckTurns > 0 {
			st.Stuck = append(st.Stuck, s)
		}
		if s.IsCoordinator() {
			st.Coordinators++
		}
		if s.UpdatedAt > st.LastActivity {
			st.LastActivity = s.UpdatedAt
		}
	}
	sort.Slice(st.Stuck, func(i, j int) bool {
		return st.Stuck[i].UpdatedAt > st.Stuck[j].UpdatedAt
	})
	return st
}

// agentSignals is the L1 layer: what about this agent deserves attention.
func agentSignals(st agentStats) []string {
	var out []string
	if n := len(st.Stuck); n > 0 {
		out = append(out, fmt.Sprintf("⚠ %d oturum takılmış (StuckTurns>0): %s",
			n, sessionNames(st.Stuck, wsSignalNames)))
	}
	if st.Coordinators > 0 {
		out = append(out, fmt.Sprintf("⇵ %d koordinatör oturumu", st.Coordinators))
	}
	return out
}

// agentConfigLines is what the agent IS — its provider binding, its behavioural
// flags, its tool restrictions and (at the full tier) its persona — as opposed to
// what its sessions are doing.
//
// Every line is omitted when the underlying field is unset. A rendered
// "model: -" would assert the agent has no model, which is a different fact from
// "this projection does not know one".
func agentConfigLines(in AgentInput, level Level) []string {
	a := in.Agent
	var out []string

	// Provider and model are NOT repeated here: the header already carries them as
	// `provider/model` (see modelLabel). Printing them a second time made the card
	// read as if the agent had two bindings.
	if t := thinkingLabel(a.ThinkingLevel); t != "" {
		out = append(out, "düşünme: "+t)
	}

	// The permission mode gates whether this agent's tool calls are auto-approved,
	// asked about, or refused outright — the difference between an agent that can
	// edit files and one that cannot. "" and "auto" are the same (documented)
	// default, so neither renders: a line that only ever repeats the default is
	// noise on every card.
	if m := strings.TrimSpace(a.PermissionMode); m != "" && m != "auto" {
		out = append(out, "izin: "+m)
	}

	var flags []string
	// A disabled agent never runs at all, which reframes every other line on the
	// card (its "0 aktif oturum" is a consequence, not a coincidence).
	if a.Disabled {
		flags = append(flags, "devre dışı")
	}
	if a.CoordinatorMode {
		flags = append(flags, "koordinatör")
	}
	if in.IsDefault {
		flags = append(flags, "varsayılan")
	}
	if len(flags) > 0 {
		out = append(out, strings.Join(flags, " · "))
	}

	if s := blockedToolsLine(a.BlockedTools); s != "" {
		out = append(out, s)
	}

	// The persona is the agent's most expensive text and only earns its tokens at
	// the full tier; a card keeps the binding and the flags.
	if level == LevelFull {
		if s := strings.TrimSpace(a.Soul); s != "" {
			out = append(out, "ruh: "+clip(s, agentPersonaWidth))
		}
		if s := strings.TrimSpace(a.Identity); s != "" {
			out = append(out, "kimlik: "+clip(s, agentPersonaWidth))
		}
	}
	return out
}

// thinkingLabel renders the extended-reasoning level, treating both "" and "off"
// as "not requested" (the field's two spellings of the same state) so the line is
// omitted rather than rendered as a setting the user never made.
func thinkingLabel(level string) string {
	l := strings.TrimSpace(level)
	if l == "" || l == "off" {
		return ""
	}
	return l
}

// blockedToolsLine renders the per-agent tool denylist. The COUNT is always exact
// and comes first: a truncated list that did not say how much it dropped would
// read as the whole restriction set. A denylist that will not parse is reported
// as unreadable rather than as "nothing blocked" — the opposite fact.
func blockedToolsLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return ""
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return "yasaklı araçlar: (liste okunamadı)"
	}
	kept := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			kept = append(kept, n)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	shown, extra := kept, 0
	if len(shown) > agentBlockedToolsShown {
		extra = len(shown) - agentBlockedToolsShown
		shown = shown[:agentBlockedToolsShown]
	}
	line := fmt.Sprintf("yasaklı araçlar (%d): %s", len(kept), strings.Join(shown, ", "))
	if extra > 0 {
		line += fmt.Sprintf(" … +%d", extra)
	}
	return line
}

// agentSessionHandleList returns up to agentSessionHandles session handles (most
// recently active first) and how many sessions were left out. Archived sessions
// are excluded — they are not part of the agent's live surface.
func agentSessionHandleList(sessions []db.Session, now time.Time) ([]Handle, int) {
	live := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.State == "archived" {
			continue
		}
		live = append(live, s)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].UpdatedAt > live[j].UpdatedAt })

	elided := 0
	if len(live) > agentSessionHandles {
		elided = len(live) - agentSessionHandles
		live = live[:agentSessionHandles]
	}
	hs := make([]Handle, 0, len(live))
	for _, s := range live {
		hs = append(hs, Handle{
			Label: "session:" + s.ID + " " + clip(orDash(s.Title), 40),
			Ref:   Ref{Kind: KindSession, ID: s.ID},
			Level: LevelCard,
		})
	}
	return hs, elided
}

// agentName falls back to the id-derived placeholder when an agent has no name.
func agentName(a db.Agent) string {
	if a.Name == "" {
		return "(isimsiz)"
	}
	return a.Name
}

// modelLabel renders the agent's provider/model binding, collapsing an empty
// model (e.g. claude-cli's session default) to just the provider.
func modelLabel(a db.Agent) string {
	switch {
	case a.Provider == "" && a.Model == "":
		return "-"
	case a.Model == "":
		return a.Provider
	case a.Provider == "":
		return a.Model
	default:
		return a.Provider + "/" + a.Model
	}
}

// agentLastActivity renders the most recent session activity, or "-" when the
// agent has no (non-archived) sessions at all.
func agentLastActivity(ts int64, now time.Time) string {
	if ts <= 0 {
		return "-"
	}
	return age(tsSec(ts), now) + " önce"
}
