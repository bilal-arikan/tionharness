package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// entitySpec describes one migratable entity type: the store subdirectory that
// holds it and the id prefix it should get. Must stay in sync with the prefixes
// in internal/db/db.go.
type entitySpec struct {
	dir      string // subdir under <ws>/store/
	prefix   string
	isFolder bool // sessions are folders (sessions/<id>/session.jsonl), not files
}

// specs is the full migratable set. Messages keep UUIDs (high-volume hot path,
// never user-facing) and task Runs keep UUIDs (legacy, not produced anymore), so
// neither is listed; their references to migrated ids are still fixed by the
// token replacement pass.
var specs = []entitySpec{
	{dir: "agents", prefix: "AGT"},
	{dir: "tasks", prefix: "TSK"},
	{dir: "flows", prefix: "FLW"},
	{dir: "flow-runs", prefix: "RUN"},
	{dir: "artifacts", prefix: "ART"},
	{dir: "knowledge", prefix: "MEM"},
	{dir: "mcp-servers", prefix: "MCP"},
	{dir: "hooks", prefix: "HOK"},
	{dir: "schedules", prefix: "SCH"},
	{dir: "sessions", prefix: "SES", isFolder: true},
}

// idRec is the minimal shape read from an entity JSON / session header.
type idRec struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"createdAt"`
}

// remap is the result of scanning a workspace: the old->new id map plus the
// per-prefix counter maxes to persist into counters.json.
type remap struct {
	m        map[string]string // old id -> new id
	counters map[string]int64  // prefix -> last n
}

// buildRemap scans every migratable entity in a workspace store and assigns a
// new prefixed id to each legacy (non-prefixed) one, continuing the per-prefix
// sequence from the existing counters.json (so it never collides with ids the
// running app already minted under the new scheme).
func buildRemap(storeDir string) (*remap, error) {
	r := &remap{m: map[string]string{}, counters: loadCounters(storeDir)}

	for _, spec := range specs {
		recs, err := scanEntities(storeDir, spec)
		if err != nil {
			return nil, err
		}
		// Stable order by creation time so older entities get lower numbers.
		sort.SliceStable(recs, func(i, j int) bool { return recs[i].CreatedAt < recs[j].CreatedAt })

		newRe := regexp.MustCompile("^" + spec.prefix + `\d+$`)
		for _, rec := range recs {
			if rec.ID == "" {
				continue
			}
			if newRe.MatchString(rec.ID) {
				// Already migrated — keep the counter ahead of it.
				if n := numSuffix(rec.ID, spec.prefix); n > r.counters[spec.prefix] {
					r.counters[spec.prefix] = n
				}
				continue
			}
			r.counters[spec.prefix]++
			r.m[rec.ID] = spec.prefix + strconv.FormatInt(r.counters[spec.prefix], 10)
		}
	}
	return r, nil
}

// scanEntities returns the id records for one entity type.
func scanEntities(storeDir string, spec entitySpec) ([]idRec, error) {
	dir := filepath.Join(storeDir, spec.dir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []idRec
	for _, e := range entries {
		if spec.isFolder {
			if !e.IsDir() {
				continue
			}
			rec := idRec{ID: e.Name()}
			// Read createdAt from the session header (first JSONL line) if present.
			if hdr, ok := readSessionHeader(filepath.Join(dir, e.Name(), "session.jsonl")); ok {
				rec.CreatedAt = hdr.CreatedAt
			}
			out = append(out, rec)
			continue
		}
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		var rec idRec
		if err := readJSON(filepath.Join(dir, name), &rec); err != nil {
			continue // unreadable file: skip, leave untouched
		}
		if rec.ID == "" {
			// Fall back to the filename (sans .json) as the id.
			rec.ID = strings.TrimSuffix(name, ".json")
		}
		out = append(out, rec)
	}
	return out, nil
}

func readSessionHeader(path string) (idRec, bool) {
	f, err := os.Open(path)
	if err != nil {
		return idRec{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	if sc.Scan() {
		var rec idRec
		if json.Unmarshal(sc.Bytes(), &rec) == nil {
			return rec, true
		}
	}
	return idRec{}, false
}

// numSuffix parses the numeric tail of a "<prefix><n>" id (0 if it doesn't fit).
func numSuffix(id, prefix string) int64 {
	n, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// ---- counters.json (matches internal/db) ----

func countersPath(storeDir string) string { return filepath.Join(storeDir, "counters.json") }

func loadCounters(storeDir string) map[string]int64 {
	c := map[string]int64{}
	var raw map[string]int64
	if readJSON(countersPath(storeDir), &raw) == nil && raw != nil {
		for k, v := range raw {
			c[k] = v
		}
	}
	return c
}

func writeCounters(storeDir string, c map[string]int64) error {
	return writeJSONAtomic(countersPath(storeDir), c)
}

// ---- small JSON helpers (atomic write mirrors internal/db) ----

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// reportRemap prints a compact per-prefix summary plus a few samples.
func reportRemap(label string, r *remap) {
	if len(r.m) == 0 {
		fmt.Printf("  %s: nothing to migrate (already prefixed or empty)\n", label)
		return
	}
	byPrefix := map[string]int{}
	for _, spec := range specs {
		byPrefix[spec.prefix] = 0
	}
	for _, nw := range r.m {
		byPrefix[prefixOf(nw)]++
	}
	fmt.Printf("  %s: %d id(s) to remap\n", label, len(r.m))
	for _, spec := range specs {
		if c := byPrefix[spec.prefix]; c > 0 {
			fmt.Printf("      %-3s x%-4d -> next counter %d\n", spec.prefix, c, r.counters[spec.prefix])
		}
	}
}

func prefixOf(newID string) string {
	i := strings.IndexFunc(newID, func(r rune) bool { return r >= '0' && r <= '9' })
	if i < 0 {
		return newID
	}
	return newID[:i]
}
