package decider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// InstanceInfo is a credential-free view of one provider instance.
type InstanceInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	BaseURL   string `json:"baseUrl,omitempty"`
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
}

// EndpointSource resolves provider instances into decision endpoints. The
// provider registry backs it in production (adapted in the API layer, since this
// package imports nothing internal); tests pass a fake.
type EndpointSource interface {
	// Endpoint returns the endpoint of one enabled, usable instance.
	Endpoint(instanceID string) (Endpoint, error)
	// Instances lists the configured provider instances.
	Instances() []InstanceInfo
	// Generation changes whenever the provider configuration is re-applied, so
	// a cached client or quarantine never outlives the settings it was built on.
	Generation() uint64
}

// HubOptions configure a Hub.
type HubOptions struct {
	// DataDir holds decider.json and decider/ledger.jsonl. "" = in memory only.
	DataDir string
	Source  EndpointSource
	Logger  *slog.Logger
	// HTTPClient overrides the shared HTTP client (tests).
	HTTPClient *http.Client
	// Now overrides the clock (tests).
	Now func() time.Time
}

// Failure handling. A decision is an optimisation on top of logic that already
// works, so a failing endpoint must cost the caller as little as possible:
// consecutive transient failures open a short circuit (callers fall back at
// once instead of waiting out another timeout), and a credential or billing
// rejection quarantines the instance until providers are re-saved or the
// quarantine expires (a 402 clears once the account is topped up).
const (
	breakerThreshold = 3
	breakerCooldown  = time.Minute
	quarantineFor    = 10 * time.Minute
)

// Hub is the app-wide decision service: it owns the configuration, resolves the
// backend client from a provider instance, redacts and bounds every request,
// backs off from a failing endpoint and keeps the decision ledger. Safe for
// concurrent use.
type Hub struct {
	opts   HubOptions
	logger *slog.Logger
	ledger *Ledger

	mu  sync.RWMutex
	cfg Config

	clientMu       sync.Mutex
	client         Decider
	clientKey      string
	clientInstance string
	clientManifest Manifest

	healthMu      sync.Mutex
	failures      int
	openUntil     time.Time
	quarantine    quarantine
	lastProblem   string
	lastProblemAt time.Time
}

type quarantine struct {
	generation uint64
	instance   string
	until      time.Time
	reason     string
}

// NewHub builds the hub, loading the persisted config (a corrupt file falls back
// to the defaults with a warning rather than failing boot).
func NewHub(opts HubOptions) *Hub {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{opts: opts, logger: logger}
	cfg := DefaultConfig()
	ledgerDir := ""
	if opts.DataDir != "" {
		loaded, err := LoadConfig(opts.DataDir)
		if err != nil {
			logger.Warn("decider config unreadable; using defaults", "path", ConfigPath(opts.DataDir), "error", err)
		}
		cfg = loaded
		ledgerDir = filepath.Join(opts.DataDir, "decider")
	}
	h.cfg = cfg
	h.ledger = OpenLedger(ledgerDir, logger)
	return h
}

func (h *Hub) now() time.Time {
	if h.opts.Now != nil {
		return h.opts.Now()
	}
	return time.Now()
}

// Config returns the current configuration.
func (h *Hub) Config() Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Update validates, persists and applies a new configuration. The cached client
// and the failure state are dropped so the new settings take effect at once.
func (h *Hub) Update(c Config) (Config, error) {
	if err := c.Validate(); err != nil {
		return h.Config(), err
	}
	c = c.Normalized()
	if h.opts.DataDir != "" {
		if err := SaveConfig(h.opts.DataDir, c); err != nil {
			return h.Config(), fmt.Errorf("save decider config: %w", err)
		}
	}
	h.mu.Lock()
	h.cfg = c
	h.mu.Unlock()
	h.resetClient()
	h.healthMu.Lock()
	h.failures, h.openUntil, h.quarantine = 0, time.Time{}, quarantine{}
	h.healthMu.Unlock()
	return c, nil
}

