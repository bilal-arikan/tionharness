// Package logbuf captures slog records into an in-memory ring buffer so the web
// UI can display application + workspace logs live. A single buffer is shared by
// the whole process (every workspace runtime logs through the same logger), so
// it holds the complete cross-workspace log stream.
package logbuf

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

const capturedStringLimit = 16 * 1024

// Entry is one captured log record in a UI-friendly shape. Component, Session,
// Agent and Workspace are first-class fields (promoted from same-named slog
// attrs) so the UI and API can filter by source without substring-scanning the
// attrs map.
type Entry struct {
	Seq       int64             `json:"seq"`
	Time      int64             `json:"time"` // unix milliseconds
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Component string            `json:"component,omitempty"` // originating subsystem (api/agent/scheduler/…)
	Session   string            `json:"session,omitempty"`   // session id, when the record carries one
	Agent     string            `json:"agent,omitempty"`     // agent id, when the record carries one
	Workspace string            `json:"workspace,omitempty"` // workspace id, when the record carries one
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// Buffer is a fixed-capacity ring of recent log entries.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	seq     atomic.Int64
	// notify, when set, receives every appended entry (after it is stored).
	// Used to fan captured logs out over the SSE event bus for live tailing.
	// Must be fast and non-blocking; called outside the buffer lock.
	notify atomic.Pointer[func(Entry)]
}

// New creates a ring buffer holding up to max entries.
func New(max int) *Buffer {
	if max <= 0 {
		max = 2000
	}
	return &Buffer{max: max}
}

// SetNotify installs a callback invoked for every appended entry (after it is
// stored). Pass nil to remove. The callback runs on the logging goroutine, so
// it must never block or log (a logging callback would recurse).
func (b *Buffer) SetNotify(fn func(Entry)) {
	if fn == nil {
		b.notify.Store(nil)
		return
	}
	b.notify.Store(&fn)
}

// add appends an entry, dropping the oldest when over capacity.
func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		// Drop the oldest chunk to amortise the cost of trimming.
		drop := len(b.entries) - b.max
		b.entries = append([]Entry(nil), b.entries[drop:]...)
	}
	b.mu.Unlock()
	if fn := b.notify.Load(); fn != nil {
		(*fn)(e)
	}
}

// Entries returns up to limit most-recent entries (oldest→newest). limit <= 0
// returns all retained entries.
// Collect returns, in chronological order, the newest entries for which keep
// is true, at most limit of them (limit <= 0 means every match). It walks the
// ring from the newest entry backwards and stops as soon as the limit is met,
// so a filtered tail costs the matching window rather than a copy of the whole
// retained buffer followed by a full filter pass.
func (b *Buffer) Collect(limit int, keep func(Entry) bool) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cap := limit
	if cap <= 0 || cap > len(b.entries) {
		cap = len(b.entries)
	}
	out := make([]Entry, 0, cap)
	for i := len(b.entries) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		if keep(b.entries[i]) {
			out = append(out, b.entries[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

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
		attrs[h.key(a.Key)] = truncateCapturedString(a.Value.String())
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[h.key(a.Key)] = truncateCapturedString(a.Value.String())
		return true
	})
	e := Entry{
		Seq:     h.buf.seq.Add(1),
		Time:    r.Time.UnixMilli(),
		Level:   r.Level.String(),
		Message: truncateCapturedString(r.Message),
	}
	// Promote well-known source attrs to first-class fields so the API/UI can
	// filter by them directly. Promoted keys are removed from the generic map.
	promote := func(key string, dst *string) {
		if v, ok := attrs[key]; ok {
			*dst = v
			delete(attrs, key)
		}
	}
	promote("component", &e.Component)
	promote("session", &e.Session)
	promote("agent", &e.Agent)
	promote("workspace", &e.Workspace)
	if len(attrs) > 0 {
		e.Attrs = attrs
	}
	h.buf.add(e)
	return h.inner.Handle(ctx, r)
}

func truncateCapturedString(s string) string {
	if len(s) <= capturedStringLimit {
		return s
	}
	marker := fmt.Sprintf("… [%d bytes omitted]", len(s))
	limit := capturedStringLimit - len(marker)
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	omitted := len(s) - limit
	marker = fmt.Sprintf("… [%d bytes omitted]", omitted)
	limit = capturedStringLimit - len(marker)
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	omitted = len(s) - limit
	return s[:limit] + fmt.Sprintf("… [%d bytes omitted]", omitted)
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
