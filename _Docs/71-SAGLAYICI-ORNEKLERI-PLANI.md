# 71 — Sağlayıcı Örnekleri (Provider Instances): Uygulama Planı

> **Durum: BİTTİ (Faz 0-5, 2026-08-18) — K1/K2/K3 kararları onaylandı, K4 kapsam dışı bırakıldı (bkz. §8).**
> Bugünkü "provider = sabit kind id" modeli **taslak (kind) → örnek (instance)**
> modeline çevrildi. Aynı kind'dan **birden fazla**, farklı token/config taşıyan
> sağlayıcı; sağlayıcı ayarları **uygulama geneli**; ajan oluştururken bu
> örneklerden seçim; mevcut ajanlar bozulmadan taşındı (sıfır veri migrasyonu).
> Faz 5'te eski typed `settings.Settings` alanlarının DTO/Patch/Store yüzeyi
> kaldırıldı ve dokümanlar güncellendi — plandan sapmalar §7 faz tablosunda ve
> aşağıdaki notlarda işaretli.
> Ajan listesindeki toplu düzenleme paneli de tekil create/edit akışıyla aynı
> `ProviderInstanceModelSelect` bileşenini kullanır. Her ajan mevcut update handler'ına yalnız
> `{provider: instanceId, model}` patch'iyle gider; handler instance id'yi doğrulayıp
> `ProviderInstanceID` kaynağını ve türetilmiş `Provider` kind alanını senkronlar.
>
> İlgili: `17-TOKEN-OPTIMIZASYON.md` (provider soyutlaması), `51-CLAUDE-CONFIG-BIRLESIK.md`,
> `69/70` (codex-cli — Faz 0 ön koşuluydu, merge edildi).

---

## 1. Bugünkü durum (doğrulanmış)

| Katman | Bugün |
|--------|-------|
| Taslak | `providers.Manifest` + `ProviderKind` (`kind_*.go`, `init()` ile `RegisterKind`) — **zaten plugin şeklinde** |
| Örnek | **Yok.** Kind id'nin kendisi örnektir: `Agent.Provider = "anthropic"` |
| Kimlik/config | `settings.Settings` içinde **kind başına tekil, typed alan**: `AnthropicKeyEnc`, `MinimaxKeyEnc/BaseURL`, `OpenRouterKey…`, `ZAIKey…`, `DeepSeekKey…`, `ClaudeCLIPath/ConfigDir/AuthKind/AuthTokenEnc`, `CodexCLIPath/ConfigDir` |
| Yarı-örnek | `settings.CustomProvider` (`id/label/kind("openai"\|"anthropic")/baseUrl/defaultModel/models/keyEnc`) — **instance modelinin %70'i zaten burada** |
| Registry | `providers.Registry` — süreç başına **tek**, `internal/app/app.go:130`'da kurulur; alanları `Server.applySettings()` ile canlı güncellenir. `resolve(id)` bir **`switch id`** ile kimliği doğru typed alana eşler (`registry.go:310`+) |
| Kapsam | Sağlayıcı örnekleri ve kimlik evleri **uygulama geneli**dir: örnekler `<dataDir>/providers.json`, otomatik oluşturulan CLI evleri `<dataDir>/provider-homes/<instance-id>` altındadır. Ajan yalnız örnek id'sini seçer; workspace CLI kimliğinin sahibi değildir. |

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

### 3.1 Migrasyonun iki açığı ve onarımı (2026-08-19)

Canlı veride iki kayıp bulundu; ikisi de **ajanın hiç çalışamaması** demekti
(`Registry.Get` bilinmeyen örneği bilinçli olarak hata sayar, sessiz fallback yok):

