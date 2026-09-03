// Package trajectory holds the pure Rota graph logic — node ids, phase
// activation, recipe seeding, plan application, status derivation, rendering,
// the deterministic run summary, transition diffing and recipe statistics.
// It depends on db (the Trajectory model) and skills (recipe specs) only; the
// runtime binder in internal/agent owns persistence, queues and hooks.
// Extracted from internal/agent on 2026-09-03 (_Docs/81, step 2).
package trajectory

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// Trajectory graph helpers (Rota F1a).
//
// Pure functions over db.Trajectory: node id conventions, seeding from a
// recipe or an agent plan, phase activation, status derivation and the compact
// text rendering the trajectory tool and the situation block share. Nothing
// here touches the store — the binder (trajectory_binder.go) loads, applies one
// of these, and persists under the store's per-root lock.

// Node id prefixes. One namespace per kind so an observed session and a
// declared phase can never collide and a reader can tell the kind from the id.
const (
	PhasePrefix     = "p:"
	SessionPrefix   = "s:"
	FlowRunPrefix   = "r:"
	GatePrefix      = "g:"
	AutomationPfx   = "a:"
	OptimizerNodeID = "o:optimizer"
	LaneMetaKey     = "lanes" // Meta: next free lane (0 = root's lane)
)

func PhaseNodeID(phaseID string) string     { return PhasePrefix + phaseID }
func SessionNodeID(sessionID string) string { return SessionPrefix + sessionID }
func FlowRunNodeID(runID string) string     { return FlowRunPrefix + runID }
func GateNodeID(askID string) string        { return GatePrefix + askID }

// NodeIndex returns the index of node id in t.Nodes, -1 when absent.
func NodeIndex(t *db.Trajectory, id string) int {
	for i := range t.Nodes {
		if t.Nodes[i].ID == id {
			return i
		}
	}
	return -1
}

// NodePtr returns a pointer to the node with id, nil when absent.
func NodePtr(t *db.Trajectory, id string) *db.TrajectoryNode {
	if i := NodeIndex(t, id); i >= 0 {
		return &t.Nodes[i]
	}
	return nil
}

// AddNode appends n unless a node with the same id exists; reports whether
// it was added. An observer that fires twice for one fact (a retried spawn, a
// re-emitted run status) must not duplicate the vertex.
func AddNode(t *db.Trajectory, n db.TrajectoryNode) bool {
	if NodeIndex(t, n.ID) >= 0 {
		return false
	}
	t.Nodes = append(t.Nodes, n)
	return true
}

// AddEdge appends the edge unless an identical one exists.
func AddEdge(t *db.Trajectory, from, to, kind, origin string) bool {
	for _, e := range t.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return false
		}
	}
	t.Edges = append(t.Edges, db.TrajectoryEdge{From: from, To: to, Kind: kind, Origin: origin})
	return true
}

// NextLane hands out the next drawing row and bumps the counter kept in
// Meta. Lane 0 belongs to the root session (and the declared phase row), so the
// counter starts at 1.
func NextLane(t *db.Trajectory) int {
	if t.Meta == nil {
		t.Meta = map[string]string{}
	}
	next, _ := strconv.Atoi(t.Meta[LaneMetaKey])
	if next < 1 {
		next = 1
	}
	t.Meta[LaneMetaKey] = strconv.Itoa(next + 1)
	return next
}

// Phases returns the declared phase nodes in graph order.
func Phases(t *db.Trajectory) []db.TrajectoryNode {
	var out []db.TrajectoryNode
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodePhase {
			out = append(out, n)
		}
	}
	return out
}

// ActivePhase returns the id of the phase currently active ("" when none).
func ActivePhase(t *db.Trajectory) string {
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodePhase && n.State == db.TrajStateActive {
			return n.ID
		}
	}
	return ""
}

// ActivatePhase makes phase node id the active one: the previously active
// phase (if another) is closed as done, the target's StartMs is stamped once.
// Returns an error when id is not a phase node.
func ActivatePhase(t *db.Trajectory, id string, nowMs int64) error {
	target := NodePtr(t, id)
	if target == nil || target.Kind != db.TrajNodePhase {
		return fmt.Errorf("phase %q is not declared on this trajectory", strings.TrimPrefix(id, PhasePrefix))
	}
	for i := range t.Nodes {
		n := &t.Nodes[i]
		if n.Kind == db.TrajNodePhase && n.State == db.TrajStateActive && n.ID != id {
			n.State = db.TrajStateDone
			if n.EndMs == 0 {
				n.EndMs = nowMs
			}
		}
	}
	if target.State != db.TrajStateActive {
		target.State = db.TrajStateActive
		target.EndMs = 0
		target.Reason = ""
	}
	if target.StartMs == 0 {
		target.StartMs = nowMs
	}
	return nil
}

