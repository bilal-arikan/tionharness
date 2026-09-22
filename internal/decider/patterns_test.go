package decider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPick(t *testing.T) {
	resp := &Response{Answers: map[string]Answer{
		"arm": {Type: QuestionChoice, Choice: "b", Probabilities: map[string]float64{"a": 0.35, "b": 0.65}},
		"yes": {Type: QuestionNoul, Probability: 0.9},
	}}
	if c, s, ok := Pick(resp, "arm", 0.6); c != "b" || s != 0.65 || !ok {
		t.Errorf("pick = %q %v %v", c, s, ok)
	}
	if c, _, ok := Pick(resp, "arm", 0.7); c != "b" || ok {
		t.Errorf("below threshold = %q %v; the lean must still be reported", c, ok)
	}
	if _, _, ok := Pick(resp, "yes", 0); ok {
		t.Error("a noul answer read as a pick")
	}
	if _, _, ok := Pick(nil, "arm", 0); ok {
		t.Error("nil response picked")
	}
}

func TestVerdict(t *testing.T) {
	one := &Response{Answers: map[string]Answer{"q": {Type: QuestionNoul, Probability: 0.75}}}
	if v, s := Verdict(one, 0.7); v != "yes" || s != 0.75 {
		t.Errorf("verdict = %q %v", v, s)
	}
	if v, _ := Verdict(one, 0.8); v != "no" {
		t.Errorf("verdict at a higher threshold = %q", v)
	}
	many := &Response{Answers: map[string]Answer{
		"risk": {Type: QuestionScore, Score: 1.6, Confidence: 0.6},
		"pick": {Type: QuestionChoice, Choice: "x", Confidence: 0.9},
	}}
	if v, s := Verdict(many, 0.5); v != "pick=x,risk=L2" || s != 0.6 {
		t.Errorf("verdict = %q %v", v, s)
	}
	if v, _ := Verdict(nil, 0.5); v != "" {
		t.Error("nil verdict not empty")
	}
}

func TestSelectRequestsAndResults(t *testing.T) {
	var cands []Candidate
	for i := range 70 {
		cands = append(cands, Candidate{Key: fmt.Sprintf("skill-%d", i), Text: fmt.Sprintf("skill %d", i)})
	}
	reqs := SelectRequests("user asks to deploy", SelectSpec{Instructions: "Is this skill needed?"}, cands, 0)
	if len(reqs) != 2 || len(reqs[0].Questions) != 64 || len(reqs[1].Questions) != 6 {
		t.Fatalf("chunks = %d (%d, %d)", len(reqs), len(reqs[0].Questions), len(reqs[1].Questions))
	}
	if q := reqs[1].Questions["c069"]; !strings.Contains(q.Instructions, "Candidate: skill 69") {
		t.Errorf("question = %+v", q)
	}
	resps := []*Response{
		{Answers: map[string]Answer{"c000": {Type: QuestionNoul, Probability: 0.9}, "c001": {Type: QuestionNoul, Probability: 0.95}, "c002": {Type: QuestionNoul, Probability: 0.2}}},
		{Answers: map[string]Answer{"c069": {Type: QuestionNoul, Probability: 0.8}}},
	}
	got := Selected(resps, cands, 0.7, 2)
	if len(got) != 2 || got[0].Key != "skill-1" || got[1].Key != "skill-0" {
		t.Errorf("selected = %+v, want the two most probable", got)
	}
	if all := Selected(resps, cands, 0.7, 0); len(all) != 3 {
		t.Errorf("uncapped = %+v", all)
	}
}

func TestTriageRequestsChunkByBytes(t *testing.T) {
	items := []Item{
		{Key: "SES1", Text: strings.Repeat("a", 3000)},
		{Key: "SES2", Text: strings.Repeat("b", 3000)},
		{Key: "SES3", Text: "short"},
	}
	spec := TriageSpec{
		Context: "Sessions of this workspace.", Instructions: "What should happen to this session?",
		Labels: map[string]string{"keep": "Still in use.", "archive": "Finished or abandoned."}, ChunkBytes: 2000,
	}
	reqs := TriageRequests(spec, items)
	if len(reqs) != 2 || len(reqs[0].Questions) != 1 || len(reqs[1].Questions) != 2 {
		t.Fatalf("chunks = %d", len(reqs))
	}
	state := reqs[1].State.(string)
	if !strings.HasPrefix(state, "Sessions of this workspace.") || !strings.Contains(state, "[i002] short") {
		t.Errorf("state = %q", state)
	}
	if q := reqs[1].Questions["i002"]; q.Type != QuestionChoice || !strings.HasSuffix(q.Instructions, "Item: [i002]") {
		t.Errorf("question = %+v", q)
	}
	for _, r := range reqs {
		if err := r.Validate(); err != nil {
			t.Errorf("triage request invalid: %v", err)
		}
	}
	resps := []*Response{
		{Answers: map[string]Answer{"i000": {Type: QuestionChoice, Choice: "archive", Confidence: 0.9}}},
		{Answers: map[string]Answer{
			"i001": {Type: QuestionChoice, Choice: "keep", Confidence: 0.55},
			"i002": {Type: QuestionChoice, Choice: "keep", Confidence: 0.95},
		}},
	}
	out := TriageResults(resps, items, 0.7)
	if out[0].Label != "archive" || out[1].Label != "" || out[1].Strength != 0.55 || out[2].Label != "keep" || out[2].Key != "SES3" {
		t.Errorf("triage = %+v", out)
	}
}

// selectServer answers "yes" (0.9) for candidates whose text mentions deploy,
// "no" (0.1) otherwise.
func selectServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Questions map[string]struct {
				Instructions string `json:"instructions"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(body, &req)
		answers := map[string]any{}
		for k, q := range req.Questions {
			p := 0.1
			if strings.Contains(q.Instructions, "deploy") {
				p = 0.9
			}
			answers[k] = map[string]any{"type": "noul", "noul": p}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestHubSelectEndToEnd(t *testing.T) {
	srv, calls := selectServer(t)
	h, _ := newTestHub(t, newDecisionServer(t))
	ownModel(t, h, "sel", srv.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn; ac.Model = "sel" })
	var cands []Candidate
	for i := range 66 {
		cands = append(cands, Candidate{Key: fmt.Sprintf("s%d", i), Text: "generic helper"})
	}
	cands[3].Text = "deploy to production"
	cands[65].Text = "deploy previews"
	got, err := h.Select(context.Background(), testGate, "Ship the release", SelectSpec{Instructions: "Is this skill needed?"}, cands)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "s3" || got[1].Key != "s65" {
		t.Errorf("selected = %+v", got)
	}
	if calls.Load() != 2 {
		t.Errorf("requests = %d, want two chunks", calls.Load())
	}
	// Off means no selection and no call.
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOff })
	if _, err := h.Select(context.Background(), testGate, "x", SelectSpec{}, cands); err == nil {
		t.Error("an off authority selected")
	}
}
