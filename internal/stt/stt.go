// Package stt shells out to an OPTIONAL external whisper.cpp speech-to-text CLI
// (plus ffmpeg for audio conversion) found on the host, mirroring the tts/Piper
// external-tool pattern: nothing is compiled in, and when the binaries/model are
// absent the server reports unavailable so the frontend falls back to the
// browser's Web Speech recognition.
//
// Running STT on the server (not the client) lets thin clients / WebView2 builds
// dictate: they just upload the recorded audio and get text back.
package stt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// transcribeTimeout caps one transcription run.
const transcribeTimeout = 120 * time.Second

// Model is one installed whisper ggml model, addressed by its file stem.
type Model struct {
	ID   string `json:"id"`   // file stem, e.g. "ggml-small"
	Name string `json:"name"` // display label (stem)
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// whisperExe resolves the whisper-cli binary: TIONHARNESS_WHISPER env, then the
// common Progs install layout, then PATH (whisper-cli or the legacy main).
func whisperExe() string {
	if p := strings.TrimSpace(os.Getenv("TIONHARNESS_WHISPER")); p != "" && fileExists(p) {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, c := range []string{
			filepath.Join(home, "Desktop", "Progs", "whisper", "Release", exeName("whisper-cli")),
			filepath.Join(home, "Desktop", "Progs", "whisper", exeName("whisper-cli")),
		} {
			if fileExists(c) {
				return c
			}
		}
	}
	for _, name := range []string{"whisper-cli", "main"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// ffmpegExe resolves ffmpeg (needed to transcode browser audio → 16 kHz mono WAV).
func ffmpegExe() string {
	if p := strings.TrimSpace(os.Getenv("TIONHARNESS_FFMPEG")); p != "" && fileExists(p) {
		return p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}

// modelDirs are scanned for ggml-*.bin models.
func modelDirs() []string {
	var dirs []string
	if v := strings.TrimSpace(os.Getenv("TIONHARNESS_WHISPER_MODELS")); v != "" {
		dirs = append(dirs, v)
	}
	if exe := whisperExe(); exe != "" {
		base := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(base, "models"), base, filepath.Join(filepath.Dir(base), "models"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Desktop", "Progs", "whisper", "models"))
	}
	return dirs
}

// Models scans the known directories for ggml-*.bin models, deduped and sorted.
func Models() []Model {
	seen := map[string]bool{}
	var out []Model
	for _, d := range modelDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if e.IsDir() || !strings.HasPrefix(name, "ggml-") || !strings.HasSuffix(name, ".bin") {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if seen[strings.ToLower(stem)] {
				continue
			}
			seen[strings.ToLower(stem)] = true
			out = append(out, Model{ID: stem, Name: stem})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// modelPath resolves a model id (stem) to its .bin path, or "" when unmatched.
func modelPath(id string) string {
	id = strings.TrimSpace(id)
	for _, d := range modelDirs() {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if strings.HasSuffix(strings.ToLower(e.Name()), ".bin") &&
				(id == "" || strings.EqualFold(stem, id)) {
				return filepath.Join(d, e.Name())
			}
		}
	}
	return ""
}

// WhisperPath returns the resolved whisper-cli path, or "" (for external-tools UI).
func WhisperPath() string { return whisperExe() }

// Available reports whether whisper-cli, ffmpeg AND at least one model are present.
func Available() bool {
	return whisperExe() != "" && ffmpegExe() != "" && modelPath("") != ""
}

// langCode reduces a BCP-47 tag (e.g. "tr-TR") to the ISO-639-1 code whisper
// expects ("tr"); "" or "auto" → "auto" (let whisper detect the language).
func langCode(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang == "" || lang == "auto" {
		return "auto"
	}
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	return lang
}

// Transcribe converts recorded audio bytes to text: ffmpeg → 16 kHz mono WAV,
// then whisper-cli with the given model (first installed when modelID is empty)
// and language. Errors when a binary/model is missing or a step fails.
func Transcribe(ctx context.Context, audio []byte, lang, modelID string) (string, error) {
	whisper := whisperExe()
	if whisper == "" {
		return "", errors.New("whisper-cli not found")
	}
	ffmpeg := ffmpegExe()
	if ffmpeg == "" {
		return "", errors.New("ffmpeg not found (needed to convert audio)")
	}
	if len(audio) == 0 {
		return "", errors.New("empty audio")
	}
	model := modelPath(modelID)
	if model == "" {
		return "", errors.New("no whisper model installed")
	}

	tmpDir, err := os.MkdirTemp("", "stt-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "in") // ffmpeg probes content; extension irrelevant
	if err := os.WriteFile(inPath, audio, 0o600); err != nil {
		return "", err
	}
	wavPath := filepath.Join(tmpDir, "in16.wav")

	cctx, cancel := context.WithTimeout(ctx, transcribeTimeout)
	defer cancel()

	// 1) Transcode to 16 kHz mono WAV (whisper.cpp requires it).
	ff := exec.CommandContext(cctx, ffmpeg, "-y", "-loglevel", "error", "-i", inPath, "-ar", "16000", "-ac", "1", wavPath)
	var ffErr bytes.Buffer
	ff.Stderr = &ffErr
	if err := ff.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(ffErr.String()))
	}

	// 2) Transcribe. -otxt writes "<prefix>.txt"; read that instead of parsing stdout.
	outPrefix := filepath.Join(tmpDir, "out")
	wc := exec.CommandContext(cctx, whisper,
		"-m", model, "-l", langCode(lang), "-nt", "-otxt", "-of", outPrefix, "-f", wavPath)
	wc.Dir = filepath.Dir(whisper) // ggml-cpu-*.dll load from the binary's dir
	var wcErr bytes.Buffer
	wc.Stderr = &wcErr
	if err := wc.Run(); err != nil {
		return "", fmt.Errorf("whisper failed: %w: %s", err, strings.TrimSpace(wcErr.String()))
	}
	data, err := os.ReadFile(outPrefix + ".txt")
	if err != nil {
		return "", fmt.Errorf("reading transcript: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
