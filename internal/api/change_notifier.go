package api

import (
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/tools"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

type toolChangeNotifier struct{ server *Server }

func (n toolChangeNotifier) workspaceFor(database *db.DB) *workspace.Workspace {
	for _, meta := range n.server.workspaces.List() {
		wsp, err := n.server.workspaces.Get(meta.ID)
		if err == nil && wsp.DB == database {
			return wsp
		}
	}
	return nil
}

func (n toolChangeNotifier) BoardChanged(database *db.DB, change tools.BoardChange) {
	publishEntityChange(n.workspaceFor(database), "board", change.Title, change.Body,
		map[string]string{"view": "board", "taskId": change.TaskID, "op": change.Op})
}

func (n toolChangeNotifier) AgentModelChanged(database *db.DB, change tools.AgentModelChange) {
	emitAgentModelChange(n.workspaceFor(database), change)
}

func emitAgentModelChange(wsp *workspace.Workspace, change tools.AgentModelChange) {
	if wsp == nil || wsp.Runtime == nil {
		return
	}
	wsp.Runtime.Emit(agentModelChangeEvent(change))
}

func agentModelChangeEvent(change tools.AgentModelChange) events.Event {
	return events.Event{
		Type:   "agent-model-changed",
		Level:  "info",
		Title:  "Model değişti: " + change.OldModel + " → " + change.NewModel,
		Body:   change.AgentName + " ajanının modeli güncellendi. Sonraki turdan itibaren geçerli; aktif konuşmanın prompt cache'i soğuyacak.",
		Target: map[string]string{"view": "agent", "agentId": change.AgentID},
		Time:   time.Now().UnixMilli(),
	}
}

func (s *Server) publishAgentModelChange(wsp *workspace.Workspace, change tools.AgentModelChange) {
	event := agentModelChangeEvent(change)
	event.WorkspaceID = wsp.ID
	s.bus.Publish(event)
}
