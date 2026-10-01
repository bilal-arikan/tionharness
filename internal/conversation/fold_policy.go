package conversation

import (
	"context"
	"github.com/bilal-arikan/tionharness/internal/db"
	"strings"
)

// FoldSegment preserves provenance. Policies edit summarizer input only.
type FoldSegment struct {
	Key, SourceID, Role, Text, Rendered string
	Mandatory                           bool
}
type FoldPlan struct{ Rendered, Protected string }
type FoldPolicy func(context.Context, string, []FoldSegment) FoldPlan
type foldPolicyKey struct{}
type foldFinishedKey struct{}
type foldSessionKey struct{}

func WithFoldPolicy(ctx context.Context, policy FoldPolicy, finished func(context.Context)) context.Context {
	ctx = context.WithValue(ctx, foldPolicyKey{}, policy)
	return context.WithValue(ctx, foldFinishedKey{}, finished)
}
func FoldSessionID(ctx context.Context) string {
	id, _ := ctx.Value(foldSessionKey{}).(string)
	return id
}
func foldPlan(ctx context.Context, existing string, segments []FoldSegment) FoldPlan {
	if policy, _ := ctx.Value(foldPolicyKey{}).(FoldPolicy); policy != nil {
		return policy(ctx, existing, segments)
	}
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Rendered)
	}
	return FoldPlan{Rendered: b.String()}
}
func foldFinished(ctx context.Context) {
	if f, _ := ctx.Value(foldFinishedKey{}).(func(context.Context)); f != nil {
		f(ctx)
	}
}
func dbFoldSegments(msgs []db.Message) []FoldSegment {
	out := make([]FoldSegment, 0, len(msgs))
	lastUser := -1
	for i, m := range msgs {
		if m.Role == "user" {
			lastUser = i
		}
	}
	for i, m := range msgs {
		out = append(out, FoldSegment{Key: "context:" + m.ID, SourceID: m.ID, Role: m.Role, Text: m.Text, Rendered: renderDBMessages([]db.Message{m}), Mandatory: i == lastUser})
	}
	return out
}
