# 54 — Capability Probe → Context Genişletme (+ per-workspace codebase-memory store)

> Generic bir katman: cihazda/işlemde bir **opsiyonel harici yetenek** mevcutsa,
> ajanın **cachelenebilir statik** sistem-prompt prefix'ine kısa bir bilgi bloğu
> enjekte edilir. Müşteriler: `codebase-memory-mcp` (kod bilgi-grafiği) ve
> **token-optimizers** (`rtk`/`sqz`, 2026-07-10).

## Token-optimizer capability (rtk / sqz) — 2026-07-10

`internal/agent/capabilities_tokenopt.go`: workspace'in **enabled** Pre/PostToolUse
hook'larını tarayıp `rtk` / `sqz` işaretçilerini (`\brtk\b` / `\bsqz\b`, kelime-sınırlı)
arar. Bulunursa statik prefix'e "# Token optimization active" bloğu enjekte edilir —
ajan bu optimizer'ların **hook ile otomatik** çalıştığını (kendisi çağırmaz) ve gördüğü
araç çıktısının kısaltılmış olabileceğini (veri kaybı değil) bilir. **Tespit hook
KOMUTUNDANDIR**, yalnız PATH'ten değil (PATH'te duran ama hook'a bağlanmamış bir ikili
hiçbir şey yapmaz → over-claim edilmez). Blok, hook'ların **matcher kapsamını** da
belirtir: WS5'te iki hook da `matcher=Bash` olduğundan, `PowerShell` aracıyla çalışan
komutlar optimize EDİLMEZ → blok "eşleşen aracı tercih et" der (kritik: bu yüzden bir
`PowerShell`-ağırlıklı oturumda ikisi de hiç ateşlenmemişti). `rtk` agent-Bash ile,
`sqz` PostToolUse çıkış sıkıştırmasıyla; ikisinin de mekaniği `17-TOKEN-OPTIMIZASYON.md`.
Test: `capabilities_tokenopt_test.go` (WS5-şekli + kelime-sınırı + wildcard).

## Amaç

- Ajan, workspace'te hangi opsiyonel araçların **var olduğunu** promptundan bilsin
  ve onları kullansın (grep yerine kod bilgi-grafiği gibi).
- Tespit + genişletme **generic** olsun: yeni bir tool eklemek = tek bir `Capability`
  kaydı. Assembler kodu değişmez.
- codebase-memory için: her workspace **izole** bir indeks store kullansın (store =
  workspace sınırı), cwd repo'su ilk kullanımda otomatik indekslensin.

## Bileşenler

### 1. Generic katman — `internal/agent/capabilities.go`

```go
type Capability struct {
    ID      string
    Detect  func(ctx, r *Runtime) bool                 // ucuz probe
    Context func(ctx, r *Runtime, cwd string) string   // enjekte edilecek blok
}
var capabilities = []Capability{ codebaseMemoryCapability }
func (r *Runtime) CapabilityContext(ctx, cwd) string   // mevcut olanların bloklarını birleştirir
```

- `Detect` **herhangi bir sinyal** olabilir: MCP-sunucu varlığı, `exec.LookPath`
  (on-PATH binary), settings-flag. Yeni tool = slice'a bir `Capability`.
- `CapabilityContext` "" dönerse assembler için güvenli no-op. Sonuç sabit →
  prompt-cache bozulmaz.

### 2. codebase-memory capability

- **Tespit:** `r.db.ListEnabledMCPServers` içinde `Transport=stdio` + `Command`'da
  `codebase-memory-mcp` işareti (`codebaseMemoryCommand`). MCP modelinde slug yok →
  Command üzerinden.
- **Blok:** kısa; araç-tercihi yönlendirmesi (`search_code`/`search_graph`/
  `get_code_snippet`/`query_graph`/`trace_path`/`get_architecture`) + izole store notu
  + cwd'den türetilen `project` id.
- **`projectIDForPath`:** path→id kuralını codebase-memory ile birebir taklit eder
  (ayraç+`:` → `-`, `[A-Za-z0-9-]` dışı düşer). İki gerçek örnekle test edildi; ıskalarsa
  blok ajanı `list_projects`'e yönlendirir (hedge).

