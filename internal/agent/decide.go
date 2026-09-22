package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Glue between the runtime and the decision-model layer (internal/decider).
// Every site follows the same contract:
//
//   - off:    the site behaves exactly as before; nothing is sent anywhere.
//   - shadow: the site decides with its own logic; the decider is asked in the
//             background (never delaying the caller) and both verdicts land in
//             the ledger, so agreement can be measured before switching on.
//   - on:     the decider's verdict drives the site; any error — no endpoint,
//             timeout, backoff — falls back to the site's previous logic.

// deciderHub returns the app-wide hub, nil when none is wired.
func (r *Runtime) deciderHub() *decider.Hub {
	if r == nil {
		return nil
	}
	return r.tun.Decider()
}

// deciderMode is a site's effective mode (off when no hub is wired).
func (r *Runtime) deciderMode(site string) decider.Mode {
	return r.deciderHub().Mode(site)
}

// deciderThreshold is a site's configured threshold.
func (r *Runtime) deciderThreshold(site string) float64 {
	return r.deciderHub().Threshold(site)
}

// decide asks the decision model on behalf of caller. The call's usage is
// recorded against the caller — billed under the backend's provider (the price
// table knows the decision model there, not under the caller's own provider)
// and labelled with the "decide" call kind. Any error means "use your own
// logic".
func (r *Runtime) decide(ctx context.Context, site string, caller db.Agent, req decider.Request) (*decider.Response, error) {
	hub := r.deciderHub()
	if hub == nil {
		return nil, decider.ErrDisabled
	}
	resp, err := hub.Decide(ctx, site, req)
	if err != nil {
		return nil, err
	}
	r.recordDecisionUsage(ctx, caller, resp)
	return resp, nil
}

// recordDecisionUsage bills one decision to caller. A call made on behalf of
// no agent (caller.ID == "") still appears in the decider ledger with its cost,
// it just has no agent budget to land in.
func (r *Runtime) recordDecisionUsage(ctx context.Context, caller db.Agent, resp *decider.Response) {
	if caller.ID == "" || r.db == nil || resp == nil {
		return
	}
	billed := caller
	billed.Provider = resp.BillingProvider
	if billed.Provider == "" {
		billed.Provider = resp.Backend
	}
	r.RecordUsage(WithCallKind(ctx, KindDecide), billed, resp.Model, providers.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, 1)
}

// logDecision appends a record to the decider ledger (no-op without a hub).
func (r *Runtime) logDecision(rec decider.Record) {
	r.deciderHub().Log(rec)
}

// decisionOff reports whether err only says the site is switched off — not a
// failure worth a ledger line.
func decisionOff(err error) bool {
	return errors.Is(err, decider.ErrDisabled) || errors.Is(err, decider.ErrSiteOff)
}

// shadowDecision asks the decider in the background, next to a verdict the site
// already reached with its own logic (baseline), and logs both — when the site
// is in shadow mode. outcome maps a response onto the site's verdict vocabulary.
func (r *Runtime) shadowDecision(ctx context.Context, site string, caller db.Agent, req decider.Request, baseline, ref string, outcome func(*decider.Response) (string, float64)) {
	if r.deciderMode(site) != decider.ModeShadow {
		return
	}
	r.backgroundDecision(ctx, site, decider.ModeShadow, caller, req, baseline, ref, outcome)
}

// backgroundDecision asks the decider without changing behaviour or delaying
// the caller, and logs its verdict next to baseline. mode is what the record
// says: shadow, or on for an on-mode site that had nobody to act for (an
// unattended run the site never blocks). It runs under the runtime's
// background-turn barrier, so a workspace close waits for it instead of closing
// the store under it.
func (r *Runtime) backgroundDecision(ctx context.Context, site string, mode decider.Mode, caller db.Agent, req decider.Request, baseline, ref string, outcome func(*decider.Response) (string, float64)) {
	if mode == decider.ModeOff || r.deciderHub() == nil {
		return
	}
	bg := context.WithoutCancel(ctx)
	r.startBackgroundTurn(func() {
		resp, err := r.decide(bg, site, caller, req)
		if decisionOff(err) {
			return
		}
		rec := decider.NewRecord(site, mode, resp, err)
		rec.Baseline, rec.Ref = baseline, ref
		if err == nil {
			rec.Outcome, rec.Strength = outcome(resp)
		} else {
			r.logger.Debug("shadow decision failed", "site", site, "error", err)
		}
		r.logDecision(rec)
	})
}
