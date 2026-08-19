package main

import (
	"fmt"
	"path/filepath"
	"time"
)

// anthropicModeVariants maps a base provider instance id to the "-anthropic"
// kind that reuses its credential. These kinds route the same account through
// the Anthropic Messages transport (native tool-use + thinking) and never had
// their own legacy settings key, so MigrateFromSettings emitted no instance for
// them — leaving every agent bound to one with an unresolvable provider.
var anthropicModeVariants = []struct {
	baseID string
	kindID string
	label  string
}{
	{baseID: "minimax", kindID: "minimax-anthropic", label: "MiniMax (Anthropic modu)"},
	{baseID: "deepseek", kindID: "deepseek-anthropic", label: "DeepSeek (Anthropic modu)"},
}

// repairInstances adds the missing "-anthropic" instances to providers.json and
// returns the set of instance ids present afterwards (the input the agent pass
// validates against). The variant reuses the base instance's ENCRYPTED secret
// blob verbatim — the cipher is app-wide, not per-instance, so relocating the
// sealed value needs no key material here.
//
// baseUrl is deliberately left empty rather than copied: the base instance's
// URL is its OpenAI-compatible endpoint, which the Anthropic transport cannot
// talk to. Empty means "use the kind's own default endpoint".
func repairInstances(dataDir string, apply bool) (map[string]bool, error) {
	path := filepath.Join(dataDir, "providers.json")
	var doc []map[string]any
	if err := readJSONDoc(path, &doc); err != nil {
		return nil, err
	}

	present := map[string]bool{}
	byID := map[string]map[string]any{}
	for _, inst := range doc {
		id, _ := inst["id"].(string)
		if id == "" {
			continue
		}
		present[id] = true
		byID[id] = inst
	}

	added := 0
	for _, v := range anthropicModeVariants {
		if present[v.kindID] {
			continue
		}
		base, ok := byID[v.baseID]
		if !ok {
			fmt.Printf("providers: %-20s skipped — base instance %q is not configured\n", v.kindID, v.baseID)
			continue
		}
		secrets, _ := base["secretsEnc"].(map[string]any)
		key, _ := secrets["key"].(string)
		if key == "" {
			fmt.Printf("providers: %-20s skipped — base instance %q has no stored key\n", v.kindID, v.baseID)
			continue
		}
		doc = append(doc, map[string]any{
			"id":           v.kindID,
			"kindId":       v.kindID,
			"label":        v.label,
			"icon":         "",
			"enabled":      true,
			"defaultModel": "",
			"models":       "",
			"config":       map[string]any{"baseUrl": ""},
			"secretsEnc":   map[string]any{"key": key},
			"createdAt":    time.Now().Format(time.RFC3339Nano),
		})
		present[v.kindID] = true
		added++
		fmt.Printf("providers: %-20s ADD (credential shared with %q)\n", v.kindID, v.baseID)
	}

	if added == 0 {
		fmt.Println("providers: nothing to add")
		return present, nil
	}
	if apply {
		if err := writeJSONDoc(path, doc, true); err != nil {
			return nil, fmt.Errorf("write providers.json: %w", err)
		}
		fmt.Printf("providers: wrote %d instance(s)\n", added)
	}
	return present, nil
}