### 3. Enjeksiyon — iki senkron assembler (mevcut desen)

| Yol | Yer | İçerik |
|-----|-----|--------|
| Chat | `api.composeTurnRequest` (statik `system` prefix) | cwd/project id'li tam blok |
| Headless | `agent.autonomousSystemPrompt` | cwd-siz farkındalık (assembler per-agent) |

`GoalUsageHint` / `EnvironmentContextBlock` ile aynı "tek-kaynak paylaşılan blok"
desenini izler. Statik prefix'te çünkü **varlık oturum boyunca sabit** →
cache-dostu.

### 4. Per-workspace store (§C)

- **`(*Runtime).CBMStoreDir()`** = `filepath.Join(filepath.Dir(workDir), "cbm-store")`
  — `skills/`, `hook-scripts/` ile aynı kardeş-dizin deseni. `workDir` boşsa "" →
  sunucunun default store'una düşülür.
- **`toolsetup.go`:** `cfgs` kurulurken codebase-memory sunucusunun stdio env'ine
  `CBM_CACHE_DIR = CBMStoreDir()` enjekte edilir (kullanıcı elle set etmişse o kazanır).
  `mcp.ServerConfig.Env` → `DialStdio` env'i → sunucu yalnız bu store'u görür.

### 5. Auto-index (§C)

- **`(*Runtime).EnsureCodebaseIndexed(ctx, cwd)`** — best-effort, arka planda
  `command cli index_repository {repo_path}` + `CBM_CACHE_DIR`. `(cwd,store)` başına
  **süreç-içi tek sefer** (`cbmIndexed sync.Map`). Chat turunda `session.WorkingDir`
  ile tetiklenir.
- **Hata yutulmaz:** başarısızlık `logger.Warn` ile loglanır ve guard silinir →
  sonraki tur retry edebilir.

## Cache güvenliği

- Bloklar statik prefix'te ama **sunucu yokken "" **→ mevcut promptlar hiç değişmez
  (cachebreak testleri yeşil kalır).
- Presence + cwd oturum içinde stabil → blok string'i sabit → prompt-cache READ olarak
  kalır. cwd nadiren değişirse cache bir kez tazelenir (kabul edilebilir).

## Genişletme (ileride)

Yeni bir harici tool için `capabilities.go`'ya tek bir `Capability` eklemek yeter;
`Detect` `exec.LookPath("...")` veya settings-flag olabilir. Assembler/enjeksiyon
noktaları değişmez.

## Workspace-geneli metin araması — `codebase_workspace_search`

`search_code` project-scoped; workspace-geneli metin araması için fan-out köprü tool:
`internal/tools/builtin_codebase_search.go` `CodebaseWorkspaceSearchTool`.

