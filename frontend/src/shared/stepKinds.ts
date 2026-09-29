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
  FoldVertical,
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
import { sharedText } from './lib/sharedI18n'

type StepStatus = 'active' | 'infra' | 'legacy'

export interface StepKindInfo {
  kind: StepKind
  label: string
  /** Shared lucide icon rendered by both the chat step card and the settings screen. */
  Icon: LucideIcon
  /** Whether the step persists in the saved trace or is live-only. */
  persisted: boolean
  /** "active" = produced today; "infra" = producer pending; "legacy" = read-only compatibility. */
  status: StepStatus
  description: string
}

function stepKind(info: Omit<StepKindInfo, 'label' | 'description'>): StepKindInfo {
  return {
    ...info,
    get label() {
      return sharedText(`stepKinds.${info.kind}.label`)
    },
    get description() {
      return sharedText(`stepKinds.${info.kind}.description`)
    },
  }
}

export const STEP_KINDS: StepKindInfo[] = [
  stepKind({
    kind: 'text',
    Icon: MessageSquare,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'thinking',
    Icon: Brain,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'tool',
    Icon: Wrench,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'delta',
    Icon: Waves,
    persisted: false,
    status: 'active',
  }),
  stepKind({
    kind: 'ask',
    Icon: MessageCircleQuestion,
    persisted: false,
    status: 'active',
  }),
  stepKind({
    kind: 'permission',
    Icon: ShieldAlert,
    persisted: false,
    status: 'active',
  }),
  stepKind({
    kind: 'plan',
    Icon: ClipboardList,
    persisted: false,
    status: 'active',
  }),
  stepKind({
    kind: 'todo',
    Icon: ListChecks,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'recovery',
    Icon: TriangleAlert,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'error',
    Icon: Ban,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'steer',
    Icon: CornerDownRight,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'tool_delta',
    Icon: Terminal,
    persisted: false,
    status: 'legacy',
  }),
  stepKind({
    kind: 'tombstone',
    Icon: Trash2,
    persisted: false,
    status: 'active',
  }),
  stepKind({
    kind: 'diff',
    Icon: Pencil,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'hook',
    Icon: Webhook,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'context_change',
    Icon: RefreshCw,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'cache_break',
    Icon: Snowflake,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'compaction',
    Icon: FoldVertical,
    persisted: true,
    status: 'active',
  }),
  stepKind({
    kind: 'subagent',
    Icon: Bot,
    persisted: true,
    status: 'active',
  }),
]

// STEP_KIND_MAP is the O(1) lookup the chat step components use to render the
// same icon the settings screen lists — the shared glyph per kind.
export const STEP_KIND_MAP: Record<StepKind, StepKindInfo> = Object.fromEntries(
  STEP_KINDS.map((s) => [s.kind, s]),
) as Record<StepKind, StepKindInfo>
