import type { Agent } from '../../types'
import { avatarGlyph, normalizeAvatar, resolveColor } from '../../lib/avatar'

interface Props {
  agent: Pick<Agent, 'id' | 'name' | 'avatar' | 'color'>
  size?: number
  active?: boolean
}

// AgentAvatar renders the agent's circular identity: a colored disc holding
// either a custom emoji or the name initials. A custom glyph is drawn larger;
// initials are scaled down and bold.
export function AgentAvatar({ agent, size = 32, active = false }: Props) {
  const color = resolveColor(agent)
  const glyph = avatarGlyph(agent)
  // Only size up as an emoji when the custom avatar is actually a usable glyph;
  // a corrupted/empty avatar falls back to initials, which render smaller + bold.
  const isEmoji = !!normalizeAvatar(agent.avatar)

  return (
    <span
      className="inline-flex shrink-0 select-none items-center justify-center rounded-full font-semibold text-white"
      style={{
        width: size,
        height: size,
        background: `linear-gradient(135deg, ${color}, ${color}cc)`,
        fontSize: isEmoji ? size * 0.52 : size * 0.4,
        boxShadow: active ? `0 0 0 2px var(--color-bg), 0 0 0 4px ${color}` : 'none',
        lineHeight: 1,
      }}
      title={agent.name}
      aria-hidden
    >
      {glyph}
    </span>
  )
}
