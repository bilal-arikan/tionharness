---
name: "TionSwarm Doc Improver"
description: "A 6-criterion audit-and-improve workflow for TionSwarm's own context documents — skills (SKILL.md), workspace CLAUDE.md/AGENTS.md rules, and _Docs reference files. Scores each doc on commands, architecture clarity, non-obvious gotchas, brevity, currency and actionability, then applies only the edits that provide real benefit. Built to catch the 'changelog-leak' anti-pattern where reference docs silently accumulate dated history that belongs in a progress log."
when_to_use: "When a SKILL.md, CLAUDE.md, or _Docs file has grown bloated, stale, or hard to scan — or after a feature lands and you want to verify the docs that describe it are still tight and correct. Use it to audit one doc or sweep many, produce a scored report, and apply targeted trims without losing unique information."
icon: "🩺"
color: "#14b8a6"
---
# TionSwarm — Doc Improver (audit & tighten context docs)

Context documents are part of the agent's runtime: skills sit in the prompt, the
workspace CLAUDE.md shapes every turn, and `_Docs` is the project's written record. When
they bloat or drift, every turn pays for it. This skill is a repeatable audit that
scores a doc on six dimensions and applies **only the edits that earn their place**
— a repeatable discipline adapted to TionSwarm's own doc surfaces.

> TionSwarm's context lives in many places (skills, workspace rules, `_Docs`), so this
> audit generalises `CLAUDE.md`-style hygiene across all of them.

## What this skill audits

| Surface | Where | Nature |
|---------|-------|--------|
| **Skills** | `internal/skills/defaults/*/SKILL.md` (shipped) · workspace skills dir | Mostly *action* (instructions) |
| **Workspace rules** | a workspace's editable prompt/instructions, `CLAUDE.md`/`AGENTS.md` | Action |
| **Reference context** | `_Docs/*.md`, the `tionswarm-project`-style reference skill | *Reference* (knowledge) |

The **action vs reference** distinction drives how hard the brevity knob turns —
see "Calibrate by doc type" below.

## The six criteria (score each /5)

1. **Commands / workflows** — are the concrete run/build/test commands present and
   copy-pasteable (Bash on this machine — PowerShell only for Windows-native work
   Bash cannot do), or buried in prose?
2. **Architecture clarity** — folder map, tech stack, key decisions legible at a glance?
3. **Non-obvious patterns (gotchas)** — the traps a fresh agent would hit
   (`--bare` breaks claude-cli login, port 8080 collides with unity-mcp, the vite
   IPv6 fix). These are the highest-value lines in any TionSwarm doc — never cut them.
4. **Brevity** — does every sentence do work, or is there padding / duplication /
   leaked changelog?
5. **Currency** — does it match the code as it is *now*? Removed features marked,
   dates absolute?
6. **Actionability** — can a reader act on it without assembling fragments?

Report a total /30 with a one-line verdict per criterion (see report format below).

## The #1 TionSwarm anti-pattern: changelog-leak

Reference docs (especially `tionswarm-project` and `_Docs` mechanics sections) tend
to absorb dated, blow-by-blow history — *"X cache (2026-06-25, file.go): now parses
… → field …; also Y breakpoint (2026-06-25) …"*. That belongs in
`_Docs/05-ILERLEME.md` (the live changelog), **not** in a reference doc that answers
"what is this / how does it work". The fix:

- **Collapse to "what is true now" + a pointer.** One sentence of current behavior,
  then `Detail/changelog → _Docs/NN`. The history still exists — in the changelog,
  where it belongs.
- **Pattern:** `**<Feature>:** <one-sentence current state>. → _Docs/NN-NAME.md`.
- A reference doc that opens with *"detailed history is NOT here → _Docs/05"* and
  then leaks history anyway is the canonical smell. Hold it to its own rule.

## Calibrate by doc type (don't over-trim healthy docs)

The improver principle is **only propose edits that provide real benefit** — never
pad, and never cut working content to hit a line target.

- **Reference docs / always-loaded context** (workspace CLAUDE.md, the
  `tionswarm-project` reference skill): brevity matters *most* — these sit in the
  prompt every turn. Trim hard; delegate detail to `_Docs`.
- **Action skills loaded on demand** (the `tionswarm-*` default skills): the body is
  pulled only by `use_skill`, so body length costs little per turn — what's always
  in the prompt is the `description`/`when_to_use`. Here, **functional density is
  fine**: a tool catalog or a settings field-reference *should* be dense. Cut only
  genuine duplication or a wall-of-text paragraph that hurts scanning. Teaching
  scaffolding and deliberate cross-references are not bloat.
- If a doc is already healthy, **say so and stop.** Forcing edits on a 28/30 doc
  violates the very principle this skill teaches.

## The workflow

1. **Discover.** Find the target doc(s): `glob` for `**/SKILL.md`, the workspace
   rules, or `_Docs/*.md`. For a sweep, measure size first (`Bash` line/char count)
   to prioritise the biggest offenders.
2. **Read fully.** Never audit from a summary — read the whole doc.
3. **Score** against the six criteria; identify the doc type to set the brevity bar.
4. **Report first, then edit.** Present the scored table + the specific findings and
   proposed diffs. In Execute mode you may proceed directly for an obvious win, but
   for a heavy rewrite show the plan and get a nod — a doc rewrite is a judgment call.
5. **Apply with `Edit`.** Preserve every unique fact and every gotcha. Prefer
   condensing over deleting; move detail to `_Docs` rather than dropping it.
6. **Validate & doc-sync** (below).

## After editing: validate and doc-sync

- **Validate a skill** you changed with `skill_validate <slug>` (or `config_validate`)
  so the frontmatter still parses.
- **Default skills are auto-discovered.** `internal/skills/defaults.go` embeds the
  whole `defaults/` tree (`//go:embed defaults`) and derives slugs from the
  subdirectories — so **adding a new default skill needs no Go change**, only a new
  `<slug>/SKILL.md` folder. Existing installs are refreshed VERSION-AWARE
  (`EnsureDefaults` + `.shipped-versions.json`): a pristine prior-shipped copy is
  updated in place, and a pristine BODY under user-tuned frontmatter
  (access/group/visibility) gets the new body with the frontmatter preserved;
  real user edits are never overwritten. A fresh global dir gets the new skill
  seeded on startup.
- **Doc-sync check (honest):** a `_Docs/05-ILERLEME.md` entry is for *behavioral*
  changes (a shipped feature/fix). A pure doc-cleanup edit — rewording a skill body,
  splitting a paragraph — changed no behavior, so it does **not** warrant a changelog
  line; adding one would itself be changelog-leak. Only log when behavior changed.
- If you renamed/added a doc that another doc *enumerates*, update the index
  (`_Docs/00-GENEL-BAKIS.md`) so references don't drift.

## Report format

```
## <doc> — N/30   [SAGLAM | IYI | DUZELT]
| Criterion | Score | Finding |
| Commands | x/5 | … |
| … | | |

Findings:
- <issue> → <proposed fix> (diff)
```

## Pitfalls

- **Cutting a gotcha** to save a line. The non-obvious traps are why the doc exists.
- **Manufacturing edits** on a healthy doc to look busy — the skill says *minimal*.
- **Dropping detail instead of delegating it** — move history to `_Docs`, don't delete it.
- **Logging cosmetic edits in the changelog** — pollutes `05-ILERLEME.md`.
- **Auditing reference and action docs with the same brevity bar** — see "Calibrate".
