package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
func ProjectArtifact(in ArtifactInput, level Level, lens Lens) (View, error) {
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
		Lens:   lens,
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
	if a.ContentFile != "" {
		meta = append(meta, "dosya: "+a.ContentFile)
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
