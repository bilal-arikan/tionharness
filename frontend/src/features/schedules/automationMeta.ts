import type {
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  TokenScope,
  TrajEndStatus,
  TrajEvent,
} from '@/types'

// Rota (F2) trigger options.
export const TRAJ_EVENTS: { value: TrajEvent; label: string }[] = [
  { value: 'exit', label: 'faz bitince (done/skipped/failed)' },
  { value: 'enter', label: 'faz başlayınca (active)' },
]

export const TRAJ_END_STATUSES: { value: TrajEndStatus; label: string }[] = [
  { value: '', label: 'her bitiş' },
  { value: 'done', label: 'tamamlandı' },
  { value: 'failed', label: 'başarısız' },
  { value: 'abandoned', label: 'terk edildi' },
]

// Trajectory-trigger prompt placeholders (kept in sync with
// agent/automation_trajectory.go trajectoryVars).
export const TRAJ_PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{trajectoryId}}', desc: 'Rota kimliği (RTA…)' },
  { name: '{{rootSessionId}}', desc: 'Rotanın kök (koordinatör) oturumu' },
  { name: '{{sessionId}}', desc: 'Kök oturum (aynı)' },
  { name: '{{recipe}}', desc: 'Reçete slug’ı (plansız rotada boş)' },
  { name: '{{phase}}', desc: 'Geçiş yapan faz (faz tetiklerinde)' },
  { name: '{{phaseState}}', desc: 'Fazın yeni durumu (done/skipped/failed/active)' },
  { name: '{{event}}', desc: 'exit | enter' },
  { name: '{{status}}', desc: 'Rota durumu (bitişte done/failed/abandoned)' },
  { name: '{{phases}}', desc: 'Faz satırı: plan ✓ → kod ● → inceleme ○' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (eski kayıtlarda 0 → ∞)' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih' },
  { name: '{{time}}', desc: 'Geçerli saat' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Fallback columns used until workspace board columns load (mirrors TaskBoard).
export const DEFAULT_COLUMNS: BoardColumnDef[] = [
  { key: 'pbi', label: 'PBI', color: '' },
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'in_progress', label: 'Devam Eden', color: '' },
  { key: 'review', label: 'İnceleme', color: '' },
  { key: 'done', label: 'Bitti', color: '' },
  { key: 'failed', label: 'Başarısız', color: '' },
  { key: 'iptal', label: 'İptal', color: '' },
]

// Board-trigger operation options (label = Turkish UI text).
export const BOARD_OPS: { value: BoardOp; label: string }[] = [
  { value: 'move', label: 'Taşındı (sütun değişti)' },
  { value: 'create', label: 'Oluşturuldu' },
  { value: 'update', label: 'Güncellendi' },
  { value: 'delete', label: 'Silindi' },
  { value: 'any', label: 'Herhangi bir değişiklik' },
]

export function boardOpLabel(op?: BoardOp): string {
  return BOARD_OPS.find((o) => o.value === (op || 'move'))?.label ?? String(op ?? '')
}

// Board-trigger action options: 'spawn' runs the target (board drives execution),
// 'archive' hides the finished card off the board with no LLM call.
export const BOARD_ACTIONS: { value: BoardAction; label: string }[] = [
  { value: 'spawn', label: 'Ajanı/akışı başlat (yürütme)' },
  { value: 'archive', label: 'Kartı arşivle (LLM yok)' },
  { value: 'move', label: 'Kartı sütuna taşı (LLM yok)' },
]

// Token-trigger scope options (label = Turkish UI text).
export const TOKEN_SCOPES: { value: TokenScope; label: string }[] = [
  { value: 'session', label: 'Oturum (bir oturumun ömür-boyu tokenı)' },
  { value: 'workspace', label: 'Workspace (bugünkü toplam token)' },
]

// Tag-trigger prompt placeholders (kept in sync with agent/automation.go turnVars).
export const PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{result}}', desc: 'Biten oturumun son yanıtı' },
  { name: '{{title}}', desc: 'Biten oturumun başlığı' },
  { name: '{{tag}}', desc: 'Tetikleyici etiket' },
  { name: '{{sessionId}}', desc: 'Biten oturumun ID’si' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (eski kayıtlarda 0 → ∞)' },
  { name: '{{agent}}', desc: 'Sonucu üreten ajanın adı ({{agentName}} eşdeğer)' },
  { name: '{{prevPrompt}}', desc: 'Bir önceki turu tetikleyen kullanıcı promptu' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih (2026-07-03)' },
  { name: '{{time}}', desc: 'Geçerli saat (03:00)' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Board-trigger prompt placeholders (kept in sync with agent/automation.go boardVars).
export const BOARD_PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{taskId}}', desc: 'Değişen kartın ID’si' },
  { name: '{{title}}', desc: 'Kartın başlığı' },
  { name: '{{op}}', desc: 'İşlem (move/create/update/delete)' },
  { name: '{{from}}', desc: 'Önceki sütun anahtarı' },
  { name: '{{to}}', desc: 'Yeni sütun anahtarı' },
  { name: '{{fromLabel}}', desc: 'Önceki sütun adı' },
  { name: '{{toLabel}}', desc: 'Yeni sütun adı' },
  { name: '{{board}}', desc: 'Güncel sütun (= {{to}})' },
  { name: '{{tags}}', desc: 'Kartın etiketleri (virgülle ayrık)' },
  { name: '{{owner}}', desc: 'Atanan ajanın adı (boş = atanmamış)' },
  { name: '{{priority}}', desc: 'Öncelik (critical/high/medium/low, boş olabilir)' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (eski kayıtlarda 0 → ∞)' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih' },
  { name: '{{time}}', desc: 'Geçerli saat' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Token-trigger prompt placeholders (kept in sync with agent/automation.go tokenVars).
export const TOKEN_PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{tokens}}', desc: 'Eşiği geçen kümülatif token toplamı' },
  { name: '{{threshold}}', desc: 'Token aralığı (eşik)' },
  { name: '{{scope}}', desc: 'Kapsam (session/workspace)' },
  { name: '{{sessionId}}', desc: 'Geçişi tetikleyen oturum (workspace kapsamında boş)' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (eski kayıtlarda 0 → ∞)' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih' },
  { name: '{{time}}', desc: 'Geçerli saat' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

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
}

// Prefill body for the "stuck session repairer" template (self-healing, _Docs/56):
// fires on the failing turn of a session tagged `stuck`, spawns a fixer that
// diagnoses via the debug journal; on success the framework clears the parent's
// stuck tag + counter, re-opening autonomy.
export const STUCK_TEMPLATE = {
  name: 'Stuck oturum onarıcısı',
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
    'Check read_lessons for known failure shapes first.\n' +
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
