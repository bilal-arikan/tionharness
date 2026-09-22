// Package decider is the decision-model layer: a small, provider-neutral seam for
// asking a fast model typed questions about some state and getting calibrated,
// machine-usable answers back — yes/no probabilities, one pick out of labelled
// options, or a level on an ordered scale — instead of free text.
//
// The layer is deliberately separate from internal/providers. A provider is a
// chat transport (messages in, text and tool calls out) and every picker in the
// app offers its models as agent models. A decision model answers nothing but
// typed questions, so it must never appear there; it is configured on its own
// settings page.
//
// Pieces:
//
//   - Request / Question / Answer / Response: the neutral wire model (types.go).
//   - Backend: one decision-model transport, self-registered like provider
//     kinds (backend.go): OpenRouter's Decisions API (openrouter.go), the
//     System One API of TypeSafe and the open OpenJev servers (systemone.go),
//     and an adapter that turns any chat model with token probabilities —
//     Ollama, LM Studio, llama.cpp, vLLM — into a decision model (logprobs.go).
//   - ModelInstance: a configured decision model ("karar modeli"), created and
//     edited like a provider instance: a backend, an endpoint with its own
//     encrypted key or a provider account's borrowed one, a model id
//     (models.go, modelstore.go).
//   - Authority: one place in the app that hands a decision to a model ("karar
//     mercii"), self-registered by the package that implements it
//     (authority.go). Each gets an off / shadow / on switch, a threshold and
//     its own model with an optional fallback and challenger (config.go).
//   - Patterns: Pick, Select and Triage, the shapes most authorities take
//     (patterns.go), so a new authority is a request builder plus wiring.
//   - Hub: the app-wide service callers talk to. It resolves each model's
//     endpoint, caches its client, redacts and bounds the state, rests a
//     failing model, tries the fallback, runs the challenger in the background
//     and keeps a local ledger of every decision, so agreement can be measured
//     before an authority is switched on or a model replaced.
//
// The package imports no other internal package: provider credentials arrive
// through the EndpointSource interface, secrets are sealed by a SecretBox the
// app passes in, and usage accounting is the caller's job (WithBilling).
package decider
