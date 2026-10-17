package providers

import (
	"fmt"
	"slices"
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
	return slices.Contains(validThinkingLevels, level)
}

// StorableThinkingLevelsFor applies provider-aware model classification before
// deciding which tiers may be stored. Codex's GPT-5 family supports the full
// CLI effort ramp even though its versioned model ids would otherwise look like
// legacy models to the provider-neutral classifier.
func StorableThinkingLevelsFor(providerKind, model string) []string {
	class := ThinkingClassForProvider(providerKind, model)
	if class == "effort" {
		// Wider than the offered ramp: the in-between tiers fold onto a real
		// effort level (see coarseEffortStorable).
		return coarseEffortStorable()
	}
	tiers := ThinkingTiersForProvider(providerKind, model)
	if class != "always-on" {
		return tiers
	}
	return append([]string{"off"}, tiers...)
}

// containsTier reports whether a tier ramp carries one token.
func containsTier(tiers []string, want string) bool {
	return slices.Contains(tiers, want)
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
	if slices.Contains(tiers, level) {
		return nil
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
//   - Coarse-effort class (DeepSeek V4.x, GLM-5.3 — see thinking_effort.go):
//     the endpoint ignores budget_tokens, so depth rides output_config.effort
//     (low/high/max) next to the enabled+budget shape; "off" is an explicit
//     {type:"disabled"}, or the lowest effort where reasoning cannot stop.
//   - Everything else (Opus/Sonnet 4.6 and older, Haiku, and non-Claude
//     endpoints that speak the Anthropic protocol such as MiniMax): the legacy
//     enabled+budget shape still applies.

// UsesAdaptiveThinking reports whether the model rejects the legacy
// enabled+budget_tokens thinking shape and requires {type:"adaptive"} +
// output_config.effort instead.
func UsesAdaptiveThinking(model string) bool {
	if AlwaysOnThinking(model) || claude55Tier(model) != "" {
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
// (Fable/Mythos 5 and Opus 5.5). Both the legacy enabled shape and an explicit
// {type:"disabled"} are rejected; the transport keeps adaptive thinking active.
func AlwaysOnThinking(model string) bool {
	return fableModel(model) || claude55Tier(model) == "opus"
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
	if claude55Tier(model) != "" {
		return true
	}
	m := strings.ToLower(model)
	for _, s := range []string{"fable-5-1", "fable-5.1", "mythos-5-1", "mythos-5.1"} {
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
//   - "effort": a three-level low/high/max effort enum (DeepSeek V4.x incl.
//     V4.1 Flash, GLM-5.3 — UsesCoarseEffort); medium/xhigh fold onto the
//     nearest level, and "off" is dropped where reasoning cannot stop (GLM-5.3).
//   - "legacy": a concrete model that reasons but has no distinct xhigh/max wire
//     form, so those clamp down to high (legacy Claude, MiniMax — whose
//     reasoning_effort tops out at "high" — GLM-5.2 and older, …).
//   - "alias": a bare family alias / custom / empty id ("opus", "sonnet",
//     "Varsayılan") whose concrete model is unknown here; offered the full ramp
//     and clamped provider-side.
func ThinkingClass(model string) string {
	switch {
	case AlwaysOnThinking(model):
		return "always-on"
	case UsesAdaptiveThinking(model):
		return "adaptive"
	case UsesCoarseEffort(model):
		// Checked before hasConcreteVersion: "deepseek-flash" carries no digit
		// and would otherwise land in the alias class.
		return "effort"
	case hasConcreteVersion(model):
		return "legacy"
	default:
		return "alias"
	}
}

// ThinkingClassForProvider adds transport knowledge where a model id alone is
// ambiguous. Codex GPT-5/GPT-6 models expose xhigh/max/ultra as real CLI effort
// values; other providers keep the existing adaptive/legacy classification.
// GPT-6 keeps the adaptive class, while ThinkingTiersForProvider applies its
// verified CLI-specific tiers rather than inheriting the generic off/ultra list.
func ThinkingClassForProvider(providerKind, model string) string {
	model = BareModelForKind(providerKind, model)
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

// ThinkingTiersForProvider returns the effective tier ramp for one provider
// transport and model pair. "ultra" is dropped for the Messages-API transports:
// there depth is output_config.effort, whose enum tops out at "max" (see
// EffortForThinkingBudget), so offering it would promise a depth the wire format
// cannot carry.
func ThinkingTiersForProvider(providerKind, model string) []string {
	model = BareModelForKind(providerKind, model)
	if tier := gpt6Tier(model); providerKind == "codex-cli" && tier != "" {
		return codexGPT6ThinkingTiers(tier)
	}
	return thinkingTiersForClass(ThinkingClassForProvider(providerKind, model), model)
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

// thinkingTiersForClass returns the offered ramp for a class. model is consulted
// only by the effort class, whose ramp loses "off" on forced-thinking models.
func thinkingTiersForClass(class, model string) []string {
	switch class {
	case "always-on":
		return []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	case "adaptive", "alias":
		return []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	case "effort":
		return coarseEffortTiers(model)
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
