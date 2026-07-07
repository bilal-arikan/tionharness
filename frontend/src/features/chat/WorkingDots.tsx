// WorkingDots is the bouncing-dots "working" indicator shown while a turn is in
// flight (empty live assistant bubble / standalone pending bubble).
export function WorkingDots() {
  return (
    <span className="inline-flex gap-1 text-[var(--color-text-dim)]">
      <span className="animate-bounce">●</span>
      <span className="animate-bounce [animation-delay:0.15s]">●</span>
      <span className="animate-bounce [animation-delay:0.3s]">●</span>
    </span>
  )
}
