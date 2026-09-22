package providers

// oaiThinking is the explicit reasoning switch DeepSeek's OpenAI-compatible
// endpoint takes ({"thinking":{"type":"enabled"|"disabled"}}); Z.ai's uses the
// same shape.
type oaiThinking struct {
	Type string `json:"type"` // "enabled" | "disabled"
}

// WithThinkingToggle makes the client send an explicit thinking switch on every
// request and returns it for chaining. Needed for endpoints that reason by
// DEFAULT (DeepSeek V4.x): without the switch an agent set to "off" still
// thinks, and in a tool loop DeepSeek then requires every earlier
// reasoning_content to be sent back (a 400 otherwise) — an echo this client does
// not perform. Off by default: an endpoint that does not know the field may
// reject it.
func (m *OpenAICompat) WithThinkingToggle() *OpenAICompat {
	m.thinkingToggle = true
	return m
}

// thinkingSwitch returns the explicit thinking switch for this request, or nil
// when the endpoint did not opt in. The native tool loop always sends a zero
// budget, so tool turns go out with reasoning disabled — which is what keeps the
// reasoning_content round-trip unnecessary. Forced-thinking models (GLM-5.3) are
// never sent "disabled" (it fails there); they stay "enabled" and effortFor
// lowers them to "low" instead.
func (m *OpenAICompat) thinkingSwitch(req Request, model string) *oaiThinking {
	if !m.thinkingToggle {
		return nil
	}
	if req.ThinkingBudget > 0 || ForcedThinking(model) {
		return &oaiThinking{Type: "enabled"}
	}
	return &oaiThinking{Type: "disabled"}
}
