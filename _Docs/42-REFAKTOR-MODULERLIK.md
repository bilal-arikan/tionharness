# 42 — Refaktör: Modülerlik / Soyutlama / Generic Yapı

**Tarih:** 2026-07-01
**Amaç:** Kod tabanını daha **modüler, soyutlanabilir ve generic** hale getirmek. Kötü mimariden değil, **ölçekten** kaynaklanan tekrar (duplication) ve tanrı-dosyalar (God files) hedeflendi. Davranış değişmedi — saf refaktör.

**Doğrulama:** `go build ./...` yeşil · **32 pakette 572 test geçiyor** · frontend `tsc` + `vite build` yeşil · tüm yeni/düzenlenen Go dosyaları gofmt-temiz.

## Faz 1 — Tekrar → Generic (düşük risk, yüksek ROI)

### db: generic entity store helper'ları
- Yeni `internal/db/filestore.go`: generic serbest fonksiyonlar
  `dbGet[T]` · `dbList[T]` · `dbFilter[T]` · `dbPersistLocked[T]` · `dbDeleteLocked[T]`.
- **Neden serbest fonksiyon, neden `EntityStore[T]` struct değil:** tüm entity map'leri **tek `d.mu`** ve tek disk kökünü paylaşır; per-store mutex bu değişmezi bozardı. Map açıkça geçilerek tip güvenliği korunur.
- Delegasyona çevrilen store'lar: `store_task.go` · `store_hook.go` · `store_flow.go` (flow + flowRun) · `store_schedule.go` · `store_mcp.go`. Get/List/Filter/persist/delete tekrarları eritildi; paylaşılan mutex ve cascade mantığı aynen korundu.

### api: generic HTTP handler helper'ları
- Yeni `internal/api/httphelpers.go`: `bindJSON[T]` (decode → 400 → `ok=false`) + `requireFields` (label/değer çiftleri, ilk boşta 400).
- **54 handler** (28 dosya) `var req…; decodeJSON…; writeError…` kalıbından `bindJSON` tek satırına migrate edildi (mevcut `writeJSON`/`writeDBError` ile simetrik: biri response, diğeri request tarafını merkezîleştirir).

### providers: `NewBuiltinKind`
- `internal/providers/kind.go`: data-driven `basicKind` + `NewBuiltinKind(manifest, available, build)`.
- **5 kind_*.go** (anthropic/claude-cli/minimax/minimax-anthropic/openrouter) her biri kendi struct tipi + 3 metot yerine tek self-registering `init()`'e indirgendi. Yeni transport = tek `init()` bloğu.

## Faz 2 — Ortak yapı taşları + God-fonksiyon bölme

### tools: `parseInput` + `NewFuncTool`
- Yeni `internal/tools/toolbuilder.go`: generic `parseInput[T](tool, input)` (unmarshal + `argErrFor`), `NewFuncTool(def, fn)` (state'siz araçlar için funcTool), paylaşılan `schemaEmpty`.
- **12 site** `var in…; json.Unmarshal…; argErrFor…` üçlüsünden `parseInput` tek çağrısına migrate.

### agent: `selfManageBuiltins`
- `buildRegistry`'nin ~116 satırlık self-management tool bloğu yeni `internal/agent/toolsetup_selfmanage.go`'ya `(*Runtime).selfManageBuiltins(agent)` olarak taşındı. `buildRegistry` artık okunur bir taslak: core tools → self-manage suite → görünürlük katmanları → MCP. HIDDEN tier işaretleme aralığı (`selfManageStart`) korundu.

## Faz 3 — God-dosya bölme

### api (aynı-paket fonksiyon taşıma → sıfır mantık değişikliği)
- `market.go` (877 sat) → `market.go` (route + registry + catalog) · `market_install.go` (install pipeline) · `market_publish.go` (publish/import/template).
- `mcp_interaction.go` (727 sat) → `mcp_interaction.go` (dispatch + advertisement core) · `mcp_interaction_tools.go` (14 adet `call*` handler).
- İçe aktarımlar `goimports` ile otomatik düzeltildi.

### frontend (ayrı ajanla, izole)
- **Faz 1–2 tam:** `lib/format.ts` (usd/tokens/bytes/percent — BudgetPanel, SessionDetailPanel, ChatMeters), `hooks/useAsync.ts` (polling + unmount-safe; BudgetPanel + ExecutionsPanel migrate), `hooks/useGroupedList.ts` (SkillsPanel migrate), `hooks/useEscapeKey.ts` + `components/common/ModalOverlay.tsx` (TaskFormModal migrate), `components/common/KeyValueRow.tsx`.
- **appPanels.tsx (871 sat)** panel-başına dosyaya bölündü; `appPanels.tsx` artık **re-export barrel** (mevcut import'lar değişmedi): Profile/Notifications/Appearance/Context/Autonomy/AppTools/Backup/About + `settingsPanelShared.tsx`.

### frontend Faz 2 yayma (2026-07-01, ikinci tur)
Ortak parçalar davranış-güvenli olan panellere yayıldı (build yeşil). **Migrate:** `MemoryGraphView` → `useAsync` · `SecretsPanel` → `useAsync` · `WorkspaceCreateModal` → `ModalOverlay`.
**Bilinçli atlananlar (gözlemlenebilir davranış farkı):** optimistik-mutasyonlu listeler (`ArtifactsPanel`/`Schedules`/`MemoryPanel`) read-only `useAsync`'e uymaz · `HooksPanel` refetch'te `loading` set etmez (useAsync paneli "Yükleniyor…"e çevirirdi) · `NetworkPanel`/`RegistryManager`/`SessionDebugCard`/`AgentActivityPanel` çift-fetch veya özel hata/poll semantiği · modaller `onClick`+`stopPropagation` (ModalOverlay `onMouseDown`) veya farklı backdrop opaklığı (`bg-black/40`) · `Lightbox`/`EmojiPicker` özel overlay/popover. **Sonuç:** doğru atlama = korunmuş davranış; yayma panel-panel, ihtiyaç oldukça sürer.

## Bilinçli atlananlar (gerekçeyle)
- **Context key'lerini tek dosyada toplamak:** zaten feature-başına ayrı dosyalarda (callkind.go/workdir_ctx.go/turnmeta.go…) ve kohezyonlu — toplamak kötüleştirirdi.
- **`policies.go`'da magic-sayı merkezîleştirme:** sabitler zaten kullanım yerinde dokümanlı; taşımak locality'yi bozar, marjinal fayda düşük.
- **`Runtime` struct facade ayrımı:** çok yüksek risk/düşük marjinal fayda; God-fonksiyon derdi `selfManageBuiltins` + api bölmeleriyle hafifletildi.
- **frontend App.tsx / useChatStream / büyük paneller (Market/Tools/Flows):** derinden bağlı (paylaşılan ref/setter ağı); kısmi çıkarım coupling'i artırırdı. Green tree'yi bozmamak için durdu.

## Sırada (opsiyonel devam)
- Faz 3'ün kalan yüksek-riskli parçaları için ayrı, dar kapsamlı seanslar: `Runtime` facade ayrımı; `App.tsx` context'i (`AppStateContext`) + `useChatStream` alt-hook'ları — her biri tek başına bir iş.
- Provider tarafında OpenAI-uyumlu (minimax/openrouter/custom) Build yollarının tek `BuildOpenAICompatKind` helper'ında birleştirilmesi; model metadata'sının (context-window/pricing/max-output) tek `ModelRegistry` kaynağında toplanması.
