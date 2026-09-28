package db

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type cachedJSONRow[T any] struct {
	Stamp fileStamp `json:"stamp"`
	Value T         `json:"value"`
}

type jsonDirectoryCache[T any] struct {
	Version int                         `json:"version"`
	Rows    map[string]cachedJSONRow[T] `json:"rows"`
}

// loadCachedJSONDir validates every canonical file's directory metadata. Only
// changed rows need opening; old binaries and external edits remain compatible.
// The sidecar is disposable and deliberately lives outside the entity directory.
func loadCachedJSONDir[T any](dir, cachePath string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cached jsonDirectoryCache[T]
	if readJSONFile(cachePath, &cached) != nil || cached.Version != 1 {
		cached.Rows = nil
	}
	type candidate struct {
		name  string
		stamp fileStamp
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{e.Name(), fileStamp{info.Size(), info.ModTime().UnixNano()}})
	}
	type result struct {
		name string
		row  cachedJSONRow[T]
		ok   bool
	}
	loaded, err := parallelLoad(candidates, func(c candidate) (result, error) {
		if row, ok := cached.Rows[c.name]; ok && row.Stamp == c.stamp {
			return result{c.name, row, true}, nil
		}
		var value T
		if err := readJSONFile(filepath.Join(dir, c.name), &value); err != nil {
			slog.Warn("skipping unreadable entity", "path", filepath.Join(dir, c.name), "error", err)
			return result{}, nil
		}
		return result{c.name, cachedJSONRow[T]{c.stamp, value}, true}, nil
	})
	if err != nil {
		return nil, err
	}
	next := jsonDirectoryCache[T]{Version: 1, Rows: make(map[string]cachedJSONRow[T], len(loaded))}
	values := make([]T, 0, len(loaded))
	for _, item := range loaded {
		if item.ok {
			next.Rows[item.name] = item.row
			values = append(values, item.row.Value)
		}
	}
	_ = atomicWriteJSON(cachePath, next)
	return values, nil
}
