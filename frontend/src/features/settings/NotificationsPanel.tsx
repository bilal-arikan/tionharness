import { useState } from 'react'
import { Bell } from 'lucide-react'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '@/shared/lib/notifyPrefs'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'

// Both scopes apply immediately. A failed server request leaves the saved
// global value intact; device preferences retain their existing local storage.
export function NotificationsPanel({
  enabled,
  saving,
  onChange,
}: {
  enabled: boolean
  saving: boolean
  onChange: (enabled: boolean) => void
}) {
  const [muted, setMuted] = useState<Set<string>>(() => mutedTypes())
  const toggleType = (type: string, on: boolean) => {
    setTypeEnabled(type, on)
    setMuted((prev) => {
      const next = new Set(prev)
      if (on) next.delete(type)
      else next.add(type)
      return next
    })
  }

  return (
    <section className="flex flex-col gap-4">
      <SubHead icon={Bell}>Desktop notifications</SubHead>
      <p className="text-xs text-[var(--color-text-dim)]">
        All workspaces · saved immediately. Your browser must also allow notifications.
      </p>
      <fieldset disabled={saving} className="min-w-0">
        <Toggle
          label="Desktop notifications"
          checked={enabled}
          onChange={onChange}
          hint="Show notifications when the app is in the background."
        />
      </fieldset>
      {saving && (
        <p role="status" className="text-xs">
          Saving…
        </p>
      )}
      <SubHead icon={Bell}>Notification types</SubHead>
      <p className="text-xs text-[var(--color-text-dim)]">
        This device only · applied immediately. These preferences take effect when desktop
        notifications are enabled. Muting a type does not mute its sound; use Sound effects below.
      </p>
      {NOTIFY_TYPES.map((type) => (
        <Toggle
          key={type.type}
          label={type.cue ? `${type.label} 🔊` : type.label}
          hint={type.hint}
          checked={!muted.has(type.type)}
          onChange={(on) => toggleType(type.type, on)}
        />
      ))}
    </section>
  )
}
