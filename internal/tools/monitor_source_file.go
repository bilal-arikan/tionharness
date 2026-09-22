package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// fileSourceMaxChunk bounds how many bytes one poll reads from a growing file.
// A process that dumps megabytes between two ticks must not turn a single poll
// into an unbounded read; the remainder is picked up by the next tick.
const fileSourceMaxChunk = 256 * 1024

// fileSource tails a file for a monitor: every poll reads whatever was appended
// since the previous one and reports it line by line.
//
// It polls os.Stat rather than using fsnotify. Three reasons, in order of weight:
//   - The manager already owns a 1s ticker driving every source (monitor.go), so a
//     watcher would add a second, independent event path for no latency the agent
//     can perceive — a wake is gated by monitorMinCooldown (5s) anyway.
//   - Appended bytes still have to be read and split on a size cursor regardless of
//     how the change was noticed, so a watcher would replace none of this code.
//   - On Windows ReadDirectoryChangesW coalesces and occasionally drops events for
//     files written by another process without FILE_SHARE semantics; size polling
//     has no such blind spot. It costs one Stat per second per monitor.
//
// Truncation (a rotated log) is detected by the size going backwards: the cursor
// resets to 0 and reading resumes from the new start rather than seeking past EOF.
type fileSource struct {
	path string
	// display is the path as it is shown to the agent (sandbox-relative when
	// possible), so the monitor list stays readable.
	display string
	// cur is the byte offset already reported. A monitor reports what happens from
	// now on, so it starts at the file's current size.
	cur int64
	// partial holds a trailing fragment that had no newline yet, so a line split
	// across two polls is reported once, whole.
	partial string
	// missing latches once the terminal "file is gone" condition was reported, so
	// it is produced exactly once.
	missing bool
}

// NewFileSource binds a monitor source to path, resolved through sb so the
// sandbox boundary applies. A path that does not exist (or is a directory) is a
// real error: silently monitoring nothing would leave the agent waiting for a
// wake that can never come.
func NewFileSource(sb Sandbox, path string) (MonitorSource, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required to monitor a file")
	}
	abs, err := sb.Resolve(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot monitor %q: %w", path, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("cannot monitor %q: it is a directory, not a file", path)
	}
	return &fileSource{path: abs, display: sb.Rel(abs), cur: st.Size()}, nil
}

// Poll reports one event per newly appended line.
func (s *fileSource) Poll(_ context.Context) ([]MonitorEvent, bool, string, error) {
	st, err := os.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if s.missing {
				return nil, true, "file " + s.display + " no longer exists", nil
			}
			s.missing = true
			return nil, true, "file " + s.display + " was removed", nil
		}
		// Transient (a sharing violation while another process writes): recorded as
		// the last error, polling continues.
		return nil, false, "", err
	}

	size := st.Size()
	if size < s.cur {
		// Truncated or rotated in place: start over from the new beginning rather
		// than sitting past EOF forever.
		s.cur = 0
		s.partial = ""
	}
	if size == s.cur {
		return nil, false, "", nil
	}

	f, err := os.Open(s.path)
	if err != nil {
		return nil, false, "", err
	}
	defer f.Close()

	n := min(size-s.cur, fileSourceMaxChunk)
	buf := make([]byte, n)
	read, err := f.ReadAt(buf, s.cur)
	if err != nil && err != io.EOF {
		return nil, false, "", err
	}
	buf = buf[:read]
	s.cur += int64(read)

	chunk := s.partial + string(bytes.ReplaceAll(buf, []byte("\r\n"), []byte("\n")))
	s.partial = ""
	lines := strings.Split(chunk, "\n")
	// A chunk that does not end in a newline holds an incomplete final line; keep
	// it for the next poll instead of reporting half a line.
	if !strings.HasSuffix(chunk, "\n") {
		s.partial = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
		// A single line longer than the chunk cap would otherwise be buffered
		// forever; flush it once it alone exceeds the cap.
		if len(s.partial) > fileSourceMaxChunk {
			lines = append(lines, s.partial)
			s.partial = ""
		}
	}

	now := time.Now()
	var events []MonitorEvent
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(line) > monitorPayloadBytes {
			line = line[:monitorPayloadBytes] + "…[truncated]"
		}
		events = append(events, MonitorEvent{At: now, Payload: line})
	}
	return events, false, "", nil
}

// Describe identifies the source in the monitor list.
func (s *fileSource) Describe() string { return "file " + s.display }

// Close releases the source. Nothing is held open between polls.
func (s *fileSource) Close() {}
