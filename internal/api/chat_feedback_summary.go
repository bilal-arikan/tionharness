package api

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Bounds for the user-feedback recap injected into the volatile dynamic suffix.
// Ratings are sparse and long-lived, so unlike the tool recap this scans the WHOLE
// history and keeps the newest few — a 👎 from twenty turns ago is still the most
// useful thing the agent can know about this user's expectations.
const (
	feedbackMaxEntries = 6   // newest rated turns carried into the prompt
	feedbackMaxExcerpt = 200 // runes kept of the rated reply, to identify it
	feedbackMaxNote    = 300 // runes kept of the user's free-text note
)

// recentFeedbackBlock renders ONE compact <user_feedback> block listing the
// assistant turns the user rated 👍/👎 in this conversation, for the volatile
// dynamic suffix.
//
// Why it exists: the rating is stored on the message (db.MessageFeedback) and
// survives in session.jsonl, but nothing ever read it back — pressing 👎 changed
// literally nothing about the next reply. This is the read side: the agent now
// sees which of its answers landed and which did not.
//
// Like recentToolActivityBlock it is deliberately NOT folded into the history
// messages: a rating can be added, flipped or cleared at any time, and rewriting a
// past turn's bytes would invalidate the rolling prompt-cache breakpoint on the
// conversation history. As a dynamic block it rides after the breakpoint, so
// rating a turn never busts the cache.
//
// Phrasing note: entries are described as "replies in this conversation" rather
// than "your replies". In a multi-agent thread the rated turn may belong to a peer,
// and telling an agent it was criticised for someone else's answer would be wrong.
//
// Returns "" when nothing in the conversation has been rated.
func recentFeedbackBlock(history []db.Message) string {
	// Newest-first scan, then flip to oldest→newest so the block reads in
	// conversation order like the rest of the prompt.
	type entry struct {
		age    int // 0 = latest assistant turn
		rating int
		note   string
		text   string
	}
	var entries []entry
	age := 0
	for i := len(history) - 1; i >= 0 && len(entries) < feedbackMaxEntries; i-- {
		if history[i].Role != providers.RoleAssistant {
			continue
		}
		fb := history[i].Feedback
		// Rating 0 means the user cleared it — an explicit "never mind", not a signal.
		if fb != nil && fb.Rating != 0 {
			entries = append([]entry{{
				age:    age,
				rating: fb.Rating,
				note:   truncateRunes(strings.TrimSpace(fb.Note), feedbackMaxNote),
				text:   truncateRunes(strings.Join(strings.Fields(history[i].Text), " "), feedbackMaxExcerpt),
			}}, entries...)
		}
		age++
	}
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<user_feedback>\n")
	b.WriteString("The user rated these earlier replies in this conversation with 👍/👎. ")
	b.WriteString("Treat it as a standing preference signal for THIS user: repeat what earned 👍, ")
	b.WriteString("and change the approach that earned 👎 — a rating is feedback on the answer, ")
	b.WriteString("not a new request. Do not bring it up, thank the user for it, or apologise for a 👎; ")
	b.WriteString("just apply it silently. Where a note is present it states exactly what to change ")
	b.WriteString("and outranks your own reading of the excerpt.\n")
	for _, e := range entries {
		label := "[latest assistant turn]"
		if e.age > 0 {
			label = fmt.Sprintf("[%d assistant turn(s) ago]", e.age)
		}
		verdict := "👎 disliked"
		if e.rating > 0 {
			verdict = "👍 liked"
		}
		b.WriteString(fmt.Sprintf("%s %s", label, verdict))
		if e.text != "" {
			b.WriteString(fmt.Sprintf(" — reply began: %q", e.text))
		}
		b.WriteString("\n")
		if e.note != "" {
			b.WriteString(fmt.Sprintf("  user's note: %s\n", e.note))
		}
	}
	b.WriteString("</user_feedback>")
	return b.String()
}
