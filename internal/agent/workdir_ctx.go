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
	return r.SessionWorkdir(SessionIDFrom(ctx))
}

// SessionWorkdir resolves a session's working directory the same way the turn
// loop does (session override when valid, else workspace default), taking an
// explicit id instead of reading it from the context. Used by the CLI Interaction
// bridge to bind a progress sink before the turn context is built. Empty id → the
// workspace default.
func (r *Runtime) SessionWorkdir(sessionID string) string {
	if sessionID != "" {
		s, err := r.db.GetSession(context.Background(), sessionID)
		if err != nil {
			// The fallback stays (every caller needs a directory back), but it must
			// not be silent: a failed lookup moves the turn's fs/shell sandbox off
			// the session's own working dir with no other trace.
			r.logWorkdirFallback("session workdir: session lookup failed, using workspace default",
				"session", sessionID, "error", err)
		} else if d := strings.TrimSpace(s.WorkingDir); d != "" {
			info, statErr := os.Stat(d)
			switch {
			case statErr != nil:
				r.logWorkdirFallback("session workdir: stat failed, using workspace default",
					"session", sessionID, "dir", d, "error", statErr)
			case !info.IsDir():
				r.logWorkdirFallback("session workdir: path is not a directory, using workspace default",
					"session", sessionID, "dir", d)
			default:
				return d
			}
		}
	}
	return r.WorkspaceDefaultDir()
}

// logWorkdirFallback reports a working-directory fallback. Runtime values built
// without a logger (bare test fixtures) are common on this path, so the nil check
// lives here instead of at every call site.
func (r *Runtime) logWorkdirFallback(msg string, args ...any) {
	if r.logger == nil {
		return
	}
	r.logger.Warn(msg, args...)
}
