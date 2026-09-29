// DependencyPicker lets the user select other tasks as dependencies for the
// current task. Selected dependencies are shown as chips below the picker.
import type { Task, BoardState } from '@/types'
import { useTranslation } from 'react-i18next'

interface Props {
  // All tasks available for selection (current task should be excluded by caller).
  tasks: Task[]
  // Currently selected dependency IDs.
  value: string[]
  onChange: (ids: string[]) => void
}

export function DependencyPicker({ tasks, value, onChange }: Props) {
  const { t: translate } = useTranslation('tasks')
  const toggle = (id: string) => {
    if (value.includes(id)) {
      onChange(value.filter((v) => v !== id))
    } else {
      onChange([...value, id])
    }
  }

  if (tasks.length === 0) {
    return <p className="text-xs text-[var(--color-text-dim)]">{translate('dependencies.empty')}</p>
  }

  return (
    <div className="flex max-h-44 flex-col gap-0.5 overflow-y-auto rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1 py-1">
      {tasks.map((t) => {
        const selected = value.includes(t.id)
        const done = t.boardState === 'done'
        return (
          <label
            key={t.id}
            className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
          >
            <input
              type="checkbox"
              checked={selected}
              onChange={() => toggle(t.id)}
              className="accent-[var(--color-accent)] shrink-0"
            />
            <span className="flex-1 truncate text-xs text-[var(--color-text)]">
              {t.title || t.description || t.id}
            </span>
            <span
              className={`ml-auto shrink-0 text-[10px] ${done ? 'text-[var(--color-success)]' : 'text-[var(--color-text-dim)]'}`}
            >
              {translate(`boardStates.${t.boardState as BoardState}`, {
                defaultValue: t.boardState,
              })}
            </span>
          </label>
        )
      })}
    </div>
  )
}
