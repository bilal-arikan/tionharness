package agent

import (
	"context"
	"os"
	"strings"
)

// workDirCtxKey carries the turn's resolved working directory (and whether the
// turn is autonomous) through the context, so buildRegistry roots the fs/shell
// sandbox at the session's working dir and decides whether to confine it.
type workDirCtxKey struct{}

// resolvedWorkDir is the per-turn working-directory decision.
type resolvedWorkDir struct {
	dir        string // absolute working directory for fs/shell tools
	autonomous bool   // true for scheduler/spawn/flow turns (no human in the loop)
}

// withResolvedWorkDir attaches the turn's working-directory decision to ctx.
func withResolvedWorkDir(ctx context.Context, dir string, autonomous bool) context.Context {
	return context.WithValue(ctx, workDirCtxKey{}, resolvedWorkDir{dir: dir, autonomous: autonomous})
}

// resolvedWorkDirFromCtx returns the turn's working-directory decision, if any.
func resolvedWorkDirFromCtx(ctx context.Context) (resolvedWorkDir, bool) {
	v, ok := ctx.Value(workDirCtxKey{}).(resolvedWorkDir)
	return v, ok
}

// effectiveWorkDir resolves the working directory for the current turn: the
// session's WorkingDir when set and pointing at a real directory, otherwise the
// workspace default (the configured default working dir, else the physical
// workspace dir). It reads the session id from the context, so it works on every
// path that stamps one (chat, scheduler, spawn, flow).
func (r *Runtime) effectiveWorkDir(ctx context.Context) string {
	if sid := SessionIDFrom(ctx); sid != "" {
		if s, err := r.db.GetSession(ctx, sid); err == nil {
			if d := strings.TrimSpace(s.WorkingDir); d != "" {
				if info, statErr := os.Stat(d); statErr == nil && info.IsDir() {
					return d
				}
			}
		}
	}
	return r.WorkspaceDefaultDir()
}
