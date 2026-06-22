package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchMessages(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s1, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "Deploy notes"})
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "Random chat"})

	d.AddMessage(ctx, Message{SessionID: s1.ID, Role: "user", Text: "How do we deploy the gateway service?"})
	d.AddMessage(ctx, Message{SessionID: s1.ID, Role: "assistant", Text: "Use docker compose to deploy it.", AgentID: agent.ID})
	d.AddMessage(ctx, Message{SessionID: s2.ID, Role: "user", Text: "totally unrelated message"})

	// Single term across sessions.
	hits, _ := d.SearchMessages(ctx, SearchOpts{Query: "deploy"})
	if len(hits) != 2 {
		t.Fatalf("deploy: want 2 hits, got %d (%v)", len(hits), hits)
	}

	// AND semantics: both terms must appear in the same message.
	hits, _ = d.SearchMessages(ctx, SearchOpts{Query: "deploy gateway"})
	if len(hits) != 1 || hits[0].SessionID != s1.ID {
		t.Fatalf("deploy gateway: want 1 hit in s1, got %d (%v)", len(hits), hits)
	}

	// Role filter.
	hits, _ = d.SearchMessages(ctx, SearchOpts{Query: "deploy", Roles: []string{"assistant"}})
	if len(hits) != 1 || hits[0].Role != "assistant" {
		t.Fatalf("role filter: want 1 assistant hit, got %d (%v)", len(hits), hits)
	}

	// ExcludeID.
	hits, _ = d.SearchMessages(ctx, SearchOpts{Query: "message", ExcludeID: s2.ID})
	if len(hits) != 0 {
		t.Fatalf("exclude: want 0 hits, got %d (%v)", len(hits), hits)
	}

	// Empty query → no hits.
	if hits, _ := d.SearchMessages(ctx, SearchOpts{Query: "   "}); hits != nil {
		t.Fatalf("empty query: want nil, got %v", hits)
	}

	// Limit.
	for i := 0; i < 5; i++ {
		d.AddMessage(ctx, Message{SessionID: s2.ID, Role: "user", Text: "spam spam spam"})
	}
	hits, _ = d.SearchMessages(ctx, SearchOpts{Query: "spam", Limit: 3})
	if len(hits) != 3 {
		t.Fatalf("limit: want 3 hits, got %d", len(hits))
	}
}

func TestSearchSnippetRuneSafe(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	defer d.Close()
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	long := strings.Repeat("çğşüöı ", 80) + "BULGU " + strings.Repeat("çğşüöı ", 80)
	d.AddMessage(ctx, Message{SessionID: s.ID, Role: "user", Text: long})

	hits, _ := d.SearchMessages(ctx, SearchOpts{Query: "bulgu"})
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if !strings.Contains(strings.ToLower(hits[0].Snippet), "bulgu") {
		t.Fatalf("snippet should contain the match: %q", hits[0].Snippet)
	}
	if !utf8Valid(hits[0].Snippet) {
		t.Fatalf("snippet broke UTF-8: %q", hits[0].Snippet)
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
