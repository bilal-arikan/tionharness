package agent

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

type policyVerdict struct {
	Outcome  string
	Items    []db.DecisionItem
	Strength float64
}

type policyApply func(policyVerdict) (string, error)

func policyAnswer(resp *decider.Response, key string) (decider.Answer, bool) {
	if resp == nil {
		return decider.Answer{}, false
	}
	a, ok := resp.Answers[key]
	return a, ok
}

func (r *Runtime) policyConfig(authority string) decider.AuthorityConfig {
	if h := r.deciderHub(); h != nil {
		return h.Config().Authorities[authority].WithWorkflowDefaults()
	}
	return (decider.AuthorityConfig{}).WithWorkflowDefaults()
}

// sessionPolicy keeps shadow calls outside the critical path. Only the active
// callback may change behavior. Both paths retain metadata and the trace link.
func (r *Runtime) sessionPolicy(ctx context.Context, agent db.Agent, authority string, req decider.Request, baseline string, evaluate func(*decider.Response) policyVerdict, apply policyApply) {
	mode := r.deciderMode(authority)
	if mode == decider.ModeOff || SessionIDFrom(ctx) == "" {
		return
	}
	req = r.fitDecisionEvidence(authority, req)
	run := func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		outcome := func(resp *decider.Response) (string, float64) { v := evaluate(resp); return v.Outcome, v.Strength }
		resp, err := r.decide(ctx, authority, agent, req, decider.WithOutcome(outcome))
		if decisionOff(err) {
			return
		}
		rec := decider.NewRecord(authority, mode, resp, err)
		rec.Ref, rec.Baseline = SessionIDFrom(ctx), baseline
		entry := db.SessionDecision{Authority: authority, Mode: string(mode), Status: "fallback", Baseline: baseline, Applied: baseline, TraceID: rec.DebugID, Items: []db.DecisionItem{}, Error: rec.Error}
		if err == nil {
			v := evaluate(resp)
			rec.Outcome, rec.Strength = v.Outcome, v.Strength
			entry.Recommended, entry.Items = v.Outcome, v.Items
			if mode == decider.ModeShadow {
				entry.Status = "observed"
			} else if apply != nil {
				actual, applyErr := apply(v)
				if applyErr == nil {
					entry.Applied = actual
					entry.Status = "applied"
					rec.Applied = actual != baseline
				} else {
					entry.Error = "application_failed"
					rec.Error = "application_failed"
				}
			}
		}
		r.logDecision(rec)
		r.savePolicyEntry(context.WithoutCancel(ctx), SessionIDFrom(ctx), entry)
	}
	if mode == decider.ModeShadow {
		bg := context.WithoutCancel(ctx)
		r.startBackgroundTurn(func() { run(bg) })
		return
	}
	run(ctx)
}

func (r *Runtime) savePolicyEntry(ctx context.Context, sessionID string, entry db.SessionDecision) {
	if r.db == nil || sessionID == "" {
		return
	}
	if err := r.db.AppendSessionDecision(ctx, sessionID, entry); err != nil {
		r.logger.Warn("session decision could not be persisted", "session", sessionID, "authority", entry.Authority, "error", err)
	}
}

// selectedVerdict uses stable keys rather than free-form model output. Every
// item is from the permission-filtered candidate list supplied by the caller.
func selectedVerdict(resp *decider.Response, candidates []db.DecisionItem, threshold float64, limit int) policyVerdict {
	v := policyVerdict{Outcome: "none", Items: make([]db.DecisionItem, 0, len(candidates))}
	for i, item := range candidates {
		a, ok := policyAnswer(resp, questionKey(i))
		item.Action = "skip"
		if ok {
			item.Strength = a.Probability
			if a.Yes(threshold) {
				item.Action = "select"
			}
		}
		v.Items = append(v.Items, item)
	}
	indices := []int{}
	for i, it := range v.Items {
		if it.Action == "select" {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool { return v.Items[indices[i]].Strength > v.Items[indices[j]].Strength })
	for i, idx := range indices {
		if i >= limit {
			v.Items[idx].Action = "budget"
		}
	}
	keys := []string{}
	for _, it := range v.Items {
		if it.Action == "select" {
			keys = append(keys, it.Key)
			v.Strength = max(v.Strength, it.Strength)
		}
	}
	if len(keys) > 0 {
		v.Outcome = strings.Join(keys, ",")
	}
	return v
}

func questionKey(i int) string { return "candidate_" + strconv.Itoa(i) }

func selectionRequest(state any, candidates []db.DecisionItem, question string) decider.Request {
	questions := make(map[string]decider.Question, len(candidates))
	for i, c := range candidates {
		questions[questionKey(i)] = decider.Noul(question+" Candidate: "+c.Label+" ("+c.Key+")", "Useful", "Not needed")
	}
	return decider.Request{State: state, Questions: questions}
}
