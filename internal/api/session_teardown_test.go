package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// testWS is the minimal workspace a teardown needs: an id (everything per-session
// is keyed by it) and no runtime/DB, so the phases that touch those are skipped.
func testWS(id string) *workspace.Workspace {
	return &workspace.Workspace{Meta: workspace.Meta{ID: id}}
}

// TestTeardown_NoLiveRun_RemovesInbox: with no in-flight turn, a delete's teardown
// drops the in-memory inbox entry so a serial worker can never re-dispatch a turn
// for a session that no longer exists.
func TestTeardown_NoLiveRun_RemovesInbox(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}}

	if err := s.teardownSessionRuntime(testWS("WS1"), "SES"); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	s.inbox.lock()
	_, ok := s.inbox.sessions[scopeKey("WS1", "SES")]
	s.inbox.unlock()
	if ok {
		t.Fatal("inbox entry must be removed after a successful teardown")
	}
}

// TestTeardown_OtherWorkspaceSameID_Untouched: session ids repeat across workspace
// stores, so deleting WS1/SES must leave WS2's own SES — its queue, and any turn
// running in it — completely alone.
func TestTeardown_OtherWorkspaceSameID_Untouched(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}}
	s.inbox.sessions[scopeKey("WS2", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "b"}}}
	// A turn running in WS2's same-numbered session must not be seen (and cancelled)
	// by WS1's teardown — with a bare-id lookup it blocked the delete instead.
	s.runs.register("R1", "SES", "WS2", func() {})

	if err := s.teardownSessionRuntime(testWS("WS1"), "SES"); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	s.inbox.lock()
	_, gone := s.inbox.sessions[scopeKey("WS1", "SES")]
	other, kept := s.inbox.sessions[scopeKey("WS2", "SES")]
	s.inbox.unlock()
	if gone {
		t.Fatal("WS1's inbox entry must be removed")
	}
	if !kept || len(other.items) != 1 {
		t.Fatal("WS2's same-id inbox must survive another workspace's teardown")
	}
	if _, live := s.runs.sessionRunInfo("WS2", "SES"); !live {
		t.Fatal("WS2's run must still be registered")
	}
}

// TestTeardown_RequiresWorkspace: without a workspace the session cannot be
// identified (ids repeat across stores), so teardown refuses instead of tearing
// down whatever session happens to carry that id.
func TestTeardown_RequiresWorkspace(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	if err := s.teardownSessionRuntime(nil, "SES"); err == nil {
		t.Fatal("want an error when no workspace is given")
	}
}

// TestStopInflightTurn_Stops: a turn whose goroutine unwinds (run.done closes on
// unregister) is reported stopped — the happy path where cancel actually kills it.
func TestStopInflightTurn_Stops(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	s.runs.register("R1", "SES", "WS", func() {})
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.runs.unregister("R1") // simulates the turn goroutine returning after cancel
	}()
	if err := s.stopInflightTurn("WS", "SES", 2*time.Second); err != nil {
		t.Fatalf("want stopped, got error: %v", err)
	}
}

// TestStopInflightTurn_Timeout: a turn that ignores cancellation (never unregisters)
// is reported un-stoppable, which is exactly what makes the caller ABORT the delete
// instead of stranding the process.
func TestStopInflightTurn_Timeout(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	s.runs.register("R1", "SES", "WS", func() {})
	if err := s.stopInflightTurn("WS", "SES", 50*time.Millisecond); err == nil {
		t.Fatal("want a timeout error for a turn that never stops")
	}
}

// TestKickInbox_FrozenDoesNotDispatch: while a teardown holds the closing freeze, the
// serial worker must not start — otherwise it would spawn a fresh turn (and subprocess)
// for a session mid-delete.
func TestKickInbox_FrozenDoesNotDispatch(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}, closing: true}

	s.kickInbox("WS1", "SES")
	s.inbox.lock()
	running := s.inbox.sessions[scopeKey("WS1", "SES")].running
	s.inbox.unlock()
	if running {
		t.Fatal("a frozen (closing) inbox must not start a worker")
	}
}

