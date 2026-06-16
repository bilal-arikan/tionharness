// Package logbuf captures slog records into an in-memory ring buffer so the web
// UI can display application + workspace logs live. A single buffer is shared by
// the whole process (every workspace runtime logs through the same logger), so
// it holds the complete cross-workspace log stream.
package logbuf

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Entry is one captured log record in a UI-friendly shape.
type Entry struct {
	Seq     int64             `json:"seq"`
	Time    int64             `json:"time"` // unix milliseconds
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

// Buffer is a fixed-capacity ring of recent log entries.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	seq     atomic.Int64
}

// New creates a ring buffer holding up to max entries.
func New(max int) *Buffer {
	if max <= 0 {
		max = 2000
	}
	return &Buffer{max: max}
}

// add appends an entry, dropping the oldest when over capacity.
func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		// Drop the oldest chunk to amortise the cost of trimming.
		drop := len(b.entries) - b.max
		b.entries = append([]Entry(nil), b.entries[drop:]...)
	}
}

// Entries returns up to limit most-recent entries (oldest→newest). limit <= 0
// returns all retained entries.
func (b *Buffer) Entries(limit int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := len(b.entries)
	start := 0
	if limit > 0 && limit < n {
		start = n - limit
	}
	out := make([]Entry, n-start)
	copy(out, b.entries[start:])
	return out
}

// Handler returns an slog.Handler that captures into this buffer and forwards to
// inner (e.g. the stdout text handler).
func (b *Buffer) Handler(inner slog.Handler) slog.Handler {
	return &handler{inner: inner, buf: b}
}

// handler implements slog.Handler, tee-ing records to the buffer and inner.
type handler struct {
	inner slog.Handler
	buf   *Buffer
	attrs []slog.Attr
	group string
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	for _, a := range h.attrs {
		attrs[h.key(a.Key)] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[h.key(a.Key)] = a.Value.String()
		return true
	})
	if len(attrs) == 0 {
		attrs = nil
	}
	h.buf.add(Entry{
		Seq:     h.buf.seq.Add(1),
		Time:    r.Time.UnixMilli(),
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrs,
	})
	return h.inner.Handle(ctx, r)
}

func (h *handler) key(k string) string {
	if h.group != "" {
		return h.group + "." + k
	}
	return k
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	merged := append(append([]slog.Attr(nil), h.attrs...), as...)
	return &handler{inner: h.inner.WithAttrs(as), buf: h.buf, attrs: merged, group: h.group}
}

func (h *handler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" && name != "" {
		g = h.group + "." + name
	}
	return &handler{inner: h.inner.WithGroup(name), buf: h.buf, attrs: h.attrs, group: g}
}
