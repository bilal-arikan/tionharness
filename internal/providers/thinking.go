package providers

import (
	"fmt"
	"strings"
)

// validThinkingLevels is the single definition of the reasoning-tier tokens the
// system accepts, in ramp order. Everything that stores or requests a level
// (agent rows, the agent API, the per-turn chat override) validates against
// this list instead of carrying its own copy.
//
// The empty string is deliberately NOT a member. It used to be stored as a
// third state next to "off" and meant two different things at runtime — no
// thinking on the native API path, but "high" effort on the CLI path — so it is
// now rejected at write time and backfilled for legacy rows (see
// db.BackfillThinkingLevels).
var validThinkingLevels = []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}

// ValidThinkingLevels returns the accepted reasoning-tier tokens in ramp order.
func ValidThinkingLevels() []string {
	out := make([]string, len(validThinkingLevels))
	copy(out, validThinkingLevels)
	return out
}

// IsValidThinkingLevel reports whether level is one of the accepted tokens. The
// empty string is not.
func IsValidThinkingLevel(level string) bool {
	for _, v := range validThinkingLevels {
		if v == level {
			return true
		}
	}
	return false
}

// StorableThinkingLevels returns the tiers that may legally be STORED for a
// model, which is not the same question as ThinkingTiersFor (what the picker
// offers as an effective choice). The two differ on the always-on class: there
// "off" does not disable anything — it means "omit the thinking field", which is
// exactly what the always-on wire format requires — so it is a harmless, fully
// representable stored state even though the picker keeps it greyed out to avoid
// promising the user that reasoning can be turned off. Legacy rows migrated from
// a native provider carry "off" (db.LegacyThinkingLevelFor is provider-based, not
// model-based), and rejecting it would make those agents unsaveable.
func StorableThinkingLevels(model string) []string {
	tiers := ThinkingTiersFor(model)
	if ThinkingClass(model) != "always-on" {
		return tiers
	}
	return append([]string{"off"}, tiers...)
}

// StorableThinkingLevelsFor applies provider-aware model classification before
// deciding which tiers may be stored. Codex's GPT-5 family supports the full
// CLI effort ramp even though its versioned model ids would otherwise look like
// legacy models to the provider-neutral classifier.
func StorableThinkingLevelsFor(providerKind, model string) []string {
	class := ThinkingClassForProvider(providerKind, model)
	tiers := ThinkingTiersForProvider(providerKind, model)
	// The Messages-API transports do not OFFER "ultra" (the effort enum stops at
	// "max"), but an agent may already store it — it was picked on a CLI provider,
	// or the provider was switched afterwards. Rejecting it would make those rows
	// unsaveable, so it stays a legal stored state that maps to the ceiling.
	if usesNativeEffort(providerKind) && containsTier(thinkingTiersForClass(class), "ultra") {
		tiers = append(tiers, "ultra")
	}
	if class != "always-on" {
		return tiers
	}
	return append([]string{"off"}, tiers...)
}

// containsTier reports whether a tier ramp carries one token.
func containsTier(tiers []string, want string) bool {
	for _, t := range tiers {
		if t == want {
			return true
		}
	}
	return false
}

// ValidateThinkingLevel checks a requested reasoning level twice: that the token
// itself is known, and that it is a legal stored value on the given model per
// StorableThinkingLevels. A tier outside that set would be a silent no-op, so it
// is reported as an error rather than accepted and ignored. Bare family aliases
// and an empty model id land in the "alias" class, which offers the full ramp —
// nothing is rejected there.
func ValidateThinkingLevel(model, level string) error {
	return ValidateThinkingLevelForProvider("", model, level)
}

// ValidateThinkingLevelForProvider validates a reasoning tier with transport
// context. Provider-neutral callers retain the historical model-only behavior;
// Codex callers can distinguish versioned GPT-5 models from legacy transports.
func ValidateThinkingLevelForProvider(providerKind, model, level string) error {
	if level == "" {
		return fmt.Errorf("thinkingLevel is required (one of: %s)", strings.Join(validThinkingLevels, ", "))
	}
	if !IsValidThinkingLevel(level) {
		return fmt.Errorf("unknown thinkingLevel %q (one of: %s)", level, strings.Join(validThinkingLevels, ", "))
	}
	tiers := StorableThinkingLevelsFor(providerKind, model)
	for _, t := range tiers {
		if t == level {
			return nil
		}
	}
	return fmt.Errorf("thinkingLevel %q is not supported by model %q (supported: %s)",
		level, model, strings.Join(tiers, ", "))
}

// Thinking wire formats on the Anthropic Messages API differ by model class:
//
//   - Adaptive class (Fable/Mythos 5, Opus 4.7/4.8, Sonnet 5): the legacy
//     {type:"enabled", budget_tokens:N} shape is REMOVED and returns 400.
//     Thinking is requested as {type:"adaptive"} and depth is steered with
//     output_config.effort. Within this class Fable/Mythos are always-on:
//     an explicit {type:"disabled"} also 400s, so "off" omits the field.
//   - Everything else (Opus/Sonnet 4.6 and older, Haiku, and non-Claude
//     endpoints that speak the Anthropic protocol such as MiniMax): the legacy
//     enabled+budget shape still applies.

