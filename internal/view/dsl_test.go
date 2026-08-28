package view

import "testing"

// withHome pins the home directory shortPath resolves against, so the
// expectations below hold on every OS (os.UserHomeDir reads USERPROFILE on
// Windows and HOME elsewhere).
func withHome(t *testing.T, home string) {
	t.Helper()
	prev := homeDir
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = prev })
}

func TestShortPath(t *testing.T) {
	withHome(t, `C:\Users\user`)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"home prefix collapses", `C:\Users\user\Desktop\Projects\app\main.go`, "~/Desktop/Projects/app/main.go"},
		{"home itself", `C:\Users\user`, "~"},
		{"backslashes normalise", `D:\repo\pkg\file.go`, "D:/repo/pkg/file.go"},
		{"partial segment is not a home match", `C:\Users\user-backup\x.go`, "C:/Users/user-backup/x.go"},
		{"non-path untouched", "build failed: unexpected token", "build failed: unexpected token"},
		{"bare tool name untouched", "npx", "npx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortPath(tc.in); got != tc.want {
				t.Errorf("shortPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompactPaths(t *testing.T) {
	withHome(t, `C:\Users\user`)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "windows path inside a sentence",
			in:   `düzenle D:\repo\pkg\file.go ve testi koş`,
			want: "düzenle D:/repo/pkg/file.go ve testi koş",
		},
		{
			name: "posix path inside a sentence",
			in:   "config /usr/local/etc/app.conf okunamadı",
			want: "config /usr/local/etc/app.conf okunamadı",
		},
		{
			name: "home prefix collapses in prose",
			in:   `açık: C:\Users\user\Desktop\notes.md`,
			want: "açık: ~/Desktop/notes.md",
		},
		{
			// Trailing punctuation must stay punctuation, not become part of the path.
			name: "path followed by comma and paren",
			in:   "bkz /etc/hosts, ayrıca (/var/log/app.log) da var",
			want: "bkz /etc/hosts, ayrıca (/var/log/app.log) da var",
		},
		{
			// Over inlinePathBudget: only the last three segments survive.
			name: "long path folds to last three segments",
			in:   `düzenle C:\Users\user\Desktop\Projects\TionHarness\internal\view\session.go tamam`,
			want: "düzenle …/internal/view/session.go tamam",
		},
		{
			name: "no path returns the string unchanged",
			in:   "build failed: unexpected token at line 12",
			want: "build failed: unexpected token at line 12",
		},
		{
			// A URL's "/host/path" is not a filesystem path; rewriting it would
			// corrupt the token it belongs to.
			name: "url left alone",
			in:   "GET https://example.com/a/b/c/d/e/f/g/h/i/j/k/l/m/n failed",
			want: "GET https://example.com/a/b/c/d/e/f/g/h/i/j/k/l/m/n failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compactPaths(tc.in); got != tc.want {
				t.Errorf("compactPaths(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestClipPath(t *testing.T) {
	withHome(t, "/home/bilal")

	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{
			// The informative half of a path is its TAIL, so the cut is marked at
			// the front and lands on a separator boundary.
			name: "front elision on a long path",
			in:   "/home/bilal/desktop/projects/tionharness/frontend/src/features/view/ViewPanel.tsx",
			max:  30,
			want: "…/features/view/ViewPanel.tsx",
		},
		{
			name: "short path returned untouched",
			in:   "internal/view/dsl.go",
			max:  40,
			want: "internal/view/dsl.go",
		},
		{
			name: "home shortening alone can make it fit",
			in:   "/home/bilal/projects/app/main.go",
			max:  25,
			want: "~/projects/app/main.go",
		},
		{
			// No separator: not a path, so clip's front-preserving behaviour is
			// what prose needs.
			name: "non-path uses clip",
			in:   "assertion failed while comparing values",
			max:  20,
			want: "assertion failed wh…",
		},
		{
			// A single segment longer than max is the one case that must be cut
			// mid-word — there is no boundary to fall back to.
			name: "single oversized segment",
			in:   "/verylongsinglesegmentwithoutanyseparators",
			max:  12,
			want: "…/separators",
		},
		{
			name: "whitespace collapses to one line",
			in:   "/home/bilal/a\n/b.go",
			max:  40,
			want: "~/a /b.go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clipPath(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("clipPath(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if n := len([]rune(got)); n > tc.max {
				t.Errorf("clipPath(%q, %d) = %q is %d runes, over budget", tc.in, tc.max, got, n)
			}
		})
	}
}
