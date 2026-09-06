package conversation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// attachRootCtxKey carries the workspace sandbox root (<DataDir>/workspace) on
// the turn context. Attachment.RelPath is stored relative to that root, but the
// Manager is shared across workspaces (see compactPromptCtxKey for the same
// rationale), so the root cannot live on the Manager — it rides per turn.
type attachRootCtxKey struct{}

// WithAttachmentRoot returns a context carrying the workspace sandbox root used
// to turn an Attachment.RelPath into an absolute path the agent can open
// directly. Empty input is a no-op.
//
// Without it the model only sees a relative path, which it cannot resolve: a
// session's cwd is its WorkingDir (an arbitrary project folder), NOT the
// workspace sandbox. That mismatch made agents hunt for their own attachment
// with repeated Glob/Grep sweeps before finding it — the reason this exists.
func WithAttachmentRoot(ctx context.Context, root string) context.Context {
	if strings.TrimSpace(root) == "" {
		return ctx
	}
	return context.WithValue(ctx, attachRootCtxKey{}, root)
}

// attachmentRootFromCtx returns the sandbox root carried by ctx, or "" when the
// caller did not supply one.
func attachmentRootFromCtx(ctx context.Context) string {
	root, _ := ctx.Value(attachRootCtxKey{}).(string)
	return root
}

// absAttachmentPath resolves an attachment's stored relative path against the
// sandbox root, in the host's native path style (the fs tools take native
// paths). Returns "" when the root is unknown, so callers can say so plainly
// rather than hand the model a path that silently resolves nowhere.
func absAttachmentPath(root, rel string) string {
	if root == "" || rel == "" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

// humanSize renders a byte count compactly (e.g. "4.7 MB"). Sizes drive the
// agent's reading strategy — a 5 MB log must be grepped/paged, not slurped —
// so the raw byte count alone buries the signal.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// largeAttachmentBytes is the size above which an attachment gets an explicit
// "page/grep it" hint. Set at 256 KB: comfortably above a file Read can take in
// a couple of calls, well below the multi-MB logs that trigger the problem.
const largeAttachmentBytes = 256 << 10

// withAttachments appends an "Attachments" block to a user message's text. Text
// and code attachments small enough to have been inlined at upload time are
// reproduced verbatim (the model reads them directly); everything else is listed
// by absolute path so an agent can open it with Read straight away.
func withAttachments(ctx context.Context, msg db.Message) string {
	if len(msg.Attachments) == 0 {
		return msg.Text
	}
	root := attachmentRootFromCtx(ctx)

	var b strings.Builder
	b.WriteString(msg.Text)
	b.WriteString("\n\n## Attachments\n")
	big := false
	for _, a := range msg.Attachments {
		if a.TextContent != "" {
			fmt.Fprintf(&b, "\n### %s (%s)\n```\n%s\n```\n", a.Name, a.Kind, a.TextContent)
			continue
		}
		if a.RelPath == "" {
			fmt.Fprintf(&b, "- %s (%s)\n", a.Name, a.Kind)
			continue
		}
		if a.Size > largeAttachmentBytes {
			big = true
		}
		if abs := absAttachmentPath(root, a.RelPath); abs != "" {
			fmt.Fprintf(&b, "- %s (%s, %s) — Read/Grep this exact path: %s\n",
				a.Name, a.Kind, humanSize(a.Size), abs)
			continue
		}
		// No sandbox root on the context. Say the path is relative instead of
		// presenting it as openable — an agent that trusts it would search the
		// filesystem for a file that was never where it looked.
		fmt.Fprintf(&b, "- %s (%s, %s) — path relative to the workspace sandbox root: %s\n",
			a.Name, a.Kind, humanSize(a.Size), a.RelPath)
	}
	if big {
		b.WriteString("\nThe file(s) above are too large to read whole: Grep for the " +
			"relevant lines first, then Read around the hits with offset/limit.\n")
	}
	return strings.TrimSpace(b.String())
}
