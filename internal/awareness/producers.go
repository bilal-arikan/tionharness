package awareness

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/progress"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// Producer renders one section of a moment. A producer that has nothing to say
// returns a Section with empty Text; an error is logged by the service and the
// section is skipped — one broken producer must never cost the whole brief.
type Producer interface {
	Key() string
	Moments() []Moment
	Produce(ctx context.Context, in Input) (Section, error)
}

// BriefProducers are the built-in session-start sections, in delivery order.
func BriefProducers() []Producer {
	return []Producer{
		briefIntro{},
		workspaceCard{},
		openLoops{moment: MomentBrief},
		resumedProgress{},
		notesRelevant{},
		recentDigests{},
		recentSessions{},
	}
}

// TurnProducers are the built-in per-turn sections the runtime appends to the
// api-side ones (clock, identity, recap …).
func TurnProducers() []Producer {
	return []Producer{
		todoChecklist{},
		sessionArtifacts{},
		pulse{},
	}
}

// ---- brief --------------------------------------------------------------------

type briefIntro struct{}

func (briefIntro) Key() string       { return "intro" }
func (briefIntro) Moments() []Moment { return []Moment{MomentBrief} }
func (briefIntro) Produce(_ context.Context, in Input) (Section, error) {
	var b strings.Builder
	b.WriteString("# Session briefing\n")
	fmt.Fprintf(&b, "Composed when this session started (%s) and frozen for the session; the per-turn context below carries live changes. ",
		in.now().Format("2006-01-02 15:04"))
	b.WriteString("Current state on demand: get_view (workspace, board, sessions, agents …). ")
	b.WriteString("Memory: note_search finds notes, note_expand shows a note's links; remember records a durable lesson or decision, record_work files what you did; note_correct fixes a note without erasing the record.")
	return Section{Key: "intro", Text: b.String(), Priority: PriorityPinned}, nil
}

type workspaceCard struct{}

func (workspaceCard) Key() string       { return "workspace" }
func (workspaceCard) Moments() []Moment { return []Moment{MomentBrief} }
func (workspaceCard) Produce(ctx context.Context, in Input) (Section, error) {
	wsIn, err := loadWorkspaceInput(ctx, in)
	if err != nil {
		return Section{}, err
	}
	card, err := view.ProjectWorkspace(wsIn, view.LevelCard)
	if err != nil {
		return Section{}, err
	}
	tiny, _ := view.ProjectWorkspace(wsIn, view.LevelTiny)
	return Section{
		Key:      "workspace",
		Text:     "## Workspace now (same projection as get_view workspace)\n" + card.Text(),
		Pointer:  "## Workspace now\n" + tiny.Text() + "\n(details: get_view workspace)",
		Priority: 2,
	}, nil
}

// loadWorkspaceInput gathers the roll-up inputs the way the Explorer does, so
// the brief and the UI show the same numbers.
func loadWorkspaceInput(ctx context.Context, in Input) (view.WorkspaceInput, error) {
	agents, err := in.Store.ListAgents(ctx)
	if err != nil {
		return view.WorkspaceInput{}, err
	}
	sessions, err := in.Store.ListSessions(ctx, "")
	if err != nil {
		return view.WorkspaceInput{}, err
	}
	tasks, err := in.Store.ListTasks(ctx)
	if err != nil {
		return view.WorkspaceInput{}, err
	}
	// Mirrors view.Projector.loadWorkspace exactly (same lists, 0 = no run
	// limit), so the brief's numbers equal the Explorer's and get_view's.
	runs, _ := in.Store.ListFlowRuns(ctx, "", 0)
	scheds, _ := in.Store.ListSchedules(ctx)
	asks, _ := in.Store.ListWaitingSessionAsks(ctx)
	return view.WorkspaceInput{
		Agents: agents, Sessions: sessions, Tasks: tasks, FlowRuns: runs, Schedules: scheds,
		WaitingAsks: asks, Name: in.WorkspaceName, Now: in.now(),
	}, nil
}

// openLoops lists what is waiting on someone: unanswered questions, failed runs,
// stale in-progress cards, stuck or blocked sessions. Pinned-adjacent (priority
// 1) because an unanswered question is the one thing a briefing must not lose.
type openLoops struct{ moment Moment }

