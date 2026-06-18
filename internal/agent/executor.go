package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/orchestration"
	"github.com/bilal/swarmgo/internal/providers"
)

// RunTask executes a task with its owner agent: it records a Run, calls the
// provider with the task prompt, stores the textual result, and moves the task
// to done/failed. trigger describes what initiated the run (manual | schedule).
//
// It returns the finished Run. A provider error is captured in the Run (status
// failure) rather than aborting, so the board always reflects the attempt.
func (r *Runtime) RunTask(ctx context.Context, taskID, trigger string) (db.Run, error) {
	return r.RunTaskStream(ctx, taskID, trigger, nil)
}

// RunTaskStream is RunTask that additionally streams each activity step to onStep
// the moment it occurs (for live SSE). onStep may be nil — then it behaves
// exactly like RunTask. For a flow-backed task, onStep receives a text step per
// node as the flow advances.
func (r *Runtime) RunTaskStream(ctx context.Context, taskID, trigger string, onStep func(TurnStep)) (db.Run, error) {
	ctx = WithCallKind(ctx, KindTask) // attribute every provider call this run makes to the Kanban board
	task, err := r.db.GetTask(ctx, taskID)
	if err != nil {
		return db.Run{}, err
	}
	// Flow-backed task: run the orchestration flow instead of a single agent.
	if task.FlowID != "" {
		return r.runTaskFlow(ctx, task, trigger, onStep)
	}
	if task.OwnerAgentID == "" {
		return db.Run{}, errors.New("task has no owner agent")
	}

	prompt := task.Prompt
	if prompt == "" {
		prompt = task.Description
	}
	if prompt == "" {
		prompt = task.Title
	}

	agent, err := r.db.GetAgent(ctx, task.OwnerAgentID)
	if err != nil {
		return db.Run{}, fmt.Errorf("owner agent: %w", err)
	}

	// Funnel the run into the task's transcript session so it reads — and streams —
	// like any chat. Each run appends a user turn (the prompt) now and an assistant
	// turn (the activity trace) when it finishes.
	session, _ := r.taskSession(ctx, task)
	if session.ID != "" {
		if _, err := r.db.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "user", Text: prompt}); err != nil {
			r.logger.Warn("task transcript: record prompt failed", "task", taskID, "session", session.ID, "error", err)
		}
	}

	// Open the run and flip the board to in_progress.
	run, err := r.db.CreateRun(ctx, db.Run{
		TaskID:    taskID,
		AgentID:   task.OwnerAgentID,
		SessionID: session.ID,
		Status:    db.RunRunning,
		Trigger:   trigger,
	})
	if err != nil {
		return db.Run{}, err
	}
	if err := r.db.MoveTask(ctx, taskID, db.BoardInProgress); err != nil {
		r.logger.Warn("task board move to in_progress failed", "task", taskID, "error", err)
	}

	// Manual run-now is user-initiated; scheduled runs are autonomous and
	// therefore subject to the agent's daily budget.
	autonomous := trigger != "manual"
	output, steps, runErr := r.invokeWithMemoryStream(ctx, agent, prompt, autonomous, onStep)

	status := db.RunSuccess
	board := db.BoardDone
	errText := ""
	if runErr != nil {
		status = db.RunFailure
		board = db.BoardFailed
		errText = runErr.Error()
	} else {
		// Record the successful run so the agent can recall/reflect on it later.
		r.Journal(ctx, task.OwnerAgentID,
			fmt.Sprintf("Task %q → %s", task.Title, output))
	}

	// Record the assistant turn (with its activity trace) in the transcript.
	msgID := r.recordRunReply(ctx, session.ID, task.OwnerAgentID, output, errText, steps)

	if err := r.db.FinishRun(ctx, run.ID, status, output, errText); err != nil {
		r.logger.Warn("finish run failed", "run", run.ID, "error", err)
	}
	if session.ID != "" {
		if err := r.db.SetRunSession(ctx, run.ID, session.ID, msgID); err != nil {
			r.logger.Warn("link run session failed", "run", run.ID, "error", err)
		}
	}
	if err := r.db.SetTaskLastRun(ctx, taskID, run.ID, status, board); err != nil {
		r.logger.Warn("set task last run failed", "task", taskID, "error", err)
	}

	run.Status = status
	run.Output = output
	run.Error = errText
	run.MessageID = msgID

	// Match the log level to the outcome and always carry the error text on
	// failure, so a scheduled (or manual) task that fails is explained in the
	// logs view instead of only in the desktop notification.
	if runErr != nil {
		r.logger.Error("task run failed",
			"task", taskID, "run", run.ID, "agent", task.OwnerAgentID,
			"trigger", trigger, "provider", agent.Provider, "model", agent.Model,
			"error", runErr)
	} else {
		r.logger.Info("task run finished",
			"task", taskID, "run", run.ID, "trigger", trigger, "status", status)
	}

	// Notify on the outcome; clicking deep-links to the board. The frontend
	// suppresses the desktop notification while its tab is focused, so a manual
	// run the user is watching won't pop a redundant notification.
	level, title, body := "success", "Görev tamamlandı: "+task.Title, output
	if status == db.RunFailure {
		level, title, body = "error", "Görev başarısız: "+task.Title, errText
	}
	r.publish(events.Event{
		Type:   "task",
		Level:  level,
		Title:  title,
		Body:   body,
		Target: map[string]string{"view": "board", "taskId": taskID},
	})

	return run, nil
}