// TestResumeInboxAfterAbortedTeardown_Unfreezes: when a delete aborts, the freeze is
// lifted so the preserved queue can drain again (session lives on).
func TestResumeInboxAfterAbortedTeardown_Unfreezes(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, closing: true} // no items → kick is a no-op

	s.resumeInboxAfterAbortedTeardown("WS1", "SES")
	s.inbox.lock()
	closing := s.inbox.sessions[scopeKey("WS1", "SES")].closing
	s.inbox.unlock()
	if closing {
		t.Fatal("resume must clear the closing freeze")
	}
}

func blockingBridgeRun(t *testing.T, s *Server, ignoreCancellation bool, lateWrite string) (*chatRun, *interactionBackend, <-chan struct{}, chan<- struct{}) {
	t.Helper()
	run := s.runs.register("R1", "SES", "WS1", func() {})
	run.cancel = func() { s.runs.unregister("R1") }
	started := make(chan struct{})
	release := make(chan struct{})
	run.setBridge([]providers.ToolDef{{Name: "block"}}, func(ctx context.Context, _ string, _ json.RawMessage) (string, error) {
		close(started)
		<-ctx.Done()
		if ignoreCancellation {
			<-release
		}
		if lateWrite != "" {
			time.Sleep(20 * time.Millisecond)
			if err := os.WriteFile(lateWrite, []byte("late"), 0o600); err != nil {
				return "", err
			}
		}
		return "stopped", nil
	})
	return run, &interactionBackend{
		runs:      s.runs,
		activated: map[string]map[string]bool{run.token: {"block": true}},
		hydrated:  map[string]bool{run.token: true},
	}, started, release
}

func TestTeardown_DrainsBridgeCallBeforeSuccess(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	run, backend, started, _ := blockingBridgeRun(t, s, false, "")
	callDone := make(chan error, 1)
	go func() {
		_, err := backend.Call(context.Background(), run.token, "block", nil)
		callDone <- err
	}()
	<-started

	if err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	select {
	case err := <-callDone:
		if err != nil {
			t.Fatalf("bridge call: %v", err)
		}
	default:
		t.Fatal("teardown returned before bridge call ended")
	}
}

func TestTeardown_RejectsNewBridgeCallAfterGateCloses(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("R1", "SES", "WS1", func() {})
	runs.closeSessionCalls("WS1", "SES")
	backend := &interactionBackend{runs: runs}
	if _, err := backend.Call(context.Background(), run.token, "block", nil); err == nil || !strings.Contains(err.Error(), "session closing") {
		t.Fatalf("want session closing error, got %v", err)
	}
}

func TestTeardown_UncooperativeBridgeCallFailsClosedAndReopens(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}}
	run, backend, started, release := blockingBridgeRun(t, s, true, "")
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		_, _ = backend.Call(context.Background(), run.token, "block", nil)
	}()
	<-started

	if err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", 40*time.Millisecond); err == nil {
		t.Fatal("want fail-closed timeout")
	}
	run.callMu.Lock()
	closing := run.closing
	run.callMu.Unlock()
	if closing {
		t.Fatal("aborted teardown must reopen lifecycle gate")
	}
	s.inbox.lock()
	ib := s.inbox.at("WS1", "SES")
	s.inbox.unlock()
	if ib == nil || ib.closing {
		t.Fatal("aborted teardown must preserve and reopen inbox")
	}
	close(release)
	<-callDone
}

func TestTeardown_WaitsForLateWriteBeforeDirectoryRemoval(t *testing.T) {
	dir := t.TempDir()
	latePath := filepath.Join(dir, "session.json")
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	run, backend, started, _ := blockingBridgeRun(t, s, false, latePath)
	go backend.Call(context.Background(), run.token, "block", nil)
	<-started

	if err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove session dir: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("session directory reappeared after delete: %v", err)
	}
}

