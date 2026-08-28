// Package tts shells out to an OPTIONAL external Piper text-to-speech CLI found
// on the host, mirroring the codebase-memory/rtk external-tool pattern: nothing
// is compiled in, and when no piper binary + voice is present the server reports
// unavailable so the frontend falls back to the browser's speechSynthesis.
//
// Running synthesis on the server (not the client) is what makes read-aloud work
// on phones and other thin clients: they just play the returned WAV.
package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// synthTimeout caps one synthesis run so a stuck binary can never hang a request.
const synthTimeout = 60 * time.Second

// Voice is one installed .onnx Piper voice model, addressed by its file stem (an
// opaque id — the absolute path is never exposed to the client).
type Voice struct {
	ID   string `json:"id"`   // file stem, e.g. "tr_TR-dfki-medium"
	Name string `json:"name"` // display label (stem for now)
	Lang string `json:"lang"` // best-effort BCP-47 tag parsed from the name
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "piper.exe"
	}
	return "piper"
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// venvScriptDir is the per-platform folder a Python venv puts console scripts in.
func venvScriptDir() string {
	if runtime.GOOS == "windows" {
		return "Scripts"
	}
	return "bin"
}

// piperExe resolves the piper binary: the TIONHARNESS_PIPER env override, then the
// common Progs install layout, then PATH. Returns "" when not found.
//
// The venv candidate comes FIRST because it is the only shape upstream still
// ships for Windows: piper1-gpl stopped publishing a standalone archive and
// releases a Python wheel instead (piper_tts-*.whl), whose console script lands
// in <venv>/Scripts. The legacy standalone layout stays in the list — an older
// rhasspy/piper install keeps working — but a machine that has both should
// report the one that is maintained. The CLI contract is unchanged across the
// two: `--model` and `--output_file` are still accepted (piper1-gpl keeps the
// underscore spellings as aliases), so Synthesize below needs no version fork.
func piperExe() string {
	if p := strings.TrimSpace(os.Getenv("TIONHARNESS_PIPER")); p != "" && fileExists(p) {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		progs := filepath.Join(home, "Desktop", "Progs", "piper")
		for _, c := range []string{
			filepath.Join(progs, ".venv", venvScriptDir(), exeName()),
			filepath.Join(progs, "piper", exeName()),
			filepath.Join(progs, exeName()),
		} {
			if fileExists(c) {
				return c
			}
		}
	}
	if p, err := exec.LookPath("piper"); err == nil {
		return p
	}
	return ""
}

// voiceDirs are scanned for *.onnx models: an env override, the dirs around the
// binary, and the Progs voices folder.
func voiceDirs() []string {
	var dirs []string
	if v := strings.TrimSpace(os.Getenv("TIONHARNESS_PIPER_VOICES")); v != "" {
		dirs = append(dirs, v)
	}
	if exe := piperExe(); exe != "" {
		base := filepath.Dir(exe)
		up1 := filepath.Dir(base)
		// Two levels up covers the venv layout, where the binary sits at
		// <install>/.venv/Scripts and the models stay at <install>/voices — one
		// level up would only reach <install>/.venv/voices, which nothing writes to.
		dirs = append(dirs, filepath.Join(base, "voices"), base,
			filepath.Join(up1, "voices"), filepath.Join(filepath.Dir(up1), "voices"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Desktop", "Progs", "piper", "voices"))
	}
	return dirs
}

// langFromName pulls a language tag from a Piper voice filename, e.g.
// "tr_TR-dfki-medium" → "tr-TR" (the head token before the first '-').
func langFromName(stem string) string {
	head := stem
	if i := strings.Index(head, "-"); i > 0 {
		head = head[:i]
	}
	return strings.ReplaceAll(head, "_", "-")
}

// Voices scans the known directories for .onnx models, deduped by stem and sorted.
func Voices() []Voice {
	seen := map[string]bool{}
	var out []Voice
	for _, d := range voiceDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".onnx") {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if seen[strings.ToLower(stem)] {
				continue
			}
			seen[strings.ToLower(stem)] = true
			out = append(out, Voice{ID: stem, Name: stem, Lang: langFromName(stem)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// voicePath resolves a voice id (stem) to its absolute .onnx path, or "" when no
// installed voice matches. Restricting to discovered voices also blocks a client
// from pointing --model at an arbitrary file.
func voicePath(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, d := range voiceDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".onnx") {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if strings.EqualFold(stem, id) {
				return filepath.Join(d, e.Name())
			}
		}
	}
	return ""
}

// Available reports whether a piper binary AND at least one voice are installed.
func Available() bool {
	return piperExe() != "" && len(Voices()) > 0
}

// BinaryPath returns the resolved piper executable path, or "" when not found.
// Exposed so the external-tools panel can detect Piper even though it usually
// lives OUTSIDE PATH (env override / the Progs install layout).
func BinaryPath() string {
	return piperExe()
}

// Synthesize renders text to WAV bytes with the given voice id (falls back to the
// first installed voice when the id is empty/unknown). Errors when piper is
// absent, no voice matches, or the process fails/produces no audio.
func Synthesize(ctx context.Context, text, voiceID string) ([]byte, error) {
	exe := piperExe()
	if exe == "" {
		return nil, errors.New("piper not found")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty text")
	}
	model := voicePath(voiceID)
	if model == "" {
		if vs := Voices(); len(vs) > 0 {
			model = voicePath(vs[0].ID)
		}
	}
	if model == "" {
		return nil, errors.New("no piper voice installed")
	}

	// piper writes WAV to a file (stdout-WAV is unreliable across builds).
	tmp, err := os.CreateTemp("", "tts-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	cctx, cancel := context.WithTimeout(ctx, synthTimeout)
	defer cancel()
	cmd := proc.CommandContext(cctx, exe, "--model", model, "--output_file", tmpPath)
	// espeak-ng-data ships next to the binary; run from there so phonemization
	// resolves (matches the manual test that set-location'd into the piper dir).
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("piper failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("piper produced no audio")
	}
	return data, nil
}
