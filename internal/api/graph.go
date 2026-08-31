package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// graphNode is one entity in the workspace collaboration network. ID is
// type-prefixed ("agent:<id>#<sessionID>", "task:<id>", "flow:<id>",
// "skill:<slug>", "mcp:<id>") so ids are unique across types and edges can
// reference them.
//
// Agents are RUNTIME INSTANCES, not definitions: one node per in-flight session
// (chat / task / flow / schedule / spawned / worker / inbox). An agent driving
// three sessions appears three times; an agent with nothing running does not
// appear at all. That is why the agent id carries the session suffix.
type graphNode struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // agent | task | flow | skill | mcp
	Label  string `json:"label"`
	Sub    string `json:"sub,omitempty"`    // subtitle (provider/model, status, …)
	Color  string `json:"color,omitempty"`  // agent accent (drives the cluster hue)
	Emoji  string `json:"emoji,omitempty"`  // agent avatar glyph, if any
	Group  string `json:"group,omitempty"`  // owning hub id (for cluster layout)
	Status string `json:"status,omitempty"` // task board state
	Desc   string `json:"desc,omitempty"`   // longer description (task tooltip)

	// Live activity (agents only): agent nodes exist ONLY while running, so
	// Running is always true on them. RunKind is the session kind driving this
	// instance and RunTarget the type-prefixed id of the task/flow it is running
	// (empty for chat/schedule/spawned).
	Running   bool   `json:"running,omitempty"`
	RunKind   string `json:"runKind,omitempty"`
	RunTarget string `json:"runTarget,omitempty"`

	// SessionID is the running session behind an agent instance node (agents
	// only) — lets the UI deep-link an instance to its transcript.
	SessionID string `json:"sessionId,omitempty"`

	// AgentID is the underlying agent definition id shared by every instance of
	// the same agent. Set on agent-instance nodes, on task nodes (the owner), and
	// on run-history nodes (the session's agent) so the Network screen can filter
	// by agent across all of them.
	AgentID string `json:"agentId,omitempty"`

	// Archived flags a node whose backing session is archived (agent instances and
	// run-history nodes). Lets the UI hide/show archived work like the board does.
	Archived bool `json:"archived,omitempty"`

	// Tags are the free-form labels on the backing session/task, so the Network
	// screen can filter by tag the same way the board does.
	Tags []string `json:"tags,omitempty"`
}

// graphEdge links two graph nodes. Kind names the relationship so the frontend
// can color/label it: owns (agent→task), created (agent→task), runs (task→flow),
// uses (flow→agent), skill (agent→skill), mcp (agent→server).
type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type workspaceGraph struct {
	Nodes []graphNode    `json:"nodes"`
	Edges []graphEdge    `json:"edges"`
	Stats map[string]int `json:"stats"`
}

// registerGraphRoutes registers the relationship-graph endpoints: the
// workspace-wide collaboration network.
func (s *Server) registerGraphRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph", s.handleWorkspaceGraph)
}

