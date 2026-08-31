package api

import "time"

// chatTurnIdle is the inactivity window that reclaims an interactive chat turn
// whose provider stream went silent (chat_stream.go). It normally reads the
// user-facing tunable, which is minute-granular — the right resolution for a
// setting, far too coarse for a test that has to watch the cut actually happen.
// chatTurnIdleOverride is that sub-minute escape hatch and is set by tests only;
// a zero override leaves the configured behaviour byte-identical.
func (s *Server) chatTurnIdle() time.Duration {
	if s.chatTurnIdleOverride > 0 {
		return s.chatTurnIdleOverride
	}
	return s.tun.ChatTurnIdleTimeout()
}
