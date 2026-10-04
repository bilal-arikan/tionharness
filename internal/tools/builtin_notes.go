package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// NotesBridge is what the memory tools need from the runtime: the store, the
// caller's project root (for the reach rule and project-scoped notes) and the
// supersede judgement (rule-based, optionally backed by the note-supersede
// decision authority). It lives on the runtime side so the tools package never
// imports internal/agent.
type NotesBridge interface {
	Store() *notes.Store
	// Project returns the calling session's working directory ("" when none).
	Project(ctx context.Context) string
	// Supersedes decides whether candidate corrects existing (same subject,
	// newer or contradicting claim). The tool only asks for pairs the lexical
	// rule already finds similar.
	Supersedes(ctx context.Context, candidate, existing notes.Note) bool
}

// NewNotesTools builds the memory tool family for one agent. agentID is bound at
// registry build time (the registry is per agent); the session comes from the
// call context (CurrentSessionID).
func NewNotesTools(bridge NotesBridge, agentID string) []Tool {
	return []Tool{
		RememberTool{bridge: bridge, agentID: agentID},
		RecordWorkTool{bridge: bridge, agentID: agentID},
		NoteSearchTool{bridge: bridge, agentID: agentID},
		NoteExpandTool{bridge: bridge},
		NoteCorrectTool{bridge: bridge, agentID: agentID},
	}
}

// NoteToolNames lists the memory tools (tier defaults, docs).
var NoteToolNames = []string{"remember", "record_work", "note_search", "note_expand", "note_correct"}

// ---- remember ---------------------------------------------------------------

// RememberTool stores one durable lesson, decision or fact as a note.
type RememberTool struct {
	bridge  NotesBridge
	agentID string
}

