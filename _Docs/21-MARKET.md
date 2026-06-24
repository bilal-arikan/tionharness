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
> **Gömülü örnekler (28 paket):** 10 skill, 5 agent (researcher/coder/editor/planner/
> support), 4 provider (openrouter/groq/ollama/deepseek), 4 flow, **2 mcp
> (filesystem/fetch), 2 workspace (software-project/research), 1 memory
> (coding-standards)**. Üreteç: `internal/market/gen_examples.py` (skill/agent/provider/
> flow için; yeni türlerin default'ları `defaults/` altında elle yazılır; hepsi
> `//go:embed` ile gömülür).
>
> **UI (2026-06-24):** Market ekranı **sol dikey kategori menüsü** kullanır (Skills/
> Agents/Providers/Flows/Workspaces/Memories/Tools(MCP)); "Tümü" seçeneği yok,
> ilk kategori varsayılan. **Claude Code skill içe aktarma** Skills ekranından markete
> taşındı (Skills kategorisi başlığındaki "İçe Aktar" butonu → `SkillImportDialog`).
> **Memory kurulumu hedef ajan seçtirir** (detayda dropdown; varsayılan ilk ajan).
> Bir item'a tıklayınca detay **ortada açılan popup/modal** olarak gelir (eski yan-panel
> yerine; `market-detail-modal`, backdrop'a tıklayınca kapanır).

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
├── pack.go          # SwarmPack + payload tipleri, schema sabitleri
├── store.go         # Store: tier tarama + lazy payload (skills.Store kardeşi)
├── install.go       # her tür için Install* (skill→dosya, flow/agent→db, provider→settings)
├── publish.go       # var olan varlık → SwarmPack paketleme (sanitize)
├── defaults.go      # //go:embed defaults — gömülü başlangıç paketleri
├── defaults/        # *.swarmpack.json başlangıç paketleri
└── *_test.go
```

### 3.1 Registry katmanları (tier)

`skills` ile aynı: artan öncelik, sonraki kazanır.

| Tier | Dizin | İçerik |
|------|-------|--------|
| **bundled** | `//go:embed defaults` | SwarmGo ile gelen başlangıç paketleri |
| **global** | `<DataDir>/market` | makine-geneli (publish edilen / indirilen) |
| **workspace** | `<workspace>/market` | workspace'e özel publish edilenler |

`market.EnsureDefaults(globalDir)` boot'ta gömülü paketleri global dizine yazar
(skills.EnsureDefaults aynası; mevcut dosyanın üzerine yazmaz).

### 3.2 Store API

```go
func New(bundledFS, globalDir, workspaceDir string) *Store
func (s *Store) List() []Pack                  // manifestler (payload'sız)
func (s *Store) ListKind(kind string) []Pack
func (s *Store) Get(id string) (Pack, bool)    // payload dâhil (lazy okur)
func (s *Store) Reload()
func (s *Store) Publish(p Pack) error           // workspace tier'a yazar
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
ile workspace tier'a yazar. Sanitize = secret/ID/CreatedBy temizliği (§1.2).

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

Market artık **4. tier** olarak harici sunuculardan paket çekebilir. Yerel tier'lar
(bundled/global/workspace) uzak paketleri **id çakışmasında gölgeler** (yerel kazanır).

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
