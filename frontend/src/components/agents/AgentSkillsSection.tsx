import { useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, Plus, Sparkles, X } from 'lucide-react'
import type { Skill, SkillSource } from '../../types'
import { api } from '../../api'

interface Props {
  // Ordered, controlled selection of skill slugs.
  selected: string[]
  onChange: (slugs: string[]) => void
  onError?: (msg: string) => void
}

const SOURCE_LABEL: Record<SkillSource, string> = {
  global: 'Global',
  workspace: 'Workspace',
  project: 'Proje',
}

// AgentSkillsSection lets the user pick which shared skills an agent gets and in
// what order. Selection + order is lifted to the parent form (saved with the
// rest of the profile). Skills are a shared library — this only selects, it
// never authors per-agent skills.
export function AgentSkillsSection({ selected, onChange, onError }: Props) {
  const [all, setAll] = useState<Skill[]>([])
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    api
      .listSkills()
      .then(setAll)
      .catch((e) => onError?.((e as Error).message))
      .finally(() => setLoaded(true))
  }, [onError])

  const bySlug = useMemo(() => {
    const m = new Map<string, Skill>()
    for (const s of all) m.set(s.slug, s)
    return m
  }, [all])

  // Shared (on-demand) skills are auto-available to every agent — shown as info,
  // not in the add list. Available-to-assign = restricted skills not yet picked.
  const shared = useMemo(() => all.filter((s) => s.shared), [all])
  const available = useMemo(
    () => all.filter((s) => !s.shared && !selected.includes(s.slug)),
    [all, selected],
  )

  const add = (slug: string) => onChange([...selected, slug])
  const remove = (slug: string) => onChange(selected.filter((s) => s !== slug))
  const move = (i: number, dir: -1 | 1) => {
    const j = i + dir
    if (j < 0 || j >= selected.length) return
    const next = [...selected]
    ;[next[i], next[j]] = [next[j], next[i]]
    onChange(next)
  }

  return (
    <div className="border-t border-[var(--color-border)] pt-4">
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Beceriler (skills)
      </h3>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Bu ajana hangi <strong>kısıtlı</strong> becerilerin verileceğini seç ve sırala. Atanan beceriler
        ajanın sistem promptunda (bu sırayla) görünür ve <code>use_skill</code> ile yüklenebilir.
        <strong> Gerektiğinde</strong> (paylaşımlı) beceriler ise atama gerekmeden tüm ajanlara zaten
        açıktır. Beceriler ortak havuzdandır — <strong>Beceriler</strong> ekranından yönetilir.
      </p>

      {/* Selected (ordered) */}
      {selected.length === 0 ? (
        <p className="mb-3 rounded border border-dashed border-[var(--color-border)] px-3 py-3 text-center text-xs text-[var(--color-text-dim)]">
          Henüz beceri seçilmedi. Aşağıdan ekle.
        </p>
      ) : (
        <ol className="mb-3 space-y-1">
          {selected.map((slug, i) => {
            const sk = bySlug.get(slug)
            return (
              <li
                key={slug}
                className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5"
              >
                <span className="w-4 shrink-0 text-center text-[11px] text-[var(--color-text-dim)]">
                  {i + 1}
                </span>
                <span className="shrink-0 text-base leading-none">{sk?.icon || '✨'}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{sk?.name || slug}</span>
                  {!sk && (
                    <span className="block truncate text-[11px] text-[var(--color-danger)]">
                      bulunamadı: {slug}
                    </span>
                  )}
                </span>
                <div className="flex shrink-0 items-center gap-0.5">
                  <button
                    onClick={() => move(i, -1)}
                    disabled={i === 0}
                    title="Yukarı"
                    className="rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-bg)] hover:text-[var(--color-text)] disabled:opacity-30"
                  >
                    <ChevronUp size={14} />
                  </button>
                  <button
                    onClick={() => move(i, 1)}
                    disabled={i === selected.length - 1}
                    title="Aşağı"
                    className="rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-bg)] hover:text-[var(--color-text)] disabled:opacity-30"
                  >
                    <ChevronDown size={14} />
                  </button>
                  <button
                    onClick={() => remove(slug)}
                    title="Kaldır"
                    className="rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-bg)] hover:text-[var(--color-danger)]"
                  >
                    <X size={14} />
                  </button>
                </div>
              </li>
            )
          })}
        </ol>
      )}

      {/* Available to add */}
      {loaded && all.length === 0 && (
        <p className="flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
          <Sparkles size={13} className="opacity-50" /> Hiç beceri yok. Önce{' '}
          <strong>Beceriler</strong> ekranından ekle.
        </p>
      )}
      {available.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {available.map((s) => (
            <button
              key={s.slug}
              onClick={() => add(s.slug)}
              title={s.description}
              className="flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
            >
              <Plus size={12} className="text-[var(--color-text-dim)]" />
              <span>{s.icon || '✨'}</span>
              <span>{s.name}</span>
              <span className="text-[10px] text-[var(--color-text-dim)]">· {SOURCE_LABEL[s.source]}</span>
            </button>
          ))}
        </div>
      )}

      {/* Shared (on-demand) skills: auto-available, shown for awareness only. */}
      {shared.length > 0 && (
        <div className="mt-3 rounded-lg border border-dashed border-[var(--color-border)] px-3 py-2">
          <p className="mb-1.5 text-[11px] font-medium text-[var(--color-success)]">
            Gerektiğinde açık (atama gerekmez)
          </p>
          <div className="flex flex-wrap gap-1.5">
            {shared.map((s) => (
              <span
                key={s.slug}
                title={s.description}
                className="flex items-center gap-1 rounded-full bg-[var(--color-surface-2)] px-2.5 py-1 text-xs text-[var(--color-text-dim)]"
              >
                <span>{s.icon || '✨'}</span>
                <span>{s.name}</span>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
