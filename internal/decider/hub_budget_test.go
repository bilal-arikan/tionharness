package decider

import (
	"encoding/json"
	"strings"
	"testing"
)

func stateBudgetTestModel(t *testing.T, h *Hub, id, url string, tokens int) {
	t.Helper()
	_, err := h.UpsertModel(ModelInput{
		ID: id, Backend: SystemOneBackendID, Model: OpenJevModel,
		Enabled: false, Credentials: CredentialsOwn, BaseURL: url,
		ContextTokens: tokens,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func stateBudgetTestHub(t *testing.T, tokens int) (*Hub, *decisionServer, *fakeSource) {
	t.Helper()
	srv := newDecisionServer(t)
	src := &fakeSource{}
	h := NewHub(HubOptions{Source: src, Secrets: fakeBox{}})
	stateBudgetTestModel(t, h, "budget-primary", srv.URL+"/v1", tokens)
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) {
		ac.Model, ac.Mode = "budget-primary", ModeOff
	})
	return h, srv, src
}

// These windows are above the existing state floor. Calculate question bytes
// independently so the assertions detect a missed overhead or safety reserve.
func expectedWorkflowStateBudget(t *testing.T, questions map[string]Question, contextTokens int) int {
	t.Helper()
	raw, err := json.Marshal(questions)
	if err != nil {
		t.Fatal(err)
	}
	return contextTokens*5/2 - len(raw) - 2048 - 512
}

func TestHubStateBudgetUsesSmallestConfiguredFallbackWindow(t *testing.T) {
	h, srv, src := stateBudgetTestHub(t, 24000)
	stateBudgetTestModel(t, h, "budget-fallback", srv.URL+"/v1", 6400)
	stateBudgetTestModel(t, h, "budget-challenger", srv.URL+"/v1", 10000)
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) {
		ac.Fallback, ac.Challenger = "budget-fallback", "budget-challenger"
	})
	questions := yesNo().Questions
	if got, want := h.StateBudget(testGate, questions), expectedWorkflowStateBudget(t, questions, 6400); got != want {
		t.Fatalf("fallback-limited budget = %d, want %d", got, want)
	}
	if srv.calls.Load() != 0 || src.resolved.Load() != 0 || h.Spend(7).Totals.Calls != 0 {
		t.Fatal("budget inspection executed a backend or resolved credentials")
	}
}

func TestHubStateBudgetUsesSmallerChallengerBeforePrimaryAndFallback(t *testing.T) {
	h, srv, _ := stateBudgetTestHub(t, 24000)
	stateBudgetTestModel(t, h, "budget-fallback", srv.URL+"/v1", 12000)
	stateBudgetTestModel(t, h, "budget-challenger", srv.URL+"/v1", 6000)
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) {
		ac.Fallback, ac.Challenger = "budget-fallback", "budget-challenger"
	})
	questions := yesNo().Questions
	if got, want := h.StateBudget(testGate, questions), expectedWorkflowStateBudget(t, questions, 6000); got != want {
		t.Fatalf("comparison-limited budget = %d, want %d", got, want)
	}
}

func TestHubStateBudgetUsesBackendContextWhenModelOverrideIsUnset(t *testing.T) {
	h, srv, _ := stateBudgetTestHub(t, 40000)
	stateBudgetTestModel(t, h, "budget-fallback", srv.URL+"/v1", 0)
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Fallback = "budget-fallback" })
	backend, ok := Lookup(SystemOneBackendID)
	if !ok {
		t.Fatal("System One backend not registered")
	}
	questions := yesNo().Questions
	if got, want := h.StateBudget(testGate, questions), expectedWorkflowStateBudget(t, questions, backend.Manifest().ContextTokens); got != want {
		t.Fatalf("manifest-limited budget = %d, want %d", got, want)
	}
}

func TestHubStateBudgetReservesQuestionBytesAndWorkflowSafety(t *testing.T) {
	h, _, _ := stateBudgetTestHub(t, 24000)
	small := yesNo().Questions
	rich := map[string]Question{"q": Noul(strings.Repeat("context criterion ", 200), "keep", "discard")}
	smallRaw, _ := json.Marshal(small)
	richRaw, _ := json.Marshal(rich)
	smallBudget, richBudget := h.StateBudget(testGate, small), h.StateBudget(testGate, rich)
	if smallBudget <= richBudget || smallBudget-richBudget != len(richRaw)-len(smallRaw) {
		t.Fatalf("questions did not consume matching state bytes: small=%d rich=%d delta=%d", smallBudget, richBudget, len(richRaw)-len(smallRaw))
	}
	if want := expectedWorkflowStateBudget(t, rich, 24000); richBudget != want {
		t.Fatalf("question/overhead/safety budget = %d, want %d", richBudget, want)
	}
}

func TestHubStateBudgetWorksWithDisabledHubAuthorityModelAndNoCredentials(t *testing.T) {
	h, srv, src := stateBudgetTestHub(t, 8000)
	model, ok := h.Model("budget-primary")
	if !ok || model.Enabled || h.Config().Enabled || h.Mode(testGate) != ModeOff {
		t.Fatal("test fixture must have model, master switch and authority disabled")
	}
	questions := yesNo().Questions
	if got, want := h.StateBudget(testGate, questions), expectedWorkflowStateBudget(t, questions, 8000); got != want {
		t.Fatalf("offline budget = %d, want %d", got, want)
	}
	if srv.calls.Load() != 0 || src.resolved.Load() != 0 || h.Spend(7).Totals.Calls != 0 {
		t.Fatal("disabled budget inspection performed billable work")
	}
}

func TestHubStateBudgetNilHubHasSafeDefaultAndQuestionAllowance(t *testing.T) {
	var h *Hub
	questions := yesNo().Questions
	got := h.StateBudget("unregistered", questions)
	if want := expectedWorkflowStateBudget(t, questions, 32000); got != want || got <= 1024 {
		t.Fatalf("nil Hub default budget = %d, want %d", got, want)
	}
	if noQuestions := h.StateBudget("", nil); noQuestions <= got {
		t.Fatalf("nil Hub did not reserve questions: no questions=%d questions=%d", noQuestions, got)
	}
}
