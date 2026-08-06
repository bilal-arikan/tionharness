package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func TestProjectAutomationRendersTriggerAndState(t *testing.T) {
	now := time.Now()
	v, err := ProjectAutomation(AutomationInput{
		Automation: db.Automation{
			ID: "AUT1", Name: "todo→review", TriggerKind: db.TriggerBoard,
			BoardOp: db.BoardOpMove, BoardToState: db.BoardInProgress,
			TargetAgentID: "AG2", Enabled: true, IterationCount: 12,
			LastFiredAt:   now.Add(-3 * time.Hour).Unix(),
			MaxIterations: 50, CooldownSec: 60,
		},
		Now: now,
	}, LevelFull, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{
		"AUTOMATION · AUT1", "todo→review",
		"tetik: board (move) to:in_progress",
		"hedef: agent:AG2",
		"● aktif", "12 ateşleme", "3sa önce",
		"guardrail: max 50 ateşleme · 60 sn soğuma",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestProjectAutomationSurfacesLastErrorAndDefaults(t *testing.T) {
	now := time.Now()
	// A tag automation with no explicit op/columns renders its defaults: the
	// empty trigger kind resolves to tag, the empty board op resolves to move.
	v, err := ProjectAutomation(AutomationInput{
		Automation: db.Automation{
			ID: "AUT2", Name: "failed kart", TriggerTag: "error",
			LastError: "provider 429",
		},
		Now: now,
	}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{"tetik: tag:error", "hata: provider 429", "○ kapalı", "hiç ateşlenmedi"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestProjectAutomationRejectsEmptyID(t *testing.T) {
	if _, err := ProjectAutomation(AutomationInput{Automation: db.Automation{}}, LevelCard, LensHealth); err == nil {
		t.Error("an automation with no id must be an error, not a blank card")
	}
}
