package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// optimizerLogSize bounds the recent-optimization ring. A turn rarely runs more
// than a few dozen shell calls, so 128 covers a long turn while keeping the log
// to a few KB (only hashes are stored, never the outputs themselves).
const optimizerLogSize = 128

// optimizerLog remembers which shell OUTPUTS were produced by a token optimizer,
// keyed by a hash of the output text.
//
// # Why a log and not a ctx sink
//
// On the claude-cli path the CLI owns the tool loop: our shell runs inside the
// Interaction MCP bridge (Runtime.NewShellRunner), while the STEPS are rebuilt
// afterwards from the CLI's stream-json trace. The two are different call stacks,
// so the per-call ctx sink the native loop uses cannot reach the trace conversion.
//
// # Why the OUTPUT is the key, not the command
//
// Keying by command would need FIFO bookkeeping (the same command may run twice
// in a turn) and would be order-sensitive. The output is the thing that actually
// travels from the runner to the trace step, and identical outputs imply an
// identical optimization — so lookup is idempotent, safe to call from both the
// live OnEvent path and the final batch conversion, and needs no draining.
//
// Misses are silent by design: a hook that rewrites the tool result downstream
// breaks the match, and showing no chip is the honest outcome — never a wrong one.
type optimizerLog struct {
	mu    sync.Mutex
	ring  []optimizerLogEntry
	next  int
	index map[string]tools.ShellOptimization
}

type optimizerLogEntry struct {
	key   string
	valid bool
}

// optimizerLogKey hashes the output a shell call returned. Whitespace at the
// edges is trimmed first because the transport (MCP result → CLI tool_result)
// may add or drop a trailing newline.
func optimizerLogKey(output string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(output)))
	return hex.EncodeToString(sum[:])
}

// record notes that output was produced under opt. Recording the oldest entry
// out of the ring also evicts it from the index, so the log stays bounded.
func (l *optimizerLog) record(output string, opt tools.ShellOptimization) {
	if strings.TrimSpace(output) == "" {
		return
	}
	key := optimizerLogKey(output)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.index == nil {
		l.index = make(map[string]tools.ShellOptimization, optimizerLogSize)
		l.ring = make([]optimizerLogEntry, optimizerLogSize)
	}
	if old := l.ring[l.next]; old.valid && old.key != key {
		delete(l.index, old.key)
	}
	l.ring[l.next] = optimizerLogEntry{key: key, valid: true}
	l.next = (l.next + 1) % optimizerLogSize
	l.index[key] = opt
}

// lookup returns the optimization that produced output, or nil when it was not
// optimized (or aged out of the ring). Non-destructive: the same output may be
// looked up by the live step emitter and again by the final trace conversion.
func (l *optimizerLog) lookup(output string) *tools.ShellOptimization {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	opt, ok := l.index[optimizerLogKey(output)]
	if !ok {
		return nil
	}
	return &opt
}
