package api

import "github.com/bilal-arikan/tionharness/internal/events"

// A shared edit invalidates rosters in every open window, independent of the
// currently selected workspace. This is a quiet control event, not a toast.
func (s *Server) publishAgentCatalogChanged() {
	if s.bus != nil && s.workspaces != nil && s.workspaces.AgentCatalog() != nil {
		s.bus.Publish(events.Event{Type: "agent-catalog", Level: "info"})
	}
}
