package db

import (
	"testing"
	"time"
)

// The fixture below is the real signature set from a live workspace's
// lessons.jsonl (66 rows, 66 distinct signatures, 2 with Count > 1) — the four
// topic clusters the LLM re-slugged plus the near misses that must stay apart.
// Signatures and Tool values are verbatim; bodies are stand-ins because the
// merge decision reads the signature only (measured: body token overlap is ~0.06
// for true and false pairs alike, i.e. it carries no signal).

var lessonFixtureShellChaining = []Lesson{
	{Signature: "lesson:powershell-no-ampersand-chaining", Text: "chain with ; not &&"},
	{Signature: "lesson:powershell-double-ampersand-chaining", Text: "no bash-style && in PowerShell"},
	{Signature: "lesson:powershell-bash-chaining-operator", Text: "shell tool is bash, not PowerShell"},
	{Signature: "lesson:bash-not-powershell", Text: "use Bash, the workspace mandates it"},
	{Signature: "lesson:env-var-syntax-bash-vs-powershell", Text: "env var syntax differs per shell"},
}

var lessonFixtureCRLF = []Lesson{
	{Signature: "lesson:crlf-shebang-bash-cr-error", Text: "bash: \\r command not found"},
	{Signature: "lesson:crlf-shebang-breaks-shell-script", Text: "write shell scripts with LF endings"},
	{Signature: "lesson:shebang-crlf-line-endings", Text: "a CRLF shebang breaks execution"},
}

var lessonFixtureAskUser = []Lesson{
	{Signature: "lesson:ask_user-timeout-fallback", Text: "do not hang the session on a timeout"},
	{Signature: "lesson:ask_user_timeout_handling", Text: "expect ask_user to time out"},
	{Signature: "lesson:ask_user-headless-incompatibility", Text: "ask_user cannot run headless"},
}

var lessonFixtureWorkerQueue = []Lesson{
	{Signature: "lesson:send_to_worker-queue-full-retry", Text: "check the worker queue before sending"},
	{Signature: "lesson:worker-queue-single-message-limit", Text: "wait for delivery before the next message"},
	{Signature: "lesson:sendto_worker_queue_check", Text: "inspect queue state before sending"},
}

// lessonFixtureUnrelated are signatures that share a word or two with the
// clusters above but describe different problems; merging any of them would be a
// regression, not a fix.
var lessonFixtureUnrelated = []Lesson{
	{Signature: "lesson:wsl-hcs-connection-timeout-shell-fallback", Text: "fall back to local PowerShell after a WSL error"},
	{Signature: "lesson:windows-tool-path-shell-mismatch", Text: "convert Windows tool paths to POSIX form"},
	{Signature: "lesson:spawn-worker-name-validation", Text: "spawn_worker rejects unknown agent types"},
	{Signature: "lesson:stop-already-finished-worker", Text: "stopping a finished worker is an error"},
}

func stampLessons(ls []Lesson, base int64) []Lesson {
	out := make([]Lesson, len(ls))
	for i, l := range ls {
		l.Time = base + int64(i)
		l.Count = 1
		out[i] = l
	}
	return out
}

// TestAddLessonMergesRewordedSignature is the write-time layer: the second and
// third wording of one topic must bump the first row, not open new ones.
func TestAddLessonMergesRewordedSignature(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cluster []Lesson
	}{
		{"shell chaining", lessonFixtureShellChaining},
		{"crlf shebang", lessonFixtureCRLF},
		{"ask_user", lessonFixtureAskUser},
		{"worker queue", lessonFixtureWorkerQueue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := lessonsDB(t)
			now := time.Now().Unix()
			for i, l := range tc.cluster {
				l.Time = now + int64(i)
				if _, err := d.AddLesson(l); err != nil {
					t.Fatalf("AddLesson: %v", err)
				}
			}
			got, err := d.ListLessons(0)
			if err != nil {
				t.Fatalf("ListLessons: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("cluster of %d wordings stored as %d lessons, want 1: %s",
					len(tc.cluster), len(got), lessonSigs(got))
			}
			if got[0].Count != len(tc.cluster) {
				t.Fatalf("Count = %d, want %d", got[0].Count, len(tc.cluster))
			}
		})
	}
}

