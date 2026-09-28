import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { PromptInfo, SlashCommand } from '@/types'
import { CommandsPanel } from './CommandsPanel'
import { StepKindsPanel } from './StepKindsPanel'

export function ReferencePanel({
  commands,
  initialTab = 'commands',
}: {
  commands: SlashCommand[]
  initialTab?: 'commands' | 'stepkinds'
}) {
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
        if (!cancelled) setError('Prompt details could not be loaded.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Read-only help. There are no settings to save here.
      </p>
      <div aria-label="Reference sections" className="flex gap-2">
        {(['commands', 'stepkinds'] as const).map((key) => (
          <button
            key={key}
            aria-pressed={tab === key}
            onClick={() => setTab(key)}
            className={`rounded px-3 py-2 text-sm ${tab === key ? 'bg-[var(--color-accent-soft)]' : 'bg-[var(--color-surface-2)]'}`}
          >
            {key === 'commands' ? 'Commands' : 'Activity step types'}
          </button>
        ))}
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
