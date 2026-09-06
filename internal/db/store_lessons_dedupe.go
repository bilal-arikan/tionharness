package db

import (
	"regexp"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/textutil"
)

// Lesson deduplication beyond the exact Signature match.
//
// Two producers write lessons. The turn reflector derives a DETERMINISTIC
// signature ("<tool>:<hash of normalized error>"), so its repeats already
// collapse on the exact key. The insight lessons-mining lens does not: its
// signature is a slug the model invents per run ("lesson:crlf-shebang-bash-cr-error",
// "lesson:shebang-crlf-line-endings", "lesson:crlf-shebang-breaks-shell-script"
// — all one topic, three rows). Measured on a live workspace: 66 lessons, 66
// distinct signatures, 2 with Count > 1. Dedup was effectively off, and because
// lessonTrust penalizes Count > 1, the duplicates even outranked the genuinely
// recurring lessons for the five injected context slots.
//
// The fix compares the TOPIC TOKENS of the signature slug — the words the model
// chose to name the problem. The lesson body itself is not used: it is free
// prose in the workspace language, and measurement showed its token overlap is
// uniformly low (~0.06) for both true and false pairs, i.e. no signal.

const (
	// lessonMergeOverlap is the minimum overlap coefficient (|a∩b| / min(|a|,|b|))
	// of two signature token sets. Overlap rather than plain Jaccard, because a
	// short slug is often fully contained in a longer one ("bash-not-powershell"
	// inside "powershell-bash-chaining-operator") and Jaccard alone would dilute
	// that to nothing.
	lessonMergeOverlap = 0.5

	// lessonMergeJaccard is the floor that keeps overlap honest. Overlap treats
	// "every word I have, you also have" as identity, so two slugs sharing only a
	// couple of generic words across a wide union would merge. The floor was set
	// against the live corpus: it keeps the observed topic clusters together while
	// rejecting the nearest false pair ("ask_user-timeout-fallback" vs
	// "wsl-hcs-connection-timeout-shell-fallback", which share only "timeout" and
	// "fallback" — Jaccard 0.25).
	lessonMergeJaccard = 0.28

	// lessonMergeMinTokens is how many topic tokens a signature must yield before
	// it may merge at all. This excludes the reflector's "<tool>:<hash>" keys (the
	// hash is dropped as opaque, the tool name is dropped as shared boilerplate,
	// leaving nothing) — those dedupe exactly and must never merge fuzzily.
	lessonMergeMinTokens = 2

	// lessonMergeMinShared is the absolute floor on shared topic words, so a merge
	// never rests on a single coincidental match.
	lessonMergeMinShared = 2
)

// lessonSigSlugPrefix is the prefix insight-mined signatures carry; it names the
// producer, not the topic, so it is stripped before tokenizing.
const lessonSigSlugPrefix = "lesson:"

// lessonOpaqueTokenRe matches a hex digest token (the reflector's signature
// hash). It names no topic, so it must not contribute similarity.
var lessonOpaqueTokenRe = regexp.MustCompile(`^[0-9a-f]{8,}$`)

// lessonTopicTokens is the token set a lesson is matched on: the words of its
// signature slug, minus the producer prefix and minus opaque hashes. The tool
// name is deliberately KEPT even though it repeats across a tool's lessons — it
// is often the most distinctive word in the slug ("send_to_worker-queue-full-retry"
// vs "worker-queue-single-message-limit"), and lessons of different tools never
// reach the comparison anyway.
func lessonTopicTokens(l Lesson) map[string]struct{} {
	sig := strings.TrimPrefix(l.Signature, lessonSigSlugPrefix)
	toks := textutil.Tokenize(sig)
	for t := range toks {
		if lessonOpaqueTokenRe.MatchString(t) {
			delete(toks, t)
		}
	}
	return toks
}

// lessonTokensMergeable reports whether two lessons' topic tokens describe the
// same failure shape. Both sides must carry enough tokens to be judged at all.
func lessonTokensMergeable(a, b map[string]struct{}) bool {
	if len(a) < lessonMergeMinTokens || len(b) < lessonMergeMinTokens {
		return false
	}
	// One word in common is a coincidence, whatever the ratios say: two two-word
	// slugs sharing their first word would otherwise clear both thresholds.
	if textutil.Intersection(a, b) < lessonMergeMinShared {
		return false
	}
	return textutil.Overlap(a, b) >= lessonMergeOverlap && textutil.Jaccard(a, b) >= lessonMergeJaccard
}

