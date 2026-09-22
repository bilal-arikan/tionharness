package db

import (
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/archive"
)

func TestAgentAssignableErr(t *testing.T) {
	live := Agent{ID: "agt_live", Name: "Live"}
	arch := Agent{ID: "agt_arch", Name: "Old", Archived: true}

	if err := live.AssignableErr(""); err != nil {
		t.Fatalf("live agent on create: %v", err)
	}
	if err := arch.AssignableErr(""); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("archived agent on create: want ErrArchived, got %v", err)
	}
	if err := arch.AssignableErr("agt_other"); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("switching to archived agent: want ErrArchived, got %v", err)
	}
	if err := arch.AssignableErr("agt_arch"); err != nil {
		t.Fatalf("keeping the stored archived target must be allowed: %v", err)
	}
}
