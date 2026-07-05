"""Generate bundled marketplace example packs (skill/agent/provider/flow).

Run from the TionSwarm repo root:
    python internal/market/gen_examples.py

Writes <kind>.<slug>.swarmpack.json files into internal/market/defaults/.
This is a one-off authoring helper kept in-tree for reproducibility; the JSON
files it emits are the source of truth (embedded via //go:embed).
"""
import json
import os

OUT = os.path.join(os.path.dirname(__file__), "defaults")
os.makedirs(OUT, exist_ok=True)


def skill_md(name, desc, when, icon, color, body):
    return (
        f"---\n"
        f'name: "{name}"\n'
        f'description: "{desc}"\n'
        f'when_to_use: "{when}"\n'
        f'icon: "{icon}"\n'
        f'color: "{color}"\n'
        f"access: shared\n"
        f"---\n"
        f"{body}"
    )


def pack(kind, slug, name, desc, icon, color, tags, payload, version="1.0.0", author="tionswarm"):
    return {
        "schema": "swarmpack/v1",
        "id": f"{kind}.{slug}",
        "kind": kind,
        "name": name,
        "description": desc,
        "version": version,
        "author": author,
        "icon": icon,
        "color": color,
        "tags": tags,
        "payload": payload,
    }


def skill_pack(slug, name, desc, when, icon, color, tags, body):
    return pack("skill", slug, name, desc, icon, color, tags,
                {"skill": {"slug": slug, "body": skill_md(name, desc, when, icon, color, body)}})


def agent_pack(slug, name, desc, icon, color, tags, soul, identity,
               provider="claude-cli", model="", thinking="medium",
               permission="ask", skills=None):
    return pack("agent", slug, name, desc, icon, color, tags, {"agent": {
        "name": name, "soul": soul, "identity": identity,
        "provider": provider, "model": model, "thinkingLevel": thinking,
        "permissionMode": permission, "avatar": icon, "color": color,
        "mcpEnabled": True, "skills": skills or [],
    }})


def provider_pack(slug, label, desc, icon, color, base, default_model, models):
    return pack("provider", slug, label, desc, icon, color, ["provider", "llm"], {"provider": {
        "label": label, "kind": "openai", "baseUrl": base,
        "defaultModel": default_model, "models": models,
    }})


def flow_pack(slug, name, desc, icon, color, graph):
    # desc is the PACK-level description (kept); flows themselves have no
    # description field anymore (removed app-wide 2026-07-04).
    return pack("flow", slug, name, desc, icon, color, ["flow", "orchestration"],
                {"flow": {"name": name, "graph": json.dumps(graph)}})


PACKS = []

# ---------------------------------------------------------------- skills (8 new)
PACKS.append(skill_pack(
    "technical-writing", "Technical Writing",
    "Write clear, accurate technical docs: lead with the answer, show examples, cut filler.",
    "When writing or editing READMEs, API docs, guides, or release notes", "📝", "#0ea5e9",
    ["writing", "docs"],
    "# Technical Writing\n\n## Principles\n\n- **Answer first.** Open with what the reader needs; put background later.\n- **Show, then tell.** A runnable example beats a paragraph of prose.\n- **One idea per sentence.** Cut adverbs, hedges, and throat-clearing.\n- **Be concrete.** Replace \"various options\" with the actual list.\n\n## Structure\n\n1. What it is (one sentence).\n2. How to use it (minimal working example).\n3. Reference detail (options, edge cases).\n4. Troubleshooting (common errors → fixes).\n\n## Checklist\n\n- Every code block runs as written.\n- No undefined jargon on first use.\n- Headings are scannable; a reader can find their answer in 10 seconds.\n"))

PACKS.append(skill_pack(
    "data-analysis", "Data Analysis",
    "Turn a dataset into a defensible answer: frame, clean, explore, validate, summarise.",
    "When analysing data, computing metrics, or answering a quantitative question", "📊", "#22c55e",
    ["data", "analysis"],
    "# Data Analysis\n\n## Workflow\n\n1. **Frame** — restate the question and the metric that answers it.\n2. **Inspect** — shape, types, missing values, obvious outliers.\n3. **Clean** — handle nulls/dupes explicitly; record every transformation.\n4. **Explore** — distributions and relationships before conclusions.\n5. **Validate** — sanity-check against a known total or a second method.\n6. **Summarise** — answer first, then the supporting numbers and caveats.\n\n## Pitfalls\n\n- Correlation is not causation — say so.\n- Averages hide skew; report spread too.\n- State sample size and time window for every figure.\n"))