1. **`-anthropic` varyantları migrasyon dışı kalmıştı.** `minimax-anthropic` ve
   `deepseek-anthropic` ayrı **kind**'lardır ama kendi legacy ayar anahtarları
   yoktur (temel sağlayıcının kimliğini paylaşırlar), dolayısıyla tabloda
   üretilmiyorlardı → bu kind'a bağlı her ajan yok olan bir örneği işaret
   ediyordu. `MigrateFromSettings` artık temel anahtar varken iki varyantı da
   üretir; `baseUrl` **boş** bırakılır (temel örneğin URL'i OpenAI-uyumlu uçtur,
   Anthropic taşıması onunla konuşamaz — boş = kind'ın kendi varsayılanı).
2. **Çalışma zamanı örnek id'sini hiç kullanmıyordu.** Tüm çağrı yerleri
   `providers.Get(agent.Provider)` (yani **kind** id'si) diyordu; varsayılan
   örnekte id == kind olduğu için bu fark edilmiyordu, ama ikinci bir örneğe
   (`PRV1`, ikinci hesap, ayrı config evi) bağlı ajan sessizce kind'ın
   **varsayılan** örneğine düşüyordu — §9'un "sessiz öksüz ajan" riski. Artık
   tek seam var: `db.Agent.ProviderRef()` (→ `ProviderInstanceID`, boşsa
   `Provider`) ve bütün `Registry.Get` çağrıları bunu kullanır. Kind-anahtarlı
   okuyucular (§4.1) `Agent.Provider`'ı okumaya devam eder.

Mevcut kurulumlar için `providers.json` bir daha migrate edilmez (dosya var),
onları **`cmd/repair-provider-migration`** onarır (varsayılan kuru çalışma,
`-apply` ile yazar, idempotent): eksik `-anthropic` örneklerini şifreli
kimliği yeniden kullanarak ekler, örneği kaybolmuş ajanları kendi kind'ının
varsayılan örneğine bağlar, ve config evi taşınmasından kalan claude-cli
resume kayıtlarını düzeltir (bkz. `51-CLAUDE-CONFIG-BIRLESIK.md`).

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
| `internal/agent/runtime_skills.go` → `isCLIProviderKind` | `provider == "" \|\| provider == "claude-cli"` (skill aracı adı: MCP namespace'li mi?) | `Transport == "cli"` |
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

### 4.4 CLI config evi — **örnek başına izolasyon**

Yeni bir `claude-cli` veya `codex-cli` örneği boş `configDir` ile oluşturulursa
`handleUpsertProvider`, örneği kaydettikten sonra otomatik olarak
`<dataDir>/provider-homes/<instance-id>` dizinini `0700` izinleriyle oluşturur ve
yolu örneğin `config["configDir"]` alanına kalıcılaştırır. Claude alt sürecine bu
yol `CLAUDE_CONFIG_DIR`, Codex alt sürecine `CODEX_HOME` olarak verilir. Böylece
aynı kind'ın iki örneği farklı login/config/oturum durumuna sahip olur ve herhangi
bir workspace'teki ajan, seçtiği uygulama-geneli örneğin kimliğini kullanır.

Kullanıcı elle dolu bir `configDir` verirse o yol korunur. Eski veya dışarıdan
oluşturulmuş bir örnekte alan hâlâ boşsa çalışma/auth çözümü geri uyumluluk için
`<workspace>/claude-home` ya da `<workspace>/codex-home` yoluna düşer. Bu yalnız
legacy fallback'tir; normal yeni örnek yaratma yolunda alan otomatik dolar.

**Uygulama durumu (2026-08-19): K1 artık CLI turlarının per-turn işlem
noktasında (seam) da uygulanıyor.** Önceden `internal/agent/toolloop.go`'daki
`isCLI` bloğu, örneğin `configDir` alanı ne olursa olsun her turda koşulsuz
workspace evini (`<workspace>/claude-home`, `r.codexHomeDir()`) dayatıyordu —
alan yalnız kozmetikti. Artık `ClaudeCLI.ConfigDir()` /
`CodexCLI.ConfigDir()` (kurulumda örneğin `config["configDir"]` değerinden
set edilir) doluysa o ev korunur; yalnız boşsa uygulama-geneli
`<dataDir>/claude-home` veya `<dataDir>/codex-home` evine düşülür ve
credential-heal (`ensureClaudeHomeCredential`) / `MkdirAll` (codex) fiilen
kullanılan eve uygulanır. Workspace başına CLI home seed'i kaldırılmıştır.

Ayrıca `internal/settings/store_providers.go`'ya (`OpenProviderStore`,
`sanitizeLegacyConfigDirs`) idempotent bir yükleme-zamanı onarım eklendi:
eski tekli-sağlayıcı modelinden migrate edilmiş bir örneğin `configDir`'i,
kendi kind'ının ESKİ global varsayılan yoluna (`<dataDir>/claude-home`,
`<dataDir>/codex-home`) **birebir** eşitse bu bir migrasyon artığı sayılır ve
BOŞA çekilir (`slog.Warn` ile loglanır) — K1 devreye girmeden önce her
kullanıcının migrate edilmiş örneği yanlışlıkla "kendi evim var" durumuna
düşüp (codex için var olmayan bir dizine, claude için sessizce workspace
evinden sabit global eve) kaymasın diye. Kullanıcının elle yazdığı farklı bir
yol asla dokunulmaz; onarım ikinci açılışta no-op'tur.

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

### 5.1 CLI kimliği — örnek başına

| Yöntem | Yol | İş |
|--------|-----|-----|
| GET | `/api/providers/{id}/auth` | Örneğin kind/install/login durumu ile kullandığı `homeDir`; Codex için varsa `tier` |
| POST | `/api/providers/{id}/auth/oauth/start` | Claude OAuth kod akışını başlatır |
| POST | `/api/providers/{id}/auth/oauth/complete` | Claude OAuth kod akışını tamamlar |
| POST | `/api/providers/{id}/auth/oauth/loopback/start` | Claude loopback OAuth akışını başlatır |
| GET | `/api/providers/{id}/auth/oauth/loopback/status` | Claude loopback OAuth durumunu okur |
| POST | `/api/providers/{id}/auth/device/start` | Codex device-code login'ini başlatır |
| GET | `/api/providers/{id}/auth/device/status` | Codex device-code durumunu okur |
| POST | `/api/providers/{id}/auth/device/cancel` | Codex device-code akışını iptal eder |
| POST | `/api/providers/{id}/auth/api-key` | Codex API anahtarını örneğin `CODEX_HOME`'una yazar |

Auth handler'ları `{id}` ile örneği çözer, yalnız `claude-cli`/`codex-cli`
kind'larını kabul eder ve akışı örneğin `configDir`/`cliPath` değerleriyle yürütür.
`/api/workspace-settings/claude-auth...` ve
`/api/workspace-settings/codex-auth...` yolları eski istemciler için korunur;
yeni UI ve istemciler örnek-bazlı yolları kullanır.

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
| **0** | ✅ Codex-cli'nin merge'ünü bekle; `CLIProvider` arayüz refactor'ı (Doc 70 §1.1) yerinde | `go build ./...` temiz, somut `*providers.ClaudeCLI` assertion'ı kalmadı | — |
| **1** | ✅ `FieldSpec` + `Manifest.Fields/Transport`; her `kind_*.go` kendi alanlarını ilan eder; `ProviderInstance` modeli + `providers.json` store (**K2** — dosya `internal/settings/store_providers.go`, tip adı `ProviderStore`; plandaki "`internal/settings/provider_instance.go`" ismi kullanılmadı) + **boot migrasyonu** (`provider_migrate.go`) | Migrasyon testi: eski settings.json → beklenen örnek seti; davranış birebir aynı (`provider_migrate_test.go`) | ~1 gün |
| **2** | ✅ `Registry` örnek-tabanlı: `SetInstances()`, `resolve()` örnekten, `KindOf()`; `Agent.ProviderInstanceID` alanı + `Provider` senkron aynası (**K3**); §4.2'deki 5 karşılaştırma `Transport`'a; `openai-compat`/`anthropic-compat` kind'ları → eski `CustomSpec` yolu düştü | Aynı kind'dan 2 örnek testi (2 anthropic, farklı key) yeşil; "`Provider` her zaman kind" invaryant testi yeşil | ~1 gün |
| **3** | ✅ **BİTTİ** (2026-08-18): `/api/provider-kinds` (kind→form şeması), `/api/providers` CRUD (`GET`/`GET {id}`/`PUT`/`DELETE {id}`), `/api/settings/test-provider` örnek id kabul ediyor. Eski `settings.CustomProvider` API yazım yolu (`handleUpsertProvider`/`handleDeleteProvider`) kaldırıldı. `market_install.go`'daki `installProviderPack` aynı `upsertProviderInstance` doğrulama yoluna taşındı (`providers.json`'a yazıyor, `applySettings()` sonrası registry'de gerçek etkisi var) — `AllowMissingRequiredSecrets` (pack'in anahtarsız kurulabilmesi) ve `ExtraConfig` (legacy `reasoning`/`promptCache` bayraklarının standart olmayan `Config` anahtarları olarak taşınması) opsiyonları bu fazda eklendi. `ProvidersPanel` generic form (`ProviderInstanceList`/`ProviderInstanceForm`) — frontend | Backend+frontend: `go build`/`go vet`/`go test ./...` yeşil, CRUD+schema+secret-sızmama+silme-etki+market-pack-kurulum testleri yeşil (`internal/api/providers_test.go`) | ~1.5 gün |
| **4** | ✅ **BİTTİ** (2026-08-18): Ajan formu (`AgentSettingsForm`, `AgentsView` oluşturma formu) `ProviderInstanceModelSelect` bileşeniyle örnek listesinden (`/api/providers`) seçim yapıyor; kind rozeti optgroup ile gösteriliyor; model listesi seçilen örneğin kind manifest'i + örnek `models` override'ından türüyor. `internal/tools/builtin_agentmgmt.go` (`create_agent`/`update_agent` MCP araçları) `agentDeps.resolveProvider` üzerinden enjekte edilen `agent.SyncProviderFields`'i kullanıyor (döngüsel import'tan kaçınmak için dependency injection), bilinmeyen örnek id'si net hata döndürüyor. Silinmiş örneğe bağlı ajan formda kırmızı uyarı şeridiyle gösteriliyor, sessiz fallback yok | Örnek silinince ilgili ajanlar formda uyarılıyor, sessiz fallback yok | ~0.5 gün |
| **5** | ✅ **BİTTİ** (2026-08-18): eski typed `settings.Settings` **DTO/Patch/Store yüzeyi** (Minimax/OpenRouter/ZAI/DeepSeek/CustomProviders — `*KeySet`/`*BaseURL` DTO alanları, `*Key`/`*BaseURL` Patch alanları, `Store.MinimaxKey()`/`OpenRouterKey()`/`ZAIKey()`/`DeepSeekKey()`/`CustomProviderKey()`/`UpsertCustomProvider()`/`DeleteCustomProvider()`, `CustomProviderDTO`) kaldırıldı. Frontend: `ProvidersPanel.tsx`'ten 4 legacy kart (MiniMax/OpenRouter/Z.ai/DeepSeek) + dead `ProviderModelSelect.tsx` (Faz 4'ün `ProviderInstanceModelSelect`'i onun yerini almıştı, sıfır importer) kaldırıldı; `AppSettings`/`SettingsPatch`'ten karşılık gelen alanlar silindi. `InstanceCatalog()` örnek-bazlı hale getirildi (§7-sapma bkz. aşağı). Dokümanlar güncellendi. **Kapsam dışı / plandan sapma:** `AnthropicKeyEnc`/`Patch.AnthropicKey`/`DTO.AnthropicKeySet` **kaldırılmadı** — `internal/app/app.go`'da `ANTHROPIC_API_KEY` env değişkeninden `providers.json`'a bir kerelik boot-seed olarak hâlâ canlı okunuyor/yazılıyor (bu yüzden "Anthropic API" kartı da `ProvidersPanel.tsx`'te kaldı); dolayısıyla "`grep AnthropicKeyEnc` sıfır sonuç" ölçütü **geçerli değil** — bilinçli istisna | `go build`/`go vet`/`go test ./... -count=1` yeşil; `internal/settings` içinde yalnız `MigrateFromSettings`in okuduğu ölü `*Enc`/`CustomProvider` struct alanları kaldı (DTO/Patch/Store erişimi yok); frontend `tsc -b --force`/`npm test`/`format:check` yeşil (bilinen görev-dışı `modelLabel.test.ts` hatası hariç) | ~0.5 gün |

**Toplam ≈ 4.5 gün** (codex işi hariç). Gerçekleşen: aynı gün içinde (2026-08-18), fazlar art arda.

---

## 8. Kararlar

**Onaylandı (2026-08-18):**

- **K1 — CLI config evi: örnek başına.** Yeni CLI örneğinde boş `configDir`
  otomatik `<dataDir>/provider-homes/<instance-id>` olur; yalnız legacy boş
  örnekler workspace evine düşer. Detay §4.4.
- **K2 — Depolama: ayrı `providers.json`** (`<dataDir>`, AES-GCM, `internal/settings`
  altında ikinci store). Detay §2.4.
- **K3 — `Agent.ProviderInstanceID` yeni alan.** `Provider` alanı korunur ama
  **türetilmiş kind id'si**ne dönüşür ve her yazımda senkronlanır. Detay §2.5.

**Kapsam dışı bırakıldı (Faz 5, 2026-08-18):**

- **K4 — Örnek başına maliyet kırılımı.** Usage satırlarının `provider` alanı
  **kind** olarak kalıyor (fiyatlama ve geçmiş uyumu için); `providerInstanceId`
  alanı `db.Usage`/`db.SessionUsage`'a eklenmedi. Gerçek ihtiyaç görülene kadar
  ertelendi — iki Anthropic örneğinin maliyeti bugün tek satırda toplanıyor.

**Faz 5'te alınan ek kararlar:**

- **Catalog örnek-bazlı genişletme (madde 3).** `providers.Catalog()` (kind
  bazlı, `ID = kind`) **değiştirilmedi** — `resolveModelLabel`/
  `thinkingInfoForModel`/`Composer.tsx` bunu `agent.Provider` (K3 gereği kind)
  ile sorguluyor; kind bazlı entry'yi kaldırmak bu üç tüketiciyi kırardı.
  Bunun yerine `Registry.InstanceCatalog()` genişletildi: artık **her etkin
  örnek için** (yalnızca eski `openai-compat`/`anthropic-compat` değil) ayrı
  bir `CatalogEntry` üretiyor, `ID = instance.ID`, `MergeCatalog` aynı ID'de
  kind entry'sinin üzerine yazıyor. Sonuç: aynı kind'ın iki örneği
  `GET /api/catalog`'da ayrı görünür (`internal/providers/kind_test.go`
  `TestInstanceCatalogPerInstanceEntries`), varsayılan tek-örnekli kurulumlarda
  davranış değişmez (id çakışması → override, ekstra satır yok).
