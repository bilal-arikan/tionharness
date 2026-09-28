package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLazyTranscriptEvictionPreservesHistory(t *testing.T) {
	ctx := context.Background()
	d, session, ids := seedTranscript(t, 4)
	root := d.root
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if st := d.Stats(); st.LoadedSessions != 0 || st.Messages != 0 {
		t.Fatalf("eager transcript: %+v", st)
	}
	d.transcriptCacheLimit = 1
	for range 2 {
		msgs, err := d.ListMessages(ctx, session.ID)
		if err != nil || len(msgs) != 4 || msgs[0].ID != ids[0] {
			t.Fatalf("history after eviction: %d %v", len(msgs), err)
		}
		if d.Stats().LoadedSessions != 0 {
			t.Fatal("oversized transcript retained after unpin")
		}
	}
	if err := d.SetMessageFeedback(ctx, session.ID, ids[1], 1, "kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: session.ID, Role: "assistant", Text: "searchable reply", Steps: `[{"kind":"tool"}]`}); err != nil {
		t.Fatal(err)
	}
	hits, err := d.SearchMessages(ctx, SearchOpts{Query: "searchable", OnlyID: session.ID})
	if err != nil || len(hits) != 1 {
		t.Fatalf("cold search: %+v %v", hits, err)
	}
	if _, err := d.DeleteMessagesFrom(ctx, session.ID, ids[3]); err != nil {
		t.Fatal(err)
	}
	msgs, err := d.ListMessages(ctx, session.ID)
	if err != nil || len(msgs) != 3 || msgs[1].Feedback == nil {
		t.Fatalf("cold mutations lost data: %+v %v", msgs, err)
	}
}

func TestTranscriptCheckpointInvalidatesAfterAppendAndCorruption(t *testing.T) {
	d, session, _ := seedTranscript(t, 2)
	root := d.root
	d.Close()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d.Close() // Creates a checkpoint for the two existing lines.
	path := filepath.Join(root, dirSessions, session.ID, sessionMsgsFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("{\"id\":\"external\",\"role\":\"assistant\",\"steps\":\"[{\\\"kind\\\":\\\"tool\\\"}]\"}\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	d, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := d.sessions[session.ID]; s.MessageCount != 3 || s.ToolCallCount != 1 {
		t.Fatalf("stale checkpoint: %+v", s)
	}
	d.Close()
	if err := os.WriteFile(filepath.Join(root, "transcripts-cache.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.sessions[session.ID].MessageCount != 3 {
		t.Fatal("corrupt cache changed canonical counters")
	}
}

func TestMessagePagesUseStableCursors(t *testing.T) {
	d, s, ids := seedTranscript(t, 10)
	defer d.Close()
	ctx := context.Background()
	page, err := d.ListMessagePage(ctx, s.ID, 3, "", "", "", "")
	if err != nil || page.Offset != 7 || page.HasNewer || !page.HasMore {
		t.Fatalf("tail: %+v %v", page, err)
	}
	if err := d.DeleteMessage(ctx, s.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	page, err = d.ListMessagePage(ctx, s.ID, 3, ids[7], "", "", "")
	if err != nil || len(page.Items) != 3 || page.Items[0].ID != ids[4] || !page.HasNewer {
		t.Fatalf("before deleted prefix: %+v %v", page, err)
	}
	page, err = d.ListMessagePage(ctx, s.ID, 3, "", "", ids[2], "")
	if err != nil || page.Items[1].ID != ids[2] {
		t.Fatalf("around: %+v %v", page, err)
	}
	page, err = d.ListMessagePage(ctx, s.ID, 3, "", ids[8], "", "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != ids[9] {
		t.Fatalf("after: %+v %v", page, err)
	}
}

func TestStreamingDecoderOnlyToleratesTornFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	good := `{"id":"one","text":"` + strings.Repeat("x", 70000) + `"}` + "\n"
	for _, suffix := range []string{"broken\n", "broken\n\n", "\n"} {
		if err := os.WriteFile(path, []byte(good+suffix), 0600); err != nil {
			t.Fatal(err)
		}
		msgs, err := readMessagesFile(path)
		if err != nil || len(msgs) != 1 {
			t.Fatalf("torn tail: %d %v", len(msgs), err)
		}
	}
	if err := os.WriteFile(path, []byte("broken\n"+good), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMessagesFile(path); err == nil {
		t.Fatal("interior corruption accepted")
	}
}
