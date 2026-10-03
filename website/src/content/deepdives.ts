export interface DeepDive {
  eyebrow: string
  title: string
  body: string
  bullets: string[]
  /** Path under public/. Rendered as a placeholder frame until the file exists. */
  shot: string
  shotAlt: string
}

export const deepDives: DeepDive[] = [
  {
    eyebrow: 'Multi-agent',
    title: 'One agent, a whole team behind it',
    body: 'Turn on coordinator mode and an agent stops doing the work itself. It spawns workers, hands each a scoped profile, and stitches the results together.',
    bullets: [
      'Profiles: explore, edit, validator and a read-only planner',
      'Unbounded depth, with guards at 5 levels and 64 subtree members',
      'Oversized worker output is written as an artifact and passed by handle',
    ],
    shot: '/shots/coordinator.png',
    shotAlt: 'Coordinator session with several workers running in parallel',
  },
  {
    eyebrow: 'Orchestration',
    title: 'Flows that evolve with every turn',
    body: 'Each agent owns one versioned flow: input, a few model / route / transform stages, output. A route can loop back for a second draft, every loop is bounded in code, and every change is a version you can revert.',
    bullets: [
      'Edit on the canvas, or let the agent edit itself with edit_flow and update_my_prompt',
      'A Flow Observer reads the last runs and proposes one measured change (propose or auto policy)',
      'One run per turn with the per-stage trace, live in the chat and on the canvas',
    ],
    shot: '/shots/flows.png',
    shotAlt: 'Flow builder canvas with a branching multi-agent graph',
  },
  {
    eyebrow: 'Automation',
    title: 'The board is the source of execution',
    body: 'Tags, board moves, cumulative token spend and message counters are all triggers. Each one spawns a session whose agent runs its own evolving flow.',
    bullets: [
      'Four trigger kinds: label, board, token threshold, activity counter',
      'Token triggers reuse one persistent maintenance session instead of spawning forever',
      'Every workspace ships two default rules, seeded disabled',
    ],
    shot: '/shots/board.png',
    shotAlt: 'Kanban board with automation rules',
  },
]
