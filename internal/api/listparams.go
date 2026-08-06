package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// listparams.go — the shared query-parameter contract behind every list
// endpoint (TSK68). The endpoints accept the same limit/offset/sort params as
// the agent list_* tools; when any of them is present the response becomes the
// standard {items,total,offset,limit,hasMore} envelope, and when none is
// present the endpoint keeps its legacy behavior (the full list, unwrapped) so
// existing UI clients never break.
//
// Malformed values are 400 errors, never silently ignored.

// listQueryParams reads limit/offset/sort from a query string. listing is true
// when at least one of them was given. limit is capped at the tool-layer
// maximum; offset is clamped at 0 (mirrors tools.PageArgs).
func listQueryParams(q url.Values) (limit, offset int, sortField string, sortAsc bool, listing bool, err error) {
	limitStr, hasLimit := q["limit"]
	offsetStr, hasOffset := q["offset"]
	sortStr, hasSort := q["sort"]
	if !hasLimit && !hasOffset && !hasSort {
		return 0, 0, "", false, false, nil
	}
	if hasLimit {
		v := strings.TrimSpace(limitStr[0])
		if v == "" {
			return 0, 0, "", false, false, fmt.Errorf("limit must be a positive integer")
		}
		n, e := strconv.Atoi(v)
		if e != nil || n <= 0 {
			return 0, 0, "", false, false, fmt.Errorf("limit must be a positive integer, got %q", v)
		}
		limit = n
	}
	if hasOffset {
		v := strings.TrimSpace(offsetStr[0])
		if v == "" {
			offset = 0
		} else {
			n, e := strconv.Atoi(v)
			if e != nil || n < 0 {
				return 0, 0, "", false, false, fmt.Errorf("offset must be a non-negative integer, got %q", v)
			}
			offset = n
		}
	}
	limit, offset = tools.PageArgs(limit, offset)
	if hasSort {
		field, asc, e := tools.SortOrder(sortStr[0])
		if e != nil {
			return 0, 0, "", false, false, e
		}
		sortField, sortAsc = field, asc
	}
	return limit, offset, sortField, sortAsc, true, nil
}

// pageJSONResponse writes the standard listing envelope (the API twin of the
// agent tools' pageResult), so a client paging through an endpoint gets the
// same shape it would from the corresponding list_* tool.
func pageJSONResponse(w http.ResponseWriter, items any, total, offset, limit int) {
	writeJSON(w, http.StatusOK, struct {
		Items   any  `json:"items"`
		Total   int  `json:"total"`
		Offset  int  `json:"offset"`
		Limit   int  `json:"limit"`
		HasMore bool `json:"hasMore"`
	}{Items: items, Total: total, Offset: offset, Limit: limit, HasMore: offset+limit < total})
}

// boolQuery parses a boolean query parameter ("1", "true" → true; "0",
// "false" → false). An empty value means "not given". Anything else is an
// error — a typo must not silently flip the filter.
func boolQuery(q url.Values, key string) (v *bool, given bool, err error) {
	raw, ok := q[key]
	if !ok {
		return nil, false, nil
	}
	s := strings.ToLower(strings.TrimSpace(raw[0]))
	switch s {
	case "1", "true":
		b := true
		return &b, true, nil
	case "0", "false":
		b := false
		return &b, true, nil
	}
	return nil, false, fmt.Errorf("%s must be true or false, got %q", key, raw[0])
}
