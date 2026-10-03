package decider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLRotationFailurePreservesJournalPolicies(t *testing.T) {
	for _, name := range []string{"ledger", "debug"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name+".jsonl")
			rotated := filepath.Join(dir, name+".1.jsonl")
			if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			// A nonempty directory blocks rotation on both Unix and Windows.
			if err := os.Mkdir(rotated, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rotated, "block"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			want := "old\n"
			if name == "ledger" {
				l := &Ledger{path: path, maxBytes: 4}
				err = l.appendLine([]byte("new\n"))
				want += "new\n"
				if err != nil {
					t.Fatalf("best-effort ledger append: %v", err)
				}
			} else {
				j := &DebugJournal{path: path, maxBytes: 4}
				err = j.persist([]byte("new\n"))
				if err == nil {
					t.Fatal("debug append must report blocked rotation")
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatalf("current log = %q, %v; want %q", data, err, want)
			}
		})
	}
}

func TestLedgerRotationAndAppendBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", ledgerFileName)
	l := &Ledger{path: path, maxBytes: 8}
	for _, line := range []string{"one\n", "two\n"} {
		if err := l.appendLine([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(rotatedLedgerPath(path)); !os.IsNotExist(err) {
		t.Fatalf("log rotated at exact size boundary: %v", err)
	}
	if err := l.appendLine([]byte("three\n")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{path: "three\n", rotatedLedgerPath(path): "one\ntwo\n"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("log %s = %q, %v; want %q", name, data, err, want)
		}
	}
}
