package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// defaultBoardAutomation describes a built-in board automation shipped into every
// workspace store by EnsureDefaultBoardAutomations. Together the seeded rules make
// the kanban board a source of EXECUTION in any workspace, out of the box: moving
// a card into "in_progress" starts an agent on it, and a finished card is archived
// off the board with no LLM call. Seed is the stable identity used both to skip
// re-seeding an existing default and to record it in the deletion ledger.
//
// They are seeded DISABLED on purpose: auto-spawning an agent on every card move
// is a real behaviour/cost change that should be an explicit, per-workspace opt-in
// (one toggle in the Automations screen), not a silent default in, say, a plain
// single-agent chat workspace. Seeding-yet-disabled means the wiring is present
// everywhere and activating "board-driven execution" is a single click.
type defaultBoardAutomation struct {
	Seed string
	Make func(agentID string) db.Automation
}

// defaultBoardAutomations is the single source of truth for the built-in board
// automations every workspace gets. The spawn rule's TargetAgentID is filled with
// the workspace's first agent at seed time (empty when there is none yet — the
// user assigns one later, exactly like a seeded flow's agent nodes).
var defaultBoardAutomations = []defaultBoardAutomation{
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
					"sonra sonucu özetle. İş bitince kartı 'İnceleme' (review) sütununa taşı.",
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
}

// seededAutomationsLedgerName is a dotfile at the store root recording which
// default board-automation seed keys have ever been provisioned into this
// workspace. Like the flows ledger, it makes a user deletion permanent: once a
// key is recorded, EnsureDefaultBoardAutomations never recreates that default.
const seededAutomationsLedgerName = ".seeded-automations.json"

type seededAutomationsLedger struct {
	Seeded []string `json:"seeded"`
}

// EnsureDefaultBoardAutomations provisions the built-in board automations into a
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
func EnsureDefaultBoardAutomations(ctx context.Context, database *db.DB, storeDir string) error {
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
	for _, dba := range defaultBoardAutomations {
		if present[dba.Seed] || recorded[dba.Seed] {
			continue
		}
		auto := dba.Make(agentID)
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
