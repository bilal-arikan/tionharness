package db

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/flow"
)

// ---- Flows (one per agent) ----

func (d *DB) persistFlowLocked(f Flow) error {
	return dbPersistLocked(d, d.flows, dirAgentFlows, f.ID, f)
}

// GetFlow loads a flow by id.
func (d *DB) GetFlow(ctx context.Context, id string) (Flow, error) {
	return dbGet(d, d.flows, id)
}

// FlowForAgent returns the agent's main flow, or ErrNotFound when none was
// created yet (see EnsureAgentFlow).
func (d *DB) FlowForAgent(ctx context.Context, agentID string) (Flow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if id, ok := d.flowByAgent[agentID]; ok {
		if f, ok := d.flows[id]; ok {
			return f, nil
		}
	}
	return Flow{}, ErrNotFound
}

// EnsureAgentFlow returns the agent's main flow, creating the default
// (input → respond → output) as version 1 when the agent has none.
func (d *DB) EnsureAgentFlow(ctx context.Context, agentID string) (Flow, error) {
	if f, err := d.FlowForAgent(ctx, agentID); err == nil {
		return f, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.flowByAgent[agentID]; ok {
		if f, ok := d.flows[id]; ok {
			return f, nil
		}
	}
	a, ok := d.agents[agentID]
	if !ok {
		return Flow{}, ErrNotFound
	}
	g := flow.DefaultGraph()
	ts := now()
	f := Flow{
		ID:        d.nextID(idFlow),
		AgentID:   agentID,
		Name:      a.Name,
		Graph:     flow.Encode(g),
		Version:   1,
		Policy:    DefaultFlowPolicy(),
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	v := FlowVersion{FlowID: f.ID, Version: 1, Graph: f.Graph, Author: FlowAuthor{Kind: FlowAuthorSystem}, Reason: "default flow", CreatedAt: ts}
	if err := d.writeFlowVersion(v); err != nil {
		return Flow{}, err
	}
	d.flowByAgent[agentID] = f.ID
	return f, d.persistFlowLocked(f)
}

// ListFlows returns every flow whose agent still exists (not deleted), newest
// activity first.
func (d *DB) ListFlows(ctx context.Context) ([]Flow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Flow, 0, len(d.flows))
	for _, f := range d.flows {
		if a, ok := d.agents[f.AgentID]; !ok || a.Deleted {
			continue
		}
		out = append(out, f)
	}
	slices.SortStableFunc(out, func(a, b Flow) int {
		if c := cmp.Compare(b.UpdatedAt, a.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out, nil
}

// CommitFlowVersion stores graph as the flow's new head version. The graph must
// already be validated by the caller; the store only records it. Returns the
// new version row.
func (d *DB) CommitFlowVersion(ctx context.Context, flowID string, graph flow.Graph, author FlowAuthor, reason, proposalID string) (FlowVersion, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[flowID]
	if !ok {
		return FlowVersion{}, ErrNotFound
	}
	prev, _ := flow.Parse(f.Graph)
	diff := flow.Diff(prev, graph)
	ts := now()
	v := FlowVersion{
		FlowID:     flowID,
		Version:    f.Version + 1,
		Parent:     f.Version,
		Graph:      flow.Encode(graph.Normalized()),
		Author:     author,
		Reason:     strings.TrimSpace(reason),
		ProposalID: proposalID,
		Diff:       diff.String(),
		CreatedAt:  ts,
	}
	if err := d.writeFlowVersion(v); err != nil {
		return FlowVersion{}, err
	}
	f.Graph = v.Graph
	f.Version = v.Version
	f.UpdatedAt = ts
	return v, d.persistFlowLocked(f)
}

// UpdateFlowLayout rewrites the head graph IN PLACE for a cosmetic change
// (node positions): the version stays, the head version file is refreshed so a
// later revert restores the positions too.
func (d *DB) UpdateFlowLayout(ctx context.Context, flowID string, graph flow.Graph) (Flow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[flowID]
	if !ok {
		return Flow{}, ErrNotFound
	}
	f.Graph = flow.Encode(graph.Normalized())
	f.UpdatedAt = now()
	if v, err := d.GetFlowVersion(ctx, flowID, f.Version); err == nil {
		v.Graph = f.Graph
		_ = d.writeFlowVersion(v)
	}
	return f, d.persistFlowLocked(f)
}

// UpdateFlowMeta edits the flow's name, note and policy (not its graph).
func (d *DB) UpdateFlowMeta(ctx context.Context, flowID string, name, note *string, policy *FlowPolicy) (Flow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[flowID]
	if !ok {
		return Flow{}, ErrNotFound
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		f.Name = strings.TrimSpace(*name)
	}
	if note != nil {
		f.Note = strings.TrimSpace(*note)
	}
	if policy != nil {
		f.Policy = policy.Normalized()
	}
	f.UpdatedAt = now()
	return f, d.persistFlowLocked(f)
}

// MarkFlowOptimized records that the observer looked at the flow now.
func (d *DB) MarkFlowOptimized(ctx context.Context, flowID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[flowID]
	if !ok {
		return ErrNotFound
	}
	f.Stats.RunsAtOptimize = f.Stats.Runs
	f.Stats.LastOptimizeAt = now()
	return d.persistFlowLocked(f)
}

// RewindFlowOptimizeMarkerForTest resets the observer's "looked at" marker so a
// test can make the policy window due again. Not used by production code.
func (d *DB) RewindFlowOptimizeMarkerForTest(flowID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if f, ok := d.flows[flowID]; ok {
		f.Stats.RunsAtOptimize = 0
		_ = d.persistFlowLocked(f)
	}
}

// DeleteFlowsForAgent removes an agent's flow, its versions, runs and
// proposals. Called when an agent is purged; a soft-deleted agent keeps its
// flow (ListFlows hides it).
func (d *DB) DeleteFlowsForAgent(ctx context.Context, agentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, ok := d.flowByAgent[agentID]
	if !ok {
		return nil
	}
	delete(d.flowByAgent, agentID)
	for rid, r := range d.flowRuns {
		if r.FlowID == id {
			if err := dbDeleteLocked(d, d.flowRuns, dirAgentFlowRuns, rid); err != nil {
				return err
			}
		}
	}
	for pid, p := range d.flowProposals {
		if p.FlowID == id {
			if err := dbDeleteLocked(d, d.flowProposals, dirAgentFlowProposals, pid); err != nil {
				return err
			}
		}
	}
	_ = os.RemoveAll(d.dir(dirAgentFlowVersions, id))
	return dbDeleteLocked(d, d.flows, dirAgentFlows, id)
}

// ---- Versions (on disk only: <store>/agent-flow-versions/<flow>/<n>.json) ----

func (d *DB) flowVersionPath(flowID string, version int) string {
	return d.dir(dirAgentFlowVersions, flowID, strconv.Itoa(version)+".json")
}

func (d *DB) writeFlowVersion(v FlowVersion) error {
	return atomicWriteJSON(d.flowVersionPath(v.FlowID, v.Version), v)
}

// GetFlowVersion reads one version.
func (d *DB) GetFlowVersion(ctx context.Context, flowID string, version int) (FlowVersion, error) {
	raw, err := os.ReadFile(d.flowVersionPath(flowID, version))
	if err != nil {
		if os.IsNotExist(err) {
			return FlowVersion{}, ErrNotFound
		}
		return FlowVersion{}, err
	}
	var v FlowVersion
	if err := json.Unmarshal(raw, &v); err != nil {
		return FlowVersion{}, fmt.Errorf("flow version %s/%d: %w", flowID, version, err)
	}
	return v, nil
}

// ListFlowVersions returns every version of a flow, newest first.
func (d *DB) ListFlowVersions(ctx context.Context, flowID string) ([]FlowVersion, error) {
	entries, err := os.ReadDir(d.dir(dirAgentFlowVersions, flowID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]FlowVersion, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		v, err := d.GetFlowVersion(ctx, flowID, n)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b FlowVersion) int { return cmp.Compare(b.Version, a.Version) })
	return out, nil
}

// ---- Runs ----

func (d *DB) persistFlowRunLocked(r FlowRun) error {
	return dbPersistLocked(d, d.flowRuns, dirAgentFlowRuns, r.ID, r)
}

// CreateFlowRun opens a run (status running) and bumps the flow's counters.
func (d *DB) CreateFlowRun(ctx context.Context, r FlowRun) (FlowRun, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[r.FlowID]
	if !ok {
		return FlowRun{}, ErrNotFound
	}
	r.ID = d.nextID(idFlowRun)
	r.Status = FlowRunning
	r.CreatedAt = now()
	r.UpdatedAt = r.CreatedAt
	if r.Version == 0 {
		r.Version = f.Version
	}
	if r.AgentID == "" {
		r.AgentID = f.AgentID
	}
	if err := d.persistFlowRunLocked(r); err != nil {
		return FlowRun{}, err
	}
	d.runningFlowRuns.Add(1)
	f.Stats.Runs++
	f.Stats.LastRunAt = r.CreatedAt
	f.Stats.LastRunID = r.ID
	f.UpdatedAt = r.CreatedAt
	return r, d.persistFlowLocked(f)
}

// FinishFlowRun closes a run with its outcome and folds it into the flow stats.
func (d *DB) FinishFlowRun(ctx context.Context, id string, status, output, errText string, steps json.RawMessage, stepCount int, durationMs int64, usage FlowRunUsage) (FlowRun, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return FlowRun{}, ErrNotFound
	}
	if r.Status == FlowRunning {
		d.runningFlowRuns.Add(-1)
	}
	r.Status = status
	r.Output = output
	r.Error = errText
	r.Steps = steps
	r.StepCount = stepCount
	r.DurationMs = durationMs
	r.Usage = usage
	r.UpdatedAt = now()
	if err := d.persistFlowRunLocked(r); err != nil {
		return FlowRun{}, err
	}
	if f, ok := d.flows[r.FlowID]; ok {
		switch status {
		case FlowSuccess:
			f.Stats.Success++
		case FlowFailure:
			f.Stats.Failure++
		}
		f.Stats.TotalMs += durationMs
		f.Stats.TotalTokens += usage.InputTokens + usage.OutputTokens
		f.UpdatedAt = r.UpdatedAt
		if err := d.persistFlowLocked(f); err != nil {
			return FlowRun{}, err
		}
	}
	return r, nil
}

// SetFlowRunGrade records the decision model's grade (1..5) of a finished
// run and folds it into the flow's rolling stats; grading the same run twice
// replaces the earlier grade.
func (d *DB) SetFlowRunGrade(ctx context.Context, id string, grade int, confidence float64) (FlowRun, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return FlowRun{}, ErrNotFound
	}
	if grade < 1 || grade > 5 {
		return FlowRun{}, fmt.Errorf("grade %d out of range 1..5", grade)
	}
	prev := r.Grade
	r.Grade, r.GradeConfidence = grade, confidence
	r.UpdatedAt = now()
	if err := d.persistFlowRunLocked(r); err != nil {
		return FlowRun{}, err
	}
	if f, ok := d.flows[r.FlowID]; ok {
		if prev > 0 {
			f.Stats.GradeSum -= prev
		} else {
			f.Stats.Graded++
		}
		f.Stats.GradeSum += grade
		f.UpdatedAt = r.UpdatedAt
		if err := d.persistFlowLocked(f); err != nil {
			return FlowRun{}, err
		}
	}
	return r, nil
}

// SetFlowRunMessage links a run to the assistant message it produced.
func (d *DB) SetFlowRunMessage(ctx context.Context, id, messageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.flowRuns[id]
	if !ok {
		return ErrNotFound
	}
	r.MessageID = messageID
	return d.persistFlowRunLocked(r)
}

// SetFlowRunFeedback mirrors a message rating onto the run that produced it.
func (d *DB) SetFlowRunFeedback(ctx context.Context, messageID string, rating int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, r := range d.flowRuns {
		if r.MessageID == messageID {
			r.Feedback = rating
			_ = dbPersistLocked(d, d.flowRuns, dirAgentFlowRuns, id, r)
			return
		}
	}
}

// GetFlowRun loads a run by id.
func (d *DB) GetFlowRun(ctx context.Context, id string) (FlowRun, error) {
	return dbGet(d, d.flowRuns, id)
}

// ListFlowRuns returns runs newest first, all flows when flowID is "". limit
// <= 0 returns everything.
func (d *DB) ListFlowRuns(ctx context.Context, flowID string, limit int) ([]FlowRun, error) {
	out := dbFilter(d, d.flowRuns,
		func(r FlowRun) bool { return flowID == "" || r.FlowID == flowID },
		func(a, b FlowRun) bool {
			if a.CreatedAt != b.CreatedAt {
				return a.CreatedAt > b.CreatedAt
			}
			return a.ID > b.ID
		})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeleteFlowRun removes one run.
func (d *DB) DeleteFlowRun(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.flowRuns[id]; ok && r.Status == FlowRunning {
		d.runningFlowRuns.Add(-1)
	}
	return dbDeleteLocked(d, d.flowRuns, dirAgentFlowRuns, id)
}

// HasRunningFlowRuns is the O(1) activity probe.
func (d *DB) HasRunningFlowRuns() bool { return d.runningFlowRuns.Load() > 0 }

// FailOrphanedFlowRuns marks every run still "running" as failed: called at
// boot, because a run only outlives its process when that process died.
func (d *DB) FailOrphanedFlowRuns(ctx context.Context) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for id, r := range d.flowRuns {
		if r.Status != FlowRunning {
			continue
		}
		r.Status = FlowFailure
		r.Error = "interrupted by restart"
		r.UpdatedAt = now()
		if err := dbPersistLocked(d, d.flowRuns, dirAgentFlowRuns, id, r); err == nil {
			n++
		}
	}
	d.runningFlowRuns.Store(0)
	return n
}

// PruneFlowRuns keeps the newest keep finished runs per flow (keep <= 0 = all).
func (d *DB) PruneFlowRuns(ctx context.Context, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	byFlow := map[string][]FlowRun{}
	for _, r := range d.flowRuns {
		if r.Status != FlowRunning {
			byFlow[r.FlowID] = append(byFlow[r.FlowID], r)
		}
	}
	removed := 0
	for _, runs := range byFlow {
		if len(runs) <= keep {
			continue
		}
		slices.SortFunc(runs, func(a, b FlowRun) int { return cmp.Compare(b.CreatedAt, a.CreatedAt) })
		for _, r := range runs[keep:] {
			if err := dbDeleteLocked(d, d.flowRuns, dirAgentFlowRuns, r.ID); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

// ---- Proposals ----

func (d *DB) persistProposalLocked(p FlowProposal) error {
	return dbPersistLocked(d, d.flowProposals, dirAgentFlowProposals, p.ID, p)
}

// CreateFlowProposal files a proposal (status pending unless preset).
func (d *DB) CreateFlowProposal(ctx context.Context, p FlowProposal) (FlowProposal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.flows[p.FlowID]
	if !ok {
		return FlowProposal{}, ErrNotFound
	}
	p.ID = d.nextID(idFlowProposal)
	if p.AgentID == "" {
		p.AgentID = f.AgentID
	}
	if p.BaseVersion == 0 {
		p.BaseVersion = f.Version
	}
	if p.Status == "" {
		p.Status = ProposalPending
	}
	p.CreatedAt = now()
	return p, d.persistProposalLocked(p)
}

// ResolveFlowProposal sets a proposal's terminal status.
func (d *DB) ResolveFlowProposal(ctx context.Context, id, status string, appliedVersion int, errText string) (FlowProposal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.flowProposals[id]
	if !ok {
		return FlowProposal{}, ErrNotFound
	}
	p.Status = status
	p.AppliedVersion = appliedVersion
	p.Error = errText
	p.ResolvedAt = now()
	return p, d.persistProposalLocked(p)
}

// GetFlowProposal loads a proposal.
func (d *DB) GetFlowProposal(ctx context.Context, id string) (FlowProposal, error) {
	return dbGet(d, d.flowProposals, id)
}

// ListFlowProposals returns a flow's proposals newest first ("" = all flows).
func (d *DB) ListFlowProposals(ctx context.Context, flowID string) ([]FlowProposal, error) {
	return dbFilter(d, d.flowProposals,
		func(p FlowProposal) bool { return flowID == "" || p.FlowID == flowID },
		func(a, b FlowProposal) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// CountPendingFlowProposals counts open proposals across the workspace.
func (d *DB) CountPendingFlowProposals() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	n := 0
	for _, p := range d.flowProposals {
		if p.Status == ProposalPending {
			n++
		}
	}
	return n
}

// ---- Agent prompt versions (<store>/agent-prompt-versions/<agent>/<n>.json) ----

func (d *DB) promptVersionPath(agentID string, version int) string {
	return d.dir(dirAgentPromptVersions, agentID, strconv.Itoa(version)+".json")
}

// ListAgentPromptVersions returns an agent's prompt history, newest first.
func (d *DB) ListAgentPromptVersions(ctx context.Context, agentID string) ([]AgentPromptVersion, error) {
	entries, err := os.ReadDir(d.dir(dirAgentPromptVersions, agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AgentPromptVersion, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(d.dir(dirAgentPromptVersions, agentID), e.Name()))
		if err != nil {
			continue
		}
		var v AgentPromptVersion
		if json.Unmarshal(raw, &v) == nil {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b AgentPromptVersion) int { return cmp.Compare(b.Version, a.Version) })
	return out, nil
}

// RecordAgentPromptVersion snapshots the agent's CURRENT soul/identity as the
// next version. The first call on an agent writes version 1 with the values as
// they were before any evolution, so a restore can always go back to the start.
func (d *DB) RecordAgentPromptVersion(ctx context.Context, agentID string, author FlowAuthor, reason, proposalID string) (AgentPromptVersion, error) {
	d.mu.RLock()
	a, ok := d.agents[agentID]
	d.mu.RUnlock()
	if !ok {
		return AgentPromptVersion{}, ErrNotFound
	}
	existing, err := d.ListAgentPromptVersions(ctx, agentID)
	if err != nil {
		return AgentPromptVersion{}, err
	}
	next := 1
	if len(existing) > 0 {
		next = existing[0].Version + 1
	}
	v := AgentPromptVersion{AgentID: agentID, Version: next, Soul: a.Soul, Identity: a.Identity, Author: author, Reason: strings.TrimSpace(reason), ProposalID: proposalID, CreatedAt: now()}
	return v, atomicWriteJSON(d.promptVersionPath(agentID, next), v)
}
