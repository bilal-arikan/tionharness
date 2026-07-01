package tools

import (
	"context"
	"encoding/json"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// parseInput unmarshals a tool's raw JSON arguments into a fresh T, wrapping any
// error with argErrFor(tool) so every built-in reports malformed arguments the
// same way. It collapses the boilerplate triple that opened almost every Call:
//
//	var in fooInput
//	if err := json.Unmarshal(input, &in); err != nil {
//		return "", argErrFor("foo", err)
//	}
//
// into a single:
//
//	in, err := parseInput[fooInput]("foo", input)
//	if err != nil {
//		return "", err
//	}
func parseInput[T any](tool string, input json.RawMessage) (T, error) {
	var in T
	if err := json.Unmarshal(input, &in); err != nil {
		return in, argErrFor(tool, err)
	}
	return in, nil
}

// schemaEmpty is the input schema for a tool that takes no arguments. Shared so
// the dozens of parameterless list/status tools stop hand-writing the same
// object-with-no-properties literal (and can never drift apart).
var schemaEmpty = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

// funcTool is a Tool assembled from a static definition plus a call function,
// for tools whose whole behaviour is "parse args, do the thing" with no state.
// It removes the need to declare an empty struct type + two method receivers for
// such tools: NewFuncTool(def, fn) yields a ready Tool.
type funcTool struct {
	def providers.ToolDef
	fn  func(ctx context.Context, input json.RawMessage) (string, error)
}

func (t funcTool) Def() providers.ToolDef { return t.def }
func (t funcTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.fn(ctx, input)
}

// NewFuncTool builds a Tool from a definition and a call function.
func NewFuncTool(def providers.ToolDef, fn func(ctx context.Context, input json.RawMessage) (string, error)) Tool {
	return funcTool{def: def, fn: fn}
}