- **Korunan claude-cli-özel kontroller.** §4.2'nin `Transport`'a taşınması
  planlanan 5 karşılaştırmadan `internal/agent/runtime.go` ve
  `internal/agent/toolsetup.go` Faz 2'de `Transport == "cli"` genellemesine
  taşındı. İki tanesi kasıtlı olarak **claude-cli'ye özgü** kaldı — bunlar
  transport-genel değil, gerçekten yalnız claude-cli'nin yaptığı işler:
  `internal/api/session_context.go:484` (`provider != "claude-cli"`, claude-cli
  login/config-dizini kontrolü — codex-cli farklı bir doğrulama yolu kullanıyor)
  ve `internal/api/catalog.go:39,46` (`e.ID == "claude-cli"`, CLI sürüm +
  abonelik rozeti — codex-cli'nin kendi `e.ID == "codex-cli"` dalı ayrı satırda
  zaten var, bkz. `catalog.go:57`).

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
- `internal/providers` (Faz 5): `TestInstanceCatalogPerInstanceEntries` —
  `InstanceCatalog()` iki aynı-kind örneğine ayrı entry veriyor, devre dışı
  örneği ve kayıtsız kind'a sahip örneği atlıyor, kind manifest metadata'sını
  (`NeedsKey`/`Models`) örnek override'ı yoksa devralıyor.

