package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// maxSessionTitleLen bounds a session title so it stays a label, not a document.
const maxSessionTitleLen = 200

// --- set_session_title ---

// SetSessionTitleTool renames this session — the SAME title shown in the sidebar
// and the one the user can edit. Useful when the agent has done enough to name
// the work meaningfully (replacing an auto-generated or empty title). No-op
// without a session sink. Non-blocking.
type SetSessionTitleTool struct{}

// NewSetSessionTitleTool constructs the set_session_title tool.
func NewSetSessionTitleTool() SetSessionTitleTool { return SetSessionTitleTool{} }

func (SetSessionTitleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_session_title",
		Description: "Rename this chat session (the title shown in the sidebar). Use a short, " +
			"specific label that reflects the work — e.g. once the topic is clear, replace a vague " +
			"or auto-generated title. This is the same title the user can edit.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "The new title (short label, max 200 chars)." }
  },
  "required": ["title"],
  "additionalProperties": false
}`),
	}
}

func (SetSessionTitleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_session_title", err)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	if len([]rune(title)) > maxSessionTitleLen {
		return "", fmt.Errorf("title too long (max %d chars)", maxSessionTitleLen)
	}
	sink := sessionFrom(ctx)
	if sink == nil {
		return "no session is available to rename for this turn", nil
	}
	if err := sink.SetTitle(ctx, title); err != nil {
		return "", err
	}
	return "session renamed to: " + title, nil
}

// --- set_working_dir ---

// SetWorkingDirTool sets this session's working directory (cwd) for the built-in
// filesystem/shell tools — like `cd /path/to/project`. Relative paths then
// resolve here and the shell starts here. An empty path resets to the workspace
// default. The path is validated to exist as a directory. No-op without a sink.
type SetWorkingDirTool struct{}

// NewSetWorkingDirTool constructs the set_working_dir tool.
func NewSetWorkingDirTool() SetWorkingDirTool { return SetWorkingDirTool{} }

func (SetWorkingDirTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_working_dir",
		Description: "Set this session's working directory (cwd) for the file and shell tools — " +
			"like `cd` in a terminal. Relative paths then resolve under it and the shell starts there. " +
			"Pass an absolute path to an existing directory; pass an empty string to reset to the " +
			"workspace default. Takes effect from the next turn.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Absolute path to an existing directory, or empty to reset to the workspace default." }
  },
  "required": ["path"],
  "additionalProperties": false
}`),
	}
}

func (SetWorkingDirTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_working_dir", err)
	}
	dir := strings.TrimSpace(in.Path)
	// A non-empty path must exist and be a directory — a bad cwd would break the
	// fs/shell tools silently. Empty resets to the workspace default (no check).
	if dir != "" {
		info, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("path does not exist or is not accessible: %s", dir)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("path is not a directory: %s", dir)
		}
	}
	sink := sessionFrom(ctx)
	if sink == nil {
		return "no session is available to set a working directory for this turn", nil
	}
	if err := sink.SetWorkingDir(ctx, dir); err != nil {
		return "", err
	}
	if dir == "" {
		return "working directory reset to the workspace default (takes effect next turn)", nil
	}
	return "working directory set to: " + dir + " (takes effect next turn)", nil
}

// --- archive_session ---

// ArchiveSessionTool marks this session as archived: it drops out of the active
// session list and the cross-session context block but is never deleted (the user
// can still find it). Use when the work is finished. No-op without a sink.
type ArchiveSessionTool struct{}

// NewArchiveSessionTool constructs the archive_session tool.
func NewArchiveSessionTool() ArchiveSessionTool { return ArchiveSessionTool{} }

func (ArchiveSessionTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "archive_session",
		Description: "Archive this chat session once its work is finished: it drops out of the " +
			"active session list (and other agents' situational-awareness view) but is never deleted. " +
			"Takes no arguments. Do not archive a session that still has open work.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (ArchiveSessionTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	sink := sessionFrom(ctx)
	if sink == nil {
		return "no session is available to archive for this turn", nil
	}
	if err := sink.Archive(ctx); err != nil {
		return "", err
	}
	return "session archived (it stays available but leaves the active list)", nil
}