// runTaskFlow executes a flow-backed task: it runs the task's linked flow with
// the task prompt as input, records the rendered transcript as a Run, and moves
// the board to done/failed. No owner agent is required — the flow's nodes carry
// their own agents. Mirrors RunTask's bookkeeping so flow tasks behave like any
// other task on the board (history, notifications, scheduling).
func (r *Runtime) runTaskFlow(ctx context.Context, task db.Task, trigger string, onStep func(TurnStep)) (db.Run, error) {
	input := task.Prompt
	if input == "" {
		input = task.Description
	}

	flow, ferr := r.db.GetFlow(ctx, task.FlowID)
	flowName := task.FlowID
	if ferr == nil {
		flowName = flow.Name
	}

	// Transcript session for the task: record the flow input as a user turn now.
	session, _ := r.taskSession(ctx, task)
	if session.ID != "" {
		userText := input
		if userText == "" {
			userText = "🔀 " + flowName
		}
		if _, err := r.db.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "user", Text: userText}); err != nil {
			r.logger.Warn("flow task transcript: record input failed", "task", task.ID, "session", session.ID, "error", err)
		}
	}

	// Open the run and flip the board to in_progress.
	run, err := r.db.CreateRun(ctx, db.Run{
		TaskID:    task.ID,
		AgentID:   task.OwnerAgentID, // optional; flows carry their own agents
		SessionID: session.ID,
		Status:    db.RunRunning,
		Trigger:   trigger,
	})
	if err != nil {
		return db.Run{}, err
	}
	if err := r.db.MoveTask(ctx, task.ID, db.BoardInProgress); err != nil {
		r.logger.Warn("flow task board move to in_progress failed", "task", task.ID, "error", err)
	}

	// Scheduled/dispatcher runs are autonomous (budget-gated per node); a manual
	// run-now is user-initiated. When streaming, surface each node as a step.
	autonomous := trigger != "manual"
	var obs orchestration.Observer
	if onStep != nil {
		obs = func(ev orchestration.NodeEvent) {
			if ev.Phase != "done" {
				return
			}
			title := ev.Title
			if title == "" {
				title = ev.NodeID
			}
			onStep(TurnStep{Kind: StepText, Text: "**" + title + "**\n\n" + ev.Output})
		}
	}
	flowRun, runErr := r.RunFlow(ctx, task.FlowID, input, autonomous, obs)
	output := renderFlowTranscript(flowName, flowRun, runErr)

	status := db.RunSuccess
	board := db.BoardDone
	errText := ""
	if runErr != nil {
		status, board, errText = db.RunFailure, db.BoardFailed, runErr.Error()
	} else if flowRun.Status == db.FlowFailure {
		status, board, errText = db.RunFailure, db.BoardFailed, flowRun.Error
	} else {
		r.Journal(ctx, task.OwnerAgentID,
			fmt.Sprintf("Flow task %q → %s", task.Title, output))
	}

	// Assistant turn: the per-node breakdown as a step trace, the rendered
	// transcript as the message body. Attributed to the flow's final agent.
	replyAgent := finalFlowAgentID(flow, flowRun)
	if replyAgent == "" {
		replyAgent = task.OwnerAgentID
	}
	msgID := ""
	if session.ID != "" {
		m, merr := r.db.AddMessage(ctx, db.Message{
			SessionID: session.ID,
			AgentID:   replyAgent,
			Role:      "assistant",
			Text:      output,
			Steps:     encodeSteps(flowStateToSteps(flowRun, runErr)),
		})
		if merr == nil {
			msgID = m.ID
		}
	}

	if err := r.db.FinishRun(ctx, run.ID, status, output, errText); err != nil {
		r.logger.Warn("finish flow-run failed", "run", run.ID, "error", err)
	}
	if session.ID != "" {
		if err := r.db.SetRunSession(ctx, run.ID, session.ID, msgID); err != nil {
			r.logger.Warn("link run session failed", "run", run.ID, "error", err)
		}
	}
	if err := r.db.SetTaskLastRun(ctx, task.ID, run.ID, status, board); err != nil {
		r.logger.Warn("set task last run failed", "task", task.ID, "error", err)
	}

	run.Status = status
	run.Output = output
	run.Error = errText
	run.MessageID = msgID

	r.logger.Info("flow task run finished", "task", task.ID, "flow", task.FlowID, "trigger", trigger, "status", status)

	level, title, body := "success", "Akış görevi tamamlandı: "+task.Title, output
	if status == db.RunFailure {
		level, title, body = "error", "Akış görevi başarısız: "+task.Title, errText
	}
	r.publish(events.Event{
		Type:   "task",
		Level:  level,
		Title:  title,
		Body:   body,
		Target: map[string]string{"view": "board", "taskId": task.ID},
	})

	return run, nil
}