// handleWorkspaceGraph returns the workspace collaboration network: agents,
// tasks and flows as nodes, with edges for ownership/authorship (agent↔task),
// flow-backing (task→flow) and flow membership (flow→agent). This is the data
// behind the "Ağ" visualization.
func (s *Server) handleWorkspaceGraph(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()

	agents, err := wsp.DB.ListAgents(ctx)
	if writeDBError(w, err, "") {
		return
	}
	tasks, err := wsp.DB.ListActiveTasks(ctx)
	if writeDBError(w, err, "") {
		return
	}
	flows, err := wsp.DB.ListFlows(ctx)
	if writeDBError(w, err, "") {
		return
	}
	mcpServers, err := wsp.DB.ListMCPServers(ctx)
	if writeDBError(w, err, "") {
		return
	}

	const (
		agentPfx = "agent:"
		taskPfx  = "task:"
		flowPfx  = "flow:"
		skillPfx = "skill:"
		mcpPfx   = "mcp:"
	)

	nodes := make([]graphNode, 0, len(agents)+len(tasks)+len(flows))
	edges := make([]graphEdge, 0)

	// Which sessions are in flight right now, derived from the process-wide
	// running session set joined to this workspace's sessions.
	running := s.runningSessionIDs(wsp)
	// Session list backs both the agent instance nodes and the completed
	// run-history nodes built later.
	sessions, _ := wsp.DB.ListSessions(ctx, "")

	agentExists := make(map[string]bool, len(agents))
	for _, a := range agents {
		agentExists[a.ID] = true
	}

	instanceNodes, agentInstances := buildAgentInstances(agents, sessions, running)
	nodes = append(nodes, instanceNodes...)

	// addAgentEdges links every live instance of an agent to another node. With
	// no running instance the relationship simply isn't drawn.
	addAgentEdges := func(agentID, other, kind string, agentIsSource bool) {
		for _, inst := range agentInstances[agentID] {
			if agentIsSource {
				edges = append(edges, graphEdge{Source: inst, Target: other, Kind: kind})
			} else {
				edges = append(edges, graphEdge{Source: other, Target: inst, Kind: kind})
			}
		}
	}

	for _, t := range tasks {
		label := t.Title
		if label == "" {
			label = "(başlıksız görev)"
		}
		// Cluster a task under its owner agent when that agent has a live
		// instance, so the layout draws tasks as spokes around their agent hub.
		// Multiple instances → cluster under the first one (a task has one hub).
		group := ""
		if inst := agentInstances[t.OwnerAgentID]; len(inst) > 0 {
			group = inst[0]
		}
		nodes = append(nodes, graphNode{
			ID:      taskPfx + t.ID,
			Type:    "task",
			Label:   label,
			Status:  t.BoardState,
			Group:   group,
			Desc:    t.Description,
			AgentID: t.OwnerAgentID,
			Tags:    t.Tags,
		})
		if t.OwnerAgentID != "" {
			addAgentEdges(t.OwnerAgentID, taskPfx+t.ID, "owns", true)
		}
		if t.CreatedBy != "" && t.CreatedBy != t.OwnerAgentID {
			addAgentEdges(t.CreatedBy, taskPfx+t.ID, "created", true)
		}
		if t.FlowID != "" {
			edges = append(edges, graphEdge{Source: taskPfx + t.ID, Target: flowPfx + t.FlowID, Kind: "runs"})
		}
	}

	for _, f := range flows {
		nodes = append(nodes, graphNode{
			ID:    flowPfx + f.ID,
			Type:  "flow",
			Label: f.Name,
			Sub:   f.ID,
		})
		// A flow "uses" every distinct agent referenced by its agent nodes — this
		// is the multi-agent collaboration signal (two agents wired into one flow).
		seen := map[string]bool{}
		if g, perr := orchestration.ParseGraph(f.Graph); perr == nil {
			for _, n := range g.Nodes {
				if n.Type == orchestration.NodeAgent && n.AgentID != "" && !seen[n.AgentID] && agentExists[n.AgentID] {
					seen[n.AgentID] = true
					addAgentEdges(n.AgentID, flowPfx+f.ID, "uses", false)
				}
			}
		}
	}

	// Skills: a shared library node per distinct slug used by a RUNNING agent,
	// with an edge to each of that agent's live instances. Skills of idle agents
	// stay off the canvas — the network only shows what is in flight. The skill's
	// organisation group (when set) rides along in Sub so the frontend can tint
	// same-group skills alike.
	skillStore := wsp.Runtime.Skills()
	skillSeen := map[string]bool{}
	skillCount := 0
	for _, a := range agents {
		if len(agentInstances[a.ID]) == 0 {
			continue
		}
		for _, slug := range a.Skills {
			if slug == "" {
				continue
			}
			if !skillSeen[slug] {
				skillSeen[slug] = true
				skillCount++
				group := ""
				if sk, ok := skillStore.Get(slug); ok {
					group = sk.Group
				}
				nodes = append(nodes, graphNode{ID: skillPfx + slug, Type: "skill", Label: slug, Sub: group})
			}
			addAgentEdges(a.ID, skillPfx+slug, "skill", true)
		}
	}

	// MCP servers: one node per enabled server; an agent→server edge for every
	// MCP-enabled agent (coarse access signal — TionHarness gates tools per agent via
	// an allowlist, not per server, so this shows "which agents can reach MCP").
	// Only agents with a live instance are wired up, and a server with no live
	// consumer is left out entirely rather than floating unconnected.
	mcpCount := 0
	liveMCPAgents := make([]string, 0, len(agents))
	for _, a := range agents {
		if a.MCPEnabled && len(agentInstances[a.ID]) > 0 {
			liveMCPAgents = append(liveMCPAgents, a.ID)
		}
	}
	if len(liveMCPAgents) > 0 {
		for _, srv := range mcpServers {
			if !srv.Enabled {
				continue
			}
			mcpCount++
			nodes = append(nodes, graphNode{ID: mcpPfx + srv.ID, Type: "mcp", Label: srv.Name, Sub: srv.Transport})
			for _, agentID := range liveMCPAgents {
				addAgentEdges(agentID, mcpPfx+srv.ID, "mcp", true)
			}
		}
	}

	// Completed run history: finished (not-running) execution sessions as "run"
	// nodes — the same task/flow/schedule transcripts the Activity
	// (executions) screen lists. The live view drifts these to a "Geçmiş"
	// (archive) anchor so finished work piles up there as titled cards. Capped to
	// the most recent runHistoryCap by recency to bound the payload.
	const runPfx = "run:"
	const runHistoryCap = 50
	agentName := make(map[string]string, len(agents))
	for _, a := range agents {
		agentName[a.ID] = a.Name
	}
	// historyKinds is every execution kind that piles up in the "Geçmiş" archive
	// anchor. Beyond the Activity feed's chat/task/flow/schedule it now also
	// includes worker/spawned and flow-coordinator, so finished coordinator
	// workers are visible (and filterable) instead of vanishing when they end.
	//
	// "inbox" is retained for sessions created BEFORE peer messages moved into the
	// recipient's ordinary chat thread (TSK507). Nothing stamps that kind any more,
	// but existing transcripts still carry it and must stay visible here.
	historyKinds := map[string]bool{
		"chat": true, "task": true, "flow": true, "schedule": true,
		"worker": true, "spawned": true, "inbox": true,
		agent.SessionKindFlowCoordinator: true,
	}
	// sessions are newest-updated first from ListSessions; take the first N
	// finished (not-running) ones. Agent is optional (flow sessions may have none)
	// and only enriches the tooltip. Archived sessions are included but flagged, so
	// the UI can hide them by default and reveal them on demand.
	runCount := 0
	for _, sess := range sessions {
		if runCount >= runHistoryCap {
			break
		}
		if running[sess.ID] {
			continue
		}
		if !historyKinds[sess.Kind] {
			continue
		}
		label := sess.Title
		if label == "" {
			label = "(" + sess.Kind + ")"
		}
		nodes = append(nodes, graphNode{
			ID:       runPfx + sess.ID,
			Type:     "run",
			Label:    label,
			RunKind:  sess.Kind,
			Sub:      agentName[sess.AgentID],
			AgentID:  sess.AgentID,
			Archived: sess.State == "archived",
			Tags:     sess.Tags,
		})
		runCount++
	}

	writeJSON(w, http.StatusOK, workspaceGraph{
		Nodes: nodes,
		Edges: edges,
		Stats: map[string]int{
			// "agents" counts live INSTANCES (what the canvas draws), while
			// "agentsTotal" keeps the definition count for the "x / y" readout.
			"agents":      instanceCount(agentInstances),
			"agentsTotal": len(agents),
			"tasks":       len(tasks),
			"flows":       len(flows),
			"skills":      skillCount,
			"mcp":         mcpCount,
			"runs":        runCount,
			"edges":       len(edges),
		},
	})
}

