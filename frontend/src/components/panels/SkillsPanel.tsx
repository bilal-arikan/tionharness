import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Sparkles } from 'lucide-react'
import type { Skill, SkillDetail, SkillSource } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'

interface Props {
  onError: (msg: string) => void
}

// Tier badge styling — project (highest priority) is accented, then workspace,
// then global. Mirrors the override order on the backend.
const SOURCE_LABEL: Record<SkillSource, string> = {
  global: 'Global',
  workspace: 'Workspace',
  project: 'Proje',
}

function SourceBadge({ source }: { source: SkillSource }) {
  const tone =
    source === 'project'
      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${tone}`}>
      {SOURCE_LABEL[source]}
    </span>
  )
}

// SkillsPanel is the two-panel Skills screen: a list of resolved skills on the
// left, the selected skill's full instructions (loaded on demand) on the right.
export function SkillsPanel({ onError }: Props) {
  const [list, setList] = useState<Skill[]>([])
  const [activeSlug, setActiveSlug] = useState<string | null>(null)
  const [active, setActive] = useState<SkillDetail | null>(null)
  const [loadingBody, setLoadingBody] = useState(false)

  const reload = useCallback(() => {
    api
      .listSkills()
      .then((rows) => {
        setList(rows)
        setActiveSlug((cur) => cur ?? rows[0]?.slug ?? null)
      })
      .catch((e) => onError((e as Error).message))
  }, [onError])

  useEffect(() => reload(), [reload])

  // Load the selected skill's full body lazily when the selection changes.
  useEffect(() => {
    if (!activeSlug) {
      setActive(null)
      return
    }
    setLoadingBody(true)
    api
      .getSkill(activeSlug)
      .then(setActive)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoadingBody(false))
  }, [activeSlug, onError])

  // Re-scan tiers on disk, then refresh the catalog + current selection.
  const rescan = useCallback(() => {
    api
      .reloadSkills()
      .then(() => {
        reload()
        if (activeSlug) api.getSkill(activeSlug).then(setActive).catch(() => {})
      })
      .catch((e) => onError((e as Error).message))
  }, [reload, activeSlug, onError])

  return (
    <div className="flex min-h-0 flex-1">
      {/* List */}
      <div className="flex w-72 flex-shrink-0 flex-col border-r border-[var(--color-border)]">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Beceriler · {list.length}
          </span>
          <button
            onClick={rescan}
            title="Diskten yeniden tara"
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <RefreshCw size={13} /> Tara
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {list.length === 0 && (
            <div className="flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)]">
              <Sparkles size={28} className="opacity-40" />
              <p>
                Henüz beceri yok. <code>SKILL.md</code> içeren bir klasörü{' '}
                <code>~/.agents/skills/</code>, workspace <code>skills/</code> ya da proje{' '}
                <code>.agents/skills/</code> altına koyup <strong>Tara</strong>'ya bas.
              </p>
            </div>
          )}
          {list.map((sk) => {
            const isActive = sk.slug === activeSlug
            return (
              <button
                key={sk.slug}
                onClick={() => setActiveSlug(sk.slug)}
                className={`group mb-1 flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                  isActive
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="mt-0.5 shrink-0 text-base leading-none">{sk.icon || '✨'}</span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5">
                    <span className="min-w-0 flex-1 truncate font-medium">{sk.name}</span>
                    <SourceBadge source={sk.source} />
                  </span>
                  <span className="mt-0.5 block truncate text-[11px] text-[var(--color-text-dim)]">
                    {sk.description || sk.slug}
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Detail */}
      <div className="flex min-w-0 flex-1 flex-col">
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            {loadingBody ? 'Yükleniyor…' : 'Görüntülemek için bir beceri seç.'}
          </div>
        ) : (
          <>
            <div className="flex items-start justify-between gap-3 border-b border-[var(--color-border)] px-5 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-lg leading-none">{active.icon || '✨'}</span>
                  <h2 className="truncate text-base font-semibold">{active.name}</h2>
                  <SourceBadge source={active.source} />
                </div>
                <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                  <code>{active.slug}</code>
                  {active.description ? ` · ${active.description}` : ''}
                </p>
                {active.whenToUse && (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">
                    <span className="font-medium">Ne zaman:</span> {active.whenToUse}
                  </p>
                )}
                {active.alwaysAllow && active.alwaysAllow.length > 0 && (
                  <p className="mt-1 flex flex-wrap items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                    <span className="font-medium">İzinli araçlar:</span>
                    {active.alwaysAllow.map((tool) => (
                      <code
                        key={tool}
                        className="rounded bg-[var(--color-surface-2)] px-1 py-0.5"
                      >
                        {tool}
                      </code>
                    ))}
                  </p>
                )}
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-5">
              {active.body ? (
                <Markdown>{active.body}</Markdown>
              ) : (
                <p className="text-sm text-[var(--color-text-dim)]">Bu becerinin gövde içeriği yok.</p>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
