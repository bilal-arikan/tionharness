package compact

import (
	"strings"
	"testing"
)

func TestDisabledIsPassthrough(t *testing.T) {
	in := "a\na\nb\n"
	out, st := Compact(in, Options{Enabled: false, Dedupe: true})
	if out != in || st.Applied {
		t.Fatalf("disabled must be a no-op: out=%q applied=%v", out, st.Applied)
	}
}

func TestDedupeConsecutive(t *testing.T) {
	in := "warn\nwarn\nwarn\nok\nwarn"
	out, st := Compact(in, Options{Enabled: true, Dedupe: true})
	if !strings.Contains(out, "warn  (×3)") {
		t.Fatalf("expected collapsed run, got:\n%s", out)
	}
	// The trailing single "warn" must survive uncollapsed.
	if !strings.HasSuffix(out, "\nwarn") {
		t.Fatalf("trailing single line lost:\n%s", out)
	}
	if !st.Applied || st.AfterBytes >= st.BeforeBytes {
		t.Fatalf("expected savings, stats=%+v", st)
	}
}

func TestCollapseBlankRuns(t *testing.T) {
	in := "a\n\n\n\nb"
	out, _ := Compact(in, Options{Enabled: true})
	if out != "a\n\nb" {
		t.Fatalf("blank run not collapsed: %q", out)
	}
}

func TestElideMiddleLines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line")
		b.WriteByte('\n')
	}
	// Distinct lines so dedupe doesn't collapse them.
	lines := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		lines = append(lines, "ln-"+string(rune('A'+i%26))+itoa(i))
	}
	in := strings.Join(lines, "\n")
	out, st := Compact(in, Options{Enabled: true, MaxLines: 10})
	got := strings.Count(out, "\n") + 1
	if got > 11 { // 10 kept + 1 marker
		t.Fatalf("too many lines kept: %d\n%s", got, out)
	}
	if !strings.Contains(out, "satır atlandı") {
		t.Fatalf("missing elision marker:\n%s", out)
	}
	if !st.Applied {
		t.Fatalf("expected applied")
	}
	// Head and tail must be preserved.
	if !strings.HasPrefix(out, lines[0]) {
		t.Fatalf("head lost:\n%s", out)
	}
	if !strings.HasSuffix(out, lines[len(lines)-1]) {
		t.Fatalf("tail lost:\n%s", out)
	}
}

func TestTruncateMiddleBytesUTF8(t *testing.T) {
	// Multi-byte runes (Turkish) to ensure we never split one.
	in := strings.Repeat("çğüş-", 2000) // well over the cap
	out, st := Compact(in, Options{Enabled: true, MaxBytes: 512})
	if len(out) > 512 {
		t.Fatalf("over byte cap: %d", len(out))
	}
	if !utf8Valid(out) {
		t.Fatalf("produced invalid UTF-8")
	}
	if !strings.Contains(out, "kırpıldı") {
		t.Fatalf("missing truncation marker:\n%s", out)
	}
	if st.Saved() == 0 {
		t.Fatalf("expected bytes saved")
	}
}

func TestNoChangeReportsNotApplied(t *testing.T) {
	in := "single clean line"
	out, st := Compact(in, Options{Enabled: true, Dedupe: true, MaxLines: 100, MaxBytes: 1000})
	if out != in || st.Applied {
		t.Fatalf("clean input should be unchanged: out=%q applied=%v", out, st.Applied)
	}
}

// itoa is a tiny helper to avoid importing strconv in the test table builder.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// utf8Valid reports whether s contains only valid UTF-8 (no split runes).
func utf8Valid(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}
