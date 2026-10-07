package view

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestGraphTimes(t *testing.T) {
	store := &fakeStore{
		agents:    []db.Agent{{ID: "AG1", CreatedAt: 10, UpdatedAt: 20}},
		sessions:  []db.Session{{ID: "SES1", AgentID: "AG1", CreatedAt: 30, UpdatedAt: 40}},
		tasks:     []db.Task{{ID: "T1", BoardState: db.BoardInProgress, CreatedAt: 50, UpdatedAt: 60}},
		artifacts: []db.Artifact{{ID: "ART1", CreatedAt: 70, UpdatedAt: 80}},
	}
	reads := map[string]db.ViewRead{
		"artifact:ART1": {At: 90, AgentID: "AG1"},
		"budget:budget": {At: 95, AgentID: "AG1"},
	}
	times, err := NewProjector(store).GraphTimes(context.Background(), reads)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]GraphTimes{
		"agent:AG1":      {Created: 10, Updated: 20},
		"session:SES1":   {Created: 30, Updated: 40},
		"board:board#T1": {Created: 50, Updated: 60},
		"artifact:ART1":  {Created: 70, Updated: 80, Read: 90, ReadBy: "AG1"},
		"budget:budget":  {Read: 95, ReadBy: "AG1"},
	}
	for key, w := range want {
		if got := times[key]; got != w {
			t.Errorf("%s = %+v, want %+v", key, got, w)
		}
	}
}
