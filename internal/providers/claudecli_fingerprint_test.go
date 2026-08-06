package providers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPersistentFingerprintHashesConfigContent locks the Doc 52 §3-D fix: the
// persistent-session fingerprint tracks MCP-config CONTENT, not its temp path. Two
// turns that write byte-identical config to DIFFERENT temp paths must produce the SAME
// fingerprint (warm reuse); a content change must produce a DIFFERENT one (cold restart).
func TestPersistentFingerprintHashesConfigContent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	const cfg = `{"mcpServers":{"tionswarm_extended":{"type":"http","url":"http://x/extended"}}}`
	pathA := write("tionswarm-mcp-A.json", cfg)
	pathB := write("tionswarm-mcp-B.json", cfg)     // same content, different path (per-turn churn)
	pathC := write("tionswarm-mcp-C.json", cfg+" ") // different content

	req := Request{Model: "claude-fable-5", PermissionMode: "ask"}
	sys := "system prefix"

	fpFor := func(path string) string {
		c := &ClaudeCLI{model: "claude-fable-5", mcpConfigPath: path}
		return c.persistentFingerprint(req, sys)
	}

	if a, b := fpFor(pathA), fpFor(pathB); a != b {
		t.Errorf("identical config content at different paths must share a fingerprint (warm reuse):\n A=%s\n B=%s", a, b)
	}
	if a, c := fpFor(pathA), fpFor(pathC); a == c {
		t.Errorf("different config content must produce different fingerprints (cold restart), both %s", a)
	}
}
