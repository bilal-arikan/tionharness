import { useEffect, useMemo, useState } from 'react'
import { Plus, Sparkles, X } from 'lucide-react'
import type { Skill, SkillSource } from '@/types'
import { api } from '@/api'

interface Props {
  // Controlled selection of skill slugs (unordered — selection is a set).
  selected: string[]
  onChange: (slugs: string[]) => void
  onError?: (msg: string) => void
}

const SOURCE_LABEL: Record<SkillSource, string> = {
  global: 'Global',
  workspace: 'Workspace',
}

// AgentSkillsSection lets the user pick which shared skills an agent gets.
// Selection is a set (no ordering) lifted to the parent form (saved with the
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

  // Shared (on-demand) skills are auto-available to every agent. They can still
  // be explicitly assigned (to pin them in the prompt in a chosen order); once
  // assigned they move into the selected list, so only the not-yet-picked ones
  // are offered here. Available-to-assign = restricted skills not yet picked.
  const shared = useMemo(
    () => all.filter((s) => s.shared && !selected.includes(s.slug)),
    [all, selected],
  )
  const available = useMemo(
    () => all.filter((s) => !s.shared && !selected.includes(s.slug)),
    [all, selected],
  )

  const add = (slug: string) => onChange([...selected, slug])
  const remove = (slug: string) => onChange(selected.filter((s) => s !== slug))

  return (
    <div className="border-t border-[var(--color-border)] pt-4">
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Skills
      </h3>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Bu ajana hangi <strong>kısıtlı</strong> skill'lerin verileceğini seç. Atanan skill'ler
        ajanın sistem promptunda görünür ve <code>use_skill</code> ile yüklenebilir.
        <strong> Gerektiğinde</strong> (paylaşımlı) skill'ler ise atama gerekmeden tüm ajanlara
        zaten açıktır. Skills ortak havuzdandır — <strong>Skills</strong> ekranından yönetilir.
      </p>

      {/* Selected (unordered set) — chips, mirroring the add pickers below. The
          whole chip is the remove control: click it to unassign (no separate X). */}
      {selected.length === 0 ? (
        <p className="mb-3 rounded border border-dashed border-[var(--color-border)] px-3 py-3 text-center text-xs text-[var(--color-text-dim)]">
          Henüz beceri seçilmedi. Aşağıdan ekle.
        </p>
      ) : (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {selected.map((slug) => {
            const sk = bySlug.get(slug)
            const missing = !sk
            return (
              <button
                key={slug}
                data-testid="skill-remove"
                data-skill-slug={slug}
                onClick={() => remove(slug)}
                title={
                  missing
                    ? `bulunamadı: ${slug} · Kaldırmak için tıkla`
                    : `${sk?.description ?? ''} · Kaldırmak için tıkla`
                }
                className={`group flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs transition hover:border-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_12%,var(--color-surface-2))] ${
                  missing
                    ? 'border-[var(--color-danger)]/40 bg-[var(--color-surface-2)] text-[var(--color-danger)]'
                    : 'border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-text)]'
                }`}
              >
                <span className="leading-none">{sk?.icon || '✨'}</span>
                <span className="max-w-40 truncate">{sk?.name || slug}</span>
                <X
                  size={12}
                  className="shrink-0 text-[var(--color-text-dim)] group-hover:text-[var(--color-danger)]"
                />
              </button>
            )
          })}
        </div>
      )}

      {/* Available to add */}
      {loaded && all.length === 0 && (
        <p className="flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
          <Sparkles size={13} className="opacity-50" /> Hiç skill yok. Önce <strong>Skills</strong>{' '}
          ekranından ekle.
        </p>
      )}
      {/* Restricted (must-be-assigned) skills: NOT available unless explicitly
          assigned to the agent. Clicking adds to the list. */}
      {available.length > 0 && (
        <div className="mt-3 rounded-lg border border-dashed border-[var(--color-border)] px-3 py-2">
          <p className="mb-1 text-[11px] font-medium text-[var(--color-accent)]">
            Atama gerektirir (atanmadan çalışmaz)
          </p>
          <p className="mb-1.5 text-[10px] text-[var(--color-text-dim)]">
            Bu beceriler yalnızca atandıkları ajanlara açıktır. Eklemeden ajan bu beceriyi
            <code className="mx-0.5">use_skill</code> ile yükleyemez.
          </p>
          <div className="flex flex-wrap gap-1.5">
            {available.map((s) => (
              <button
                key={s.slug}
                data-testid="skill-add-restricted"
                data-skill-slug={s.slug}
                onClick={() => add(s.slug)}
                title={`${s.description} · Atanmadan ajan bu beceriyi kullanamaz`}
                className="flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
              >
                <Plus size={12} className="text-[var(--color-text-dim)]" />
                <span>{s.icon || '✨'}</span>
                <span>{s.name}</span>
                <span className="text-[10px] text-[var(--color-text-dim)]">
                  · {SOURCE_LABEL[s.source]}
                </span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Shared (on-demand) skills: auto-available, but can still be explicitly
          assigned to pin them in the prompt order. Clicking adds to the list. */}
      {shared.length > 0 && (
        <div className="mt-3 rounded-lg border border-dashed border-[var(--color-border)] px-3 py-2">
          <p className="mb-1 text-[11px] font-medium text-[var(--color-success)]">
            Gerektiğinde açık (atama gerekmez)
          </p>
          <p className="mb-1.5 text-[10px] text-[var(--color-text-dim)]">
            Bu beceriler atama olmadan zaten açık. Yine de ekleyerek ajanın sistem promptunda
            sabitleyip sıralayabilirsin.
          </p>
          <div className="flex flex-wrap gap-1.5">
            {shared.map((s) => (
              <button
                key={s.slug}
                data-testid="skill-add-shared"
                data-skill-slug={s.slug}
                onClick={() => add(s.slug)}
                title={`${s.description} · Atayarak prompt sırasına sabitle`}
                className="flex items-center gap-1 rounded-full border border-[var(--color-success)]/40 bg-[var(--color-surface-2)] px-2.5 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-success)] hover:bg-[var(--color-bg)]"
              >
                <Plus size={12} className="text-[var(--color-success)]" />
                <span>{s.icon || '✨'}</span>
                <span>{s.name}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
