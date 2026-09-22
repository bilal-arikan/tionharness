package orchestration

import (
	"context"
	"fmt"
)

// MatchJudge is the match mode that asks a decision model instead of matching
// text. On a branch node (MatchMode) each arm's Contains is a plain-language
// description of an option and the model picks the arm the last output belongs
// to; on a loop node (UntilMode) Until is a plain-language condition and the
// model says whether it holds. It replaces the "have an agent print one word,
// then substring-match it" pattern: no extra agent session, no "bug" matching
// inside "debug", no loop running to the step cap because a model broke the
// VERDICT line's format.
const MatchJudge = "judge"

// JudgeRequest is one judgement for a JudgeRunner.
type JudgeRequest struct {
	// Value is the text being judged (the last output, or its JSONField).
	Value string
	// Question optionally rephrases what is asked (Node.JudgeQuestion).
	Question string
	// Options are the branch arms' descriptions in arm order (branch only; the
	// default arm is not an option — it is what an unsure answer falls back to).
	Options []string
	// Condition is the loop's exit condition (loop only).
	Condition string
	// AgentID is the agent whose output is being judged, for usage billing
	// ("" when no agent node ran yet).
	AgentID string
}

// JudgeRunner is an OPTIONAL extension powering MatchJudge. Runners that do not
// implement it make judge-mode branches take their default arm (or fail when
// there is none) and judge-mode loops fail.
type JudgeRunner interface {
	// JudgeBranch returns the index of the option that describes the value,
	// or -1 when no option is a confident fit. confidence is the model's
	// probability behind its pick.
	JudgeBranch(ctx context.Context, req JudgeRequest) (pick int, confidence float64, err error)
	// JudgeCondition reports whether the condition holds for the value.
	JudgeCondition(ctx context.Context, req JudgeRequest) (holds bool, probability float64, err error)
}

// evalBranchJudge routes a judge-mode branch. An unsure answer takes the default
// arm like a text branch with no matching arm; a judge that cannot answer at all
// (not wired, decider off or failing) also takes the default arm, and fails the
// node when there is none — a flow must not silently end because a decision
// service was down.
func (e *Engine) evalBranchJudge(ctx context.Context, g Graph, node Node, st State) (next, label string, err error) {
	value := st.Last
	if node.JSONField != "" {
		if v, ok := jsonTopField(value, node.JSONField); ok {
			value = v
		}
	}
	var arms []Branch
	var def *Branch
	for i := range node.Branches {
		if node.Branches[i].Contains == "" {
			def = &node.Branches[i]
			continue
		}
		arms = append(arms, node.Branches[i])
	}
	fallback := func(why string) (string, string, error) {
		if def != nil {
			return def.Next, "default (" + why + ")", nil
		}
		return "", "", fmt.Errorf("judge could not route (%s) and the branch has no default arm", why)
	}
	jr, ok := e.runner.(JudgeRunner)
	if !ok {
		return fallback("judge not available")
	}
	if len(arms) == 0 {
		return fallback("no options")
	}
	options := make([]string, len(arms))
	for i, a := range arms {
		options[i] = a.Contains
	}
	pick, conf, err := jr.JudgeBranch(WithNodeID(ctx, node.ID), JudgeRequest{
		Value: value, Question: node.JudgeQuestion, Options: options, AgentID: lastAgentID(g, st),
	})
	if err != nil {
		return fallback("judge error: " + shortError(err))
	}
	if pick < 0 || pick >= len(arms) {
		if def != nil {
			return def.Next, fmt.Sprintf("default (judge unsure, %.2f)", conf), nil
		}
		return "", fmt.Sprintf("no match (judge unsure, %.2f)", conf), nil
	}
	return arms[pick].Next, fmt.Sprintf("%s (judge %.2f)", arms[pick].Contains, conf), nil
}

// judgeLoopDone asks whether a judge-mode loop's exit condition holds. A judge
// error ends the loop with an error when nothing else bounds it (MaxIters == 0);
// with an iteration cap the loop simply goes around again.
func (e *Engine) judgeLoopDone(ctx context.Context, g Graph, node Node, st State) (bool, error) {
	jr, ok := e.runner.(JudgeRunner)
	if !ok {
		return false, fmt.Errorf("loop %q uses the judge match mode but no judge is available", node.ID)
	}
	holds, _, err := jr.JudgeCondition(WithNodeID(ctx, node.ID), JudgeRequest{
		Value: st.Last, Question: node.JudgeQuestion, Condition: node.Until, AgentID: lastAgentID(g, st),
	})
	if err != nil {
		if node.MaxIters > 0 {
			return false, nil
		}
		return false, fmt.Errorf("loop judge failed: %w", err)
	}
	return holds, nil
}

// loopUntilHolds evaluates a loop's exit condition in either mode.
func (e *Engine) loopUntilHolds(ctx context.Context, g Graph, node Node, st State) (bool, error) {
	if node.UntilMode == MatchJudge {
		return e.judgeLoopDone(ctx, g, node, st)
	}
	return loopMatches(node, st.Last), nil
}

// shortError keeps a judge error readable inside a trace label.
func shortError(err error) string {
	const maxLen = 120
	s := err.Error()
	if r := []rune(s); len(r) > maxLen {
		return string(r[:maxLen]) + "…"
	}
	return s
}

// lastAgentID returns the agent of the most recent agent/coordinator node in the
// trace — the one whose output a judge is about to evaluate.
func lastAgentID(g Graph, st State) string {
	for i := len(st.Trace) - 1; i >= 0; i-- {
		t := st.Trace[i]
		if t.Type != NodeAgent && t.Type != NodeCoordinator {
			continue
		}
		if n, ok := g.node(t.NodeID); ok && n.AgentID != "" {
			return n.AgentID
		}
	}
	return ""
}
