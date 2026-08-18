package providers

import "testing"

// TestManifestFieldsWellFormed checks every registered kind's Manifest against
// the invariants the Faz 1A field schema promises: a valid Transport, a
// non-empty field list with unique keys, every Secret field typed "password",
// and every "select" field carrying options.
func TestManifestFieldsWellFormed(t *testing.T) {
	for _, k := range Kinds() {
		m := k.Manifest()
		t.Run(m.Kind, func(t *testing.T) {
			if m.Transport != TransportAPI && m.Transport != TransportCLI {
				t.Errorf("Transport = %q, want %q or %q", m.Transport, TransportAPI, TransportCLI)
			}
			if len(m.Fields) == 0 {
				t.Fatal("Fields is empty")
			}
			seen := make(map[string]bool, len(m.Fields))
			for _, f := range m.Fields {
				if seen[f.Key] {
					t.Errorf("duplicate field key %q", f.Key)
				}
				seen[f.Key] = true
				if f.Secret && f.Type != "password" {
					t.Errorf("field %q: Secret=true but Type=%q, want \"password\"", f.Key, f.Type)
				}
				if f.Type == "select" && len(f.Options) == 0 {
					t.Errorf("field %q: Type=\"select\" but Options is empty", f.Key)
				}
			}
		})
	}
}

// TestManifestKeyFieldMatchesNeedsKey verifies every kind whose Manifest.NeedsKey
// is true declares a required, secret "key" field — the field schema must agree
// with the pre-existing availability flag, not silently diverge from it.
func TestManifestKeyFieldMatchesNeedsKey(t *testing.T) {
	for _, k := range Kinds() {
		m := k.Manifest()
		if !m.NeedsKey {
			continue
		}
		t.Run(m.Kind, func(t *testing.T) {
			f, ok := m.FieldByKey(FieldKeyAPIKey)
			if !ok {
				t.Fatalf("NeedsKey=true but no %q field declared", FieldKeyAPIKey)
			}
			if !f.Required {
				t.Error("key field is not Required")
			}
			if !f.Secret {
				t.Error("key field is not Secret")
			}
		})
	}
}

// TestManifestCLIPathFieldForCLITransport verifies every CLI-transport kind
// declares a "cliPath" field, since the CLI transport is only usable once the
// binary is located.
func TestManifestCLIPathFieldForCLITransport(t *testing.T) {
	for _, k := range Kinds() {
		m := k.Manifest()
		if m.Transport != TransportCLI {
			continue
		}
		t.Run(m.Kind, func(t *testing.T) {
			if _, ok := m.FieldByKey(FieldKeyCLIPath); !ok {
				t.Errorf("Transport=%q but no %q field declared", TransportCLI, FieldKeyCLIPath)
			}
		})
	}
}

// TestTransportOfUnknownKind verifies TransportOf returns "" for an
// unregistered kind id instead of panicking or guessing.
func TestTransportOfUnknownKind(t *testing.T) {
	if got := TransportOf("does-not-exist"); got != "" {
		t.Errorf("TransportOf(unknown) = %q, want \"\"", got)
	}
}
