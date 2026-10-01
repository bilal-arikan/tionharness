package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellLongHeredocPreservesCompleteUnicodeFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewShellTool(NewSandbox(dir))
	if !tool.Available() {
		t.Skip("no POSIX shell")
	}
	body := strings.Repeat("Full Unicode payload: şğüıöç\n", 1400)
	command := "cat > document.md <<'END_DOCUMENT'\n" + body + "END_DOCUMENT\nprintf 'done'"
	input, _ := json.Marshal(shellArgs{Command: command, NoCompress: true})
	out, err := tool.Call(context.Background(), input)
	if err != nil || strings.Contains(out, "exit error") || !strings.Contains(out, "done") {
		t.Fatalf("out=%s err=%v", out, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "document.md"))
	if err != nil || string(got) != body {
		t.Fatalf("heredoc cut: bytes=%d want=%d err=%v", len(got), len(body), err)
	}
}

func TestWindowsBashPythonOutputUsesUTF8(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows legacy code page regression")
	}
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("Python is not installed")
	}
	t.Setenv("PYTHONUTF8", "0")
	t.Setenv("PYTHONIOENCODING", "cp1254")
	tool := NewShellTool(NewSandbox(t.TempDir()))
	if !tool.Available() {
		t.Skip("no Bash shell")
	}
	input, _ := json.Marshal(shellArgs{Command: `python -c "print('🧠 Turkish: şğüıöç')"`, NoCompress: true})
	out, err := tool.Call(context.Background(), input)
	if err != nil || !strings.Contains(out, "🧠 Turkish: şğüıöç") || strings.Contains(out, "UnicodeEncodeError") {
		t.Fatalf("Python UTF-8 output: out=%s err=%v", out, err)
	}
}
