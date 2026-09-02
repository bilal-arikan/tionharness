package tools

import (
	"context"
	"fmt"
	"sync"
)

// SkillLedger records which skill bodies a session has already loaded, so a
// second use_skill for the same slug returns a short pointer instead of the full
// markdown again.
//
// Why this exists: a coordinator re-reads its doctrine/context skills on almost
// every turn. In SES2570 (a 20h coordinator run) use_skill was called 174 times
// and 167 of those loads were slugs the SAME session had already loaded —
// ~2.25 MB of duplicate skill body, which was essentially the whole growth of
// that session's context between folds. The body is already in the model's
// window; resending it buys nothing.
//
// Epochs make the suppression safe. A rolling-summary fold drops the older
// messages — including the skill body — out of the window, so a reload AFTER a
// fold is legitimate and must return the real text. Callers pass the session's
// current fold count as the epoch; an entry from an older epoch is treated as
// absent. Session-scoped and in-memory: a restart re-serves every body once.
type SkillLedger struct {
	mu     sync.Mutex
	loaded map[string]skillLoad
	n      int
}

// skillLoad is one recorded load: the epoch it happened in and its ordinal
// within the session (1-based), which the pointer text quotes so the model can
// find the body in its own history.
type skillLoad struct {
	epoch   int
	ordinal int
}

// NewSkillLedger constructs an empty ledger.
func NewSkillLedger() *SkillLedger {
	return &SkillLedger{loaded: map[string]skillLoad{}}
}

// Note records a load of slug at the given epoch and reports whether the same
// slug was already loaded in that same epoch. ordinal is the 1-based position of
// the ORIGINAL load within the session. A nil ledger reports "not loaded", so
// callers without one keep the pre-ledger behaviour of always sending the body.
func (l *SkillLedger) Note(slug string, epoch int) (already bool, ordinal int) {
	if l == nil || slug == "" {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if prev, ok := l.loaded[slug]; ok && prev.epoch == epoch {
		return true, prev.ordinal
	}
	l.n++
	l.loaded[slug] = skillLoad{epoch: epoch, ordinal: l.n}
	return false, l.n
}

// Forget drops a slug from the ledger so its next load serves the full body
// again. Used when the skill's content changed under the session (edited or
// re-imported) — the cached-in-context copy is then stale, not redundant.
func (l *SkillLedger) Forget(slug string) {
	if l == nil || slug == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.loaded, slug)
}

// SkillReloadPointer is the text served instead of a duplicate skill body. It
// states plainly that the body was withheld (never pretend the skill is
// missing), where to find it, and the exact escape hatch — so a model that
// genuinely lost the instructions can always get them back.
func SkillReloadPointer(slug string, ordinal int) string {
	return fmt.Sprintf(
		"# Skill: %s — already loaded (body withheld)\n\n"+
			"You loaded this skill earlier in THIS session (load #%d) and its instructions are\n"+
			"still in your context above. The body was not sent again to save tokens.\n\n"+
			"Scroll back to that load and follow it. If the instructions are genuinely no longer\n"+
			"visible to you, re-request them explicitly: use_skill with slug=%q and force=true.",
		slug, ordinal, slug)
}

// skillLedgerKey keys the session skill ledger on a request context.
type skillLedgerKey struct{}

// skillLedgerRef binds a ledger to the epoch of the turn that carries it, so the
// tool does not have to look the session's fold count up again.
type skillLedgerRef struct {
	ledger *SkillLedger
	epoch  int
}

// WithSkillLedger attaches the session's skill ledger and the current epoch (the
// session's rolling-summary fold count) to ctx.
func WithSkillLedger(ctx context.Context, l *SkillLedger, epoch int) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, skillLedgerKey{}, skillLedgerRef{ledger: l, epoch: epoch})
}

// SkillLedgerFrom returns the ledger and epoch attached to ctx. The ledger is nil
// when none is present, and a nil ledger's Note reports "not loaded".
func SkillLedgerFrom(ctx context.Context) (*SkillLedger, int) {
	ref, _ := ctx.Value(skillLedgerKey{}).(skillLedgerRef)
	return ref.ledger, ref.epoch
}
