package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	d.markMutatedLocked()
	if err := removeFile(d.dir(dirFlows, id+".json")); err != nil {
		return err
	}
	for _, r := range d.flowRuns {
		if r.FlowID == id {
			d.deleteFlowRunLocked(r)
		}
	}
	return nil
}

// ---- Flow runs ----

// persistFlowRunLocked stores r and keeps runningFlowRuns in sync. prev is the
// row's status BEFORE this write ("" for a brand-new run).
//
// prev is a required parameter rather than something read back from the map on
// purpose: every caller already loads the old row in order to mutate it, so it
// costs nothing — and making it mandatory is what forces a future
// status-flipping path to confront the counter instead of silently skipping it
// (the compiler flags the missing argument). The caller must hold d.mu.
func (d *DB) persistFlowRunLocked(prev string, r FlowRun) error {
	if err := dbPersistLocked(d, d.flowRuns, dirFlowRuns, r.ID, r); err != nil {
		return err
	}
	// Checkpoint first, cleanup second: a crash can leave a detectable stale
	// journal, but can never leave deltas without their checkpoint.
	if err := d.deleteFlowRunStateDeltasLocked(r.ID); err != nil {
		return err
	}
	d.applyFlowRunDelta(prev, r.Status)
	return nil
}

// applyFlowRunDelta moves runningFlowRuns by the running-ness EDGE between two
// statuses: a no-op when both sides are running or neither is. Deleting a run is
// expressed as next == "".
func (d *DB) applyFlowRunDelta(prev, next string) {
	switch {
	case prev != FlowRunning && next == FlowRunning:
		d.runningFlowRuns.Add(1)
	case prev == FlowRunning && next != FlowRunning:
		d.runningFlowRuns.Add(-1)
	}
}

// deleteFlowRunLocked removes a run row plus its file and releases its running
// slot. Caller must hold d.mu.
func (d *DB) deleteFlowRunLocked(r FlowRun) {
	delete(d.flowRuns, r.ID)
	d.markMutatedLocked()
	_ = removeFile(d.dir(dirFlowRuns, r.ID+".json"))
	_ = d.deleteFlowRunStateDeltasLocked(r.ID)
	d.applyFlowRunDelta(r.Status, "")
}

func flowStateCheckpointID(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func (d *DB) flowRunDeltaDir(id string) string { return d.dir(dirFlowRunStateDeltas, id) }

func (d *DB) deleteFlowRunStateDeltasLocked(id string) error {
	err := os.RemoveAll(d.flowRunDeltaDir(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d *DB) loadFlowRunStateDeltas(id string) ([]FlowRunStateDelta, error) {
	entries, err := os.ReadDir(d.flowRunDeltaDir(id))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	deltas := make([]FlowRunStateDelta, 0, len(files))
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(d.flowRunDeltaDir(id), name))
		if err != nil {
			return nil, err
		}
		var delta FlowRunStateDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			return nil, fmt.Errorf("flow run %s delta %s: %w", id, name, err)
		}
		deltas = append(deltas, delta)
	}
	return deltas, nil
}

