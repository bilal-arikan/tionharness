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

// DetectUnbackedSpawnClaim reports whether a coordinator turn claims delegation
// without any tool call in the persisted turn trace.
func DetectUnbackedSpawnClaim(text string, toolCallCount int, coordinatorMode bool) bool {
	if !coordinatorMode || toolCallCount > 0 || strings.TrimSpace(text) == "" {
		return false
	}

	lower := strings.ToLower(text)
	for _, sentence := range spawnClaimSentencePattern.FindAllString(lower, -1) {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || strings.HasSuffix(sentence, "?") || hasSpawnClaimFuture(sentence) {
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

func hasSpawnClaimFuture(sentence string) bool {
	for _, pattern := range spawnClaimFuturePatterns {
		if pattern.MatchString(sentence) {
			return true
		}
	}
	return false
}
