package providers

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CodexResumeHomeGrace keeps an unreferenced resume home this long after its
// last write. A cold turn writes its rollout before the session row records the
// new cliSessionId, so a young home without a referencing session is normal.
const CodexResumeHomeGrace = 24 * time.Hour

const codexResumeTrashPrefix = ".trash-"

// codexResumeHomesInUse counts the turns holding each resume home, so a sweep
// never removes the home of a turn that is starting or running. The mutex also
// orders a sweep's rename against prepareCodexTurnHome's MkdirAll.
var codexResumeHomesInUse = struct {
	sync.Mutex
	refs map[string]int
}{refs: map[string]int{}}

func acquireCodexResumeHome(dir string) (release func()) {
	codexResumeHomesInUse.Lock()
	codexResumeHomesInUse.refs[dir]++
	codexResumeHomesInUse.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			codexResumeHomesInUse.Lock()
			defer codexResumeHomesInUse.Unlock()
			if codexResumeHomesInUse.refs[dir]--; codexResumeHomesInUse.refs[dir] <= 0 {
				delete(codexResumeHomesInUse.refs, dir)
			}
		})
	}
}

// ResumeHomePruneResult reports one PruneCodexResumeHomes pass.
type ResumeHomePruneResult struct {
	Scanned    int
	Removed    int
	FreedBytes int64
}

// PruneCodexResumeHomes deletes the scoped resume homes under base that no
// session can resume any more. Each scope (session, agent, provider, model,
// system prompt) gets its own home, so a prompt or model change, a deleted
// session or a deleted agent abandons the old one for good; nothing else ever
// removes them.
//
// A home is kept when any rollout in it belongs to a thread in live (the
// cliSessionId values sessions still hold), when it was written within grace,
// or when a turn holds it right now. The rest are renamed aside under the
// in-use lock and deleted outside it, so a turn starting meanwhile gets a fresh
// home instead of a half-deleted one.
func PruneCodexResumeHomes(base string, live map[string]struct{}, grace time.Duration, now time.Time) (ResumeHomePruneResult, error) {
	var res ResumeHomePruneResult
	if strings.TrimSpace(base) == "" {
		return res, nil
	}
	root := filepath.Join(base, codexResumeHomesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, name)
		if strings.HasPrefix(name, codexResumeTrashPrefix) {
			// Left behind by a pass interrupted mid-delete.
			res.FreedBytes += dirSize(dir)
			_ = os.RemoveAll(dir)
			continue
		}
		if !isCodexResumeHomeName(name) {
			continue
		}
		res.Scanned++
		if codexHomeHoldsLiveThread(dir, live) || now.Sub(newestWrite(dir)) < grace {
			continue
		}
		trash := filepath.Join(root, codexResumeTrashPrefix+name)
		if !moveIdleResumeHome(dir, trash) {
			continue
		}
		res.FreedBytes += dirSize(trash)
		if err := os.RemoveAll(trash); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

// moveIdleResumeHome renames dir to trash unless a turn holds it.
func moveIdleResumeHome(dir, trash string) bool {
	codexResumeHomesInUse.Lock()
	defer codexResumeHomesInUse.Unlock()
	if codexResumeHomesInUse.refs[dir] > 0 {
		return false
	}
	return os.Rename(dir, trash) == nil
}

// isCodexResumeHomeName matches the hex sha256 names codexResumeHome creates,
// so a stray folder someone put there is never touched.
func isCodexResumeHomeName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for _, r := range name {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// codexHomeHoldsLiveThread reports whether a rollout in the home belongs to a
// live thread, matching file names the way codexThreadExists does.
func codexHomeHoldsLiveThread(home string, live map[string]struct{}) bool {
	if len(live) == 0 {
		return false
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		base := strings.TrimSuffix(entry.Name(), ".jsonl")
		for id := range live {
			if strings.HasSuffix(base, "-"+id) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

// newestWrite returns the latest modification time of the home, its top-level
// entries (config, sqlite state and WAL files) and its rollouts. The plugin and
// cache trees are skipped: they are copied in, not written by a turn.
func newestWrite(home string) time.Time {
	var newest time.Time
	note := func(info fs.FileInfo) {
		if info != nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if info, err := os.Stat(home); err == nil {
		note(info)
	}
	entries, _ := os.ReadDir(home)
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil {
			note(info)
		}
	}
	_ = filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil {
			if info, err := entry.Info(); err == nil {
				note(info)
			}
		}
		return nil
	})
	return newest
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
