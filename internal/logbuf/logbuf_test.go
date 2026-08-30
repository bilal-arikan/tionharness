package logbuf

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHandlerCapsLargeMessageAndStringAttrs(t *testing.T) {
	buf := New(1)
	logger := slog.New(buf.Handler(slog.NewTextHandler(io.Discard, nil)))
	large := strings.Repeat("uyarı-", capturedStringLimit)
	logger.LogAttrs(context.Background(), slog.LevelError, large,
		slog.String("detail", large), slog.Int("attempt", 7))
	entry := buf.Entries(0)[0]
	for name, value := range map[string]string{"message": entry.Message, "attr": entry.Attrs["detail"]} {
		if len(value) > capturedStringLimit {
			t.Errorf("%s length = %d, want <= %d", name, len(value), capturedStringLimit)
		}
		if !strings.Contains(value, "bytes omitted") || !strings.Contains(value, "uyarı-") {
			t.Errorf("%s lacks prefix or truncation marker", name)
		}
		if !utf8.ValidString(value) {
			t.Errorf("%s contains invalid UTF-8", name)
		}
	}
	if got := entry.Attrs["attempt"]; got != "7" {
		t.Fatalf("non-string attr = %q, want 7", got)
	}
}
