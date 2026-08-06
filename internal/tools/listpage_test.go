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

func TestSortByFieldDescTiesPreserveOrder(t *testing.T) {
	// TSK68 review finding: the desc branch used to negate the ascending
	// comparison, which made EQUAL keys compare "i < j" in both directions (no
	// strict weak ordering). sort.SliceStable then reversed equal-key rows
	// (a b c d → d c b a) instead of preserving their order, and asc/desc were
	// not inverses. Regression: with all timestamps equal, both directions must
	// keep the original order.
	type row struct {
		ID  string
		Upd int64
	}
	rows := []row{{"a", 100}, {"b", 100}, {"c", 100}, {"d", 100}}
	less, err := SortByField(rows, "updated", false,
		func(r row) int64 { return r.Upd },
		func(r row) int64 { return 0 },
		nil,
	)
	if err != nil {
		t.Fatalf("SortByField: %v", err)
	}
	sort.SliceStable(rows, less)
	want := []string{"a", "b", "c", "d"}
	for i, r := range rows {
		if r.ID != want[i] {
			t.Fatalf("desc with equal keys = %v, want %v (ties must keep original order)",
				[]string{rows[0].ID, rows[1].ID, rows[2].ID, rows[3].ID}, want)
		}
	}
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
