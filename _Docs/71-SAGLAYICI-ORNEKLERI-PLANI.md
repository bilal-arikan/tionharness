# 71 — Sağlayıcı Örnekleri (Provider Instances): Uygulama Planı

> **Durum: PLAN (kod yok) — K1/K2/K3 kararları onaylandı (2026-08-18, bkz. §8).**
> Kapsam: bugünkü "provider = sabit kind id" modelini
> **taslak (kind) → örnek (instance)** modeline çevirmek. Hedef: aynı kind'dan
> **birden fazla**, farklı token/config taşıyan sağlayıcı; sağlayıcı ayarları
> **uygulama geneli**; ajan oluştururken bu örneklerden seçim; mevcut ajanların
> bozulmadan taşınması.
>
> İlgili: `17-TOKEN-OPTIMIZASYON.md` (provider soyutlaması), `51-CLAUDE-CONFIG-BIRLESIK.md`,
> `69/70` (codex-cli — **paralel yürüyen iş**, bkz. §7 Sıralama).

---

## 1. Bugünkü durum (doğrulanmış)

| Katman | Bugün |
|--------|-------|
| Taslak | `providers.Manifest` + `ProviderKind` (`kind_*.go`, `init()` ile `RegisterKind`) — **zaten plugin şeklinde** |
| Örnek | **Yok.** Kind id'nin kendisi örnektir: `Agent.Provider = "anthropic"` |
| Kimlik/config | `settings.Settings` içinde **kind başına tekil, typed alan**: `AnthropicKeyEnc`, `MinimaxKeyEnc/BaseURL`, `OpenRouterKey…`, `ZAIKey…`, `DeepSeekKey…`, `ClaudeCLIPath/ConfigDir/AuthKind/AuthTokenEnc`, `CodexCLIPath/ConfigDir` |
| Yarı-örnek | `settings.CustomProvider` (`id/label/kind("openai"\|"anthropic")/baseUrl/defaultModel/models/keyEnc`) — **instance modelinin %70'i zaten burada** |
| Registry | `providers.Registry` — süreç başına **tek**, `internal/app/app.go:130`'da kurulur; alanları `Server.applySettings()` ile canlı güncellenir. `resolve(id)` bir **`switch id`** ile kimliği doğru typed alana eşler (`registry.go:310`+) |
| Kapsam | Sağlayıcı ayarları **zaten uygulama geneli** (`<dataDir>/settings.json`). Workspace'e özel olan tek şey: `<workspace>/claude-home` (claude-cli login evi) |

Kodda zaten iki yerde "Faz 2 = data-driven instance model" notu duruyor
(`kind.go:22`, `registry.go:310`). Bu doküman o fazın planıdır.

---

## 2. Hedef model

### 2.1 Kavramlar

- **ProviderKind (taslak)** — kodda tanımlı, kullanıcı yaratamaz. Ne olduğunu
  ve **hangi alanları istediğini** kendi manifest'inde ilan eder.
- **ProviderInstance (örnek)** — kullanıcının yarattığı gerçek sağlayıcı:
  bir kind + bir label + doldurulmuş alanlar (+ şifreli sırlar).
  Aynı kind'dan N tane olabilir: "Anthropic — iş", "Anthropic — kişisel",
  "Claude CLI — Max hesabı", "Claude CLI — API key".

### 2.2 Manifest'e eklenecek: alan şeması

Bugün `Manifest.NeedsKey`/`NeedsBaseURL` var ama **UI'ya açılmıyor**; her
sağlayıcının ayar formu `ProvidersPanel.tsx` + `settings.go` içinde elle yazılı.
Yeni kind = 3 yerde elle iş. Bunun yerine kind kendi formunu ilan etsin:

