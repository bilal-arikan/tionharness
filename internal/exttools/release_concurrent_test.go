package exttools

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLatestReleaseSharesConcurrentRequests(t *testing.T) {
	resetCacheForTest(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://example.test/release"}`))
	}))
	defer server.Close()
	previous := githubAPI
	githubAPI = server.URL
	defer func() { githubAPI = previous }()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			rel, _, err := LatestRelease(t.Context(), "owner/shared")
			if err != nil || rel.Tag != "v1.2.3" {
				t.Errorf("release = %v, %v", rel, err)
			}
		})
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", hits.Load())
	}
}