// UsesAdaptiveThinking reports whether the model rejects the legacy
// enabled+budget_tokens thinking shape and requires {type:"adaptive"} +
// output_config.effort instead.
func UsesAdaptiveThinking(model string) bool {
	if AlwaysOnThinking(model) {
		return true
	}
	m := strings.ToLower(model)
	for _, s := range []string{"opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "sonnet-5"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// AlwaysOnThinking reports whether the model runs with thinking permanently on
// (the Fable/Mythos 5 class): both the legacy enabled shape AND an explicit
// {type:"disabled"} return 400 there, so "off" must omit the thinking field
// entirely (the server thinks anyway).
func AlwaysOnThinking(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "fable") || strings.Contains(m, "mythos")
}

// SupportsTaskBudget reports whether the model accepts the (beta) task-budget
// directive (output_config.task_budget + the task-budgets beta header): the
// model sees a running token countdown for the whole agentic loop and paces
// itself. Supported on exactly the adaptive-thinking class (Fable/Mythos 5,
// Opus 4.7/4.8, Sonnet 5).
func SupportsTaskBudget(model string) bool { return UsesAdaptiveThinking(model) }

// SupportsStructuredOutputs reports whether the model accepts
// output_config.format (JSON-schema-constrained replies). Per the capability
// matrix: Fable/Mythos 5, Opus 4.8, Sonnet 5, Haiku 4.5, plus legacy Opus
// 4.5/4.1 — notably NOT Opus 4.6/4.7 or Sonnet 4.6.
func SupportsStructuredOutputs(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{"opus-4-8", "opus-4.8", "sonnet-5", "haiku-4-5", "haiku-4.5", "opus-4-5", "opus-4.5", "opus-4-1", "opus-4.1"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsSystemInMessages reports whether the model accepts mid-conversation
// {"role":"system"} entries in the messages array (the cache-safe, non-spoofable
// operator channel): Claude Opus 4.8, Opus 5 and the Fable/Mythos 5.x class —
// NOT Sonnet 5 (folded into the preceding user turn there).
func SupportsSystemInMessages(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{"opus-4-8", "opus-4.8", "opus-5"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsThinkingBinding reports whether the model enforces "preserved
// thinking" (Claude Fable 5.1 / Mythos 5.1): a thinking block's signature is
// bound to the conversation prefix that produced it, so any edit to an earlier
// turn — a moved dynamic block, a pruned tool result, an activated tool schema,
// an in-flight compaction — invalidates every later block and returns 400 on
// enforced organizations. The anthropic client sends the binding controls
// (block_binding.prefix_mismatch_behavior = drop_block + its beta) for exactly
// this class so such edits degrade to a re-plan instead of failing the turn.
func SupportsThinkingBinding(model string) bool {
	m := strings.ToLower(model)
	for _, s := range []string{"fable-5-1", "fable-5.1", "mythos-5-1", "mythos-5.1"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsDynamicWebTools reports whether the model accepts the _20260209 web
// search/fetch variants (dynamic filtering): the Claude 4.6+ class — Opus
// 4.6/4.7/4.8, Sonnet 4.6, Sonnet 5, Fable/Mythos. Older models use the basic
// variants instead.
func SupportsDynamicWebTools(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{
		"opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8",
		"sonnet-4-6", "sonnet-4.6", "sonnet-5",
	} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsProgrammaticTools reports whether the model supports programmatic
// tool calling (code_execution_20260120 + allowed_callers): Claude Opus 4.5+
// and Sonnet 4.5+ (incl. Sonnet 5) and the Fable/Mythos class.
func SupportsProgrammaticTools(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{
		"opus-4-5", "opus-4.5", "opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8",
		"sonnet-4-5", "sonnet-4.5", "sonnet-4-6", "sonnet-4.6", "sonnet-5",
	} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// ThinkingClass classifies how a model handles extended reasoning. It is the
// single classifier the UI reads twice: ThinkingTiersFor gates which tier
// buttons are active, and the client uses the class to explain WHY an inactive
// tier is inactive. One of:
//
//   - "always-on": thinking cannot be disabled (Fable/Mythos) — {type:"disabled"}
//     400s and "off" merely omits the field, so "off" is dropped.
//   - "adaptive": the full effort ramp incl. xhigh/max, reaching the model as
//     output_config.effort (Opus 4.7/4.8, Sonnet 5).
//   - "non-thinking": the model does not reason at all, so only "off" is
//     meaningful (DeepSeek V4 Flash — "düşünmeyen mod").
//   - "legacy": a concrete model that reasons but has no distinct xhigh/max wire
//     form, so those clamp down to high (legacy Claude, MiniMax — whose
//     reasoning_effort tops out at "high" — DeepSeek Pro, …).
//   - "alias": a bare family alias / custom / empty id ("opus", "sonnet",
//     "Varsayılan") whose concrete model is unknown here; offered the full ramp
//     and clamped provider-side.
func ThinkingClass(model string) string {
	switch {
	case AlwaysOnThinking(model):
		return "always-on"
	case UsesAdaptiveThinking(model):
		return "adaptive"
	case isNonThinking(model):
		return "non-thinking"
	case hasConcreteVersion(model):
		return "legacy"
	default:
		return "alias"
	}
}

// ThinkingClassForProvider adds transport knowledge where a model id alone is
// ambiguous. Codex GPT-5/GPT-6 models expose xhigh/max/ultra as real CLI effort
// values; other providers keep the existing adaptive/legacy classification.
// GPT-6 Astra documents low/medium/high/xhigh plus max (Responses API), so it
// lands in the same ramp as the GPT-5.6 tiers — note Astra drops "none", which
// this class never emitted for Codex anyway.
func ThinkingClassForProvider(providerKind, model string) string {
	if providerKind == "codex-cli" && codexEffortModel(model) {
		return "adaptive"
	}
	return ThinkingClass(model)
}

// codexEffortModel reports whether a Codex CLI slug takes the real reasoning
// effort ramp. Matched on the generation prefix rather than a bare "gpt" so an
// older 4.x slug typed into the custom-model field keeps its legacy tiers.
func codexEffortModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "gpt-5") || strings.HasPrefix(m, "gpt-6")
}

// isNonThinking reports whether the model has no extended-reasoning mode at all,
// so every tier but "off" is a no-op. DeepSeek's V4 Flash tier is the sole
// current case; its Pro sibling reasons and is left in the "legacy" class.
func isNonThinking(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "deepseek") && strings.Contains(m, "flash")
}

// ThinkingTiersFor returns the reasoning tiers a model meaningfully supports, as
// the stable tokens the UI pickers use: "off","low","medium","high","xhigh",
// "max","ultra". The set is derived from ThinkingClass so the composer / agent
// pickers can grey out tiers that would be a silent no-op on the selected model
// (they are shown disabled with a reason, not hidden).
func ThinkingTiersFor(model string) []string {
	return thinkingTiersForClass(ThinkingClass(model))
}

// ThinkingTiersForProvider returns the effective tier ramp for one provider
// transport and model pair. "ultra" is dropped for the Messages-API transports:
// there depth is output_config.effort, whose enum tops out at "max" (see
// EffortForThinkingBudget), so offering it would promise a depth the wire format
// cannot carry.
func ThinkingTiersForProvider(providerKind, model string) []string {
	tiers := thinkingTiersForClass(ThinkingClassForProvider(providerKind, model))
	if usesNativeEffort(providerKind) {
		tiers = withoutTier(tiers, "ultra")
	}
	return tiers
}

// usesNativeEffort reports whether a provider kind sends its turns over the
// Anthropic Messages API, where reasoning depth rides output_config.effort
// rather than a CLI effort flag. Both kinds share the anthropic.go request
// builder and therefore the same effort enum.
func usesNativeEffort(providerKind string) bool {
	switch providerKind {
	case "anthropic", "anthropic-compat":
		return true
	default:
		return false
	}
}

// withoutTier returns tiers with one token removed, leaving the input untouched.
func withoutTier(tiers []string, drop string) []string {
	out := make([]string, 0, len(tiers))
	for _, t := range tiers {
		if t != drop {
			out = append(out, t)
		}
	}
	return out
}

func thinkingTiersForClass(class string) []string {
	switch class {
	case "always-on":
		return []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	case "adaptive", "alias":
		return []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	case "non-thinking":
		return []string{"off"}
	default: // legacy
		return []string{"off", "low", "medium", "high"}
	}
}

// hasConcreteVersion reports whether the model id carries a version digit, i.e.
// it names a concrete model rather than a bare family alias ("opus", "sonnet").
// Bare aliases resolve to whatever the provider currently points them at (often
// the adaptive flagship), so they get the full ramp rather than the clamped
// legacy set.
func hasConcreteVersion(model string) bool {
	return strings.ContainsAny(model, "0123456789")
}

// EffortForThinkingBudget maps a legacy thinking token budget (as produced by
// the agent's ThinkingLevel) to the output_config.effort value used by
// adaptive-class models. 0 means "no override" (server default). The xhigh/max
// tiers exist only on the adaptive class; the legacy enabled+budget path clamps
// those budgets down instead (see thinkingFor).
//
// The ceiling is "max" on purpose: the Messages API effort enum is
// low|medium|high|xhigh|max, so the "ultra" tier — real on the CLI transports
// (climcp, codexcli) — has no wire representation here and an "ultra" budget
// reports the top of the enum. ThinkingTiersForProvider keeps "ultra" off the
// offered ramp for those provider kinds so the mapping is never a silent
// downgrade of a tier the user was told they could pick.
func EffortForThinkingBudget(budget int) string {
	switch {
	case budget <= 0:
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	case budget <= 16384:
		return "high"
	case budget <= 32768:
		return "xhigh"
	case budget <= 65536:
		return "max"
	default: // "ultra" (131072) and anything above: the enum stops at "max".
		return "max"
	}
}
