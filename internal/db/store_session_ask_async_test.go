package db

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAsyncAnswersClaimOnceAndPersist(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ask, err := d.CreateSessionAsk(ctx, SessionAsk{SessionID: "SES1", AgentID: "A1", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ClaimSessionAsk(ctx, ask.ID, "Blue"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]string{{"SES2", "A1"}, {"SES1", "A2"}} {
		got, err := d.TakeAsyncSessionAnswers(ctx, scope[0], scope[1])
		if err != nil || len(got) != 0 {
			t.Fatalf("cross-scope delivery: %+v %v", got, err)
		}
	}
	var delivered atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			got, err := d.TakeAsyncSessionAnswers(ctx, "SES1", "A1")
			if err != nil {
				t.Error(err)
			}
			delivered.Add(int32(len(got)))
		})
	}
	wg.Wait()
	if delivered.Load() != 1 {
		t.Fatalf("deliveries=%d", delivered.Load())
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	stored, err := d.GetSessionAsk(ctx, ask.ID)
	if err != nil || !stored.Async || !stored.AnswerDelivered || stored.Answer != "Blue" {
		t.Fatalf("reopened=%+v %v", stored, err)
	}
	got, err := d.TakeAsyncSessionAnswers(ctx, "SES1", "")
	if err != nil || len(got) != 0 {
		t.Fatalf("redelivered after restart: %+v %v", got, err)
	}
}
