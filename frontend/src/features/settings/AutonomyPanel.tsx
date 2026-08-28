import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AutoTitlePanel({ draft, set }: PanelProps) {
  // Title generation always runs through the "titler" system agent, which owns
  // the prompt and the model; there is no per-setting provider/model override.
  return (
    <Toggle
      label="Otomatik başlık üretimi"
      hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir."
      checked={draft.autoTitleEnabled}
      onChange={(v) => set('autoTitleEnabled', v)}
    />
  )
}
