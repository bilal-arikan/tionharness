package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TrajectoryInput is one trajectory ("Rota", db.Trajectory) to project.
type TrajectoryInput struct {
	Trajectory db.Trajectory
	// Now is the clock used for age stamps. Zero means time.Now().
	Now time.Time
}

// trajectoryTopN bounds the bound-node handles a card/full view lists; the
// remainder is reported as Elided (unit "düğüm").
const trajectoryTopN = 12

// ProjectTrajectory renders a trajectory: the declared phases with their
// states, what actually happened under them (sessions, flow runs, automation
// fires) and how much of the declared plan is still a ghost. The same text an
// agent gets from get_view is what the Rota screen's side panel shows.
func ProjectTrajectory(in TrajectoryInput, level Level) (View, error) {
	t := in.Trajectory
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if t.ID == "" {
		return View{}, fmt.Errorf("view: trajectory has no id")
	}
	v := View{
		Ref:    Ref{Kind: KindTrajectory, ID: t.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("rev%d@%d", t.Revision, t.UpdatedAt),
	}
	template := t.TemplateRef
	if template == "" {
		template = "plansız"
	}
	v.Header = fmt.Sprintf("TRAJECTORY · %s · root %s · %s · %s · rev %d · asOf %s",
		t.ID, t.RootSessionID, template, t.Status, t.Revision, hhmmss(now))
	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var phases, sessions, runs, autos, gates []db.TrajectoryNode
	ghosts := 0
	for _, n := range t.Nodes {
		switch n.Kind {
		case db.TrajNodePhase:
			phases = append(phases, n)
		case db.TrajNodeSession:
			sessions = append(sessions, n)
		case db.TrajNodeFlowRun:
			runs = append(runs, n)
		case db.TrajNodeAutomation:
			autos = append(autos, n)
		case db.TrajNodeGate:
			gates = append(gates, n)
		}
		if n.State == db.TrajStateGhost {
			ghosts++
		}
	}

	var l lines
	if len(phases) == 0 {
		l.add("fazlar: (ilan edilmemiş)")
	} else {
		parts := make([]string, 0, len(phases))
		for _, p := range phases {
			parts = append(parts, phaseGlyph(p.State)+" "+phaseLabel(p))
		}
		l.add("fazlar: %s", strings.Join(parts, " → "))
	}
	l.add("gözlenen: %d oturum · %d akış koşusu · %d otomasyon · %d kapı · %d hayalet",
		len(sessions), len(runs), len(autos), len(gates), ghosts)
	declared, observed := 0, 0
	for _, n := range t.Nodes {
		if n.Origin == db.TrajOriginDeclared {
			declared++
		} else {
			observed++
		}
	}
	l.add("köken: %d ilan · %d gözlem · %d kenar", declared, observed, len(t.Edges))
	if s := t.Summary; s != nil {
		line := fmt.Sprintf("özet: %s · %d token", dur(time.Duration(s.DurationSec)*time.Second), s.Tokens)
		if s.CostUSD > 0 {
			line += fmt.Sprintf(" · $%.2f", s.CostUSD)
			if !s.Priced {
				line += "~"
			}
		}
		line += fmt.Sprintf(" · %d worker", s.Sessions)
		if s.FailedSess > 0 {
			line += fmt.Sprintf(" (%d ✗)", s.FailedSess)
		}
		if s.Gates > 0 {
			line += fmt.Sprintf(" · %d kapı %s", s.Gates, dur(time.Duration(s.GateWaitSec)*time.Second))
		}
		if s.Unannounced > 0 {
			line += fmt.Sprintf(" · %d plansız", s.Unannounced)
		}
		if len(s.GhostPhases) > 0 {
			line += " · hayalet faz: " + strings.Join(s.GhostPhases, ",")
		}
		if len(s.UnfiredWatchers) > 0 {
			line += " · sessiz izleyici: " + strings.Join(s.UnfiredWatchers, ",")
		}
		l.add("%s", line)
	}

	if level == LevelFull {
		for _, p := range phases {
			line := fmt.Sprintf("  %s %s [%s]", phaseGlyph(p.State), phaseLabel(p), p.State)
			if p.Gate != nil {
				line += " kapı:" + p.Gate.Kind
				if p.Gate.Value != "" {
					line += "=" + clip(p.Gate.Value, 24)
				}
			}
			if p.Reason != "" {
				line += " — " + clip(p.Reason, 60)
			}
			if p.StartMs > 0 && p.EndMs >= p.StartMs {
				line += " · " + dur(time.Duration(p.EndMs-p.StartMs)*time.Millisecond)
			}
			l.add("%s", line)
		}
		for _, n := range t.Nodes {
			if n.Kind == db.TrajNodePhase {
				continue
			}
			under := ""
			if n.PhaseID != "" {
				under = " ⊂ " + n.PhaseID
			}
			l.add("  %s %s %s [%s]%s", nodeGlyph(n.Kind), n.Kind, orDash(n.RefID), n.State, under)
		}
	}
	v.Body = l.String()

	// Handles: the bound entities, so a reader can descend into the session, run
	// or rule behind a node — exactly what the Rota screen's double-click does.
	handles := make([]Handle, 0, len(t.Nodes))
	for _, n := range t.Nodes {
		if n.RefKind == "" || n.RefID == "" {
			continue
		}
		handles = append(handles, Handle{
			Label: nodeGlyph(n.Kind) + " " + n.RefKind + ":" + n.RefID + " " + clip(orDash(n.Label), 40),
			Ref:   Ref{Kind: Kind(n.RefKind), ID: n.RefID},
			Level: LevelCard,
		})
	}
	if len(handles) > trajectoryTopN {
		v.Elided = len(handles) - trajectoryTopN
		v.ElidedUnit = "düğüm"
		handles = handles[:trajectoryTopN]
	}
	v.Handles = handles
	v.finalize()
	return v, nil
}

func phaseLabel(p db.TrajectoryNode) string {
	if p.Label != "" {
		return p.Label
	}
	return strings.TrimPrefix(p.ID, "p:")
}

func phaseGlyph(state string) string {
	switch state {
	case db.TrajStateDone:
		return "✓"
	case db.TrajStateActive:
		return "●"
	case db.TrajStateFailed:
		return "✗"
	case db.TrajStateSkipped:
		return "↷"
	case db.TrajStateGhost:
		return "◌"
	}
	return "○"
}

func nodeGlyph(kind string) string {
	switch kind {
	case db.TrajNodeSession:
		return "⌘"
	case db.TrajNodeFlowRun:
		return "⇶"
	case db.TrajNodeAutomation:
		return "⚡"
	case db.TrajNodeGate:
		return "⛩"
	case db.TrajNodeOptimizer:
		return "✦"
	}
	return "·"
}

// loadTrajectory reads a trajectory by id.
func (p *Projector) loadTrajectory(ctx contextT, id string) (TrajectoryInput, error) {
	t, err := p.store.GetTrajectory(ctx, id)
	if err != nil {
		return TrajectoryInput{}, fmt.Errorf("view: trajectory %s: %w", id, err)
	}
	return TrajectoryInput{Trajectory: t}, nil
}

// trajectoryHandleFor returns the session's trajectory handle when one exists
// (nil otherwise) — the structural link from a root session to its Rota.
func (p *Projector) trajectoryHandleFor(ctx contextT, sessionID string) *Handle {
	t, err := p.store.GetTrajectoryByRoot(ctx, sessionID)
	if err != nil {
		return nil
	}
	h := Handle{
		Label: "rota:" + t.ID + " " + clip(orDash(t.TemplateRef), 30) + " [" + t.Status + "]",
		Ref:   Ref{Kind: KindTrajectory, ID: t.ID},
		Level: LevelCard,
	}
	return &h
}

// trajectoryChildren lists the entities bound under a trajectory's nodes.
func (p *Projector) trajectoryChildren(ctx contextT, id string) ([]Handle, error) {
	t, err := p.store.GetTrajectory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("view: trajectory %s: %w", id, err)
	}
	hs := make([]Handle, 0, len(t.Nodes))
	for _, n := range t.Nodes {
		if n.RefKind == "" || n.RefID == "" {
			continue
		}
		hs = append(hs, Handle{
			Label: nodeGlyph(n.Kind) + " " + n.RefKind + ":" + n.RefID + " " + clip(orDash(n.Label), 40),
			Ref:   Ref{Kind: Kind(n.RefKind), ID: n.RefID},
			Level: LevelCard,
		})
	}
	return hs, nil
}
