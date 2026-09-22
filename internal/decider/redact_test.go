package decider

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactMasksCommonSecrets(t *testing.T) {
	in := strings.Join([]string{
		"OPENROUTER_API_KEY=sk-or-v1-0000aaaa1111bbbb2222cccc3333dddd",
		"curl -H 'Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123' https://x",
		"git clone https://bilal:hunter2secret@github.com/x/y",
		"token: ghp_0123456789abcdefghijABCDEFGHIJ",
		`{"password": "p@ss word"}`,
		"aws AKIAABCDEFGHIJKLMNOP",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----",
	}, "\n")
	out := Redact(in)
	for _, leak := range []string{"0000aaaa", "abcdefghijklmnopqrstuvwxyz0123", "hunter2secret", "ghp_0123", "p@ss word", "AKIAABCDEFGHIJKLMNOP", "MIIE"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q survived redaction:\n%s", leak, out)
		}
	}
	// Ordinary text is left alone.
	plain := "git push --force origin main && go test ./... # token budget is fine"
	if got := Redact(plain); got != plain {
		t.Errorf("plain text changed: %q", got)
	}
}

func TestRedactValueWalksStructures(t *testing.T) {
	v := redactValue(map[string]any{
		"cmd":  "export API_KEY=sk-abcdefghijklmnopqrstuv",
		"list": []any{"sk-abcdefghijklmnopqrstuv", 3},
	}).(map[string]any)
	if strings.Contains(v["cmd"].(string), "sk-abc") || strings.Contains(v["list"].([]any)[0].(string), "sk-abc") {
		t.Errorf("nested secret survived: %v", v)
	}
	if v["list"].([]any)[1] != 3 {
		t.Error("non-string value changed")
	}
}

func TestTrimMiddleKeepsHeadTailAndUTF8(t *testing.T) {
	s := strings.Repeat("ğüşiöç", 2000) // 2-byte runes
	out := TrimMiddle(s, 1000)
	if len(out) > 1000 {
		t.Errorf("len = %d, want ≤ 1000", len(out))
	}
	if !utf8.ValidString(out) {
		t.Error("trim split a rune")
	}
	if !strings.Contains(out, "[trimmed]") {
		t.Error("no trim marker")
	}
	if TrimMiddle("short", 1000) != "short" {
		t.Error("short text changed")
	}
}