func (o openLoops) Key() string       { return "open-loops" }
func (o openLoops) Moments() []Moment { return []Moment{o.moment} }
func (o openLoops) Produce(ctx context.Context, in Input) (Section, error) {
	loops, counts := collectOpenLoops(ctx, in)
	if len(loops) == 0 {
		return Section{}, nil
	}
	var b strings.Builder
	b.WriteString("## Open loops (waiting on someone)\n")
	for _, l := range loops {
		b.WriteString("- " + l + "\n")
	}
	return Section{
		Key:      "open-loops",
		Text:     strings.TrimSpace(b.String()),
		Pointer:  "## Open loops\n" + counts + " (details: get_view workspace, list_sessions, get_view board)",
		Priority: 1,
	}, nil
}

// collectOpenLoops is shared by the brief section, the pulse and the digest.
// It returns the itemised lines and a one-line count summary.
func collectOpenLoops(ctx context.Context, in Input) (lines []string, counts string) {
	return renderOpenLoops(CollectOpenLoops(ctx, in.Store, in.Settings, in.now(), in.Session.ID), in.Settings.StaleCardDays)
}

type resumedProgress struct{}

func (resumedProgress) Key() string       { return "progress" }
func (resumedProgress) Moments() []Moment { return []Moment{MomentBrief} }
func (resumedProgress) Produce(_ context.Context, in Input) (Section, error) {
	if !in.ProgressResume || in.ProgressDir == "" {
		return Section{}, nil
	}
	rec, ok, err := progress.Load(in.ProgressDir)
	if err != nil || !ok {
		return Section{}, err
	}
	text := RenderResumedProgress(rec)
	if text == "" {
		return Section{}, nil
	}
	return Section{Key: "progress", Text: text, Priority: PriorityPinned}, nil
}

// RenderResumedProgress formats a previous session's persisted checklist as a
// resume hint. "" for an empty or fully-completed list.
func RenderResumedProgress(rec progress.Record) string {
	if len(rec.Todos) == 0 {
		return ""
	}
	allDone := true
	for _, t := range rec.Todos {
		if t.Status != "completed" {
			allDone = false
			break
		}
	}
	if allDone {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Resumed progress (from a previous session)\n")
	b.WriteString("This checklist was persisted by an earlier session working on this project. Continue from where it left off; keep it current by calling todo_write as you start and finish items.\n")
	for _, t := range rec.Todos {
		mark := " "
		switch t.Status {
		case "completed":
			mark = "x"
		case "in_progress":
			mark = "~"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, t.Content)
	}
	return strings.TrimSpace(b.String())
}

// notesRelevant serves the notes whose declared reach covers this session,
// rule-ranked (lessons and gotchas first, this agent's own notes first, then
// recency) and optionally re-ranked by the brief-relevance authority.
type notesRelevant struct{}

func (notesRelevant) Key() string       { return "notes" }
func (notesRelevant) Moments() []Moment { return []Moment{MomentBrief} }
func (notesRelevant) Produce(ctx context.Context, in Input) (Section, error) {
	if in.Notes == nil || in.Settings.NoteCount <= 0 {
		return Section{}, nil
	}
	reader := &notes.Reader{AgentID: in.Agent.ID, Project: in.Cwd}
	all := in.Notes.List(notes.Filter{Reader: reader})
	if len(all) == 0 {
		return Section{}, nil
	}
	ranked := RankNotes(all, in.Agent.ID)
	limit := in.Settings.NoteCount
	chosen := ranked
	if in.RankNotes != nil {
		if picked := in.RankNotes(ctx, ranked, limit); picked != nil {
			chosen = picked
		}
	}
	if len(chosen) > limit {
		chosen = chosen[:limit]
	}
	now := in.now()
	var b strings.Builder
	b.WriteString("## Memory (notes that reach this session)\n")
	for _, n := range chosen {
		b.WriteString(n.Line(now) + "\n")
	}
	fmt.Fprintf(&b, "%d note(s) reach this session in total; note_search <query> finds the rest, note_expand <id> shows a note's links and corrections.", len(all))
	return Section{
		Key:      "notes",
		Text:     strings.TrimSpace(b.String()),
		Pointer:  fmt.Sprintf("## Memory\n%d note(s) reach this session (not inlined); note_search <query> finds them.", len(all)),
		Priority: 3,
	}, nil
}

// RankNotes is the rule-based order: actionable kinds first, the reader's own
// notes before others', verified before inferred, newest first.
func RankNotes(all []notes.Note, agentID string) []notes.Note {
	out := append([]notes.Note(nil), all...)
	kindRank := map[notes.Kind]int{
		notes.KindLesson: 0, notes.KindGotcha: 0, notes.KindDecision: 1, notes.KindPattern: 2,
		notes.KindWork: 3, notes.KindProfile: 4, notes.KindReference: 5,
	}
	confRank := map[notes.Confidence]int{notes.ConfidenceVerified: 0, notes.ConfidenceInferred: 1, notes.ConfidenceUnverified: 2}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ka, kb := kindRank[a.Kind], kindRank[b.Kind]; ka != kb {
			return ka < kb
		}
		if oa, ob := a.SourceAgent == agentID && agentID != "", b.SourceAgent == agentID && agentID != ""; oa != ob {
			return oa
		}
		if ca, cb := confRank[a.Confidence], confRank[b.Confidence]; ca != cb {
			return ca < cb
		}
		return a.Updated > b.Updated
	})
	return out
}