func (RememberTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "remember",
		Description: "Record a DURABLE memory note for future sessions: a lesson or gotcha that cost time, a decision and why, " +
			"a pattern you keep seeing, a profile of a person/system, a reference. The test is whether it would help a " +
			"later session (yours or another agent's) — a transient status is NOT a note; use record_work for what " +
			"happened in this session. Declare the reach honestly: scope \"workspace\" reaches every session here, " +
			"\"agent\" only you, \"project\" only sessions in this working directory. Set confidence honestly and give " +
			"verification when you claim \"verified\". A note that matches an existing one on the same subject is filed " +
			"as its CORRECTION (the old note is kept, marked superseded) instead of a duplicate. " +
			"[[Note title]] or [[NOTE12]] in the body links notes.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "kind": { "type": "string", "enum": ["lesson","decision","gotcha","pattern","profile","reference"], "description": "What the note records." },
    "title": { "type": "string", "description": "Short, specific, searchable title (it is also the wikilink target)." },
    "body": { "type": "string", "description": "The note itself in Markdown: the claim, the why, how to apply it. Split rather than exceed ~20KB." },
    "scope": { "type": "string", "enum": ["workspace","agent","project"], "description": "Who this reaches. Default workspace." },
    "confidence": { "type": "string", "enum": ["verified","inferred","unverified"], "description": "Default inferred." },
    "verification": { "type": "string", "description": "How you verified it (required for confidence=verified)." },
    "tags": { "type": "array", "items": { "type": "string" } },
    "supersedes": { "type": "string", "description": "Optional id of the note this one corrects (otherwise similar notes are detected)." }
  },
  "required": ["kind","title","body"],
  "additionalProperties": false
}`),
	}
}

type rememberInput struct {
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Scope        string   `json:"scope"`
	Confidence   string   `json:"confidence"`
	Verification string   `json:"verification"`
	Tags         []string `json:"tags"`
	Supersedes   string   `json:"supersedes"`
}

func (t RememberTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[rememberInput]("remember", input)
	if err != nil {
		return "", err
	}
	store := t.bridge.Store()
	if store == nil {
		return "", fmt.Errorf("the notes store is unavailable in this workspace")
	}
	kind := notes.Kind(strings.ToLower(strings.TrimSpace(in.Kind)))
	if kind == notes.KindWork {
		return "", fmt.Errorf("kind \"work\" is written with record_work, not remember")
	}
	n := notes.Note{
		Kind: kind, Title: in.Title, Body: in.Body,
		Scope:        notes.Scope(defaultStr(in.Scope, string(notes.ScopeWorkspace))),
		Confidence:   notes.Confidence(defaultStr(in.Confidence, string(notes.ConfidenceInferred))),
		Verification: in.Verification, Tags: in.Tags,
		Source: notes.SourceAgent, SourceAgent: t.agentID, SourceSession: CurrentSessionID(ctx),
	}
	applyReach(ctx, t.bridge, t.agentID, &n)
	if err := n.Validate(); err != nil {
		return "", fmt.Errorf("remember: %w", err)
	}
	var out notes.Note
	superseded := ""
	switch {
	case strings.TrimSpace(in.Supersedes) != "":
		old, ok := store.Get(in.Supersedes)
		if !ok {
			return "", fmt.Errorf("remember: supersedes %q: note not found", in.Supersedes)
		}
		out, err = store.Correct(old.ID, n)
		superseded = old.ID
	default:
		if old, ok := similarNote(store, n); ok && t.bridge.Supersedes(ctx, n, old) {
			out, err = store.Correct(old.ID, n)
			superseded = old.ID
		} else {
			out, err = store.Put(n)
		}
	}
	if err != nil {
		return "", fmt.Errorf("remember: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "recorded %s [%s·%s] %q · reach: %s", out.ID, out.Kind, out.Confidence, out.Title, reachLabel(out))
	if superseded != "" {
		fmt.Fprintf(&b, " · supersedes %s (kept as a dated record)", superseded)
	}
	if out.Occurrences > 1 {
		fmt.Fprintf(&b, " · merged into an existing note of the same shape (seen %d×)", out.Occurrences)
	}
	if w := store.LinkWarnings(out); len(w) > 0 {
		fmt.Fprintf(&b, "\nunresolved wikilinks (fine if you will write them later): %s", strings.Join(w, ", "))
	}
	return b.String(), nil
}

// applyReach fills the scope's list from the caller's identity: an agent note
// reaches the writer; a project note reaches the session's working directory.
// The runtime, not the model, supplies both — a session cannot claim to be a
// project it is not.
func applyReach(ctx context.Context, bridge NotesBridge, agentID string, n *notes.Note) {
	switch n.Scope {
	case notes.ScopeAgent:
		if agentID != "" {
			n.Agents = []string{agentID}
		}
	case notes.ScopeProject:
		if p := strings.TrimSpace(bridge.Project(ctx)); p != "" {
			n.Projects = []string{p}
		}
	}
}

// similarNote finds an active note of the same kind whose title shares most of
// its words with the candidate — the lexical half of the supersede rule.
func similarNote(store *notes.Store, n notes.Note) (notes.Note, bool) {
	var best notes.Note
	bestScore := 0.0
	for _, other := range store.List(notes.Filter{Kinds: []notes.Kind{n.Kind}}) {
		if s := TitleSimilarity(n.Title, other.Title); s > bestScore {
			best, bestScore = other, s
		}
	}
	if bestScore >= 0.5 {
		return best, true
	}
	return notes.Note{}, false
}

// TitleSimilarity is the Jaccard overlap of the two titles' word sets (short
// words dropped), in [0,1]. Exported for the runtime's supersede rule so both
// sides use one measure.
func TitleSimilarity(a, b string) float64 {
	wa, wb := wordSet(a), wordSet(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	return float64(inter) / float64(union)
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, `"'.,;:!?()[]{}`)
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

