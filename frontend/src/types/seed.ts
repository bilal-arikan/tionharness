// How a shipped default file on disk compares to the version TionSwarm ships
// (mirrors Go seed.State). It exists to make ONE thing legible: which files have
// stopped receiving shipped improvements.
//   'default' — untouched; refreshes automatically on upgrade.
//   'tuned'   — only config differs (e.g. a UI toggle); STILL refreshes
//               automatically, because the refresh compares bodies separately.
//   'edited'  — the content itself was changed; frozen until restored.
// Absent means the file is not a shipped default at all (user-authored, imported,
// or a workspace-tier override of a global skill).
export type SeedDefaultState = 'default' | 'tuned' | 'edited'
