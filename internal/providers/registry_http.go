package providers

import (
	"fmt"
	"net/http"
)

// HTTPAccess is what a non-chat HTTP client needs to call the service behind a
// provider instance with that instance's account: its kind, its configured base
// URL and a function that stamps the credential onto a request. The key never
// leaves this package as a string — Authorize closes over it — so a caller can
// neither log nor persist it by accident.
//
// The decider layer (internal/decider) is the consumer: a decision model shares
// an account and a bill with a chat instance (an OpenRouter key) but is served
// on a different endpoint and must never be built as a chat Provider.
type HTTPAccess struct {
	InstanceID string
	Kind       string
	BaseURL    string
	Authorize  func(http.Header)
}

// HTTPAccess resolves an enabled, configured API-transport instance. A CLI
// instance has no HTTP credential to lend and is refused, as is a disabled one.
func (r *Registry) HTTPAccess(id string) (HTTPAccess, error) {
	inst, k, err := r.resolveInstance(id)
	if err != nil {
		return HTTPAccess{}, err
	}
	if !inst.Enabled {
		return HTTPAccess{}, fmt.Errorf("provider instance %q is disabled", inst.ID)
	}
	if TransportOf(inst.KindID) != TransportAPI {
		return HTTPAccess{}, fmt.Errorf("provider instance %q (%s) is not an API instance", inst.ID, inst.KindID)
	}
	cfg := r.resolve(inst)
	if !k.Available(cfg) {
		return HTTPAccess{}, fmt.Errorf("provider instance %q is not configured (API key missing?)", inst.ID)
	}
	key := cfg.Key
	return HTTPAccess{
		InstanceID: inst.ID,
		Kind:       inst.KindID,
		BaseURL:    cfg.BaseURL,
		Authorize: func(h http.Header) {
			// A local endpoint may legitimately run without a key; send no
			// Authorization header rather than a malformed empty one.
			if key != "" {
				h.Set("Authorization", "Bearer "+key)
			}
		},
	}, nil
}

// InstanceBaseURL returns an instance's configured base URL ("" when unset or
// unknown). A base URL is configuration, not a credential, so it is safe to
// list; the decider settings screen uses it to tell which openai-compat
// instances point at a service a decision backend can use.
func (r *Registry) InstanceBaseURL(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.instances[id].Values[FieldKeyBaseURL]
}
