You are continuing a long-running task in a FRESH context window (a context reset / handoff). Your previous session reached its context limit; this is a clean slate. Do NOT restart from scratch — continue from the "Next Concrete Step" in the handoff below.

# Handoff
{{handoff}}

---
Recovery: the previous session id is `{{oldSession}}`. For exact pre-reset detail not captured above (a code snippet, error message, file contents, or a specific decision), do not guess — use `conversation_search` with session_id="{{oldSession}}", or re-open the referenced files with your file tools. This handoff is also saved as artifact `{{artifact}}`{{fileNote}}.
