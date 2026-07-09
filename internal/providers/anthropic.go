package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Anthropic beta feature flags (sent via the anthropic-beta header).
// The 1M-context beta (context-1m-2025-08-07) was retired: Anthropic made the
// 1M window GA at standard pricing on 2026-03-13 (no header needed) and turned
// the beta header off on 2026-04-30, so it is no longer sent.
const (
	betaExtendedCacheTTL  = "extended-cache-ttl-2025-04-11"
	betaContextManagement = "context-management-2025-06-27"
	// betaServerCompaction enables API-native compaction: the server summarizes
	// earlier history into compaction blocks once the prompt nears the trigger
	// (default ~150K tokens); the blocks must be echoed back on later requests
	// (the RawContent verbatim echo handles that within a turn).
	betaServerCompaction = "compact-2026-01-12"
	// betaServerFallback enables the server-side refusal fallback: a request the
	// safety classifiers decline is transparently re-served by the fallback model
	// inside the same call (a decline before output isn't billed; the rescue
	// bills at the fallback model's rates). Per-request, Fable-class models only.
	betaServerFallback = "server-side-fallback-2026-06-01"
)

// refusalFallbackModel is the substitute model for refused Fable-class requests
// — the only supported server-side fallback target at launch.
const refusalFallbackModel = "claude-opus-4-8"

// Server-side web tools. The _20260209 variants carry dynamic filtering (a
// code-execution environment under the hood) and require the 4.6+ model class;
// older models — and PTC turns, which already run their own code-execution
// environment (two would confuse the model) — use the basic variants.
const (
	webSearchDynType   = "web_search_20260209"
	webSearchBasicType = "web_search_20250305"
	webSearchName      = "web_search"
	webFetchDynType    = "web_fetch_20260209"
	webFetchBasicType  = "web_fetch_20250910"
	webFetchName       = "web_fetch"
	// Per-turn use caps: web search is billed per search, so both tools carry a
	// conservative max_uses ceiling rather than an unbounded loop.
	webSearchMaxUses = 8
	webFetchMaxUses  = 12
)

// Context-editing defaults (P3): the server clears old tool_use/tool_result blocks
// from the cached prefix in place (cache_edits) once the prompt grows past the
// trigger, keeping the most recent tool uses — Claude Code's microcompact analogue.
// Conservative floors so short turns are untouched and recent context is preserved.
const (
	contextClearTriggerTokens = 100000 // start clearing past this input size
	contextClearKeepToolUses  = 3      // always keep the newest N tool uses
	contextClearAtLeastTokens = 5000   // minimum tokens to reclaim per clear
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	DefaultModel     = "claude-sonnet-5"
	defaultMaxTokens = 4096
	// Per-request wall-clock budgets (ctx deadlines, retries included). The
	// adaptive class — Fable 5 especially — can legitimately run a SINGLE
	// request for many minutes on hard tasks; the legacy budget would kill it
	// mid-generation and then re-bill it via the transport retry.
	requestTimeoutSecs         = 120
	adaptiveRequestTimeoutSecs = 600
	// clientTimeoutSecs is the http.Client transport safety net, kept above the
	// largest per-request budget so the ctx deadline (model-class aware) is
	// always the binding limit.
	clientTimeoutSecs = adaptiveRequestTimeoutSecs + 30
)

// cacheTTL is the SINGLE ttl used by EVERY prompt-cache breakpoint (tools, static
// system, rolling history). Anthropic requires that a breakpoint's TTL never be
// shorter than one appearing later in the tools → system → messages prefix order,
// so a mixed TTL would silently break caching. Defining it once (P5 hardening)
// makes a mid-request TTL drift impossible: change the policy here, everywhere.
const cacheTTL = "1h"

// Anthropic is a thin client for the Anthropic Messages API.
// It avoids the official SDK to stay dependency-light and version-stable.
//
// The endpoint, default model and provider name are configurable (via
// WithEndpoint) so the same client can drive any Anthropic-compatible host —
// e.g. MiniMax's /anthropic/v1, which accepts the same request shape (tool-use,
// thinking, cache_control, x-api-key auth) and returns native Messages format.
type Anthropic struct {
	apiKey       string
	client       *http.Client
	baseURL      string // full /messages endpoint
	defaultModel string // model applied when a request omits one
	name         string // provider identity reported by Name()

	extendedCache    bool // 1h extended prompt cache TTL beta
	contextEditing   bool // API-native context editing (clear_tool_uses) beta
	serverCompaction bool // API-native compaction (compact_20260112) beta
	refusalFallback  bool // server-side refusal fallback (Fable-class requests only)
}

// NewAnthropic creates a client with the given API key, defaulting to the
// official Anthropic endpoint and model.
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey:       apiKey,
		client:       &http.Client{Timeout: clientTimeoutSecs * time.Second},
		baseURL:      anthropicURL,
		defaultModel: DefaultModel,
		name:         "anthropic",
	}
}

// WithEndpoint points the client at an Anthropic-compatible host. Empty
// arguments keep the current value, so callers can override only what differs.
// messagesURL is the full /messages endpoint (e.g.
// "https://api.minimax.io/anthropic/v1/messages"). Returns the client for
// chaining.
func (a *Anthropic) WithEndpoint(name, messagesURL, defaultModel string) *Anthropic {
	if name != "" {
		a.name = name
	}
	if messagesURL != "" {
		a.baseURL = messagesURL
	}
	if defaultModel != "" {
		a.defaultModel = defaultModel
	}
	return a
}

// WithBetas enables optional Anthropic beta capabilities and returns the client
// for chaining. extendedCache = 1h prompt-cache TTL; contextEditing = API-native
// context editing (server-side clear_tool_uses, the microcompact analogue);
// serverCompaction = API-native compaction (server-side history summarization
// into compaction blocks once the prompt nears the trigger).
func (a *Anthropic) WithBetas(extendedCache, contextEditing, serverCompaction bool) *Anthropic {
	a.extendedCache = extendedCache
	a.contextEditing = contextEditing
	a.serverCompaction = serverCompaction
	return a
}

