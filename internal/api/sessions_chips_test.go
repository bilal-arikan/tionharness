package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

type chipPage struct {
	Items      []db.Session   `json:"items"`
	Total      int            `json:"total"`
	HasMore    bool           `json:"hasMore"`
	ChipCounts map[string]int `json:"chipCounts"`
}

func TestParseSessionIDsIsBounded(t *testing.T) {
	parts := make([]string, maxSessionLookupIDs+10)
	for i := range parts {
		parts[i] = fmt.Sprintf("SES%d", i)
	}

	ids := parseSessionIDs(strings.Join(parts, ","))
	if len(ids) != maxSessionLookupIDs {
		t.Fatalf("parsed %d ids, want %d", len(ids), maxSessionLookupIDs)
	}
	if ids[parts[maxSessionLookupIDs]] {
		t.Fatalf("parsed id beyond the %d-item bound", maxSessionLookupIDs)
	}
}

func listSessionsPage(t *testing.T, server *Server, wsp *db.DB, query string) chipPage {
	t.Helper()
	req := withTestWS(httptest.NewRequest("GET", "/api/sessions?"+query, nil), wsp)
	rec := httptest.NewRecorder()
	server.handleListSessions(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var page chipPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return page
}

// The sidebar pages the list, so the chip selection must narrow it BEFORE the
// page window: with 20 workers ahead of 5 chats, a chat-only selection paged at
// 3 has to report 5 chats and hasMore, not "3 of 25 mixed rows".
func TestListSessionsChipFilterPagesFilteredSet(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if _, err := wsp.DB.CreateSession(ctx, db.Session{
			Kind: "worker", Title: fmt.Sprintf("worker-%d", i),
			Category: db.CategoryWorker, CoordinatorSessionID: "SES1",
		}); err != nil {
			t.Fatalf("create worker: %v", err)
		}
	}
	for i := 0; i < 5; i++ {
		if _, err := wsp.DB.CreateSession(ctx, db.Session{Title: fmt.Sprintf("chat-%d", i)}); err != nil {
			t.Fatalf("create chat: %v", err)
		}
	}

	page := listSessionsPage(t, server, wsp.DB, "chips=chat&limit=3&offset=0")
	if page.Total != 5 {
		t.Fatalf("total = %d, want 5 (chat sessions only)", page.Total)
	}
	if len(page.Items) != 3 || !page.HasMore {
		t.Fatalf("items = %d, hasMore = %v; want 3 and true", len(page.Items), page.HasMore)
	}
	for _, s := range page.Items {
		if s.Kind != "" && s.Kind != "chat" {
			t.Fatalf("unexpected kind %q in chat-only page", s.Kind)
		}
	}

	rest := listSessionsPage(t, server, wsp.DB, "chips=chat&limit=3&offset=3")
	if len(rest.Items) != 2 || rest.HasMore {
		t.Fatalf("second page items = %d, hasMore = %v; want 2 and false", len(rest.Items), rest.HasMore)
	}

	// Counts are reported over the whole set, so an unticked chip still shows
	// how many rows it is hiding.
	if got := page.ChipCounts["worker"]; got != 20 {
		t.Fatalf("chipCounts[worker] = %d, want 20", got)
	}
	if got := page.ChipCounts["chat"]; got != 5 {
		t.Fatalf("chipCounts[chat] = %d, want 5", got)
	}
}

// A worker (or archived) session needs its scope chip on top of its kind chip,
// exactly like the sidebar predicate.
func TestListSessionsChipScopeChips(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	for _, s := range []db.Session{
		{Title: "plain chat"},
		{Title: "worker chat", CoordinatorSessionID: "SES1"},
		{Title: "archived chat", State: "archived"},
	} {
		if _, err := wsp.DB.CreateSession(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	only := listSessionsPage(t, server, wsp.DB, "chips=chat&limit=50")
	if only.Total != 1 || only.Items[0].Title != "plain chat" {
		t.Fatalf("chips=chat returned %d rows (%+v), want only the plain chat", only.Total, only.Items)
	}

	withScopes := listSessionsPage(t, server, wsp.DB, "chips=chat,worker,archived&limit=50")
	if withScopes.Total != 3 {
		t.Fatalf("chips=chat,worker,archived total = %d, want 3", withScopes.Total)
	}
}

// An empty chips value is a real selection (everything unticked) and must match
// nothing; omitting the parameter keeps the unfiltered list.
func TestListSessionsChipsEmptyVersusAbsent(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Title: "chat"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if got := listSessionsPage(t, server, wsp.DB, "chips=&limit=50").Total; got != 0 {
		t.Fatalf("empty chips total = %d, want 0", got)
	}
	if got := listSessionsPage(t, server, wsp.DB, "limit=50").Total; got != 1 {
		t.Fatalf("absent chips total = %d, want 1", got)
	}
}

func TestListSessionsChipCountsRespectNonChipScope(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	for _, s := range []db.Session{
		{Title: "active chat", State: "active"},
		{Title: "archived chat", State: "archived"},
		{Title: "active flow", Kind: "flow", State: "active"},
	} {
		if _, err := wsp.DB.CreateSession(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	page := listSessionsPage(t, server, wsp.DB, "state=active&chips=chat&limit=50")
	if got := page.ChipCounts["chat"]; got != 1 {
		t.Fatalf("chipCounts[chat] = %d, want 1 in active state scope", got)
	}
	if got := page.ChipCounts["flow"]; got != 1 {
		t.Fatalf("chipCounts[flow] = %d, want 1 in active state scope", got)
	}
	if got := page.ChipCounts["archived"]; got != 0 {
		t.Fatalf("chipCounts[archived] = %d, want 0 outside active state scope", got)
	}

	kindPage := listSessionsPage(t, server, wsp.DB, "kind=flow&chips=flow&limit=50")
	if got := kindPage.ChipCounts["chat"]; got != 0 {
		t.Fatalf("kind-scoped chipCounts[chat] = %d, want 0", got)
	}
}

func TestListSessionsBoundedExactIDs(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	first, err := wsp.DB.CreateSession(ctx, db.Session{Title: "first"})
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Title: "second"}); err != nil {
		t.Fatalf("create second session: %v", err)
	}

	page := listSessionsPage(t, server, wsp.DB, "ids="+first.ID+"&limit=2")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != first.ID {
		t.Fatalf("exact lookup = %+v, want only %s", page.Items, first.ID)
	}
}

func TestSessionChipKeyClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		sess db.Session
		want string
	}{
		{name: "empty kind is chat", sess: db.Session{}, want: "chat"},
		{name: "category wins", sess: db.Session{Kind: "worker", Category: db.CategorySubagent}, want: chipSubagent},
		{name: "schedule rolls into automation", sess: db.Session{Kind: "schedule-run"}, want: "automation"},
		{name: "unknown kind falls to other", sess: db.Session{Kind: "quantum"}, want: chipOther},
		{name: "insight keeps its own chip", sess: db.Session{Kind: db.SessionKindInsight}, want: "insight"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionChipKey(test.sess); got != test.want {
				t.Fatalf("sessionChipKey = %q, want %q", got, test.want)
			}
		})
	}
}