func TestTeardown_SerializesSameSession(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	s.stopWorker = func(context.Context, *workspace.Workspace, string) error {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			close(firstEntered)
			<-releaseFirst
			return errors.New("first teardown failed")
		}
		close(secondEntered)
		return nil
	}

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second) }()
	<-firstEntered
	go func() { secondDone <- s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second) }()
	select {
	case <-secondEntered:
		t.Fatal("second teardown entered before first released session lock")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err == nil {
		t.Fatal("first teardown must fail")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second teardown: %v", err)
	}
}

func TestDeleteSession_SerializesThroughDurableDelete(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	session, err := wsp.DB.CreateSession(context.Background(), db.Session{Title: "delete me"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	deleteEntered := make(chan struct{})
	releaseDelete := make(chan struct{})
	var once sync.Once
	s.deleteSession = func(ctx context.Context, gotWSP *workspace.Workspace, id string) error {
		once.Do(func() {
			close(deleteEntered)
			<-releaseDelete
		})
		return gotWSP.DB.DeleteSession(ctx, id)
	}

	callDelete := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+session.ID, nil)
		req.SetPathValue("id", session.ID)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		s.handleDeleteSession(rec, req)
		return rec
	}
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- callDelete() }()
	<-deleteEntered
	go func() { secondDone <- callDelete() }()
	select {
	case <-secondDone:
		t.Fatal("second HTTP delete passed session lock while first DB delete was blocked")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseDelete)
	first := <-firstDone
	second := <-secondDone
	if first.Code != http.StatusOK {
		t.Fatalf("first delete status %d: %s", first.Code, first.Body.String())
	}
	// The second request enters only after durable deletion. Runtime teardown then
	// fails closed because the worker-owned session no longer exists; importantly it
	// never overlapped or reopened the first request's lifecycle state.
	if second.Code != http.StatusConflict {
		t.Fatalf("second delete status %d: %s", second.Code, second.Body.String())
	}
}

func TestDeleteSession_DBErrorRollsBackEntireRuntimePreparation(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	session, err := wsp.DB.CreateSession(ctx, db.Session{Title: "survives"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	key := scopeKey(wsp.ID, session.ID)
	s.inbox.sessions[key] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "queued"}}}
	run := s.runs.register("delete-rollback", session.ID, wsp.ID, func() {})
	run.cancel = func() { s.runs.unregister("delete-rollback") }
	_, hubCh, _ := s.hub.Subscribe(wsp.ID, session.ID)
	s.deleteSession = func(context.Context, *workspace.Workspace, string) error {
		return errors.New("forced DB delete failure")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+session.ID, nil)
	req.SetPathValue("id", session.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleDeleteSession(rec, req)
	if rec.Code < 400 {
		t.Fatalf("delete status %d, want error", rec.Code)
	}
	if _, err := wsp.DB.GetSession(ctx, session.ID); err != nil {
		t.Fatalf("session must survive DB delete error: %v", err)
	}
	s.inbox.lock()
	ib := s.inbox.sessions[key]
	s.inbox.unlock()
	if ib == nil || ib.closing || len(ib.items) != 1 {
		t.Fatalf("inbox rollback incomplete: %+v", ib)
	}
	run.callMu.Lock()
	closing := run.closing
	run.callMu.Unlock()
	if closing {
		t.Fatal("bridge call gate must reopen after DB delete error")
	}
	s.hub.Publish(wsp.ID, session.ID, "probe", nil, false)
	select {
	case _, ok := <-hubCh:
		if !ok {
			t.Fatal("hub subscriber was closed despite failed DB delete")
		}
	case <-time.After(time.Second):
		t.Fatal("hub state was not preserved after failed DB delete")
	}
}

