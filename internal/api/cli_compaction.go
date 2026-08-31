package api

import (
	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// isCompletedNativeCompaction reports whether one trace entry is a FINISHED
// CLI-side compaction lifecycle event. The claude-cli parser emits the same
// (kind, source) pair twice — once running, once completed — so the !running
// test is what separates "the CLI started compacting" from "the CLI's window has
// actually been rebuilt".
//
// Both compaction paths gate on this: the explicit /compact command
// (nativeCompactSession) turns the completions into transcript steps, and the
// ordinary turn path (chat_stream) uses it to detect a compaction the CLI fired
// on its OWN. Keeping one predicate stops the two from drifting apart.
//
// Takes the fields rather than a struct because the two callers hold different
// types for the same event: providers.TraceStep before the turn is converted,
// agent.TurnStep after.
func isCompletedNativeCompaction(kind, source string, running bool) bool {
	return kind == "compaction" && source == "cli-native" && !running
}

// cliCompactionBoundary inspects one turn's persisted steps for a compaction the
// CLI performed on ITSELF, and returns the transcript boundary to re-baseline to
// plus whether there was one at all.
//
// Both CLI providers reach here: claude-cli reports it through its compact_boundary
// / PostCompact / status lifecycle (claudecli_stream.go) and codex-cli through the
// context_compaction item (codexcli_events.go, Trigger "auto"). Neither provider's
// MANUAL compaction runs on this path — /compact goes through CompactNative in its
// own call — so no trigger filter is needed here.
//
// rawLen is the transcript length BEFORE this turn's reply is appended: the
// compaction fired mid-turn, so the reply's own trace landed after it and is still
// held by the provider.
func cliCompactionBoundary(steps []agent.TurnStep, rawLen int) (int, bool) {
	for _, st := range steps {
		if isCompletedNativeCompaction(string(st.Kind), st.Source, st.Running) {
			return rawLen, true
		}
	}
	return 0, false
}

// warmCLIStepBaseline returns the transcript index from which persisted assistant
// Steps are still inside the provider's live context — i.e. which tool trace the
// warm CLI thread will re-send on the next turn and therefore has to be charged
// to the context meter and the fold gate.
//
// It is the LATER of two baselines: the rolling summary boundary (everything
// before it was folded away by TionHarness) and the CLI's own compaction boundary
// (everything before it was folded away by the CLI itself). Without the second
// term an auto-compaction left the meter charging a trace the CLI had already
// dropped, so the gate fired folds that were not needed.
//
// Returns -1 when there is no warm thread at all: a cold turn replays the
// composed history and retains no trace, so nothing may be counted.
func warmCLIStepBaseline(session db.Session, historyLen int) int {
	if !hasWarmCLIThread(session) {
		return -1
	}
	base := session.SummaryMsgCount
	if session.CLICompactMsgCount > base {
		base = session.CLICompactMsgCount
	}
	// A boundary past the end of the transcript means the history shrank under it
	// (message edits/deletes). Fall back to counting the whole pending window, the
	// same conservative choice the meter made before this baseline existed.
	if base < 0 || base > historyLen {
		return 0
	}
	return base
}
