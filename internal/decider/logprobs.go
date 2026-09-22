package decider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LLM-logprobs backend: turns an ordinary chat model into a decision model.
// Every question is asked as a one-label multiple choice ("A) yes / B) no",
// "A) option / B) option …", "0) level … 3) level") over an OpenAI-compatible
// /chat/completions endpoint with logprobs on, and the answer's distribution is
// read from the token probabilities at the answer position — the technique the
// open Jev stand-ins use. It works with Ollama (0.12.11+), LM Studio (0.3.39+),
// llama.cpp, vLLM and any server that returns logprobs, so any local model the
// user already runs can serve as a decider.
//
// The probabilities are the model's own, not calibrated like Jev's, and a
// question costs one generation of a few tokens, so questions run concurrently.
// A server that ignores the logprobs flag still works: the answer is read from
// the text and reported as a hard 0/1 with a warning.
const (
	LogprobsBackendID = "llm-logprobs"

	ollamaDefaultBase   = "http://127.0.0.1:11434/v1"
	lmStudioDefaultBase = "http://127.0.0.1:1234/v1"
	logprobsErrPrefix   = "llm-logprobs"

	// Backend-specific model fields.
	fieldSuffix      = "suffix"
	fieldExtraBody   = "extraBody"
	fieldTopLogprobs = "topLogprobs"
	fieldMaxTokens   = "maxTokens"
	fieldParallel    = "parallel"

	defaultTopLogprobs = 20
	defaultMaxTokens   = 8
	defaultParallel    = 4
)

func init() { Register(logprobsBackend{}) }

type logprobsBackend struct{}

func (logprobsBackend) Manifest() Manifest {
	return Manifest{
		ID:             LogprobsBackendID,
		Label:          "Any LLM (token probabilities)",
		Description:    "Turns an ordinary chat model into a decision model: each question is asked as a one-letter multiple choice and the answer's probabilities are read from the model's token probabilities. Works with Ollama, LM Studio, llama.cpp, vLLM and any OpenAI-compatible server that returns logprobs. Approximate: the probabilities are the model's own, not calibrated.",
		ProviderKinds:  []string{"lmstudio", "openai-compat", "openrouter"},
		DefaultBaseURL: ollamaDefaultBase,
		KeyRequired:    false,
		Fields: []Field{
			{Key: fieldSuffix, Label: "Question suffix", Type: "text", Placeholder: "/no_think",
				Help: "Appended to every question. Hybrid reasoning models must answer at once: /no_think for Qwen3."},
			{Key: fieldExtraBody, Label: "Extra request fields (JSON)", Type: "textarea", Placeholder: `{"chat_template_kwargs": {"enable_thinking": false}}`,
				Help: "Merged into every request body, e.g. to switch reasoning off on vLLM or llama.cpp."},
			{Key: fieldTopLogprobs, Label: "Top logprobs", Type: "number", Default: strconv.Itoa(defaultTopLogprobs),
				Help: "Alternatives read per token (1-20). Options outside them count as zero."},
			{Key: fieldMaxTokens, Label: "Max answer tokens", Type: "number", Default: strconv.Itoa(defaultMaxTokens),
				Help: "Tokens the model may spend before its label (an empty <think></think> block, a leading space)."},
			{Key: fieldParallel, Label: "Parallel questions", Type: "number", Default: strconv.Itoa(defaultParallel),
				Help: "Questions asked at once. Lower it if the server runs one request at a time."},
		},
		Models: []Model{
			{ID: "qwen3:4b", Label: "Qwen3 4B (Ollama)", Description: "Small and quick; pair with the /no_think suffix."},
			{ID: "qwen3-8b", Label: "Qwen3 8B (LM Studio)", Description: "LM Studio's id for Qwen3 8B; pair with the /no_think suffix."},
			{ID: "gemma3:4b", Label: "Gemma 3 4B (Ollama)", Description: "Answers directly, no reasoning switch needed."},
		},
		DefaultModel: "qwen3:4b",
		// Local servers often run a 4k context unless told otherwise.
		ContextTokens:    4096,
		DefaultTimeoutMs: 15000,
		// Options are labelled A-Z then a-z, levels 0-9.
		Limits:       Limits{MaxOptions: len(choiceLabels), MaxLevels: 10},
		DecisionOnly: false,
		Calibrated:   false,
		Presets: []Preset{
			{
				ID:          "ollama-local",
				Label:       "Ollama · this machine",
				Description: "A model served by Ollama on this computer (port 11434).",
				Credentials: CredentialsOwn,
				BaseURL:     ollamaDefaultBase,
				Model:       "qwen3:4b",
				Config:      map[string]string{fieldSuffix: "/no_think"},
			},
			{
				ID:          "lmstudio-local",
				Label:       "LM Studio · this machine",
				Description: "The model loaded in LM Studio's local server (port 1234).",
				Credentials: CredentialsOwn,
				BaseURL:     lmStudioDefaultBase,
				Model:       "qwen3-8b",
				Config:      map[string]string{fieldSuffix: "/no_think"},
			},
		},
	}
}

