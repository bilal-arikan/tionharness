package decider

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// chatRequest is what the fake server reads from a request.
type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens   int            `json:"max_tokens"`
	Logprobs    bool           `json:"logprobs"`
	TopLogprobs int            `json:"top_logprobs"`
	Extra       map[string]any `json:"chat_template_kwargs"`
}

func (r chatRequest) user() string {
	for _, m := range r.Messages {
		if m.Role == "user" {
			return m.Content
		}
	}
	return ""
}

// tok builds one generated position: the chosen token plus alternatives with
// the given probabilities.
func tok(chosen string, alts map[string]float64) map[string]any {
	top := []map[string]any{}
	for t, p := range alts {
		top = append(top, map[string]any{"token": t, "logprob": math.Log(p)})
	}
	return map[string]any{"token": chosen, "logprob": math.Log(alts[chosen] + 1e-12), "top_logprobs": top}
}

// chatServer answers with the positions answer(req) returns.
func chatServer(t *testing.T, answer func(req chatRequest) []map[string]any) (*httptest.Server, *[]chatRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req chatRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		positions := answer(req)
		text := ""
		for _, p := range positions {
			text += p["token"].(string)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": req.Model + "-gguf",
			"choices": []map[string]any{{
				"message":  map[string]any{"role": "assistant", "content": text},
				"logprobs": map[string]any{"content": positions},
			}},
			"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": len(positions)},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func newLogprobsClient(t *testing.T, base string, cfg map[string]string) Decider {
	t.Helper()
	d, err := logprobsBackend{}.New(Endpoint{BaseURL: base}, ClientOptions{Model: "qwen3:4b", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestLogprobsReadsDistributions(t *testing.T) {
	srv, seen := chatServer(t, func(req chatRequest) []map[string]any {
		u := req.user()
		switch {
		case strings.Contains(u, "Question: approve?"):
			// An empty think block and a leading space before the label.
			return []map[string]any{
				tok("<think>", map[string]float64{"<think>": 1}),
				tok("\n\n", map[string]float64{"\n\n": 1}),
				tok("</think>", map[string]float64{"</think>": 1}),
				tok(" A", map[string]float64{" A": 0.6, "A": 0.2, "B": 0.2}),
			}
		case strings.Contains(u, "Question: which tier?"):
			// Options sorted by key: balanced=A, fast=B, frontier=C.
			return []map[string]any{tok("B", map[string]float64{"B": 0.7, "A": 0.25, "C": 0.05})}
		case strings.Contains(u, "Question: how risky?"):
			return []map[string]any{tok("2", map[string]float64{"2": 0.5, "1": 0.5})}
		}
		return []map[string]any{tok("?", map[string]float64{"?": 1})}
	})
	d := newLogprobsClient(t, srv.URL+"/v1", map[string]string{fieldSuffix: "/no_think", fieldExtraBody: `{"chat_template_kwargs":{"enable_thinking":false}}`})
	resp, err := d.Decide(context.Background(), Request{State: map[string]any{"cmd": "git push --force"}, Questions: map[string]Question{
		"approve": Noul("approve?", "destructive", "harmless"),
		"tier":    Choice("which tier?", map[string]string{"fast": "quick", "balanced": "mid", "frontier": "big"}),
		"risk":    Score("how risky?", "none", "some", "a lot"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if p := resp.Answers["approve"].Probability; math.Abs(p-0.8) > 1e-9 {
		t.Errorf("P(yes) = %v, want 0.8 (\" A\" and \"A\" are the same answer)", p)
	}
	if a := resp.Answers["tier"]; a.Choice != "fast" || a.Probabilities["balanced"] != 0.25 || a.Confidence != 0.7 {
		t.Errorf("choice = %+v", a)
	}
	if a := resp.Answers["risk"]; math.Abs(a.Score-1.5) > 1e-9 || a.Confidence != 0.5 {
		t.Errorf("score = %+v", a)
	}
	if resp.Usage.InputTokens != 150 || resp.ServedModel != "qwen3:4b-gguf" || resp.BillingProvider != billingLocal || len(resp.Warnings) != 0 {
		t.Errorf("accounting = %+v", resp)
	}
	if len(*seen) != 3 {
		t.Fatalf("requests = %d, want one per question", len(*seen))
	}
	for _, r := range *seen {
		if !r.Logprobs || r.TopLogprobs != defaultTopLogprobs || r.MaxTokens != defaultMaxTokens || r.Extra["enable_thinking"] != false {
			t.Errorf("request = %+v", r)
		}
		if !strings.HasSuffix(r.user(), "and nothing else. /no_think") || !strings.Contains(r.user(), `{"cmd":"git push --force"}`) {
			t.Errorf("prompt = %q", r.user())
		}
	}
}

func TestLogprobsWithoutProbabilities(t *testing.T) {
	// A server that ignores the logprobs flag: the label is read from the text.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Answer: B"}}],"usage":{"prompt_tokens":9}}`)
	}))
	defer srv.Close()
	d := newLogprobsClient(t, srv.URL+"/v1", nil)
	resp, err := d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{"q": Noul("?", "", "")}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answers["q"].Probability != 0 || len(resp.Warnings) != 1 {
		t.Errorf("hard answer = %+v warnings = %v", resp.Answers["q"], resp.Warnings)
	}
}

func TestLogprobsExplainsAReasoningModel(t *testing.T) {
	srv, _ := chatServer(t, func(chatRequest) []map[string]any {
		return []map[string]any{
			tok("<think>", map[string]float64{"<think>": 1}),
			tok("Okay", map[string]float64{"Okay": 1}),
			tok(" A", map[string]float64{" A": 1}), // inside the think block: not the answer
		}
	})
	d := newLogprobsClient(t, srv.URL+"/v1", nil)
	_, err := d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{"q": Noul("?", "", "")}})
	if err == nil || !strings.Contains(err.Error(), "/no_think") {
		t.Errorf("err = %v, want a hint to switch reasoning off", err)
	}
}

func TestLogprobsRunsQuestionsConcurrently(t *testing.T) {
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"A"},"logprobs":{"content":[{"token":"A","logprob":0,"top_logprobs":[{"token":"A","logprob":0}]}]}}]}`)
	}))
	defer srv.Close()
	d := newLogprobsClient(t, srv.URL+"/v1", map[string]string{fieldParallel: "2"})
	qs := map[string]Question{}
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		qs[k] = Noul("?", "", "")
	}
	if _, err := d.Decide(context.Background(), Request{State: "s", Questions: qs}); err != nil {
		t.Fatal(err)
	}
	if p := peak.Load(); p != 2 {
		t.Errorf("peak concurrency = %d, want the configured 2", p)
	}
}

func TestLogprobsLabels(t *testing.T) {
	ls := labelsFor(Choice("?", map[string]string{"z": "Z", "a": "A"}))
	if strings.Join(ls.labels, "") != "AB" || ls.keys[0] != "a" {
		t.Errorf("labels = %+v", ls)
	}
	for tokIn, want := range map[string]string{" A": "A", "A)": "A", "**B**": "B", "Yes": "A", "no": "B"} {
		if got, ok := labelsFor(Noul("?", "", "")).match(tokIn); !ok || got != want {
			t.Errorf("match(%q) = %q, %v; want %q", tokIn, got, ok, want)
		}
	}
	if _, ok := labelsFor(Choice("?", map[string]string{"x": "X", "y": "Y"})).match("Yes"); ok {
		t.Error("a yes/no synonym matched a choice label")
	}
	if got := labelList(labelsFor(Score("?", "0", "1", "2", "3", "4", "5", "6", "7")).labels); got != "0-7" {
		t.Errorf("label list = %q", got)
	}
	big := map[string]string{}
	for i := range 60 {
		big["o"+string(rune('a'+i%26))+string(rune('a'+i/26))] = "x"
	}
	if err := (Request{State: "s", Questions: map[string]Question{"q": Choice("?", big)}}).ValidateFor(logprobsBackend{}.Manifest().Limits); err == nil {
		t.Error("a choice with more options than labels was accepted")
	}
}

func TestChatCompletionsURLAndOptions(t *testing.T) {
	cases := map[string]string{
		"":                             "http://127.0.0.1:11434/v1/chat/completions",
		"http://localhost:1234/v1/":    "http://localhost:1234/v1/chat/completions",
		"https://openrouter.ai/api/v1": "https://openrouter.ai/api/v1/chat/completions",
		"http://gpu.lan:8000/v1/chat/completions/": "http://gpu.lan:8000/v1/chat/completions",
	}
	for in, want := range cases {
		if got, err := ChatCompletionsURL(in); err != nil || got != want {
			t.Errorf("ChatCompletionsURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := (logprobsBackend{}).New(Endpoint{}, ClientOptions{}); err == nil {
		t.Error("a client without a model id was built")
	}
	if err := (logprobsBackend{}).ValidateConfig(map[string]string{fieldParallel: "0"}); err == nil {
		t.Error("parallel=0 accepted")
	}
	if !(logprobsBackend{}).Accepts("lmstudio", "") || (logprobsBackend{}).Accepts("anthropic", "") {
		t.Error("wrong provider kinds accepted")
	}
}
