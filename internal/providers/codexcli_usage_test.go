package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func usageRecord(id string, in, cached, out int) string {
	line, _ := json.Marshal(map[string]any{"type": "token_usage_record", "payload": map[string]any{
		"response_id": id, "usage": map[string]int{"input_tokens": in, "cached_input_tokens": cached, "output_tokens": out, "reasoning_output_tokens": 2},
	}})
	return string(line) + "\n"
}

func TestCodexResumedUsageExcludesEarlierTurns(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-thread-1.jsonl")
	old := usageRecord("old", 1000, 800, 100)
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	before := checkpointCodexUsage(home, "thread-1")
	newRecords := usageRecord("new-1", 2000, 1900, 30) + usageRecord("new-2", 2200, 2000, 40)
	if err := os.WriteFile(path, []byte(old+newRecords), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := &Response{SessionID: "thread-1", Usage: Usage{InputTokens: 500, CacheReadTokens: 4700, OutputTokens: 170}}
	applyCodexRolloutUsage(resp, home, before)
	if resp.Usage.InputTokens != 300 || resp.Usage.CacheReadTokens != 3900 || resp.Usage.OutputTokens != 70 || resp.ProviderCalls != 2 || resp.FirstCallPromptTokens != 2000 {
		t.Fatalf("resumed usage includes history: %+v", resp)
	}
	if resp.Usage.ThinkingTokens != 4 || !resp.Usage.ThinkingTokensMeasured {
		t.Fatalf("reasoning: %+v", resp.Usage)
	}
}

func TestCodexCallUsageDedupesRecordsAndIgnoresTruncatedTail(t *testing.T) {
	line := usageRecord("one", 100, 80, 10)
	u, calls, first := readCodexCallUsage(strings.NewReader(line + line + `{"type":"token_usage_record"`))
	if calls != 1 || first != 100 || u.InputTokens != 20 || u.OutputTokens != 10 {
		t.Fatalf("usage=%+v calls=%d first=%d", u, calls, first)
	}
}

func TestCodexCallUsageLegacyTokenEvents(t *testing.T) {
	line := `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1100},"last_token_usage":{"input_tokens":100,"cached_input_tokens":90,"output_tokens":10}}}}` + "\n"
	u, calls, _ := readCodexCallUsage(strings.NewReader(line + line))
	if calls != 1 || u.InputTokens != 10 || u.CacheReadTokens != 90 {
		t.Fatalf("usage=%+v calls=%d", u, calls)
	}
}

func TestCodexUsageWithoutRolloutPreservesWireCounters(t *testing.T) {
	u := Usage{InputTokens: 20, OutputTokens: 10}
	resp := &Response{SessionID: "missing", Usage: u}
	applyCodexRolloutUsage(resp, t.TempDir(), codexUsageCheckpoint{})
	if resp.Usage != u {
		t.Fatalf("lost wire usage: %+v", resp.Usage)
	}
}
