package api

import "testing"

// TestRoutes_RegistersWithoutPanic guards the per-domain route registration:
// net/http's ServeMux panics on a duplicate or malformed pattern, so a clean
// Routes() build proves every register*Routes helper is wired and conflict-free.
func TestRoutes_RegistersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("route registration panicked: %v", r)
		}
	}()
	if h := (&Server{}).Routes(); h == nil {
		t.Fatal("Routes() returned nil handler")
	}
}
