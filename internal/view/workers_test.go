package view

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWorkersBlockKeepsItsAuthoritativeFraming(t *testing.T) {
	now := time.Now()
	in := WorkersInput{Now: now, Workers: []Worker{
		{SessionID: "SES1", AgentName: "scout", Running: true, StartedAt: now.Add(-14 * time.Minute).Unix()},
		{SessionID: "SES2", AgentName: "lead", Delegating: true},
		{SessionID: "SES3", AgentName: "writer", Summary: "rapor hazır"},
	}}

	v, err := ProjectWorkers(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	// The framing is what defeats the coalesced-notification stall (_Docs/47);
	// it must survive any refactor of this block.
	for _, want := range []string{
		"# Worker status (live, authoritative)",
		"trust THIS over the notifications in history",
		"- scout [RUNNING",
		"- lead [DELEGATING (its own workers are running; it has not reported yet)] (SES2)",
		"- writer [finished] (SES3) — rapor hazır",
		"Summary: 2 running, 1 finished.",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	// A delegating sub-coordinator counts as running: it still owes a report.
	if strings.Contains(txt, "ALL workers are finished") {
		t.Errorf("delegating worker must not read as an idle fleet:\n%s", txt)
	}
}

// TestWorkersVerdictBadgeAndTally: a finished validator's contracted VERDICT line
// is hoisted into a PASS/FAIL badge and tallied, so a coordinator scans outcomes
// without re-reading each report (P4). Non-verdict workers get no badge, and a
// FAIL is flagged as unfinished work.
func TestWorkersVerdictBadgeAndTally(t *testing.T) {
	now := time.Now()
	in := WorkersInput{Now: now, Workers: []Worker{
		{SessionID: "SES1", AgentName: "validator", Summary: "VERDICT: PASS\ntests: 42/42"},
		{SessionID: "SES2", AgentName: "validator", Summary: "VERDICT: FAIL — auth_test.go:88"},
		{SessionID: "SES3", AgentName: "writer", Summary: "rapor hazır"},
	}}

	v, err := ProjectWorkers(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{
		"- validator [finished] ✅ PASS (SES1)",
		"- validator [finished] ❌ FAIL (SES2)",
		"- writer [finished] (SES3) — rapor hazır", // no badge for a non-verdict worker
		"Verdicts: 1 PASS, 1 FAIL.",
		"A FAIL is unfinished work",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestParseVerdict(t *testing.T) {
	cases := map[string]string{
		"VERDICT: PASS":             verdictPass,
		"  verdict:  pass  ":        verdictPass,
		"VERDICT: FAIL — x.go:1":    verdictFail,
		"VERDICT: PASS\ntests: 1/1": verdictPass,
		"rapor hazır":               "",
		"the VERDICT: is unclear":   "", // marker must start the line
		"VERDICT: MAYBE":            "",
	}
	for in, want := range cases {
		if got := parseVerdict(in); got != want {
			t.Errorf("parseVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWorkersRunningShowsElapsed: elapsed time is what turns "RUNNING" into a
// decision. It was available (WorkerInfo.StartedAt) but unused before the block
// moved into this package.
func TestWorkersRunningShowsElapsed(t *testing.T) {
	now := time.Now()
	in := WorkersInput{Now: now, Workers: []Worker{
		{SessionID: "SES1", AgentName: "scout", Running: true, StartedAt: now.Add(-14 * time.Minute).Unix()},
		{SessionID: "SES2", AgentName: "ghost", Running: true}, // start time unknown
	}}

	v, err := ProjectWorkers(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "RUNNING for 14m00s") {
		t.Errorf("elapsed time missing:\n%s", txt)
	}
	// An unknown start must print no duration rather than a fabricated one.
	if !strings.Contains(txt, "- ghost [RUNNING] (SES2)") {
		t.Errorf("unknown start time must omit the duration:\n%s", txt)
	}
}

// TestWorkersIdleFleetGetsTheNudge pins the line that stops a coordinator
// waiting forever once everything has reported.
func TestWorkersIdleFleetGetsTheNudge(t *testing.T) {
	v, err := ProjectWorkers(WorkersInput{Workers: []Worker{
		{SessionID: "SES1", AgentName: "a", Summary: "bitti"},
		{SessionID: "SES2", AgentName: "b", Summary: "bitti"},
	}}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "Summary: 0 running, 2 finished.") {
		t.Errorf("counts wrong:\n%s", txt)
	}
	if !strings.Contains(txt, "ALL workers are finished") {
		t.Errorf("idle-fleet nudge missing:\n%s", txt)
	}
}

// TestWorkersWideFleetIsCappedButCountsStayWhole is the defect this migration
// fixed: the block is regenerated into EVERY coordinator turn, so an uncapped
// list let a wide fleet dominate the prompt. Capping must never corrupt the
// arithmetic the coordinator acts on.
func TestWorkersWideFleetIsCappedButCountsStayWhole(t *testing.T) {
	now := time.Now()
	in := WorkersInput{Now: now}
	for i := 0; i < 30; i++ {
		in.Workers = append(in.Workers, Worker{
			SessionID: fmt.Sprintf("SESf%d", i), AgentName: fmt.Sprintf("done%d", i), Summary: "ok",
		})
	}
	// Two still-running workers, added LAST so a naive head-truncation would
	// drop exactly the entries that matter.
	in.Workers = append(in.Workers,
		Worker{SessionID: "SESr1", AgentName: "busy1", Running: true, StartedAt: now.Add(-time.Minute).Unix()},
		Worker{SessionID: "SESr2", AgentName: "busy2", Delegating: true})

	v, err := ProjectWorkers(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	if v.Elided != len(in.Workers)-workersMaxListed {
		t.Errorf("elided = %d, want %d", v.Elided, len(in.Workers)-workersMaxListed)
	}
	// The note must be ENGLISH: this block is injected into an English prompt,
	// and the default Turkish elision sentence would switch languages mid-block.
	wantNote := fmt.Sprintf("%d more finished worker(s) not listed", v.Elided)
	if !strings.Contains(txt, wantNote) {
		t.Errorf("english elision note missing (%q):\n%s", wantNote, txt)
	}
	if strings.Contains(txt, "gizlendi") {
		t.Errorf("turkish elision sentence leaked into an english prompt block:\n%s", txt)
	}
	// The running ones survive the cap…
	for _, want := range []string{"busy1", "busy2"} {
		if !strings.Contains(txt, want) {
			t.Errorf("a running worker was dropped by the cap (%s):\n%s", want, txt)
		}
	}
	// …and the summary still counts the WHOLE fleet, elided entries included.
	if !strings.Contains(txt, "Summary: 2 running, 30 finished.") {
		t.Errorf("cap corrupted the fleet arithmetic:\n%s", txt)
	}
}

// TestWorkersEmptyFleetStillReportsZero: the caller skips the block entirely for
// an empty fleet, but the projection itself must not produce a dangling header.
func TestWorkersEmptyFleetStillReportsZero(t *testing.T) {
	v, err := ProjectWorkers(WorkersInput{}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "Summary: 0 running, 0 finished.") {
		t.Errorf("empty fleet must still state its counts:\n%s", v.Text())
	}
}
