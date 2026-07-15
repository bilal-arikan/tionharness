---
name: "Adversarial Verification"
kind: coordinator-workflow
pattern: adversarial
description: "For each claim or change a worker produces, spawn a SEPARATE worker whose only job is to refute it. Keep only what survives."
worker_targets: [explore, reviewer]
icon: "⚖️"
color: "#ef4444"
access: shared
auto_summary: false
---
# Adversarial Verification

Use this when correctness matters more than speed: fact-checking claims, validating a plan, or reviewing code another worker wrote.

## Steps
1. **Produce.** A worker (or you) generates the claims / changes to be checked. List them explicitly.
2. **Refute, don't rubber-stamp.** For each item, spawn a FRESH `spawn_worker` verifier with a skeptical brief: "Try to REFUTE this. Reproduce it, run the test, find the counter-example. Default to 'not verified' when uncertain." Fresh context is the point — never reuse the author worker to check its own work.
3. **Collect verdicts.** Verifier notifications arrive and auto-start your turn. For high-stakes items, spawn 2–3 verifiers with DIFFERENT lenses (correctness, security, does-it-reproduce) and require a majority.
4. **Keep survivors.** Drop anything the verifiers refute; report only what held up, with the evidence.

## Notes
- Real verification means proving it works (run it), not confirming it exists.
- A verifier that agrees too easily is a failure — prompt for genuine skepticism.