func applyFlowRunStateDelta(state string, deltas []FlowRunStateDelta) (string, error) {
	if len(deltas) == 0 {
		return state, nil
	}
	checkpointID := flowStateCheckpointID(state)
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(state), &document); err != nil {
		return "", err
	}
	for i, delta := range deltas {
		expected := uint64(i + 1)
		if delta.Version != FlowRunStateDeltaVersion {
			return "", fmt.Errorf("unsupported delta version %d", delta.Version)
		}
		if delta.CheckpointID != checkpointID {
			return "", fmt.Errorf("delta checkpoint mismatch: got %q want %q", delta.CheckpointID, checkpointID)
		}
		if delta.Sequence != expected {
			return "", fmt.Errorf("delta sequence %d, want %d", delta.Sequence, expected)
		}
		for key, value := range delta.Scalars {
			document[key] = value
		}
		var outputs map[string]string
		if raw := document["outputs"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &outputs); err != nil {
				return "", err
			}
		}
		if outputs == nil {
			outputs = map[string]string{}
		}
		for key, value := range delta.OutputsUpsert {
			outputs[key] = value
		}
		for _, key := range delta.OutputsDelete {
			delete(outputs, key)
		}
		if len(delta.OutputsUpsert) > 0 || len(delta.OutputsDelete) > 0 {
			document["outputs"], _ = json.Marshal(outputs)
		}
		for key, tail := range map[string][]json.RawMessage{"trace": delta.TraceAppend, "thread": delta.ThreadAppend} {
			if len(tail) == 0 {
				continue
			}
			var values []json.RawMessage
			if raw := document[key]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &values); err != nil {
					return "", err
				}
			}
			values = append(values, tail...)
			document[key], _ = json.Marshal(values)
		}
		if delta.Spawned != nil {
			document["spawned"] = delta.Spawned
		}
	}
	data, err := json.Marshal(document)
	return string(data), err
}

func (d *DB) materializeFlowRunState(id, checkpoint string) (string, error) {
	deltas, err := d.loadFlowRunStateDeltas(id)
	if err != nil {
		return "", err
	}
	state, err := applyFlowRunStateDelta(checkpoint, deltas)
	if err != nil {
		return "", fmt.Errorf("materialize flow run %s: %w", id, err)
	}
	return state, nil
}

// FlowRunStateJournalInfo returns the immutable checkpoint identity and the
// last durable sequence. Callers use it to continue a journal after restart.
func (d *DB) FlowRunStateJournalInfo(ctx context.Context, id string) (string, uint64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.flowRuns[id]; !ok {
		return "", 0, ErrNotFound
	}
	data, err := os.ReadFile(d.dir(dirFlowRuns, id+".json"))
	if err != nil {
		return "", 0, err
	}
	var checkpoint FlowRun
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return "", 0, err
	}
	deltas, err := d.loadFlowRunStateDeltas(id)
	if err != nil {
		return "", 0, err
	}
	if _, err := applyFlowRunStateDelta(checkpoint.State, deltas); err != nil {
		return "", 0, err
	}
	return flowStateCheckpointID(checkpoint.State), uint64(len(deltas)), nil
}

// AppendFlowRunStateDelta atomically adds one ordered sidecar and updates the
// in-memory materialized State exposed to all DB/API readers.
func (d *DB) AppendFlowRunStateDelta(ctx context.Context, id string, delta FlowRunStateDelta) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	deltas, err := d.loadFlowRunStateDeltas(id)
	if err != nil {
		return err
	}
	checkpointData, err := os.ReadFile(d.dir(dirFlowRuns, id+".json"))
	if err != nil {
		return err
	}
	var checkpoint FlowRun
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		return err
	}
	if delta.Sequence != uint64(len(deltas)+1) {
		return fmt.Errorf("delta sequence %d, want %d", delta.Sequence, len(deltas)+1)
	}
	if delta.CheckpointID != flowStateCheckpointID(checkpoint.State) {
		return fmt.Errorf("delta checkpoint mismatch")
	}
	all := append(deltas, delta)
	materialized, err := applyFlowRunStateDelta(checkpoint.State, all)
	if err != nil {
		return err
	}
	path := filepath.Join(d.flowRunDeltaDir(id), fmt.Sprintf("%020d.json", delta.Sequence))
	if err := atomicWriteJSON(path, delta); err != nil {
		return err
	}
	r.State = materialized
	r.UpdatedAt = now()
	d.flowRuns[id] = r
	d.markMutatedLocked()
	return nil
}

