package db

import (
	"context"
	"errors"
	"os"
	"sync"
)

// Evolver bookkeeping (_Docs/83 E2): per goal, when the evolver last ran, how
// many scoped sessions it had seen, and which proposal signatures keep coming
// back — persisted as evolution/state.json next to the snapshots.

// EvolutionGoalState is one goal's evolver bookkeeping.
type EvolutionGoalState struct {
	LastAt       int64  `json:"lastAt"`
	SessionsSeen int    `json:"sessionsSeen"` // scoped sessions at the last pass
	Trigger      string `json:"trigger,omitempty"`
	Proposals    int    `json:"proposals"`
	Dropped      int    `json:"dropped"`
	Skipped      string `json:"skipped,omitempty"`
	SnapshotHash string `json:"snapshotHash,omitempty"`
	// Repeats counts consecutive passes an OPEN proposal signature returned;
	// Escalated holds the signatures already turned into a human escalation
	// (they are not re-proposed).
	Repeats   map[string]int  `json:"repeats,omitempty"`
	Escalated map[string]bool `json:"escalated,omitempty"`
}

// EvolutionState is the whole map.
type EvolutionState struct {
	Goals map[string]EvolutionGoalState `json:"goals"`
}

const evolutionStateFile = "state.json"

var evolutionMu sync.Mutex

// GetEvolutionState loads the state; an absent file is an empty state.
func (d *DB) GetEvolutionState(ctx context.Context) (EvolutionState, error) {
	evolutionMu.Lock()
	defer evolutionMu.Unlock()
	return d.loadEvolutionStateLocked()
}

func (d *DB) loadEvolutionStateLocked() (EvolutionState, error) {
	var st EvolutionState
	err := readJSONFile(d.dir(dirEvolution, evolutionStateFile), &st)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return EvolutionState{}, err
	}
	if st.Goals == nil {
		st.Goals = map[string]EvolutionGoalState{}
	}
	return st, nil
}

// SetEvolutionGoalState records one goal's latest pass.
func (d *DB) SetEvolutionGoalState(ctx context.Context, goalID string, s EvolutionGoalState) error {
	evolutionMu.Lock()
	defer evolutionMu.Unlock()
	st, err := d.loadEvolutionStateLocked()
	if err != nil {
		return err
	}
	st.Goals[goalID] = s
	return atomicWriteJSON(d.dir(dirEvolution, evolutionStateFile), st)
}

// DeleteEvolutionGoalState forgets a goal's bookkeeping (goal deleted).
func (d *DB) DeleteEvolutionGoalState(ctx context.Context, goalID string) error {
	evolutionMu.Lock()
	defer evolutionMu.Unlock()
	st, err := d.loadEvolutionStateLocked()
	if err != nil {
		return err
	}
	if _, ok := st.Goals[goalID]; !ok {
		return nil
	}
	delete(st.Goals, goalID)
	return atomicWriteJSON(d.dir(dirEvolution, evolutionStateFile), st)
}
