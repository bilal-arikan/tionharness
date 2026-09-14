package goals

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Built-in starter goals. A fresh workspace ships a board, a starter flow and
// three (disabled) automations but NO goals at all, so the Hedefler screen is
// empty until the user states one in their own words. These seeds give the
// evolution machinery something measurable to attach to from day one without
// pre-empting that conversation: they are provisioned as DRAFTS, so nothing is
// measured or proposed until the user reviews and activates one.
//
// Why these three: every metric below is Available in the catalog today and is
// measurable in ANY workspace — they need neither a recipe nor an enabled board
// automation. Together they cover the three axes the evolver can act on (cost,
// quality, human load), and they guardrail each other: goal 1's guardrails are
// goal 3's primary metric, so "cheaper" cannot be bought by asking the user
// more, and "fewer questions" cannot be bought by charging ahead wrongly.
//
// Deliberately NOT seeded: board.cycleTimeSec (meaningless until the
// board-run-in-progress automation is enabled), recipe.* (a plain chat
// workspace runs no recipes) and anything with Available:false in the catalog
// (judge.rubricScore, feedback.upRatio, …) — a goal on an unmeasured metric
// reports "not measured yet" forever and only clutters the screen.

// defaultGoal describes one built-in starter goal. Seed is the stable identity
// used both to skip re-seeding an existing goal and to record it in the
// deletion ledger.
type defaultGoal struct {
	Seed string
	Make func() db.Goal
}

// floatPtr returns a pointer to a float literal (guardrail bounds are *float64
// so "unset" is distinguishable from zero).
func floatPtr(v float64) *float64 { return &v }

// defaultGoals is the single source of truth for the starter goals every
// workspace gets. Scope is left empty throughout: a seed cannot know which
// recipes or agents a workspace will grow, and empty means the whole workspace.
var defaultGoals = []defaultGoal{
	{
		Seed: "starter-cost-per-session",
		Make: func() db.Goal {
			return db.Goal{
				Name: "Oturum başına maliyeti düşür",
				Description: "Bir oturumun ortalama USD maliyeti azalsın; kalite ve insan yükü bozulmasın. " +
					"Maliyet tek başına bir amaç değildir: daha ucuz bir yapılandırma, işi yarım bırakarak " +
					"ya da kullanıcıya daha çok soru sorarak ucuzluyorsa kazanç sayılmaz — guardrail'ler bunu engeller. " +
					"Başlangıç hedefi; eşikler (hata oranı %10, oturum başına 2 soru) tahminî başlangıç " +
					"değerleridir, workspace'in kendi ölçümleri biriktikçe düzeltilmelidir.",
				Primary: db.GoalMetric{Metric: "usage.costUSDPerSession", Direction: db.GoalDirectionMin},
				Guardrails: []db.GoalGuardrail{
					{Metric: "session.errorTurnsRatio", Max: floatPtr(0.10)},
					{Metric: "session.humanAsksPerSession", Max: floatPtr(2)},
				},
			}
		},
	},
	{
		Seed: "starter-cache-hit-ratio",
		Make: func() db.Goal {
			return db.Goal{
				Name: "Prompt cache isabetini yükselt",
				Description: "cacheRead oranı artsın; oturum maliyeti bunun karşılığında yükselmesin. Yeni workspace'ler " +
					"promptEpoch açık gelir (oturum başındaki statik prompt önekini dondurur), bu hedef de " +
					"o mekanizmanın gerçekten işe yarayıp yaramadığını ölçen sayıdır. " +
					"Başlangıç hedefi; maliyet tavanı ($5/oturum) tahminî bir başlangıç değeridir, " +
					"cache isabeti uğruna maliyetin patlamasını engellemek için vardır.",
				Primary: db.GoalMetric{Metric: "usage.cacheHitRatio", Direction: db.GoalDirectionMax},
				Guardrails: []db.GoalGuardrail{
					{Metric: "usage.costUSDPerSession", Max: floatPtr(5)},
				},
			}
		},
	},
	{
		Seed: "starter-human-load",
		Make: func() db.Goal {
			return db.Goal{
				Name: "İnsan müdahalesini azalt",
				Description: "Oturum başına sorulan soru sayısı azalsın; hata oranı ve takılı döngüler artmasın. " +
					"Guardrail'ler burada taşıyıcıdır: daha az soru sormak, yanlış varsayımla ilerleyerek ya da " +
					"döngüye girerek kolayca 'başarılabilir' — hata oranı ve takılı döngü sayısı bunu yakalar. " +
					"Başlangıç hedefi; eşikler tahminî başlangıç değerleridir, workspace'in kendi " +
					"ölçümleri biriktikçe düzeltilmelidir.",
				Primary: db.GoalMetric{Metric: "session.humanAsksPerSession", Direction: db.GoalDirectionMin},
				Guardrails: []db.GoalGuardrail{
					{Metric: "session.errorTurnsRatio", Max: floatPtr(0.10)},
					{Metric: "session.stuckLoops", Max: floatPtr(1)},
				},
			}
		},
	},
}

