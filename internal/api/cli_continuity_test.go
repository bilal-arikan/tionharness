package api

import (
	"context"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"testing"
)

func TestWorkerCodexResumeIgnoresPromptAuthorButRejectsOtherResponder(t *testing.T) {
	session := db.Session{ID: "SES1", Kind: "worker", AgentID: "AGT1", CoordinatorSessionID: "SES0", Participants: []string{"AGT0", "AGT1"}, CLISessionID: "thread-1", CLISentMsgCount: 2}
	raw := []db.Message{
		{Role: providers.RoleUser, AuthorKind: db.AuthorAgent, AuthorID: "AGT0", RecipientID: "AGT1"},
		{Role: providers.RoleAssistant, AgentID: "AGT1", AuthorKind: db.AuthorAgent, AuthorID: "AGT1"},
		{Role: providers.RoleUser, AuthorKind: db.AuthorAgent, AuthorID: "AGT0", RecipientID: "AGT1"},
	}
	agent := db.Agent{ID: "AGT1", Provider: "codex-cli", Model: "gpt-test"}
	for _, tc := range []struct {
		name      string
		assistant db.Message
		want      bool
	}{
		{"worker responder", raw[1], true},
		{"other responder", db.Message{Role: providers.RoleAssistant, AgentID: "AGT0"}, false},
		{"unknown legacy responder", db.Message{Role: providers.RoleAssistant}, false},
		{"contradictory author", db.Message{Role: providers.RoleAssistant, AgentID: "AGT1", AuthorKind: db.AuthorAgent, AuthorID: "AGT0"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history := append([]db.Message(nil), raw...)
			history[1] = tc.assistant
			req := providers.Request{System: "persona", Messages: []providers.Message{{Role: providers.RoleUser, Text: "full history"}}}
			plan := (&Server{}).planCodexResume(context.Background(), &scopedResumeTestProvider{ready: true, can: true}, 1, session, agent, history, false, &req)
			if plan.active != tc.want {
				t.Fatalf("plan = %+v", plan)
			}
			if tc.want && (req.ResumeSessionID != "thread-1" || len(req.Messages) != 1 || plan.reason != "continued") {
				t.Fatalf("warm delta not used: %+v %+v", plan, req)
			}
		})
	}
	session.Kind = "chat"
	if workerResumeOwner(session, "AGT1", raw) {
		t.Fatal("multi-persona chat bypassed ownership gate")
	}
}
