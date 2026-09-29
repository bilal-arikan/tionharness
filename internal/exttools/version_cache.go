package exttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const localVersionTTL = 5 * time.Minute

type versionEntry struct {
	value   string
	expires time.Time
}

type versionCache struct {
	mu         sync.Mutex
	generation uint64
	entries    map[string]versionEntry
	requests   sharedRequests[string]
}

var localVersions versionCache

// InvalidateVersionCache also prevents an already running old probe from
// repopulating the cache after an install or explicit refresh.
func InvalidateVersionCache() { localVersions.invalidate() }

func (c *versionCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.entries = nil
}

func versionProbeKey(path string, args []string) string {
	if resolved, err := lookPath(path); err == nil {
		path = resolved
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	var stamp string
	if info, err := os.Stat(path); err == nil {
		stamp = fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	}
	encoded, _ := json.Marshal(append([]string{path, stamp}, args...))
	return string(encoded)
}

func (c *versionCache) get(ctx context.Context, key string, probe func(context.Context) (string, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	gen := c.generation
	entry, found := c.entries[key]
	fresh := found && time.Now().Before(entry.expires)
	c.mu.Unlock()
	if fresh {
		return entry.value, nil
	}
	return c.requests.do(ctx, fmt.Sprintf("%d:%s", gen, key), versionTimeout, func(work context.Context) (string, error) {
		c.mu.Lock()
		entry, found := c.entries[key]
		fresh := c.generation == gen && found && time.Now().Before(entry.expires)
		c.mu.Unlock()
		if fresh {
			return entry.value, nil
		}
		value, err := probe(work)
		if err == nil {
			c.mu.Lock()
			if c.generation == gen {
				if c.entries == nil {
					c.entries = make(map[string]versionEntry)
				}
				// Drop expired probes so changed executable fingerprints do not accumulate.
				for k, v := range c.entries {
					if !time.Now().Before(v.expires) {
						delete(c.entries, k)
					}
				}
				c.entries[key] = versionEntry{value: value, expires: time.Now().Add(localVersionTTL)}
			}
			c.mu.Unlock()
		}
		return value, err
	})
}
