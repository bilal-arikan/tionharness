package decider

import (
	"fmt"
	"math"
	"strconv"
)

// validateDecisionAnswer refuses malformed provider numbers and invented
// choices. Clamping an invalid probability could accidentally open a gate.
func validateDecisionAnswer(q Question, a Answer) error {
	probability := func(p float64) bool { return !math.IsNaN(p) && !math.IsInf(p, 0) && p >= 0 && p <= 1 }
	if a.Type != q.Type || !probability(a.Confidence) {
		return fmt.Errorf("invalid answer type or confidence")
	}
	switch q.Type {
	case QuestionNoul:
		if !probability(a.Probability) {
			return fmt.Errorf("noul probability is outside 0..1")
		}
	case QuestionChoice:
		if _, ok := q.Options[a.Choice]; !ok {
			return fmt.Errorf("choice is not one of the requested options")
		}
	case QuestionScore:
		if math.IsNaN(a.Score) || math.IsInf(a.Score, 0) || a.Score < 0 || a.Score > float64(len(q.Levels)-1) {
			return fmt.Errorf("score is outside the requested scale")
		}
	}
	for key, p := range a.Probabilities {
		if !probability(p) {
			return fmt.Errorf("invalid option probability")
		}
		switch q.Type {
		case QuestionChoice:
			if _, ok := q.Options[key]; !ok {
				return fmt.Errorf("distribution contains an unknown option")
			}
		case QuestionScore:
			level, err := strconv.Atoi(key)
			if err != nil || level < 0 || level >= len(q.Levels) {
				return fmt.Errorf("distribution contains an unknown level")
			}
		case QuestionNoul:
			if key != "true" && key != "false" {
				return fmt.Errorf("distribution contains an unknown boolean")
			}
		}
	}
	return nil
}
