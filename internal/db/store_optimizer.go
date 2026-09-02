package db

import (
	"context"
	"errors"
	"os"
	"sync"
)

// Recipe optimizer state (Rota F4): per recipe slug, when the optimizer last
// ran and how many summarized terminal runs it had seen — the threshold
// bookkeeping that keeps the LLM pass rare (brief §7.2).

// OptimizerSlugState is one recipe's optimizer bookkeeping.
type OptimizerSlugState struct {
	LastAt   int64  `json:"lastAt"`
	RunsSeen int    `json:"runsSeen"` // summarized terminal runs at the last pass
	Trigger  string `json:"trigger,omitempty"`
	// Proposals is how many proposals the last pass filed.
	Proposals int `json:"proposals"`
	// Skipped explains a pass that produced nothing (no agent, no runs …).
	Skipped string `json:"skipped,omitempty"`
}

// OptimizerState is the whole map, persisted as optimizer/state.json.
type OptimizerState struct {
	Slugs map[string]OptimizerSlugState `json:"slugs"`
}

const (
	dirOptimizer       = "optimizer"
	optimizerStateFile = "state.json"
)

var optimizerMu sync.Mutex

// GetOptimizerState loads the state; an absent file is an empty state.
func (d *DB) GetOptimizerState(ctx context.Context) (OptimizerState, error) {
	optimizerMu.Lock()
	defer optimizerMu.Unlock()
	return d.loadOptimizerStateLocked()
}

func (d *DB) loadOptimizerStateLocked() (OptimizerState, error) {
	var st OptimizerState
	err := readJSONFile(d.dir(dirOptimizer, optimizerStateFile), &st)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return OptimizerState{}, err
	}
	if st.Slugs == nil {
		st.Slugs = map[string]OptimizerSlugState{}
	}
	return st, nil
}

// SetOptimizerSlugState records one recipe's latest pass.
func (d *DB) SetOptimizerSlugState(ctx context.Context, slug string, s OptimizerSlugState) error {
	optimizerMu.Lock()
	defer optimizerMu.Unlock()
	st, err := d.loadOptimizerStateLocked()
	if err != nil {
		return err
	}
	st.Slugs[slug] = s
	return atomicWriteJSON(d.dir(dirOptimizer, optimizerStateFile), st)
}
