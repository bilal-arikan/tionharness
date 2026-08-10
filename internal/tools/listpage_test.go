package tools

import (
	"encoding/json"
	"sort"
	"testing"
)

func TestPageArgs(t *testing.T) {
	cases := []struct {
		name   string
		limit  int
		offset int
		wantL  int
		wantO  int
	}{
		{"zeroes default to page 1", 0, 0, 20, 0},
		{"negative limit defaults", -3, 0, 20, 0},
		{"explicit small page", 5, 0, 5, 0},
		{"over max is capped", 500, 0, 100, 0},
		{"negative offset clamped", 10, -2, 10, 0},
		{"normal paging", 20, 40, 20, 40},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, o := PageArgs(c.limit, c.offset)
			if l != c.wantL || o != c.wantO {
				t.Fatalf("PageArgs(%d,%d) = (%d,%d), want (%d,%d)", c.limit, c.offset, l, o, c.wantL, c.wantO)
			}
		})
	}
}

func TestSortOrder(t *testing.T) {
	field, asc, err := SortOrder("")
	if err != nil || field != "updated" || asc {
		t.Fatalf(`SortOrder("") = (%q,%v,%v), want ("updated",false,nil)`, field, asc, err)
	}
	field, asc, err = SortOrder("name_asc")
	if err != nil || field != "name" || !asc {
		t.Fatalf(`SortOrder("name_asc") = (%q,%v,%v)`, field, asc, err)
	}
	for _, bad := range []string{"updated", "desc", "foo_bar", "name_x", "UPDATED_DESC", "updated_asc_extra"} {
		if _, _, err := SortOrder(bad); err == nil {
			t.Fatalf("SortOrder(%q): want error, got nil", bad)
		}
	}
}

func TestSlicePage(t *testing.T) {
	all := []int{0, 1, 2, 3, 4}
	page, total := SlicePage(all, 0, 2)
	if total != 5 || len(page) != 2 || page[0] != 0 || page[1] != 1 {
		t.Fatalf("first page = %v (total %d), want [0 1] (total 5)", page, total)
	}
	page, total = SlicePage(all, 4, 2)
	if total != 5 || len(page) != 1 || page[0] != 4 {
		t.Fatalf("last page = %v (total %d), want [4] (total 5)", page, total)
	}
	page, total = SlicePage(all, 10, 2)
	if total != 5 || len(page) != 0 {
		t.Fatalf("offset past end = %v (total %d), want [] (total 5)", page, total)
	}
	page, total = SlicePage([]int{}, 0, 20)
	if total != 0 || len(page) != 0 || page == nil {
		t.Fatalf("empty slice page = %#v (total %d), want non-nil empty (total 0)", page, total)
	}
}

func TestPageResultEnvelope(t *testing.T) {
	out, err := pageResult([]map[string]any{{"id": "A"}}, 25, 0, 20)
	if err != nil {
		t.Fatalf("pageResult: %v", err)
	}
	var env struct {
		Items   []map[string]any `json:"items"`
		Total   int              `json:"total"`
		Offset  int              `json:"offset"`
		Limit   int              `json:"limit"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Total != 25 || env.Offset != 0 || env.Limit != 20 || !env.HasMore {
		t.Fatalf("envelope = %+v, want total=25 offset=0 limit=20 hasMore=true", env)
	}
	out, _ = pageResult([]int{}, 0, 0, 20)
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Total != 0 || env.HasMore {
		t.Fatalf("empty page envelope = %s (err %v), want total=0 hasMore=false", out, err)
	}
}

type sortRow struct {
	ID  string
	Upd int64
}

// sortIDs sorts a copy of rows by the resolved key and returns the id sequence.
func sortIDs(t *testing.T, rows []sortRow, field string, asc bool) []string {
	t.Helper()
	cp := append([]sortRow(nil), rows...)
	less, err := SortByField(cp, field, asc,
		func(r sortRow) int64 { return r.Upd },
		func(r sortRow) int64 { return 0 },
		func(r sortRow) string { return r.ID },
		func(r sortRow) string { return r.ID },
	)
	if err != nil {
		t.Fatalf("SortByField: %v", err)
	}
	sort.SliceStable(cp, less)
	out := make([]string, len(cp))
	for i, r := range cp {
		out[i] = r.ID
	}
	return out
}

func TestSortByFieldTiesAreDeterministic(t *testing.T) {
	// TSK68 review findings, both about EQUAL sort keys:
	//
	//  1. The desc branch used to negate the ascending comparison, so equal keys
	//     compared "i < j" in both directions — no strict weak ordering.
	//  2. Ties then fell through to sort.SliceStable's input order, which is Go
	//     MAP-ITERATION order (see db.dbList): randomized per call. Since paging
	//     issues offset=0 and offset=20 as separate calls, a reader skipped rows
	//     and saw others twice.
	//
	// The id tiebreak fixes both: ties resolve by id, so the order is identical
	// no matter how the store handed the rows over, and asc/desc stay inverses.
	rows := []sortRow{{"a", 100}, {"b", 100}, {"c", 100}, {"d", 100}}
	shuffled := []sortRow{{"c", 100}, {"a", 100}, {"d", 100}, {"b", 100}}

	asc := sortIDs(t, rows, "updated", true)
	if want := []string{"a", "b", "c", "d"}; !slicesEqual(asc, want) {
		t.Fatalf("asc with equal keys = %v, want %v", asc, want)
	}
	// A different arrival order must produce the SAME result — this is the
	// property that makes paging safe.
	if got := sortIDs(t, shuffled, "updated", true); !slicesEqual(got, asc) {
		t.Fatalf("arrival order leaked into the result: %v vs %v", got, asc)
	}
	// desc is the exact reverse of asc.
	desc := sortIDs(t, rows, "updated", false)
	if want := []string{"d", "c", "b", "a"}; !slicesEqual(desc, want) {
		t.Fatalf("desc with equal keys = %v, want %v", desc, want)
	}
}

// TestSortByFieldRequiresID: the tiebreak is mandatory, so no future call site
// can silently reintroduce map-order-dependent paging.
func TestSortByFieldRequiresID(t *testing.T) {
	_, err := SortByField([]sortRow{}, "updated", true,
		func(r sortRow) int64 { return r.Upd },
		func(r sortRow) int64 { return 0 },
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("want error when the id tiebreak is missing")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSplitTagsAndHasAllTags(t *testing.T) {
	got := SplitTags(" a , b ,, c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("SplitTags = %v, want [a b c]", got)
	}
	if SplitTags("") != nil {
		t.Fatal("SplitTags(\"\") should be nil")
	}
	if !HasAllTags([]string{"x", "a", "b"}, []string{"a", "b"}) {
		t.Fatal("HasAllTags([x a b], [a b]) should be true")
	}
	if HasAllTags([]string{"x", "a"}, []string{"a", "b"}) {
		t.Fatal("HasAllTags([x a], [a b]) should be false")
	}
}
