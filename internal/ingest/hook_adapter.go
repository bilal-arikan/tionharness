package ingest

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/fetch"
	"github.com/bilal-arikan/tionswarm/internal/market"
	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// hookAdapter detects Claude Code plugin HOOK definitions (a `hooks` block in a
// plugin.json, a standalone hooks.json, or a settings.json) and converts each
// individual hook command into a native KindHook SwarmPack. The referenced hook
// scripts (${CLAUDE_PLUGIN_ROOT}/...) are bundled into Pack.Files so the install
// authority can materialise them locally and rewrite the placeholder. This is
// what lets a caveman-style package install its lifecycle hooks (SessionStart /
// UserPromptSubmit) — not just its skills and agents — in one import.
//
// Limitation: fetch strips node_modules/, so a hook script with external runtime
// dependencies is bundled without them; pure-stdlib scripts run as-is, otherwise
// the user runs the package manager in the materialised dir. Surfaced as a warning.
type hookAdapter struct{}

func (hookAdapter) Kind() string { return market.KindHook }

// ccHookCmd is one command inside a Claude Code hook entry.
type ccHookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// ccHookEntry groups commands under an optional matcher for one event.
type ccHookEntry struct {
	Matcher string      `json:"matcher"`
	Hooks   []ccHookCmd `json:"hooks"`
}

// pluginRootPlaceholder is the Claude Code variable that points at a plugin's
// install root; we keep it in the stored command and the installer rewrites it.
const pluginRootPlaceholder = "${CLAUDE_PLUGIN_ROOT}"

var pluginRootRefRe = regexp.MustCompile(`\$\{CLAUDE_PLUGIN_ROOT\}/([^\s"'` + "`" + `]+)`)

func (hookAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	files := fetch.FindFiles(tree, prefix, func(n string) bool {
		ln := strings.ToLower(n)
		return ln == "plugin.json" || ln == "hooks.json" || ln == "settings.json"
	})
	var out []Discovered
	for _, cfgPath := range files {
		hooksMap := extractHooksMap(tree[cfgPath])
		if len(hooksMap) == 0 {
			continue
		}
		root := pluginRootFor(cfgPath)
		// Deterministic event order for stable slugs/preview.
		events := make([]string, 0, len(hooksMap))
		for ev := range hooksMap {
			events = append(events, ev)
		}
		sort.Strings(events)
		for _, event := range events {
			for _, entry := range hooksMap[event] {
				for ci, cmd := range entry.Hooks {
					cmd := cmd
					if strings.TrimSpace(cmd.Command) == "" {
						continue
					}
					slug := hookSlug(event, cmd.Command, ci)
					name := event + ": " + hookLabel(cmd.Command)
					desc := "Imported " + event + " hook — " + cmd.Command
					files, warns := bundleHookScripts(tree, root, cmd.Command)
					matcher := strings.TrimSpace(entry.Matcher)
					timeout := cmd.Timeout
					command := cmd.Command
					relPath := cfgPath + "#" + event + "/" + slug
					out = append(out, Discovered{
						Key:         market.KindHook + ":" + relPath,
						Kind:        market.KindHook,
						Slug:        slug,
						Name:        name,
						Description: strings.TrimSpace(desc),
						RelPath:     relPath,
						Files:       sortedKeys(files),
						Warnings:    warns,
						build: func(opts Options) (market.Pack, error) {
							return market.Pack{
								Schema:      market.SchemaV1,
								ID:          market.KindHook + "." + slug,
								Kind:        market.KindHook,
								Name:        name,
								Description: strings.TrimSpace(desc),
								Version:     "1.0.0",
								Files:       files,
								Payload: market.Payload{Hook: &market.HookPayload{
									Event:      event,
									Matcher:    matcher,
									Command:    command,
									TimeoutSec: timeout,
								}},
							}, nil
						},
					})
				}
			}
		}
	}
	return out
}

