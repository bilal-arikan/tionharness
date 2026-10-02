package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

func liveTestURL(base string, channels []liveChannel) string {
	data, _ := json.Marshal(channels)
	return base + "/api/live/stream?" + url.Values{"channels": {string(data)}}.Encode()
}

func readLiveFrame(t *testing.T, reader *bufio.Reader) liveFrame {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read live frame: %v", err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame liveFrame
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		return frame
	}
}

func TestLiveStreamMultiplexReplayAndNotifications(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	s.hub.Publish(wsp.ID, sess.ID, sessionhub.KindStep, json.RawMessage(`{"text":"in flight"}`), false)
	s.hub.PublishWorkspace(wsp.ID, sessionhub.KindWSLiveness, json.RawMessage(`{"n":1}`))
	s.hub.PublishWorkspace(wsp.ID, sessionhub.KindWSLiveness, json.RawMessage(`{"n":2}`))
	server := httptest.NewServer(http.HandlerFunc(s.handleLiveStream))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	channels := []liveChannel{
		{Key: "global", Scope: "global"},
		{Key: "workspace", Scope: "workspace", WorkspaceID: wsp.ID, Since: 1, Epoch: s.hub.Epoch()},
		{Key: "session", Scope: "session", WorkspaceID: wsp.ID, SessionID: sess.ID},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, liveTestURL(server.URL, channels), nil)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	seen := map[string]bool{}
	for !seen["global"] || !seen["workspace"] || !seen["session"] {
		frame := readLiveFrame(t, reader)
		switch {
		case frame.Channel == "global" && strings.Contains(frame.Frame, ": connected"):
			seen["global"] = true
		case frame.Channel == "workspace" && strings.Contains(frame.Frame, `"seq":2`):
			seen["workspace"] = true
		case frame.Channel == "session" && strings.Contains(frame.Frame, "in flight"):
			seen["session"] = true
		}
	}
	// Global notifications still cover other workspaces while the local session
	// and workspace feeds retain their independent replay histories.
	s.bus.Publish(events.Event{Type: events.TypeChat, WorkspaceID: "OTHER", Title: "finished"})
	for {
		frame := readLiveFrame(t, reader)
		if frame.Channel == "global" && strings.Contains(frame.Frame, "finished") {
			if !strings.Contains(frame.Frame, "OTHER") || !strings.Contains(frame.Frame, "event: notify") {
				t.Fatalf("notification lost its scope: %+v", frame)
			}
			break
		}
	}
	cancel()
}

func TestLiveStreamThreeTabsLeaveHTTP1CapacityForJSON(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/probe" {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		s.handleLiveStream(w, r)
	}))
	defer server.Close()
	transport := &http.Transport{MaxConnsPerHost: 6}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	channels := []liveChannel{{Key: "g", Scope: "global"}, {Key: "w", Scope: "workspace", WorkspaceID: wsp.ID}, {Key: "s", Scope: "session", WorkspaceID: wsp.ID, SessionID: sess.ID}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 3 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, liveTestURL(server.URL, channels), nil)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("stream status %d", response.StatusCode)
		}
	}
	response, err := client.Get(server.URL + "/api/probe")
	if err != nil {
		t.Fatalf("three tabs starved a normal JSON read: %v", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if string(data) != "{\"ok\":true}\n" {
		t.Fatalf("probe: %s", data)
	}
	cancel()
}

func TestLiveStreamMissingSessionDoesNotDisableOtherFeeds(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	server := httptest.NewServer(http.HandlerFunc(s.handleLiveStream))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(liveTestURL(server.URL, []liveChannel{{Key: "g", Scope: "global"}, {Key: "s", Scope: "session", WorkspaceID: wsp.ID, SessionID: "missing"}}))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	seen := map[string]bool{}
	for !seen["error"] || !seen["global"] {
		frame := readLiveFrame(t, reader)
		if frame.Channel == "s" && strings.Contains(frame.Frame, "session not found") {
			seen["error"] = true
		}
		if frame.Channel == "g" && strings.Contains(frame.Frame, ": connected") {
			seen["global"] = true
		}
	}
}

func TestLiveStreamValidatesSubscriptionsBeforeOpening(t *testing.T) {
	s := newTestServer()
	for _, channels := range [][]liveChannel{nil, {{Key: "a", Scope: "unknown"}}, {{Key: "a", Scope: "global"}, {Key: "a", Scope: "global"}}} {
		r := httptest.NewRequest(http.MethodGet, liveTestURL("http://test", channels), nil)
		w := httptest.NewRecorder()
		s.handleLiveStream(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid subscription status %d", w.Code)
		}
	}
	if !workspaceOptionalPath("/api/live/stream") {
		t.Fatal("first-run live events must work without a workspace")
	}
}