// WithRefusalFallback toggles the server-side refusal fallback: Fable-class
// requests carry fallbacks=[claude-opus-4-8] + its beta header, so a safety-
// classifier decline is re-served by Opus 4.8 inside the same call instead of
// failing the turn. Anthropic's guidance is to ship Fable code with this on.
func (a *Anthropic) WithRefusalFallback(enabled bool) *Anthropic {
	a.refusalFallback = enabled
	return a
}

// requestCtx bounds one API call (retries included) by model class: adaptive
// models get the long budget (single Fable requests can run for minutes), the
// rest keep the historical 120s. A sooner parent deadline still wins.
func (a *Anthropic) requestCtx(ctx context.Context, model string) (context.Context, context.CancelFunc) {
	d := time.Duration(requestTimeoutSecs) * time.Second
	if UsesAdaptiveThinking(model) {
		d = time.Duration(adaptiveRequestTimeoutSecs) * time.Second
	}
	return context.WithTimeout(ctx, d)
}

// Name implements Provider.
func (a *Anthropic) Name() string { return a.name }

// anthropicReq mirrors the Messages API request body.
type anthropicReq struct {
	Model             string             `json:"model"`
	MaxTokens         int                `json:"max_tokens"`
	System            any                `json:"system,omitempty"` // string, or []systemBlock when caching
	Messages          []anthropicMessage `json:"messages"`
	Tools             []anthropicTool    `json:"tools,omitempty"`
	Thinking          *thinkingParam     `json:"thinking,omitempty"`
	OutputConfig      *outputConfig      `json:"output_config,omitempty"`
	ContextManagement *contextManagement `json:"context_management,omitempty"`
	// Container resumes a code-execution container from a prior response of the
	// same turn (REQUIRED while a programmatic tool call is pending).
	Container string `json:"container,omitempty"`
	// Fallbacks (beta): substitute models that transparently re-serve the
	// request when the safety classifiers decline it (Fable-class only).
	Fallbacks []fallbackParam `json:"fallbacks,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

// contextManagement carries the API-native context-editing directives (P3). The
// server applies them to the CACHED prefix in place — clearing old tool_use/
// tool_result blocks past a size trigger while keeping the newest N — so the warm
// prefix shrinks without a full rewrite (cache_edits), the microcompact analogue.
type contextManagement struct {
	Edits []contextEdit `json:"edits"`
}

type contextEdit struct {
	Type         string            `json:"type"`
	Trigger      *contextThreshold `json:"trigger,omitempty"`
	Keep         *contextThreshold `json:"keep,omitempty"`
	ClearAtLeast *contextThreshold `json:"clear_at_least,omitempty"`
	ExcludeTools []string          `json:"exclude_tools,omitempty"`
}

type contextThreshold struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

// contextMgmt builds the context-management directives from the enabled betas,
// else nil (the field is omitted). Two independent strategies: context editing
// (clear_tool_uses_20250919 with conservative defaults — see the constants) and
// server-side compaction (compact_20260112, server-default trigger ~150K).
func (a *Anthropic) contextMgmt() *contextManagement {
	var edits []contextEdit
	if a.contextEditing {
		edits = append(edits, contextEdit{
			Type:         "clear_tool_uses_20250919",
			Trigger:      &contextThreshold{Type: "input_tokens", Value: contextClearTriggerTokens},
			Keep:         &contextThreshold{Type: "tool_uses", Value: contextClearKeepToolUses},
			ClearAtLeast: &contextThreshold{Type: "input_tokens", Value: contextClearAtLeastTokens},
		})
	}
	if a.serverCompaction {
		edits = append(edits, contextEdit{Type: "compact_20260112"})
	}
	if len(edits) == 0 {
		return nil
	}
	return &contextManagement{Edits: edits}
}

// thinkingParam enables extended reasoning. Adaptive-class models take
// {type:"adaptive"} (budget_tokens is rejected there); legacy models take
// {type:"enabled", budget_tokens:N}.
type thinkingParam struct {
	Type         string `json:"type"`                    // "adaptive" | "disabled" | "enabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // legacy enabled shape only
	Display      string `json:"display,omitempty"`       // "summarized" — adaptive class defaults to "omitted" (empty traces)
}

// outputConfig carries response-level controls; effort steers thinking depth on
// adaptive-class models (the replacement for budget_tokens).
type outputConfig struct {
	Effort     string        `json:"effort,omitempty"` // "low" | "medium" | "high" | "xhigh" | "max"
	TaskBudget *taskBudget   `json:"task_budget,omitempty"`
	Format     *outputFormat `json:"format,omitempty"`
}

// outputFormat constrains the reply to a JSON Schema (structured outputs, GA).
type outputFormat struct {
	Type   string          `json:"type"` // "json_schema"
	Schema json.RawMessage `json:"schema"`
}

// applyOutputSchema folds the request's structured-output schema into the
// output config when the model supports it (no beta header — GA).
func applyOutputSchema(model string, schema json.RawMessage, cfg *outputConfig) *outputConfig {
	if len(schema) == 0 || !SupportsStructuredOutputs(model) {
		return cfg
	}
	if cfg == nil {
		cfg = &outputConfig{}
	}
	cfg.Format = &outputFormat{Type: "json_schema", Schema: schema}
	return cfg
}

// taskBudget is the (beta) agentic-loop token budget: the server shows the model
// a running countdown so it paces thinking/tool use/output against it. A soft,
// model-aware limit — max_tokens stays the enforced per-response ceiling.
// Requires the task-budgets beta header; only adaptive-class models accept it.
type taskBudget struct {
	Type  string `json:"type"` // "tokens"
	Total int    `json:"total"`
}

// minTaskBudgetTokens is the API's minimum accepted task budget; smaller
// configured values are raised to it rather than rejected.
const minTaskBudgetTokens = 20000

// betaTaskBudgets is the per-request beta flag enabling output_config.task_budget.
const betaTaskBudgets = "task-budgets-2026-03-13"

// applyTaskBudget folds the request's task budget into the output config when
// the model supports it, returning the (possibly newly allocated) config and
// whether the task-budgets beta header must be added to this request.
func applyTaskBudget(model string, budget int, cfg *outputConfig) (*outputConfig, bool) {
	if budget <= 0 || !SupportsTaskBudget(model) {
		return cfg, false
	}
	if budget < minTaskBudgetTokens {
		budget = minTaskBudgetTokens
	}
	if cfg == nil {
		cfg = &outputConfig{}
	}
	cfg.TaskBudget = &taskBudget{Type: "tokens", Total: budget}
	return cfg, true
}

// thinkingFor returns the thinking parameter, the optional output_config, and
// the max_tokens to use for the given model.
//
// Adaptive class (UsesAdaptiveThinking — Fable/Mythos 5, Opus 4.7/4.8,
// Sonnet 5): the legacy enabled+budget shape 400s, so the budget is translated
// to {type:"adaptive"} + output_config.effort, with display:"summarized" so the
// thinking trace carries text (these models default to "omitted"). Budget 0 →
// explicit {type:"disabled"}, except always-on models (Fable/Mythos) where
// disabled also 400s and the field is omitted entirely.
//
// Legacy models keep enabled+budget_tokens; max_tokens must be strictly greater
// than the budget, so it is bumped to leave room for the visible answer.
func thinkingFor(model string, budget, maxTokens int) (*thinkingParam, *outputConfig, int) {
	if UsesAdaptiveThinking(model) {
		if budget <= 0 {
			if AlwaysOnThinking(model) {
				return nil, nil, maxTokens
			}
			return &thinkingParam{Type: "disabled"}, nil, maxTokens
		}
		var cfg *outputConfig
		if effort := EffortForThinkingBudget(budget); effort != "" {
			cfg = &outputConfig{Effort: effort}
		}
		return &thinkingParam{Type: "adaptive", Display: "summarized"}, cfg, maxTokens
	}
	if budget <= 0 {
		return nil, nil, maxTokens
	}
	// The xhigh/max tiers exist only as effort levels on the adaptive class;
	// clamp legacy budgets so an "xhigh"/"max" agent on an old model neither
	// blows past family output caps nor sends an absurd budget.
	if budget > 16384 {
		budget = 16384
	}
	if maxTokens <= budget {
		maxTokens = budget + defaultMaxTokens
	}
	return &thinkingParam{Type: "enabled", BudgetTokens: budget}, nil, maxTokens
}

// systemBlock is the structured form of the system prompt, used when extended
// prompt caching is on so a cache_control breakpoint can be attached.
type systemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`          // "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "1h" with the extended-cache beta
}

