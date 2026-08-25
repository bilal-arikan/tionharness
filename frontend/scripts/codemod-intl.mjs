// One-shot codemod: route every locale-sensitive formatting call through the
// shared Intl layer (src/shared/lib/intl.ts, src/shared/lib/format.ts) instead of
// a hardcoded 'tr-TR' / 'tr' tag.
//
// Kept in the repo rather than run-and-deleted so the next locale migration has a
// worked example, and so the SKIP list below documents which call sites are
// deliberately NOT locale-aware.
//
// Usage:  node scripts/codemod-intl.mjs [--dry]
//
// It rewrites three constructs, resolving the receiver with a backwards balanced
// scan so `new Date(a * 1000).toLocaleString('tr-TR')` is handled as one unit:
//   <date>.toLocaleDateString(...)  -> formatDate(<date>, ...)
//   <date>.toLocaleTimeString(...)  -> formatTime(<date>, ...)
//   <date>.toLocaleString(...)      -> formatDateTime(<date>, ...)   [Date receiver]
//   <number>.toLocaleString(...)    -> count(<number>)               [number receiver]
//   a.localeCompare(b[, 'tr'])      -> compareText(a, b)
// then adds the needed imports.

import { readFileSync, writeFileSync } from 'node:fs'
import { join, dirname, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const SRC = join(ROOT, 'src')
const DRY = process.argv.includes('--dry')

// Files whose comparisons/format calls are intentionally locale-INDEPENDENT.
// Sorting a URL query string or a BCP-47 voice code by the user's collation would
// make the output depend on the UI language, which is exactly wrong for machine
// -facing data.
const SKIP = new Set([
  'app/url.ts', // query-parameter keys: canonical order must not vary by locale
  'features/settings/TtsSettings.tsx', // sorts by BCP-47 lang code, not display text
  'shared/lib/intl.ts', // the layer itself
  'shared/lib/format.ts', // the layer itself
  'shared/lib/time.ts', // already migrated by hand
])

// Every file the grep found, minus SKIP. Listing them explicitly (rather than
// globbing) keeps the codemod auditable: a file not on this list was never touched.
const FILES = [
  'features/market/previewParts.tsx',
  'features/view/ViewPanel.tsx',
  'features/settings/WorkspacePanel.tsx',
  'features/settings/LessonsList.tsx',
  'features/settings/BackupPanel.tsx',
  'features/settings/AboutPanel.tsx',
  'shared/components/markdown/DiffView.tsx',
  'features/dashboard/DashboardPanel.tsx',
  'features/tools/useToolsPanelState.ts',
  'features/artifacts/ArtifactsPanel.tsx',
  'features/flows/RunView.tsx',
  'features/flows/FlowsPanel.tsx',
  'features/flows/FlowsListPane.tsx',
  'features/flows/FlowsHeader.tsx',
  'features/chat/ChangesModal.tsx',
  'features/chat/composer/toolAccessGroups.ts',
  'features/tasks/views/filterTasks.ts',
  'features/tasks/views/deriveColumns.ts',
  'features/tasks/views/BoardFilterBar.tsx',
  'features/agents/AgentContextModal.tsx',
  'features/sessions/SessionContextModal.tsx',
  'features/sessions/SessionDebugCard.tsx',
  'features/sessions/sessionDetailFormat.ts',
  'features/sessions/viz/ThinkingShareChart.tsx',
  'features/sessions/viz/SelfHealingEvents.tsx',
  'features/sessions/viz/PromptCacheEvents.tsx',
  'features/sessions/viz/ConcurrencyTimeline.tsx',
  'features/skills/SkillsPanel.tsx',
  'features/schedules/AutomationCard.tsx',
  'features/schedules/AutomationBoard.tsx',
  'features/schedules/timeUtils.ts',
  'features/network/NetworkFilters.tsx',
].filter((f) => !SKIP.has(f))

const OPEN = { ')': '(', ']': '[', '}': '{' }

// scanReceiverStart walks backwards from the '.' of a member expression and
// returns the index where the receiver expression begins, balancing brackets and
// skipping string literals so `new Date(x[0] + ')')` stays intact.
function scanReceiverStart(s, dotIdx) {
  let i = dotIdx - 1
  const stack = []
  while (i >= 0) {
    const c = s[i]
    if (stack.length === 0 && (c === '"' || c === "'" || c === '`')) {
      // A string literal receiver — walk to its opening quote.
      i--
      while (i >= 0 && s[i] !== c) i--
      i--
      continue
    }
    if (c === ')' || c === ']' || c === '}') {
      stack.push(OPEN[c])
      i--
      continue
    }
    if (c === '(' || c === '[' || c === '{') {
      if (stack.length === 0) return i + 1
      stack.pop()
      i--
      continue
    }
    if (stack.length > 0) {
      i--
      continue
    }
    // At depth 0, the receiver ends where an identifier/member chain stops.
    if (/[A-Za-z0-9_$.?]/.test(c)) {
      i--
      continue
    }
    break
  }
  let start = i + 1
  // Absorb a leading `new ` so `new Date(x)` is captured whole.
  const before = s.slice(0, start)
  const m = /(^|[^A-Za-z0-9_$])new\s+$/.exec(before)
  if (m) start = before.length - (m[0].length - m[1].length)
  return start
}

// splitArgs splits a call's argument text on top-level commas.
function splitArgs(text) {
  const out = []
  let depth = 0
  let cur = ''
  let quote = null
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quote) {
      cur += c
      if (c === quote && text[i - 1] !== '\\') quote = null
      continue
    }
    if (c === '"' || c === "'" || c === '`') {
      quote = c
      cur += c
      continue
    }
    if ('([{'.includes(c)) depth++
    if (')]}'.includes(c)) depth--
    if (c === ',' && depth === 0) {
      out.push(cur.trim())
      cur = ''
      continue
    }
    cur += c
  }
  if (cur.trim()) out.push(cur.trim())
  return out
}

