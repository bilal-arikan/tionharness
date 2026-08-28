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
	}, LevelCard)
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
	if _, err := ProjectArtifact(ArtifactInput{Artifact: db.Artifact{}}, LevelCard); err == nil {
		t.Error("an artifact with no id must be an error, not a blank card")
	}
}

func TestProjectArtifactTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectArtifact(ArtifactInput{
		Artifact: db.Artifact{ID: "ART1", Title: "x", Kind: "text", UpdatedAt: 1},
	}, LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}

// TestProjectArtifactRendersSizeAndSourcePath locks the two "is it worth opening"
// facts: a text artifact reports its body size, and a media artifact reports the
// file its bytes live in (its Content is only a caption, so no size is claimed).
func TestProjectArtifactRendersSizeAndSourcePath(t *testing.T) {
	now := time.Now()
	text, err := ProjectArtifact(ArtifactInput{
		Artifact: db.Artifact{
			ID: "ART2", Title: "rapor", Kind: db.ArtifactMarkdown,
			Content: strings.Repeat("a", 4200), UpdatedAt: now.Unix(),
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project text: %v", err)
	}
	if !strings.Contains(text.Text(), "boyut: 4k karakter") {
		t.Errorf("text artifact must report its size:\n%s", text.Text())
	}

	media, err := ProjectArtifact(ArtifactInput{
		Artifact: db.Artifact{
			ID: "ART3", Title: "ekran", Kind: db.ArtifactImage,
			SourcePath: "uploads/ART3.png", Content: "başlık", UpdatedAt: now.Unix(),
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project media: %v", err)
	}
	txt := media.Text()
	if !strings.Contains(txt, "dosya: uploads/ART3.png") {
		t.Errorf("media artifact must name where its bytes live:\n%s", txt)
	}
	if strings.Contains(txt, "boyut:") {
		t.Errorf("a caption length must not be rendered as the file size:\n%s", txt)
	}
}
