package agent

import "github.com/bilal-arikan/tionharness/internal/decider"

// The decision authorities ("karar mercileri") the runtime implements. Each
// registers with the decider layer, which gives it a settings entry, the
// off / shadow / on switch, a threshold, its own decision model with a fallback
// and a challenger, and the ledger. The logic sits next to each: decide_stall.go,
// decide_toolrisk.go, flow_judge.go, trajectory_gate_judge.go. A new authority
// is one more registration here plus its call site.
const (
	// authStallJudge: "did the coordinator stop without doing what it said?"
	authStallJudge = "stall-judge"
	// authToolRisk: a second look at shell commands that would run without a
	// human decision (auto mode, or an "always allow" grant).
	authToolRisk = "tool-risk"
	// authFlowJudge: branch/loop nodes whose match mode is "judge".
	authFlowJudge = "flow-judge"
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
