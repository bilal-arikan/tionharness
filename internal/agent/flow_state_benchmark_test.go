package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

const minimumFlowStateMarshalByteReduction = 0.90

func benchmarkFlowStates() []orchestration.State {
	states := make([]orchestration.State, 100)
	state := orchestration.State{Current: "0", Outputs: map[string]string{}}
	for i := range states {
		state = cloneFlowState(state)
		state.Current = fmt.Sprintf("%d", i+1)
		state.Steps = i + 1
		state.Outputs[state.Current] = strings.Repeat("x", 32<<10)
		state.Thread = append(state.Thread, orchestration.Msg{Role: "assistant", Text: strings.Repeat("t", 1024)})
		state.Trace = append(state.Trace, orchestration.TraceEntry{NodeID: state.Current, Output: state.Outputs[state.Current]})
		states[i] = state
	}
	return states
}

func marshalFlowStates(states []orchestration.State, delta bool) (int64, error) {
	var total int64
	var writer *flowStateDeltaWriter
	if delta {
		writer = newFlowStateDeltaWriter("checkpoint", 0, orchestration.State{Outputs: map[string]string{}})
	}
	for _, state := range states {
		value := any(state)
		if delta {
			flowDelta, err := writer.next(state)
			if err != nil {
				return 0, err
			}
			value = flowDelta
		}
		data, err := json.Marshal(value)
		if err != nil {
			return 0, err
		}
		total += int64(len(data))
	}
	return total, nil
}

func TestFlowStateDeltaMarshalBytesReductionAtLeast90Percent(t *testing.T) {
	states := benchmarkFlowStates()
	wholesaleBytes, err := marshalFlowStates(states, false)
	if err != nil {
		t.Fatal(err)
	}
	deltaBytes, err := marshalFlowStates(states, true)
	if err != nil {
		t.Fatal(err)
	}
	reduction := 1 - float64(deltaBytes)/float64(wholesaleBytes)
	t.Logf("JSON marshal bytes: wholesale=%d delta=%d reduction=%.2f%%", wholesaleBytes, deltaBytes, reduction*100)
	if reduction < minimumFlowStateMarshalByteReduction {
		t.Fatalf("JSON marshal byte reduction %.2f%%, want at least %.2f%%", reduction*100, minimumFlowStateMarshalByteReduction*100)
	}
}

// BenchmarkFlowStatePersistence reports two distinct byte metrics:
// marshal_B/op is the exact JSON payload produced by json.Marshal, while the
// standard B/op metric is the acceptance metric for allocated bytes. allocs/op
// remains diagnostic because delta construction may trade larger allocations
// for more small, short-lived allocations.
func BenchmarkFlowStatePersistence(b *testing.B) {
	states := benchmarkFlowStates()
	b.Run("wholesale", func(b *testing.B) {
		var marshaledBytes int64
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			iterationBytes, err := marshalFlowStates(states, false)
			if err != nil {
				b.Fatal(err)
			}
			marshaledBytes += iterationBytes
		}
		b.ReportMetric(float64(marshaledBytes)/float64(b.N), "marshal_B/op")
	})
	b.Run("delta", func(b *testing.B) {
		var marshaledBytes int64
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			iterationBytes, err := marshalFlowStates(states, true)
			if err != nil {
				b.Fatal(err)
			}
			marshaledBytes += iterationBytes
		}
		b.ReportMetric(float64(marshaledBytes)/float64(b.N), "marshal_B/op")
	})
}
