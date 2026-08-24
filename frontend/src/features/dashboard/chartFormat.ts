// Number/currency formatters for the dashboard charts. Split out so charts.tsx
// exports only components (fast refresh).
export function compact(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${Math.round(n / 100) / 10}k`
  return `${Math.round(n / 100_000) / 10}M`
}

// fmtUsd renders a USD amount for a chart total or tile. Sub-cent figures keep
// three decimals so a busy-but-cheap window does not round to "$0.00"; large ones
// compact to $1.2k so a header stays short. estimated prefixes "~" (subscription
// providers are priced by an equivalent-API estimate, not a real invoice).
export function fmtUsd(n: number, estimated = false): string {
  const p = estimated ? '~$' : '$'
  if (n > 0 && n < 0.01) return `${p}${n.toFixed(3)}`
  if (n < 1000) return `${p}${n.toFixed(2)}`
  return `${p}${compact(n)}`
}

// DeltaBadge shows the period-over-period change as a coloured arrow. invert flips
// the colour meaning for metrics where "up" is the bad direction (cost): an
// increase there is red, a decrease green. A null pct (no baseline) reads as
// "yeni" when there is new activity, and renders nothing when both periods were
// empty — a badge that is always there stops being a signal.
