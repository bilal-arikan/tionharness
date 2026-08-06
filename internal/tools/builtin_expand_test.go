package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// expandDB opens a scratch store with a two-column board for the drill-down tests.
func expandDB(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.CreateTask(ctx, db.Task{Title: "a", BoardState: db.BoardTodo}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := database.CreateTask(ctx, db.Task{Title: "b", BoardState: db.BoardInProgress}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return database
}

func callExpand(t *testing.T, database *db.DB, input string) (string, error) {
	t.Helper()
	return NewExpandTool(database).Call(context.Background(), json.RawMessage(input))
}

func TestExpandWorkspaceListsElevenBuckets(t *testing.T) {
	out, err := callExpand(t, expandDB(t), `{"kind":"workspace","id":"workspace"}`)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !strings.Contains(out, "CHILDREN of workspace:workspace") || !strings.Contains(out, "· 11") {
		t.Errorf("workspace should expand into 11 buckets:\n%s", out)
	}
	// The category buckets are themselves expandable; the hint must steer to expand.
	for _, want := range []string{`expand{kind:"category",id:"sessions"}`, `expand{kind:"board",id:"board"}`, `get_view{kind:"budget"`, `get_view{kind:"logs"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing hint %q:\n%s", want, out)
		}
	}
}

func TestExpandBoardListsColumns(t *testing.T) {
	out, err := callExpand(t, expandDB(t), `{"kind":"board","id":"board"}`)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// Two columns exist (todo, in_progress); each is an expandable category node.
	if !strings.Contains(out, "· 2") {
		t.Errorf("board should expand into 2 columns:\n%s", out)
	}
	if !strings.Contains(out, `expand{kind:"category",id:"col:in_progress"}`) {
		t.Errorf("column child hint missing:\n%s", out)
	}
}

func TestExpandSingletonIdDefaulting(t *testing.T) {
	// An omitted id for the workspace/board singletons is filled in, not rejected.
	out, err := callExpand(t, expandDB(t), `{"kind":"workspace","id":""}`)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !strings.Contains(out, "· 11") {
		t.Errorf("empty workspace id should default to the singleton:\n%s", out)
	}
}

func TestExpandLeafReturnsEmpty(t *testing.T) {
	// A category column with no matching cards is a valid empty result, not an error.
	out, err := callExpand(t, expandDB(t), `{"kind":"category","id":"col:review"}`)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !strings.Contains(out, "· 0") {
		t.Errorf("an empty column should report zero children:\n%s", out)
	}
}

func TestExpandUnknownKindIsError(t *testing.T) {
	if _, err := callExpand(t, expandDB(t), `{"kind":"galaxy","id":"x"}`); err == nil {
		t.Error("an unsupported kind must be an error, not an empty list")
	}
}

func TestExpandUnknownCategoryIsError(t *testing.T) {
	if _, err := callExpand(t, expandDB(t), `{"kind":"category","id":"nope"}`); err == nil {
		t.Error("an unknown category id must be an error")
	}
}
