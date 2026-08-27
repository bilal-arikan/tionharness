package mcp

import (
	"strings"
	"sync"
	"time"
)

// stderrTailCap is how much of a server's stderr we keep. Small on purpose: the
// useful part of a startup failure is the last line or two, and this buffer is
// held for the whole life of every pooled connection.
const stderrTailCap = 8 << 10

// stderrTail is a bounded, non-blocking sink for a subprocess's stderr.
//
// WHY THIS EXISTS: stderr used to go to io.Discard, with the comment "so a chatty
// server can't block on a full pipe". The blocking concern is real, but discarding
// is a costly way to solve it -- it throws away the only channel a stdio MCP server
// has to explain itself. On 2026-08-27 codebase-memory-mcp refused every new client
// with "CBM daemon is active or starting but could not accept this client within
// 30000 ms" and exited; all TionHarness ever logged was "mcp initialize: mcp read:
// EOF", which says a pipe closed but not why, and every agent turn in the workspace
// stalled before its first LLM call while the log repeated that same empty warning
// every 32 seconds.
//
// Writes never block and never grow: once full, the oldest bytes are dropped.
type stderrTail struct {
	mu  sync.Mutex
	buf []byte
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Report the full length as written regardless of how much we keep: this is a
	// sink, and a short write would make os/exec's copier treat it as an error and
	// stop draining the pipe -- reintroducing exactly the block we are avoiding.
	if len(p) >= stderrTailCap {
		s.buf = append(s.buf[:0], p[len(p)-stderrTailCap:]...)
		return len(p), nil
	}
	if len(s.buf)+len(p) > stderrTailCap {
		drop := len(s.buf) + len(p) - stderrTailCap
		s.buf = append(s.buf[:0], s.buf[drop:]...)
	}
	s.buf = append(s.buf, p...)
	return len(p), nil
}

// Tail returns the retained stderr, trimmed and collapsed to a single line so it
// can ride along inside a one-line slog error field.
func (s *stderrTail) Tail() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := strings.TrimSpace(string(s.buf))
	if out == "" {
		return ""
	}
	fields := strings.Fields(out)
	return strings.Join(fields, " ")
}

// waitNonEmpty blocks until something has been written or d elapses.
//
// WHY: os/exec copies the child's stderr on its own goroutine and only guarantees
// that copy is finished once Wait returns. StdioClient.Close waits for Wait, but
// caps that wait at 2s and then kills -- so on a loaded machine the diagnostic
// could be read before the copier had flushed, and the whole point of this buffer
// is to be there exactly when things are going wrong. This is only ever called on
// the failure path, where a short wait costs nothing.
func (s *stderrTail) waitNonEmpty(d time.Duration) {
	if s == nil {
		return
	}
	deadline := time.Now().Add(d)
	for {
		s.mu.Lock()
		n := len(s.buf)
		s.mu.Unlock()
		if n > 0 || !time.Now().Before(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// lastLines keeps only the final n non-empty lines of the tail. Startup failures
// usually end with the actionable message, while everything before it is banner
// and allocator noise.
func (s *stderrTail) lastLines(n int) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	raw := string(s.buf)
	s.mu.Unlock()
	lines := []string{}
	for _, ln := range strings.Split(raw, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
