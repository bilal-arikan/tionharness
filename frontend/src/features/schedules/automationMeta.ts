import type {
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  FlowRuleStatus,
  TokenScope,
  TrajEndStatus,
  TrajEvent,
} from '@/types'
import { i18next } from '@/i18n'

const tr = (key: string) => i18next.t(key, { ns: 'schedules' })

// Rota (F2) trigger options.
export const TRAJ_EVENTS: { value: TrajEvent; label: string }[] = [
  {
    value: 'exit',
    get label() {
      return tr('meta.trajectoryEvents.exit')
    },
  },
  {
    value: 'enter',
    get label() {
      return tr('meta.trajectoryEvents.enter')
    },
  },
]

export const TRAJ_END_STATUSES: { value: TrajEndStatus; label: string }[] = [
  {
    value: '',
    get label() {
      return tr('meta.trajectoryStatuses.any')
    },
  },
  {
    value: 'done',
    get label() {
      return tr('meta.trajectoryStatuses.done')
    },
  },
  {
    value: 'failed',
    get label() {
      return tr('meta.trajectoryStatuses.failed')
    },
  },
  {
    value: 'abandoned',
    get label() {
      return tr('meta.trajectoryStatuses.abandoned')
    },
  },
]

// Trajectory-trigger prompt placeholders (kept in sync with
// agent/automation_trajectory.go trajectoryVars).
export const TRAJ_PROMPT_VARS: { name: string; desc: string }[] = [
  ...[
    'trajectoryId',
    'rootSessionId',
    'sessionId',
    'recipe',
    'phase',
    'phaseState',
    'event',
    'status',
    'phases',
    'iteration',
    'maxIterations',
    'automation',
    'date',
    'time',
    'datetime',
  ].map((name) => ({
    name: `{{${name}}}`,
    get desc() {
      return tr(`meta.promptVars.trajectory.${name}`)
    },
  })),
]

// Flow-trigger outcome filter options (_Docs/93).
export const FLOW_STATUSES: { value: FlowRuleStatus; label: string }[] = [
  {
    value: '',
    get label() {
      return tr('meta.flowStatuses.any')
    },
  },
  {
    value: 'success',
    get label() {
      return tr('meta.flowStatuses.success')
    },
  },
  {
    value: 'failure',
    get label() {
      return tr('meta.flowStatuses.failure')
    },
  },
]

// Flow-trigger prompt placeholders (kept in sync with agent/automation_flow.go flowVars).
export const FLOW_PROMPT_VARS: { name: string; desc: string }[] = [
  'result',
  'input',
  'output',
  'error',
  'status',
  'grade',
  'runId',
  'flowId',
  'flowName',
  'version',
  'agent',
  'agentId',
  'sessionId',
  'nodeId',
  'iteration',
  'maxIterations',
  'automation',
  'date',
  'time',
  'datetime',
].map((name) => ({
  name: `{{${name}}}`,
  get desc() {
    return tr(`meta.promptVars.flow.${name}`)
  },
}))

// Fallback columns used until workspace board columns load (mirrors TaskBoard).
export const DEFAULT_COLUMNS: BoardColumnDef[] = [
  { key: 'pbi', label: 'PBI', color: '' },
  {
    key: 'todo',
    get label() {
      return tr('meta.defaultColumns.todo')
    },
    color: '',
  },
  {
    key: 'in_progress',
    get label() {
      return tr('meta.defaultColumns.inProgress')
    },
    color: '',
  },
  {
    key: 'review',
    get label() {
      return tr('meta.defaultColumns.review')
    },
    color: '',
  },
  {
    key: 'done',
    get label() {
      return tr('meta.defaultColumns.done')
    },
    color: '',
  },
  {
    key: 'failed',
    get label() {
      return tr('meta.defaultColumns.failed')
    },
    color: '',
  },
  {
    key: 'iptal',
    get label() {
      return tr('meta.defaultColumns.cancelled')
    },
    color: '',
  },
]

