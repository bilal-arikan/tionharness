import type { AutomationTriggerKind, BoardColumnDef, BoardOp } from '@/types'

// Fallback columns used until workspace board columns load (mirrors TaskBoard).
export const DEFAULT_COLUMNS: BoardColumnDef[] = [
  { key: 'pbi', label: 'PBI', color: '' },
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'in_progress', label: 'Devam Eden', color: '' },
  { key: 'review', label: 'İnceleme', color: '' },
  { key: 'done', label: 'Bitti', color: '' },
  { key: 'failed', label: 'Başarısız', color: '' },
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

// Tag-trigger prompt placeholders (kept in sync with agent/automation.go turnVars).
export const PROMPT_VARS: { name: string; desc: string }[] = [
  { name: '{{result}}', desc: 'Biten oturumun son yanıtı' },
  { name: '{{title}}', desc: 'Biten oturumun başlığı' },
  { name: '{{tag}}', desc: 'Tetikleyici etiket' },
  { name: '{{sessionId}}', desc: 'Biten oturumun ID’si' },
  { name: '{{iteration}}', desc: 'Bu ateşlemenin sıra no’su (1-tabanlı)' },
  { name: '{{maxIterations}}', desc: 'Üst sınır (0 → ∞)' },
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
  { name: '{{maxIterations}}', desc: 'Üst sınır (0 → ∞)' },
  { name: '{{automation}}', desc: 'Otomasyonun adı' },
  { name: '{{date}}', desc: 'Geçerli tarih' },
  { name: '{{time}}', desc: 'Geçerli saat' },
  { name: '{{datetime}}', desc: 'Tarih + saat' },
]

// Default prompt template for a fresh automation of each kind.
export const DEFAULT_PROMPT: Record<AutomationTriggerKind, string> = {
  tag: 'Devam et. Önceki sonuç:\n{{result}}',
  board: 'Bir kart taşındı: {{title}} ({{op}} → {{toLabel}}). Gereğini yap.',
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
} as const
