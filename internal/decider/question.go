package decider

import (
	"fmt"
	"sort"
	"strings"
)

// Limits on a request's shape. They are far below what the services accept and
// exist to catch a caller bug (a question per file in a repository) before it
// turns into an expensive or rejected call.
const (
	maxQuestions    = 64
	maxQuestionKey  = 64
	maxChoiceOption = 32
	maxScoreLevels  = 16
)

// Noul builds a yes/no question. ifTrue and ifFalse describe when the answer is
// yes and no; pass both, or neither.
func Noul(instructions, ifTrue, ifFalse string) Question {
	return Question{Type: QuestionNoul, Instructions: instructions, True: ifTrue, False: ifFalse}
}

// Choice builds a pick-one question over options (option key → description).
func Choice(instructions string, options map[string]string) Question {
	return Question{Type: QuestionChoice, Instructions: instructions, Options: options}
}

// Score builds an ordinal question. levels are ordered lowest first; the answer
// is an index into them.
func Score(instructions string, levels ...string) Question {
	return Question{Type: QuestionScore, Instructions: instructions, Levels: levels}
}

// Normalized returns a copy of the request with every question made acceptable
// to the services: a noul question with only one side described gets a neutral
// description for the other (the Decisions API rejects a half-specified pair).
func (r Request) Normalized() Request {
	out := r
	out.Questions = make(map[string]Question, len(r.Questions))
	for k, q := range r.Questions {
		if q.Type == QuestionNoul {
			q.True = strings.TrimSpace(q.True)
			q.False = strings.TrimSpace(q.False)
			switch {
			case q.True != "" && q.False == "":
				q.False = "Otherwise: the description above does not hold."
			case q.False != "" && q.True == "":
				q.True = "Otherwise: the description above does not hold."
			}
		}
		out.Questions[k] = q
	}
	return out
}

// Validate checks a request before it is sent: a state, at least one question,
// well-formed keys, and criteria shaped for each question's type.
func (r Request) Validate() error {
	if r.State == nil {
		return fmt.Errorf("decision request has no state")
	}
	if s, ok := r.State.(string); ok && strings.TrimSpace(s) == "" {
		return fmt.Errorf("decision request has an empty state")
	}
	if len(r.Questions) == 0 {
		return fmt.Errorf("decision request has no questions")
	}
	if len(r.Questions) > maxQuestions {
		return fmt.Errorf("decision request has %d questions (max %d)", len(r.Questions), maxQuestions)
	}
	for _, k := range sortedKeys(r.Questions) {
		if err := validKey(k); err != nil {
			return fmt.Errorf("question key %q: %w", k, err)
		}
		if err := r.Questions[k].validate(); err != nil {
			return fmt.Errorf("question %q: %w", k, err)
		}
	}
	return nil
}

func (q Question) validate() error {
	switch q.Type {
	case QuestionNoul:
		return nil
	case QuestionChoice:
		if len(q.Options) < 2 {
			return fmt.Errorf("a choice needs at least two options")
		}
		if len(q.Options) > maxChoiceOption {
			return fmt.Errorf("a choice allows at most %d options", maxChoiceOption)
		}
		for k := range q.Options {
			if err := validKey(k); err != nil {
				return fmt.Errorf("option %q: %w", k, err)
			}
		}
		return nil
	case QuestionScore:
		if len(q.Levels) < 2 {
			return fmt.Errorf("a score needs at least two levels")
		}
		if len(q.Levels) > maxScoreLevels {
			return fmt.Errorf("a score allows at most %d levels", maxScoreLevels)
		}
		for i, l := range q.Levels {
			if strings.TrimSpace(l) == "" {
				return fmt.Errorf("score level %d is empty", i)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown question type %q", q.Type)
	}
}

// validKey accepts the identifier-like keys callers use for questions and
// options: letters, digits, '_', '-', '.', at most maxQuestionKey bytes.
func validKey(k string) error {
	if k == "" {
		return fmt.Errorf("empty key")
	}
	if len(k) > maxQuestionKey {
		return fmt.Errorf("longer than %d bytes", maxQuestionKey)
	}
	for _, c := range k {
		ok := c == '_' || c == '-' || c == '.' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			return fmt.Errorf("invalid character %q", c)
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