func reachLabel(n notes.Note) string {
	switch n.Scope {
	case notes.ScopeAgent:
		return "agent " + strings.Join(n.Agents, ",")
	case notes.ScopeProject:
		return "project " + strings.Join(n.Projects, ",")
	}
	return "workspace"
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

// ---- record_work ------------------------------------------------------------

// RecordWorkTool files what happened in this session as a work note.
type RecordWorkTool struct {
	bridge  NotesBridge
	agentID string
}

func (RecordWorkTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "record_work",
		Description: "File what happened in THIS session as a work note for the next session: what changed, the " +
			"decisions and the alternatives rejected, what was learned, what is still open, how it was verified. " +
			"Use it at a natural stopping point (a feature landed, a hand-off, before a long pause). A lesson that " +
			"would help a DIFFERENT project is a `remember`, not a work note; do both when both are true. Scope " +
			"defaults to the session's project when it has a working directory, else the workspace.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "What this piece of work was, in a few words." },
    "summary": { "type": "string", "description": "What changed and why (Markdown)." },
    "decisions": { "type": "array", "items": { "type": "string" }, "description": "Decisions made, each with the alternative rejected." },
    "learned": { "type": "array", "items": { "type": "string" }, "description": "What was learned (also consider remember for anything general)." },
    "open": { "type": "array", "items": { "type": "string" }, "description": "What is still open / next steps." },
    "verification": { "type": "string", "description": "How the work was verified (tests run, checks made)." },
    "scope": { "type": "string", "enum": ["project","workspace","agent"] },
    "tags": { "type": "array", "items": { "type": "string" } }
  },
  "required": ["title","summary"],
  "additionalProperties": false
}`),
	}
}

type recordWorkInput struct {
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Decisions    []string `json:"decisions"`
	Learned      []string `json:"learned"`
	Open         []string `json:"open"`
	Verification string   `json:"verification"`
	Scope        string   `json:"scope"`
	Tags         []string `json:"tags"`
}

func (t RecordWorkTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[recordWorkInput]("record_work", input)
	if err != nil {
		return "", err
	}
	store := t.bridge.Store()
	if store == nil {
		return "", fmt.Errorf("the notes store is unavailable in this workspace")
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(in.Summary) + "\n")
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n## " + title + "\n")
		for _, it := range items {
			if it = strings.TrimSpace(it); it != "" {
				b.WriteString("- " + it + "\n")
			}
		}
	}
	section("Decisions", in.Decisions)
	section("Learned", in.Learned)
	section("Open", in.Open)
	if v := strings.TrimSpace(in.Verification); v != "" {
		b.WriteString("\n## Verification\n" + v + "\n")
	}
	scope := notes.Scope(strings.TrimSpace(in.Scope))
	if scope == "" {
		if strings.TrimSpace(t.bridge.Project(ctx)) != "" {
			scope = notes.ScopeProject
		} else {
			scope = notes.ScopeWorkspace
		}
	}
	conf := notes.ConfidenceInferred
	if strings.TrimSpace(in.Verification) != "" {
		conf = notes.ConfidenceVerified
	}
	n := notes.Note{
		Kind: notes.KindWork, Title: in.Title, Body: b.String(), Scope: scope, Confidence: conf,
		Verification: in.Verification, Tags: in.Tags,
		Source: notes.SourceWork, SourceAgent: t.agentID, SourceSession: CurrentSessionID(ctx),
	}
	applyReach(ctx, t.bridge, t.agentID, &n)
	if n.Scope == notes.ScopeProject && len(n.Projects) == 0 {
		n.Scope = notes.ScopeWorkspace
	}
	out, err := store.Put(n)
	if err != nil {
		return "", fmt.Errorf("record_work: %w", err)
	}
	return fmt.Sprintf("filed %s [work·%s] %q · reach: %s · %d open item(s)", out.ID, out.Confidence, out.Title, reachLabel(out), len(in.Open)), nil
}

// ---- note_search ------------------------------------------------------------

// NoteSearchTool finds notes that reach the caller.
type NoteSearchTool struct {
	bridge  NotesBridge
	agentID string
}

func (NoteSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "note_search",
		Description: "Search the workspace's memory notes (lessons, decisions, gotchas, work records, profiles) that " +
			"reach this session. Every query word must match (title, body or tags). Returns the best matches with " +
			"their ids; note_expand <id> reads one in full with its links and corrections. Superseded notes are " +
			"shown only with their correction; archived and private notes are never shown. An empty result on a " +
			"fresh workspace is normal — the store fills as sessions record.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Words to match (AND)." },
    "kind": { "type": "string", "enum": ["lesson","decision","work","gotcha","pattern","profile","reference"], "description": "Optional kind filter." },
    "limit": { "type": "integer", "description": "Max results (default 8)." },
    "all": { "type": "boolean", "description": "Ignore reach and search every active note in the workspace (default false)." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
	}
}

type noteSearchInput struct {
	Query string `json:"query"`
	Kind  string `json:"kind"`
	Limit int    `json:"limit"`
	All   bool   `json:"all"`
}

func (t NoteSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[noteSearchInput]("note_search", input)
	if err != nil {
		return "", err
	}
	store := t.bridge.Store()
	if store == nil {
		return "", fmt.Errorf("the notes store is unavailable in this workspace")
	}
	f := notes.Filter{}
	if k := strings.TrimSpace(in.Kind); k != "" {
		f.Kinds = []notes.Kind{notes.Kind(k)}
	}
	if !in.All {
		f.Reader = &notes.Reader{AgentID: t.agentID, Project: t.bridge.Project(ctx)}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	hits := store.Search(in.Query, f, limit)
	if len(hits) == 0 {
		total := store.Count(notes.Filter{})
		if total == 0 {
			return "no notes match; the workspace memory is empty so far (remember / record_work fill it)", nil
		}
		return fmt.Sprintf("no notes match %q among the %d note(s) that reach you (try fewer words, or all=true to search every note)", in.Query, store.Count(f)), nil
	}
	now := time.Now()
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q:\n", len(hits), in.Query)
	for _, h := range hits {
		b.WriteString(h.Note.Line(now) + "\n")
		if h.Snippet != "" && !strings.Contains(h.Note.Line(now), h.Snippet) {
			b.WriteString("    " + h.Snippet + "\n")
		}
	}
	b.WriteString("note_expand <id> reads a note in full.")
	return b.String(), nil
}

// ---- note_expand ------------------------------------------------------------

// NoteExpandTool reads one note in full with its neighbourhood.
type NoteExpandTool struct{ bridge NotesBridge }

func (NoteExpandTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "note_expand",
		Description: "Read one memory note in full by id or title: its frontmatter (kind, confidence, reach, " +
			"provenance), body, the notes it links to, the notes that link to it, and its correction chain " +
			"(what it replaced / what replaced it). A superseded note is shown with the note that corrected it.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": { "id": { "type": "string", "description": "Note id (NOTE12) or exact title." } },
  "required": ["id"],
  "additionalProperties": false
}`),
	}
}

