package db

import (
	"fmt"
	"sort"
	"strings"
)

// Trigger registry (_Docs/77 R5).
//
// Each automation trigger kind registers ONE TriggerSpec: the kind-specific
// validation of a merged automation and a human label. ValidateAutomationShape
// looks the kind up here instead of switching on it, so a new kind — the
// trajectory phase / trajectory_end triggers the Rota work adds — is a new
// file that calls RegisterTrigger, not another branch in three places. The
// matching/dispatch side stays with the engine (internal/agent/automation*.go),
// which already has one entry point per kind; this registry only owns what the
// STORE must agree on for every write path.
type TriggerSpec struct {
	Kind  string
	Label string
	// Validate checks the kind-specific fields of a merged automation. The common
	// checks (prompt template, session mode, runnable target) run afterwards in
	// ValidateAutomationShape unless NoTarget reports the rule needs none.
	Validate func(a Automation) error
	// NoTarget reports whether this particular automation performs bookkeeping
	// only (no session/flow launch) and therefore needs no target and no prompt.
	NoTarget func(a Automation) bool
}

var triggerSpecs = map[string]TriggerSpec{}

// RegisterTrigger adds (or replaces) a trigger kind. Called from init() by each
// kind's file; a later Rota package may register more at boot.
func RegisterTrigger(spec TriggerSpec) {
	if spec.Kind == "" {
		panic("db: RegisterTrigger with empty kind")
	}
	triggerSpecs[spec.Kind] = spec
}

// TriggerKinds lists the registered kinds, sorted.
func TriggerKinds() []string {
	out := make([]string, 0, len(triggerSpecs))
	for k := range triggerSpecs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ValidTriggerKind reports whether k is registered. The empty kind is the
// legacy spelling of tag and is accepted.
func ValidTriggerKind(k string) bool {
	if k == "" {
		return true
	}
	_, ok := triggerSpecs[k]
	return ok
}

// triggerSpecFor resolves a kind, mapping the legacy empty kind to tag.
func triggerSpecFor(kind string) (TriggerSpec, bool) {
	if kind == "" {
		kind = TriggerTag
	}
	spec, ok := triggerSpecs[kind]
	return spec, ok
}

func init() {
	RegisterTrigger(TriggerSpec{
		Kind: TriggerTag, Label: "Etiket",
		Validate: func(a Automation) error {
			if a.TriggerTag == "" {
				return fmt.Errorf("%w: triggerTag is required for tag automations (an empty tag never fires)", ErrAutomationShape)
			}
			return nil
		},
	})
	RegisterTrigger(TriggerSpec{
		Kind: TriggerBoard, Label: "Pano",
		Validate: func(a Automation) error {
			if !ValidBoardOp(a.BoardOp) {
				return fmt.Errorf("%w: invalid boardOp %q (any|move|create|update|delete)", ErrAutomationShape, a.BoardOp)
			}
			if !ValidBoardAction(a.BoardAction) {
				return fmt.Errorf("%w: invalid boardAction %q (spawn|archive|move)", ErrAutomationShape, a.BoardAction)
			}
			if a.BoardAction == BoardActionMove {
				if a.BoardMoveToState == "" {
					return fmt.Errorf("%w: boardMoveToState is required for move board actions", ErrAutomationShape)
				}
				if a.BoardToState == "" {
					return fmt.Errorf("%w: boardToState is required for move board actions and must differ from boardMoveToState to prevent self-triggering", ErrAutomationShape)
				}
				if a.BoardMoveToState == a.BoardToState {
					return fmt.Errorf("%w: boardMoveToState must differ from boardToState to prevent self-triggering", ErrAutomationShape)
				}
			}
			return nil
		},
		// Archive and move actions do bookkeeping with no LLM call, so they need
		// neither a target nor a prompt.
		NoTarget: func(a Automation) bool {
			return a.BoardAction == BoardActionArchive || a.BoardAction == BoardActionMove
		},
	})
	RegisterTrigger(TriggerSpec{
		Kind: TriggerToken, Label: "Token",
		Validate: func(a Automation) error {
			if !ValidTokenScope(a.TokenScope) {
				return fmt.Errorf("%w: invalid tokenScope %q (session|workspace)", ErrAutomationShape, a.TokenScope)
			}
			return ValidateTokenThreshold(a.TokenThreshold)
		},
	})
}

// validateCommonShape is the kind-independent tail of ValidateAutomationShape.
func validateCommonShape(a Automation) error {
	if strings.TrimSpace(a.PromptTemplate) == "" {
		return fmt.Errorf("%w: promptTemplate is required", ErrAutomationShape)
	}
	// Session mode is a free choice across kinds, but the value must be known.
	if !ValidSessionMode(a.SessionMode) {
		return fmt.Errorf("%w: invalid sessionMode %q (spawn|continue)", ErrAutomationShape, a.SessionMode)
	}
	// Every automation that reaches here spawns a session or runs a flow, so it
	// needs exactly one runnable target.
	if a.FlowID == "" && a.TargetAgentID == "" {
		return fmt.Errorf("%w: targetAgentId or flowId is required", ErrAutomationShape)
	}
	return nil
}
