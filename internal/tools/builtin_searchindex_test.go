package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeIndexBridge records what the tool asked for and returns canned state.
type fakeIndexBridge struct {
	roots     []string
	entries   []SearchIndexEntry
	started   bool
	err       error
	gotRoot   string
	gotAction string
	calls     int
}

func (f *fakeIndexBridge) SearchIndexStatus(_ context.Context, root string) []SearchIndexEntry {
	f.gotRoot = root
	return f.entries
}

func (f *fakeIndexBridge) RefreshSearchIndex(_ context.Context, root, action string) (SearchIndexEntry, bool, error) {
	f.calls++
	f.gotRoot, f.gotAction = root, action
	if f.err != nil {
		return SearchIndexEntry{}, false, f.err
	}
	return SearchIndexEntry{Tool: "zg", Root: root, Phase: "indexing", Action: action, Managed: true}, f.started, nil
}

func (f *fakeIndexBridge) IndexRoots(context.Context) []string { return f.roots }

// callIndexTool runs the tool and decodes its JSON reply.
func callIndexTool(t *testing.T, b SearchIndexBridge, input string) (searchIndexResult, error) {
	t.Helper()
	out, err := NewSearchIndexTool(b).Call(context.Background(), json.RawMessage(input))
	if err != nil {
		return searchIndexResult{}, err
	}
	var res searchIndexResult
	if uErr := json.Unmarshal([]byte(out), &res); uErr != nil {
		t.Fatalf("reply is not the documented JSON shape: %v\n%s", uErr, out)
	}
	return res, nil
}

func TestSearchIndexStatusIsTheDefaultAction(t *testing.T) {
	b := &fakeIndexBridge{
		roots:   []string{`C:\repo`},
		entries: []SearchIndexEntry{{Tool: "zg", Root: `C:\repo`, Phase: "ready", Usable: true, Managed: true}},
	}
	// No input at all must not be an error: status is the safe default.
	res, err := callIndexTool(t, b, `{}`)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if res.Action != "status" {
		t.Errorf("action = %q, want status", res.Action)
	}
	if len(res.Indexes) != 1 || res.Indexes[0].Phase != "ready" {
		t.Errorf("status did not report the bridge entries: %+v", res.Indexes)
	}
	if b.gotRoot != `C:\repo` {
		t.Errorf("status asked about %q, want the session root", b.gotRoot)
	}
}

func TestSearchIndexRefreshAndRebuildRouteToTheManager(t *testing.T) {
	for _, action := range []string{"refresh", "rebuild"} {
		b := &fakeIndexBridge{roots: []string{`C:\repo`}, started: true}
		res, err := callIndexTool(t, b, `{"action":"`+action+`"}`)
		if err != nil {
			t.Fatalf("%s failed: %v", action, err)
		}
		if b.gotAction != action {
			t.Errorf("manager received %q, want %q", b.gotAction, action)
		}
		if !res.Started {
			t.Errorf("%s: started = false, want true", action)
		}
		if !strings.Contains(res.Message, "background") {
			t.Errorf("%s: reply must say the run is asynchronous, got %q", action, res.Message)
		}
	}
}

// A run already in flight is the work the caller asked for, so it is reported
// as a non-started success rather than an error the agent would retry.
func TestSearchIndexReportsAnInFlightRunAsSuccess(t *testing.T) {
	b := &fakeIndexBridge{roots: []string{`C:\repo`}, started: false}
	res, err := callIndexTool(t, b, `{"action":"refresh"}`)
	if err != nil {
		t.Fatalf("in-flight run reported as an error: %v", err)
	}
	if res.Started {
		t.Error("started = true for a run that was already in flight")
	}
	if !strings.Contains(res.Message, "already in flight") {
		t.Errorf("reply should explain the run is already happening, got %q", res.Message)
	}
}

// The root restriction is the reason this goes through TionHarness at all: an
// unrestricted root would let an agent aim a repository-wide re-embed anywhere
// on the machine.
func TestSearchIndexRejectsAForeignRoot(t *testing.T) {
	b := &fakeIndexBridge{roots: []string{`C:\repo`}}
	_, err := NewSearchIndexTool(b).Call(context.Background(), json.RawMessage(`{"action":"rebuild","root":"C:\\Users\\Bilal"}`))
	if err == nil {
		t.Fatal("a foreign root was accepted")
	}
	if b.calls != 0 {
		t.Error("a rejected root still reached the manager")
	}
	if !strings.Contains(err.Error(), "working root") {
		t.Errorf("error should explain the restriction, got %v", err)
	}
}

// The session's own root must still be accepted when named explicitly, in any
// spelling the host filesystem treats as the same path.
func TestSearchIndexAcceptsTheSessionRootInAnySpelling(t *testing.T) {
	for _, spelling := range []string{`C:\repo`, `c:\repo`, `C:/repo`, `C:\repo\`} {
		b := &fakeIndexBridge{roots: []string{`C:\repo`}, started: true}
		if _, err := callIndexTool(t, b, `{"action":"refresh","root":"`+strings.ReplaceAll(spelling, `\`, `\\`)+`"}`); err != nil {
			t.Errorf("%s was refused: %v", spelling, err)
		}
	}
}

func TestSearchIndexRejectsUnknownActionsIncludingDrop(t *testing.T) {
	// drop is deliberately absent: deleting an index is destructive and stays a
	// user action behind the Settings confirmation gate.
	for _, action := range []string{"drop", "delete", "create", "nonsense"} {
		b := &fakeIndexBridge{roots: []string{`C:\repo`}}
		_, err := NewSearchIndexTool(b).Call(context.Background(), json.RawMessage(`{"action":"`+action+`"}`))
		if err == nil {
			t.Errorf("action %q was accepted", action)
		}
		if b.calls != 0 {
			t.Errorf("action %q reached the manager", action)
		}
	}
}

// The tool's schema must not advertise an action it refuses, or the model will
// keep trying it.
func TestSearchIndexSchemaOffersOnlyTheThreeActions(t *testing.T) {
	def := SearchIndexTool{}.Def()
	if def.Name != "search_index" {
		t.Errorf("name = %q", def.Name)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"status"`, `"refresh"`, `"rebuild"`} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema missing action %s", want)
		}
	}
	if strings.Contains(schema, `"drop"`) {
		t.Error("schema advertises drop")
	}
	var parsed map[string]any
	if err := json.Unmarshal(def.InputSchema, &parsed); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
}

func TestSearchIndexWithoutARootHasNothingToActOn(t *testing.T) {
	b := &fakeIndexBridge{roots: nil}
	if _, err := NewSearchIndexTool(b).Call(context.Background(), json.RawMessage(`{"action":"status"}`)); err == nil {
		t.Fatal("a session with no working root should not resolve a target")
	}
}
