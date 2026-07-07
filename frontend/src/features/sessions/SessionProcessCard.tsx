import { Loader2, Square, RotateCcw, Flame } from 'lucide-react'
import type { SessionInfo } from '@/types'
import { ProcBtn } from './SessionDetailBits'
import { formatElapsed } from './sessionDetailFormat'

interface Props {
  info: SessionInfo
  nowTick: number
  procBusy: '' | 'stop' | 'restart' | 'drop'
  onStop: () => void
  onRestart: () => void
  onDrop: () => void
  // Presence gates the restart button (mirrors the panel's onRerun prop).
  hasRerun: boolean
}

// Background process: an in-flight turn and/or a warm persistent CLI
// process for this session — with stop / restart / recycle controls.
export function SessionProcessCard({ info, nowTick, procBusy, onStop, onRestart, onDrop, hasRerun }: Props) {
  return (
    <section className="flex flex-col gap-2 rounded-lg border border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-2.5 py-2">
      {info.running && (
        <>
          <div className="flex items-center gap-1.5 text-[11px]">
            <Loader2 size={13} className="shrink-0 animate-spin text-[var(--color-accent)]" />
            <span className="font-medium text-[var(--color-text)]">
              {info.running.autonomous ? 'Otonom tur çalışıyor' : 'Tur çalışıyor'}
            </span>
            {info.running.provider && (
              <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)]">
                {info.running.provider}
              </span>
            )}
            <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
              {formatElapsed(Math.max(0, nowTick - info.running.startedAt))}
            </span>
          </div>
          <div className="flex items-center gap-1.5">
            <ProcBtn
              icon={Square}
              label="Durdur"
              onClick={onStop}
              busy={procBusy === 'stop'}
              disabled={procBusy !== ''}
            />
            {hasRerun && (
              <ProcBtn
                icon={RotateCcw}
                label="Yeniden başlat"
                onClick={onRestart}
                busy={procBusy === 'restart'}
                disabled={procBusy !== ''}
              />
            )}
          </div>
        </>
      )}
      {info.warmCliProcess && (
        <div className="flex items-center gap-1.5">
          <Flame size={13} className="shrink-0 text-[var(--color-warning)]" />
          <span className="min-w-0 flex-1 text-[11px] text-[var(--color-text-dim)]">
            Sıcak claude-cli süreci (turlar arası)
          </span>
          <ProcBtn
            icon={RotateCcw}
            label="Tazele"
            onClick={onDrop}
            busy={procBusy === 'drop'}
            disabled={procBusy !== ''}
            title="Sıcak süreci kapat — sonraki tur temiz başlar (konuşma korunur)"
          />
        </div>
      )}
    </section>
  )
}
