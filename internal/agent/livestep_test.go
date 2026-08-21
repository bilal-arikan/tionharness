package agent

import "testing"

func TestLiveCardLifecycle(t *testing.T) {
	var got []TurnStep
	card := openLive(func(st TurnStep) { got = append(got, st) }, "call-1", TurnStep{
		Kind: StepTool,
		Tool: "Bash",
	})
	card.Chunk("one")
	card.Update(func(st *TurnStep) { st.Output = "one" })
	card.Close(TurnStep{Kind: StepTool, Tool: "Bash", Output: "done"})

	if len(got) != 4 {
		t.Fatalf("got %d frames, want 4", len(got))
	}
	for i, st := range got {
		if st.ID != "call-1" {
			t.Fatalf("frame %d ID = %q", i, st.ID)
		}
	}
	if !got[0].Running || got[0].Append {
		t.Fatalf("open frame flags = running:%v append:%v", got[0].Running, got[0].Append)
	}
	if !got[1].Running || !got[1].Append || got[1].Output != "one" {
		t.Fatalf("chunk frame = %+v", got[1])
	}
	if !got[2].Running || got[2].Append || got[2].Output != "one" {
		t.Fatalf("update frame = %+v", got[2])
	}
	if got[3].Running || got[3].Append || got[3].Output != "done" {
		t.Fatalf("close frame = %+v", got[3])
	}
}