// buildAgentInstances turns the running sessions into agent instance nodes: ONE
// node per in-flight session, so a single agent driving a chat, a task and two
// spawned runs shows up as four copies. An agent with nothing in flight
// contributes no node at all — the network only ever shows live work.
//
// It returns the nodes plus an agentID → instance-node-ids index, so the
// relationship edges can fan out across every live copy of an agent.
func buildAgentInstances(agents []db.Agent, sessions []db.Session, running map[string]bool) ([]graphNode, map[string][]string) {
	agentByID := make(map[string]db.Agent, len(agents))
	for _, a := range agents {
		agentByID[a.ID] = a
	}
	nodes := make([]graphNode, 0, len(agents))
	instances := make(map[string][]string)
	for _, sess := range sessions {
		if !running[sess.ID] || sess.AgentID == "" {
			continue
		}
		a, ok := agentByID[sess.AgentID]
		if !ok {
			continue // orphan session pointing at a deleted agent
		}
		target := ""
		switch sess.Kind {
		case "task":
			if sess.SourceID != "" {
				target = "task:" + sess.SourceID
			}
		case "flow", agent.SessionKindFlowCoordinator:
			if sess.SourceID != "" {
				target = "flow:" + sess.SourceID
			}
		}
		id := "agent:" + sess.AgentID + "#" + sess.ID
		instances[sess.AgentID] = append(instances[sess.AgentID], id)
		nodes = append(nodes, graphNode{
			ID:        id,
			Type:      "agent",
			Label:     a.Name,
			Sub:       instanceSub(sess.Kind, sess.Title),
			Color:     a.Color,
			Emoji:     a.Avatar,
			Running:   true,
			RunKind:   sess.Kind,
			RunTarget: target,
			SessionID: sess.ID,
			AgentID:   sess.AgentID,
			Archived:  sess.State == "archived",
			Tags:      sess.Tags,
		})
	}
	return nodes, instances
}

// instanceCount totals the agent instance nodes across all agents.
func instanceCount(m map[string][]string) int {
	n := 0
	for _, ids := range m {
		n += len(ids)
	}
	return n
}

// instanceRunLabel names the execution path behind an agent instance in Turkish
// (the UI language) — what makes two copies of the same agent tell apart.
var instanceRunLabel = map[string]string{
	"chat":             "Sohbet",
	"task":             "Görev",
	"flow":             "Akış",
	"flow-coordinator": "Akış koordinatörü",
	"schedule":         "Zamanlama",
	"spawned":          "Spawn",
	"worker":           "Worker",
	// Legacy only — no new session carries "inbox" (TSK507); kept so old
	// transcripts still render a name instead of a raw kind string.
	"inbox":               "Inbox",
	db.SessionKindInsight: "İçgörü taraması",
}

// instanceSub builds an agent instance's subtitle: the run kind plus the
// session title when it has one, e.g. "Görev · Refactor the parser".
func instanceSub(kind, title string) string {
	label := instanceRunLabel[kind]
	if label == "" {
		label = kind
	}
	if label == "" {
		label = "Çalışıyor"
	}
	if title != "" {
		return label + " · " + title
	}
	return label
}
