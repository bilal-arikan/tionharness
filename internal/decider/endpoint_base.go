package decider

import "strings"

// endpointBase resolves the base URL an endpoint points at: the model's own
// configured value, "" when the backend's default applies.
func endpointBase(ep Endpoint) string {
	return strings.TrimSpace(ep.BaseURL)
}
