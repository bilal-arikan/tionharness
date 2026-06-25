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
