// Parses a shell command string (Bash or PowerShell) to detect the program(s)
// invoked — git, npm, docker, python, … — so the chat can show a small brand icon
// next to the tool step. Pure string logic (no fs), adapted from external-agent-oss'
// cli-icon-resolver. Handles: env prefixes (FOO=bar cmd), transparent prefixes
// (sudo/time/env/timeout), chains (&&, ||, ;), pipes (|), path prefixes
// (/usr/bin/node → node, C:\tools\git.exe → git), and shell wrappers
// (bash -lc '…', pwsh -Command "…") via recursion.

// Transparent prefix commands: they run another command, so skip to the next token.
const PREFIX_COMMANDS = new Set([
  'sudo',
  'time',
  'nice',
  'nohup',
  'env',
  'timeout',
  'strace',
  'ltrace',
  'ionice',
  'taskset',
  'watch',
  'caffeinate',
  'doas',
  'stdbuf',
  'xargs',
])

// Shell hosts whose `-c`/`-Command` argument carries the REAL command to inspect.
const SHELL_HOSTS = new Set(['bash', 'zsh', 'sh', 'dash', 'ksh', 'pwsh', 'powershell'])

// isEnvAssignment reports whether a token is a leading `NAME=value` env prefix.
function isEnvAssignment(token: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*=/.test(token)
}

// baseName strips a path prefix (POSIX or Windows) and a trailing .exe/.cmd/.bat/.ps1.
function baseName(p: string): string {
  const parts = p.split(/[\\/]/)
  const last = parts[parts.length - 1] || p
  return last.replace(/\.(exe|cmd|bat|ps1|com)$/i, '')
}

// tokenize splits a single sub-command into whitespace-separated tokens, stripping
// (but respecting) single/double quotes so a quoted inner command stays one token.
function tokenize(s: string): string[] {
  const tokens: string[] = []
  let cur = ''
  let quote: '"' | "'" | null = null
  let had = false
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quote) {
      if (c === quote) quote = null
      else cur += c
      continue
    }
    if (c === '"' || c === "'") {
      quote = c
      had = true
      continue
    }
    if (c === ' ' || c === '\t' || c === '\n' || c === '\r') {
      if (cur || had) tokens.push(cur)
      cur = ''
      had = false
      continue
    }
    cur += c
    had = true
  }
  if (cur || had) tokens.push(cur)
  return tokens
}

// splitCommands splits a command string on &&, ||, ;, | (and PowerShell's ;),
// respecting quotes, into individual sub-commands.
export function splitCommands(commandStr: string): string[] {
  const commands: string[] = []
  let current = ''
  let single = false
  let dbl = false
  let i = 0
  while (i < commandStr.length) {
    const char = commandStr[i]
    const next = commandStr[i + 1]
    if (char === "'" && !dbl) {
      single = !single
      current += char
      i++
      continue
    }
    if (char === '"' && !single) {
      if (i > 0 && commandStr[i - 1] === '\\') {
        current += char
        i++
        continue
      }
      dbl = !dbl
      current += char
      i++
      continue
    }
    if (!single && !dbl) {
      if ((char === '&' && next === '&') || (char === '|' && next === '|')) {
        if (current.trim()) commands.push(current.trim())
        current = ''
        i += 2
        continue
      }
      if (char === '|' || char === ';') {
        if (current.trim()) commands.push(current.trim())
        current = ''
        i++
        continue
      }
    }
    current += char
    i++
  }
  if (current.trim()) commands.push(current.trim())
  return commands
}

// extractCommandName returns the bare program name of one sub-command, resolving
// env/prefix skips, path prefixes and shell `-c` wrappers (recursively).
export function extractCommandName(subCommand: string): string | undefined {
  const tokens = tokenize(subCommand)
  let idx = 0
  while (idx < tokens.length && isEnvAssignment(tokens[idx]!)) idx++

  while (idx < tokens.length) {
    const cmdName = baseName(tokens[idx]!).toLowerCase()
    if (PREFIX_COMMANDS.has(cmdName)) {
      idx++
      while (idx < tokens.length && tokens[idx]!.startsWith('-')) idx++
      if (cmdName === 'timeout' && idx < tokens.length && /^\d/.test(tokens[idx]!)) idx++
      continue
    }
    if (SHELL_HOSTS.has(cmdName)) {
      const rest = tokens.slice(idx + 1)
      const cIdx = rest.findIndex(
        (t) => /^-(-?c(ommand)?|lc|l?c)$/i.test(t) || (t.startsWith('-') && /c/i.test(t)),
      )
      if (cIdx !== -1 && cIdx + 1 < rest.length) {
        const inner = rest[cIdx + 1]
        if (inner) {
          const names = extractCommandNames(inner)
          return names[0]
        }
      }
    }
    break
  }
  if (idx >= tokens.length) return undefined
  const name = baseName(tokens[idx]!).toLowerCase()
  return name || undefined
}

// extractCommandNames returns every program name in a command string, in order —
// e.g. "git add . && npm publish" → ["git", "npm"].
export function extractCommandNames(commandStr: string): string[] {
  if (!commandStr || !commandStr.trim()) return []
  const names: string[] = []
  for (const sub of splitCommands(commandStr)) {
    const name = extractCommandName(sub)
    if (name) names.push(name)
  }
  return names
}
