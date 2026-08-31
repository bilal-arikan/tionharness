package api

import (
	"errors"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// errNativeCompactUnavailable marks the "this session cannot be compacted by the
// CLI itself" class of failure — an unsupported/too-old provider, or a session
// with no resumable CLI thread yet. It is deliberately separate from a real
// runtime error (a failed CompactNative call, a persist error): the manual path
// reports either one to the user, but the automatic gate has to tell them apart
// so it can fall back to the rolling fold on unavailability and surface a genuine
// failure. Check with errors.Is, never by string.
var errNativeCompactUnavailable = errors.New("native CLI compaction unavailable for this session")

// nativeCompactMode names the call path invoking the CLI's native compactor. The
// two paths differ only in how many messages the turn adds around the compaction,
// which is what nativeCompactBoundary encodes.
type nativeCompactMode int

const (
	// nativeCompactManual is the explicit /compact command: the command message
	// and its report are appended to the transcript around the compaction.
	nativeCompactManual nativeCompactMode = iota
	// nativeCompactAuto is the gate firing inside an ordinary turn: no command
	// message is appended, so the boundary is the raw history length.
	nativeCompactAuto
)

// nativeCompactBoundary returns the transcript index the CLI's rebuilt window
// starts at, for both the stored resume boundary and the context-meter boundary.
//
// historyLen is the pre-command transcript length. The manual path adds +2 on top
// of it because the /compact command message and the report this turn writes back
// both land after the snapshot and are already inside the CLI's fresh window. The
// automatic path appends no command message, so its boundary is historyLen as is.
func nativeCompactBoundary(mode nativeCompactMode, historyLen int) int {
	if mode == nativeCompactManual {
		return historyLen + 2
	}
	return historyLen
}

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
