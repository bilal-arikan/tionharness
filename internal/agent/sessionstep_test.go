package agent

import "testing"

func TestBusForwardableRejectsEphemeralLiveFrames(t *testing.T) {
	for _, st := range []TurnStep{
		{Kind: StepTool, ID: "running", Running: true},
		{Kind: StepTool, ID: "append", Append: true},
	} {
		if busForwardable(st) {
			t.Fatalf("ephemeral step was forwardable: %+v", st)
		}
	}

	if st := (TurnStep{Kind: StepTool, ID: "final"}); !busForwardable(st) {
		t.Fatalf("final step was not forwardable: %+v", st)
	}
}
