package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Resume on the opencode transport rides opencode's OWN session store: every
// turn in a home shares one sqlite database under <home>/data, so `-s <id>`
// resumes any session that home has seen. There is no per-scope CODEX_HOME to
// isolate (codex needed that because its rollout files are the resume state);
// the scope only needs a durable record of which opencode session it last ran
// as, so a stored id that does not belong to the scope (home replaced, session
// deleted) is detected before a delta is sent. That record is one small JSON
// file per scope under <home>/scopes.

const opencodeScopesDir = "scopes"

type opencodeScopeRecord struct {
	SessionID string `json:"sessionID"`
}

func opencodeScopePath(home, scope string) string {
	sum := sha256.Sum256([]byte(scope))
	return filepath.Join(home, opencodeScopesDir, hex.EncodeToString(sum[:16])+".json")
}

// ResumeScopeReady implements ScopedCLIResumer: a home is required (the
// session database lives there) and the scope directory must be writable.
func (c *OpenCodeCLI) ResumeScopeReady(scope string) bool {
	if c.configDir == "" || strings.TrimSpace(scope) == "" {
		return false
	}
	return os.MkdirAll(filepath.Join(c.configDir, opencodeScopesDir), 0o700) == nil
}

// CanResumeScoped implements ScopedCLIResumer: the stored session id must be
// the one this scope last recorded AND the session row must still exist in
// opencode's store. The second check is cheap only through opencode itself,
// so it is skipped: a deleted session fails the resume turn with opencode's
// own error, which the caller treats like any cold start.
func (c *OpenCodeCLI) CanResumeScoped(scope, sessionID string) bool {
	if c.configDir == "" || sessionID == "" || !safeOpenCodeSessionID(sessionID) {
		return false
	}
	data, err := os.ReadFile(opencodeScopePath(c.configDir, scope))
	if err != nil {
		return false
	}
	var rec opencodeScopeRecord
	if json.Unmarshal(data, &rec) != nil {
		return false
	}
	return rec.SessionID == sessionID
}

// recordScopeSession stores the opencode session a scope last ran as. Errors
// are swallowed: a missing record only costs the next turn a cold start.
func (c *OpenCodeCLI) recordScopeSession(scope, sessionID string) {
	if c.configDir == "" || scope == "" || sessionID == "" {
		return
	}
	data, err := json.Marshal(opencodeScopeRecord{SessionID: sessionID})
	if err != nil {
		return
	}
	path := opencodeScopePath(c.configDir, scope)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

// safeOpenCodeSessionID accepts opencode's "ses_<alnum>" ids only, so a stored
// value can never smuggle an extra argument onto the command line.
func safeOpenCodeSessionID(id string) bool {
	if !strings.HasPrefix(id, "ses_") || len(id) > 80 {
		return false
	}
	for _, r := range id[4:] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return len(id) > 4
}
