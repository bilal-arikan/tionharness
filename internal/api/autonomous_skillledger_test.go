package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestAutonomousBridgeSharesChatSkillLedger(t *testing.T) {
	s, rt := newAutonomousTestServer(t)
	ledger := tools.NewSkillLedger()
	ledger.Note("doctrine", 2)
	ctx := tools.WithSkillLedger(context.Background(), ledger, 2)
	_, done := s.autonomousInteraction(rt)(ctx, db.Agent{ID: "AG1", Provider: "codex-cli"}, "SES1")
	defer done()
	got, epoch := activeRun(t, s).skillLedgerFor()
	if got != ledger || epoch != 2 {
		t.Fatal("autonomous CLI bridge lost the shared skill ledger")
	}
}
