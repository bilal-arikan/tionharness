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
		promptKey := analysisPromptKeys[key]
		if promptKey == "" {
			promptKey = "lesson"
		}
		r.logger.Warn("analysis system agent resolution failed; using embedded behavior", "systemKey", key, "error", err)
		return agent, r.readPrompt(promptKey), nil
	}

	return r.systemAgentExecutor(key, agent, systemAgent), systemAgent.Soul, nil
}

// pinsProvider reports whether the user explicitly chose a provider for this
// system agent (the "provider" key sits in its overrides and names a provider).
func pinsProvider(a db.Agent) bool {
	if a.Provider == "" {
		return false
	}
	for _, k := range a.Overrides {
		if k == "provider" {
			return true
		}
	}
	return false
}

// analysisPromptKeys maps an analysis system agent to the prompt-registry key
// its embedded fallback reads when the agent cannot be resolved.
var analysisPromptKeys = map[string]string{
	"lesson-extractor":  "lesson",
	"insight":           "insight-analyzer",
	"recipe-optimizer":  "recipe-optimizer",
	"stall-judge":       "stall-judge",
	"goal-writer":       "goal-writer",
	"workspace-evolver": "workspace-evolver",
}
