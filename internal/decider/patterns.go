package decider

// Pick reads the choice answer under key: the chosen option, its strength, and
// whether the strength clears threshold. The option is returned even when it
// does not, so a caller can log what the model leaned towards.
func Pick(resp *Response, key string, threshold float64) (choice string, strength float64, ok bool) {
	if resp == nil {
		return "", 0, false
	}
	a, found := resp.Answers[key]
	if !found || a.Type != QuestionChoice || a.Choice == "" {
		return "", 0, false
	}
	s := a.Strength()
	return a.Choice, s, s >= threshold
}
