import { useEffect, useState } from 'react'
import { BookOpen, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from './InfoPopover'
import type { Skill } from '@/types'

// The coordination-recipe picker, shared by the session info panel (a live
// coordinator session) and the flow editor's coordinator node — the same list in
// both places, so a pattern learned in one is selectable in the other.

// What a coordinator workflow IS — the picker offers slugs with no explanation of
// the concept, which is the gap this note fills.
export const WORKFLOW_HELP =
  "Kayıtlı bir orkestrasyon reçetesi: koordinatör prompt'unun üzerine bindirilir, " +
  "worker'ların hangi sırayla başlayıp sonuçlarının nasıl birleşeceğini tarif eder.\n\n" +
  'Her reçete bir skill dosyasıdır (kind: coordinator-workflow) — satırdaki 📖 ile ' +
  'içeriğini açabilirsin. "Serbest" seçilirse koordinatör kendi kararıyla ilerler.'

// Human labels for the orchestration strategies a recipe can encode, so the
// per-recipe note explains the pattern instead of echoing a raw slug.
const PATTERN_LABEL: Record<string, string> = {
  fanout: "Fan-out & sentez — paralel worker'lar, sonuçları koordinatör birleştirir",
  adversarial: 'Karşıt doğrulama — bir üretici, bir de onu çürütmeye çalışan worker',
  loop: 'Bitene kadar döngü — durma koşulu sağlanana dek tur tekrarlanır',
  classify: 'Sınıflandır & yönlendir — girdi önce etiketlenir, sonra ilgili profile gider',
  'generate-filter': 'Üret & süz — çok sayıda aday üretilir, sonra elenir',
  tournament: 'Turnuva — adaylar ikişerli karşılaştırılarak eleme yapılır',
}

// recipeHelp composes one recipe's popover note from its frontmatter. Everything
// here is optional in the skill file, so each line is emitted only when present —
// a recipe with nothing but a name still gets a note rather than an empty bubble.
function recipeHelp(r: Skill): string {
  const lines: string[] = []
  if (r.description) lines.push(r.description)
  const pattern = r.pattern ? (PATTERN_LABEL[r.pattern] ?? r.pattern) : ''
  if (pattern) lines.push(`Desen: ${pattern}`)
  if (r.workerTargets?.length) lines.push(`Önerilen worker'lar: ${r.workerTargets.join(', ')}`)
  if (r.stopCondition) lines.push(`Durma koşulu: ${r.stopCondition}`)
  if (r.maxTurns) lines.push(`Tur üst sınırı: ${r.maxTurns}`)
  lines.push(`Skill: ${r.slug}`)
  return lines.join('\n\n')
}

// recipeRowClass renders a picker row; the selected one gets the accent tint that
// a native <option> highlight used to provide.
function recipeRowClass(selected: boolean): string {
  return [
    'flex cursor-pointer items-center gap-1.5 rounded-md px-1.5 py-1 text-[11px] transition',
    selected
      ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
      : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]',
  ].join(' ')
}

interface Props {
  // Currently selected recipe slug ('' / undefined = free coordination).
  value?: string
  onChange: (slug: string) => void
  // Disables every row (e.g. while a save is in flight).
  disabled?: boolean
  // Radio-group name; must be unique per rendered picker on a page.
  groupName: string
  // Opens the Skills screen on a slug. A recipe IS a skill, so this is how the
  // picker links to its actual instructions. Optional — the link hides when absent.
  onOpenSkill?: (slug: string) => void
}

// CoordinatorWorkflowPicker lists the workspace's coordinator-workflow skills as
// a radio group. A radio LIST rather than a <select>: an <option> cannot carry
// the per-recipe (ⓘ) and "open skill" buttons, and picking a recipe blind — by
// name alone — is the thing those buttons fix. Native radios keep arrow-key and
// screen-reader behaviour for free.
export function CoordinatorWorkflowPicker({
  value,
  onChange,
  disabled,
  groupName,
  onOpenSkill,
}: Props) {
  const [recipes, setRecipes] = useState<Skill[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    api
      .listSkills()
      .then((all) => {
        if (alive) setRecipes(all.filter((s) => s.kind === 'coordinator-workflow'))
      })
      .catch(() => {
        if (alive) setRecipes([])
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [])

  return (
    <div role="radiogroup" aria-label="Workflow" className="space-y-0.5">
      <label className={recipeRowClass(!value)}>
        <input
          type="radio"
          name={groupName}
          checked={!value}
          onChange={() => onChange('')}
          disabled={disabled}
          className="accent-[var(--color-accent)]"
        />
        <span className="truncate">Serbest (recipe yok)</span>
      </label>
      {loading && (
        <div className="flex items-center gap-1.5 px-1.5 py-1 text-[11px] text-[var(--color-text-dim)]">
          <Loader2 size={11} className="animate-spin" /> Reçeteler yükleniyor…
        </div>
      )}
      {!loading && recipes.length === 0 && (
        <div className="px-1.5 py-1 text-[11px] text-[var(--color-text-dim)]">
          Kayıtlı reçete yok — Skills ekranından <code>kind: coordinator-workflow</code> bir skill
          ekleyin.
        </div>
      )}
      {recipes.map((r) => (
        <div key={r.slug} className="flex items-center gap-0.5">
          <label className={`min-w-0 flex-1 ${recipeRowClass(value === r.slug)}`}>
            <input
              type="radio"
              name={groupName}
              checked={value === r.slug}
              onChange={() => onChange(r.slug)}
              disabled={disabled}
              className="accent-[var(--color-accent)]"
            />
            <span className="truncate">
              {r.icon ? `${r.icon} ` : ''}
              {r.name}
            </span>
          </label>
          <InfoPopover text={recipeHelp(r)} label={`${r.name} — ne yapar?`} fixed />
          {/* The recipe's own skill file — where its orchestration instructions live. */}
          {onOpenSkill && (
            <button
              type="button"
              onClick={() => onOpenSkill(r.slug)}
              title={`"${r.name}" skill'ini Skills ekranında aç`}
              aria-label={`${r.name} skill'ini aç`}
              className="inline-flex h-4 w-4 shrink-0 items-center justify-center text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <BookOpen size={12} />
            </button>
          )}
        </div>
      ))}
    </div>
  )
}
