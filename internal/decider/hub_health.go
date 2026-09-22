package decider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Failure handling, per decision model. A decision is an optimisation on top of
// logic that already works, so a failing model must cost the caller as little
// as possible: consecutive transient failures open a short circuit (callers
// fall back at once instead of waiting out another timeout), and a credential
// or billing rejection quarantines the model until its settings (or the
// provider settings it borrows from) change or the quarantine expires (a 402
// clears once the account is topped up). One model resting never affects
// another, so a dead local server does not take the hosted fallback with it.
const (
	breakerThreshold = 3
	breakerCooldown  = time.Minute
	quarantineFor    = 10 * time.Minute
)

type modelHealth struct {
	failures      int
	openUntil     time.Time
	quarantine    time.Time
	quarantineGen uint64
	quarantineWhy string
	lastProblem   string
	lastProblemAt time.Time
}

// healthOf returns id's record, creating it. Called with healthMu held.
func (h *Hub) healthOf(id string) *modelHealth {
	mh := h.health[id]
	if mh == nil {
		mh = &modelHealth{}
		h.health[id] = mh
	}
	return mh
}

// quarantined reports whether a quarantine still applies to m: it has not
// expired and, for borrowed credentials, the provider settings have not been
// re-saved since.
func (h *Hub) quarantined(m ModelInstance, mh *modelHealth) bool {
	if mh == nil || !h.now().Before(mh.quarantine) {
		return false
	}
	return m.Credentials != CredentialsProvider || mh.quarantineGen == h.providerGeneration()
}

func (h *Hub) checkHealth(m ModelInstance) error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	mh := h.health[m.ID]
	if mh == nil {
		return nil
	}
	if h.now().Before(mh.openUntil) {
		return ErrBackoff
	}
	if h.quarantined(m, mh) {
		return fmt.Errorf("%w: %s", ErrBackoff, mh.quarantineWhy)
	}
	return nil
}

func (h *Hub) noteFailure(ctx context.Context, m ModelInstance, err error) {
	// A caller that gave up (turn cancelled) says nothing about the model.
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return
	}
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	mh := h.healthOf(m.ID)
	mh.lastProblem, mh.lastProblemAt = err.Error(), h.now()
	if IsAuthError(err) {
		mh.quarantine = h.now().Add(quarantineFor)
		mh.quarantineGen = h.providerGeneration()
		mh.quarantineWhy = err.Error()
		h.logger.Warn("decision model rejected the credentials; pausing it", "model", m.ID, "error", err, "for", quarantineFor)
		return
	}
	var he *HTTPError
	if errors.As(err, &he) && !retryableStatus(he.Status) {
		// A 400/404/413 is about this request (or the model id), not the
		// model's health: do not trip the circuit for everyone else.
		return
	}
	mh.failures++
	if mh.failures >= breakerThreshold {
		mh.openUntil = h.now().Add(breakerCooldown)
		mh.failures = 0
		h.logger.Warn("decision model failing repeatedly; callers fall back for a while", "model", m.ID, "error", err, "for", breakerCooldown)
	}
}

func (h *Hub) noteSuccess(id string) {
	h.healthMu.Lock()
	if mh := h.health[id]; mh != nil {
		mh.failures = 0
	}
	h.healthMu.Unlock()
}

func (h *Hub) noteProblem(id, msg string) {
	h.healthMu.Lock()
	mh := h.healthOf(id)
	mh.lastProblem, mh.lastProblemAt = msg, h.now()
	h.healthMu.Unlock()
}

func (h *Hub) forgetHealth(id string) {
	h.healthMu.Lock()
	delete(h.health, id)
	h.healthMu.Unlock()
}

func (h *Hub) resetAllHealth() {
	h.healthMu.Lock()
	h.health = map[string]*modelHealth{}
	h.healthMu.Unlock()
}

// ModelStatus is one decision model's health for the settings screen.
type ModelStatus struct {
	// Ready reports whether a call could be attempted right now.
	Ready bool `json:"ready"`
	// Problem explains why not, or describes the most recent failure.
	Problem      string `json:"problem,omitempty"`
	ProblemAt    int64  `json:"problemAt,omitempty"`
	BackoffUntil int64  `json:"backoffUntil,omitempty"`
	// Provider is the provider account a borrowing model would use now.
	Provider string `json:"provider,omitempty"`
	// Endpoint is the host requests go to (never a path or a key).
	Endpoint string `json:"endpoint,omitempty"`
}

// ModelStatus reports one decision model's health.
func (h *Hub) ModelStatus(id string) ModelStatus {
	m, ok := h.models.Get(id)
	if !ok {
		return ModelStatus{Problem: fmt.Sprintf("%v: %q", ErrNoModel, id)}
	}
	b, ok := Lookup(m.Backend)
	if !ok {
		return ModelStatus{Problem: fmt.Sprintf("unknown decision backend %q", m.Backend)}
	}
	manifest := b.Manifest()
	var st ModelStatus
	setup := ""
	if m.Credentials == CredentialsProvider {
		st.Provider = h.providerFor(m, b)
		if st.Provider == "" {
			setup = ErrNoEndpoint.Error()
		} else {
			st.Endpoint = hostOf(h.providerBaseURL(st.Provider))
		}
	} else {
		st.Endpoint = hostOf(firstNonEmpty(m.BaseURL, manifest.DefaultBaseURL))
		if manifest.KeyRequired && m.SecretsEnc[SecretAPIKey] == "" {
			setup = "no API key"
		}
	}
	if !m.Enabled && setup == "" {
		setup = ErrModelDisabled.Error()
	}

	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	mh := h.health[id]
	if mh != nil && mh.lastProblem != "" {
		st.Problem, st.ProblemAt = mh.lastProblem, mh.lastProblemAt.UnixMilli()
	}
	now := h.now()
	switch {
	case setup != "":
		st.Problem = setup
	case mh != nil && now.Before(mh.openUntil):
		st.BackoffUntil = mh.openUntil.UnixMilli()
	case h.quarantined(m, mh):
		st.BackoffUntil = mh.quarantine.UnixMilli()
		st.Problem = mh.quarantineWhy
	default:
		st.Ready = true
	}
	return st
}

// providerBaseURL is where a provider instance's requests go: its configured
// base URL, else its kind's default ("" = unknown).
func (h *Hub) providerBaseURL(id string) string {
	if h.opts.Source == nil {
		return ""
	}
	for _, inst := range h.opts.Source.Instances() {
		if inst.ID == id {
			base, _ := endpointBase(Endpoint{InstanceID: inst.ID, Kind: inst.Kind, BaseURL: inst.BaseURL})
			return base
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Status is the decider's overall health: the master switch and the state of
// the model that answers by default.
type Status struct {
	Enabled bool `json:"enabled"`
	// Model is the decision model answering by default ("" = none).
	Model        string `json:"model,omitempty"`
	Ready        bool   `json:"ready"`
	Problem      string `json:"problem,omitempty"`
	ProblemAt    int64  `json:"problemAt,omitempty"`
	BackoffUntil int64  `json:"backoffUntil,omitempty"`
}

// Status reports the decider's overall health.
func (h *Hub) Status() Status {
	cfg := h.Config()
	st := Status{Enabled: cfg.Enabled, Model: h.effectiveModel(cfg, "")}
	if st.Model == "" {
		st.Problem = ErrNoModel.Error()
		return st
	}
	ms := h.ModelStatus(st.Model)
	st.Ready, st.Problem, st.ProblemAt, st.BackoffUntil = ms.Ready, ms.Problem, ms.ProblemAt, ms.BackoffUntil
	return st
}
