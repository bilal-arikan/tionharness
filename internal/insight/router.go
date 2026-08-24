package insight

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Routing sends findings to their channel sinks (_Docs/60 §6). App-fix (Channel
// A) findings are rendered in-app and, when a repo is configured, appended to
// that repo's _Docs backlog. Workspace-opt (Channel B) findings are appended to
// a workspace-local actions doc — the symmetric sink (Faz 2.5). Both sinks are
// documents only: no automated workspace mutation happens here (that stays a
// deliberate user/agent decision).

// backlogRelPath is the repo-relative backlog file app-fix findings append to.
var backlogRelPath = filepath.Join("_Docs", "INSIGHT-BACKLOG.md")

// workspaceActionsRelPath is the store-root-relative doc that workspace-opt
// findings append to (the Channel B sink).
var workspaceActionsRelPath = filepath.Join("insight", "WORKSPACE-ACTIONS.md")

// RenderAppFixReport builds a dev-facing markdown report of the given app-fix
// findings, newest-first. generatedAt is passed in (not read from the clock) so
// the output is deterministic and testable.
func RenderAppFixReport(findings []Finding, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# TionHarness — App Fix Findings\n\n")
	fmt.Fprintf(&b, "_Generated: %s_\n\n", generatedAt)
	if len(findings) == 0 {
		b.WriteString("No app-fix findings.\n")
		return b.String()
	}
	for _, f := range findings {
		writeFindingBlock(&b, f, "")
	}
	return b.String()
}

func writeFindingBlock(b *strings.Builder, f Finding, repoDir string) {
	fmt.Fprintf(b, "## %s\n\n", strings.TrimSpace(orDash(f.Title)))
	if f.Severity != "" {
		fmt.Fprintf(b, "- **Severity:** %s\n", f.Severity)
	}
	if f.Regressed {
		b.WriteString("- **⚠️ REGRESSED** — this was closed and recurred\n")
	}
	fmt.Fprintf(b, "- **Occurrences:** %d\n", f.Occurrences)
	if len(f.EvidenceSessionIDs) > 0 {
		fmt.Fprintf(b, "- **Evidence sessions:** %s\n", strings.Join(f.EvidenceSessionIDs, ", "))
	}
	if f.FilePointer != "" {
		// The file pointer is an LLM suggestion; verify it against the repo when we
		// know where the repo is, and flag pointers that don't resolve so a reader
		// doesn't chase a hallucinated path.
		if repoDir != "" && !CheckFilePointer(repoDir, f.FilePointer) {
			fmt.Fprintf(b, "- **File:** `%s` ⚠️ (not found in repo — pointer unverified)\n", f.FilePointer)
		} else {
			fmt.Fprintf(b, "- **File:** `%s`\n", f.FilePointer)
		}
	}
	if f.RootCause != "" {
		fmt.Fprintf(b, "\n**Root cause:** %s\n", f.RootCause)
	}
	if f.ProposedFix != "" {
		fmt.Fprintf(b, "\n**Proposed fix:** %s\n", f.ProposedFix)
	}
	// A stable machine marker so re-appends are idempotent (dedup by signature).
	fmt.Fprintf(b, "\n<!-- insight-sig:%s -->\n\n", f.Signature)
}

// AppendBacklog appends the app-fix findings not already present to
// <repoDir>/_Docs/INSIGHT-BACKLOG.md, idempotently: a finding whose signature
// already appears in the file (via its <!-- insight-sig:... --> marker) is
// skipped, so repeated scans do not duplicate entries. A blank repoDir is a
// no-op. Returns how many were appended.
func AppendBacklog(repoDir string, findings []Finding) (int, error) {
	if strings.TrimSpace(repoDir) == "" {
		return 0, nil
	}
	header := "# TionHarness — Insight App-Fix Backlog\n\n" +
		"Auto-appended by the retrospective scanner (_Docs/60). Each entry carries a\n" +
		"stable `insight-sig` marker so re-scans never duplicate it.\n\n"
	return appendFindingsFile(filepath.Join(repoDir, backlogRelPath), header, findings, repoDir)
}

// AppendWorkspaceActions appends workspace-opt findings not already present to
// <storeRoot>/insight/WORKSPACE-ACTIONS.md, idempotently (same signature-marker
// dedupe as the backlog). This is the Channel B sink: a workspace-local, human-
// reviewable list of proposed optimizations — a document, never an auto-applied
// mutation. A blank storeRoot is a no-op. Returns how many were appended.
func AppendWorkspaceActions(storeRoot string, findings []Finding) (int, error) {
	if strings.TrimSpace(storeRoot) == "" {
		return 0, nil
	}
	header := "# TionHarness — Workspace Optimization Actions\n\n" +
		"Auto-appended by the retrospective scanner (_Docs/60, Channel B). Each entry is a\n" +
		"proposed workspace optimization for you (or an agent) to review and apply — nothing\n" +
		"here is applied automatically. A stable `insight-sig` marker keeps re-scans idempotent.\n\n"
	return appendFindingsFile(filepath.Join(storeRoot, workspaceActionsRelPath), header, findings, "")
}

// appendFindingsFile is the shared idempotent-append engine behind both channel
// sinks: it skips findings whose signature already appears in the file, writes
// the header once when the file is new, and appends the rest. Returns the count
// appended.
func appendFindingsFile(path, header string, findings []Finding, repoDir string) (int, error) {
	if len(findings) == 0 {
		return 0, nil
	}
	present, err := backlogSignatures(path)
	if err != nil {
		return 0, err
	}

	var block strings.Builder
	appended := 0
	for _, f := range findings {
		if f.Signature != "" && present[f.Signature] {
			continue
		}
		writeFindingBlock(&block, f, repoDir)
		appended++
	}
	if appended == 0 {
		return 0, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	// Create the header once when the file is new.
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			return 0, err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.WriteString(block.String()); err != nil {
		return 0, err
	}
	return appended, nil
}

// backlogSignatures reads the signatures already recorded in a backlog file
// (from the insight-sig markers). A missing file yields an empty set.
func backlogSignatures(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		const marker = "<!-- insight-sig:"
		if i := strings.Index(line, marker); i >= 0 {
			rest := line[i+len(marker):]
			if j := strings.Index(rest, " -->"); j >= 0 {
				if sig := strings.TrimSpace(rest[:j]); sig != "" {
					out[sig] = true
				}
			}
		}
	}
	return out, sc.Err()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(untitled)"
	}
	return s
}
