package tools

import (
	"strings"
	"testing"
)

// TestTreeBudgetLine covers the coordinator-facing quota line appended to a
// spawn_worker result: it stays silent when no ceiling is configured, reports
// remaining capacity when one is, and escalates its warning at the 75% and 90%
// thresholds so a coordinator sees the wall coming instead of only hitting it.
func TestTreeBudgetLine(t *testing.T) {
	cases := []struct {
		name       string
		used, tot  int
		wantEmpty  bool
		wantSubstr []string
		noSubstr   []string
	}{
		{name: "unlimited", used: 5, tot: 0, wantEmpty: true},
		{name: "unlimited-negative", used: 5, tot: -1, wantEmpty: true},
		{
			name: "healthy", used: 2, tot: 10,
			wantSubstr: []string{"2/10", "8 remaining"},
			noSubstr:   []string{"⚠️"},
		},
		{
			name: "warn-75", used: 8, tot: 10,
			wantSubstr: []string{"8/10", "Over 75%"},
			noSubstr:   []string{"Over 90%"},
		},
		{
			name: "warn-90", used: 9, tot: 10,
			wantSubstr: []string{"9/10", "Over 90%"},
			noSubstr:   []string{"Over 75%"},
		},
		{
			// At capacity the line still renders (0 remaining) and carries the 90% warning.
			name: "full", used: 10, tot: 10,
			wantSubstr: []string{"10/10", "0 remaining", "Over 90%"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := treeBudgetLine(c.used, c.tot)
			if c.wantEmpty {
				if got != "" {
					t.Fatalf("expected no budget line, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("expected a budget line for %d/%d, got empty", c.used, c.tot)
			}
			for _, sub := range c.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("budget line %q missing %q", got, sub)
				}
			}
			for _, sub := range c.noSubstr {
				if strings.Contains(got, sub) {
					t.Errorf("budget line %q should not contain %q", got, sub)
				}
			}
		})
	}
}
