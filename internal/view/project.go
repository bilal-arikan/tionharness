package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// decodeState parses a persisted orchestration state snapshot.
func decodeState(data string, st *orchestration.State) error {
	return json.Unmarshal([]byte(data), st)
}

// Store is the narrow slice of the workspace store a projection needs. Keeping
// it an interface (rather than taking *db.DB) is what lets projections be tested
// with hand-built fixtures and keeps this package a leaf.
type Store interface {
	GetFlowRun(ctx context.Context, id string) (db.FlowRun, error)
	GetFlow(ctx context.Context, id string) (db.Flow, error)
}

// Projector resolves a Ref against a store and renders the matching projection.
type Projector struct {
	store Store
}

// NewProjector wires a projector to a store.
func NewProjector(s Store) *Projector { return &Projector{store: s} }

// Project renders the view for ref at the requested level and lens.
//
// Unknown kinds are an error, not an empty view: a caller asking for a
// projection that does not exist has a bug, and silently handing back a blank
// summary would hide it behind plausible-looking output.
func (p *Projector) Project(ctx context.Context, ref Ref, level Level, lens Lens) (View, error) {
	if p == nil || p.store == nil {
		return View{}, fmt.Errorf("view: projector has no store")
	}
	if strings.TrimSpace(ref.ID) == "" {
		return View{}, fmt.Errorf("view: ref has no id")
	}
	switch ref.Kind {
	case KindFlowRun:
		in, err := p.loadFlowRun(ctx, ref.ID)
		if err != nil {
			return View{}, err
		}
		in.Sub = ref.Sub
		return ProjectFlowRun(in, level, lens)
	default:
		return View{}, fmt.Errorf("view: unsupported kind %q", ref.Kind)
	}
}

// loadFlowRun gathers the run, its flow and the parsed graph/state.
//
// A run whose graph or state JSON will not parse is reported as an error rather
// than rendered as an empty chain — a view that shows "0/0 node" for a corrupted
// run is worse than no view at all, because it reads like a healthy empty run.
func (p *Projector) loadFlowRun(ctx context.Context, id string) (FlowRunInput, error) {
	run, err := p.store.GetFlowRun(ctx, id)
	if err != nil {
		return FlowRunInput{}, fmt.Errorf("view: flow run %s: %w", id, err)
	}

	in := FlowRunInput{Run: run}

	// The flow may legitimately be gone (deleted after the run); the run itself
	// is still projectable, it just loses its name.
	if flow, err := p.store.GetFlow(ctx, run.FlowID); err == nil {
		in.Flow = flow
		g, err := orchestration.ParseGraph(flow.Graph)
		if err != nil {
			return FlowRunInput{}, fmt.Errorf("view: flow run %s graph: %w", id, err)
		}
		in.Graph = g
	}

	if s := strings.TrimSpace(run.State); s != "" {
		if err := decodeState(s, &in.State); err != nil {
			return FlowRunInput{}, fmt.Errorf("view: flow run %s state: %w", id, err)
		}
	}
	return in, nil
}
