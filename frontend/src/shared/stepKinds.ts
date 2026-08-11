// Single source of truth describing every assistant-turn StepKind: the icon,
// label, persistence and status shared by BOTH the chat renderer (features/chat/*
// step components) and the Settings → "Adım Türleri" reference screen. Each kind
// carries a lucide icon component so the two surfaces render the SAME glyph and
// can never drift. Mirrors the StepKind constants in internal/agent/trace.go.
import {
  Ban,
  Bot,
  Brain,
  ClipboardList,
  CornerDownRight,
  ListChecks,
  MessageCircleQuestion,
  MessageSquare,
  Pencil,
  RefreshCw,
  ShieldAlert,
  Snowflake,
  Terminal,
  Trash2,
  TriangleAlert,
  Waves,
  Webhook,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import type { StepKind } from '@/types'

export type StepStatus = 'active' | 'infra'

export interface StepKindInfo {
  kind: StepKind
  label: string
  /** Shared lucide icon rendered by both the chat step card and the settings screen. */
  Icon: LucideIcon
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
    Icon: MessageSquare,
    persisted: true,
    status: 'active',
    description: 'Modelin araç çağrıları arasında ürettiği ara anlatım metni.',
  },
  {
    kind: 'thinking',
    label: 'Düşünme',
    Icon: Brain,
    persisted: true,
    status: 'active',
    description: 'Modelin akıl yürütme (thinking) içeriği — sağlayıcı sunduğunda.',
  },
  {
    kind: 'tool',
    label: 'Araç',
    Icon: Wrench,
    persisted: true,
    status: 'active',
    description: 'Tek bir araç çağrısı: girdi + sonuç. Edit/Write çıktısı diff olarak gösterilir.',
  },
  {
    kind: 'delta',
    label: 'Akış parçası',
    Icon: Waves,
    persisted: false,
    status: 'active',
    description:
      'Yanıt token token üretilirken gelen anlık metin parçası. Geçici — tam metin mesaja yazılır.',
  },
  {
    kind: 'ask',
    label: 'Soru',
    Icon: MessageCircleQuestion,
    persisted: false,
    status: 'active',
    description:
      'ask_user aracı kullanıcıya soru sorup yanıt bekler (tur askıya alınır). Geçici; yanıt gelince araç adımı kalıcılaşır.',
  },
  {
    kind: 'permission',
    label: 'İzin onayı',
    Icon: ShieldAlert,
    persisted: false,
    status: 'active',
    description:
      'İzin kapısı (ask modu): yazma/komut aracı için kullanıcıdan onay bekler (İzin ver / Her zaman izin ver / Reddet). Geçici; karar verilince araç çalışır ya da permission_denied hatası yazılır. Hem native hem claude-cli (permission-prompt) yolunda.',
  },
  {
    kind: 'plan',
    label: 'Plan onayı',
    Icon: ClipboardList,
    persisted: false,
    status: 'active',
    description:
      'Plan onayı (claude-cli): ajan ExitPlanMode ile planını sunup kullanıcıdan onay bekler (Planı onayla / Reddet). Geçici; onaylanınca tur devam eder, reddedilince model planı revize eder. Yalnız ask ve salt-okunur modda; otonom turda otomatik onaylanır.',
  },
  {
    kind: 'todo',
    label: 'Görev listesi',
    Icon: ListChecks,
    persisted: true,
    status: 'active',
    description: 'todo_write aracının çalışma checklist’i; ilk-sınıf kart olarak render edilir.',
  },
  {
    kind: 'recovery',
    label: 'Kurtarma',
    Icon: TriangleAlert,
    persisted: true,
    status: 'active',
    description:
      'Döngü mutlu-yol dışına çıktı (ör. iterasyon limiti). Reason makine etiketi taşır.',
  },
  {
    kind: 'error',
    label: 'Hata',
    Icon: Ban,
    persisted: true,
    status: 'active',
    description:
      'Tur düzeyinde hata (sağlayıcı hatası, bütçe aşımı, iptal) — araç hatasından (tool+isError) ayrı.',
  },
  {
    kind: 'steer',
    label: 'Yönlendirme',
    Icon: CornerDownRight,
    persisted: true,
    status: 'active',
    description:
      'Çalışan tura canlı eklenen kullanıcı yönlendirmesi (steer); modelden ayrı gösterilir.',
  },
  {
    kind: 'tool_delta',
    label: 'Araç çıktı akışı',
    Icon: Terminal,
    persisted: false,
    status: 'active',
    description:
      'Uzun bir aracın çıktısını çalışırken parça parça akıtır (aynı ID birleştirilir). shell aracı stdout/stderr’i canlı akıtır.',
  },
  {
    kind: 'tombstone',
    label: 'Geri çekme',
    Icon: Trash2,
    persisted: false,
    status: 'active',
    description:
      'Daha önce yayılan canlı bir adımı UI’dan kaldıran kontrol sinyali (Ref hedef adım ID’si). Kendisi render edilmez. Akış bitince placeholder’ı geri çeker; iptalde de kullanılır.',
  },
  {
    kind: 'diff',
    label: 'Dosya değişikliği',
    Icon: Pencil,
    persisted: true,
    status: 'active',
    description:
      'Write / Edit ile yapılan dosya değişikliği; yol + eklenen/silinen satır sayısı ve açılabilir birleşik diff (patch) olarak gösterilir.',
  },
  {
    kind: 'hook',
    label: 'Hook',
    Icon: Webhook,
    persisted: true,
    status: 'active',
    description:
      'Kullanıcı-tanımlı PreToolUse/PostToolUse hook bir araç çağrısının etrafında çalıştı: girdiyi/çıktıyı değiştirdi, otomatik onayladı, ek bağlam ekledi ya da çağrıyı engelledi. Reason makine kararını taşır (hook_block / hook_modify / hook_allow / hook_context). Yalnız native (anthropic/minimax) yolda.',
  },
  {
    kind: 'context_change',
    label: 'Bağlam değişikliği',
    Icon: RefreshCw,
    persisted: true,
    status: 'active',
    description:
      'Oturumun dondurulmuş statik bağlamı (persona/talimat/skill/araç kataloğu) oturum ortasında değişti; prompt cache’i korumak için önbelleğe giren prefix hâlâ oturum-başı snapshot’ını taşır. Değişen bloklar +/- diff olarak gösterilir. Drift başına bir kez yayılır; değişiklik bir sonraki bağlam yenilemesinde (/refresh-context, compaction, boşta kalma) tam uygulanır.',
  },
  {
    kind: 'cache_break',
    label: 'Cache kırılması',
    Icon: Snowflake,
    persisted: true,
    status: 'active',
    description:
      'Bu tur oturumun sıcak prompt-cache önekini kaybetti ve öneki baştan (soğuk) ödedi. Yalnız “bir şey değişti” sebepleri kart olur: model değişimi ve sistem promptu/araç şeması değişimi. Uzun boşluk sonrası TTL soğuması kart açmaz (normaldir) — o yalnız mesaj debug panelinde ve transkriptteki soğuk ayracında görünür. Prompt epoch açıkken prompt/araç kaynaklı kırılım oturum ortasında BEKLENMEZ; görülüyorsa araştırılmalıdır.',
  },
  {
    kind: 'subagent',
    label: 'Alt-ajan',
    Icon: Bot,
    persisted: true,
    status: 'active',
    description:
      'run_subagent ile başlatılan izole alt-ajan: kendi temiz bağlamında bir görevi yürütüp yalnız final sonucunu döndürür (ana bağlam kirlenmez). Katlanabilir kart; açılınca alt-ajanın kendi iz ağacı (SubSteps) iç içe gösterilir. Tek turda birden çok çağrı paralel koşar.',
  },
]

// STEP_KIND_MAP is the O(1) lookup the chat step components use to render the
// same icon the settings screen lists — the shared glyph per kind.
export const STEP_KIND_MAP: Record<StepKind, StepKindInfo> = Object.fromEntries(
  STEP_KINDS.map((s) => [s.kind, s]),
) as Record<StepKind, StepKindInfo>
