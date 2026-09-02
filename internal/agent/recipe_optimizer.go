package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// Recipe optimizer (Rota F4, brief §7.2–7.4). The one LLM pass of the Rota
// work, and a rare one: it runs for a recipe only when N=3 new summarized
// terminal runs have accumulated since its last pass, when a run FAILED, or
// when the user asks. It reads the recipe, the per-version statistics and the
// last summaries, and files PROPOSALS as insight findings on the recipe-opt
// channel — v1 never edits the recipe itself (brief: suggestions first, opt-in
// auto-apply of the pruning class later).
//
// Invariants enforced in code, not in the prompt: a proposal without evidence
// is dropped; a generic negative judgement is dropped; an addition that would
// push phases + watchers over the growth budget must name what it removes;
// only the recipe named in the pass may be targeted.

const (
	optimizerMinNewRuns   = 3
	optimizerRecentRuns   = 5
	optimizerMaxTokens    = 2500
	optimizerSystemKey    = "recipe-optimizer"
	optimizerLensID       = "recipe-optimizer"
	optimizerTriggerRuns  = "runs"
	optimizerTriggerFail  = "failed"
	optimizerTriggerHuman = "manual"
)

// Proposal actions the optimizer may file.
var optimizerActions = map[string]bool{
	"prune_phase": true, "make_optional": true, "prune_watcher": true, "change_profile": true,
	"add_gate": true, "bind_watcher": true, "split_phase": true, "merge_phase": true, "rollback_version": true,
}

// Actions that add to the recipe (growth budget applies).
var optimizerAdditive = map[string]bool{"add_gate": true, "bind_watcher": true, "split_phase": true}

// Generic negative judgements are refused whatever the rest of the text says
// (brief §7.4 "negatif yakalama yasağı").
var optimizerBannedPhrases = []string{
	"işe yaramaz", "güvenilmez", "çalışmıyor", "does not work", "doesn't work", "unreliable", "useless", "never works", "is broken",
}

var optimizerSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "proposals": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "action":    { "type": "string" },
          "target":    { "type": "string" },
          "value":     { "type": "string" },
          "removes":   { "type": "string" },
          "title":     { "type": "string" },
          "rationale": { "type": "string" },
          "evidence":  { "type": "string" },
          "severity":  { "type": "string", "enum": ["low", "med", "high"] }
        },
        "required": ["action", "title", "evidence"]
      }
    }
  },
  "required": ["proposals"]
}`)

// OptimizerResult is what one pass produced.
type OptimizerResult struct {
	Slug      string            `json:"slug"`
	Trigger   string            `json:"trigger"`
	Ran       bool              `json:"ran"`
	Skipped   string            `json:"skipped,omitempty"`
	Proposals []insight.Finding `json:"proposals"`
	Dropped   int               `json:"dropped"` // proposals refused by the invariants
	// Applied counts pruning proposals applied to the recipe (auto_prune: true).
	Applied int `json:"applied"`
}

// rawProposal is the model's output shape.
type rawProposal struct {
	Action    string `json:"action"`
	Target    string `json:"target"`
	Value     string `json:"value"`
	Removes   string `json:"removes"`
	Title     string `json:"title"`
	Rationale string `json:"rationale"`
	Evidence  string `json:"evidence"`
	Severity  string `json:"severity"`
}

// optimizerFlights serialises passes per slug.
var optimizerFlights sync.Map

// MaybeOptimizeRecipe runs the optimizer when the threshold is met: ≥
// optimizerMinNewRuns new summarized terminal runs since the last pass, or a
// failed run (trigger optimizerTriggerFail). Returns ran=false with the reason
// otherwise. Safe to call from the trajectory queue: the LLM call happens on a
// detached goroutine.
func (r *Runtime) MaybeOptimizeRecipe(ctx context.Context, slug, trigger string) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return
	}
	stats, ok := r.recipeStatsFor(ctx, slug)
	if !ok {
		return
	}
	st, err := r.db.GetOptimizerState(ctx)
	if err != nil {
		r.logger.Warn("optimizer: state read failed", "slug", slug, "error", err)
		return
	}
	prev := st.Slugs[slug]
	if trigger != optimizerTriggerFail && stats.Summarized-prev.RunsSeen < optimizerMinNewRuns {
		return
	}
	go func() {
		if _, err := r.RunRecipeOptimizer(context.Background(), slug, trigger); err != nil {
			r.logger.Warn("optimizer: pass failed", "slug", slug, "trigger", trigger, "error", err)
		}
	}()
}

// recipeStatsFor aggregates every version of a slug into one row.
func (r *Runtime) recipeStatsFor(ctx context.Context, slug string) (RecipeStats, bool) {
	var agg RecipeStats
	found := false
	for _, st := range RecipeStatsFromIndex(r.db.ListTrajectories(ctx, db.TrajectoryFilter{})) {
		if st.Slug != slug {
			continue
		}
		found = true
		agg.Slug = slug
		agg.Runs += st.Runs
		agg.Done += st.Done
		agg.Failed += st.Failed
		agg.Abandoned += st.Abandoned
		agg.Summarized += st.Summarized
		if st.LastAt > agg.LastAt {
			agg.LastAt, agg.LatestID, agg.Version, agg.TemplateRef = st.LastAt, st.LatestID, st.Version, st.TemplateRef
		}
	}
	return agg, found
}

// RunRecipeOptimizer performs one pass for a recipe and files the proposals.
func (r *Runtime) RunRecipeOptimizer(ctx context.Context, slug, trigger string) (OptimizerResult, error) {
	slug = strings.TrimSpace(slug)
	res := OptimizerResult{Slug: slug, Trigger: trigger, Proposals: []insight.Finding{}}
	if slug == "" {
		return res, errors.New("optimizer: empty recipe slug")
	}
	if _, busy := optimizerFlights.LoadOrStore(slug, true); busy {
		res.Skipped = "a pass is already running for this recipe"
		return res, nil
	}
	defer optimizerFlights.Delete(slug)

	sk, err := skills.ResolveRecipe(r.skills, slug)
	if err != nil || sk == nil {
		res.Skipped = "recipe not found"
		return res, fmt.Errorf("optimizer: %s: %w", slug, errOrUnknown(err))
	}
	body, err := r.skills.Body(slug)
	if err != nil {
		body = ""
	}
	rows := r.db.ListTrajectories(ctx, db.TrajectoryFilter{})
	stats := RecipeStatsFromIndex(rows)
	var mine []RecipeStats
	for _, st := range stats {
		if st.Slug == slug {
			mine = append(mine, st)
		}
	}
	recent := recentSummaries(rows, slug, optimizerRecentRuns)
	summarized := 0
	for _, st := range mine {
		summarized += st.Summarized
	}
	finish := func(skipped string, n int) (OptimizerResult, error) {
		res.Skipped = skipped
		res.Ran = skipped == ""
		_ = r.db.SetOptimizerSlugState(ctx, slug, db.OptimizerSlugState{
			LastAt: time.Now().Unix(), RunsSeen: summarized, Trigger: trigger, Proposals: n, Skipped: skipped,
		})
		return res, nil
	}
	if len(recent) == 0 {
		return finish("no summarized runs yet", 0)
	}
	base, err := r.pickInsightAgent(ctx, "")
	if err != nil {
		return finish("no agent available", 0)
	}
	agent, system, err := r.resolveAnalysisSystemAgent(optimizerSystemKey, base)
	if err != nil {
		return finish("optimizer agent unavailable", 0)
	}
	if system == "" || strings.Contains(system, "retrospective analyst") {
		// resolveAnalysisSystemAgent's fallback is the lesson/insight prompt; the
		// optimizer has its own.
		system = r.readPrompt("recipe-optimizer")
	}
	curatorHints := r.curatorHintsFor(ctx, slug)
	lang := ""
	if r.tun != nil {
		lang = r.tun.Language()
	}
	user := optimizerUserPrompt(*sk, body, mine, recent, curatorHints, lang)
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), optimizerSystemKey, system), agent, providers.Request{
		Model: agent.Model, System: system, MaxTokens: optimizerMaxTokens, OutputSchema: optimizerSchema,
		Messages: []providers.Message{{Role: providers.RoleUser, Text: user}},
	}, false)
	if err != nil {
		_, _ = finish("model call failed: "+err.Error(), 0)
		return res, err
	}
	raw := extractJSONObject(resp.Text)
	var parsed struct {
		Proposals []rawProposal `json:"proposals"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &parsed) != nil {
		_, _ = finish("no JSON in reply", 0)
		return res, errors.New("optimizer: no JSON in reply")
	}
	budget := recipeBudgetUsed(sk.Recipe)
	store, err := insight.OpenFindingStore(r.db.Root())
	if err != nil {
		return res, err
	}
	now := time.Now().Unix()
	evidenceSessions := recentRoots(rows, slug, optimizerRecentRuns)
	for _, p := range parsed.Proposals {
		f, ok := optimizerFinding(slug, sk.Version, p, budget, now, evidenceSessions)
		if !ok {
			res.Dropped++
			continue
		}
		stored, err := store.Upsert(f)
		if err != nil {
			r.logger.Warn("optimizer: finding upsert failed", "slug", slug, "error", err)
			continue
		}
		// F4-v2: a recipe that opted in (auto_prune: true) gets its PRUNING
		// proposals applied on the spot; the finding closes as applied with the
		// recipe as evidence, and the recipe version moves up.
		if sk.Recipe != nil && sk.Recipe.AutoPrune && skills.PruneActions[f.Proposal.Action] && !insight.ClosedStatus(stored.Status) {
			if v, aerr := skills.ApplyRecipeProposal(r.skills, slug, f.Proposal.Action, f.Proposal.Target, f.Proposal.Value); aerr != nil {
				r.logger.Warn("optimizer: auto-prune failed", "slug", slug, "action", f.Proposal.Action, "target", f.Proposal.Target, "error", aerr)
			} else {
				if _, serr := store.SetStatus(stored.ID, insight.StatusApplied, now, &insight.AppliedEntity{EntityType: "skill", EntityID: slug}); serr != nil {
					r.logger.Warn("optimizer: mark applied failed", "finding", stored.ID, "error", serr)
				} else {
					stored.Status = insight.StatusApplied
					res.Applied++
					r.logger.Info("optimizer: auto-pruned", "slug", slug, "action", f.Proposal.Action, "target", f.Proposal.Target, "version", v)
				}
			}
		}
		res.Proposals = append(res.Proposals, stored)
	}
	r.logger.Info("optimizer pass", "slug", slug, "trigger", trigger, "proposals", len(res.Proposals), "dropped", res.Dropped)
	if len(res.Proposals) > 0 {
		r.publish(events.Event{
			Type: "insight", Level: "info",
			Title:  fmt.Sprintf("✦ Reçete optimizer — %s için %d öneri", slug, len(res.Proposals)),
			Body:   "İçgörü ▸ recipe-opt kanalı",
			Target: map[string]string{"view": "insights", "channel": string(insight.ChannelRecipeOpt)},
		})
	}
	return finish("", len(res.Proposals))
}

