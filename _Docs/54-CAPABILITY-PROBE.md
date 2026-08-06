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
- **Blok:** kısa; araç-tercihi yönlendirmesi — **tam namespaced adlarla**
  (`<server>__search_code`, `<server>__search_graph`, `<server>__get_code_snippet`,
  `<server>__query_graph`, `<server>__trace_path`, `<server>__get_architecture`,
  `<server>__index_repository`; server adı canlı MCP satırından, `codebaseMemoryServerName`)
  + izole store notu + cwd'den türetilen `project` id. Çıplak adlar (sadece
  `search_code`) **bilerek kullanılmaz** — model yanlış namespace tahmin edip
  (`codebase_memory__search_code`) "no server" hatası alıyordu; tam ad yazınca tahmin
  sıfırlanır. Sunucu satırı yoksa blok üretilmez.
- **`projectIDForPath`:** path→id kuralını codebase-memory ile birebir taklit eder
  (ayraç+`:` → `-`, `[A-Za-z0-9-]` dışı düşer). İki gerçek örnekle test edildi; ıskalarsa
  blok ajanı `list_projects`'e yönlendirir (hedge).
- **Namespace hatası kurtarma:** model yine de yanlış namespace'li bir ad üretirse
  (ör. `codebase_memory__search_code`), registry `SuggestServers` ile yakın eşleşme
  önerir: hataya `; did you mean codebase-memory-mcp?` eklenir (eşik 0.5 benzerlik,
  en iyi 3, case-insensitive Levenshtein). Hem `manager.go` (registry yolu) hem
  `pool.go` (gateway yolu) bu öneriyi verir — bkz. `internal/mcp/suggest_test.go`.

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
- **tool-update** (⚠): kurulu bir aracın daha yeni sürümü yayımlanmış → Harici Araçlar.
- **cli-tools**: PATH'te `wire=cli` araçlar (mmdc) → tek bilgi kartı, Harici Araçlar.

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
- **Sürüm/güncelleme (2026-08-01):** `internal/exttools/{catalog,version,compare,release,update}.go`
  (+ `{version,compare,release}_test.go`) · `internal/api/external_tools.go` (yeniden yazıldı,
  katalog taşındı) · `internal/api/server.go` (2 yeni rota) · `frontend/src/types/settings.ts`
  (`ExternalToolUpdate`/`ExternalToolUpdateResult`) · `api/system.ts` · `ExternalToolsPanel.tsx`

## Harici araç sürüm + güncelleme kontrolü — `internal/exttools` (2026-08-01)

Katalog (eskiden `internal/api.knownExternalTools`) kendi paketine taşındı; `api`
yalnız HTTP katmanı olarak kaldı. Girdi başına iki yeni alan:

```go
VersionArgs []string   // ör. {"--version"}; boş = sürüm okunamaz
Update      UpdateSpec // Kind: "command" | "manual"
```

- **`Tool.Repo()`** GitHub slug'ını **URL'den türetir** (ikinci bir alan tutulmaz
  → sapamaz). `ffmpeg` GitHub'da olmadığı için `""` → release kontrolü yok.
