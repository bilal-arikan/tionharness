package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestWriteDBErrorClientClosed: a request the client aborted (a panel's
// AbortController) answers 499, not a 500 that logs as a server ERROR.
func TestWriteDBErrorClientClosed(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{context.Canceled, statusClientClosedRequest},
		{fmt.Errorf("read: %w", context.Canceled), statusClientClosedRequest},
		{db.ErrNotFound, http.StatusNotFound},
		{errors.New("disk"), http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		if !writeDBError(rec, tc.err, "") || rec.Code != tc.want {
			t.Fatalf("writeDBError(%v) = %d, want %d", tc.err, rec.Code, tc.want)
		}
	}
}
