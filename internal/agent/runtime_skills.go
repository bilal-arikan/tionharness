package agent

// Skill wiring for the runtime: how an agent's selected skills become the
// Available Skills prompt block, and the sinks the use_skill / skill_search /
// skill-management tools dispatch through. The library is shared workspace-wide;
// an agent only ever picks from it, so every accessor here is scoped by the
// agent's own slug allowlist.

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// SkillsCatalogBlockForAgent renders the Available Skills system-prompt section
// an agent sees: its assigned skills (in the agent's chosen order) plus every
// shared (on-demand) skill. Returns "" when neither exists. Skills are a shared
// library; agents pick from it — they never own skills.
func (r *Runtime) SkillsCatalogBlockForAgent(agent db.Agent) string {
	if r.skills == nil {
		return ""
	}
	// Name the use_skill tool exactly as THIS agent will see it. A claude-cli agent
	// reaches TionHarness's built-ins through the Interaction MCP bridge, where they are
	// namespaced (mcp__tionharness_interaction__use_skill). Advertising the bare name to
	// it makes the model emit an unqualified `use_skill` call the CLI rejects with
	// "No such tool available: use_skill" on the first turn (it recovers on retry by
	// finding the namespaced tool, but the wasted round-trip + error is avoidable).
	return r.skills.CatalogBlockForAgentTool(agent.Skills, skillToolNameFor(agent.Provider))
}

// skillToolNameFor returns the identifier the use_skill tool carries for an agent
// on the given provider: native (API) providers register the bare name, while a
// CLI provider reaches it namespaced through the Interaction MCP bridge. The
// empty provider is the keyless claude-cli default. Custom providers are only
// ever OpenAI/Anthropic-compatible (native), so they take the bare name.
func skillToolNameFor(provider string) string {
	if isCLIProviderKind(provider) {
		return interactionToolPrefix + skills.DefaultSkillTool
	}
	return skills.DefaultSkillTool
}

// isCLIProviderKind reports whether provider (a KIND id — Agent.Provider,
// always a kind, never a provider instance id, _Docs/71 §2.5) drives a
// locally-installed CLI through the Interaction MCP bridge (claude-cli,
// codex-cli) rather than a native API call — both dialects namespace bridged
// tool names the same way, so every call site that branches on "is this a CLI
// turn" shares this one check. Driven by Manifest.Transport (_Docs/71 §4.2)
// rather than a hard-coded id list, so a future CLI-transport kind is covered
// automatically. The empty provider is the keyless claude-cli default.
func isCLIProviderKind(provider string) bool {
	if provider == "" {
		provider = "claude-cli"
	}
	return providers.TransportOf(provider) == providers.TransportCLI
}

