package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// ReadLessonsTool lets an agent list the workspace's auto-collected failure
// lessons (self-healing, hata→ders döngüsü) beyond the newest-5 block injected
// into its context — including each lesson's id, so a stale or wrong lesson
// can be pruned with delete_lesson.
type ReadLessonsTool struct{ db *db.DB }

// NewReadLessonsTool binds the tool to a workspace DB.
func NewReadLessonsTool(database *db.DB) ReadLessonsTool {
	return ReadLessonsTool{db: database}
}

func (ReadLessonsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "read_lessons",
		Description: "List this workspace's auto-collected failure lessons (distilled from past failed " +
			"turns), newest first, with their ids. The newest few already ride your context as 'Lessons " +
			"from past failures'; use this to see the full set — e.g. before deleting a stale lesson " +
			"with delete_lesson, or to review recurring failure shapes (count = times seen).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "limit": { "type": "integer", "description": "Max lessons to return, newest first (default 20, 0 = all)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ReadLessonsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Limit *int `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	// Omitted → default 20. An explicit 0 (or negative) means "all", matching
	// the documented contract and ListLessons' limit<=0 semantics. Using a
	// pointer is the only way to tell an omitted limit from an explicit 0.
	limit := 20
	if args.Limit != nil {
		limit = *args.Limit
	}
	if limit < 0 {
		limit = 0
	}
	lessons, err := t.db.ListLessons(limit)
	if err != nil {
		return "", err
	}
	if len(lessons) == 0 {
		return "No failure lessons recorded in this workspace yet.", nil
	}
	now := time.Now().Unix()
	var b strings.Builder
	fmt.Fprintf(&b, "%d failure lesson(s), newest first:\n", len(lessons))
	for _, l := range lessons {
		fmt.Fprintf(&b, "\n- id: %s", l.ID)
		if l.Tool != "" {
			fmt.Fprintf(&b, " · tool: %s", l.Tool)
		}
		if l.Count > 1 {
			fmt.Fprintf(&b, " · seen %d times", l.Count)
		}
		fmt.Fprintf(&b, " · last: %s ago\n  %s\n", lessonAge(now-l.Time), strings.TrimSpace(l.Text))
	}
	return strings.TrimSpace(b.String()), nil
}

// DeleteLessonTool prunes one failure lesson by id — for a lesson that is
// stale (the underlying cause was fixed) or wrong. If the same failure shape
// recurs, the reflector records a fresh lesson.
type DeleteLessonTool struct{ db *db.DB }

// NewDeleteLessonTool binds the tool to a workspace DB.
func NewDeleteLessonTool(database *db.DB) DeleteLessonTool {
	return DeleteLessonTool{db: database}
}

func (DeleteLessonTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "delete_lesson",
		Description: "Delete one auto-collected failure lesson by id (get ids from read_lessons). Use " +
			"when a lesson is stale — its root cause was fixed — or wrong; it stops being injected into " +
			"future turns. If the same failure recurs, a fresh lesson is recorded automatically.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "Lesson id to delete." }
  },
  "required": ["id"],
  "additionalProperties": false
}`),
	}
}

func (t DeleteLessonTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.ID) == "" {
		return "", fmt.Errorf("id is required")
	}
	if err := t.db.DeleteLesson(strings.TrimSpace(args.ID)); err != nil {
		return "", err
	}
	return "Lesson deleted. It will no longer be injected into future turns.", nil
}

// lessonAge renders a compact age like "3h" / "2d" for lesson listings.
func lessonAge(sec int64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh", sec/3600)
	default:
		return fmt.Sprintf("%dd", sec/86400)
	}
}
