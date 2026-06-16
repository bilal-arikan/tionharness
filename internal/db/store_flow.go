package db

import (
	"context"
	"sort"
)

// ---- Flows ----

func (d *DB) persistFlowLocked(f Flow) error {
	d.flows[f.ID] = f
	return atomicWriteJSON(d.dir(dirFlows, f.ID+".json"), f)
}

// CreateFlow inserts a new flow and returns the stored row.
func (d *DB) CreateFlow(ctx context.Context, f Flow) (Flow, error) {
	f.ID = newID()
	f.CreatedAt = now()
	f.UpdatedAt = f.CreatedAt
	if f.Graph == "" {
		f.Graph = "{}"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return f, d.persistFlowLocked(f)
}

// GetFlow loads a flow by id.
func (d *DB) GetFlow(ctx context.Context, id string) (Flow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	f, ok := d.flows[id]
	if !ok {
		return Flow{}, ErrNotFound
	}
	return f, nil
}

// ListFlows returns all flows, newest first.
func (d *DB) ListFlows(ctx context.Context) ([]Flow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Flow, 0, len(d.flows))
	for _, f := range d.flows {
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// UpdateFlow edits a flow's name/description/graph.
func (d *DB) UpdateFlow(ctx context.Context, f Flow) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.flows[f.ID]
	if !ok {
		return ErrNotFound
	}
	cur.Name = f.Name
	cur.Description = f.Description
	cur.Graph = f.Graph
	cur.UpdatedAt = now()
	return d.persistFlowLocked(cur)
}

// DeleteFlow removes a flow and all of its runs.
func (d *DB) DeleteFlow(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.flows[id]; !ok {
		return ErrNotFound
	}
	delete(d.flows, id)
	if err := removeFile(d.dir(dirFlows, id+".json")); err != nil {
		return err
	}
	for rid, r := range d.flowRuns {
		if r.FlowID == id {
			delete(d.flowRuns, rid)
			_ = removeFile(d.dir(dirFlowRuns, rid+".json"))
		}
	}
	return nil
}

// ---- Flow runs ----

func (d *DB) persistFlowRunLocked(r FlowRun) error {
	d.flowRuns[r.ID] = r
	return atomicWriteJSON(d.dir(dirFlowRuns, r.ID+".json"), r)
}

// CreateFlowRun opens a new run in the running state.
func (d *DB) CreateFlowRun(ctx context.Context, r FlowRun) (FlowRun, error) {
	r.ID = newID()
	r.CreatedAt = now()
	r.UpdatedAt = r.CreatedAt
	if r.Status == "" {
		r.Status = FlowRunning
	}
	if r.State == "" {
		r.State = "{}"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return r, d.persistFlowRunLocked(r)
}

// GetFlowRun loads a run by id.
func (d *DB) GetFlowRun(ctx context.Context, id string) (FlowRun, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return FlowRun{}, ErrNotFound
	}
	return r, nil
}

// ListFlowRuns returns runs for a flow (or all if flowID is empty), newest first.
func (d *DB) ListFlowRuns(ctx context.Context, flowID string) ([]FlowRun, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]FlowRun, 0)
	for _, r := range d.flowRuns {
		if flowID == "" || r.FlowID == flowID {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// SetFlowRunState persists the restart-safe state snapshot mid-run.
func (d *DB) SetFlowRunState(ctx context.Context, id, state string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	r.State = state
	r.UpdatedAt = now()
	return d.persistFlowRunLocked(r)
}

// FinishFlowRun records the terminal status, final output and error.
func (d *DB) FinishFlowRun(ctx context.Context, id, status, output, errText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	r.Output = output
	r.Error = errText
	r.UpdatedAt = now()
	return d.persistFlowRunLocked(r)
}

// ListRunningFlowRuns returns runs still in the running state (for resume on boot),
// oldest first.
func (d *DB) ListRunningFlowRuns(ctx context.Context) ([]FlowRun, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]FlowRun, 0)
	for _, r := range d.flowRuns {
		if r.Status == FlowRunning {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}
