package db

import (
	"context"
	"errors"
	"os"
	"sync"
)

// Curator report (Rota F3, _Docs/78 §8). The LLM-free curator pass runs weekly
// when the workspace is idle (or on demand) and writes ONE report: what it
// archived and what it only suggests. The report is the audit trail the UI
// shows; the entities themselves carry the Archived flag.

// Curator action kinds and entity kinds.
const (
	CuratorActionArchive = "archive" // applied: the entity was archived
	CuratorActionSuggest = "suggest" // not applied: needs a human (provenance / class)

	CuratorEntityAutomation = "automation"
	CuratorEntitySchedule   = "schedule"
	CuratorEntityHook       = "hook"
	CuratorEntityRecipe     = "recipe"
)

// CuratorAction is one thing the curator did or proposes.
type CuratorAction struct {
	Kind   string `json:"kind"`   // CuratorAction*
	Entity string `json:"entity"` // CuratorEntity*
	ID     string `json:"id"`     // entity id (recipe: slug)
	Name   string `json:"name,omitempty"`
	// Reason is the short machine reason (exhausted, expired, never_fired,
	// unfired_watcher, ghost_phase, one_shot_done); Detail the human line.
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
	Applied bool   `json:"applied"`
	// Evidence names the trajectories / counts the recipe suggestions rest on.
	Evidence string `json:"evidence,omitempty"`
}

// CuratorReport is the outcome of one pass.
type CuratorReport struct {
	At          int64           `json:"at"`
	Trigger     string          `json:"trigger"` // manual | weekly
	Idle        bool            `json:"idle"`
	Archived    int             `json:"archived"`
	Suggestions int             `json:"suggestions"`
	Actions     []CuratorAction `json:"actions"`
}

const (
	dirCurator        = "curator"
	curatorReportFile = "last.json"
)

var curatorMu sync.Mutex

// SaveCuratorReport persists the latest pass (one file, overwritten).
func (d *DB) SaveCuratorReport(ctx context.Context, rep CuratorReport) error {
	curatorMu.Lock()
	defer curatorMu.Unlock()
	if rep.Actions == nil {
		rep.Actions = []CuratorAction{}
	}
	return atomicWriteJSON(d.dir(dirCurator, curatorReportFile), rep)
}

// GetCuratorReport returns the latest pass; ok=false when none ran yet.
func (d *DB) GetCuratorReport(ctx context.Context) (CuratorReport, bool, error) {
	curatorMu.Lock()
	defer curatorMu.Unlock()
	var rep CuratorReport
	err := readJSONFile(d.dir(dirCurator, curatorReportFile), &rep)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CuratorReport{}, false, nil
		}
		return CuratorReport{}, false, err
	}
	if rep.Actions == nil {
		rep.Actions = []CuratorAction{}
	}
	return rep, true, nil
}
