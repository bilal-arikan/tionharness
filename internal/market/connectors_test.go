package market

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeSkillsMPSearch(t *testing.T) {
	// Search API shape: {success, data:{skills:[...]}}.
	wrapped := `{"success":true,"data":{"skills":[{"id":"a","name":"A","githubUrl":"https://github.com/o/r/tree/main/skills/a"}]}}`
	items := decodeSkillsMPSearch([]byte(wrapped))
	if len(items) != 1 || items[0].Name != "A" {
		t.Fatalf("data.skills decode: %+v", items)
	}
	// Bare array fallback.
	items = decodeSkillsMPSearch([]byte(`[{"id":"b","name":"B","githubUrl":"https://github.com/o/r"}]`))
	if len(items) != 1 || items[0].Name != "B" {
		t.Fatalf("bare array decode: %+v", items)
	}
}

func TestSkillsMPEntriesMapping(t *testing.T) {
	entries := skillsmpEntries([]skillsmpItem{
		{ID: "x", Name: "Doc", Author: "u", Description: "d", GithubURL: "https://github.com/u/r/tree/main/skills/x"},
		{ID: "y", Name: "NoGH", GithubURL: ""}, // skipped
	})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Kind != KindSkill || e.Source == nil || e.Source.Type != "github" || e.ID == "" {
		t.Errorf("bad entry: %+v", e)
	}
}

func TestFilterCrossAIToolsSortsAndMaps(t *testing.T) {
	items := []crossaitoolsItem{
		{ID: "a", Name: "alpha seo", Repo: "o/r", Path: "a", Stars: 5},
		{ID: "b", Name: "beta seo", Repo: "o/r", Path: "b", Stars: 50},
		{ID: "c", Name: "gamma", Repo: "o/r", Path: "", Stars: 99}, // no "seo" match
	}
	got := filterCrossAITools(items, "seo", 10)
	if len(got) != 2 {
		t.Fatalf("want 2 matches for 'seo', got %d", len(got))
	}
	// Sorted by stars desc → beta(50) before alpha(5).
	if got[0].Name != "beta seo" || got[1].Name != "alpha seo" {
		t.Errorf("order wrong: %s, %s", got[0].Name, got[1].Name)
	}
	if got[0].Source.URL != "https://github.com/o/r/tree/main/b" {
		t.Errorf("tree url: %q", got[0].Source.URL)
	}
	// Limit cap.
	if len(filterCrossAITools(items, "", 1)) != 1 {
		t.Errorf("limit not applied")
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
	p, ok := s.Get("skill.x")
	if !ok {
		t.Fatal("source-ref pack not resolvable")
	}
	if p.SourceRef == nil || p.SourceRef.URL != "owner/repo" {
		t.Errorf("SourceRef not carried: %+v", p.SourceRef)
	}
}
