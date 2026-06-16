import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { api } from '../api'
import { ProviderModelSelect } from './ProviderModelSelect'
import { THEME_PRESETS } from '../lib/themePresets'
import { STEP_KINDS } from '../lib/stepKinds'
import type {
  AppSettings,
  ProviderTestResult,
  SettingsPatch,
  SlashCommand,
  WorkspaceSettings,
} from '../types'

interface Props {
  onError: (msg: string) => void
  // Re-apply theme/accent globally after an app-settings save.
  onSaved: (s: AppSettings) => void
  // Notify the app that workspace metadata (e.g. name) changed, so the
  // workspace list/switcher can refresh.
  onWorkspaceChanged?: () => void
  // Delete the active workspace (app handles confirm/switch). Returns whether
  // the deletion proceeded.
  onDeleteWorkspace?: () => void
  // Slash commands available in the chat composer — shown read-only in the
  // "Komutlar" reference category.
  commands?: SlashCommand[]
}

// Category keys: the app-global sections plus the per-workspace section.
type Cat =
  | 'profile'
  | 'appearance'
  | 'notifications'
  | 'providers'
  | 'context'
  | 'budget'
  | 'autonomy'
  | 'autotitle'
  | 'mcp'
  | 'commands'
  | 'stepkinds'
  | 'diagnostics'
  | 'about'
  | 'workspace'

const APP_CATS: { key: Cat; label: string; icon: string }[] = [
  { key: 'profile', label: 'Profil', icon: '👤' },
  { key: 'appearance', label: 'Görünüm', icon: '🎨' },
  { key: 'notifications', label: 'Bildirimler & Ekran', icon: '🔔' },
  { key: 'providers', label: 'Sağlayıcılar', icon: '🔑' },
  { key: 'context', label: 'Bağlam & Bellek', icon: '🧠' },
  { key: 'budget', label: 'Bütçe', icon: '🛡' },
  { key: 'autonomy', label: 'Otonomi', icon: '⚙' },
  { key: 'autotitle', label: 'Otomatik Başlık', icon: '🏷' },
  { key: 'mcp', label: 'MCP & Araçlar', icon: '🔌' },
  { key: 'commands', label: 'Komutlar', icon: '⌘' },
  { key: 'stepkinds', label: 'Adım Türleri', icon: '🧩' },
  { key: 'diagnostics', label: 'Tanılama', icon: '🩺' },
  { key: 'about', label: 'Hakkında', icon: 'ℹ️' },
]

const WS_CATS: { key: Cat; label: string; icon: string }[] = [
  { key: 'workspace', label: 'Genel', icon: '🧩' },
]

// ---- small reusable field primitives ----

const inputCls =
  'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {hint && <span className="text-xs text-[var(--color-text-dim)]">{hint}</span>}
    </label>
  )
}