// renderFlowTranscript turns a finished flow run into a plain-text transcript for
// storage as a task Run output: a header plus one section per executed node.
func renderFlowTranscript(flowName string, fr db.FlowRun, setupErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 %s akışı çalıştı\n", flowName)
	if setupErr != nil {
		fmt.Fprintf(&b, "\n⚠️ Akış başlatılamadı: %s", setupErr.Error())
		return strings.TrimSpace(b.String())
	}
	var st orchestration.State
	_ = json.Unmarshal([]byte(fr.State), &st)
	for i, t := range st.Trace {
		title := t.Title
		if title == "" {
			title = t.NodeID
		}
		fmt.Fprintf(&b, "\n%d. %s\n%s\n", i+1, title, t.Output)
	}
	if fr.Status == db.FlowFailure {
		fmt.Fprintf(&b, "\n⚠️ Durum: hata — %s", fr.Error)
	}
	return strings.TrimSpace(b.String())
}

// TaskSession is the exported accessor for a task's transcript session, so the
// API can resolve the session id up front (e.g. to register a streaming run for
// the executions feed's live "running" flag).
func (r *Runtime) TaskSession(ctx context.Context, task db.Task) (db.Session, error) {
	return r.taskSession(ctx, task)
}

// taskSession returns (creating if absent) the transcript session that holds a
// task's run history. Keyed by the task id so every run of the task threads into
// one conversation, viewable in the same streamable transcript as a chat.
func (r *Runtime) taskSession(ctx context.Context, task db.Task) (db.Session, error) {
	title := task.Title
	if title == "" {
		title = "Görev"
	}
	session, err := r.db.GetOrCreateSourceSession(ctx, "task", task.ID, task.OwnerAgentID, title)
	if err != nil {
		r.logger.Warn("task session create failed", "task", task.ID, "error", err)
	}
	return session, err
}