// HasRunningFlowRuns reports, in O(1) and WITHOUT taking d.mu, whether any flow
// run is in the running state. It is the fast-negative gate for the activity
// endpoints: a false answer skips the full scan entirely (the idle case), a true
// answer only means "now run the real query". Waiting runs do NOT count,
// mirroring ListRunningFlowRuns — a suspended run must not pulse the UI.
func (d *DB) HasRunningFlowRuns() bool { return d.runningFlowRuns.Load() > 0 }

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
	return r, d.persistFlowRunLocked("", r)
}

// GetFlowRun loads a run by id.
func (d *DB) GetFlowRun(ctx context.Context, id string) (FlowRun, error) {
	return dbGet(d, d.flowRuns, id)
}

func (d *DB) GetFlowRunByDispatchKey(ctx context.Context, key string) (FlowRun, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, run := range d.flowRuns {
		if key != "" && run.DispatchKey == key {
			return run, nil
		}
	}
	return FlowRun{}, ErrNotFound
}

// ListFlowRuns returns runs for a flow (or all if flowID is empty), newest first.
//
// The order is the exact reverse of flowRunBefore, NOT a plain CreatedAt compare:
// CreatedAt has second granularity (see now()), so two runs started within the
// same second are indistinguishable by time and a bare timestamp sort leaves
// their relative order down to map iteration — i.e. different on every call.
// Callers that take runs[0] as "the newest run" (the executions feed's status
// chip) would then flip between them at random. The id counter breaks the tie.
func (d *DB) ListFlowRuns(ctx context.Context, flowID string) ([]FlowRun, error) {
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return flowID == "" || r.FlowID == flowID },
		func(a, b FlowRun) bool { return flowRunBefore(b, a) }), nil
}

// ListRootFlowRuns is ListFlowRuns restricted to runs nothing else launched, so
// a run list is not flooded by every subflow/spawn child of a composed flow.
// Children remain reachable via ListFlowRunTree (or directly by id).
func (d *DB) ListRootFlowRuns(ctx context.Context, flowID string) ([]FlowRun, error) {
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return (flowID == "" || r.FlowID == flowID) && r.IsRootRun() },
		func(a, b FlowRun) bool { return flowRunBefore(b, a) }), nil // same tie-break as ListFlowRuns
}

// flowRunSeq extracts the monotonic counter nextID appended to a run id
// ("RUN12" → 12), used to order runs created within the same second. Ids are not
// zero-padded, so a lexicographic compare would put "RUN10" before "RUN2".
// Returns -1 for an unparseable id, which sorts such runs first but stably.
func flowRunSeq(id string) int64 {
	i := len(id)
	for i > 0 && id[i-1] >= '0' && id[i-1] <= '9' {
		i--
	}
	if i == len(id) {
		return -1
	}
	n, err := strconv.ParseInt(id[i:], 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// flowRunBefore is the deterministic creation order of two runs. CreatedAt alone
// is NOT enough: it has second granularity (see now()), and a composed flow
// creates a parent and its children within the same second — leaving their
// relative order arbitrary. The id counter breaks the tie.
func flowRunBefore(a, b FlowRun) bool {
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	return flowRunSeq(a.ID) < flowRunSeq(b.ID)
}

// ListFlowRunTree returns every run in rootID's tree — the root itself plus all
// descendants at any depth — ordered BREADTH-FIRST from the root, so a parent
// always precedes its children and siblings stay grouped. Resolving membership
// by RootRunID keeps this a single scan instead of a walk per level; the parent
// links then only order what that scan already found.
//
// Ordering walks ParentRunID rather than trusting timestamps: CreatedAt is
// second-granular, so a parent and the child it launches milliseconds later are
// routinely indistinguishable by time. Siblings are ordered by creation
// (flowRunBefore).
//
// An unknown or non-root id yields an empty result rather than an error: a tree
// that no longer exists is an empty tree, not a failure. Any member the walk
// cannot reach (a parent row deleted out from under it) is appended at the end
// in creation order rather than silently dropped.
func (d *DB) ListFlowRunTree(ctx context.Context, rootID string) ([]FlowRun, error) {
	if rootID == "" {
		return nil, nil
	}
	members := dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return r.RootOf() == rootID },
		flowRunBefore)
	if len(members) == 0 {
		return nil, nil
	}

	childrenOf := map[string][]FlowRun{}
	var root *FlowRun
	for i, r := range members {
		if r.ID == rootID {
			root = &members[i]
			continue
		}
		childrenOf[r.ParentRunID] = append(childrenOf[r.ParentRunID], r)
	}

	out := make([]FlowRun, 0, len(members))
	seen := map[string]bool{}
	if root != nil {
		queue := []FlowRun{*root}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if seen[cur.ID] { // defensive: a cycle must not spin forever
				continue
			}
			seen[cur.ID] = true
			out = append(out, cur)
			queue = append(queue, childrenOf[cur.ID]...)
		}
	}
	for _, r := range members {
		if !seen[r.ID] {
			out = append(out, r)
		}
	}
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
	return d.persistFlowRunLocked(r.Status, r) // status untouched: zero delta
}

