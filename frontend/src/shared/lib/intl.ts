// Locale-aware Intl factories.
//
// Before i18n the app formatted dates, numbers and sort order against a hardcoded
// 'tr-TR' / 'tr' tag scattered across ~50 call sites. Everything locale-sensitive
// now goes through this module, which reads the active UI locale at call time so a
// language switch changes formatting without any caller passing a tag around.
//
// Intl constructors are comparatively expensive, so each formatter is memoised per
// (locale, options) pair. The cache is bounded by how many distinct option sets the
// app actually uses — a handful — so it needs no eviction.

import { currentLocale, intlTag } from '@/i18n'

const dateCache = new Map<string, Intl.DateTimeFormat>()
const numberCache = new Map<string, Intl.NumberFormat>()
const relativeCache = new Map<string, Intl.RelativeTimeFormat>()
const collatorCache = new Map<string, Intl.Collator>()

function key(tag: string, opts: unknown): string {
  return tag + '|' + JSON.stringify(opts ?? {})
}

// activeTag is the BCP-47 tag for the locale the UI is currently rendered in.
function activeTag(): string {
  return intlTag(currentLocale())
}

export function dateFormat(opts?: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const tag = activeTag()
  const k = key(tag, opts)
  let f = dateCache.get(k)
  if (!f) {
    f = new Intl.DateTimeFormat(tag, opts)
    dateCache.set(k, f)
  }
  return f
}

export function numberFormat(opts?: Intl.NumberFormatOptions): Intl.NumberFormat {
  const tag = activeTag()
  const k = key(tag, opts)
  let f = numberCache.get(k)
  if (!f) {
    f = new Intl.NumberFormat(tag, opts)
    numberCache.set(k, f)
  }
  return f
}

// --- Date/time convenience wrappers -----------------------------------------
// These replace the direct Date.prototype.toLocale*String calls that used to be
// spread across the panels with a hardcoded 'tr-TR' tag. Each accepts a Date, a
// millisecond epoch or a `null`-ish value (rendering "" rather than "Invalid
// Date", which is what a missing timestamp used to produce).

type When = Date | number | null | undefined

function toDate(v: When): Date | null {
  if (v === null || v === undefined) return null
  const d = v instanceof Date ? v : new Date(v)
  return Number.isNaN(d.getTime()) ? null : d
}

// formatDate renders a calendar date only ("25.08.2026" / "8/25/2026").
export function formatDate(v: When, opts: Intl.DateTimeFormatOptions = { dateStyle: 'short' }) {
  const d = toDate(v)
  return d ? dateFormat(opts).format(d) : ''
}

// formatTime renders a wall-clock time. Seconds are included by default because
// every current caller is a debug/telemetry timeline where they matter.
export function formatTime(v: When, opts: Intl.DateTimeFormatOptions = { timeStyle: 'medium' }) {
  const d = toDate(v)
  return d ? dateFormat(opts).format(d) : ''
}

// formatDateTime renders date + time, the default for "created at" style labels.
export function formatDateTime(
  v: When,
  opts: Intl.DateTimeFormatOptions = { dateStyle: 'short', timeStyle: 'medium' },
) {
  const d = toDate(v)
  return d ? dateFormat(opts).format(d) : ''
}

// collator returns a locale-aware string comparator. Use it instead of
// `a.localeCompare(b)` for any user-visible ordering: Turkish sorts ç/ğ/ı/ö/ş/ü in
// their own positions, and the default (implementation) locale would get that wrong
// whenever the browser locale differs from the UI locale.
function collator(opts?: Intl.CollatorOptions): Intl.Collator {
  const tag = activeTag()
  const k = key(tag, opts)
  let c = collatorCache.get(k)
  if (!c) {
    c = new Intl.Collator(tag, opts)
    collatorCache.set(k, c)
  }
  return c
}

// compareText is the ordering used for user-visible lists (names, titles, tags).
export function compareText(a: string, b: string): number {
  return collator().compare(a, b)
}

// clearIntlCaches drops every memoised formatter. Only needed by tests that switch
// locale between assertions; the app never has to call it because the cache key
// already includes the locale tag.
export function clearIntlCaches() {
  dateCache.clear()
  numberCache.clear()
  relativeCache.clear()
  collatorCache.clear()
}
