package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// FlowRunInput is everything the flow-run projection reads. The caller loads it
// (see Projector.loadFlowRun) so this file stays pure: given the same input it
// always renders the same bytes, which is what makes the result cacheable and
// testable.
type FlowRunInput struct {
	Run   db.FlowRun
	Flow  db.Flow
	Graph orchestration.Graph
	State orchestration.State
	// Sub, when set, narrows the projection to a single node of the run — the
	// drill-down target the run view's handles point at.
	Sub string
	// Now is the clock used for elapsed/age computations. Zero means time.Now().
	Now time.Time
}

// chain-rendering limits. A card keeps the whole run on a couple of lines; when a
// run is longer than that the middle is collapsed and the dropped segments are
// reported via View.Elided rather than silently vanishing.
const (
	chainMaxSegments  = 24
	fullDetailMax     = 40
	chainLineWidth    = 92
	chainContinuation = "      → "
)

// ProjectFlowRun renders a flow run: what ran, how long it took, where it is now,
// and what is wrong with it. This is the highest value-per-token projection in
// the system — a 40-node run collapses to two lines without losing the shape of
// the graph.
func ProjectFlowRun(in FlowRunInput, level Level, lens Lens) (View, error) {
	if in.Run.ID == "" {
		return View{}, fmt.Errorf("flow run input has no run")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindFlowRun, ID: in.Run.ID, Sub: in.Sub},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%s@%d/%d", in.Run.Status, in.Run.UpdatedAt, len(in.State.Trace)),
	}

	if in.Sub != "" {
		return projectFlowNode(in, v, level, now)
	}

	segs, elided := flowChain(in, now)
	v.Elided = elided
	v.Header = flowHeader(in, now)

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if len(segs) > 0 {
		l.add("%s", wrapChain(segs))
	}
	for _, w := range flowSignals(in, now, lens) {
		l.add("%s", w)
	}
	if level == LevelFull {
		if detail, more := flowDetail(in, now); detail != "" {
			l.add("--")
			l.add("%s", detail)
			v.Elided += more
		}
	}
	v.Body = l.String()
	v.Handles = flowHandles(in, level)
	v.finalize()
	return v, nil
}

// projectFlowNode renders one node of a run — the drill-down a handle points at.
// A node that never executed is reported as such rather than rendered blank, and
// a node id that is not in the graph at all is an error: following a stale handle
// must not look like an empty result.
func projectFlowNode(in FlowRunInput, v View, level Level, now time.Time) (View, error) {
	node, inGraph := in.Graph.NodeByID(in.Sub)

	var entry *orchestration.TraceEntry
	for i := range in.State.Trace {
		if in.State.Trace[i].NodeID == in.Sub {
			entry = &in.State.Trace[i] // last occurrence wins (loops re-run a node)
		}
	}
	if !inGraph && entry == nil {
		return View{}, fmt.Errorf("view: run %s has no node %q", in.Run.ID, in.Sub)
	}

	title, typ := in.Sub, "?"
	if inGraph {
		typ = node.Type
		if node.Title != "" {
			title = node.Title
		}
	}
	if entry != nil {
		typ, title = entry.Type, entry.Title
	}

	state := "PENDING"
	switch {
	case in.State.WaitingAt == in.Sub:
		state = "WAITING"
	case in.State.Current == in.Sub && in.Run.Status == db.FlowRunning:
		state = "RUNNING"
	case entry != nil:
		state = "DONE"
	}

	v.Header = fmt.Sprintf("NODE %s#%s %q · %s · %s · asOf %s",
		in.Run.ID, in.Sub, title, typ, state, hhmmss(now))

	var l lines
	if entry == nil {
		l.add("(bu node henüz çalışmadı)")
	} else {
		outMax, inMax := 400, 200
		if level == LevelFull {
			outMax, inMax = 4000, 2000
		}
		l.addIf(entry.Input != "", "girdi: %s", clip(entry.Input, inMax))
		l.add("çıktı: %s", clip(entry.Output, outMax))
		l.addIf(entry.StartMs > 0 && entry.EndMs > entry.StartMs, "süre: %s", durMs(entry.EndMs-entry.StartMs))
		l.addIf(entry.ThreadLen > 0, "önceki bağlam: %d mesaj", entry.ThreadLen)
	}
	v.Body = l.String()
	v.Handles = []Handle{{
		Label: "koşunun tamamı",
		Ref:   Ref{Kind: KindFlowRun, ID: in.Run.ID},
		Level: LevelCard,
	}}
	v.finalize()
	return v, nil
}

