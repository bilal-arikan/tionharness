import { useEffect, useState } from 'react'
import { BookOpen, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from './InfoPopover'
import type { Skill } from '@/types'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'

// The coordination-recipe picker, shared by the session info panel (a live
// coordinator session) and the flow editor's coordinator node — the same list in
// both places, so a pattern learned in one is selectable in the other.

// Human labels for the orchestration strategies a recipe can encode, so the
// per-recipe note explains the pattern instead of echoing a raw slug.
const PATTERN_KEY: Record<string, string> = {
  fanout: 'workflow.patterns.fanout',
  adversarial: 'workflow.patterns.adversarial',
  loop: 'workflow.patterns.loop',
  classify: 'workflow.patterns.classify',
  'generate-filter': 'workflow.patterns.generateFilter',
  tournament: 'workflow.patterns.tournament',
}

const ADVANCED_PATTERNS = new Set(['classify', 'loop', 'tournament'])

// recipeHelp composes one recipe's popover note from its frontmatter. Everything
// here is optional in the skill file, so each line is emitted only when present —
// a recipe with nothing but a name still gets a note rather than an empty bubble.
function recipeHelp(t: TFunction<'sharedUi'>, r: Skill): string {
  const lines: string[] = []
  if (r.description) lines.push(r.description)
  const pattern = r.pattern ? (PATTERN_KEY[r.pattern] ? t(PATTERN_KEY[r.pattern]) : r.pattern) : ''
  if (pattern) lines.push(t('workflow.details.pattern', { pattern }))
  if (r.workerTargets?.length)
    lines.push(t('workflow.details.workers', { workers: r.workerTargets.join(', ') }))
  if (r.stopCondition)
    lines.push(t('workflow.details.stopCondition', { condition: r.stopCondition }))
  if (r.maxTurns) lines.push(t('workflow.details.maxTurns', { maxTurns: r.maxTurns }))
  // Structured plan (R6): the phases a trajectory will be seeded with, or the
  // reason the block was rejected (the recipe still works as prose).
  if (r.recipe?.phases?.length) {
    const phases = r.recipe.phases
      .map((p) => (p.profile ? `${p.id} (${p.profile})` : p.id) + (p.optional ? '?' : ''))
      .join(' → ')
    lines.push(t('workflow.details.phases', { phases }))
  }
  if (r.recipeError) lines.push(t('workflow.details.invalidPhases', { error: r.recipeError }))
  lines.push(
    r.version
      ? t('workflow.details.skillVersion', { slug: r.slug, version: r.version })
      : t('workflow.details.skill', { slug: r.slug }),
  )
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
  const { t } = useTranslation('sharedUi')
  const [recipes, setRecipes] = useState<Skill[]>([])
  const [loading, setLoading] = useState(true)

  const standardRecipes = recipes.filter((recipe) => !ADVANCED_PATTERNS.has(recipe.pattern ?? ''))
  const advancedRecipes = recipes.filter((recipe) => ADVANCED_PATTERNS.has(recipe.pattern ?? ''))
  const selectedAdvanced = advancedRecipes.some((recipe) => recipe.slug === value)

  useEffect(() => {
    let alive = true
    api
      .listSkills()
      .then((all) => {
        // Archived recipes are refused by the backend, so never offer them.
        if (alive) setRecipes(all.filter((s) => s.kind === 'coordinator-workflow' && !s.archived))
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
    <div role="radiogroup" aria-label={t('workflow.label')} className="space-y-0.5">
      <label className={recipeRowClass(!value)}>
        <input
          type="radio"
          name={groupName}
          checked={!value}
          onChange={() => onChange('')}
          disabled={disabled}
          className="accent-[var(--color-accent)]"
        />
        <span className="truncate">{t('workflow.free')}</span>
      </label>
      {loading && (
        <div className="flex items-center gap-1.5 px-1.5 py-1 text-[11px] text-[var(--color-text-dim)]">
          <Loader2 size={11} className="animate-spin" /> {t('workflow.loading')}
        </div>
      )}
      {!loading && recipes.length === 0 && (
        <div className="px-1.5 py-1 text-[11px] text-[var(--color-text-dim)]">
          {t('workflow.emptyBefore')} <code>kind: coordinator-workflow</code>{' '}
          {t('workflow.emptyAfter')}
        </div>
      )}
      {standardRecipes.map((r) => (
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
          <InfoPopover
            text={recipeHelp(t, r)}
            label={t('workflow.whatDoesItDo', { name: r.name })}
            fixed
          />
          {/* The recipe's own skill file — where its orchestration instructions live. */}
          {onOpenSkill && (
            <button
              type="button"
              onClick={() => onOpenSkill(r.slug)}
              title={t('workflow.openInSkills', { name: r.name })}
              aria-label={t('workflow.openSkill', { name: r.name })}
              className="inline-flex h-4 w-4 shrink-0 items-center justify-center text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <BookOpen size={12} />
            </button>
          )}
        </div>
      ))}
      {advancedRecipes.length > 0 && (
        <details open={selectedAdvanced} className="pt-0.5">
          <summary className="cursor-pointer px-1.5 py-1 text-[11px] text-[var(--color-text-dim)]">
            {t('workflow.advanced')}
          </summary>
          <div className="space-y-0.5 pl-2">
            {advancedRecipes.map((r) => (
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
                <InfoPopover
                  text={recipeHelp(t, r)}
                  label={t('workflow.whatDoesItDo', { name: r.name })}
                  fixed
                />
                {onOpenSkill && (
                  <button
                    type="button"
                    onClick={() => onOpenSkill(r.slug)}
                    title={t('workflow.openInSkills', { name: r.name })}
                    aria-label={t('workflow.openSkill', { name: r.name })}
                    className="inline-flex h-4 w-4 shrink-0 items-center justify-center text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                  >
                    <BookOpen size={12} />
                  </button>
                )}
              </div>
            ))}
          </div>
        </details>
      )}
    </div>
  )
}
