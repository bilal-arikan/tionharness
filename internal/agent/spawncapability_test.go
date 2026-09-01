package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestAgentMutationToolsUnconstrained(t *testing.T) {
	for _, raw := range []string{"", "[]", `["*"]`} {
		allowed, constrained, err := agentMutationTools(db.Agent{ID: "A1", AllowedTools: raw})
		if err != nil {
			t.Fatalf("allowed_tools %q: unexpected error %v", raw, err)
		}
		if constrained {
			t.Fatalf("allowed_tools %q must read as unconstrained", raw)
		}
		if len(allowed) != 0 {
			t.Fatalf("allowed_tools %q: unconstrained agent needs no enumeration, got %v", raw, allowed)
		}
	}
}

func TestAgentMutationToolsReadOnlyProfile(t *testing.T) {
	// The exact allowlist "Worker: Planner" (AGT166) carries in WS5.
	a := db.Agent{ID: "AGT166", Name: "Worker: Planner", AllowedTools: `["Read","LS","Glob","Grep","WebFetch"]`}
	allowed, constrained, err := agentMutationTools(a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !constrained {
		t.Fatal("a non-empty allowlist must read as constrained")
	}
	if len(allowed) != 0 {
		t.Fatalf("planner must expose no mutation tool, got %v", allowed)
	}
}

func TestAgentMutationToolsCoderProfile(t *testing.T) {
	a := db.Agent{ID: "AGT102", Name: "Worker: Coder", AllowedTools: `["Read","LS","Glob","Grep","Write","Edit","Bash"]`}
	allowed, _, err := agentMutationTools(a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowed) == 0 {
		t.Fatal("coder must expose mutation tools")
	}
}

// A malformed allowlist must NOT read as permissive: the real tool filter denies
// everything in that case, so reporting "can write" would be a lie.
func TestAgentMutationToolsMalformedIsError(t *testing.T) {
	if _, _, err := agentMutationTools(db.Agent{ID: "A1", AllowedTools: `["Read"`}); err == nil {
		t.Fatal("a malformed allowed_tools list must be an error")
	}
}

func TestTaskNeedsMutation(t *testing.T) {
	needs := []string{
		"Sadece brief dosyasını oluştur, satır sayısı ve SHA-256 kanıtıyla teslim et.",
		"tsk622-backend-cas-plan.md dosyasına tam planı yaz",
		"Write the migration file under internal/db.",
		"Değişikliği commit et ve gitleaks taraması yap",
		"run git commit after the tests pass",
	}
	for _, task := range needs {
		if got := taskNeedsMutation(task); got == "" {
			t.Errorf("taskNeedsMutation(%q) = \"\", want a trigger", task)
		}
	}
	readOnly := []string{
		"TSK622 için salt-okunur mimari plan hazırla ve üç yaklaşımı karşılaştır.",
		"Adversarial olarak incele; bulguları PASS/FAIL olarak raporla.",
		"Repository'yi keşfet, node konumu kalıcılığının nerede kırıldığını bul.",
		"Review the implementation and report defects.",
	}
	for _, task := range readOnly {
		if got := taskNeedsMutation(task); got != "" {
			t.Errorf("taskNeedsMutation(%q) = %q, want \"\" (read-only brief must still spawn)", task, got)
		}
	}
}

func TestCheckWorkerCapability(t *testing.T) {
	rt := &Runtime{}
	planner := db.Agent{ID: "AGT166", Name: "Worker: Planner", AllowedTools: `["Read","LS","Glob","Grep","WebFetch"]`}
	coder := db.Agent{ID: "AGT102", Name: "Worker: Coder", AllowedTools: `["Read","Write","Edit","Bash"]`}
	writeTask := "Sadece brief dosyasını oluştur ve satır sayısını doğrula."
	readTask := "Planı hazırla ve üç yaklaşımı karşılaştır."

	err := rt.checkWorkerCapability(planner, writeTask)
	if err == nil {
		t.Fatal("a write brief on a read-only agent must be rejected at spawn time")
	}
	if !strings.Contains(err.Error(), "Worker: Planner") || !strings.Contains(err.Error(), "Write/Edit/Bash") {
		t.Fatalf("error must name the agent and the way out, got %q", err)
	}
	if err := rt.checkWorkerCapability(planner, readTask); err != nil {
		t.Fatalf("read-only brief on a read-only agent must pass, got %v", err)
	}
	if err := rt.checkWorkerCapability(coder, writeTask); err != nil {
		t.Fatalf("write brief on a write-capable agent must pass, got %v", err)
	}
}
