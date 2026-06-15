import { useEffect, useRef } from 'react'
import type { Message } from '../types'

interface Props {
  messages: Message[]
  pending: boolean
}

export function MessageList({ messages, pending }: Props) {
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, pending])

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="mx-auto flex max-w-3xl flex-col gap-4">
        {messages.map((m) => (
          <div
            key={m.id}
            className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <div
              className={`max-w-[80%] whitespace-pre-wrap rounded-2xl px-4 py-3 text-sm leading-relaxed ${
                m.role === 'user'
                  ? 'bg-[var(--color-accent)] text-white'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              }`}
            >
              {m.text}
            </div>
          </div>
        ))}

        {pending && (
          <div className="flex justify-start">
            <div className="rounded-2xl bg-[var(--color-surface-2)] px-4 py-3 text-sm text-[var(--color-text-dim)]">
              <span className="inline-flex gap-1">
                <span className="animate-bounce">●</span>
                <span className="animate-bounce [animation-delay:0.15s]">●</span>
                <span className="animate-bounce [animation-delay:0.3s]">●</span>
              </span>
            </div>
          </div>
        )}

        {messages.length === 0 && !pending && (
          <div className="mt-20 text-center text-[var(--color-text-dim)]">
            <p className="text-lg">Sohbete başla</p>
            <p className="mt-1 text-sm">Aşağıya bir mesaj yaz.</p>
          </div>
        )}

        <div ref={endRef} />
      </div>
    </div>
  )
}
