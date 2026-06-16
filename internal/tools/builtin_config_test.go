package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigTools_ReadWriteList(t *testing.T) {
	root := t.TempDir()
	sb := NewSandbox(root)
	if !sb.Ready() {
		t.Fatal("sandbox not ready")
	}
	ctx := context.Background()
	write := NewConfigWriteTool(sb)
	read := NewConfigReadTool(sb)
	list := NewConfigListTool(sb)

	// write_config creates the file (and parent dirs) under the config root.
	in, _ := json.Marshal(map[string]string{"path": "prompts/title.md", "content": "HELLO TITLE"})
	if _, err := write.Call(ctx, in); err != nil {
		t.Fatalf("write_config: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "prompts", "title.md")); err != nil || string(data) != "HELLO TITLE" {
		t.Fatalf("file not written correctly: %q err=%v", string(data), err)
	}

	// read_config returns the content.
	rin, _ := json.Marshal(map[string]string{"path": "prompts/title.md"})
	got, err := read.Call(ctx, rin)
	if err != nil || got != "HELLO TITLE" {
		t.Fatalf("read_config = %q err=%v", got, err)
	}

	// list_config includes the written file.
	listed, err := list.Call(ctx, json.RawMessage(`{}`))
	if err != nil || !strings.Contains(listed, "prompts/title.md") {
		t.Fatalf("list_config = %q err=%v", listed, err)
	}
}

func TestConfigTools_PathTraversalBlocked(t *testing.T) {
	root := t.TempDir()
	sb := NewSandbox(root)
	ctx := context.Background()

	// ".." traversal must be rejected on every platform. (An absolute path is
	// platform-dependent: on Windows "/etc/x" is relative and stays in-sandbox,
	// so we don't assert on it here.)
	for _, bad := range []string{"../escape.md", "prompts/../../escape.md", "../../etc/passwd"} {
		in, _ := json.Marshal(map[string]string{"path": bad, "content": "x"})
		if _, err := NewConfigWriteTool(sb).Call(ctx, in); err == nil {
			t.Fatalf("write_config allowed escaping path %q", bad)
		}
		rin, _ := json.Marshal(map[string]string{"path": bad})
		if _, err := NewConfigReadTool(sb).Call(ctx, rin); err == nil {
			t.Fatalf("read_config allowed escaping path %q", bad)
		}
	}
}