// extractHooksMap parses a config blob and returns its event->entries map,
// accepting both the plugin.json shape ({"hooks": {"Event":[...]}}) and a bare
// hooks.json ({"Event":[...]}). A "hooks" field that is a string path (a
// reference to another file) is ignored here — the referenced file is scanned on
// its own by FindFiles when it matches hooks.json.
func extractHooksMap(blob []byte) map[string][]ccHookEntry {
	if len(blob) == 0 {
		return nil
	}
	// Shape 1: {"hooks": {event: [entry]}}
	var wrapped struct {
		Hooks json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(blob, &wrapped); err == nil && len(wrapped.Hooks) > 0 {
		var m map[string][]ccHookEntry
		if err := json.Unmarshal(wrapped.Hooks, &m); err == nil && hasHookCommands(m) {
			return m
		}
	}
	// Shape 2: bare {event: [entry]}
	var m map[string][]ccHookEntry
	if err := json.Unmarshal(blob, &m); err == nil && hasHookCommands(m) {
		return filterKnownEvents(m)
	}
	return nil
}

// hasHookCommands reports whether any entry carries at least one command — a
// guard so an unrelated JSON object with an event-like key isn't mistaken for a
// hooks map.
func hasHookCommands(m map[string][]ccHookEntry) bool {
	for _, entries := range m {
		for _, e := range entries {
			for _, c := range e.Hooks {
				if strings.TrimSpace(c.Command) != "" {
					return true
				}
			}
		}
	}
	return false
}

// knownHookEvents is the set of event keys accepted from a bare hooks.json, so a
// generic settings.json with unrelated top-level arrays isn't misread.
var knownHookEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true, "UserPromptSubmit": true,
	"SessionStart": true, "Stop": true, "SubagentStop": true,
	"PreCompact": true, "Notification": true, "SessionEnd": true,
}

func filterKnownEvents(m map[string][]ccHookEntry) map[string][]ccHookEntry {
	out := map[string][]ccHookEntry{}
	for ev, entries := range m {
		if knownHookEvents[ev] {
			out[ev] = entries
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// pluginRootFor resolves the ${CLAUDE_PLUGIN_ROOT} directory for a config file:
// the parent of a `.claude-plugin/` folder, else the config file's own directory.
// A "." result (config at the tree root) is normalised to "" so tree-path joins
// stay clean.
func pluginRootFor(cfgPath string) string {
	dir := path.Dir(cfgPath)
	if path.Base(dir) == ".claude-plugin" {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return ""
	}
	return dir
}

// bundleHookScripts collects the script files a command references via
// ${CLAUDE_PLUGIN_ROOT}/<path>, plus their sibling files (to catch local
// requires), keyed relative to the plugin root. Returns the file map and any
// warnings (missing script / external deps).
func bundleHookScripts(tree fetch.Tree, root, command string) (map[string][]byte, []string) {
	files := map[string][]byte{}
	var warns []string
	seenDir := map[string]bool{}
	for _, m := range pluginRootRefRe.FindAllStringSubmatch(command, -1) {
		rel := strings.Trim(m[1], "/")
		full := joinTreePath(root, rel)
		if data, ok := tree[full]; ok {
			files[rel] = data
		} else {
			warns = append(warns, "referenced script not found in tree: "+rel)
		}
		// Bundle sibling files in the script's directory (local requires), once.
		relDir := path.Dir(rel)
		if seenDir[relDir] {
			continue
		}
		seenDir[relDir] = true
		fullDir := joinTreePath(root, relDir)
		for p, data := range tree {
			if path.Dir(p) == fullDir {
				sib := treeRel(root, p)
				if sib != "" {
					files[sib] = data
				}
			}
		}
	}
	if len(files) == 0 && strings.Contains(command, pluginRootPlaceholder) {
		warns = append(warns, "hook references plugin scripts but none were bundled (check paths / external deps)")
	}
	return files, warns
}

// joinTreePath joins a plugin root with a relative path into a tree key.
func joinTreePath(root, rel string) string {
	root = strings.Trim(root, "/")
	rel = strings.Trim(rel, "/")
	if root == "" {
		return rel
	}
	if rel == "" {
		return root
	}
	return root + "/" + rel
}

// treeRel strips the plugin root prefix from a tree key, yielding a Files key.
func treeRel(root, full string) string {
	root = strings.Trim(root, "/")
	if root == "" {
		return full
	}
	return strings.TrimPrefix(full, root+"/")
}

// hookSlug builds a stable, unique slug for a hook command.
func hookSlug(event, command string, idx int) string {
	base := hookLabel(command)
	slug := skills.Slugify(base + "-" + event)
	if slug == "" {
		slug = skills.Slugify(event) + "-" + itoa(idx)
	}
	return slug
}

// hookLabel derives a short human label from a command — the basename of the
// first ${CLAUDE_PLUGIN_ROOT} script, else the first token.
func hookLabel(command string) string {
	if m := pluginRootRefRe.FindStringSubmatch(command); m != nil {
		b := path.Base(m[1])
		return strings.TrimSuffix(b, path.Ext(b))
	}
	fields := strings.Fields(command)
	if len(fields) > 0 {
		return path.Base(fields[0])
	}
	return "hook"
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
