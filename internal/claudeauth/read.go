package claudeauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadCredentials loads <homeDir>/.credentials.json — the file Claude Code keeps
// its subscription login in — and returns the OAuth credential it holds.
//
// Read-only counterpart of WriteCredentials. Errors are returned rather than
// folded into a zero Credential: "file missing" (never logged in) and "malformed
// JSON" (corrupt home) are different problems and the caller should be able to
// tell them apart.
func ReadCredentials(homeDir string) (Credential, error) {
	if strings.TrimSpace(homeDir) == "" {
		return Credential{}, fmt.Errorf("empty claude-home dir")
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".credentials.json"))
	if err != nil {
		return Credential{}, err
	}
	var f credentialsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Credential{}, fmt.Errorf("parse credentials: %w", err)
	}
	return f.ClaudeAiOauth, nil
}
