package conversation

import (
	"strings"
	"testing"
)

// TestEstimateTextDensity verifies the density-aware estimator (CG-9): packed/
// encoded blobs are counted at ~1.5 chars/token, prose at ~3, so dense tool
// output can't silently undercount and overflow the budget.
func TestEstimateTextDensity(t *testing.T) {
	// Natural prose: plenty of whitespace → ~3 chars/token.
	prose := strings.Repeat("the quick brown fox jumps over ", 40) // ~1240 runes
	proseTokens := estimateText(prose)
	if want := len([]rune(prose)) / charsPerToken; proseTokens < want-2 || proseTokens > want+2 {
		t.Fatalf("prose: got %d, expected ~%d (%d chars/token)", proseTokens, want, charsPerToken)
	}

	// Dense base64-like blob: no whitespace → ~1.5 chars/token (runes*2/3).
	dense := strings.Repeat("aGVsbG93b3JsZA", 100) // 1400 runes, zero spaces
	denseTokens := estimateText(dense)
	if want := len([]rune(dense)) * 2 / 3; denseTokens < want-2 || denseTokens > want+2 {
		t.Fatalf("dense: got %d, expected ~%d (1.5 chars/token)", denseTokens, want)
	}

	// The same byte length scores far more tokens when dense — the whole point.
	if denseTokens <= estimateText(strings.Repeat("ab cd ef ", 155)) {
		t.Fatalf("dense estimate (%d) should exceed equal-length prose", denseTokens)
	}

	// Short strings stay on the prose ratio even with no whitespace (not worth it).
	short := "abcdefghijklmnop" // 16 runes, < denseMinRunes
	if got, want := estimateText(short), len([]rune(short))/charsPerToken+1; got != want {
		t.Fatalf("short dense-looking string: got %d, want %d (prose ratio)", got, want)
	}

	// Empty string is one token, never zero.
	if estimateText("") != 1 {
		t.Fatalf("empty: want 1")
	}
}
