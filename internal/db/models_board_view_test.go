package db

import "testing"

func TestBoardViewValidate(t *testing.T) {
	base := BoardViewDef{ID: "my-view", Label: "Benim görünümüm"}

	t.Run("minimal view is valid", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("rejects a non-slug id", func(t *testing.T) {
		for _, id := range []string{"", "My View", "-leading", "üzgün", "a" + string(make([]byte, 70))} {
			v := base
			v.ID = id
			if err := v.Validate(); err == nil {
				t.Errorf("id %q should be rejected", id)
			}
		}
	})

	t.Run("requires a label", func(t *testing.T) {
		v := base
		v.Label = ""
		if err := v.Validate(); err == nil {
			t.Error("empty label should be rejected")
		}
	})

	t.Run("rejects unknown groupBy and sort", func(t *testing.T) {
		v := base
		v.GroupBy = "galaxy"
		if err := v.Validate(); err == nil {
			t.Error("unknown groupBy should be rejected")
		}
		v = base
		v.Sort = "vibes"
		if err := v.Validate(); err == nil {
			t.Error("unknown sort should be rejected")
		}
	})

	t.Run("rejects unknown filter enums", func(t *testing.T) {
		v := base
		v.Filter = BoardFilter{Dues: []string{"yesterday"}}
		if err := v.Validate(); err == nil {
			t.Error("unknown due bucket should be rejected")
		}
		v = base
		v.Filter = BoardFilter{Dep: "stuck"}
		if err := v.Validate(); err == nil {
			t.Error("unknown dep bucket should be rejected")
		}
		v = base
		// An empty priority means "unset", which is not a legal FILTER value:
		// selecting it would be indistinguishable from not filtering at all.
		v.Filter = BoardFilter{Priorities: []string{""}}
		if err := v.Validate(); err == nil {
			t.Error("empty priority should be rejected in a filter")
		}
	})

	t.Run("accepts free-form tags and columns", func(t *testing.T) {
		v := base
		v.Filter = BoardFilter{Tags: []string{"Büyük Iş"}, Columns: []string{"custom_col"}}
		if err := v.Validate(); err != nil {
			t.Fatalf("user-defined tags/columns must pass: %v", err)
		}
	})
}

func TestValidateBoardViewsRejectsDuplicateIDs(t *testing.T) {
	views := []BoardViewDef{
		{ID: "a", Label: "A"},
		{ID: "a", Label: "A tekrar"},
	}
	if err := ValidateBoardViews(views); err == nil {
		t.Fatal("duplicate ids should be rejected — 'update this view' would be ambiguous")
	}
}

func TestBoardFilterIsZero(t *testing.T) {
	if !(BoardFilter{}).IsZero() {
		t.Error("zero filter should report IsZero")
	}
	if (BoardFilter{Dues: []string{DueToday}}).IsZero() {
		t.Error("an active facet must not report IsZero")
	}
}
