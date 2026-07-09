package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTodoSink struct {
	called bool
	items  []TodoSinkItem
	err    error
}

func (f *fakeTodoSink) SaveTodos(_ context.Context, todos []TodoSinkItem) error {
	f.called = true
	f.items = todos
	return f.err
}

func (f *fakeTodoSink) LoadTodos(_ context.Context) ([]TodoSinkItem, bool, error) {
	return f.items, len(f.items) > 0, nil
}

func TestTodoWritePersistsViaSink(t *testing.T) {
	sink := &fakeTodoSink{}
	ctx := WithTodoSink(context.Background(), sink)
	in := json.RawMessage(`{"todos":[{"content":"a","status":"completed","category":"tests","steps":["run go test"]},{"content":"b","status":"pending"}]}`)
	if _, err := (TodoWriteTool{}).Call(ctx, in); err != nil {
		t.Fatalf("call: %v", err)
	}
	if !sink.called || len(sink.items) != 2 || sink.items[0].Status != "completed" {
		t.Fatalf("sink not invoked correctly: %+v", sink)
	}
	if sink.items[0].Category != "tests" || len(sink.items[0].Steps) != 1 || sink.items[0].Steps[0] != "run go test" {
		t.Fatalf("rich feature fields not passed through: %+v", sink.items[0])
	}
}

func TestTodoWriteNoSinkIsNoOp(t *testing.T) {
	in := json.RawMessage(`{"todos":[{"content":"a","status":"pending"}]}`)
	// No sink attached → must still succeed (live UI checklist only).
	if _, err := (TodoWriteTool{}).Call(context.Background(), in); err != nil {
		t.Fatalf("call without sink: %v", err)
	}
}

// TestTodoWriteSetMergesStatuses: the compact `set` form flips statuses on the
// persisted list without resending item contents, persists the merged list back
// and returns it as JSON so the trace can render the checklist card.
func TestTodoWriteSetMergesStatuses(t *testing.T) {
	sink := &fakeTodoSink{items: []TodoSinkItem{
		{Content: "a", Status: "in_progress", Category: "tests"},
		{Content: "b", Status: "pending"},
		{Content: "c", Status: "pending"},
	}}
	ctx := WithTodoSink(context.Background(), sink)
	out, err := (TodoWriteTool{}).Call(ctx, json.RawMessage(`{"set":{"1":"completed","2":"in_progress"}}`))
	if err != nil {
		t.Fatalf("set call: %v", err)
	}
	if sink.items[0].Status != "completed" || sink.items[1].Status != "in_progress" || sink.items[2].Status != "pending" {
		t.Fatalf("statuses not merged: %+v", sink.items)
	}
	if sink.items[0].Category != "tests" {
		t.Fatalf("untouched fields must survive a set update: %+v", sink.items[0])
	}
	var res struct {
		Todos []TodoSinkItem `json:"todos"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || len(res.Todos) != 3 {
		t.Fatalf("set result must carry the merged full list as JSON, got %q (%v)", out, err)
	}
}

// TestTodoWriteSetErrors: invalid usage of the `set` form fails loudly.
func TestTodoWriteSetErrors(t *testing.T) {
	sink := &fakeTodoSink{items: []TodoSinkItem{{Content: "a", Status: "pending"}}}
	ctx := WithTodoSink(context.Background(), sink)
	cases := map[string]string{
		"out-of-range index": `{"set":{"5":"completed"}}`,
		"non-numeric index":  `{"set":{"x":"completed"}}`,
		"invalid status":     `{"set":{"1":"done"}}`,
		"both forms at once": `{"todos":[{"content":"a","status":"pending"}],"set":{"1":"completed"}}`,
	}
	for name, in := range cases {
		if _, err := (TodoWriteTool{}).Call(ctx, json.RawMessage(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// No sink → set cannot resolve the previous list.
	if _, err := (TodoWriteTool{}).Call(context.Background(), json.RawMessage(`{"set":{"1":"completed"}}`)); err == nil {
		t.Error("set without a sink must fail with a full-list hint")
	}
	// Sink present but no list persisted yet.
	empty := WithTodoSink(context.Background(), &fakeTodoSink{})
	if _, err := (TodoWriteTool{}).Call(empty, json.RawMessage(`{"set":{"1":"completed"}}`)); err == nil {
		t.Error("set before any full-list call must fail")
	}
}

func TestTodoWriteSinkErrorDoesNotFailTool(t *testing.T) {
	sink := &fakeTodoSink{err: errors.New("disk full")}
	ctx := WithTodoSink(context.Background(), sink)
	in := json.RawMessage(`{"todos":[{"content":"a","status":"pending"}]}`)
	out, err := (TodoWriteTool{}).Call(ctx, in)
	if err != nil {
		t.Fatalf("a sink failure must not fail the tool: %v", err)
	}
	if out == "" {
		t.Fatal("expected confirmation text despite sink error")
	}
}