PACKS.append(skill_pack(
    "debugging", "Systematic Debugging",
    "Find root causes with the scientific method instead of guess-and-check.",
    "When investigating a bug, failure, or unexpected behaviour", "🐞", "#ef4444",
    ["debugging", "code"],
    "# Systematic Debugging\n\n## Loop\n\n1. **Reproduce** — a reliable, minimal repro is half the fix.\n2. **Observe** — read the actual error and the surrounding state; don't assume.\n3. **Hypothesise** — one falsifiable cause at a time.\n4. **Test** — change ONE thing; predict the outcome before running.\n5. **Locate** — bisect (git, input, code path) to narrow the region.\n6. **Fix + verify** — confirm the repro is gone AND nothing else broke.\n\n## Rules\n\n- Read the stack trace bottom-up to the first line you own.\n- Question your assumptions before the framework's.\n- If stuck, explain the problem aloud — the gap usually surfaces.\n"))

PACKS.append(skill_pack(
    "prompt-engineering", "Prompt Engineering",
    "Design prompts that are specific, structured, and testable.",
    "When writing or improving an LLM prompt or system message", "✨", "#a855f7",
    ["llm", "prompting"],
    "# Prompt Engineering\n\n## Anatomy\n\n- **Role** — who the model is and its expertise.\n- **Task** — the exact goal, in one sentence.\n- **Context** — inputs, constraints, definitions of done.\n- **Format** — the precise output shape (and an example).\n- **Guardrails** — what NOT to do; how to handle uncertainty.\n\n## Techniques\n\n- Give one or two worked examples (few-shot) for tricky formats.\n- Ask for reasoning before the answer when accuracy matters.\n- Prefer explicit enums (\"reply POSITIVE or NEGATIVE\") over free text.\n- Iterate against a small fixed test set, not vibes.\n"))

PACKS.append(skill_pack(
    "sql-expert", "SQL Expert",
    "Write correct, readable, performant SQL and explain query plans.",
    "When writing, reviewing, or optimising SQL queries", "🗃️", "#f59e0b",
    ["sql", "database"],
    "# SQL Expert\n\n## Writing\n\n- Start from the grain: what does one row of the result mean?\n- Filter early (WHERE) before you aggregate (GROUP BY/HAVING).\n- Be explicit with JOIN types; never rely on implicit cross joins.\n- Alias tables; qualify every column in multi-table queries.\n\n## Performance\n\n- Read the EXPLAIN plan: seq scan on a big table = missing index.\n- Index the columns you filter and join on, not everything.\n- Avoid SELECT * in production paths; fetch only needed columns.\n- Beware N+1 patterns — set-based beats row-by-row.\n"))

PACKS.append(skill_pack(
    "git-workflow", "Git Workflow",
    "Clean commits, sane branches, and safe history operations.",
    "When committing, branching, rebasing, or resolving Git trouble", "🌿", "#f97316",
    ["git", "vcs"],
    "# Git Workflow\n\n## Commits\n\n- One logical change per commit; imperative subject (\"Add X\", not \"Added\").\n- Subject ≤ 50 chars; body explains WHY, not what the diff already shows.\n- Never commit secrets, generated files, or unrelated churn.\n\n## Branches\n\n- Short-lived feature branches off an up-to-date main.\n- Rebase to keep a linear history; merge for shared long-lived branches.\n\n## Safety\n\n- Before `reset --hard` / `push --force`, check there's no safer option.\n- `git reflog` recovers almost anything — don't panic.\n- Force-push only your own branches, never shared ones.\n"))

PACKS.append(skill_pack(
    "api-design", "API Design",
    "Design REST/HTTP APIs that are consistent, predictable, and evolvable.",
    "When designing or reviewing an HTTP/REST API surface", "🔌", "#06b6d4",
    ["api", "design"],
    "# API Design\n\n## Resources\n\n- Nouns, not verbs: `/orders/{id}`, not `/getOrder`.\n- Use HTTP methods for intent (GET/POST/PUT/PATCH/DELETE) and status codes for outcome.\n- Plural collections; stable, opaque ids.\n\n## Contracts\n\n- Consistent error shape: `{ error, message, details }`.\n- Paginate lists; never return unbounded result sets.\n- Version from day one (`/v1`) and add fields additively.\n\n## Hygiene\n\n- Validate input at the edge; return 4xx with a clear message.\n- Idempotent PUT/DELETE; document side effects.\n- Never leak internals or secrets in errors.\n"))

