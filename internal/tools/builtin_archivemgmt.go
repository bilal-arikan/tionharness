package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// set_archived is the agent-facing half of the shared entity archive (the REST
// pair POST /api/<entity>/{id}/archive|unarchive). One tool covers every
// entity that mirrors the kanban card's archive — agents, skills, artifacts,
// and automations — because the operation, its arguments and its result
// are identical across them; only the store call differs. Four near-copies
// would multiply the tool surface an agent has to scan for no extra
// expressiveness. Kanban cards keep their own set_archived_task (it predates
// this tool and also notifies the board).
//
// Every kind delegates to the same store function the REST routes call, so
// refusals (a system agent cannot be archived) are identical on both paths.

// Archivable entity kinds accepted by set_archived.
const (
	ArchiveKindAgent      = "agent"
	ArchiveKindSkill      = "skill"
	ArchiveKindArtifact   = "artifact"
	ArchiveKindAutomation = "automation"
)

var archiveKinds = []string{ArchiveKindAgent, ArchiveKindSkill, ArchiveKindArtifact, ArchiveKindAutomation}

// SkillArchiver archives or restores a workspace skill by slug. It is injected
// (rather than the tool importing the skills package) like SkillWriter; nil
// means the runtime has no skill store and kind "skill" is refused.
type SkillArchiver func(slug string, archived bool) error

type SetArchivedTool struct {
	db     *db.DB
	skills SkillArchiver
}

func NewSetArchivedTool(database *db.DB, skills SkillArchiver) SetArchivedTool {
	return SetArchivedTool{db: database, skills: skills}
}

func (SetArchivedTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_archived",
		Description: "Archive or restore an agent, skill, artifact or automation. archived=true hides it from default lists " +
			"(list_agents/list_artifacts/list_automations show it only with archived:true) while keeping its configuration and " +
			"history; archived=false restores it. Reversible, unlike the delete_* tools. An archived agent cannot run, an archived " +
			"automation does not fire, an archived skill is not offered to agents. System (built-in) agents cannot be archived. For kanban cards use set_archived_task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"kind":{"type":"string","enum":["agent","skill","artifact","automation"],"description":"Entity type"},
				"id":{"type":"string","description":"Entity id (for kind=skill: the skill slug)"},
				"archived":{"type":"boolean","description":"true to archive, false to restore"}
			},
			"required":["kind","id","archived"],
			"additionalProperties":false
		}`),
	}
}

func (t SetArchivedTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		Archived *bool  `json:"archived"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.ID = strings.TrimSpace(in.ID)
	if !slices.Contains(archiveKinds, in.Kind) {
		return "", enumErr("kind", in.Kind, archiveKinds...)
	}
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if in.Archived == nil {
		return "", fmt.Errorf("archived is required")
	}
	if err := t.set(ctx, in.Kind, in.ID, *in.Archived); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", fmt.Errorf("no %s with id %q (use the matching list tool)", in.Kind, in.ID)
		}
		return "", fmt.Errorf("set %s archived: %w", in.Kind, err)
	}
	b, _ := json.Marshal(map[string]any{"kind": in.Kind, "id": in.ID, "archived": *in.Archived})
	return string(b), nil
}

// set routes one archive toggle to the entity's store function — the same one
// the REST archive routes use (internal/api/archive_entities.go).
func (t SetArchivedTool) set(ctx context.Context, kind, id string, archived bool) error {
	switch kind {
	case ArchiveKindAgent:
		_, err := t.db.SetAgentArchived(ctx, id, archived)
		return err
	case ArchiveKindArtifact:
		_, err := t.db.SetArtifactArchived(ctx, id, archived)
		return err
	case ArchiveKindAutomation:
		return t.db.SetAutomationArchived(ctx, id, archived)
	case ArchiveKindSkill:
		if t.skills == nil {
			return fmt.Errorf("no skill store is available in this runtime")
		}
		return t.skills(id, archived)
	}
	return enumErr("kind", kind, archiveKinds...)
}