```go
// providers/kind.go
type FieldSpec struct {
    Key         string // "key" | "baseUrl" | "cliPath" | "configDir" | "authKind" | ...
    Label       string
    Type        string // "text" | "password" | "path" | "dir" | "select"
    Options     []string // Type=="select" için
    Required    bool
    Default     string
    Placeholder string
    Help        string
    Secret      bool // true → AES-GCM saklanır, API'ye maskeli çıkar
}

type Manifest struct {
    // ...mevcut alanlar...
    Transport string      // "api" | "cli"  ← davranış anahtarı (§4.2)
    Fields    []FieldSpec // örnek formunun tamamı
    Multi     bool        // aynı kind'dan >1 örnek anlamlı mı (varsayılan true)
}
```

**Standart alan anahtarları** (kind'lar bunları kullanır, `ResolvedConfig`'in
typed alanlarına birebir map'lenir): `key`, `baseUrl`, `cliPath`, `configDir`,
`authKind`, `authToken`.

Böylece **yeni kind eklemek = tek `kind_*.go` dosyası**; ayar formu, API DTO'su
ve UI otomatik gelir.

### 2.3 Örnek modeli

```go
// internal/settings/provider_instance.go (veya internal/providers/instance.go)
type ProviderInstance struct {
    ID           string            `json:"id"`           // "anthropic" (migrasyon) veya "PRV3" (yeni)
    KindID       string            `json:"kindId"`       // "anthropic" | "claude-cli" | "codex-cli" | ...
    Label        string            `json:"label"`        // "Anthropic — iş hesabı"
    Icon         string            `json:"icon"`         // opsiyonel emoji/renk
    Enabled      bool              `json:"enabled"`
    DefaultModel string            `json:"defaultModel"`
    Models       string            `json:"models"`       // opsiyonel model listesi override'ı
    Config       map[string]string `json:"config"`       // açık alanlar (baseUrl, cliPath, configDir, authKind…)
    SecretsEnc   map[string]string `json:"secretsEnc"`   // AES-GCM; API'ye ASLA çıkmaz
    CreatedAt    time.Time         `json:"createdAt"`
}
```

DTO'da `SecretsEnc` yerine `secretsSet: {"key": true}` çıkar (mevcut
`CustomProviderDTO.KeySet` deseninin genelleştirilmişi).

### 2.4 Depolama — **K2 kararı: ayrı `providers.json`**

`settings.json` içinde `providerInstances: []` olarak **değil**, ayrı
`<dataDir>/providers.json` dosyasında; `internal/settings` altında aynı `Cipher`'ı
kullanan ikinci bir store (`store_providers.go`).

Gerekçe: settings.json zaten ~120 alan; sağlayıcı CRUD'u ayrı endpoint'ten
yürüyor (`/api/providers` bugün de öyle); ayrı dosya = ayrı kilit, ayrı yedek,
`Patch` şişmesi yok. **Kapsam yine uygulama geneli** (dataDir kökü, workspace değil).

### 2.5 Ajan bağlantısı — **K3 kararı: yeni alan**

```go
// internal/db/models.go — Agent
ProviderInstanceID string `json:"providerInstanceId"` // seçilen ÖRNEK (tek gerçek kaynak)
Provider           string `json:"provider"`           // artık TÜRETİLMİŞ: örneğin KIND id'si
```

- **`ProviderInstanceID` tek gerçek kaynaktır.** `Registry.Get()` bu id ile çağrılır.
- **`Provider` alanı kalır ama anlamı değişir:** artık "seçilen sağlayıcı" değil,
  o örneğin **kind id'si**. Her yazımda `KindOf(ProviderInstanceID)` ile senkronlanır
  (tek yönlü ayna — `BlockedTools`'un `ToolOverrides`'tan türetilmesiyle aynı desen).
