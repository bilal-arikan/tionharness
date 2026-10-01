package decider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
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

// EndpointSource resolves provider instances into endpoints, for decision
// models that borrow provider credentials. The provider registry backs it in
// production (adapted in the API layer, since this package imports nothing
// internal); tests pass a fake.
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
	// DataDir holds decider.json and decider/{models.json,ledger.jsonl}.
	// "" = in memory only.
	DataDir string
	Source  EndpointSource
	// Secrets seals the API keys decision models carry themselves.
	Secrets SecretBox
	Logger  *slog.Logger
	// HTTPClient overrides the shared HTTP client (tests).
	HTTPClient *http.Client
	// Now overrides the clock (tests).
	Now func() time.Time
}

// Hub is the app-wide decision service. It owns the configuration and the
// decision models, answers on behalf of authorities (primary model, fallback,
// challenger), caches one client per model, redacts and bounds every request,
// rests a failing model and keeps the decision ledger. Safe for concurrent use.
type Hub struct {
	opts   HubOptions
	logger *slog.Logger
	ledger *Ledger
	debug  *DebugJournal
	models *ModelStore

	mu  sync.RWMutex
	cfg Config

	clientMu sync.Mutex
	clients  map[string]cachedClient

	healthMu sync.Mutex
	health   map[string]*modelHealth

	// challengeSlots bounds concurrent challenger calls; a challenge that finds
	// no free slot is skipped rather than queued.
	challengeSlots chan struct{}
}

// NewHub builds the hub: it loads the config and the decision models (a
// corrupt file falls back to defaults with a warning rather than failing boot)
// and, on the first run of the model registry, turns the old single connection
// — or the out-of-the-box Jev-through-OpenRouter default — into the first
// decision model.
func NewHub(opts HubOptions) *Hub {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{
		opts:           opts,
		logger:         logger,
		clients:        map[string]cachedClient{},
		health:         map[string]*modelHealth{},
		challengeSlots: make(chan struct{}, maxConcurrentChallenges),
	}
	cfg := DefaultConfig()
	var legacy *legacyConnection
	dir := ""
	if opts.DataDir != "" {
		loaded, leg, err := loadConfig(opts.DataDir)
		if err != nil {
			logger.Warn("decider config unreadable; using defaults", "path", ConfigPath(opts.DataDir), "error", err)
		}
		cfg, legacy = loaded, leg
		dir = filepath.Join(opts.DataDir, "decider")
	}
	h.ledger = OpenLedger(dir, logger)
	h.debug = openDebugJournal(dir, logger)
	h.models = OpenModelStore(dir, opts.Secrets, logger)
	if opts.Now != nil {
		h.models.now = opts.Now
	}
	if !h.models.Existed() && len(h.models.List()) == 0 {
		if m, err := h.models.Upsert(seedModel(legacy)); err != nil {
			logger.Warn("could not create the first decision model", "error", err)
		} else if legacy != nil {
			logger.Info("decider settings moved to decision models", "model", m.ID, "backend", m.Backend)
		}
	}
	h.cfg = cfg.withoutMissingModels(h.modelExists)
	if legacy != nil && opts.DataDir != "" {
		// Persist the converted config so the conversion runs once.
		if err := saveConfig(opts.DataDir, h.cfg); err != nil {
			logger.Warn("could not save the converted decider config", "error", err)
		}
	}
	return h
}

func (h *Hub) now() time.Time {
	if h.opts.Now != nil {
		return h.opts.Now()
	}
	return time.Now()
}

// Config returns a copy of the current configuration.
func (h *Hub) Config() Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c := h.cfg
	c.Authorities = maps.Clone(h.cfg.Authorities)
	return c
}

