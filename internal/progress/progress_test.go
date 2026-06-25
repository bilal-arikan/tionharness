package progress

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rec := Record{
		UpdatedAt: 1719300000,
		SessionID: "SES1",
		AgentID:   "AGT1",
		Todos: []TodoItem{
			{Content: "build progress pkg", Status: "completed"},
			{Content: "wire sink", Status: "in_progress"},
		},
		Log: []LogEntry{{TS: 1719300000, SessionID: "SES1", Note: "1/2 completed"}},
	}
	if err := Save(dir, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(dir)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Version != Version {
		t.Fatalf("version not stamped: %d", got.Version)
	}
	if len(got.Todos) != 2 || got.Todos[0].Status != "completed" {
		t.Fatalf("todos round-trip wrong: %+v", got.Todos)
	}
	if got.AgentID != "AGT1" || got.SessionID != "SES1" {
		t.Fatalf("ids round-trip wrong: %+v", got)
	}
}

func TestSaveLoadRichFeatureFields(t *testing.T) {
	dir := t.TempDir()
	rec := Record{Todos: []TodoItem{
		{Content: "new-chat button works", Status: "pending", Category: "functional", Steps: []string{"click", "verify empty", "verify sidebar"}},
	}}
	if err := Save(dir, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(dir)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	it := got.Todos[0]
	if it.Category != "functional" || len(it.Steps) != 3 || it.Steps[2] != "verify sidebar" {
		t.Fatalf("rich fields not round-tripped: %+v", it)
	}
}

func TestLoadMissingIsNotError(t *testing.T) {
	_, ok, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if ok {
		t.Fatal("ok must be false when no progress file exists")
	}
}

func TestLoadEmptyDir(t *testing.T) {
	_, ok, err := Load("")
	if err != nil || ok {
		t.Fatalf("empty dir: ok=%v err=%v", ok, err)
	}
}

func TestSaveTrimsLog(t *testing.T) {
	dir := t.TempDir()
	var log []LogEntry
	for i := 0; i < maxLog+10; i++ {
		log = append(log, LogEntry{TS: int64(i), Note: "n"})
	}
	if err := Save(dir, Record{Log: log}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _, _ := Load(dir)
	if len(got.Log) != maxLog {
		t.Fatalf("log not trimmed: got %d want %d", len(got.Log), maxLog)
	}
	// Oldest entries dropped: first kept entry is index 10.
	if got.Log[0].TS != 10 {
		t.Fatalf("trim kept wrong window: first TS=%d", got.Log[0].TS)
	}
}

func TestFilePath(t *testing.T) {
	got := File("/proj")
	want := filepath.Join("/proj", ".swarmgo", "progress.json")
	if got != want {
		t.Fatalf("File() = %q want %q", got, want)
	}
}
