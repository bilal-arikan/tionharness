package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type runnerTestClient struct{}

func (*runnerTestClient) ListTools(context.Context) ([]Tool, error) { return nil, nil }
func (*runnerTestClient) CallTool(context.Context, string, json.RawMessage) (CallToolResult, error) {
	return CallToolResult{}, nil
}
func (*runnerTestClient) Close() error                   { return nil }
func (*runnerTestClient) Alive() bool                    { return true }
func (*runnerTestClient) SetOnToolsChanged(func())       {}
func (*runnerTestClient) SetLogger(*slog.Logger, string) {}

func TestCatalogSerializesSamePackageAcrossScopes(t *testing.T) {
	p := NewPool()
	defer p.Close()
	var active atomic.Int32
	var max atomic.Int32
	dial := func(context.Context, string, []string, []string, string) (Client, error) {
		n := active.Add(1)
		for old := max.Load(); n > old && !max.CompareAndSwap(old, n); old = max.Load() {
		}
		time.Sleep(40 * time.Millisecond)
		active.Add(-1)
		return &runnerTestClient{}, nil
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, scope := range []string{"session-a|agent", "session-b|agent"} {
		wg.Add(1)
		go func(scope string) {
			defer wg.Done()
			<-start
			cfg := ServerConfig{Name: "playwright-" + scope, Command: "npx", Args: []string{"@Playwright/MCP@latest"}, ScopeKey: scope, stdioDial: dial}
			if _, _, errs := p.Catalog(context.Background(), []ServerConfig{cfg}); len(errs) != 0 {
				t.Errorf("Catalog errors: %v", errs)
			}
		}(scope)
	}
	close(start)
	wg.Wait()
	if got := max.Load(); got != 1 {
		t.Fatalf("max launcher concurrency = %d, want 1", got)
	}
}

func TestCatalogAllowsDifferentPackagesInParallel(t *testing.T) {
	p := NewPool()
	defer p.Close()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	dial := func(context.Context, string, []string, []string, string) (Client, error) {
		entered <- struct{}{}
		<-release
		return &runnerTestClient{}, nil
	}
	var wg sync.WaitGroup
	for _, pkg := range []string{"package-a", "package-b"} {
		wg.Add(1)
		go func(pkg string) {
			defer wg.Done()
			cfg := ServerConfig{Name: pkg, Command: "bunx", Args: []string{pkg}, stdioDial: dial}
			p.Catalog(context.Background(), []ServerConfig{cfg})
		}(pkg)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("different package launchers did not overlap")
		}
	}
	close(release)
	wg.Wait()
}

func TestPackageRunnerRetryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		firstErr error
		cancel   bool
		wantCall int
		wantOK   bool
	}{
		{name: "transient succeeds second attempt", firstErr: errors.New("EBUSY: failed copying files from cache"), wantCall: 2, wantOK: true},
		{name: "permanent error gets one retry", firstErr: errors.New("permanent startup failure"), wantCall: 2},
		{name: "canceled is not retried", firstErr: context.Canceled, cancel: true, wantCall: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			dial := func(context.Context, string, []string, []string, string) (Client, error) {
				calls++
				if calls == 1 {
					if tt.cancel {
						cancel()
					}
					return nil, tt.firstErr
				}
				if tt.wantOK {
					return &runnerTestClient{}, nil
				}
				return nil, tt.firstErr
			}
			_, err := dialPackageRunner(ctx, "npm", []string{"exec", "--", "pkg@1.2.3"}, nil, "", dial)
			if calls != tt.wantCall {
				t.Fatalf("dial calls = %d, want %d", calls, tt.wantCall)
			}
			if tt.wantOK && err != nil {
				t.Fatalf("second attempt error: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("expected final error")
			}
		})
	}
}

func TestPackageRunnerIdentity(t *testing.T) {
	for _, tc := range []struct {
		command string
		args    []string
		want    string
	}{
		{"npx.cmd", []string{"--yes", "@Playwright/MCP@1.2.3"}, "@playwright/mcp"},
		{"bunx.exe", []string{"foo@latest"}, "foo"},
		{"npm", []string{"exec", "--package=@Scope/Tool@next", "--", "tool"}, "@scope/tool"},
	} {
		got, ok := packageRunnerIdentity(tc.command, tc.args)
		if !ok || got != tc.want {
			t.Errorf("packageRunnerIdentity(%q, %v) = %q, %v; want %q, true", tc.command, tc.args, got, ok, tc.want)
		}
	}
}
