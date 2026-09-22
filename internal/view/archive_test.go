package view

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// The Map hides archived agents, artifacts, automations and skills the way the
// board hides archived cards: they are put away, not part of the live
// structure. Live siblings stay.
func TestGraphHidesArchivedEntities(t *testing.T) {
	store := &fakeStore{
		agents: []db.Agent{
			{ID: "AGT1", Name: "live"},
			{ID: "AGT2", Name: "shelved", Archived: true},
		},
		artifacts: []db.Artifact{
			{ID: "ART1", Title: "live"},
			{ID: "ART2", Title: "shelved", Archived: true},
		},
		automations: []db.Automation{
			{ID: "AUT1", Name: "live"},
			{ID: "AUT2", Name: "shelved", Archived: true},
		},
	}
	p := NewProjector(store)
	p.WithSources(Sources{Skills: fakeSkillsSource{catalog: []skills.Skill{
		{Slug: "live-skill", Name: "Live"},
		{Slug: "shelved-skill", Name: "Shelved", Archived: true},
	}}})

	nodes, _, err := p.GraphStructure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	present := map[Ref]bool{}
	for _, n := range nodes {
		present[n.Ref] = true
	}
	for _, ref := range []Ref{
		{Kind: KindAgent, ID: "AGT1"}, {Kind: KindArtifact, ID: "ART1"},
		{Kind: KindAutomation, ID: "AUT1"}, {Kind: KindSkill, ID: "live-skill"},
	} {
		if !present[ref] {
			t.Errorf("live node %v missing from the map", ref)
		}
	}
	for _, ref := range []Ref{
		{Kind: KindAgent, ID: "AGT2"}, {Kind: KindArtifact, ID: "ART2"},
		{Kind: KindAutomation, ID: "AUT2"}, {Kind: KindSkill, ID: "shelved-skill"},
	} {
		if present[ref] {
			t.Errorf("archived node %v must be hidden from the map", ref)
		}
	}
}
