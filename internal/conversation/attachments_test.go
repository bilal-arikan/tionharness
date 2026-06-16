package conversation

import (
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// TestWithAttachments verifies that user-message attachments are folded into the
// provider text: text/code inline verbatim, binary/image listed by read_file path.
func TestWithAttachments(t *testing.T) {
	msg := db.Message{
		Role: providers.RoleUser,
		Text: "look at these",
		Attachments: []db.Attachment{
			{Name: "notes.txt", Kind: "text", TextContent: "hello inline"},
			{Name: "shot.png", Kind: "image", Size: 1234, RelPath: "uploads/s1/ab-shot.png"},
		},
	}
	got := withAttachments(msg)

	if !strings.Contains(got, "look at these") {
		t.Fatalf("original text dropped: %q", got)
	}
	if !strings.Contains(got, "hello inline") {
		t.Errorf("text attachment not inlined: %q", got)
	}
	if !strings.Contains(got, "uploads/s1/ab-shot.png") {
		t.Errorf("binary attachment path missing: %q", got)
	}
	if !strings.Contains(got, "## Attachments") {
		t.Errorf("attachments header missing: %q", got)
	}
}

// TestWithAttachmentsNone leaves a plain message untouched.
func TestWithAttachmentsNone(t *testing.T) {
	msg := db.Message{Role: providers.RoleUser, Text: "plain"}
	if got := withAttachments(msg); got != "plain" {
		t.Errorf("expected unchanged text, got %q", got)
	}
}
