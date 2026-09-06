package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// ConfigSnapshot is the canonical, content-addressed picture of every surface
// the evolution machinery may later mutate (_Docs/83 §4.2). Sessions are
// stamped with the hash of the snapshot in force when they were created, so
// fitness can be grouped "per configuration version" without a separate
// history table. Maps are used throughout because encoding/json writes map
// keys in sorted order, which makes the marshalled form canonical.
type ConfigSnapshot struct {
	Hash string `json:"hash"`
	At   int64  `json:"at"` // when this content was first seen

	Agents      map[string]AgentGenome      `json:"agents"`      // by agent id (non-system + role customizations)
	Tools       ToolsGenome                 `json:"tools"`       // workspace-wide tool config
	Recipes     map[string]string           `json:"recipes"`     // recipe slug → version
	Automations map[string]AutomationGenome `json:"automations"` // by automation id (enabled only)
	Schedules   map[string]ScheduleGenome   `json:"schedules"`   // by schedule id (enabled only)
	Prompts     map[string]string           `json:"prompts"`     // prompt key → override content hash ("" = default)
	Settings    SettingsGenome              `json:"settings"`
	Models      map[string]string           `json:"models"` // "<provider>|<requested>" → resolved concrete model
}

// AgentGenome is the 16-key inheritable surface of one agent
// (internal/db/agent_inherit.go inheritableFields), as effective values.
type AgentGenome struct {
	Name                string   `json:"name"`
	SystemKey           string   `json:"systemKey,omitempty"`
	ParentID            string   `json:"parentId,omitempty"`
	SoulHash            string   `json:"soulHash"`
	SoulChars           int      `json:"soulChars"`
	IdentityHash        string   `json:"identityHash"`
	Provider            string   `json:"provider"`
	ProviderInstanceID  string   `json:"providerInstanceId,omitempty"`
	Model               string   `json:"model"`
	ThinkingLevel       string   `json:"thinkingLevel,omitempty"`
	NativeWebSearch     *bool    `json:"nativeWebSearch,omitempty"`
	PermissionMode      string   `json:"permissionMode,omitempty"`
	MCPEnabled          bool     `json:"mcpEnabled"`
	ToolOverridesHash   string   `json:"toolOverridesHash"`
	VisibleToolCount    int      `json:"visibleToolCount"` // -1 when unknown
	AllowedToolsHash    string   `json:"allowedToolsHash"`
	Skills              []string `json:"skills"`
	CoordinatorMode     bool     `json:"coordinatorMode"`
	CoordinatorWorkflow string   `json:"coordinatorWorkflow,omitempty"`
	CoordinatorPrompt   string   `json:"coordinatorPromptHash,omitempty"`
	Disabled            bool     `json:"disabled,omitempty"`
}

type ToolsGenome struct {
	Disabled   []string          `json:"disabled"`
	Visibility map[string]string `json:"visibility"`
	MCPServers map[string]bool   `json:"mcpServers"` // server id → enabled
}

type AutomationGenome struct {
	Name          string `json:"name"`
	TriggerKind   string `json:"triggerKind"`
	TriggerHash   string `json:"triggerHash"` // hash of every trigger field
	TargetAgentID string `json:"targetAgentId,omitempty"`
	FlowID        string `json:"flowId,omitempty"`
	SessionMode   string `json:"sessionMode,omitempty"`
	PromptHash    string `json:"promptHash"`
	MaxIterations int    `json:"maxIterations"`
	CooldownSec   int    `json:"cooldownSec"`
}

type ScheduleGenome struct {
	Name     string `json:"name"`
	CronExpr string `json:"cronExpr"`
	AgentID  string `json:"agentId,omitempty"`
	FlowID   string `json:"flowId,omitempty"`
}

type SettingsGenome struct {
	TerseMode        bool   `json:"terseMode"`
	InstructionsHash string `json:"instructionsHash"`
	PromptEpoch      bool   `json:"promptEpoch"`
}