type recentDigests struct{}

func (recentDigests) Key() string       { return "recent-work" }
func (recentDigests) Moments() []Moment { return []Moment{MomentBrief} }
func (recentDigests) Produce(_ context.Context, in Input) (Section, error) {
	if in.Digests == nil || in.Settings.RecentDigests <= 0 {
		return Section{}, nil
	}
	entries := in.Digests.RecentDigests(in.Settings.RecentDigests, in.Session.ID)
	if len(entries) == 0 {
		return Section{}, nil
	}
	now := in.now().Unix()
	var b strings.Builder
	b.WriteString("## Recently finished work in this workspace (session digests)\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- %s · %s ago · %s\n", e.SessionID, notes.Age(now-e.At), e.Line)
	}
	b.WriteString("Full digest of a session: get_view session:<id>; its transcript: conversation_search.")
	return Section{
		Key:      "recent-work",
		Text:     strings.TrimSpace(b.String()),
		Pointer:  fmt.Sprintf("## Recently finished work\n%d recent session digest(s) (not inlined); list_sessions, get_view session:<id>.", len(entries)),
		Priority: 4,
	}, nil
}

type recentSessions struct{}

func (recentSessions) Key() string       { return "sessions" }
func (recentSessions) Moments() []Moment { return []Moment{MomentBrief} }
func (recentSessions) Produce(ctx context.Context, in Input) (Section, error) {
	if in.Settings.RecentSessions <= 0 {
		return Section{}, nil
	}
	sessions, err := in.Store.ListSessions(ctx, "")
	if err != nil {
		return Section{}, err
	}
	now := in.now().Unix()
	var lines []string
	total := 0
	for _, s := range sessions {
		if s.ID == in.Session.ID || s.Kind != "chat" {
			continue
		}
		total++
		if len(lines) < in.Settings.RecentSessions {
			lines = append(lines, formatSessionLine(s, now, in.Running[s.ID]))
		}
	}
	if total == 0 {
		return Section{}, nil
	}
	var b strings.Builder
	b.WriteString("## Other sessions in this workspace\n")
	b.WriteString("Situational awareness only; list_sessions pages through all of them, conversation_search reads their transcripts.\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	if total > len(lines) {
		fmt.Fprintf(&b, "… and %d more", total-len(lines))
	}
	return Section{
		Key:      "sessions",
		Text:     strings.TrimSpace(b.String()),
		Pointer:  fmt.Sprintf("## Other sessions\n%d other chat session(s) in this workspace (not listed); list_sessions.", total),
		Priority: 5,
	}, nil
}

// formatSessionLine renders one session as a compact bullet.
func formatSessionLine(s db.Session, now int64, running bool) string {
	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = "(untitled)"
	}
	line := fmt.Sprintf("- %s %q · %d msg · %s", s.ID, clip(title, 60), s.MessageCount, notes.Age(now-s.UpdatedAt))
	if running {
		line += " · running"
	} else if s.State == "archived" {
		line += " · archived"
	}
	if snip := firstLine(s.Summary, 120); snip != "" {
		line += " — " + snip
	}
	return line
}

// ---- turn ---------------------------------------------------------------------

// todoChecklist surfaces the session's live checklist so the agent keeps
// tracking it after the original todo_write scrolled out of context. Pinned:
// open tasks are never traded for plumbing.
type todoChecklist struct{}