// collapseLessons folds every group of mergeable lessons into one row and
// returns the collapsed slice plus groupOf, mapping each input index to its
// survivor's index in the output. Grouping is TRANSITIVE (a↔b and b↔c put all
// three together), which matters because the wordings of one topic form a chain
// rather than a star: "bash-not-powershell" is not similar enough to
// "powershell-no-ampersand-chaining" on its own, but both are similar to
// "powershell-bash-chaining-operator" between them.
//
// Each group survives as its NEWEST member (the freshest wording of the rule)
// carrying the summed Count, so trust and the "seen N times" hint reflect the
// real recurrence. Survivors keep their first-seen order.
//
// Both the write path and the backfill pass run this same function, so a store
// written incrementally and one rebuilt in bulk converge on the same rows.
func collapseLessons(lessons []Lesson) (out []Lesson, groupOf []int) {
	groupOf = make([]int, len(lessons))
	if len(lessons) < 2 {
		return lessons, groupOf
	}
	toks := make([]map[string]struct{}, len(lessons))
	for i, l := range lessons {
		toks[i] = lessonTopicTokens(l)
	}
	// Union-find over every mergeable pair gives the transitive groups in one pass.
	parent := make([]int, len(lessons))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i := range lessons {
		for j := i + 1; j < len(lessons); j++ {
			if lessons[i].Tool != lessons[j].Tool || !lessonTokensMergeable(toks[i], toks[j]) {
				continue
			}
			if a, b := find(i), find(j); a != b {
				parent[a] = b
			}
		}
	}

	survivorOf := map[int]int{} // group root -> index in out
	out = lessons[:0:0]
	for i, l := range lessons {
		root := find(i)
		at, seen := survivorOf[root]
		if !seen {
			survivorOf[root] = len(out)
			groupOf[i] = len(out)
			out = append(out, l)
			continue
		}
		groupOf[i] = at
		merged := out[at]
		merged.Count = lessonCount(merged) + lessonCount(l)
		if l.Time >= merged.Time {
			// The newer occurrence owns the wording and the provenance fields; its
			// signature becomes the row's key so the next repeat matches exactly.
			merged.Time = l.Time
			merged.Text = l.Text
			merged.Signature = l.Signature
			merged.SessionID = l.SessionID
			if l.AgentID != "" {
				merged.AgentID = l.AgentID
			}
		}
		out[at] = merged
	}
	return out, groupOf
}

// LessonDedupeResult reports what one maintenance pass collapsed.
type LessonDedupeResult struct {
	Before int `json:"before"` // rows read
	After  int `json:"after"`  // rows written
	Merged int `json:"merged"` // rows folded into a survivor (Before - After)
}

// DedupeLessons collapses near-duplicate lessons already on disk — the backlog
// written before write-time similarity matching existed. Grouping is transitive
// (a↔b and b↔c put all three together) so a topic split across several wordings
// converges to one row instead of a chain of pairs.
//
// Each group survives as its NEWEST member (the freshest wording of the rule),
// carrying the summed Count so trust and the "seen N times" hint reflect the
// real recurrence. Output keeps the survivors in their original file order.
//
// Idempotent: a second pass over its own output finds no mergeable pair, because
// every group collapsed to a single row and distinct groups are by construction
// not similar. It rewrites the file only when something actually merged.
func (d *DB) DedupeLessons() (LessonDedupeResult, error) {
	d.lessonsMu.Lock()
	defer d.lessonsMu.Unlock()

	lessons, err := readLessonsFile(d.lessonsPath())
	if err != nil {
		return LessonDedupeResult{}, err
	}
	res := LessonDedupeResult{Before: len(lessons), After: len(lessons)}
	if len(lessons) < 2 {
		return res, nil
	}
	out, _ := collapseLessons(lessons)
	res.After = len(out)
	res.Merged = res.Before - res.After
	if res.Merged == 0 {
		return res, nil
	}
	if err := writeLessonsFile(d.lessonsPath(), out); err != nil {
		return LessonDedupeResult{}, err
	}
	return res, nil
}

// lessonCount is Count with the legacy zero treated as one occurrence (rows
// written before Count existed, and any hand-edited file).
func lessonCount(l Lesson) int {
	if l.Count <= 0 {
		return 1
	}
	return l.Count
}
