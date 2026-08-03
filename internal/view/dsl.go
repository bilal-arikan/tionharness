package view

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// lines is a tiny accumulator for the compact DSL. It exists so projections read
// as a list of facts instead of a pile of string concatenation, and so every
// projection formats durations and elisions identically.
type lines struct {
	b strings.Builder
}

// add appends one formatted line.
func (l *lines) add(format string, args ...any) {
	if len(args) == 0 {
		l.b.WriteString(format)
	} else {
		fmt.Fprintf(&l.b, format, args...)
	}
	l.b.WriteString("\n")
}

// addIf appends the line only when cond holds — the common shape for L1 signals.
func (l *lines) addIf(cond bool, format string, args ...any) {
	if cond {
		l.add(format, args...)
	}
}

// empty reports whether nothing has been written yet.
func (l *lines) empty() bool { return l.b.Len() == 0 }

// String returns the accumulated text with the trailing newline trimmed.
func (l *lines) String() string { return strings.TrimRight(l.b.String(), "\n") }

// dur renders a duration in the shortest unambiguous form: "820ms", "31s",
// "2m14s", "3s12d" style is avoided — durations here are always wall-clock
// elapsed, never calendar ages (see age for that).
func dur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		s := int(d.Seconds())
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	default:
		m := int(d.Minutes())
		return fmt.Sprintf("%dh%02dm", m/60, m%60)
	}
}

// durMs renders a millisecond span. 0 means "unknown", which the caller should
// check before calling — an unknown span must not render as "0ms".
func durMs(ms int64) string { return dur(time.Duration(ms) * time.Millisecond) }

// age renders how long ago a unix-millisecond timestamp was, in calendar-ish
// units ("7g" = 7 gün). Used for staleness signals.
func age(unixMs int64, now time.Time) string {
	if unixMs <= 0 {
		return "?"
	}
	d := now.Sub(time.UnixMilli(unixMs))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%dsn", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%ddk", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dsa", int(d.Hours()))
	default:
		return fmt.Sprintf("%dg", int(d.Hours()/24))
	}
}

// clip shortens s to at most max runes, marking the cut with "…". Newlines and
// control characters collapse to spaces first: a projection line must stay ONE
// line, or the DSL stops being parseable by eye.
func clip(s string, max int) string {
	s = strings.TrimSpace(collapseSpace(s))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// collapseSpace turns every run of whitespace (including newlines) into a single
// space.
func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteRune(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// hhmmss formats a wall-clock stamp for the asOf marker.
func hhmmss(t time.Time) string { return t.Format("15:04:05") }