func (todoChecklist) Key() string       { return "todo" }
func (todoChecklist) Moments() []Moment { return []Moment{MomentTurn} }
func (todoChecklist) Produce(ctx context.Context, in Input) (Section, error) {
	if in.Session.ID == "" {
		return Section{}, nil
	}
	const tail = 64
	msgs, from, err := in.Store.ListMessagesTail(ctx, in.Session.ID, tail)
	if err != nil {
		return Section{}, err
	}
	roll := view.LatestTodos(msgs)
	if roll.Empty() && from > 0 {
		if all, err := in.Store.ListMessages(ctx, in.Session.ID); err == nil {
			roll = view.LatestTodos(all)
		}
	}
	if !roll.Empty() {
		if roll.AllDone() {
			return Section{}, nil
		}
		text := "## Active todo list (this session)\n" +
			"This is the checklist you are tracking with the todo_write tool. It persists here even if the original message has scrolled out of context. Keep it current: flip statuses with the compact form todo_write {\"set\":{\"<index>\":\"<status>\"}} using the 1-based indices below.\n" +
			roll.RenderChecklist()
		return Section{Key: "todo", Text: text, Priority: PriorityPinned, Volatile: true}, nil
	}
	// No checklist of its own yet and not the first turn (the brief already
	// carried the resumed file then): offer the durable progress file, so a
	// session continuing after a context reset still sees it.
	if in.Fresh || !in.ProgressResume || in.ProgressDir == "" {
		return Section{}, nil
	}
	rec, ok, err := progress.Load(in.ProgressDir)
	if err != nil || !ok {
		return Section{}, err
	}
	if text := RenderResumedProgress(rec); text != "" {
		return Section{Key: "todo", Text: text, Priority: PriorityPinned, Volatile: true}, nil
	}
	return Section{}, nil
}

type sessionArtifacts struct{}

func (sessionArtifacts) Key() string       { return "artifacts" }
func (sessionArtifacts) Moments() []Moment { return []Moment{MomentTurn} }
func (sessionArtifacts) Produce(ctx context.Context, in Input) (Section, error) {
	if in.Session.ID == "" {
		return Section{}, nil
	}
	arts, err := in.Store.ListArtifacts(ctx, in.Session.ID)
	if err != nil || len(arts) == 0 {
		return Section{}, err
	}
	var b strings.Builder
	b.WriteString("## Artifacts in this session\n")
	b.WriteString("You have already created these artifacts. To revise one, call update_artifact with its id instead of creating a duplicate.\n")
	const max = 30
	for i, a := range arts {
		if i >= max {
			fmt.Fprintf(&b, "- … and %d more\n", len(arts)-max)
			break
		}
		fmt.Fprintf(&b, "- id=%s · %q · kind=%s", a.ID, a.Title, a.Kind)
		if a.Language != "" {
			b.WriteString("/" + a.Language)
		}
		b.WriteString("\n")
	}
	return Section{
		Key:      "artifacts",
		Text:     strings.TrimSpace(b.String()),
		Pointer:  fmt.Sprintf("## Artifacts in this session\n%d artifact(s) exist; list_artifacts shows them, update_artifact revises one by id.", len(arts)),
		Priority: 6,
		Volatile: true,
	}, nil
}

// pulse is the one-line workspace heartbeat. It renders only the non-zero
// facts and nothing at all when the workspace is quiet; the service drops it
// when it did not change since the last turn.
type pulse struct{}

func (pulse) Key() string       { return "pulse" }
func (pulse) Moments() []Moment { return []Moment{MomentTurn} }
func (pulse) Produce(ctx context.Context, in Input) (Section, error) {
	text, urgent := BuildPulse(ctx, in)
	if text == "" {
		return Section{}, nil
	}
	return Section{Key: "pulse", Text: text, Priority: 2, Volatile: true, Urgent: urgent}, nil
}

// BuildPulse renders the heartbeat line and the rule-based urgency flag:
// urgent when a human answer is pending, a session is stuck or a run failed.
func BuildPulse(ctx context.Context, in Input) (text string, urgent bool) {
	var parts []string
	running := 0
	for id, on := range in.Running {
		if on && id != in.Session.ID {
			running++
		}
	}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d other session(s) running", running))
	}
	_, counts := collectOpenLoops(ctx, in)
	if counts != "" {
		parts = append(parts, counts)
		urgent = strings.Contains(counts, "waiting") || strings.Contains(counts, "stuck") || strings.Contains(counts, "failed")
	}
	if len(parts) == 0 {
		return "", false
	}
	line := "[workspace pulse · " + in.now().Format("15:04") + "] " + strings.Join(parts, " · ") + " — details: get_view workspace"
	if budget := in.Settings.PulseBudgetBytes; budget > 0 && len(line) > budget {
		line = line[:budget-1] + "…"
	}
	return line, urgent
}

// ---- helpers --------------------------------------------------------------------

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func joinCapped(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" +%d", len(items)-max)
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

func firstLine(s string, n int) string {
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return clip(ln, n)
		}
	}
	return ""
}
