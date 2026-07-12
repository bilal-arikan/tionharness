// Maps a shell program name (git, npm, docker, …) to a simple-icons brand glyph,
// so a Bash/PowerShell tool step can show a small colored icon of the program it
// runs. Named imports keep the bundle to only the icons listed here.
import {
  siGit,
  siGithub,
  siNpm,
  siPnpm,
  siYarn,
  siBun,
  siNodedotjs,
  siDeno,
  siPython,
  siGo,
  siRust,
  siDocker,
  siKubernetes,
  siHelm,
  siDotnet,
  siRuby,
  siPhp,
  siComposer,
  siPostgresql,
  siMysql,
  siRedis,
  siTerraform,
  siGradle,
  siApachemaven,
  siGnubash,
  siCurl,
} from 'simple-icons'

export interface ProgramIcon {
  title: string
  /** Brand hex WITHOUT the leading '#'. */
  hex: string
  /** SVG path `d` for a 24×24 viewBox. */
  path: string
}

type SI = { title: string; hex: string; path: string }
const ico = (s: SI): ProgramIcon => ({ title: s.title, hex: s.hex, path: s.path })

// Command name (lower-case, no path/extension) → brand icon. Aliases share an entry.
const PROGRAM_ICONS: Record<string, ProgramIcon> = {
  git: ico(siGit),
  gh: ico(siGithub),
  npm: ico(siNpm),
  npx: ico(siNpm),
  pnpm: ico(siPnpm),
  pnpx: ico(siPnpm),
  yarn: ico(siYarn),
  bun: ico(siBun),
  bunx: ico(siBun),
  node: ico(siNodedotjs),
  deno: ico(siDeno),
  python: ico(siPython),
  python3: ico(siPython),
  pip: ico(siPython),
  pip3: ico(siPython),
  poetry: ico(siPython),
  uv: ico(siPython),
  go: ico(siGo),
  gofmt: ico(siGo),
  cargo: ico(siRust),
  rustc: ico(siRust),
  rustup: ico(siRust),
  docker: ico(siDocker),
  'docker-compose': ico(siDocker),
  podman: ico(siDocker),
  kubectl: ico(siKubernetes),
  k9s: ico(siKubernetes),
  helm: ico(siHelm),
  dotnet: ico(siDotnet),
  ruby: ico(siRuby),
  gem: ico(siRuby),
  bundle: ico(siRuby),
  rake: ico(siRuby),
  php: ico(siPhp),
  composer: ico(siComposer),
  psql: ico(siPostgresql),
  mysql: ico(siMysql),
  'redis-cli': ico(siRedis),
  terraform: ico(siTerraform),
  tofu: ico(siTerraform),
  gradle: ico(siGradle),
  './gradlew': ico(siGradle),
  gradlew: ico(siGradle),
  mvn: ico(siApachemaven),
  maven: ico(siApachemaven),
  bash: ico(siGnubash),
  curl: ico(siCurl),
}

// programIconFor returns the brand icon for a single program name, or null.
export function programIconFor(name: string): ProgramIcon | null {
  return PROGRAM_ICONS[name.toLowerCase()] ?? null
}

// resolveProgramIcon parses a command string and returns the icon of the FIRST
// recognized program in it, or null when none maps.
import { extractCommandNames } from './commandProgram'

export function resolveProgramIcon(command: string): ProgramIcon | null {
  for (const name of extractCommandNames(command)) {
    const icon = programIconFor(name)
    if (icon) return icon
  }
  return null
}
