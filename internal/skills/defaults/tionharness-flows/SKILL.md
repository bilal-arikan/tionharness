---
name: "TionHarness Flows"
description: "How an agent's evolving main flow works and how to read, edit, revert and test it with get_flow / edit_flow / revert_flow / list_flow_runs / update_my_prompt."
when_to_use: "When you want to change how your own turns are processed (add a critic loop, a planning stage, a classifier), when the user asks you to improve your flow or prompts, or when you review your recent runs."
icon: "🧬"
color: "#5B8DEF"
access: shared
auto_summary: false
---
# TionHarness Flows — evolving per-agent flows

Every agent has ONE main flow. Each of your turns runs through it: the user's
input enters at the `input` node, every `llm` node is a model call made with
your own request (history, tools, permissions), a `route` node picks an
outgoing edge by matching the last output, a `transform` node renders a
template, and the `output` node's text is the reply the user sees. The default
flow is `input → respond → output`, which is exactly a plain turn.

Flows are versioned. Every change — by the user on the canvas, by you with
`edit_flow`, or by the Flow Observer system agent after a run — is an
immutable version with an author and a reason; anything can be reverted.

## Node types and fields

| Type | Fields | Notes |
|------|--------|-------|
| `input` | — | exactly one; `{{input}}` |
| `llm` | `prompt` (template), `context` (`thread` = session history + prompt, `fresh` = only the system prompt + prompt), `tools` (`inherit` / `none`), optional `outputSchema`, `model`, `agentId` | one model call |
| `route` | `mode` (`contains` / `equals` / `regex` / `json` / `judge` / `criteria`), `jsonField` (json mode), `question` (judge mode), `criteria` (criteria mode: yes/no statements), `maxVisits` (loop cap, default 3) | picks an outgoing edge by its `when` label; the unlabelled edge is the default arm. `judge`: the decision model (JEV) picks the arm whose label fits the output. `criteria`: the decision model checks every statement in one call — all true → the arm labelled `pass`, otherwise the arm labelled `fail` (keep a default arm as the fallback) |
| `transform` | `template` | no model call |
| `trigger` | `automationId`, `template` (payload, default `{{last}}`) | fires that automation (any kind) with the rendered payload as `{{result}}`; the flow's last output passes through untouched, a declined fire is noted on the step |
| `output` | `template` (default `{{last}}`) | exactly one |

Templates: `{{input}}`, `{{last}}` (previous node's output), `{{node.<id>}}`,
`{{visit}}` (how many times this node ran this turn), `{{step}}`.

Invariants (checked in code, an invalid edit changes nothing): one input, one
output, every node reachable from the input and able to reach the output,
linear nodes have exactly one outgoing edge, every loop passes through a
route node that has a default arm, the graph stays under the policy's node cap.

Decision-model mechanisms around flows (Settings → Decision authorities): `flow-judge`
and `flow-criteria` drive the two judged route modes; `flow-grade` grades every
successful run's reply 1..5 (visible in `list_flow_runs` as `grade`, in the observer's
evidence and usable by flow-kind automations: "when graded ≤ 2, spawn the reviewer");
`flow-proposal-gate` gives the observer's auto-applied proposals a second look.
Automations of kind `flow` fire when a flow run finishes (filters: agent, outcome,
max grade) — the event-side complement of the `trigger` node.

## Ops for `edit_flow`

```json
{"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","title":"Eleştiri","context":"fresh","tools":"none","prompt":"..."}}
{"op":"add_node","node":{...}}
{"op":"remove_node","id":"critic"}          // a linear node is bridged (in → out)
{"op":"update_node","id":"respond","fields":{"prompt":"...","title":"..."}}   // id/type are immutable
{"op":"add_edge","edge":{"from":"check","to":"respond","when":"REVISE"}}
{"op":"update_edge","id":"e_check_output","edge":{"when":"APPROVE"}}
{"op":"remove_edge","id":"e_..."}
{"op":"set_max_steps","maxSteps":30}
```

Edge ids default to `e_<from>_<to>`. Call `get_flow` first so your ops name
real ids.

## A feedback loop in five ops

```json
[
 {"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","title":"Eleştiri","context":"fresh","tools":"none","prompt":"Evaluate the answer against the request. Reply 'APPROVE' or 'REVISE: <what to fix>'.\n\nRequest: {{input}}\n\nAnswer: {{node.respond}}"}},
 {"op":"insert_between","from":"critic","to":"output","node":{"id":"check","type":"route","title":"Onay?","mode":"contains","maxVisits":2}},
 {"op":"update_edge","id":"e_check_output","edge":{"when":"APPROVE"}},
 {"op":"add_edge","edge":{"from":"check","to":"respond"}},
 {"op":"update_node","id":"respond","fields":{"prompt":"{{input}}\n\n{{node.critic}}"}},
 {"op":"update_node","id":"output","fields":{"template":"{{node.respond}}"}}
]
```

`check` loops back to `respond` at most twice (`maxVisits`), then takes its
default arm; the second draft sees the critic's note through `{{node.critic}}`.
The last op matters: the output node renders `{{last}}` by default, which after
the route would be the critic's verdict — point it at the answer instead.

## Working rules

- Read `list_flow_runs` (with `steps:true` when needed) BEFORE changing the
  flow; cite what the runs showed in `reason`.
- Prefer the smallest change. A stage that never changes the outcome costs
  tokens and time; remove it.
- `update_my_prompt` rewrites your soul/identity in full and keeps the old text
  as a prompt version. Use it only for lasting lessons about your role.
- `revert_flow` restores an earlier version as a new head version; nothing in
  the history is lost.
- The Flow Observer (policy `propose` / `auto`) files proposals after every N
  runs; the user applies or rejects them on the Flows screen. In `auto` mode a
  confident proposal is applied by itself.
