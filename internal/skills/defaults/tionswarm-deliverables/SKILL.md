---
name: "TionSwarm Deliverables"
description: "How to surface produced output — files, documents, datasets, reports, diagrams, images, video — so the user actually sees it (artifacts vs. inline chat media)."
when_to_use: "When a task asks you to produce a file/document/dataset/report/code, or to show a diagram, image, video, or gallery to the user."
icon: "📦"
color: "#0ea5e9"
access: shared
---
# Deliverables → Artifacts (and what renders inline)

The static system prompt carries only a two-line summary of this. This skill is
the full rule set: load it whenever you are about to deliver substantial output
or show media to the user.

## 1. Substantial output → write it as an artifact

When asked to produce a **file / document / dataset / report / code**, register it
as an artifact by calling `create_artifact` (the artifacts API does the same). Do
NOT deliver substantial output only as inline chat text — the user never gets a
real, openable deliverable in the Artifacts screen.

> **There is no automatic capture.** Writing a file never creates an artifact by
> itself: artifacts exist only when you create them **deliberately** with
> `create_artifact`. Ordinary edits to project source files therefore stay out of
> the Artifacts screen.

For a **binary file already on disk** (e.g. a screenshot, a generated PDF), call
`create_artifact` with `kind=image|file` and `sourcePath` set to the path on
disk. Never base64-embed raw bytes into `content` — it blows the token budget
and is rejected for large payloads.

To revise an existing artifact, prefer `update_artifact` by its `id` over
creating a duplicate (the session's existing artifacts are surfaced to you each
turn for exactly this reason).

## 2. Content meant to be SEEN in the chat → put it INLINE in your reply

Some output is meant to be looked at *in the conversation*. That renders inline
in chat, so put it **directly in your reply** — NOT only as a `create_artifact`,
which would hide it in a separate Artifacts tab where the user has to go dig for
it.

- **Diagrams:** a ```` ```mermaid ```` fenced block renders as a real diagram;
  a ```` ```diff ```` block renders as a unified diff view.
- **A single image OR video:** standard markdown `![alt](path-or-URL)` renders
  inline. Local file paths are served automatically. An image is click-to-zoom;
  a video file (`.mp4` / `.webm` / …) becomes an inline player.
- **Several media at once:** put each on its **own line** as `![alt](path)` —
  consecutive image/video lines are auto-grouped into one thumbnail gallery.
  Alternatively write an explicit ```` ```gallery ```` block containing either:
  - JSON: `{"images":[{"src":"path-or-URL","alt":"..."}, ...]}`, or
  - a newline-separated list of paths.

  The gallery opens a zoom/pan lightbox with prev/next; videos play inside it.

## 3. Both at once is fine

Saving the same thing as an artifact **in addition** to showing it inline is
fine. But remember: the inline block/image is what the user actually sees in
chat — the artifact is the openable, persistent copy. When in doubt, show it
inline AND capture it as an artifact.

## Quick decision guide

| You produced…                                   | Do this                                                        |
|-------------------------------------------------|----------------------------------------------------------------|
| A document / dataset / report / code file       | `write_file` / `Write`, or `create_artifact`                   |
| A binary file already on disk (image, PDF)      | `create_artifact` with `kind` + `sourcePath` (never base64)    |
| A diagram                                        | inline ```` ```mermaid ```` (or ```` ```diff ````) in reply    |
| One image or video to show                       | inline `![alt](path-or-URL)` in reply                          |
| Several images/videos to show                    | one `![alt](path)` per line, or a ```` ```gallery ```` block   |
| A revision of an existing artifact               | `update_artifact` by its `id` (don't duplicate)                |
