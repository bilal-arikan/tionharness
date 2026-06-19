// Shared building blocks for the settings screen: field primitives, the category
// rail button, the read-only prompt viewer and the category taxonomy.
import { type ReactNode } from 'react'
import {
  User, Palette, KeyRound, Brain, Shield, Command,
  Blocks, Info, Boxes, FileText, FolderOpen, Wrench, SlidersHorizontal, Webhook, Plug, type LucideIcon,
} from 'lucide-react'
import type { AppSettings, PromptInfo, WorkspaceSettings } from '../../types'
import { CopyPathButton } from '../CopyPathButton'
import { displayPath } from '../../lib/paths'

// Category keys: the app-global sections plus the per-workspace section.
export type Cat =
  | 'profile'
  | 'appearance'
  | 'providers'
  | 'context'
  | 'budget'
  | 'tools'
  | 'mcptools'
  | 'hooks'
  | 'advanced'
  | 'commands'
  | 'stepkinds'
  | 'about'
  | 'workspace'
  | 'wsfiles'

export interface CatMeta {
  key: Cat
  label: string
  icon: LucideIcon
}

export const APP_CATS: CatMeta[] = [
  { key: 'profile', label: 'Profil', icon: User },
  { key: 'appearance', label: 'Görünüm', icon: Palette },
  { key: 'providers', label: 'Sağlayıcılar', icon: KeyRound },
  { key: 'context', label: 'Bağlam & Bellek', icon: Brain },
  { key: 'budget', label: 'Bütçe', icon: Shield },
  { key: 'tools', label: 'Yetenekler (Araçlar)', icon: Wrench },
  { key: 'mcptools', label: 'Araçlar & MCP', icon: Plug },
  { key: 'hooks', label: 'Hooks', icon: Webhook },
  // Combined screen: notifications, autonomy, auto-title, MCP, diagnostics.
  { key: 'advanced', label: 'Gelişmiş', icon: SlidersHorizontal },
  { key: 'commands', label: 'Komutlar', icon: Command },
  { key: 'stepkinds', label: 'Adım Türleri', icon: Blocks },
  { key: 'about', label: 'Hakkında', icon: Info },
]

export const WS_CATS: CatMeta[] = [
  { key: 'workspace', label: 'Genel', icon: Boxes },
  { key: 'wsfiles', label: 'Promptlar & Dosyalar', icon: FileText },
]

// Setters threaded into the per-category panels.
export type AppSet = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) => void
export type WsSet = <K extends keyof WorkspaceSettings>(key: K, val: WorkspaceSettings[K]) => void

export const inputCls =
  'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {hint && <span className="text-xs text-[var(--color-text-dim)]">{hint}</span>}
    </label>
  )
}

export function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <button
      onClick={() => onChange(!checked)}
      className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-left"
    >
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-[var(--color-text-dim)]">{hint}</span>}
      </span>
      <span
        className={`relative h-5 w-9 flex-shrink-0 rounded-full transition ${
          checked ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-surface-2)]'
        }`}
      >
        <span
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${
            checked ? 'left-4' : 'left-0.5'
          }`}
        />
      </span>
    </button>
  )
}

export function CatButton({
  c,
  active,
  onClick,
  dirty,
}: {
  c: CatMeta
  active: boolean
  onClick: () => void
  dirty: boolean
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
        active ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <c.icon size={16} className="shrink-0" />
      <span className="flex-1 truncate">{c.label}</span>
      {dirty && <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]" title="Kaydedilmemiş" />}
    </button>
  )
}

// PromptDetails renders one built-in prompt read-only (System / User-turn blocks
// + note) with a button to open the source folder. Used inside the expandable
// command cards on the Komutlar screen.
export function PromptDetails({ p, dir, onReveal }: { p: PromptInfo; dir: string; onReveal: () => void }) {
  return (
    <>
      <div className="mb-2 flex items-center justify-between gap-2">
        <code className="rounded bg-[var(--color-surface-2)] px-1 text-[10px] text-[var(--color-text-dim)]">{p.file}</code>
        <div className="flex shrink-0 items-center gap-1">
          <CopyPathButton path={dir} />
          <button
            onClick={onReveal}
            disabled={!dir}
            title={dir ? displayPath(dir) : 'Klasör yolu bilinmiyor'}
            className="flex shrink-0 items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)] disabled:opacity-40"
          >
            <FolderOpen size={12} /> Klasörü aç
          </button>
        </div>
      </div>
      {p.system && (
        <div className="mb-2">
          <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">System</div>
          <pre className="mt-0.5 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-xs text-[var(--color-text)]">{p.system}</pre>
        </div>
      )}
      {p.user && (
        <div className="mb-2">
          <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">User turn</div>
          <pre className="mt-0.5 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-xs text-[var(--color-text)]">{p.user}</pre>
        </div>
      )}
      {p.note && <p className="text-xs text-[var(--color-text-dim)]">{p.note}</p>}
    </>
  )
}
