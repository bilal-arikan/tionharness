package providers

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// watchCLI registers an agentic-CLI subprocess (claude-cli, codex-cli) in the
// process ledger and returns its handle plus a context the caller must run the
// command under.
//
// The returned context is a cancellable child of ctx, and that cancel func is
// what the process panel's stop button calls. Killing a CLI transport this way
// takes the SAME path a user stop or the idle watchdog takes — the read loop
// sees ctx.Done, TreeKill reaps the tree, and the turn ends with an error — so
// the panel adds a button, not a new failure mode.
//
// The caller must call stop (typically `defer stop()`) exactly as it would a
// context.CancelFunc, and Finish the handle once the process is reaped.
func watchCLI(ctx context.Context, label, bin string, args []string) (context.Context, context.CancelFunc, *procwatch.Handle) {
	runCtx, cancel := context.WithCancel(ctx)
	h := procwatch.Default().Begin(runCtx, procwatch.Meta{
		Kind:    procwatch.KindProvider,
		Label:   label,
		Command: strings.TrimSpace(bin + " " + strings.Join(args, " ")),
		Stop:    cancel,
	})
	return runCtx, cancel, h
}
