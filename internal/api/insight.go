// Retrospective session scanning (Insight, _Docs/60). The scan itself runs in
// the agent runtime (RunInsightScan, which wires the cheap-model analyzer); the
// API exposes the editable lenses, the findings store and the settings, plus the
// manual scan trigger. Everything is workspace-scoped via ws(r).
package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// lensByID loads the lens registry and returns the lens with id (and its file
// path). ok is false when the id is unknown.
func (s *Server) lensByID(r *http.Request, id string) (insight.Lens, bool) {
	dir := insight.LensesDir(ws(r).DB.Root())
	_ = insight.EnsureDefaults(dir)
	reg, _ := insight.LoadRegistry(dir)
	return reg.Get(id)
}

// handleGetInsightLensRaw returns a lens file's raw markdown (frontmatter + body)
// so the UI can edit it in place.
func (s *Server) handleGetInsightLensRaw(w http.ResponseWriter, r *http.Request) {
	l, ok := s.lensByID(r, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "lens not found")
		return
	}
	raw, err := os.ReadFile(l.Path)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": l.ID, "raw": string(raw)})
}

// handleUpdateInsightLens overwrites a lens file with new raw content, rejecting
// content that fails to parse (a bad lens must never silently replace a good one).
func (s *Server) handleUpdateInsightLens(w http.ResponseWriter, r *http.Request) {
	l, ok := s.lensByID(r, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "lens not found")
		return
	}
	req, ok := bindJSON[struct {
		Raw string `json:"raw"`
	}](w, r)
	if !ok {
		return
	}
	if _, err := insight.ParseLens(req.Raw, l.Path); err != nil {
		writeError(w, http.StatusBadRequest, "invalid lens: "+err.Error())
		return
	}
	if err := os.WriteFile(l.Path, []byte(req.Raw), 0o644); writeDBError(w, err, "") {
		return
	}
	updated, _ := insight.ParseLens(req.Raw, l.Path)
	writeJSON(w, http.StatusOK, updated)
}

// handleToggleInsightLens flips a lens's enabled flag in its frontmatter.
func (s *Server) handleToggleInsightLens(w http.ResponseWriter, r *http.Request) {
	l, ok := s.lensByID(r, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "lens not found")
		return
	}
	req, ok := bindJSON[struct {
		Enabled bool `json:"enabled"`
	}](w, r)
	if !ok {
		return
	}
	cur, err := os.ReadFile(l.Path)
	if writeDBError(w, err, "") {
		return
	}
	next := insight.SetFrontmatterEnabled(cur, req.Enabled)
	if err := os.WriteFile(l.Path, next, 0o644); writeDBError(w, err, "") {
		return
	}
	updated, _ := insight.ParseLens(string(next), l.Path)
	writeJSON(w, http.StatusOK, updated)
}

// handleRestoreInsightLens overwrites a lens with its shipped default, discarding
// local changes. The deliberate counterpart to the automatic refresh: EnsureDefaults
// only touches files it can PROVE are untouched prior ships, so a lens that was
// edited — or that was seeded before the shipped-hash ledger existed — stays frozen
// until the user asks for the default back from here.
func (s *Server) handleRestoreInsightLens(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !insight.HasDefault(id) {
		writeError(w, http.StatusNotFound, "lens has no shipped default")
		return
	}
	dir := insight.LensesDir(ws(r).DB.Root())
	if err := insight.RestoreDefault(dir, id); writeDBError(w, err, "") {
		return
	}
	reg, _ := insight.LoadRegistry(dir)
	restored, ok := reg.Get(id)
	if !ok {
		writeError(w, http.StatusInternalServerError, "restored lens failed to reload")
		return
	}
	s.logger.Info("insight lens restored to default", "lens", id)
	writeJSON(w, http.StatusOK, restored)
}

