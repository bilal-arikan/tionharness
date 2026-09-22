package decider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Patterns: the shapes most authorities take, so a new authority is a request
// builder and a few lines of wiring rather than its own decision logic.
//
//	gate   - one Noul question, read with Answer.Yes(threshold)
//	pick   - one Choice question, read with Pick
//	rate   - one Score question, read with Answer.Level
//	select - Hub.Select: which of many candidates (skills, tools, artifacts,
//	         models) are relevant — one yes/no question per candidate
//	triage - Hub.Triage: which label each item of a list (sessions, tasks)
//	         gets — one choice question per item, all items in one state

// maxParallelRequests bounds how many requests of one Select/Triage run at once.
const maxParallelRequests = 4

// Pick reads the choice answer under key: the chosen option, its strength, and
// whether the strength clears threshold. The option is returned even when it
// does not, so a caller can log what the model leaned towards.
func Pick(resp *Response, key string, threshold float64) (choice string, strength float64, ok bool) {
	if resp == nil {
		return "", 0, false
	}
	a, found := resp.Answers[key]
	if !found || a.Type != QuestionChoice || a.Choice == "" {
		return "", 0, false
	}
	s := a.Strength()
	return a.Choice, s, s >= threshold
}

// Candidate is one thing Select can choose.
type Candidate struct {
	// Key identifies the candidate to the caller (a skill name, a tool name).
	Key string `json:"key"`
	// Text is what the model reads about it: name and description.
	Text string `json:"text"`
}

// Scored is a selected candidate with the probability that it is relevant.
type Scored struct {
	Candidate
	Probability float64 `json:"probability"`
}

// SelectSpec describes one selection.
type SelectSpec struct {
	// Instructions is asked about every candidate; the candidate's text
	// follows it ("Is this skill needed to handle the request?").
	Instructions string
	// IfTrue / IfFalse describe a relevant and an irrelevant candidate.
	IfTrue, IfFalse string
	// Threshold is the minimum probability (0 = the authority's threshold).
	Threshold float64
	// MaxK caps the result, most probable first (0 = no cap).
	MaxK int
}

func selectKey(i int) string { return fmt.Sprintf("c%03d", i) }

// SelectRequests builds the requests of a selection: one yes/no question per
// candidate, at most perRequest per request, all about the same state.
func SelectRequests(state any, spec SelectSpec, cands []Candidate, perRequest int) []Request {
	if perRequest <= 0 || perRequest > maxQuestions {
		perRequest = maxQuestions
	}
	instructions := strings.TrimSpace(spec.Instructions)
	var out []Request
	for start := 0; start < len(cands); start += perRequest {
		end := min(start+perRequest, len(cands))
		qs := make(map[string]Question, end-start)
		for i := start; i < end; i++ {
			qs[selectKey(i)] = Noul(instructions+"\n\nCandidate: "+preview(cands[i].Text, maxItemRunes), spec.IfTrue, spec.IfFalse)
		}
		out = append(out, Request{State: state, Questions: qs})
	}
	return out
}

