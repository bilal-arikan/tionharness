package decider

// WithWorkflowDefaults supplies bounded limits at the workflow's point of use.
// Legacy authorities can retain omitted fields in their persisted configuration.
func (ac AuthorityConfig) WithWorkflowDefaults() AuthorityConfig {
	if ac.CandidateLimit == 0 {
		ac.CandidateLimit = 32
	}
	ac.CandidateLimit = min(max(ac.CandidateLimit, 4), 48)
	if ac.SelectionLimit == 0 {
		ac.SelectionLimit = 8
	}
	ac.SelectionLimit = min(max(ac.SelectionLimit, 1), 16)
	if ac.RemindEvery == 0 {
		ac.RemindEvery = 3
	}
	ac.RemindEvery = min(max(ac.RemindEvery, 1), 20)
	if ac.ContextBudget == 0 {
		ac.ContextBudget = 8192
	}
	ac.ContextBudget = min(max(ac.ContextBudget, 1024), 32768)
	return ac
}
