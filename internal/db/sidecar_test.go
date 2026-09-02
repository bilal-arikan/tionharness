package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sidecarProbe struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TestSidecarRoundTrip: absent → save → load → clear, with the file created
// under a not-yet-existing parent directory (atomicWriteJSON mkdirs).
func TestSidecarRoundTrip(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	sc := newSidecar[sidecarProbe](d, "", d.dir("probe", "nested", "probe.json"))

	if _, ok, err := sc.Load(); err != nil || ok {
		t.Fatalf("absent sidecar: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if sc.Exists() {
		t.Fatal("Exists() must be false before the first save")
	}
	if err := sc.Save(sidecarProbe{Name: "a", Count: 2}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := sc.Load()
	if err != nil || !ok || got != (sidecarProbe{Name: "a", Count: 2}) {
		t.Fatalf("load = %+v ok=%v err=%v", got, ok, err)
	}
	if _, err := os.Stat(sc.Path() + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file must not survive a successful save")
	}
	if err := sc.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := sc.Clear(); err != nil {
		t.Fatalf("clear of an absent file must be a no-op, got %v", err)
	}
	if _, ok, _ := sc.Load(); ok {
		t.Fatal("cleared sidecar still loads")
	}
}

// TestSidecarCorruptIsQuarantined: an undecodable file is moved aside (never
// deleted), reported once as *SidecarCorruptError with a debug event on the
// owning session, and reads as absent afterwards.
func TestSidecarCorruptIsQuarantined(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID})
	sc := newSidecar[sidecarProbe](d, sess.ID, d.dir(dirSessions, sess.ID, "probe.json"))

	if err := os.WriteFile(sc.Path(), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	_, ok, err := sc.Load()
	if ok || err == nil || !IsSidecarCorrupt(err) {
		t.Fatalf("corrupt load: ok=%v err=%v, want ok=false + SidecarCorruptError", ok, err)
	}
	var ce *SidecarCorruptError
	if !asSidecarCorrupt(err, &ce) || ce.Quarantined == "" {
		t.Fatalf("error must carry the quarantine path, got %v", err)
	}
	if !strings.HasPrefix(filepath.Base(ce.Quarantined), "probe.json.corrupt-") {
		t.Fatalf("quarantine name = %q, want probe.json.corrupt-<unix>", ce.Quarantined)
	}
	if raw, rerr := os.ReadFile(ce.Quarantined); rerr != nil || string(raw) != "{not json" {
		t.Fatalf("quarantined bytes must be preserved verbatim, got %q err=%v", raw, rerr)
	}
	if sc.Exists() {
		t.Fatal("the corrupt file must have been moved, not left in place")
	}
	// Second read: absent, not an error — the operation can proceed.
	if _, ok, err := sc.Load(); ok || err != nil {
		t.Fatalf("post-quarantine load: ok=%v err=%v, want absent", ok, err)
	}
	// Debug journal attribution on the owning session. The reader redacts free
	// text (Detail/Error) and fingerprints unknown names, so the named event must
	// be on the reader's allowlist to remain recognisable here.
	events, _ := d.ReadDebugEvents(ctx, sess.ID, DebugError, 10)
	found := false
	for _, ev := range events {
		if ev.Name == "sidecar_corrupt" && ev.Err {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a sidecar_corrupt debug event on %s, got %+v", sess.ID, events)
	}
}

func asSidecarCorrupt(err error, target **SidecarCorruptError) bool {
	for err != nil {
		if ce, ok := err.(*SidecarCorruptError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
