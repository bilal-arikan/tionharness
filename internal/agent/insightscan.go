package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// ErrInsightScanBusy is returned when a scan is requested while one is already
// running for this workspace (scans are single-flight per workspace).
var ErrInsightScanBusy = errors.New("insight: a scan is already running")

// InsightScanActive reports whether a retrospective scan is currently running
// for this workspace, so callers (the API) can reject or annotate a new trigger.
func (r *Runtime) InsightScanActive() bool { return r.insightScanActive.Load() }

// RunInsightScan runs one retrospective scan (_Docs/60) over this workspace's
// sessions and routes app-fix findings. It seeds+loads the editable lenses,
// opens the incremental ledger + findings store, wires the cheap-model analyzer,
// runs the scanner, and — when an app-fix repo is configured — appends new
// app-fix findings to that repo's backlog. agentID selects the analysis agent
// (empty = the workspace's default agent).
func (r *Runtime) RunInsightScan(ctx context.Context, scope insight.ScanScope, agentID string) (insight.ScanResult, error) {
	if r == nil || r.db == nil {
		return insight.ScanResult{}, errors.New("insight: runtime not ready")
	}
	// One scan at a time per workspace: a scan is long (LLM per pair), so reject a
	// concurrent trigger rather than running two overlapping passes over the store.
	if !r.insightScanActive.CompareAndSwap(false, true) {
		return insight.ScanResult{}, ErrInsightScanBusy
	}
	// Broadcast start/finish so any window (nav rail + workspace list) can show a
	// live "scanning / done" indicator for THIS workspace (the event carries the
	// workspace id via r.publish). doneFindings is captured by the finish defer.
	r.publish(events.Event{
		Type: "insight", Level: "info", Title: "İçgörü taraması başladı",
		Target: map[string]string{"scanning": "true"},
	})
	doneFindings := 0
	defer func() {
		r.insightScanActive.Store(false)
		r.publish(events.Event{
			Type: "insight", Level: "success", Title: "İçgörü taraması tamamlandı",
			Body:   fmt.Sprintf("%d bulgu", doneFindings),
			Target: map[string]string{"scanning": "false", "findings": strconv.Itoa(doneFindings)},
		})
	}()
	root := r.db.Root()

	lensDir := insight.LensesDir(root)
	if err := insight.EnsureDefaults(lensDir); err != nil {
		return insight.ScanResult{}, err
	}
	reg, lensErrs := insight.LoadRegistry(lensDir)
	for _, e := range lensErrs {
		r.logger.Warn("insight lens load", "error", e)
	}

	ledger, err := insight.OpenLedger(root)
	if err != nil {
		return insight.ScanResult{}, err
	}
	findings, err := insight.OpenFindingStore(root)
	if err != nil {
		return insight.ScanResult{}, err
	}
	settings, err := insight.LoadSettings(root)
	if err != nil {
		return insight.ScanResult{}, err
	}
	// The settings caps are defaults; an explicit scope cap wins.
	if scope.MaxSessions == 0 && settings.MaxSessions > 0 {
		scope.MaxSessions = settings.MaxSessions
	}
	if scope.MaxAnalyzed == 0 && settings.MaxAnalyzed > 0 {
		scope.MaxAnalyzed = settings.MaxAnalyzed
	}
	// Age filter: an explicit scope cutoff wins; otherwise derive one from the
	// settings' ScanSinceDays so a scan skips sessions older than that window.
	if scope.SinceUnix == 0 && settings.ScanSinceDays > 0 {
		scope.SinceUnix = time.Now().Unix() - int64(settings.ScanSinceDays)*86400
	}

	// Analysis agent: an explicit caller agentID wins; otherwise the persistent
	// setting (so the manual "Tara" button and the cron share one default);
	// otherwise pickInsightAgent's fallback (first agent).
	if agentID == "" {
		agentID = settings.AutoScanAgentID
	}
	analysisAgent, err := r.pickInsightAgent(ctx, agentID)
	if err != nil {
		return insight.ScanResult{}, err
	}
	// An EXPLICITLY chosen agent runs on its own provider+model — that is the whole
	// point of selecting it. Only the implicit default (no agent picked) falls back
	// to the cheap title-model override.
	model := r.insightModel(analysisAgent)
	if agentID != "" && analysisAgent.Model != "" {
		model = analysisAgent.Model
	}
	analyzer := &insightAnalyzer{rt: r, agent: analysisAgent, model: model}

	start := time.Now()
	scanner := insight.NewScanner(r.db, reg, ledger, findings, analyzer, nil)
	res, err := scanner.Scan(ctx, scope)
	if err != nil {
		return res, err
	}

	// Route app-fix findings to the configured repo backlog (idempotent append).
	if settings.AppFixRepoPath != "" {
		appfix := findings.List("", insight.ChannelAppFix)
		if n, aErr := insight.AppendBacklog(settings.AppFixRepoPath, appfix); aErr != nil {
			r.logger.Warn("insight backlog append failed", "error", aErr, "repo", settings.AppFixRepoPath)
		} else if n > 0 {
			r.logger.Info("insight backlog appended", "count", n, "repo", settings.AppFixRepoPath)
		}
	}

	// Route workspace-opt findings to the workspace-local actions doc (Channel B
	// sink, idempotent append). This is a reviewable document, not an auto-applied
	// mutation — workspace changes stay a deliberate user/agent decision.
	wsopt := findings.List("", insight.ChannelWorkspaceOpt)
	if n, wErr := insight.AppendWorkspaceActions(root, wsopt); wErr != nil {
		r.logger.Warn("insight workspace actions append failed", "error", wErr, "root", root)
	} else if n > 0 {
		r.logger.Info("insight workspace actions appended", "count", n)
	}

	// Lessons-mining synergy: promote fresh findings from the lessons-mining lens
	// into the runtime lessons store so future turns carry them. Only THIS run's
	// produced findings are fed (not the whole store) — AddLesson dedupes by
	// signature, so a recurring lesson bumps its Count instead of duplicating.
	r.promoteMinedLessons(res.Produced)

	// Lifecycle maintenance: auto-verify applied fixes that stopped recurring and
	// prune long-resolved findings, so the store doesn't accumulate stale noise.
	if m, mErr := findings.Maintain(time.Now().Unix()); mErr != nil {
		r.logger.Warn("insight maintain failed", "error", mErr)
	} else if m.AutoVerified > 0 || m.Pruned > 0 {
		r.logger.Info("insight maintain", "autoVerified", m.AutoVerified, "pruned", m.Pruned)
	}

	// Compact the append-only ledger back to one line per (lens,session): every
	// re-scan of a changed session appends a superseded line, so without this the
	// file — and the startup load that reads it — grows unboundedly with scans.
	if cErr := ledger.Compact(); cErr != nil {
		r.logger.Warn("insight ledger compact failed", "error", cErr)
	}

	// Observability: record the run in an append-only log (NOT a session), so scans
	// stay auditable (when/how long/what) without cluttering the chat list.
	if rErr := insight.AppendRun(root, insight.RunRecord{
		At:          time.Now().Unix(),
		DurationMs:  time.Since(start).Milliseconds(),
		LensIDs:     scope.LensIDs,
		Sessions:    res.Sessions,
		Analyzed:    res.Analyzed,
		Skipped:     res.Skipped,
		Prefiltered: res.Prefiltered,
		Findings:    res.Findings,
		Errors:      len(res.Errors),
	}); rErr != nil {
		r.logger.Warn("insight run log failed", "error", rErr)
	}

	doneFindings = res.Findings // surfaced in the finish event's body/target
	return res, nil
}

