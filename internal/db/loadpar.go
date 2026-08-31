package db

import (
	"runtime"
	"sync"
)

// Concurrent boot loading.
//
// Opening a store is LATENCY-bound, not CPU-bound. On Windows the first open of
// a file goes through the antivirus filter driver, and that dominates everything
// else: reading this machine's WS1 store (720 files, 3.9 MB) took 15.02 ms PER
// FILE cold versus 0.14 ms warm — a 107x gap that has nothing to do with parsing.
// A serial loader therefore spends the whole boot waiting on syscalls with the
// CPU idle.
//
// Issuing many opens at once hides that latency almost linearly: the same
// measurement on a 1315-file store went from 15.9 s serial to 1.9 s with 16
// workers (8.5x). See _Docs/16-PROFILLEME.md, "Vaka: 59 sn'lik boot".
//
// The pool is capped instead of unbounded: thousands of simultaneous opens make
// the filter driver thrash and give the win back.

// maxLoadWorkers caps boot read concurrency. Deliberately above the "one per
// core" instinct — the workers are blocked in the kernel, not computing — but
// low enough not to swamp the filter driver.
const maxLoadWorkers = 16

// loadWorkers picks the pool size for n items.
func loadWorkers(n int) int {
	if n <= 1 {
		return 1
	}
	w := runtime.NumCPU() * 2
	if w > maxLoadWorkers {
		w = maxLoadWorkers
	}
	if w > n {
		w = n
	}
	if w < 1 {
		w = 1
	}
	return w
}

// parallelLoad applies fn to every item with a bounded worker pool and returns
// the results IN INPUT ORDER, plus the FIRST error by input order.
//
// Input-order results and input-order error selection are both load-bearing:
// which item gets blamed — and whether the caller fails at all — must not depend
// on goroutine scheduling. A scheduling-dependent boot failure is the kind of bug
// that reproduces once a month and never in a test. (loadJSONDir no longer routes
// per-file corruption through this error path — it skips and logs — but callers
// that DO treat fn's error as fatal still get a deterministic one.)
//
// fn must be safe to call concurrently. Every current caller only reads files
// and fills a value it owns; nothing touches the store's maps, which are
// populated serially by the caller after this returns.
func parallelLoad[In, Out any](items []In, fn func(In) (Out, error)) ([]Out, error) {
	out := make([]Out, len(items))
	errs := make([]error, len(items))

	if workers := loadWorkers(len(items)); workers == 1 {
		for i := range items {
			out[i], errs[i] = fn(items[i])
		}
	} else {
		idx := make(chan int)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Each goroutine writes only the slots it pulls off idx, so the
				// shared slices need no lock.
				for i := range idx {
					out[i], errs[i] = fn(items[i])
				}
			}()
		}
		for i := range items {
			idx <- i
		}
		close(idx)
		wg.Wait()
	}

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
