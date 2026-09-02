package db

import (
	"context"
	"fmt"
	"sort"
)

// Flow run garbage collection (_Docs/77 R8).
//
// Runs accumulated forever: there was no delete endpoint and no retention. Both
// operate on TREES (a root run plus every subflow/spawn descendant) so a
// composed flow never leaves orphaned children behind, and both refuse to touch
// a tree with a live (running or waiting) member — a running run is driven by a
// goroutine that would otherwise checkpoint into a deleted row, and a waiting
// run is a durable suspend a human may still resume.

// FlowRunTreeLive reports whether any run in the tree is running or waiting.
func (d *DB) FlowRunTreeLive(ctx context.Context, rootID string) (bool, error) {
	tree, err := d.ListFlowRunTree(ctx, rootID)
	if err != nil {
		return false, err
	}
	for _, r := range tree {
		if r.Status == FlowRunning || r.Status == FlowWaiting {
			return true, nil
		}
	}
	return false, nil
}

// DeleteFlowRunTree removes a run and every descendant of its tree. id may name
// any member; the whole tree keyed by its root goes. Returns the deleted run
// ids. ErrNotFound for an unknown id; ErrConflict when a member is still live.
func (d *DB) DeleteFlowRunTree(ctx context.Context, id string) ([]string, error) {
	run, err := d.GetFlowRun(ctx, id)
	if err != nil {
		return nil, err
	}
	rootID := run.RootOf()
	if live, lerr := d.FlowRunTreeLive(ctx, rootID); lerr != nil {
		return nil, lerr
	} else if live {
		return nil, fmt.Errorf("flow run tree %s has a running or waiting member: %w", rootID, ErrConflict)
	}
	tree, err := d.ListFlowRunTree(ctx, rootID)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	deleted := make([]string, 0, len(tree))
	for _, r := range tree {
		if cur, ok := d.flowRuns[r.ID]; ok {
			d.deleteFlowRunLocked(cur)
			deleted = append(deleted, r.ID)
		}
	}
	return deleted, nil
}

// PruneFlowRuns applies retention to ONE flow: keep the newest `keep` terminal
// ROOT runs (success/failure), delete the older ones with their trees. keep <= 0
// means unlimited (no-op). Live trees are never counted nor deleted. Returns the
// deleted run ids across all pruned trees.
func (d *DB) PruneFlowRuns(ctx context.Context, flowID string, keep int) ([]string, error) {
	if keep <= 0 || flowID == "" {
		return nil, nil
	}
	roots, err := d.ListRootFlowRuns(ctx, flowID)
	if err != nil {
		return nil, err
	}
	// Terminal roots, newest first (ListRootFlowRuns is newest-first already; sort
	// defensively so retention never depends on that ordering staying).
	var terminal []FlowRun
	for _, r := range roots {
		if r.Status == FlowSuccess || r.Status == FlowFailure {
			terminal = append(terminal, r)
		}
	}
	sort.Slice(terminal, func(i, j int) bool {
		if terminal[i].CreatedAt != terminal[j].CreatedAt {
			return terminal[i].CreatedAt > terminal[j].CreatedAt
		}
		return terminal[i].ID > terminal[j].ID
	})
	if len(terminal) <= keep {
		return nil, nil
	}
	var deleted []string
	for _, r := range terminal[keep:] {
		ids, derr := d.DeleteFlowRunTree(ctx, r.ID)
		if derr != nil {
			// A tree that turned live between listing and deleting is simply kept.
			continue
		}
		deleted = append(deleted, ids...)
	}
	return deleted, nil
}
