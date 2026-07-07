package db

import (
	"path/filepath"
	"testing"
	"time"
)

func lessonsDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestLessons_AddListRoundTrip(t *testing.T) {
	d := lessonsDB(t)
	l, err := d.AddLesson(Lesson{Time: now() - 300, Tool: "Read", Signature: "Read:abc", Text: "check the path exists first"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if l.ID == "" || l.Count != 1 {
		t.Fatalf("lesson = %+v, want id set + count 1", l)
	}
	got, err := d.ListLessons(0)
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %v, %v; want 1 lesson", got, err)
	}
	if got[0].Text != "check the path exists first" {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestLessons_DedupeBySignature(t *testing.T) {
	d := lessonsDB(t)
	base := time.Now().Unix()
	_, _ = d.AddLesson(Lesson{Time: base - 300, Tool: "Bash", Signature: "Bash:x", Text: "old wording", SessionID: "SES1"})
	l, err := d.AddLesson(Lesson{Time: base - 200, Tool: "Bash", Signature: "Bash:x", Text: "new wording", SessionID: "SES2"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if l.Count != 2 || l.Text != "new wording" || l.SessionID != "SES2" || l.Time != base-200 {
		t.Fatalf("dedupe result = %+v, want count 2 + refreshed fields", l)
	}
	got, _ := d.ListLessons(0)
	if len(got) != 1 {
		t.Fatalf("list len = %d, want 1 (deduped)", len(got))
	}
}

func TestLessons_NewestFirstAndLimit(t *testing.T) {
	d := lessonsDB(t)
	_, _ = d.AddLesson(Lesson{Time: now() - 300, Signature: "a", Text: "oldest"})
	_, _ = d.AddLesson(Lesson{Time: now() - 100, Signature: "b", Text: "newest"})
	_, _ = d.AddLesson(Lesson{Time: now() - 200, Signature: "c", Text: "middle"})
	got, _ := d.ListLessons(2)
	if len(got) != 2 || got[0].Text != "newest" || got[1].Text != "middle" {
		t.Fatalf("list = %+v, want [newest, middle]", got)
	}
}

func TestLessons_Delete(t *testing.T) {
	d := lessonsDB(t)
	l, _ := d.AddLesson(Lesson{Time: now() - 300, Signature: "a", Text: "x"})
	if err := d.DeleteLesson(l.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := d.DeleteLesson(l.ID); err != ErrNotFound {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
	got, _ := d.ListLessons(0)
	if len(got) != 0 {
		t.Errorf("list after delete = %+v, want empty", got)
	}
}

func TestLessons_CapKeepsNewest(t *testing.T) {
	d := lessonsDB(t)
	base := time.Now().Unix()
	for i := 0; i < DefaultLessonsCap+10; i++ {
		_, err := d.AddLesson(Lesson{Time: base - int64(DefaultLessonsCap+10) + int64(i), Signature: sigN(i), Text: "t"})
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	got, _ := d.ListLessons(0)
	if len(got) != DefaultLessonsCap {
		t.Fatalf("list len = %d, want cap %d", len(got), DefaultLessonsCap)
	}
	if got[0].Time != base-1 {
		t.Errorf("newest kept = %d, want %d", got[0].Time, base-1)
	}
}

func TestLessons_AgingPrunesStale(t *testing.T) {
	d := lessonsDB(t)
	base := time.Now().Unix()
	// One lesson well past LessonMaxAge, one fresh.
	_, _ = d.AddLesson(Lesson{Time: base - int64(LessonMaxAge.Seconds()) - 3600, Signature: "old", Text: "stale lesson"})
	_, _ = d.AddLesson(Lesson{Time: base - 60, Signature: "new", Text: "fresh lesson"})
	got, _ := d.ListLessons(0)
	if len(got) != 1 || got[0].Text != "fresh lesson" {
		t.Fatalf("list = %+v, want only the fresh lesson (stale aged out)", got)
	}
}

func sigN(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))
}
