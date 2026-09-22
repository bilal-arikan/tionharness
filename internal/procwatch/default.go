package procwatch

import (
	"context"
	"errors"
	"os/exec"
)

// std is the process-wide ledger. A default instance is the one concession this
// package makes to convenience, and it is a deliberate one: the spawn sites are
// scattered across layers that have no wiring path to each other (a stdio MCP
// client, a provider transport, a shell tool), and threading a registry through
// all of them would add a parameter to constructors that otherwise have no
// business knowing about observability. Logging solved the same problem the
// same way.
//
// Tests that assert on the ledger construct their own Registry with New and
// call its methods directly, so they never race with the default one.
var std = New(DefaultHistory)

// Default returns the process-wide ledger — what the API serves and what the
// instrumented spawn sites record into.
func Default() *Registry { return std }

// Begin registers a process on the default ledger. See Registry.Begin.
func Begin(ctx context.Context, m Meta) *Handle { return std.Begin(ctx, m) }

// TrackRun is cmd.Run() with a ledger entry around it: one line at a call site
// that only wants the process to be visible, with no handle to hold.
//
// The pid is read AFTER the run, so a short probe appears in the panel with its
// pid and its outcome together. A site whose process is long-lived enough that
// the pid matters WHILE it runs (a shell, a CLI transport, an MCP server) uses
// Begin/Started/Finish directly instead.
func TrackRun(ctx context.Context, m Meta, cmd *exec.Cmd) error {
	h := std.Begin(ctx, m)
	err := cmd.Run()
	h.Started(cmd)
	h.Finish(err)
	return err
}

// TrackOutput is TrackRun for cmd.Output(): same bracketing, and the command's
// stderr (which Output attaches to the ExitError) becomes the entry's output
// tail, so a probe that failed explains itself in the panel.
func TrackOutput(ctx context.Context, m Meta, cmd *exec.Cmd) ([]byte, error) {
	h := std.Begin(ctx, m)
	out, err := cmd.Output()
	h.Started(cmd)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		h.AppendOutput(string(exitErr.Stderr))
	}
	h.Finish(err)
	return out, err
}
