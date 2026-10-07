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
//
// On macOS the login Keychain is consulted first, exactly like the CLI does (see
// keychain.go); the file is the fallback.
func ReadCredentials(homeDir string) (Credential, error) {
	data, err := ReadCredentialsRaw(homeDir)
	if err != nil {
		return Credential{}, err
	}
	var f credentialsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Credential{}, fmt.Errorf("parse credentials: %w", err)
	}
	return f.ClaudeAiOauth, nil
}

// ReadCredentialsRaw returns the raw credential document the claude CLI would use
// for homeDir: the macOS Keychain item when one exists, else
// <homeDir>/.credentials.json. Raw bytes keep fields this package does not model
// (refreshTokenExpiresAt, rateLimitTier…) intact for callers that copy them.
func ReadCredentialsRaw(homeDir string) ([]byte, error) {
	if strings.TrimSpace(homeDir) == "" {
		return nil, fmt.Errorf("empty claude-home dir")
	}
	if keychainActive() {
		if data, err := readKeychainRaw(homeDir); err == nil {
			return data, nil
		}
	}
	return os.ReadFile(filepath.Join(homeDir, ".credentials.json"))
}
