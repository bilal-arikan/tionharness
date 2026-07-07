package db

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// lessonsFile is the workspace-scoped failure-lesson store (self-healing,
// hata→ders döngüsü). Like debug.jsonl it is a plain JSONL sidecar owned by its
// own mutex, but it is WORKSPACE-wide (store root, not per-session): a lesson
// learned from one session's failure must reach every future turn.
const lessonsFile = "lessons.jsonl"

// DefaultLessonsCap bounds the lessons file (newest kept). Lessons are meant to
// be a small, high-signal set — not a log.
const DefaultLessonsCap = 200

// Lesson is one distilled failure lesson, produced by the lesson reflector
// after a turn that ended badly. Signature identifies the failure shape
// (tool + normalized error) so repeats update the existing lesson (Count++)
// instead of piling up duplicates.
type Lesson struct {
	ID        string `json:"id"`
	Time      int64  `json:"ts"` // unix seconds of the LAST occurrence
	AgentID   string `json:"agentId,omitempty"`
	SessionID string `json:"sessionId,omitempty"` // session of the last occurrence
	Tool      string `json:"tool,omitempty"`      // failing tool ("" for turn-level errors)
	Signature string `json:"sig"`                 // dedupe key (tool + error shape)
	Text      string `json:"text"`                // the lesson itself (model-facing English)
	Count     int    `json:"count"`               // how many times this failure shape was seen
}

func (d *DB) lessonsPath() string { return filepath.Join(d.root, lessonsFile) }

// AddLesson records a lesson, deduplicating by Signature: a repeat bumps Count
// and refreshes Time/Text/SessionID on the existing entry (the newest wording
// wins — it reflects the latest occurrence). The file is rewritten capped to
// DefaultLessonsCap newest entries. Guarded by its own mutex so it never
// contends with the store hot path.
func (d *DB) AddLesson(l Lesson) (Lesson, error) {
	d.lessonsMu.Lock()
	defer d.lessonsMu.Unlock()

	lessons, err := readLessonsFile(d.lessonsPath())
	if err != nil {
		return Lesson{}, err
	}
	updated := false
	for i := range lessons {
		if l.Signature != "" && lessons[i].Signature == l.Signature {
			lessons[i].Count++
			lessons[i].Time = l.Time
			lessons[i].SessionID = l.SessionID
			if l.Text != "" {
				lessons[i].Text = l.Text
			}
			l = lessons[i]
			updated = true
			break
		}
	}
	if !updated {
		if l.ID == "" {
			l.ID = newID()
		}
		if l.Count <= 0 {
			l.Count = 1
		}
		lessons = append(lessons, l)
	}
	if len(lessons) > DefaultLessonsCap {
		lessons = lessons[len(lessons)-DefaultLessonsCap:]
	}
	if err := writeLessonsFile(d.lessonsPath(), lessons); err != nil {
		return Lesson{}, err
	}
	return l, nil
}

// ListLessons returns the stored lessons NEWEST-first (by last occurrence).
// limit <= 0 returns all. A missing file yields an empty slice.
func (d *DB) ListLessons(limit int) ([]Lesson, error) {
	d.lessonsMu.Lock()
	lessons, err := readLessonsFile(d.lessonsPath())
	d.lessonsMu.Unlock()
	if err != nil {
		return nil, err
	}
	// File order is append order; sort newest-first by Time (stable enough for
	// a capped small set — repeats refresh Time in place).
	for i, j := 0, len(lessons)-1; i < j; i, j = i+1, j-1 {
		lessons[i], lessons[j] = lessons[j], lessons[i]
	}
	sortLessonsByTimeDesc(lessons)
	if limit > 0 && len(lessons) > limit {
		lessons = lessons[:limit]
	}
	return lessons, nil
}

// DeleteLesson removes one lesson by id (a fixer pruning a stale lesson).
// Returns ErrNotFound when absent.
func (d *DB) DeleteLesson(id string) error {
	d.lessonsMu.Lock()
	defer d.lessonsMu.Unlock()
	lessons, err := readLessonsFile(d.lessonsPath())
	if err != nil {
		return err
	}
	kept := lessons[:0:0]
	found := false
	for _, l := range lessons {
		if l.ID == id {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return ErrNotFound
	}
	return writeLessonsFile(d.lessonsPath(), kept)
}

// sortLessonsByTimeDesc is a tiny insertion sort (the set is capped at 200 and
// nearly sorted already) keeping newest-first order deterministic.
func sortLessonsByTimeDesc(ls []Lesson) {
	for i := 1; i < len(ls); i++ {
		for j := i; j > 0 && ls[j].Time > ls[j-1].Time; j-- {
			ls[j], ls[j-1] = ls[j-1], ls[j]
		}
	}
}

func readLessonsFile(path string) ([]Lesson, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Lesson
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var l Lesson
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Text != "" {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

func writeLessonsFile(path string, lessons []Lesson) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, l := range lessons {
		if err := enc.Encode(l); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
