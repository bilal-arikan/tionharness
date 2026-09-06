// Package goals holds the evolution goal domain (_Docs/83 §4.1): the closed
// metric catalog a goal may reference, goal validation, and the conversion of a
// goal-writer draft into a stored db.Goal. It is a small leaf over internal/db
// so the api, agent and (later) evolution packages share one definition of
// what a well-formed goal is.
package goals

import "sort"

// Metric is one entry of the closed catalog. Keys are stable identifiers the
// fitness computation (later phase) resolves against existing telemetry; a
// goal may only reference keys from this list.
type Metric struct {
	Key string `json:"key"`
	// Label / Hint are Turkish UI strings (the app's UI language).
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
	// Unit: usd | sec | tokens | count | ratio | score.
	Unit string `json:"unit"`
	// DefaultDirection is the improvement direction the writer should assume
	// unless the user says otherwise.
	DefaultDirection string `json:"defaultDirection"`
	// Source names where the number comes from (recipe stats, usage, board,
	// automation counters, ratings, judge).
	Source string `json:"source"`
	// Scopes lists which scope kinds the metric can be narrowed by.
	Scopes []string `json:"scopes,omitempty"`
	// Available is false while no evaluator exists yet (fitness.go); such a
	// metric can be named in a goal but shows "not measured yet".
	Available bool `json:"available"`
}

