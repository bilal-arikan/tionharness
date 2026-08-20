package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

type recordingChangeNotifier struct {
	board []BoardChange
}

func (n *recordingChangeNotifier) BoardChanged(_ *db.DB, change BoardChange) {
	n.board = append(n.board, change)
}

func (*recordingChangeNotifier) AgentModelChanged(_ *db.DB, _ AgentModelChange) {}

func TestCreateTaskPublishesBoardChange(t *testing.T) {
	notifier := &recordingChangeNotifier{}
	SetChangeNotifier(notifier)
	t.Cleanup(func() { SetChangeNotifier(nil) })

	tool := NewCreateTaskTool(openTestDB(t), "actor-1")
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"title":"Ship it","boardState":"review"}`)); err != nil {
		t.Fatalf("create_task: %v", err)
	}
	if len(notifier.board) != 1 {
		t.Fatalf("board notifications = %d, want 1", len(notifier.board))
	}
	got := notifier.board[0]
	if got.Title != "Görev oluşturuldu: Ship it" || got.Body != "review" || got.TaskID == "" || got.Op != "create" {
		t.Fatalf("unexpected board notification: %+v", got)
	}
}
