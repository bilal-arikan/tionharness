package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// A delegated session must say WHICH session delegated it, so a subagent row in
// the session list is traceable back to its origin thread.
func TestSubagentTitleNamesParentSession(t *testing.T) {
	title := subagentTitle(db.Agent{ID: "AGT7", Name: "Kâşif"}, tools.RunAgentSpec{Target: "Kâşif", Task: "graf taramasını çalıştır"}, "SES1", "Rota ekranı düzeltmesi")

	if !strings.Contains(title, "Kâşif") {
		t.Errorf("title = %q, want it to name the target agent", title)
	}
	if !strings.Contains(title, "graf taramasını") {
		t.Errorf("title = %q, want a snippet of the task", title)
	}
	if !strings.Contains(title, "Rota ekranı düzeltmesi") {
		t.Errorf("title = %q, want it to name the parent session", title)
	}
}

// An untitled parent (titled only after its first turn) still gets named: the id
// fallback is explicit, not a silently dropped origin.
func TestSubagentTitleFallsBackToParentID(t *testing.T) {
	title := subagentTitle(db.Agent{Name: "Kâşif"}, tools.RunAgentSpec{Task: "testi düzelt"}, "SES42", "   ")

	if !strings.Contains(title, "SES42") {
		t.Errorf("title = %q, want the parent id as the origin fallback", title)
	}
}

// With neither a parent title nor a parent id there is nothing to reference, so
// the title degrades to the parentless form instead of trailing an empty marker.
func TestSubagentTitleWithoutParentDropsOriginSegment(t *testing.T) {
	title := subagentTitle(db.Agent{Name: "Kâşif"}, tools.RunAgentSpec{Task: "testi düzelt"}, "", "")

	if strings.Contains(title, "⤴") {
		t.Errorf("title = %q, want no origin segment without a parent reference", title)
	}
	if !strings.Contains(title, "Kâşif") || !strings.Contains(title, "testi düzelt") {
		t.Errorf("title = %q, want the parentless target — task form", title)
	}
}

// A long parent title is shortened so the target and the task stay readable.
func TestSubagentTitleTruncatesLongParentTitle(t *testing.T) {
	parent := strings.Repeat("uzun başlık ", 20)
	title := subagentTitle(db.Agent{Name: "Kâşif"}, tools.RunAgentSpec{Task: "testi düzelt"}, "SES1", parent)

	if strings.Contains(title, strings.TrimSpace(parent)) {
		t.Errorf("title = %q, want the parent title shortened", title)
	}
	if !strings.Contains(title, "uzun başlık") {
		t.Errorf("title = %q, want the opening of the parent title kept", title)
	}
	if n := len([]rune(title)); n > 160 {
		t.Errorf("title runes = %d, want a bounded title", n)
	}
}

// The child session row carries the composed title, so the session list never
// falls back to its "new chat" placeholder for a delegated run.
func TestSubagentSessionMetaCarriesParentAwareTitle(t *testing.T) {
	meta := subagentSessionMeta("SES1", "Rota ekranı düzeltmesi", db.Agent{ID: "AGT7", Name: "Kâşif"}, false, tools.RunAgentSpec{Target: "Kâşif", Task: "graf taramasını çalıştır"})

	if meta.Title == "" {
		t.Fatal("Title is empty; a delegated run must be named")
	}
	if !strings.Contains(meta.Title, "Rota ekranı düzeltmesi") {
		t.Errorf("Title = %q, want it to name the parent session", meta.Title)
	}
	if strings.Contains(meta.Title, "\n") {
		t.Errorf("Title = %q, want a single line", meta.Title)
	}
}
