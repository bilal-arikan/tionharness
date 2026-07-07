import type { HTMLAttributes } from 'react'

// Card — a bordered surface container. The default sits on --color-bg; pass
// surface to lift it onto --color-surface for nested/elevated content.
interface Props extends HTMLAttributes<HTMLDivElement> {
  surface?: boolean
}

export function Card({ surface = false, className = '', ...props }: Props) {
  const bg = surface ? 'bg-[var(--color-surface)]' : 'bg-[var(--color-bg)]'
  return (
    <div
      className={`rounded-lg border border-[var(--color-border)] ${bg} ${className}`}
      {...props}
    />
  )
}
