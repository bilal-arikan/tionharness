package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Decision authorities of the awareness layer (_Docs/94 §6). Each keeps a
// rule-based baseline so the layer works with no decision model at all, runs
// the model in the background in shadow mode, and lets the model decide in on
// mode. All four default to shadow.
const (
	// authBriefRelevance: which of the notes that reach a session deserve the
	// few inline slots of its briefing.
	authBriefRelevance = "brief-relevance"
	// authPulseUrgency: does a changed workspace pulse need the agent's attention
	// before it continues the user's request?
	authPulseUrgency = "pulse-urgency"
	// authWrapUpMemory: did a finished session learn something worth a note?
	authWrapUpMemory = "wrap-up-memory"
	// authNoteSupersede: does a new note correct an existing similar one?
	authNoteSupersede = "note-supersede"
)

func init() {
	decider.RegisterAuthority(decider.Authority{
		ID:               authBriefRelevance,
		Group:            decider.GroupSession,
		Pattern:          decider.PatternSelect,
		Label:            "Briefing note relevance",
		Description:      "Picks which memory notes are inlined in a session's start-of-session briefing. The rule orders by kind, author and recency; in on mode the decision model keeps only the notes likely to matter for this agent and session.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.6,
		ThresholdHint:    "Minimum probability that a note is relevant before it takes an inline slot.",
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authPulseUrgency,
		Group:            decider.GroupSession,
		Pattern:          decider.PatternGate,
		Label:            "Workspace pulse urgency",
		Description:      "When the one-line workspace pulse changes, decides whether it needs the agent's attention before it continues (a stuck session, a waiting question, a failed run). The rule flags waiting/stuck/failed; in on mode the decision model decides.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.7,
		ThresholdHint:    "Minimum probability of \"urgent\" before the pulse is marked for immediate attention.",
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authWrapUpMemory,
		Group:            decider.GroupSession,
		Pattern:          decider.PatternGate,
		Label:            "Wrap-up memory suggestion",
		Description:      "After a turn, decides whether the session digest shows something durable worth a memory note, and flags the digest so the user (and the agent, next turn) see the suggestion. The rule flags repeated tool errors, stuck turns and failed children.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.7,
		ThresholdHint:    "Minimum probability that the session learned something worth keeping.",
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authNoteSupersede,
		Group:            decider.GroupSession,
		Pattern:          decider.PatternGate,
		Label:            "Note correction detection",
		Description:      "When remember writes a note whose title closely matches an existing one, decides whether the new note corrects the old (filed as its supersession, old text kept) or is a separate note. The rule requires a near-identical title.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.75,
		ThresholdHint:    "Minimum probability that the new note supersedes the old before they are chained.",
	})
}

// ---- brief-relevance ---------------------------------------------------------

// briefNoteRanker returns the awareness.Input hook for the brief: nil when the
// authority is off (rule order stands). In shadow mode it returns the rule
// order and asks the model in the background; in on mode the model's picks are
// served, falling back to the rule order when it cannot answer.
func (r *Runtime) briefNoteRanker(agent db.Agent) func(context.Context, []notes.Note, int) []notes.Note {
	mode := r.deciderMode(authBriefRelevance)
	if mode == decider.ModeOff {
		return nil
	}
	return func(ctx context.Context, candidates []notes.Note, limit int) []notes.Note {
		if len(candidates) == 0 {
			return nil
		}
		// Ask about at most 3× the slots: the rule already ranked them.
		asked := candidates
		if len(asked) > limit*3 && limit > 0 {
			asked = asked[:limit*3]
		}
		req := briefRelevanceRequest(agent, asked)
		threshold := r.deciderThreshold(authBriefRelevance)
		baseline := noteIDs(candidates, limit)
		outcome := func(resp *decider.Response) (string, float64) {
			picked, strength := pickRelevant(resp, asked, threshold, limit)
			return strings.Join(noteIDs(picked, 0), ","), strength
		}
		if mode == decider.ModeShadow {
			r.shadowDecision(ctx, authBriefRelevance, agent, req, strings.Join(baseline, ","), SessionIDFrom(ctx), outcome)
			return nil
		}
		resp, err := r.decide(ctx, authBriefRelevance, agent, req, decider.WithOutcome(outcome))
		rec := decider.NewRecord(authBriefRelevance, decider.ModeOn, resp, err)
		rec.Ref, rec.Baseline = SessionIDFrom(ctx), strings.Join(baseline, ",")
		if err != nil {
			if !decisionOff(err) {
				r.logDecision(rec)
			}
			return nil
		}
		picked, strength := pickRelevant(resp, asked, threshold, limit)
		rec.Outcome, rec.Strength, rec.Applied = strings.Join(noteIDs(picked, 0), ","), strength, true
		r.logDecision(rec)
		if len(picked) == 0 {
			return nil // nothing cleared the bar: serve the rule order rather than an empty memory
		}
		return picked
	}
}

