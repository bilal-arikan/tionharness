package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux
	"os"
	"runtime"
	"time"
)

// Lock-contention sampling rates. Without these, /debug/pprof/mutex and
// /debug/pprof/block return an EMPTY profile — the two endpoints exist but
// record nothing until the runtime is told to sample. They carry a small
// always-on cost, so they ride the same TIONSWARM_PPROF gate as the endpoints
// themselves and stay off in production builds.
const (
	// One in five contended mutex events is recorded: enough signal to rank the
	// hot locks, cheap enough to leave on for a multi-minute profiling session.
	mutexProfileFraction = 5
	// Sample blocking events longer than ~1ms. Finer rates flood the profile with
	// channel/select noise from the SSE and event-bus goroutines.
	blockProfileRateNanos = 1_000_000
)

// startPprof launches a loopback-only profiling server when TIONSWARM_PPROF is
// truthy. It is OFF by default: production binaries expose no profiling surface.
//
// The endpoints (/debug/pprof/...) are served on http.DefaultServeMux, which the
// main API server does not use (it has its own mux via server.Routes()), so the
// profiler stays fully isolated from application routes.
//
// Override the bind address with TIONSWARM_PPROF_ADDR (default 127.0.0.1:6060).
// Binding to loopback only keeps the profiler off the network and away from the
// Windows Firewall inbound prompt.
func startPprof(logger *slog.Logger) {
	v := os.Getenv("TIONSWARM_PPROF")
	if v != "1" && v != "true" && v != "TRUE" {
		return
	}

	addr := os.Getenv("TIONSWARM_PPROF_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6060"
	}

	// Enable lock-contention sampling, without which the mutex/block endpoints
	// are served but always empty — the profiles that answer "is a poll queueing
	// behind the store write lock?".
	runtime.SetMutexProfileFraction(mutexProfileFraction)
	runtime.SetBlockProfileRate(blockProfileRateNanos)

	srv := &http.Server{
		Addr:              addr,
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Warn("pprof profiling server enabled", "addr", "http://"+addr+"/debug/pprof/")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("pprof server failed", "error", err)
		}
	}()
}
