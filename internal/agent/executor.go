package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
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
	return run, nil
}

// invoke calls the agent's provider with a single user prompt.
func (r *Runtime) invoke(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, error) {
	return r.complete(ctx, agent, buildSystemPrompt(agent), prompt, autonomous)
}

// invokeWithMemory is like invoke but first recalls relevant memories and
// injects them into the system prompt, so the agent answers with context.
func (r *Runtime) invokeWithMemory(ctx context.Context, agent db.Agent, prompt string, autonomous bool) (string, error) {
	system := buildSystemPrompt(agent)
	if block := r.mem.ContextBlock(ctx, agent.ID, prompt, 5); block != "" {
		system = strings.TrimSpace(system + "\n\n" + block)
	}
	return r.complete(ctx, agent, system, prompt, autonomous)
}

// complete is the shared provider call used by invoke variants. It routes
// through CompleteWithTools, which enforces the daily budget (when autonomous),
// records usage, and runs the agentic tool loop when the agent has tools
// enabled.
func (r *Runtime) complete(ctx context.Context, agent db.Agent, system, prompt string, autonomous bool) (string, error) {
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return "", err
	}
	resp, err := r.CompleteWithTools(ctx, agent, provider, providers.Request{
		Model:  agent.Model,
		System: system,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: prompt},
		},
	}, autonomous)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
