package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// maxActiveSessionsInBlock bounds how many active sessions the cross-session
// context block lists, so a busy workspace can't blow up the prompt.
const maxActiveSessionsInBlock = 12

// sessionsContextBlock builds a short, system-prompt section giving an agent
// situational awareness of the workspace's OTHER chat sessions: the active ones
// plus the most recent past ones. It reuses each session's existing Title and
// rolling Summary — no new LLM call — and is kept in the dynamic (uncached)
// suffix since the session list changes over time. Returns "" when there is
// nothing else to show. currentID is excluded (the agent is already in it).
func sessionsContextBlock(ctx context.Context, database *db.DB, currentID string, recentCount int) string {
	sessions, err := database.ListSessions(ctx, "") // workspace-wide, UpdatedAt desc
	if err != nil || len(sessions) == 0 {
		return ""
	}
	if recentCount <= 0 {
		recentCount = 5
	}

	now := time.Now().Unix()
	var active, past []string
	for _, s := range sessions {
		if s.Kind != "chat" || s.ID == currentID {
			continue
		}
		if s.State == "active" {
			if len(active) < maxActiveSessionsInBlock {
				active = append(active, formatSessionLine(s, now))
			}
		} else if len(past) < recentCount {
			past = append(past, formatSessionLine(s, now))
		}
	}
	if len(active) == 0 && len(past) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Other sessions in this workspace\n")
	b.WriteString("Situational awareness only — these are the workspace's other chat sessions. Use the list_sessions tool if you need fuller detail.\n")
	if len(active) > 0 {
		b.WriteString("\nActive:\n")
		for _, l := range active {
			b.WriteString(l + "\n")
		}
	}
	if len(past) > 0 {
		b.WriteString("\nRecent:\n")
		for _, l := range past {
			b.WriteString(l + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// formatSessionLine renders one session as a compact bullet: title, message
// count, relative age and an optional one-line summary snippet (from the rolling
// Summary). All free text is length-capped to keep the block small.
func formatSessionLine(s db.Session, now int64) string {
	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = "(untitled)"
	}
	line := fmt.Sprintf("- %q · %d msg · %s", clip(title, 60), s.MessageCount, relAge(now-s.UpdatedAt))
	if snip := summarySnippet(s.Summary); snip != "" {
		line += " — " + snip
	}
	return line
}

// summarySnippet returns the first non-empty line of a rolling summary, capped.
func summarySnippet(summary string) string {
	for _, ln := range strings.Split(summary, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return clip(ln, 120)
		}
	}
	return ""
}

// clip truncates s to at most n runes, appending an ellipsis when cut.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// relAge renders a positive age in seconds as a short relative label.
func relAge(sec int64) string {
	switch {
	case sec < 60:
		return "just now"
	case sec < 3600:
		return fmt.Sprintf("%dm ago", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh ago", sec/3600)
	default:
		return fmt.Sprintf("%dd ago", sec/86400)
	}
}