// AutoStartPhase activates the first pending phase when the trajectory has
// declared phases but none is active yet — the moment the first worker spawns
// the plan is evidently under way even if the agent never called the tool.
func AutoStartPhase(t *db.Trajectory, nowMs int64) {
	if ActivePhase(t) != "" {
		return
	}
	for _, p := range Phases(t) {
		if p.State == db.TrajStatePending {
			_ = ActivatePhase(t, p.ID, nowMs)
			return
		}
	}
}

// OpenGate reports whether an observed gate (a durable ask) is still open.
func OpenGate(t *db.Trajectory) bool {
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodeGate && n.State == db.TrajStateActive {
			return true
		}
	}
	return false
}

// DeriveStatus recomputes Trajectory.Status from the graph. Terminal
// statuses set explicitly (done/failed/abandoned) are kept: the derivation only
// moves a live trajectory between planned / running / waiting, and closes it
// when every required declared phase has finished.
func DeriveStatus(t *db.Trajectory) {
	if t.IsTerminal() {
		return
	}
	if OpenGate(t) {
		t.Status = db.TrajStatusWaiting
		return
	}
	phases := Phases(t)
	if len(phases) > 0 {
		allClosed := true
		for _, p := range phases {
			switch p.State {
			case db.TrajStateFailed:
				t.Status = db.TrajStatusFailed
				return
			case db.TrajStateActive:
				t.Status = db.TrajStatusRunning
				return
			case db.TrajStateDone, db.TrajStateSkipped:
			default:
				if !p.Optional {
					allClosed = false
				}
			}
		}
		if allClosed {
			t.Status = db.TrajStatusDone
			return
		}
	}
	for _, n := range t.Nodes {
		if (n.Kind == db.TrajNodeSession || n.Kind == db.TrajNodeFlowRun) && n.State == db.TrajStateActive && n.Lane != 0 {
			t.Status = db.TrajStatusRunning
			return
		}
	}
	if len(phases) > 0 {
		t.Status = db.TrajStatusPlanned
		return
	}
	// No plan at all: once anything was observed the trajectory is running, a
	// bare root node alone is still "planned".
	if len(t.Nodes) > 1 {
		t.Status = db.TrajStatusRunning
	} else {
		t.Status = db.TrajStatusPlanned
	}
}

// PhaseFromSpec builds a declared phase node from a recipe phase.
func PhaseFromSpec(p skills.PhaseSpec) db.TrajectoryNode {
	n := db.TrajectoryNode{
		ID: PhaseNodeID(p.ID), Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared,
		Label: p.Label, Profile: p.Profile, Optional: p.Optional, State: db.TrajStatePending,
	}
	if n.Label == "" {
		n.Label = p.ID
	}
	if p.Gate != nil {
		n.Gate = &db.TrajectoryGate{Kind: p.Gate.Kind, Value: p.Gate.Value}
	}
	return n
}

// RootNode is the observed node for the root session (lane 0, active).
func RootNode(root db.Session) db.TrajectoryNode {
	return db.TrajectoryNode{
		ID: SessionNodeID(root.ID), Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved,
		Label: root.Title, RefKind: "session", RefID: root.ID, Lane: 0,
		State: db.TrajStateActive, StartMs: root.CreatedAt * 1000,
	}
}

// Seed builds the initial graph for a root session: its own node plus, when
// a recipe with a structured plan is given, the declared phases (next-chained),
// per-phase and trajectory-wide watcher automations as ghosts, and the optimizer.
// ref is the versioned recipe ref recorded as TemplateRef ("" = agent-planned).
func Seed(root db.Session, spec *skills.RecipeSpec, ref string) db.Trajectory {
	t := db.Trajectory{
		RootSessionID: root.ID,
		TemplateRef:   ref,
		Status:        db.TrajStatusPlanned,
		Nodes:         []db.TrajectoryNode{RootNode(root)},
		Edges:         []db.TrajectoryEdge{},
		Meta:          map[string]string{LaneMetaKey: "1"},
	}
	if spec == nil {
		return t
	}
	ApplyRecipe(&t, spec)
	return t
}

// ApplyRecipe adds a recipe's declared nodes to a graph that has none yet.
func ApplyRecipe(t *db.Trajectory, spec *skills.RecipeSpec) {
	prev := ""
	for _, p := range spec.Phases {
		n := PhaseFromSpec(p)
		if !AddNode(t, n) {
			continue
		}
		if prev != "" {
			AddEdge(t, prev, n.ID, db.TrajEdgeNext, db.TrajOriginDeclared)
		}
		prev = n.ID
		for _, w := range p.Watchers {
			AddWatcher(t, w, n.ID)
		}
	}
	for _, w := range spec.Watchers {
		AddWatcher(t, w, "")
	}
	if opt := strings.TrimSpace(spec.Optimizer); opt != "" {
		AddNode(t, db.TrajectoryNode{
			ID: OptimizerNodeID, Kind: db.TrajNodeOptimizer, Origin: db.TrajOriginDeclared,
			Label: opt, RefKind: "agent", RefID: opt, State: db.TrajStateGhost,
		})
	}
}

