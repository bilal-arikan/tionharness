package agent

// PromptEpochStale inspects the frozen state after exercising live epoch APIs.
func (r *Runtime) PromptEpochStale(sessionID, agentID string) bool {
	if sessionID == "" || !r.PromptEpochEnabled() {
		return false
	}
	r.epochMu.Lock()
	defer r.epochMu.Unlock()
	if entries, ok := r.epochCache[sessionID]; ok {
		if epoch := entries[agentID]; epoch != nil {
			return epoch.systemStale || epoch.toolsStale
		}
	}
	return false
}
