package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// repairSessions reconciles every stored claude-cli resume id with the config
// home the CLI now actually runs against (<dataDir>/claude-home).
//
// Claude Code keeps one transcript per conversation at
// <CLAUDE_CONFIG_DIR>/projects/<cwd-slug>/<session-id>.jsonl. When the config
// home moved from <workspace>/claude-home to the app-global one, every stored
// id kept naming a transcript that only exists in the old home, so the next
// turn died on "No conversation found with session ID" — and, because that
// failure looks retryable, burned its retries on the same dead id and left the
// session tagged `stuck`.
//
// Per session, in order of preference:
//   - transcript already in the new home  → leave the warm session alone;
//   - transcript still in the workspace's legacy home → COPY it into the new
//     home under the same project subdir, so the session keeps resuming warm;
//   - neither → clear the resume bookkeeping so the next turn starts cold with
//     the full transcript from TionHarness's own store (no history is lost — the
//     CLI's copy is a cache, not the record).
//
// stuckTurns / the `stuck` tag are cleared only on sessions this pass actually
// touched: that counter records exactly the turns this breakage killed, and a
// session left untouched here may be stuck for an unrelated reason.
func repairSessions(dataDir string, apply bool) error {
	roots, err := workspaceDirs(dataDir)
	if err != nil {
		return err
	}
	newHome := filepath.Join(dataDir, "claude-home")
	var relocated, cleared, warm int

	for _, root := range roots {
		ws := filepath.Base(root)
		legacyHome := filepath.Join(root, "claude-home")
		dir := filepath.Join(root, "store", "sessions")
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name(), "session.json")
			var session map[string]any
			if err := readJSONDoc(path, &session); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			cliID, _ := session["cliSessionId"].(string)
			if cliID == "" {
				continue
			}
			if findTranscript(newHome, cliID) != "" {
				warm++
				continue
			}
			id, _ := session["id"].(string)
			if src := findTranscript(legacyHome, cliID); src != "" {
				dst := filepath.Join(newHome, "projects", filepath.Base(filepath.Dir(src)), cliID+".jsonl")
				fmt.Printf("%s/%s relocate transcript → %s\n", ws, id, dst)
				relocated++
				if apply {
					if err := copyFile(src, dst); err != nil {
						return fmt.Errorf("relocate transcript for %s/%s: %w", ws, id, err)
					}
				}
			} else {
				fmt.Printf("%s/%s clear resume id %s (no transcript anywhere)\n", ws, id, cliID)
				delete(session, "cliSessionId")
				delete(session, "cliSentMsgCount")
				cleared++
			}
			clearStuck(session)
			if apply {
				if err := writeJSONDoc(path, session, false); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
			}
		}
	}
	fmt.Printf("sessions: %d already warm, %d transcripts relocated, %d resume ids cleared\n", warm, relocated, cleared)
	return nil
}

// clearStuck drops the failure bookkeeping this breakage produced: the
// consecutive-stuck-turn counter and the `stuck` tag that the threshold set.
// Other tags (including error/tool-error, which describe what the user saw)
// are left in place.
func clearStuck(session map[string]any) {
	delete(session, "stuckTurns")
	tags, ok := session["tags"].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(tags))
	for _, t := range tags {
		if s, _ := t.(string); s == "stuck" {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		delete(session, "tags")
		return
	}
	session["tags"] = kept
}

// findTranscript returns the path of <home>/projects/*/<id>.jsonl, or "" when
// this home does not hold that conversation. The project subdirectory is not
// derived from the session's working dir: the slug encoding is the CLI's own
// business and a session may have been started from a different cwd.
func findTranscript(home, id string) string {
	projects := filepath.Join(home, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(projects, e.Name(), id+".jsonl")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// copyFile copies src to dst, creating dst's directory. It refuses to overwrite
// an existing destination: the only way one exists here is a transcript the new
// home already had, which findTranscript would have found first — so hitting it
// means the two homes disagree, and clobbering would destroy the live copy.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