// AddWatcher adds a ghost automation node for a recipe watcher, hung under
// phaseID ("" = trajectory-wide). The watcher string is an automation id or
// name; it is recorded as RefID as written — F2 resolves and fires it.
func AddWatcher(t *db.Trajectory, watcher, phaseID string) {
	watcher = strings.TrimSpace(watcher)
	if watcher == "" {
		return
	}
	id := AutomationPfx + watcher
	if phaseID != "" {
		id += "@" + strings.TrimPrefix(phaseID, PhasePrefix)
	}
	AddNode(t, db.TrajectoryNode{
		ID: id, Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared,
		Label: watcher, RefKind: "automation", RefID: watcher, PhaseID: phaseID,
		State: db.TrajStateGhost,
	})
}

// PlanPhase is one phase of an agent-declared plan (trajectory tool,
// action "plan"). Mirrors skills.PhaseSpec minus the recipe-only knobs.
type PlanPhase struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	GateKind string `json:"gateKind,omitempty"`
	GateVal  string `json:"gateValue,omitempty"`
}

// ApplyPlan replaces the declared phase list with plan. Phases that already
// exist keep their state; a phase that is active or done cannot be dropped (the
// plan may only grow past what has happened); pending phases not in the plan
// are removed together with their edges. The next-chain is rebuilt in plan
// order.
func ApplyPlan(t *db.Trajectory, plan []PlanPhase) error {
	if len(plan) == 0 {
		return fmt.Errorf("a plan needs at least one phase")
	}
	keep := map[string]bool{}
	for i, p := range plan {
		p.ID = strings.TrimSpace(p.ID)
		if !skills.ValidPhaseID(p.ID) {
			return fmt.Errorf("phases[%d]: id %q must be lowercase [a-z0-9_-], 1..64 chars", i, p.ID)
		}
		if keep[PhaseNodeID(p.ID)] {
			return fmt.Errorf("phases[%d]: duplicate id %q", i, p.ID)
		}
		keep[PhaseNodeID(p.ID)] = true
		if p.GateKind != "" {
			known := false
			for _, k := range skills.GateKinds {
				if p.GateKind == k {
					known = true
				}
			}
			if !known {
				return fmt.Errorf("phase %q: gate kind %q must be one of %s", p.ID, p.GateKind, strings.Join(skills.GateKinds, "|"))
			}
		}
	}
	for _, n := range Phases(t) {
		if !keep[n.ID] && n.State != db.TrajStatePending && n.State != db.TrajStateGhost {
			return fmt.Errorf("phase %q is %s and cannot be dropped from the plan", strings.TrimPrefix(n.ID, PhasePrefix), n.State)
		}
	}
	// Drop pending phases not in the plan and every edge touching them, and
	// every declared next-edge (rebuilt below).
	var nodes []db.TrajectoryNode
	dropped := map[string]bool{}
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodePhase && !keep[n.ID] {
			dropped[n.ID] = true
			continue
		}
		nodes = append(nodes, n)
	}
	var edges []db.TrajectoryEdge
	for _, e := range t.Edges {
		if dropped[e.From] || dropped[e.To] {
			continue
		}
		if e.Kind == db.TrajEdgeNext && e.Origin == db.TrajOriginDeclared {
			continue
		}
		edges = append(edges, e)
	}
	t.Nodes, t.Edges = nodes, edges
	for i := range t.Nodes {
		if dropped[t.Nodes[i].PhaseID] {
			t.Nodes[i].PhaseID = ""
		}
	}
	prev := ""
	for _, p := range plan {
		id := PhaseNodeID(strings.TrimSpace(p.ID))
		n := NodePtr(t, id)
		if n == nil {
			t.Nodes = append(t.Nodes, db.TrajectoryNode{
				ID: id, Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStatePending,
			})
			n = NodePtr(t, id)
		}
		n.Label = strings.TrimSpace(p.Label)
		if n.Label == "" {
			n.Label = strings.TrimSpace(p.ID)
		}
		n.Profile = strings.TrimSpace(p.Profile)
		n.Optional = p.Optional
		if p.GateKind != "" {
			n.Gate = &db.TrajectoryGate{Kind: p.GateKind, Value: strings.TrimSpace(p.GateVal)}
		} else {
			n.Gate = nil
		}
		if prev != "" {
			AddEdge(t, prev, id, db.TrajEdgeNext, db.TrajOriginDeclared)
		}
		prev = id
	}
	// Phases are drawn in plan order: move them to the front, in order, so a
	// reader that walks Nodes sees the plan before the observations.
	order := map[string]int{}
	for i, p := range plan {
		order[PhaseNodeID(strings.TrimSpace(p.ID))] = i
	}
	sort.SliceStable(t.Nodes, func(i, j int) bool {
		pi, iok := order[t.Nodes[i].ID]
		pj, jok := order[t.Nodes[j].ID]
		if iok != jok {
			return iok
		}
		return iok && pi < pj
	})
	return nil
}

