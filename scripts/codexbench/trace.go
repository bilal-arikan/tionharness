package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

type toolRecord struct {
	Tool         string `json:"tool"`
	Input        string `json:"input"`
	Output       string `json:"output"`
	IsError      bool   `json:"isError"`
	EndMs        int64  `json:"endMs"`
	DurationMs   *int64 `json:"durationMs"`
	TimingSource string `json:"timingSource"`
}

type toolTrace struct {
	start   time.Time
	records []toolRecord
}

func (t *toolTrace) providerEvent(step providers.TraceStep) {
	if step.Kind != "tool" || step.Running {
		return
	}
	r := toolRecord{Tool: step.Tool, Input: string(step.Input), Output: step.Output, IsError: step.IsError, EndMs: time.Since(t.start).Milliseconds(), TimingSource: "provider-stream"}
	// A zero in the production trace cannot distinguish instantaneous work from
	// a missing start event. Do not treat it as a measured zero.
	if step.DurMs > 0 {
		ms := step.DurMs
		r.DurationMs = &ms
	}
	t.records = append(t.records, r)
}

type nativeStream struct {
	pending   []byte
	trace     *toolTrace
	usage     providers.Usage
	starts    map[string]int64
	seen      map[string]bool
	completed bool
	err       error
}

func (s *nativeStream) Write(p []byte) (int, error) {
	s.pending = append(s.pending, p...)
	for {
		i := bytes.IndexByte(s.pending, '\n')
		if i < 0 {
			break
		}
		s.feed(s.pending[:i])
		s.pending = s.pending[i+1:]
	}
	return len(p), nil
}

func (s *nativeStream) flush() {
	if len(s.pending) > 0 {
		s.feed(s.pending)
		s.pending = nil
	}
}

func (s *nativeStream) feed(line []byte) {
	if len(bytes.TrimSpace(line)) == 0 {
		return
	}
	var event struct {
		Type string `json:"type"`
		Item struct {
			ID        string          `json:"id"`
			Type      string          `json:"type"`
			Command   string          `json:"command"`
			Output    string          `json:"aggregated_output"`
			Status    string          `json:"status"`
			ExitCode  *int            `json:"exit_code"`
			Changes   json.RawMessage `json:"changes"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
			Error     json.RawMessage `json:"error"`
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Query     string          `json:"query"`
			Items     json.RawMessage `json:"items"`
		} `json:"item"`
		Usage struct {
			Input  int `json:"input_tokens"`
			Cached int `json:"cached_input_tokens"`
			Write  int `json:"cache_write_input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		s.err = fmt.Errorf("invalid CLI event: %w", err)
		return
	}
	if event.Type == "turn.completed" {
		s.completed = true
		s.usage = providers.Usage{InputTokens: event.Usage.Input - event.Usage.Cached - event.Usage.Write, CacheReadTokens: event.Usage.Cached, CacheWriteTokens: event.Usage.Write, OutputTokens: event.Usage.Output}
	}
	item := event.Item
	if event.Type == "item.started" && item.ID != "" {
		s.starts[item.ID] = time.Since(s.trace.start).Milliseconds()
		return
	}
	if event.Type != "item.completed" || (item.ID != "" && s.seen[item.ID]) {
		return
	}
	r := toolRecord{EndMs: time.Since(s.trace.start).Milliseconds(), IsError: item.Status == "failed" || item.Status == "declined", TimingSource: "native-stream"}
	switch item.Type {
	case "command_execution":
		r.Tool = "shell"
		r.Input = item.Command
		r.Output = item.Output
		r.IsError = r.IsError || (item.ExitCode != nil && *item.ExitCode != 0)
	case "file_change":
		r.Tool = "apply_patch"
		r.Input = string(item.Changes)
		r.Output = item.Status
	case "mcp_tool_call":
		r.Tool = "mcp__" + item.Server + "__" + item.Tool
		r.Input = string(item.Arguments)
		r.Output = string(item.Result)
		var result struct {
			IsError bool `json:"is_error"`
		}
		_ = json.Unmarshal(item.Result, &result)
		r.IsError = r.IsError || result.IsError || (len(item.Error) > 0 && string(item.Error) != "null")
	case "web_search":
		r.Tool = "web_search"
		r.Input = item.Query
	case "todo_list":
		r.Tool = "todo_list"
		r.Input = string(item.Items)
	default:
		return
	}
	if start, ok := s.starts[item.ID]; ok {
		ms := r.EndMs - start
		r.DurationMs = &ms
		delete(s.starts, item.ID)
	}
	if item.ID != "" {
		s.seen[item.ID] = true
	}
	s.trace.records = append(s.trace.records, r)
}
