import { useEffect, useState } from 'react'
import { Flame, Snowflake } from 'lucide-react'
import type { Message } from '@/types'
import { serverNow } from '@/shared/lib/serverClock'
import { cacheRemaining, formatCountdown } from '@/features/sessions/sessionDetailFormat'

interface Props {
  messages: Message[]
}

// Warn (amber) once the warm window is this short: still usable, but a reply
// written now is worth sending before the prefix cools.
const SOON_SEC = 5 * 60

// CacheWarmthStrip sits above the composer and is the only PREVENTIVE cache
// surface in the chat: it shows whether the session's prompt cache is still warm
// and how long is left, so the user can choose to answer now rather than pay a
// full cold prefix later. Everything else about caching in the chat reports a
// cost already incurred.
//
// The warm window starts at the last turn, which the transcript already knows —
// the newest message's timestamp. That is the same clock the backend's 1h TTL
// runs on (providers.cacheTTL), so no backend field is needed; it is a
// approximation only in that the LLM call happens slightly before the reply is
// persisted, which shortens the shown window by seconds, never lengthens it.
export function CacheWarmthStrip({ messages }: Props) {
  const [now, setNow] = useState(() => serverNow())
  const last = messages[messages.length - 1]
  const lastAt = last?.createdAt ?? 0
  const remaining = cacheRemaining(lastAt, now)
  const warm = remaining > 0

  // Tick only while there is something to count down. Once cold the label is
  // static, so a running interval would just wake the render loop forever.
  useEffect(() => {
    if (!warm) return
    const id = setInterval(() => setNow(serverNow()), 1000)
    return () => clearInterval(id)
  }, [warm])

  // Cold turns this session: an assistant reply that (re)wrote the cached prefix
  // without reading it. The FIRST assistant turn is skipped — its cold prefix is
  // the unavoidable price of starting a session, not a lost cache. Mirrors the
  // backend detector, which likewise only flags a break after the cache was warm.
  const coldTurns = countColdTurns(messages)

  if (!lastAt) return null

  return (
    <div className="flex justify-center pb-1">
      <span
        className="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-0.5 text-[10px] shadow-[var(--shadow-sm)]"
        title={
          warm
            ? "Prompt cache sıcak: şimdi gönderilen tur cache'li öneki okur (ucuz). Süre dolunca önek baştan yazılır."
            : 'Prompt cache soğuk (1sa TTL doldu) — sonraki tur öneki baştan öder. Bekleyen bağlam değişiklikleri de bu turda bedavaya adopte edilir.'
        }
      >
        {warm ? (
          <>
            <Flame size={11} className="shrink-0 text-[var(--color-warning)]" />
            <span
              className={
                remaining <= SOON_SEC
                  ? 'font-mono text-[var(--color-warning)]'
                  : 'font-mono text-[var(--color-text-dim)]'
              }
            >
              Cache sıcak · {formatCountdown(remaining)}
            </span>
          </>
        ) : (
          <>
            <Snowflake size={11} className="shrink-0 text-[var(--color-text-dim)]" />
            <span className="text-[var(--color-text-dim)]">Cache soğuk</span>
          </>
        )}
        {coldTurns > 0 && (
          <span
            className="text-[var(--color-text-dim)] opacity-70"
            title="Bu oturumda cache önekini baştan ödeyen tur sayısı (ilk tur hariç). Sebebi için o mesajın debug panelini aç."
          >
            · {coldTurns} soğuk tur
          </span>
        )}
      </span>
    </div>
  )
}

// countColdTurns counts assistant replies that paid for the prefix (cacheWrite)
// without reading it, skipping the session's first assistant turn. Turns with no
// cache counters at all are not counted — OpenRouter bills a cold prefix as plain
// input and reports no write, so counting them would need a guess.
function countColdTurns(messages: Message[]): number {
  let seenAssistant = false
  let cold = 0
  for (const m of messages) {
    if (m.role === 'user' || !m.usage) continue
    if (!seenAssistant) {
      seenAssistant = true
      continue
    }
    if (!m.usage.cacheRead && (m.usage.cacheWrite ?? 0) > 0) cold++
  }
  return cold
}
