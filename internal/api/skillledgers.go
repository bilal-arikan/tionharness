package api

import (
	"sync"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// skillLedgerStore holds the per-session use_skill dedupe ledgers so a skill body
// is sent once per session (per fold epoch) instead of on every turn that
// re-reads it. Both provider paths share one instance per session: the native
// tool reads it off the turn context, the claude-cli / codex-cli bridge reads it
// off the run. In-memory, exactly like permGrantStore — a restart re-serves every
// body once, which is correct because a restarted CLI thread has lost them.
//
// Keyed by scopeKey(workspaceID, sessionID) for the same reason grants are:
// session ids repeat across workspace stores and must never share state.
type skillLedgerStore struct {
	mu        sync.Mutex
	bySession map[string]*tools.SkillLedger
}

func newSkillLedgerStore() *skillLedgerStore {
	return &skillLedgerStore{bySession: map[string]*tools.SkillLedger{}}
}

// forSession returns the ledger for a session, creating it on first use.
func (s *skillLedgerStore) forSession(wsID, sessionID string) *tools.SkillLedger {
	key := scopeKey(wsID, sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.bySession[key]
	if l == nil {
		l = tools.NewSkillLedger()
		s.bySession[key] = l
	}
	return l
}
