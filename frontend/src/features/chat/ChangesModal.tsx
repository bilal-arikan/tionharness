import { useEffect, useMemo, useRef, useState } from 'react'
import { X, Copy, Download, FileText } from 'lucide-react'
import { toast } from '@/shared/components'
import type { TurnStep } from '@/types'
import { api } from '@/api'
import { ModalOverlay } from '@/shared/components/ModalOverlay'
import { DiffView } from '@/shared/components/markdown/DiffView'
import { shortPath } from '@/shared/lib/paths'
import {
  extractFileChanges,
  groupByPath,
  stepToFileChange,
  type FileChange,
  type FileGroup,
} from '@/shared/lib/fileChanges'

type Scope = 'turn' | 'session'

interface Props {
  sessionId?: string
  /** The turn this popup was opened from — its already-parsed trace. */
  turnSteps: TurnStep[]
  /** Message id of that turn; used to refetch its untrimmed trace. */
  msgId: string
  /** Any of the turn's patches was cut server-side → refetch on open. */
  turnTruncated: boolean
  onClose: () => void
  onOpenFile?: (path: string) => void
}

// ChangesModal shows every file this agent touched, as a master-detail browser:
// files on the left, the selected file's diff on the right.
//
// Master-detail rather than one long scroll of stacked diffs on purpose — only
// the selected file's patch is ever mounted, so twenty changed files cost the
// same as one. The heavy lifting inside a single patch (context folding, row
// virtualization, a hard cap) lives in DiffView's `panel` variant.
export function ChangesModal({
  sessionId,
  turnSteps,
  msgId,
  turnTruncated,
  onClose,
  onOpenFile,
}: Props) {
  const [scope, setScope] = useState<Scope>('turn')
  const [selected, setSelected] = useState<string>('')

  // The turn's own changes. The transcript arrives with patches cut to the
  // server's read-path cap, so when any of them is flagged we swap in the
  // untrimmed trace — a diff viewer showing a silently clipped patch is worse
  // than not showing one.
  const [fullTurnSteps, setFullTurnSteps] = useState<TurnStep[] | null>(null)
  useEffect(() => {
    if (!turnTruncated || !sessionId || fullTurnSteps) return
    let alive = true
    api
      .getMessageSteps(sessionId, msgId)
      .then((raw) => {
        if (!alive) return
        const parsed = JSON.parse(raw)
        if (Array.isArray(parsed)) setFullTurnSteps(parsed as TurnStep[])
      })
      .catch(() => {
        // Leave the trimmed patches in place; each one carries its own
        // "kırpılmış" badge, so the state stays honest without a modal-wide error.
      })
    return () => {
      alive = false
    }
  }, [turnTruncated, sessionId, msgId, fullTurnSteps])

  const turnChanges = useMemo(
    () => extractFileChanges(fullTurnSteps ?? turnSteps, { msgId }),
    [fullTurnSteps, turnSteps, msgId],
  )

  // The whole session's changes, fetched once when the tab is first opened. The
  // fetch is guarded by a ref rather than a loading flag so the effect body sets
  // no state — only its callbacks do.
  const [sessionChanges, setSessionChanges] = useState<FileChange[] | null>(null)
  const [sessionErr, setSessionErr] = useState<string | null>(null)
  const requested = useRef(false)
  useEffect(() => {
    if (scope !== 'session' || !sessionId || requested.current) return
    requested.current = true
    api
      .getSessionChanges(sessionId)
      .then((rows) => {
        const out: FileChange[] = []
        for (const row of rows) {
          const c = stepToFileChange(row.step, {
            msgId: row.msgId,
            at: row.createdAt,
            nested: row.nested,
          })
          if (c) out.push(c)
        }
        setSessionChanges(out)
      })
      .catch((e) => setSessionErr((e as Error).message))
  }, [scope, sessionId])
  const loadingSession = scope === 'session' && !sessionChanges && !sessionErr

  const changes = useMemo(
    () => (scope === 'turn' ? turnChanges : (sessionChanges ?? [])),
    [scope, turnChanges, sessionChanges],
  )
  const groups = useMemo(() => groupByPath(changes), [changes])

  // The selection is only a hint: a file picked in one scope may not exist in
  // the other, so the active group falls back to the first rather than being
  // corrected through state (which would re-render on every tab switch).
  const active = groups.find((g) => g.path === selected) ?? groups[0]

  const totals = groups.reduce(
    (acc, g) => ({ added: acc.added + g.added, removed: acc.removed + g.removed }),
    { added: 0, removed: 0 },
  )

  return (
    <ModalOverlay onClose={onClose}>
      <div className="flex h-[min(800px,88vh)] w-[min(1120px,95vw)] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] shadow-[var(--shadow-lg)]">
        <header className="flex shrink-0 items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5">
          <span className="text-sm font-semibold">Dosya değişiklikleri</span>
          <div className="flex rounded-md border border-[var(--color-border)] p-0.5 text-xs">
            <Tab active={scope === 'turn'} onClick={() => setScope('turn')}>
              Bu tur
            </Tab>
            <Tab
              active={scope === 'session'}
              onClick={() => setScope('session')}
              disabled={!sessionId}
              title={sessionId ? undefined : 'Bu görünümde oturum bağlamı yok'}
            >
              Tüm oturum
            </Tab>
          </div>
          <span className="text-xs text-[var(--color-text-dim)]">
            {groups.length} dosya
            {groups.length > 0 && (
              <>
                {' · '}
                <span className="text-[var(--color-success)]">+{totals.added}</span>{' '}
                <span className="text-[var(--color-danger)]">−{totals.removed}</span>
              </>
            )}
          </span>
          <button
            onClick={onClose}
            aria-label="Kapat"
            className="ml-auto text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
          >
            <X size={16} />
          </button>
        </header>

        <div className="flex min-h-0 flex-1">
          {/* Master: the changed files. */}
          <ul className="w-64 shrink-0 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] py-1 max-md:w-44">
            {groups.map((g) => (
              <li key={g.path}>
                <button
                  onClick={() => setSelected(g.path)}
                  title={g.path}
                  className={`flex w-full flex-col gap-0.5 px-3 py-1.5 text-left transition ${
                    active?.path === g.path
                      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span className="truncate font-mono text-[11px]">{shortPath(g.path, 2)}</span>
                  <span className="flex items-center gap-1.5 text-[10px]">
                    {g.added > 0 && <span className="text-[var(--color-success)]">+{g.added}</span>}
                    {g.removed > 0 && (
                      <span className="text-[var(--color-danger)]">−{g.removed}</span>
                    )}
                    {g.created && <Pill>yeni</Pill>}
                    {g.changes.length > 1 && <Pill>{g.changes.length}×</Pill>}
                    {g.changes.some((c) => c.nested) && <Pill>alt-ajan</Pill>}
                  </span>
                </button>
              </li>
            ))}
            {groups.length === 0 && (
              <li className="px-3 py-4 text-xs text-[var(--color-text-dim)]">
                {loadingSession
                  ? 'Yükleniyor…'
                  : sessionErr
                    ? `Alınamadı: ${sessionErr}`
                    : 'Dosya değişikliği yok.'}
              </li>
            )}
          </ul>

          {/* Detail: only the selected file's patches are mounted. */}
          <div className="flex min-w-0 flex-1 flex-col gap-2 overflow-y-auto p-3">
            {active && <FileDetail group={active} onOpenFile={onOpenFile} />}
          </div>
        </div>

        <footer className="shrink-0 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-[10px] text-[var(--color-text-dim)]">
          Gösterilen fark, değişikliğin yapıldığı andaki halidir — dosyanın şu anki içeriği değil.
        </footer>
      </div>
    </ModalOverlay>
  )
}

// FileDetail renders every change made to one file, in order. Each is its own
// patch: TionHarness records per-edit diffs and never the file's before/after
// content, so a merged patch cannot be computed — showing them stacked and
// labelled is the honest form.
function FileDetail({ group, onOpenFile }: { group: FileGroup; onOpenFile?: (p: string) => void }) {
  const joined = group.changes.map((c) => c.patch).join('\n')
  return (
    <>
      <div className="flex shrink-0 items-center gap-2 text-xs">
        <button
          onClick={() => onOpenFile?.(group.path)}
          title={group.path}
          className="min-w-0 truncate text-left font-mono text-[var(--color-accent)] underline decoration-dotted underline-offset-2 hover:opacity-80"
        >
          {group.path}
        </button>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <CopyButton text={joined} />
          <IconAction
            title="Yaması .patch olarak indir"
            onClick={() => downloadPatch(group.path, joined)}
          >
            <Download size={13} />
          </IconAction>
          {onOpenFile && (
            <IconAction title="Dosyayı aç" onClick={() => onOpenFile(group.path)}>
              <FileText size={13} />
            </IconAction>
          )}
        </div>
      </div>
      {group.changes.length > 1 && (
        <p className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
          Bu dosya {group.changes.length} kez değiştirildi. Aşağıdakiler sıralı değişikliklerdir,
          birleştirilmiş tek bir fark değildir.
        </p>
      )}
      {group.changes.map((c, i) => (
        <ChangeBlock key={i} change={c} index={i} total={group.changes.length} />
      ))}
    </>
  )
}

// A newly created file's "diff" is its entire content as added lines — the one
// case that reliably runs to thousands of rows without being interesting. It
// starts collapsed behind its line count; edits start open.
function ChangeBlock({
  change,
  index,
  total,
}: {
  change: FileChange
  index: number
  total: number
}) {
  const lineCount = change.patch ? change.patch.split('\n').length : 0
  const [open, setOpen] = useState(!change.created)

  return (
    <div className="flex min-h-0 flex-col gap-1">
      {(total > 1 || change.created || change.synthesized || change.truncated) && (
        <div className="flex flex-wrap items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]">
          {total > 1 && <span>#{index + 1}</span>}
          <span className="font-mono">{change.tool}</span>
          {change.created && <Pill>yeni dosya · {lineCount.toLocaleString('tr-TR')} satır</Pill>}
          {change.synthesized && (
            <Pill title="Bu farkı çevre bağlamı olmadan araç girdisinden ürettik (claude-cli düzenlemeyi kendisi uyguladı).">
              sentezlendi
            </Pill>
          )}
          {change.truncated && (
            <Pill title="Sunucu bu yamayı okuma yolunda kırptı; tam iz alınamadı.">kırpılmış</Pill>
          )}
        </div>
      )}
      {open ? (
        <div className="min-h-0" style={{ height: `${Math.min(560, 60 + lineCount * 18)}px` }}>
          <DiffView text={change.patch} variant="panel" />
        </div>
      ) : (
        <button
          onClick={() => setOpen(true)}
          className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-left text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          Yeni dosya · {lineCount.toLocaleString('tr-TR')} satır — içeriği göster
        </button>
      )}
    </div>
  )
}

function downloadPatch(path: string, patch: string) {
  const name = (path.split(/[\\/]/).pop() || 'changes') + '.patch'
  const url = URL.createObjectURL(new Blob([patch], { type: 'text/plain' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

function CopyButton({ text }: { text: string }) {
  return (
    <IconAction
      title="Yamayı panoya kopyala"
      onClick={() => {
        navigator.clipboard.writeText(text).then(() => toast.info('Panoya kopyalandı'))
      }}
    >
      <Copy size={13} />
    </IconAction>
  )
}

function IconAction({
  title,
  onClick,
  children,
}: {
  title: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      aria-label={title}
      className="rounded border border-[var(--color-border)] p-1 text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
    >
      {children}
    </button>
  )
}

function Pill({ children, title }: { children: React.ReactNode; title?: string }) {
  return (
    <span
      title={title}
      className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 text-[9px] text-[var(--color-text-dim)]"
    >
      {children}
    </span>
  )
}

function Tab({
  active,
  onClick,
  disabled,
  title,
  children,
}: {
  active: boolean
  onClick: () => void
  disabled?: boolean
  title?: string
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`rounded px-2 py-0.5 transition disabled:opacity-40 ${
        active
          ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
          : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
      }`}
    >
      {children}
    </button>
  )
}
