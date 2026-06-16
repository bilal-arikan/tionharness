package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/providers"
)

// RunTask executes a task with its owner agent: it records a Run, calls the
// provider with the task prompt, stores the textual result, and moves the task
// to done/failed. trigger describes what initiated the run (manual | schedule).
//
// It returns the finished Run. A provider error is captured in the Run (status
// failure) rather than aborting, so the board always reflects the attempt.
func (r *Runtime) RunTask(ctx context.Context, taskID, trigger string) (db.Run, error) {
	task, err := r.db.GetTask(ctx, taskID)
	if err != nil {
		return db.Run{}, err
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

	// Open the run and flip the board to in_progress.
	run, err := r.db.CreateRun(ctx, db.Run{
		TaskID:  taskID,
		AgentID: task.OwnerAgentID,
		Status:  db.RunRunning,
		Trigger: trigger,
	})
	if err != nil {
		return db.Run{}, err
	}
	_ = r.db.MoveTask(ctx, taskID, db.BoardInProgress)

	// Manual run-now is user-initiated; scheduled runs are autonomous and
	// therefore subject to the agent's daily budget.
	autonomous := trigger != "manual"
	output, runErr := r.invokeWithMemory(ctx, agent, prompt, autonomous)

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

	if err := r.db.FinishRun(ctx, run.ID, status, output, errText); err != nil {
		r.logger.Warn("finish run failed", "run", run.ID, "error", err)
	}
	if err := r.db.SetTaskLastRun(ctx, taskID, run.ID, status, board); err != nil {
		r.logger.Warn("set task last run failed", "task", taskID, "error", err)
	}

	run.Status = status
	run.Output = output
	run.Error = errText

	r.logger.Info("task run finished", "task", taskID, "trigger", trigger, "status", status)

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

// invoke calls the agent's provider with a single user prompt.
func (r *Runtime) invoke(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, error) {
	return r.complete(ctx, agent, r.systemPrompt(agent), "", prompt, autonomous)
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