func errOrUnknown(err error) error {
	if err == nil {
		return errors.New("unknown recipe")
	}
	return err
}

// recipeBudgetUsed counts phases + watchers (phase-level and global).
func recipeBudgetUsed(spec *skills.RecipeSpec) int {
	if spec == nil {
		return 0
	}
	n := len(spec.Phases) + len(spec.Watchers)
	for _, p := range spec.Phases {
		n += len(p.Watchers)
	}
	return n
}

// optimizerFinding validates one proposal against the invariants and shapes
// it as a recipe-opt finding. ok=false drops it.
func optimizerFinding(slug, version string, p rawProposal, budgetUsed int, now int64, evidenceSessions []string) (insight.Finding, bool) {
	action := strings.ToLower(strings.TrimSpace(p.Action))
	if !optimizerActions[action] {
		return insight.Finding{}, false
	}
	evidence := strings.TrimSpace(p.Evidence)
	if evidence == "" || !strings.ContainsAny(evidence, "0123456789") {
		return insight.Finding{}, false // no measured evidence
	}
	text := strings.ToLower(p.Title + " " + p.Rationale)
	for _, banned := range optimizerBannedPhrases {
		if strings.Contains(text, banned) {
			return insight.Finding{}, false
		}
	}
	if optimizerAdditive[action] && budgetUsed+1 > skills.RecipeGrowthBudget && strings.TrimSpace(p.Removes) == "" {
		return insight.Finding{}, false // over budget without naming a removal
	}
	target := strings.TrimSpace(p.Target)
	sev := normalizeSeverity(p.Severity)
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = action + " " + target
	}
	fix := strings.TrimSpace(p.Rationale)
	if v := strings.TrimSpace(p.Value); v != "" {
		fix = strings.TrimSpace(fix + "\nDeğer: " + v)
	}
	if rm := strings.TrimSpace(p.Removes); rm != "" {
		fix = strings.TrimSpace(fix + "\nKaldırır: " + rm)
	}
	return insight.Finding{
		LensID:             optimizerLensID,
		Channel:            insight.ChannelRecipeOpt,
		Signature:          "recipe-opt:" + slug + ":" + action + ":" + strings.ToLower(target),
		Title:              title,
		RootCause:          evidence,
		ProposedFix:        fix,
		FilePointer:        "skill/" + slug,
		Severity:           sev,
		EvidenceSessionIDs: evidenceSessions,
		Occurrences:        1,
		Status:             insight.StatusNew,
		FirstSeen:          now,
		LastSeen:           now,
		Proposal: &insight.RecipeProposal{
			Slug: slug, Version: version, Action: action, Target: target,
			Value: strings.TrimSpace(p.Value), Removes: strings.TrimSpace(p.Removes), Evidence: evidence,
		},
	}, true
}

