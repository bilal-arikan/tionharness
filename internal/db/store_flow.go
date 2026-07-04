package db

import (
	"context"
)

// ---- Flows ----

func (d *DB) persistFlowLocked(f Flow) error {
	return dbPersistLocked(d, d.flows, dirFlows, f.ID, f)
}

// CreateFlow inserts a new flow and returns the stored row.
func (d *DB) CreateFlow(ctx context.Context, f Flow) (Flow, error) {
	f.ID = d.nextID(idFlow)
	f.CreatedAt = now()
	f.UpdatedAt = f.CreatedAt
	if f.Graph == "" {
		f.Graph = "{}"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return f, d.persistFlowLocked(f)
}

// FlowPath returns the absolute path of a flow's on-disk JSON file (one file per
// flow under the workspace store's flows/ folder).
func (d *DB) FlowPath(flowID string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.flows[flowID]; !ok {
		return "", ErrNotFound
	}
	return d.dir(dirFlows, flowID+".json"), nil
}

// GetFlow loads a flow by id.
func (d *DB) GetFlow(ctx context.Context, id string) (Flow, error) {
	return dbGet(d, d.flows, id)
}

// ListFlows returns all flows, newest first.
func (d *DB) ListFlows(ctx context.Context) ([]Flow, error) {
	return dbList(d, d.flows, func(a, b Flow) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// UpdateFlow edits a flow's name/graph.
func (d *DB) UpdateFlow(ctx context.Context, f Flow) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.flows[f.ID]
	if !ok {
		return ErrNotFound
	}
	cur.Name = f.Name
	cur.Graph = f.Graph
	cur.UpdatedAt = now()
	return d.persistFlowLocked(cur)
}

// SetFlowEmoji replaces a flow's cosmetic emoji without touching its other
// fields (mirrors SetFlowTags), so it survives independent name/graph saves.
func (d *DB) SetFlowEmoji(ctx context.Context, id, emoji string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.flows[id]
	if !ok {
		return ErrNotFound
	}
	cur.Emoji = emoji
	cur.UpdatedAt = now()
	return d.persistFlowLocked(cur)
}

// SetFlowTags replaces a flow's free-form tags without touching its other fields.
func (d *DB) SetFlowTags(ctx context.Context, id string, tags []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.flows[id]
	if !ok {
		return ErrNotFound
	}
	cur.Tags = normalizeTags(tags)
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
	return dbPersistLocked(d, d.flowRuns, dirFlowRuns, r.ID, r)
}

// CreateFlowRun opens a new run in the running state.
func (d *DB) CreateFlowRun(ctx context.Context, r FlowRun) (FlowRun, error) {
	r.ID = d.nextID(idFlowRun)
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
	return dbGet(d, d.flowRuns, id)
}

// ListFlowRuns returns runs for a flow (or all if flowID is empty), newest first.
func (d *DB) ListFlowRuns(ctx context.Context, flowID string) ([]FlowRun, error) {
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return flowID == "" || r.FlowID == flowID },
		func(a, b FlowRun) bool { return a.CreatedAt > b.CreatedAt }), nil
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
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return r.Status == FlowRunning },
		func(a, b FlowRun) bool { return a.CreatedAt < b.CreatedAt }), nil
}
