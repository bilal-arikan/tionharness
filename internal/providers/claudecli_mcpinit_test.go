package providers

import (
	"strings"
	"testing"
)

// traceText concatenates every text trace step, so a note can be asserted
// regardless of where it landed in the trace.
func traceText(steps []TraceStep) string {
	var b strings.Builder
	for _, s := range steps {
		if s.Kind == "text" {
			b.WriteString(s.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// A server the CLI could not connect costs the turn its tools without failing
// the turn — the init inventory is the only place that says so.
func TestCLIInitNotesUnusableMCPServers(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"init","session_id":"s1","mcp_servers":[{"name":"ok_one","status":"connected"},{"name":"warming","status":"pending"},{"name":"dead","status":"failed"},{"name":"gated","status":"needs-auth"}],"failed_mcp_servers":[{"name":"broken","errorCode":"ENOENT","error":"spawn npx ENOENT"}]}`)

	got := traceText(p.resp.Trace)
	for _, want := range []string{"dead (failed)", "gated (needs-auth)", "broken (spawn npx ENOENT)"} {
		if !strings.Contains(got, want) {
			t.Errorf("note %q missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"ok_one", "warming"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("note %q must not mention healthy server %q", got, unwanted)
		}
	}
}

func TestCLIInitSilentWhenAllMCPServersHealthy(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"init","session_id":"s1","mcp_servers":[{"name":"ok_one","status":"connected"}]}`)
	if got := traceText(p.resp.Trace); strings.TrimSpace(got) != "" {
		t.Errorf("trace = %q, want no note", got)
	}
}

// The note is a per-turn statement, not a per-event one: a second init (resume)
// must not repeat it.
func TestCLIInitNotesMCPOnlyOnce(t *testing.T) {
	p := newCLIParser("", nil)
	line := `{"type":"system","subtype":"init","mcp_servers":[{"name":"dead","status":"failed"}]}`
	p.feed(line)
	p.feed(line)
	if n := strings.Count(traceText(p.resp.Trace), "dead (failed)"); n != 1 {
		t.Errorf("note emitted %d times, want 1", n)
	}
}

func TestDescribeUnusableMCPServersFallbacks(t *testing.T) {
	got := describeUnusableMCPServers(cliEvent{
		FailedMCPServers: []cliFailedMCPServer{
			{Name: "", ErrorCode: "ECONNREFUSED"}, // no name, no message
			{Name: "silent"},                      // no reason at all
		},
	})
	for _, want := range []string{"(unnamed server) (ECONNREFUSED)", "silent (failed to start)"} {
		if !strings.Contains(got, want) {
			t.Errorf("note %q missing %q", got, want)
		}
	}
}
