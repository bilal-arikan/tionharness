package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/logbuf"
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
	v, err := ProjectLogs(LogsInput{Entries: logFixture(now), Now: now}, LevelCard)
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

func TestProjectLogsCapsAndElides(t *testing.T) {
	now := time.Now()
	entries := make([]logbuf.Entry, 0, logsRows+25)
	for i := 0; i < logsRows+25; i++ {
		entries = append(entries, logbuf.Entry{
			Seq: int64(i + 1), Time: now.Add(time.Duration(i) * time.Minute).UnixMilli(),
			Level: "INFO", Message: "kayıt",
		})
	}
	v, err := ProjectLogs(LogsInput{Entries: entries, Now: now}, LevelCard)
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
	v, err := ProjectLogs(LogsInput{}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "(log kaydı yok)") {
		t.Errorf("empty log stream must say so:\n%s", v.Text())
	}
}

func TestProjectLogsTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectLogs(LogsInput{Entries: logFixture(time.Now())}, LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}

// TestLogsHeaderCountsFailuresInWindow pins the one question a log tail is opened
// to answer. The count covers the RENDERED window only — counting elided rows too
// would promise a scan the body cannot back up.
func TestLogsHeaderCountsFailuresInWindow(t *testing.T) {
	now := time.Now()
	v, err := ProjectLogs(LogsInput{Entries: logFixture(now), Now: now}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "son 4 kayıtta 2 hata / 1 uyarı") {
		t.Errorf("failure tally missing or wrong: %q", v.Header)
	}

	// A clean stream says nothing extra.
	clean, err := ProjectLogs(LogsInput{
		Entries: []logbuf.Entry{{Seq: 1, Time: now.UnixMilli(), Level: "INFO", Message: "ok"}},
		Now:     now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project clean: %v", err)
	}
	if strings.Contains(clean.Header, "hata") {
		t.Errorf("a clean tail must not claim failures: %q", clean.Header)
	}
}

// TestLogsShortensEmbeddedPaths locks compactPaths on the message: a log line is
// prose that routinely quotes the file it failed to open, and clip cuts exactly
// that tail off.
func TestLogsShortensEmbeddedPaths(t *testing.T) {
	now := time.Now()
	v, err := ProjectLogs(LogsInput{
		Entries: []logbuf.Entry{{
			Seq: 1, Time: now.UnixMilli(), Level: "ERROR",
			Message: "açılamadı: C:/Users/user/Desktop/Projects/TionHarness/internal/view/logs.go",
		}},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "logs.go") {
		t.Errorf("the identifying tail of the path was cut:\n%s", v.Text())
	}
}