PACKS.append(skill_pack(
    "summarization", "Summarization",
    "Compress long content to its load-bearing points without distortion.",
    "When condensing articles, threads, transcripts, or documents", "🧾", "#64748b",
    ["writing", "summary"],
    "# Summarization\n\n## Method\n\n1. Identify the purpose: what will the reader DO with this?\n2. Extract the claims that change a decision; drop the rest.\n3. Preserve nuance that flips meaning (negations, conditions, numbers).\n4. Lead with the bottom line, then supporting points.\n\n## Forms\n\n- **TL;DR** — one or two sentences.\n- **Bullets** — for parallel points or action items.\n- **Structured** — when the source has clear sections.\n\n## Don'ts\n\n- Don't invent detail to fill gaps.\n- Don't flatten disagreement into false consensus.\n"))

# ---------------------------------------------------------------- agents (5)
PACKS.append(agent_pack(
    "researcher", "Researcher", "A rigorous research analyst that gathers, cross-checks, and cites.",
    "🔎", "#0ea5e9", ["research"],
    "You are a meticulous research analyst. You value primary sources, cross-check every load-bearing fact, and separate what you know from what you infer.",
    "Researcher — gathers evidence, weighs sources, and synthesises cited answers.",
    skills=["web-research", "summarization"]))

PACKS.append(agent_pack(
    "coder", "Coder", "A pragmatic software engineer that writes clean code and tests it.",
    "💻", "#22c55e", ["engineering"],
    "You are a pragmatic senior engineer. You read the surrounding code before writing, match its conventions, keep changes small, and verify by running them.",
    "Coder — implements, refactors, and debugs with a bias for working software.",
    permission="auto", skills=["debugging", "code-review", "git-workflow"]))

PACKS.append(agent_pack(
    "editor", "Editor", "A sharp copy editor that tightens prose without changing meaning.",
    "✒️", "#a855f7", ["writing"],
    "You are a sharp copy editor. You cut filler, fix structure, and preserve the author's voice and intent. You explain non-trivial changes briefly.",
    "Editor — improves clarity, flow, and correctness of written content.",
    thinking="low", skills=["technical-writing", "summarization"]))

PACKS.append(agent_pack(
    "planner", "Planner", "A project planner that decomposes goals into ordered, checkable steps.",
    "🗺️", "#f59e0b", ["planning"],
    "You are a project planner. You turn fuzzy goals into a dependency-ordered plan with clear milestones, owners, and a definition of done for each step.",
    "Planner — breaks goals into actionable, verifiable plans.",
    thinking="high"))

PACKS.append(agent_pack(
    "support", "Support Agent", "A calm, empathetic customer-support agent.",
    "🎧", "#06b6d4", ["support"],
    "You are a calm, empathetic support agent. You acknowledge the issue, confirm understanding, give clear next steps, and never over-promise.",
    "Support — resolves customer issues with empathy and precision.",
    thinking="low", permission="ask"))

# ---------------------------------------------------------------- providers (4)
PACKS.append(provider_pack(
    "openrouter", "OpenRouter", "Unified gateway to many models (OpenAI-compatible). Bring your own key.",
    "🌐", "#6366f1", "https://openrouter.ai/api/v1",
    "anthropic/claude-3.5-sonnet",
    "anthropic/claude-3.5-sonnet\nopenai/gpt-4o\ngoogle/gemini-pro-1.5\nmeta-llama/llama-3.1-70b-instruct"))

PACKS.append(provider_pack(
    "groq", "Groq", "Ultra-fast inference for open models (OpenAI-compatible). Bring your own key.",
    "⚡", "#f97316", "https://api.groq.com/openai/v1",
    "llama-3.3-70b-versatile",
    "llama-3.3-70b-versatile\nllama-3.1-8b-instant\nmixtral-8x7b-32768"))

PACKS.append(provider_pack(
    "ollama", "Ollama (Local)", "Run open models locally via Ollama's OpenAI-compatible endpoint. No key needed.",
    "🦙", "#64748b", "http://localhost:11434/v1",
    "llama3.1",
    "llama3.1\nqwen2.5\nmistral\nphi3"))

PACKS.append(provider_pack(
    "deepseek", "DeepSeek", "DeepSeek chat & reasoning models (OpenAI-compatible). Bring your own key.",
    "🐳", "#2563eb", "https://api.deepseek.com/v1",
    "deepseek-chat",
    "deepseek-chat\ndeepseek-reasoner"))

