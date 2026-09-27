// Command codexbench runs opt-in, account-backed coding comparisons.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type result struct {
	Task       string          `json:"task"`
	Model      string          `json:"model"`
	Route      string          `json:"route"`
	Seconds    float64         `json:"seconds"`
	Usage      providers.Usage `json:"usage"`
	Tools      int             `json:"tools"`
	Passed     bool            `json:"passed"`
	Error      string          `json:"error,omitempty"`
	Validation string          `json:"validation"`
}

func main() {
	bin := flag.String("bin", "", "Codex executable path (required)")
	home := flag.String("home", "", "Authenticated Codex home (required)")
	fixtures := flag.String("fixtures", "scripts/codexbench/testdata", "Task fixtures")
	out := flag.String("out", "", "JSON results path (required)")
	flag.Parse()
	if *bin == "" || *home == "" || *out == "" {
		panic("bin, home and out are required; this benchmark consumes account quota")
	}
	version, err := proc.Command(*bin, "--version").Output()
	must(err)
	results := []result{}
	for taskIndex, task := range []string{"intervals", "ttl_cache", "dependency_layers"} {
		for modelIndex, model := range []string{"gpt-6-astra", "gpt-6-sol"} {
			routes := []string{"tionharness", "codex-cli"}
			if (taskIndex+modelIndex)%2 == 1 {
				routes[0], routes[1] = routes[1], routes[0]
			}
			for _, route := range routes {
				fmt.Printf("START %s %s %s\n", task, model, route)
				r := run(*bin, *home, *fixtures, task, model, route)
				results = append(results, r)
				report := map[string]any{"cliVersion": strings.TrimSpace(string(version)), "effort": "high", "recordedAt": time.Now().Format(time.RFC3339), "results": results}
				data, err := json.MarshalIndent(report, "", "  ")
				must(err)
				must(os.WriteFile(*out, append(data, '\n'), 0600))
				fmt.Printf("DONE %s %s %s passed=%v seconds=%.2f input=%d cached=%d output=%d error=%s\n", task, model, route, r.Passed, r.Seconds, r.Usage.InputTokens, r.Usage.CacheReadTokens, r.Usage.OutputTokens, r.Error)
			}
		}
	}
}

func run(bin, home, fixtures, task, model, route string) result {
	r := result{Task: task, Model: model, Route: route}
	dir, err := os.MkdirTemp("", "tion-codex-bench-")
	must(err)
	defer os.RemoveAll(dir)
	starter, err := os.ReadFile(filepath.Join(fixtures, task+".py"))
	must(err)
	must(os.WriteFile(filepath.Join(dir, "solution.py"), starter, 0600))
	spec, err := os.ReadFile(filepath.Join(fixtures, task+".txt"))
	must(err)
	prompt := string(spec) + "\nImplement solution.py in the current directory. You may run local Python checks. Do not use network, install packages, delegate, or access files outside this directory. Hidden tests will run after you finish. Do not change the public API."
	system := "Complete the coding task in the working directory. Use only the Python standard library. Finish with a brief summary."
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	start := time.Now()
	if route == "tionharness" {
		response, callErr := providers.NewCodexCLI(bin, "", home).Complete(ctx, providers.Request{
			Model: model, CLIEffortLevel: "high", PermissionMode: "auto", WorkDir: dir,
			System: system, NativeWebSearch: false, Messages: []providers.Message{{Role: "user", Text: prompt}},
		})
		err = callErr
		if response != nil {
			r.Usage = response.Usage
			for _, step := range response.Trace {
				if step.Kind == "tool" {
					r.Tools++
				}
			}
		}
	} else {
		r.Usage, r.Tools, err = native(ctx, bin, home, dir, model, system, prompt)
	}
	r.Seconds = time.Since(start).Seconds()
	if err != nil {
		r.Error = err.Error()
	}
	// The evaluator is introduced only after the agent exits.
	tests, readErr := os.ReadFile(filepath.Join(fixtures, task+"_test.py"))
	must(readErr)
	must(os.WriteFile(filepath.Join(dir, "evaluate.py"), tests, 0600))
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer checkCancel()
	check := proc.CommandContext(checkCtx, "python", "-I", "-c", "import sys,runpy; sys.path.insert(0,'.'); runpy.run_path('evaluate.py',run_name='__main__')")
	check.Dir = dir
	validation, checkErr := check.CombinedOutput()
	r.Validation = string(validation)
	r.Passed = err == nil && checkErr == nil
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
