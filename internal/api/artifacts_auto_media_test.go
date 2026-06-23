package api

import (
	"testing"
)

// TestExtractProducedMediaPaths covers the heuristic that pulls saved media file
// paths out of a tool's output (e.g. a screenshot tool's JSON result).
func TestExtractProducedMediaPaths(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "json windows path with escaped backslashes",
			output: `{"success":true,"path":"C:\\Users\\user\\Downloads\\shot_2026.png"}`,
			want:   []string{`C:\Users\user\Downloads\shot_2026.png`},
		},
		{
			name:   "plain windows path",
			output: `Saved screenshot to C:\tmp\out.jpg done`,
			want:   []string{`C:\tmp\out.jpg`},
		},
		{
			name:   "posix path",
			output: `wrote /home/u/exports/report.pdf`,
			want:   []string{`/home/u/exports/report.pdf`},
		},
		{
			name:   "no media path",
			output: `{"success":true,"message":"navigated"}`,
			want:   nil,
		},
		{
			name:   "dedup repeats",
			output: `C:\a\x.png and again C:\a\x.png`,
			want:   []string{`C:\a\x.png`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractProducedMediaPaths(tc.output)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestMediaKindForExt and artifactKindForPath media routing.
func TestArtifactKindForPath_Media(t *testing.T) {
	cases := map[string]string{
		`C:\x\a.png`:  "image",
		"a.JPG":       "image",
		"clip.mp4":    "video",
		"voice.mp3":   "audio",
		"report.pdf":  "file",
		"data.zip":    "file",
		"notes.md":    "markdown",
		"main.go":     "code",
		"diagram.svg": "svg",
	}
	for in, want := range cases {
		if got, _ := artifactKindForPath(in); got != want {
			t.Errorf("artifactKindForPath(%q) = %q, want %q", in, got, want)
		}
	}
}
