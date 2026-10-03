package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

func TestMultiAgent_RepliesReceiveSkillCatalogAndCompactedSummary(t *testing.T) {
	prov := newScriptedProvider(sayText("Ada replied."), sayText("Bryn replied."))
	h := newHarness(t, prov)
	h.convo.SetLimits(150, 4)
	ada := h.newAgent("Ada")
	bryn := h.newAgent("Bryn")
	sess := h.newSession(ada)
	if _, err := h.rt.Skills().Create("shared-recipe", skills.SkillInput{
		Name: "Shared Recipe", Description: "Shared instructions.", Shared: true, Body: "# Recipe",
	}); err != nil {
		t.Fatal(err)
	}
	for i := range 24 {
		role := providers.RoleUser
		if i%2 == 1 {
			role = providers.RoleAssistant
		}
		if _, err := h.db.AddMessage(context.Background(), db.Message{
			SessionID: sess.ID, Role: role, Text: strings.Repeat("context ", 12),
		}); err != nil {
			t.Fatal(err)
		}
	}

	results := h.sendMulti([]db.Agent{ada, bryn}, sess, "Continue together.")
	if !results[0].prep.Compacted {
		t.Fatal("the first reply must compact the seeded backlog")
	}
	if !strings.Contains(prov.lastReq.System, "shared-recipe") {
		t.Fatalf("second agent did not receive the skill catalog: %q", prov.lastReq.System)
	}
	if !strings.Contains(prov.lastReq.SystemDynamic, prov.summaryReply) {
		t.Fatalf("second agent did not receive the compacted summary: %q", prov.lastReq.SystemDynamic)
	}
}
