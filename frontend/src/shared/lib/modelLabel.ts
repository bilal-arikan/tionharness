import type { CatalogEntry, CatalogModel } from '@/types'

// Pure label logic for provider/model display, split out of catalog.ts so it can
// be unit-tested in the repo's `node` vitest environment — catalog.ts pulls in
// the api client, which touches localStorage at import time.

// stripTagline drops the descriptive suffix catalog labels append after a
// space-delimited dash ("Sonnet — dengeli" → "Sonnet", "MiniMax M3 - guncel
// amiral" → "MiniMax M3"). The dash must be surrounded by spaces so in-name
// hyphens ("GPT-5.5 Pro") and parenthetical variants ("Opus 4.8 (Fast)") are
// left intact. Full labels with taglines stay in the model pickers; only the
// agent-adjacent display (this resolver) shows the bare name.
function stripTagline(label: string): string {
  return label.replace(/\s+[—–-]\s+.*$/, '').trim()
}

// Model families we can name. Anything outside this list is left as its raw id
// rather than guessed at — a mislabelled model is worse than an unlabelled one.
const MODEL_FAMILIES = ['opus', 'sonnet', 'haiku', 'fable'] as const

// formatModelVersion turns a concrete model id into the versioned name people
// actually use: "claude-opus-5" → "Opus 5", "claude-sonnet-4-5-20250929" →
// "Sonnet 4.5", "claude-3-5-haiku-20241022" → "Haiku 3.5".
//
// It parses rather than table-maps because ids churn constantly; the shape
// (family token + dash-separated version parts + optional yyyymmdd stamp) has
// held across every naming scheme so far, and collecting the digits separately
// from the family token makes the token ORDER irrelevant — which is what lets
// the same code read both the current and the legacy version-first ids. Returns
// "" when no family token is present, letting the caller fall back to the raw id.
export function formatModelVersion(id: string): string {
  const parts = id
    .toLowerCase()
    .split(/[-_.]/)
    .filter(Boolean)
    .filter((p) => p !== 'claude' && !/^\d{8}$/.test(p)) // drop the vendor prefix + date stamp
  const family = parts.find((p) => (MODEL_FAMILIES as readonly string[]).includes(p))
  if (!family) return ''
  const version = parts.filter((p) => /^\d+$/.test(p)).join('.')
  const name = family.charAt(0).toUpperCase() + family.slice(1)
  return version ? `${name} ${version}` : name
}

// modelDisplayName is THE way to show a concrete model id anywhere in the app:
// the version people talk about ("Opus 5"), the raw id when we cannot name it,
// and an explicit placeholder when there is no model at all. The placeholder
// says only what we know — that no model id was recorded — because this function
// has no catalog and therefore cannot name the model that actually ran.
//
// Callers should keep the exact id in a `title`. It is the value that has to
// match a provider invoice or a bug report, so it must stay reachable — just not
// be the default reading.
//
// Distinct from resolveModelLabel, which starts from an agent's CONFIGURED model
// (possibly an alias) and needs the catalog to learn what it resolved to. Use
// this one when the id in hand is already concrete: a turn's model, a usage row,
// a debug event.
export function modelDisplayName(id: string): string {
  if (!id) return '(model belirtilmemiş)'
  return formatModelVersion(id) || id
}

// An observed resolution wins over the curated label: "opus" is a moving target,
// "Opus 5" is what actually answered. Falls back to the label when the alias has
// not resolved yet (fresh workspace) or the id is unparseable.
function labelForModel(info: CatalogModel, provider: string): string {
  return (
    formatModelVersion(info.resolvedModel ?? '') || stripTagline(info.label || info.id || provider)
  )
}

// resolveModelLabel returns a human label for an agent's effective model: the
// configured model's catalog label (or its raw id when it's a custom value).
// When the agent left the model empty, the catalog's own empty-id entry answers
// — it carries both the honest name of that mode ("claude oturum modeli") and,
// once observed, the model the provider actually served. Falls back to the raw
// concrete model id when the catalog isn't loaded yet; an empty model stays an
// explicit unknown instead of borrowing the provider name.
export function resolveModelLabel(
  catalog: CatalogEntry[],
  provider: string,
  model: string,
): string {
  const entry = catalog.find((c) => c.id === provider)
  if (!entry) return model ? formatModelVersion(model) || model : '(model belirtilmemiş)'
  const exact = entry.models.find((m) => m.id === model)
  if (exact) return labelForModel(exact, provider)
  // A custom model id not present in the curated list. It is already concrete, so
  // it needs no resolution — only prettying, when we recognise the family.
  if (model) return formatModelVersion(model) || model
  // Empty model and no empty-id catalog entry to explain it: say exactly that.
  // Falling back to the catalog's FIRST entry used to print a model the agent was
  // never configured with — a plausible lie is worse than an admitted gap.
  return '(model belirtilmemiş)'
}
