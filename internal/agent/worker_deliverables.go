package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Explicit file contracts are presence checks, not claims of functional or
// independent acceptance. Existing tasks without a contract remain compatible.
func normalizeWorkerDeliverables(paths []string, cwd string) ([]string, error) {
	if len(paths) > 16 {
		return nil, fmt.Errorf("expectedDeliverables supports at most 16 files")
	}
	result := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || len(path) > 4096 || strings.ContainsAny(path, "\x00\r\n*?") {
			return nil, fmt.Errorf("expectedDeliverables requires concrete file paths")
		}
		if !filepath.IsAbs(path) {
			if !filepath.IsAbs(cwd) {
				return nil, fmt.Errorf("relative deliverables require an absolute worker cwd")
			}
			path = filepath.Join(cwd, path)
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if !seen[key] {
			result = append(result, path)
			seen[key] = true
		}
	}
	return result, nil
}

type workerFileCheck struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func checkWorkerDeliverables(status, text string, paths []string) (string, string, *TurnStep) {
	if len(paths) == 0 || status != turnStatusCompleted {
		return status, text, nil
	}
	checks := make([]workerFileCheck, 0, len(paths))
	missing := 0
	for _, path := range paths {
		check := workerFileCheck{Path: path, Status: "present"}
		info, err := os.Lstat(path)
		switch {
		case err != nil:
			check.Status = "unavailable"
		case !info.Mode().IsRegular():
			check.Status = "not_file"
		case info.Size() == 0:
			check.Status = "empty"
		}
		if check.Status != "present" {
			missing++
		}
		checks = append(checks, check)
	}
	output, _ := json.Marshal(checks)
	step := &TurnStep{Kind: StepText, Operation: "delivery_check", Status: "present", Target: append([]string(nil), paths...), Output: string(output),
		Text: "Expected files are present. Content correctness and independent acceptance are separate checks."}
	if missing > 0 {
		status, step.Status = turnStatusIncomplete, "missing"
		step.Text = "Expected files are missing, empty, or not regular files. Delivery remains unfinished."
		text = fmt.Sprintf("Delivery is incomplete: %d of %d expected files are missing, empty, or not regular files. The task is unfinished.\n\n%s", missing, len(paths), text)
	}
	return status, text, step
}
