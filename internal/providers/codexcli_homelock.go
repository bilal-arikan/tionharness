package providers

import (
	"context"
	"fmt"
	"sync"
)

// Codex turns now use turn-local shadow homes. This lock remains for paths that
// write directly to a shared base CODEX_HOME.
//
// Historical rationale: writeCodexConfig renders the turn's MCP servers and developer
// instructions into <CODEX_HOME>/config.toml — the home's REAL config, not a
// per-turn temp file — and PinCodexHome points every codex agent at the one
// app-global <dataDir>/codex-home. Two concurrent turns therefore write the
// same path, and the loser reads a config that belongs to the other turn. The
// observed failure: a turn whose MCP fallback had just rewritten config.toml
// WITHOUT a dead server still died on that server, because a second codex call
// had rewritten the file with the full server set in between.
//
// A directly used base home also holds auth.json, whose OAuth refresh token is single-use
// and rotated on refresh — overlapping subprocesses race it too (the reason
// applyInsightScanConcurrency pins codex scans to concurrency 1). Holding the
// lock across the whole write+run window closes both races at once.
//
// The lock is held for the DURATION OF THE SUBPROCESS, not just the write: a
// write-only lock would still let the next turn's write land before this
// turn's codex process reads the file.

var (
	codexHomeLocksMu sync.Mutex
	codexHomeLocks   = map[string]chan struct{}{}
)

// codexHomeLock returns the semaphore guarding a CODEX_HOME, creating it on
// first use. A 1-buffered channel rather than sync.Mutex because acquisition
// must be cancellable — see acquireCodexHome.
func codexHomeLock(dir string) chan struct{} {
	codexHomeLocksMu.Lock()
	defer codexHomeLocksMu.Unlock()
	lock, ok := codexHomeLocks[dir]
	if !ok {
		lock = make(chan struct{}, 1)
		codexHomeLocks[dir] = lock
	}
	return lock
}

// acquireCodexHome takes exclusive ownership of dir and returns the release
// func. It blocks while another turn is running against the same home.
//
// Cancellation is honoured while waiting: a turn whose context dies in the
// queue returns that error instead of holding up every later turn until its
// predecessor's subprocess finishes. An empty dir means the caller inherits the
// ambient ~/.codex and writes no config, so there is nothing to serialize.
func acquireCodexHome(ctx context.Context, dir string) (func(), error) {
	if dir == "" {
		return func() {}, nil
	}
	lock := codexHomeLock(dir)
	select {
	case lock <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-lock }) }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("codex CLI: waiting for CODEX_HOME %s: %w", dir, ctx.Err())
	}
}
