import { useEffect, useState } from 'react'
import { ShieldCheck, Save } from 'lucide-react'
import { api } from '@/api'
import type { AppSettings } from '@/types'
import { Field, Toggle, inputCls } from '@/features/settings/primitives'
import { LoadingState } from '@/shared/components'

interface Props {
  onError: (msg: string) => void
}

// LessonsTab hosts the full self-healing surface (loop-protection guardrails +
// stuck-session threshold + failure→lesson reflection + the stored lessons),
// moved here from Settings ▸ Context so all self-improvement lives in the Insight
// cockpit. It edits the app-global settings directly (draft + Save).
export function LessonsTab({ onError }: Props) {
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))

  const dirty = !!draft && !!original && JSON.stringify(draft) !== JSON.stringify(original)

  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      const updated = await api.updateSettings({
        toolGuardWarnings: draft.toolGuardWarnings,
        toolGuardHardStop: draft.toolGuardHardStop,
        guardExactWarn: draft.guardExactWarn,
        guardExactBlock: draft.guardExactBlock,
        guardSameToolWarn: draft.guardSameToolWarn,
        guardSameToolHalt: draft.guardSameToolHalt,
        guardNoProgressWarn: draft.guardNoProgressWarn,
        guardNoProgressBlock: draft.guardNoProgressBlock,
        stuckTurnThreshold: draft.stuckTurnThreshold,
        lessonReflect: draft.lessonReflect,
        lessonMaxAgeDays: draft.lessonMaxAgeDays,
      })
      setDraft(updated)
      setOriginal(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (!draft) {
    return <LoadingState label="Yükleniyor…" />
  }

  return (
    <div className="max-w-2xl space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
            <ShieldCheck size={14} />
          </span>
          Self-healing (döngü koruması & ders çıkarma)
        </h3>
        <button
          onClick={save}
          disabled={!dirty || saving}
          className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-white disabled:opacity-50"
        >
          <Save className="h-4 w-4" /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
        </button>
      </div>

      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Kendi kendini onaran oturum akışları (<code>56-SELF-HEALING</code>): araç döngüsü tur içinde
        tekrar eden hataları izler (aynı çağrı 2 hatada uyarılır, aynı araç 3 ardışık hatada
        uyarılır); devre kesici açıksa 5 tekrarında çağrı bloklanır, 8 ardışık hatada tur kontrollü
        durdurulur. Üst üste kötü biten oturumlar <code>stuck</code> etiketi alır ve otonom turları
        askıya alınır (manuel sohbet hiç etkilenmez). Ders çıkarma, başarısız turlardan kısa dersler
        damıtıp sonraki turlara enjekte eder.
      </div>

      <Toggle
        label="Guardrail uyarıları"
        hint="Tekrar eden başarısız araç çağrısının sonucuna eyleme dönük kurtarma ipucu eklenir (teşhis et, farklı argüman/araç dene). Yürütmeyi asla engellemez."
        checked={draft.toolGuardWarnings}
        onChange={(v) => set('toolGuardWarnings', v)}
      />
      <Toggle
        label="Devre kesici (hard stop)"
        hint="Eşik üstü tekrar: birebir aynı başarısız çağrı blok eşiğinde çalıştırılmadan bloklanır; aynı araç halt eşiğinde turu kontrollü sonlandırır (guardrail_halt). Varsayılan kapalı."
        checked={draft.toolGuardHardStop}
        onChange={(v) => set('toolGuardHardStop', v)}
      />
      <div className="grid grid-cols-3 gap-3">
        <Field
          label="Aynı çağrı: uyarı"
          hint="Birebir aynı (araç+argüman) başarısız çağrı bu sayıda uyarı alır (0 = varsayılan 2)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardExactWarn}
            onChange={(e) => set('guardExactWarn', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Aynı çağrı: blok"
          hint="Devre kesici açıkken birebir aynı başarısız çağrı bu sayıda çalıştırılmadan bloklanır (0 = varsayılan 5)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardExactBlock}
            onChange={(e) => set('guardExactBlock', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Aynı araç: uyarı"
          hint="Aynı araç (farklı argümanlarla da olsa) bu kadar ardışık hatada uyarı alır (0 = varsayılan 3)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardSameToolWarn}
            onChange={(e) => set('guardSameToolWarn', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Aynı araç: tur durdur"
          hint="Devre kesici açıkken aynı araç bu kadar ardışık hatada turu kontrollü sonlandırır (0 = varsayılan 8)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardSameToolHalt}
            onChange={(e) => set('guardSameToolHalt', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="İlerleme yok: uyarı"
          hint="Aynı salt-okunur çağrının başarılı tekrarları bu sayıyı aşınca uyarı alır (0 = varsayılan 2)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardNoProgressWarn}
            onChange={(e) => set('guardNoProgressWarn', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="İlerleme yok: blok"
          hint="Devre kesici açıkken aynı salt-okunur çağrı bu sayıda tekrarda bloklanır (0 = varsayılan 5)."
        >
          <input
            type="number"
            min={0}
            max={50}
            value={draft.guardNoProgressBlock}
            onChange={(e) => set('guardNoProgressBlock', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>
      <Field
        label="Stuck oturum eşiği"
        hint="Üst üste bu kadar tur kötü biten (tur hatası / guardrail halt) oturum 'stuck' etiketi alır ve OTONOM turları reddedilir; temiz bir tur sayacı sıfırlar, etiketi kaldırmak da sıfırlar. 0 = kapalı."
      >
        <input
          type="number"
          min={0}
          max={20}
          value={draft.stuckTurnThreshold}
          onChange={(e) => set('stuckTurnThreshold', Number(e.target.value))}
          className={inputCls}
        />
      </Field>
      <Toggle
        label="Hatalardan ders çıkar (lesson reflect)"
        hint="Kötü biten turdan arka planda kısa bir ders damıtılır (başlık modeli varsa o, yoksa ajanın modeli — hata turu başına 1 ucuz çağrı) ve workspace-geneli lessons.jsonl'e yazılır; en yeni 5 ders her turun dinamik bağlamına enjekte edilir. Aynı hata şekli tekrarında mevcut ders güncellenir (yığılmaz)."
        checked={draft.lessonReflect}
        onChange={(v) => set('lessonReflect', v)}
      />
      <Field
        label="Ders ömrü (gün)"
        hint="Bir dersin hata şekli bu kadar gün içinde tekrar etmezse bayat sayılıp budanır (bir sonraki ders yazımında). Düşük = daha hızlı unutma. 0 = yerleşik varsayılan (2 gün)."
      >
        <input
          type="number"
          min={0}
          max={365}
          value={draft.lessonMaxAgeDays}
          onChange={(e) => set('lessonMaxAgeDays', Number(e.target.value))}
          className={inputCls}
        />
      </Field>
    </div>
  )
}
