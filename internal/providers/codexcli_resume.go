package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const codexResumeHomesDir = "resume-homes"

// ResumeScopeReady is the home safety gate used by API orchestration. Ambient
// CODEX_HOME cannot provide a deterministic, isolated home, so it fails closed.
func (c *CodexCLI) ResumeScopeReady(scope string) bool {
	return strings.TrimSpace(c.configDir) != "" && strings.TrimSpace(scope) != ""
}

// CanResumeScoped verifies that the stored thread belongs to this exact
// session/persona scope. A thread found in another Codex home is deliberately
// unusable: resuming it could continue the wrong persona or provider instance.
func (c *CodexCLI) CanResumeScoped(scope, sessionID string) bool {
	if !c.ResumeScopeReady(scope) || !safeCodexThreadID(sessionID) {
		return false
	}
	home := codexResumeHome(c.configDir, scope)
	return codexThreadExists(home, sessionID)
}

func codexResumeHome(base, scope string) string {
	sum := sha256.Sum256([]byte(scope))
	return filepath.Join(base, codexResumeHomesDir, hex.EncodeToString(sum[:]))
}

// prepareCodexTurnHome preserves the existing disposable shadow home for every
// non-chat auxiliary call. A scoped chat turn gets a durable, isolated home so
// Codex can persist and later resume its rollout without config.toml races with
// another session.
func prepareCodexTurnHome(base, scope string) (dir string, cleanup func(), err error) {
	if strings.TrimSpace(scope) == "" {
		return prepareShadowHome(base)
	}
	if strings.TrimSpace(base) == "" {
		return "", func() {}, errors.New("codex CLI: scoped resume requires a configured CODEX_HOME")
	}
	dir = codexResumeHome(base, scope)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", func() {}, err
	}
	for _, name := range []string{"auth.json", "models_cache.json", "installation_id"} {
		if err := seedCodexResumeFile(base, dir, name); err != nil {
			return "", func() {}, err
		}
	}
	return dir, func() {}, nil
}

func seedCodexResumeFile(base, dir, name string) error {
	_, err := os.Stat(filepath.Join(dir, name))
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return copyShadowHomeFile(base, dir, name)
}

func safeCodexThreadID(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && !strings.ContainsAny(id, `/\\`)
}

func codexThreadExists(home, sessionID string) bool {
	sessions := filepath.Join(home, "sessions")
	wantSuffix := "-" + strings.TrimSpace(sessionID) + ".jsonl"
	found := errors.New("codex thread found")
	err := filepath.WalkDir(sessions, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), wantSuffix) {
			return found
		}
		return nil
	})
	return errors.Is(err, found)
}

var _ ScopedCLIResumer = (*CodexCLI)(nil)
var _ CLICompactionLifecycle = (*CodexCLI)(nil)
