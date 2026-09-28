package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTranscriptEditFailurePreservesCachedHistory(t *testing.T) {
	for _, edit := range []string{"feedback", "delete", "rewind"} {
		t.Run(edit, func(t *testing.T) {
			d, s, ids := seedTranscript(t, 3)
			defer d.Close()
			path := d.dir(dirSessions, s.ID, sessionMsgsFile)
			// A directory at the file's destination makes atomic replacement fail
			// consistently on Windows and Unix without relying on chmod semantics.
			if err := os.Rename(path, path+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch edit {
			case "feedback":
				err = d.SetMessageFeedback(context.Background(), s.ID, ids[1], 1, "changed")
			case "delete":
				err = d.DeleteMessage(context.Background(), s.ID, ids[1])
			case "rewind":
				_, err = d.DeleteMessagesFrom(context.Background(), s.ID, ids[1])
			}
			if err == nil {
				t.Fatal("write failure was ignored")
			}
			msgs, err := d.ListMessages(context.Background(), s.ID)
			if err != nil || len(msgs) != 3 || msgs[1].Feedback != nil || d.sessions[s.ID].MessageCount != 3 {
				t.Fatalf("failed edit changed memory: %+v %v", msgs, err)
			}
		})
	}
}

func TestTranscriptCacheConcurrentEvictionAndRead(t *testing.T) {
	d, first, firstIDs := seedTranscript(t, 3)
	defer d.Close()
	ctx := context.Background()
	second, err := d.CreateSession(ctx, Session{AgentID: first.AgentID, Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.AddMessage(ctx, Message{SessionID: second.ID, Role: "user", Text: "second"})
	if err != nil {
		t.Fatal(err)
	}
	d.transcriptCacheLimit = 1
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sid, count, id := first.ID, 3, firstIDs[0]
			if i%2 == 1 {
				sid, count, id = second.ID, 1, m.ID
			}
			for range 25 {
				msgs, err := d.ListMessages(ctx, sid)
				if err != nil || len(msgs) != count || msgs[0].ID != id {
					errors <- fmt.Errorf("lost pinned history: %s %d %v", sid, len(msgs), err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if d.Stats().LoadedSessions != 0 {
		t.Fatal("oversized transcripts remained resident")
	}
}

func TestTranscriptPageSummarySurvivesPagingAndRewind(t *testing.T) {
	d, s, ids := seedTranscript(t, 3)
	defer d.Close()
	ctx := context.Background()
	m, err := d.AddMessage(ctx, Message{SessionID: s.ID, Role: "assistant", Steps: `[{"kind":"todo","id":"todo-one","todos":[{"content":"Current task","status":"pending"}]}]`})
	if err != nil {
		t.Fatal(err)
	}
	page, err := d.ListMessagePage(ctx, s.ID, 1, ids[1], "", "", "")
	if err != nil || page.Summary.Todo == nil || page.Summary.Todo.OccurrenceID != fmt.Sprintf("[%q,%q]", m.ID, "todo-one") {
		t.Fatalf("old page lost current todo: %+v %v", page, err)
	}
	if _, err := d.DeleteMessagesFrom(ctx, s.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	page, err = d.ListMessagePage(ctx, s.ID, 1, "", "", "", "")
	if err != nil || page.Summary.Todo != nil || page.Total != 3 {
		t.Fatalf("rewind retained discarded summary: %+v %v", page, err)
	}
}
