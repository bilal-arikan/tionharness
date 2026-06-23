// Package e2e holds black-box, end-to-end tests that drive a fully wired agent
// runtime (real file store, memory, skills, sandbox, conversation manager)
// through the same turn pipeline the chat API uses — only the LLM is replaced by
// a deterministic, network-free scripted provider. The tests cover the behaviours
// that unit tests cannot reach as a whole: multi-turn conversations, the native
// tool-use loop, on-demand skill loading, memory (core + recall), and long
// sessions that overflow the context budget and compact.
//
// There is no production code in this package; everything lives in *_test.go.
package e2e
