package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeSkillWriter records the last UpdateSkill call so tests can assert the
// tool forwards exactly the fields the caller supplied (nil = unchanged).
type fakeSkillWriter struct {
	gotSlug                                   string
	name, description, whenToUse, group, body *string
	shared                                    *bool
	updateCalled                              bool
}

func (f *fakeSkillWriter) CreateSkill(string, string, string, string, string, string, bool) error {
	return nil
}
func (f *fakeSkillWriter) DeleteSkill(string) error { return nil }
func (f *fakeSkillWriter) ImportSkill(source, location, slug string, shared bool) (SkillImportResult, error) {
	return SkillImportResult{Slug: "imported"}, nil
}
func (f *fakeSkillWriter) UpdateSkill(slug string, name, description, whenToUse, group, body *string, shared *bool) error {
	f.updateCalled = true
	f.gotSlug, f.name, f.description, f.whenToUse, f.group, f.body, f.shared = slug, name, description, whenToUse, group, body, shared
	return nil
}

func TestUpdateSkill_PartialForwarding(t *testing.T) {
	f := &fakeSkillWriter{}
	tool := NewUpdateSkillTool(f)

	// Only body supplied → only body pointer is non-nil; the rest stay nil
	// (unchanged) so the store keeps their current values.
	_, err := tool.Call(context.Background(), json.RawMessage(`{"slug":"weekly-report","body":"new body"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.updateCalled || f.gotSlug != "weekly-report" {
		t.Fatalf("update not forwarded: called=%v slug=%q", f.updateCalled, f.gotSlug)
	}
	if f.body == nil || *f.body != "new body" {
		t.Fatalf("body should be forwarded, got %v", f.body)
	}
	if f.name != nil || f.description != nil || f.whenToUse != nil || f.group != nil || f.shared != nil {
		t.Fatalf("omitted fields must stay nil (unchanged), got name=%v desc=%v when=%v group=%v shared=%v",
			f.name, f.description, f.whenToUse, f.group, f.shared)
	}
}

func TestUpdateSkill_Validation(t *testing.T) {
	tool := NewUpdateSkillTool(&fakeSkillWriter{})

	if _, err := tool.Call(context.Background(), json.RawMessage(`{"slug":""}`)); err == nil {
		t.Fatal("expected error for empty slug")
	}
	// slug present but no editable field → reject (avoids a no-op rewrite).
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"slug":"x"}`)); err == nil {
		t.Fatal("expected error when no fields are provided")
	}
}