- Neden kaldırılmıyor: kind id'sini bekleyen **çok sayıda** okuyucu var ve hepsi
  bedavaya doğru çalışmaya devam eder —
  `billing.go` (`PriceFor`/`EstimateFor`), `conversation/budget.go`
  (`ContextWindowFor`/`AdaptiveBudgetFraction`), `agent/maxoutput.go`,
  `agent/cachebreak.go` (`CoolingWaste`), usage rollup'ları (`store_usage.go`,
  `store_session_usage.go`), `api/agents.go` provider filtresi, `view/agent.go`,
  `view/budget.go`, market pack + workspace şablonları + `ingest/agent_adapter.go`.
- **Sonuç:** §4.1'deki "kind sanan fonksiyon" listesi **kendiliğinden çözülür**;
  onlara `agent.Provider` geçmeye devam edilir, tek şart yazım anında senkron
  tutulması. Sadece `Registry.Get`/`Available` çağrıları örnek id'sine geçer.
- **Örnek başına maliyet kırılımı** isteniyorsa: usage satırlarının `provider`
  alanı **kind** olarak kalır (fiyatlama bozulmaz), yanına opsiyonel
  `providerInstanceId` alanı eklenir (boş = eski satır). Bkz. §8-K4.

---

## 3. Migrasyon (mevcut ajanları bozmadan)

**Anahtar hile: varsayılan örneklerin ID'si kind ID'sine EŞİT olsun.**

Boot'ta bir kereye mahsus (`providers.json` yoksa):

| Kaynak | Üretilen örnek |
|--------|----------------|
| `AnthropicKeyEnc != ""` | `ID:"anthropic"`, KindID:`anthropic`, secrets:`{key: …}` |
| `ClaudeCLI*` (her zaman) | `ID:"claude-cli"`, config:`{cliPath, configDir, authKind}`, secrets:`{authToken}` |
| `CodexCLI*` (kurulu ise) | `ID:"codex-cli"` |
| `MinimaxKeyEnc`, `OpenRouter*`, `ZAI*`, `DeepSeek*` | `ID` = kind id'si |
| `CustomProviders[]` | `ID` = mevcut custom id, `KindID` = `openai-compat` \| `anthropic-compat` |

Sonuç:

- `Agent.Provider` değerlerine **hiç dokunulmaz**: değeri zaten kind id'sidir ve
  yeni anlamıyla (§2.5) doğru kalır. Yeni `ProviderInstanceID` alanı **boşsa**
  okuma anında `Provider`'dan doldurulur (id'ler eşit olduğu için birebir
  eşleşir); yazımda kalıcılaşır. Ayrı bir toplu ajan migrasyonu **gerekmez**.
  `Provider == ""` olan ajan → varsayılan `claude-cli` örneği.
