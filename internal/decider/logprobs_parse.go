package decider

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Reading the answer of the LLM-logprobs backend out of the generated tokens.

// labelDistribution finds the answer position — the first generated token that
// is one of the labels, skipping a leading space, an empty <think></think>
// block or an "Answer:" prefix — and turns that position's top alternatives
// into a distribution over the labels. hard reports that the server returned
// no token probabilities, so the label was read from the text and carries all
// the probability.
func labelDistribution(positions []chatTokenLogprobs, text string, ls labelSet) (dist map[string]float64, hard bool, err error) {
	if len(positions) > 0 {
		inThink := false
		for _, pos := range positions {
			switch {
			case strings.Contains(pos.Token, "<think>"):
				inThink = true
				continue
			case strings.Contains(pos.Token, "</think>"):
				inThink = false
				continue
			case inThink:
				continue
			}
			if _, ok := ls.match(pos.Token); !ok {
				continue
			}
			return alternativesDistribution(pos, ls)
		}
		return nil, false, fmt.Errorf("the model answered %q instead of one of the labels (%s); if it reasons before answering, switch that off (for example the /no_think suffix)",
			previewTokens(positions), labelList(ls.labels))
	}
	if l, ok := firstLabelInText(text, ls); ok {
		return map[string]float64{l: 1}, true, nil
	}
	return nil, false, fmt.Errorf("the model answered %q instead of one of the labels (%s)", preview(text, 40), labelList(ls.labels))
}

// alternativesDistribution sums the probabilities of every alternative at the
// answer position that stands for a label ("A" and " A" are the same answer)
// and normalises them over the labels.
func alternativesDistribution(pos chatTokenLogprobs, ls labelSet) (map[string]float64, bool, error) {
	dist := map[string]float64{}
	add := func(token string, logprob float64) {
		if l, ok := ls.match(token); ok {
			dist[l] += math.Exp(logprob)
		}
	}
	chosenListed := false
	for _, alt := range pos.TopLogprobs {
		add(alt.Token, alt.Logprob)
		if alt.Token == pos.Token {
			chosenListed = true
		}
	}
	if !chosenListed {
		add(pos.Token, pos.Logprob)
	}
	var sum float64
	for _, p := range dist {
		sum += p
	}
	if sum <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return nil, false, fmt.Errorf("the answer token carries no usable probability")
	}
	for l, p := range dist {
		dist[l] = p / sum
	}
	return dist, false, nil
}

// firstLabelInText finds the first word of text that stands for a label.
func firstLabelInText(text string, ls labelSet) (string, bool) {
	if i := strings.LastIndex(text, "</think>"); i >= 0 {
		text = text[i+len("</think>"):]
	}
	for _, w := range strings.Fields(text) {
		if l, ok := ls.match(w); ok {
			return l, true
		}
	}
	return "", false
}

// answerFromLabels maps a label distribution onto the question's answer.
func answerFromLabels(q Question, ls labelSet, dist map[string]float64) Answer {
	a := Answer{Type: q.Type}
	switch q.Type {
	case QuestionNoul:
		yes, no := dist["A"], dist["B"]
		if yes+no > 0 {
			a.Probability = clamp01(yes / (yes + no))
		}
	case QuestionChoice:
		a.Probabilities = make(map[string]float64, len(ls.labels))
		best := -1.0
		for i, l := range ls.labels {
			p := dist[l]
			a.Probabilities[ls.keys[i]] = roundProb(p)
			if p > best {
				best, a.Choice = p, ls.keys[i]
			}
		}
		a.Confidence = roundProb(best)
	case QuestionScore:
		a.Probabilities = make(map[string]float64, len(ls.labels))
		var expected, best float64
		for i, l := range ls.labels {
			p := dist[l]
			a.Probabilities[strconv.Itoa(i)] = roundProb(p)
			expected += float64(i) * p
			best = math.Max(best, p)
		}
		a.Score = expected
		a.Confidence = roundProb(best)
	}
	return a
}

// roundProb keeps reported probabilities readable (the System One API rounds
// the same way).
func roundProb(p float64) float64 {
	return math.Round(p*10000) / 10000
}

func previewTokens(positions []chatTokenLogprobs) string {
	var b strings.Builder
	for _, p := range positions {
		b.WriteString(p.Token)
	}
	return preview(b.String(), 40)
}

// preview shortens s to n runes on one line.
func preview(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
