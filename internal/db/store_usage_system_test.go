package db

import "testing"

func TestSystemUsageKindPreservesOrigin(t *testing.T) {
	tests := map[string]string{
		UsageKindTitle:   "system:titler:title",
		UsageKindReflect: "system:titler:reflect",
		"":               "system:titler:other",
	}
	for input, want := range tests {
		got, err := SystemAgentUsageKind("titler", input)
		if err != nil || got != want {
			t.Errorf("SystemUsageKind(%q, %q) = %q, %v, want %q, nil", "titler", input, got, err, want)
		}
	}
}

func TestSystemUsageKindRejectsUnknownKey(t *testing.T) {
	if _, err := SystemAgentUsageKind("", UsageKindTitle); err == nil {
		t.Fatal("SystemAgentUsageKind accepted empty system key")
	}
}
