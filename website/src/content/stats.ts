import { site } from '../site.config'

export interface Stat {
  value: string
  label: string
  note: string
}

export const stats: Stat[] = [
  {
    value: `${site.binarySizeMb} MB`,
    label: 'single binary',
    note: 'Web UI embedded with go:embed. Nothing else to install.',
  },
  {
    value: '0',
    label: 'databases',
    note: 'Entities are JSON, sessions are JSONL. Back up by copying a folder.',
  },
  {
    value: '0',
    label: 'API keys required',
    note: 'Runs through your local claude-cli / codex-cli login.',
  },
  {
    value: '7',
    label: 'provider kinds',
    note: 'claude-cli, codex-cli, anthropic, minimax, deepseek, openrouter, zai.',
  },
]
