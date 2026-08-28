package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// handlePublishMarket packages an existing entity into the workspace tier of the
// registry. MVP: skill kind (by slug). Other kinds return 501.
func (s *Server) handlePublishMarket(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	wsp := ws(r)
	switch req.Kind {
	case market.KindSkill:
		sk, ok := wsp.Runtime.Skills().Get(req.SourceID)
		if !ok || sk.Path == "" {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		// Carry the full SKILL.md verbatim (frontmatter + body) for a lossless
		// reinstall — read the backing file directly.
		raw, err := os.ReadFile(sk.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Collect bundled resource files (nested dirs) so publish is lossless too.
		files := collectSkillFiles(filepath.Dir(sk.Path))
		pack, err := market.BuildSkillPack(sk.Slug, sk.Name, sk.Description, sk.Icon, sk.Color, string(raw), "", 0, files)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stored, err := wsp.Runtime.Market().Publish(pack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stored)
	case market.KindWorkspace:
		// Capture the ACTIVE workspace's current state into a portable template
		// pack (the inverse of seeding). SourceID is ignored — the workspace scope
		// comes from the X-Workspace-Id header.
		payload, err := s.buildWorkspaceTemplatePayload(r.Context(), wsp, req.Include)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(payload.Agents) == 0 {
			writeError(w, http.StatusBadRequest, "şablonlanacak ajan yok — en az bir ajan seçilmeli")
			return
		}
		cfg := wsp.Settings()
		// Pack metadata: honour the user-supplied name/description/version, falling
		// back to derived defaults when a field is left blank.
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = wsp.Name
		}
		desc := strings.TrimSpace(req.Description)
		if desc == "" {
			desc = "“" + wsp.Name + "” workspace'inden dışa aktarıldı."
		}
		slug := slugify(name)
		if slug == "" {
			slug = strings.ToLower(wsp.ID)
		}
		pack, err := market.BuildWorkspacePack(slug, name, desc, strings.TrimSpace(req.Version), cfg.Icon, cfg.Color, "", 0, payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stored, err := wsp.Runtime.Market().Publish(pack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// The template picker reads the server-level market store (separate Store
		// instance over the same global dir); reload it so the freshly published
		// template appears in the create picker immediately.
		s.market.Reload()
		writeJSON(w, http.StatusOK, stored)
	default:
		writeError(w, http.StatusNotImplemented, "publishing "+req.Kind+" is not yet supported")
	}
}

// buildWorkspaceTemplatePayload captures a live workspace's current state into a
// portable WorkspacePayload — the inverse of seedWorkspaceTeam: workspace-tier
// skills, the agent team (with stable local keys), flows (agent ids rewritten to
// "tmpl:<key>"), schedules and automations (wired by key/name), and identity/
// instructions/board layout. Secrets, sessions, artifacts and other runtime data
// are never included.
//
// inc selects which parts to capture. A nil pointer means "everything" (backward
// compatible with callers that don't select). When present, its id/slug slices
// filter agents/flows/skills/schedules/automations independently (nil slice = all,
// present slice = exactly its members) and its boolean flags gate the file
// categories.
func (s *Server) buildWorkspaceTemplatePayload(ctx context.Context, wsp *workspace.Workspace, inc *publishInclude) (market.WorkspacePayload, error) {
	all := inc == nil
	incInstructions := all || inc.Instructions
	incPrompts := all || inc.Prompts
	incBoard := all || inc.BoardColumns

	// Selection predicates. everything=true means no filtering (nil slice / whole
	// export); otherwise only ids present in the set pass.
	var (
		agentSet, flowSet, skillSet, schedSet, autoSet     map[string]bool
		allAgents, allFlows, allSkills, allSched, allAutos bool
	)
	agentSet, allAgents = wantSet(all, sliceOrNil(inc, func(i *publishInclude) []string { return i.AgentIDs }))
	flowSet, allFlows = wantSet(all, sliceOrNil(inc, func(i *publishInclude) []string { return i.FlowIDs }))
	skillSet, allSkills = wantSet(all, sliceOrNil(inc, func(i *publishInclude) []string { return i.SkillSlugs }))
	schedSet, allSched = wantSet(all, sliceOrNil(inc, func(i *publishInclude) []string { return i.ScheduleIDs }))
	autoSet, allAutos = wantSet(all, sliceOrNil(inc, func(i *publishInclude) []string { return i.AutomationIDs }))

	cfg := wsp.Settings()
	wp := market.WorkspacePayload{
		Name:  wsp.Name,
		Icon:  cfg.Icon,
		Color: cfg.Color,
	}
	if incInstructions {
		wp.Instructions = cfg.Instructions
	}
	if incBoard && len(cfg.BoardColumns) > 0 {
		wp.Columns = fromBoardColumnDefs(cfg.BoardColumns)
	}
	// Non-default runtime prompts + README (both editable config files). Only
	// prompts that differ from their compiled-in default are carried, so a fresh
	// install keeps the newest defaults for untouched keys.
	if incPrompts {
		prompts := map[string]string{}
		for _, key := range agent.PromptKeys {
			cur := readFileOr(agent.PromptFilePath(wsp.DataDir, key), "")
			if strings.TrimSpace(cur) != "" && strings.TrimSpace(cur) != strings.TrimSpace(agent.PromptDefault(key)) {
				prompts[key] = cur
			}
		}
		if len(prompts) > 0 {
			wp.Prompts = prompts
		}
		if rm := readFileOr(agent.ReadmeFilePath(wsp.DataDir), ""); strings.TrimSpace(rm) != "" {
			wp.Readme = rm
		}
	}

	// Agents → template agents with stable local keys (id → key map for wiring).
	agents, err := wsp.DB.ListAgents(ctx)
	if err != nil {
		return wp, err
	}
	idToKey := make(map[string]string, len(agents))
	usedKeys := map[string]bool{}
	for _, a := range agents {
		if !allAgents && !agentSet[a.ID] {
			continue // not selected for this export
		}
		key := uniqueAgentKey(a.Name, usedKeys)
		idToKey[a.ID] = key
		wp.Agents = append(wp.Agents, market.WorkspaceTemplateAgent{
			Key: key, Name: a.Name, Soul: a.Soul, Identity: a.Identity,
			Provider: a.Provider, Model: a.Model,
			ThinkingLevel: a.ThinkingLevel, PermissionMode: a.PermissionMode,
			NativeWebSearch: a.NativeWebSearch,
			Avatar:          a.Avatar, Color: a.Color,
			MCPEnabled: a.MCPEnabled, AllowedTools: a.AllowedTools, BlockedTools: a.BlockedTools,
			ToolOverrides: a.ToolOverrides,
			Skills:        a.Skills,
			// Without these a published team loses its orchestration shape: the agents
			// come back as plain chat agents and the user has to rediscover which one
			// was the coordinator.
			CoordinatorMode:     a.CoordinatorMode,
			CoordinatorWorkflow: a.CoordinatorWorkflow,
			CoordinatorPrompt:   a.CoordinatorPrompt,
		})
	}

	// Flows → rewrite agent node ids to "tmpl:<key>" so the graph is portable.
	flows, ferr := wsp.DB.ListFlows(ctx)
	if ferr != nil {
		return wp, ferr
	}
	flowIDToName := make(map[string]string, len(flows))
	for _, f := range flows {
		if !allFlows && !flowSet[f.ID] {
			continue // not selected for this export
		}
		graph, perr := orchestration.ParseGraph(f.Graph)
		if perr != nil {
			continue // skip an unparseable flow rather than failing the whole export
		}
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.Type == orchestration.NodeAgent && n.AgentID != "" {
				if key, ok := idToKey[n.AgentID]; ok {
					n.AgentID = market.TemplateAgentKeyPrefix + key
				}
			}
		}
		raw, merr := json.Marshal(graph)
		if merr != nil {
			continue
		}
		wp.Flows = append(wp.Flows, market.WorkspaceTemplateFlow{
			Name: f.Name, Graph: string(raw),
		})
		flowIDToName[f.ID] = f.Name
	}

	// Schedules → reference the agent by key (skip orphans).
	scheds, serr := wsp.DB.ListSchedules(ctx)
	if serr != nil {
		return wp, serr
	}
	for _, sc := range scheds {
		if !allSched && !schedSet[sc.ID] {
			continue // not selected for this export
		}
		if sc.OneShot {
			continue // a spent/pending wake has no cron — exporting it yields a broken row
		}
		key, ok := idToKey[sc.AgentID]
		if !ok {
			continue // agent not in the exported team → drop the orphan
		}
		wp.Schedules = append(wp.Schedules, market.WorkspaceTemplateSchedule{
			Name: sc.Name, AgentKey: key, CronExpr: sc.CronExpr, Prompt: sc.Prompt,
		})
	}

	// Automations → reference the agent by key / the flow by name (skip orphans).
	// The built-in seeded board defaults are excluded: every workspace provisions
	// its own copy at open time (EnsureDefaultAutomations), so exporting them
	// would either duplicate the rule on install or resurrect one the installing
	// user had deliberately deleted.
	autos, aerr := wsp.DB.ListAutomations(ctx)
	if aerr != nil {
		return wp, aerr
	}
	for _, a := range autos {
		if !allAutos && !autoSet[a.ID] {
			continue // not selected for this export
		}
		if a.Seed != "" {
			continue // built-in default, provisioned per workspace
		}
		agentKey := ""
		if a.TargetAgentID != "" {
			key, ok := idToKey[a.TargetAgentID]
			if !ok {
				continue // agent not in the exported team → drop the orphan
			}
			agentKey = key
		}
		flowName := ""
		if a.FlowID != "" {
			name, ok := flowIDToName[a.FlowID]
			if !ok {
				continue // flow not in the export → drop the orphan
			}
			flowName = name
		}
		if agentKey == "" && flowName == "" && a.BoardAction != db.BoardActionArchive {
			continue // nothing to run
		}
		wp.Automations = append(wp.Automations, market.WorkspaceTemplateAutomation{
			Name:            a.Name,
			TriggerKind:     a.TriggerKind,
			TriggerTag:      a.TriggerTag,
			BoardOp:         a.BoardOp,
			BoardFromState:  a.BoardFromState,
			BoardToState:    a.BoardToState,
			BoardPriority:   a.BoardPriority,
			BoardExclusive:  a.BoardExclusive,
			BoardAction:     a.BoardAction,
			TokenScope:      a.TokenScope,
			TokenThreshold:  a.TokenThreshold,
			CounterMetric:   a.CounterMetric,
			CounterScope:    a.CounterScope,
			CounterInterval: a.CounterInterval,
			AgentKey:        agentKey,
			FlowName:        flowName,
			SessionMode:     a.SessionMode,
			PromptTemplate:  a.PromptTemplate,
			SpawnTags:       a.SpawnTags,
			MaxIterations:   a.MaxIterations,
			CooldownSec:     a.CooldownSec,
		})
	}

	// Workspace-tier skills → embed verbatim (SKILL.md + nested files) so the
	// template is self-contained. Global/bundled skills are shared, not exported.
	skillList := wsp.Runtime.Skills().List()
	for _, sk := range skillList {
		if sk.Source != skills.SourceWorkspace || sk.Path == "" {
			continue
		}
		if !allSkills && !skillSet[sk.Slug] {
			continue // not selected for this export
		}
		body, rerr := os.ReadFile(sk.Path)
		if rerr != nil {
			continue
		}
		wp.Skills = append(wp.Skills, market.WorkspaceTemplateSkill{
			Slug: sk.Slug, Body: string(body), Files: collectSkillFiles(filepath.Dir(sk.Path)),
		})
	}

	return wp, nil
}

// sliceOrNil returns pick(inc) when inc is non-nil, else nil. It lets the caller
// read a selection slice off a possibly-nil *publishInclude without a nil check
// at every call site (a nil pointer means "whole export").
func sliceOrNil(inc *publishInclude, pick func(*publishInclude) []string) []string {
	if inc == nil {
		return nil
	}
	return pick(inc)
}

// wantSet turns a selection slice into a membership set + an "everything" flag.
// all=true (whole export) or a nil slice means everything (set is nil, flag true);
// a non-nil slice — even empty — restricts to exactly its members (empty ⇒ none).
func wantSet(all bool, ids []string) (map[string]bool, bool) {
	if all || ids == nil {
		return nil, true
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, false
}

// fromBoardColumnDefs is the inverse of toBoardColumnDefs: db board columns → the
// market envelope's dependency-free BoardColumn list, for publishing a template.
func fromBoardColumnDefs(in []db.BoardColumnDef) []market.BoardColumn {
	out := make([]market.BoardColumn, 0, len(in))
	for _, c := range in {
		out = append(out, market.BoardColumn{Key: c.Key, Label: c.Label, Color: c.Color})
	}
	return out
}

// uniqueAgentKey derives a stable, unique local key for a template agent from its
// name, deduping with a numeric suffix. Used to wire flows/schedules by key.
func uniqueAgentKey(name string, used map[string]bool) string {
	base := slugify(name)
	if base == "" {
		base = "agent"
	}
	key := base
	for i := 2; used[key]; i++ {
		key = fmt.Sprintf("%s-%d", base, i)
	}
	used[key] = true
	return key
}

// slugify lowercases a string and keeps only [a-z0-9], collapsing runs of other
// characters to single dashes. Non-ASCII (e.g. Turkish) letters are dropped, so
// callers fall back to another id when the result is empty.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// collectSkillFiles reads a skill folder's bundled resource files (every file
// except SKILL.md, nested dirs included) keyed by forward-slashed relative path, so
// publish carries them into the pack. Returns nil when only SKILL.md is present.
func collectSkillFiles(dir string) map[string][]byte {
	files := map[string][]byte{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil || strings.EqualFold(rel, "SKILL.md") {
			return nil
		}
		if b, rderr := os.ReadFile(p); rderr == nil {
			files[filepath.ToSlash(rel)] = b
		}
		return nil
	})
	if len(files) == 0 {
		return nil
	}
	return files
}

// handleImportMarket stores a raw pasted HarnessPack JSON into the registry.
func (s *Server) handleImportMarket(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	stored, err := ws(r).Runtime.Market().Import(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stored)
}
