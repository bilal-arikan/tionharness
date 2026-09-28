import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { AppSettings, SettingsPatch, SlashCommand } from '@/types'
import { LoadingState, toast } from '@/shared/components'
import { CatButton, NumberValidityProvider, type Cat } from './primitives'
import { useNumberValidity } from './numberValidity'
import { APP_CATS, HELP_CATS, resolveSettingsCat } from './settingsCats'
import {
  CATEGORY_FIELDS,
  categoryPatch,
  dirtyCategories,
  rebaseSettingsDraft,
} from './settingsFields'
import { SettingsSaveBar } from './SettingsSaveBar'
import { GeneralPanel } from './GeneralPanel'
import { DiagnosticsPanel } from './DiagnosticsPanel'
import { ExecutionPanel } from './ExecutionPanel'
import { ProviderOptionsPanel } from './ProviderOptionsPanel'
import { ReferencePanel } from './ReferencePanel'
import {
  ProfilePanel,
  NotificationsPanel,
  SoundPanel,
  ContextPanel,
  ToolsPanel,
  BackupPanel,
  AboutPanel,
} from './appPanels'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { CollapsibleListShell } from '@/shared/components'
import { ProvidersPanel } from './ProvidersPanel'
import { HooksPanel } from './HooksPanel'
import { ExternalToolsPanel } from './ExternalToolsPanel'
import { SecretsPanel } from './SecretsPanel'
import { SystemAgentsPanel } from './SystemAgentsPanel'
import { DeciderPanel } from '@/features/decider'

interface Props {
  onError: (msg: string) => void
  // Re-apply theme/accent globally after an app-settings save.
  onSaved: (s: AppSettings) => void
  // Slash commands available in the chat composer — shown read-only in the
  // "Komutlar" reference category.
  commands?: SlashCommand[]
  // Controlled active category (deep-link aware). When onCatChange is provided
  // the category is fully controlled by the parent (URL-synced); otherwise it is
  // tracked internally.
  cat?: string | null
  onCatChange?: (c: Cat) => void
  // Bumped by the parent when an agent changes app settings (settings SSE
  // event). On change the app-settings form reloads — but only when it has no
  // unsaved edits, so a concurrent agent change never clobbers in-progress typing.
  reloadNonce?: number
  // Left category rail collapse — controlled from the app header's list toggle,
  // matching every other list screen. Defaults to open when not provided.
  navOpen?: boolean
  onToggleNav?: () => void
}

