package decider

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// DebugEvent is a metadata-only lifecycle event. No state, instructions,
// credentials, URLs or provider error bodies are stored here.
type DebugEvent struct {
	At            int64             `json:"at"`
	TraceID       string            `json:"traceId"`
	Authority     string            `json:"authority"`
	Mode          Mode              `json:"mode"`
	Stage         string            `json:"stage"`
	Role          string            `json:"role,omitempty"`
	Ref           string            `json:"ref,omitempty"`
	SessionID     string            `json:"sessionId,omitempty"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	TurnID        string            `json:"turnId,omitempty"`
	ConfigHash    string            `json:"configHash,omitempty"`
	ModelHash     string            `json:"modelHash,omitempty"`
	Threshold     float64           `json:"threshold"`
	Instance      string            `json:"instance,omitempty"`
	Backend       string            `json:"backend,omitempty"`
	Model         string            `json:"model,omitempty"`
	ServedModel   string            `json:"servedModel,omitempty"`
	RequestHash   string            `json:"requestHash,omitempty"`
	StateBytes    int               `json:"stateBytes,omitempty"`
	PreparedBytes int               `json:"preparedBytes,omitempty"`
	StateTrimmed  bool              `json:"stateTrimmed,omitempty"`
	TimeoutMs     int               `json:"timeoutMs,omitempty"`
	ContextTokens int               `json:"contextTokens,omitempty"`
	QuestionID    string            `json:"questionId,omitempty"`
	QuestionTypes map[string]int    `json:"questionTypes,omitempty"`
	Answers       map[string]Answer `json:"answers,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
	LatencyMs     int64             `json:"latencyMs,omitempty"`
	HTTPAttempt   int               `json:"httpAttempt,omitempty"`
	RetryWaitMs   int64             `json:"retryWaitMs,omitempty"`
	Error         string            `json:"error,omitempty"`
	InputTokens   int               `json:"inputTokens,omitempty"`
	OutputTokens  int               `json:"outputTokens,omitempty"`
	CostUSD       float64           `json:"costUsd,omitempty"`
	Outcome       string            `json:"outcome,omitempty"`
	Baseline      string            `json:"baseline,omitempty"`
	Strength      float64           `json:"strength,omitempty"`
	Applied       bool              `json:"applied,omitempty"`
}

const (
	debugCapacity = 5000
	debugMaxBytes = 4 << 20
)

// DebugJournal retains a bounded event tail in memory and two bounded JSONL
// files on disk. Persistence is best-effort and cannot interrupt decisions.
type DebugJournal struct {
	mu       sync.Mutex
	path     string
	events   []DebugEvent
	capacity int
	maxBytes int64
	logger   *slog.Logger
}

func openDebugJournal(dir string, logger *slog.Logger) *DebugJournal {
	j := &DebugJournal{capacity: debugCapacity, maxBytes: debugMaxBytes, logger: logger}
	if dir != "" {
		j.path = filepath.Join(dir, "debug.jsonl")
		j.events = append(readDebugTail(j.rotatedPath(), j.capacity), readDebugTail(j.path, j.capacity)...)
		if len(j.events) > j.capacity {
			j.events = slices.Clone(j.events[len(j.events)-j.capacity:])
		}
	}
	return j
}

func readDebugTail(path string, capacity int) []DebugEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var events []DebugEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 256<<10)
	for sc.Scan() {
		var event DebugEvent
		if json.Unmarshal(sc.Bytes(), &event) == nil && event.TraceID != "" {
			events = append(events, event)
			if len(events) > capacity {
				events = slices.Delete(events, 0, len(events)-capacity)
			}
		}
	}
	return events
}

func (j *DebugJournal) rotatedPath() string {
	return filepath.Join(filepath.Dir(j.path), "debug.1.jsonl")
}

func (j *DebugJournal) append(event DebugEvent) {
	// Round-trip detaches response maps from callers, bounds the stored shape,
	// and rejects non-finite numbers instead of poisoning the JSON endpoint.
	data, err := json.Marshal(event)
	if err != nil || len(data) > 128<<10 {
		return
	}
	var detached DebugEvent
	if json.Unmarshal(data, &detached) != nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.events = append(j.events, detached)
	if len(j.events) > j.capacity {
		j.events = slices.Delete(j.events, 0, len(j.events)-j.capacity)
	}
	if j.path == "" {
		return
	}
	if err := j.persist(append(data, '\n')); err != nil {
		j.logger.Warn("decision debug journal write failed", "error", err)
	}
}

func (j *DebugJournal) persist(line []byte) error {
	if err := os.MkdirAll(filepath.Dir(j.path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(j.path); err == nil && info.Size()+int64(len(line)) > j.maxBytes {
		// Windows cannot replace an existing destination with os.Rename.
		if err := os.Remove(j.rotatedPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(j.path, j.rotatedPath()); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

func (j *DebugJournal) snapshot() []DebugEvent {
	j.mu.Lock()
	defer j.mu.Unlock()
	// The API never receives mutable references into the journal.
	data, _ := json.Marshal(j.events)
	var out []DebugEvent
	_ = json.Unmarshal(data, &out)
	return out
}

func (j *DebugJournal) lastForTrace(id string) (DebugEvent, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := len(j.events) - 1; i >= 0; i-- {
		if j.events[i].TraceID == id {
			return j.events[i], true
		}
	}
	return DebugEvent{}, false
}
