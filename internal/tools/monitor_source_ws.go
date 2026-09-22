package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// wsSourceQueueCap bounds how many unread messages the reader goroutine keeps
	// for the next poll. A chatty socket must not grow memory without bound
	// between two ticks; overflow is reported as a dropped-message notice so the
	// loss is visible rather than silent.
	wsSourceQueueCap = 256
	// wsSourceReadLimit bounds a single incoming message. The default coder limit
	// (32KB) is already modest; this pins it explicitly next to the payload cap.
	wsSourceReadLimit = 128 * 1024
	// wsSourceHandshakeTimeout bounds the connect, so an unreachable endpoint
	// fails the start call instead of hanging the agent's turn.
	wsSourceHandshakeTimeout = 15 * time.Second
)

// wsSource subscribes to a WebSocket endpoint and reports one event per incoming
// text message.
//
// Unlike the file and URL sources, a socket is PUSH: messages arrive whenever the
// server sends them, not when the manager ticks. So this source owns a reader
// goroutine that drains the connection into a bounded queue, and Poll simply
// hands over whatever accumulated since the last tick. The manager's ticker still
// controls when the agent is woken; the goroutine only makes sure no message is
// missed between two ticks.
//
// Dependency choice: github.com/coder/websocket (formerly nhooyr.io/websocket).
// It is the only widely used Go WebSocket client with ZERO transitive
// dependencies — it adds exactly one module to a tree that has three direct deps
// — has a context-aware API that matches Poll's signature, and dials through a
// caller-supplied *http.Client, which is what lets the same SSRF-guarded
// transport as WebFetch apply here (gorilla/websocket uses its own dialer and
// would need the guard reimplemented).
type wsSource struct {
	rawURL string
	conn   *websocket.Conn
	cancel context.CancelFunc
	client *http.Client

	mu sync.Mutex
	// queue holds messages the reader collected since the last Poll.
	queue []MonitorEvent
	// dropped counts messages discarded because the queue was full.
	dropped int
	// closedReason is set once the reader goroutine ends; a non-empty value is
	// the terminal condition.
	closedReason string
	// reported latches the terminal condition so it is produced exactly once.
	reported bool
}

// NewWSSource dials rawURL and starts draining it. A failed handshake is a real
// error returned to the agent now, rather than a monitor that can never fire.
func NewWSSource(ctx context.Context, rawURL string) (MonitorSource, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required to monitor a WebSocket")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("invalid url %q: a WebSocket url must use ws:// or wss://", rawURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid url %q: no host", rawURL)
	}

	// The same SSRF-guarded transport WebFetch uses: the handshake is a plain
	// HTTP request, so the resolved-IP check applies to it unchanged.
	client := newGuardedHTTPClient(0) // no client timeout: the connection is long-lived
	dialCtx, cancelDial := context.WithTimeout(ctx, wsSourceHandshakeTimeout)
	defer cancelDial()
	conn, _, err := websocket.Dial(dialCtx, rawURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		client.CloseIdleConnections()
		return nil, fmt.Errorf("cannot connect to %s: %w", rawURL, err)
	}
	conn.SetReadLimit(wsSourceReadLimit)

	// The reader outlives the caller's ctx deliberately: the monitor owns the
	// connection until it is stopped or reaches a terminal state.
	readCtx, cancel := context.WithCancel(context.Background())
	s := &wsSource{rawURL: rawURL, conn: conn, cancel: cancel, client: client}
	go s.read(readCtx)
	return s, nil
}

// read drains the connection into the bounded queue until it fails or is closed.
func (s *wsSource) read(ctx context.Context) {
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			s.mu.Lock()
			if s.closedReason == "" {
				if ctx.Err() != nil {
					s.closedReason = "monitor stopped"
				} else {
					s.closedReason = fmt.Sprintf("websocket %s closed: %v", s.rawURL, err)
				}
			}
			s.mu.Unlock()
			return
		}
		// Binary frames carry no text a regex can meaningfully match; note the
		// arrival without pretending the bytes are a string.
		payload := string(data)
		if typ == websocket.MessageBinary {
			payload = fmt.Sprintf("[binary message, %d bytes]", len(data))
		}
		if len(payload) > monitorPayloadBytes {
			payload = payload[:monitorPayloadBytes] + "…[truncated]"
		}

		s.mu.Lock()
		if len(s.queue) >= wsSourceQueueCap {
			s.dropped++
		} else {
			s.queue = append(s.queue, MonitorEvent{At: time.Now(), Payload: payload})
		}
		s.mu.Unlock()
	}
}

// Poll hands over the messages collected since the previous call.
func (s *wsSource) Poll(_ context.Context) ([]MonitorEvent, bool, string, error) {
	s.mu.Lock()
	events := s.queue
	s.queue = nil
	dropped := s.dropped
	s.dropped = 0
	reason := s.closedReason
	alreadyReported := s.reported
	if reason != "" {
		s.reported = true
	}
	s.mu.Unlock()

	if dropped > 0 {
		events = append(events, MonitorEvent{
			At:      time.Now(),
			Payload: fmt.Sprintf("[%d message(s) dropped: the monitor queue holds at most %d between polls]", dropped, wsSourceQueueCap),
		})
	}
	if reason != "" && !alreadyReported {
		// Hand over the final messages together with the terminal condition, so a
		// match on the last message is not lost to the close.
		return events, true, reason, nil
	}
	return events, false, "", nil
}

// Describe identifies the source in the monitor list.
func (s *wsSource) Describe() string { return "ws " + s.rawURL }

// Close stops the reader and closes the connection. Idempotent.
func (s *wsSource) Close() {
	s.mu.Lock()
	if s.closedReason == "" {
		s.closedReason = "monitor stopped"
	}
	s.mu.Unlock()
	s.cancel()
	// StatusNormalClosure is best-effort: the peer may already be gone.
	_ = s.conn.Close(websocket.StatusNormalClosure, "monitor stopped")
	s.client.CloseIdleConnections()
}