// anthropicMessage carries an array of content blocks (text / tool_use /
// tool_result), which is the form required once tools are involved. Raw, when
// set, is a verbatim provider-native content array (Message.RawContent) that
// replaces the structured blocks at marshal time — the echo path for server
// blocks (tool search results / server tool use) the block union cannot model.
type anthropicMessage struct {
	Role    string          `json:"role"`
	Content []contentBlock  `json:"content"`
	Raw     json.RawMessage `json:"-"`
}

// MarshalJSON emits the verbatim Raw content when present, else the structured
// blocks. Keeping this at the marshal seam means every builder path (breakpoints,
// dynamic suffix) can keep operating on Content without knowing about Raw.
func (m anthropicMessage) MarshalJSON() ([]byte, error) {
	if len(m.Raw) > 0 {
		return json.Marshal(struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}{m.Role, m.Raw})
	}
	return json.Marshal(struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}{m.Role, m.Content})
}

// contentBlock is a tagged union over the block types we use.
type contentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	// CacheControl marks a rolling cache breakpoint on the conversation history
	// (attached to the last block of the last message). See toAnthropicMessages.
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type anthropicTool struct {
	// Type identifies an Anthropic-defined server tool (e.g. the tool-search
	// tool); empty for ordinary user-defined tools.
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// DeferLoading (native tool search): the def is indexed server-side but kept
	// out of the model's context until discovered via the search tool. Discovered
	// schemas are APPENDED to the prompt, preserving the cached prefix.
	DeferLoading bool `json:"defer_loading,omitempty"`
	// Strict (structured tool use): the API guarantees tool_use inputs validate
	// against InputSchema. Incompatible with allowed_callers (PTC).
	Strict bool `json:"strict,omitempty"`
	// AllowedCallers (programmatic tool calling): which contexts may invoke the
	// tool — ["direct"] and/or ["code_execution_20260120"].
	AllowedCallers []string `json:"allowed_callers,omitempty"`
	// MaxUses caps how many times a server tool (web search/fetch) may run per
	// turn — web search is billed per search, so the ceiling bounds cost.
	MaxUses int `json:"max_uses,omitempty"`
	// CacheControl, when set on the LAST tool, marks a cache breakpoint after the
	// whole tools block. Anthropic caches by prefix in tools → system → messages
	// order, so this caches the tool schemas INDEPENDENTLY of the (possibly
	// changing) system block — a system-prompt edit no longer invalidates the
	// tool-definition cache. Only the last tool carries it (one breakpoint covers
	// the entire preceding block).
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// anthropicResp mirrors the relevant parts of the response body.
type anthropicResp struct {
	Content []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		Thinking string          `json:"thinking"`
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
		// Caller identifies how a tool_use was invoked (PTC): {"type":"direct"}
		// or {"type":"code_execution_20260120","tool_id":"srvtoolu_..."}.
		Caller *struct {
			Type string `json:"type"`
		} `json:"caller"`
		// SubContent is the nested content of server tool-result blocks (e.g.
		// code_execution_tool_result → {stdout, stderr, return_code}).
		SubContent json.RawMessage `json:"content"`
	} `json:"content"`
	// Container is the code-execution container of this response (PTC).
	Container *struct {
		ID string `json:"id"`
	} `json:"container"`
	StopReason string `json:"stop_reason"`
	// StopDetails classifies a refusal (populated only on stop_reason "refusal").
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
	Model string `json:"model"`
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// fallbackParam names one substitute model for the server-side refusal fallback.
type fallbackParam struct {
	Model string `json:"model"`
}

// Complete implements Provider.
func (a *Anthropic) Complete(ctx context.Context, req Request) (*Response, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}

	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	// Model-class-aware wall clock: adaptive models (Fable especially) may run a
	// single request for minutes; legacy models keep the historical budget.
	ctx, cancel := a.requestCtx(ctx, model)
	defer cancel()
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, outCfg, maxTokens := thinkingFor(model, req.ThinkingBudget, maxTokens)
	outCfg, taskBudgetBeta := applyTaskBudget(model, req.TaskBudgetTokens, outCfg)
	outCfg = applyOutputSchema(model, req.OutputSchema, outCfg)
	ptc := req.ProgrammaticTools && SupportsProgrammaticTools(model)

	sysField, msgs := a.buildSystemAndMessages(req, model)
	body := anthropicReq{
		Model:             model,
		MaxTokens:         maxTokens,
		System:            sysField,
		Messages:          msgs,
		Tools:             toAnthropicTools(req.Tools, a.extendedCache, serverToolOpts{ptc: ptc, webTools: req.WebTools, model: model}),
		Thinking:          thinking,
		OutputConfig:      outCfg,
		ContextManagement: a.contextMgmt(),
		Container:         req.ContainerID,
	}

	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	// Server-side refusal fallback: Fable-class requests (always-on classifiers)
	// carry the fallbacks param so a policy decline is transparently re-served by
	// Opus 4.8 inside the same call. First-party endpoint only — protocol
	// lookalikes (minimax-anthropic, custom) would reject the param.
	refusalFallback := a.refusalFallback && a.name == "anthropic" && AlwaysOnThinking(model)
	if refusalFallback {
		body.Fallbacks = []fallbackParam{{Model: refusalFallbackModel}}
	}

	beta := a.betaHeader()
	if taskBudgetBeta {
		beta = joinNonEmptyComma(beta, betaTaskBudgets)
	}
	if refusalFallback {
		beta = joinNonEmptyComma(beta, betaServerFallback)
	}
	if beta != "" {
		headers["anthropic-beta"] = beta
	}

	var parsed anthropicResp
	status, raw, retryAfter, err := postJSON(ctx, a.client, a.name, a.baseURL, headers, body, &parsed)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		// Surface the server's own wait hint so the turn-level retry layer can
		// honor it instead of guessing a backoff (parsed by agent errclass).
		raSuffix := RetryAfterSuffix(retryAfter)
		if parsed.Error != nil {
			msg := parsed.Error.Message
			// Fable 5 requires 30-day data retention: a ZDR/short-retention org
			// gets 400 on EVERY request with a payload-shaped error. Attach the
			// actionable hint so the user doesn't debug the request body.
			if AlwaysOnThinking(model) && strings.Contains(strings.ToLower(msg), "retention") {
				msg += " (hint: Claude Fable 5 requires 30-day data retention; organizations configured for zero/short retention get 400 on every request — check the org's data-retention setting, not the request)"
			}
			return nil, fmt.Errorf("anthropic API error (%s): %s%s", parsed.Error.Type, msg, raSuffix)
		}
		return nil, fmt.Errorf("anthropic HTTP %d: %s%s", status, string(raw), raSuffix)
	}

	var text string
	var calls []ToolCall
	var trace []TraceStep
	for _, c := range parsed.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "thinking":
			// Extended-reasoning block (precedes the answer); surface it as a
			// thinking trace step, mirroring the keyless claude-cli path.
			if c.Thinking != "" {
				trace = append(trace, TraceStep{Kind: "thinking", Text: c.Thinking})
			}
		case "tool_use":
			call := ToolCall{ID: c.ID, Name: c.Name, Input: c.Input}
			if c.Caller != nil {
				call.Caller = c.Caller.Type
			}
			calls = append(calls, call)
		case "server_tool_use":
			// Server-executed tool (native tool search / code execution): nothing
			// to run client-side, but surface it so the UI shows the step.
			trace = append(trace, TraceStep{Kind: "tool", Tool: c.Name, Input: c.Input, Output: "(executed server-side)"})
		case "code_execution_tool_result", "bash_code_execution_tool_result":
			// Completed code run: show stdout/stderr as the step's output.
			trace = append(trace, TraceStep{Kind: "tool", Tool: codeExecToolName, Output: renderCodeExecResult(c.SubContent)})
		case "web_search_tool_result":
			trace = append(trace, TraceStep{Kind: "tool", Tool: webSearchName, Output: renderWebToolResult(c.SubContent, "result")})
		case "web_fetch_tool_result":
			trace = append(trace, TraceStep{Kind: "tool", Tool: webFetchName, Output: renderWebToolResult(c.SubContent, "document")})
		case "compaction":
			// Server-side compaction replaced earlier history with a summary
			// block; surface the event (the block itself rides RawContent).
			trace = append(trace, TraceStep{Kind: "text", Text: "(conversation history compacted server-side)"})
		case "fallback":
			// Refusal fallback switch point: the requested model declined and the
			// fallback model continued inside the same call.
			trace = append(trace, TraceStep{Kind: "text", Text: "(safety refusal — fallback model continued the request)"})
		}
	}

	// Verbatim content array for the tool loop's assistant echo: server blocks
	// (tool search results / server tool use) the typed union above cannot model
	// must ride back exactly as received on subsequent loop iterations.
	var rawEnv struct {
		Content json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(raw, &rawEnv)
	// A mid-output refusal fallback constrains the echo: thinking/tool_use blocks
	// BEFORE the final fallback boundary must be omitted when the content is sent
	// back (text and post-boundary blocks echo normally).
	rawContent := sanitizeFallbackEcho(rawEnv.Content)

	containerID := ""
	if parsed.Container != nil {
		containerID = parsed.Container.ID
	}
	var stopDetails *StopDetails
	if parsed.StopReason == StopRefusal && parsed.StopDetails != nil {
		stopDetails = &StopDetails{Category: parsed.StopDetails.Category, Explanation: parsed.StopDetails.Explanation}
	}

	return &Response{
		Text:        text,
		ToolCalls:   calls,
		RawContent:  rawContent,
		ContainerID: containerID,
		StopDetails: stopDetails,
		StopReason:  parsed.StopReason,
		Model:       parsed.Model,
		Trace:       trace,
		Usage: Usage{
			InputTokens:      parsed.Usage.InputTokens,
			OutputTokens:     parsed.Usage.OutputTokens,
			CacheWriteTokens: parsed.Usage.CacheCreationInputTokens,
			CacheReadTokens:  parsed.Usage.CacheReadInputTokens,
		},
	}, nil
}