// TestAddLessonKeepsUnrelatedLessonsApart pins the other side of the threshold:
// signatures that merely share a generic word stay separate rows.
func TestAddLessonKeepsUnrelatedLessonsApart(t *testing.T) {
	d := lessonsDB(t)
	now := time.Now().Unix()
	seed := append(append([]Lesson{}, lessonFixtureAskUser...), lessonFixtureWorkerQueue...)
	seed = append(seed, lessonFixtureUnrelated...)
	for i, l := range seed {
		l.Time = now + int64(i)
		if _, err := d.AddLesson(l); err != nil {
			t.Fatalf("AddLesson: %v", err)
		}
	}
	got, err := d.ListLessons(0)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	// Two clusters collapse to one row each; every unrelated lesson keeps its own.
	want := 2 + len(lessonFixtureUnrelated)
	if len(got) != want {
		t.Fatalf("stored %d lessons, want %d: %s", len(got), want, lessonSigs(got))
	}
}

// TestAddLessonNeverMergesAcrossTools: same words, different tool → different rule.
func TestAddLessonNeverMergesAcrossTools(t *testing.T) {
	d := lessonsDB(t)
	now := time.Now().Unix()
	for _, tool := range []string{"shell", "send_to_worker"} {
		for i, l := range lessonFixtureWorkerQueue {
			l.Tool = tool
			l.Time = now + int64(i)
			if _, err := d.AddLesson(l); err != nil {
				t.Fatalf("AddLesson: %v", err)
			}
		}
	}
	got, err := d.ListLessons(0)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored %d lessons, want one per tool: %s", len(got), lessonSigs(got))
	}
}

// TestAddLessonNeverMergesReflectorSignatures: the reflector's "<tool>:<hash>"
// keys carry no topic tokens, so they must dedupe exactly and never fuzzily.
func TestAddLessonNeverMergesReflectorSignatures(t *testing.T) {
	d := lessonsDB(t)
	now := time.Now().Unix()
	sigs := []string{"shell:e0d6fc2e2e0db48f", "shell:55ede445e3c9e63a", "shell:4590819a567cdfd1"}
	for i, s := range sigs {
		if _, err := d.AddLesson(Lesson{Time: now + int64(i), Tool: "shell", Signature: s, Text: "a distinct shell failure"}); err != nil {
			t.Fatalf("AddLesson: %v", err)
		}
	}
	got, err := d.ListLessons(0)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(got) != len(sigs) {
		t.Fatalf("stored %d lessons, want %d: %s", len(got), len(sigs), lessonSigs(got))
	}
}

// TestDedupeLessonsBackfill covers the maintenance pass over a file written
// before write-time matching existed: every cluster collapses transitively, the
// counts are summed, and a second pass changes nothing.
func TestDedupeLessonsBackfill(t *testing.T) {
	d := lessonsDB(t)
	base := time.Now().Unix() - 600
	var seeded []Lesson
	for _, c := range [][]Lesson{lessonFixtureShellChaining, lessonFixtureCRLF, lessonFixtureAskUser, lessonFixtureWorkerQueue, lessonFixtureUnrelated} {
		seeded = append(seeded, stampLessons(c, base)...)
	}
	for i := range seeded {
		seeded[i].ID = newID()
		seeded[i].Time = base + int64(i)
	}
	if err := writeLessonsFile(d.lessonsPath(), seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := d.DedupeLessons()
	if err != nil {
		t.Fatalf("DedupeLessons: %v", err)
	}
	if res.Before != len(seeded) {
		t.Fatalf("Before = %d, want %d", res.Before, len(seeded))
	}
	want := 4 + len(lessonFixtureUnrelated) // one row per cluster + the untouched rest
	if res.After != want {
		got, _ := d.ListLessons(0)
		t.Fatalf("After = %d, want %d: %s", res.After, want, lessonSigs(got))
	}
	if res.Merged != res.Before-res.After {
		t.Fatalf("Merged = %d, want %d", res.Merged, res.Before-res.After)
	}

	got, err := d.ListLessons(0)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	total := 0
	for _, l := range got {
		total += l.Count
	}
	if total != len(seeded) {
		t.Fatalf("summed Count = %d, want %d (occurrences must survive the merge)", total, len(seeded))
	}

	// Idempotency: the pass over its own output must be a no-op.
	again, err := d.DedupeLessons()
	if err != nil {
		t.Fatalf("DedupeLessons (second pass): %v", err)
	}
	if again.Merged != 0 || again.After != res.After {
		t.Fatalf("second pass merged %d (after %d), want a no-op", again.Merged, again.After)
	}
}

func lessonSigs(ls []Lesson) string {
	out := ""
	for _, l := range ls {
		out += "\n  " + l.Signature
	}
	return out
}
