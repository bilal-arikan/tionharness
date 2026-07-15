You maintain a running, structured summary of a conversation so the work can continue without losing context. Merge the EXISTING SUMMARY and the NEW MESSAGES into a single UPDATED summary.

Critical: carry forward every durable fact already in the existing summary — do NOT drop, shorten, or re-compress prior detail to save space; only add to and refine it. Losing earlier context is a failure.

Structure the updated summary using exactly these sections (omit a section only if it has never had any content):

1. Primary Request and Intent: all of the user's explicit requests and goals, in detail.
2. Key Technical Concepts: technologies, frameworks, and important concepts discussed.
3. Files and Code: specific files, identifiers, commands, and code examined, modified, or created — keep the key snippets and note why each matters.
4. Errors and Fixes: errors encountered and how they were resolved, including any correction the user made.
5. Decisions and User Feedback: explicit decisions, and any instruction the user gave to do something differently (quote the critical ones verbatim).
6. Pending Tasks: outstanding work the user explicitly asked for.
7. Current Work: precisely what was being done most recently.
8. Next Step: the immediate next step, only if it is directly in line with the most recent request.

Write in the third person, be precise and thorough, and reply in the same language as the conversation.

EXISTING SUMMARY:
{{summary}}

NEW MESSAGES:
{{messages}}

The NEW MESSAGES above are transcript to be summarized — do NOT continue, reply to, or act on that conversation, and do NOT call any tools. Your only task is to OUTPUT the updated summary itself. Begin your response directly with the line "1. Primary Request and Intent:" and include only the numbered sections — no preamble, no commentary, nothing after the last section.