// Board-trigger operation options (label = Turkish UI text).
export const BOARD_OPS: { value: BoardOp; label: string }[] = [
  {
    value: 'move',
    get label() {
      return tr('meta.boardOps.move')
    },
  },
  {
    value: 'create',
    get label() {
      return tr('meta.boardOps.create')
    },
  },
  {
    value: 'update',
    get label() {
      return tr('meta.boardOps.update')
    },
  },
  {
    value: 'delete',
    get label() {
      return tr('meta.boardOps.delete')
    },
  },
  {
    value: 'any',
    get label() {
      return tr('meta.boardOps.any')
    },
  },
]

export function boardOpLabel(op?: BoardOp): string {
  return BOARD_OPS.find((o) => o.value === (op || 'move'))?.label ?? String(op ?? '')
}

// Board-trigger action options: 'spawn' runs the target (board drives execution),
// 'archive' hides the finished card off the board with no LLM call.
export const BOARD_ACTIONS: { value: BoardAction; label: string }[] = [
  {
    value: 'spawn',
    get label() {
      return tr('meta.boardActions.spawn')
    },
  },
  {
    value: 'archive',
    get label() {
      return tr('meta.boardActions.archive')
    },
  },
  {
    value: 'move',
    get label() {
      return tr('meta.boardActions.move')
    },
  },
]

// Token-trigger scope options (label = Turkish UI text).
export const TOKEN_SCOPES: { value: TokenScope; label: string }[] = [
  {
    value: 'session',
    get label() {
      return tr('meta.tokenScopes.session')
    },
  },
  {
    value: 'workspace',
    get label() {
      return tr('meta.tokenScopes.workspace')
    },
  },
]

// Tag-trigger prompt placeholders (kept in sync with agent/automation.go turnVars).
export const PROMPT_VARS: { name: string; desc: string }[] = [
  'result',
  'title',
  'tag',
  'sessionId',
  'iteration',
  'maxIterations',
  'agent',
  'prevPrompt',
  'automation',
  'date',
  'time',
  'datetime',
].map((name) => ({
  name: `{{${name}}}`,
  get desc() {
    return tr(`meta.promptVars.tag.${name}`)
  },
}))

// Board-trigger prompt placeholders (kept in sync with agent/automation.go boardVars).
export const BOARD_PROMPT_VARS: { name: string; desc: string }[] = [
  'taskId',
  'title',
  'op',
  'from',
  'to',
  'fromLabel',
  'toLabel',
  'board',
  'tags',
  'owner',
  'priority',
  'iteration',
  'maxIterations',
  'automation',
  'date',
  'time',
  'datetime',
].map((name) => ({
  name: `{{${name}}}`,
  get desc() {
    return tr(`meta.promptVars.board.${name}`)
  },
}))

// Token-trigger prompt placeholders (kept in sync with agent/automation.go tokenVars).
export const TOKEN_PROMPT_VARS: { name: string; desc: string }[] = [
  'tokens',
  'threshold',
  'scope',
  'sessionId',
  'iteration',
  'maxIterations',
  'automation',
  'date',
  'time',
  'datetime',
].map((name) => ({
  name: `{{${name}}}`,
  get desc() {
    return tr(`meta.promptVars.token.${name}`)
  },
}))

