package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// snippetSchema mirrors codebase-memory-mcp's get_code_snippet contract, the tool
// whose omitted `project` produced a "not indexed" error that read as a missing
// index and cost a real session ten fallback greps.
const snippetSchema = `{
	"type":"object",
	"properties":{
		"qualified_name":{"type":"string"},
		"project":{"type":"string"},
		"include_neighbors":{"type":"boolean"}
	},
	"required":["qualified_name","project"]
}`

func TestMissingRequiredArgs(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect []string
	}{
		{"all present", `{"qualified_name":"pkg.Fn","project":"P"}`, nil},
		{"project omitted", `{"qualified_name":"pkg.Fn"}`, []string{"project"}},
		{"project blank", `{"qualified_name":"pkg.Fn","project":"   "}`, []string{"project"}},
		{"project null", `{"qualified_name":"pkg.Fn","project":null}`, []string{"project"}},
		{"both omitted", `{}`, []string{"project", "qualified_name"}},
		{"optional omitted only", `{"qualified_name":"pkg.Fn","project":"P"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingRequiredArgs(json.RawMessage(snippetSchema), json.RawMessage(tc.input))
			if strings.Join(got, ",") != strings.Join(tc.expect, ",") {
				t.Errorf("missing = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestMissingRequiredArgs_NoContractNoOpinion(t *testing.T) {
	// No schema at all (a built-in, or a server that declared none).
	if got := missingRequiredArgs(nil, json.RawMessage(`{}`)); got != nil {
		t.Errorf("absent schema must yield no findings; got %v", got)
	}
	// A schema without a `required` array constrains nothing.
	if got := missingRequiredArgs(json.RawMessage(`{"type":"object","properties":{"a":{}}}`), json.RawMessage(`{}`)); got != nil {
		t.Errorf("schema without `required` must yield no findings; got %v", got)
	}
	// Unparsable schema → pass through rather than block on a guess.
	if got := missingRequiredArgs(json.RawMessage(`not json`), json.RawMessage(`{}`)); got != nil {
		t.Errorf("unparsable schema must yield no findings; got %v", got)
	}
	// Input that is not an object is the server's business, not ours.
	if got := missingRequiredArgs(json.RawMessage(snippetSchema), json.RawMessage(`"scalar"`)); got != nil {
		t.Errorf("non-object input must yield no findings; got %v", got)
	}
}

func TestPrefillMCPArgs_FillsProjectFromSessionCwd(t *testing.T) {
	call := mcpCall(searchTool, map[string]any{"qualified_name": "pkg.Fn"})
	fixed, ok := prefillMCPArgs(call, []string{"project"}, tionswarmCwd)
	if !ok {
		t.Fatal("expected the project argument to be filled from the session cwd")
	}
	if got := callProjectArg(fixed); got != "C-Users-user-Desktop-Projects-TionSwarm" {
		t.Errorf("project = %q", got)
	}
	// The gate is satisfied after the prefill — that is the whole point.
	if got := missingRequiredArgs(json.RawMessage(snippetSchema), fixed.Input); got != nil {
		t.Errorf("prefilled call still reports missing args: %v", got)
	}
}

func TestPrefillMCPArgs_DeclinesWhatItCannotKnow(t *testing.T) {
	call := mcpCall(searchTool, map[string]any{"project": "P"})
	// qualified_name is model intent, not context TionSwarm holds — never invented.
	if _, ok := prefillMCPArgs(call, []string{"qualified_name"}, tionswarmCwd); ok {
		t.Error("only `project` may be prefilled")
	}
	// Without a session cwd there is nothing to derive.
	if _, ok := prefillMCPArgs(call, []string{"project"}, ""); ok {
		t.Error("an unknown session cwd must not produce a project id")
	}
}

func TestMissingArgsMessage_StatesTheCallWasNotSent(t *testing.T) {
	msg := missingArgsMessage(searchTool, []string{"project"})
	if !strings.Contains(msg, "was not sent") {
		t.Errorf("message must say the call never reached the server; got %q", msg)
	}
	if !strings.Contains(msg, "`project`") {
		t.Errorf("message must name the missing argument; got %q", msg)
	}
	// It must not be mistakable for a verdict about the requested data.
	if !strings.Contains(msg, "local schema check") {
		t.Errorf("message must frame itself as a local check; got %q", msg)
	}
}

func TestArgSupplied(t *testing.T) {
	cases := []struct {
		raw    string
		expect bool
	}{
		{``, false},
		{`null`, false},
		{`""`, false},
		{`"  "`, false},
		{`"x"`, true},
		{`0`, true},
		{`false`, true},
		{`[]`, true},
		{`{}`, true},
	}
	for _, tc := range cases {
		if got := argSupplied(json.RawMessage(tc.raw)); got != tc.expect {
			t.Errorf("argSupplied(%q) = %v, want %v", tc.raw, got, tc.expect)
		}
	}
}

func TestMissingRequiredArgs_EmptyInput(t *testing.T) {
	// A tool call with no arguments at all still reports the full required set.
	call := providers.ToolCall{Name: searchTool}
	got := missingRequiredArgs(json.RawMessage(snippetSchema), call.Input)
	if strings.Join(got, ",") != "project,qualified_name" {
		t.Errorf("missing = %v", got)
	}
}
