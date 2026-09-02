// Curator vocabulary (Rota F3): reasons and entity kinds in the user's words.
export const CURATOR_REASON_LABEL: Record<string, string> = {
  exhausted: 'iterasyon tavanı',
  expired: 'süresi doldu',
  one_shot_done: 'tek seferlik çalıştı',
  never_fired: 'hiç tetiklenmedi',
  unfired_watcher: 'izleyici hiç ateşlenmedi',
  ghost_phase: 'faz hiç başlamadı',
}

export const CURATOR_ENTITY_LABEL: Record<string, string> = {
  automation: 'otomasyon',
  schedule: 'zamanlama',
  hook: 'hook',
  recipe: 'reçete',
}
