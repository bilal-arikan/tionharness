package db

import (
	"context"
	"os"
	"testing"
)

// TestActivatedToolsRoundTrip covers the gateway activation sidecar: a missing
// file reads as "nothing activated", a written set round-trips sorted and
// de-duplicated, a second write overwrites (never appends), and an empty write
// clears the file.
func TestActivatedToolsRoundTrip(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()
	sess, err := d.CreateSession(context.Background(), Session{})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// No sidecar yet.
	got, ok, err := d.ReadActivatedTools(sess.ID)
	if err != nil || ok || got != nil {
		t.Fatalf("absent sidecar = (%v, %v, %v), want (nil, false, nil)", got, ok, err)
	}

	if err := d.WriteActivatedTools(sess.ID, []string{"notify", "focus_view", "notify"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok, err = d.ReadActivatedTools(sess.ID)
	if err != nil || !ok {
		t.Fatalf("read after write = (ok=%v, err=%v)", ok, err)
	}
	if len(got) != 2 || got[0] != "focus_view" || got[1] != "notify" {
		t.Fatalf("round-trip = %v, want sorted+deduped [focus_view notify]", got)
	}

	// Overwrite replaces the whole set.
	if err := d.WriteActivatedTools(sess.ID, []string{"schedule_wake"}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _, _ = d.ReadActivatedTools(sess.ID)
	if len(got) != 1 || got[0] != "schedule_wake" {
		t.Fatalf("after overwrite = %v, want [schedule_wake]", got)
	}

	// An empty set clears the sidecar.
	if err := d.WriteActivatedTools(sess.ID, nil); err != nil {
		t.Fatalf("clear via empty write: %v", err)
	}
	if _, ok, _ := d.ReadActivatedTools(sess.ID); ok {
		t.Fatalf("empty write must remove the sidecar")
	}
	// Clearing again is a no-op, not an error.
	if err := d.ClearActivatedTools(sess.ID); err != nil {
		t.Fatalf("clear missing sidecar: %v", err)
	}
}

// TestActivatedToolsCorruptSidecarErrors locks the "don't swallow it" rule: a
// corrupt sidecar must surface as an error, not as a silent empty set (which is
// indistinguishable from a genuine reset).
func TestActivatedToolsCorruptSidecarErrors(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()
	sess, err := d.CreateSession(context.Background(), Session{})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := d.WriteActivatedTools(sess.ID, []string{"notify"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(d.activatedToolsPath(sess.ID), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("corrupt sidecar: %v", err)
	}
	if _, _, err := d.ReadActivatedTools(sess.ID); err == nil {
		t.Fatal("corrupt sidecar must return an error")
	}
}