// CountTokens implements TokenCounter via POST /v1/messages/count_tokens: the
// server counts the EXACT prompt tokens for this request (system + messages +
// tools) with the target model's real tokenizer. Used to show accurate figures
// next to the local heuristic estimate (the heuristic keeps driving compaction
// so behaviour is unchanged); costs one free HTTP call, no generation.
func (a *Anthropic) CountTokens(ctx context.Context, req Request) (int, error) {
	if a.apiKey == "" {
		return 0, fmt.Errorf("anthropic: missing API key")
	}
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	// Counting is cheap and fast — the legacy budget is always enough.
	ctx, cancel := context.WithTimeout(ctx, requestTimeoutSecs*time.Second)
	defer cancel()
	sysField, msgs := a.buildSystemAndMessages(req, model)
	if len(msgs) == 0 {
		// The endpoint requires a non-empty messages array.
		msgs = []anthropicMessage{{Role: RoleUser, Content: []contentBlock{{Type: "text", Text: "."}}}}
	}
	body := struct {
		Model    string             `json:"model"`
		System   any                `json:"system,omitempty"`
		Messages []anthropicMessage `json:"messages"`
		Tools    []anthropicTool    `json:"tools,omitempty"`
	}{Model: model, System: sysField, Messages: msgs, Tools: toAnthropicTools(req.Tools, false, serverToolOpts{})}

	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	var parsed struct {
		InputTokens int `json:"input_tokens"`
		Error       *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	status, raw, _, err := postJSON(ctx, a.client, a.name, a.baseURL+"/count_tokens", headers, body, &parsed)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		if parsed.Error != nil {
			return 0, fmt.Errorf("anthropic count_tokens (%s): %s", parsed.Error.Type, parsed.Error.Message)
		}
		return 0, fmt.Errorf("anthropic count_tokens HTTP %d: %s", status, string(raw))
	}
	return parsed.InputTokens, nil
}

