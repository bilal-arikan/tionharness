package api

import (
	"context"
	"net/http"
	"runtime"
	"testing"
)

func TestSelectArtifact(t *testing.T) {
	list := []updateArtifact{
		{OS: "linux", Arch: "amd64", File: "th-linux-amd64", URL: "https://x.test/l-amd64"},
		{OS: "windows", Arch: "amd64", File: "th-windows-amd64.exe", URL: "https://x.test/w-amd64"},
		{OS: "darwin", Arch: "arm64", File: "th-darwin-arm64", URL: "https://x.test/d-arm64"},
		{OS: "linux", Arch: "arm64", File: "th-linux-arm64", URL: ""},
	}
	cases := []struct {
		name    string
		goos    string
		goarch  string
		wantURL string
		wantOK  bool
	}{
		{"exact match", "windows", "amd64", "https://x.test/w-amd64", true},
		{"other platform", "darwin", "arm64", "https://x.test/d-arm64", true},
		{"arch must match too", "darwin", "amd64", "", false},
		{"os must match too", "openbsd", "amd64", "", false},
		{"entry without url is unusable", "linux", "arm64", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := selectArtifact(list, tc.goos, tc.goarch)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tc.wantOK, got)
			}
			if got.URL != tc.wantURL {
				t.Fatalf("url = %q, want %q", got.URL, tc.wantURL)
			}
		})
	}
}

func TestSelectArtifactEmptyList(t *testing.T) {
	if _, ok := selectArtifact(nil, "linux", "amd64"); ok {
		t.Fatal("empty artifact list yielded a match")
	}
}

// A feed carrying a build for this platform must produce a direct download link.
func TestUpdateCheckPicksArtifactForThisPlatform(t *testing.T) {
	stampVersion(t, "0.1.0")
	feed := `{"version":"0.2.0","artifacts":[
		{"os":"plan9","arch":"386","file":"other","url":"https://x.test/other"},
		{"os":"` + runtime.GOOS + `","arch":"` + runtime.GOARCH + `","file":"mine","url":"https://x.test/mine"}
	]}`
	feedServer(t, http.StatusOK, feed)

	got := newUpdateChecker().Status(context.Background(), nil)
	if got.DownloadURL != "https://x.test/mine" || got.DownloadFile != "mine" {
		t.Fatalf("download = %q/%q, want https://x.test/mine/mine", got.DownloadURL, got.DownloadFile)
	}
}

// No artifact for this platform: link the releases page, never a foreign binary.
func TestUpdateCheckFallsBackToReleasesPage(t *testing.T) {
	stampVersion(t, "0.1.0")
	feedServer(t, http.StatusOK, okFeed) // okFeed has an empty artifact list

	got := newUpdateChecker().Status(context.Background(), nil)
	if got.DownloadFile != "" {
		t.Fatalf("downloadFile = %q, want empty on fallback", got.DownloadFile)
	}
	if want := FeedURL() + "/releases"; got.DownloadURL != want {
		t.Fatalf("downloadUrl = %q, want %q", got.DownloadURL, want)
	}
}