// recordRunReply appends the assistant turn for a finished task run to its
// transcript session and returns the message id. On failure it records the error
// text so the transcript explains what went wrong. A no-op (empty id) when the
// session is unavailable.
func (r *Runtime) recordRunReply(ctx context.Context, sessionID, agentID, output, errText string, steps []TurnStep) string {
	if sessionID == "" {
		return ""
	}
	text := output
	if errText != "" {
		text = "⚠️ " + errText
		steps = append(steps, TurnStep{Kind: StepError, Reason: "task_run", Text: errText})
	}
	m, err := r.db.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		AgentID:   agentID,
		Role:      "assistant",
		Text:      text,
		Steps:     encodeSteps(steps),
	})
	if err != nil {
		r.logger.Warn("record run reply failed", "session", sessionID, "error", err)
		return ""
	}
	return m.ID
}

// invoke calls the agent's provider with a single user prompt.
func (r *Runtime) invoke(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, error) {
	return r.complete(ctx, agent, r.systemPrompt(agent), "", prompt, autonomous)
}

// invokeWithMemoryStream is invokeWithMemory plus the activity trace: it recalls
// relevant memories into the dynamic system suffix and returns the agent's
// thinking/tool steps so a task run can be persisted as a rich chat turn. When
// onStep is non-nil each step is also delivered live (for SSE streaming).
func (r *Runtime) invokeWithMemoryStream(ctx context.Context, agent db.Agent, prompt string, autonomous bool, onStep func(TurnStep)) (string, []TurnStep, error) {
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return "", nil, err
	}
	dynamic := strings.TrimSpace(r.mem.ContextBlock(ctx, agent.ID, prompt, 5))
	resp, steps, err := r.CompleteWithToolsStream(ctx, agent, provider, providers.Request{
		Model:         agent.Model,
		System:        r.systemPrompt(agent),
		SystemDynamic: dynamic,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}, autonomous, onStep)
	if err != nil {
		return "", nil, err
	}
	return resp.Text, steps, nil
}

// invokeTraced is like invoke but also returns the agent's activity trace
// (thinking/tool steps), so callers can persist a rich chat turn rather than a
// bare text reply. Used by the scheduler so scheduled runs render like normal
// chat turns in the agent's schedule session.
func (r *Runtime) invokeTraced(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, []TurnStep, error) {
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return "", nil, err
	}
	resp, steps, err := r.CompleteWithToolsTraced(ctx, agent, provider, providers.Request{
		Model:  agent.Model,
		System: r.systemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}, autonomous)
	if err != nil {
		return "", nil, err
	}
	return resp.Text, steps, nil
}

// invokeWithMemory is like invoke but first recalls relevant memories and
// passes them as the dynamic (uncached) system suffix, so the agent answers with
// context without invalidating the cached static persona prefix.
func (r *Runtime) invokeWithMemory(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, error) {
	dynamic := strings.TrimSpace(r.mem.ContextBlock(ctx, agent.ID, prompt, 5))
	return r.complete(ctx, agent, r.systemPrompt(agent), dynamic, prompt, autonomous)
}

// complete is the shared provider call used by invoke variants. system is the
// static prefix, systemDynamic the volatile suffix (see providers.Request). It
// routes through CompleteWithTools, which enforces the daily budget (when
// autonomous), records usage, and runs the agentic tool loop when the agent has
// tools enabled.
func (r *Runtime) complete(ctx context.Context, agent db.Agent, system, systemDynamic, prompt string, autonomous bool) (string, error) {
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return "", err
	}
	resp, err := r.CompleteWithTools(ctx, agent, provider, providers.Request{
		Model:         agent.Model,
		System:        system,
		SystemDynamic: systemDynamic,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}, autonomous)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
