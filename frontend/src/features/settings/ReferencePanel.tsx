import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { PromptInfo, SlashCommand } from '@/types'
import { CommandsPanel } from './CommandsPanel'
import { InfoPopover } from '@/shared/components/InfoPopover'
import { StepKindsIntro, StepKindsPanel } from './StepKindsPanel'
import { useTranslation } from 'react-i18next'

export function ReferencePanel({
  commands,
  initialTab = 'commands',
}: {
  commands: SlashCommand[]
  initialTab?: 'commands' | 'stepkinds'
}) {
  const { t } = useTranslation('settings')
  const [tab, setTab] = useState(initialTab)
  const [prompts, setPrompts] = useState<PromptInfo[]>([])
  const [promptsDir, setPromptsDir] = useState('')
  const [openCmds, setOpenCmds] = useState<Record<string, boolean>>({})
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    api
      .getPrompts()
      .then((result) => {
        if (cancelled) return
        setPrompts(result.prompts)
        setPromptsDir(result.dir)
      })
      .catch(() => {
        if (!cancelled) setError(t('reference.loadError'))
      })
    return () => {
      cancelled = true
    }
  }, [t])

  return (
    <>
      <div aria-label={t('reference.sectionsAria')} className="flex items-center gap-2">
        {(['commands', 'stepkinds'] as const).map((key) => (
          <button
            key={key}
            aria-pressed={tab === key}
            onClick={() => setTab(key)}
            className={`rounded px-3 py-2 text-sm ${tab === key ? 'bg-[var(--color-accent-soft)]' : 'bg-[var(--color-surface-2)]'}`}
          >
            {key === 'commands' ? t('reference.commands') : t('reference.stepKinds')}
          </button>
        ))}
        {/* One (ⓘ) for the page and, on the step-kinds tab, that list's intro. */}
        <InfoPopover
          text={
            <>
              {t('reference.description')}
              {tab === 'stepkinds' && (
                <span className="mt-1.5 block">
                  <StepKindsIntro />
                </span>
              )}
            </>
          }
        />
      </div>
      <div className="space-y-4">
        {tab === 'commands' ? (
          <>
            {error && <p role="alert">{error}</p>}
            <CommandsPanel
              commands={commands}
              prompts={prompts}
              promptsDir={promptsDir}
              openCmds={openCmds}
              setOpenCmds={setOpenCmds}
            />
          </>
        ) : (
          <StepKindsPanel />
        )}
      </div>
    </>
  )
}