// Update validates, persists and applies a new configuration. Every model's
// failure state is dropped, so saving the settings also means "try again".
func (h *Hub) Update(c Config) (Config, error) {
	if err := c.Validate(); err != nil {
		return h.Config(), err
	}
	if err := h.checkModelRefs(c); err != nil {
		return h.Config(), err
	}
	c = c.Normalized()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.opts.DataDir != "" {
		if err := saveConfig(h.opts.DataDir, c); err != nil {
			return h.cfg, fmt.Errorf("save decider config: %w", err)
		}
	}
	h.cfg = c
	h.resetAllHealth()
	return c, nil
}

// checkModelRefs refuses a config that names a decision model that does not exist.
func (h *Hub) checkModelRefs(c Config) error {
	if c.DefaultModel != "" && !h.modelExists(c.DefaultModel) {
		return fmt.Errorf("unknown decision model %q", c.DefaultModel)
	}
	for id, ac := range c.Authorities {
		for _, ref := range []string{ac.Model, ac.Fallback, ac.Challenger} {
			if ref != "" && !h.modelExists(ref) {
				return fmt.Errorf("authority %q: unknown decision model %q", id, ref)
			}
		}
	}
	return nil
}

// Mode is the effective mode of an authority.
func (h *Hub) Mode(authority string) Mode {
	if h == nil {
		return ModeOff
	}
	return h.Config().Mode(authority)
}

// Threshold is an authority's configured threshold.
func (h *Hub) Threshold(authority string) float64 {
	if h == nil {
		return 0.8
	}
	return h.Config().Threshold(authority)
}

// CallOption tunes one Decide call.
type CallOption func(*callOptions)

type callOptions struct {
	bill        func(context.Context, *Response)
	background  func(func())
	ref         string
	outcome     func(*Response) (string, float64)
	sessionID   string
	workspaceID string
	turnID      string
	trace       *debugCall
	role        string
}

// WithBilling records every model call a decision makes — the primary, a
// fallback and a background challenger — against the caller. This package
// keeps no usage books of its own.
func WithBilling(fn func(ctx context.Context, resp *Response)) CallOption {
	return func(o *callOptions) { o.bill = fn }
}

// WithBackground runs background work (the challenger) under the caller's
// lifecycle, e.g. a runtime's shutdown barrier. Default: a plain goroutine.
func WithBackground(run func(func())) CallOption {
	return func(o *callOptions) { o.background = run }
}

// WithRef tags ledger records with a session, run or trajectory id.
func WithRef(ref string) CallOption {
	return func(o *callOptions) { o.ref = ref }
}

// WithOutcome renders a response in the authority's own vocabulary for the
// challenger comparison ("stalled"/"ok" rather than "yes"/"no"). Default:
// Verdict at the authority's threshold.
func WithOutcome(fn func(*Response) (string, float64)) CallOption {
	return func(o *callOptions) { o.outcome = fn }
}