- **`LocalVersion`** aracı `--version` ile çalıştırır (3 sn timeout, `proc.TreeKill`,
  `HardenedEnv`). Paketin geri kalanı hiçbir şey çalıştırmaz; bu **bilinçli**
  istisnadır — sürüm bayrağı yan etkisizdir ve timeout, bayrağı tanımayıp stdio
  MCP sunucusu olarak beklemeye geçecek `codebase-memory-mcp`'yi de keser.
  stdout+stderr birleşik okunur (ffmpeg sürümü stderr'e yazar), çıkış kodu
  sürüm bulunduysa yok sayılır (bazı araçlar bilinmeyen bayrakta 1 döner ama
  yine de sürümü basar).
- **`LatestRelease`** `releases/latest` + **6 sa disk cache**
  (`<dataDir>/cache/exttools-releases.json`, tmp→rename). Kimliksiz GitHub limiti
  saatte 60; 7 araçla cache olmadan birkaç tıkta biterdi. Ağ/limit hatasında
  **fail-open** → bayat cache `stale=true` ile döner; cache yoksa hata.
- **`Compare`** ayrıştıramadığında **`unknown`** — tahmin yok. Yerel sürüm
  ileriyse `up-to-date` (dev build "eski" gösterilmez).

### Güncelleme neden sadece kısmen otomatik

| Kind | Araçlar | Neden |
|------|---------|-------|
| `command` | `mmdc` (npm), `ffmpeg` (winget), `git` (winget `Git.Git`), `bun` (winget `Oven-sh.Bun`), `npm` (`npm i -g npm@latest`) | Paket yöneticisi kurulum dizinini ve çalışan ikiliyi kendi yönetir |
| `manual` | claude, rtk, sqz, codebase-memory-mcp, piper, whisper-cli | İkiliyi/arşivi **yerinde değiştirmek** gerekir; Windows'ta çalışan alt-süreç (MCP stdio sunucusu kendi `.exe`'sini, süren bir claude-cli turu `claude`'u) dosyayı kilitler → yarım kalan kopya aracı geri dönüşsüz bozar |

`RunUpdate` komutu **katalogdan** alır, istekten değil → enjeksiyon yolu yok.
5 dk timeout + `TreeKill`; `HardenedEnv` sayesinde soru soracak bir paket
yöneticisi asılmak yerine hızlı başarısız olur.

### API

| Endpoint | İş |
|----------|-----|
| `GET /api/external-tools` | tespit + sürüm (probe'lar paralel) + `updateKind`/`updateCommand`/`updateNote` |
| `POST /api/external-tools/check-updates` | GitHub karşılaştırması; `?refresh=1` cache'i atlar |
| `POST /api/external-tools/{name}/update` | yalnız `command` araçları; `manual` → **409** + talimat |

Güncelleme **ajan aracı olarak açılmadı**: ajan koştuğu makineyi sessizce
değiştirmemeli. Yol `isWorkspaceExempt` kapsamında (workspace-bağımsız).

### UI

`ExternalToolsPanel`: satırda `v<sürüm>` çipi (`data-testid="tool-version"`),
güncelleme varsa release'e giden `↑ <tag>` rozeti (`tool-outdated`), `command`
araçlarda **Güncelle** (`tool-update`) + çıktı kutusu, `manual` araçlarda talimat
callout'u (`tool-manual-update`), komut kopyalama (`tool-copy-update-cmd`).
Üstte "Güncellemeleri kontrol et" (`ext-tools-check-updates`) + "Önbelleği atla"
(`ext-tools-refresh-updates`). Release kontrolü bu panelde **açılışta çalışmaz** —
ağa çıkar; tespit anında, karşılaştırma istek üzerine.

### `claude` katalog girdisi + yol geçersiz kılma (2026-08-01)

Claude Code CLI de katalogda (`exttools.ClaudeToolName = "claude"` — ikilinin adı,
sağlayıcı id'si `claude-cli` değil). Kategori `provider`, `Wire: "provider"` →
panelde **Sağlayıcı** rozeti. Katalogdaki tek **çekirdek** bağımlılıktır (anahtarsız
`claude-cli` sağlayıcısı bu ikilidir); yine de burada listelenir çünkü bir claude-cli
ajanı bozulduğunda sorulan sorular tam olarak bu panelin cevapladıklarıdır
(nerede · hangi sürüm · güncel mi). Güncelleme `manual`: Claude Code zaten kendini
arka planda günceller, elle yol `claude update` / `npm i -g @anthropic-ai/claude-code`.

**`SetPathOverride(name, path)`** (`catalog.go`): `Detect` önce bu haritaya bakar.
`applySettings` her ayar değişiminde `ClaudeCLIPath`'i buraya iter → panel ile
sağlayıcının çalıştırdığı ikili **daima aynı**. Override varsa PATH'e **düşülmez**:
yol boşsa dürüst cevap "kurulu değil"dir, PATH'teki asla kullanılmayacak başka bir
`claude`'un sürümü değil. Boş değer override'ı siler (PATH'e döner).

### `git` katalog girdisi — release akışı neden git-for-windows (2026-08-01)

`git` de katalogda (kategori `dev`, `Wire: "cli"`). TionSwarm git'e üç yerde
dayanır: oturum bağlamına **branch enjeksiyonu**, `scripts\worktree.ps1`, ve
`internal/proc`'un non-interactive git env'i — ayrıca ajanın kendi shell komutları.

Release akışı **`gitProjectURL`** ile çalışma anında seçilir:

| GOOS | URL | Sonuç |
|------|-----|-------|
| windows | `github.com/git-for-windows/git` | release var → karşılaştırma çalışır |
| diğer | `git-scm.com` | GitHub slug'ı yok → "release akışı yok" (dürüst) |

**`git/git` kullanılamaz:** o depo GitHub'da salt-okunur ayna; **tag yayımlar ama
release yayımlamaz** → `releases/latest` **404** → araç sonsuza dek "sürüm
karşılaştırılamadı" gösterirdi. git-for-windows release yayımlar ve tag'i inşa
ettiği **upstream sürümü adlandırır** (`v2.55.0.windows.3` → `2.55.0`), yani
karşılaştırma anlamlıdır. Yine de bir Windows dağıtımı olduğu için diğer
platformlarda akış kapatılır — Linux kullanıcısına Windows build numarası
göstermektense "bilmiyorum" demek doğru.

Ölçülen (2026-08-01): yerel `2.50.1` ↔ `v2.55.0.windows.3` → `outdated` ✓

### `node` / `npm` — release akışı neden BİLEREK yok (2026-08-02)

İkisi de katalogda (kategori `dev`, `Wire: "cli"`), ama **GitHub akışı bağlanmadı**.
Bu bir eksiklik değil, ölçülmüş bir karar — her iki aday da denendi:

| Aday | `releases/latest` | Neden kullanılamaz |
|------|-------------------|--------------------|
| `nodejs/node` | `v26.5.1` "(Current)" | Endpoint **tarihe göre en yenisini** verir = **Current** hattı. LTS'teki kullanıcıyı "outdated" gösterip LTS'ten **iterdi**. Node'un LTS bilgisi `nodejs.org/dist/index.json`'daki `lts` alanında — GitHub release akışı değil, ikinci bir fetcher gerekir. |
| `npm/cli` | `libnpmpack-v10.0.2` | npm CLI **değil**, monorepo'nun bir workspace paketi. `semverRe` içinden `10.0.2`'yi çeker, npm'in gerçek sürümüyle karşılaştırır → **kendinden emin ve anlamsız** bir verdict. |

GitHub olmayan URL (`nodejs.org`, `npmjs.com`) → `Repo()` boş → "release akışı yok".
Uydurmak yerine bilmediğini söyler.

Panelin asıl değeri zaten **varlık + sürüm + yol** — ve `node` bunun neden önemli
olduğunun ders kitabı örneği: bu makinede **iki ayrı Node** var ve hangisinin
görüneceği **sürecin PATH'ine** bağlı.

| Bağlam | Çözülen yol | Sürüm |
|--------|-------------|-------|
| PowerShell / winget kurulumu | `C:\Program Files\nodejs\node.exe` | v24 (LTS) |
| Git Bash (nvm-sh `.bashrc`'den PATH'i öne alır) | `~\.nvm\versions\node\vNN\bin\node.exe` | ayrı, gölgeleyen sürüm |

Yani `GET /api/external-tools` çıktısı **backend'in nasıl başlatıldığına** göre
değişir (`dev.ps1` → PowerShell → Program Files; bash'ten başlatılırsa → nvm).
Bu bir hata değil, `exec.LookPath`'in doğru davranışı — ama "mmdc neden bozuldu /
hangi node ile çalışıyorum" derdine düşen birinin görmesi gereken tam olarak
budur, ve `Path` alanı bunu tek bakışta söyler.

`node` güncellemesi bu yüzden `manual`: kurulumun sahibi nvm mi installer mı
bilinemez, winget nvm'in üstüne kurarsa çakışır. `npm` ise `command`
(`npm i -g npm@latest`) — npm kendi kendini yönetir.

**Windows notu (ölçüldü):** `npm` PATH'te `npm.cmd` olarak çözülür ve Go'nun
`exec`'i batch dosyasını sorunsuz çalıştırır → `10.8.2` okundu. Ayrı bir
`cmd /c` sarmalayıcısına gerek yok.

### `python` — tespit neden `lookPath` DEĞİL (2026-08-02)

`python` katalogda (kategori `dev`, `Wire: ""` → voice araçları gibi "bir TionSwarm
alt sistemi otomatik kullanır"). Opsiyonel bir güzellik değil **gerçek bağımlılık**:
`run_code` + `transform_data` ona shell eder, code-mode'un ürettiği binding'ler
onda koşar.

**Windows tuzağı (ölçüldü):** bu makinede

```
lookPath("python3") → C:\Users\...\AppData\Local\Microsoft\WindowsApps\python3.exe   ← Store stub
proc.LookInterpreter → C:\Python313\python.exe                                        ← gerçek CPython 3.13.7
```

Store "app execution alias" stub'ı 0-baytlık bir reparse point; sadeleştirilmiş
env ile çalıştırılınca `Python was not found` yazıp **9009** ile çıkar. Naif
`lookPath("python3")` yapan bir tespit "kurulu ✓" der, sürüm probe'u patlar ve
panel gayet çalışan bir makinede "sürüm okunamadı" gösterirdi.

**Çözüm — tek kaynak:** aday sırası (Windows'ta `python` önce) + WindowsApps
stub'ını atlama kuralı `internal/proc/interp.go`'ya taşındı
(`PythonCandidates`, `LookInterpreter`, `IsWindowsAppAlias`). Hem
`tools.resolveInterpreter` hem `exttools.Detect` artık **aynı** fonksiyonu çağırır
→ panelin raporladığı ikili ile `run_code`'un çalıştırdığı ikili ayrışamaz.
`internal/proc` zaten ikisinin de bağımlı olduğu leaf paket, yeni bağımlılık yok.

Release akışı yok: `python/cpython` `releases/latest`'e **404** verir (tag yayımlar,
release yayımlamaz) — `git/git` ile aynı durum, varsayılmadı, denendi.

### "Güncelle" butonu ne zaman çıkar — `canOfferUpdate` (2026-08-03)

Kural tek bir isimlendirilmiş yardımcıda (`ExternalToolsPanel.canOfferUpdate`):

```
kurulu  ∧  updateKind === 'command'  ∧  release akışı "geride" dedi
```

Yani **release akışı olmayan** `command` araçlarında (`ffmpeg`, `npm`, `node`,
`python`, Windows dışında `git`) buton **hiç render edilmez** — statüleri ancak
`unknown` olabilir ve *"bilmiyorum"* kullanıcının makinesinde paket yöneticisi
koşturmak için gerekçe değildir. Onlarda satırın altındaki **komut kopyalama
çipi** manuel çıkış kapısıdır.

Bu davranış zaten böyleydi ama **emergent**'ti: JSX içindeki satır-içi
`status === 'outdated'` kontrolünün yan etkisiydi, kimse bunu kural olarak
yazmamıştı. İsimlendirildi ki statü mantığı ileride değişirse gerekçesiz
güncelleme önerisi sessizce geri gelmesin.

Backend ayrıca ikinci kapıdır: `POST /api/external-tools/{name}/update` yalnız
`command` kind'ını çalıştırır, `manual` olana **409** + talimat döner.

### Linux/sunucu davranışı — `winget` artık GOOS'a bağlı (2026-08-03)

TionSwarm Windows masaüstünde de Ubuntu sunucuda da koşar. Katalogun **tespit +
sürüm** katmanı zaten çapraz-platformdu:

- `exec.LookPath` Linux PATH'ini doğal olarak kullanır; `--version` probe'ları aynı.
- `proc.PythonCandidates()` Linux'ta `python3`'ü öne alır, `IsWindowsAppAlias` orada
  daima `false` (Store stub'ı yalnız Windows sorunudur).
- `tts`/`stt` çözücüleri `exeName()` ile `.exe`'yi düşürür ve PATH'e fallback yapar.
- `gitProjectURL` Linux'ta akışı zaten kapatıyordu.

**Güncelleme katmanı ise Windows'a çakılıydı** — üç girdi `winget` ilan ediyordu.
Bu yalnız "ölü düğme" değildi: panel her `command` spec'i için **komut kopyalama
çipi** de render eder, yani Ubuntu kullanıcısına otoriter görünen ama asla
çalışamayacak bir `winget upgrade --id …` satırı verilirdi. Yanlış talimat,
talimatsızlıktan kötüdür.

| Araç | Windows | Linux/macOS |
|------|---------|-------------|
| `git` | winget `Git.Git` (`command`) | `manual` + apt notu |
| `ffmpeg` | winget `Gyan.FFmpeg` (`command`) | `manual` + apt notu |
| `bun` | winget `Oven-sh.Bun` (`command`) | **`bun upgrade`** (`command`) — bun kendi güncelleyicisini taşır |

`bun` özellikle önemliydi: release akışı Linux'ta da çalıştığı için statü
`outdated` olabiliyor → `canOfferUpdate` **düğmeyi gösteriyordu** → düğme `winget`
çağırıp hata veriyordu. Diğer ikisinde akış olmadığı için düğme zaten çıkmıyordu,
yalnız çip yanıltıyordu.

**`node` / `python` — Kind değil, NOT platforma bağlı.** İkisi her platformda
`manual` kalır (TionSwarm kurulumun sahibini bilemez: nvm, dağıtım paketi, pyenv,
brew, conda, installer — yanlış seçmek gerçek sahiple kavga eder). Değişen yalnız
**not metnidir**, çünkü not kullanıcının gerçekten uygulayacağı talimattır:

| | Windows | Linux | macOS |
|---|---|---|---|
| `node` | nvm · nodejs.org · winget `OpenJS.NodeJS.LTS` | nvm · **NodeSource** (apt'taki node çok eskidir) | nvm · `brew upgrade node` |
| `python` | python.org · winget `Python.Python.3.13` · pyenv-win | **`deadsnakes` PPA / pyenv + venv** | `brew upgrade python@3.13` |

Linux python notu ayrıca **uyarı** taşır: Debian/Ubuntu'da sistem `python3`'ü
apt'ın kendi araçlarının koştuğu yorumlayıcıdır; yerinde yükseltmek sunucuyu
bozmanın bilinen yoludur → yan yana kurulum (deadsnakes) veya pyenv + `venv`.

Tüm platform kararları `wingetSpec(goos, …)` / `bunUpdateSpec(goos)` /
`nodeUpdateSpec(goos)` / `pythonUpdateSpec(goos)` ile **parametreli** verilir
(doğrudan `runtime.GOOS` okunmaz), böylece her dal tek bir hosttan test edilebilir.
`platform_test.go` şunları bağlar: Linux dalında komut winget değil ve
`UpdateCommandLine()` winget satırı döndürmüyor; manual notlar o platformda
**var olmayan** paket yöneticisini anmıyor (Windows notunda `apt`/`brew`, Linux
notunda `winget`/`brew` yasak) ve en az bir geçerli yol gösteriyor; Linux python
notu `apt` + `pyenv` uyarısını taşıyor; `TestCatalogHasNoWingetOffWindows` canlı
katalogu Linux host'ta tarar.

### Öneri kuralı `tool-update`

`recommendations.ts`'e eklenen kural, kurulu bir aracın daha yenisi yayımlanmışsa
uyarı kartı çıkarır (kaç tanesinin tek tıkla güncellenebildiğini de söyler).
İki tasarım kararı:

- **Yalnız `outdated` sayılır.** `unknown` (bir taraf ayrıştırılamadı) kart
  çıkarmaz — manual araçlarda yanlış tahmin kullanıcıyı gereksiz ve riskli bir
  ikili değiştirmeye iter.
- **`fetchRecommendationData` içindeki tek ağ probe'u** olduğu için çağrı
  `.catch(() => [])` ile sarılıdır: çevrimdışı bir makinede `Promise.all`
  reddedip **tüm** önerileri düşürmesin. Boş dizi "fikrim yok" demektir, "hepsi
  güncel" değil. (Backend'in 6sa cache'i sayesinde bu çoğunlukla cache okumasıdır.)

Test: `recommendations.test.ts` (outdated/up-to-date/unknown/offline + tek-tık sayımı).

## Sıradaki adımlar (opsiyonel)

- `codebase_workspace_search` fan-out'unu paralelleştir (şu an sıralı; project sayısı
  arttıkça hızlanır).
- Mevcut default-store'daki eski TionSwarm kopyasının temizliği (çok-workspace geçişte).
- Fan-out'u CLI-shell yerine canlı MCP pool üzerinden (process/SQLite contention'ı azaltır).
