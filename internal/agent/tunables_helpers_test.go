package agent

import "time"

// SetCoordinatorWorkerBatchWindow lets batching tests use deterministic windows.
func (t *Tunables) SetCoordinatorWorkerBatchWindow(window time.Duration) {
	t.mu.Lock()
	t.coordWorkerBatch = window
	t.mu.Unlock()
}

// SetCoordinatorStallHaltTotal isolates the cumulative halt threshold in fixtures.
func (t *Tunables) SetCoordinatorStallHaltTotal(n int) {
	t.mu.Lock()
	t.coordStallHaltTotal = n
	t.mu.Unlock()
}
