import type { PickerOption } from './pickerOptions'

interface Props {
  option: Pick<PickerOption, 'icon' | 'Icon'>
  size: number
}

// PickerGlyph renders a picker option's leading mark: a component from the
// app's icon set when the option has one, otherwise its text glyph (the
// thinking-level intensity ramp).
export function PickerGlyph({ option, size }: Props) {
  const { Icon, icon } = option
  if (Icon) return <Icon size={size} className="shrink-0" />
  return icon ? <span>{icon}</span> : null
}
