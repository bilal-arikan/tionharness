package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestDeciderDebugReadOnlyEmptyAndBoundedFilters(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	for _, path := range []string{"/api/decider/debug", "/api/decider/debug?days=1&limit=1&ref=SES1", "/api/decider/debug?traceId=trace&instance=DM1"} {
		rec := deciderRequest(t, s, http.MethodGet, path, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"events":[]`) || !strings.Contains(rec.Body.String(), `"retainedEvents":0`) {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, query := range []string{"days=0", "days=91", "days=no", "limit=0", "limit=5001", "ref=" + strings.Repeat("x", 129)} {
		rec := deciderRequest(t, s, http.MethodGet, "/api/decider/debug?"+query, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d", query, rec.Code)
		}
	}
}