// flowHeader is the always-present first line: identity, progress, elapsed time,
// status, freshness.
func flowHeader(in FlowRunInput, now time.Time) string {
	name := in.Flow.Name
	if name == "" {
		name = in.Run.FlowID
	}
	if in.Flow.Emoji != "" {
		name = in.Flow.Emoji + " " + name
	}

	seen := map[string]bool{}
	for _, t := range in.State.Trace {
		seen[t.NodeID] = true
	}

	elapsed := time.Duration(0)
	if in.Run.CreatedAt > 0 {
		end := now
		if in.Run.Status != db.FlowRunning && in.Run.UpdatedAt > 0 {
			end = time.UnixMilli(in.Run.UpdatedAt)
		}
		elapsed = end.Sub(time.UnixMilli(in.Run.CreatedAt))
	}

	head := fmt.Sprintf("FLOW run:%s %q · %d/%d node · %s · %s · asOf %s",
		in.Run.ID, name, len(seen), len(in.Graph.Nodes), dur(elapsed),
		strings.ToUpper(in.Run.Status), hhmmss(now))
	if in.Run.ParentRunID != "" {
		head += fmt.Sprintf("\n  ↑ child of run:%s node:%s", in.Run.ParentRunID, in.Run.ParentNodeID)
	}
	return head
}

// flowChain renders the executed path as compact segments, folding a parallel
// node's children into their parent. Returns the segments plus how many were
// dropped from the middle of an over-long run.
func flowChain(in FlowRunInput, now time.Time) ([]string, int) {
	parentOf := parallelParents(in.Graph)

	var segs []string
	var pending []orchestration.TraceEntry // buffered parallel children
	pendingParent := ""

	// Sequential nodes carry only an `At` stamp, so their duration is the gap to
	// the previous stamp. The engine runs them one at a time, so that gap IS the
	// node's wall time (plus negligible bookkeeping). Parallel children carry
	// explicit StartMs/EndMs and use those instead.
	prev := in.Run.CreatedAt

	flushPending := func() {
		for _, p := range pending {
			segs = append(segs, entrySegment(p, p.EndMs-p.StartMs))
		}
		pending = nil
		pendingParent = ""
	}

	for _, t := range in.State.Trace {
		if parent, ok := parentOf[t.NodeID]; ok && t.Type == orchestration.NodeAgent {
			if pendingParent != "" && pendingParent != parent {
				flushPending()
			}
			pendingParent = parent
			pending = append(pending, t)
			prev = t.At
			continue
		}
		if t.Type == orchestration.NodeParallel && pendingParent == t.NodeID {
			segs = append(segs, parallelSegment(t, pending))
			pending = nil
			pendingParent = ""
			prev = t.At
			continue
		}
		flushPending()
		gap := int64(0)
		if prev > 0 && t.At > prev {
			gap = t.At - prev
		}
		segs = append(segs, entrySegment(t, gap))
		prev = t.At
	}
	flushPending()

	// The node the run is sitting on right now never has a trace entry yet.
	if cur := currentSegment(in, now, prev); cur != "" {
		segs = append(segs, cur)
	}

	// Nodes the run has not reached at all.
	seen := map[string]bool{}
	for _, t := range in.State.Trace {
		seen[t.NodeID] = true
	}
	if pendingCount := len(in.Graph.Nodes) - len(seen); pendingCount > 0 && in.Run.Status == db.FlowRunning {
		segs = append(segs, fmt.Sprintf("…%d pending", pendingCount))
	}

	if len(segs) <= chainMaxSegments {
		return segs, 0
	}
	half := chainMaxSegments / 2
	dropped := len(segs) - chainMaxSegments
	folded := append([]string{}, segs[:half]...)
	folded = append(folded, fmt.Sprintf("…%d node atlandı…", dropped))
	folded = append(folded, segs[len(segs)-half:]...)
	return folded, dropped
}

