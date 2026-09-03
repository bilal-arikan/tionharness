---
name: docs-update
description: >
  How to record a change in this repo's Turkish documentation: append a dated entry to
  `_Docs/05-ILERLEME.md`, refresh the affected doc's top "Özet" block, keep the index in
  `_Docs/00-GENEL-BAKIS.md` complete. Use after any feature/fix, for "/docs-update",
  "dokümanı güncelle", or before a commit that changed behaviour.
---

# Documentation update pattern

Documents are Turkish; code and comments English. Every `_Docs/*.md` starts with an H1
and, right below it, a blockquote `> **Özet (YYYY-MM-DD):** ...` (3–5 sentences: topic,
status, key decisions, owning packages). Agents read the Özet first, so keep it true.

Steps for a change:

1. **Progress log.** Prepend a section to `_Docs/05-ILERLEME.md` right after the H1 and
   its Özet block:

   ```markdown
   ## <Kısa başlık> (YYYY-MM-DD) ✅

   <Ne değişti, neden, hangi dosyalar, hangi testler; ölçüm varsa rakamla.>
   ```

2. **Topic doc.** Update the doc that owns the topic (find it via the index in
   `_Docs/00-GENEL-BAKIS.md`). If the change alters its status or a key decision, edit
   its Özet block too; do not leave a stale "plan" status on implemented work.
3. **New doc?** Number it after the highest existing one (`NN-KONU.md`), add the Özet
   block, and add a row to the index table in `00-GENEL-BAKIS.md`.
4. **Rules that agents need every turn** go to `CLAUDE.md` (keep it small, ~5 KB);
   reference material goes to `_Docs/80-AJAN-REFERANSI.md`.
5. **Whitespace gate.** No trailing whitespace in `.md`/`.yml`; run `git diff --check`.

Dates are absolute (`2026-09-03`), never "bugün"/"dün".
