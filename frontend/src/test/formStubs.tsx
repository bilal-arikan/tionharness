import type { ButtonHTMLAttributes, TextareaHTMLAttributes } from 'react'

export function Button(props: ButtonHTMLAttributes<HTMLButtonElement>) {
  return <button {...props} />
}

export function PromptEditor({
  value,
  onChange,
  ...props
}: Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <textarea {...props} value={value} onChange={(event) => onChange(event.currentTarget.value)} />
  )
}
