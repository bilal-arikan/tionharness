import { i18next } from '@/i18n'

/** Translate shared, user-facing copy at call time so live locale changes are reflected. */
export function sharedText(key: string, params?: Record<string, unknown>): string {
  return i18next.t(key, { ns: 'shared', ...params }) as string
}
