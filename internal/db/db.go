// Package db is TionSwarm's persistence layer. It is backed entirely by the
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
	"strconv"
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
	mcp       map[string]MCPServer
	flows       map[string]Flow
	flowRuns    map[string]FlowRun
	automations map[string]Automation
	artifacts   map[string]Artifact
	hooks     map[string]Hook
	usage     map[string]Usage // keyed by agentID + "|" + day
	sessionUsage map[string]SessionUsage // keyed by session id (lifetime rollup)

	toolConfig WorkspaceToolConfig // workspace-wide tool activation (singleton)

	// boardHook is an optional observer invoked (best-effort) after a task's
	// board state changes or a task is created/deleted. It backs board-triggered
	// automations; the workspace manager wires it to the AutomationEngine. It is
	// called AFTER the store lock is released, and the registered callback is
	// expected to return promptly (dispatch on its own goroutine), so a board
	// mutation is never blocked by automation dispatch. Guarded by boardHookMu
	// since SetBoardHook runs during boot while a mutation may already be firing.
	boardHook   BoardChangeFn
	boardHookMu sync.RWMutex

	// debugCount tracks the on-disk line count of each session's debug.jsonl so
	// the append path can cap the file (oldest events pruned) without re-reading
	// it every write. Guarded by its own mutex (independent of mu) so a debug
	// emit never contends with the store hot path — debug.jsonl is a separate
	// file, just like the inflight sidecar.
	debugMu    sync.Mutex
	debugCount map[string]int

	// lessonsMu guards the workspace-wide lessons.jsonl sidecar (failure
	// lessons, self-healing) — independent of mu for the same reason as debugMu.
	lessonsMu sync.Mutex

	// counters holds the per-entity monotonic id sequence (prefix -> last n).
	// It is persisted to counters.json so a number is never reused, even across
	// deletions or restarts. Guarded by its own mutex (independent of mu) so it
	// can be called both before and while mu is held.
	countersMu sync.Mutex
	counters   map[string]int64
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
		mcp:       map[string]MCPServer{},
		flows:       map[string]Flow{},
		flowRuns:    map[string]FlowRun{},
		automations: map[string]Automation{},
		artifacts:   map[string]Artifact{},
		hooks:     map[string]Hook{},
		usage:        map[string]Usage{},
		sessionUsage: map[string]SessionUsage{},
		debugCount:   map[string]int{},
		counters:  map[string]int64{},
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

// Root returns the store root directory for this workspace. Used by callers that
// need to place auxiliary files (e.g. per-agent progress fallback) alongside the
// entity store without importing internal paths.
func (d *DB) Root() string { return d.root }

const (
	dirAgents    = "agents"
	dirSessions  = "sessions"
	dirTasks     = "tasks"
	dirRuns      = "runs"
	dirSchedules = "schedules"
	dirMCP       = "mcp-servers"
	dirFlows       = "flows"
	dirFlowRuns    = "flow-runs"
	dirAutomations = "automations"
	dirArtifacts = "artifacts"
	dirRender    = "render" // per-session render_template output (transient, swept)
	dirHooks     = "hooks"
	dirUsage     = "usage"
	dirSessionUsage = "session-usage"
)

// countersFile stores the per-entity id sequence at the workspace store root.
const countersFile = "counters.json"

// Human-readable id prefixes (English mnemonics). A new entity gets
// "<prefix><n>" (e.g. "TSK7"). Legacy UUID ids keep working unchanged; lookups
// are by opaque string so the two schemes coexist. Prefixes are pure letters,
// numbers are pure digits, so an id is trivially parseable and can never
// collide with a UUID.
const (
	idAgent     = "AGT"
	idSession   = "SES"
	idTask      = "TSK"
	idFlow      = "FLW"
	idFlowRun   = "RUN"
	idArtifact  = "ART"
	idKnowledge = "MEM"
	idMCP       = "MCP"
	idHook       = "HOK"
	idSchedule   = "SCH"
	idAutomation = "AUT"
)

// loadCounters reads the persisted id sequence. A missing file is fine (fresh
// store): every counter simply starts at zero.
func (d *DB) loadCounters() error {
	var c map[string]int64
	err := readJSONFile(d.dir(countersFile), &c)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if c != nil {
		d.counters = c
	}
	return nil
}

// nextID allocates the next monotonic, never-reused id for the given entity
// prefix. The counter is bumped in memory and persisted before returning, so a
// deletion never frees a number and a restart never reissues one. It takes its
// own mutex, making it safe to call both before mu is acquired (most Create*
// paths) and while mu is already held (e.g. createSessionLocked).
func (d *DB) nextID(prefix string) string {
	d.countersMu.Lock()
	defer d.countersMu.Unlock()
	d.counters[prefix]++
	n := d.counters[prefix]
	// Best-effort persist: a write error here only risks a future restart
	// reissuing this number, which is acceptably rare for a local file store.
	_ = atomicWriteJSON(d.dir(countersFile), d.counters)
	return prefix + strconv.FormatInt(n, 10)
}

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
	if err := d.loadCounters(); err != nil {
		return err
	}

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

	automations, err := loadJSONDir[Automation](d.dir(dirAutomations))
	if err != nil {
		return err
	}
	for _, a := range automations {
		d.automations[a.ID] = a
	}

	artifacts, err := loadJSONDir[Artifact](d.dir(dirArtifacts))
	if err != nil {
		return err
	}
	var migrate []Artifact
	for _, a := range artifacts {
		switch {
		case isTextArtifact(a.Kind) && a.Content == "" && a.ContentFile != "":
			// Body lives in a content file — read it back into memory.
			d.readArtifactContent(&a)
			d.artifacts[a.ID] = a
		case isTextArtifact(a.Kind) && a.Content != "" && a.ContentFile == "":
			// Legacy artifact with an embedded body: move it to a content file.
			d.artifacts[a.ID] = a
			migrate = append(migrate, a)
		default:
			d.artifacts[a.ID] = a
		}
	}
	// One-time migration of legacy embedded bodies → files (idempotent: once a
	// ContentFile is set the artifact takes the read-back branch on next boot).
	for _, a := range migrate {
		if err := d.persistArtifactLocked(&a); err != nil {
			return err
		}
	}

	hooks, err := loadJSONDir[Hook](d.dir(dirHooks))
	if err != nil {
		return err
	}
	for _, h := range hooks {
		d.hooks[h.ID] = h
	}

	if err := d.loadUsage(); err != nil {
		return err
	}
	if err := d.loadSessionUsage(); err != nil {
		return err
	}
	if err := d.loadToolConfig(); err != nil {
		return err
	}
	if err := d.loadSessions(); err != nil {
		return err
	}
	// Reclaim any assistant turn that was streaming when the process last died,
	// so a mid-turn crash leaves a partial-but-saved reply instead of nothing.
	if err := d.recoverInflight(); err != nil {
		return err
	}
	// Consolidate any pre-unification files into the per-session artifacts layout
	// and back existing chat attachments with artifacts (idempotent, best-effort).
	d.migrateUnifiedLayout()
	// Reclaim transient render_template output: drop dirs for sessions that no
	// longer exist and TTL-sweep stale files (startup-only, best-effort).
	d.cleanupRenders()
	return nil
}

// ---- context is accepted for API parity but not used by the file store ----
var _ = context.Background