// entrySegment renders one executed node. spanMs <= 0 renders without a duration
// rather than printing a misleading "0ms".
func entrySegment(t orchestration.TraceEntry, spanMs int64) string {
	label := t.Type + ":" + clip(t.Title, 24)
	switch t.Type {
	case orchestration.NodeStart, orchestration.NodeEnd:
		label = t.Type
	case orchestration.NodeBranch:
		// The engine records the chosen arm as "→ <label>" in Output.
		arm := clip(strings.TrimPrefix(t.Output, "→ "), 20)
		return fmt.Sprintf("branch:%s⑂[%s]", clip(t.Title, 20), arm)
	}
	seg := label + "✓"
	if spanMs > 0 {
		seg += "(" + durMs(spanMs) + ")"
	}
	return seg
}

// parallelSegment folds a fan-out into one segment: how many children ran and
// the wall time of the whole fan-out (max end − min start), which is the number
// that actually matters for a parallel node.
func parallelSegment(parent orchestration.TraceEntry, children []orchestration.TraceEntry) string {
	if len(children) == 0 {
		return "parallel:" + clip(parent.Title, 20) + "✓"
	}
	var minStart, maxEnd int64
	for _, c := range children {
		if c.StartMs > 0 && (minStart == 0 || c.StartMs < minStart) {
			minStart = c.StartMs
		}
		if c.EndMs > maxEnd {
			maxEnd = c.EndMs
		}
	}
	seg := fmt.Sprintf("parallel:%s[%d/%d✓", clip(parent.Title, 20), len(children), len(children))
	if minStart > 0 && maxEnd > minStart {
		seg += " " + durMs(maxEnd-minStart)
	}
	return seg + "]"
}

// currentSegment renders the node the run is parked on — running, suspended at
// an await-input, or failed. Returns "" for a run that has finished cleanly.
func currentSegment(in FlowRunInput, now time.Time, lastAt int64) string {
	if in.State.WaitingAt != "" {
		n, ok := in.Graph.NodeByID(in.State.WaitingAt)
		title := in.State.WaitingAt
		if ok && n.Title != "" {
			title = n.Title
		}
		return fmt.Sprintf("await:%s⏸WAITING", clip(title, 24))
	}
	if in.Run.Status == db.FlowFailure {
		return "✗FAILED"
	}
	if in.Run.Status != db.FlowRunning || in.State.Current == "" {
		return ""
	}
	n, ok := in.Graph.NodeByID(in.State.Current)
	if !ok {
		return fmt.Sprintf("%s⚡RUNNING", clip(in.State.Current, 24))
	}
	title := n.Title
	if title == "" {
		title = n.ID
	}
	seg := fmt.Sprintf("%s:%s⚡RUNNING", n.Type, clip(title, 24))
	if lastAt > 0 {
		seg += "(" + dur(now.Sub(time.UnixMilli(lastAt))) + ")"
	}
	return seg
}

