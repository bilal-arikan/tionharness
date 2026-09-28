package interaction

import "testing"

func TestAsyncUserInputPreservesStructuredToolOutput(t *testing.T) {
	out := toolContent(`{"ok":true}`, false, "[User reply] Blue")
	content := out["content"].([]map[string]string)
	if len(content) != 2 || content[0]["text"] != `{"ok":true}` || content[1]["text"] != "[User reply] Blue" {
		t.Fatalf("content=%+v", content)
	}
}
