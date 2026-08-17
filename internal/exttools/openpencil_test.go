package exttools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// OpenPencil's binary is `op`, which is ALSO the 1Password CLI. These tests pin
// the resolution rules that keep the panel from reporting one tool's version as
// the other's — the failure mode is not "no answer" but a confident wrong one.

func TestOpenPencilCatalogEntry(t *testing.T) {
	tool := Find(OpenPencilToolName)
	if tool == nil {
		t.Fatal("openpencil katalogda yok")
	}
	if got := tool.Repo(); got != "ZSeven-W/openpencil" {
		t.Errorf("release akışı çözülemedi: %q", got)
	}
	if tool.Update.Kind != UpdateManual {
		t.Errorf("güncelleme manual olmalı (iki arşiv + çalışan sunucu kilidi), alınan %q", tool.Update.Kind)
	}
	// A manual spec's only user-facing instruction is the note; an empty one
	// leaves the panel with a dead "Güncelle" area.
	if tool.Update.Note == "" {
		t.Error("manual güncelleme notu boş")
	}
	if len(tool.VersionArgs) == 0 {
		t.Error("sürüm bayrağı yok — panel sürüm çipi gösteremez")
	}
}

// The catalog key must not be the executable name: Find("op") landing on
// OpenPencil would make the update endpoint ambiguous the day 1Password is added.
func TestOpenPencilCatalogKeyIsNotBareOp(t *testing.T) {
	if OpenPencilToolName == "op" {
		t.Fatal("katalog anahtarı `op` olamaz — 1Password CLI ile çakışır")
	}
	if Find("op") != nil {
		t.Error("katalogda `op` adlı bir giriş var; anahtar benzersiz olmalı")
	}
}

func TestOpenPencilEnvOverride(t *testing.T) {
	exe := filepath.Join(t.TempDir(), openPencilExeName())
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TIONSWARM_OPENPENCIL", exe)

	if got := openPencilExe(); got != exe {
		t.Errorf("env override kullanılmadı: %q", got)
	}
	found, p := Detect(OpenPencilToolName)
	if !found || p != exe {
		t.Errorf("Detect env override'ı görmedi: found=%v path=%q", found, p)
	}
}

// A configured-but-missing path means "not installed". Falling through to PATH
// here would run a binary the user did not configure — and on most machines that
// binary is 1Password's.
func TestOpenPencilBrokenOverrideDoesNotFallBack(t *testing.T) {
	t.Setenv("TIONSWARM_OPENPENCIL", filepath.Join(t.TempDir(), "yok", openPencilExeName()))
	if got := openPencilExe(); got != "" {
		t.Errorf("kırık override PATH'e düşmemeli, alınan %q", got)
	}
}

// isolateHome points os.UserHomeDir at an empty temp dir so the Progs candidates
// cannot match. Without it this test passes or fails depending on whether the
// developer happens to have OpenPencil installed under Desktop\Progs — which is
// exactly the case on the machine this entry was written for.
func isolateHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp) // windows
	t.Setenv("HOME", tmp)        // unix
}

// The core guard: a PATH `op` is accepted only when its path names openpencil.
func TestOpenPencilPathFallbackRejects1Password(t *testing.T) {
	t.Setenv("TIONSWARM_OPENPENCIL", "")
	isolateHome(t)
	orig := lookPath
	defer func() { lookPath = orig }()

	cases := []struct {
		name   string
		path   string
		accept bool
	}{
		{"1Password winget", `C:\Users\x\AppData\Local\Microsoft\WinGet\Links\op.exe`, false},
		{"1Password unix", "/usr/local/bin/op", false},
		{"scoop openpencil", `C:\Users\x\scoop\apps\openpencil\current\op.exe`, true},
		{"brew openpencil", "/opt/homebrew/Cellar/openpencil/0.8.4/bin/op", true},
		{"case-insensitive", `C:\Progs\OpenPencil\cli\op.exe`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lookPath = func(string) (string, error) { return c.path, nil }
			got := openPencilExe()
			if c.accept && got != c.path {
				t.Errorf("kabul edilmeliydi: %q → %q", c.path, got)
			}
			if !c.accept && got != "" {
				t.Errorf("REDDEDİLMELİYDİ (1Password olabilir): %q → %q", c.path, got)
			}
		})
	}
}

// OpenPencil marks every release prerelease, so the stable endpoint 404s. The
// opt-in must be declared on the entry, otherwise the panel reports "yayımlanmış
// release yok" for a tool that ships weekly.
func TestOpenPencilOptsIntoPreReleases(t *testing.T) {
	if !Find(OpenPencilToolName).PreRelease {
		t.Error("PreRelease açık olmalı — releases/latest bu repoda 404 döner")
	}
	// The opt-in is per-tool on purpose: a blanket fallback would compare stable
	// users against betas elsewhere in the catalog.
	for _, tool := range Catalog {
		if tool.PreRelease && tool.Name != OpenPencilToolName {
			t.Errorf("%s: PreRelease açılmış — repo'nun gerçekten yalnız prerelease yayımladığı doğrulandı mı?", tool.Name)
		}
	}
}

// The 404-on-stable → newest-from-list fallback, with drafts skipped.
func TestLatestPreReleaseFallsBackToList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v9.9.9","draft":true,"html_url":"u-draft"},
			{"tag_name":"v0.8.4","draft":false,"html_url":"u","published_at":"2026-08-11T17:15:41Z"}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	orig := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = orig }()
	InvalidateCache()

	rel, stale, err := LatestPreRelease(context.Background(), "o/r")
	if err != nil {
		t.Fatalf("prerelease fallback başarısız: %v", err)
	}
	if stale {
		t.Error("taze sonuç stale işaretlendi")
	}
	if rel.Tag != "v0.8.4" {
		t.Errorf("draft atlanmadı ya da yanlış release seçildi: %q", rel.Tag)
	}

	// The stable path must be unaffected — that is the whole reason the opt-in exists.
	InvalidateCache()
	if _, _, err := LatestRelease(context.Background(), "o/r"); err == nil {
		t.Error("stabil yol 404'ü yutup listeye düşmemeli")
	}
}

// `op --version` prints JSON, not a bare semver — the shared version probe must
// still read it. Guards the parser against the one tool in the catalog whose
// version output is structured.
func TestOpenPencilJSONVersionParses(t *testing.T) {
	m := semverRe.FindStringSubmatch(`{"version":"0.8.4"}`)
	if m == nil {
		t.Fatal("op --version JSON çıktısından sürüm okunamadı")
	}
	if got := normalizeVersion(m); got != "0.8.4" {
		t.Errorf("sürüm %q", got)
	}
}
