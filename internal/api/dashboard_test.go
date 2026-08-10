package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func dashboardFixture(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.CreateAgent(ctx, db.Agent{Name: "builder"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "iş"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := database.CreateTask(ctx, db.Task{Title: "kart", BoardState: db.BoardTodo}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := database.CreateFlow(ctx, db.Flow{Name: "f", Graph: `{"start":"s","nodes":[{"id":"s","type":"start"}]}`}); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	if _, err := database.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1", Status: db.FlowSuccess}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return database
}

// TestDashboardReturnsSeriesAndProjection pins the shape the overview screen
// depends on: counters, a full-width day series, categorical breakdowns and the
// workspace projection text.
func TestDashboardReturnsSeriesAndProjection(t *testing.T) {
	database := dashboardFixture(t)

	rec := serveFlowRuns((&Server{}).handleDashboard, database, "/api/dashboard?days=7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeView(t, rec.Body.Bytes())

	counters, _ := got["counters"].(map[string]any)
	if counters == nil {
		t.Fatalf("no counters in %s", rec.Body.String())
	}
	for key, want := range map[string]float64{"agents": 1, "sessions": 1, "tasks": 1, "runs": 1} {
		if n, _ := counters[key].(float64); n != want {
			t.Errorf("counters[%s] = %v, want %v", key, counters[key], want)
		}
	}

	// The x-axis is always the full window, so a quiet day shows as a gap rather
	// than silently disappearing from the trend.
	series, _ := got["sessionsByDay"].([]any)
	if len(series) != 7 {
		t.Errorf("sessionsByDay has %d buckets, want 7 (quiet days must still appear)", len(series))
	}

	summary, _ := got["summary"].(map[string]any)
	text, _ := summary["text"].(string)
	if !strings.Contains(text, "WORKSPACE") {
		t.Errorf("workspace projection missing from summary:\n%s", text)
	}
	if n, _ := summary["tokens"].(float64); n <= 0 {
		t.Errorf("summary token estimate not reported: %v", summary["tokens"])
	}
}

// TestDashboardClampsWindow keeps a hostile or mistyped ?days= from turning the
// overview into an unbounded scan.
func TestDashboardClampsWindow(t *testing.T) {
	database := dashboardFixture(t)

	for _, q := range []string{"days=0", "days=9999", "days=abc", ""} {
		rec := serveFlowRuns((&Server{}).handleDashboard, database, "/api/dashboard?"+q, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%q -> status %d: %s", q, rec.Code, rec.Body.String())
		}
		got := decodeView(t, rec.Body.Bytes())
		series, _ := got["sessionsByDay"].([]any)
		if len(series) < 1 || len(series) > dashboardMaxDays {
			t.Errorf("%q produced %d buckets, outside 1..%d", q, len(series), dashboardMaxDays)
		}
	}
}

// TestDayKeysCoverTheWindowOldestFirst pins the axis contract the charts rely on.
func TestDayKeysCoverTheWindowOldestFirst(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	keys := dayKeys(3, now)

	want := []string{"2026-08-02", "2026-08-03", "2026-08-04"}
	if len(keys) != len(want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("keys[%d] = %s, want %s", i, keys[i], want[i])
		}
	}
}

// TestBucketByDayIgnoresOutOfWindowStamps: a session created last month must not
// be folded into the oldest visible bucket, which would fake a spike there.
func TestBucketByDayIgnoresOutOfWindowStamps(t *testing.T) {
	now := time.Now()
	stamps := []int64{
		now.Unix(),                    // today
		now.AddDate(0, 0, -1).Unix(),  // yesterday
		now.AddDate(0, 0, -30).Unix(), // outside a 7-day window
		0,                             // unset
	}
	points := bucketByDay(stamps, 7, now)

	var total int64
	for _, p := range points {
		total += p.Value
	}
	if total != 2 {
		t.Errorf("counted %d stamps, want 2 (out-of-window and unset must be dropped)", total)
	}
	if points[len(points)-1].Value != 1 {
		t.Errorf("today's bucket = %d, want 1", points[len(points)-1].Value)
	}
}

// TestDashboardSummaryMatchesWorkspaceView pins the claim _Docs/66 makes about
// the Panel screen: its summary block is not a prettier parallel rendering, it
// is byte-for-byte the workspace projection an agent reads. The two used to be
// built by differently-configured projectors — one carried the workspace name,
// the other did not — so the documented invariant was quietly false.
func TestDashboardSummaryMatchesWorkspaceView(t *testing.T) {
	database := dashboardFixture(t)
	srv := &Server{}

	dash := serveFlowRuns(srv.handleDashboard, database, "/api/dashboard?days=7", nil)
	if dash.Code != http.StatusOK {
		t.Fatalf("dashboard status %d: %s", dash.Code, dash.Body.String())
	}
	view := serveFlowRuns(srv.handleGetView, database,
		"/api/views/workspace/workspace?level=card&lens=health",
		map[string]string{"kind": "workspace", "id": "workspace"})
	if view.Code != http.StatusOK {
		t.Fatalf("view status %d: %s", view.Code, view.Body.String())
	}

	summary, _ := decodeView(t, dash.Body.Bytes())["summary"].(map[string]any)
	if summary == nil {
		t.Fatalf("no summary in %s", dash.Body.String())
	}
	got, _ := summary["text"].(string)
	want, _ := decodeView(t, view.Body.Bytes())["text"].(string)

	// asOf is a live clock and legitimately differs between the two calls; every
	// other byte must match.
	strip := func(s string) string {
		if i := strings.Index(s, " · asOf "); i >= 0 {
			if nl := strings.Index(s[i:], "\n"); nl >= 0 {
				return s[:i] + s[i+nl:]
			}
			return s[:i]
		}
		return s
	}
	if strip(got) != strip(want) {
		t.Errorf("dashboard summary and get_view{workspace} drifted apart:\n dashboard: %q\n get_view:  %q", got, want)
	}
}

// TestDashboardCountersComeFromTheProjection guards the other half of the same
// invariant: the stat tiles are the projection's own L0 pass, not a second tally
// written next to it. A card in a non-done column must show up as open.
func TestDashboardCountersComeFromTheProjection(t *testing.T) {
	database := dashboardFixture(t)

	rec := serveFlowRuns((&Server{}).handleDashboard, database, "/api/dashboard?days=7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	counters, _ := decodeView(t, rec.Body.Bytes())["counters"].(map[string]any)
	if counters == nil {
		t.Fatalf("no counters in %s", rec.Body.String())
	}
	for key, want := range map[string]float64{"tasksOpen": 1, "sessionsArchived": 0, "runsFailed": 0} {
		if n, ok := counters[key].(float64); !ok || n != want {
			t.Errorf("counters[%s] = %v, want %v", key, counters[key], want)
		}
	}
}
