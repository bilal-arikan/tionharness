package agent

import (
	"regexp"
	"strings"
)

var spawnClaimSentencePattern = regexp.MustCompile(`[^.!?\n]+[.!?]?`)

var spawnClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bworker\b.*(?:başladı|başlat(?:tım|tık|tı|ıldı|ılmış|ıldı)|spawn)`),
	regexp.MustCompile(`\bvalidator\b.*(?:başladı|başlat(?:tım|tık|tı|ıldı|ılmış|ıldı)|spawn)`),
	regexp.MustCompile(`\bspawned\b`),
	regexp.MustCompile(`\bspawning\b`),
	regexp.MustCompile(`\bstarted\s+(?:a\s+)?(?:worker|validator)\b`),
	regexp.MustCompile(`\bdelege\b`),
}

var spawnClaimFuturePatterns = []*regexp.Regexp{
	regexp.MustCompile(`başlat(?:acağım|acağız|acak|ılacak)`),
	regexp.MustCompile(`\b(?:will|going\s+to)\s+(?:start|spawn|delegate)\b`),
}

var spawnClaimNegativePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bworker\b.*başlatma(?:dım|dık|dı|mış)`),
	regexp.MustCompile(`\bvalidator\b.*başlatma(?:dım|dık|dı|mış)`),
	regexp.MustCompile(`\bspawn\s+etme(?:dim|dik|di|miş)`),
	regexp.MustCompile(`\b(?:did\s+not|didn't)\s+(?:start|spawn|delegate)\b`),
	regexp.MustCompile(`\bno\s+(?:worker|validator)\s+(?:was|has\s+been)\s+(?:started|spawned)\b`),
}

var (
	spawnClaimFencedCode = regexp.MustCompile("(?s)```.*?```")
	spawnClaimInlineCode = regexp.MustCompile("`[^`\n]*`")
	spawnClaimQuoteLine  = regexp.MustCompile(`(?m)^\s*>.*$`)
)

// DetectUnbackedSpawnClaim reports whether a coordinator turn claims delegation
// without a matching spawn tool invocation in the persisted turn trace.
func DetectUnbackedSpawnClaim(text string, toolNames []string, coordinatorMode bool) bool {
	if !coordinatorMode || hasSpawnToolCall(toolNames) || strings.TrimSpace(text) == "" {
		return false
	}

	lower := strings.ToLower(stripSpawnClaimMarkdown(text))
	for _, sentence := range spawnClaimSentencePattern.FindAllString(lower, -1) {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || strings.HasSuffix(sentence, "?") || hasSpawnClaimFuture(sentence) || hasSpawnClaimNegative(sentence) {
			continue
		}
		for _, pattern := range spawnClaimPatterns {
			if pattern.MatchString(sentence) {
				return true
			}
		}
	}
	return false
}

func hasSpawnToolCall(toolNames []string) bool {
	for _, name := range toolNames {
		segments := strings.Split(strings.ToLower(name), "__")
		switch segments[len(segments)-1] {
		case "spawn_worker", "run_subagent", "spawn_session":
			return true
		}
	}
	return false
}

func stripSpawnClaimMarkdown(text string) string {
	text = spawnClaimFencedCode.ReplaceAllString(text, "")
	text = spawnClaimInlineCode.ReplaceAllString(text, "")
	return spawnClaimQuoteLine.ReplaceAllString(text, "")
}

func hasSpawnClaimFuture(sentence string) bool {
	for _, pattern := range spawnClaimFuturePatterns {
		if pattern.MatchString(sentence) {
			return true
		}
	}
	return false
}

func hasSpawnClaimNegative(sentence string) bool {
	for _, pattern := range spawnClaimNegativePatterns {
		if pattern.MatchString(sentence) {
			return true
		}
	}
	return false
}