// seededGoalsLedgerName is a dotfile at the store root recording which starter
// goal seed keys have ever been provisioned into this workspace. Like the flows
// and automations ledgers, it makes a user deletion permanent: once a key is
// recorded, EnsureDefaultGoals never recreates that goal, even though the row
// is gone.
const seededGoalsLedgerName = ".seeded-goals.json"

type seededGoalsLedger struct {
	Seeded []string `json:"seeded"`
}

// EnsureDefaultGoals provisions the built-in starter goals into a workspace's
// store, idempotently and respecting user deletion — the same contract as
// EnsureDefaultFlows / EnsureDefaultAutomations:
//
//   - If a goal with the same Seed key already exists, it is left untouched.
//   - Else if the deletion ledger records the key, the goal was seeded before
//     and the user deleted it → it is NOT resurrected.
//   - Else the goal is created as a DRAFT and the key is recorded.
//
// open() calls this on every startup, so existing workspaces are backfilled
// with newly shipped starter goals on the next launch.
//
// A seed that fails Validate is skipped WITHOUT being recorded (a malformed
// built-in is a bug caught by the unit test, not something to persist).
func EnsureDefaultGoals(ctx context.Context, database *db.DB, storeDir string) error {
	if database == nil || storeDir == "" {
		return nil
	}
	ledger := loadSeededGoalsLedger(storeDir)
	recorded := map[string]bool{}
	for _, s := range ledger.Seeded {
		recorded[s] = true
	}

	existing, err := database.ListGoals(ctx)
	if err != nil {
		return err
	}
	// Seeded goals are identified by their name: db.Goal carries no Seed field
	// (unlike Automation), and adding one would put a provisioning detail into
	// the user-facing goal shape. The ledger is the durable guard; this check
	// only stops a duplicate inside one workspace.
	present := map[string]bool{}
	for _, g := range existing {
		present[g.Name] = true
	}

	changed := false
	for _, dg := range defaultGoals {
		if recorded[dg.Seed] {
			continue
		}
		g := dg.Make()
		if present[g.Name] {
			// Already there under this name (an older seeding, or the user wrote
			// the same goal themselves). Record it so it is not re-checked.
			ledger.Seeded = append(ledger.Seeded, dg.Seed)
			recorded[dg.Seed] = true
			changed = true
			continue
		}
		g.Status = db.GoalStatusDraft
		g.Policy.Mode = db.GoalModePropose
		g.CreatedBy = db.GoalBySeed
		Normalize(&g)
		if err := Validate(g); err != nil {
			continue
		}
		if _, err := database.CreateGoal(ctx, g, db.GoalBySeed, "başlangıç hedefi olarak eklendi"); err != nil {
			return err
		}
		ledger.Seeded = append(ledger.Seeded, dg.Seed)
		recorded[dg.Seed] = true
		changed = true
	}

	if changed {
		return saveSeededGoalsLedger(storeDir, ledger)
	}
	return nil
}

func loadSeededGoalsLedger(storeDir string) seededGoalsLedger {
	var l seededGoalsLedger
	data, err := os.ReadFile(filepath.Join(storeDir, seededGoalsLedgerName))
	if err != nil {
		return l
	}
	_ = json.Unmarshal(data, &l)
	return l
}

func saveSeededGoalsLedger(storeDir string, l seededGoalsLedger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storeDir, seededGoalsLedgerName), data, 0o644)
}
