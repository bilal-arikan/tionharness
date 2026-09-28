package api

import (
	"container/list"
	"crypto/sha256"
	"strings"
	"sync"
)

const stepsPreviewCacheBytes = 4 << 20

type stepsPreviewEntry struct {
	key     [32]byte
	preview string
}

var stepsPreviews = struct {
	sync.Mutex
	rows  map[[32]byte]*list.Element
	lru   list.List
	bytes int
}{rows: make(map[[32]byte]*list.Element)}

// Keys describe content, so edits and colliding session ids across workspaces
// cannot serve stale previews. Neither full traces nor unbounded entries stay
// resident; the cached strings own only their trimmed bytes.
func trimStepsJSON(raw string) string {
	if len(raw) <= stepsTrimFloor {
		return raw
	}
	key := sha256.Sum256([]byte(raw))
	stepsPreviews.Lock()
	if e := stepsPreviews.rows[key]; e != nil {
		stepsPreviews.lru.MoveToFront(e)
		preview := e.Value.(stepsPreviewEntry).preview
		stepsPreviews.Unlock()
		return preview
	}
	stepsPreviews.Unlock()
	preview := trimStepsJSONUncached(raw)
	// Invalid or already-small traces need no second retained copy.
	if preview == raw || len(preview) > stepsPreviewCacheBytes/4 {
		return preview
	}
	preview = strings.Clone(preview)
	stepsPreviews.Lock()
	defer stepsPreviews.Unlock()
	if e := stepsPreviews.rows[key]; e != nil {
		return e.Value.(stepsPreviewEntry).preview
	}
	stepsPreviews.rows[key] = stepsPreviews.lru.PushFront(stepsPreviewEntry{key, preview})
	stepsPreviews.bytes += len(preview) + 128
	for stepsPreviews.bytes > stepsPreviewCacheBytes {
		e := stepsPreviews.lru.Back()
		row := e.Value.(stepsPreviewEntry)
		stepsPreviews.bytes -= len(row.preview) + 128
		delete(stepsPreviews.rows, row.key)
		stepsPreviews.lru.Remove(e)
	}
	return preview
}
