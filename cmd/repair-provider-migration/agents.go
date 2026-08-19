package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// repairAgents repoints agents whose providerInstanceId names an instance that
// no longer exists. Registry.Get rejects an unknown instance outright (there is
// deliberately no silent fallback), so such an agent fails every turn with
// "unknown provider instance".
//
// The replacement is the default instance of the agent's own KIND (whose id
// equals the kind id for every migrated instance) — the closest thing to what
// the agent was configured for. An agent whose kind has no instance either is
// reported and left alone: guessing a different provider would silently change
// which model and account it runs on, which is the user's call, not a repair.
func repairAgents(dataDir string, instanceIDs map[string]bool, apply bool) error {
	roots, err := workspaceDirs(dataDir)
	if err != nil {
		return err
	}
	fixed, unresolved := 0, 0
	for _, root := range roots {
		ws := filepath.Base(root)
		dir := filepath.Join(root, "store", "agents")
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			var agent map[string]any
			if err := readJSONDoc(path, &agent); err != nil {
				return err
			}
			id, _ := agent["id"].(string)
			name, _ := agent["name"].(string)
			kind, _ := agent["provider"].(string)
			inst, _ := agent["providerInstanceId"].(string)

			// An empty instance id is backfilled from the kind at read time by the
			// app itself, so it is only broken when the KIND has no instance either.
			effective := inst
			if effective == "" {
				effective = kind
			}
			if effective == "" || instanceIDs[effective] {
				continue
			}
			if !instanceIDs[kind] {
				fmt.Printf("%s/%s %-24s UNRESOLVED — instance %q and kind %q both missing; pick a provider in the UI\n", ws, id, name, effective, kind)
				unresolved++
				continue
			}
			fmt.Printf("%s/%s %-24s %q → %q (missing instance repointed to its kind's default)\n", ws, id, name, effective, kind)
			agent["providerInstanceId"] = kind
			fixed++
			if apply {
				if err := writeJSONDoc(path, agent, false); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
			}
		}
	}
	fmt.Printf("agents: %d repointed, %d unresolved\n", fixed, unresolved)
	return nil
}
