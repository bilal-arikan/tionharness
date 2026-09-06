package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// Configuration snapshot provider (_Docs/83 §4.2, E1). BuildSnapshot reads
// every optimizable surface into a goals.ConfigSnapshot; CurrentSnapshotHash
// caches the result briefly and persists a newly seen snapshot, and is what
// the store calls to stamp each new session. Cheap: everything but the prompt
// override files is already in memory.

const snapshotCacheTTL = 2 * time.Second

type snapshotCache struct {
	mu   sync.Mutex
	hash string
	at   time.Time
}

var snapshotCaches sync.Map // *Runtime → *snapshotCache

func (r *Runtime) snapshotCache() *snapshotCache {
	v, _ := snapshotCaches.LoadOrStore(r, &snapshotCache{})
	return v.(*snapshotCache)
}

// CurrentSnapshotHash returns the hash of the configuration in force, saving
// the snapshot the first time a new hash appears.
func (r *Runtime) CurrentSnapshotHash() string {
	if r == nil || r.db == nil {
		return ""
	}
	c := r.snapshotCache()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hash != "" && time.Since(c.at) < snapshotCacheTTL {
		return c.hash
	}
	snap, err := r.BuildSnapshot(context.Background())
	if err != nil {
		return c.hash
	}
	if snap.Hash != c.hash {
		if err := r.db.SaveSnapshot(context.Background(), snap.Hash, snap, time.Now().Unix()); err != nil {
			r.logger.Warn("snapshot: save failed", "hash", snap.Hash, "error", err)
		}
	}
	c.hash, c.at = snap.Hash, time.Now()
	return c.hash
}

// BuildSnapshot captures the current configuration.
func (r *Runtime) BuildSnapshot(ctx context.Context) (goals.ConfigSnapshot, error) {
	snap := goals.ConfigSnapshot{
		At:          time.Now().Unix(),
		Agents:      map[string]goals.AgentGenome{},
		Recipes:     map[string]string{},
		Automations: map[string]goals.AutomationGenome{},
		Schedules:   map[string]goals.ScheduleGenome{},
		Prompts:     map[string]string{},
		Models:      map[string]string{},
		Tools:       goals.ToolsGenome{Disabled: []string{}, Visibility: map[string]string{}, MCPServers: map[string]bool{}},
	}
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return snap, err
	}
	for _, a := range agents {
		// Locked built-ins are re-imposed from code every boot; only the
		// user's agents and role customizations are part of the genome.
		if a.Locked {
			continue
		}
		snap.Agents[a.ID] = agentGenome(a)
	}
	if tc, err := r.db.GetWorkspaceToolConfig(ctx); err == nil {
		snap.Tools.Disabled = append(snap.Tools.Disabled, tc.DisabledTools...)
		for k, v := range tc.ToolVisibility {
			snap.Tools.Visibility[k] = v
		}
	}
	if servers, err := r.db.ListMCPServers(ctx); err == nil {
		for _, s := range servers {
			snap.Tools.MCPServers[s.ID] = s.Enabled
		}
	}
	if r.skills != nil {
		for _, sk := range r.skills.List() {
			if sk.Recipe != nil {
				snap.Recipes[sk.Slug] = sk.Recipe.Version
			}
		}
	}
	if autos, err := r.db.ListAutomations(ctx); err == nil {
		for _, a := range autos {
			if !a.Enabled || a.Archived {
				continue
			}
			snap.Automations[a.ID] = automationGenome(a)
		}
	}
	if scheds, err := r.db.ListSchedules(ctx); err == nil {
		for _, s := range scheds {
			if !s.Enabled || s.Archived {
				continue
			}
			snap.Schedules[s.ID] = goals.ScheduleGenome{Name: s.Name, CronExpr: s.CronExpr, AgentID: s.AgentID, FlowID: s.FlowID}
		}
	}
	if dir := r.configDir(); dir != "" {
		for _, key := range prompts.Keys() {
			raw, err := os.ReadFile(PromptFilePath(filepath.Dir(r.workDir), key))
			if err != nil {
				continue
			}
			snap.Prompts[key] = goals.HashText(string(raw))
		}
	}
	instructions := ""
	if p := r.instructions.Load(); p != nil {
		instructions = *p
	}
	snap.Settings = goals.SettingsGenome{
		TerseMode:        r.TerseModeEnabled(),
		InstructionsHash: goals.HashText(instructions),
		PromptEpoch:      r.promptEpochEnabled.Load(),
	}
	for k, res := range r.db.ModelResolutions(ctx) {
		snap.Models[k] = res.Resolved
	}
	if err := snap.Finalize(); err != nil {
		return snap, err
	}
	return snap, nil
}

func agentGenome(a db.Agent) goals.AgentGenome {
	return goals.AgentGenome{
		Name:                a.Name,
		SystemKey:           a.SystemKey,
		ParentID:            a.ParentID,
		SoulHash:            goals.HashText(a.Soul),
		SoulChars:           len([]rune(a.Soul)),
		IdentityHash:        goals.HashText(a.Identity),
		Provider:            a.Provider,
		ProviderInstanceID:  a.ProviderInstanceID,
		Model:               a.Model,
		ThinkingLevel:       a.ThinkingLevel,
		NativeWebSearch:     a.NativeWebSearch,
		PermissionMode:      a.PermissionMode,
		InboundPolicy:       a.InboundPolicy,
		MCPEnabled:          a.MCPEnabled,
		ToolOverridesHash:   goals.HashText(a.ToolOverrides),
		VisibleToolCount:    -1,
		AllowedToolsHash:    goals.HashText(a.AllowedTools),
		Skills:              append([]string{}, a.Skills...),
		CoordinatorMode:     a.CoordinatorMode,
		CoordinatorWorkflow: a.CoordinatorWorkflow,
		CoordinatorPrompt:   goals.HashText(a.CoordinatorPrompt),
		Disabled:            a.Disabled,
	}
}

func automationGenome(a db.Automation) goals.AutomationGenome {
	trigger := goals.HashText(strings.Join([]string{
		a.TriggerKind, a.TokenScope, itoa(a.TokenThreshold),
		a.TrajPhase, a.TrajRecipe, a.TrajEvent, a.TrajStatus, a.TriggerTag,
		a.BoardOp, a.BoardFromState, a.BoardToState, itoa(a.BoardPriority), fmtBool(a.BoardExclusive), a.BoardAction, a.BoardMoveToState,
	}, "|"))
	return goals.AutomationGenome{
		Name: a.Name, TriggerKind: a.TriggerKind, TriggerHash: trigger,
		TargetAgentID: a.TargetAgentID, FlowID: a.FlowID, SessionMode: a.SessionMode,
		PromptHash: goals.HashText(a.PromptTemplate), MaxIterations: a.MaxIterations, CooldownSec: a.CooldownSec,
	}
}

func fmtBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
