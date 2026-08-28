package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ArtifactInput is one saved artifact. The projection renders METADATA only —
// the artifact's content stays in the Artifacts screen; a map node needs
// identity, origin and age to decide whether it is worth opening, not its body.
type ArtifactInput struct {
	Artifact db.Artifact
	// Now is the clock used for age stamps. Zero means time.Now().
	Now time.Time
}

// ProjectArtifact renders one artifact's metadata: origin (chat/manual/agent/
// tool/plan), the originating session/agent, kind/language/group and how long
// ago it last changed.
func ProjectArtifact(in ArtifactInput, level Level) (View, error) {
	a := in.Artifact
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if a.ID == "" {
		return View{}, fmt.Errorf("view: artifact has no id")
	}

	v := View{
		Ref:    Ref{Kind: KindArtifact, ID: a.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d", a.UpdatedAt),
	}
	title := clip(orDash(a.Title), 60)
	kind := orDash(a.Kind)
	if kind == "" {
		kind = "?"
	}
	v.Header = fmt.Sprintf("ARTIFACT · %s · %q · %s · asOf %s", a.ID, title, kind, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	origin := orDash(a.Origin)
	if origin == "" {
		origin = "?"
	}
	originLine := "origin: " + origin
	if a.SessionID != "" {
		originLine += " · session:" + a.SessionID
	}
	if a.AgentID != "" {
		originLine += " · agent:" + a.AgentID
	}
	l.add("%s", originLine)

	var meta []string
	if a.Group != "" {
		meta = append(meta, "grup: "+a.Group)
	}
	if a.Language != "" {
		meta = append(meta, "dil: "+a.Language)
	}
	// Where the bytes live. A media/file artifact keeps them at SourcePath and its
	// Content is only a caption, so naming ContentFile alone left every image and
	// upload with no location at all.
	switch {
	case a.SourcePath != "":
		meta = append(meta, "dosya: "+clipPath(a.SourcePath, 60))
	case a.ContentFile != "":
		meta = append(meta, "dosya: "+clipPath(a.ContentFile, 60))
	}
	// Size is the "is this worth opening" signal a metadata card exists to answer.
	// Measured in runes, and only for the text kinds whose Content IS the body —
	// on a media artifact the same number would be the caption's length and read
	// as the file's size, which is a different fact. Artifacts are not versioned
	// (an update overwrites in place), so there is no revision to report.
	if a.SourcePath == "" && a.Content != "" {
		meta = append(meta, fmt.Sprintf("boyut: %s karakter", compactCount(int64(len([]rune(a.Content))))))
	}
	if a.Archived {
		meta = append(meta, "arşivli")
	}
	if len(meta) > 0 {
		l.add("%s", strings.Join(meta, " · "))
	}
	l.add("değişti %s önce", age(tsSec(a.UpdatedAt), now))

	v.Body = l.String()
	v.finalize()
	return v, nil
}
