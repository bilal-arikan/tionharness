package decider

// StateBudget gives workflow callers room for evidence after questions and
// wire overhead. Use the smallest configured primary/fallback/comparison window
// so richer context does not disable smaller decision models.
func (h *Hub) StateBudget(authority string, questions map[string]Question) int {
	contextTokens := 0
	if h != nil {
		cfg := h.Config()
		ac := cfg.Authorities[authority]
		for _, id := range []string{h.effectiveModel(cfg, ac.Model), ac.Fallback, ac.Challenger} {
			m, ok := h.Model(id)
			if !ok {
				continue
			}
			tokens := m.ContextTokens
			if tokens <= 0 {
				for _, backend := range Manifests() {
					if backend.ID == m.Backend {
						tokens = backend.ContextTokens
						break
					}
				}
			}
			if tokens > 0 && (contextTokens == 0 || tokens < contextTokens) {
				contextTokens = tokens
			}
		}
	}
	return max(1024, stateBudgetBytes(questions, contextTokens)-512)
}
