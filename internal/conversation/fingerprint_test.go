package conversation

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestCatalogFingerprintIsOrderInsensitiveAndContentSensitive(t *testing.T) {
	a := providers.ToolDef{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)}
	b := providers.ToolDef{Name: "bash", Description: "Run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`)}

	ab := CatalogFingerprint([]providers.ToolDef{a, b})
	ba := CatalogFingerprint([]providers.ToolDef{b, a})
	if ab != ba {
		t.Fatalf("order changed the fingerprint: %s vs %s", ab, ba)
	}
	if len(ab) != fingerprintHexLen {
		t.Fatalf("fingerprint length = %d, want %d", len(ab), fingerprintHexLen)
	}

	// A re-described tool is a different catalog.
	b2 := b
	b2.Description = "Run a shell command"
	if CatalogFingerprint([]providers.ToolDef{a, b2}) == ab {
		t.Fatal("description change did not change the fingerprint")
	}
	// A changed schema is a different catalog.
	b3 := b
	b3.InputSchema = json.RawMessage(`{"type":"object"}`)
	if CatalogFingerprint([]providers.ToolDef{a, b3}) == ab {
		t.Fatal("schema change did not change the fingerprint")
	}
	// Fewer tools is a different catalog.
	if CatalogFingerprint([]providers.ToolDef{a}) == ab {
		t.Fatal("dropping a tool did not change the fingerprint")
	}
	// Field boundaries are framed: name+description must not slide into each other.
	x := providers.ToolDef{Name: "ab", Description: "c"}
	y := providers.ToolDef{Name: "a", Description: "bc"}
	if CatalogFingerprint([]providers.ToolDef{x}) == CatalogFingerprint([]providers.ToolDef{y}) {
		t.Fatal("unframed fields collided")
	}
}

func TestPrefixFingerprintCoversModelSystemAndTools(t *testing.T) {
	defs := []providers.ToolDef{{Name: "bash", Description: "Run"}}
	base := PrefixFingerprint("claude-sonnet-5", "You are helpful.", defs)
	if base == PrefixFingerprint("claude-opus-5", "You are helpful.", defs) {
		t.Fatal("model did not change the prefix fingerprint")
	}
	if base == PrefixFingerprint("claude-sonnet-5", "You are terse.", defs) {
		t.Fatal("system text did not change the prefix fingerprint")
	}
	if base == PrefixFingerprint("claude-sonnet-5", "You are helpful.", nil) {
		t.Fatal("tool catalog did not change the prefix fingerprint")
	}
	if base != PrefixFingerprint("claude-sonnet-5", "You are helpful.", defs) {
		t.Fatal("fingerprint is not deterministic")
	}
}
