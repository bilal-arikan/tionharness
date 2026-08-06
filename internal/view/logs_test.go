package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/logbuf"
)

// logFixture builds a small stream: an info heartbeat, a warn, and two errors
// (oldest first, the buffer's natural order).
func logFixture(now time.Time) []logbuf.Entry {
	return []logbuf.Entry{
		{Seq: 1, Time: now.Add(-3 * time.Minute).UnixMilli(), Level: "INFO", Message: "turn ok", Component: "api"},
		{Seq: 2, Time: now.Add(-2 * time.Minute).UnixMilli(), Level: "WARN", Message: "yavaş sağlayıcı", Component: "provider"},
		{Seq: 3, Time: now.Add(-1 * time.Minute).UnixMilli(), Level: "ERROR", Message: "provider 429", Component: "provider", Agent: "AG1"},
		{Seq: 4, Time: now.UnixMilli(), Level: "ERROR", Message: "cron hedefi yok", Session: "SES1"},
	}
}

func TestProjectLogsRendersTailOldestFirst(t *testing.T) {
	now := time.Now()
	v, err := ProjectLogs(LogsInput{Entries: logFixture(now), Now: now}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{"LOGS · 4 kayıt", "turn ok", "provider 429", "cron hedefi yok"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	// The stream reads top-down in time order: seq 1 before seq 4.
	if strings.Index(txt, "turn ok") > strings.Index(txt, "cron hedefi yok") {
		t.Errorf("tail order wrong (oldest must come first):\n%s", txt)
	}
}

func TestProjectLogsErrorsLensKeepsOnlyErrors(t *testing.T) {
	now := time.Now()
	v, err := ProjectLogs(LogsInput{Entries: logFixture(now), Now: now}, LevelCard, LensErrors)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "LOGS · 2 hata kaydı") {
		t.Errorf("errors lens count wrong:\n%s", txt)
	}
	for _, unwanted := range []string{"turn ok", "yavaş sağlayıcı"} {
		if strings.Contains(txt, unwanted) {
			t.Errorf("errors lens leaked %q:\n%s", unwanted, txt)
		}
	}
}

func TestProjectLogsCapsAndElides(t *testing.T) {
	now := time.Now()
	entries := make([]logbuf.Entry, 0, logsRows+25)
	for i := 0; i < logsRows+25; i++ {
		entries = append(entries, logbuf.Entry{
			Seq: int64(i + 1), Time: now.Add(time.Duration(i) * time.Minute).UnixMilli(),
			Level: "INFO", Message: "kayıt",
		})
	}
	v, err := ProjectLogs(LogsInput{Entries: entries, Now: now}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Elided != 25 {
		t.Errorf("elided = %d, want 25", v.Elided)
	}
	if v.ElidedUnit != "kayıt" {
		t.Errorf("elidedUnit = %q, want kayıt", v.ElidedUnit)
	}
	if !strings.Contains(v.Text(), "…25 kayıt gizlendi") {
		t.Errorf("elision notice missing:\n%s", v.Text())
	}
}

func TestProjectLogsEmptySaysSo(t *testing.T) {
	v, err := ProjectLogs(LogsInput{}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "(log kaydı yok)") {
		t.Errorf("empty log stream must say so:\n%s", v.Text())
	}
}

func TestProjectLogsTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectLogs(LogsInput{Entries: logFixture(time.Now())}, LevelTiny, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}
