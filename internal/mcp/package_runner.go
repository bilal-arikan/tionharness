package mcp

import (
	"context"
	"errors"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type packageStartLock struct {
	mu   sync.Mutex
	refs int
}

var packageStarts = struct {
	sync.Mutex
	locks map[string]*packageStartLock
}{locks: make(map[string]*packageStartLock)}

type stdioDialFunc func(context.Context, string, []string, []string, string) (Client, error)

// dialPackageRunner serializes the cache-mutating startup window of bunx, npx,
// and `npm exec` by package identity. The lock ends after initialize; the live
// MCP process is not held under it.
func dialPackageRunner(ctx context.Context, command string, args, env []string, dir string, dial stdioDialFunc) (Client, error) {
	pkg, ok := packageRunnerIdentity(command, args)
	if !ok {
		return dial(ctx, command, args, env, dir)
	}
	unlock, err := lockPackageStart(ctx, pkg)
	if err != nil {
		return nil, err
	}
	defer unlock()

	client, firstErr := dial(ctx, command, args, env, dir)
	if firstErr == nil || errors.Is(firstErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return client, firstErr
	}
	if isTransientPackageRunnerError(firstErr) {
		if err := waitPackageRunnerBackoff(ctx); err != nil {
			return nil, err
		}
	}
	return dial(ctx, command, args, env, dir)
}

func lockPackageStart(ctx context.Context, key string) (func(), error) {
	packageStarts.Lock()
	l := packageStarts.locks[key]
	if l == nil {
		l = &packageStartLock{}
		packageStarts.locks[key] = l
	}
	l.refs++
	packageStarts.Unlock()

	locked := make(chan struct{})
	go func() {
		l.mu.Lock()
		close(locked)
	}()
	select {
	case <-locked:
		return func() {
			l.mu.Unlock()
			packageStarts.Lock()
			l.refs--
			if l.refs == 0 {
				delete(packageStarts.locks, key)
			}
			packageStarts.Unlock()
		}, nil
	case <-ctx.Done():
		go func() {
			<-locked
			l.mu.Unlock()
			packageStarts.Lock()
			l.refs--
			if l.refs == 0 {
				delete(packageStarts.locks, key)
			}
			packageStarts.Unlock()
		}()
		return nil, ctx.Err()
	}
}

func packageRunnerIdentity(command string, args []string) (string, bool) {
	name := strings.ToLower(filepath.Base(command))
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".exe"), ".cmd")
	start := 0
	switch name {
	case "bunx", "npx":
	case "npm":
		if len(args) == 0 || strings.ToLower(args[0]) != "exec" {
			return "", false
		}
		start = 1
	default:
		return "", false
	}
	for i := start; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "--" {
			continue
		}
		if a == "-p" || a == "--package" {
			if i+1 < len(args) {
				pkg := normalizePackageIdentity(args[i+1])
				return pkg, pkg != ""
			}
			return "", false
		}
		if strings.HasPrefix(a, "--package=") {
			pkg := normalizePackageIdentity(strings.TrimPrefix(a, "--package="))
			return pkg, pkg != ""
		}
		if packageRunnerFlagTakesValue(a) {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		pkg := normalizePackageIdentity(a)
		return pkg, pkg != ""
	}
	return "", false
}

func packageRunnerFlagTakesValue(flag string) bool {
	switch flag {
	case "--cache", "--call", "-c", "--registry", "--userconfig", "--script-shell", "--workspace", "-w":
		return true
	default:
		return false
	}
}

func normalizePackageIdentity(spec string) string {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if strings.HasPrefix(spec, "@") {
		if slash := strings.IndexByte(spec, '/'); slash >= 0 {
			if version := strings.IndexByte(spec[slash+1:], '@'); version >= 0 {
				return spec[:slash+1+version]
			}
		}
		return spec
	}
	if version := strings.IndexByte(spec, '@'); version >= 0 {
		return spec[:version]
	}
	return spec
}

func isTransientPackageRunnerError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ebusy") ||
		strings.Contains(msg, "failed copying files from cache") ||
		strings.Contains(msg, "could not determine executable to run")
}

func waitPackageRunnerBackoff(ctx context.Context) error {
	d := 40*time.Millisecond + time.Duration(rand.Intn(41))*time.Millisecond
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
