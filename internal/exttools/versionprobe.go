package exttools

import (
	"os"
	"path/filepath"
	"runtime"
)

// pipMetadataExpr asks a Python interpreter for an installed distribution's
// version. Used for tools that ship as a wheel and therefore have no --version
// flag of their own; importlib.metadata reads the .dist-info the installer
// wrote, so the answer is the version pip actually resolved.
func pipMetadataExpr(dist string) string {
	return "import importlib.metadata as m; print(m.version(" + pyLiteral(dist) + "))"
}

// pyLiteral renders a string as a Python single-quoted literal. Distribution
// names are ASCII identifiers from this package's own catalog, never user input,
// so escaping only needs to be correct, not exhaustive.
func pyLiteral(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '\'')
	for _, r := range s {
		if r == '\'' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(append(out, '\''))
}

// venvPython returns the interpreter of the virtualenv that owns the console
// script at path, and whether path is in one at all.
//
// The pyvenv.cfg check is what makes this safe. Matching on the parent folder
// name alone would claim /usr/bin/piper — a standalone binary that happens to
// live next to /usr/bin/python3 — and the probe would then report on whatever
// piper-tts that system interpreter has, which is usually none. A missing
// pyvenv.cfg means "not a venv", and the caller falls back to the tool's own
// version flag.
func venvPython(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	scripts := filepath.Dir(path)
	root := filepath.Dir(scripts)
	if st, err := os.Stat(filepath.Join(root, "pyvenv.cfg")); err != nil || st.IsDir() {
		return "", false
	}
	name := "python"
	if runtime.GOOS == "windows" {
		name = "python.exe"
	}
	py := filepath.Join(scripts, name)
	if st, err := os.Stat(py); err != nil || st.IsDir() {
		return "", false
	}
	return py, true
}

// VersionProbe returns the command + args that make this tool report its
// version, given the path Detect resolved. Normally that is the tool itself with
// VersionArgs; empty args mean the tool cannot report a version at all and the
// caller must skip the probe.
//
// piper is the exception. Upstream (OHF-Voice/piper1-gpl) ships a wheel rather
// than a standalone archive on Windows, and its CLI has no --version flag: the
// flag is rejected with usage text carrying no version-shaped token, so a plain
// probe would leave a permanent "sürüm okunamadı" in the panel. When the
// resolved binary is a venv console script we ask that venv's interpreter for
// the piper-tts distribution version instead. A legacy standalone piper is not
// in a venv, keeps answering --version, and takes the default path below.
func (t Tool) VersionProbe(path string) (string, []string) {
	if t.Name == "piper" {
		if py, ok := venvPython(path); ok {
			return py, []string{"-c", pipMetadataExpr("piper-tts")}
		}
	}
	return path, t.VersionArgs
}
