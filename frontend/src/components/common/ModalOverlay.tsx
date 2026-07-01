import { useEscapeKey } from '../../hooks/useEscapeKey'

interface Props {
  onClose: () => void
  children: React.ReactNode
  // Outer padding around the centered dialog (Tailwind class). Defaults to p-4.
  padding?: string
  // Extra classes for the backdrop container.
  className?: string
  // When true (default), Escape closes the modal.
  closeOnEscape?: boolean
}

// ModalOverlay is the shared dark-backdrop container that centers a dialog,
// closes on Escape, and closes on a backdrop (outside-the-dialog) mousedown.
// The dialog itself is passed as children; clicks inside it do not close.
export function ModalOverlay({
  onClose,
  children,
  padding = 'p-4',
  className = '',
  closeOnEscape = true,
}: Props) {
  useEscapeKey(onClose, closeOnEscape)
  return (
    <div
      className={`fixed inset-0 z-50 flex items-center justify-center bg-black/50 ${padding} ${className}`}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      {children}
    </div>
  )
}
