// Package flows holds the runtime-free parts of flow execution: the checkpoint
// state-delta writer and the built-in flow seeding/migration helpers. The
// executor itself stays in internal/agent. Extracted on 2026-09-03 (_Docs/81,
// step 5).
package flows

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

type StateDeltaWriter struct {
	checkpointID string
	sequence     uint64
	previous     orchestration.State
}

func NewStateDeltaWriter(checkpointID string, sequence uint64, state orchestration.State) *StateDeltaWriter {
	return &StateDeltaWriter{checkpointID: checkpointID, sequence: sequence, previous: CloneState(state)}
}

func CloneState(state orchestration.State) orchestration.State {
	cloned := state
	cloned.Outputs = make(map[string]string, len(state.Outputs))
	for key, value := range state.Outputs {
		cloned.Outputs[key] = value
	}
	cloned.Trace = append([]orchestration.TraceEntry(nil), state.Trace...)
	cloned.Thread = append([]orchestration.Msg(nil), state.Thread...)
	if state.Spawned != nil {
		cloned.Spawned = make(map[string][]string, len(state.Spawned))
		for key, ids := range state.Spawned {
			cloned.Spawned[key] = append([]string(nil), ids...)
		}
	}
	return cloned
}

func rawJSON(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	return json.RawMessage(data), err
}

func (w *StateDeltaWriter) Next(current orchestration.State) (db.FlowRunStateDelta, error) {
	if len(current.Trace) < len(w.previous.Trace) {
		return db.FlowRunStateDelta{}, fmt.Errorf("flow state trace prefix shrank from %d to %d", len(w.previous.Trace), len(current.Trace))
	}
	for i := range w.previous.Trace {
		if !reflect.DeepEqual(current.Trace[i], w.previous.Trace[i]) {
			return db.FlowRunStateDelta{}, fmt.Errorf("flow state trace prefix changed at %d", i)
		}
	}
	if len(current.Thread) < len(w.previous.Thread) {
		return db.FlowRunStateDelta{}, fmt.Errorf("flow state thread prefix shrank from %d to %d", len(w.previous.Thread), len(current.Thread))
	}
	for i := range w.previous.Thread {
		if !reflect.DeepEqual(current.Thread[i], w.previous.Thread[i]) {
			return db.FlowRunStateDelta{}, fmt.Errorf("flow state thread prefix changed at %d", i)
		}
	}
	delta := db.FlowRunStateDelta{
		Version: db.FlowRunStateDeltaVersion, CheckpointID: w.checkpointID,
		Sequence: w.sequence + 1, Scalars: map[string]json.RawMessage{},
		OutputsUpsert: map[string]string{},
	}
	scalars := map[string]any{
		"current": current.Current, "last": current.Last, "steps": current.Steps,
		"iter": current.Iter, "waitingAt": current.WaitingAt, "subflowRun": current.SubflowRun,
	}
	previousScalars := map[string]any{
		"current": w.previous.Current, "last": w.previous.Last, "steps": w.previous.Steps,
		"iter": w.previous.Iter, "waitingAt": w.previous.WaitingAt, "subflowRun": w.previous.SubflowRun,
	}
	for key, value := range scalars {
		if reflect.DeepEqual(value, previousScalars[key]) {
			continue
		}
		raw, err := rawJSON(value)
		if err != nil {
			return db.FlowRunStateDelta{}, err
		}
		delta.Scalars[key] = raw
	}
	for key, value := range current.Outputs {
		if old, ok := w.previous.Outputs[key]; !ok || old != value {
			delta.OutputsUpsert[key] = value
		}
	}
	for key := range w.previous.Outputs {
		if _, ok := current.Outputs[key]; !ok {
			delta.OutputsDelete = append(delta.OutputsDelete, key)
		}
	}
	for _, entry := range current.Trace[len(w.previous.Trace):] {
		raw, err := rawJSON(entry)
		if err != nil {
			return db.FlowRunStateDelta{}, err
		}
		delta.TraceAppend = append(delta.TraceAppend, raw)
	}
	for _, message := range current.Thread[len(w.previous.Thread):] {
		raw, err := rawJSON(message)
		if err != nil {
			return db.FlowRunStateDelta{}, err
		}
		delta.ThreadAppend = append(delta.ThreadAppend, raw)
	}
	if !reflect.DeepEqual(current.Spawned, w.previous.Spawned) {
		raw, err := rawJSON(current.Spawned)
		if err != nil {
			return db.FlowRunStateDelta{}, err
		}
		delta.Spawned = raw
	}
	w.previous = CloneState(current)
	w.sequence++
	return delta, nil
}