// SettingsPanel is the two-pane configuration screen: a category rail on the
// left (like the chat session list) and the selected category's fields on the
// right. App-global settings and per-workspace settings are separate scopes.
// The per-category forms live in ./settings/*.
export function SettingsPanel({
  onError,
  onSaved,
  commands = [],
  cat: catProp,
  onCatChange,
  reloadNonce = 0,
  navOpen,
  onToggleNav,
}: Props) {
  // Category is controlled by the parent (URL deep-link) when onCatChange is
  // given; an unknown/empty routed category falls back to 'profile'.
  const [catState, setCatState] = useState<Cat>('profile')
  const [agentHeaderTarget, setAgentHeaderTarget] = useState<HTMLDivElement | null>(null)
  const controlled = onCatChange !== undefined
  const rawCat = controlled ? catProp : catState
  const cat = resolveSettingsCat(rawCat)
  const setCat = (c: Cat) => (onCatChange ? onCatChange(c) : setCatState(c))

  // App-global settings scope.
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [saving, setSaving] = useState(false)

  const savingRef = useRef(false)

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Numeric fields report empty / out-of-range input here; an invalid field never
  // writes to `draft`, and Save stays disabled until it is corrected so a bad
  // value can't be persisted as 0 or null.
  const numberValidity = useNumberValidity()

  const changedCategories = useMemo(() => dirtyCategories(draft, original), [draft, original])
  const dirtyApp = changedCategories.size > 0
  const dirty = changedCategories.has(cat)
  const hasHeaderSave = !!CATEGORY_FIELDS[cat] && cat !== 'providers'
  // Surface unsaved settings on the nav "Ayarlar" item + workspace label.
  useRegisterDirty('settings', dirtyApp)

  // Live reload: when an agent changes app settings (parent bumps reloadNonce),
  // re-fetch and refresh the form — but skip while the user has unsaved edits so
  // their in-progress changes are never clobbered. reloadNonce starts at 0; the
  // first bump (>0) is the first real signal.
  useEffect(() => {
    if (reloadNonce === 0 || dirtyApp || savingRef.current) return
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadNonce])

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))

  const persist = async (patch: SettingsPatch) => {
    if (!original || savingRef.current || Object.keys(patch).length === 0) return
    savingRef.current = true
    setSaving(true)
    try {
      const updated = await api.updateSettings(patch)
      setDraft((current) =>
        current ? rebaseSettingsDraft(current, original, patch, updated) : updated,
      )
      setOriginal(updated)
      onSaved(updated)
      toast.success('Saved')
    } catch (error) {
      onError((error as Error).message)
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  const save = () => {
    if (!draft || !original || numberValidity.hasInvalid) return
    void persist(categoryPatch(cat, draft, original))
  }
  const saveProps = { dirty, saving, invalid: numberValidity.hasInvalid, onSave: save }

  const catMeta = [...APP_CATS, ...HELP_CATS].find((c) => c.key === cat)
  const CatIcon = catMeta?.icon

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: category rail (collapsible via the app header toggle; mobile drawer) */}
      <CollapsibleListShell
        open={navOpen ?? true}
        onToggle={onToggleNav ?? (() => {})}
        label="Ayarlar"
      >
        <aside className="th-col flex h-full w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2 max-md:w-[85vw] max-md:max-w-sm square:w-48">
          <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Settings
          </div>
          {APP_CATS.map((c) => (
            <CatButton
              key={c.key}
              c={c}
              active={cat === c.key}
              onClick={() => setCat(c.key)}
              dirty={changedCategories.has(c.key)}
            />
          ))}
          <div className="px-2 pb-1 pt-4 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Help
          </div>
          {HELP_CATS.map((c) => (
            <CatButton
              key={c.key}
              c={c}
              active={cat === c.key}
              onClick={() => setCat(c.key)}
              dirty={false}
            />
          ))}
        </aside>
      </CollapsibleListShell>

      {/* Right: content for the active category. min-w-0 lets this flex column
          shrink below its content's intrinsic width on narrow screens (otherwise
          a wide child — e.g. a Hooks/ExternalTools code sample — forces overflow). */}
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--color-border)] px-6 py-3">
          <span className="flex items-center gap-2 text-sm font-semibold">
            {CatIcon && (
              <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                <CatIcon size={14} />
              </span>
            )}
            {catMeta?.label ?? ''}
          </span>
          <div className="flex items-center gap-3">
            {cat === 'sysagents' && (
              <div ref={setAgentHeaderTarget} className="flex items-center gap-3" />
            )}
            {hasHeaderSave && <SettingsSaveBar {...saveProps} />}
          </div>
        </div>

        {cat === 'secrets' ? (
          // Secrets manages its own list/forms; render full-bleed (the tool
          // catalog "Araçlar & MCP" now lives as a top-level NavRail view).
          <SecretsPanel onError={onError} />
        ) : cat === 'sysagents' ? (
          // Two-pane roster + settings form of its own; render full-bleed.
          <SystemAgentsPanel onError={onError} headerTarget={agentHeaderTarget} />
        ) : (
          <div className="th-column mx-auto w-full max-w-2xl flex-1 space-y-4 overflow-y-auto p-4 sm:p-6 3xl:max-w-4xl">
            {!draft ? (
              <LoadingState label="Yükleniyor…" />
            ) : (
              <NumberValidityProvider value={numberValidity}>
                {cat === 'profile' && <ProfilePanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'general' && <GeneralPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'execution' && (
                  <ExecutionPanel draft={draft} set={set} setDraft={setDraft} />
                )}
                {cat === 'diagnostics' && (
                  <DiagnosticsPanel draft={draft} set={set} setDraft={setDraft} />
                )}
                {cat === 'providers' && (
                  <>
                    <ProvidersPanel onOpenDecider={() => setCat('decider')} />
                    <ProviderOptionsPanel
                      draft={draft}
                      set={set}
                      setDraft={setDraft}
                      save={saveProps}
                    />
                  </>
                )}
                {cat === 'context' && <ContextPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'tools' && <ToolsPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'backup' && <BackupPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'hooks' && <HooksPanel onError={onError} />}
                {cat === 'exttools' && <ExternalToolsPanel onError={onError} />}
                {cat === 'decider' && (
                  <DeciderPanel onError={onError} onOpenProviders={() => setCat('providers')} />
                )}
                {cat === 'sound' && (
                  <>
                    <NotificationsPanel
                      enabled={original?.desktopNotifications ?? false}
                      saving={saving}
                      onChange={(enabled) => {
                        void persist({ desktopNotifications: enabled })
                      }}
                    />
                    <SoundPanel />
                  </>
                )}
                {cat === 'reference' && (
                  <ReferencePanel
                    key={rawCat}
                    commands={commands}
                    initialTab={rawCat === 'stepkinds' ? 'stepkinds' : 'commands'}
                  />
                )}
                {cat === 'about' && <AboutPanel />}
              </NumberValidityProvider>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
