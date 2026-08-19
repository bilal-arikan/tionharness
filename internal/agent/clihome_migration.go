package agent

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// MigrateSharedCLIHomes copies a legacy workspace login into the app-global home
// only when exactly one workspace has a login. Legacy homes are never removed.
func MigrateSharedCLIHomes(dataDir string, workspaceRoots []string, logger *slog.Logger) error {
	if dataDir == "" {
		return fmt.Errorf("migrate shared CLI homes: data dir is empty")
	}
	for _, provider := range []struct {
		name, home, auth string
	}{
		{name: "claude", home: "claude-home", auth: ".credentials.json"},
		{name: "codex", home: "codex-home", auth: "auth.json"},
	} {
		dst := filepath.Join(dataDir, provider.home)
		if fileHasContent(filepath.Join(dst, provider.auth)) {
			continue
		}
		empty, err := dirEmpty(dst)
		if err != nil {
			return fmt.Errorf("inspect app-global %s home: %w", provider.name, err)
		}
		if !empty {
			continue
		}
		var loggedIn []string
		for _, root := range workspaceRoots {
			home := filepath.Join(root, provider.home)
			if fileHasContent(filepath.Join(home, provider.auth)) {
				loggedIn = append(loggedIn, home)
			}
		}
		switch len(loggedIn) {
		case 0:
			continue
		case 1:
			if err := copyCLIHome(loggedIn[0], dst); err != nil {
				return fmt.Errorf("migrate %s login from %s: %w", provider.name, loggedIn[0], err)
			}
			if logger != nil {
				logger.Info("migrated workspace CLI login to app-global home", "provider", provider.name, "source", loggedIn[0], "destination", dst)
			}
		default:
			if logger != nil {
				logger.Warn("multiple workspace CLI logins found; app-global migration skipped", "provider", provider.name, "count", len(loggedIn))
			}
		}
	}
	return nil
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