// HashText is the short content hash used for every text surface.
func HashText(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// Canonical returns the snapshot without its Hash/At header, so equal content
// always hashes equal regardless of when it was captured.
func (s ConfigSnapshot) Canonical() ([]byte, error) {
	body := s
	body.Hash = ""
	body.At = 0
	if body.Agents == nil {
		body.Agents = map[string]AgentGenome{}
	}
	if body.Recipes == nil {
		body.Recipes = map[string]string{}
	}
	if body.Automations == nil {
		body.Automations = map[string]AutomationGenome{}
	}
	if body.Schedules == nil {
		body.Schedules = map[string]ScheduleGenome{}
	}
	if body.Prompts == nil {
		body.Prompts = map[string]string{}
	}
	if body.Models == nil {
		body.Models = map[string]string{}
	}
	if body.Tools.Disabled == nil {
		body.Tools.Disabled = []string{}
	}
	if body.Tools.Visibility == nil {
		body.Tools.Visibility = map[string]string{}
	}
	if body.Tools.MCPServers == nil {
		body.Tools.MCPServers = map[string]bool{}
	}
	sort.Strings(body.Tools.Disabled)
	for id, a := range body.Agents {
		if a.Skills == nil {
			a.Skills = []string{}
		}
		sk := append([]string(nil), a.Skills...)
		sort.Strings(sk)
		a.Skills = sk
		body.Agents[id] = a
	}
	return json.Marshal(body)
}

// Finalize computes and sets Hash from the canonical content.
func (s *ConfigSnapshot) Finalize() error {
	raw, err := s.Canonical()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	s.Hash = hex.EncodeToString(sum[:8])
	return nil
}

// SnapshotChange is one human-readable difference between two snapshots.
type SnapshotChange struct {
	Surface string `json:"surface"` // agent | tools | recipe | automation | schedule | prompt | settings | model
	Entity  string `json:"entity,omitempty"`
	Field   string `json:"field,omitempty"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
}

// Diff lists what changed from a to b, surface by surface. It is the text the
// history view shows on an edge between two configuration versions.
func Diff(a, b ConfigSnapshot) []SnapshotChange {
	var out []SnapshotChange
	add := func(surface, entity, field, before, after string) {
		if before != after {
			out = append(out, SnapshotChange{Surface: surface, Entity: entity, Field: field, Before: before, After: after})
		}
	}
	for _, id := range unionKeys(mapKeys(a.Agents), mapKeys(b.Agents)) {
		ga, okA := a.Agents[id]
		gb, okB := b.Agents[id]
		switch {
		case !okA:
			out = append(out, SnapshotChange{Surface: "agent", Entity: id, Field: "added", After: gb.Name})
			continue
		case !okB:
			out = append(out, SnapshotChange{Surface: "agent", Entity: id, Field: "removed", Before: ga.Name})
			continue
		}
		add("agent", id, "soul", ga.SoulHash, gb.SoulHash)
		add("agent", id, "identity", ga.IdentityHash, gb.IdentityHash)
		add("agent", id, "provider", ga.Provider+"/"+ga.ProviderInstanceID, gb.Provider+"/"+gb.ProviderInstanceID)
		add("agent", id, "model", ga.Model, gb.Model)
		add("agent", id, "thinkingLevel", ga.ThinkingLevel, gb.ThinkingLevel)
		add("agent", id, "nativeWebSearch", boolPtr(ga.NativeWebSearch), boolPtr(gb.NativeWebSearch))
		add("agent", id, "permissionMode", ga.PermissionMode, gb.PermissionMode)
		add("agent", id, "tools", fmt.Sprintf("%v/%s", ga.MCPEnabled, ga.ToolOverridesHash), fmt.Sprintf("%v/%s", gb.MCPEnabled, gb.ToolOverridesHash))
		add("agent", id, "allowedTools", ga.AllowedToolsHash, gb.AllowedToolsHash)
		add("agent", id, "skills", joinSorted(ga.Skills), joinSorted(gb.Skills))
		add("agent", id, "coordinatorMode", fmt.Sprint(ga.CoordinatorMode), fmt.Sprint(gb.CoordinatorMode))
		add("agent", id, "coordinatorWorkflow", ga.CoordinatorWorkflow, gb.CoordinatorWorkflow)
		add("agent", id, "coordinatorPrompt", ga.CoordinatorPrompt, gb.CoordinatorPrompt)
		add("agent", id, "parent", ga.ParentID, gb.ParentID)
		add("agent", id, "disabled", fmt.Sprint(ga.Disabled), fmt.Sprint(gb.Disabled))
	}
	add("tools", "", "disabled", joinSorted(a.Tools.Disabled), joinSorted(b.Tools.Disabled))
	for _, k := range unionKeys(mapKeys(a.Tools.Visibility), mapKeys(b.Tools.Visibility)) {
		add("tools", k, "visibility", a.Tools.Visibility[k], b.Tools.Visibility[k])
	}
	for _, k := range unionKeys(mapKeys(a.Tools.MCPServers), mapKeys(b.Tools.MCPServers)) {
		add("tools", k, "mcp", boolOrAbsent(a.Tools.MCPServers, k), boolOrAbsent(b.Tools.MCPServers, k))
	}
	for _, k := range unionKeys(mapKeys(a.Recipes), mapKeys(b.Recipes)) {
		add("recipe", k, "version", a.Recipes[k], b.Recipes[k])
	}
	for _, k := range unionKeys(mapKeys(a.Automations), mapKeys(b.Automations)) {
		ga, okA := a.Automations[k]
		gb, okB := b.Automations[k]
		switch {
		case !okA:
			out = append(out, SnapshotChange{Surface: "automation", Entity: k, Field: "added", After: gb.Name})
			continue
		case !okB:
			out = append(out, SnapshotChange{Surface: "automation", Entity: k, Field: "removed", Before: ga.Name})
			continue
		}
		add("automation", k, "trigger", ga.TriggerKind+"/"+ga.TriggerHash, gb.TriggerKind+"/"+gb.TriggerHash)
		add("automation", k, "target", ga.TargetAgentID+"/"+ga.FlowID+"/"+ga.SessionMode, gb.TargetAgentID+"/"+gb.FlowID+"/"+gb.SessionMode)
		add("automation", k, "prompt", ga.PromptHash, gb.PromptHash)
		add("automation", k, "limits", fmt.Sprintf("%d/%d", ga.MaxIterations, ga.CooldownSec), fmt.Sprintf("%d/%d", gb.MaxIterations, gb.CooldownSec))
	}
	for _, k := range unionKeys(mapKeys(a.Schedules), mapKeys(b.Schedules)) {
		ga, okA := a.Schedules[k]
		gb, okB := b.Schedules[k]
		switch {
		case !okA:
			out = append(out, SnapshotChange{Surface: "schedule", Entity: k, Field: "added", After: gb.Name})
			continue
		case !okB:
			out = append(out, SnapshotChange{Surface: "schedule", Entity: k, Field: "removed", Before: ga.Name})
			continue
		}
		add("schedule", k, "cron", ga.CronExpr, gb.CronExpr)
		add("schedule", k, "target", ga.AgentID+"/"+ga.FlowID, gb.AgentID+"/"+gb.FlowID)
	}
	for _, k := range unionKeys(mapKeys(a.Prompts), mapKeys(b.Prompts)) {
		add("prompt", k, "override", a.Prompts[k], b.Prompts[k])
	}
	add("settings", "", "terseMode", fmt.Sprint(a.Settings.TerseMode), fmt.Sprint(b.Settings.TerseMode))
	add("settings", "", "instructions", a.Settings.InstructionsHash, b.Settings.InstructionsHash)
	add("settings", "", "promptEpoch", fmt.Sprint(a.Settings.PromptEpoch), fmt.Sprint(b.Settings.PromptEpoch))
	for _, k := range unionKeys(mapKeys(a.Models), mapKeys(b.Models)) {
		add("model", k, "resolved", a.Models[k], b.Models[k])
	}
	return out
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func unionKeys(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range append(a, b...) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func joinSorted(l []string) string {
	c := append([]string(nil), l...)
	sort.Strings(c)
	raw, _ := json.Marshal(c)
	return string(raw)
}

func boolPtr(b *bool) string {
	if b == nil {
		return ""
	}
	return fmt.Sprint(*b)
}

func boolOrAbsent(m map[string]bool, k string) string {
	v, ok := m[k]
	if !ok {
		return ""
	}
	return fmt.Sprint(v)
}
