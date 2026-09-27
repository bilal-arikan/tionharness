package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func native(ctx context.Context, bin, base, dir, model, system, prompt string) (providers.Usage, int, error) {
	var usage providers.Usage
	home, err := os.MkdirTemp("", "tion-codex-baseline-")
	if err != nil {
		return usage, 0, err
	}
	defer os.RemoveAll(home)
	for _, name := range []string{"auth.json", "models_cache.json", "installation_id"} {
		data, err := os.ReadFile(filepath.Join(base, name))
		if os.IsNotExist(err) && name != "auth.json" {
			continue
		}
		if err != nil {
			return usage, 0, err
		}
		if err := os.WriteFile(filepath.Join(home, name), data, 0600); err != nil {
			return usage, 0, err
		}
	}
	quoted, _ := json.Marshal(system)
	config := fmt.Sprintf("developer_instructions = %s\nmodel_reasoning_effort = \"high\"\nweb_search = \"disabled\"\n[agents]\nenabled = false\n", quoted)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		return usage, 0, err
	}
	cmd := proc.CommandContextNested(ctx, bin, "exec", "--json", "--ephemeral", "--skip-git-repo-check", "--strict-config", "-m", model, "--dangerously-bypass-approvals-and-sandbox", "-")
	cmd.Dir = dir
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if strings.EqualFold(key, "CODEX_HOME") || strings.EqualFold(key, "OPENAI_API_KEY") {
			continue
		}
		cmd.Env = append(cmd.Env, env)
	}
	cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, runErr := cmd.Output()
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	completed, tools := false, 0
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
			Usage struct {
				Input  int `json:"input_tokens"`
				Cached int `json:"cached_input_tokens"`
				Write  int `json:"cache_write_input_tokens"`
				Output int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return usage, tools, err
		}
		if event.Type == "turn.completed" {
			completed = true
			usage.InputTokens += event.Usage.Input - event.Usage.Cached - event.Usage.Write
			usage.CacheReadTokens += event.Usage.Cached
			usage.CacheWriteTokens += event.Usage.Write
			usage.OutputTokens += event.Usage.Output
		}
		if event.Type == "item.completed" {
			switch event.Item.Type {
			case "command_execution", "file_change", "mcp_tool_call", "web_search", "todo_list":
				tools++
			}
		}
	}
	if runErr != nil {
		return usage, tools, fmt.Errorf("CLI failed: %w: %s", runErr, stderr.String())
	}
	if err := scanner.Err(); err != nil {
		return usage, tools, err
	}
	if !completed {
		return usage, tools, fmt.Errorf("CLI emitted no turn.completed")
	}
	return usage, tools, nil
}