---

## 11. Yeni bir sağlayıcı kind'ı eklemek

Yeni bir taşıma (transport) veya API-uyumlu uç eklemek **tek dosya** ile olur —
kayıt, ayar formu, API DTO'su ve UI otomatik türer:

1. `internal/providers/kind_<isim>.go` dosyası aç, `init()` içinde
   `RegisterKind(NewBuiltinKind(Manifest{...}, available, build))` çağır.
   Bkz. `kind_anthropic.go` (basit API-key kind'ı) veya
   `kind_anthropic_compat.go` (generic, kullanıcı örnek başına base URL girer).
2. `Manifest.Fields` ile örnek formunu ilan et — standart anahtarlar
   (`FieldKeyAPIKey`/`FieldKeyBaseURL`/`FieldKeyCLIPath`/`FieldKeyConfigDir`/
   `FieldKeyAuthKind`/`FieldKeyAuthToken`) `ResolvedConfig`'in typed alanlarına
   otomatik eşlenir (`Registry.resolve`); kind'a özel bir alan da eklenebilir,
   o zaman `cfg.Values[key]` ile build fonksiyonunda okunur.
3. `Manifest.Transport` = `"api"` (native tool loop sürer) veya `"cli"` (kendi
   ajan döngüsünü çalıştıran subprocess) — bu, hook-passthrough/skill-aracı-adı
   gibi davranış anahtarlarını otomatik doğru tarafa yönlendirir (§4.2).
4. `available`/`build` fonksiyonlarını yaz (bkz. mevcut `kind_*.go` dosyaları).
   Yeni bir dosya + `RegisterKind` dışında **registry'de, API katmanında ne de
   `ProvidersPanel.tsx`'te hiçbir değişiklik gerekmez** — `/api/provider-kinds`
   `providers.Kinds()`'ı gezerek formu, `ProviderInstanceForm` da o formu
   render eder.
5. Fiyatlandırma göstermek istiyorsan `internal/providers/pricing.go`
   `priceTable`'a kind slug'ı → model fiyatları ekle (opsiyonel; eksikse UI
   "—" gösterir).
6. Test: en az `Available`/`Build` için birim test + `Catalog()` sıralaması
   bozulmadığını doğrulayan bir satır (bkz. `kind_test.go`
   `TestCatalogDerivedFromKinds`'daki `wantOrder` listesini güncelle).

---

## İlgili dokümanlar

`17-TOKEN-OPTIMIZASYON.md` (provider soyutlaması, prompt-cache) ·
`51-CLAUDE-CONFIG-BIRLESIK.md` (claude-home) ·
`69-CODEX-CLI-SAGLAYICI.md` / `70-CODEX-CLI-UYGULAMA-PLANI.md` (paralel iş) ·
`02-VERI-MODELI.md` (entity/ID konvansiyonu) · `21-MARKET.md` (pack provider payload)
