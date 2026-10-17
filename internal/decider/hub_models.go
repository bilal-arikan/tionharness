package decider

import (
	"fmt"
	"net/http"
	"slices"
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
	slices.Sort(authorities)
	if cfg.DefaultModel == id {
		return append([]string{"default"}, authorities...)
	}
	return authorities
}

// clientFor returns the cached client of a decision model, building it when
// the model or the provider configuration it borrows from changed.
func (h *Hub) clientFor(m ModelInstance) (Decider, Manifest, error) {
	b, ok := Lookup(m.Backend)
	if !ok {
		return nil, Manifest{}, fmt.Errorf("unknown decision backend %q", m.Backend)
	}
	manifest := b.Manifest()
	// A hosted backend without a key cannot be reached: fail here with the
	// setup error instead of sending an unauthenticated request.
	if manifest.KeyRequired && h.models.Secret(m.ID, SecretAPIKey) == "" {
		return nil, manifest, ErrNoEndpoint
	}
	key := fmt.Sprintf("%d", m.UpdatedAt.UnixNano())
	h.clientMu.Lock()
	defer h.clientMu.Unlock()
	if c, ok := h.clients[m.ID]; ok && c.key == key {
		return c.d, c.manifest, nil
	}
	ep := h.endpointFor(m)
	d, err := b.New(ep, ClientOptions{Model: m.Model, Timeout: m.Timeout(), Config: m.Config, HTTPClient: h.opts.HTTPClient})
	if err != nil {
		return nil, manifest, err
	}
	h.clients[m.ID] = cachedClient{key: key, d: d, manifest: manifest}
	return d, manifest, nil
}

// endpointFor resolves where a model\'s requests go and with which credential.
func (h *Hub) endpointFor(m ModelInstance) Endpoint {
	ep := Endpoint{BaseURL: m.BaseURL}
	if key := h.models.Secret(m.ID, SecretAPIKey); key != "" {
		ep.Authorize = func(hd http.Header) { hd.Set("Authorization", "Bearer "+key) }
	}
	return ep
}

func (h *Hub) dropClient(id string) {
	h.clientMu.Lock()
	delete(h.clients, id)
	h.clientMu.Unlock()
}
