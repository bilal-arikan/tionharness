package providers

import (
	"strings"
	"testing"
)

func TestSanitizeTranscriptText_StripsHarnessMarkup(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Tamamdır. Sıra sütunu güncellendi.", "Tamamdır. Sıra sütunu güncellendi."},
		{"reminder", "before <system-reminder>The assistant message is malformed</system-reminder> after", "before  after"},
		{"stray tags", "ok</parameter></parameter></function_results>", "ok"},
		{"function block", "x<function_calls><invoke name=\"Edit\"></invoke></function_calls>y", "xy"},
		{"no markup with angle", "a < b and c > d", "a < b and c > d"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeTranscriptText(c.in); got != c.want {
				t.Fatalf("sanitizeTranscriptText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsRepairArtifact(t *testing.T) {
	if !isRepairArtifact("<system-reminder>malformed</system-reminder> ...") {
		t.Fatal("expected reminder text to be flagged")
	}
	if !isRepairArtifact("noise </function_results> tail") {
		t.Fatal("expected stray function_results tag to be flagged")
	}
	if isRepairArtifact("a normal answer with no markup") {
		t.Fatal("clean answer must not be flagged")
	}
}

func TestSerializeTranscript_SanitizesAssistantTurns(t *testing.T) {
	out := serializeTranscript([]Message{
		{Role: RoleUser, Text: "ilk soru"},
		{Role: RoleAssistant, Text: "cevap</parameter></function_results>"},
		{Role: RoleUser, Text: "5. satırdaki sıra numarasını 9999 yap"},
	})
	if strings.Contains(out, "</function_results>") || strings.Contains(out, "</parameter>") {
		t.Fatalf("transcript still contains leaked markup:\n%s", out)
	}
	if !strings.Contains(out, "Assistant: cevap") {
		t.Fatalf("sanitized assistant text missing:\n%s", out)
	}
	if !strings.Contains(out, "5. satırdaki sıra numarasını 9999 yap") {
		t.Fatalf("final user message missing:\n%s", out)
	}
}
