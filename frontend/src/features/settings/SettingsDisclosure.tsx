import { createContext, useContext, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { InfoPopover } from '@/shared/components/InfoPopover'

// The title row's (ⓘ) slot, so a section's body can put its description next to
// the title it explains even when the parent owns the <SettingsDisclosure>.
const InfoSlotContext = createContext<HTMLElement | null>(null)

// SettingsDisclosure groups related settings under a titled card. It is always
// open — every control stays visible, so nothing that blocks saving is hidden.
// `description` (or a <SettingsDisclosureInfo> inside the body) sits behind an
// (ⓘ) next to the title instead of being printed above the controls.
export function SettingsDisclosure({
  title,
  description,
  children,
}: {
  title: string
  description?: ReactNode
  children: ReactNode
}) {
  const [slot, setSlot] = useState<HTMLElement | null>(null)
  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      <h3 className="flex items-center gap-1 px-4 py-3 text-sm font-semibold">
        {title}
        {description && <InfoPopover text={description} />}
        <span ref={setSlot} className="contents" />
      </h3>
      <div className="flex flex-col gap-4 border-t border-[var(--color-border)] p-4">
        <InfoSlotContext.Provider value={slot}>{children}</InfoSlotContext.Provider>
      </div>
    </section>
  )
}

// SettingsDisclosureInfo renders a section description as an (ⓘ) in the
// enclosing SettingsDisclosure's title row; outside one it stays inline.
export function SettingsDisclosureInfo({ text }: { text: ReactNode }) {
  const slot = useContext(InfoSlotContext)
  const info = <InfoPopover text={text} />
  return slot ? createPortal(info, slot) : info
}
