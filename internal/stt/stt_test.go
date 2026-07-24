package stt

import (
	"context"
	"os"
	"testing"
)

// TestTranscribeSmoke verifies end-to-end STT when whisper-cli + ffmpeg + a model
// are installed. It feeds the Piper-generated Turkish sample (a WAV; ffmpeg
// resamples it) and asserts a non-empty transcript. Skips when unavailable.
func TestTranscribeSmoke(t *testing.T) {
	if !Available() {
		t.Skip("whisper-cli/ffmpeg/model not installed on this host; skipping")
	}
	home, _ := os.UserHomeDir()
	sample := home + `\Desktop\Progs\piper\test_tr.wav`
	audio, err := os.ReadFile(sample)
	if err != nil {
		t.Skipf("sample audio not found (%s); skipping", sample)
	}
	text, err := Transcribe(context.Background(), audio, "tr-TR", "")
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if len(text) < 3 {
		t.Fatalf("transcript suspiciously short: %q", text)
	}
	t.Logf("transcript: %s", text)
}