// Mode is the effective mode of a site.
func (h *Hub) Mode(site string) Mode {
	if h == nil {
		return ModeOff
	}
	return h.Config().SiteMode(site)
}

// Threshold is a site's configured threshold.
func (h *Hub) Threshold(site string) float64 {
	if h == nil {
		return 0.8
	}
	return h.Config().SiteThreshold(site)
}

// Decide asks the configured decision model on behalf of site. It fails fast
// with ErrDisabled / ErrSiteOff when the site is off, and with ErrBackoff while
// the endpoint is resting after failures; callers fall back to their own logic
// on any error.
func (h *Hub) Decide(ctx context.Context, site string, req Request) (*Response, error) {
	if h == nil {
		return nil, ErrDisabled
	}
	cfg := h.Config()
	if !cfg.Enabled {
		return nil, ErrDisabled
	}
	if cfg.SiteMode(site) == ModeOff {
		return nil, ErrSiteOff
	}
	return h.decide(ctx, cfg, req)
}

// Test runs a small fixed request through the configured backend, regardless of
// the master switch and site modes, so the settings screen can check a
// configuration before switching it on.
func (h *Hub) Test(ctx context.Context) (*Response, error) {
	cfg := h.Config()
	return h.decide(ctx, cfg, Request{
		State: "Tool call about to run: `git push --force origin main` in a repository shared with the team.",
		Questions: map[string]Question{
			"needs_approval": Noul("Should a human approve this tool call before it runs?",
				"The call is destructive, irreversible or affects shared/remote state.",
				"The call only reads data or makes a local, easily reversible change."),
			"risk": Score("How risky is the tool call?", "Read-only", "Local reversible change", "Destructive or outward-facing"),
		},
	})
}

func (h *Hub) decide(ctx context.Context, cfg Config, req Request) (*Response, error) {
	if err := h.checkCircuit(); err != nil {
		return nil, err
	}
	client, instance, manifest, err := h.clientFor(cfg)
	if err != nil {
		h.noteProblem(err.Error())
		return nil, err
	}
	if req.Model == "" {
		req.Model = cfg.Model
	}
	state, err := prepareState(req.State, req.Questions, manifest.ContextTokens)
	if err != nil {
		return nil, err
	}
	req.State = state
	resp, err := client.Decide(ctx, req)
	if err != nil {
		h.noteFailure(ctx, instance, err)
		return nil, err
	}
	h.noteSuccess()
	return resp, nil
}

// clientFor returns the cached client for cfg, rebuilding it when the config or
// the provider generation changed.
func (h *Hub) clientFor(cfg Config) (Decider, string, Manifest, error) {
	b, ok := Lookup(cfg.Backend)
	if !ok {
		return nil, "", Manifest{}, fmt.Errorf("unknown decision backend %q", cfg.Backend)
	}
	manifest := b.Manifest()
	if h.opts.Source == nil {
		return nil, "", manifest, ErrNoEndpoint
	}
	gen := h.opts.Source.Generation()
	instance := cfg.ProviderInstanceID
	if instance == "" {
		instance = h.pickInstance(b)
		if instance == "" {
			return nil, "", manifest, ErrNoEndpoint
		}
	}
	if err := h.checkQuarantine(gen, instance); err != nil {
		return nil, instance, manifest, err
	}
	key := fmt.Sprintf("%s|%s|%s|%d|%d", cfg.Backend, instance, cfg.Model, cfg.TimeoutMs, gen)
	h.clientMu.Lock()
	defer h.clientMu.Unlock()
	if h.client != nil && h.clientKey == key {
		return h.client, h.clientInstance, h.clientManifest, nil
	}
	ep, err := h.opts.Source.Endpoint(instance)
	if err != nil {
		return nil, instance, manifest, fmt.Errorf("%w: %v", ErrNoEndpoint, err)
	}
	if !b.Accepts(ep.Kind, ep.BaseURL) {
		return nil, instance, manifest, fmt.Errorf("%w: instance %q (%s) cannot reach %s", ErrNoEndpoint, instance, ep.Kind, manifest.Label)
	}
	d, err := b.New(ep, ClientOptions{Model: cfg.Model, Timeout: cfg.Timeout(), HTTPClient: h.opts.HTTPClient})
	if err != nil {
		return nil, instance, manifest, err
	}
	h.client, h.clientKey, h.clientInstance, h.clientManifest = d, key, instance, manifest
	return d, instance, manifest, nil
}

