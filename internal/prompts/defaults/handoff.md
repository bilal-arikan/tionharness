You are writing a HANDOFF document so a DIFFERENT agent, starting fresh in a clean context window, can take over a long-running task without losing progress and without restarting from scratch. The previous context is about to be discarded entirely — this document is the ONLY thing that survives, so it must carry every durable fact needed to continue.

Merge the EXISTING SUMMARY, the TRANSCRIPT, and the ENVIRONMENT SNAPSHOT into one handoff. Carry forward every durable fact — do NOT drop or re-compress prior detail to save space; losing earlier context is a failure.

Structure the handoff using exactly these sections (omit one only if it has never had any content):

1. Objective / Primary Intent: the overarching goal and all explicit user requests, in detail.
2. Key Technical Concepts: technologies, frameworks, and important concepts in play.
3. Files and Code: specific files, identifiers, commands, and code created or modified — keep key snippets and note why each matters and its current state.
4. Errors and Fixes: errors hit and how they were resolved, including any user correction.
5. Decisions and User Feedback: explicit decisions and any instruction to do something differently (quote the critical ones verbatim).
6. Progress — DONE: a checklist of what is already complete (use "- [x]").
7. Pending — TODO: an ordered checklist of what remains (use "- [ ]"), most important first.
8. Environment State: working directory, git branch/status, active todos, and existing artifacts (take these from the ENVIRONMENT SNAPSHOT).
9. Next Concrete Step: the single, immediate next action the fresh agent should take. Be specific and actionable.

Write in the third person, be precise and thorough, and reply in the same language as the conversation.

EXISTING SUMMARY:
{{summary}}

TRANSCRIPT:
{{transcript}}

ENVIRONMENT SNAPSHOT:
{{environment}}

The TRANSCRIPT above is conversation to be summarized into a handoff — do NOT continue, reply to, or act on it, and do NOT call any tools. Output ONLY the handoff document. Begin directly with the line "1. Objective / Primary Intent:" and include only the numbered sections — no preamble, no commentary, nothing after the last section.
