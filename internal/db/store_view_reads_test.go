package db

import "testing"

func TestRecordViewRead(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RecordViewRead("artifact:ART1", "AG1", "SES1"); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordViewRead("  ", "AG1", "SES1"); err != nil {
		t.Fatal(err)
	}
	got := d.ViewReads()
	if len(got) != 1 || got["artifact:ART1"].AgentID != "AG1" || got["artifact:ART1"].At == 0 {
		t.Fatalf("reads = %+v, want one stamped artifact read", got)
	}

	// A repeat read by another agent replaces the reader at once; the ledger
	// survives a reopen of the workspace.
	if err := d.RecordViewRead("artifact:ART1", "AG2", "SES2"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := reopened.ViewReads()["artifact:ART1"]; r.AgentID != "AG2" || r.SessionID != "SES2" {
		t.Fatalf("reopened read = %+v, want AG2/SES2", r)
	}
}

func TestPruneViewReadsKeepsNewest(t *testing.T) {
	reads := map[string]ViewRead{"a": {At: 1}, "b": {At: 3}, "c": {At: 2}}
	pruneViewReads(reads, 2)
	if _, ok := reads["a"]; ok || len(reads) != 2 {
		t.Fatalf("reads = %+v, want b and c", reads)
	}
}