// LoadSkillForAgent returns a skill's full body for the CLI path (the Interaction
// MCP use_skill bridge), enforcing the SAME per-agent allowlist as the native
// use_skill built-in. It mirrors the agentSkillLib the native tool loop builds,
// so both provider paths advertise an identical contract over one skill store.
func (r *Runtime) LoadSkillForAgent(agent db.Agent, slug string) (string, error) {
	if r.skills == nil {
		return "", fmt.Errorf("skills are not available")
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.Body(slug)
}

// SearchSkillsForAgent powers the CLI-path skill_search bridge: it searches the
// library but returns only skills the agent may load (assigned + shared), so a
// claude-cli agent can discover on-demand/conditional skills the same way native
// agents do via the skill_search tool. (SK-2)
func (r *Runtime) SearchSkillsForAgent(agent db.Agent, query string, limit int) []tools.SkillHit {
	if r.skills == nil {
		return nil
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.SearchSkills(query, limit)
}

// SkillAllowedToolsForAgent returns the allowed-tools the named skill declares,
// for the CLI-path use_skill bridge to auto-grant — SK-3 parity with the native
// use_skill tool.
func (r *Runtime) SkillAllowedToolsForAgent(agent db.Agent, slug string) []string {
	if r.skills == nil {
		return nil
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.AllowedTools(slug)
}

// agentSkillLib restricts the use_skill tool to an agent's selected slugs, so an
// agent cannot load a skill it has not been given.
type agentSkillLib struct {
	store *skills.Store
	allow map[string]bool
}

func (l agentSkillLib) Body(slug string) (string, error) {
	if !l.allow[slug] {
		return "", fmt.Errorf("skill %q is not enabled for this agent", slug)
	}
	return l.store.UseSkillBody(slug, l.allow)
}

// AllowedTools returns the tool-permission patterns the named skill declares,
// restricted to skills this agent may load. Powers SK-3 (loading a skill
// auto-grants its tools for the session).
func (l agentSkillLib) AllowedTools(slug string) []string {
	if !l.allow[slug] {
		return nil
	}
	sk, ok := l.store.Get(slug)
	if !ok {
		return nil
	}
	return sk.AlwaysAllow
}

// SearchSkills powers the skill_search tool: it searches the full library but
// returns only skills this agent may load (assigned + shared), so discovery never
// reveals a skill the agent could not then use. (SK-2)
func (l agentSkillLib) SearchSkills(query string, limit int) []tools.SkillHit {
	out := []tools.SkillHit{}
	for _, sk := range l.store.Search(query, 0) {
		if !l.allow[sk.Slug] {
			continue
		}
		out = append(out, tools.SkillHit{Slug: sk.Slug, Description: sk.Description, WhenToUse: sk.WhenToUse})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// agentSkillWriter adapts *skills.Store to the tools.SkillWriter interface so the
// create_skill / delete_skill self-management tools can author workspace skills
// without the tools package importing the skills package. db (optional) lets
// DeleteSkill strip the removed slug from every agent's skill selection.
type agentSkillWriter struct {
	store *skills.Store
	db    *db.DB
}

// ValidateSkill adapts skills.Store.ValidateSkill onto the tools-layer view so the
// skill_validate tool stays decoupled from the skills package.
func (w agentSkillWriter) ValidateSkill(slug string) tools.SkillValidation {
	r := w.store.ValidateSkill(slug)
	return tools.SkillValidation{
		Found:    r.Found,
		Tier:     r.Tier,
		Path:     r.Path,
		Valid:    r.Valid,
		Errors:   r.Errors,
		Warnings: r.Warnings,
	}
}

func (w agentSkillWriter) CreateSkill(slug, name, description, whenToUse, group, body string, shared bool) error {
	_, err := w.store.Create(slug, skills.SkillInput{
		Name:        name,
		Description: description,
		WhenToUse:   whenToUse,
		Group:       group,
		Body:        body,
		Shared:      shared,
	})
	return err
}

// ImportSkill imports a Claude Code skill (local dir or github URL) into the
// workspace tier and maps the skills.ImportResult onto the tools view. (SK-IMP)
func (w agentSkillWriter) ImportSkill(source, location, slug string, shared bool) (tools.SkillImportResult, error) {
	_, res, err := w.store.ImportFromSource(source, location, slug, shared)
	if err != nil {
		return tools.SkillImportResult{}, err
	}
	return tools.SkillImportResult{Slug: res.Slug, Warnings: res.Warnings, Files: res.Files}, nil
}

// UpdateSkill edits a workspace skill in place. Each pointer field is applied
// only when non-nil (partial update), merging over the skill's current values
// so the agent can change just the body. Restricted to workspace-tier skills so
// bundled/global skills can't be overwritten (parity with DeleteSkill).
func (w agentSkillWriter) UpdateSkill(slug string, name, description, whenToUse, group, body *string, shared *bool) error {
	cur, ok := w.store.Get(slug)
	if !ok {
		return fmt.Errorf("no skill with slug %q (check the skill catalog)", slug)
	}
	if cur.Source != skills.SourceWorkspace {
		return fmt.Errorf("skill %q is a %s skill and cannot be edited (only workspace skills are editable)", slug, cur.Source)
	}
	curBody, _ := w.store.Body(slug)
	in := skills.SkillInput{
		Name:        cur.Name,
		Description: cur.Description,
		WhenToUse:   cur.WhenToUse,
		Icon:        cur.Icon,
		Color:       cur.Color,
		Group:       cur.Group,
		Shared:      cur.Shared,
		Body:        curBody,
	}
	if name != nil {
		in.Name = *name
	}
	if description != nil {
		in.Description = *description
	}
	if whenToUse != nil {
		in.WhenToUse = *whenToUse
	}
	if group != nil {
		in.Group = *group
	}
	if body != nil {
		in.Body = *body
	}
	if shared != nil {
		in.Shared = *shared
	}
	_, err := w.store.Update(slug, in)
	return err
}

func (w agentSkillWriter) DeleteSkill(slug string) error {
	if err := w.store.Delete(slug); err != nil {
		return err
	}
	// Drop the now-deleted slug from any agent that referenced it so no agent
	// keeps a dangling skill reference.
	if w.db != nil {
		_, _ = w.db.RemoveSkillFromAgents(context.Background(), slug)
	}
	return nil
}