// pickInstance chooses the instance used when the config names none: the first
// enabled, available instance the backend accepts, preferring the backend's
// first-listed provider kind, then by id for a deterministic pick.
func (h *Hub) pickInstance(b Backend) string {
	cands := candidates(b, h.opts.Source.Instances())
	if len(cands) == 0 {
		return ""
	}
	return cands[0].ID
}

// Candidates lists the enabled instances the configured backend can use, in the
// order the automatic pick prefers them.
func (h *Hub) Candidates() []InstanceInfo {
	b, ok := Lookup(h.Config().Backend)
	if !ok || h.opts.Source == nil {
		return nil
	}
	return candidates(b, h.opts.Source.Instances())
}

func candidates(b Backend, all []InstanceInfo) []InstanceInfo {
	kinds := b.Manifest().ProviderKinds
	rank := func(kind string) int {
		for i, k := range kinds {
			if k == kind {
				return i
			}
		}
		return len(kinds)
	}
	var out []InstanceInfo
	for _, inst := range all {
		if inst.Enabled && inst.Available && b.Accepts(inst.Kind, inst.BaseURL) {
			out = append(out, inst)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i].Kind), rank(out[j].Kind); ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (h *Hub) resetClient() {
	h.clientMu.Lock()
	h.client, h.clientKey, h.clientInstance = nil, "", ""
	h.clientMu.Unlock()
}

func (h *Hub) checkCircuit() error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	if h.now().Before(h.openUntil) {
		return ErrBackoff
	}
	return nil
}

func (h *Hub) checkQuarantine(gen uint64, instance string) error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	q := h.quarantine
	if q.instance == instance && q.generation == gen && h.now().Before(q.until) {
		return fmt.Errorf("%w: %s", ErrBackoff, q.reason)
	}
	return nil
}

func (h *Hub) noteFailure(ctx context.Context, instance string, err error) {
	// A caller that gave up (turn cancelled) says nothing about the endpoint.
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return
	}
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	h.lastProblem, h.lastProblemAt = err.Error(), h.now()
	if IsAuthError(err) {
		var gen uint64
		if h.opts.Source != nil {
			gen = h.opts.Source.Generation()
		}
		h.quarantine = quarantine{generation: gen, instance: instance, until: h.now().Add(quarantineFor), reason: err.Error()}
		h.logger.Warn("decision endpoint rejected the credentials; pausing it", "instance", instance, "error", err, "for", quarantineFor)
		return
	}
	var he *HTTPError
	if errors.As(err, &he) && !retryableStatus(he.Status) {
		// A 400/404/413 is about this request (or the model id), not the
		// endpoint's health: do not trip the circuit for everyone else.
		return
	}
	h.failures++
	if h.failures >= breakerThreshold {
		h.openUntil = h.now().Add(breakerCooldown)
		h.failures = 0
		h.logger.Warn("decision endpoint failing repeatedly; callers fall back for a while", "error", err, "for", breakerCooldown)
	}
}

func (h *Hub) noteSuccess() {
	h.healthMu.Lock()
	h.failures = 0
	h.healthMu.Unlock()
}

func (h *Hub) noteProblem(msg string) {
	h.healthMu.Lock()
	h.lastProblem, h.lastProblemAt = msg, h.now()
	h.healthMu.Unlock()
}

