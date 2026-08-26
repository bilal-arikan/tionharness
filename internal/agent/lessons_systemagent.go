package agent

import "github.com/bilal-arikan/tionharness/internal/db"

func (r *Runtime) resolveLessonConfig(agent db.Agent) (db.Agent, string, error) {
	return r.resolveAnalysisSystemAgent("lesson-extractor", agent)
}

func (r *Runtime) resolveInsightConfig(agent db.Agent) (db.Agent, string, error) {
	return r.resolveAnalysisSystemAgent("insight", agent)
}

func (r *Runtime) resolveAnalysisSystemAgent(key string, agent db.Agent) (db.Agent, string, error) {
	systemAgent, _, err := r.ResolveSystemAgent(key)
	if err != nil {
		promptKey := "lesson"
		if key == "insight" {
			promptKey = "insight-analyzer"
		}
		r.logger.Warn("analysis system agent resolution failed; using embedded behavior", "systemKey", key, "error", err)
		return agent, r.readPrompt(promptKey), nil
	}

	// Keep provider credentials and the billing agent ID on the calling agent.
	// Only model, prompt, and usage actor identity come from the system agent.
	agent.Model = systemAgent.Model
	agent.System = true
	agent.SystemKey = systemAgent.SystemKey
	return agent, systemAgent.Soul, nil
}