function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <button
      onClick={() => onChange(!checked)}
      className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-left"
    >
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-[var(--color-text-dim)]">{hint}</span>}
      </span>
      <span
        className={`relative h-5 w-9 flex-shrink-0 rounded-full transition ${
          checked ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-surface-2)]'
        }`}
      >
        <span
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${
            checked ? 'left-4' : 'left-0.5'
          }`}
        />
      </span>
    </button>
  )
}

// SettingsPanel is the two-pane configuration screen: a category rail on the
// left (like the chat session list) and the selected category's fields on the
// right. App-global settings and per-workspace settings are separate scopes.
export function SettingsPanel({ onError, onSaved, onWorkspaceChanged, onDeleteWorkspace, commands = [] }: Props) {
  const [cat, setCat] = useState<Cat>('profile')

  // App-global settings scope.
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [keyInput, setKeyInput] = useState('')
  const [minimaxKeyInput, setMinimaxKeyInput] = useState('')
  const [test, setTest] = useState<Record<string, ProviderTestResult | 'pending'>>({})

  // Per-workspace settings scope.
  const [ws, setWs] = useState<WorkspaceSettings | null>(null)
  const [wsOrig, setWsOrig] = useState<WorkspaceSettings | null>(null)

  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.getSettings().then((s) => { setDraft(s); setOriginal(s) }).catch((e) => onError((e as Error).message))
    api.getWorkspaceSettings().then((s) => { setWs(s); setWsOrig(s) }).catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const isWs = cat === 'workspace'
  const dirtyApp = useMemo(
    () =>
      (draft && original && JSON.stringify(draft) !== JSON.stringify(original)) ||
      keyInput.length > 0 ||
      minimaxKeyInput.length > 0,
    [draft, original, keyInput, minimaxKeyInput],
  )
  const dirtyWs = useMemo(
    () => ws && wsOrig && JSON.stringify(ws) !== JSON.stringify(wsOrig),
    [ws, wsOrig],
  )
  const dirty = isWs ? dirtyWs : dirtyApp

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))
  const setWsField = <K extends keyof WorkspaceSettings>(key: K, val: WorkspaceSettings[K]) =>
    setWs((d) => (d ? { ...d, [key]: val } : d))

  const saveApp = async () => {
    if (!draft) return
    const patch: SettingsPatch = {
      theme: draft.theme, accent: draft.accent, themePreset: draft.themePreset, language: draft.language,
      defaultProvider: draft.defaultProvider, defaultModel: draft.defaultModel, claudeCliPath: draft.claudeCliPath,
      minimaxBaseUrl: draft.minimaxBaseUrl,
      oneMillionContext: draft.oneMillionContext, extendedPromptCache: draft.extendedPromptCache,
      desktopNotifications: draft.desktopNotifications, keepAwake: draft.keepAwake,
      userName: draft.userName, userTimezone: draft.userTimezone, userCity: draft.userCity,
      userCountry: draft.userCountry, userNotes: draft.userNotes,
      maxContextTokens: draft.maxContextTokens, keepRecentMsgs: draft.keepRecentMsgs,
      recallTopN: draft.recallTopN, recallMinScore: draft.recallMinScore,
      defaultDailyCallLimit: draft.defaultDailyCallLimit, defaultDailyTokenLimit: draft.defaultDailyTokenLimit,
      defaultHeartbeatSec: draft.defaultHeartbeatSec, pauseAutonomy: draft.pauseAutonomy,
      autoTitleEnabled: draft.autoTitleEnabled, titleModel: draft.titleModel,
      mcpGatewayUrl: draft.mcpGatewayUrl, logLevel: draft.logLevel,
    }
    if (keyInput) patch.anthropicKey = keyInput
    if (minimaxKeyInput) patch.minimaxKey = minimaxKeyInput
    const updated = await api.updateSettings(patch)
    setDraft(updated); setOriginal(updated); setKeyInput(''); setMinimaxKeyInput('')
    onSaved(updated)
  }

  const saveWs = async () => {
    if (!ws) return
    const updated = await api.updateWorkspaceSettings({
      name: ws.name, instructions: ws.instructions, icon: ws.icon, color: ws.color,
      defaultProvider: ws.defaultProvider, defaultModel: ws.defaultModel,
      pauseAutonomy: ws.pauseAutonomy,
    })
    setWs(updated); setWsOrig(updated)
    onWorkspaceChanged?.()
  }

  const save = async () => {
    setSaving(true)
    try {
      if (isWs) await saveWs()
      else await saveApp()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const clearKey = async (which: 'anthropic' | 'minimax') => {
    try {
      const updated = await api.updateSettings(
        which === 'anthropic' ? { anthropicKey: '' } : { minimaxKey: '' },
      )
      setDraft(updated); setOriginal(updated)
      if (which === 'anthropic') setKeyInput('')
      else setMinimaxKeyInput('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const runTest = async (provider: string) => {
    setTest((t) => ({ ...t, [provider]: 'pending' }))
    try {
      const r = await api.testProvider(provider)
      setTest((t) => ({ ...t, [provider]: r }))
    } catch (e) {
      setTest((t) => ({ ...t, [provider]: { ok: false, error: (e as Error).message } }))
    }
  }

  const testBadge = (provider: string) => {
    const r = test[provider]
    if (!r) return null
    if (r === 'pending') return <span className="text-xs text-amber-400">test ediliyor…</span>
    if (r.ok) return <span className="text-xs text-emerald-400">✓ bağlandı{r.model ? ` (${r.model})` : ''}</span>
    return <span className="text-xs text-red-400">✗ {r.error}</span>
  }

  const catLabel = [...APP_CATS, ...WS_CATS].find((c) => c.key === cat)?.label ?? ''

  return (
    <div className="flex h-full">
      {/* Left: category rail */}
      <aside className="flex w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Uygulama
        </div>
        {APP_CATS.map((c) => (
          <CatButton key={c.key} c={c} active={cat === c.key} onClick={() => setCat(c.key)} dirty={c.key !== 'about' && !!dirtyApp} />
        ))}
        <div className="px-2 pb-1 pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Bu Workspace{ws ? ` · ${ws.name}` : ''}
        </div>
        {WS_CATS.map((c) => (
          <CatButton key={c.key} c={c} active={cat === c.key} onClick={() => setCat(c.key)} dirty={!!dirtyWs} />
        ))}
      </aside>

      {/* Right: content for the active category */}
      <div className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
          <span className="text-sm font-semibold">{catLabel}</span>
          <div className="flex items-center gap-3">
            <span className="text-xs text-[var(--color-text-dim)]">
              {dirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}
            </span>
            {cat !== 'about' && cat !== 'commands' && cat !== 'stepkinds' && (
              <button
                onClick={save}
                disabled={!dirty || saving}
                className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-30"
              >
                {saving ? 'Kaydediliyor…' : 'Kaydet'}
              </button>
            )}
          </div>
        </div>

        <div className="mx-auto w-full max-w-2xl flex-1 space-y-4 overflow-y-auto p-6">
          {!draft || !ws ? (
            <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
          ) : (
            <>
              {cat === 'profile' && (
                <>
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                    Bu bilgiler ajanların yanıtlarını sana göre kişiselleştirmesi için sohbet bağlamına eklenir.
                  </div>
                  <Field label="Ad" hint="Ajan sana nasıl hitap etsin."><input value={draft.userName} onChange={(e) => set('userName', e.target.value)} placeholder="örn. Ada" className={inputCls} /></Field>
                  <Field label="Saat dilimi" hint="'yarın', 'gelecek hafta' gibi göreli tarihler için."><input value={draft.userTimezone} onChange={(e) => set('userTimezone', e.target.value)} placeholder="örn. Europe/Istanbul" className={inputCls} /></Field>
                  <div className="grid grid-cols-2 gap-3">
                    <Field label="Şehir"><input value={draft.userCity} onChange={(e) => set('userCity', e.target.value)} placeholder="örn. İstanbul" className={inputCls} /></Field>
                    <Field label="Ülke"><input value={draft.userCountry} onChange={(e) => set('userCountry', e.target.value)} placeholder="örn. Türkiye" className={inputCls} /></Field>
                  </div>
                  <Field label="Notlar" hint="Tercihlerini anlatan serbest metin (talimatlar, çalışma şekli…)."><textarea value={draft.userNotes} onChange={(e) => set('userNotes', e.target.value)} rows={5} className={`${inputCls} resize-none`} placeholder="Ajanların bilmesi gereken tercihlerin…" /></Field>
                </>
              )}

              {cat === 'notifications' && (
                <>
                  <Toggle label="Masaüstü bildirimleri" hint="Pencere arkadayken asistan cevabı gelince tarayıcı bildirimi gösterir (izin ister)." checked={draft.desktopNotifications} onChange={(v) => set('desktopNotifications', v)} />
                  <Toggle label="Ekranı açık tut" hint="Uygulama açıkken ekran uyku moduna geçmez (Wake Lock)." checked={draft.keepAwake} onChange={(v) => set('keepAwake', v)} />
                </>
              )}

              {cat === 'appearance' && (
                <>
                  <Field label="Tema paleti" hint="Hazır bir palet seç; tüm arayüz yeniden renklenir. Vurgu rengini aşağıdan ince ayarlayabilirsin.">
                    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                      {THEME_PRESETS.map((p) => {
                        const sel = draft.themePreset === p.id
                        return (
                          <button
                            key={p.id}
                            type="button"
                            onClick={() => setDraft((d) => (d ? { ...d, themePreset: p.id, accent: p.tokens.accent } : d))}
                            className={`flex items-center gap-2.5 rounded-lg border px-2.5 py-2 text-left transition ${
                              sel
                                ? 'border-[var(--color-accent)] ring-1 ring-[var(--color-accent)]'
                                : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                            }`}
                          >
                            <span
                              className="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-md border"
                              style={{ background: p.tokens.bg, borderColor: p.tokens.border }}
                            >
                              <span className="h-3.5 w-3.5 rounded-full" style={{ background: p.tokens.accent }} />
                            </span>
                            <span className="min-w-0">
                              <span className="block truncate text-xs font-medium text-[var(--color-text)]">{p.label}</span>
                              <span className="block text-[10px] text-[var(--color-text-dim)]">{p.dark ? 'Koyu' : 'Açık'}</span>
                            </span>
                          </button>
                        )
                      })}
                    </div>
                  </Field>
                  <Field label="Temel mod" hint="Yalnızca özel palet kullanılmadığında (sistem otomatik açık/koyu) etkilidir.">
                    <select value={draft.theme} onChange={(e) => set('theme', e.target.value as AppSettings['theme'])} className={inputCls}>
                      <option value="dark">Koyu</option>
                      <option value="light">Açık</option>
                      <option value="system">Sistem</option>
                    </select>
                  </Field>
                  <Field label="Vurgu rengi (accent)">
                    <div className="flex items-center gap-2">
                      <input type="color" value={draft.accent} onChange={(e) => set('accent', e.target.value)} className="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-bg)]" />
                      <input value={draft.accent} onChange={(e) => set('accent', e.target.value)} className={`${inputCls} w-32`} />
                    </div>
                  </Field>
                  <Field label="Dil" hint="UI dili tercihi (tam çeviri kademeli ekleniyor).">
                    <select value={draft.language} onChange={(e) => set('language', e.target.value as AppSettings['language'])} className={inputCls}>
                      <option value="tr">Türkçe</option>
                      <option value="en">English</option>
                    </select>
                  </Field>
                </>
              )}

              {cat === 'providers' && (
                <>
                  <div>
                    <div className="mb-1 text-sm font-medium">Varsayılan sağlayıcı + model</div>
                    <p className="mb-2 text-xs text-[var(--color-text-dim)]">Yeni ajanlar bunlarla oluşturulur (model boş = sağlayıcı varsayılanı).</p>
                    <ProviderModelSelect
                      provider={draft.defaultProvider}
                      model={draft.defaultModel}
                      onChange={(p, m) => setDraft((d) => (d ? { ...d, defaultProvider: p, defaultModel: m } : d))}
                    />
                    <div className="mt-2 flex items-center gap-2">
                      <button onClick={() => runTest(draft.defaultProvider)} className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]">
                        Bağlantıyı test et
                      </button>
                      {testBadge(draft.defaultProvider)}
                    </div>
                  </div>

                  <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Anthropic</div>
                  <Field label="API anahtarı" hint={draft.anthropicKeySet ? 'Kayıtlı (şifreli). Değiştirmek için yeni anahtar gir, temizlemek için sil.' : 'Henüz ayarlanmadı. anthropic sağlayıcısı için gerekli.'}>
                    <div className="flex items-center gap-2">
                      <input type="password" value={keyInput} onChange={(e) => setKeyInput(e.target.value)} placeholder={draft.anthropicKeySet ? '•••••••••• (kayıtlı)' : 'sk-ant-...'} className={`${inputCls} flex-1`} />
                      {draft.anthropicKeySet && (
                        <button onClick={() => clearKey('anthropic')} className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs text-red-400 hover:border-red-400">Sil</button>
                      )}
                    </div>
                  </Field>
                  <Field label="claude CLI yolu" hint="Boş = PATH üzerinden otomatik tespit.">
                    <input value={draft.claudeCliPath} onChange={(e) => set('claudeCliPath', e.target.value)} placeholder="otomatik" className={inputCls} />
                  </Field>

                  <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">MiniMax (OpenAI-uyumlu)</div>
                  <Field label="MiniMax API anahtarı" hint={draft.minimaxKeySet ? 'Kayıtlı (şifreli). Değiştirmek için yeni anahtar gir, temizlemek için sil.' : 'MiniMax modelleri için gerekli.'}>
                    <div className="flex items-center gap-2">
                      <input type="password" value={minimaxKeyInput} onChange={(e) => setMinimaxKeyInput(e.target.value)} placeholder={draft.minimaxKeySet ? '•••••••••• (kayıtlı)' : 'MiniMax API key'} className={`${inputCls} flex-1`} />
                      {draft.minimaxKeySet && (
                        <button onClick={() => clearKey('minimax')} className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs text-red-400 hover:border-red-400">Sil</button>
                      )}
                    </div>
                  </Field>
                  <Field label="MiniMax base URL" hint="Boş = https://api.minimax.io/v1 (OpenAI-uyumlu uç).">
                    <input value={draft.minimaxBaseUrl} onChange={(e) => set('minimaxBaseUrl', e.target.value)} placeholder="https://api.minimax.io/v1" className={inputCls} />
                  </Field>

                  <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Anthropic beta (yalnız anthropic sağlayıcı)</div>
                  <Toggle label="1 milyon token bağlam" hint="Anthropic 1M context penceresi beta'sı (anthropic-beta başlığı). claude-cli'da etkisizdir." checked={draft.oneMillionContext} onChange={(v) => set('oneMillionContext', v)} />
                  <Toggle label="Uzatılmış prompt cache (1 saat)" hint="Sistem promptunu 1 saatlik cache_control ile önbelleğe alır — tekrar eden büyük persona/bağlam ucuzlar." checked={draft.extendedPromptCache} onChange={(v) => set('extendedPromptCache', v)} />
                </>
              )}

              {cat === 'context' && (
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Maks. bağlam token" hint="Aşılınca eski turlar özetlenir."><input type="number" value={draft.maxContextTokens} onChange={(e) => set('maxContextTokens', Number(e.target.value))} className={inputCls} /></Field>
                  <Field label="Korunan son mesaj" hint="Her zaman aynen gönderilir."><input type="number" value={draft.keepRecentMsgs} onChange={(e) => set('keepRecentMsgs', Number(e.target.value))} className={inputCls} /></Field>
                  <Field label="Recall sonuç sayısı (top-N)"><input type="number" value={draft.recallTopN} onChange={(e) => set('recallTopN', Number(e.target.value))} className={inputCls} /></Field>
                  <Field label="Recall min skor" hint="0–1 arası benzerlik eşiği."><input type="number" step="0.01" value={draft.recallMinScore} onChange={(e) => set('recallMinScore', Number(e.target.value))} className={inputCls} /></Field>
                </div>
              )}

              {cat === 'budget' && (
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Günlük çağrı limiti" hint="0 = sınırsız"><input type="number" value={draft.defaultDailyCallLimit} onChange={(e) => set('defaultDailyCallLimit', Number(e.target.value))} className={inputCls} /></Field>
                  <Field label="Günlük token limiti" hint="0 = sınırsız"><input type="number" value={draft.defaultDailyTokenLimit} onChange={(e) => set('defaultDailyTokenLimit', Number(e.target.value))} className={inputCls} /></Field>
                  <p className="col-span-2 text-xs text-[var(--color-text-dim)]">Bu varsayılanlar yalnızca yeni oluşturulan ajanlara uygulanır.</p>
                </div>
              )}

              {cat === 'autonomy' && (
                <>
                  <Toggle label="Tüm otonomiyi duraklat (uygulama geneli)" hint="Heartbeat ve zamanlanmış çağrılar modele gitmeden bloklanır. Manuel sohbet etkilenmez." checked={draft.pauseAutonomy} onChange={(v) => set('pauseAutonomy', v)} />
                  <Field label="Varsayılan heartbeat aralığı (sn)"><input type="number" value={draft.defaultHeartbeatSec} onChange={(e) => set('defaultHeartbeatSec', Number(e.target.value))} className={inputCls} /></Field>
                </>
              )}

              {cat === 'autotitle' && (
                <>
                  <Toggle label="Otomatik başlık üretimi" hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir." checked={draft.autoTitleEnabled} onChange={(v) => set('autoTitleEnabled', v)} />
                  <Field label="Başlık modeli" hint="Boş = ajanın kendi modeli. Ucuz bir model seçebilirsin."><input value={draft.titleModel} onChange={(e) => set('titleModel', e.target.value)} placeholder="örn. haiku" className={inputCls} /></Field>
                </>
              )}

              {cat === 'mcp' && (
                <Field label="MCP Gateway URL" hint="Araç entegrasyonları için ağ geçidi adresi."><input value={draft.mcpGatewayUrl} onChange={(e) => set('mcpGatewayUrl', e.target.value)} placeholder="http://localhost:9091/mcp" className={inputCls} /></Field>
              )}

              {cat === 'commands' && (
                <>
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                    Sohbet kutusuna <code className="rounded bg-[var(--color-bg)] px-1">/</code> yazınca açılan komut paleti. Bir komutu komut olarak değil düz metin olarak göndermek istersen tırnak içine al: <code className="rounded bg-[var(--color-bg)] px-1">"/komut"</code>.
                  </div>
                  {commands.length === 0 ? (
                    <div className="text-sm text-[var(--color-text-dim)]">Kayıtlı komut yok.</div>
                  ) : (
                    <div className="flex flex-col gap-1.5">
                      {commands.map((c) => (
                        <div
                          key={c.name}
                          className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
                        >
                          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-2)] text-base">
                            {c.icon ?? '⚡'}
                          </span>
                          <div className="min-w-0">
                            <div className="font-mono text-sm font-medium text-[var(--color-text)]">/{c.name}</div>
                            <div className="truncate text-xs text-[var(--color-text-dim)]">{c.description}</div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}

              {cat === 'stepkinds' && (
                <>
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                    Bir asistan turunun aktivite izi (<code className="rounded bg-[var(--color-bg)] px-1">TurnStep</code>) farklı <strong>türlerden</strong> oluşur. Aşağıda her türün ne anlama geldiği, kalıcı mı yoksa yalnız-canlı mı olduğu ve şu an aktif mi listelenir. (Kaynak: <code className="rounded bg-[var(--color-bg)] px-1">internal/agent/trace.go</code>)
                  </div>
                  <div className="flex flex-col gap-1.5">
                    {STEP_KINDS.map((s) => (
                      <div
                        key={s.kind}
                        className="flex items-start gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2"
                      >
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-2)] text-base">
                          {s.icon}
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium text-[var(--color-text)]">{s.label}</span>
                            <code className="rounded bg-[var(--color-surface-2)] px-1 font-mono text-[11px] text-[var(--color-text-dim)]">
                              {s.kind}
                            </code>
                            <span
                              className={`rounded px-1.5 py-0.5 text-[10px] ${
                                s.status === 'active'
                                  ? 'bg-green-500/15 text-green-400'
                                  : 'bg-amber-500/15 text-amber-400'
                              }`}
                            >
                              {s.status === 'active' ? 'aktif' : 'altyapı hazır'}
                            </span>
                            <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
                              {s.persisted ? 'kalıcı' : 'yalnız-canlı'}
                            </span>
                          </div>
                          <div className="mt-0.5 text-xs text-[var(--color-text-dim)]">{s.description}</div>
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              )}

              {cat === 'diagnostics' && (
                <Field label="Log seviyesi" hint="Yeniden başlatınca uygulanır.">
                  <select value={draft.logLevel} onChange={(e) => set('logLevel', e.target.value)} className={inputCls}>
                    <option value="debug">debug</option>
                    <option value="info">info</option>
                    <option value="warn">warn</option>
                    <option value="error">error</option>
                  </select>
                </Field>
              )}

              {cat === 'about' && (
                <p className="text-sm text-[var(--color-text-dim)]">
                  SwarmGo — çok-ajanlı AI runtime. Uygulama ayarları{' '}
                  <code className="rounded bg-[var(--color-surface-2)] px-1">settings.json</code>,
                  workspace ayarları her workspace'in{' '}
                  <code className="rounded bg-[var(--color-surface-2)] px-1">ws-settings.json</code>{' '}
                  dosyasında saklanır.
                </p>
              )}

              {cat === 'workspace' && (
                <>
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                    Bu ayarlar yalnızca <span className="font-medium text-[var(--color-text)]">{ws.name}</span> workspace'ine özeldir. Boş bırakılan sağlayıcı/model uygulama-geneli varsayılana düşer.
                  </div>

                  {/* Stats */}
                  <div className="grid grid-cols-4 gap-2">
                    {[
                      { label: 'Ajan', value: ws.agentCount },
                      { label: 'Oturum', value: ws.sessionCount },
                      { label: 'Görev', value: ws.taskCount },
                      { label: 'Oluşturma', value: new Date(ws.createdAt * 1000).toLocaleDateString('tr-TR') },
                    ].map((s) => (
                      <div key={s.label} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center">
                        <div className="text-sm font-semibold">{s.value}</div>
                        <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">{s.label}</div>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-end gap-3">
                    <Field label="İkon (emoji)">
                      <input value={ws.icon} onChange={(e) => setWsField('icon', e.target.value)} placeholder="🧩" maxLength={4} className={`${inputCls} w-20 text-center text-lg`} />
                    </Field>
                    <Field label="Renk">
                      <div className="flex items-center gap-2">
                        <input type="color" value={ws.color || '#4f8cff'} onChange={(e) => setWsField('color', e.target.value)} className="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-bg)]" />
                        <input value={ws.color} onChange={(e) => setWsField('color', e.target.value)} placeholder="(varsayılan)" className={`${inputCls} w-28`} />
                      </div>
                    </Field>
                    <div className="flex items-center gap-2 pb-1 text-sm">
                      <span className="flex h-9 w-9 items-center justify-center rounded-lg text-lg" style={{ backgroundColor: (ws.color || '#1e3a66') + '33' }}>{ws.icon || '⬡'}</span>
                      <span className="text-xs text-[var(--color-text-dim)]">önizleme</span>
                    </div>
                  </div>

                  <Field label="Workspace adı"><input value={ws.name} onChange={(e) => setWsField('name', e.target.value)} className={inputCls} /></Field>
                  <Field label="Talimatlar (bu workspace)" hint="Bu workspace'teki tüm agent'lara eklenen yönergeler."><textarea value={ws.instructions} onChange={(e) => setWsField('instructions', e.target.value)} rows={4} className={`${inputCls} resize-none`} placeholder="Örn. Tüm cevapları Türkçe ver; commit at ama push'lama." /></Field>
                  <Field label="Varsayılan sağlayıcı (bu workspace)" hint="Boş = uygulama varsayılanı.">
                    <select value={ws.defaultProvider} onChange={(e) => setWsField('defaultProvider', e.target.value)} className={inputCls}>
                      <option value="">(uygulama varsayılanı)</option>
                      <option value="claude-cli">claude-cli (abonelik)</option>
                      <option value="anthropic">anthropic (API key)</option>
                    </select>
                  </Field>
                  <Field label="Varsayılan model (bu workspace)" hint="Boş = uygulama varsayılanı."><input value={ws.defaultModel} onChange={(e) => setWsField('defaultModel', e.target.value)} placeholder="(uygulama varsayılanı)" className={inputCls} /></Field>
                  <Toggle label="Bu workspace'te otonomiyi duraklat" hint="Yalnızca bu workspace'in heartbeat/zamanlama çağrılarını bloklar." checked={ws.pauseAutonomy} onChange={(v) => setWsField('pauseAutonomy', v)} />

                  {onDeleteWorkspace && (
                    <div className="mt-2 flex items-center justify-between rounded-lg border border-red-500/30 bg-red-500/5 px-3 py-2">
                      <span className="text-xs text-[var(--color-text-dim)]">Bu workspace'i ve tüm verisini kalıcı olarak sil.</span>
                      <button
                        onClick={onDeleteWorkspace}
                        className="rounded border border-red-500/40 px-3 py-1 text-xs text-red-400 hover:bg-red-500/10"
                      >
                        Workspace'i sil
                      </button>
                    </div>
                  )}
                </>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function CatButton({
  c,
  active,
  onClick,
  dirty,
}: {
  c: { key: Cat; label: string; icon: string }
  active: boolean
  onClick: () => void
  dirty: boolean
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
        active ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <span className="text-base leading-none">{c.icon}</span>
      <span className="flex-1 truncate">{c.label}</span>
      {dirty && <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]" title="Kaydedilmemiş" />}
    </button>
  )
}