func (t NoteExpandTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[struct {
		ID string `json:"id"`
	}]("note_expand", input)
	if err != nil {
		return "", err
	}
	store := t.bridge.Store()
	if store == nil {
		return "", fmt.Errorf("the notes store is unavailable in this workspace")
	}
	ex, err := store.Expand(in.ID)
	if err != nil {
		return "", fmt.Errorf("note_expand: %w", err)
	}
	if ex.Note.Private {
		return "", fmt.Errorf("note_expand: %s is private", ex.Note.ID)
	}
	return RenderExpansion(ex, time.Now()), nil
}

// RenderExpansion formats a note with its neighbourhood for the model.
func RenderExpansion(ex notes.Expansion, now time.Time) string {
	n := ex.Note
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s\n", n.ID, n.Title)
	fmt.Fprintf(&b, "kind: %s · confidence: %s · reach: %s", n.Kind, n.Confidence, reachLabel(n))
	if n.Verification != "" {
		b.WriteString(" · verified by: " + n.Verification)
	}
	b.WriteString("\n")
	meta := []string{}
	if n.Source != "" {
		meta = append(meta, "source "+n.Source)
	}
	if n.SourceAgent != "" {
		meta = append(meta, "by "+n.SourceAgent)
	}
	if n.SourceSession != "" {
		meta = append(meta, "in "+n.SourceSession)
	}
	if n.Updated > 0 {
		meta = append(meta, "updated "+notes.Age(now.Unix()-n.Updated)+" ago")
	}
	if n.Occurrences > 1 {
		meta = append(meta, fmt.Sprintf("seen %d×", n.Occurrences))
	}
	if len(n.Tags) > 0 {
		meta = append(meta, "tags "+strings.Join(n.Tags, ","))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " · ") + "\n")
	}
	if n.Archived {
		b.WriteString("status: ARCHIVED\n")
	}
	if n.Retired() {
		fmt.Fprintf(&b, "status: SUPERSEDED by %s — the text below is the dated record, not the current claim\n", n.SupersededBy)
	}
	b.WriteString("\n" + strings.TrimSpace(n.Body) + "\n")
	list := func(title string, ns []notes.Note) {
		if len(ns) == 0 {
			return
		}
		b.WriteString("\n" + title + ":\n")
		for _, x := range ns {
			fmt.Fprintf(&b, "- %s [%s] %s\n", x.ID, x.Kind, x.Title)
		}
	}
	list("links to", ex.Links)
	if len(ex.Unresolved) > 0 {
		b.WriteString("\nunresolved links: " + strings.Join(ex.Unresolved, ", ") + "\n")
	}
	list("linked from", ex.Backlinks)
	list("corrects (older)", ex.Predecessors)
	list("corrected by (newer)", ex.Successors)
	return strings.TrimRight(b.String(), "\n")
}

