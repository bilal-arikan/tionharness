package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// One HTTP connection carries the existing feeds without changing their replay,
// presence or notification contracts. Three HTTP/1.1 feeds per browser tab can
// otherwise exhaust the browser's six origin connections and starve JSON reads.
type liveChannel struct {
	Key         string `json:"key"`
	Scope       string `json:"scope"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	Since       int64  `json:"since,omitempty"`
	Epoch       string `json:"epoch,omitempty"`
}

type liveFrame struct {
	Channel string `json:"channel"`
	Frame   string `json:"frame"`
}

type liveFeed struct {
	channel liveChannel
	wsp     *workspace.Workspace
	handler http.HandlerFunc
	err     string
}

func (s *Server) liveFeeds(r *http.Request) ([]liveFeed, error) {
	raw := r.URL.Query().Get("channels")
	if len(raw) > 8192 {
		return nil, errors.New("live subscriptions too large")
	}
	var channels []liveChannel
	if json.Unmarshal([]byte(raw), &channels) != nil || len(channels) == 0 || len(channels) > 8 {
		return nil, errors.New("between one and eight live subscriptions required")
	}
	seen := make(map[string]bool)
	feeds := make([]liveFeed, 0, len(channels))
	for _, ch := range channels {
		if ch.Key == "" || len(ch.Key) > 200 || seen[ch.Key] || ch.Since < 0 {
			return nil, errors.New("invalid live subscription")
		}
		seen[ch.Key] = true
		feed := liveFeed{channel: ch}
		if ch.Scope != "global" {
			if ch.WorkspaceID == "" {
				return nil, errors.New("workspace id required")
			}
			wsp, err := s.workspaces.Get(ch.WorkspaceID)
			if err != nil || wsp == nil {
				return nil, errors.New("unknown workspace " + ch.WorkspaceID)
			}
			feed.wsp = wsp
		}
		switch ch.Scope {
		case "global":
			if s.bus == nil {
				return nil, errors.New("event bus unavailable")
			}
			feed.handler = s.handleEvents
		case "workspace":
			if s.hub == nil {
				return nil, errors.New("session hub unavailable")
			}
			feed.handler = s.handleWorkspaceStream
		case "session":
			if s.hub == nil || ch.SessionID == "" {
				return nil, errors.New("session subscription unavailable")
			}
			if _, err := feed.wsp.DB.GetSession(r.Context(), ch.SessionID); err != nil {
				feed.err = "session not found"
			}
			feed.handler = s.handleSessionStream
		default:
			return nil, errors.New("unknown live subscription scope")
		}
		feeds = append(feeds, feed)
	}
	return feeds, nil
}

func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	feeds, err := s.liveFeeds(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	frames := make(chan liveFrame, 64)
	for _, feed := range feeds {
		if feed.err != "" {
			data, _ := json.Marshal(map[string]string{"message": feed.err})
			frames <- liveFrame{Channel: feed.channel.Key, Frame: "event: error\ndata: " + string(data) + "\n\n"}
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			// If any feed ends (e.g. a deleted session), reconnect the multiplex
			// rather than leaving one channel silently dead forever.
			defer cancel()
			inner := r.Clone(context.WithValue(ctx, workspaceCtxKey, feed.wsp))
			query := inner.URL.Query()
			query.Del("channels")
			query.Set("since", strconv.FormatInt(feed.channel.Since, 10))
			query.Set("epoch", feed.channel.Epoch)
			inner.URL.RawQuery = query.Encode()
			inner.SetPathValue("id", feed.channel.SessionID)
			writer := &liveChannelWriter{ctx: ctx, key: feed.channel.Key, frames: frames, header: make(http.Header)}
			s.withRecover(feed.handler).ServeHTTP(writer, inner)
		}()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-frames:
			data, _ := json.Marshal(frame)
			if _, err := fmt.Fprintf(w, "event: live\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// Each legacy handler owns its writer. Flush moves a complete frame to the
// outer writer; only handleLiveStream ever touches the actual HTTP response.
type liveChannelWriter struct {
	ctx    context.Context
	key    string
	frames chan<- liveFrame
	header http.Header
	buf    bytes.Buffer
}

func (w *liveChannelWriter) Header() http.Header { return w.header }
func (w *liveChannelWriter) WriteHeader(status int) {
	if status >= 400 {
		// Validation happens before opening the outer response. A race with a
		// workspace/session deletion must still close the affected connection.
		return
	}
}
func (w *liveChannelWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.buf.Write(p)
}
func (w *liveChannelWriter) Flush() {
	if w.buf.Len() == 0 {
		return
	}
	frame := liveFrame{Channel: w.key, Frame: w.buf.String()}
	w.buf.Reset()
	select {
	case w.frames <- frame:
	case <-w.ctx.Done():
	}
}
