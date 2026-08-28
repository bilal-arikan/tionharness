# 21 — Uygulama İçi Market Sistemi (Marketplace)

> **Durum:** Tasarım + **yedi türde de kurulum çalışır** (skill/agent/provider/flow/
> **workspace/mcp/hook** install). workspace/mcp 2026-06-24'te, `hook` (yabancı plugin
> hook'ları, ingest ile) sonradan eklendi. (Board türü kısa süre denendi, 2026-06-24'te
> **kaldırıldı** — workspace şablonu zaten opsiyonel kanban düzeni taşıyor.
> **`memory` türü 2026-07-05'te hafıza alt sistemiyle birlikte tamamen kaldırıldı.**)
> **Bayatlık tazelemesi 2026-07-28 → §8** (hook UI'ı, MCP scope/headers, node ikonları,
> şablon sürümleri).
> **Uzak kayıt defteri (remote registry) — 2026-06-25:** market artık harici
> sunuculardan paket çekebilir (`harnessregistry/v1` index). Kaynak ekle/çıkar/yenile,
> uzak paketleri listele+kur (lazy indirme, opsiyonel sha256), ve **sürüm bazlı
> "Güncelle"** algısı (install ledger). Detay §7.
> Yayınlama (publish) şimdilik **skill** ve **workspace şablonu** için
> (`internal/api/market_publish.go`: `market.KindSkill` + `market.KindWorkspace`);
> diğer türlerin publish + import/export UI sonraki dilim.
> **Hedef:** Skiller, Agentlar, Sağlayıcılar, Flow taslakları, **Workspace şablonları,
> MCP araç sunucuları ve hook'lar** uygulama içinden paketlenip (publish),
> gözatılıp (browse) ve kurulabilsin (install).
>
> **Katman sadeleştirme (2026-06-25) + kısmi geri alma:** workspace pack tier'ı
> **kaldırıldı**; **bundled tier daha sonra geri geldi** — `//go:embed defaults` +
> `internal/market/defaults/*.harnesspack.json` yalnız **gömülü workspace şablonlarını**
> taşır (taze kurulumda market ve "workspace oluştur" seçicisi boş kalmasın diye).
> Diğer tüm paketler global dizinde (`<DataDir>/market`, ~/.tionharness/market) ve uzak
> registry'lerde yaşar; workspace'te market klasörü yok. Mevcut başlangıç
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
> taşır ve TionHarness'in token/maliyet/cache/düşünme sistemlerine bağlanır:
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
> Capability zinciri (Faz 5 güncel, _Docs/71): `market.ProviderPayload` → `installProviderPack`
> → `upsertProviderInstance` (aynı doğrulama yolu `/api/providers` handler'ının kullandığı) →
> `providers.json` (`settings.ProviderInstance`, kind = `openai-compat`/`anthropic-compat`) →
> `Registry.resolve` → `OpenAICompat`/`Anthropic` build fonksiyonu. `reasoning`/`promptCache`
> standart `FieldSpec` alanı değil — `ExtraConfig` ile instance `Config` map'ine yazılır,
> `registryInstances` aynı anahtarlardan geri okur. Metadata hem MarketPanel önizlemesinde
> (cache/düşünme rozetleri) hem `ProviderInstanceDTO`'da yüzeyleniyor.
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
> **UI (2026-06-24, sıralama 2026-07-28):** Market ekranı **sol dikey kategori menüsü**
> kullanır — **Workspaces**/Skills/Agents/Providers/Flows/Tools(MCP)/**Hooks**; "Tümü"
> seçeneği yok, ilk kategori (Workspaces — tek gömülü paket tipi) varsayılan.
> **Claude Code skill içe aktarma** Skills ekranından markete taşındı (sol paneldeki
> "İçe Aktar" butonu → `SkillImportDialog`). Bir item'a tıklayınca detay **ortada açılan
> popup/modal** olarak gelir (eski yan-panel yerine; `market-detail-modal`, backdrop'a
> tıklayınca kapanır).
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

TionHarness'te yedi "paylaşılabilir varlık" var:

| Tür | Kaynak | Depolama | Kurulum hedefi |
|-----|--------|----------|----------------|
| **skill** | `internal/skills` | `<dir>/<slug>/SKILL.md` (dosya) | workspace skills dizini |
| **agent** | `db.Agent` | JSON entity | `db.CreateAgent` |
| **provider** | `settings.ProviderInstance` | şifreli `providers.json` (ayrı dosya, K2 — _Docs/71) | `upsertProviderInstance` (`installProviderPack`) |
| **flow** | `db.Flow` (graph JSON) | JSON entity | `db.CreateFlow` |
| **workspace** | `workspace.Manager` | workspace registry + ws-settings | `Manager.Create` + `UpdateSettings` (yeni workspace; opsiyonel kanban düzeni dahil) |
| **mcp** | `db.MCPServer` | JSON entity | `db.CreateMCPServer` (enabled; sonraki turda yüklenir) |
| **hook** | `db.Hook` (ingest ile yabancı plugin'den) | JSON entity + hook-scripts dosyaları | `db.CreateHook` (enabled; dedup yok) |

**Kurulum semantiği farkları:**
- **hook** benzersiz-kimlikli varlık yaratmaz (ad/slug yok) → "zaten kurulu" işareti yok, tekrar kurulabilir; kurmadan önce komut önizlemesi okunmalı (makinede kabuk komutu çalıştırır).
- **workspace** kurulumu **yeni bir workspace oluşturur** (ad çakışırsa engellenir); workspace şablonu opsiyonel kanban kolon düzeni taşıyabilir (`WorkspacePayload.Columns` → `WSSettings.BoardColumns`).
- **mcp** ve **workspace** ad-bazlı dedup; **agent**/**flow** ad-bazlı; **skill** slug; **provider** id (Upsert → "Güncelle").

**Built-in araçlar markete eklenemez:** yerleşik araçlar (`Read`/`Write`/`Bash`…) Go ile
binary'e derilidir, dosya-tabanlı değildir. Markete "araç eklemenin" karşılığı **mcp**
türüdür (harici MCP sunucu config'i).

Market, bu türleri **tek bir paket formatı (HarnessPack)** altında toplar; bir
varlığı dışa paketler (publish), bir kayıt defterinde (registry) listeler ve bir
workspace'e geri kurar (install). Mimari, mevcut `skills.Store`'un birebir
kardeşidir: çok-katmanlı (tier), tembel (lazy) dosya-tabanlı bir mağaza.

**Tasarım ilkeleri:**

1. **Bağımlılıksız & dosya-tabanlı** — registry, `*.harnesspack.json` dosyalarından
   oluşan bir dizin. DB yok, ağ zorunluluğu yok (uzak registry ileride additive).
2. **Sır sızdırmaz** — provider paketi **asla** `keyEnc` taşımaz; agent paketi
   ID/CreatedBy/secret taşımaz. Kurulumda kullanıcı kendi anahtarını girer.
3. **Agent-agnostik taslaklar** — flow ve agent paketleri, hedef workspace'in
   kendi ajanlarına bağlanır (flow graph'taki `agentId` slotları boşaltılır;
   `flowTemplates.ts`'teki `instantiateTemplate` kalıbının aynısı).
4. **Provenance** — kurulan varlıklar `CreatedBy=""` (kullanıcı eylemi) damgalanır.

---

## 2. Paket formatı — HarnessPack v1

Her paket bir **manifest zarfı + tür-özel payload**'tan oluşur. Tek dosya:
`<id>.harnesspack.json`.

```jsonc
{
  "schema": "harnesspack/v1",
  "id": "skill.web-research",        // kararlı paket kimliği (kind.slug)
  "kind": "skill",                   // skill|agent|provider|flow|workspace|mcp|hook
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
  "thinkingLevel": "...",
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

**workspace** — workspace şablonu (kimlik + başlangıç ekosistemi):
```jsonc
"payload": { "workspace": {
  "name": "...", "icon": "🛠", "color": "#...", "instructions": "...",
  "columns": [{ "key": "todo", "label": "Yapılacak" }],
  "prompts": { "summary": "..." },   // yalnız NON-DEFAULT prompt override'ları
  "readme": "...",                   // config/README.md
  "skills":  [{ "slug": "...", "body": "---\n...", "files": {} }],
  "agents":  [{ "key": "execute", "name": "...", "soul": "...",
                "permissionMode": "auto", "toolOverrides": "{...}",
                "coordinatorMode": true,               // ajanın açtığı oturumlar koordinatör doğar
                "coordinatorWorkflow": "coordinator-wf-plan-dev-test",  // opsiyonel reçete
                "coordinatorPrompt": "Önce planla, sonra iki worker aç.", // koordinatör-özel prompt
                "skills": ["..."] }],
  "flows":   [{ "name": "...", "steps": [...] }],   // veya "graph": "{...}" (agentId = "tmpl:<key>")
  "schedules": [{ "agentKey": "...", "cronExpr": "0 8 * * *", "prompt": "...",
                  "enabled": true }],
  "automations": [{ "name": "...", "triggerKind": "board", "boardOp": "move",
                    "boardToState": "in_progress", "boardAction": "spawn",
                    "boardExclusive": true,
                    "agentKey": "cto",                 // veya "flowName": "..."
                    "promptTemplate": "...", "enabled": true }]
}}
```
> Seed sırası: skills → agents → flows → schedules → automations. Zamanlama ve
> otomasyonlarda `enabled` geriye uyumludur: alan yoksa `false` ve eski pasif kurulum
> davranışı korunur; paket açıkça `true` verirse kayıt etkin kurulur. Böylece sıradan
> şablonlarda kablolama harcama başlatmazken ilan ettiği kontrol döngüsü olan bir paket
> gerekli schedule/kuralı açılışta çalıştırabilir.
>
> **Referanslar isimledir, id'yle değil:** otomasyon ajana `agentKey`, akışa `flowName`
> ile bağlanır; çözülmeyen referans **atlanır** (hedefi boş bir pano kuralı her kart
> taşımasında ateşleyip başarısız olurdu). Publish tarafı simetriktir; `Seed != ""` olan
> gömülü default kurallar dışa aktarılmaz (her workspace kendi kopyasını açılışta üretir).
>
> **`coordinatorMode`** ajan tanımına yazılır, oturuma **doğuşta** kopyalanır (bkz.
> `_Docs/47` §15). Pinlenmiş `coordinatorWorkflow` kurulumda çözülemezse **düşürülür** —
> ajan serbest koordinasyonla çalışır, kurulum patlamaz. `coordinatorPrompt` serbest
> metindir, çözülecek bir referansı yoktur: **olduğu gibi** taşınır ve kurulur; boşsa
> hiç yazılmaz (`omitempty`). Aynı üç alan **agent** paketinde de taşınır.

### Blank workspace başlangıç kontrol döngüsü

Gömülü `workspace-blank` paketi tek `Asistan` yerine iki ajan kurar: yalnız gözlem,
mesajlaşma ve delegasyon yapan **CEO** ile tam araç erişimli yürütücü **PM**. CEO'nun
etkin `*/20 * * * *` schedule'ı kalıcı `kind="schedule"` oturumunda board'u okur;
duran, başarısız veya ilerlemeyen işte PM'i dürter ve kendisi uygulama işi yapmaz.
`failed` ve `review` durumlarına kart taşıma olaylarını izleyen iki etkin pano
otomasyonu da PM'i `sessionMode="continue"` ile aynı kalıcı otomasyon oturumunda
uyandırır.

Kurulumda schedule/automation `agentKey` değerleri önce seed edilen gerçek ajan
ID'lerine, varsa `flowName` gerçek akış ID'sine çözülür; çözülemeyen hedef atlanır.
`internal/api/templates_test.go::TestBlankTemplateSeedsCEOAndPMControlLoop`, ajan
rolleri ve CEO araç sınırıyla birlikte bu etkin kontrol döngüsünü kilitler.

**mcp** — MCP araç sunucusu:
```jsonc
"payload": { "mcp": {
  "name": "github", "description": "GitHub MCP",
  "transport": "http",                 // stdio | http (sse desteklenmiyor)
  "command": "", "args": "[]", "url": "https://…/mcp",
  "envConfig": "{...}",                // stdio env
  "headersConfig": "{\"Authorization\":\"Bearer …\"}",  // http başlıkları (2026-07-28)
  "scope": "shared"                    // shared (vars.) | scoped (oturum+ajan başına)
}}
```
> Sırlar publisher'ın sorumluluğu — `envConfig`/`headersConfig` verbatim taşınır.
> Bilinmeyen `scope` değeri kurulumda `shared`'a düşürülür.

**hook** — yabancı plugin'den (Claude Code `plugin.json`/`hooks.json`) içe aktarılan
lifecycle/tool hook'u:
```jsonc
"payload": { "hook": {
  "event": "PreToolUse", "matcher": "Bash", "timeoutSec": 30,
  "command": "${CLAUDE_PLUGIN_ROOT}/scripts/guard.sh"
}}
```
> Paketin `files` alanındaki scriptler workspace'in hook-scripts klasörüne yazılır ve
> `${CLAUDE_PLUGIN_ROOT}` o dizine yeniden yazılır. Kurulum **dedup yapmaz** (hook'un
> tekil kimliği yok) → UI de "zaten kurulu" işareti göstermez.

---

## 3. Backend — `internal/market` paketi

`internal/skills` ile simetrik:

```
internal/market/
├── pack.go            # HarnessPack + payload tipleri, schema sabitleri
├── store.go           # Store: global tier tarama + lazy payload + remote birleştirme
├── remote.go          # uzak registry çekme (fetchIndex/fetchPayload) + semver
├── registry_store.go  # kaynak config + remote cache + install ledger
├── install.go         # InstallSkill (dosya); diğerleri API handler'ında
├── publish.go         # var olan varlık → HarnessPack paketleme (sanitize)
└── *_test.go
```

### 3.1 Registry katmanları (tier)

2026-06-25'te bundled/workspace tier'ları kaldırılmıştı; **bundled tier daha sonra geri
geldi** (gömülü workspace şablonları, `embed.go` + `defaults/*.harnesspack.json`) — taze
kurulumda market ve "workspace oluştur" seçicisi boş kalmasın diye. Workspace tier'ı
gerçekten yok. Öncelik: **global > bundled**, ikisi de uzak paketleri gölgeler.

| Tier | Dizin | İçerik |
|------|-------|--------|
| **bundled** | binary içi `defaults/` | gömülü workspace şablonları (salt-okunur, en düşük öncelik) |
| **global** | `<DataDir>/market` | tek yazılabilir yerel kaynak (publish edilen / elle konan) |
| **remote** | uzak `registry.json` | harici registry'lerden çekilenler (§7) |

Workspace'te market klasörü yok. Install
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

### 3.3 Install (kurulum) — ✅ yedi türün hepsi çalışır

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
- **provider** → `installProviderPack`: `upsertProviderInstance` (`providers.json`) +
  `applySettings()` (canlı registry push). Key (pakette **yok**) gövdedeki `apiKey`'den
  gelir, AES-GCM şifrelenir; boşsa yine kurulur (`AllowMissingRequiredSecrets` —
  kullanıcı sonra Ayarlar'dan girer). Provider id = pack id'den (`provider.<slug>` →
  `<slug>`); kind = `openai-compat` | `anthropic-compat`.

- **workspace** → `installWorkspacePack`: şablondan **yeni workspace** yaratır ve
  ekosistemi seed eder (skills → agents → flows → schedules). Dedup workspace adına göre.
- **mcp** → `installMCPPack`: `db.CreateMCPServer` (enabled). Ad'a göre dedup. Payload'daki
  `description`/`headersConfig`/`scope` aynen aktarılır; `scope` yalnız `scoped` ise
  korunur, aksi hâlde `shared` (2026-07-28 — önceden sabit `shared` idi ve
  headers/description düşüyordu).
- **hook** → `installHookPack`: `db.CreateHook` (enabled, `CreatedBy=""`). Paketin
  script'leri workspace hook-scripts dizinine yazılır, `${CLAUDE_PLUGIN_ROOT}` o dizine
  rewrite edilir. **Dedup yok.**

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
| POST | `/api/market/import` | gövde: ham HarnessPack JSON → registry'e ekle |
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

### 7.1 Index formatı — `harnessregistry/v1`

Bir registry, tek bir `registry.json` sunar (HTTP/HTTPS):

```jsonc
{
  "schema": "harnessregistry/v1",
  "name": "Test Registry",
  "updatedAt": 1750000000,
  "packs": [
    {
      "id": "skill.web-research", "kind": "skill", "name": "...",
      "description": "...", "version": "2.0.0", "author": "...",
      "icon": "🔎", "tags": ["research"],
      "url": "https://.../skill.web-research.harnesspack.json", // payload (lazy indirilir)
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

---

## 8. Bayatlık tazelemesi — 2026-07-28

Market ekranı ve gömülü item'lar, son dönemde eklenen alt sistemlerin (hook ingest'i,
hibrit MCP kapsamı, genişleyen flow node tipleri, merkezi prompt registry) gerisinde
kalmıştı. Yapılan düzeltmeler:

| # | Sorun | Düzeltme |
|---|-------|----------|
| 1 | `hook` türü backend'de kurulabilirken UI'da yoktu (sekme yok, `KIND_LABEL['hook']` boş → İçe Aktar diyaloğunda başlıksız grup) | `PackKind`/`IngestKind`'a `hook`, `KIND_NAV`'a **Hooks** sekmesi (`Webhook` ikonu), `KIND_LABEL`/`INSTALL_LABEL` girdileri, `PackPreview`'da hook kartı (olay/matcher/timeout + komut + "kurmadan önce komutu oku" uyarısı) |
| 2 | `MCPPayload` scope/headers/description taşımıyordu; kurulum `Scope:"shared"` sabitliyordu → auth başlıklı HTTP MCP sunucusu paketle taşınamıyordu | Payload'a `description`/`headersConfig`/`scope`; `installMCPPack` üçünü de aktarır (`scope` allow-list'li). `mcp_adapter` artık `.mcp.json`'daki `headers` bloğunu da okur |
| 3 | `NODE_ICON` yalnız 5 node tipini biliyordu | 12 tipin tamamı (start/end/agent/branch/parallel/delay/transform/loop/await-input/subflow/spawn/join); workspace önizlemesindeki akış çipleri de artık tüm non-lineer tipleri gösterir |
| 4 | Kaldırılmış `memory` türüne ait ölü yorumlar; `AgentPayload.capabilities` ölü alan; `toolOverrides`/`prompts`/`readme` TS tiplerinde yoktu | Temizlendi + eklendi; workspace önizlemesine **prompt override'ları** ve **config/README.md** bölümleri, ajan kartına araç-override çipi |
| 5 | Gömülü 5 şablonda `version` yoktu → `updateAvailable()` hep false, ledger'a boş sürüm | Hepsine `"version": "1.0.0"` |
| 6 | Market varsayılan sekmesi `skill` idi ama gömülü skill paketi 0 → ilk açılışta boş ızgara | `KIND_NAV` sırası **Workspaces** ile başlar (tek gömülü paket tipi) |
| 7 | `SourceWorkspace` tier'ı ölü koddu; paket/store yorumları "dört tür / üç tier" diyordu | `SourceWorkspace` kaldırıldı (frontend `PackSource` + `SOURCE_LABEL` dâhil); yorumlar 7 tür + bundled/global/remote olarak güncellendi |

**Doğrulama:** `go build ./...` + `go vet` + `go test ./internal/market/... ./internal/ingest/...`
(19 test) yeşil; frontend `tsc --noEmit` temiz.

**Kalan (içerik kararı):** workspace şablonları agents + flow + schedule + automation
taşıyor; `await-input`/`subflow`/`spawn-join` düğümlerini kullanan gömülü örnekler ve
insight lens'lerinin şablonla taşınması hâlâ eksik.