// recentSummaries returns the newest summarized terminal runs of a slug.
func recentSummaries(rows []db.TrajectoryIndexEntry, slug string, n int) []db.TrajectoryIndexEntry {
	var out []db.TrajectoryIndexEntry
	for _, e := range rows {
		if e.Summary == nil || !(db.Trajectory{Status: e.Status}).IsTerminal() {
			continue
		}
		s := e.TemplateRef
		if i := strings.LastIndex(s, "@"); i > 0 {
			s = s[:i]
		}
		if s == slug {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func recentRoots(rows []db.TrajectoryIndexEntry, slug string, n int) []string {
	var ids []string
	for _, e := range recentSummaries(rows, slug, n) {
		ids = append(ids, e.RootSessionID)
	}
	return ids
}

// curatorHintsFor lists the curator's recipe suggestions for a slug, so the
// optimizer starts from the deterministic findings instead of rediscovering
// them.
func (r *Runtime) curatorHintsFor(ctx context.Context, slug string) []string {
	rep, ok, err := r.db.GetCuratorReport(ctx)
	if err != nil || !ok {
		return nil
	}
	var out []string
	for _, a := range rep.Actions {
		if a.Entity == db.CuratorEntityRecipe && a.ID == slug {
			out = append(out, a.Detail+" ("+a.Evidence+")")
		}
	}
	return out
}

// optimizerUserPrompt lays the evidence out most-stable-first (recipe, stats,
// then the per-run summaries).
func optimizerUserPrompt(sk skills.Skill, body string, stats []RecipeStats, recent []db.TrajectoryIndexEntry, curatorHints []string, lang string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Recipe: %s (version %s)\n\n", sk.Slug, orDash(sk.Version))
	if sk.Recipe != nil {
		fmt.Fprintf(&b, "Growth budget: %d of %d used (phases + watchers).\n\n", recipeBudgetUsed(sk.Recipe), skills.RecipeGrowthBudget)
		b.WriteString("Declared plan:\n")
		for _, p := range sk.Recipe.Phases {
			fmt.Fprintf(&b, "- %s", p.ID)
			if p.Profile != "" {
				fmt.Fprintf(&b, " · profile %s", p.Profile)
			}
			if p.Gate != nil {
				fmt.Fprintf(&b, " · gate %s %s", p.Gate.Kind, p.Gate.Value)
			}
			if p.Optional {
				b.WriteString(" · optional")
			}
			if len(p.Watchers) > 0 {
				fmt.Fprintf(&b, " · watchers %s", strings.Join(p.Watchers, ","))
			}
			b.WriteString("\n")
		}
		if len(sk.Recipe.Watchers) > 0 {
			fmt.Fprintf(&b, "- recipe watchers: %s\n", strings.Join(sk.Recipe.Watchers, ","))
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(body) != "" {
		b.WriteString("Recipe body:\n```\n" + strings.TrimSpace(clipText(body, 6000)) + "\n```\n\n")
	}
	b.WriteString("# Statistics per version\n")
	for _, st := range stats {
		fmt.Fprintf(&b, "- %s: %d terminal runs (%d done, %d failed, %d abandoned), %d live; avg %s, %d tokens, $%.2f, %.1f workers, %d failed workers, %d unannounced, gate wait %s",
			st.TemplateRef, st.Runs, st.Done, st.Failed, st.Abandoned, st.Live,
			optDur(time.Duration(st.AvgDurationSec)*time.Second), st.AvgTokens, st.AvgCostUSD, st.AvgSessions, st.FailedSessions, st.Unannounced,
			optDur(time.Duration(st.GateWaitSec)*time.Second))
		for _, w := range sortedKeys(st.UnfiredWatchers) {
			fmt.Fprintf(&b, "; watcher %s unfired in %d/%d", w, st.UnfiredWatchers[w], st.Summarized)
		}
		for _, p := range sortedKeys(st.GhostPhases) {
			fmt.Fprintf(&b, "; phase %s unreached in %d/%d", p, st.GhostPhases[p], st.Summarized)
		}
		b.WriteString("\n")
	}
	if len(curatorHints) > 0 {
		b.WriteString("\n# Curator (deterministic) suggestions already on file\n")
		for _, h := range curatorHints {
			b.WriteString("- " + h + "\n")
		}
	}
	b.WriteString("\n# Last runs\n")
	for _, e := range recent {
		s := e.Summary
		fmt.Fprintf(&b, "- %s (%s, %s): %s, %d tokens, %d workers (%d failed), %d runs (%d failed), %d unannounced, gates %d (%s)",
			e.ID, e.TemplateRef, e.Status, optDur(time.Duration(s.DurationSec)*time.Second), s.Tokens, s.Sessions, s.FailedSess, s.FlowRuns, s.FailedRuns, s.Unannounced, s.Gates, optDur(time.Duration(s.GateWaitSec)*time.Second))
		if len(s.GhostPhases) > 0 {
			fmt.Fprintf(&b, "; unreached phases %s", strings.Join(s.GhostPhases, ","))
		}
		if len(s.UnfiredWatchers) > 0 {
			fmt.Fprintf(&b, "; unfired watchers %s", strings.Join(s.UnfiredWatchers, ","))
		}
		for _, id := range sortedPhaseIDs(s.PerPhase) {
			ps := s.PerPhase[id]
			fmt.Fprintf(&b, "; %s: %d workers/%d failed/%s", id, ps.Sessions, ps.Failed, optDur(time.Duration(ps.DurationSec)*time.Second))
		}
		b.WriteString("\n")
	}
	if lang != "" {
		fmt.Fprintf(&b, "\nWrite title and rationale in %s; keep ids, names and the evidence string verbatim.\n", lang)
	}
	return b.String()
}

func sortedPhaseIDs(m map[string]db.PhaseStat) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… (truncated)"
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// optDur renders a duration compactly for the prompt (1h05m, 12m, 40s).
func optDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
