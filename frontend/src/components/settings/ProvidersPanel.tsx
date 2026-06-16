// Providers category: default provider/model picker, Anthropic + MiniMax keys,
// connection test and Anthropic beta toggles.
import type { AppSettings, ProviderTestResult } from '../../types'
import { ProviderModelSelect } from '../agents/ProviderModelSelect'
import { Field, Toggle, inputCls, type AppSet } from './primitives'

interface Props {
  draft: AppSettings
  set: AppSet
  setDraft: React.Dispatch<React.SetStateAction<AppSettings | null>>
  keyInput: string
  setKeyInput: (v: string) => void
  minimaxKeyInput: string
  setMinimaxKeyInput: (v: string) => void
  test: Record<string, ProviderTestResult | 'pending'>
  runTest: (provider: string) => void
  clearKey: (which: 'anthropic' | 'minimax') => void
}

function testBadge(test: Props['test'], provider: string) {
  const r = test[provider]
  if (!r) return null
  if (r === 'pending') return <span className="text-xs text-amber-400">test ediliyor…</span>
  if (r.ok) return <span className="text-xs text-emerald-400">✓ bağlandı{r.model ? ` (${r.model})` : ''}</span>
  return <span className="text-xs text-red-400">✗ {r.error}</span>
}

export function ProvidersPanel({
  draft,
  set,
  setDraft,
  keyInput,
  setKeyInput,
  minimaxKeyInput,
  setMinimaxKeyInput,
  test,
  runTest,
  clearKey,
}: Props) {
  return (
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
          {testBadge(test, draft.defaultProvider)}
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
  )
}
