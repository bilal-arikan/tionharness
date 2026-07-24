package tts

import (
	"context"
	"testing"
)

// TestSynthesizeSmoke verifies end-to-end synthesis when Piper is installed on
// the host. It is a no-op (skip) on machines without Piper so CI stays green.
func TestSynthesizeSmoke(t *testing.T) {
	if !Available() {
		t.Skip("piper not installed on this host; skipping synthesis smoke test")
	}
	voices := Voices()
	if len(voices) == 0 {
		t.Fatal("Available() true but no voices listed")
	}
	data, err := Synthesize(context.Background(), "Merhaba, bu bir testtir.", voices[0].ID)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	// A RIFF/WAVE header is 44 bytes; any real clip is far larger.
	if len(data) < 44 {
		t.Fatalf("WAV too small: %d bytes", len(data))
	}
	if string(data[:4]) != "RIFF" {
		t.Fatalf("output is not a WAV (no RIFF header): %q", data[:4])
	}
}
