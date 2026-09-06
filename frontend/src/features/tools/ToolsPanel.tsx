import { Search, Plug, Check, Ban } from 'lucide-react'
import { VISIBILITY_TIERS, toolLabel } from './toolMeta'
import { toolIcon } from '@/shared/lib/toolIcons'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader } from '@/shared/components'
import { useToolsPanelState } from './useToolsPanelState'
import { VisibilityBadge } from './VisibilityControls'
import { ToolDetail } from './ToolDetail'
import { ServerManagement } from './ServerManagement'

interface Props {
  onError: (msg: string) => void
  // Deep-linked tool group (#/w/{ws}/tools/{group}); undefined = uncontrolled.
  group?: string | null
  onGroupChange?: (key: string | null) => void
}

// ToolsPanel is the workspace-wide tools screen. The left column lists every
// available tool (built-ins + tools from enabled MCP servers), grouped by
// origin and filterable; clicking a tool shows its details on the right, where
// it can be activated/deactivated for the whole workspace. The right column also
// hosts MCP server management when no tool is selected.
export function ToolsPanel({ onError, group, onGroupChange }: Props) {
  const {
    servers,
    testing,
    testResult,
    name,
    setName,
    transport,
    setTransport,
    command,
    setCommand,
    argsText,
    setArgsText,
    url,
    setUrl,
    headersText,
    setHeadersText,
    importText,
    setImportText,
    importing,
    importMsg,
    tools,
    savingTool,
    visBusy,
    selectedName,
    setSelectedName,
    listOpen,
    toggleList,
    query,
    setQuery,
    visFilter,
    setVisFilter,
    statusFilter,
    setStatusFilter,
    toggleVisFilter,
    activeGroup,
    setActiveGroup,
    visibleGroups,
    toggleTool,
    setToolVisibility,
    setServerVisibility,
    scope,
    setScope,
    editingId,
    startEdit,
    cancelEdit,
    poolStats,
    addServer,
    importServers,
    toggleServer,
    testServer,
    removeServer,
    importable,
    addingImportable,
    loadImportable,
    addImportable,
    activeCount,
    filtered,
    filtersActive,
    groups,
    sel,
    orderedNames,
    bulkSetEnabled,
    bulkSetVisibility,
    selected,
    params,
  } = useToolsPanelState(onError, { group, onGroupChange })

  return (
    <div className="flex h-full min-h-0 flex-1">
      {/* Left: searchable, grouped tool list — full-height sibling column (like chat). */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionharness.toolsListWidth"
        defaultWidth={288}
        label="Araçlar"
        testId="tools-list-toggle"
      >
        <div className="border-b border-[var(--color-border)] p-3">
          {/* Prominent, clearly-clickable jump to MCP server management. */}
          <button
            data-testid="tools-mcp-servers"
            onClick={() => setSelectedName(null)}
            title="MCP sunucularını yönet"
            className={`mb-3 flex w-full items-center justify-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition ${
              !selected
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-text)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
            }`}
          >
            <Plug size={15} /> MCP Sunucuları
          </button>
          <div className="relative">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
            />
            <input
              data-testid="tools-search-input"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Araç ara…"
              className="w-full rounded-lg bg-[var(--color-surface-2)] py-2 pl-8 pr-3 text-sm outline-none"
            />
          </div>
          {/* Filters: visibility tiers (multi-select) + enabled/disabled status. */}
          <div className="mt-2 flex flex-wrap items-center gap-1">
            {VISIBILITY_TIERS.map((tier) => {
              const on = visFilter.has(tier.value)
              return (
                <button
                  key={tier.value}
                  data-testid="tools-filter-vis"
                  data-tier={tier.value}
                  data-on={on}
                  onClick={() => toggleVisFilter(tier.value)}
                  title={`Görünürlük: ${tier.label}`}
                  className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide transition"
                  style={
                    on
                      ? {
                          backgroundColor: `color-mix(in srgb, ${tier.color} 22%, transparent)`,
                          color: tier.color,
                        }
                      : {
                          backgroundColor: 'var(--color-surface-2)',
                          color: 'var(--color-text-dim)',
                        }
                  }
                >
                  {tier.label}
                </button>
              )
            })}
            <span className="mx-0.5 h-3 w-px bg-[var(--color-border)]" />
            {(['enabled', 'disabled'] as const).map((st) => {
              const on = statusFilter === st
              return (
                <button
                  key={st}
                  data-testid="tools-filter-status"
                  data-status={st}
                  data-on={on}
                  onClick={() => setStatusFilter((prev) => (prev === st ? 'all' : st))}
                  className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide transition ${
                    on
                      ? st === 'enabled'
                        ? 'bg-[color-mix(in_srgb,var(--color-success)_22%,transparent)] text-[var(--color-success)]'
                        : 'bg-[var(--color-surface-2)] text-[var(--color-text)] ring-1 ring-inset ring-[var(--color-border)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {st === 'enabled' ? 'Aktif' : 'Devre dışı'}
                </button>
              )
            })}
            {filtersActive && (
              <button
                data-testid="tools-filter-clear"
                onClick={() => {
                  setVisFilter(new Set())
                  setStatusFilter('all')
                }}
                title="Filtreleri temizle"
                className="rounded px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
              >
                Temizle
              </button>
            )}
          </div>
          <p className="mt-2 px-0.5 text-xs text-[var(--color-text-dim)]">
            {filtersActive ? `${filtered.length} eşleşme · ` : ''}
            {activeCount}/{tools.length} aktif
          </p>
        </div>

        {/* Group picker: one button per group, "Tümü" shows every group. Replaces
            the old fold-in/out accordion — a selected group narrows the list. */}
        {groups.length > 0 && (
          <div
            className="flex flex-wrap gap-1 border-b border-[var(--color-border)] px-2 py-2"
            role="group"
            aria-label="Araç grupları"
          >
            <button
              data-testid="tools-group-all"
              onClick={() => setActiveGroup(null)}
              aria-pressed={activeGroup === null}
              className={`rounded-full border px-2 py-0.5 text-[11px] transition ${
                activeGroup === null
                  ? 'border-transparent bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              Tümü
            </button>
            {groups.map((g) => {
              const on = activeGroup === g.key
              return (
                <button
                  key={g.key}
                  data-testid="tools-group-toggle"
                  data-group={g.key}
                  onClick={() => setActiveGroup(on ? null : g.key)}
                  aria-pressed={on}
                  className={`flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition ${
                    on
                      ? 'border-transparent bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                      : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                  }`}
                >
                  <span className="truncate">{g.label}</span>
                  <span className="tabular-nums opacity-70">{g.tools.length}</span>
                </button>
              )
            })}
          </div>
        )}

        <div className="flex-1 overflow-y-auto py-2">
          {visibleGroups.map((g) => {
            return (
              <div key={g.key} className="mb-1">
                {visibleGroups.length > 1 && (
                  <div className="flex items-center gap-1.5 px-3 py-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                    <span className="truncate">{g.label}</span>
                    <span className="ml-auto font-normal tabular-nums opacity-70">
                      {g.tools.length}
                    </span>
                  </div>
                )}
                {g.tools.map((t) => {
                  const active = selectedName === t.name
                  const ToolIcon = toolIcon(t.name)
                  return (
                    <button
                      key={t.name}
                      data-testid="tools-list-item"
                      data-tool-name={t.name}
                      onClick={(e) => {
                        if (sel.handleClick(e, t.name, orderedNames, selectedName)) return
                        setSelectedName(t.name)
                      }}
                      className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition ${
                        sel.isSelected(t.name)
                          ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)] ring-1 ring-inset ring-[var(--color-accent)]'
                          : active
                            ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                            : 'hover:bg-[var(--color-surface-2)]'
                      }`}
                    >
                      <span
                        title={t.enabled ? 'Aktif' : 'Devre dışı'}
                        className={`h-1.5 w-1.5 flex-shrink-0 rounded-full ${
                          t.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--color-border)]'
                        }`}
                      />
                      <ToolIcon
                        size={14}
                        className={`shrink-0 ${active ? '' : 'text-[var(--color-text-dim)]'}`}
                      />
                      <span
                        className={`min-w-0 truncate ${t.enabled ? '' : 'text-[var(--color-text-dim)]'}`}
                      >
                        {toolLabel(t)}
                      </span>
                      <VisibilityBadge visibility={t.visibility} className="ml-auto" />
                    </button>
                  )
                })}
              </div>
            )
          })}
          {groups.length === 0 && (
            <p className="px-3 py-3 text-sm text-[var(--color-text-dim)]">
              {tools.length === 0 ? 'Henüz araç yok.' : 'Eşleşen araç yok.'}
            </p>
          )}
        </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedNames.length ? () => sel.selectAll(orderedNames) : undefined}
        >
          <SelectionBarButton icon={<Check size={13} />} onClick={() => bulkSetEnabled(true)}>
            Etkinleştir
          </SelectionBarButton>
          <SelectionBarButton icon={<Ban size={13} />} onClick={() => bulkSetEnabled(false)}>
            Devre dışı
          </SelectionBarButton>
          {VISIBILITY_TIERS.map((tier) => (
            <SelectionBarButton key={tier.value} onClick={() => bulkSetVisibility(tier.value)}>
              {tier.label}
            </SelectionBarButton>
          ))}
        </SelectionBar>
      </ListPane>

      {/* Right column: title bar + selected tool detail / MCP server management.
          The title bar sits ONLY here, to the right of the list — like the chat header. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title="Araçlar & MCP"
          subtitle={selected ? `· ${selected.name}` : undefined}
          listOpen={listOpen}
          onToggleList={toggleList}
        />
        <div className="flex-1 overflow-y-auto p-6">
          {selected ? (
            <ToolDetail
              tool={selected}
              params={params}
              saving={savingTool === selected.name}
              visBusy={visBusy === selected.name}
              onToggle={() => toggleTool(selected)}
              onSetVisibility={(tier) => setToolVisibility(selected, tier)}
            />
          ) : (
            <ServerManagement
              servers={servers}
              tools={tools}
              onServerVisibility={setServerVisibility}
              testing={testing}
              testResult={testResult}
              name={name}
              setName={setName}
              transport={transport}
              setTransport={setTransport}
              command={command}
              setCommand={setCommand}
              argsText={argsText}
              setArgsText={setArgsText}
              url={url}
              setUrl={setUrl}
              headersText={headersText}
              setHeadersText={setHeadersText}
              scope={scope}
              setScope={setScope}
              editingId={editingId}
              onEdit={startEdit}
              onCancelEdit={cancelEdit}
              poolStats={poolStats}
              onAdd={addServer}
              onToggle={toggleServer}
              onTest={testServer}
              onRemove={removeServer}
              importText={importText}
              setImportText={setImportText}
              importing={importing}
              importMsg={importMsg}
              onImport={importServers}
              importable={importable}
              addingImportable={addingImportable}
              onLoadImportable={loadImportable}
              onAddImportable={addImportable}
            />
          )}
        </div>
      </div>
    </div>
  )
}
