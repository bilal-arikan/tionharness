package agent

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// MigrateSharedCLIHomes seeds the app-global CLI login home from a legacy
// per-workspace one, so switching to a shared home does not log the user out of
// every workspace at once. Legacy homes are never removed.
//
// It runs whenever the app-global home has no usable login, and picks the BEST
// candidate rather than requiring a unique one: with one home per workspace, a
// user who worked in several workspaces has many logins for the same account,
// and refusing to choose left the shared home unauthenticated — i.e. every agent
// failing with "claude-home is not logged in" (the exact state this found in the
// wild). "Best" means a live access token first, then the latest expiry
// (credentialLiveness), which is the same ranking the per-turn credential heal
// uses; for codex, whose auth.json carries no expiry TionSwarm can read, it means
// the most recently written file.
//
// Only the credential file is copied into a home that already exists: the rest
// of a CLI home is settings, caches and conversation transcripts that belong to
// the workspace that produced them. A wholly absent/empty destination is seeded
// with the full tree instead, so a first run inherits settings too.
func MigrateSharedCLIHomes(dataDir string, workspaceRoots []string, logger *slog.Logger) error {
	if dataDir == "" {
		return fmt.Errorf("migrate shared CLI homes: data dir is empty")
	}
	for _, provider := range []struct {
		name, home, auth string
		// usable reports whether an auth file at this path can actually
		// authenticate (claude can tell an expired/wiped credential apart; codex
		// only "is there content").
		usable func(path string) bool
		// better reports whether candidate a is a better source than b.
		better func(a, b string) bool
	}{
		{
			name: "claude", home: "claude-home", auth: ".credentials.json",
			usable: credentialUsable,
			better: func(a, b string) bool { return credentialLiveness(a).betterThan(credentialLiveness(b)) },
		},
		{
			name: "codex", home: "codex-home", auth: "auth.json",
			usable: fileHasContent,
			better: newerFile,
		},
	} {
		dst := filepath.Join(dataDir, provider.home)
		dstAuth := filepath.Join(dst, provider.auth)
		if provider.usable(dstAuth) {
			continue
		}

		best := ""
		for _, root := range workspaceRoots {
			candidate := filepath.Join(root, provider.home, provider.auth)
			if !provider.usable(candidate) {
				continue
			}
			if best == "" || provider.better(candidate, best) {
				best = candidate
			}
		}
		if best == "" {
			continue
		}

		empty, err := dirEmpty(dst)
		if err != nil {
			return fmt.Errorf("inspect app-global %s home: %w", provider.name, err)
		}
		if empty {
			if err := copyCLIHome(filepath.Dir(best), dst); err != nil {
				return fmt.Errorf("migrate %s login from %s: %w", provider.name, filepath.Dir(best), err)
			}
		} else if err := copyFile(best, dstAuth, 0o600); err != nil {
			return fmt.Errorf("migrate %s credential from %s: %w", provider.name, best, err)
		}
		if logger != nil {
			logger.Info("seeded app-global CLI login from a workspace home",
				"provider", provider.name, "source", best, "destination", dst, "full_home", empty)
		}
	}
	return nil
}

// newerFile reports whether a was modified more recently than b. An unreadable
// file is treated as older, so it never wins a comparison.
func newerFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return true
	}
	return ai.ModTime().After(bi.ModTime())
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func copyCLIHome(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}