// Status is the hub's health for the settings screen.
type Status struct {
	Enabled bool   `json:"enabled"`
	Backend string `json:"backend"`
	Model   string `json:"model"`
	// Instance is the provider instance the next call would use ("" = none).
	Instance string `json:"instance,omitempty"`
	// Ready reports whether a call could be attempted right now.
	Ready bool `json:"ready"`
	// Problem explains why not, or describes the most recent failure.
	Problem      string `json:"problem,omitempty"`
	ProblemAt    int64  `json:"problemAt,omitempty"`
	BackoffUntil int64  `json:"backoffUntil,omitempty"`
}

// Status reports the hub's current health.
func (h *Hub) Status() Status {
	cfg := h.Config()
	st := Status{Enabled: cfg.Enabled, Backend: cfg.Backend, Model: cfg.Model}
	b, ok := Lookup(cfg.Backend)
	if !ok {
		st.Problem = fmt.Sprintf("unknown decision backend %q", cfg.Backend)
		return st
	}
	st.Instance = cfg.ProviderInstanceID
	if st.Instance == "" && h.opts.Source != nil {
		st.Instance = h.pickInstance(b)
	}
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	if h.lastProblem != "" {
		st.Problem, st.ProblemAt = h.lastProblem, h.lastProblemAt.UnixMilli()
	}
	now := h.now()
	switch {
	case st.Instance == "":
		st.Problem = ErrNoEndpoint.Error()
	case now.Before(h.openUntil):
		st.BackoffUntil = h.openUntil.UnixMilli()
	case h.quarantine.instance == st.Instance && now.Before(h.quarantine.until) &&
		(h.opts.Source == nil || h.quarantine.generation == h.opts.Source.Generation()):
		st.BackoffUntil = h.quarantine.until.UnixMilli()
		st.Problem = h.quarantine.reason
	default:
		st.Ready = true
	}
	return st
}

// Log appends a record to the ledger.
func (h *Hub) Log(rec Record) {
	if h == nil {
		return
	}
	h.ledger.Append(rec)
}

// NewRecord starts a ledger record for one call: site, mode, model, latency,
// cost and error class filled from its result. Callers add the verdicts.
func NewRecord(site string, mode Mode, resp *Response, err error) Record {
	rec := Record{Site: site, Mode: mode, Error: errorClass(err)}
	if resp != nil {
		rec.Model = resp.Model
		rec.LatencyMs = resp.LatencyMs
		rec.InputTokens = resp.Usage.InputTokens
		rec.CostUSD = resp.Usage.CostUSD
	}
	return rec
}

// Stats aggregates the ledger over the last window.
func (h *Hub) Stats(window time.Duration) []SiteStats {
	return h.ledger.Stats(h.now().Add(-window))
}

// Recent returns the newest ledger records.
func (h *Hub) Recent(n int) []Record {
	return h.ledger.Recent(n)
}

// prepareState masks secrets in the state and keeps the request inside the
// backend's context window. A string state is trimmed from the middle; a
// structured state that does not fit is refused (the caller must shrink it,
// since cutting JSON arbitrarily would change its meaning).
func prepareState(state any, questions map[string]Question, contextTokens int) (any, error) {
	budget := stateBudgetBytes(questions, contextTokens)
	if s, ok := state.(string); ok {
		return TrimMiddle(Redact(s), budget), nil
	}
	// Round-trip through JSON so struct fields are redacted like map entries.
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode decision state: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("decode decision state: %w", err)
	}
	generic = redactValue(generic)
	if out, _ := json.Marshal(generic); len(out) > budget {
		return nil, fmt.Errorf("decision state is %d bytes, over the %d-byte budget", len(out), budget)
	}
	return generic, nil
}

// stateBudgetBytes estimates how many bytes of state fit beside the questions.
// 2.5 bytes per token is conservative for mixed Turkish/English text and JSON.
func stateBudgetBytes(questions map[string]Question, contextTokens int) int {
	if contextTokens <= 0 {
		contextTokens = 32000
	}
	qBytes := 0
	if raw, err := json.Marshal(questions); err == nil {
		qBytes = len(raw)
	}
	budget := contextTokens*5/2 - qBytes - 2048
	return max(budget, 4096)
}
