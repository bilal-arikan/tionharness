package decider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Prompting for the LLM-logprobs backend: each typed question becomes a small
// multiple choice whose answer is a single label, so the whole decision sits in
// one token's probability distribution.

// choiceLabels are the labels a choice question's options get, in order.
const choiceLabels = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const classifierSystemPrompt = "You are a careful classifier. You read a state and answer one question about it " +
	"by replying with the label of exactly one option. You never explain and never add other text."

// labelSet maps the labels shown to the model onto the answer's vocabulary:
// yes/no for a noul question, option keys for a choice, level indexes for a
// score.
type labelSet struct {
	qtype  QuestionType
	labels []string
	keys   []string
}

// labelsFor labels a question's options: A/B for yes/no, A…Z then a…z for a
// choice's options (sorted by key, so the prompt is deterministic), digits for
// a score's levels.
func labelsFor(q Question) labelSet {
	ls := labelSet{qtype: q.Type}
	switch q.Type {
	case QuestionNoul:
		ls.labels = []string{"A", "B"}
		ls.keys = []string{"yes", "no"}
	case QuestionChoice:
		for i, k := range sortedKeys(q.Options) {
			ls.labels = append(ls.labels, choiceLabels[i:i+1])
			ls.keys = append(ls.keys, k)
		}
	case QuestionScore:
		for i := range q.Levels {
			ls.labels = append(ls.labels, strconv.Itoa(i))
			ls.keys = append(ls.keys, strconv.Itoa(i))
		}
	}
	return ls
}

// noulSynonyms lets a yes/no answer count when the model writes the word
// instead of the label.
var noulSynonyms = map[string]string{
	"yes": "A", "y": "A", "true": "A",
	"no": "B", "n": "B", "false": "B",
}

// match reports which label a generated token stands for: surrounding spaces,
// brackets, quotes and punctuation are ignored ("A)", " A", "**A**").
func (ls labelSet) match(token string) (string, bool) {
	t := strings.Trim(strings.TrimSpace(token), "()[]{}<>.:;,*\"'`")
	if t == "" {
		return "", false
	}
	for _, l := range ls.labels {
		if t == l {
			return l, true
		}
	}
	if ls.qtype == QuestionNoul {
		if l, ok := noulSynonyms[strings.ToLower(t)]; ok {
			return l, true
		}
	}
	return "", false
}

// renderState turns a request's state into prompt text: a string as is, any
// other value as compact JSON.
func renderState(state any) (string, error) {
	if s, ok := state.(string); ok {
		return s, nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode decision state: %w", err)
	}
	return string(raw), nil
}

// renderQuestion is the user message for one question.
func renderQuestion(state string, q Question, ls labelSet, suffix string) string {
	var b strings.Builder
	b.WriteString("<state>\n")
	b.WriteString(state)
	b.WriteString("\n</state>\n\nQuestion: ")
	b.WriteString(strings.TrimSpace(q.Instructions))
	b.WriteString("\n\n")
	switch q.Type {
	case QuestionNoul:
		b.WriteString("Options:\n")
		b.WriteString("A) Yes." + criterion(q.True) + "\n")
		b.WriteString("B) No." + criterion(q.False) + "\n")
	case QuestionChoice:
		b.WriteString("Options:\n")
		for i, l := range ls.labels {
			fmt.Fprintf(&b, "%s) %s\n", l, oneLine(q.Options[ls.keys[i]]))
		}
	case QuestionScore:
		b.WriteString("Levels, lowest first:\n")
		for i, l := range ls.labels {
			fmt.Fprintf(&b, "%s) %s\n", l, oneLine(q.Levels[i]))
		}
	}
	fmt.Fprintf(&b, "\nReply with one label (%s) and nothing else.", labelList(ls.labels))
	if suffix != "" {
		b.WriteString(" ")
		b.WriteString(suffix)
	}
	return b.String()
}

func criterion(s string) string {
	if s = oneLine(s); s == "" {
		return ""
	}
	return " " + s
}

// oneLine collapses whitespace so an option stays on its own line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// labelList renders labels for the reply instruction: "A or B", "A, B, C",
// or a range for long lists ("A-Z, a-f").
func labelList(labels []string) string {
	switch n := len(labels); {
	case n == 2:
		return labels[0] + " or " + labels[1]
	case n <= 6:
		return strings.Join(labels, ", ")
	case n <= 26:
		return labels[0] + "-" + labels[n-1]
	default:
		return labels[0] + "-" + labels[25] + ", " + labels[26] + "-" + labels[n-1]
	}
}
