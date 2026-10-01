package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

type AskUserAsyncTool struct{}

func NewAskUserAsyncTool() AskUserAsyncTool { return AskUserAsyncTool{} }

func (AskUserAsyncTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "ask_user_async",
		Description: "Ask the user one or more clarifying questions without stopping independent work. " +
			"Uses the same question/options or questions array as ask_user. Returns a request_id, not an answer. " +
			"The user's reply will arrive automatically with that id at a later tool/model boundary, or as a continuation after this turn. " +
			"Continue work that does not depend on the answer. When only dependent work remains, end the turn and wait for the reply. " +
			"Do not repeat or poll the question, invent an answer, or treat silence as consent. " +
			"For action approval use request_confirmation or the permission flow instead.",
		InputSchema: NewAskUserTool().Def().InputSchema,
	}
}

func (AskUserAsyncTool) Call(ctx context.Context, raw json.RawMessage) (string, error) {
	input := AsyncInputFrom(ctx)
	if IsAutonomous(ctx) || input == nil || input.Ask == nil {
		return "", fmt.Errorf("ask_user_async is only available in interactive chat sessions")
	}
	questions, err := ParseAskInputMulti(raw)
	if err != nil {
		return "", argErrFor("ask_user_async", err)
	}
	if len(questions) == 0 {
		return "", fmt.Errorf("question is required")
	}
	if text, skip := CheckClarification(ctx, raw); skip {
		return text, nil
	}
	id, err := input.Ask(ctx, questions)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(map[string]string{
		"request_id": id,
		"status":     "pending",
		"message":    "Question posted. Continue independent work; the user's answer will arrive separately. End the turn when further work requires the answer.",
	})
	return string(result), err
}
