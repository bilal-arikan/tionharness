package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// mutationTools are the built-in tools that let a worker CHANGE something on
// disk: write or edit a file, apply a patch, or run an arbitrary command. An
// agent whose allowlist contains none of these can read, search and reason, but
// it cannot produce a file — however the task is phrased.
var mutationTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"apply_patch":  true,
	"Bash":         true,
	"PowerShell":   true,
	"run_code":     true,
	"write_config": true,
}

// mutationTaskPatterns detect a task that CANNOT be satisfied without writing.
// Deliberately narrow: each pattern needs a write verb bound to a concrete file
// artifact (an extension or an explicit "file"/"dosya" object) or a git write
// command. "Implement", "fix" and bare "write" are NOT here — a planner is
// routinely asked to plan an implementation or write a summary in its reply, and
// blocking those spawns would be worse than the problem this guard solves.
//
// Turkish and English both appear because worker briefs in this workspace are
// written in Turkish while the tool surface is English.
var mutationTaskPatterns = []*regexp.Regexp{
	// "... .md dosyasını yaz / oluştur / güncelle / düzenle", also without the
	// extension: "brief dosyasını oluştur".
	regexp.MustCompile(`(?i)\bdosya(?:sı|sını|yı|ya|nın)?\b[^.\n]{0,60}\b(yaz|olu[sş]tur|g[uü]ncelle|d[uü]zenle|kaydet)`),
	regexp.MustCompile(`(?i)\b(yaz|olu[sş]tur|g[uü]ncelle|d[uü]zenle|kaydet)[a-zçğıöşü]*\b[^.\n]{0,60}\bdosya`),
	// English mirror: "write/create/update the <name>.md file".
	regexp.MustCompile(`(?i)\b(write|create|update|edit|save)\b[^.\n]{0,60}\bfile\b`),
	// A write verb aimed at a concrete path/extension in either language.
	regexp.MustCompile(`(?i)\b(yaz|olu[sş]tur|g[uü]ncelle|d[uü]zenle|kaydet|write|create|update|edit|save)[a-zçğıöşü]*\b[^\n]{0,60}\.(md|go|ts|tsx|js|jsx|json|ya?ml|py|txt|css|html|sh|ps1)\b`),
	// Version-control writes: unambiguous, and always need Bash.
	regexp.MustCompile(`(?i)\bgit\s+(commit|push|add|checkout|merge|rebase|stash)\b`),
	regexp.MustCompile(`(?i)\b(commit|push)\s+(et|edip|edecek|yap)`),
}

// agentMutationTools returns the mutation tools an agent may actually call, and
// whether its allowlist constrains it at all. An empty allowlist means
// "everything is allowed" (the user-facing default — see allowFunc), so such an
// agent is reported as unconstrained with no need to enumerate.
//
// A malformed allowlist is an ERROR, not "assume unrestricted": the same
// document drives the real tool filter, where an unreadable allowlist denies
// every tool. Treating it as permissive here would tell the coordinator a worker
// can write when it can do nothing at all.
func agentMutationTools(a db.Agent) (allowed []string, constrained bool, err error) {
	raw := strings.TrimSpace(a.AllowedTools)
	if raw == "" {
		return nil, false, nil
	}
	var patterns []string
	if uerr := json.Unmarshal([]byte(raw), &patterns); uerr != nil {
		return nil, true, fmt.Errorf("agent %s has an unreadable allowed_tools list: %w", a.ID, uerr)
	}
	if len(patterns) == 0 {
		return nil, false, nil
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "*" {
			return nil, false, nil
		}
		if mutationTools[p] {
			allowed = append(allowed, p)
			continue
		}
		// A trailing-wildcard pattern can still cover a mutation tool (the same
		// prefix rule patternPredicate applies).
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			for name := range mutationTools {
				if strings.HasPrefix(name, prefix) {
					allowed = append(allowed, name)
				}
			}
		}
	}
	sort.Strings(allowed)
	return allowed, true, nil
}

// taskNeedsMutation reports the first unmistakable write requirement found in a
// task brief, or "" when the brief reads as read-only work.
func taskNeedsMutation(task string) string {
	for _, re := range mutationTaskPatterns {
		if m := re.FindString(task); m != "" {
			return strings.TrimSpace(m)
		}
	}
	return ""
}

// checkWorkerCapability rejects a spawn whose brief plainly requires writing
// while the target agent has no tool that can write. Returning an error here
// costs the coordinator one corrected re-spawn; NOT returning it cost SES2570
// roughly 70 minutes and six workers, all re-assigning a file-writing brief to
// a read-only planner that answered "the file is not on disk" every time.
//
// The guard only fires on the narrow patterns above; anything ambiguous spawns
// as before. When it does fire, the error names the phrase that triggered it and
// the agents that could run the task, so the fix is a single re-target.
func (r *Runtime) checkWorkerCapability(a db.Agent, task string) error {
	allowed, constrained, err := agentMutationTools(a)
	if err != nil {
		return err
	}
	if !constrained || len(allowed) > 0 {
		return nil
	}
	trigger := taskNeedsMutation(task)
	if trigger == "" {
		return nil
	}
	return fmt.Errorf(
		"agent %q cannot run this task: the brief requires writing (%q) but the agent's allowed tools "+
			"contain no write or exec tool — it can only read, search and report. Re-target this brief at an "+
			"agent that has Write/Edit/Bash, or split it so %q only produces the analysis and a write-capable "+
			"agent creates the file",
		a.Name, trigger, a.Name)
}
