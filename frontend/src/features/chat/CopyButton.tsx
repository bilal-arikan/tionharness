import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { actionChip, actionChipActive } from './messageActions'
import { useTranslation } from 'react-i18next'

// CopyButton copies a message's raw text to the clipboard, flashing a checkmark
// for a moment to confirm the copy actually happened (clipboard writes are
// silent otherwise). Lives in the turn footer next to the other actions.
export function CopyButton({ text }: { text: string }) {
  const { t } = useTranslation('chat')
  const [copied, setCopied] = useState(false)
  const onClick = async () => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <button
      onClick={onClick}
      title={copied ? t('copy.copied') : t('copy.message')}
      aria-label={t('copy.message')}
      className={copied ? actionChipActive('positive') : actionChip()}
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
    </button>
  )
}
