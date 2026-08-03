package api

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
)

// defaultGitignore is the starter ignore file written into a repository TionSwarm
// itself initialises. A fresh `git init` with no .gitignore is the single easiest
// way to commit a .env or a 200 MB build folder on the first commit, so the repo
// gets a language-agnostic baseline instead of nothing. It is a starting point,
// not a policy: the file is plain text in the user's repo and theirs to edit.
//
//go:embed defaults/gitignore.txt
var defaultGitignore string

// writeDefaultGitignore drops the starter .gitignore into dir.
//
// It NEVER overwrites: an existing .gitignore (a folder that already had one
// before being versioned) is left exactly as it is, and that is a success, not an
// error. Only a real write/stat failure is reported.
func writeDefaultGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil // already present — the user's file wins
	} else if !errors.Is(err, os.ErrNotExist) {
		return err // unreadable path: report rather than clobber blindly
	}
	return os.WriteFile(path, []byte(defaultGitignore), 0o644)
}
