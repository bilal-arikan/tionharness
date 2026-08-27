package view

import (
	"strings"
	"testing"
	"time"
)

func TestProjectInsightRendersFinding(t *testing.T) {
	now := time.Now()
	v, err := ProjectInsight(InsightInput{
		Finding: InsightFinding{
			ID: "FND1", LensID: "errors", Channel: "app-fix",
			Title: "provider 429 döngüsü", RootCause: "rate limit", ProposedFix: "backoff ekle",
			Severity: "high", Status: "new", Occurrences: 3,
			EvidenceSessionIDs: []string{"SES1", "SES2"},
			LastSeen:           now.Add(-1 * time.Hour).Unix(),
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{
		"INSIGHT · FND1", "[high]", "provider 429 döngüsü",
		"durum: new", "kanal: app-fix", "lens: errors", "3 oluşum", "1sa önce",
		"kanıt: SES1, SES2", "kök neden: rate limit", "öneri: backoff ekle",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestProjectInsightMarksRegression(t *testing.T) {
	now := time.Now()
	v, err := ProjectInsight(InsightInput{
		Finding: InsightFinding{
			ID: "FND2", Title: "geri dönen ders", Status: "applied",
			Regressed: true, Occurrences: 5, LastSeen: now.Unix(),
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "durum: regressed") {
		t.Errorf("a regressed finding must read as regressed:\n%s", v.Text())
	}
}

func TestProjectInsightRejectsEmptyID(t *testing.T) {
	if _, err := ProjectInsight(InsightInput{Finding: InsightFinding{}}, LevelCard); err == nil {
		t.Error("a finding with no id must be an error, not a blank card")
	}
}