// findCallEnd returns the index just past the ')' closing the call that opens at
// `openIdx`.
function findCallEnd(s, openIdx) {
  let depth = 0
  let quote = null
  for (let i = openIdx; i < s.length; i++) {
    const c = s[i]
    if (quote) {
      if (c === quote && s[i - 1] !== '\\') quote = null
      continue
    }
    if (c === '"' || c === "'" || c === '`') {
      quote = c
      continue
    }
    if (c === '(') depth++
    if (c === ')') {
      depth--
      if (depth === 0) return i + 1
    }
  }
  return -1
}

const DEFAULTS = {
  toLocaleDateString: "{ dateStyle: 'short' }",
  toLocaleTimeString: "{ timeStyle: 'medium' }",
  toLocaleString: "{ dateStyle: 'short', timeStyle: 'medium' }",
}
const FN = {
  toLocaleDateString: 'formatDate',
  toLocaleTimeString: 'formatTime',
  toLocaleString: 'formatDateTime',
}

// looksLikeDate reports whether a receiver expression is a Date rather than a
// number. `toLocaleString` is the only ambiguous method name of the three.
function looksLikeDate(receiver) {
  return /new\s+Date\b/.test(receiver) || /\bdate\b/i.test(receiver)
}

function rewriteToLocale(src) {
  const used = new Set()
  let out = src
  let guard = 0
  for (;;) {
    if (guard++ > 500) throw new Error('codemod did not converge')
    const m = /\.(toLocaleDateString|toLocaleTimeString|toLocaleString)\s*\(/.exec(out)
    if (!m) break
    const dotIdx = m.index
    const method = m[1]
    const openIdx = dotIdx + m[0].length - 1
    const endIdx = findCallEnd(out, openIdx)
    if (endIdx < 0) throw new Error('unbalanced call near: ' + out.slice(dotIdx, dotIdx + 60))
    const start = scanReceiverStart(out, dotIdx)
    const receiver = out.slice(start, dotIdx)
    const args = splitArgs(out.slice(openIdx + 1, endIdx - 1))
    // args[0] is the locale tag (dropped — the Intl layer supplies the active
    // locale); args[1], when present, is the options object we must preserve.
    const opts = args[1]
    let replacement
    if (method === 'toLocaleString' && !looksLikeDate(receiver)) {
      replacement = `count(${receiver})`
      used.add('count')
    } else {
      const fn = FN[method]
      const optText = opts ?? DEFAULTS[method]
      replacement = `${fn}(${receiver}, ${optText})`
      used.add(fn)
    }
    out = out.slice(0, start) + replacement + out.slice(endIdx)
  }
  return { out, used }
}

function rewriteLocaleCompare(src) {
  const used = new Set()
  let out = src
  let guard = 0
  for (;;) {
    if (guard++ > 500) throw new Error('codemod did not converge')
    const m = /\.localeCompare\s*\(/.exec(out)
    if (!m) break
    const dotIdx = m.index
    const openIdx = dotIdx + m[0].length - 1
    const endIdx = findCallEnd(out, openIdx)
    const start = scanReceiverStart(out, dotIdx)
    const receiver = out.slice(start, dotIdx)
    const args = splitArgs(out.slice(openIdx + 1, endIdx - 1))
    out = out.slice(0, start) + `compareText(${receiver}, ${args[0]})` + out.slice(endIdx)
    used.add('compareText')
  }
  return { out, used }
}

// addImport inserts (or extends) a named import from `from`, placed after the
// last existing import so the module's import block stays contiguous.
function addImport(src, names, from) {
  const wanted = [...names]
  const existing = new RegExp(
    `import\\s*\\{([^}]*)\\}\\s*from\\s*'${from.replace(/[/@.]/g, '\\$&')}'`,
  ).exec(src)
  if (existing) {
    const have = existing[1].split(',').map((s) => s.trim())
    const add = wanted.filter((n) => !have.includes(n))
    if (!add.length) return src
    const merged = [...have.filter(Boolean), ...add].join(', ')
    return src.replace(existing[0], `import { ${merged} } from '${from}'`)
  }
  // Find the line where the LAST import statement ends. A naive "last line
  // starting with `import`" lands inside a multi-line `import {\n a,\n b,\n}
  // from '…'` block and splices the new statement into the middle of it, so the
  // scan tracks whether it is still inside an unterminated import.
  const lines = src.split('\n')
  let last = -1
  let inImport = false
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (!inImport && /^import\s/.test(line)) inImport = true
    if (!inImport) continue
    // An import statement is complete once its `from '...'` (or a bare
    // side-effect `import '...'`) appears on the line.
    if (/\bfrom\s*['"]/.test(line) || /^import\s*['"]/.test(line)) {
      last = i
      inImport = false
    }
  }
  const stmt = `import { ${wanted.join(', ')} } from '${from}'`
  lines.splice(last + 1, 0, stmt)
  return lines.join('\n')
}

const INTL_FNS = new Set(['formatDate', 'formatTime', 'formatDateTime', 'compareText'])

let changed = 0
for (const rel of FILES) {
  const file = join(SRC, rel)
  const before = readFileSync(file, 'utf8')
  const a = rewriteToLocale(before)
  const b = rewriteLocaleCompare(a.out)
  let out = b.out
  const used = new Set([...a.used, ...b.used])
  if (out === before) {
    console.log(`  unchanged: ${rel}`)
    continue
  }
  const intl = [...used].filter((n) => INTL_FNS.has(n)).sort()
  const fmt = [...used].filter((n) => !INTL_FNS.has(n)).sort()
  if (intl.length) out = addImport(out, intl, '@/shared/lib/intl')
  if (fmt.length) out = addImport(out, fmt, '@/shared/lib/format')
  if (!DRY) writeFileSync(file, out)
  changed++
  console.log(`  rewrote:   ${relative(ROOT, file)}  [${[...used].sort().join(', ')}]`)
}
console.log(DRY ? `\n(dry run) ${changed} files would change` : `\n${changed} files rewritten`)