// lessonsMiningLensID is the built-in lens whose findings double as lessons.
const lessonsMiningLensID = "lessons-mining"

// promoteMinedLessons writes lessons-mining findings into the lessons store. The
// finding's proposedFix is the rule (lesson body); title fronts it for context.
func (r *Runtime) promoteMinedLessons(produced []insight.Finding) {
	for _, f := range produced {
		if f.LensID != lessonsMiningLensID {
			continue
		}
		text := f.Title
		if f.ProposedFix != "" {
			text += " — " + f.ProposedFix
		}
		var sess string
		if len(f.EvidenceSessionIDs) > 0 {
			sess = f.EvidenceSessionIDs[0]
		}
		if _, err := r.db.AddLesson(db.Lesson{
			SessionID: sess,
			Signature: f.Signature,
			Text:      text,
			Time:      f.LastSeen,
		}); err != nil {
			r.logger.Warn("insight lesson promote failed", "sig", f.Signature, "error", err)
		}
	}
}

// pickInsightAgent resolves the agent whose provider/model runs the analysis.
// A blank id selects the workspace's default (newest) agent.
func (r *Runtime) pickInsightAgent(ctx context.Context, agentID string) (db.Agent, error) {
	if agentID != "" {
		return r.db.GetAgent(ctx, agentID)
	}
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return db.Agent{}, err
	}
	if len(agents) == 0 {
		return db.Agent{}, errors.New("insight: no agent available to run analysis")
	}
	return agents[0], nil
}

// insightModel prefers the configured title-model override (cheap/fast, same
// policy as lesson reflection + utility summaries) and falls back to the agent's
// own model.
func (r *Runtime) insightModel(agent db.Agent) string {
	if override := r.tun.TitleModel(); override != "" {
		return override
	}
	return agent.Model
}
