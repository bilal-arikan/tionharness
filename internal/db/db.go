// Package db is SwarmGo's persistence layer. It is backed entirely by the
// filesystem (no SQL database): every entity is a human-readable JSON file and
// each conversation is a JSONL file (header line + one message per line), so a
// workspace's data is portable, git-friendly and inspectable on disk.
//
// All data is loaded into memory at Open() and served from there; every
// mutation writes the affected entity back to disk atomically (tmp file +
// rename). A single RWMutex guards the in-memory state, which is safe for the
// concurrent agent runtime (per-agent goroutines, scheduler, flow runners).
package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("not found")

func now() int64 { return time.Now().Unix() }

func newID() string { return uuid.NewString() }

// DB is the in-memory + on-disk store for a single workspace.
type DB struct {
	root string // store root directory

	mu        sync.RWMutex
	agents    map[string]Agent
	sessions  map[string]Session
	messages  map[string][]Message // keyed by session id, chronological
	tasks     map[string]Task
	runs      map[string]Run
	schedules map[string]Schedule
	knowledge map[string]KnowledgeSource
	mcp       map[string]MCPServer
	flows     map[string]Flow
	flowRuns  map[string]FlowRun
	artifacts map[string]Artifact
	usage     map[string]Usage // keyed by agentID + "|" + day
}

// Open opens (creating if missing) the file-backed store rooted at path and
// loads every entity into memory.
func Open(path string) (*DB, error) {
	d := &DB{
		root:      path,
		agents:    map[string]Agent{},
		sessions:  map[string]Session{},
		messages:  map[string][]Message{},
		tasks:     map[string]Task{},
		runs:      map[string]Run{},
		schedules: map[string]Schedule{},
		knowledge: map[string]KnowledgeSource{},
		mcp:       map[string]MCPServer{},
		flows:     map[string]Flow{},
		flowRuns:  map[string]FlowRun{},
		artifacts: map[string]Artifact{},
		usage:     map[string]Usage{},
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	if err := d.load(); err != nil {
		return nil, err
	}
	return d, nil
}

// Close is a no-op kept for API parity (all writes are synchronous).
func (d *DB) Close() error { return nil }

// ---- paths ----

func (d *DB) dir(parts ...string) string {
	return filepath.Join(append([]string{d.root}, parts...)...)
}

const (
	dirAgents    = "agents"
	dirSessions  = "sessions"
	dirTasks     = "tasks"
	dirRuns      = "runs"
	dirSchedules = "schedules"
	dirKnowledge = "knowledge"
	dirMCP       = "mcp-servers"
	dirFlows     = "flows"
	dirFlowRuns  = "flow-runs"
	dirArtifacts = "artifacts"
	dirUsage     = "usage"
)

// ---- generic disk helpers ----

// atomicWriteJSON marshals v to a pretty JSON file, writing via a temp file and
// rename so a crash never leaves a half-written file.
func atomicWriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteBytes(path, data)
}

func atomicWriteBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// loadJSONDir reads every "*.json" file in a directory and unmarshals each into
// a fresh T, returning the slice. A missing directory yields an empty slice.
func loadJSONDir[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		var v T
		if err := readJSONFile(filepath.Join(dir, name), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// load reads every entity from disk into the in-memory maps.
func (d *DB) load() error {
	agents, err := loadJSONDir[Agent](d.dir(dirAgents))
	if err != nil {
		return err
	}
	for _, a := range agents {
		d.agents[a.ID] = a
	}

	tasks, err := loadJSONDir[Task](d.dir(dirTasks))
	if err != nil {
		return err
	}
	for _, t := range tasks {
		d.tasks[t.ID] = t
	}

	runs, err := loadJSONDir[Run](d.dir(dirRuns))
	if err != nil {
		return err
	}
	for _, r := range runs {
		d.runs[r.ID] = r
	}

	schedules, err := loadJSONDir[Schedule](d.dir(dirSchedules))
	if err != nil {
		return err
	}
	for _, s := range schedules {
		d.schedules[s.ID] = s
	}

	mcps, err := loadJSONDir[MCPServer](d.dir(dirMCP))
	if err != nil {
		return err
	}
	for _, m := range mcps {
		d.mcp[m.ID] = m
	}

	flows, err := loadJSONDir[Flow](d.dir(dirFlows))
	if err != nil {
		return err
	}
	for _, f := range flows {
		d.flows[f.ID] = f
	}

	flowRuns, err := loadJSONDir[FlowRun](d.dir(dirFlowRuns))
	if err != nil {
		return err
	}
	for _, r := range flowRuns {
		d.flowRuns[r.ID] = r
	}

	artifacts, err := loadJSONDir[Artifact](d.dir(dirArtifacts))
	if err != nil {
		return err
	}
	for _, a := range artifacts {
		d.artifacts[a.ID] = a
	}

	if err := d.loadKnowledge(); err != nil {
		return err
	}
	if err := d.loadUsage(); err != nil {
		return err
	}
	return d.loadSessions()
}

// ---- context is accepted for API parity but not used by the file store ----
var _ = context.Background