// SetFlowRunSession links a run to the transcript session it produced. Merges
// into the existing record (state/status untouched) so it can be called after
// the run finishes.
func (d *DB) SetFlowRunSession(ctx context.Context, id, sessionID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	r.SessionID = sessionID
	r.UpdatedAt = now()
	return d.persistFlowRunLocked(r.Status, r) // status untouched: zero delta
}

// FinishFlowRun records the terminal status, final output and error.
func (d *DB) FinishFlowRun(ctx context.Context, id, status, output, errText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	prev := r.Status // read BEFORE the mutation below overwrites it
	r.Status = status
	r.Output = output
	r.Error = errText
	r.UpdatedAt = now()
	return d.persistFlowRunLocked(prev, r)
}

// ListRunningFlowRuns returns runs still in the running state (for resume on boot),
// oldest first. Waiting runs are intentionally excluded — they sleep until input,
// so boot never revives them (no orphan).
func (d *DB) ListRunningFlowRuns(ctx context.Context) ([]FlowRun, error) {
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return r.Status == FlowRunning },
		func(a, b FlowRun) bool { return a.CreatedAt < b.CreatedAt }), nil
}

// MarkFlowRunWaiting durably suspends a run at an await-input node: it persists
// the state snapshot AND flips the status to waiting in one step, so a crash
// between the two can't leave a "running" row with await-input state.
func (d *DB) MarkFlowRunWaiting(ctx context.Context, id, state string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	prev := r.Status // read BEFORE the mutation below overwrites it
	r.State = state
	r.Status = FlowWaiting
	r.UpdatedAt = now()
	return d.persistFlowRunLocked(prev, r)
}

// ListWaitingFlowRuns returns runs suspended at an await-input node (for the
// timeout sweeper), oldest suspend first.
func (d *DB) ListWaitingFlowRuns(ctx context.Context) ([]FlowRun, error) {
	return dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return r.Status == FlowWaiting },
		func(a, b FlowRun) bool { return a.UpdatedAt < b.UpdatedAt }), nil
}

// ClaimWaitingFlowRun atomically transitions a run from waiting → running and
// returns it, so exactly one resume wins the race (concurrent input from multiple
// windows). Returns ErrNotFound if the run is missing and a plain error if the run
// is not currently waiting (already resumed, finished, or never suspended).
func (d *DB) ClaimWaitingFlowRun(ctx context.Context, id string) (FlowRun, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return FlowRun{}, ErrNotFound
	}
	if r.Status != FlowWaiting {
		return FlowRun{}, fmt.Errorf("flow run %s is not waiting (status %q)", id, r.Status)
	}
	prev := r.Status // FlowWaiting, checked above — read before the flip
	r.Status = FlowRunning
	r.UpdatedAt = now()
	if err := d.persistFlowRunLocked(prev, r); err != nil {
		return FlowRun{}, err
	}
	return r, nil
}
