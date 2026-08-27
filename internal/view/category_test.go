package view

import (
	"fmt"
	"strings"
	"testing"
)

// sessionMemberHandles builds n session handles for a category fixture.
func sessionMemberHandles(n int) []Handle {
	hs := make([]Handle, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("SES%d", i)
		hs = append(hs, Handle{Label: "session:" + id, Ref: Ref{Kind: KindSession, ID: id}, Level: LevelCard})
	}
	return hs
}

func TestCategoryHeaderCountsMembers(t *testing.T) {
	v, err := ProjectCategory(CategoryInput{ID: CategorySessions, Members: sessionMemberHandles(12)}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "OTURUMLAR · 12 oturum") {
		t.Errorf("category header wrong: %q", v.Header)
	}
	if len(v.Handles) != 12 {
		t.Errorf("handles = %d, want 12", len(v.Handles))
	}
}

func TestCategoryCapsAtTopNAndCountsElided(t *testing.T) {
	v, err := ProjectCategory(CategoryInput{ID: CategorySessions, Members: sessionMemberHandles(categoryTopN + 7)}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(v.Handles) != categoryTopN {
		t.Errorf("handles = %d, want cap %d", len(v.Handles), categoryTopN)
	}
	// The COUNT stays honest (header names every member) even though the handle
	// list is capped — the elision is reported, never silent.
	if !strings.Contains(v.Header, fmt.Sprintf("%d oturum", categoryTopN+7)) {
		t.Errorf("header must count all members: %q", v.Header)
	}
	if v.Elided != 7 || v.ElidedUnit != "oturum" {
		t.Errorf("elision wrong: elided=%d unit=%q", v.Elided, v.ElidedUnit)
	}
}

func TestCategoryColumnLabelsByKey(t *testing.T) {
	v, err := ProjectCategory(CategoryInput{ID: categoryColumnPrefix + "in_progress", Members: nil}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "SÜTUN:in_progress · 0 kart") {
		t.Errorf("column category header wrong: %q", v.Header)
	}
	// An empty bucket says so rather than rendering a blank body.
	if !strings.Contains(v.Text(), "bu kategoride kart yok") {
		t.Errorf("empty category not stated:\n%s", v.Text())
	}
}

func TestCategoryUnknownIdIsRejected(t *testing.T) {
	if _, err := ProjectCategory(CategoryInput{ID: "galaxy"}, LevelCard); err == nil {
		t.Error("an unknown category id must be an error, not a blank node")
	}
}

func TestCategoryTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectCategory(CategoryInput{ID: CategoryAgents, Members: sessionMemberHandles(3)}, LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" || len(v.Handles) != 0 {
		t.Errorf("tiny level must not emit body/handles: body=%q handles=%d", v.Body, len(v.Handles))
	}
}

// TestCategoryFullListsMembersCardDoesNot pins the budget tier: at card level a
// category's members are handles only (clickable in the panel, invisible in the
// text), so card and full used to render identical DSL and the panel's `full`
// button did nothing.
func TestCategoryFullListsMembersCardDoesNot(t *testing.T) {
	in := CategoryInput{ID: CategorySessions, Members: []Handle{
		{Label: "auth refactor", Ref: Ref{Kind: KindSession, ID: "SES1"}},
		{Label: "board planner", Ref: Ref{Kind: KindSession, ID: "SES2"}},
	}}

	card, err := ProjectCategory(in, LevelCard)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	full, err := ProjectCategory(in, LevelFull)
	if err != nil {
		t.Fatalf("full: %v", err)
	}

	if strings.Contains(card.Text(), "SES1") {
		t.Errorf("card level spelled a member out; it should stay handle-only:\n%s", card.Text())
	}
	for _, want := range []string{"auth refactor", "session:SES1", "board planner", "session:SES2"} {
		if !strings.Contains(full.Text(), want) {
			t.Errorf("full level missing %q:\n%s", want, full.Text())
		}
	}
	if len(full.Handles) != 2 {
		t.Errorf("full dropped the handles (%d), they stay clickable", len(full.Handles))
	}
}

// TestCategoryFullElisionAddsToTopNOverflow: a member list too long for the byte
// budget must ADD to the topN overflow, not replace it — both are members the
// view is not showing, and reporting one of them understates the gap.
func TestCategoryFullElisionAddsToTopNOverflow(t *testing.T) {
	members := make([]Handle, 0, categoryTopN+7)
	for i := 0; i < categoryTopN+7; i++ {
		members = append(members, Handle{
			Label: strings.Repeat("uzun-oturum-basligi-", 6),
			Ref:   Ref{Kind: KindSession, ID: "SES" + string(rune('A'+i%26))},
		})
	}

	v, err := ProjectCategory(CategoryInput{ID: CategorySessions, Members: members}, LevelFull)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if v.Elided <= 7 {
		t.Errorf("Elided = %d, want > 7 (topN overflow + byte-budget drop)", v.Elided)
	}
	if v.ElidedUnit != "oturum" {
		t.Errorf("ElidedUnit = %q, want \"oturum\"", v.ElidedUnit)
	}
	if !strings.Contains(v.Text(), "gizlendi") {
		t.Errorf("elision not rendered:\n%s", v.Text())
	}
}
