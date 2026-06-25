# 21 — Uygulama İçi Market Sistemi (Marketplace)

> **Durum:** Tasarım + **yedi türde de kurulum çalışır** (skill/agent/provider/flow/
> **workspace/memory/mcp** install). Yeni türler (workspace/memory/mcp) 2026-06-24'te
> eklendi. (Board türü kısa süre denendi, 2026-06-24'te **kaldırıldı** — workspace
> şablonu zaten opsiyonel kanban düzeni taşıyor.)
> **Uzak kayıt defteri (remote registry) — 2026-06-25:** market artık harici
> sunuculardan paket çekebilir (`swarmregistry/v1` index). Kaynak ekle/çıkar/yenile,
> uzak paketleri listele+kur (lazy indirme, opsiyonel sha256), ve **sürüm bazlı
> "Güncelle"** algısı (install ledger). Detay §7.
> Yayınlama (publish) şimdilik yalnız skill için; diğer türlerin publish + import/export UI sonraki dilim.
> **Hedef:** Skiller, Agentlar, Sağlayıcılar, Flow taslakları, **Workspace şablonları,
> Bellek tohumları ve MCP araç sunucuları** uygulama içinden paketlenip (publish),
> gözatılıp (browse) ve kurulabilsin (install).
>
> **Katman sadeleştirme (2026-06-25):** bundled (binary'e gömülü) ve workspace
> pack tier'ları **kaldırıldı**. Paketler artık **yalnız global dizinde**
> (`<DataDir>/market`, ~/.swarmgo/market) ve **uzak registry'lerde** yaşar.
> `//go:embed defaults` + `EnsureDefaults` + `internal/market/defaults/` silindi →
> binary market item taşımıyor, workspace'te market klasörü yok. Mevcut başlangıç
> paketleri (52 adet; 2026-06-25'te **GAN üçlüsü** eklendi — `flow.gan-generator-evaluator`
> + `agent.skeptical-evaluator` + `mcp.playwright`, generator↔evaluator döngüsü için, bkz.
> `_Docs/15-FLOW-CANVAS.md`) global dizinde duruyor; yeni kurulumlarda market boş başlar
> ve global dizine elle paket konarak ya da uzak registry eklenerek doldurulur.
>
> **Provider preset kataloğu (2026-06-25):** 21 yeni provider pack'i global dizine
> eklendi (toplam **25 sağlayıcı**). Hepsi OpenAI- veya Anthropic-uyumlu uç noktalar,
> **anahtarsız** (key kurulumda sır kasasından gelir). Kapsam: xAI/Grok, Mistral,
> Google Gemini (OpenAI-compat), Together, Fireworks, Perplexity/Sonar, Cerebras,
> SambaNova, DeepInfra, Hyperbolic, Novita, Nebius, NVIDIA NIM, Cohere, Moonshot/Kimi,
> Qwen (DashScope Intl), Zhipu GLM (Z.AI), SiliconFlow, GitHub Models, ve
> iki Anthropic-uyumlu uç (`kimi-anthropic`, `glm-anthropic`). MiniMax **eklenmedi**
> (slug yerleşik `minimax` provider id'siyle çakışıyor — reserved). Bu, yol haritası
> **SC-1**'i (built-in API provider preset kataloğu) karşılar. Üretici betik:
> `sessions/.../data/gen_providers.py`. Model listeleri kurulumda düzenlenebilir.
>
> **Sistem entegrasyonu (2026-06-25):** Provider pack'leri artık **capability metadata**
> taşır ve SwarmGo'nun token/maliyet/cache/düşünme sistemlerine bağlanır:
> - **`payload.provider.reasoning`** (bool) — `true` ise OpenAI-uyumlu uç için
>   `reasoning_effort` (ajanın ThinkingLevel'ından `low/medium/high`) gönderilir
>   (`OpenAICompat`, gated; bilinmeyen alan 400'ünü önlemek için varsayılan kapalı).
>   Anthropic-kind uçlar (`kimi-anthropic`, `glm-anthropic`) thinking'i native destekler.
> - **`payload.provider.promptCache`** (`native`|`auto`|`none`) — UI'da cache rozeti
>   gösterir; `native` ise sistem prefix'ine `cache_control` breakpoint enjekte edilir
>   (OpenRouter + Anthropic uçları). Token-bazlı cache muhasebesi (`cached_tokens` vb.)
>   zaten tüm OpenAI-uyumlu uçlarda parse ediliyor.
> - **Fiyatlandırma** — `internal/providers/pricing.go` `priceTable`'a her sağlayıcı
>   slug'ı → model fiyatları eklendi (yaklaşık, USD/1M, ~Haziran 2026). Bütçe/usage
>   ekranı bu sağlayıcılar için artık maliyet + cache tasarrufu gösterir. `cacheRead`
>   çarpanı auto-cache uçlarda ~0.25, Anthropic uçlarda 0.10×.
>
> Capability zinciri: `market.ProviderPayload` → install → `settings.CustomProvider`
> → `providers.CustomSpec` → `buildCustom` → `OpenAICompat.WithCaps()`. Metadata
> hem MarketPanel önizlemesinde (cache/düşünme rozetleri) hem `CustomProviderDTO`'da
> yüzeyleniyor.
>
> **Model listeleri + fiyat gösterimi (2026-06-25):** Her provider pack'i artık
> **~8-13 güncel model** taşır (önceden 3-5). Önizlemede modeller **fiyatlarıyla**
> listelenir (giriş/çıkış $/1M token). Bunun için yeni **`GET /api/prices`** endpoint'i
> `providers.AllPrices()` ile tüm `priceTable`'ı döndürür; MarketPanel bir kez çekip
> her modelin yanında gösterir (bilinmeyen → "—"). **Tek kaynak:** `data/gen_providers.py`
> hem pack JSON'larını hem **üretilen `internal/providers/pricing_market.go`**'yu
> (`var marketPrices`, init'te `priceTable`'a merge) yazar — pack ve fiyatlar drift etmez.
> NVIDIA NIM ve GitHub Models fiyatsız (GPU/kota bazlı) → "—" gösterilir.
>
> **ProvidersPanel entegrasyonu (2026-06-25):** Ayarlar → Sağlayıcılar'daki özel
> sağlayıcı **edit formu** artık `reasoning` (checkbox) + `promptCache` (native/auto/
> none) alanlarını taşır; kaydedince `UpsertProviderInput` ile backend'e gider
> (handler zaten destekliyordu). Liste kartlarında **Düşünme / Cache rozetleri** ve
> **varsayılan modelin fiyatı** gösterilir; düzenlerken modellerin **fiyat listesi**
> (`/api/prices`'ten) görünür. Böylece market-kurulumu olmayan, elle eklenen
> sağlayıcılar da bu sistemlere bağlanabilir.
>
> **UI (2026-06-24):** Market ekranı **sol dikey kategori menüsü** kullanır (Skills/
> Agents/Providers/Flows/Workspaces/Memories/Tools(MCP)); "Tümü" seçeneği yok,
> ilk kategori varsayılan. **Claude Code skill içe aktarma** Skills ekranından markete
> taşındı (Skills kategorisi başlığındaki "İçe Aktar" butonu → `SkillImportDialog`).
> **Memory kurulumu hedef ajan seçtirir** (detayda dropdown; varsayılan ilk ajan).
> Bir item'a tıklayınca detay **ortada açılan popup/modal** olarak gelir (eski yan-panel
> yerine; `market-detail-modal`, backdrop'a tıklayınca kapanır).
>
> **Generic içe aktarma (SK-IMP2→SK-IMP3, 2026-06-25):** içe aktarma artık **jenerik ve
> çok-türlü** — tek skill klasörü değil, bir **GitHub repo / Claude Code plugin / `skills/`
> klasörü / local ağaç** (ör. `juliusbrussee/caveman`, `leonxlnx/taste-skill`,
> `coreyhaines31/marketingskills`) baştan sona taranır ve içindeki **skill + agent +
> command + MCP** artifact'larının hepsi keşfedilir. Akış üç adım: **Tara → Seç (kind'e
> gruplu) → İçe aktar** (`SkillImportDialog` kind-agnostik). GitHub yolu repo'yu **tek
> tarball** (`codeload.github.com`, API rate-limit'ine tabi DEĞİL) çeker. **Nested
> kaynaklar** (`references/`/`evals/`/`scripts/`) korunur; `(kind,slug)` kopyalar
> dedup'lanır (en sığ yol); slug çakışması batch'i durdurmaz; opsiyonel **slug öneki**.
> `owner/repo` kısayolu + `>`/`|` block-scalar açıklamalar desteklenir. Çıktı `[]market.Pack`
> → market ile **aynı kurulum otoritesi** (`installPackInto`). Kod `internal/ingest`'te
> (market'in içinde değil). Dizin siteleri doğrudan kazınmaz; GitHub repo URL'si yapıştırılır.
> Detay: §4.1 ve **`_Docs\37-INGEST-MIMARISI.md`**.

---

## 1. Amaç ve kapsam

SwarmGo'da yedi "paylaşılabilir varlık" var:

| Tür | Kaynak | Depolama | Kurulum hedefi |
|-----|--------|----------|----------------|
| **skill** | `internal/skills` | `<dir>/<slug>/SKILL.md` (dosya) | workspace skills dizini |
| **agent** | `db.Agent` | JSON entity | `db.CreateAgent` |
| **provider** | `settings.CustomProvider` | şifreli settings.json | `settings.Upsert` |
| **flow** | `db.Flow` (graph JSON) | JSON entity | `db.CreateFlow` |
| **workspace** | `workspace.Manager` | workspace registry + ws-settings | `Manager.Create` + `UpdateSettings` (yeni workspace; opsiyonel kanban düzeni dahil) |
| **memory** | `db.KnowledgeSource` | JSON entity (ajan-başına) | `Runtime.Memory().Remember` (**seçilen** ajana tohum) |
| **mcp** | `db.MCPServer` | JSON entity | `db.CreateMCPServer` (enabled; sonraki turda yüklenir) |

**Kurulum semantiği farkları:**
- **memory** bir **eylem** (tohum ekle), benzersiz-kimlikli varlık yaratmaz → "zaten kurulu" işareti yok, tekrar çalıştırılabilir ("Belleğe ekle").
- **memory** kurulumu **seçilen ajana** yazar (install body `agentId`; verilmezse ilk ajan; ajan yoksa hata verir, UI'da buton pasif).
- **workspace** kurulumu **yeni bir workspace oluşturur** (ad çakışırsa engellenir); workspace şablonu opsiyonel kanban kolon düzeni taşıyabilir (`WorkspacePayload.Columns` → `WSSettings.BoardColumns`).
- **mcp** ve **workspace** ad-bazlı dedup; **agent**/**flow** ad-bazlı; **skill** slug; **provider** id (Upsert → "Güncelle").

**Built-in araçlar markete eklenemez:** yerleşik araçlar (`Read`/`Write`/`Bash`…) Go ile
binary'e derilidir, dosya-tabanlı değildir. Markete "araç eklemenin" karşılığı **mcp**
türüdür (harici MCP sunucu config'i).

Market, bu türleri **tek bir paket formatı (SwarmPack)** altında toplar; bir
varlığı dışa paketler (publish), bir kayıt defterinde (registry) listeler ve bir
workspace'e geri kurar (install). Mimari, mevcut `skills.Store`'un birebir
kardeşidir: çok-katmanlı (tier), tembel (lazy) dosya-tabanlı bir mağaza.

**Tasarım ilkeleri:**

1. **Bağımlılıksız & dosya-tabanlı** — registry, `*.swarmpack.json` dosyalarından
   oluşan bir dizin. DB yok, ağ zorunluluğu yok (uzak registry ileride additive).
2. **Sır sızdırmaz** — provider paketi **asla** `keyEnc` taşımaz; agent paketi
   ID/CreatedBy/secret taşımaz. Kurulumda kullanıcı kendi anahtarını girer.
3. **Agent-agnostik taslaklar** — flow ve agent paketleri, hedef workspace'in
   kendi ajanlarına bağlanır (flow graph'taki `agentId` slotları boşaltılır;
   `flowTemplates.ts`'teki `instantiateTemplate` kalıbının aynısı).
4. **Provenance** — kurulan varlıklar `CreatedBy=""` (kullanıcı eylemi) damgalanır.

---

## 2. Paket formatı — SwarmPack v1

Her paket bir **manifest zarfı + tür-özel payload**'tan oluşur. Tek dosya:
`<id>.swarmpack.json`.

```jsonc
{
  "schema": "swarmpack/v1",
  "id": "skill.web-research",        // kararlı paket kimliği (kind.slug)
  "kind": "skill",                   // skill|agent|provider|flow|workspace|memory|mcp
  "name": "Web Research",
  "description": "Derin web araştırması için adım adım yöntem.",
  "version": "1.0.0",
  "author": "bilal",
  "icon": "🔎",
  "color": "#7c3aed",
  "tags": ["research", "web"],
  "createdAt": 1718750000,
  "payload": { /* türe göre değişir, bkz. §3 */ }
}
```

Manifest (payload hariç) **ucuz** tutulur: katalog yalnız manifestleri tarar,
payload yalnız detay/kurulum anında okunur (skills'teki body-lazy kalıbı).

### 2.1 Türe göre payload

**skill** — SKILL.md'nin taşınabilir hâli:
```jsonc
"payload": {
  "slug": "web-research",
  "body": "---\nname: Web Research\n...\n---\n\n# ...markdown gövdesi..."
}
```
> `body` = tam SKILL.md metni (frontmatter dâhil). Kurulum = bu metni
> `<workspace skills>/<slug>/SKILL.md`'e yazmak. Basit, kayıpsız.

**agent** — sterilize edilmiş ajan konfigürasyonu (ID/secret yok):
```jsonc
"payload": {
  "name": "Researcher",
  "soul": "...", "identity": "...",
  "provider": "anthropic", "model": "claude-...",
  "planningMode": "...", "thinkingLevel": "...",
  "permissionMode": "ask", "avatar": "🤖", "color": "#...",
  "mcpEnabled": true, "allowedTools": "[...]",
  "skills": ["web-research"]   // slug referansı; eksikse kurulumda atlanır
}
```

**provider** — özel sağlayıcı, **anahtarsız**:
```jsonc
"payload": {
  "label": "OpenRouter", "kind": "openai",
  "baseUrl": "https://openrouter.ai/api/v1",
  "defaultModel": "...", "models": "..."
  // keyEnc YOK — kurulumda kullanıcı kendi anahtarını girer
}
```

**flow** — agent-agnostik orchestration taslağı:
```jsonc
"payload": {
  "name": "Map-Reduce Araştırma",
  "description": "...",
  "graph": "{...JSON...}"   // node agentId slotları boş (kurulumda atanır)
}
```

---

## 3. Backend — `internal/market` paketi

`internal/skills` ile simetrik:

```
internal/market/
├── pack.go            # SwarmPack + payload tipleri, schema sabitleri
├── store.go           # Store: global tier tarama + lazy payload + remote birleştirme
├── remote.go          # uzak registry çekme (fetchIndex/fetchPayload) + semver
├── registry_store.go  # kaynak config + remote cache + install ledger
├── install.go         # InstallSkill (dosya); diğerleri API handler'ında
├── publish.go         # var olan varlık → SwarmPack paketleme (sanitize)
└── *_test.go
```

### 3.1 Registry katmanları (tier) — sadeleştirildi (2026-06-25)

Eskiden bundled/global/workspace üçlüsü vardı; **bundled ve workspace pack tier'ları
kaldırıldı**. Kalan yerel tek tier **global**; uzak registry'ler 4. (en düşük öncelikli)
kaynaktır. Yerel (global) paketler, id çakışmasında uzak paketleri gölgeler.

| Tier | Dizin | İçerik |
|------|-------|--------|
| **global** | `<DataDir>/market` | tek yerel kaynak (publish edilen / elle konan) |
| **remote** | uzak `registry.json` | harici registry'lerden çekilenler (§7) |

Embed/seed yok: binary market item taşımaz, workspace'te market klasörü yok. Install
ledger (`installed.json`) per-workspace olarak workspace kökünde tutulur (pack değil,
yalnız "hangi sürüm kuruldu" takibi).

### 3.2 Store API

```go
func New(globalDir, ledgerDir string) *Store   // tek yerel tier = global; ledger per-workspace
func (s *Store) List() []Pack                  // global + remote manifestler (payload'sız)
func (s *Store) ListKind(kind string) []Pack
func (s *Store) Get(id string) (Pack, bool)    // payload dâhil (yerel disk / uzak indir)
func (s *Store) Reload()
func (s *Store) Publish(p Pack) error           // global dizine yazar
```

### 3.3 Install (kurulum) — ✅ dört tür de çalışır

`market.InstallSkill` dosya işidir (market paketinde); agent/provider/flow ise db/
settings gerektirdiğinden **API handler'ında** (`api/market.go`) yapılır — market
paketini db/settings bağımlılığından uzak tutar (publish'in `BuildSkillPack` kalıbı).
`handleInstallMarketPack` türe göre `switch`'ler:

- **skill** → `market.InstallSkill`: `<workspace skills>/<slug>/SKILL.md` yaz +
  `skillStore.Reload()`. Çakışma: `overwrite=false` ise 409, true ise üzerine yaz.
- **agent** → `installAgentPack`: bilinmeyen skill slug'ları elenir (workspace'te
  çözülemeyen skill kurulumu bloklamaz), `db.CreateAgent` (CreatedBy=""). Provider/
  model boşsa db varsayılanlarına düşer.
- **flow** → `installFlowPack`: `db.CreateFlow`. Graph agent-agnostik (boş agentId);
  **hemen çalışsın diye** boş slotlar workspace'in ilk ajanına atanır (motor boş
  agentId'yi reddeder — `flowTemplates` `instantiateTemplate` kuralının aynısı).
- **provider** → `installProviderPack`: `settings.UpsertCustomProvider` + `applySettings()`
  (canlı registry push). Key (pakette **yok**) gövdedeki `apiKey`'den gelir, AES-GCM
  şifrelenir; boşsa yine kurulur (kullanıcı sonra Ayarlar'dan girer). Provider id =
  pack id'den (`provider.<slug>` → `<slug>`).

**Tekrar-kurulum koruması (dedup):** agent/flow kurulumu, aynı **ada** sahip bir varlık
zaten varsa **409** döner (`nameExists` + `agentNames`/`flowNames`) — kullanıcının işini
çoğaltmaz. skill zaten dosya çakışmasında 409 verir (overwrite guard). provider id-keyed
(`Upsert`) olduğundan tekrar kurulum **çoğaltmaz, günceller**.

### 3.4 Publish (paketleme) — `publish.go`

`PublishSkill(slug)`, `PublishAgent(id)`, `PublishProvider(id)`, `PublishFlow(id)`
→ ilgili store/db'den varlığı çekip **sanitize** ederek `Pack` üretir, `Store.Publish`
ile global dizine yazar. Sanitize = secret/ID/CreatedBy temizliği (§1.2).

---

## 4. API yüzeyi

`internal/api/market.go` (skills.go aynası), `registerMarketRoutes`:

| Metot | Yol | İş |
|-------|-----|-----|
| GET | `/api/market` | katalog (manifestler), `?kind=` filtresi |
| GET | `/api/market/{id}` | paket detayı (payload dâhil) |
| POST | `/api/market/{id}/install` | workspace'e kur (gövde: `{overwrite?, apiKey?, ...}`) |
| POST | `/api/market/publish` | gövde: `{kind, sourceId}` → yerel registry'e paketle |
| POST | `/api/market/reload` | yeniden tara |
| POST | `/api/market/import` | gövde: ham SwarmPack JSON → registry'e ekle |
| GET | `/api/market/{id}/export` | paket JSON'unu indir (dosya paylaşımı) |

`Runtime`'a `Market() *market.Store` accessor'ı (Skills() aynası); dizinler
`marketGlobalDir()` / `workspaceMarketDir(workDir)`.

### 4.1 İçe aktarma — generic ingest (SK-IMP3)

> İçe aktarma artık **skill-özel değil, jenerik**: tek bir boru hattı GitHub repo /
> plugin / local klasörden **skill + agent + command + MCP** keşfeder ve hepsini market
> ile **aynı kurulum otoritesine** bağlar. Tam mimari: **`_Docs\37-INGEST-MIMARISI.md`**.

Boru hattı: `internal/fetch` (edinme) → `internal/ingest` (adapter'lar: keşif+çeviri) →
`[]market.Pack` → `api.installPackInto` (tek kurulum otoritesi). Import kodu market'in
**içinde değil**; market'i bağımlılık olarak kullanır. Endpoint'ler `internal/api/ingest.go`:

| Metot | Yol | İş |
|-------|-----|-----|
| POST | `/api/ingest/scan` | `{source, path\|url}` → keşfedilen artifact'lar (kind/slug/name/desc/files/warnings/exists); yazma yok |
| POST | `/api/ingest/install` | `{source, path\|url, keys[], slugPrefix?, shared?}` → seçilenleri kur (installed + skipped + warnings) |
| POST | `/api/skills/import` | **tek** skill (geriye uyumlu): `{source, path\|url, slug?, shared?}` |

- **Edinme** (`internal/fetch`): `TreeFrom` repo'yu `codeload.github.com/.../tar.gz/<ref>`
  ile **tek tarball** çeker (`main`→`master`, `archive/tar`+`gzip`, **yeni bağımlılık yok**,
  rate-limit dostu) ya da local ağacı `WalkDir` ile toplar. `GroupByMarker` (SKILL.md) +
  `FindFiles` (agent/command/mcp) ortak gruplama.
- **Adapter'lar** (`internal/ingest`, registry'ye eklenir): `skill` (`**/SKILL.md`),
  `agent` (`**/agents/*.md` → AgentPayload, body→Soul, tools→AllowedTools), `command`
  (`**/commands/*.md` → loadable skill; `.toml` atlanır), `mcp` (`.mcp.json` `mcpServers`
  → MCPPayload). Yeni özellik = yeni `Adapter`.
- **Dedup:** `(kind, slug)` aynı olan kopyalar elenir, **en sığ** yol tutulur (repo'nun
  `plugins/`/`dist/` altında kendini aynalaması). Slug çakışması batch'i durdurmaz; install
  conflict'i `Skipped`'a düşer.
- **Nested kaynaklar:** `market.Pack.Files` (Faz 2) ile taşınır; `InstallSkill`
  `safeRelPath` ile (`..`/mutlak yol reddi) alt klasörleri yazar.
- **Frontmatter `>`/`|` block-scalar** açıklamalar parse edilir (`frontmatter.go::collectBlockScalar`).
- **Tek install otoritesi:** `installPackInto` kind switch'i; market install **ve** ingest install
  ortak çağırır.
- **UI:** `SkillImportDialog` kind-agnostik (Tara → Seç (kind'e gruplu) → İçe aktar); "İçe Aktar"
  butonu her market kategorisinde.
- **Dizin siteleri** (crossaitools/skillsmp/claudeskillsmarket) doğrudan kazınmaz; işaret
  ettikleri GitHub repo URL'si yapıştırılır. İleride registry köprüsü.
- Testler: `fetch/*_test.go`, `ingest/*_test.go` (+ network-gated `ingest_live_test.go`),
  `market/install_test.go`. Canlı: caveman 10 (3 agent+7 skill), taste-skill 13, marketingskills 45.

---

## 5. Frontend

- **Paylaşılan `Button` primitifi:** Market paneli, uygulamanın ortak `Button`
  bileşenini (`components/common/Button.tsx`, `variant: primary|secondary|danger`,
  `size: sm|md|lg`) benimser. Aynı bileşen uygulama genelinde ~15 panelde kullanılır;
  renk ve boyut tutarlılığı tek noktadan sağlanır.
- **NavRail**: yeni görünüm `market` — `{ key:'market', label:'Market', icon: Store }`.
- **`components/panels/MarketPanel.tsx`**: tür sekmeleri (Tümü / Beceri / Ajan /
  Sağlayıcı / Akış), kart ızgarası (ikon + ad + açıklama + sürüm + yazar + "Kur"),
  sağ detay çekmecesi (skill → markdown önizleme, flow → salt-okunur
  `FlowCanvas readOnly` önizleme, agent/provider → alan özeti), "Yayınla" akışı
  (var olan varlığı seç → registry'e paketle).
- **`api/market.ts`** + **`types/market.ts`**: Pack/PackDetail tipleri, list/get/
  install/publish/import çağrıları.
- Kurulum sonrası ilgili panel verisi tazelenir (skill listesi, flows, agents).
- **Provider API anahtarı = secret vault'tan seçim** (serbest metin değil — Ayarlar →
  Sağlayıcılar paneliyle aynı politika): provider detayında `listSecrets` ile dolu bir
  dropdown; seçilince `revealSecret(name)` ile değer çözülür ve install gövdesine
  `apiKey` olarak gider (UI'da plaintext tutulmaz/gösterilmez). "Sırlar →" butonu
  (`onManageSecrets` prop'u, App'te `setView('secrets')`) Sırlar ekranına atlar.
  Sır yoksa anahtarsız kurulur (sonra Ayarlar'dan girilebilir).
- **Zaten kurulu işareti:** panel açılışta mevcut skill/agent/flow/provider'ları çeker
  (`loadExisting`); bir paketin hedefi varsa kartta "Kuruldu" rozeti + detayda buton
  **disabled "Zaten kurulu"** (provider'da "Güncelle"). `packTargetKey`: skill→slug,
  agent/flow→ad, provider→id.
- **Kurulum sonrası tazeleme:** `onInstalled(kind)` prop'u host'a haber verir; App
  `kind==='agent'` olunca `listAgents()`→`setAgents` ile App-state ajan listesini yeniler
  → yeni ajan **Ajanlar ekranında manuel yenileme olmadan** görünür (flow/skill/provider
  panelleri zaten mount'ta yüklenir).

### 5.1 Kurulum akışı (UML)

```mermaid
sequenceDiagram
    participant U as Kullanıcı
    participant M as MarketPanel
    participant API as /api/market
    participant S as Store/Install
    U->>M: "Kur" (skill paketi)
    M->>API: POST /api/market/{id}/install
    API->>S: Install(pack)
    S->>S: SKILL.md yaz + skills.Reload()
    S-->>API: InstallResult{slug}
    API-->>M: 200
    M->>M: skill listesini tazele + toast
```

---

## 6. Gerçekleşen kapsam

**Dilim 1 (skill MVP):** pack.go + store.go + skill install/publish + 2 gömülü
paket + testler; API list/get/install/publish/import/reload; MarketPanel + NavRail.

**Dilim 2 (bu oturum):** **dört türde de kurulum** + **20 yeni örnek paket**:
- `api/market.go`: `installAgentPack`/`installFlowPack`/`installProviderPack` (§3.3).
- 23 gömülü örnek (`gen_examples.py` üreteci): 10 skill / 5 agent / 4 provider / 4 flow.
- Frontend: MarketPanel artık her tür için "Kur" gösterir (provider'da API-key
  girişi), tür-özel önizleme (`PackPreview`: skill→markdown, agent→persona+alanlar,
  provider→baseUrl+modeller, flow→düğüm listesi).
- **Doğrulama:** `go build/vet/test` + `tsc` yeşil; **canlı API E2E** (port 8090):
  agent→`CreateAgent`, flow→`CreateFlow`(ilk-ajan otomatik atama), provider→`Upsert`+
  AES-GCM key (`keySet:true`), skill→workspace+reload — dördü de 200 + entity oluştu.

**Sonraki dilim:** agent/provider/flow **publish** (sanitize), import/export UI,
Playwright UI smoke, uzak registry (§7).

---

## 7. Uzak kayıt defteri (Remote Registry) — 2026-06-25

Market artık uzak sunuculardan paket çekebilir — yerel tek tier (**global**) uzak
paketleri **id çakışmasında gölgeler** (yerel kazanır).

### 7.1 Index formatı — `swarmregistry/v1`

Bir registry, tek bir `registry.json` sunar (HTTP/HTTPS):

```jsonc
{
  "schema": "swarmregistry/v1",
  "name": "Test Registry",
  "updatedAt": 1750000000,
  "packs": [
    {
      "id": "skill.web-research", "kind": "skill", "name": "...",
      "description": "...", "version": "2.0.0", "author": "...",
      "icon": "🔎", "tags": ["research"],
      "url": "https://.../skill.web-research.swarmpack.json", // payload (lazy indirilir)
      "sha256": "abc…",          // OPSİYONEL bütünlük (boşsa atlanır)
      "minAppVersion": "0.9.0"   // taşınır; şimdilik zorlayıcı değil
    }
  ]
}
```

Index = ucuz (yalnız manifest + `url`); payload **kurulum anında** `url`'den indirilir
(yerel lazy kalıbının HTTP karşılığı). `sha256` verildiyse indirme doğrulanır, **verilmediyse
atlanır** (doğrulama opsiyonel, kullanıcı tercihi).

### 7.2 Backend (`internal/market`)

- `remote.go`: index/pack çekme (`fetchIndex`/`fetchPayload`), boyut sınırı (8MiB/4MiB),
  20sn timeout, yalnız http(s), opsiyonel sha256, `compareVersions` (semver-lite).
- `registry_store.go`: kaynak config'i `registries.json` (global market dir), index cache'i
  `<global>/.remote-cache/<hash>.json` (restart-safe; `scanDir` dizinleri atladığı için
  yerel katalogu kirletmez), per-workspace **install ledger** `<workspace>/market/installed.json`
  (packID→version → "Güncelle" algısı).
- `store.go`: `List()` yerel + uzak birleştirir (uzak `Source=remote`, `RegistryName`),
  `Get()` yerelde yoksa uzaktan indirir. `New(globalDir, workspaceDir)` artık `globalDir`'i saklar.
- Eşzamanlılık: her workspace'in Store'u aynı global cache'i okur (salt-okunur paylaşım).

### 7.3 API

| Endpoint | İş |
|----------|-----|
| `GET /api/market/registries` | kaynakları listele |
| `POST /api/market/registries` `{name,url}` | ekle (+ ilk refresh, best-effort) |
| `POST /api/market/registries/delete` `{url}` | kaldır (+ cache temizle) |
| `POST /api/market/registries/refresh` | tüm enabled kaynakları yeniden çek |
| `GET /api/market` | yerel+uzak katalog; her pack `installedVersion` ile dekore |
| `POST /api/market/{id}/install` | yerel/uzak fark etmez; başarıda ledger'a `version` yazılır |

### 7.4 UI (`MarketPanel.tsx` + `RegistryManager.tsx`)

- Header'da **"Kaynaklar"** butonu → `RegistryManager` modal (ekle/sil/yenile,
  `market-registries-modal`). **"Yenile"** butonu artık önce uzak index'leri çeker.
- Pack kartında **kaynak rozeti** (Yerel / registry adı) + **"Güncelle (vX→vY)"** rozeti
  (catalog version > installedVersion).
- Detay popup'ında install butonu güncelleme varsa **"Güncelle"** (overwrite=true).

### 7.5 Doğrulama (uçtan uca, 2026-06-25)

Yerel HTTP sunucusunda `registry.json` + pack yayınlandı → ekle → uzak pack `source=remote`
ile listede → kur (payload indirildi, skill oluştu) → ledger `installedVersion=2.0.0` →
registry sürümü 3.0.0'a çıkarıldı + refresh → "güncelleme var" = true. ✅

### 7.6 Sonraki adımlar

- İmza (ed25519) / publish-to-remote (Faz 5).
- `minAppVersion` zorlaması (şimdilik yalnız taşınıyor).
- Bağımlılıklar (agent paketi referans skill paketlerini önersin).
- Çakışma politikası (slug çakışmasında yeniden adlandırma vs üzerine yazma).