// handleListInsightLenses seeds the default lenses (if missing) and returns the
// workspace's lens catalog.
func (s *Server) handleListInsightLenses(w http.ResponseWriter, r *http.Request) {
	dir := insight.LensesDir(ws(r).DB.Root())
	if err := insight.EnsureDefaults(dir); writeDBError(w, err, "") {
		return
	}
	reg, _ := insight.LoadRegistry(dir)
	lenses := reg.List()
	if lenses == nil {
		lenses = []insight.Lens{}
	}
	writeJSON(w, http.StatusOK, lenses)
}

type insightScanReq struct {
	LensIDs         []string `json:"lensIds"`         // empty = all enabled lenses
	SessionAgentID  string   `json:"sessionAgentId"`  // filter scanned sessions by owning agent ("" = all)
	AnalysisAgentID string   `json:"analysisAgentId"` // agent whose model runs analysis ("" = default)
	IncludeArchived bool     `json:"includeArchived"`
	MaxSessions     int      `json:"maxSessions"`
}

// insightScanTimeout bounds a background scan. A first full scan over a large
// workspace makes one LLM call per (lens,session) pair and can take many minutes;
// subsequent scans are incremental (the ledger skips unchanged sessions).
const insightScanTimeout = 60 * time.Minute

// handleInsightScan starts one manual scan in the BACKGROUND and returns
// immediately. A scan can run for many minutes (one LLM call per (lens,session)
// pair), so running it inline would block the HTTP request and — since it used
// the request context — be cancelled the moment the user navigated away. The
// result surfaces asynchronously: findings land in the store and the run is
// recorded as a turn in the "🔍 İçgörü Taraması" session (with a live event).
func (s *Server) handleInsightScan(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[insightScanReq](w, r)
	if !ok {
		return
	}
	rt := ws(r).Runtime
	if rt.InsightScanActive() {
		writeError(w, http.StatusConflict, "bir tarama zaten çalışıyor")
		return
	}
	scope := insight.ScanScope{
		LensIDs:         req.LensIDs,
		AgentID:         req.SessionAgentID,
		IncludeArchived: req.IncludeArchived,
		MaxSessions:     req.MaxSessions,
	}
	agentID := req.AnalysisAgentID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), insightScanTimeout)
		defer cancel()
		res, err := rt.RunInsightScan(ctx, scope, agentID)
		if err != nil {
			if err != agent.ErrInsightScanBusy {
				s.logger.Warn("insight background scan failed", "error", err)
			}
			return
		}
		s.logger.Info("insight background scan done",
			"sessions", res.Sessions, "analyzed", res.Analyzed,
			"findings", res.Findings, "errors", len(res.Errors))
	}()
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// handleInsightScanStatus reports whether a background scan is currently running,
// so the panel can show a live "scanning…" state and auto-refresh findings when
// it finishes.
func (s *Server) handleInsightScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"scanning": ws(r).Runtime.InsightScanActive()})
}