// ---- note_correct -----------------------------------------------------------

// NoteCorrectTool files a correction: a new note that supersedes an old one.
type NoteCorrectTool struct {
	bridge  NotesBridge
	agentID string
}

func (NoteCorrectTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "note_correct",
		Description: "Correct a memory note without erasing the record: files a NEW note that supersedes the old " +
			"one (the old text stays as a dated record, marked superseded, and is served only next to this " +
			"correction). Use when a note is wrong or outdated. Fields you omit are copied from the old note.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "The note to correct." },
    "body": { "type": "string", "description": "The corrected text." },
    "title": { "type": "string" },
    "confidence": { "type": "string", "enum": ["verified","inferred","unverified"] },
    "verification": { "type": "string" },
    "reason": { "type": "string", "description": "One line on what was wrong (appended to the correction)." }
  },
  "required": ["id","body"],
  "additionalProperties": false
}`),
	}
}

type noteCorrectInput struct {
	ID           string `json:"id"`
	Body         string `json:"body"`
	Title        string `json:"title"`
	Confidence   string `json:"confidence"`
	Verification string `json:"verification"`
	Reason       string `json:"reason"`
}

func (t NoteCorrectTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[noteCorrectInput]("note_correct", input)
	if err != nil {
		return "", err
	}
	store := t.bridge.Store()
	if store == nil {
		return "", fmt.Errorf("the notes store is unavailable in this workspace")
	}
	old, ok := store.Resolve(in.ID)
	if !ok {
		return "", fmt.Errorf("note_correct: %q not found", in.ID)
	}
	if old.Private {
		return "", fmt.Errorf("note_correct: %s is private", old.ID)
	}
	n := old
	n.Body = in.Body
	if r := strings.TrimSpace(in.Reason); r != "" {
		n.Body = strings.TrimRight(n.Body, "\n") + "\n\n_Correction of " + old.ID + ": " + r + "_"
	}
	if in.Title != "" {
		n.Title = in.Title
	}
	if in.Confidence != "" {
		n.Confidence = notes.Confidence(in.Confidence)
	}
	if in.Verification != "" {
		n.Verification = in.Verification
	}
	n.Source = notes.SourceCorrection
	n.SourceAgent = t.agentID
	n.SourceSession = CurrentSessionID(ctx)
	n.Occurrences = 0
	n.Archived = false
	out, err := store.Correct(old.ID, n)
	if err != nil {
		return "", fmt.Errorf("note_correct: %w", err)
	}
	return fmt.Sprintf("filed %s as the correction of %s (old text kept as a dated record)", out.ID, old.ID), nil
}
