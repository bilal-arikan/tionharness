package providers

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The exec JSON stream on Codex 0.158 reports thread-lifetime usage, including
// earlier resumed turns. The rollout carries measured per-response usage. Read
// only records appended by this attempt, including calls made before a failure.
type codexUsageCheckpoint struct {
	path string
	size int64
}

func codexRolloutPath(home, threadID string) string {
	if home == "" || !safeCodexThreadID(threadID) {
		return ""
	}
	var found string
	_ = filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !e.IsDir() && strings.HasSuffix(e.Name(), "-"+threadID+".jsonl") {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func checkpointCodexUsage(home, threadID string) codexUsageCheckpoint {
	path := codexRolloutPath(home, threadID)
	if st, err := os.Stat(path); err == nil {
		return codexUsageCheckpoint{path: path, size: st.Size()}
	}
	return codexUsageCheckpoint{}
}

func applyCodexRolloutUsage(resp *Response, home string, before codexUsageCheckpoint) {
	path := codexRolloutPath(home, resp.SessionID)
	if path == "" {
		return // Older/ephemeral CLIs have no rollout: retain their wire usage.
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if before.path == path {
		if _, err := f.Seek(before.size, io.SeekStart); err != nil {
			return
		}
	}
	u, calls, first := readCodexCallUsage(f)
	if calls > 0 {
		resp.Usage = u
		resp.ProviderCalls = calls
		resp.FirstCallPromptTokens = first
	}
}

func readCodexCallUsage(r io.Reader) (Usage, int, int) {
	type accounting struct {
		usage Usage
		calls int
		first int
	}
	var measured, legacy accounting
	add := func(a *accounting, u codexUsage) {
		if a.calls == 0 {
			a.first = u.InputTokens
			a.usage.ThinkingTokensMeasured = true
		}
		a.calls++
		a.usage.InputTokens += u.freshInput()
		a.usage.CacheReadTokens += u.CachedInputTokens
		a.usage.CacheWriteTokens += u.CacheWriteInputTokens
		a.usage.OutputTokens += u.OutputTokens
		if u.ReasoningOutputTokens != nil {
			a.usage.ThinkingTokens += *u.ReasoningOutputTokens
		} else {
			a.usage.ThinkingTokensMeasured = false
		}
	}
	seen := map[string]bool{}
	lastLegacyTotal := ""
	rd := bufio.NewReader(r)
	for {
		line, err := rd.ReadBytes('\n')
		// Ignore an incomplete trailing record (a killed subprocess may leave one).
		if len(line) > 0 && line[len(line)-1] == '\n' {
			var ev struct {
				Type    string `json:"type"`
				Payload struct {
					Type       string     `json:"type"`
					ResponseID string     `json:"response_id"`
					Usage      codexUsage `json:"usage"`
					Info       *struct {
						Last  codexUsage      `json:"last_token_usage"`
						Total json.RawMessage `json:"total_token_usage"`
					} `json:"info"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &ev) == nil {
				p := ev.Payload
				if ev.Type == "token_usage_record" && (p.ResponseID == "" || !seen[p.ResponseID]) {
					seen[p.ResponseID] = true
					add(&measured, p.Usage)
				} else if ev.Type == "event_msg" && p.Type == "token_count" && p.Info != nil {
					total := string(p.Info.Total)
					if total != "" && total != lastLegacyTotal {
						add(&legacy, p.Info.Last)
						lastLegacyTotal = total
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	if measured.calls > 0 {
		return measured.usage, measured.calls, measured.first
	}
	return legacy.usage, legacy.calls, legacy.first
}