# ---------------------------------------------------------------- flows (4)
PACKS.append(flow_pack(
    "research-synthesis", "Research → Synthesis",
    "Gather findings, then synthesise a cited, decision-ready brief.",
    "🔎", "#0ea5e9", {
        "start": "research", "edgeStyle": "smoothstep", "nodes": [
            {"id": "research", "type": "agent", "title": "Research", "agentId": "",
             "prompt": "Research this thoroughly and list key findings with sources:\n{{input}}",
             "next": "synthesize", "x": 100, "y": 60},
            {"id": "synthesize", "type": "agent", "title": "Synthesize", "agentId": "",
             "prompt": "Synthesise these findings into a decision-ready brief with citations:\n{{last}}",
             "next": "", "x": 100, "y": 240},
        ]}))

PACKS.append(flow_pack(
    "review-and-fix", "Review → Fix",
    "Review a change, branch on whether issues were found, then fix or approve.",
    "🔬", "#f59e0b", {
        "start": "review", "nodes": [
            {"id": "review", "type": "agent", "title": "Review", "agentId": "",
             "prompt": "Review this change. If you find blocking issues, start your reply with ISSUES; otherwise start with CLEAN.\n{{input}}",
             "next": "route", "x": 100, "y": 60},
            {"id": "route", "type": "branch", "title": "Route", "matchMode": "contains",
             "branches": [{"contains": "ISSUES", "next": "fix"}, {"contains": "", "next": "approve"}],
             "x": 100, "y": 240},
            {"id": "fix", "type": "agent", "title": "Fix", "agentId": "",
             "prompt": "Apply concrete fixes for the issues raised:\n{{last}}", "next": "", "x": 360, "y": 170},
            {"id": "approve", "type": "agent", "title": "Approve", "agentId": "",
             "prompt": "Write a short approval note summarising why this is good to merge.", "next": "", "x": 360, "y": 320},
        ]}))

PACKS.append(flow_pack(
    "parallel-brainstorm", "Parallel Brainstorm",
    "Three agents brainstorm angles concurrently, then a fourth merges the best ideas.",
    "💡", "#a855f7", {
        "start": "fan", "animated": True, "nodes": [
            {"id": "fan", "type": "parallel", "title": "Brainstorm", "parallel": ["a", "b", "c"],
             "joinNext": "merge", "x": 100, "y": 60},
            {"id": "a", "type": "agent", "title": "Angle A", "agentId": "",
             "prompt": "Brainstorm bold, unconventional ideas for: {{input}}", "next": "", "x": 360, "y": 20},
            {"id": "b", "type": "agent", "title": "Angle B", "agentId": "",
             "prompt": "Brainstorm practical, low-risk ideas for: {{input}}", "next": "", "x": 360, "y": 150},
            {"id": "c", "type": "agent", "title": "Angle C", "agentId": "",
             "prompt": "Brainstorm ideas focused on the end user for: {{input}}", "next": "", "x": 360, "y": 280},
            {"id": "merge", "type": "agent", "title": "Merge", "agentId": "",
             "prompt": "Merge these idea sets, dedupe, and rank the top 5:\n{{last}}", "next": "", "x": 100, "y": 320},
        ]}))

PACKS.append(flow_pack(
    "draft-edit-finalize", "Draft → Edit → Finalize",
    "Draft content, edit it critically, then produce a polished final version.",
    "📝", "#22c55e", {
        "start": "draft", "edgeStyle": "smoothstep", "nodes": [
            {"id": "draft", "type": "agent", "title": "Draft", "agentId": "",
             "prompt": "Write a first draft for:\n{{input}}", "next": "edit", "x": 100, "y": 40},
            {"id": "edit", "type": "agent", "title": "Edit", "agentId": "",
             "prompt": "Critically edit this draft — tighten, fix structure, flag weak spots:\n{{last}}",
             "next": "finalize", "x": 100, "y": 210},
            {"id": "finalize", "type": "agent", "title": "Finalize", "agentId": "",
             "prompt": "Produce the polished final version incorporating the edits:\n{{last}}",
             "next": "", "x": 100, "y": 380},
        ]}))

# ---------------------------------------------------------------- write
written = []
for p in PACKS:
    fn = f"{p['id']}.swarmpack.json"
    with open(os.path.join(OUT, fn), "w", encoding="utf-8") as f:
        json.dump(p, f, ensure_ascii=False, indent=2)
    written.append(fn)

print(f"wrote {len(written)} packs:")
for w in written:
    print(" -", w)
