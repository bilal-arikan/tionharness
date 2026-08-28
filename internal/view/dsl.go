package view

import (
	"fmt"
	"os"
	"regexp"
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

// durSec renders a second-resolution span.
func durSec(s int64) string { return dur(time.Duration(s) * time.Second) }

// tsSec / tsMs convert a stored timestamp to a time.Time, naming the unit at the
// call site.
//
// This exists because the persisted models MIX units and nothing in the types
// says which is which: db.now() (every entity's CreatedAt/UpdatedAt) and
// orchestration.TraceEntry.At are unix SECONDS, while TraceEntry.StartMs/EndMs
// are MILLISECONDS. Passing a raw int64 around made it possible — and it
// happened — to read a seconds value as millis and render a card updated
// yesterday as "20648g önce". A projection whose whole promise is "these numbers
// are computed, so trust them" cannot afford that, so the unit is now spelled out
// wherever a timestamp enters this package.
//
// A non-positive value means "unset" and yields the zero Time, which age()
// reports as "?" rather than as 1970.
func tsSec(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func tsMs(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(v)
}

// age renders how long ago t was, in calendar-ish units ("7g" = 7 gün). Used for
// staleness signals. A zero t (unset timestamp) renders "?".
func age(t, now time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
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

// homeDir resolves the current user's home directory. It is a var so tests can
// pin a home regardless of the host OS (os.UserHomeDir reads USERPROFILE on
// Windows and HOME elsewhere, which would make the expectations platform-bound).
var homeDir = os.UserHomeDir

// shortPath makes a filesystem path readable inside a one-line projection
// WITHOUT throwing information away: separators normalise to "/" (so a Windows
// path and its bash-mounted twin read the same) and the user's home prefix
// collapses to "~", which is where most of the uninformative length lives.
//
// A string with no separator is not a path and is returned untouched — rewriting
// a title or a tool name here would misrepresent what the value is.
func shortPath(s string) string {
	p := strings.ReplaceAll(s, `\`, "/")
	if !strings.Contains(p, "/") {
		return s
	}
	home, err := homeDir()
	if err != nil || home == "" {
		return p
	}
	h := strings.TrimRight(strings.ReplaceAll(home, `\`, "/"), "/")
	if h == "" || len(p) < len(h) || !strings.EqualFold(p[:len(h)], h) {
		return p
	}
	// Only a whole-segment prefix match counts: "/home/bil" must not swallow the
	// first segment of "/home/bilal-backup".
	rest := p[len(h):]
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return p
	}
	return "~" + rest
}

// clipPath is clip for values that MAY be filesystem paths. It shortens with
// shortPath first, then — if the result is still longer than max runes — keeps
// the TAIL and marks the cut at the FRONT with "…/".
//
// clip cuts from the right, which is exactly backwards for a path:
// "C:/Users/user/Desktop/Projects/TionHar…" identifies nothing, while
// "…/features/view/ViewPanel.tsx" identifies the file. The cut lands on a
// separator boundary so a rendered segment is always a real segment; a single
// segment longer than max is the one case that must be cut mid-word.
//
// A value with no separator is not a path, so it falls back to clip and keeps
// the front-preserving behaviour prose needs. Whitespace collapses like clip —
// a projection line must stay ONE line.
func clipPath(s string, max int) string {
	s = strings.TrimSpace(collapseSpace(s))
	if !strings.ContainsAny(s, `/\`) {
		return clip(s, max)
	}
	s = shortPath(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	const marker = "…/"
	budget := max - len([]rune(marker))
	if budget <= 0 {
		return clip(s, max)
	}
	tail := string(r[len(r)-budget:])
	// Prefer whole segments: drop the partial leading segment when one remains.
	if i := strings.Index(tail, "/"); i >= 0 && i+1 < len(tail) {
		tail = tail[i+1:]
	}
	return marker + tail
}

// pathStopChars are the characters an absolute path may NOT contain in prose.
// They are the delimiters a path is normally wrapped in — whitespace, quotes,
// backticks — plus the punctuation a sentence puts right after one. Stopping
// there is what keeps a trailing "," or ")" out of the rewritten path.
const pathStopChars = "\\s\"'" + "`" + ",;)>"

// absPathRe matches an absolute filesystem path anywhere inside a string:
// a Windows drive path ("C:\..." / "C:/...") or a POSIX path of at least TWO
// segments ("/usr/bin"). One segment is not enough — a lone "/x" is far more
// often a separator in prose than a path. Compiled once: this runs on every
// rendered line.
var absPathRe = regexp.MustCompile(
	`[A-Za-z]:[\\/][^` + pathStopChars + `]*` +
		`|/[^` + pathStopChars + `/]+(?:/[^` + pathStopChars + `]*)+`)

// inlinePathBudget is how many runes one path may occupy inside a prose line.
// The lines this runs on are clipped at 70–200 runes, so a path longer than
// ~44 would eat most of the sentence it sits in and the clip would then cut the
// prose instead. Three tail segments ("…/internal/view/session.go") stay under
// it in practice while still identifying the file.
const inlinePathBudget = 44

// compactPaths rewrites every absolute path occurring INSIDE s, leaving the
// surrounding prose byte-identical.
//
// clipPath only helps when the whole value IS a path. Most session lines are
// prose that merely CONTAINS one — a chat message, an error string, a "şu an:"
// activity line — and there clip() cuts the sentence, so the embedded path is
// exactly what loses its informative tail. Shortening the path in place keeps
// both the sentence and the file it names.
//
// A string with no path match is returned unchanged, not rebuilt.
func compactPaths(s string) string {
	locs := absPathRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		// A path-looking run glued to the previous character is part of a bigger
		// token — the "/host/path" of a URL after "://", or the tail of a path the
		// previous match already consumed. Rewriting it would corrupt that token.
		if start > 0 && !isPathBoundary(s[start-1]) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(compactOnePath(s[start:end]))
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// isPathBoundary reports whether c may directly precede a path. Anything that
// could be part of a longer token (letters, digits, ":" of a URL scheme, a
// separator) disqualifies the match.
func isPathBoundary(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	case c == ':', c == '/', c == '\\', c == '.', c == '_', c == '-', c == '~':
		return false
	default:
		return true
	}
}

// compactOnePath shortens a single matched path: shortPath first (home → "~",
// separators normalised), then — only if it is still over budget — the last
// three segments behind a "…/" marker.
func compactOnePath(p string) string {
	p = shortPath(p)
	if len([]rune(p)) <= inlinePathBudget {
		return p
	}
	segs := strings.Split(p, "/")
	if len(segs) <= 3 {
		// Nothing to fold: the length lives in the segments themselves, and cutting
		// mid-segment here would produce the half-eaten path this helper exists to
		// prevent.
		return p
	}
	return "…/" + strings.Join(segs[len(segs)-3:], "/")
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

// usd renders a dollar amount for the DSL. Sub-cent figures keep three decimals
// so a busy-but-cheap workspace does not round to "$0.00" and read as free;
// everything else uses two. estimated prefixes "~" (subscription providers price
// via an equivalent-API estimate, so the figure is not a real invoice).
func usd(amount float64, estimated bool) string {
	prefix := "$"
	if estimated {
		prefix = "~$"
	}
	if amount > 0 && amount < 0.01 {
		return fmt.Sprintf("%s%.3f", prefix, amount)
	}
	return fmt.Sprintf("%s%.2f", prefix, amount)
}
