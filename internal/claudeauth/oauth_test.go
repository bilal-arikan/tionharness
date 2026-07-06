package claudeauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBeginBuildsValidPKCEAuthorizeURL(t *testing.T) {
	authURL, pending, err := Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authURL: %v", err)
	}
	q := u.Query()
	if got := q.Get("client_id"); got != clientID {
		t.Errorf("client_id = %q, want %q", got, clientID)
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q", q.Get("response_type"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	if q.Get("state") != pending.State {
		t.Errorf("state in URL (%q) != pending.State (%q)", q.Get("state"), pending.State)
	}
	// The challenge must be S256(verifier).
	sum := sha256.Sum256([]byte(pending.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if q.Get("code_challenge") != want {
		t.Errorf("code_challenge mismatch: got %q want %q", q.Get("code_challenge"), want)
	}
	if pending.Verifier == "" || pending.State == "" {
		t.Error("empty verifier/state")
	}
}

func TestSplitCodeState(t *testing.T) {
	cases := []struct{ in, code, state string }{
		{"abc#xyz", "abc", "xyz"},
		{"  abc#xyz  ", "abc", "xyz"},
		{"bareCode", "bareCode", ""},
		{"https://platform.claude.com/oauth/code/callback?code=AAA&state=BBB", "AAA", "BBB"},
	}
	for i, c := range cases {
		gc, gs := splitCodeState(c.in)
		if gc != c.code || gs != c.state {
			t.Errorf("[%d] splitCodeState(%q) = (%q,%q), want (%q,%q)", i, c.in, gc, gs, c.code, c.state)
		}
	}
}

func TestWriteCredentialsShapeAndBackup(t *testing.T) {
	dir := t.TempDir()
	// Seed an existing credential so we exercise the backup path.
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cred := Credential{AccessToken: "tok", RefreshToken: "ref", ExpiresAt: 1234, Scopes: []string{"user:inference"}, SubscriptionType: "max"}
	if err := WriteCredentials(dir, cred); err != nil {
		t.Fatalf("WriteCredentials: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("expected backup .bak, got: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got credentialsFile
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal written credentials: %v", err)
	}
	if got.ClaudeAiOauth.AccessToken != "tok" || got.ClaudeAiOauth.SubscriptionType != "max" {
		t.Errorf("written credential mismatch: %+v", got.ClaudeAiOauth)
	}
	if !strings.Contains(string(raw), "claudeAiOauth") {
		t.Error("credential file missing claudeAiOauth top-level key")
	}
}
