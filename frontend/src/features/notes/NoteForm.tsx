import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button, InfoPopover, ModalOverlay, TagEditor } from '@/shared/components'
import type { NoteConfidence, NoteKind, NoteScope, NoteWrite } from '@/types'
import {
  NOTE_CONFIDENCES,
  NOTE_KINDS,
  NOTE_SCOPES,
  normalizeNoteWrite,
  parseList,
  validateNoteWrite,
  type NoteFormError,
} from './notesHelpers'

export type NoteFormMode = 'create' | 'edit' | 'correct'

interface Props {
  mode: NoteFormMode
  initial: NoteWrite
  submitting: boolean
  onSubmit: (write: NoteWrite) => void
  onCancel: () => void
}

const inputCls =
  'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'
const labelCls = 'block text-xs font-medium text-[var(--color-text-dim)]'

// NoteForm is the one dialog behind New / Edit / Correct. Validation mirrors the
// backend's required-field rules (validateNoteWrite) and surfaces inline after
// the first submit attempt; an invalid form never reaches the API.
export function NoteForm({ mode, initial, submitting, onSubmit, onCancel }: Props) {
  const { t } = useTranslation('notes')
  const [draft, setDraft] = useState<NoteWrite>(initial)
  // Reach lists are edited as free text and parsed on change, so the typed
  // separators stay on screen while the draft holds the parsed list.
  const [agentsText, setAgentsText] = useState((initial.agents ?? []).join(', '))
  const [projectsText, setProjectsText] = useState((initial.projects ?? []).join('\n'))
  const [attempted, setAttempted] = useState(false)

  const errors = validateNoteWrite(draft)
  const shown = attempted ? errors : []
  const has = (e: NoteFormError) => shown.includes(e)
  const set = <K extends keyof NoteWrite>(key: K, val: NoteWrite[K]) =>
    setDraft((d) => ({ ...d, [key]: val }))

  const submit = () => {
    setAttempted(true)
    if (errors.length > 0) return
    onSubmit(normalizeNoteWrite(draft))
  }

  const errorText = (e: NoteFormError) =>
    has(e) ? (
      <span role="alert" className="text-xs text-[var(--color-danger)]">
        {t(`form.errors.${e}`)}
      </span>
    ) : null

  return (
    <ModalOverlay onClose={onCancel}>
      <form
        data-testid="note-form"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="flex max-h-[90vh] w-[42rem] max-w-full flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
      >
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <h2 className="flex items-center gap-1 text-sm font-semibold">
            {t(`form.title.${mode}`)}
            {mode === 'correct' && <InfoPopover text={t('form.correctHint')} />}
          </h2>
          <button
            type="button"
            onClick={onCancel}
            aria-label={t('form.cancel')}
            className="rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <X size={16} />
          </button>
        </div>

        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.kind')}</span>
              <select
                data-testid="note-form-kind"
                value={draft.kind}
                onChange={(e) => set('kind', e.target.value as NoteKind)}
                className={inputCls}
              >
                {NOTE_KINDS.map((k) => (
                  <option key={k} value={k}>
                    {t(`kind.${k}`)}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.scope')}</span>
              <select
                data-testid="note-form-scope"
                value={draft.scope}
                onChange={(e) => set('scope', e.target.value as NoteScope)}
                className={inputCls}
              >
                {NOTE_SCOPES.map((s) => (
                  <option key={s} value={s}>
                    {t(`scope.${s}`)}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.confidence')}</span>
              <select
                data-testid="note-form-confidence"
                value={draft.confidence}
                onChange={(e) => set('confidence', e.target.value as NoteConfidence)}
                className={inputCls}
              >
                {NOTE_CONFIDENCES.map((c) => (
                  <option key={c} value={c}>
                    {t(`confidence.${c}`)}
                  </option>
                ))}
              </select>
            </label>
          </div>

          {draft.scope === 'agent' && (
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.agents')}</span>
              <input
                data-testid="note-form-agents"
                value={agentsText}
                onChange={(e) => {
                  setAgentsText(e.target.value)
                  set('agents', parseList(e.target.value))
                }}
                placeholder={t('form.agentsPlaceholder')}
                className={inputCls}
              />
              {errorText('agents')}
            </label>
          )}
          {draft.scope === 'project' && (
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.projects')}</span>
              <textarea
                data-testid="note-form-projects"
                value={projectsText}
                rows={2}
                onChange={(e) => {
                  setProjectsText(e.target.value)
                  set('projects', parseList(e.target.value))
                }}
                placeholder={t('form.projectsPlaceholder')}
                className={`${inputCls} font-mono`}
              />
              {errorText('projects')}
            </label>
          )}
          {draft.confidence === 'verified' && (
            <label className="flex flex-col gap-1">
              <span className={labelCls}>{t('form.verification')}</span>
              <input
                data-testid="note-form-verification"
                value={draft.verification ?? ''}
                onChange={(e) => set('verification', e.target.value)}
                placeholder={t('form.verificationPlaceholder')}
                className={inputCls}
              />
              {errorText('verification')}
            </label>
          )}

          <label className="flex flex-col gap-1">
            <span className={labelCls}>{t('form.noteTitle')}</span>
            <input
              data-testid="note-form-title"
              value={draft.title}
              onChange={(e) => set('title', e.target.value)}
              maxLength={200}
              aria-invalid={has('title') || undefined}
              className={inputCls}
            />
            {errorText('title')}
          </label>

          <label className="flex flex-col gap-1">
            <span className={labelCls}>{t('form.body')}</span>
            <textarea
              data-testid="note-form-body"
              value={draft.body}
              rows={10}
              onChange={(e) => set('body', e.target.value)}
              placeholder={t('form.bodyPlaceholder')}
              aria-invalid={has('body') || undefined}
              className={`${inputCls} min-h-40 font-mono`}
            />
            {errorText('body')}
          </label>

          <div className="flex flex-col gap-1">
            <span className={labelCls}>{t('form.tags')}</span>
            <TagEditor tags={draft.tags ?? []} onChange={(tags) => set('tags', tags)} />
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              data-testid="note-form-private"
              checked={!!draft.private}
              onChange={(e) => set('private', e.target.checked)}
            />
            <span>{t('form.private')}</span>
            <InfoPopover text={t('form.privateHint')} />
          </label>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--color-border)] px-4 py-3">
          <Button type="button" variant="secondary" onClick={onCancel}>
            {t('form.cancel')}
          </Button>
          <Button type="submit" data-testid="note-form-submit" disabled={submitting}>
            {submitting ? t('form.saving') : t(`form.submit.${mode}`)}
          </Button>
        </div>
      </form>
    </ModalOverlay>
  )
}
