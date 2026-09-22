package skills

import (
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/archive"
)

// Archiving a skill writes a frontmatter marker and takes the skill out of
// every agent-facing surface, while the catalog (Skills screen, REST) keeps it.
func TestSetArchivedHidesSkillFromAgents(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "keep", "---\nname: Keep\ndescription: stays\naccess: shared\n---\nbody\n")
	writeSkill(t, dir, "shelf", "---\nname: Shelf\ndescription: shelved\naccess: shared\n---\nshelf body\n")
	writeSkill(t, dir, "parent", "---\nname: Parent\ndescription: p\nsubskills: [shelf, keep]\n---\nparent body\n")
	s := New(dir, "")

	sk, err := s.SetArchived("shelf", true)
	if err != nil {
		t.Fatal(err)
	}
	if !sk.Archived {
		t.Fatal("SetArchived(true) did not mark the skill")
	}
	if got, ok := s.Get("shelf"); !ok || !got.Archived {
		t.Fatal("archived skill must stay resolvable for the Skills screen")
	}
	if len(s.List()) != 3 || len(s.ActiveList()) != 2 {
		t.Fatalf("List=%d ActiveList=%d, want 3 and 2", len(s.List()), len(s.ActiveList()))
	}

	for name, block := range map[string]string{
		"CatalogBlock":         s.CatalogBlock(),
		"CatalogBlockFor":      s.CatalogBlockFor([]string{"shelf", "keep"}),
		"CatalogBlockForAgent": s.CatalogBlockForAgent([]string{"shelf"}),
	} {
		if strings.Contains(block, "`shelf`") {
			t.Errorf("%s advertises the archived skill:\n%s", name, block)
		}
		if !strings.Contains(block, "`keep`") {
			t.Errorf("%s dropped the live skill:\n%s", name, block)
		}
	}
	for _, hit := range s.Search("shel", 0) {
		if hit.Slug == "shelf" {
			t.Error("skill_search must not return an archived skill")
		}
	}
	if _, err := s.UseSkillBody("shelf", nil); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("UseSkillBody err = %v, want ErrArchived", err)
	}
	if body, err := s.Body("shelf"); err != nil || !strings.Contains(body, "shelf body") {
		t.Fatalf("the raw body stays readable for the detail view: %q, %v", body, err)
	}
	parent, err := s.UseSkillBody("parent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(parent, "`shelf`") || !strings.Contains(parent, "`keep`") {
		t.Errorf("sub-skill footer must skip archived children:\n%s", parent)
	}

	sk, err = s.SetArchived("shelf", false)
	if err != nil || sk.Archived {
		t.Fatalf("unarchive: %+v, %v", sk, err)
	}
	if !strings.Contains(s.CatalogBlock(), "`shelf`") {
		t.Error("a restored skill must be advertised again")
	}
}

func TestResolveRecipeRefusesArchived(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "wf", "---\nname: WF\nkind: coordinator-workflow\npattern: fanout\narchived: true\n---\nsteps\n")
	if _, err := ResolveRecipe(New(dir, ""), "wf"); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("err = %v, want ErrArchived", err)
	}
}