func TestTeardown_StopWorkerTimeoutFailsClosed(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}}
	s.stopWorker = func(ctx context.Context, _ *workspace.Workspace, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	if err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", 30*time.Millisecond); err == nil {
		t.Fatal("worker timeout must fail closed")
	}
	s.inbox.lock()
	ib := s.inbox.at("WS1", "SES")
	s.inbox.unlock()
	if ib == nil || ib.closing {
		t.Fatal("worker timeout must preserve and reopen inbox")
	}
}

func TestTeardown_StopWorkerErrorFailsClosed(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}}
	s.stopWorker = func(context.Context, *workspace.Workspace, string) error {
		return errors.New("stop failed")
	}

	err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second)
	if err == nil || !strings.Contains(err.Error(), "stop failed") {
		t.Fatalf("want worker stop error, got %v", err)
	}
	s.inbox.lock()
	ib := s.inbox.at("WS1", "SES")
	s.inbox.unlock()
	if ib == nil || ib.closing {
		t.Fatal("worker error must preserve and reopen inbox")
	}
}

func TestTeardown_WaitsForWorkerLateWrite(t *testing.T) {
	dir := t.TempDir()
	latePath := filepath.Join(dir, "session.json")
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.stopWorker = func(context.Context, *workspace.Workspace, string) error {
		time.Sleep(20 * time.Millisecond)
		return os.WriteFile(latePath, []byte("late"), 0o600)
	}

	if err := s.teardownSessionRuntimeWithGrace(testWS("WS1"), "SES", time.Second); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if _, err := os.Stat(latePath); err != nil {
		t.Fatalf("worker write must finish before teardown: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove session dir: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("session directory reappeared after worker teardown: %v", err)
	}
}

// TestUnregister_ReleasesTeardownContext: the per-run teardown context is rooted in
// context.Background(), so an ordinary turn that ends without ever being deleted
// must release it. Otherwise every turn in the process leaks a live cancel func.
func TestUnregister_ReleasesTeardownContext(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("R1", "SES", "WS1", func() {})
	ctx := run.teardownCtx
	if ctx.Err() != nil {
		t.Fatal("teardown context must be live while the run is registered")
	}

	runs.unregister("R1")

	if ctx.Err() == nil {
		t.Fatal("unregister must cancel the teardown context of a run with no admitted call")
	}
	run.callMu.Lock()
	released := run.teardownCancel == nil
	run.callMu.Unlock()
	if !released {
		t.Fatal("unregister must drop the teardown cancel func so it cannot be retained")
	}
}

// TestUnregister_DrainingRunReleasesTeardownContextOnLastCall: a run unregistered
// while a bridge call is still admitted keeps its teardown context alive (the
// delete path may still need to cancel that call), and releases it only when the
// last call ends.
func TestUnregister_DrainingRunReleasesTeardownContextOnLastCall(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("R1", "SES", "WS1", func() {})
	_, _, end, err := runs.beginCall(context.Background(), run.token)
	if err != nil {
		t.Fatalf("beginCall: %v", err)
	}
	ctx := run.teardownCtx

	runs.unregister("R1")
	if ctx.Err() != nil {
		t.Fatal("teardown context must stay live while a call is still admitted")
	}

	end()
	if ctx.Err() == nil {
		t.Fatal("the last draining call ending must release the teardown context")
	}
}

// TestCancelSessionCalls_AfterRelease: deleting a session whose run already
// finished and released its teardown context must not panic on the nil cancel.
func TestCancelSessionCalls_AfterRelease(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("R1", "SES", "WS1", func() {})
	runs.unregister("R1")

	closed := runs.closeSessionCalls("WS1", "SES")
	if waits := runs.cancelSessionCalls(append(closed, run)); len(waits) != 0 {
		t.Fatalf("a released run has no call to drain, got %d waits", len(waits))
	}
	runs.reopenSessionCalls([]*chatRun{run})
}
