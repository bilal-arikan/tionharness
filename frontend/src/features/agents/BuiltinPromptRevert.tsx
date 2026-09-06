// "Revert to the code prompt" control for a system agent's soul editor.
//
// A system agent's shipped prompt lives in internal/prompts/defaults/<key>.md
// and is compiled into the binary. Once a role is customised, the only ways
// back used to be deleting the customisation (which also throws away its
// model/tool choices) or pasting the original text by hand. This button fetches
// the compiled-in prompt for the agent's system role and hands it to the form,
// which stages it as an ordinary unsaved edit — so the change is still
// reviewable and only lands on Save.
import { useState } from 'react'
import { RotateCcw } from 'lucide-react'
import { api } from '@/api'

interface Props {
  /** Agent whose system role supplies the built-in prompt. */
  agentId: string
  /** Current editor text, used to disable the button when already identical. */
  current: string
  /** Receives the compiled-in prompt text. */
  onRevert: (soul: string) => void
  /** Surfaces a fetch failure inline in the form. */
  onError: (msg: string) => void
  disabled?: boolean
}

export function BuiltinPromptRevert({ agentId, current, onRevert, onError, disabled }: Props) {
  const [loading, setLoading] = useState(false)
  // Cached so repeated clicks (and the "already at default" check after a
  // revert) do not re-hit the endpoint.
  const [builtin, setBuiltin] = useState<string | null>(null)

  const atBuiltin = builtin !== null && current.trim() === builtin.trim()

  const doRevert = async () => {
    if (builtin !== null) {
      onRevert(builtin)
      return
    }
    setLoading(true)
    try {
      const res = await api.agentBuiltinPrompt(agentId)
      setBuiltin(res.soul)
      onRevert(res.soul)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <button
      type="button"
      onClick={doRevert}
      disabled={disabled || loading || atBuiltin}
      data-testid="agent-soul-revert-builtin"
      title="Bu ajanın koddaki gömülü sistem promptunu geri yükler. Değişiklik kaydedene kadar uygulanmaz."
      className="inline-flex items-center gap-1 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-normal text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-30 disabled:hover:border-[var(--color-border)] disabled:hover:text-[var(--color-text-dim)]"
    >
      <RotateCcw size={11} />
      {loading ? 'Yükleniyor…' : atBuiltin ? 'koddaki prompt' : 'Koddaki prompta dön'}
    </button>
  )
}