// handleInsightRuns returns the workspace's recent scan-run log (append-only
// observability: when each scan ran, how long, what it covered/produced).
func (s *Server) handleInsightRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := insight.ReadRuns(ws(r).DB.Root(), 50)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// handleInsightFleetFindings aggregates app-fix findings across ALL workspaces
// into one deduplicated, fleet-wide backlog — the same TionHarness bug surfacing in
// several workspaces collapses to one row carrying combined weight + which
// workspaces hit it. Read-only; each workspace's own store is untouched.
func (s *Server) handleInsightFleetFindings(w http.ResponseWriter, r *http.Request) {
	byWorkspace := map[string][]insight.Finding{}
	for _, meta := range s.workspaces.List() {
		wsp, err := s.workspaces.Get(meta.ID)
		if err != nil || wsp == nil || wsp.DB == nil {
			continue
		}
		store, err := insight.OpenFindingStore(wsp.DB.Root())
		if err != nil {
			continue // a broken store must not blind the fleet view
		}
		if af := store.List("", insight.ChannelAppFix); len(af) > 0 {
			byWorkspace[meta.Name] = af
		}
	}
	out := insight.RollupAppFix(byWorkspace)
	if out == nil {
		out = []insight.FleetFinding{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListInsightFindings returns stored findings, optionally filtered by
// ?lens= and ?channel=.
func (s *Server) handleListInsightFindings(w http.ResponseWriter, r *http.Request) {
	store, err := insight.OpenFindingStore(ws(r).DB.Root())
	if writeDBError(w, err, "") {
		return
	}
	out := store.List(r.URL.Query().Get("lens"), insight.Channel(r.URL.Query().Get("channel")))
	if out == nil {
		out = []insight.Finding{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleResetInsight clears the workspace's accumulated insight data (findings +
// run log + workspace-opt actions doc; the ledger too when deep=true). Rejected
// while a scan is running so it can't race the in-flight writer.
func (s *Server) handleResetInsight(w http.ResponseWriter, r *http.Request) {
	if ws(r).Runtime.InsightScanActive() {
		writeError(w, http.StatusConflict, "bir tarama çalışıyor — bitmesini bekleyin")
		return
	}
	req, ok := bindJSON[struct {
		Deep bool `json:"deep"`
	}](w, r)
	if !ok {
		return
	}
	if err := insight.Reset(ws(r).DB.Root(), req.Deep); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true, "deep": req.Deep})
}

// handleDeleteInsightFinding removes one finding by id entirely (distinct from
// dismissing it, which keeps the row with a dismissed status). Backs the board-
// style triage UI's delete action.
func (s *Server) handleDeleteInsightFinding(w http.ResponseWriter, r *http.Request) {
	store, err := insight.OpenFindingStore(ws(r).DB.Root())
	if writeDBError(w, err, "") {
		return
	}
	found, err := store.Delete(r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if !found {
		http.Error(w, "finding not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("id")})
}

type insightStatusReq struct {
	Status string `json:"status"`
	// Evidence names the workspace entity the fix was applied to. Mandatory for
	// the "applied" status (insight.ErrAppliedNeedsEvidence); ignored otherwise.
	Evidence *insight.AppliedEntity `json:"evidence"`
}

// handleSetInsightFindingStatus updates one finding's lifecycle status (triage:
// accepted / dismissed / applied / verified). This is the workspace-opt review
// loop (_Docs/60 §5); no workspace mutation is performed here — the proposed fix
// stays advisory and the user/agent acts on it.
func (s *Server) handleSetInsightFindingStatus(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[insightStatusReq](w, r)
	if !ok {
		return
	}
	status := insight.FindingStatus(req.Status)
	if !insight.ValidStatus(status) {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	store, err := insight.OpenFindingStore(ws(r).DB.Root())
	if writeDBError(w, err, "") {
		return
	}
	found, err := store.SetStatus(r.PathValue("id"), status, time.Now().Unix(), req.Evidence)
	if errors.Is(err, insight.ErrAppliedNeedsEvidence) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if writeDBError(w, err, "") {
		return
	}
	if !found {
		http.Error(w, "finding not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok", "status": req.Status})
}

// handleGetInsightSettings returns the workspace insight settings.
func (s *Server) handleGetInsightSettings(w http.ResponseWriter, r *http.Request) {
	st, err := insight.LoadSettings(ws(r).DB.Root())
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleUpdateInsightSettings persists the workspace insight settings.
func (s *Server) handleUpdateInsightSettings(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[insight.Settings](w, r)
	if !ok {
		return
	}
	if err := insight.SaveSettings(ws(r).DB.Root(), req); writeDBError(w, err, "") {
		return
	}
	// Re-arm the auto-scan timer so a changed AutoScanCron takes effect immediately.
	if err := ws(r).Runtime.ReloadInsightCron(r.Context()); err != nil {
		s.logger.Warn("insight cron reload failed", "error", err)
	}
	writeJSON(w, http.StatusOK, req)
}
