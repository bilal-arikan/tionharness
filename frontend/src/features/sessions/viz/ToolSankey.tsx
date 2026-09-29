import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { MermaidDiagram } from '@/shared/components/markdown/MermaidDiagram'
import { buildToolSankey } from './flowVizData'
import { useTranslation } from 'react-i18next'

// ToolSankey renders the tool-execution Sankey for a session: Agent → Tool
// (→ Tamam | Hata when failures exist), sized by call count. It reuses the app's
// MermaidDiagram (lazy mermaid, theme-aware, expandable) with a `sankey-beta`
// source built from the raw debug events.
export function ToolSankey({
  events,
  agentNames,
}: {
  events: SessionDebugEvent[]
  agentNames: Record<string, string>
}) {
  const { t } = useTranslation('sessions')
  const code = useMemo(() => buildToolSankey(events, agentNames), [events, agentNames])
  if (!code) {
    return (
      <p className="py-2 text-[11px] text-[var(--color-text-dim)]">
        {t('visualization.sankey.empty')}
      </p>
    )
  }
  return <MermaidDiagram code={code} />
}