// Accepts lends the credentials (and base URL) of any chat-completions
// provider: LM Studio, a custom OpenAI-compatible server, or OpenRouter.
func (logprobsBackend) Accepts(kind, _ string) bool {
	switch kind {
	case "lmstudio", "openai-compat", "openrouter":
		return true
	}
	return false
}

// ValidateConfig checks the backend-specific fields before a model is saved.
func (logprobsBackend) ValidateConfig(cfg map[string]string) error {
	_, err := parseLogprobsOptions(cfg)
	return err
}

func (b logprobsBackend) New(ep Endpoint, opts ClientOptions) (Decider, error) {
	if ep.Kind != "" && !b.Accepts(ep.Kind, ep.BaseURL) {
		return nil, fmt.Errorf("provider instance %q (%s) has no chat-completions endpoint", ep.InstanceID, ep.Kind)
	}
	root, err := endpointBase(ep)
	if err != nil {
		return nil, err
	}
	endpoint, err := ChatCompletionsURL(root)
	if err != nil {
		return nil, err
	}
	lo, err := parseLogprobsOptions(opts.Config)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		return nil, fmt.Errorf("an LLM decision model needs a model id (the id the server lists)")
	}
	return &logprobsClient{
		url:       endpoint,
		authorize: ep.Authorize,
		model:     model,
		timeout:   timeoutOrDefault(opts.Timeout),
		client:    opts.HTTPClient,
		opts:      lo,
		billing:   billingProviderFor(endpoint, ep.Kind),
	}, nil
}

// ChatCompletionsURL derives the chat-completions URL from a base URL
// (".../v1" + "/chat/completions"; a full URL is kept). "" = Ollama's.
func ChatCompletionsURL(base string) (string, error) {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		b = ollamaDefaultBase
	}
	if !strings.HasPrefix(b, "http://") && !strings.HasPrefix(b, "https://") {
		return "", fmt.Errorf("base URL %q must start with http:// or https://", base)
	}
	if strings.HasSuffix(b, "/chat/completions") {
		return b, nil
	}
	return b + "/chat/completions", nil
}

// logprobsOptions are the parsed backend-specific fields.
type logprobsOptions struct {
	suffix      string
	extraBody   map[string]any
	topLogprobs int
	maxTokens   int
	parallel    int
}

func parseLogprobsOptions(cfg map[string]string) (logprobsOptions, error) {
	o := logprobsOptions{
		suffix:      strings.TrimSpace(cfg[fieldSuffix]),
		topLogprobs: defaultTopLogprobs,
		maxTokens:   defaultMaxTokens,
		parallel:    defaultParallel,
	}
	if raw := strings.TrimSpace(cfg[fieldExtraBody]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &o.extraBody); err != nil || o.extraBody == nil {
			return o, fmt.Errorf("extra request fields must be a JSON object")
		}
	}
	ints := []struct {
		key      string
		dst      *int
		min, max int
	}{
		{fieldTopLogprobs, &o.topLogprobs, 1, 20},
		{fieldMaxTokens, &o.maxTokens, 1, 64},
		{fieldParallel, &o.parallel, 1, 16},
	}
	for _, f := range ints {
		raw := strings.TrimSpace(cfg[f.key])
		if raw == "" {
			continue
		}
		v, err := strconv.Atoi(raw)
		if err != nil || v < f.min || v > f.max {
			return o, fmt.Errorf("%s must be a whole number from %d to %d", f.key, f.min, f.max)
		}
		*f.dst = v
	}
	return o, nil
}

