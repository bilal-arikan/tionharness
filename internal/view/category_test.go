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
	v, err := ProjectCategory(CategoryInput{ID: CategorySessions, Members: sessionMemberHandles(12)}, LevelCard, LensHealth)
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
	v, err := ProjectCategory(CategoryInput{ID: CategorySessions, Members: sessionMemberHandles(categoryTopN + 7)}, LevelCard, LensHealth)
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
	v, err := ProjectCategory(CategoryInput{ID: categoryColumnPrefix + "in_progress", Members: nil}, LevelCard, LensHealth)
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
	if _, err := ProjectCategory(CategoryInput{ID: "galaxy"}, LevelCard, LensHealth); err == nil {
		t.Error("an unknown category id must be an error, not a blank node")
	}
}

func TestCategoryTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectCategory(CategoryInput{ID: CategoryAgents, Members: sessionMemberHandles(3)}, LevelTiny, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" || len(v.Handles) != 0 {
		t.Errorf("tiny level must not emit body/handles: body=%q handles=%d", v.Body, len(v.Handles))
	}
}
