import type { SimpleIcon } from 'simple-icons'

interface Props {
  /** A simple-icons glyph, e.g. `siGithub`. */
  icon: SimpleIcon
  /** Pixel box, matching lucide-react's `size` prop so the two are interchangeable. */
  size?: number
  className?: string
  /**
   * Paint the glyph in the brand's own colour instead of inheriting the text
   * colour. Off by default: most placements sit inside a link or button whose
   * hover/active states drive the colour, and a hardcoded brand hex would ignore
   * them (and clash with the light/dark themes).
   */
  brandColor?: boolean
}

/**
 * BrandIcon renders a simple-icons brand glyph with lucide-react's prop shape.
 *
 * WHY: lucide-react dropped its brand icons (`Github` and friends) in v1, which
 * broke the build with "Module 'lucide-react' has no exported member 'Github'".
 * simple-icons is already a dependency and already the project's source for brand
 * marks (shared/lib/programIcons.ts), but it ships path DATA, not components — so
 * a lucide-style icon slot could not consume it directly. This adapter closes that
 * gap, so any icon slot can take a brand mark without each caller hand-rolling an
 * <svg>.
 */
export function BrandIcon({ icon, size = 16, className, brandColor = false }: Props) {
  return (
    <svg
      role="img"
      aria-label={icon.title}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      className={className}
      fill={brandColor ? `#${icon.hex}` : 'currentColor'}
    >
      <title>{icon.title}</title>
      <path d={icon.path} />
    </svg>
  )
}
