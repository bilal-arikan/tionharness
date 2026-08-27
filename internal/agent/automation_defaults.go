package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// defaultAutomation describes a built-in automation shipped into every workspace
// store by EnsureDefaultAutomations. Together the seeded rules make the kanban
// board a source of EXECUTION in any workspace, out of the box: moving a card
// into "in_progress" starts an agent on it, and a finished card is archived off
// the board with no LLM call. Seed is the stable identity used both to skip
// re-seeding an existing default and to record it in the deletion ledger.
//
// They are seeded DISABLED on purpose: auto-spawning an agent on every card move
// is a real behaviour/cost change that should be an explicit, per-workspace opt-in
// (one toggle in the Automations screen), not a silent default in, say, a plain
// single-agent chat workspace. Seeding-yet-disabled means the wiring is present
// everywhere and activating "board-driven execution" is a single click.
//
// AgentSystemKey pins the rule to a specific BUILT-IN agent instead of the
// workspace's first agent: a rule that only makes sense with one system agent
// (the insight applier) must not be seeded onto an arbitrary chat agent. When the
// key resolves to no live agent the rule is skipped WITHOUT being recorded, so a
// later startup backfills it.
type defaultAutomation struct {
	Seed           string
	AgentSystemKey string
	Make           func(agentID string) db.Automation
}

// defaultAutomations is the single source of truth for the built-in automations
// every workspace gets. A rule without AgentSystemKey gets the workspace's first
// agent at seed time (empty when there is none yet — the user assigns one later,
// exactly like a seeded flow's agent nodes).
var defaultAutomations = []defaultAutomation{
	{
		Seed: "board-run-in-progress",
		Make: func(agentID string) db.Automation {
			return db.Automation{
				Name:          "▶️ Kartı çalışmaya al → ajanı başlat",
				TriggerKind:   db.TriggerBoard,
				BoardOp:       db.BoardOpMove,
				BoardToState:  db.BoardInProgress,
				BoardAction:   db.BoardActionSpawn,
				TargetAgentID: agentID,
				PromptTemplate: "Bu pano kartı üzerinde çalış ve bitir.\n\n" +
					"Kart: {{title}} ({{taskId}})\n\n" +
					"Kartın açıklaması/prompt'u kaynaktır — oku, işi yap, ilgili testleri/kontrolleri çalıştır, " +
					"sonra sonucu özetle. İş bitince kartı 'İnceleme' (review) sütununa taşı.\n\n" +
					"Kart kapsamı dışında eksik veya hatalı bir şey görürsen mevcut işi durdurma. Önce list_tasks ile " +
					"aynı konuyu taşıyan açık bir kart olup olmadığını kontrol et. Yoksa bulgu için ayrı bir kart aç: " +
					"boardState: \"pbi\" ve tags: [\"scope-out\", \"parent:{{taskId}}\"]. Soy bağını dependencies ile " +
					"kurma; dependencies alanı kartı bağımlı olduğu kart bitene kadar bloke eder, çoğu kapsam dışı bulgu " +
					"ise bağımsız çalışabilir. Soy bağı için yalnız parent:{{taskId}} etiketini kullan. Bu kartın kendi işini " +
					"bitirmeden yan bulgunun peşine düşme.",
				Enabled:       false,
				MaxIterations: db.MaxIterationsHardCap,
				CooldownSec:   2,
				Seed:          "board-run-in-progress",
			}
		},
	},
	{
		Seed: "board-archive-done",
		Make: func(agentID string) db.Automation {
			return db.Automation{
				Name:         "🗄️ Biten kartı arşivle",
				TriggerKind:  db.TriggerBoard,
				BoardOp:      db.BoardOpMove,
				BoardToState: db.BoardDone,
				BoardAction:  db.BoardActionArchive,
				// PromptTemplate is unused by the archive action but kept non-empty so
				// the rule reads clearly in the editor.
				PromptTemplate: "(kartı arşivle — LLM çağrısı yok)",
				Enabled:        false,
				MaxIterations:  db.MaxIterationsHardCap,
				Seed:           "board-archive-done",
			}
		},
	},
	{
		Seed: "insight-apply-workspace-opt",
		// The applier is a system agent with a deliberately narrow allowlist; the
		// rule is meaningless pointed at anything else.
		AgentSystemKey: "insight-applier",
		Make: func(agentID string) db.Automation {
			return db.Automation{
				Name:          "🛠️ İçgörü taraması bitti → workspace-opt bulgularını uygula",
				TriggerKind:   db.TriggerTag,
				TriggerTag:    insightScanSessionTag,
				TargetAgentID: agentID,
				SessionMode:   db.SessionModeSpawn,
				PromptTemplate: "Az önce biten içgörü taramasının workspace-opt bulgularını uygula.\n\n" +
					"Tarama oturumu: {{title}} ({{sessionId}})\n\n" +
					"insight_list_findings ile channel: \"workspace-opt\", status: \"new\" bulguları çek ve " +
					"her birini uygun workspace varlığında (skill / agent / hook / automation) düzelt. " +
					"app-fix kanalına dokunma. Uyguladığın bulguyu insight_apply_finding ile applied işaretle; " +
					"uygulamadıklarını gerekçesiyle raporla.",
				// Break the loop with a DIFFERENT tag: nil would default to the trigger
				// tag (the applier session would re-fire this rule), and an empty slice
				// cannot express it either — normalizeTags collapses [] back to nil at
				// create time. The tag also makes applier sessions easy to filter.
				SpawnTags:     []string{"insight-applied"},
				Enabled:       false,
				MaxIterations: 20,
				CooldownSec:   300,
				Seed:          "insight-apply-workspace-opt",
			}
		},
	},
}

