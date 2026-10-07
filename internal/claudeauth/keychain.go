package claudeauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// keychain.go — macOS Keychain storage of the claude-cli login.
//
// On macOS Claude Code keeps its OAuth credential in the login Keychain, not in
// <config dir>/.credentials.json: it reads the Keychain first and only falls back
// to the plaintext file when no Keychain item exists, and its own token refreshes
// rewrite the Keychain item. A file-only reader therefore sees a missing or stale
// login on a Mac (verified on a live host: the TionHarness claude-home had a
// Keychain item updated that day and no .credentials.json at all).
//
// Item naming, verified against a live macOS host:
//   - service "Claude Code-credentials" for the default config dir (~/.claude),
//   - service "Claude Code-credentials-<first 8 hex of sha256(config dir)>" when
//     CLAUDE_CONFIG_DIR points elsewhere (TionHarness's claude-home),
//   - account = the login user name.
// The secret is the same JSON document as .credentials.json ({"claudeAiOauth":…});
// `security -w` may print it hex-encoded, so both spellings are accepted.

// errNoKeychainItem means the Keychain holds no credential for the home.
var errNoKeychainItem = errors.New("no keychain credential")

// securityRunner runs /usr/bin/security with args and stdin; replaced in tests.
var securityRunner = func(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := proc.CommandContext(ctx, "/usr/bin/security", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("security %s: %w (%s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// keychainForced lets this package's tests exercise the Keychain paths (with a
// fake securityRunner) on any OS.
var keychainForced bool

// keychainActive reports whether the Keychain is consulted: on macOS, outside
// `go test` (tests of other packages must never touch the developer's real
// Keychain), unless TIONHARNESS_NO_KEYCHAIN=1.
func keychainActive() bool {
	if keychainForced {
		return true
	}
	return runtime.GOOS == "darwin" && !testing.Testing() && os.Getenv("TIONHARNESS_NO_KEYCHAIN") != "1"
}

// keychainService returns the Keychain service name Claude Code uses for a config
// dir. homeDir is hashed exactly as given (it is what CLAUDE_CONFIG_DIR carries).
func keychainService(homeDir string) string {
	const base = "Claude Code-credentials"
	if isDefaultClaudeDir(homeDir) {
		return base
	}
	sum := sha256.Sum256([]byte(homeDir))
	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

func isDefaultClaudeDir(homeDir string) bool {
	uh, err := os.UserHomeDir()
	return err == nil && uh != "" && filepath.Clean(homeDir) == filepath.Join(uh, ".claude")
}

func keychainAccount() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// readKeychainRaw returns the credential JSON stored for homeDir, or
// errNoKeychainItem when there is none.
func readKeychainRaw(homeDir string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := securityRunner(ctx, nil, "find-generic-password", "-a", keychainAccount(), "-s", keychainService(homeDir), "-w")
	if err != nil {
		// security exits 44 for "item could not be found"; any failure reads as
		// "no item" so the plaintext file stays the fallback.
		return nil, errNoKeychainItem
	}
	return decodeKeychainSecret(out)
}

// decodeKeychainSecret accepts the secret as JSON or as the hex spelling of it.
func decodeKeychainSecret(out []byte) ([]byte, error) {
	s := bytes.TrimSpace(out)
	if len(s) == 0 {
		return nil, errNoKeychainItem
	}
	if s[0] == '{' {
		return s, nil
	}
	if b, err := hex.DecodeString(string(s)); err == nil && len(b) > 0 && bytes.TrimSpace(b)[0] == '{' {
		return bytes.TrimSpace(b), nil
	}
	return nil, fmt.Errorf("keychain credential is neither JSON nor hex JSON")
}

// writeKeychainRaw stores data as homeDir's credential, replacing any existing
// item (-U). The secret goes through `security -i` on stdin as hex (-X) so it
// never appears in a process argument list.
func writeKeychainRaw(homeDir string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	line := fmt.Sprintf("add-generic-password -U -a %s -s %s -X %s\n",
		securityQuote(keychainAccount()), securityQuote(keychainService(homeDir)), hex.EncodeToString(data))
	_, err := securityRunner(ctx, []byte(line), "-i")
	return err
}

// securityQuote double-quotes a value for `security -i`'s command parser.
func securityQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// KeychainActive reports whether credentials are read from and written to the
// macOS Keychain (see keychainActive). Callers that copy credential FILES between
// homes must switch to ReadCredentialsRaw/WriteCredentialsRaw when it is true.
func KeychainActive() bool { return keychainActive() }
