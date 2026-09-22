package decider

import (
	"fmt"
	"strings"
)

// kindDefaultBase is the base URL a provider kind uses when its instance leaves
// the field empty. The provider kinds apply the same defaults when they build
// their chat clients (internal/providers kind_*.go); the instance itself stores
// the empty value, so the decision layer has to know them too.
var kindDefaultBase = map[string]string{
	"openrouter": openRouterDefaultBase,
	"lmstudio":   lmStudioDefaultBase,
}

// endpointBase resolves the base URL an endpoint points at. A model's own
// endpoint may leave it empty (the backend's default applies). A borrowed
// provider endpoint uses the instance's base URL or its kind's default, and
// never the backend's: an OpenRouter key must not end up at TypeSafe's (or
// any other) default host just because the provider form left the field empty.
func endpointBase(ep Endpoint) (string, error) {
	base := strings.TrimSpace(ep.BaseURL)
	if ep.Kind == "" || base != "" {
		return base, nil
	}
	if def, ok := kindDefaultBase[ep.Kind]; ok {
		return def, nil
	}
	return "", fmt.Errorf("provider instance %q (%s) has no base URL to reach", ep.InstanceID, ep.Kind)
}
