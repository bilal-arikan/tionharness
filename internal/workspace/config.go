package workspace

import (
	"os"
	"path/filepath"

	"github.com/bilal/swarmgo/internal/agent"
)

// syncConfigFiles seeds the workspace's editable config/ tree (prompts,
// instructions, README) on first open and makes instructions.md authoritative:
// a user editing the file on disk wins over the value cached in ws-settings.json.
// Called after loadSettings so it sees the persisted instructions to seed from.
func (w *Workspace) syncConfigFiles() {
	instr := w.Settings().Instructions
	if err := agent.SeedWorkspaceConfig(w.DataDir, instr); err != nil {
		return
	}
	// File is the source of truth: adopt instructions.md back into the live
	// settings + runtime so on-disk edits take effect on the next boot.
	data, err := os.ReadFile(agent.InstructionsFilePath(w.DataDir))
	if err != nil {
		return
	}
	fileInstr := string(data)
	w.settings.mu.Lock()
	w.settings.cur.Instructions = fileInstr
	w.settings.mu.Unlock()
	if w.Runtime != nil {
		w.Runtime.SetInstructions(fileInstr)
	}
}

// writeInstructionsFile mirrors an instructions update to config/instructions.md
// so the file and ws-settings.json stay in sync when edited via the UI.
func (w *Workspace) writeInstructionsFile(text string) {
	dir := filepath.Join(agent.WorkspaceConfigDir(w.DataDir))
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(agent.InstructionsFilePath(w.DataDir), []byte(text), 0o644)
}
