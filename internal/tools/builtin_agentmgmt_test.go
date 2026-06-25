package tools

import "testing"

// TestRepairMojibake covers the UTF-8→Latin-1 repair used when persisting agent
// fields: corrupted emoji/prose is restored, while genuine glyphs and real text
// (including Turkish, whose ş/ğ/ı sit above U+00FF) are left untouched.
func TestRepairMojibake(t *testing.T) {
	// Mojibake inputs are built from the original UTF-8 bytes (each byte as a
	// rune ≤ 0xFF) — pasting the rendered mojibake as a literal would silently
	// drop the non-printable C1 bytes (0x9F/0x97/0x8F) and misrepresent the case.
	mojibake := func(b ...byte) string {
		rs := make([]rune, len(b))
		for i, x := range b {
			rs[i] = rune(x)
		}
		return string(rs)
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		// 🗺️ = F0 9F 97 BA EF B8 8F
		{"map emoji mojibake", mojibake(0xF0, 0x9F, 0x97, 0xBA, 0xEF, 0xB8, 0x8F), "🗺️"},
		// ✈️ = E2 9C 88 EF B8 8F
		{"plane emoji mojibake", mojibake(0xE2, 0x9C, 0x88, 0xEF, 0xB8, 0x8F), "✈️"},
		// 🏨 = F0 9F 8F A8
		{"hotel emoji mojibake", mojibake(0xF0, 0x9F, 0x8F, 0xA8), "🏨"},
		// © = C2 A9
		{"copyright mojibake", mojibake(0xC2, 0xA9), "©"},
		{"clean emoji untouched", "🗺️", "🗺️"},
		{"ascii untouched", "Hotel Researcher", "Hotel Researcher"},
		{"turkish untouched", "Şehir gezgini ığ", "Şehir gezgini ığ"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := repairMojibake(c.in); got != c.want {
				t.Errorf("repairMojibake(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
