package textutil

import "testing"

func TestFoldLowerMapsDottedCapitalI(t *testing.T) {
	if got := FoldLower("İstanbul LOGIN"); got != "istanbul login" {
		t.Fatalf("FoldLower = %q", got)
	}
	if got := FoldLower("already lower"); got != "already lower" {
		t.Fatalf("ascii lower should pass through, got %q", got)
	}
}

func TestIndexFoldMatchesAcrossCase(t *testing.T) {
	cases := []struct {
		hay, needle string
		want        int
	}{
		{"Deploy the Gateway", "gateway", 11},
		{"İstanbul'da bulgu var", "istanbul", 0},
		{"no match here", "zzz", -1},
		{"ÇAĞRI merkezi", "çağri", 0}, // ASCII 'I' folds to 'i', multi-byte runes fold too
		{"abc", "", 0},
	}
	for _, c := range cases {
		if got := IndexFold(c.hay, c.needle); got != c.want {
			t.Errorf("IndexFold(%q, %q) = %d, want %d", c.hay, c.needle, got, c.want)
		}
	}
}

func TestCountFoldCountsNonOverlapping(t *testing.T) {
	if got := CountFold("spam Spam SPAM spamspam", "spam"); got != 5 {
		t.Fatalf("CountFold = %d, want 5", got)
	}
	if got := CountFold("İİİ", "i"); got != 3 {
		t.Fatalf("CountFold over multi-byte runes = %d, want 3", got)
	}
	if got := CountFold("anything", ""); got != 0 {
		t.Fatalf("empty needle should count 0, got %d", got)
	}
}
