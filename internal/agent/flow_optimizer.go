package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Flow evolution (_Docs/93). Three authors change a flow: the user (canvas),
// the agent itself (edit_flow / update_my_prompt) and the OBSERVER — the
// flow-optimizer system agent that looks at the last runs after every N turns
// and files a proposal (ops on the graph, optionally new soul/identity). Every
// change is a new immutable version; invariants live in code, not in prompts:
// the graph must validate, stay under the policy's growth budget, and a
// proposal below the confidence floor is filed, never auto-applied.

const (
	flowOptimizerSystemKey  = "flow-optimizer"
	flowOptimizerRecentRuns = 8
	flowOptimizerMaxTokens  = 4000
	flowOptimizerMaxOps     = 8
	flowOptimizerRunExcerpt = 500
	flowOptimizerStepOutput = 240
	flowOptimizerSoulRunes  = 2500

	flowOptimizeTriggerAuto   = "auto"
	flowOptimizeTriggerManual = "manual"

	// flowHealthyGrade is the grade (1..5) from which a run counts as "fine"
	// for the observer's skip rule.
	flowHealthyGrade = 4
)

var flowOptimizerSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "noChange":   { "type": "boolean" },
    "reason":     { "type": "string" },
    "expected":   { "type": "string" },
    "confidence": { "type": "number" },
    "ops":        { "type": "array", "items": { "type": "object" } },
    "prompt": {
      "type": "object",
      "properties": {
        "soul":     { "type": "string" },
        "identity": { "type": "string" }
      }
    }
  },
  "required": ["noChange", "reason", "confidence"]
}`)

// FlowOptimizeResult is what one observer pass produced.
type FlowOptimizeResult struct {
	FlowID   string           `json:"flowId"`
	Trigger  string           `json:"trigger"`
	Ran      bool             `json:"ran"`
	Skipped  string           `json:"skipped,omitempty"`
	Proposal *db.FlowProposal `json:"proposal,omitempty"`
	Applied  bool             `json:"applied"`
	// Held says why the proposal gate kept a confident proposal pending
	// instead of auto-applying it.
	Held      string               `json:"held,omitempty"`
	SessionID string               `json:"sessionId,omitempty"`
	Version   *db.FlowVersion      `json:"version,omitempty"`
	Prompt    *db.FlowPromptChange `json:"prompt,omitempty"`
}

type flowOptimizerReply struct {
	NoChange   bool                 `json:"noChange"`
	Reason     string               `json:"reason"`
	Expected   string               `json:"expected"`
	Confidence float64              `json:"confidence"`
	Ops        json.RawMessage      `json:"ops"`
	Prompt     *db.FlowPromptChange `json:"prompt"`
}

// flowOptimizerFlights serialises passes per flow.
var flowOptimizerFlights sync.Map

// maybeObserveFlow schedules an observer pass when the policy allows and enough
// new runs accumulated since the last look. Detached: it never delays the turn.
func (r *Runtime) maybeObserveFlow(ctx context.Context, flowID string) {
	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return
	}
	pol := f.Policy.Normalized()
	if pol.Mode == db.FlowPolicyOff {
		return
	}
	if f.Stats.Runs-f.Stats.RunsAtOptimize < pol.EveryRuns {
		return
	}
	if _, busy := flowOptimizerFlights.Load(flowID); busy {
		return
	}
	// A window of graded, well-rated runs has nothing to fix: move the marker
	// and save the observer call (grades come from the flow-grade authority).
	if recent, _ := r.db.ListFlowRuns(ctx, flowID, pol.EveryRuns); flowRunsHealthy(recent, pol.EveryRuns) {
		_ = r.db.MarkFlowOptimized(ctx, flowID)
		r.logger.Info("flow observer: recent runs all graded well, pass skipped", "flow", flowID, "runs", len(recent))
		return
	}
	if !r.startBackgroundTurn(func() {
		if _, err := r.OptimizeFlow(context.Background(), flowID, flowOptimizeTriggerAuto); err != nil {
			r.logger.Warn("flow observer: pass failed", "flow", flowID, "error", err)
		}
	}) {
		r.logger.Debug("flow observer: pass skipped, workspace closing", "flow", flowID)
	}
}

// flowRunsHealthy reports whether every one of the last n runs succeeded, was
// graded at least flowHealthyGrade and got no thumbs-down — the case where an
// observer pass would only spend tokens to say "no change".
func flowRunsHealthy(runs []db.FlowRun, n int) bool {
	if n <= 0 || len(runs) < n {
		return false
	}
	for _, run := range runs[:n] {
		if run.Status != db.FlowSuccess || run.Grade < flowHealthyGrade || run.Feedback < 0 {
			return false
		}
	}
	return true
}

// OptimizeFlow runs one observer pass: gather the evidence, ask the optimizer
// agent, validate its answer in code, file a proposal and (policy auto +
// confident) apply it as a new version.
func (r *Runtime) OptimizeFlow(ctx context.Context, flowID, trigger string) (FlowOptimizeResult, error) {
	res := FlowOptimizeResult{FlowID: flowID, Trigger: trigger}
	if _, busy := flowOptimizerFlights.LoadOrStore(flowID, true); busy {
		res.Skipped = "a pass is already running for this flow"
		return res, nil
	}
	defer flowOptimizerFlights.Delete(flowID)

	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return res, fmt.Errorf("flow %s: %w", flowID, err)
	}
	pol := f.Policy.Normalized()
	if pol.Mode == db.FlowPolicyOff && trigger != flowOptimizeTriggerManual {
		res.Skipped = "policy is off"
		return res, nil
	}
	g, err := flow.Parse(f.Graph)
	if err != nil {
		return res, fmt.Errorf("flow %s graph: %w", flowID, err)
	}
	owner, err := r.db.GetAgent(ctx, f.AgentID)
	if err != nil {
		return res, fmt.Errorf("flow %s agent: %w", flowID, err)
	}
	runs, _ := r.db.ListFlowRuns(ctx, flowID, flowOptimizerRecentRuns)
	// Every pass moves the marker, including the ones that end in "nothing to
	// say": the policy's EveryRuns counts new runs since the observer LOOKED.
	_ = r.db.MarkFlowOptimized(ctx, flowID)
	if len(runs) == 0 {
		res.Skipped = "no runs yet"
		return res, nil
	}
	proposals, _ := r.db.ListFlowProposals(ctx, flowID)

	exec, system, err := r.resolveAnalysisSystemAgent(flowOptimizerSystemKey, owner)
	if err != nil {
		res.Skipped = "optimizer agent unavailable"
		return res, nil
	}
	if strings.TrimSpace(system) == "" {
		system = r.readPrompt("flow-optimizer")
	}
	lang := ""
	if r.tun != nil {
		lang = r.tun.Language()
	}
	user := flowOptimizerUserPrompt(f, g, owner, runs, proposals, lang)
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), flowOptimizerSystemKey, system), exec, providers.Request{
		Model: exec.Model, System: system, MaxTokens: flowOptimizerMaxTokens, OutputSchema: flowOptimizerSchema,
		Messages: []providers.Message{{Role: providers.RoleUser, Text: user}},
	}, false)
	if err != nil {
		return res, fmt.Errorf("flow observer: model call: %w", err)
	}
	res.SessionID = r.recordSystemAgentSession(ctx, flowOptimizerSystemKey, exec, "Akış gözlemcisi: "+f.Name, user, resp.Text)
	raw := extractJSONObject(resp.Text)
	var reply flowOptimizerReply
	if raw == "" || json.Unmarshal([]byte(raw), &reply) != nil {
		return res, errors.New("flow observer: no JSON in reply")
	}
	res.Ran = true
	if reply.NoChange || (len(reply.Ops) == 0 && reply.Prompt == nil) {
		res.Skipped = "observer found nothing to change: " + strings.TrimSpace(reply.Reason)
		r.logger.Info("flow observer: no change", "flow", flowID, "reason", reply.Reason)
		return res, nil
	}
	if !pol.AllowPromptChanges {
		reply.Prompt = nil
	}
	if reply.Prompt != nil && reply.Prompt.Soul == nil && reply.Prompt.Identity == nil {
		reply.Prompt = nil
	}
	if reply.Confidence < 0 {
		reply.Confidence = 0
	}
	if reply.Confidence > 1 {
		reply.Confidence = 1
	}
	evidence := make([]string, 0, len(runs))
	for _, run := range runs {
		evidence = append(evidence, run.ID)
	}
	prop := db.FlowProposal{
		FlowID:      f.ID,
		AgentID:     f.AgentID,
		BaseVersion: f.Version,
		Author:      db.FlowAuthor{Kind: db.FlowAuthorObserver, ID: exec.ID},
		Trigger:     trigger,
		Ops:         compactJSON(reply.Ops),
		Prompt:      reply.Prompt,
		Reason:      strings.TrimSpace(reply.Reason),
		Expected:    strings.TrimSpace(reply.Expected),
		Confidence:  reply.Confidence,
		Evidence:    strings.Join(evidence, ","),
	}
	// Validate in code before filing: an unparseable or invalid op set is
	// recorded as invalid so the history shows what the observer tried.
	if verr := r.validateProposal(g, pol, prop); verr != nil {
		prop.Status = db.ProposalInvalid
		prop.Error = verr.Error()
	}
	prop, err = r.db.CreateFlowProposal(ctx, prop)
	if err != nil {
		return res, err
	}
	res.Proposal = &prop
	if prop.Status == db.ProposalInvalid {
		r.emitFlowChanged(f.ID, "🧬 Akış gözlemcisi geçersiz bir öneri üretti — "+f.Name, prop.Error)
		return res, nil
	}
	if pol.Mode == db.FlowPolicyAuto && prop.Confidence >= pol.MinConfidence {
		if ok, hold := r.gateFlowProposal(ctx, exec, f, g, prop); !ok {
			res.Held = hold
			r.emitFlowChanged(f.ID, "🧬 Akış için öneri bekliyor (kapı tuttu) — "+f.Name, hold+": "+prop.Reason)
			return res, nil
		}
		applied, aerr := r.ApplyFlowProposal(ctx, prop.ID, db.FlowAuthor{Kind: db.FlowAuthorObserver, ID: exec.ID})
		if aerr != nil {
			r.logger.Warn("flow observer: auto-apply failed", "flow", flowID, "proposal", prop.ID, "error", aerr)
		} else {
			res.Applied = true
			res.Proposal = &applied
			r.emitFlowChanged(f.ID, "🧬 Akış evrildi — "+f.Name, fmt.Sprintf("v%d: %s", applied.AppliedVersion, applied.Reason))
			return res, nil
		}
	}
	r.emitFlowChanged(f.ID, "🧬 Akış için öneri var — "+f.Name, prop.Reason)
	return res, nil
}

// validateProposal checks a proposal's ops against the head graph and the
// policy budget without applying anything.
func (r *Runtime) validateProposal(head flow.Graph, pol db.FlowPolicy, p db.FlowProposal) error {
	if len(p.Ops) > 0 && string(p.Ops) != "null" {
		ops, err := flow.ParseOps(p.Ops)
		if err != nil {
			return err
		}
		if len(ops) > flowOptimizerMaxOps {
			return fmt.Errorf("too many ops (%d > %d)", len(ops), flowOptimizerMaxOps)
		}
		next, err := flow.Apply(head, ops)
		if err != nil {
			return err
		}
		if len(next.Nodes) > pol.MaxNodes {
			return fmt.Errorf("growth budget: %d nodes exceeds the policy cap %d", len(next.Nodes), pol.MaxNodes)
		}
	} else if p.Prompt == nil {
		return errors.New("proposal changes nothing")
	}
	if p.Prompt != nil {
		if p.Prompt.Soul != nil && strings.TrimSpace(*p.Prompt.Soul) == "" {
			return errors.New("proposed soul is empty")
		}
	}
	return nil
}

// ApplyFlowProposal applies a pending proposal on top of the CURRENT head
// (not necessarily its base version): ops are re-applied and re-validated, the
// prompt change becomes a prompt version. The proposal is resolved either way.
func (r *Runtime) ApplyFlowProposal(ctx context.Context, proposalID string, author db.FlowAuthor) (db.FlowProposal, error) {
	p, err := r.db.GetFlowProposal(ctx, proposalID)
	if err != nil {
		return p, err
	}
	if p.Status != db.ProposalPending {
		return p, fmt.Errorf("proposal %s is %s", p.ID, p.Status)
	}
	f, err := r.db.GetFlow(ctx, p.FlowID)
	if err != nil {
		return p, err
	}
	version := 0
	if len(p.Ops) > 0 && string(p.Ops) != "null" {
		ops, perr := flow.ParseOps(p.Ops)
		if perr != nil {
			resolved, _ := r.db.ResolveFlowProposal(ctx, p.ID, db.ProposalInvalid, 0, perr.Error())
			return resolved, perr
		}
		v, aerr := r.ApplyFlowOps(ctx, f.ID, ops, author, p.Reason, p.ID)
		if aerr != nil {
			resolved, _ := r.db.ResolveFlowProposal(ctx, p.ID, db.ProposalInvalid, 0, aerr.Error())
			return resolved, aerr
		}
		version = v.Version
	}
	if p.Prompt != nil {
		perr := r.UpdateAgentPrompts(ctx, f.AgentID, p.Prompt.Soul, p.Prompt.Identity, author, p.Reason, p.ID)
		if perr != nil && !errors.Is(perr, errPromptUnchanged) {
			resolved, _ := r.db.ResolveFlowProposal(ctx, p.ID, db.ProposalInvalid, version, perr.Error())
			return resolved, fmt.Errorf("prompt change: %w", perr)
		}
	}
	return r.db.ResolveFlowProposal(ctx, p.ID, db.ProposalApplied, version, "")
}

// RejectFlowProposal closes a pending proposal without applying it.
func (r *Runtime) RejectFlowProposal(ctx context.Context, proposalID string) (db.FlowProposal, error) {
	p, err := r.db.GetFlowProposal(ctx, proposalID)
	if err != nil {
		return p, err
	}
	if p.Status != db.ProposalPending {
		return p, fmt.Errorf("proposal %s is %s", p.ID, p.Status)
	}
	return r.db.ResolveFlowProposal(ctx, p.ID, db.ProposalRejected, 0, "")
}

// ApplyFlowOps applies ops to the head graph and commits the result as a new
// version. Agents and the observer are held to the policy's growth budget; the
// user is bounded only by the hard cap.
func (r *Runtime) ApplyFlowOps(ctx context.Context, flowID string, ops []flow.Op, author db.FlowAuthor, reason, proposalID string) (db.FlowVersion, error) {
	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return db.FlowVersion{}, err
	}
	head, err := flow.Parse(f.Graph)
	if err != nil {
		return db.FlowVersion{}, fmt.Errorf("head graph: %w", err)
	}
	next, err := flow.Apply(head, ops)
	if err != nil {
		return db.FlowVersion{}, err
	}
	return r.commitFlowGraph(ctx, f, head, next, author, reason, proposalID)
}

// SaveFlowGraph commits a whole graph (the canvas save) as a new version.
func (r *Runtime) SaveFlowGraph(ctx context.Context, flowID string, g flow.Graph, author db.FlowAuthor, reason string) (db.FlowVersion, error) {
	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return db.FlowVersion{}, err
	}
	head, _ := flow.Parse(f.Graph)
	g = g.Normalized()
	if err := g.Validate(); err != nil {
		return db.FlowVersion{}, err
	}
	// A layout-only change (positions) keeps the version: it is persisted in
	// place and reported as "no change" so the caller does not announce a new
	// version for a drag.
	if flow.Diff(head, g).Empty() {
		if flow.Encode(head) != flow.Encode(g) {
			if _, err := r.db.UpdateFlowLayout(ctx, f.ID, g); err != nil {
				return db.FlowVersion{}, err
			}
		}
		return db.FlowVersion{}, errors.New("no change")
	}
	return r.commitFlowGraph(ctx, f, head, g, author, reason, "")
}

// RevertFlow makes an earlier version the head again (as a NEW version, so the
// history stays append-only).
func (r *Runtime) RevertFlow(ctx context.Context, flowID string, version int, author db.FlowAuthor, reason string) (db.FlowVersion, error) {
	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return db.FlowVersion{}, err
	}
	if version == f.Version {
		return db.FlowVersion{}, fmt.Errorf("version %d is already the head", version)
	}
	old, err := r.db.GetFlowVersion(ctx, flowID, version)
	if err != nil {
		return db.FlowVersion{}, fmt.Errorf("version %d: %w", version, err)
	}
	g, err := flow.Parse(old.Graph)
	if err != nil {
		return db.FlowVersion{}, fmt.Errorf("version %d graph: %w", version, err)
	}
	if err := g.Validate(); err != nil {
		return db.FlowVersion{}, fmt.Errorf("version %d no longer validates: %w", version, err)
	}
	head, _ := flow.Parse(f.Graph)
	if strings.TrimSpace(reason) == "" {
		reason = fmt.Sprintf("v%d sürümüne geri dönüldü", version)
	}
	return r.commitFlowGraph(ctx, f, head, g, author, reason, "")
}

func (r *Runtime) commitFlowGraph(ctx context.Context, f db.Flow, head, next flow.Graph, author db.FlowAuthor, reason, proposalID string) (db.FlowVersion, error) {
	if author.Kind != db.FlowAuthorUser {
		if cap := f.Policy.Normalized().MaxNodes; len(next.Nodes) > cap {
			return db.FlowVersion{}, fmt.Errorf("growth budget: %d nodes exceeds the policy cap %d", len(next.Nodes), cap)
		}
	}
	if flow.Diff(head, next).Empty() && flow.Encode(head.Normalized()) == flow.Encode(next.Normalized()) {
		return db.FlowVersion{}, errors.New("no change")
	}
	v, err := r.db.CommitFlowVersion(ctx, f.ID, next, author, reason, proposalID)
	if err != nil {
		return v, err
	}
	r.logger.Info("flow: new version", "flow", f.ID, "agent", f.AgentID, "version", v.Version, "author", author.Kind, "diff", v.Diff)
	if author.Kind != db.FlowAuthorUser {
		r.emitFlowChanged(f.ID, "🧬 Akış evrildi — "+f.Name, fmt.Sprintf("v%d (%s): %s", v.Version, author.Kind, v.Reason))
	}
	return v, nil
}

// errPromptUnchanged reports a prompt update that would write the same text.
var errPromptUnchanged = errors.New("prompt unchanged")

// UpdateAgentPrompts changes an agent's soul/identity with a recorded prompt
// version before and after, so the change is reversible. nil leaves a field.
func (r *Runtime) UpdateAgentPrompts(ctx context.Context, agentID string, soul, identity *string, author db.FlowAuthor, reason, proposalID string) error {
	if soul == nil && identity == nil {
		return errors.New("nothing to change")
	}
	a, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if soul != nil && strings.TrimSpace(*soul) == "" && author.Kind != db.FlowAuthorUser {
		return errors.New("soul cannot be empty")
	}
	if (soul == nil || strings.TrimSpace(*soul) == strings.TrimSpace(a.Soul)) && (identity == nil || strings.TrimSpace(*identity) == strings.TrimSpace(a.Identity)) {
		return errPromptUnchanged
	}
	// Version 1 is the state before any evolution; written once, lazily.
	if existing, _ := r.db.ListAgentPromptVersions(ctx, agentID); len(existing) == 0 {
		if _, err := r.db.RecordAgentPromptVersion(ctx, agentID, db.FlowAuthor{Kind: db.FlowAuthorSystem}, "evrim öncesi prompt", ""); err != nil {
			return err
		}
	}
	patch := db.AgentProfilePatch{}
	if soul != nil {
		s := strings.TrimSpace(*soul)
		patch.Soul = &s
	}
	if identity != nil {
		i := strings.TrimSpace(*identity)
		patch.Identity = &i
	}
	if _, err := r.db.UpdateAgent(ctx, agentID, patch); err != nil {
		return err
	}
	if _, err := r.db.RecordAgentPromptVersion(ctx, agentID, author, reason, proposalID); err != nil {
		return err
	}
	r.logger.Info("flow: agent prompts evolved", "agent", agentID, "author", author.Kind, "reason", reason)
	return nil
}

// RestoreAgentPromptVersion puts an earlier prompt version back (as a new one).
func (r *Runtime) RestoreAgentPromptVersion(ctx context.Context, agentID string, version int, author db.FlowAuthor) error {
	versions, err := r.db.ListAgentPromptVersions(ctx, agentID)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if v.Version == version {
			return r.UpdateAgentPrompts(ctx, agentID, &v.Soul, &v.Identity, author, fmt.Sprintf("prompt v%d geri yüklendi", version), "")
		}
	}
	return fmt.Errorf("prompt version %d not found", version)
}

func compactJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var buf strings.Builder
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	buf.Write(b)
	return json.RawMessage(buf.String())
}

// flowOptimizerUserPrompt renders the evidence the observer reasons over.
func flowOptimizerUserPrompt(f db.Flow, g flow.Graph, owner db.Agent, runs []db.FlowRun, proposals []db.FlowProposal, lang string) string {
	var b strings.Builder
	pol := f.Policy.Normalized()
	fmt.Fprintf(&b, "# Flow\nname: %s (%s)\nagent: %s (%s)\nversion: v%d\npolicy: mode=%s everyRuns=%d minConfidence=%.2f maxNodes=%d promptChanges=%v\n",
		f.Name, f.ID, owner.Name, owner.ID, f.Version, pol.Mode, pol.EveryRuns, pol.MinConfidence, pol.MaxNodes, pol.AllowPromptChanges)
	if f.Note != "" {
		fmt.Fprintf(&b, "note: %s\n", f.Note)
	}
	fmt.Fprintf(&b, "shape: %s\n", g.Summary())
	fmt.Fprintf(&b, "\n## Graph JSON\n%s\n", flow.Encode(g))
	fmt.Fprintf(&b, "\n# Agent prompts\n## soul\n%s\n## identity\n%s\n", truncateRunes(owner.Soul, flowOptimizerSoulRunes), truncateRunes(owner.Identity, flowOptimizerSoulRunes/2))
	st := f.Stats
	avgMs, avgTok := int64(0), int64(0)
	if st.Runs > 0 {
		avgMs = st.TotalMs / int64(st.Runs)
		avgTok = st.TotalTokens / int64(st.Runs)
	}
	fmt.Fprintf(&b, "\n# Stats (all runs)\nruns=%d success=%d failure=%d avgMs=%d avgTokens=%d", st.Runs, st.Success, st.Failure, avgMs, avgTok)
	if st.Graded > 0 {
		fmt.Fprintf(&b, " avgGrade=%.1f/5 (%d graded by the decision model)", st.AvgGrade(), st.Graded)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "\n# Recent runs (newest first, %d)\n", len(runs))
	for _, run := range runs {
		fb := ""
		switch {
		case run.Feedback > 0:
			fb = " feedback=👍"
		case run.Feedback < 0:
			fb = " feedback=👎"
		}
		if run.Grade > 0 {
			fb += fmt.Sprintf(" grade=%d/5", run.Grade)
		}
		fmt.Fprintf(&b, "\n- %s [%s] v%d %d steps %dms %d+%d tokens%s\n  input: %s\n", run.ID, run.Status, run.Version, run.StepCount, run.DurationMs,
			run.Usage.InputTokens, run.Usage.OutputTokens, fb, oneLineExcerpt(run.Input, flowOptimizerRunExcerpt))
		if run.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", oneLineExcerpt(run.Error, flowOptimizerStepOutput))
		}
		var steps []flow.Step
		if len(run.Steps) > 0 && json.Unmarshal(run.Steps, &steps) == nil {
			for _, s := range steps {
				line := fmt.Sprintf("  %d. %s(%s)", s.Index, s.NodeID, s.Type)
				if s.Visit > 1 {
					line += fmt.Sprintf(" visit %d", s.Visit)
				}
				if s.Type == flow.NodeRoute {
					line += fmt.Sprintf(" → %s", s.Edge)
				}
				if s.Detail != "" {
					line += " [" + oneLineExcerpt(s.Detail, 160) + "]"
				}
				if s.DurationMs > 0 {
					line += fmt.Sprintf(" %dms", s.DurationMs)
				}
				if s.Type == flow.NodeLLM || s.Type == flow.NodeOutput {
					line += ": " + oneLineExcerpt(s.Output, flowOptimizerStepOutput)
				}
				if s.Error != "" {
					line += " ERROR " + oneLineExcerpt(s.Error, 120)
				}
				b.WriteString(line + "\n")
			}
		} else {
			fmt.Fprintf(&b, "  output: %s\n", oneLineExcerpt(run.Output, flowOptimizerRunExcerpt))
		}
	}
	if len(proposals) > 0 {
		b.WriteString("\n# Earlier proposals (newest first)\n")
		for i, p := range proposals {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "- %s [%s] conf=%.2f base=v%d: %s", p.ID, p.Status, p.Confidence, p.BaseVersion, oneLineExcerpt(p.Reason, 200))
			if p.Error != "" {
				fmt.Fprintf(&b, " (error: %s)", oneLineExcerpt(p.Error, 120))
			}
			b.WriteString("\n")
		}
	}
	if lang != "" {
		fmt.Fprintf(&b, "\nWrite reason/expected in this language: %s. Node titles and prompts too.\n", lang)
	}
	return b.String()
}

func oneLineExcerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, n)
}

// FlowRunsSince counts the runs recorded after a point in time (used by tests
// and the API summary).
func (r *Runtime) FlowRunsSince(ctx context.Context, flowID string, since time.Time) int {
	runs, _ := r.db.ListFlowRuns(ctx, flowID, 0)
	n := 0
	for _, run := range runs {
		if run.CreatedAt >= since.Unix() {
			n++
		}
	}
	return n
}
