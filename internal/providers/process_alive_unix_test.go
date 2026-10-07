//go:build !windows

package providers

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestParsePSLstart(t *testing.T) {
	for _, in := range []string{"Tue Oct  7 03:23:12 2026\n", "Tue Oct 17 03:23:12 2026"} {
		got, ok := parsePSLstart(in)
		if !ok || got.Year() != 2026 || got.Month() != time.October || got.Hour() != 3 {
			t.Errorf("parsePSLstart(%q) = %v, %v", in, got, ok)
		}
	}
	if _, ok := parsePSLstart("garbage"); ok {
		t.Error("parsePSLstart accepted garbage")
	}
}

// The own process start time must be known on macOS and Linux and lie in the
// recent past, or stale codex shadow homes are never swept there.
func TestProcessStartTimeOwnProcess(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("start time is only resolved on macOS and Linux")
	}
	got, ok := processStartTime(os.Getpid())
	if !ok {
		t.Fatal("start time of the test process is unavailable")
	}
	if age := time.Since(got); age < -2*time.Second || age > time.Hour {
		t.Fatalf("start time %v is implausible (age %v)", got, age)
	}
}
