package claudeauth

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKeychain stands in for /usr/bin/security: an in-memory service→secret map.
type fakeKeychain struct{ items map[string][]byte }

func useFakeKeychain(t *testing.T) *fakeKeychain {
	t.Helper()
	fk := &fakeKeychain{items: map[string][]byte{}}
	prevRunner, prevForced := securityRunner, keychainForced
	securityRunner = fk.run
	keychainForced = true
	t.Cleanup(func() { securityRunner, keychainForced = prevRunner, prevForced })
	return fk
}

func (fk *fakeKeychain) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	switch args[0] {
	case "find-generic-password":
		svc := argAfter(args, "-s")
		if v, ok := fk.items[svc]; ok {
			return append(v, '\n'), nil
		}
		return nil, errors.New("exit status 44")
	case "-i":
		line := string(stdin)
		svc := between(line, `-s "`, `"`)
		data, err := hex.DecodeString(strings.TrimSpace(line[strings.Index(line, "-X ")+3:]))
		if err != nil {
			return nil, err
		}
		fk.items[svc] = data
		return nil, nil
	}
	return nil, errors.New("unexpected security call")
}

func argAfter(args []string, flag string) string {
	for i := range args[:len(args)-1] {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	return s[:strings.Index(s, end)]
}

// The service name must match what Claude Code itself uses, verified on a live
// macOS host: sha256("/Users/monster/.tionharness/claude-home")[:8] = bb0b9893.
func TestKeychainServiceName(t *testing.T) {
	if got, want := keychainService("/Users/monster/.tionharness/claude-home"), "Claude Code-credentials-bb0b9893"; got != want {
		t.Errorf("keychainService = %q, want %q", got, want)
	}
	if uh, err := os.UserHomeDir(); err == nil && uh != "" {
		if got := keychainService(filepath.Join(uh, ".claude")); got != "Claude Code-credentials" {
			t.Errorf("default config dir service = %q, want the unsuffixed name", got)
		}
	}
}

// The Keychain item wins over the plaintext file — the CLI reads it first, so the
// file may be stale (or absent) while the login is live.
func TestReadCredentialsPrefersKeychain(t *testing.T) {
	fk := useFakeKeychain(t)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"stale","expiresAt":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fk.items[keychainService(home)] = []byte(`{"claudeAiOauth":{"accessToken":"live","expiresAt":2,"subscriptionType":"max"}}`)

	cred, err := ReadCredentials(home)
	if err != nil || cred.AccessToken != "live" || cred.SubscriptionType != "max" {
		t.Fatalf("ReadCredentials = %+v, %v; want the keychain credential", cred, err)
	}
}

func TestReadCredentialsFallsBackToFile(t *testing.T) {
	useFakeKeychain(t)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if cred, err := ReadCredentials(home); err != nil || cred.AccessToken != "file" {
		t.Fatalf("ReadCredentials = %+v, %v; want the file credential", cred, err)
	}
}

// `security -w` prints a secret it cannot render as text in hex.
func TestDecodeKeychainSecretHex(t *testing.T) {
	doc := `{"claudeAiOauth":{"accessToken":"x"}}`
	for _, in := range []string{doc + "\n", hex.EncodeToString([]byte(doc)) + "\n"} {
		got, err := decodeKeychainSecret([]byte(in))
		if err != nil || string(got) != doc {
			t.Errorf("decodeKeychainSecret(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := decodeKeychainSecret([]byte("zz-not-json")); err == nil {
		t.Error("garbage secret accepted")
	}
}

// A write lands in both the file and the Keychain, byte-identical, so a stale
// Keychain item can no longer shadow an in-app login.
func TestWriteCredentialsRawUpdatesKeychain(t *testing.T) {
	fk := useFakeKeychain(t)
	home := t.TempDir()
	doc := []byte(`{"claudeAiOauth":{"accessToken":"new","refreshTokenExpiresAt":99}}`)
	if err := WriteCredentialsRaw(home, doc); err != nil {
		t.Fatalf("WriteCredentialsRaw: %v", err)
	}
	if got := fk.items[keychainService(home)]; string(got) != string(doc) {
		t.Errorf("keychain item = %q, want %q", got, doc)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".credentials.json")); string(b) != string(doc) {
		t.Errorf("file = %q, want %q", b, doc)
	}
}

func TestSecurityQuote(t *testing.T) {
	if got := securityQuote(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Errorf("securityQuote = %s", got)
	}
}
