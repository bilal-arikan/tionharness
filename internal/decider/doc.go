// Package decider is the decision-model layer: a small, provider-neutral seam for
// asking a fast model typed questions about some state and getting calibrated,
// machine-usable answers back — yes/no probabilities, one pick out of labelled
// options, or a level on an ordered scale — instead of free text.
//
// The layer is deliberately separate from internal/providers. A provider is a
// chat transport (messages in, text and tool calls out) and every picker in the
// app offers its models as agent models. A decision model answers nothing but
// typed questions, so it must never appear there; it is reached through its own
// endpoint and configured on its own settings page.
//
// Pieces:
//
//   - Request / Question / Answer / Response: the neutral wire model (types.go).
//   - Backend: one decision-model transport, self-registered like provider kinds
//     (backend.go). The first backend is OpenRouter's Decisions API serving
//     TypeSafe's Jev (openrouter.go); alternatives plug in as further backends
//     without touching callers.
//   - Config / Site: which backend, model and provider instance to use, and how
//     each consumer site in the app uses the layer (off / shadow / on).
//   - Hub: the app-wide service callers talk to. It resolves credentials from a
//     provider instance, caches the client, redacts and bounds the state, backs
//     off from a failing endpoint and keeps a local ledger of every decision so
//     shadow-mode agreement can be measured before a site is switched on.
//
// The package imports no other internal package: credentials arrive through the
// EndpointSource interface and usage accounting is the caller's job.
package decider
