package view

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestIDSummaryAdaptersPreserveOrderAndRemainder(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		max  int
		want string
	}{
		{"nil", nil, 3, ""},
		{"empty zero budget", []string{}, 0, ""},
		{"zero budget", []string{"B", "A"}, 0, " +2"},
		{"under budget", []string{"B", "A"}, 3, "B, A"},
		{"exact budget", []string{"B", "A"}, 2, "B, A"},
		{"over budget", []string{"B", "A", "C"}, 2, "B, A +1"},
		{"Unicode and blank ID", []string{"ğ", "", "😀"}, 3, "ğ, , 😀"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tasks []db.Task
			var sessions []db.Session
			var runs []db.FlowRun
			for _, id := range tc.ids {
				tasks = append(tasks, db.Task{ID: id})
				sessions = append(sessions, db.Session{ID: id})
				runs = append(runs, db.FlowRun{ID: id})
			}
			for name, got := range map[string]string{
				"tasks":    namesOf(tasks, tc.max),
				"sessions": sessionNames(sessions, tc.max),
				"runs":     runNames(runs, tc.max),
			} {
				if got != tc.want {
					t.Errorf("%s = %q, want %q", name, got, tc.want)
				}
			}
		})
	}
}