// insightScanSessionTag is the session tag the insight-apply rule waits for.
// Scan sessions are opened with it (openInsightSession) and fire the turn hooks
// when their transcript is written (insightStepRecorder.finish), so enabling the
// rule is all that is needed — it ships DISABLED because applying findings to the
// workspace stays a deliberate user decision.
const insightScanSessionTag = "insight-scan"

// seededAutomationsLedgerName is a dotfile at the store root recording which
// default automation seed keys have ever been provisioned into this workspace.
// Like the flows ledger, it makes a user deletion permanent: once a key is
// recorded, EnsureDefaultAutomations never recreates that default.
const seededAutomationsLedgerName = ".seeded-automations.json"

type seededAutomationsLedger struct {
	Seeded []string `json:"seeded"`
}

// EnsureDefaultAutomations provisions the built-in automations into a
// workspace's store, idempotently and respecting user deletion — the same
// contract as EnsureDefaultFlows:
//
//   - If an automation with the same Seed key already exists, it is left untouched.
//   - Else if the deletion ledger records the key, the rule was seeded before and
//     the user deleted it → it is NOT resurrected.
//   - Else the rule is created (the spawn rule gets the workspace's first agent, or
//     an empty agentId when there is none yet) and the key is recorded.
//
// open() calls this on every startup, so existing workspaces are backfilled with
// newly shipped defaults on the next launch.
func EnsureDefaultAutomations(ctx context.Context, database *db.DB, storeDir string) error {
	if database == nil || storeDir == "" {
		return nil
	}
	ledger := loadSeededAutomationsLedger(storeDir)
	recorded := map[string]bool{}
	for _, s := range ledger.Seeded {
		recorded[s] = true
	}

	existing, err := database.ListAutomations(ctx)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, a := range existing {
		if a.Seed != "" {
			present[a.Seed] = true
		}
	}

	agentID := firstAgentID(ctx, database)

	changed := false
	for _, dba := range defaultAutomations {
		if present[dba.Seed] || recorded[dba.Seed] {
			continue
		}
		target := agentID
		if dba.AgentSystemKey != "" {
			pinned, ok := database.FindAgentBySystemKey(dba.AgentSystemKey)
			if !ok {
				// The system agent is seeded before this runs; a miss means an older
				// store that has not been backfilled yet. Skip WITHOUT recording so the
				// next startup provisions it.
				continue
			}
			target = pinned.ID
		}
		auto := dba.Make(target)
		// Same shape contract the create/update paths enforce (db.ValidateAutomationShape).
		// The spawn seed intentionally carries an EMPTY target when the workspace has
		// no agent yet: that fails the shape check, so we skip WITHOUT recording it in
		// the ledger — the next startup (once an agent exists) backfills it. Any other
		// shape failure means a malformed built-in seed (caught by the unit test); we
		// likewise skip it rather than persist an unfireable rule.
		if err := db.ValidateAutomationShape(auto); err != nil {
			continue
		}
		if _, err := database.CreateAutomation(ctx, auto); err != nil {
			return err
		}
		ledger.Seeded = append(ledger.Seeded, dba.Seed)
		recorded[dba.Seed] = true
		changed = true
	}

	if changed {
		return saveSeededAutomationsLedger(storeDir, ledger)
	}
	return nil
}

func loadSeededAutomationsLedger(storeDir string) seededAutomationsLedger {
	var l seededAutomationsLedger
	data, err := os.ReadFile(filepath.Join(storeDir, seededAutomationsLedgerName))
	if err != nil {
		return l
	}
	_ = json.Unmarshal(data, &l)
	return l
}

func saveSeededAutomationsLedger(storeDir string, l seededAutomationsLedger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storeDir, seededAutomationsLedgerName), data, 0o644)
}
