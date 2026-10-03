package providers

import "time"

// Keep watchdog time injection inside the package's test binary.
func setCLIStartupTimeout(d time.Duration) {
	cliStartupMu.Lock()
	defer cliStartupMu.Unlock()
	if d <= 0 {
		d = cliStartupTimeoutDefault
	}
	cliStartupTimeoutDuration = d
}

func setCLISessionIdleTimeout(d time.Duration) {
	cliSessionIdleMu.Lock()
	defer cliSessionIdleMu.Unlock()
	cliSessionIdleTimeoutDuration = d
}
