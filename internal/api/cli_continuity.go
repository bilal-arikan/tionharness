package api

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// A coordinator authors worker prompts but does not own its CLI persona.
// Relax the participant gate only when the transcript proves one responder.
func workerResumeOwner(session db.Session, responder string, history []db.Message) bool {
	if session.Kind != "worker" || session.CoordinatorSessionID == "" || session.AgentID != responder {
		return false
	}
	for _, m := range history {
		if m.Role != providers.RoleAssistant {
			continue
		}
		author := strings.TrimSpace(m.AgentID)
		if author == "" && m.AuthorKind == db.AuthorAgent {
			author = strings.TrimSpace(m.AuthorID)
		}
		if author == "" || author != responder || (m.AuthorKind == db.AuthorAgent && m.AuthorID != "" && m.AuthorID != responder) {
			return false
		}
	}
	for _, participant := range db.SessionParticipants(session) {
		if participant != responder && !workerPromptAuthor(participant, responder, history) {
			return false
		}
	}
	return true
}

func workerPromptAuthor(participant, responder string, history []db.Message) bool {
	for _, m := range history {
		if m.Role == providers.RoleUser && m.AuthorKind == db.AuthorAgent && m.AuthorID == participant && m.RecipientID == responder {
			return true
		}
	}
	return false
}

func cliContinuationStep(plan cliResumePlan) *agent.TurnStep {
	if plan.reason == "" {
		return nil
	}
	status := "disabled"
	if plan.active {
		status = "cold"
		if !plan.coldStart {
			status = "warm"
		}
	}
	return &agent.TurnStep{Kind: agent.StepText, Operation: "cli_resume", Status: status, Reason: plan.reason,
		Text: "CLI context continuity", Provider: "codex-cli"}
}
