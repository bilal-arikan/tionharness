package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// maxSessionTitleLen bounds a session title so it stays a label, not a document.
const maxSessionTitleLen = 200

// maxSessionGoalLen bounds a goal so it can never blow up the context window
// (it is re-injected into every turn). Mirrors the HTTP handler's limit.
const maxSessionGoalLen = 2000

// UpdateSessionTool mutates THIS session's metadata in one call: rename it, set its
// working directory, set/complete its persistent goal, edit its tags, or archive it.
// It replaces the former one-field-each set of tools (set_session_title /
// set_working_dir / archive_session / set_session_goal / complete_goal /
// set_session_tags) — the same shape as the entity update_* tools (id + only the
// fields to change). Every field is optional; only the ones supplied are applied.
//
// It reads its sink from the SAME context key as the session-edit tools
// (sessionFrom → SessionSink), which is a superset of GoalSink, so one attach
// covers title/working-dir/archive/tags AND goal. No-op (graceful message) without
// a session sink, i.e. an autonomous run with no bound session.
type UpdateSessionTool struct{}

// NewUpdateSessionTool constructs the update_session tool.
func NewUpdateSessionTool() UpdateSessionTool { return UpdateSessionTool{} }

func (UpdateSessionTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_session",
		Description: "Edit THIS chat session's metadata — pass only the fields you want to change " +
			"(like the update_* tools take an id + changed fields). Fields: `title` (sidebar label); " +
			"`working_dir` (cwd for file/shell tools, empty string resets to the workspace default, " +
			"takes effect next turn); `goal` (the session's persistent north-star objective, injected " +
			"into every turn); `goal_done` (mark the current goal achieved so it stops steering turns); " +
			"`tags` (replace the whole tag set) or `add`/`remove` (incremental); `archive` (true drops " +
			"the session out of the active list once its work is finished); `refresh_context` (true drops " +
			"this session's frozen prompt snapshot so the next turn recomposes tools/skills/instructions " +
			"from live state — use after you know your static context is stale). Tags are shared with the UI " +
			"and drive tag-triggered automations. Use get_session_info to read the current values first.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title":       { "type": "string", "description": "New sidebar title (short label, max 200 chars)." },
    "working_dir": { "type": "string", "description": "Absolute path to an existing directory, or empty string to reset to the workspace default. Takes effect next turn." },
    "goal":        { "type": "string", "description": "Set the session's persistent objective (max 2000 chars); overwrites the current goal." },
    "goal_done":   { "type": "boolean", "description": "Mark the current goal as achieved (it stays visible but stops steering future turns)." },
    "tags":        { "type": "array", "items": { "type": "string" }, "description": "Replace all tags with this exact set." },
    "add":         { "type": "array", "items": { "type": "string" }, "description": "Tags to add (kept alongside existing ones)." },
    "remove":      { "type": "array", "items": { "type": "string" }, "description": "Tags to remove." },
    "archive":     { "type": "boolean", "description": "Set true to archive this session (do not archive one that still has open work)." },
    "refresh_context": { "type": "boolean", "description": "Set true to drop this session's frozen prompt snapshot; the next turn recomposes the static context (tools/skills/instructions) from live state." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"title":"Refactor tool registry"}`),
			json.RawMessage(`{"goal":"Ship the merged update_session tool with green tests"}`),
			json.RawMessage(`{"add":["loop"]}`),
			json.RawMessage(`{"working_dir":"C:\\Users\\user\\Desktop\\Projects\\TionSwarm"}`),
			json.RawMessage(`{"goal_done":true,"archive":true}`),
		},
	}
}

func (UpdateSessionTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Title      *string   `json:"title"`
		WorkingDir *string   `json:"working_dir"`
		Goal       *string   `json:"goal"`
		GoalDone   *bool     `json:"goal_done"`
		Tags       *[]string `json:"tags"`
		Add        []string  `json:"add"`
		Remove     []string  `json:"remove"`
		Archive    *bool     `json:"archive"`
		RefreshCtx *bool     `json:"refresh_context"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("update_session", err)
	}

	// Validate every supplied field BEFORE mutating anything, so a bad value can't
	// leave the session half-updated.
	var title string
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
		if title == "" {
			return "", fmt.Errorf("title is empty; omit the field to leave the title unchanged")
		}
		if len([]rune(title)) > maxSessionTitleLen {
			return "", fmt.Errorf("title too long (max %d chars)", maxSessionTitleLen)
		}
	}
	var goal string
	if in.Goal != nil {
		goal = strings.TrimSpace(*in.Goal)
		if goal == "" {
			return "", fmt.Errorf("goal is empty; omit the field to leave the goal unchanged")
		}
		if len([]rune(goal)) > maxSessionGoalLen {
			return "", fmt.Errorf("goal too long (max %d chars)", maxSessionGoalLen)
		}
	}
	var dir string
	if in.WorkingDir != nil {
		// A non-empty path must exist and be a directory — a bad cwd would break the
		// fs/shell tools silently. Empty resets to the workspace default (no check).
		dir = strings.TrimSpace(*in.WorkingDir)
		if dir != "" {
			info, err := os.Stat(dir)
			if err != nil {
				return "", fmt.Errorf("path does not exist or is not accessible: %s", dir)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("path is not a directory: %s", dir)
			}
		}
	}
	if in.Tags != nil && (len(in.Add) > 0 || len(in.Remove) > 0) {
		return "", fmt.Errorf("pass either `tags` (full replacement) or `add`/`remove` (incremental), not both")
	}

	sink := sessionFrom(ctx)
	if sink == nil {
		return "no session is available to edit for this turn", nil
	}

	var applied []string
	if in.Title != nil {
		if err := sink.SetTitle(ctx, title); err != nil {
			return "", err
		}
		applied = append(applied, "title set to: "+title)
	}
	if in.WorkingDir != nil {
		if err := sink.SetWorkingDir(ctx, dir); err != nil {
			return "", err
		}
		if dir == "" {
			applied = append(applied, "working directory reset to the workspace default (takes effect next turn)")
		} else {
			applied = append(applied, "working directory set to: "+dir+" (takes effect next turn)")
		}
	}
	if in.Goal != nil {
		prev, _ := sink.Goal(ctx)
		if err := sink.SetGoal(ctx, goal, false); err != nil {
			return "", err
		}
		msg := "goal set: " + goal
		if p := strings.TrimSpace(prev.Text); p != "" && p != goal {
			msg += " (replaced previous goal: " + goalClip(p) + ")"
		}
		applied = append(applied, msg)
	}
	// goal_done acts on the goal AFTER a set in the same call (mark the just-set goal
	// done), or on the pre-existing goal when no new goal is supplied.
	if in.GoalDone != nil && *in.GoalDone {
		cur, err := sink.Goal(ctx)
		if err != nil {
			return "", err
		}
		switch {
		case strings.TrimSpace(cur.Text) == "":
			applied = append(applied, "no goal is set; nothing to complete")
		case cur.Done:
			applied = append(applied, "goal was already marked complete")
		default:
			if err := sink.SetGoal(ctx, cur.Text, true); err != nil {
				return "", err
			}
			applied = append(applied, "goal marked complete (it stays visible but no longer steers future turns)")
		}
	}
	if in.Tags != nil || len(in.Add) > 0 || len(in.Remove) > 0 {
		var next []string
		if in.Tags != nil {
			next = *in.Tags
		} else {
			cur, err := sink.Tags(ctx)
			if err != nil {
				return "", err
			}
			next = applyTagDelta(cur, in.Add, in.Remove)
		}
		if err := sink.SetTags(ctx, next); err != nil {
			return "", err
		}
		applied = append(applied, "tags: "+strings.Join(next, ", "))
	}
	if in.Archive != nil && *in.Archive {
		if err := sink.Archive(ctx); err != nil {
			return "", err
		}
		applied = append(applied, "session archived (it stays available but leaves the active list)")
	}
	if in.RefreshCtx != nil && *in.RefreshCtx {
		if err := sink.RefreshContext(ctx); err != nil {
			return "", err
		}
		applied = append(applied, "context snapshot dropped: the next turn recomposes tools/skills/instructions from live state")
	}

	if len(applied) == 0 {
		return "no fields supplied; nothing changed", nil
	}
	return "session updated:\n- " + strings.Join(applied, "\n- "), nil
}

// goalClip caps a goal echo so a long replaced goal can't bloat the result.
func goalClip(s string) string {
	r := []rune(s)
	if len(r) <= 80 {
		return s
	}
	return strings.TrimSpace(string(r[:80])) + "…"
}

// applyTagDelta returns cur with add-ed tags appended (de-duplicated) and
// remove-d tags dropped, preserving order.
func applyTagDelta(cur, add, remove []string) []string {
	rm := make(map[string]struct{}, len(remove))
	for _, t := range remove {
		rm[strings.TrimSpace(t)] = struct{}{}
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cur)+len(add))
	appendTag := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, drop := rm[t]; drop {
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range cur {
		appendTag(t)
	}
	for _, t := range add {
		appendTag(t)
	}
	return out
}
