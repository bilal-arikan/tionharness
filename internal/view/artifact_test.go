package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestProjectArtifactRendersMetadata(t *testing.T) {
	now := time.Now()
	v, err := ProjectArtifact(ArtifactInput{
		Artifact: db.Artifact{
			ID: "ART1", Title: "görev raporu", Kind: db.ArtifactMarkdown,
			Origin: "tool", SessionID: "SES1", AgentID: "AG1",
			Group: "raporlar", Language: "", ContentFile: "artifacts/ART1.md",
			UpdatedAt: now.Add(-2 * time.Hour).Unix(),
		},
		Now: now,
	}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{
		"ARTIFACT · ART1", "görev raporu", "markdown",
		"origin: tool", "session:SES1", "agent:AG1",
		"grup: raporlar", "dosya: artifacts/ART1.md", "2sa önce",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestProjectArtifactRejectsEmptyID(t *testing.T) {
	if _, err := ProjectArtifact(ArtifactInput{Artifact: db.Artifact{}}, LevelCard, LensHealth); err == nil {
		t.Error("an artifact with no id must be an error, not a blank card")
	}
}

func TestProjectArtifactTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectArtifact(ArtifactInput{
		Artifact: db.Artifact{ID: "ART1", Title: "x", Kind: "text", UpdatedAt: 1},
	}, LevelTiny, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}
