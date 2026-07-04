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
  // On portrait phones (`< md`) every modal becomes a bottom sheet: pinned to the
  // bottom edge, full-width, flat bottom corners, capped at 92dvh with the dialog's
  // own inner scroll. The `[&>*]` important overrides win over each child's fixed
  // width / max-width / rounding, so all 13 ModalOverlay consumers adapt from one
  // place. On `md+` the classes are inert and the centered desktop dialog is intact.
  // z-[60] sits ABOVE the fixed MobileNavBar (z-50) so a bottom-sheet modal on
  // portrait phones is not partly hidden behind the bottom navbar.
  return (
    <div
      className={`fixed inset-0 z-[60] flex items-center justify-center bg-black/50 ${padding} ${className} max-md:items-end max-md:p-0 max-md:[&>*]:!w-full max-md:[&>*]:!max-w-none max-md:[&>*]:!max-h-[92dvh] max-md:[&>*]:!rounded-b-none`}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      {children}
    </div>
  )
}
