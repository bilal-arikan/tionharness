package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestUsageCallKindQualifiesSystemAgentWithoutLosingOperation(t *testing.T) {
	ctx := WithCallKind(context.Background(), KindTitle)
	if got, err := usageCallKind(ctx, db.Agent{System: true, SystemKey: "titler"}); err != nil || got != "system:titler:title" {
		t.Fatalf("usageCallKind(system titler) = %q, %v, want %q, nil", got, err, "system:titler:title")
	}
	if got, err := usageCallKind(ctx, db.Agent{}); err != nil || got != db.UsageKindTitle {
		t.Fatalf("usageCallKind(regular agent) = %q, %v, want %q, nil", got, err, db.UsageKindTitle)
	}
}

func TestUsageCallKindRejectsUnknownSystemAgentKey(t *testing.T) {
	if _, err := usageCallKind(context.Background(), db.Agent{System: true, SystemKey: "unknown"}); err == nil {
		t.Fatal("usageCallKind accepted unknown system agent key")
	}
}

func TestResolveTitleConfigCarriesSystemActorForUsage(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	got, _ := rt.resolveTitleConfig(db.Agent{ID: "calling-agent", Model: "session-model"})
	if !got.System || got.SystemKey != "titler" {
		t.Fatalf("resolved titler actor = system:%v key:%q, want system titler", got.System, got.SystemKey)
	}
	if got.ID != "calling-agent" {
		t.Fatalf("resolved titler agent ID = %q, want calling agent billing ID", got.ID)
	}
}
