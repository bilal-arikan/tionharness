package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wsTestServer runs a WebSocket endpoint that pushes whatever is sent on the
// returned channel, and returns its ws:// URL. Closing the channel closes the
// connection, which is how the terminal path is exercised.
func wsTestServer(t *testing.T) (string, chan<- string) {
	t.Helper()
	msgs := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		for m := range msgs {
			if err := c.Write(r.Context(), websocket.MessageText, []byte(m)); err != nil {
				return
			}
		}
		c.Close(websocket.StatusNormalClosure, "done")
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), msgs
}

// dialLoopbackWS connects without the egress guard, which refuses loopback in
// production. TestWSSourceRejectsLoopbackThroughEgressGuard covers the guard, so
// bypassing it here cannot hide a missing boundary.
func dialLoopbackWS(t *testing.T, rawURL string) *wsSource {
	t.Helper()
	ctx, cancelDial := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDial()
	conn, _, err := websocket.Dial(ctx, rawURL, &websocket.DialOptions{HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.SetReadLimit(wsSourceReadLimit)
	readCtx, cancel := context.WithCancel(context.Background())
	s := &wsSource{rawURL: rawURL, conn: conn, cancel: cancel, client: &http.Client{}}
	go s.read(readCtx)
	t.Cleanup(s.Close)
	return s
}

// pollUntil polls s until it returns events or a terminal state, or the deadline
// passes. The reader is a goroutine, so a test must not assume the first poll
// after a send already sees the message.
func pollUntil(t *testing.T, s *wsSource) ([]MonitorEvent, bool, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		evs, done, reason, err := s.Poll(context.Background())
		if err != nil {
			t.Fatalf("Poll: %v", err)
		}
		if len(evs) > 0 || done {
			return evs, done, reason
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, false, ""
}

// TestWSSourceReportsIncomingMessages is the core contract: each text message
// becomes one event a regex can match.
func TestWSSourceReportsIncomingMessages(t *testing.T) {
	url, msgs := wsTestServer(t)
	src := dialLoopbackWS(t, url)

	msgs <- `{"type":"deploy","status":"green"}`
	evs, done, _ := pollUntil(t, src)
	if done {
		t.Fatal("the monitor ended before delivering the message")
	}
	if len(evs) != 1 || !strings.Contains(evs[0].Payload, `"type":"deploy"`) {
		t.Fatalf("message not delivered: %+v", evs)
	}
}

// TestWSSourceReportsCloseOnce pins the terminal contract: a closed socket ends
// the monitor, and the reason is produced exactly once.
func TestWSSourceReportsCloseOnce(t *testing.T) {
	url, msgs := wsTestServer(t)
	src := dialLoopbackWS(t, url)

	close(msgs) // server closes the connection
	_, done, reason := pollUntil(t, src)
	if !done {
		t.Fatal("a closed socket must end the monitor")
	}
	if reason == "" {
		t.Fatal("the terminal state carried no reason")
	}

	// A second poll must NOT report the terminal condition again.
	_, done2, _, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if done2 {
		t.Fatal("the terminal condition was reported twice")
	}
}

// TestWSSourceDeliversFinalMessagesWithClose: a match on the LAST message before
// the socket closed must not be lost to the terminal state.
func TestWSSourceDeliversFinalMessagesWithClose(t *testing.T) {
	url, msgs := wsTestServer(t)
	src := dialLoopbackWS(t, url)

	msgs <- "last words"
	close(msgs)

	// Give the reader time to see both the message and the close.
	deadline := time.Now().Add(3 * time.Second)
	var got []MonitorEvent
	var sawDone bool
	for time.Now().Before(deadline) && !sawDone {
		evs, done, _, err := src.Poll(context.Background())
		if err != nil {
			t.Fatalf("Poll: %v", err)
		}
		got = append(got, evs...)
		sawDone = done
		time.Sleep(10 * time.Millisecond)
	}
	if !sawDone {
		t.Fatal("the close was never reported")
	}
	var found bool
	for _, e := range got {
		if strings.Contains(e.Payload, "last words") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the final message was lost to the close: %+v", got)
	}
}

// TestWSSourceRejectsBadURL keeps the source honest about what it can dial.
func TestWSSourceRejectsBadURL(t *testing.T) {
	for _, bad := range []string{"", "https://example.com", "ftp://example.com", "ws://"} {
		if _, err := NewWSSource(context.Background(), bad); err == nil {
			t.Fatalf("url %q was accepted", bad)
		}
	}
}

// TestWSSourceRejectsLoopbackThroughEgressGuard proves the guard the production
// dialer carries: a monitor must not become an SSRF probe into the host.
func TestWSSourceRejectsLoopbackThroughEgressGuard(t *testing.T) {
	url, _ := wsTestServer(t)
	if _, err := NewWSSource(context.Background(), url); err == nil {
		t.Fatal("the egress guard allowed a loopback WebSocket handshake")
	}
}