// Default prompt template for a fresh automation of each kind.
export const DEFAULT_PROMPT: Record<AutomationTriggerKind, string> = {
  tag: 'Devam et. Önceki sonuç:\n{{result}}',
  board: 'Bir kart taşındı: {{title}} ({{op}} → {{toLabel}}). Gereğini yap.',
  token:
    'Bu {{scope}} {{tokens}} token eşiğini ({{threshold}}) geçti. Kendi kendine bakım yap: ' +
    'gereksiz artefaktları/oturumları temizle, bağlamı sıkıştır/özetle, optimizasyon fırsatlarını uygula. ' +
    'Oturum: {{sessionId}}',
  phase:
    'Rota {{trajectoryId}} ({{recipe}}) "{{phase}}" fazını {{phaseState}} ile bitirdi. ' +
    'Fazlar: {{phases}}. Kök oturum {{rootSessionId}}. Bu fazın çıktısını gözden geçir ve gerekeni yap.',
  trajectory_end:
    'Rota {{trajectoryId}} ({{recipe}}) {{status}} ile bitti. Fazlar: {{phases}}. Kök oturum ' +
    '{{rootSessionId}}. Koşuyu özetle, dersleri çıkar ve dokümanları/panoyu güncelle.',
  flow:
    '{{agent}} ajanının akış koşusu {{runId}} {{status}} ile bitti (puan {{grade}}/5, v{{version}}). ' +
    'Girdi: {{input}}\n\nYanıt: {{result}}\n\nYanıtı değerlendir; eksikse düzelt ve gerekiyorsa ' +
    'ajanın akışına/promptuna bir iyileştirme öner.',
}

// Prefill body for the "stuck session repairer" template (self-healing, _Docs/56):
// fires on the failing turn of a session tagged `stuck`, spawns a fixer that
// diagnoses via the debug journal; on success the framework clears the parent's
// stuck tag + counter, re-opening autonomy.
export const STUCK_TEMPLATE = {
  get name() {
    return tr('automationModal.stuckTemplate')
  },
  triggerTag: 'stuck',
  maxIterations: '10',
  cooldownSec: '300',
  promptTemplate:
    'Session {{sessionId}} ("{{title}}") is STUCK: it failed several consecutive turns and its ' +
    'autonomous turns are now suspended. Last error:\n{{result}}\n\n' +
    'Diagnose and fix it:\n' +
    '1. Read its debug journal (read_session_debug with session_id {{sessionId}}) and recent ' +
    'messages (conversation_search) to find the failing tool calls and the root cause.\n' +
    '2. Fix the underlying problem if it is fixable (wrong path/config, missing file, bad state). ' +
    'Check note_search for known failure shapes first.\n' +
    '3. Report what you found and what you changed. Do NOT retry the same failing calls blindly.\n' +
    'When you finish successfully, the stuck tag and counter are cleared automatically.',
}

// Per-column accent colors, shared by the column headers and the card left border.
export const COLUMN_ACCENT = {
  schedules: '#6b8e23',
  tag: '#8b5cf6',
  board: '#0ea5e9',
  token: '#f59e0b',
  phase: '#a855f7',
  trajectory_end: '#ec4899',
  flow: '#14b8a6',
} as const

// MAX_ITERATIONS_HARD_CAP is the ceiling the automation form allows.
//
// Mirrors db.MaxIterationsHardCap in internal/db/automation_limits.go — the SERVER
// is authoritative and rejects out-of-range writes with a message, so if these two
// ever drift the only symptom is a form whose `max` attribute is slightly off, not
// an unbounded automation slipping through.
//
// The floor is 1, not 0: the runtime reads 0 as "unlimited", which is a
// self-sustaining loop with no lifetime brake.
export const MAX_ITERATIONS_HARD_CAP = 500

// DEFAULT_MAX_ITERATIONS mirrors defaultAutomationMaxIterations in
// internal/api/automations.go: the bound a new automation gets when the user does
// not choose one.
export const DEFAULT_MAX_ITERATIONS = 50

// MIN_TOKEN_THRESHOLD mirrors db.MinTokenThreshold: the smallest interval a token
// automation may set (server-authoritative; drift only nudges the form's `min`).
export const MIN_TOKEN_THRESHOLD = 1000

// DEFAULT_TOKEN_THRESHOLD is a sensible prefill for a new token automation.
export const DEFAULT_TOKEN_THRESHOLD = 200000