func newCallOptions(opts []CallOption) callOptions {
	var o callOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Decide asks on behalf of an authority: its model (or the default model),
// then its fallback when that model cannot answer, and — after an answer — its
// challenger in the background. It fails fast with ErrDisabled / ErrSiteOff
// when the authority is off; callers fall back to their own logic on any error.
func (h *Hub) Decide(ctx context.Context, authority string, req Request, opts ...CallOption) (resp *Response, err error) {
	if h == nil {
		return nil, ErrDisabled
	}
	cfg := h.Config()
	o := newCallOptions(opts)
	o.trace, o.role = h.startDebug(authority, cfg, o, "primary"), "primary"
	start := time.Now()
	defer func() {
		o.trace.finish(resp, err, start)
		if err != nil {
			err = &debugError{id: o.trace.base.TraceID, err: err}
		}
	}()
	if !cfg.Enabled {
		o.trace.emit(DebugEvent{Stage: "skipped", Error: "disabled"})
		return nil, ErrDisabled
	}
	mode := cfg.Mode(authority)
	if mode == ModeOff {
		o.trace.emit(DebugEvent{Stage: "skipped", Error: "authority_off"})
		return nil, ErrSiteOff
	}
	o.trace.emit(DebugEvent{Stage: "started"})
	ac := cfg.Authority(authority)
	primary := h.effectiveModel(cfg, ac.Model)
	resp, err = h.ask(ctx, primary, req, o, false)
	if err != nil && ac.Fallback != "" && ac.Fallback != primary && fallbackWorthy(ctx, err) {
		fallbackOptions := o
		fallbackOptions.role = "fallback"
		fb, ferr := h.ask(ctx, ac.Fallback, req, fallbackOptions, false)
		if ferr != nil {
			return nil, errors.Join(err, ferr)
		}
		h.logger.Debug("decision answered by the fallback model", "authority", authority, "primary", primary, "fallback", ac.Fallback, "error", err)
		fb.Fallback = true
		resp, err = fb, nil
	}
	if err != nil {
		return nil, err
	}
	if ac.Challenger != "" && ac.Challenger != resp.Instance {
		h.challenge(ctx, authority, mode, ac.Challenger, req, resp, cfg.Threshold(authority), o)
	}
	return resp, nil
}

// Test runs a small fixed request through one decision model (the default
// model when id is ""), regardless of the master switch, the authorities and
// the model's own enabled flag, so the settings screen can check a model
// before relying on it. Nothing is billed; the model's health is updated.
func (h *Hub) Test(ctx context.Context, id string) (resp *Response, err error) {
	if id == "" {
		id = h.effectiveModel(h.Config(), "")
	}
	o := callOptions{role: "test"}
	o.trace = h.startDebug("model-test", h.Config(), o, "test")
	start := time.Now()
	o.trace.emit(DebugEvent{Stage: "started"})
	defer func() { o.trace.finish(resp, err, start) }()
	return h.ask(ctx, id, testRequest(), o, true)
}

func testRequest() Request {
	return Request{
		State: "Tool call about to run: `git push --force origin main` in a repository shared with the team.",
		Questions: map[string]Question{
			"needs_approval": Noul("Should a human approve this tool call before it runs?",
				"The call is destructive, irreversible or affects shared/remote state.",
				"The call only reads data or makes a local, easily reversible change."),
			"risk": Score("How risky is the tool call?", "Read-only", "Local reversible change", "Destructive or outward-facing"),
		},
	}
}

// ModelError carries the decision model a failure came from, so the ledger can
// attribute it. Its text is the underlying error's.
type ModelError struct {
	Instance string
	Err      error
}

func (e *ModelError) Error() string { return e.Err.Error() }
func (e *ModelError) Unwrap() error { return e.Err }

// ask puts req to one decision model.
func (h *Hub) ask(ctx context.Context, id string, req Request, o callOptions, allowDisabled bool) (resp *Response, err error) {
	trace := o.trace.forModel(id, o.role)
	start := time.Now()
	event := DebugEvent{Stage: "attempt"}
	defer func() {
		event.Error, event.LatencyMs = errorClass(err), time.Since(start).Milliseconds()
		debugResponse(resp, &event)
		trace.emit(event)
	}()
	if id == "" {
		return nil, ErrNoModel
	}
	m, ok := h.models.Get(id)
	if !ok {
		return nil, &ModelError{Instance: id, Err: fmt.Errorf("%w: %q", ErrNoModel, id)}
	}
	event.Model, event.Backend, event.ModelHash = debugToken(m.Model), debugToken(m.Backend), debugHash(m)
	event.TimeoutMs = int(m.Timeout().Milliseconds())
	if !m.Enabled && !allowDisabled {
		return nil, &ModelError{Instance: id, Err: ErrModelDisabled}
	}
	if err := req.Normalized().Validate(); err != nil {
		return nil, err
	}
	if err := h.checkHealth(m); err != nil {
		return nil, &ModelError{Instance: id, Err: err}
	}
	client, manifest, err := h.clientFor(m)
	if err != nil {
		h.noteProblem(id, err.Error())
		return nil, &ModelError{Instance: id, Err: err}
	}
	if err := req.checkLimits(manifest.Limits); err != nil {
		return nil, &ModelError{Instance: id, Err: err}
	}
	contextTokens := m.ContextTokens
	if contextTokens <= 0 {
		contextTokens = manifest.ContextTokens
	}
	event.ContextTokens, event.StateBytes = contextTokens, debugStateBytes(req.State)
	state, err := prepareState(req.State, req.Questions, contextTokens)
	if err != nil {
		return nil, &ModelError{Instance: id, Err: err}
	}
	req.State = state
	req = copyQuestionMetadata(req)
	metadata := debugRequest(req)
	event.RequestHash, event.QuestionTypes = metadata.RequestHash, metadata.QuestionTypes
	event.PreparedBytes = metadata.StateBytes
	event.StateTrimmed = event.PreparedBytes < event.StateBytes
	if trace != nil {
		ctx = context.WithValue(ctx, debugCallKey{}, trace)
	}
	resp, err = client.Decide(ctx, req)
	if err != nil {
		h.noteFailure(ctx, m, err)
		return nil, &ModelError{Instance: id, Err: err}
	}
	h.noteSuccess(id)
	resp.Instance = id
	fillCost(manifest, resp)
	if o.bill != nil {
		o.bill(ctx, resp)
	}
	return resp, nil
}

// fillCost prices a call the service did not price (TypeSafe's own API reports
// tokens only) from the backend's model list. A local server stays free.
func fillCost(m Manifest, resp *Response) {
	if resp.Usage.CostUSD > 0 || resp.BillingProvider == billingLocal {
		return
	}
	for _, mod := range m.Models {
		if mod.ID == resp.Model {
			resp.Usage.CostUSD = (float64(resp.Usage.InputTokens)*mod.InputPerMTok + float64(resp.Usage.OutputTokens)*mod.OutputPerMTok) / 1e6
			return
		}
	}
}

// effectiveModel resolves an authority's model: its own, else the default
// model, else the first enabled model.
func (h *Hub) effectiveModel(cfg Config, id string) string {
	if id != "" {
		return id
	}
	if cfg.DefaultModel != "" {
		return cfg.DefaultModel
	}
	for _, m := range h.models.List() {
		if m.Enabled {
			return m.ID
		}
	}
	return ""
}

// EffectiveModel is the decision model that answers for authority right now.
func (h *Hub) EffectiveModel(authority string) string {
	cfg := h.Config()
	return h.effectiveModel(cfg, cfg.Authority(authority).Model)
}

// Log appends a record to the ledger.
func (h *Hub) Log(rec Record) {
	if h == nil {
		return
	}
	h.ledger.Append(rec)
	h.debugOutcome(rec)
}

// NewRecord starts a ledger record for one call: authority, mode, model,
// latency, cost and error class filled from its result. Callers add the verdicts.
func NewRecord(authority string, mode Mode, resp *Response, err error) Record {
	rec := Record{Authority: authority, Mode: mode, Error: errorClass(err), DebugID: debugID(resp, err)}
	if resp != nil {
		rec.Instance = resp.Instance
		rec.Model = resp.Model
		rec.Fallback = resp.Fallback
		rec.LatencyMs = resp.LatencyMs
		rec.InputTokens = resp.Usage.InputTokens
		rec.CostUSD = resp.Usage.CostUSD
	}
	var me *ModelError
	if rec.Instance == "" && errors.As(err, &me) {
		rec.Instance = me.Instance
	}
	return rec
}

// Stats aggregates the ledger over the last window, per authority.
func (h *Hub) Stats(window time.Duration) []AuthorityStats {
	return h.ledger.Stats(h.now().Add(-window))
}

// ModelStats aggregates the ledger over the last window, per decision model.
func (h *Hub) ModelStats(window time.Duration) []ModelStats {
	return h.ledger.ModelStats(h.now().Add(-window))
}

// Recent returns the newest ledger records.
func (h *Hub) Recent(n int) []Record {
	return h.ledger.Recent(n)
}