// SetPhaseState applies a phase transition requested by the agent. active
// goes through ActivatePhase (closing the previous one); done / skipped /
// failed stamp EndMs and the reason.
func SetPhaseState(t *db.Trajectory, phaseID, state, reason string, nowMs int64) error {
	id := PhaseNodeID(strings.TrimSpace(phaseID))
	switch state {
	case db.TrajStateActive:
		return ActivatePhase(t, id, nowMs)
	case db.TrajStateDone, db.TrajStateSkipped, db.TrajStateFailed:
		n := NodePtr(t, id)
		if n == nil || n.Kind != db.TrajNodePhase {
			return fmt.Errorf("phase %q is not declared on this trajectory", strings.TrimSpace(phaseID))
		}
		n.State = state
		n.Reason = strings.TrimSpace(reason)
		if n.StartMs == 0 && state == db.TrajStateDone {
			n.StartMs = nowMs
		}
		n.EndMs = nowMs
		return nil
	default:
		return fmt.Errorf("state must be one of active|done|skipped|failed, got %q", state)
	}
}

// PhaseGlyph is the one-character state marker used by the text renders.
func PhaseGlyph(state string) string {
	switch state {
	case db.TrajStateActive:
		return "●"
	case db.TrajStateDone:
		return "✓"
	case db.TrajStateFailed:
		return "✗"
	case db.TrajStateSkipped:
		return "↷"
	default:
		return "○"
	}
}

// PhaseLine renders "plan ✓ → code ● (coder) → review ○ [gate verdict …]".
func PhaseLine(t *db.Trajectory) string {
	phases := Phases(t)
	if len(phases) == 0 {
		return ""
	}
	parts := make([]string, 0, len(phases))
	for _, p := range phases {
		s := strings.TrimPrefix(p.ID, PhasePrefix) + " " + PhaseGlyph(p.State)
		var tags []string
		if p.Profile != "" {
			tags = append(tags, p.Profile)
		}
		if p.Optional {
			tags = append(tags, "optional")
		}
		if p.Gate != nil {
			g := "gate " + p.Gate.Kind
			if p.Gate.Value != "" {
				g += " " + strconv.Quote(p.Gate.Value)
			}
			tags = append(tags, g)
		}
		if len(tags) > 0 {
			s += " (" + strings.Join(tags, ", ") + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " → ")
}

// Render is the full text view the trajectory tool returns for "get".
func Render(t *db.Trajectory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Trajectory %s — status %s, revision %d", t.ID, t.Status, t.Revision)
	if t.TemplateRef != "" {
		fmt.Fprintf(&b, ", recipe %s", t.TemplateRef)
	}
	b.WriteString("\n")
	if line := PhaseLine(t); line != "" {
		b.WriteString("Phases: " + line + "\n")
	} else {
		b.WriteString("Phases: none declared (call trajectory{action:\"plan\"} to announce them)\n")
	}
	byPhase := map[string][]db.TrajectoryNode{}
	var gates []db.TrajectoryNode
	for _, n := range t.Nodes {
		switch n.Kind {
		case db.TrajNodeSession, db.TrajNodeFlowRun:
			if n.Lane == 0 {
				continue // the root itself
			}
			byPhase[n.PhaseID] = append(byPhase[n.PhaseID], n)
		case db.TrajNodeGate:
			if n.State == db.TrajStateActive {
				gates = append(gates, n)
			}
		}
	}
	writeGroup := func(title string, list []db.TrajectoryNode) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s:\n", title)
		for _, n := range list {
			kind := "worker"
			if n.Kind == db.TrajNodeFlowRun {
				kind = "flow run"
			}
			fmt.Fprintf(&b, "- %s %s [%s]", kind, n.RefID, n.State)
			if n.Label != "" {
				fmt.Fprintf(&b, " %s", n.Label)
			}
			if n.Reason != "" {
				fmt.Fprintf(&b, " — %s", n.Reason)
			}
			b.WriteString("\n")
		}
	}
	for _, p := range Phases(t) {
		writeGroup("Under "+strings.TrimPrefix(p.ID, PhasePrefix), byPhase[p.ID])
	}
	writeGroup("Unassigned to a phase", byPhase[""])
	if len(gates) > 0 {
		b.WriteString("Open gates (waiting on a human):\n")
		for _, g := range gates {
			fmt.Fprintf(&b, "- %s %s\n", g.RefID, g.Label)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
