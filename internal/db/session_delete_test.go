package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestDeleteSessionKeepsStateWhenDirectoryRemovalFails(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "Tester", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	wantErr := errors.New("folder busy")

	err = d.deleteSession(ctx, session.ID, func(string) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("delete error = %v, want %v", err, wantErr)
	}
	if _, err := d.GetSession(ctx, session.ID); err != nil {
		t.Fatalf("session removed after failed directory removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Root(), dirSessions, session.ID)); err != nil {
		t.Fatalf("persisted session removed after failed directory removal: %v", err)
	}
}

func TestRemoveSessionDirRetriesWindowsSharingViolation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows sharing violations only")
	}
	attempts := 0
	err := removeSessionDirWithRetry(context.Background(), "unused", func(string) error {
		attempts++
		if attempts < 3 {
			return &os.PathError{Op: "unlinkat", Path: "scratchpad", Err: syscall.Errno(32)}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("remove with transient sharing violation: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