- Geçmiş **usage/billing** satırları (`store_usage.go`, `store_session_usage.go`,
  `ByProvider` rollup'ları) geçerli kalır.
- Market pack'leri, workspace şablonları (`api/templates.go`), ingest adaptörü
  (`ingest/agent_adapter.go`) provider id string'i tutuyor → çalışmaya devam eder.
- **Yeni** örnekler `PRV<n>` prefix'li id alır (AGT/SES/WS konvansiyonu), böylece
  kind id'leriyle bir daha çakışma olmaz.

`openai-compat` / `anthropic-compat` iki **yeni generic kind**'dır: bugünkü
`buildCustom()` fonksiyonunun (registry.go) kind'laştırılmış hâli. Bu ikisi
gelince `Registry.custom`/`CustomSpec`/`CustomCatalog()` özel yolu tamamen düşer —
her şey tek yoldan (kind + instance) akar.

---

## 4. Kırılacak yerler (doğrulanmış envanter)

### 4.1 Kind-anahtarlı fonksiyonlar — **K3 sayesinde büyük ölçüde çözüldü**

Bu fonksiyonlar `provider` parametresini **kind** sanıyor; örnek id'si (`PRV3`)
geçseydi sessizce 0/false dönerlerdi (fiyat yok, context window yok) —
**sessiz bozulma**. K3 ile `Agent.Provider` kind id'si tuttuğu için bu çağrılar
**değişmeden doğru kalır**. Şart: `Provider` alanının her yazımda
`KindOf(ProviderInstanceID)` ile senkronlanması (§2.5) + aşağıdaki listenin
Faz 2 testlerinde regresyon olarak korunması:

| Dosya:satır | Çağrı |
|-------------|-------|
| `internal/agent/cachebreak.go:142` | `providers.CoolingWaste(agent.Provider, …)` |
| `internal/agent/maxoutput.go:48` | `providers.MaxOutputFor(provider, …)` |
| `internal/api/agents.go:375-376` | `PriceFor` / `EstimateFor` |
| `internal/billing/billing.go:31,44,58,61` | `PriceFor` / `EstimateFor` |
| `internal/conversation/budget.go:45,52` | `ContextWindowFor`, `AdaptiveBudgetFraction` |

`Registry.KindOf(id string) string` yine gerekir — ama artık bu çağrılar için
değil, **senkronizasyon** (agent yazımı) ve **API/UI** (katalog rozeti, örnek
listesi) için. Registry'ye erişimi olmayan `billing`/`conversation` katmanlarına
örnek id'si **hiç sızmamalı**; global resolver hook'u (gizli tekil state)
**kullanılmayacak**.

⚠ Tuzak: bir yerde `agent.ProviderInstanceID` yanlışlıkla bu fonksiyonlara
geçerse hata değil **sessiz sıfır** üretir. Faz 2'de her biri için "örnek id
verildiğinde de doğru fiyat/window" değil, "`Provider` her zaman kind'dır"
invaryantını doğrulayan test yazılacak (agent yazım yolu → alan senkronu).

### 4.2 Davranış anahtarı olarak string karşılaştırma

| Dosya:satır | Bugün | Sonra |
|-------------|-------|-------|
| `internal/agent/runtime.go:741` | `provider == "" \|\| provider == "claude-cli"` (skill aracı adı: MCP namespace'li mi?) | `Transport == "cli"` |
| `internal/agent/toolsetup.go:811` | aynı kontrol (lazy tool kataloğu CLI formunda mı?) | `Transport == "cli"` |
| `internal/api/chat_control.go:133` | `provider != "claude-cli"` | kind/transport |
| `internal/api/session_context.go:475` | `provider != "claude-cli"` | kind/transport |
| `internal/api/catalog.go:39` | `e.ID == "claude-cli"` (CLI sürümü/abonelik rozeti) | `KindID == "claude-cli"` |

`Manifest.Transport` alanı bu genellemeyi codex-cli için de doğru yapar
(codex de MCP köprüsü kullanır → `cli`). Doc 70 §1.1'deki `CLIProvider`
arayüz refactor'u ile aynı yöne bakar; **o refactor bu planın ön koşuludur.**

### 4.3 Registry'nin `switch id` seam'i

`registry.go:resolve(id)` → örnek deposundan okuyacak:

```go
func (r *Registry) resolve(id string) ResolvedConfig {
    inst := r.instances[id]            // yoksa → hata (aşağıya bak)
    cfg := ResolvedConfig{Values: inst.Merged()}   // yeni: generic map
    cfg.Key, cfg.BaseURL = inst.Get("key"), inst.Get("baseUrl")
    cfg.CLIPath, cfg.CLIConfigDir = inst.Get("cliPath"), inst.Get("configDir")
    cfg.CLIAuthKind, cfg.CLIAuthToken = inst.Get("authKind"), inst.Get("authToken")
    cfg.CodexPath, cfg.CodexConfigDir = …
    cfg.ExtendedCache, … = r.betas…      // beta bayrakları uygulama geneli kalır
    return cfg
}
```

Typed alanlar korunur → **mevcut `kind_*.go` Build fonksiyonlarının hiçbiri
değişmez**. `Values` map'i yalnız yeni kind'ların ihtiyacı için.

**Bilinmeyen id davranışı:** `Get(name)` bugünkü gibi hata döndürür
(`unknown provider: %q`) — sessiz claude-cli fallback'i **eklenmeyecek**.
Silinmiş bir örneğe bağlı ajan, çalışma anında net hata vermeli.

### 4.4 CLI config evi — **K1 kararı: geri uyumlu**

Bugün `CLAUDE_CONFIG_DIR = <workspace>/claude-home` (workspace/manager.go:266,
`EnsureWorkspaceClaudeHome`) — yani login **workspace başına**.
"Aynı kind'dan iki farklı token" isteği bunu doğrudan zorluyor.

**Karar:**
- Örneğin `config["configDir"]` **boşsa** → bugünkü davranış korunur
  (`<workspace>/claude-home`, `EnsureWorkspaceClaudeHome`).
- **Doluysa** → örneğin kendi evi kullanılır. UI "kendi config evini oluştur"
  butonu ile `<dataDir>/provider-homes/<PRV…>/` yolunu üretip alana yazar
  (kullanıcı elle de yol verebilir).

Mevcut kurulumlar aynen çalışır; iki hesap isteyen kullanıcı ikinci örneğe ayrı
bir ev verir. Aynısı `CODEX_HOME` için geçerli.

⚠ Bu, "aynı workspace'te iki claude-cli örneği" senaryosunda **login'in
workspace'ten örneğe kaydığı** tek yerdir: boş bırakılan örnek workspace evini
paylaşmaya devam eder, dolu olan izole olur. UI'da bu ayrım açıkça yazılmalı
(yoksa kullanıcı iki örnek yaratıp ikisinin de aynı login'i kullandığını
göremez). Dolu `configDir` login'siz ise ilk turda `codexcli_errors.go:95`
benzeri net bir "bu ev login'li değil" hatası dönmeli — sessiz ambient-home
fallback'i **yok**.

---

## 5. API yüzeyi

| Yöntem | Yol | İş |
|--------|-----|-----|
| GET | `/api/provider-kinds` | **Taslak kataloğu**: manifest + `fields[]` + model listesi + `multi` |
| GET | `/api/providers` | Örnek listesi (sırlar maskeli) + `available` |
| PUT | `/api/providers` | Örnek oluştur/güncelle (id boşsa `PRV<n>` üretilir) |
| DELETE | `/api/providers/{id}` | Örnek sil — **önce** "N ajan kullanıyor" sayısını döndürür, `?force=1` ile siler |
| POST | `/api/providers/{id}/test` | Canlı doğrulama: API kind'ları için minik `Complete`, CLI kind'ları için `--version` + login kontrolü |
| GET | `/api/catalog` | **Korunur** — artık örnek başına entry (`id` = örnek id, `kindId` eklenir). Frontend model/provider seçicileri kırılmaz |

`GET /api/providers` bugün custom provider listesi döndürüyor; sözleşmesi
genişliyor (ek alanlar), kırılmıyor.

**Ajan API'si:** `POST/PATCH /api/agents` istek şemasına `providerInstanceId`
eklenir. Dikkat — son commit (`07cea1f fix(api): reject unknown fields on agent
create/update`) bilinmeyen alanları **reddediyor**: alan şemaya eklenmeden
frontend göndermeye başlarsa 400 alır. Sıra: backend şeması → frontend.
`provider` alanı istekte kabul edilmeye devam eder (legacy istemciler,
market/ingest yolu): geldiğinde `providerInstanceId` boşsa ondan çözülür.
Yanıtta ikisi de döner.

---

## 6. Frontend

| Ekran | İş |
|-------|-----|
| `features/settings/ProvidersPanel.tsx` | **Yeniden yazım.** Bugün kind başına elle yazılmış kartlar; yerine: "Sağlayıcı ekle" → taslak seçici (kind listesi) → `fields[]`'ten **generic form** render. Örnek kartları: label, kind rozeti, availability, "Test et", "Düzenle", "Sil" |
| `features/agents/AgentSettingsForm.tsx` | Provider seçici artık örnek listesi (label + kind rozeti + ⚠ yapılandırılmamış); form `providerInstanceId` gönderir, `provider` alanını **göndermez** (backend türetir). Model seçici örneğin kind manifest'i + örnek `models` override'ı |
| `api/providers.ts` | `CustomProvider` → `ProviderInstance`; `listProviderKinds`, `testProvider` eklenir |
| `features/budget`, `dashboard` | Örnek başına maliyet kırılımı bedava gelir (usage zaten provider id'ye göre grupluyor) — label göstermek için küçük bir id→label çözümü |
| Ajan onarım akışı | Ayarlar → Sağlayıcılar'da "⚠ 3 ajan silinmiş sağlayıcıya bağlı" uyarı şeridi + toplu yeniden atama diyaloğu |

---

## 7. Fazlar ve sıralama

> **Faz 0 zorunlu:** codex-cli işi (`_Docs/70`) **aynı dosyalara** dokunuyor —
> `internal/providers/registry.go`, `internal/settings/settings.go`,
> `internal/providers/kind.go`. Şu an çalışma ağacında commit edilmemiş codex
> dosyaları var. Bu plan **codex Faz 1-3 bittikten sonra** başlamalı; aksi hâlde
> iki iş aynı `switch id` bloğunda çakışır.

| Faz | İçerik | Çıktı ölçütü | Efor |
|-----|--------|--------------|------|
| **0** | Codex-cli'nin merge'ünü bekle; `CLIProvider` arayüz refactor'ı (Doc 70 §1.1) yerinde | `go build ./...` temiz, somut `*providers.ClaudeCLI` assertion'ı kalmadı | — |
| **1** | `FieldSpec` + `Manifest.Fields/Transport`; her `kind_*.go` kendi alanlarını ilan eder; `ProviderInstance` modeli + `providers.json` store (**K2**) + **boot migrasyonu**. Registry hâlâ eski yoldan okur (ikili yazım) | Migrasyon testi: eski settings.json → beklenen örnek seti; davranış birebir aynı | ~1 gün |
| **2** | `Registry` örnek-tabanlı: `SetInstances()`, `resolve()` örnekten, `KindOf()`; `Agent.ProviderInstanceID` alanı + `Provider` senkron aynası (**K3**); §4.2'deki 5 karşılaştırma `Transport`'a; `openai-compat`/`anthropic-compat` kind'ları → `CustomSpec` yolu düşer | Aynı kind'dan 2 örnek testi (2 anthropic, farklı key) yeşil; "`Provider` her zaman kind" invaryant testi yeşil | ~1 gün |
| **3** | API (`/api/provider-kinds`, CRUD, `/test`, ajan şemasına `providerInstanceId`) + `ProvidersPanel` generic form + CLI örnekleri için "kendi config evini oluştur" (**K1**) | UI'dan 2. bir Anthropic örneği yaratılıp ajana bağlanabiliyor; 2. claude-cli örneği ayrı ev + ayrı login ile çalışıyor | ~1.5 gün |
| **4** | Ajan formu + bozuk referans raporu/onarım + katalog rozetleri | Örnek silinince ilgili ajanlar listeleniyor, sessiz fallback yok | ~0.5 gün |
| **5** | `settings.Settings`'ten eski typed provider alanlarını **kaldır** (bir sürüm deprecated kaldıktan sonra); dokümanlar (`00`, `05`, `17`, `51`, `69/70`) + `tionswarm-project` skill'i güncellenir | `grep AnthropicKeyEnc` sıfır sonuç | ~0.5 gün |

**Toplam ≈ 4.5 gün** (codex işi hariç).

---

## 8. Kararlar

**Onaylandı (2026-08-18):**

- **K1 — CLI config evi: geri uyumlu.** Boş `configDir` → `<workspace>/claude-home`
  (bugünkü davranış); dolu → örneğe özel ev. Detay + tuzaklar §4.4.
- **K2 — Depolama: ayrı `providers.json`** (`<dataDir>`, AES-GCM, `internal/settings`
  altında ikinci store). Detay §2.4.
- **K3 — `Agent.ProviderInstanceID` yeni alan.** `Provider` alanı korunur ama
  **türetilmiş kind id'si**ne dönüşür ve her yazımda senkronlanır. Detay §2.5.

**Açık (Faz 2'de karara bağlanacak):**

- **K4 — Örnek başına maliyet kırılımı.** Usage satırlarının `provider` alanı
  **kind** olarak kalıyor (fiyatlama ve geçmiş uyumu için). Örnek bazlı maliyet
  isteniyorsa `db.Usage` / `db.SessionUsage` satırlarına opsiyonel
  `providerInstanceId` alanı eklenir (boş = eski satır, kind ile aynı) ve Bütçe
  ekranında ikinci bir kırılım açılır. Aksi hâlde iki Anthropic örneğinin
  maliyeti tek satırda toplanır. Karar Faz 2'de, gerçek ihtiyaç görülünce.

## 9. Riskler

| Risk | Azaltım |
|------|---------|
| Codex işiyle merge çakışması | Faz 0 kapısı: codex merge'lenmeden başlama |
| Kind-anahtarlı fonksiyonların sessiz 0 dönmesi (fiyat/window) | K3 ile `Provider` = kind kalıyor; risk **senkron kaçırma**ya indi → agent yazım yollarının tamamında (`api/agents.go`, `tools/builtin_agentmgmt.go`, `market`, `ingest`, `coordination.go:703` worker klonu, şablonlar) tek bir `setProviderInstance()` yardımcı fonksiyonu + invaryant testi |
| Örnek silinince yetim ajanlar | Silme öncesi kullanım sayısı + `force` zorunluluğu; runtime'da net hata, fallback yok |
| Geçmiş usage/billing satırlarının kopması | Varsayılan örnek id'leri = kind id'leri (§3) |
| Market pack / şablon / ingest provider id'leri | Aynı hile; ayrıca `market/pack.go` provider payload'ı `UpsertProviderInstance`'a map'lenir |
| `providers.json` sırlarının API'ye sızması | DTO'da yalnız `secretsSet` map'i; `SecretsEnc` json tag'i `-` |

## 10. Test planı

- `internal/settings`: migrasyon (eski settings → örnek seti), sır maskeleme,
  id çakışma reddi, silme + kullanım sayacı.
- `internal/providers`: `KindOf`, aynı kind'dan iki örneğin **bağımsız** client
  üretmesi, bilinmeyen id hatası, `resolve()` alan eşlemesi, generic
  `openai-compat` kind'ının eski `buildCustom` davranışıyla eşdeğerliği.
- `internal/agent`: transport tabanlı skill-aracı adı ve lazy-katalog formu
  (claude-cli örneği `PRV7` olsa bile CLI yolunu seçmeli).
- `internal/db` + `internal/api`: **invaryant testi** — her ajan yazım yolundan
  sonra `Provider == KindOf(ProviderInstanceID)`; eski (alan'sız) ajan dosyası
  yüklenince `ProviderInstanceID` doğru dolduruluyor.
- `internal/billing`: mevcut fiyatlama davranışı **değişmemeli** (regresyon) —
  `model_change_poc_test.go` deseni.
- Frontend `vitest`: örnek listesi → seçici dönüşümü, availability rozetleri.

---

## İlgili dokümanlar

`17-TOKEN-OPTIMIZASYON.md` (provider soyutlaması, prompt-cache) ·
`51-CLAUDE-CONFIG-BIRLESIK.md` (claude-home) ·
`69-CODEX-CLI-SAGLAYICI.md` / `70-CODEX-CLI-UYGULAMA-PLANI.md` (paralel iş) ·
`02-VERI-MODELI.md` (entity/ID konvansiyonu) · `21-MARKET.md` (pack provider payload)
