// Maps a shell program name (git, npm, docker, …) to a simple-icons brand glyph,
// so a Bash/PowerShell tool step (or a transform_data/run_code language) can show
// a small colored icon of the program it runs. Named imports keep the bundle to
// only the icons listed here (tree-shaken).
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
  siPodman,
  siKubernetes,
  siHelm,
  siDotnet,
  siRuby,
  siPhp,
  siComposer,
  siPostgresql,
  siMysql,
  siMongodb,
  siSqlite,
  siRedis,
  siTerraform,
  siPulumi,
  siGradle,
  siApachemaven,
  siApacheant,
  siOpenjdk,
  siGnubash,
  siCurl,
  siGooglecloud,
  siAnsible,
  siVagrant,
  siFlutter,
  siDart,
  siSwift,
  siLlvm,
  siCmake,
  siMake,
  siElixir,
  siScala,
  siKotlin,
  siJulia,
  siR,
  siPerl,
  siLua,
  siNeovim,
  siVim,
  siHomebrew,
  siArchlinux,
  siNginx,
  siPrisma,
  siVite,
  siWebpack,
  siEsbuild,
  siRollupdotjs,
  siTurborepo,
  siEslint,
  siPrettier,
  siBiome,
  siJest,
  siVitest,
  siCypress,
  siTypescript,
  siNextdotjs,
  siAstro,
  siPytest,
  siAnaconda,
  siVercel,
  siNetlify,
  siSupabase,
  siFirebase,
  siCloudflare,
  siWasmer,
} from 'simple-icons'

interface ProgramIcon {
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
  // Version control / hosting
  git: ico(siGit),
  gh: ico(siGithub),
  // JS/TS runtimes + package managers
  npm: ico(siNpm),
  npx: ico(siNpm),
  pnpm: ico(siPnpm),
  pnpx: ico(siPnpm),
  yarn: ico(siYarn),
  bun: ico(siBun),
  bunx: ico(siBun),
  node: ico(siNodedotjs),
  deno: ico(siDeno),
  // JS/TS build + quality + test
  tsc: ico(siTypescript),
  tsx: ico(siTypescript),
  'ts-node': ico(siTypescript),
  vite: ico(siVite),
  webpack: ico(siWebpack),
  esbuild: ico(siEsbuild),
  rollup: ico(siRollupdotjs),
  turbo: ico(siTurborepo),
  next: ico(siNextdotjs),
  astro: ico(siAstro),
  prisma: ico(siPrisma),
  eslint: ico(siEslint),
  prettier: ico(siPrettier),
  biome: ico(siBiome),
  jest: ico(siJest),
  vitest: ico(siVitest),
  cypress: ico(siCypress),
  // Python
  python: ico(siPython),
  python3: ico(siPython),
  pip: ico(siPython),
  pip3: ico(siPython),
  poetry: ico(siPython),
  pdm: ico(siPython),
  uv: ico(siPython),
  pytest: ico(siPytest),
  conda: ico(siAnaconda),
  // Go / Rust
  go: ico(siGo),
  gofmt: ico(siGo),
  cargo: ico(siRust),
  rustc: ico(siRust),
  rustup: ico(siRust),
  // Containers / orchestration / IaC
  docker: ico(siDocker),
  'docker-compose': ico(siDocker),
  podman: ico(siPodman),
  kubectl: ico(siKubernetes),
  k9s: ico(siKubernetes),
  helm: ico(siHelm),
  terraform: ico(siTerraform),
  tofu: ico(siTerraform),
  pulumi: ico(siPulumi),
  ansible: ico(siAnsible),
  'ansible-playbook': ico(siAnsible),
  vagrant: ico(siVagrant),
  // JVM
  dotnet: ico(siDotnet),
  java: ico(siOpenjdk),
  javac: ico(siOpenjdk),
  jar: ico(siOpenjdk),
  gradle: ico(siGradle),
  gradlew: ico(siGradle),
  './gradlew': ico(siGradle),
  mvn: ico(siApachemaven),
  maven: ico(siApachemaven),
  ant: ico(siApacheant),
  scala: ico(siScala),
  sbt: ico(siScala),
  kotlin: ico(siKotlin),
  kotlinc: ico(siKotlin),
  // Ruby / PHP
  ruby: ico(siRuby),
  gem: ico(siRuby),
  bundle: ico(siRuby),
  rake: ico(siRuby),
  php: ico(siPhp),
  composer: ico(siComposer),
  // Other languages
  dart: ico(siDart),
  flutter: ico(siFlutter),
  swift: ico(siSwift),
  elixir: ico(siElixir),
  mix: ico(siElixir),
  iex: ico(siElixir),
  julia: ico(siJulia),
  r: ico(siR),
  rscript: ico(siR),
  perl: ico(siPerl),
  lua: ico(siLua),
  // Native build
  clang: ico(siLlvm),
  'clang++': ico(siLlvm),
  cmake: ico(siCmake),
  make: ico(siMake),
  gmake: ico(siMake),
  // Databases
  psql: ico(siPostgresql),
  mysql: ico(siMysql),
  mongo: ico(siMongodb),
  mongosh: ico(siMongodb),
  'redis-cli': ico(siRedis),
  sqlite3: ico(siSqlite),
  // Servers / infra
  nginx: ico(siNginx),
  // Cloud / deploy CLIs
  gcloud: ico(siGooglecloud),
  gsutil: ico(siGooglecloud),
  vercel: ico(siVercel),
  netlify: ico(siNetlify),
  supabase: ico(siSupabase),
  firebase: ico(siFirebase),
  wrangler: ico(siCloudflare),
  // Editors / package tooling
  nvim: ico(siNeovim),
  vim: ico(siVim),
  brew: ico(siHomebrew),
  pacman: ico(siArchlinux),
  wasmer: ico(siWasmer),
  // Shell / net
  bash: ico(siGnubash),
  curl: ico(siCurl),
}

// programIconFor returns the brand icon for a single program name, or null.
function programIconFor(name: string): ProgramIcon | null {
  return PROGRAM_ICONS[name.toLowerCase()] ?? null
}

/** A program recognized inside a command string: its bare name plus its brand icon. */
export interface Program {
  /** Bare program name as written in the command, lower-cased (e.g. "curl", "npm"). */
  name: string
  icon: ProgramIcon
}

// resolveProgram parses a command string and returns the FIRST recognized program
// in it — name and icon together — or null when none maps. Also accepts a bare
// program name (e.g. transform_data's "python3"), which parses to itself.
import { extractCommandNames } from './commandProgram'

export function resolveProgram(command: string): Program | null {
  for (const name of extractCommandNames(command)) {
    const icon = programIconFor(name)
    if (icon) return { name, icon }
  }
  return null
}