// Selected collects the candidates whose probability clears threshold, most
// probable first (ties keep candidate order), at most maxK (0 = all).
func Selected(resps []*Response, cands []Candidate, threshold float64, maxK int) []Scored {
	var out []Scored
	for i, c := range cands {
		key := selectKey(i)
		for _, r := range resps {
			if r == nil {
				continue
			}
			if a, ok := r.Answers[key]; ok {
				if a.Yes(threshold) {
					out = append(out, Scored{Candidate: c, Probability: a.Probability})
				}
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Probability > out[j].Probability })
	if maxK > 0 && len(out) > maxK {
		out = out[:maxK]
	}
	return out
}

// Select asks an authority's model which candidates are relevant to state. It
// fails as a whole when any request fails, so the caller falls back to its own
// logic rather than acting on half an answer.
func (h *Hub) Select(ctx context.Context, authority string, state any, spec SelectSpec, cands []Candidate, opts ...CallOption) ([]Scored, error) {
	if len(cands) == 0 {
		return nil, nil
	}
	threshold := spec.Threshold
	if threshold <= 0 {
		threshold = h.Threshold(authority)
	}
	resps, err := h.decideAll(ctx, authority, SelectRequests(state, spec, cands, maxQuestions), opts)
	if err != nil {
		return nil, err
	}
	return Selected(resps, cands, threshold, spec.MaxK), nil
}

// Item is one entry Triage labels.
type Item struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// TriageSpec describes one triage.
type TriageSpec struct {
	// Context precedes the item list in the state ("Sessions of this
	// workspace, oldest activity first").
	Context string
	// Instructions is asked about every item ("What should happen to this
	// session?").
	Instructions string
	// Labels maps a label key to its description.
	Labels map[string]string
	// Threshold is the minimum strength for a label to count; below it the
	// item stays unlabelled. 0 = the authority's threshold.
	Threshold float64
	// ChunkBytes bounds the item text of one request (0 = 8000), so a long
	// list is split across requests instead of being cut.
	ChunkBytes int
}

// Triaged is one item with its label ("" when the model was not sure enough).
type Triaged struct {
	Item
	Label    string  `json:"label"`
	Strength float64 `json:"strength"`
}

// maxItemRunes caps one item's (or candidate's) text inside a request.
const maxItemRunes = 1200

func itemKey(i int) string { return fmt.Sprintf("i%03d", i) }

// TriageRequests builds the requests of a triage: the items listed in the
// state, one choice question per item pointing at it by key.
func TriageRequests(spec TriageSpec, items []Item) []Request {
	budget := spec.ChunkBytes
	if budget <= 0 {
		budget = 8000
	}
	instructions := strings.TrimSpace(spec.Instructions)
	var out []Request
	var list strings.Builder
	qs := map[string]Question{}
	flush := func() {
		if len(qs) == 0 {
			return
		}
		state := strings.TrimSpace(spec.Context + "\n\nItems:\n" + list.String())
		out = append(out, Request{State: state, Questions: qs})
		list.Reset()
		qs = map[string]Question{}
	}
	for i, it := range items {
		key := itemKey(i)
		line := "[" + key + "] " + preview(it.Text, maxItemRunes) + "\n"
		if len(qs) > 0 && (list.Len()+len(line) > budget || len(qs) >= maxQuestions) {
			flush()
		}
		list.WriteString(line)
		qs[key] = Choice(instructions+"\nItem: ["+key+"]", spec.Labels)
	}
	flush()
	return out
}

// TriageResults maps the answers back onto the items.
func TriageResults(resps []*Response, items []Item, threshold float64) []Triaged {
	out := make([]Triaged, len(items))
	for i, it := range items {
		out[i] = Triaged{Item: it}
		key := itemKey(i)
		for _, r := range resps {
			if r == nil {
				continue
			}
			if _, ok := r.Answers[key]; !ok {
				continue
			}
			choice, strength, ok := Pick(r, key, threshold)
			out[i].Strength = strength
			if ok {
				out[i].Label = choice
			}
			break
		}
	}
	return out
}

// Triage asks an authority's model which label each item gets. Like Select it
// fails as a whole when any request fails.
func (h *Hub) Triage(ctx context.Context, authority string, spec TriageSpec, items []Item, opts ...CallOption) ([]Triaged, error) {
	if len(items) == 0 {
		return nil, nil
	}
	threshold := spec.Threshold
	if threshold <= 0 {
		threshold = h.Threshold(authority)
	}
	resps, err := h.decideAll(ctx, authority, TriageRequests(spec, items), opts)
	if err != nil {
		return nil, err
	}
	return TriageResults(resps, items, threshold), nil
}

// decideAll runs reqs for one authority, a few at a time. The first failure
// cancels the rest and is returned.
func (h *Hub) decideAll(ctx context.Context, authority string, reqs []Request, opts []CallOption) ([]*Response, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]*Response, len(reqs))
	errs := make([]error, len(reqs))
	sem := make(chan struct{}, maxParallelRequests)
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			out[i], errs[i] = h.Decide(ctx, authority, req, opts...)
			if errs[i] != nil {
				cancel()
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return nil, err
		}
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
