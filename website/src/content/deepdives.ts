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
    title: 'Flows you can draw and then trust',
    body: 'The graph engine checkpoints after every node, so a flow survives a restart, an await-input suspension, or a loop that runs for a week.',
    bullets: [
      'Twelve node kinds, including parallel, join, subflow and spawn',
      'Live run viewer over SSE with a run lineage tree',
      'Durable await-input: suspend, resume days later, keep the context',
    ],
    shot: '/shots/flows.png',
    shotAlt: 'Flow builder canvas with a branching multi-agent graph',
  },
  {
    eyebrow: 'Automation',
    title: 'The board is the source of execution',
    body: 'Tags, board moves, cumulative token spend and message counters are all triggers. Each one can spawn a session or fire a flow.',
    bullets: [
      'Four trigger kinds: label, board, token threshold, activity counter',
      'Token triggers reuse one persistent maintenance session instead of spawning forever',
      'Every workspace ships two default rules, seeded disabled',
    ],
    shot: '/shots/board.png',
    shotAlt: 'Kanban board with automation rules',
  },
]
