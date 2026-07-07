import { Field, Toggle, inputCls } from './primitives'
import type { PanelProps } from './settingsPanelShared'

// Note: the app-global autonomy pause was removed (2026-07-01) — autonomy pause is
// now per-workspace and toggled on the Schedules screen. This file keeps only the
// auto-title panel.

export function AutoTitlePanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle label="Otomatik başlık üretimi" hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir." checked={draft.autoTitleEnabled} onChange={(v) => set('autoTitleEnabled', v)} />
      <Field label="Başlık modeli" hint="Boş = ajanın kendi modeli. Ucuz bir model seçebilirsin."><input value={draft.titleModel} onChange={(e) => set('titleModel', e.target.value)} placeholder="örn. haiku" className={inputCls} /></Field>
    </>
  )
}
