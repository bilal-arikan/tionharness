package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

type codexRPCEnvelope struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Method string          `json:"method"`
	Params struct {
		ThreadID string `json:"threadId"`
		Item     struct {
			Type string `json:"type"`
		} `json:"item"`
	} `json:"params"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// CompactNative invokes Codex App Server's documented thread/compact/start
// request. codex exec has no manual-compaction subcommand; sending "/compact"
// to exec would be an ordinary agent prompt and is deliberately forbidden.
func (c *CodexCLI) CompactNative(ctx context.Context, resumeSessionID string, req Request) (*Response, error) {
	if c.binPath == "" {
		return nil, errors.New("codex CLI: no binary configured")
	}
	if strings.TrimSpace(req.CLIResumeScope) == "" {
		return nil, errors.New("codex native compaction requires a CLI resume scope")
	}
	if !c.CanResumeScoped(req.CLIResumeScope, resumeSessionID) {
		return nil, fmt.Errorf("codex native compaction cannot resume thread %s in this session scope", resumeSessionID)
	}
	home, cleanup, err := prepareCodexTurnHome(c.configDir, req.CLIResumeScope)
	if err != nil {
		return nil, fmt.Errorf("codex CLI: prepare native compaction home: %w", err)
	}
	defer cleanup()

	cmd := proc.CommandContextNested(ctx, c.binPath, "app-server", "--listen", "stdio://", "--strict-config")
	cmd.Env = append(codexBaseEnv(), "CODEX_HOME="+home)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	request := func(id int, method string, params any) error {
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for scan.Scan() {
			var envelope codexRPCEnvelope
			if err := json.Unmarshal(scan.Bytes(), &envelope); err != nil || envelope.ID != id {
				continue
			}
			if envelope.Error != nil {
				return fmt.Errorf("codex app-server %s failed (%d): %s", method, envelope.Error.Code, envelope.Error.Message)
			}
			return nil
		}
		if err := scan.Err(); err != nil {
			return fmt.Errorf("read codex app-server %s response: %w", method, err)
		}
		return fmt.Errorf("codex app-server exited during %s: %s", method, strings.TrimSpace(stderr.String()))
	}

	if err := request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "tionharness", "version": "1"}}); err != nil {
		return nil, err
	}
	if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "initialized"}); err != nil {
		return nil, err
	}
	if err := request(2, "thread/resume", map[string]any{"threadId": resumeSessionID}); err != nil {
		return nil, err
	}

	stepID := "codex-compact-" + resumeSessionID
	if req.OnEvent != nil {
		req.OnEvent(TraceStep{ID: stepID, Running: true, Kind: "compaction", Source: "cli-native", Provider: c.Name(), SessionAction: "native-compact"})
	}
	if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "thread/compact/start", "params": map[string]any{"threadId": resumeSessionID}}); err != nil {
		if req.OnEvent != nil {
			req.OnEvent(TraceStep{Kind: "tombstone", Ref: stepID})
		}
		return nil, err
	}
	acknowledged, lifecycleCompleted := false, false
	for scan.Scan() {
		var envelope codexRPCEnvelope
		if err := json.Unmarshal(scan.Bytes(), &envelope); err != nil {
			continue
		}
		if envelope.ID == 3 {
			if envelope.Error != nil {
				if req.OnEvent != nil {
					req.OnEvent(TraceStep{Kind: "tombstone", Ref: stepID})
				}
				return nil, fmt.Errorf("codex app-server thread/compact/start failed (%d): %s", envelope.Error.Code, envelope.Error.Message)
			}
			acknowledged = true
		}
		if codexCompactCompleted(envelope, resumeSessionID) {
			lifecycleCompleted = true
		}
		if acknowledged && lifecycleCompleted {
			break
		}
	}
	if !acknowledged || !lifecycleCompleted {
		if req.OnEvent != nil {
			req.OnEvent(TraceStep{Kind: "tombstone", Ref: stepID})
		}
		if err := scan.Err(); err != nil {
			return nil, fmt.Errorf("read codex native compaction lifecycle: %w", err)
		}
		return nil, fmt.Errorf("codex app-server exited before native compaction completed: %s", strings.TrimSpace(stderr.String()))
	}
	completed := TraceStep{ID: stepID, Kind: "compaction", Trigger: "manual", Source: "cli-native", Provider: c.Name(), SessionAction: "native-compact"}
	if req.OnEvent != nil {
		req.OnEvent(completed)
	}
	return &Response{SessionID: resumeSessionID, Trace: []TraceStep{completed}}, nil
}

func codexCompactCompleted(envelope codexRPCEnvelope, threadID string) bool {
	return envelope.Params.ThreadID == threadID &&
		(envelope.Method == "thread/compacted" ||
			envelope.Method == "item/completed" && envelope.Params.Item.Type == "contextCompaction")
}

var _ CLINativeManualCompactor = (*CodexCLI)(nil)
