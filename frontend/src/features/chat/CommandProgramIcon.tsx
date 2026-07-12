import { resolveProgramIcon } from '@/shared/lib/programIcons'

interface Props {
  /** The shell command string (Bash/PowerShell tool input). */
  command: string
  size?: number
}

// CommandProgramIcon renders a small brand glyph of the program a shell command
// runs (git, npm, docker, …) next to the tool icon. Renders nothing when the
// command maps to no known program (e.g. a PowerShell cmdlet or plain `ls`).
export function CommandProgramIcon({ command, size = 13 }: Props) {
  const icon = resolveProgramIcon(command)
  if (!icon) return null
  return (
    <svg
      role="img"
      aria-label={icon.title}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      className="shrink-0"
    >
      <title>{icon.title}</title>
      <path d={icon.path} fill={`#${icon.hex}`} />
    </svg>
  )
}
