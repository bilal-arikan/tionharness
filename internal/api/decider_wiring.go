package api

import (
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// registryEndpoints adapts the provider registry to decider.EndpointSource. The
// decider package imports nothing internal, so the bridge lives here, next to
// the other wiring of process-wide services.
type registryEndpoints struct {
	reg *providers.Registry
}

func (r registryEndpoints) Endpoint(id string) (decider.Endpoint, error) {
	acc, err := r.reg.HTTPAccess(id)
	if err != nil {
		return decider.Endpoint{}, err
	}
	return decider.Endpoint{InstanceID: acc.InstanceID, Kind: acc.Kind, BaseURL: acc.BaseURL, Authorize: acc.Authorize}, nil
}

func (r registryEndpoints) Instances() []decider.InstanceInfo {
	list := r.reg.ListInstances()
	out := make([]decider.InstanceInfo, 0, len(list))
	for _, inst := range list {
		out = append(out, decider.InstanceInfo{
			ID:        inst.ID,
			Kind:      inst.KindID,
			Label:     inst.Label,
			BaseURL:   r.reg.InstanceBaseURL(inst.ID),
			Enabled:   inst.Enabled,
			Available: inst.Available,
		})
	}
	return out
}

func (r registryEndpoints) Generation() uint64 {
	return r.reg.Generation()
}

// initDecider builds the process-wide decision-model hub over the provider
// registry and hands it to the shared tunables, where every workspace runtime
// finds it. Its settings live in <dataDir>/decider.json, apart from settings.json.
func (s *Server) initDecider() {
	hub := decider.NewHub(decider.HubOptions{
		DataDir: s.dataDir,
		Source:  registryEndpoints{reg: s.providers},
		Logger:  s.logger.With("subsystem", "decider"),
	})
	s.tun.SetDecider(hub)
}
