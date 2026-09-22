package decider

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
)

// cachedClient is the backend client of one decision model, valid while key
// (the model's revision and, for borrowed credentials, the provider account and
// provider generation) is unchanged.
type cachedClient struct {
	key      string
	d        Decider
	manifest Manifest
}

func (h *Hub) modelExists(id string) bool {
	_, ok := h.models.Get(id)
	return ok
}

// Models lists the decision models.
func (h *Hub) Models() []ModelInstance {
	return h.models.List()
}

// Model returns one decision model.
func (h *Hub) Model(id string) (ModelInstance, bool) {
	return h.models.Get(id)
}

// UpsertModel creates or updates a decision model. Its cached client and
// failure state are dropped so the new settings apply to the next call.
func (h *Hub) UpsertModel(in ModelInput) (ModelInstance, error) {
	m, err := h.models.Upsert(in)
	if err != nil {
		return ModelInstance{}, err
	}
	h.dropClient(m.ID)
	h.forgetHealth(m.ID)
	return m, nil
}

// DeleteModel removes a decision model and every reference to it: the default
// model and each authority's model, fallback and challenger (an authority then
// uses the default model). It returns who relied on the model.
func (h *Hub) DeleteModel(id string) ([]string, error) {
	users := h.ModelUsers(id)
	if err := h.models.Delete(id); err != nil {
		return nil, err
	}
	h.dropClient(id)
	h.forgetHealth(id)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = h.cfg.withoutMissingModels(h.modelExists)
	if h.opts.DataDir != "" {
		if err := saveConfig(h.opts.DataDir, h.cfg); err != nil {
			return users, fmt.Errorf("save decider config: %w", err)
		}
	}
	return users, nil
}

// ModelUsers lists what relies on a decision model: "default" when it is the
// default model, and every authority naming it as model, fallback or challenger.
func (h *Hub) ModelUsers(id string) []string {
	cfg := h.Config()
	var authorities []string
	for aid, ac := range cfg.Authorities {
		if ac.Model == id || ac.Fallback == id || ac.Challenger == id {
			authorities = append(authorities, aid)
		}
	}
	sort.Strings(authorities)
	if cfg.DefaultModel == id {
		return append([]string{"default"}, authorities...)
	}
	return authorities
}

// ProviderCandidates lists the enabled provider instances a model of backend
// can borrow credentials from, in the order the automatic pick prefers them.
func (h *Hub) ProviderCandidates(backend string) []InstanceInfo {
	b, ok := Lookup(backend)
	if !ok || h.opts.Source == nil {
		return nil
	}
	return providerCandidates(b, h.opts.Source.Instances())
}

func providerCandidates(b Backend, all []InstanceInfo) []InstanceInfo {
	kinds := b.Manifest().ProviderKinds
	rank := func(kind string) int {
		if i := slices.Index(kinds, kind); i >= 0 {
			return i
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

// providerFor resolves the provider account a borrowing model uses: its named
// one, else the first candidate. "" when there is none.
func (h *Hub) providerFor(m ModelInstance, b Backend) string {
	if m.ProviderInstanceID != "" {
		return m.ProviderInstanceID
	}
	if h.opts.Source == nil {
		return ""
	}
	if c := providerCandidates(b, h.opts.Source.Instances()); len(c) > 0 {
		return c[0].ID
	}
	return ""
}

// providerGeneration is the provider configuration generation (0 without a source).
func (h *Hub) providerGeneration() uint64 {
	if h.opts.Source == nil {
		return 0
	}
	return h.opts.Source.Generation()
}

// clientFor returns the cached client of a decision model, building it when
// the model or the provider configuration it borrows from changed.
func (h *Hub) clientFor(m ModelInstance) (Decider, Manifest, error) {
	b, ok := Lookup(m.Backend)
	if !ok {
		return nil, Manifest{}, fmt.Errorf("unknown decision backend %q", m.Backend)
	}
	manifest := b.Manifest()
	provider := ""
	var gen uint64
	if m.Credentials == CredentialsProvider {
		if h.opts.Source == nil {
			return nil, manifest, ErrNoEndpoint
		}
		gen = h.providerGeneration()
		provider = h.providerFor(m, b)
		if provider == "" {
			return nil, manifest, ErrNoEndpoint
		}
	}
	key := fmt.Sprintf("%d|%s|%d", m.UpdatedAt.UnixNano(), provider, gen)
	h.clientMu.Lock()
	defer h.clientMu.Unlock()
	if c, ok := h.clients[m.ID]; ok && c.key == key {
		return c.d, c.manifest, nil
	}
	ep, err := h.endpointFor(m, b, provider)
	if err != nil {
		return nil, manifest, err
	}
	d, err := b.New(ep, ClientOptions{Model: m.Model, Timeout: m.Timeout(), Config: m.Config, HTTPClient: h.opts.HTTPClient})
	if err != nil {
		return nil, manifest, err
	}
	h.clients[m.ID] = cachedClient{key: key, d: d, manifest: manifest}
	return d, manifest, nil
}

// endpointFor resolves where a model's requests go and with which credential.
func (h *Hub) endpointFor(m ModelInstance, b Backend, provider string) (Endpoint, error) {
	if m.Credentials == CredentialsProvider {
		ep, err := h.opts.Source.Endpoint(provider)
		if err != nil {
			return Endpoint{}, fmt.Errorf("%w: %v", ErrNoEndpoint, err)
		}
		if !b.Accepts(ep.Kind, ep.BaseURL) {
			return Endpoint{}, fmt.Errorf("%w: provider %q (%s) cannot reach %s", ErrNoEndpoint, provider, ep.Kind, b.Manifest().Label)
		}
		return ep, nil
	}
	ep := Endpoint{BaseURL: m.BaseURL}
	if key := h.models.Secret(m.ID, SecretAPIKey); key != "" {
		ep.Authorize = func(hd http.Header) { hd.Set("Authorization", "Bearer "+key) }
	}
	return ep, nil
}

func (h *Hub) dropClient(id string) {
	h.clientMu.Lock()
	delete(h.clients, id)
	h.clientMu.Unlock()
}
