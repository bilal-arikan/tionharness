package market

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPublishRejectsReservedIDs: a pack whose id collides with a literal segment
// under /api/market would be permanently unreachable, because Go's ServeMux
// gives the literal route precedence over GET /api/market/{id} — the caller
// would silently get the registry list instead of the pack. Publish must refuse
// it rather than write an unreadable pack to disk.
func TestPublishRejectsReservedIDs(t *testing.T) {
	st := New(t.TempDir(), t.TempDir())

	for _, id := range []string{"registries", "connectors", "reload", "publish", "import"} {
		t.Run(id, func(t *testing.T) {
			_, err := st.Publish(Pack{Schema: SchemaV1, ID: id, Kind: KindSkill, Name: id})
			if err == nil {
				t.Fatalf("Publish(id=%q) succeeded; want a reserved-id error", id)
			}
			if !strings.Contains(err.Error(), "reserved") {
				t.Fatalf("error = %v, want it to explain the id is reserved", err)
			}
			if _, ok := st.Get(id); ok {
				t.Fatalf("pack %q was stored despite the rejection", id)
			}
		})
	}

	t.Run("case and space insensitive", func(t *testing.T) {
		// safeFileName/route matching would still collide, so " Registries " must
		// not slip past a naive exact-match check.
		if _, err := st.Publish(Pack{Schema: SchemaV1, ID: " Registries ", Kind: KindSkill}); err == nil {
			t.Fatal("Publish(id=\" Registries \") succeeded; want rejection")
		}
	})

	t.Run("ordinary ids still publish", func(t *testing.T) {
		got, err := st.Publish(Pack{
			Schema: SchemaV1, ID: "skill.registry-helper", Kind: KindSkill,
			Name: "Registry Helper",
		})
		if err != nil {
			t.Fatalf("Publish of a non-reserved id failed: %v", err)
		}
		if got.ID != "skill.registry-helper" {
			t.Fatalf("id = %q, want it stored unchanged", got.ID)
		}
	})
}

// TestImportRejectsReservedIDs: Import delegates to Publish, so the guard must
// hold on the pasted-JSON path too — that is the one an outside caller uses.
func TestImportRejectsReservedIDs(t *testing.T) {
	st := New(t.TempDir(), t.TempDir())
	raw, err := json.Marshal(Pack{Schema: SchemaV1, ID: "connectors", Kind: KindSkill, Name: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := st.Import(raw); err == nil {
		t.Fatal("Import of a reserved id succeeded; want rejection")
	}
}
