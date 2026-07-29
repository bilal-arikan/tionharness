package tools

import "context"

// ShellOptimization records that an external token optimizer shrank a shell
// tool's output before it re-entered the model's context. The chat UI renders it
// as a small chip on the tool card ("sqz −71%"), so the compression is VISIBLE
// instead of silently changing what the agent read.
//
// Kind is the optimizer that acted:
//   - "sqz" — the in-process output filter (see agent.sqzShellFilter). It reports
//     exact token counts on stderr, so InTokens/OutTokens are populated.
//   - "rtk" — the agent (or a PreToolUse hook) wrapped the COMMAND itself in the
//     `rtk` proxy. rtk shrinks the output before we ever see it, so there is no
//     before/after pair to measure: the counts stay 0 and the chip shows the
//     optimizer name only. Reporting a fake percentage would be worse than none.
type ShellOptimization struct {
	Kind      string `json:"kind"`
	InTokens  int    `json:"inTokens,omitempty"`
	OutTokens int    `json:"outTokens,omitempty"`
	// Dedup marks sqz's strongest and most surprising mode: the command produced
	// output BYTE-IDENTICAL to something it compressed before, so it replaced the
	// whole result with a back-reference (`§ref:<hash>§`) instead of the text. The
	// saving is near-total but it reports no token pair, and the tool card would
	// otherwise show a one-line result for a command that printed thousands —
	// which reads exactly like a broken tool. Flagged so the chip can say so.
	Dedup bool `json:"dedup,omitempty"`
	// Command is the REWRITTEN command, set only when an optimizer changed what
	// actually ran (rtk turns `go test -v ./...` into `go test -json ./...`).
	// Surfaced so a silent command substitution never happens behind the user's
	// back — they must be able to see what was executed on their machine.
	Command string `json:"command,omitempty"`
	// Degraded marks a rewritten command that FAILED. rtk reports a summary rather
	// than the raw output, and its summarizers can lose the real error — a broken
	// go.mod surfaces as "No tests found" (measured). The flag drives an explicit
	// note to the agent and a warning on the card, instead of a confident summary
	// of a failure nobody can see.
	Degraded bool `json:"degraded,omitempty"`
}

// Measured reports whether this record carries a real before/after token pair
// (sqz) rather than just the optimizer's name (rtk).
func (o ShellOptimization) Measured() bool {
	return o.InTokens > 0 && o.OutTokens > 0 && o.OutTokens < o.InTokens
}

// Percent is the reduction as a whole percentage (28 → 8 tokens = 71). 0 when
// unmeasured, so a caller can branch on it without a second check.
func (o ShellOptimization) Percent() int {
	if !o.Measured() {
		return 0
	}
	return (o.InTokens - o.OutTokens) * 100 / o.InTokens
}

// optimizerSink collects the optimization applied to the current tool call. One
// sink is attached per call, so it holds at most one record. Mirrors diffSink.
type optimizerSink struct{ last *ShellOptimization }

// Take returns the recorded optimization (if any) and clears it.
func (s *optimizerSink) Take() *ShellOptimization {
	if s == nil {
		return nil
	}
	o := s.last
	s.last = nil
	return o
}

type optimizerKey struct{}

// WithOptimizerSink attaches a fresh optimizer sink to ctx and returns both. The
// shell execution core calls recordOptimization to populate it; the caller reads
// it via Take after the tool returns. When no sink is attached (e.g. autonomous
// runs that don't render a trace) recordOptimization is a no-op.
func WithOptimizerSink(ctx context.Context) (context.Context, *optimizerSink) {
	s := &optimizerSink{}
	return context.WithValue(ctx, optimizerKey{}, s), s
}

// recordOptimization stores a record on the sink attached to ctx, if present. A
// later call overwrites an earlier one, so the MEASURED sqz record wins over the
// name-only rtk record when a command is both rtk-wrapped and sqz-compressed
// (see the call order in runShell).
func recordOptimization(ctx context.Context, o ShellOptimization) {
	if s, ok := ctx.Value(optimizerKey{}).(*optimizerSink); ok {
		s.last = &o
	}
}
