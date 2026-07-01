import { Field, Toggle, inputCls } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AutonomyPanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle label="Tüm otonomiyi duraklat (uygulama geneli)" hint="Zamanlanmış çağrılar modele gitmeden bloklanır. Manuel sohbet etkilenmez." checked={draft.pauseAutonomy} onChange={(v) => set('pauseAutonomy', v)} />
    </>
  )
}

export function AutoTitlePanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle label="Otomatik başlık üretimi" hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir." checked={draft.autoTitleEnabled} onChange={(v) => set('autoTitleEnabled', v)} />
      <Field label="Başlık modeli" hint="Boş = ajanın kendi modeli. Ucuz bir model seçebilirsin."><input value={draft.titleModel} onChange={(e) => set('titleModel', e.target.value)} placeholder="örn. haiku" className={inputCls} /></Field>
    </>
  )
}
