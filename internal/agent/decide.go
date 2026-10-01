package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Glue between the runtime and the decision-model layer (internal/decider).
// Every authority (decide_authorities.go) follows the same contract:
//
//   - off:    it behaves exactly as before; nothing is sent anywhere.
//   - shadow: it decides with its own logic; the decider is asked in the
//             background (never delaying the caller) and both verdicts land in
//             the ledger, so agreement can be measured before switching on.
//   - on:     the decider's verdict drives it; any error — no model, timeout,
//             backoff, the fallback failing too — falls back to the previous
//             logic (a fail-closed authority refuses instead).

// deciderHub returns the app-wide hub, nil when none is wired.
func (r *Runtime) deciderHub() *decider.Hub {
	if r == nil {
		return nil
	}
	return r.tun.Decider()
}

// deciderMode is an authority's effective mode (off when no hub is wired).
func (r *Runtime) deciderMode(authority string) decider.Mode {
	return r.deciderHub().Mode(authority)
}

// deciderThreshold is an authority's configured threshold.
func (r *Runtime) deciderThreshold(authority string) float64 {
	return r.deciderHub().Threshold(authority)
}

// decide asks the decision model on behalf of caller. Every model call the
// decision makes — the authority's model, its fallback, a background
// challenger — is recorded against the caller, billed under the model's own
// price-table provider (not the caller's) and labelled with the "decide" call
// kind; the challenger runs under the runtime's background-turn barrier. opts
// add to (and may override) these, e.g. a ledger ref or an outcome vocabulary.
// Any error means "use your own logic".
func (r *Runtime) decide(ctx context.Context, authority string, caller db.Agent, req decider.Request, opts ...decider.CallOption) (*decider.Response, error) {
	hub := r.deciderHub()
	if hub == nil {
		return nil, decider.ErrDisabled
	}
	all := append([]decider.CallOption{
		decider.WithBilling(func(bctx context.Context, resp *decider.Response) { r.recordDecisionUsage(bctx, caller, resp) }),
		decider.WithBackground(func(f func()) { r.startBackgroundTurn(f) }),
		decider.WithRef(SessionIDFrom(ctx)),
		decider.WithLocation(SessionIDFrom(ctx), TurnIDFrom(ctx)),
		decider.WithWorkspace(r.WorkspaceID()),
	}, opts...)
	return hub.Decide(ctx, authority, req, all...)
}

// recordDecisionUsage bills one decision call to caller. A call made on behalf
// of no agent (caller.ID == "") still appears in the decider ledger with its
// cost, it just has no agent budget to land in.
func (r *Runtime) recordDecisionUsage(ctx context.Context, caller db.Agent, resp *decider.Response) {
	if caller.ID == "" || r.db == nil || resp == nil {
		return
	}
	billed := caller
	billed.Provider = resp.BillingProvider
	if billed.Provider == "" {
		billed.Provider = resp.Backend
	}
	r.RecordUsage(WithCallKind(ctx, KindDecide), billed, resp.BilledModel(), providers.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, 1)
}

// logDecision appends a record to the decider ledger (no-op without a hub).
func (r *Runtime) logDecision(rec decider.Record) {
	r.deciderHub().Log(rec)
}

// decisionOff reports whether err only says the authority is switched off —
// not a failure worth a ledger line.
func decisionOff(err error) bool {
	return errors.Is(err, decider.ErrDisabled) || errors.Is(err, decider.ErrSiteOff)
}

// shadowDecision asks the decider in the background, next to a verdict the
// authority already reached with its own logic (baseline), and logs both — when
// the authority is in shadow mode. outcome maps a response onto the
// authority's verdict vocabulary.
func (r *Runtime) shadowDecision(ctx context.Context, authority string, caller db.Agent, req decider.Request, baseline, ref string, outcome func(*decider.Response) (string, float64)) {
	if r.deciderMode(authority) != decider.ModeShadow {
		return
	}
	r.backgroundDecision(ctx, authority, decider.ModeShadow, caller, req, baseline, ref, outcome)
}

// backgroundDecision asks the decider without changing behaviour or delaying
// the caller, and logs its verdict next to baseline. mode is what the record
// says: shadow, or on for an on-mode authority that had nobody to act for (an
// unattended run it never blocks). It runs under the runtime's background-turn
// barrier, so a workspace close waits for it instead of closing the store
// under it.
func (r *Runtime) backgroundDecision(ctx context.Context, authority string, mode decider.Mode, caller db.Agent, req decider.Request, baseline, ref string, outcome func(*decider.Response) (string, float64)) {
	if mode == decider.ModeOff || r.deciderHub() == nil {
		return
	}
	bg := context.WithoutCancel(ctx)
	r.startBackgroundTurn(func() {
		resp, err := r.decide(bg, authority, caller, req, decider.WithRef(ref), decider.WithOutcome(outcome))
		if decisionOff(err) {
			return
		}
		rec := decider.NewRecord(authority, mode, resp, err)
		rec.Baseline, rec.Ref = baseline, ref
		if err == nil {
			rec.Outcome, rec.Strength = outcome(resp)
		} else {
			r.logger.Debug("background decision failed", "authority", authority, "error", err)
		}
		r.logDecision(rec)
	})
}
