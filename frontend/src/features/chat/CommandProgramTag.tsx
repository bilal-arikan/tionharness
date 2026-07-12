import { resolveProgram } from '@/shared/lib/programIcons'

interface Props {
  /** The shell command string (Bash/PowerShell tool input), or a bare language name. */
  command: string
  size?: number
}

// CommandProgramTag renders the program a shell command runs — its brand glyph
// plus its bare name in parentheses, e.g. "🐙 (curl)" ahead of the "Bash" label.
// Renders nothing when the command maps to no known program (e.g. a PowerShell
// cmdlet or plain `ls`), so unrecognized commands stay as bare tool labels.
export function CommandProgramTag({ command, size = 13 }: Props) {
  const program = resolveProgram(command)
  if (!program) return null
  const { name, icon } = program
  return (
    <span className="flex shrink-0 items-center gap-1">
      <svg role="img" aria-label={icon.title} width={size} height={size} viewBox="0 0 24 24">
        <title>{icon.title}</title>
        <path d={icon.path} fill={`#${icon.hex}`} />
      </svg>
      <span className="font-mono text-[var(--color-text-dim)]">({name})</span>
    </span>
  )
}
