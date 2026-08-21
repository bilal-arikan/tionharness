package agent

import (
	"context"
	"strings"
	"testing"
)

func TestFormatMCPFailureNote_NamesEveryServerAndReason(t *testing.T) {
	note := formatMCPFailureNote([]mcpFailedServer{
		{Name: "playwright", Err: "dial tcp 127.0.0.1:9222: connect: connection refused"},
		{Name: "codebase-memory", Err: "handshake timeout"},
	})
	for _, want := range []string{"playwright", "connection refused", "codebase-memory", "handshake timeout"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q missing %q", note, want)
		}
	}
}

func TestFormatMCPFailureNote_EmptyWhenNothingFailed(t *testing.T) {
	if got := formatMCPFailureNote(nil); got != "" {
		t.Errorf("note = %q, want empty", got)
	}
}

func TestMCPFailureCollector_SortedAndDeduped(t *testing.T) {
	ctx, c := withMCPFailures(context.Background())
	if mcpFailuresFrom(ctx) != c {
		t.Fatal("collector not reachable from ctx")
	}
	c.record("zeta", "boom")
	c.record("alpha", "first")
	c.record("alpha", "second") // last write wins, one entry
	got := c.list()
	if len(got) != 2 {
		t.Fatalf("list = %+v, want 2 entries", got)
	}
	if got[0].Name != "alpha" || got[0].Err != "second" {
		t.Errorf("first entry = %+v, want alpha/second", got[0])
	}
	if got[1].Name != "zeta" {
		t.Errorf("second entry = %+v, want zeta", got[1])
	}
}

func TestMCPFailureCollector_NilIsNoOp(t *testing.T) {
	var c *mcpFailureCollector
	c.record("x", "boom") // must not panic
	if got := c.list(); got != nil {
		t.Errorf("list = %+v, want nil", got)
	}
	// A context without a collector yields nil, so build paths need no branch.
	if mcpFailuresFrom(context.Background()) != nil {
		t.Error("bare context returned a collector")
	}
}

// A reasonless failure must still be reported: the server is down either way.
func TestMCPFailureCollector_EmptyReasonStillReported(t *testing.T) {
	_, c := withMCPFailures(context.Background())
	c.record("srv", "")
	note := formatMCPFailureNote(c.list())
	if !strings.Contains(note, "srv (no error text)") {
		t.Errorf("note = %q", note)
	}
}

func TestShortMCPErr(t *testing.T) {
	if got := shortMCPErr("  line one\nline  two \r"); got != "line one line two" {
		t.Errorf("collapse = %q", got)
	}
	if got := shortMCPErr(""); got != "no error text" {
		t.Errorf("empty = %q", got)
	}
	long := strings.Repeat("x", mcpErrTextLimit+50)
	got := shortMCPErr(long)
	if len(got) <= mcpErrTextLimit || !strings.HasSuffix(got, "…") {
		t.Errorf("long error not truncated: len=%d", len(got))
	}
}
