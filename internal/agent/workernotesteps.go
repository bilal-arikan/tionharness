package agent

import "strings"

// digestWorkerSteps reduces a finished worker's full turn trace to the part the
// coordinator's <task-notification> card actually renders: the FILE CHANGES the
// worker made (plus its checklist), and nothing else.
//
// Why: notifyCoordinator persisted the worker's ENTIRE trace onto the injected
// user message. In SES2570 that was 8.2 MB of the coordinator's 12.3 MB
// messages.jsonl — single notifications carried 545 KB of shell output, and one
// step alone was a 67 KB `git diff`. None of it was ever displayed: the UI feeds
// those steps to notificationChanges(), which keeps only diff/edit steps
// (frontend/src/features/chat/parseTaskNotification.ts). The full trace already
// lives, intact, in the worker's own session — the notification names that
// session id, so nothing is lost by not duplicating it here.
//
// The trace is NOT sent to the model on either provider path (EstimateTokens
// excludes persisted steps, and EstimatePersistedStepTokens counts assistant
// messages only), so this is a storage/boot-memory fix, not a token fix: the
// whole store is loaded into RAM at boot.
//
// Kept, per step: file changes (kind diff, or a tool step whose tool is an edit
// tool — the UI synthesizes a diff from its input when no patch is attached) and
// todo steps (tiny, and the checklist card reads them). Dropped: tool outputs,
// thinking, plain text, ask/permission cards, and every other tool call.
func digestWorkerSteps(steps []TurnStep) []TurnStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]TurnStep, 0, len(steps))
	for _, st := range steps {
		// Recurse first: a subagent step's nested trace can hold real edits, and
		// the change extractor descends into it.
		nested := digestWorkerSteps(st.SubSteps)
		if kept, ok := keepWorkerStep(st); ok {
			kept.SubSteps = nested
			out = append(out, kept)
			continue
		}
		// The parent is noise but its children are not: hoist them rather than
		// losing edits made inside a subagent run.
		out = append(out, nested...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// keepWorkerStep decides whether one step survives the digest and returns the
// trimmed copy. The copy carries only the fields the change/checklist cards read
// — notably NOT Output, which is where the megabytes were.
func keepWorkerStep(st TurnStep) (TurnStep, bool) {
	switch {
	case st.Kind == StepDiff:
		return TurnStep{
			Kind:    StepDiff,
			Tool:    st.Tool,
			Path:    st.Path,
			Patch:   st.Patch,
			Added:   st.Added,
			Removed: st.Removed,
			Created: st.Created,
			IsError: st.IsError,
			ID:      st.ID,
		}, true
	case st.Kind == StepTool && isEditToolName(st.Tool):
		// Input is retained: with no patch attached the UI synthesizes the diff
		// from the tool's arguments (stepToFileChange → synthDiffData).
		return TurnStep{
			Kind:    StepTool,
			Tool:    st.Tool,
			Input:   st.Input,
			Path:    st.Path,
			Patch:   st.Patch,
			Added:   st.Added,
			Removed: st.Removed,
			Created: st.Created,
			IsError: st.IsError,
			ID:      st.ID,
		}, true
	case st.Kind == StepTodo:
		return TurnStep{Kind: StepTodo, Todos: st.Todos, ID: st.ID}, true
	default:
		return TurnStep{}, false
	}
}

// editToolNames are the tools whose calls the change extractor treats as file
// mutations. Mirrors isEditToolBase in frontend/src/shared/lib/fileChanges.ts —
// the two lists must agree, or a change kept here renders as a blank row (or a
// real edit is dropped before it ever reaches the card).
var editToolNames = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"apply_patch":  true,
	"write_config": true,
}

// isEditToolName reports whether a step's tool name denotes a file mutation,
// accepting the namespaced form a CLI provider emits
// (mcp__tionharness_extended__apply_patch) as well as the bare one.
func isEditToolName(tool string) bool {
	if i := strings.LastIndex(tool, "__"); i >= 0 {
		tool = tool[i+2:]
	}
	return editToolNames[tool]
}
