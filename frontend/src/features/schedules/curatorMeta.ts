// Curator vocabulary (Rota F3): reasons and entity kinds in the user's words.
import { i18next } from '@/i18n'

const label = (key: string) => i18next.t(key, { ns: 'schedules' })

export const CURATOR_REASON_LABEL: Record<string, string> = {
  get exhausted() {
    return label('curator.reasons.exhausted')
  },
  get expired() {
    return label('curator.reasons.expired')
  },
  get one_shot_done() {
    return label('curator.reasons.oneShotDone')
  },
  get never_fired() {
    return label('curator.reasons.neverFired')
  },
  get unfired_watcher() {
    return label('curator.reasons.unfiredWatcher')
  },
  get ghost_phase() {
    return label('curator.reasons.ghostPhase')
  },
}

export const CURATOR_ENTITY_LABEL: Record<string, string> = {
  get automation() {
    return label('curator.entities.automation')
  },
  get schedule() {
    return label('curator.entities.schedule')
  },
  hook: 'hook',
  get recipe() {
    return label('curator.entities.recipe')
  },
}