type logprobsClient struct {
	url       string
	authorize func(http.Header)
	model     string
	timeout   time.Duration
	client    *http.Client
	opts      logprobsOptions
	billing   string
}

// questionResult is one question's answer and usage.
type questionResult struct {
	key     string
	answer  Answer
	usage   Usage
	served  string
	warning string
	err     error
}

func (c *logprobsClient) Decide(ctx context.Context, req Request) (*Response, error) {
	req = req.Normalized()
	if err := req.ValidateFor(Limits{MaxOptions: len(choiceLabels), MaxLevels: 10}); err != nil {
		return nil, err
	}
	model := c.model
	if m := strings.TrimSpace(req.Model); m != "" {
		model = m
	}
	state, err := renderState(req.State)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys := sortedKeys(req.Questions)
	results := make([]questionResult, len(keys))
	sem := make(chan struct{}, c.opts.parallel)
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = questionResult{key: k, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			results[i] = c.ask(ctx, model, state, k, req.Questions[k])
			if results[i].err != nil {
				cancel()
			}
		})
	}
	wg.Wait()

	resp := &Response{
		Backend:         LogprobsBackendID,
		Model:           model,
		BillingProvider: c.billing,
		Answers:         make(map[string]Answer, len(keys)),
	}
	warnings := map[string]bool{}
	for _, r := range results {
		if r.err != nil {
			return nil, firstRealError(results)
		}
		resp.Answers[r.key] = r.answer
		resp.Usage.InputTokens += r.usage.InputTokens
		resp.Usage.OutputTokens += r.usage.OutputTokens
		if resp.ServedModel == "" {
			resp.ServedModel = r.served
		}
		if r.warning != "" {
			warnings[r.warning] = true
		}
	}
	for w := range warnings {
		resp.Warnings = append(resp.Warnings, w)
	}
	sort.Strings(resp.Warnings)
	resp.LatencyMs = time.Since(start).Milliseconds()
	return resp, nil
}

// firstRealError prefers a question's own failure over the cancellations it
// caused in its siblings.
func firstRealError(results []questionResult) error {
	var fallback error
	for _, r := range results {
		if r.err == nil {
			continue
		}
		if fallback == nil {
			fallback = r.err
		}
		if !errors.Is(r.err, context.Canceled) {
			return r.err
		}
	}
	return fallback
}

// chat-completions wire shapes (only the fields the adapter reads).
type chatLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
}

type chatTokenLogprobs struct {
	chatLogprob
	TopLogprobs []chatLogprob `json:"top_logprobs"`
}

type chatCompletion struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Logprobs *struct {
			Content []chatTokenLogprobs `json:"content"`
		} `json:"logprobs"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// ask puts one question to the model and reads the label distribution.
func (c *logprobsClient) ask(ctx context.Context, model, state, key string, q Question) questionResult {
	labels := labelsFor(q)
	body := map[string]any{}
	maps.Copy(body, c.opts.extraBody)
	body["model"] = model
	body["messages"] = []map[string]string{
		{"role": "system", "content": classifierSystemPrompt},
		{"role": "user", "content": renderQuestion(state, q, labels, c.opts.suffix)},
	}
	body["max_tokens"] = c.opts.maxTokens
	body["temperature"] = 0
	body["logprobs"] = true
	body["top_logprobs"] = c.opts.topLogprobs
	body["stream"] = false

	var out chatCompletion
	if err := postJSON(ctx, c.client, logprobsErrPrefix, c.url, c.authorize, c.timeout, body, &out); err != nil {
		return questionResult{key: key, err: err}
	}
	res := questionResult{
		key:    key,
		served: out.Model,
		usage:  Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens},
	}
	if len(out.Choices) == 0 {
		res.err = fmt.Errorf("%s: question %q: the server returned no choices", logprobsErrPrefix, key)
		return res
	}
	choice := out.Choices[0]
	var positions []chatTokenLogprobs
	if choice.Logprobs != nil {
		positions = choice.Logprobs.Content
	}
	dist, hard, err := labelDistribution(positions, choice.Message.Content, labels)
	if err != nil {
		res.err = fmt.Errorf("%s: question %q: %w", logprobsErrPrefix, key, err)
		return res
	}
	if hard {
		res.warning = "the server returned no token probabilities: answers are hard 0/1, not probabilities"
	}
	res.answer = answerFromLabels(q, labels, dist)
	return res
}
