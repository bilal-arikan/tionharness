package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstructionFileCandidates are the working-directory instruction files a turn
// probes for, in priority order. CLAUDE.md wins when both exist: it is the
// TionHarness-native name, and a repo carrying both usually keeps AGENTS.md for
// other tools. AGENTS.md is the cross-vendor convention (codex-cli and others),
// so a repo that only has it still gets a pointer.
//
// The file's CONTENT is deliberately never read or inlined: claude-cli and
// codex-cli load their project file natively, so inlining it here would ship the
// same bytes twice. Only the pointer line is emitted.
var InstructionFileCandidates = []string{"CLAUDE.md", "AGENTS.md"}

// statInstructionFile is os.Stat, indirected only so the tests can exercise the
// "stat failed for a reason other than absence" branch. That branch cannot be
// provoked portably: Windows reports ENOTDIR-style failures as ErrNotExist, so a
// real unreadable path proves nothing about the code on this machine.
var statInstructionFile = os.Stat

// ResolveInstructionFile reports the name of the instruction file present in dir
// (not its path), or "" when dir holds none. Only os.Stat is used — the file is
// never opened.
//
// A stat error other than "does not exist" (a permission denial, an I/O fault, a
// path component that is not a directory) is RETURNED, not swallowed: it means
// the probe could not answer, which is different from "no instruction file here",
// and silently reporting the latter would hide a broken working directory.
//
// Case sensitivity follows the filesystem, not this function: on Windows (and on
// macOS's default APFS) `claude.md` satisfies the `CLAUDE.md` stat, while on
// Linux it does not. Callers must not rely on either behaviour.
func ResolveInstructionFile(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	for _, name := range InstructionFileCandidates {
		_, err := statInstructionFile(filepath.Join(dir, name))
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

// InstructionFilePointerLine renders the prompt line that tells the agent an
// instruction file is present, or "" when dir holds none (the line is then
// omitted entirely rather than emitted empty). A stat failure is surfaced to the
// caller, which decides whether a broken working directory should fail the turn.
func InstructionFilePointerLine(dir string) (string, error) {
	name, err := ResolveInstructionFile(dir)
	if err != nil || name == "" {
		return "", err
	}
	return "- A `" + name + "` is present — read it for project structure, conventions and build/test commands.\n", nil
}

// InstructionFilePromptLine is InstructionFilePointerLine for the two
// prompt-block builders, which compose a plain string and have no error channel
// to a caller. A probe failure is reported TO THE AGENT rather than dropped: it
// is the one consumer that can act on it (re-check the path, pick another
// working dir) and it lands in the transcript, whereas returning "" would be
// indistinguishable from "this repo has no instruction file".
func InstructionFilePromptLine(dir string) string {
	line, err := InstructionFilePointerLine(dir)
	if err != nil {
		return "- Could not probe this directory for an instruction file (" +
			strings.TrimSpace(err.Error()) + ") — the working directory may be unreadable.\n"
	}
	return line
}