// Catalog returns the metric catalog in display order (a copy).
func Catalog() []Metric {
	out := make([]Metric, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns the catalog entry for key.
func Lookup(key string) (Metric, bool) {
	m, ok := byKey[key]
	return m, ok
}

// Keys returns the sorted metric keys (for prompts and error messages).
func Keys() []string {
	out := make([]string, 0, len(byKey))
	for k := range byKey {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

const (
	scopeRecipe     = "recipes"
	scopeAgent      = "agents"
	scopeAutomation = "automations"
	scopeTag        = "tags"
)

var catalog = []Metric{
	// Recipe (trajectory) statistics — internal/trajectory/recipestats.go.
	{Key: "recipe.successRate", Label: "Reçete başarı oranı", Hint: "Tamamlanan koşu / tüm terminal koşular", Unit: "ratio", DefaultDirection: "max", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.avgCostUSD", Label: "Koşu başına maliyet", Hint: "Reçete koşusu başına ortalama USD", Unit: "usd", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.avgDurationSec", Label: "Koşu başına süre", Hint: "Reçete koşusu başına ortalama saniye", Unit: "sec", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.avgTokens", Label: "Koşu başına token", Unit: "tokens", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.avgSessions", Label: "Koşu başına oturum", Hint: "Bir koşuda açılan işçi/oturum sayısı", Unit: "count", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.failedSessions", Label: "Koşu başına başarısız işçi", Unit: "count", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.gateWaitSec", Label: "Kapı bekleme süresi", Hint: "Faz kapılarında (insan/verdict) geçen ortalama saniye", Unit: "sec", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.ghostPhases", Label: "Hayalet faz sayısı", Hint: "İlan edilip hiç başlamayan fazlar", Unit: "count", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},
	{Key: "recipe.unfiredWatchers", Label: "Ateşlenmeyen izleyici", Unit: "count", DefaultDirection: "min", Source: "recipe-stats", Scopes: []string{scopeRecipe}},

	// Usage / cost — internal/db/store_usage.go.
	{Key: "usage.costUSDPerDay", Label: "Günlük maliyet", Unit: "usd", DefaultDirection: "min", Source: "usage", Scopes: []string{scopeAgent}},
	{Key: "usage.cacheHitRatio", Label: "Prompt cache isabet oranı", Hint: "cacheRead / (input + cacheRead + cacheWrite)", Unit: "ratio", DefaultDirection: "max", Source: "usage", Scopes: []string{scopeAgent}},
	{Key: "usage.costUSDPerSession", Label: "Oturum başına maliyet", Unit: "usd", DefaultDirection: "min", Source: "session-usage", Scopes: []string{scopeAgent, scopeTag}},
	{Key: "usage.tokensPerSession", Label: "Oturum başına token", Unit: "tokens", DefaultDirection: "min", Source: "session-usage", Scopes: []string{scopeAgent, scopeTag}},

	// Board outcomes — internal/api/dashboard_outcomes.go.
	{Key: "board.cycleTimeSec", Label: "Kart çevrim süresi", Hint: "Kartın açılıştan kapanışa ortalama süresi", Unit: "sec", DefaultDirection: "min", Source: "board"},
	{Key: "board.cardsDonePerDay", Label: "Günde kapanan kart", Unit: "count", DefaultDirection: "max", Source: "board"},
	{Key: "board.runSuccessRate", Label: "Koşu başarı oranı (pano)", Unit: "ratio", DefaultDirection: "max", Source: "board", Available: false},

	// Automations — Automation counters + automation-fires ledger.
	{Key: "automation.errorRate", Label: "Otomasyon hata oranı", Hint: "Hatalı ateşleme / tüm ateşlemeler", Unit: "ratio", DefaultDirection: "min", Source: "automation", Scopes: []string{scopeAutomation}},
	{Key: "automation.firesPerDay", Label: "Günlük otomasyon ateşlemesi", Unit: "count", DefaultDirection: "min", Source: "automation", Scopes: []string{scopeAutomation}},

	// Self-healing / human load — error classes, asks, gates.
	{Key: "session.errorTurnsRatio", Label: "Hatalı tur oranı", Hint: "Hata sınıfı taşıyan tur / tüm turlar", Unit: "ratio", DefaultDirection: "min", Source: "session", Scopes: []string{scopeAgent, scopeTag}},
	{Key: "session.humanAsksPerSession", Label: "Oturum başına insan sorusu", Hint: "ask_user / onay kapısı sayısı", Unit: "count", DefaultDirection: "min", Source: "session", Scopes: []string{scopeAgent, scopeTag}},
	{Key: "session.stuckLoops", Label: "Takılı döngü sayısı", Unit: "count", DefaultDirection: "min", Source: "session", Scopes: []string{scopeAgent, scopeTag}},

	// Human feedback and judged quality.
	{Key: "feedback.upRatio", Label: "Beğeni oranı", Hint: "Kullanıcının 👍 verdiği tur / puanlanan tur (E5'te ölçülecek)", Unit: "ratio", DefaultDirection: "max", Source: "feedback", Scopes: []string{scopeAgent, scopeTag}, Available: false},
	{Key: "judge.rubricScore", Label: "Rubrik puanı", Hint: "Açık uçlu hedefler için ayrı bağlamda çalışan yargıç puanı (0–1); rubrik metni hedefte tanımlanır (E5'te ölçülecek)", Unit: "score", DefaultDirection: "max", Source: "judge", Scopes: []string{scopeRecipe, scopeAgent, scopeTag}, Available: false},

	// Config size budgets (guardrails against bloat).
	{Key: "config.soulChars", Label: "Ajan soul uzunluğu", Hint: "Karakter; şişmeye karşı guardrail", Unit: "count", DefaultDirection: "min", Source: "config", Scopes: []string{scopeAgent}},
	{Key: "config.toolCount", Label: "Ajan başına görünür araç", Unit: "count", DefaultDirection: "min", Source: "config", Scopes: []string{scopeAgent}, Available: false},
	{Key: "config.skillCount", Label: "Ajan başına skill", Unit: "count", DefaultDirection: "min", Source: "config", Scopes: []string{scopeAgent}},
}

// unavailable names the entries that carry Available: false above; every other
// entry is measurable today and is flagged so at init.
var unavailable = map[string]bool{"board.runSuccessRate": true, "feedback.upRatio": true, "judge.rubricScore": true, "config.toolCount": true}

var byKey = func() map[string]Metric {
	m := make(map[string]Metric, len(catalog))
	for i := range catalog {
		catalog[i].Available = !unavailable[catalog[i].Key]
		m[catalog[i].Key] = catalog[i]
	}
	return m
}()

// AutoApplySurfaces lists the reversible, low-risk surfaces a goal's auto
// policy may name (_Docs/83 §4.5). Everything else always waits for a human.
var AutoApplySurfaces = []string{
	"thinkingLevel",      // lower an agent's thinking level
	"toolVisibility",     // move a tool to summary/name-only/hidden
	"automationCooldown", // raise an automation's cooldown / threshold
	"skillAutoSummary",   // switch a skill to auto-summary
}

// IsAutoApplySurface reports whether s is an allowed auto-apply surface.
func IsAutoApplySurface(s string) bool {
	for _, k := range AutoApplySurfaces {
		if k == s {
			return true
		}
	}
	return false
}
