package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// asst builds an assistant turn carrying an optional rating. rating 0 with an
// empty note means "no feedback row at all" (what the store writes when the user
// clears a rating).
func asst(text string, rating int, note string) db.Message {
	m := db.Message{Role: providers.RoleAssistant, Text: text}
	if rating != 0 || note != "" {
		m.Feedback = &db.MessageFeedback{Rating: rating, Note: note}
	}
	return m
}

func usr(text string) db.Message {
	return db.Message{Role: providers.RoleUser, Text: text}
}

// TestFeedbackBlockEmptyWithoutRatings: an unrated conversation must inject
// nothing at all — no empty <user_feedback> shell burning prompt tokens.
func TestFeedbackBlockEmptyWithoutRatings(t *testing.T) {
	got := recentFeedbackBlock([]db.Message{usr("soru"), asst("cevap", 0, "")})
	if got != "" {
		t.Fatalf("expected empty block for an unrated conversation, got:\n%s", got)
	}
}

// TestFeedbackBlockSkipsClearedRating: rating 0 is an explicit "never mind" (the
// user un-clicked the thumb), so it must not be reported as feedback.
func TestFeedbackBlockSkipsClearedRating(t *testing.T) {
	got := recentFeedbackBlock([]db.Message{asst("cevap", 0, "stale note")})
	if got != "" {
		t.Fatalf("cleared rating (0) must not surface, got:\n%s", got)
	}
}

// TestFeedbackBlockRendersVerdictsAndAges checks the two ratings, their turn-age
// labels, and that entries read oldest→newest like the rest of the prompt.
func TestFeedbackBlockRendersVerdictsAndAges(t *testing.T) {
	got := recentFeedbackBlock([]db.Message{
		usr("ilk"),
		asst("eski cevap", 1, ""), // 2 assistant turns ago
		usr("ikinci"),
		asst("ara cevap", 0, ""), // unrated, still ages the counter
		usr("ucuncu"),
		asst("son cevap", -1, "cok uzun"), // latest assistant turn
	})
	if got == "" {
		t.Fatal("expected a block, got empty")
	}
	for _, want := range []string{
		"<user_feedback>",
		"</user_feedback>",
		"👍 liked",
		"👎 disliked",
		"[latest assistant turn]",
		"[2 assistant turn(s) ago]",
		"user's note: cok uzun",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("block missing %q:\n%s", want, got)
		}
	}
	// The unrated middle turn must not appear as an entry.
	if strings.Contains(got, "ara cevap") {
		t.Errorf("unrated turn leaked into the block:\n%s", got)
	}
	// Oldest first: the 👍 from 2 turns ago precedes the latest 👎.
	if strings.Index(got, "👍 liked") > strings.Index(got, "👎 disliked") {
		t.Errorf("entries not in oldest→newest order:\n%s", got)
	}
}

// TestFeedbackBlockCapsEntries: ratings are sparse but a long session can still
// accumulate many, and the block rides in EVERY turn's prompt — so it must stay
// bounded to the newest few.
func TestFeedbackBlockCapsEntries(t *testing.T) {
	var history []db.Message
	for i := 0; i < feedbackMaxEntries+5; i++ {
		history = append(history, usr("q"), asst(fmt.Sprintf("reply-%d", i), -1, ""))
	}
	got := recentFeedbackBlock(history)
	if n := strings.Count(got, "👎 disliked"); n != feedbackMaxEntries {
		t.Fatalf("expected %d entries, got %d:\n%s", feedbackMaxEntries, n, got)
	}
	// The kept ones must be the NEWEST: the very first reply must have dropped out.
	if strings.Contains(got, `"reply-0"`) {
		t.Errorf("oldest rating survived the cap:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf(`"reply-%d"`, feedbackMaxEntries+4)) {
		t.Errorf("newest rating missing from the block:\n%s", got)
	}
}

// TestFeedbackBlockTruncatesLongExcerpt guards the prompt-size bound: a huge rated
// reply must be clipped, not pasted back in full.
func TestFeedbackBlockTruncatesLongExcerpt(t *testing.T) {
	long := strings.Repeat("uzun cevap ", 500)
	got := recentFeedbackBlock([]db.Message{asst(long, 1, "")})
	if len([]rune(got)) > feedbackMaxExcerpt+800 {
		t.Fatalf("block not truncated: %d runes", len([]rune(got)))
	}
	if !strings.Contains(got, "…") {
		t.Errorf("expected an ellipsis marking the cut:\n%s", got)
	}
}
