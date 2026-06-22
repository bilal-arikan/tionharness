package tools

import (
	"net/url"
	"strings"
	"testing"
)

func TestHTMLToMarkdown(t *testing.T) {
	base, _ := url.Parse("https://example.com/docs/page")
	in := `<!doctype html><html><head><title>T</title><style>.x{color:red}</style></head>
	<body>
	<nav>menu menu</nav>
	<h2>Hello &amp; Welcome</h2>
	<p>Some <strong>bold</strong> and <em>italic</em> text with <code>inline</code>.</p>
	<ul><li>first</li><li>second</li></ul>
	<a href="/guide">Guide</a>
	<script>var leak = 1;</script>
	</body></html>`

	out := htmlToMarkdown(in, base)

	checks := []string{
		"## Hello & Welcome", // heading + entity unescape
		"**bold**",
		"*italic*",
		"`inline`",
		"- first",
		"- second",
		"[Guide](https://example.com/guide)", // relative href resolved against base
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("output missing %q\n---\n%s", c, out)
		}
	}
	// Non-content blocks must be gone.
	for _, leak := range []string{"var leak", "color:red", "menu menu", "<", ">"} {
		if strings.Contains(out, leak) {
			t.Errorf("output should not contain %q\n---\n%s", leak, out)
		}
	}
}

func TestWebFetchDefShape(t *testing.T) {
	def := NewWebFetchTool().Def()
	if def.Name != "WebFetch" {
		t.Fatalf("tool name = %q, want WebFetch", def.Name)
	}
	if !strings.Contains(string(def.InputSchema), "\"url\"") {
		t.Fatal("WebFetch schema must declare a url property")
	}
}
