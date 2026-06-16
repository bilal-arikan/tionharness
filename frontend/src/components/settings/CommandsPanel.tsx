// Komutlar category: read-only reference of the chat slash commands, each an
// expandable card revealing the built-in prompt behind it.
import { api } from '../../api'
import type { PromptInfo, SlashCommand } from '../../types'
import { PromptDetails } from './primitives'

interface Props {
  commands: SlashCommand[]
  prompts: PromptInfo[]
  promptsDir: string
  openCmds: Record<string, boolean>
  setOpenCmds: React.Dispatch<React.SetStateAction<Record<string, boolean>>>
  onError: (msg: string) => void
}

export function CommandsPanel({ commands, prompts, promptsDir, openCmds, setOpenCmds, onError }: Props) {
  const reveal = () => api.revealPrompts().catch((e) => onError((e as Error).message))
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        Sohbet kutusuna <code className="rounded bg-[var(--color-bg)] px-1">/</code> yazınca açılan komut paleti. Bir komutu komut olarak değil düz metin olarak göndermek istersen tırnak içine al: <code className="rounded bg-[var(--color-bg)] px-1">"/komut"</code>.
      </div>
      {commands.length === 0 ? (
        <div className="text-sm text-[var(--color-text-dim)]">Kayıtlı komut yok.</div>
      ) : (
        <div className="flex flex-col gap-1.5">
          {commands.map((c) => {
            // Map a command to the built-in prompt behind it; /tools is
            // deterministic (no prompt).
            const p =
              c.name === 'reflect'
                ? prompts.find((x) => x.key === 'reflect')
                : c.name === 'memory' || c.name === 'board' || c.name === 'flows'
                  ? prompts.find((x) => x.key === 'summary')
                  : undefined
            const open = !!openCmds[c.name]
            return (
              <div key={c.name} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)]">
                <button
                  onClick={() => setOpenCmds((o) => ({ ...o, [c.name]: !o[c.name] }))}
                  className="flex w-full items-center gap-3 px-3 py-2 text-left"
                >
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-2)] text-base">
                    {c.icon ?? '⚡'}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="font-mono text-sm font-medium text-[var(--color-text)]">/{c.name}</div>
                    <div className="truncate text-xs text-[var(--color-text-dim)]">{c.description}</div>
                  </div>
                  <span className="shrink-0 text-xs text-[var(--color-text-dim)]">{open ? '▾' : '▸'}</span>
                </button>
                {open && (
                  <div className="border-t border-[var(--color-border)] px-3 py-2.5">
                    {p ? (
                      <PromptDetails p={p} dir={promptsDir} onReveal={reveal} />
                    ) : (
                      <p className="text-xs text-[var(--color-text-dim)]">
                        Bu komut deterministiktir — model/prompt kullanmaz; sonuç runtime'da doğrudan üretilir.
                      </p>
                    )}
                  </div>
                )}
              </div>
            )
          })}

          {/* Auto-title isn't a slash command but shares the prompt folder;
              surface it here as one more collapsible card. */}
          {prompts
            .filter((p) => p.key === 'title')
            .map((p) => {
              const open = !!openCmds['__title']
              return (
                <div key={p.key} className="rounded-lg border border-dashed border-[var(--color-border)] bg-[var(--color-bg)]">
                  <button
                    onClick={() => setOpenCmds((o) => ({ ...o, __title: !o.__title }))}
                    className="flex w-full items-center gap-3 px-3 py-2 text-left"
                  >
                    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-2)] text-base">🏷</span>
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-medium text-[var(--color-text)]">Otomatik başlık</div>
                      <div className="truncate text-xs text-[var(--color-text-dim)]">Komut değil — sohbet/görev başlığı üretimi</div>
                    </div>
                    <span className="shrink-0 text-xs text-[var(--color-text-dim)]">{open ? '▾' : '▸'}</span>
                  </button>
                  {open && (
                    <div className="border-t border-[var(--color-border)] px-3 py-2.5">
                      <PromptDetails p={p} dir={promptsDir} onReveal={reveal} />
                    </div>
                  )}
                </div>
              )
            })}
        </div>
      )}
    </>
  )
}
