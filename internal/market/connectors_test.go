package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeSkillsMP(t *testing.T) {
	// Bare array.
	arr := `[{"id":"a","name":"A","githubUrl":"https://github.com/o/r/tree/main/skills/a"}]`
	items, err := decodeSkillsMP([]byte(arr))
	if err != nil || len(items) != 1 || items[0].Name != "A" {
		t.Fatalf("bare array decode: %v %+v", err, items)
	}
	// Wrapped under skills.
	wrapped := `{"skills":[{"id":"b","name":"B","githubUrl":"https://github.com/o/r"}]}`
	items, err = decodeSkillsMP([]byte(wrapped))
	if err != nil || len(items) != 1 || items[0].Name != "B" {
		t.Fatalf("wrapped decode: %v %+v", err, items)
	}
}

func TestFetchSkillsMPMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":"x","name":"Doc Hygiene","author":"phuongnse","description":"d","githubUrl":"https://github.com/phuongnse/axis/tree/main/.agents/skills/x"},
			{"id":"y","name":"No GH","description":"d","githubUrl":""}
		]`))
	}))
	defer srv.Close()
	entries, err := fetchSkillsMP(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// Only the GitHub-backed entry is mapped; the urlless one is skipped.
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Kind != KindSkill || e.Source == nil || e.Source.Type != "github" {
		t.Errorf("entry not a github source-ref: %+v", e)
	}
	if e.Source.URL == "" || e.ID == "" {
		t.Errorf("entry missing url/id: %+v", e)
	}
}

func TestFetchCrossAIToolsSortsCapsAndMaps(t *testing.T) {
	// Three listed + one unlisted + one repo-less; expect popularity order and skips.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":"a","name":"A","repo":"o/r","path":"a","stars":5,"installs":1,"listingStatus":"listed"},
			{"id":"b","name":"B","repo":"o/r","path":"b","stars":50,"installs":9,"listingStatus":"listed"},
			{"id":"c","name":"C","repo":"o/r","path":"","stars":10,"installs":1,"listingStatus":"listed"},
			{"id":"d","name":"D","repo":"o/r","path":"d","stars":99,"listingStatus":"unlisted"},
			{"id":"e","name":"E","repo":"","path":"e","stars":99,"listingStatus":"listed"}
		]`))
	}))
	defer srv.Close()
	entries, err := fetchCrossAITools(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// d (unlisted) and e (no repo) skipped → 3 entries, sorted by stars desc: B,C,A.
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Name != "B" || entries[1].Name != "C" || entries[2].Name != "A" {
		t.Errorf("popularity order wrong: %s,%s,%s", entries[0].Name, entries[1].Name, entries[2].Name)
	}
	// Root-path entry C → bare repo URL; others → tree URL.
	if entries[1].Source.URL != "https://github.com/o/r" {
		t.Errorf("root-path URL = %q", entries[1].Source.URL)
	}
	if entries[2].Source.URL != "https://github.com/o/r/tree/main/a" {
		t.Errorf("tree URL = %q", entries[2].Source.URL)
	}
	if entries[0].Kind != KindSkill || entries[0].Source.Type != "github" {
		t.Errorf("entry not github skill source-ref: %+v", entries[0])
	}
}

func TestGithubTreeURL(t *testing.T) {
	if got := githubTreeURL("o/r", "skills/x"); got != "https://github.com/o/r/tree/main/skills/x" {
		t.Errorf("tree url = %q", got)
	}
	if got := githubTreeURL("o/r", ""); got != "https://github.com/o/r" {
		t.Errorf("root url = %q", got)
	}
}

func TestSourceRefPackResolvesFromCache(t *testing.T) {
	global := t.TempDir()
	s := New(global, t.TempDir())
	if _, err := s.AddRegistry("t", "https://example.com/r.json"); err != nil {
		t.Fatal(err)
	}
	// Write a cached index containing a SOURCE-REF entry (no payload URL).
	ci := cachedIndex{
		RegistryName: "t", URL: "https://example.com/r.json",
		Index: RegistryIndex{Schema: RegistrySchemaV1, Name: "t", Packs: []RegistryEntry{
			{ID: "skill.x", Kind: KindSkill, Name: "X", Source: &SourceRef{Type: "github", URL: "owner/repo"}},
		}},
	}
	dir := s.remoteCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(ci)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName(ci.URL)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s.Reload()

	// The source-ref entry must be in the catalog and Get must return the manifest
	// (with SourceRef) WITHOUT trying to download a payload.
	p, ok := s.Get("skill.x")
	if !ok {
		t.Fatal("source-ref pack not resolvable")
	}
	if p.SourceRef == nil || p.SourceRef.URL != "owner/repo" {
		t.Errorf("SourceRef not carried: %+v", p.SourceRef)
	}
}