func briefRelevanceRequest(agent db.Agent, candidates []notes.Note) decider.Request {
	var state strings.Builder
	fmt.Fprintf(&state, "Agent: %s\n", strings.TrimSpace(agent.Name))
	if soul := strings.TrimSpace(agent.Soul); soul != "" {
		state.WriteString("Agent role: " + truncateRunes(strings.Join(strings.Fields(soul), " "), 600) + "\n")
	}
	state.WriteString("Candidate memory notes (id · kind · confidence · title · first sentence):\n")
	for _, n := range candidates {
		fmt.Fprintf(&state, "- %s · %s · %s · %s · %s\n", n.ID, n.Kind, n.Confidence, n.Title, notes.FirstSentence(n.Body, 160))
	}
	qs := map[string]decider.Question{}
	for _, n := range candidates {
		qs[noteQuestionKey(n.ID)] = decider.Noul(
			fmt.Sprintf("Is note %s likely to matter for what this agent will do in a typical session, so that it deserves one of the few inline slots of its start-of-session briefing?", n.ID),
			"It states a rule, decision, gotcha or fact this agent is likely to need or must not violate.",
			"It is a narrow work log, stale, irrelevant to this agent's role, or already obvious.",
		)
	}
	return decider.Request{State: state.String(), Questions: qs}
}

// noteQuestionKey makes a note id a valid question key.
func noteQuestionKey(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", "_"))
}

// pickRelevant keeps the candidates the model cleared, ordered by probability,
// at most limit; strength is the mean probability of the kept notes.
func pickRelevant(resp *decider.Response, candidates []notes.Note, threshold float64, limit int) ([]notes.Note, float64) {
	type scored struct {
		n notes.Note
		p float64
	}
	var kept []scored
	for _, n := range candidates {
		a, ok := resp.Answers[noteQuestionKey(n.ID)]
		if !ok || !a.Yes(threshold) {
			continue
		}
		kept = append(kept, scored{n, a.Probability})
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].p > kept[j].p })
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	out := make([]notes.Note, 0, len(kept))
	sum := 0.0
	for _, k := range kept {
		out = append(out, k.n)
		sum += k.p
	}
	if len(kept) == 0 {
		return nil, 0
	}
	return out, sum / float64(len(kept))
}

func noteIDs(ns []notes.Note, limit int) []string {
	out := make([]string, 0, len(ns))
	for i, n := range ns {
		if limit > 0 && i >= limit {
			break
		}
		out = append(out, n.ID)
	}
	return out
}

// ---- pulse-urgency -----------------------------------------------------------

const urgencyKey = "urgent"

func pulseUrgencyRequest(pulse string) decider.Request {
	return decider.Request{
		State: "Workspace pulse line just delivered to an agent mid-conversation:\n\n" + truncateRunes(pulse, 1000),
		Questions: map[string]decider.Question{
			urgencyKey: decider.Noul(
				"Does this change need the agent's attention BEFORE it continues the user's current request?",
				"A human is waiting for an answer, a session is stuck or blocked, a run failed, or a worker died.",
				"Only routine activity: sessions running, counts moving, nothing waiting or broken.",
			),
		},
	}
}

func urgencyVerdict(u bool) string {
	if u {
		return "urgent"
	}
	return "routine"
}

// pulseUrgencyJudge returns the awareness.Input hook: nil when off.
func (r *Runtime) pulseUrgencyJudge(agent db.Agent) func(context.Context, string) (bool, bool) {
	mode := r.deciderMode(authPulseUrgency)
	if mode == decider.ModeOff {
		return nil
	}
	return func(ctx context.Context, pulse string) (bool, bool) {
		threshold := r.deciderThreshold(authPulseUrgency)
		outcome := func(resp *decider.Response) (string, float64) {
			a := resp.Answers[urgencyKey]
			return urgencyVerdict(a.Yes(threshold)), a.Probability
		}
		if mode == decider.ModeShadow {
			_, rule := awareness.BuildPulse(ctx, r.awarenessInput(ctx, db.Session{ID: SessionIDFrom(ctx)}, agent, false))
			r.shadowDecision(ctx, authPulseUrgency, agent, pulseUrgencyRequest(pulse), urgencyVerdict(rule), SessionIDFrom(ctx), outcome)
			return false, false
		}
		resp, err := r.decide(ctx, authPulseUrgency, agent, pulseUrgencyRequest(pulse), decider.WithOutcome(outcome))
		rec := decider.NewRecord(authPulseUrgency, decider.ModeOn, resp, err)
		rec.Ref = SessionIDFrom(ctx)
		if err != nil {
			if !decisionOff(err) {
				r.logDecision(rec)
			}
			return false, false
		}
		a := resp.Answers[urgencyKey]
		urgent := a.Yes(threshold)
		rec.Outcome, rec.Strength, rec.Applied = urgencyVerdict(urgent), a.Probability, true
		r.logDecision(rec)
		return urgent, true
	}
}

// ---- wrap-up-memory ----------------------------------------------------------

const memoryKey = "worth_a_note"

