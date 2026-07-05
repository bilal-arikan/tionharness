package api

import (
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// claudeResumePlan captures the claude-cli resume decision for one turn so the
// caller can persist the rotated session id afterwards. See planClaudeResume.
type claudeResumePlan struct {
	active    bool // resume engaged this turn (toggle on + single-agent claude-cli)
	sentCount int  // raw message count the CLI will know AFTER this turn's user msg
}

// planClaudeResume decides whether to resume the claude-cli session for this turn
// and, if so, mutates llmReq to carry the resume id and send only the delta (the
// messages the CLI has not seen yet) instead of the full transcript — so the CLI
// reuses its warm prompt cache. Opt-in (ClaudeResume setting), claude-cli only,
// and single-agent only (the resume id is tracked per session, so a multi-agent
// thread would collide). rawHistory is the un-annotated message list (it includes
// this turn's just-added user message). Returns a plan whose sentCount is stored
// after the turn together with the rotated Response.SessionID.
func (s *Server) planClaudeResume(provider providers.Provider, agentCount int, session db.Session, rawHistory []db.Message, llmReq *providers.Request) claudeResumePlan {
	set := s.settings.Get()
	enabled := resumeGateEnabled(set.ClaudeResume, set.ClaudePersistentSession, agentCount, provider.Name())
	plan, resumeID, deltaStart := claudeResumeDecision(enabled, session.CLISessionID, session.CLISentMsgCount, len(rawHistory))
	if resumeID != "" {
		// Warm resume: send only the unseen delta and ask the CLI to --resume.
		llmReq.ResumeSessionID = resumeID
		llmReq.Messages = conversation.ToProviderMessages(rawHistory[deltaStart:])
	}
	return plan
}

// resumeGateEnabled is the pure (testable) gate for the --resume delta path. It is
// MUTUALLY EXCLUSIVE with ClaudePersistentSession: the persistent long-lived process
// holds the conversation itself and needs the FULL transcript on a cold (re)start, so
// when it is on we must NOT trim to the delta here — persistent supersedes --resume.
// Also single-agent only (the resume id is tracked per session, so a multi-agent
// thread would collide) and claude-cli only.
func resumeGateEnabled(claudeResume, persistentSession bool, agentCount int, providerName string) bool {
	return claudeResume && !persistentSession && agentCount == 1 && providerName == "claude-cli"
}

// claudeResumeDecision is the pure (testable) core of planClaudeResume. Given the
// gate result and the session's recorded resume boundary, it returns the plan to
// persist after the turn plus, when a warm resume applies, the id to --resume and
// the index in rawHistory where the unseen delta begins. A zero resumeID means a
// cold start (send the full transcript, capture a fresh id). The warm path engages
// only when a prior id exists, the boundary is in (0, rawLen], and there is at
// least one unseen message — otherwise it falls back to cold (e.g. after edits
// shrank the history past the boundary).
func claudeResumeDecision(enabled bool, cliSessionID string, sentCount, rawLen int) (plan claudeResumePlan, resumeID string, deltaStart int) {
	if !enabled {
		return claudeResumePlan{}, "", 0
	}
	plan = claudeResumePlan{active: true, sentCount: rawLen}
	if cliSessionID != "" && sentCount > 0 && sentCount < rawLen {
		return plan, cliSessionID, sentCount
	}
	return plan, "", 0
}
