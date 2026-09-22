package tools

import (
	"encoding/json"
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