// Stream implements Streamer via the Messages API with "stream": true. It
// parses the SSE event sequence (message_start → content_block_delta →
// message_delta → message_stop), forwarding each text/thinking chunk to onDelta
// (tagged via StreamDelta.Kind) and accumulating the full text/usage/stop
// reason for the returned Response. When extended reasoning is on, the full
// thinking text is also returned as a thinking TraceStep so it can be persisted.
// Tools are not used on the streaming path (text-only turns).
func (a *Anthropic) Stream(ctx context.Context, req Request, onDelta func(StreamDelta)) (*Response, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	ctx, cancel := a.requestCtx(ctx, model)
	defer cancel()
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	thinking, outCfg, maxTokens := thinkingFor(model, req.ThinkingBudget, maxTokens)

	sysField, msgs := a.buildSystemAndMessages(req, model)
	body := anthropicReq{
		Model:        model,
		MaxTokens:    maxTokens,
		System:       sysField,
		Messages:     msgs,
		Thinking:     thinking,
		OutputConfig: outCfg,
		Stream:       true,
	}
	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if beta := a.betaHeader(); beta != "" {
		headers["anthropic-beta"] = beta
	}

	var sb strings.Builder // visible answer text
	var tb strings.Builder // extended-reasoning (thinking) text
	out := &Response{Model: model, StopReason: StopEndTurn}
	parseErr := error(nil)

	err := postSSE(ctx, a.client, a.name, a.baseURL, headers, body, func(event string, data []byte) bool {
		switch event {
		case "message_start":
			var ev struct {
				Message struct {
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(data, &ev) == nil {
				out.Usage.InputTokens = ev.Message.Usage.InputTokens
				out.Usage.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens
				out.Usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			}
		case "content_block_delta":
			var ev struct {
				Delta struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					Thinking string `json:"thinking"`
				} `json:"delta"`
			}
			if json.Unmarshal(data, &ev) != nil {
				return true
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					sb.WriteString(ev.Delta.Text)
					onDelta(StreamDelta{Kind: DeltaText, Text: ev.Delta.Text})
				}
			case "thinking_delta":
				if ev.Delta.Thinking != "" {
					tb.WriteString(ev.Delta.Thinking)
					onDelta(StreamDelta{Kind: DeltaThinking, Text: ev.Delta.Thinking})
				}
			}
		case "message_delta":
			var ev struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(data, &ev) == nil {
				if ev.Delta.StopReason != "" {
					out.StopReason = ev.Delta.StopReason
				}
				out.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "error":
			var ev struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(data, &ev)
			parseErr = fmt.Errorf("anthropic stream error (%s): %s", ev.Error.Type, ev.Error.Message)
			return false
		case "message_stop":
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	out.Text = sb.String()
	if tb.Len() > 0 {
		out.Trace = []TraceStep{{Kind: "thinking", Text: tb.String()}}
	}
	return out, nil
}

// foldSystemMessages prepares mid-conversation RoleSystem entries for the wire.
// On supporting models (Opus 4.8) they pass through untouched and are emitted
// with role "system". Elsewhere each one's text is folded into the PRECEDING
// user message (as a <system-reminder> block) so role alternation stays valid;
// a system message with no user predecessor downgrades to a plain user turn.
func foldSystemMessages(msgs []Message, native bool) []Message {
	if native {
		return msgs
	}
	hasSystem := false
	for _, m := range msgs {
		if m.Role == RoleSystem {
			hasSystem = true
			break
		}
	}
	if !hasSystem {
		return msgs
	}
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != RoleSystem {
			out = append(out, m)
			continue
		}
		note := "<system-reminder>\n" + strings.TrimSpace(m.Text) + "\n</system-reminder>"
		if n := len(out); n > 0 && out[n-1].Role == RoleUser && len(out[n-1].RawContent) == 0 && !out[n-1].OnlyToolResults {
			prev := &out[n-1]
			if prev.Text == "" {
				prev.Text = note
			} else {
				prev.Text = prev.Text + "\n\n" + note
			}
			continue
		}
		out = append(out, Message{Role: RoleUser, Text: note})
	}
	return out
}

// sanitizeFallbackEcho prepares a response content array for the verbatim echo
// when it contains a refusal-fallback boundary: thinking/redacted_thinking/
// tool_use blocks BEFORE the final "fallback" block must be omitted on replay
// (the fallback model never produced them); everything else — text blocks, the
// fallback marker itself, all post-boundary blocks — echoes unchanged. Content
// without a fallback block is returned byte-identical (the common case).
func sanitizeFallbackEcho(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !strings.Contains(string(raw), `"fallback"`) {
		return raw
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw
	}
	blockType := func(b json.RawMessage) string {
		var t struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(b, &t)
		return t.Type
	}
	last := -1
	for i, b := range blocks {
		if blockType(b) == "fallback" {
			last = i
		}
	}
	if last < 0 {
		return raw // "fallback" appeared only inside a string value
	}
	out := make([]json.RawMessage, 0, len(blocks))
	for i, b := range blocks {
		if i < last {
			switch blockType(b) {
			case "thinking", "redacted_thinking", "tool_use":
				continue
			}
		}
		out = append(out, b)
	}
	merged, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return merged
}

// renderWebToolResult summarizes a web search/fetch result block for the trace:
// a success carries an array (search) or object (fetch) of content; an error
// carries an object with error_code. Kept to a one-line summary — the model has
// the full content in its own context; the trace is for the human.
func renderWebToolResult(sub json.RawMessage, unit string) string {
	if len(sub) == 0 {
		return "(no content)"
	}
	// Error shape: {"type":"...","error_code":"max_uses_exceeded"}.
	var errObj struct {
		ErrorCode string `json:"error_code"`
	}
	if json.Unmarshal(sub, &errObj) == nil && errObj.ErrorCode != "" {
		return "error: " + errObj.ErrorCode
	}
	var items []json.RawMessage
	if json.Unmarshal(sub, &items) == nil {
		return fmt.Sprintf("%d %s(s) (executed server-side)", len(items), unit)
	}
	return "1 " + unit + " (executed server-side)"
}

// renderCodeExecResult flattens a code-execution result block's nested content
// ({stdout, stderr, return_code}) into a readable trace line.
func renderCodeExecResult(sub json.RawMessage) string {
	var r struct {
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		ReturnCode int    `json:"return_code"`
	}
	if len(sub) == 0 || json.Unmarshal(sub, &r) != nil {
		return "(code execution finished)"
	}
	out := strings.TrimSpace(r.Stdout)
	if s := strings.TrimSpace(r.Stderr); s != "" {
		if out != "" {
			out += "\n"
		}
		out += "[stderr] " + s
	}
	if r.ReturnCode != 0 {
		out = strings.TrimSpace(fmt.Sprintf("[exit %d] %s", r.ReturnCode, out))
	}
	if out == "" {
		out = "(no output)"
	}
	return out
}

// joinNonEmptyComma joins the non-empty parts with a comma — for the
// anthropic-beta header, which takes a comma-separated flag list.
func joinNonEmptyComma(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}

// betaHeader builds the comma-separated anthropic-beta header from the enabled
// beta flags ("" when none).
func (a *Anthropic) betaHeader() string {
	var betas []string
	if a.extendedCache {
		betas = append(betas, betaExtendedCacheTTL)
	}
	if a.contextEditing {
		betas = append(betas, betaContextManagement)
	}
	if a.serverCompaction {
		betas = append(betas, betaServerCompaction)
	}
	return strings.Join(betas, ",")
}

// systemField builds the system prompt from a stable static prefix and a
// volatile dynamic suffix. When extended caching is on, the cache_control
// breakpoint (1h TTL) is placed on the static block, so the static prefix — plus
// the tool definitions that precede it in the request — is cached across calls,
// while the dynamic suffix (recalled memory + running summary) that changes every
// turn stays outside the cached prefix and never invalidates it. Without caching
// the two parts are concatenated into a plain string.
// buildSystemAndMessages assembles the system field and message list for one
// request, placing the rolling summary (req.Summary) for maximum cache reuse.
// With caching ON the summary rides a synthetic head user message INSIDE the
// cached prefix (before the rolling history breakpoint) so it is a cache READ
// between folds, while the volatile dynamic trails AFTER the breakpoint. With
// caching OFF there is no cached prefix to protect, so both the summary and the
// dynamic fold back into the system prompt (byte-parity with the pre-cache path).
func (a *Anthropic) buildSystemAndMessages(req Request, model string) (any, []anthropicMessage) {
	if a.extendedCache {
		msgs := prependSummaryMessage(req.Messages, req.Summary)
		return a.systemField(req.System, req.SystemDynamic), toAnthropicMessages(msgs, true, req.SystemDynamic, model)
	}
	dyn := joinNonEmpty(req.SystemDynamic, req.Summary)
	return a.systemField(req.System, dyn), toAnthropicMessages(req.Messages, false, dyn, model)
}

func (a *Anthropic) systemField(static, dynamic string) any {
	static = strings.TrimSpace(static)
	dynamic = strings.TrimSpace(dynamic)
	if static == "" && dynamic == "" {
		return nil
	}
	if !a.extendedCache {
		// Caching off: placement is irrelevant, keep the historical single-block
		// concatenation (static + dynamic) so the non-cached path is unchanged.
		return strings.TrimSpace(static + "\n\n" + dynamic)
	}

	// Caching ON: the system field is STATIC-ONLY so it is fully cacheable. The
	// volatile dynamic suffix is NOT placed here — it moves to a trailing block on
	// the last message (see toAnthropicMessages), i.e. AFTER the rolling history
	// breakpoint, so it never invalidates the cached prefix. This lets tools +
	// system + the whole message history all become cache READS turn-to-turn
	// (previously the dynamic sat upstream of tools/messages and busted both every
	// turn — only the static system prefix ever hit). Mirrors the claude-cli path,
	// which already weaves the dynamic into the last user message. When there is no
	// static prefix, system is nil (dynamic still rides the messages).
	if static == "" {
		return nil
	}
	return []systemBlock{{Type: "text", Text: static, CacheControl: &cacheControl{Type: "ephemeral", TTL: cacheTTL}}}
}

// toAnthropicMessages converts provider messages to content-block form,
// skipping the system role (passed separately in the Anthropic API). When
// extendedCache is on, a rolling cache breakpoint (1h TTL) is attached to the
// last block of the last message so the ENTIRE conversation prefix up to the
// current turn is cached — the biggest lever on a long session, where the raw
// transcript (not the static system/tools) dominates input tokens. Anthropic
// caches by prefix in tools → system → messages order and allows up to 4
// breakpoints; tools(1) + system-static(1) + a hedge on the previous message(1)
// + this rolling one(1) uses exactly that budget. On turn N the breakpoint
// marks the prefix as a cache write; on turn N+1 that same prefix is a cache
// read (0.10×) and the new breakpoint moves forward to the newest message — the
// standard "sliding breakpoint" pattern. Gated on the same extendedCache flag
// as the system/tool breakpoints so the caching on/off policy stays unified.
func toAnthropicMessages(msgs []Message, extendedCache bool, dynamic, model string) []anthropicMessage {
	// Mid-conversation system messages: Opus 4.8 accepts {"role":"system"}
	// entries natively (the cache-safe, non-spoofable operator channel). Older
	// models 400 on them, so the fallback folds the text into the PRECEDING user
	// message as a <system-reminder> block (keeps role alternation intact); a
	// system message with no user predecessor downgrades to a user turn.
	msgs = foldSystemMessages(msgs, SupportsSystemInMessages(model))
	// A trailing message answering a PROGRAMMATIC tool batch must contain pure
	// tool_result blocks — no dynamic-suffix text may be appended to it.
	pureTail := len(msgs) > 0 && msgs[len(msgs)-1].OnlyToolResults
	out := make([]anthropicMessage, 0, len(msgs))
	// Anthropic requires strictly alternating roles, so back-to-back same-role
	// plain-text turns (e.g. two agents' replies in a shared thread) must merge
	// into one message. The merge is BLOCK-WISE: the later turn rides as an extra
	// text block on the earlier message, so the earlier blocks' bytes stay
	// identical and a cached prefix ending on that message keeps hitting. (A
	// text-level merge would rewrite the cached block and bust the prefix from
	// that point. minimax keeps the text-level coalescePlainSameRole — its chat
	// format has no content blocks and no prefix cache to protect.)
	lastPlain := false
	for _, m := range msgs {
		// Verbatim echo: an assistant turn captured from a prior response in this
		// tool loop carries the exact content array (incl. server-tool blocks the
		// union below cannot model) and must go back byte-identical.
		if len(m.RawContent) > 0 {
			out = append(out, anthropicMessage{Role: m.Role, Raw: m.RawContent})
			lastPlain = false
			continue
		}
		plain := len(m.ToolCalls) == 0 && len(m.ToolResults) == 0
		if plain && lastPlain && len(out) > 0 && out[len(out)-1].Role == m.Role {
			if m.Text != "" {
				prev := &out[len(out)-1]
				// A placeholder empty text block (from an all-empty message) is
				// filled in place; anything else gets a fresh trailing block.
				if n := len(prev.Content); n > 0 && prev.Content[n-1].Type == "text" && prev.Content[n-1].Text == "" {
					prev.Content[n-1].Text = m.Text
				} else {
					prev.Content = append(prev.Content, contentBlock{Type: "text", Text: m.Text})
				}
			}
			continue
		}
		var blocks []contentBlock
		if m.Text != "" {
			blocks = append(blocks, contentBlock{Type: "text", Text: m.Text})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, contentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
		}
		for _, tr := range m.ToolResults {
			blocks = append(blocks, contentBlock{
				Type:      "tool_result",
				ToolUseID: tr.CallID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, contentBlock{Type: "text", Text: ""})
		}
		out = append(out, anthropicMessage{Role: m.Role, Content: blocks})
		lastPlain = plain
	}
	if extendedCache {
		// Rolling history breakpoint: mark the last block of the last (persisted)
		// message so tools + system + the whole conversation prefix is cached. On
		// turn N this prefix is a cache write; on N+1 the same prefix is a cache
		// read and the breakpoint slides forward to the newest turn. A verbatim
		// (Raw) last message cannot carry a breakpoint — that only occurs on a
		// pause_turn resume, where the prior turn's breakpoint still serves reads.
		if len(out) > 0 {
			last := &out[len(out)-1]
			if n := len(last.Content); n > 0 && len(last.Raw) == 0 {
				last.Content[n-1].CacheControl = &cacheControl{Type: "ephemeral", TTL: cacheTTL}
			}
		}
		// Hedge breakpoint: ALSO mark the newest PRIOR message that can carry one.
		// Anthropic's cache lookup only scans ~20 content blocks back from a
		// breakpoint; a call that appends a bigger batch (many parallel tool_use /
		// tool_result blocks) would leave the previous call's cached prefix beyond
		// that horizon and re-WRITE the whole history. The hedge sits at (or very
		// near) the previous call's breakpoint position, so the old prefix is found
		// there and only the new tail is written. Raw (verbatim-echo) messages
		// cannot carry markers and are skipped — the walk lands on the newest
		// non-Raw predecessor instead (typically the previous user/tool_result
		// message, exactly where the previous breakpoint sat). Budget: tools(1) +
		// system(1) + hedge(1) + rolling(1) = 4, the API maximum.
		for i := len(out) - 2; i >= 0; i-- {
			prev := &out[i]
			if n := len(prev.Content); n > 0 && len(prev.Raw) == 0 {
				prev.Content[n-1].CacheControl = &cacheControl{Type: "ephemeral", TTL: cacheTTL}
				break
			}
		}
		// Volatile dynamic (date/time, recalled memory, running summary, …) rides as
		// a trailing text block on the last message — AFTER the breakpoint above, so
		// it stays OUTSIDE the cached prefix. It is request-time only (never
		// persisted), so the cached prefix is byte-identical turn-to-turn and hits;
		// the dynamic block's absence in the next turn's rebuilt history is
		// irrelevant because it was never part of the cached prefix. When there are
		// no messages yet, seed one so the dynamic is not dropped.
		if d := strings.TrimSpace(dynamic); d != "" && !pureTail {
			if len(out) == 0 {
				out = append(out, anthropicMessage{Role: RoleUser, Content: []contentBlock{{Type: "text", Text: d}}})
			} else if last := &out[len(out)-1]; len(last.Raw) == 0 {
				// A verbatim (Raw) last message must stay byte-identical — on that
				// rare pause_turn resume the dynamic is simply skipped for one call.
				last.Content = append(last.Content, contentBlock{Type: "text", Text: d})
			}
		}
	}
	return out
}

// toAnthropicTools converts the tool defs and, when caching is on, attaches a
// cache breakpoint to the LAST tool so the whole tools block is cached on its own
// prefix (independent of the system block). The breakpoint uses the same 1h TTL as
// the system block — valid because tools precede system in Anthropic's ordering,
// so a 1h tool breakpoint never lands after a shorter-TTL one. Gated on the same
// extendedCache flag as the system breakpoint, so the caching on/off policy is
// unchanged; only its granularity improves.
// serverToolOpts selects which Anthropic server tools ride the request and how.
type serverToolOpts struct {
	ptc      bool   // programmatic tool calling (code execution + allowed_callers)
	webTools bool   // server-side web search + web fetch
	model    string // resolved model — picks dynamic vs basic web tool variants
}

func toAnthropicTools(tools []ToolDef, extendedCache bool, opts serverToolOpts) []anthropicTool {
	if len(tools) == 0 && !opts.webTools {
		return nil
	}
	out := make([]anthropicTool, 0, len(tools)+4)
	deferred := false
	for _, t := range tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		deferred = deferred || t.DeferLoading
		at := anthropicTool{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  schema,
			DeferLoading: t.DeferLoading,
			Strict:       t.Strict,
		}
		// PTC: mark code-callable defs; strict is incompatible with
		// allowed_callers, so it is dropped on those.
		if opts.ptc && t.CodeCallable {
			at.AllowedCallers = []string{codeExecToolType}
			at.Strict = false
		}
		out = append(out, at)
	}
	// Server tools LEAD the list (stable prefix); the rolling cache breakpoint
	// stays on the last USER tool, never a server-tool entry. Assembled in one
	// slice so their relative order is fixed: search → code exec → web.
	var servers []anthropicTool
	// Native tool search: any deferred def requires the search server tool in the
	// same request (deferred tools are unreachable without it, and an all-deferred
	// list is a 400 — the eager set is always non-deferred, so that cannot occur).
	if deferred {
		servers = append(servers, anthropicTool{Type: nativeToolSearchType, Name: nativeToolSearchName})
	}
	// Programmatic tool calling requires the code-execution tool in the request.
	if opts.ptc {
		servers = append(servers, anthropicTool{Type: codeExecToolType, Name: codeExecToolName})
	}
	// Web search/fetch: the dynamic (_20260209) variants require the 4.6+ class
	// AND run their own code-execution environment under the hood, so PTC turns
	// (which already carry one) fall back to the basic variants too.
	if opts.webTools {
		searchType, fetchType := webSearchBasicType, webFetchBasicType
		if !opts.ptc && SupportsDynamicWebTools(opts.model) {
			searchType, fetchType = webSearchDynType, webFetchDynType
		}
		servers = append(servers,
			anthropicTool{Type: searchType, Name: webSearchName, MaxUses: webSearchMaxUses},
			anthropicTool{Type: fetchType, Name: webFetchName, MaxUses: webFetchMaxUses},
		)
	}
	out = append(servers, out...)
	// The rolling breakpoint rides the last USER tool only — when the list is
	// all server tools (web tools with no user defs) it is skipped rather than
	// attached to a server-tool entry.
	if extendedCache && len(tools) > 0 {
		out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral", TTL: cacheTTL}
	}
	return out
}

// Native (server-side) tool search: the model discovers deferred tool defs by
// regex search; discovered schemas are appended without invalidating the cached
// prefix. Distinct from TionSwarm's own builtin `tool_search` (the client-side
// catalog search), which stays available alongside.
const (
	nativeToolSearchType = "tool_search_tool_regex_20251119"
	nativeToolSearchName = "tool_search_tool_regex"
)

// Code execution server tool (programmatic tool calling): Claude writes Python
// that calls code-callable tools as async functions inside the Anthropic-hosted
// container; only the script's final output re-enters the model's context.
const (
	codeExecToolType = "code_execution_20260120"
	codeExecToolName = "code_execution"
)