func wrapUpMemoryRequest(d awareness.Digest) decider.Request {
	return decider.Request{
		State: "Digest of a finished agent session:\n\n" + truncateRunes(d.Text, 3000),
		Questions: map[string]decider.Question{
			memoryKey: decider.Noul(
				"Did this session learn or decide something DURABLE — a reusable lesson, a gotcha, a decision with reasoning — that a later session would benefit from as a memory note, and that it did not already record?",
				"Repeated failures of the same shape, a non-obvious fix, a design decision, a constraint discovered.",
				"Routine work with nothing generalizable, or the lesson was already recorded as a note.",
			),
		},
	}
}

func suggestVerdict(s bool) string {
	if s {
		return "suggest"
	}
	return "nothing"
}

// digestNoteSuggester returns the awareness.Input hook for the digest: nil
// when off (the rule stands).
func (r *Runtime) digestNoteSuggester(agent db.Agent) func(context.Context, awareness.Digest) (bool, bool) {
	mode := r.deciderMode(authWrapUpMemory)
	if mode == decider.ModeOff {
		return nil
	}
	return func(ctx context.Context, d awareness.Digest) (bool, bool) {
		threshold := r.deciderThreshold(authWrapUpMemory)
		outcome := func(resp *decider.Response) (string, float64) {
			a := resp.Answers[memoryKey]
			return suggestVerdict(a.Yes(threshold)), a.Probability
		}
		if mode == decider.ModeShadow {
			r.shadowDecision(ctx, authWrapUpMemory, agent, wrapUpMemoryRequest(d), suggestVerdict(awareness.SuggestNoteRule(d)), d.SessionID, outcome)
			return false, false
		}
		resp, err := r.decide(ctx, authWrapUpMemory, agent, wrapUpMemoryRequest(d), decider.WithOutcome(outcome))
		rec := decider.NewRecord(authWrapUpMemory, decider.ModeOn, resp, err)
		rec.Ref = d.SessionID
		if err != nil {
			if !decisionOff(err) {
				r.logDecision(rec)
			}
			return false, false
		}
		a := resp.Answers[memoryKey]
		s := a.Yes(threshold)
		rec.Outcome, rec.Strength, rec.Applied = suggestVerdict(s), a.Probability, true
		r.logDecision(rec)
		return s, true
	}
}

// ---- note-supersede ----------------------------------------------------------

const supersedeKey = "supersedes"

// supersedeRuleThreshold is the title similarity at which the rule alone files
// a correction. The tool asks only for pairs above 0.5; between the two the
// rule says "separate note" unless the model says otherwise.
const supersedeRuleThreshold = 0.8

func noteSupersedeRequest(candidate, existing notes.Note) decider.Request {
	var state strings.Builder
	fmt.Fprintf(&state, "EXISTING note %s [%s·%s] %q (updated %d):\n%s\n\n", existing.ID, existing.Kind, existing.Confidence, existing.Title, existing.Updated, truncateRunes(existing.Body, 1200))
	fmt.Fprintf(&state, "NEW note [%s·%s] %q:\n%s\n", candidate.Kind, candidate.Confidence, candidate.Title, truncateRunes(candidate.Body, 1200))
	return decider.Request{
		State: state.String(),
		Questions: map[string]decider.Question{
			supersedeKey: decider.Noul(
				"Does the NEW note correct, update or replace the EXISTING note (same subject, newer or contradicting claim), so that the existing one should be marked superseded by it?",
				"Same subject; the new text revises, narrows or contradicts the old claim.",
				"A different subject that happens to share words, or an addition that leaves the old note valid.",
			),
		},
	}
}

func supersedeVerdict(s bool) string {
	if s {
		return "supersedes"
	}
	return "separate"
}

// decideNoteSupersedes is the judgement behind remember's correction detection.
func (r *Runtime) decideNoteSupersedes(ctx context.Context, candidate, existing notes.Note) bool {
	rule := tools.TitleSimilarity(candidate.Title, existing.Title) >= supersedeRuleThreshold
	mode := r.deciderMode(authNoteSupersede)
	if mode == decider.ModeOff {
		return rule
	}
	agent, _ := r.db.GetAgent(ctx, candidate.SourceAgent)
	threshold := r.deciderThreshold(authNoteSupersede)
	outcome := func(resp *decider.Response) (string, float64) {
		a := resp.Answers[supersedeKey]
		return supersedeVerdict(a.Yes(threshold)), a.Probability
	}
	ref := candidate.SourceSession
	if ref == "" {
		ref = SessionIDFrom(ctx)
	}
	if mode == decider.ModeShadow {
		r.shadowDecision(ctx, authNoteSupersede, agent, noteSupersedeRequest(candidate, existing), supersedeVerdict(rule), ref, outcome)
		return rule
	}
	resp, err := r.decide(ctx, authNoteSupersede, agent, noteSupersedeRequest(candidate, existing), decider.WithOutcome(outcome))
	rec := decider.NewRecord(authNoteSupersede, decider.ModeOn, resp, err)
	rec.Ref, rec.Baseline = ref, supersedeVerdict(rule)
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return rule
	}
	a := resp.Answers[supersedeKey]
	s := a.Yes(threshold)
	rec.Outcome, rec.Strength, rec.Applied = supersedeVerdict(s), a.Probability, true
	r.logDecision(rec)
	return s
}
