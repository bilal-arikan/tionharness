package providers

import "time"

// Watchdog kill reasons. They are distinct because they mean opposite things to
// the caller: a startup hang ran nothing and is safe to retry, an idle hang may
// have run tools already and is not.
const (
	WatchdogReasonStartup = "startup" // no output at all within the startup window
	WatchdogReasonIdle    = "idle"    // stream went silent after producing output
)

// WatchdogKill describes a CLI subprocess that one of the provider watchdogs tore
// down. It is a diagnostic record, not an error: the turn error is returned
// separately by the provider. Fields are what a human (or an agent reading its own
// debug journal) needs to tell a wedged subprocess apart from a slow one.
type WatchdogKill struct {
	Provider string        // provider kind that owned the subprocess, e.g. "codex-cli"
	Model    string        // model the killed turn was running
	Reason   string        // WatchdogReason* above
	Window   time.Duration // the silence window that expired
	Detail   string        // stdout tail captured before the kill (may be empty)
}

// reportWatchdogKill hands the record to the caller's sink, if any. Providers sit
// below the persistence layer and must not import it, so the observability sink is
// supplied per request — same shape as Request.OnEvent. A nil sink is a no-op:
// dropping the record must never change the turn's outcome.
func reportWatchdogKill(req Request, kill WatchdogKill) {
	if req.OnWatchdog == nil {
		return
	}
	req.OnWatchdog(kill)
}