// flowSignals is the L1 layer: the rule-based warnings that carry most of a
// view's value. Still no LLM, still no invented numbers.
func flowSignals(in FlowRunInput, now time.Time, lens Lens) []string {
	var out []string

	if e := strings.TrimSpace(in.Run.Error); e != "" {
		out = append(out, "⚠ hata: "+clip(e, 180))
	}
	if in.State.WaitingAt != "" {
		out = append(out, fmt.Sprintf("⏸ await-input node:%s — %s bekliyor",
			in.State.WaitingAt, age(in.Run.UpdatedAt, now)))
	}
	if lens == LensErrors {
		// The errors lens deliberately stops here: failures only, nothing else.
		return out
	}

	if in.State.Iter > 0 {
		out = append(out, fmt.Sprintf("↻ loop iterasyon %d", in.State.Iter))
	}
	if n := countSpawned(in.State); n > 0 {
		out = append(out, fmt.Sprintf("⑃ %d async çocuk koşu bekliyor (spawn)", n))
	}
	if in.Run.Status == db.FlowRunning && in.Run.UpdatedAt > 0 {
		if idle := now.Sub(time.UnixMilli(in.Run.UpdatedAt)); idle > 2*time.Minute {
			out = append(out, fmt.Sprintf("⚠ %s'dir aynı node'da — takılmış olabilir", dur(idle)))
		}
	}
	if in.Run.SessionID != "" && lens == LensHealth {
		out = append(out, "↳ transkript session:"+in.Run.SessionID)
	}
	return out
}

// countSpawned totals the async child runs a spawn node launched and no join has
// collected yet.
func countSpawned(st orchestration.State) int {
	n := 0
	for _, ids := range st.Spawned {
		n += len(ids)
	}
	return n
}

// flowDetail is the LevelFull tail: one line per executed node with a clipped
// output, newest last. Returns the text and how many entries it dropped.
func flowDetail(in FlowRunInput, now time.Time) (string, int) {
	trace := in.State.Trace
	dropped := 0
	if len(trace) > fullDetailMax {
		dropped = len(trace) - fullDetailMax
		trace = trace[len(trace)-fullDetailMax:]
	}
	var l lines
	for _, t := range trace {
		l.add("%s %-10s %-24s %s", hhmmss(time.UnixMilli(t.At)), t.Type, clip(t.Title, 24), clip(t.Output, 110))
	}
	if l.empty() {
		return "", dropped
	}
	return l.String(), dropped
}

// flowHandles points at the parts the projection left out.
func flowHandles(in FlowRunInput, level Level) []Handle {
	var hs []Handle
	if level != LevelFull {
		hs = append(hs, Handle{
			Label: "tüm node çıktıları",
			Ref:   Ref{Kind: KindFlowRun, ID: in.Run.ID},
			Level: LevelFull,
		})
	}
	if in.Run.Status == db.FlowFailure && len(in.State.Trace) > 0 {
		last := in.State.Trace[len(in.State.Trace)-1]
		hs = append(hs, Handle{
			Label: "başarısız node: " + last.NodeID,
			Ref:   Ref{Kind: KindFlowRun, ID: in.Run.ID, Sub: last.NodeID},
			Level: LevelFull,
		})
	}
	return hs
}

// parallelParents maps each parallel child node id to its parent parallel node,
// so the chain renderer can fold a fan-out instead of listing its children.
func parallelParents(g orchestration.Graph) map[string]string {
	m := map[string]string{}
	for _, n := range g.Nodes {
		if n.Type != orchestration.NodeParallel {
			continue
		}
		for _, c := range n.Parallel {
			m[c] = n.ID
		}
	}
	return m
}

// wrapChain joins segments with arrows, breaking to a continuation line before
// the width limit so a long run stays readable in a terminal and in the panel.
func wrapChain(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	width := 0
	for i, s := range segs {
		if i > 0 {
			if width+len(s)+3 > chainLineWidth {
				b.WriteString("\n" + chainContinuation)
				width = len(chainContinuation)
			} else {
				b.WriteString(" → ")
				width += 3
			}
		}
		b.WriteString(s)
		width += len([]rune(s))
	}
	return b.String()
}
