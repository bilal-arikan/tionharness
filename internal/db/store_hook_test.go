package db

import (
	"context"
	"testing"
)

func TestHookStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	created, err := d.CreateHook(ctx, Hook{
		Event:      HookPreToolUse,
		Matcher:    "shell",
		Command:    "echo hi",
		TimeoutSec: 10,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Type != "command" || created.CreatedAt == 0 {
		t.Fatalf("create defaults wrong: %+v", created)
	}

	// Enabled-by-event filter.
	pre, err := d.ListEnabledHooksByEvent(ctx, HookPreToolUse)
	if err != nil || len(pre) != 1 {
		t.Fatalf("list enabled pre: got %d err %v", len(pre), err)
	}
	post, _ := d.ListEnabledHooksByEvent(ctx, HookPostToolUse)
	if len(post) != 0 {
		t.Fatalf("expected 0 post hooks, got %d", len(post))
	}

	// Update mutable fields.
	created.Matcher = "write_file"
	created.Event = HookPostToolUse
	if err := d.UpdateHook(ctx, created); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Toggle off.
	if err := d.SetHookEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("toggle: %v", err)
	}

	// Reopen → persistence survives.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetHook(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.Matcher != "write_file" || got.Event != HookPostToolUse || got.Enabled {
		t.Fatalf("reopened hook mismatch: %+v", got)
	}

	// Delete.
	if err := d2.DeleteHook(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := d2.GetHook(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
