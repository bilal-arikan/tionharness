package agent

import "github.com/bilal-arikan/tionharness/internal/decider"

// The decision authorities ("karar mercileri") the runtime implements. Each
// registers with the decider layer, which gives it a settings entry, the
// off / shadow / on switch, a threshold, its own decision model with a fallback
// and a challenger, and the ledger. The logic sits next to each: decide_stall.go,
// decide_toolrisk.go, flowturn_judge.go, trajectory_gate_judge.go. A new authority
// is one more registration here plus its call site.
const (
	// authStallJudge: "did the coordinator stop without doing what it said?"
	authStallJudge = "stall-judge"
	// authToolRisk: a second look at shell commands that would run without a
	// human decision (auto mode, or an "always allow" grant).
	authToolRisk = "tool-risk"
	// authFlowJudge: route nodes whose mode is "judge" (_Docs/93).
	authFlowJudge = "flow-judge"
	// authFlowCriteria: route nodes whose mode is "criteria" — one yes/no
	// question per criterion, all must hold (_Docs/93).
	authFlowCriteria = "flow-criteria"
	// authFlowGrade: grades every finished flow run's reply on a 1..5 scale,
	// the quality signal the observer and flow-kind automations read.
	authFlowGrade = "flow-grade"
	// authFlowProposalGate: a second look at an observer proposal before the
	// auto policy applies it.
	authFlowProposalGate = "flow-proposal-gate"
	// authPhaseGate: Rota phase gates of kind "judge".
	authPhaseGate = "phase-gate"
)

func init() {
	decider.RegisterAuthority(decider.Authority{
		ID:               authToolRisk,
		Group:            decider.GroupSafety,
		Pattern:          decider.PatternGate,
		Label:            "Shell command risk check",
		Description:      "Takes a second look at shell commands that would run without asking (auto mode or an \"always allow\" grant). In on mode a risky command is turned into an approval prompt; it never blocks an unattended run.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.8,
		ThresholdHint:    "Minimum probability that the command needs approval before the user is asked.",
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authStallJudge,
		Group:            decider.GroupCoordination,
		Pattern:          decider.PatternGate,
		Label:            "Coordinator stall judge",
		Description:      "Decides whether a coordinator's last message promised worker spawns it never made. Replaces a Haiku call per idle coordinator turn.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.7,
		ThresholdHint:    "Minimum probability of \"stalled\" before the coordinator is nudged.",
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authFlowJudge,
		Group:            decider.GroupFlows,
		Pattern:          decider.PatternPick,
		Label:            "Flow judge nodes",
		Description:      "Branch and loop nodes whose match mode is \"judge\" ask the decider which arm the last output belongs to, or whether the loop's exit condition holds.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeOn},
		DefaultMode:      decider.ModeOn,
		DefaultThreshold: 0.6,
		ThresholdHint:    "Minimum probability for an arm to be taken (otherwise the default arm) or for the exit condition to count as met.",
		Explicit:         true,
		Order:            1,
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authFlowCriteria,
		Group:            decider.GroupFlows,
		Pattern:          decider.PatternSelect,
		Label:            "Flow criteria gates",
		Description:      "Route nodes whose match mode is \"criteria\" ask the decider one yes/no question per criterion about the last output; every criterion holding takes the pass arm, otherwise the fail arm.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeOn},
		DefaultMode:      decider.ModeOn,
		DefaultThreshold: 0.6,
		ThresholdHint:    "Minimum probability for a criterion to count as met.",
		Explicit:         true,
		Order:            3,
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authFlowGrade,
		Group:            decider.GroupFlows,
		Pattern:          decider.PatternRate,
		Label:            "Flow run grading",
		Description:      "After every successful flow run, grades how well the reply satisfied the request (1..5). The grade feeds the observer's evidence, the runs list and flow-kind automations (\"when graded 2 or below…\"). One cheap decision call per turn.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeOn},
		DefaultMode:      decider.ModeOff,
		DefaultThreshold: 0.5,
		ThresholdHint:    "Minimum confidence before a grade is recorded.",
		Order:            4,
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authFlowProposalGate,
		Group:            decider.GroupFlows,
		Pattern:          decider.PatternGate,
		Label:            "Flow proposal gate",
		Description:      "Before the auto policy applies an observer proposal, asks the decider whether the change is a plausible, proportionate improvement given the recent runs. A held proposal stays pending for a human.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeShadow, decider.ModeOn},
		DefaultMode:      decider.ModeShadow,
		DefaultThreshold: 0.7,
		ThresholdHint:    "Minimum probability that the proposal improves the flow before it is auto-applied.",
		Order:            5,
	})
	decider.RegisterAuthority(decider.Authority{
		ID:               authPhaseGate,
		Group:            decider.GroupFlows,
		Pattern:          decider.PatternGate,
		Label:            "Rota judge gates",
		Description:      "Phase gates of kind \"judge\" ask the decider whether the phase's exit condition holds, judged against the root session's recent transcript.",
		Modes:            []decider.Mode{decider.ModeOff, decider.ModeOn},
		DefaultMode:      decider.ModeOn,
		DefaultThreshold: 0.8,
		ThresholdHint:    "Minimum probability that the exit condition holds before the gate opens.",
		Explicit:         true,
		FailClosed:       true,
		Order:            2,
	})
}