- **Akış:** `<command> cli list_projects` → store'daki her project için `<command> cli
  search_code {project,pattern,mode,limit}` → sonuçlar project'e göre birleşir.
- **Sınırlar:** toplam sonuç `limit` (vars. 30), fan-out `maxCodebaseProjects` (40);
  boş-sonuç project'ler atlanır, bir project'in hatası tüm aramayı düşürmez.
- **Bağlama:** `toolsetup.go` — yalnız enabled codebase-memory sunucusu varsa
  (`r.codebaseMemoryCmd(ctx) != ""`), `CBMStoreDir()` ile aynı store'a. Risk `RiskRead`
  (classify.go) → read-only modda da açık; kategori `CategorySearch`.
- **Not:** graf/mimari sorguları (`get_architecture`, `query_graph`) zaten native
  fleet; bu tool yalnız metin-arama boşluğunu doldurur.

## Frontend — "izole store" rozeti

`ToolsPanel.tsx` MCP sunucu satırında, `command` içinde `codebase-memory-mcp` geçen
sunucuya **"izole store"** rozeti + tooltip (`CBM_CACHE_DIR = <workspace>/cbm-store`).
`data-testid="mcp-server-isolated-store"`.

## Aç/kapa — workspace toggle

Tüm codebase-memory yeteneği bir workspace ayarıyla açılıp kapanır (**default açık**):

- **Ayar:** `WSSettings.CodebaseMemoryEnabled` (`workspace/settings.go`, json
  `codebaseMemoryEnabled`) — default seed `true`, patch `*bool`, `loadSettings` +
  `UpdateSettings` → `Runtime.SetCodebaseMemory`.
- **Runtime gate:** `runtime.go` `codebaseMemoryEnabled atomic.Bool` (NewRuntime'da
  `true` tohum) + `CodebaseMemoryEnabled()`/`SetCodebaseMemory`.
- **Tek choke-point:** `codebaseMemoryCmd(ctx)` kapalıyken `""` döner → hint bloğu
  (Detect), auto-index (EnsureCodebaseIndexed) ve `codebase_workspace_search` kaydı
  birlikte kaybolur. Env enjeksiyonu da `toolsetup`'ta `CodebaseMemoryEnabled()` ile
  gate'li (kapalı → sunucu kendi default store'unda, tamamen vanilla).
- **API:** `workspaceSettingsDTO` (GET/PUT `/api/workspace-settings`) alanı taşır —
  **DTO'ya eklenmezse patch kaydedilir ama UI hep boş görür** (smoke test bunu yakaladı).
- **UI:** `WorkspacePanel.tsx` "Kod bilgi-grafiği" bölümünde Toggle; harici-tools
  ekranında (`ExternalToolsPanel.tsx`) bilgilendirici callout ("Ayarlar ▸ Bu Workspace'ten
  aç/kapa"). Frontend tip + patch + `WorkspaceView` mapping güncellendi.
  - **Callout artık `codebase-memory-mcp` tool satırının altına gömülü** (üstteki
    bağımsız kutu kaldırıldı) ve içine **tek-tık MCP aç/kapa** butonu eklendi
    (`data-testid="cbm-mcp-toggle"`): sunucu ekli değilse tespit edilen PATH exe'siyle
    `createMCPServer({name:"codebase-memory-mcp", transport:"stdio", command:<path>})`,
    ekliyse `deleteMCPServer(id)`. Ekli/eksik durumu MCP listesinden komut-marker'ıyla
    türetilir (backend'in izole-store yönlendirmesiyle aynı kural). Buton araç PATH'te
    bulunmazsa devre dışı.

## Workspace oluşturma sonrası öneri kartları (2026-07-13)

`WorkspaceRecommendations.tsx` — `ClaudeAuthGate` ile aynı desende, App'te ayrı bir
`recsTrigger` sayacıyla **yalnız create/attach başarısında** tetiklenir (chat-open'da
DEĞİL → kullanıcı her sohbet açılışında rahatsız edilmez).

**Mimari — kural motoru:** modül-seviyesi düz `RULES: Rule[]` dizisi; her kural saf bir
`(ctx) => Rec | null` fonksiyonu. Tetikte tek bir probe (`external-tools` + `mcp-servers`
+ `hooks` + `workspace-settings` + `app-settings` + `agents`) paralel çekilir, `ctx`
kurulur, her kural bir kez koşar ve null-olmayanlar kapatılabilir sağ-alt kart olur.
**Yeni öneri = tek `RULES` girdisi.** Kart varyantı `accent`|`warning`.

Kurallar (sırayla — kartlar üstten alta yığılır):
- **token-conflict** (⚠): rtk+sqz ikisi de enabled → "Harici araçlar"a yönlendir.
- **no-agents**: 0 ajan → Ajanlar ekranı.
- **workdir**: `defaultWorkingDir` boş → "Bu Workspace" (path native picker ister).
- **cbm-add**: codebase-memory-mcp kurulu ama MCP yok → `createMCPServer(...)`.
- **cbm-enable**: MCP ekli ama `codebaseMemoryEnabled=false` → `updateWorkspaceSettings`.
- **token**: rtk (yoksa sqz) kurulu ama hiç token-hook yok → `createHook(...)`.
- **no-mcp**: hiç MCP yok (ve cbm bekleyen öneri değilse) → Market.
- **backup-off**: `backupEnabled=false` → Yedekleme ayarları.
- **cli-tools**: PATH'te `wire=cli` araçlar (mmdc/crabbox) → tek bilgi kartı, Harici Araçlar.

Yeni algılama endpoint'i eklenmedi; hepsi mevcut endpoint'leri tüketir. Kartlar
`data-testid="workspace-rec-<key>"`, aksiyon `workspace-rec-act-<key>`.

**Paylaşılan katalog:** kural motoru `recommendations.ts`'e taşındı — her kural statik
`meta` (key/icon/title/summary) + `detect(ctx)` taşır. İki yüzey aynı kataloğu kullanır:
toast (`WorkspaceRecommendations.tsx`) ve **"Öneriler" workspace sekmesi**
(`RecommendationsPanel.tsx`).

**Ignore + yönetim (kalıcı):** Toast'ta "X" artık kartı **kalıcı yok sayar** — key,
per-workspace `WSSettings.IgnoredRecommendations` listesine yazılır (`ignoredRecommendations`
DTO + patch alanı; runtime etkisi yok, saf UI). Bir aksiyon **uygulanınca** ignore
yazılmaz (koşul zaten çözülür). WorkspaceView ▸ **Öneriler** sekmesi tüm kuralları
listeler: her biri için "Şu an geçerli / Yok sayıldı / Uygulanabilir değil" rozeti +
**Yok say / Yok saymayı kaldır** düğmesi (`updateWorkspaceSettings({ignoredRecommendations})`).
Test id'leri `rec-row-<key>` / `rec-toggle-<key>`.

**Manuel gösterme:** Panelde **"Kartları göster"** düğmesi (`rec-show-cards`) App'teki
`recsTrigger`'ı bumlar → toast'ı istek üzerine tekrar açar (create beklemeden). Geçerli
ve yok sayılmamış öneri yoksa devre dışı (sayacı buton üstünde gösterir). Toast App
seviyesinde her view'da mount olduğu için workspace ekranında da görünür.

## Canlı duman testi (2026-07-07)

- **CLI kontratı:** izole temp store → `index_repository` (10056 node) → `list_projects`
  (yalnız TionSwarm → izolasyon ✓) → `search_code` fan-out (sonuç döndü). `EnsureCodebaseIndexed`
  + `CodebaseWorkspaceSearchTool`'un dayandığı JSON kontratı doğrulandı.
- **Uygulama boot:** binary izole `TIONSWARM_DATA_DIR` + loopback portta panic'siz boot etti.
- **Toggle round-trip (canlı API):** default `true` → PUT `false` → re-GET `false` (kalıcı) →
  PUT `true`. DTO düzeltmesi bu testte ortaya çıktı ve giderildi.

## Dosyalar

- `internal/agent/capabilities.go` (yeni: Capability + codebase-memory + `sessionCwd`
  + `codebaseMemoryCmd` + `EnsureCodebaseIndexed`) · `capabilities_test.go` (yeni)
- `internal/agent/runtime.go` (`cbmIndexed` alanı + headless enjeksiyon, `autonomousSystemPrompt(ctx,a)`)
- `internal/agent/executor.go` · `subagent.go` (ctx'li çağrı)
- `internal/agent/toolsetup.go` (`CBM_CACHE_DIR` env enjeksiyonu + tool kaydı)
- `internal/api/chat_turn.go` (chat enjeksiyon + `EnsureCodebaseIndexed`)
- `internal/tools/builtin_codebase_search.go` (+test) · `classify.go` · `categories.go`
- `frontend/src/components/panels/ToolsPanel.tsx` (izole store rozeti)
- **Toggle:** `internal/workspace/settings.go` (`CodebaseMemoryEnabled` alan/default/patch/apply +test)
  · `internal/agent/runtime.go` (atomic + `Set/CodebaseMemoryEnabled`) · `internal/agent/toolsetup.go`
  + `capabilities.go` (gate) · `internal/api/workspace_settings.go` (DTO alanı) ·
  `frontend/.../WorkspacePanel.tsx` (Toggle) · `ExternalToolsPanel.tsx` (callout) ·
  `types/workspace.ts` + `WorkspaceView.tsx` (tip/patch/mapping)

## Sıradaki adımlar (opsiyonel)

- `codebase_workspace_search` fan-out'unu paralelleştir (şu an sıralı; project sayısı
  arttıkça hızlanır).
- Mevcut default-store'daki eski TionSwarm kopyasının temizliği (çok-workspace geçişte).
- Fan-out'u CLI-shell yerine canlı MCP pool üzerinden (process/SQLite contention'ı azaltır).
