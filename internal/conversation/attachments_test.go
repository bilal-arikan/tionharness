package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestWithAttachments verifies that user-message attachments are folded into the
// provider text: text/code inline verbatim, everything else listed by an
// ABSOLUTE path (resolved against the ctx-carried sandbox root) so the agent can
// open it without searching for it.
func TestWithAttachments(t *testing.T) {
	root := filepath.Join("C:", "ws", "workspace")
	msg := db.Message{
		Role: providers.RoleUser,
		Text: "look at these",
		Attachments: []db.Attachment{
			{Name: "notes.txt", Kind: "text", TextContent: "hello inline"},
			{Name: "shot.png", Kind: "image", Size: 1234, RelPath: "artifacts/s1/ab-shot.png"},
		},
	}
	got := withAttachments(WithAttachmentRoot(context.Background(), root), msg)

	if !strings.Contains(got, "look at these") {
		t.Fatalf("original text dropped: %q", got)
	}
	if !strings.Contains(got, "hello inline") {
		t.Errorf("text attachment not inlined: %q", got)
	}
	want := filepath.Join(root, "artifacts", "s1", "ab-shot.png")
	if !strings.Contains(got, want) {
		t.Errorf("absolute attachment path missing (want %q): %q", want, got)
	}
	if !strings.Contains(got, "## Attachments") {
		t.Errorf("attachments header missing: %q", got)
	}
	// A small file must not carry the paging hint.
	if strings.Contains(got, "too large to read whole") {
		t.Errorf("unexpected large-file hint for a 1234-byte attachment: %q", got)
	}
}

// TestWithAttachmentsNoRoot documents the degraded path: without a sandbox root
// on the context the relative path is still shown, but LABELLED as relative so
// the model does not treat it as directly openable.
func TestWithAttachmentsNoRoot(t *testing.T) {
	msg := db.Message{
		Role:        providers.RoleUser,
		Text:        "hi",
		Attachments: []db.Attachment{{Name: "a.bin", Kind: "binary", Size: 10, RelPath: "artifacts/s1/a.bin"}},
	}
	got := withAttachments(context.Background(), msg)
	if !strings.Contains(got, "artifacts/s1/a.bin") {
		t.Errorf("path dropped: %q", got)
	}
	if !strings.Contains(got, "relative to the workspace sandbox root") {
		t.Errorf("relative path not labelled as such: %q", got)
	}
}

// TestWithAttachmentsLargeHint checks that a multi-MB attachment (the case that
// sent agents Read-ing blind) tells the agent to grep first.
func TestWithAttachmentsLargeHint(t *testing.T) {
	msg := db.Message{
		Role: providers.RoleUser,
		Text: "why does it freeze",
		Attachments: []db.Attachment{
			{Name: "log.txt", Kind: "text", Size: 4942432, RelPath: "artifacts/s1/log.txt"},
		},
	}
	got := withAttachments(WithAttachmentRoot(context.Background(), "/ws"), msg)
	if !strings.Contains(got, "4.7 MB") {
		t.Errorf("human size missing: %q", got)
	}
	if !strings.Contains(got, "too large to read whole") {
		t.Errorf("large-file hint missing: %q", got)
	}
}

// TestWithAttachmentsNone leaves a plain message untouched.
func TestWithAttachmentsNone(t *testing.T) {
	msg := db.Message{Role: providers.RoleUser, Text: "plain"}
	if got := withAttachments(context.Background(), msg); got != "plain" {
		t.Errorf("expected unchanged text, got %q", got)
	}
}

// TestHumanSize covers the unit boundaries the attachment line depends on.
func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{16 << 10, "16.0 KB"},
		{4942432, "4.7 MB"},
		{5 << 30, "5.0 GB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
