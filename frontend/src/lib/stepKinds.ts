// Single source of truth describing every assistant-turn StepKind: used by the
// chat renderer and the Settings → "Adım Türleri" reference screen. Mirrors the
// StepKind constants in internal/agent/trace.go.
import type { StepKind } from '../types'

export type StepStatus = 'active' | 'infra'

export interface StepKindInfo {
  kind: StepKind
  label: string
  icon: string
  /** Whether the step persists in the saved trace or is live-only. */
  persisted: boolean
  /** "active" = a producer emits it today; "infra" = type/UI ready, producer pending. */
  status: StepStatus
  description: string
}

export const STEP_KINDS: StepKindInfo[] = [
  {
    kind: 'text',
    label: 'Metin',
    icon: '💬',
    persisted: true,
    status: 'active',
    description: 'Modelin araç çağrıları arasında ürettiği ara anlatım metni.',
  },
  {
    kind: 'thinking',
    label: 'Düşünme',
    icon: '🧠',
    persisted: true,
    status: 'active',
    description: 'Modelin akıl yürütme (thinking) içeriği — sağlayıcı sunduğunda.',
  },
  {
    kind: 'tool',
    label: 'Araç',
    icon: '🛠️',
    persisted: true,
    status: 'active',
    description: 'Tek bir araç çağrısı: girdi + sonuç. Edit/Write çıktısı diff olarak gösterilir.',
  },
  {
    kind: 'delta',
    label: 'Akış parçası',
    icon: '🌊',
    persisted: false,
    status: 'active',
    description:
      'Yanıt token token üretilirken gelen anlık metin parçası. Geçici — tam metin mesaja yazılır.',
  },
  {
    kind: 'ask',
    label: 'Soru',
    icon: '❓',
    persisted: false,
    status: 'active',
    description:
      'ask_user aracı kullanıcıya soru sorup yanıt bekler (tur askıya alınır). Geçici; yanıt gelince araç adımı kalıcılaşır.',
  },
  {
    kind: 'todo',
    label: 'Görev listesi',
    icon: '✅',
    persisted: true,
    status: 'active',
    description: 'todo_write aracının çalışma checklist’i; ilk-sınıf kart olarak render edilir.',
  },
  {
    kind: 'recovery',
    label: 'Kurtarma',
    icon: '⚠️',
    persisted: true,
    status: 'active',
    description:
      'Döngü mutlu-yol dışına çıktı (ör. iterasyon limiti). Reason makine etiketi taşır.',
  },
  {
    kind: 'error',
    label: 'Hata',
    icon: '⛔',
    persisted: true,
    status: 'active',
    description:
      'Tur düzeyinde hata (sağlayıcı hatası, bütçe aşımı, iptal) — araç hatasından (tool+isError) ayrı.',
  },
  {
    kind: 'steer',
    label: 'Yönlendirme',
    icon: '↪️',
    persisted: true,
    status: 'active',
    description: 'Çalışan tura canlı eklenen kullanıcı yönlendirmesi (steer); modelden ayrı gösterilir.',
  },
  {
    kind: 'tool_delta',
    label: 'Araç çıktı akışı',
    icon: '📟',
    persisted: false,
    status: 'active',
    description:
      'Uzun bir aracın çıktısını çalışırken parça parça akıtır (aynı ID birleştirilir). shell aracı stdout/stderr’i canlı akıtır.',
  },
  {
    kind: 'tombstone',
    label: 'Geri çekme',
    icon: '🗑️',
    persisted: false,
    status: 'active',
    description:
      'Daha önce yayılan canlı bir adımı UI’dan kaldıran kontrol sinyali (Ref hedef adım ID’si). Kendisi render edilmez. Akış bitince placeholder’ı geri çeker; iptalde de kullanılır.',
  },
]
