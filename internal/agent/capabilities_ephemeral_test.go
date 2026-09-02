package agent

import "testing"

// A per-task worktree or a session scratchpad is a throwaway COPY of a repo the
// parent index already covers. Indexing one mints a fresh multi-hundred-MB store
// that is discarded with the task; left unchecked this grew the shared cache to
// 16GB and pushed the server's initialize past the pool's dial deadline.
func TestIsEphemeralWorkdir(t *testing.T) {
	ephemeral := []string{
		`C:\Users\user\Desktop\Projects\.tionharness-worktrees\WS5\tsk760`,
		"/c/Users/user/Desktop/Projects/.tionharness-worktrees/WS27/tsk115",
		`C:\Users\user\.tionharness\workspaces\WS19\store\sessions\SES704\scratchpad`,
		"/home/u/ws/sessions/SES1/scratchpad/tsk213-isolated",
	}
	for _, cwd := range ephemeral {
		if !isEphemeralWorkdir(cwd) {
			t.Errorf("isEphemeralWorkdir(%q) = false, want true", cwd)
		}
	}

	// A real repo must still be indexed -- including one whose own name merely
	// CONTAINS a marker word, which a substring match would wrongly exclude.
	real := []string{
		`C:\Users\user\Desktop\Projects\TionHarness`,
		"/c/Users/user/Desktop/Projects/SampleRepo",
		`C:\Users\user\Desktop\Projects\my-scratchpad-tool`,
		"/home/u/scratchpadding",
		"",
	}
	for _, cwd := range real {
		if isEphemeralWorkdir(cwd) {
			t.Errorf("isEphemeralWorkdir(%q) = true, want false", cwd)
		}
	}
}
