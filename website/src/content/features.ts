export interface Feature {
  icon: string
  title: string
  body: string
}

/** Nine cards, one line each. Anything that needs a paragraph belongs in a deep dive. */
export const features: Feature[] = [
  {
    icon: 'agent',
    title: 'Agents with an identity',
    body: 'Each agent carries its own soul prompt, provider instance, model and thinking level -- from off all the way to max.',
  },
  {
    icon: 'tree',
    title: 'Coordinator and workers',
    body: 'An agent can spawn a tree of workers, stream their results back as notifications, and synthesise the answer itself.',
  },
  {
    icon: 'flow',
    title: 'Evolving flows',
    body: 'Every agent has one main flow its turns run through. Insert a critic loop or a planning stage on the canvas, let the agent edit it with tools, or let the observer propose changes from real runs.',
  },
  {
    icon: 'board',
    title: 'Board-driven execution',
    body: 'A kanban card moving into a column is a trigger: rules spawn agents or archive cards without an LLM in the loop.',
  },
  {
    icon: 'workspace',
    title: 'Physical workspace isolation',
    body: 'Every workspace gets its own store, runtime, scheduler and theme. Nothing leaks across the boundary.',
  },
  {
    icon: 'tool',
    title: 'Tools and MCP',
    body: 'Built-in file and shell tools plus external MCP servers over stdio or streamable HTTP, with lazy loading so the catalogue stays cheap.',
  },
  {
    icon: 'shield',
    title: 'Permission layer',
    body: 'Tools are risk-classified and gated by auto, ask or read-only modes, with argument patterns like Bash(git *) and a full audit trail.',
  },
  {
    icon: 'gear',
    title: 'Self-management',
    body: 'Agents create their own schedules, automations, skills and secrets and evolve their own flow and prompts through tools -- the app is its own API surface.',
  },
  {
    icon: 'search',
    title: 'Retrospective insight',
    body: 'Editable lenses scan past sessions for wasted spend and broken habits, then file findings as app fixes or workspace tweaks.',
  },
]
