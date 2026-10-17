# 95 — OpenCode CLI Sağlayıcı ve Native Anthropic'in Kaldırılması

> **Özet (2026-10-13):** TionHarness'in üçüncü CLI taşıyıcısı `opencode-cli`
> ([anomalyco/opencode](https://github.com/anomalyco/opencode)) eklendi ve native
> Anthropic HTTP istemcisi (`anthropic.go`) ile ona dayanan kind'lar (`anthropic`,
> `anthropic-compat`, `minimax-anthropic`, `deepseek-anthropic`, `zai`) kaldırıldı.
> Karar: proje "üç CLI" modeline geçer — `claude-cli` (abonelik), `codex-cli`
> (ChatGPT/Codex aboneliği) ve `opencode-cli` (75+ sağlayıcıya opencode üzerinden
> erişim). OpenCode tur başına `opencode run --format json` alt süreci olarak
> çalışır; TionHarness araçları Interaction MCP köprüsüyle ulaşır. Aynı gün ikinci
> adımda OpenAI-uyumlu HTTP ailesi (`openrouter`, `deepseek`, `minimax`, `lmstudio`,
> `openai-compat`) de kaldırıldı; karar katmanı (`internal/decider`) artık sağlayıcı
> örneklerinden anahtar ödünç almaz, her karar modeli kendi anahtarını taşır (§7).
> Dayandığı paketler:
> `internal/providers` (`opencodecli*.go`, `kind_opencodecli.go`), `internal/agent`
> (`codexhome.go`, `codexmcp.go`, `toolloop_phases.go`), `internal/api`
> (`catalog_opencodecli.go`, `provider_auth.go`, `chat_resume.go`).

## 1. Neden OpenCode, neden şimdi

Soru şuydu: *"Bütün projede alt yapı olarak opencode'u tek harness kaynağı olarak
kullanabilir miyiz?"* Değerlendirme (sohbet kaydı, 2026-10-13):

- **Tek harness olarak hayır.** OpenCode dokümantasyonu Anthropic'in Claude Pro/Max
  aboneliğinin opencode üzerinden kullanılmasını yasakladığını yazar (1.3.0'dan
  itibaren ilgili eklentiler kaldırılmış). TionHarness'in ana yolu `claude-cli`
  tam olarak bu abonelik yoludur; OpenCode'a devredilemez. Codex tarafı ise ChatGPT
  aboneliğiyle opencode içinde de çalışır, ama `codex-cli` köprüsü (resume, usage
  muhasebesi, plugin desteği) zaten olgundur.
- **Üçüncü CLI olarak evet.** OpenCode, TionHarness'in native HTTP kind'larıyla
  ulaştığı her şeye (OpenRouter, DeepSeek, yerel modeller, OpenAI API anahtarı)
  tek köprüyle ulaşır. Böylece native Anthropic istemcisi ve Anthropic-protokollü
  kind'lar silinebildi; prompt-cache, efor ve araç menüsü yönetimi opencode'a geçti.
- **Kullanıcı kararı (2026-10-13):** tur başına alt süreç (`opencode run`), native
  Anthropic parite ölçümü beklenmeden silinir, doküman yazılır, baştan sona uygulanır.

## 2. Doğrulama ortamı

| Kaynak | Konum |
|---|---|
| OpenCode CLI | `opencode 1.19.0` — `npm i -g opencode-ai` (bu Mac'te `/opt/homebrew/bin/opencode`) |
| Dokümanlar | `https://opencode.ai/docs/{cli,config,agents,providers,server,sdk}/` |
| Canlı sondalar | `opencode run --format json` ile ücretsiz `opencode/big-pickle` modeli; stdin prompt, XDG izolasyonu, dosya tabanlı config, `-s` resume, izin reddi |

Canlı doğrulanan davranışlar:

- `opencode run [message..]` mesaj verilmezse **stdin'den okur** (Windows ~32 KB
  komut satırı sınırı için gerekli).
- `--format json` satır başına bir JSON olayı yazar: `step_start`, `text`,
  `reasoning`, `tool_use`, `step_finish`, `error`. Her olay üst düzeyde
  `sessionID` taşır (`-s <id>` ile sabit; `ses_` öneki).
- `step_finish.part.tokens` = `{input, output, reasoning, cache{read,write}}`;
  **`input` önbellekten okunanı içermez** (ölçüm: sıcak adımda input 152 + cache.read
  6659). Bu yüzden `Usage` eşlemesi doğrudan toplama, codex'teki gibi çıkarma değil.
- `tool_use.part.state.status` ∈ `pending|running|completed|error`; `error`
  durumunda `state.error` metni gelir. İzin reddinde opencode kendisi
  `"The user rejected permission to use this specific tool call."` döndürür.
- `run` modunda izin istekleri **otomatik reddedilir** (`permission requested …;
  auto-rejecting`). Yani `ask` modu per-tool onay değildir (codex `exec` ile aynı
  sınır).
- `XDG_DATA_HOME` + `XDG_CONFIG_HOME` ortam değişkenleri auth deposunu
  (`<data>/opencode/auth.json`) ve oturum veritabanını (`opencode.db`) izole eder.
- `OPENCODE_CONFIG=<dosya>` ile verilen `opencode.json` içinde `agent.<ad>.prompt =
  "{file:./system.md}"` çalışır; özel `mode: primary` ajan `--agent <ad>` ile seçilir
  ve sistem promptu opencode'un varsayılan "build" personasının yerine geçer
  (doğrulama: yanıt "TH: Hi!" ile başladı).

## 3. Tasarım

### 3.1 Kind ve ev (home) düzeni

- Kind id `opencode-cli`, `Transport: cli`, `AppliesToolHooks: false`, `Order: 12`.
  Alanlar: `cliPath`, `configDir`, `effortVariant` (`""` | `auto`).
- Model id'leri opencode biçimindedir: `<vendor>/<model>` (`anthropic/claude-sonnet-5-5`,
  `openai/gpt-5.5`, `opencode/big-pickle`). Boş id opencode evinin varsayılanını kullanır.
  Aile tabloları (`ContextWindowFor`, `MaxOutputFor`, thinking tier'ları, fiyat tahmini)
  `BareModelForKind` ile son `/` segmentine bakar.
- Ev: `<dataDir>/opencode-home` (uygulama geneli) ya da örneğin kendi `configDir`'i
  (`<dataDir>/provider-homes/<instance-id>`, oluşturmada otomatik). Alt süreç
  `XDG_DATA_HOME=<home>/data`, `XDG_CONFIG_HOME=<home>/config`, `XDG_CACHE_HOME`,
  `XDG_STATE_HOME` ile başlar; `opencode auth login` bu ev içinde yapılır.
- Tur başına yalnız `<home>/turns/turn-XXXX/{opencode.json,system.md}` yazılır ve tur
  sonunda silinir; bir saatten eski artıklar süpürülür. Codex'teki gibi scope başına
  ayrı ev **yoktur**: oturum veritabanı paylaşımlıdır, `-s` herhangi bir oturumu
  sürdürür.

### 3.2 Komut satırı ve config

```
opencode run --format json --agent tionharness [-m <vendor/model>] [--variant <v>]
             [--dir <workdir>] [-s <ses_...>]      # prompt stdin'den
```

Üretilen `opencode.json` (`opencodecli_config.go`):

| Alan | Değer |
|---|---|
| `agent.tionharness` | `mode: primary`, `prompt: {file:system.md}` (statik sistem promptu + interaction notu) |
| `agent.general.disable` | `true` (opencode'un görünmez alt-ajanı kapalı) |
| `permission` | auto → `{"*":"allow"}`; ask → `edit/bash: ask` (otomatik reddedilir, köprü araçları gerçek onayla çalışır); read-only → `edit/bash/webfetch: deny` |
| `tools` | `task: false` daima; köprülü kabuk açıksa `bash: false`; `NativeWebSearch` kapalıysa `websearch/webfetch: false`; read-only'de mutasyon araçları kapalı; yardımcı (araçsız) çağrıda **tüm** yerleşikler kapalı; MCP `DisabledTools` → `<server>_<tool>: false` |
| `mcp` | Interaction köprüsü iki girdi (`tionharness_interaction` `/core?full=1`, `tionharness_extended` `/extended?full=1`, Bearer başlığı) + etkin harici MCP sunucuları (remote `url/headers` ya da local `command/environment`) |
| `share` / `autoupdate` | `disabled` / `false` |

Dinamik bağlam (`SystemDynamic`, özet) codex yolundaki gibi stdin promptunun
`[Context]` bloğunda gider; statik önek `system.md`'de kalır (önbellek için).

### 3.3 Olay ayrıştırıcı (`opencodecli_events.go`)

- `text` parçaları: son gelen nihai yanıt, öncekiler ara `text` adımı.
- `tool_use`: `callID` ile izlenir, `completed`/`error`'da yayınlanır. Araç adları:
  `<serverkey>_<tool>` → `mcp__<serverkey>__<tool>` (mevcut önek soyma yardımcıları
  çalışsın diye), `todowrite` → `todo_list` (checklist kartı + progress aynası).
- `step_finish`: kullanım toplanır; ilk adımın `input+cache.read+cache.write`
  değeri `FirstCallPromptTokens`, adım sayısı `ProviderCalls`.
- `error`: `name`/`data.message` sınıflandırılır (auth/quota/model); auth ve model
  hataları `permanentProviderError` olur, ilk görüldüğünde süreç erken öldürülür.
- Kesilen akış `salvage()` ile kısmi yanıt olarak `CLIInterruption` içinde döner.

### 3.4 Süreç yönetimi

`runAttempt` codex'in kalıbını izler: `proc.CommandContextNested` + `TreeKill`,
procwatch ledger'ı (`watchCLI`), 120 sn başlangıç ve paylaşılan stdout-idle
penceresi (ayarlardaki `codexStdoutIdleSec` her iki taşıyıcıya uygulanır), ortam
`proc.HardenedEnv(CredentialSafeEnvExcept(..., OPENCODE_*))`. Temiz çökme (hiç
`step` görülmedi, araç koşmadı) bir kez yeniden denenir; `OPENCODE_CONFIG_CONTENT`
ve `OPENCODE_PERMISSION` boşaltılır ki ortamdan sızan bir config turu değiştirmesin.

### 3.5 Resume

`ScopedCLIResumer` uygulanır: `<home>/scopes/<sha256(scope)>.json` dosyası scope'un
son opencode oturum id'sini tutar; `CanResumeScoped` kayıtlı id ile gelen id'yi
karşılaştırır. API tarafında `planCodexResume` aynen kullanılır
(`chat_resume.go`, `case "codex-cli", "opencode-cli"`). Native compaction olayı
**yoktur** (`CLICompactionLifecycle` uygulanmaz); TionHarness kendi özet katlamasını
yapar.

### 3.6 Ajan katmanı ve API

- `PinOpenCodeHome` (`codexhome.go`) evi seçer ve oluşturur; `PinCLIHome` üçünü de
  çağırır. `spawn.go`, `toolloop_phases.go` ve `configureCLIMCP`
  (`usesServerSpec`: codex **ve** opencode `CLIMCPSpec.Servers` tüketir) güncellendi.
- `api/provider_auth.go`: `opencode-cli` örneği için durum = kurulu mu + evde hangi
  vendor'ların kimlik bilgisi var (`auth.json` anahtarları). Giriş diyaloğu yok;
  kullanıcı `opencode auth login`'i ilgili `XDG_DATA_HOME` ile çalıştırır. Ücretsiz
  `opencode/*` modelleri kimlik bilgisi istemez.
- Katalog: `cliVersion` ve `subscription` (kimlik bilgisi olan vendor listesi).
- Frontend: `providerMetadata.ts` etiket/alan yardımları, `ProviderInstanceList`
  durum satırı (diyalog düğmesi yalnız claude/codex), i18n `providers.opencodeCli`,
  `fields.help.opencodeCli.*`, `fields.labels.effortVariant`.

### 3.7 Efor varyantı

`--variant` opencode'da vendor'a özgüdür ve bilinmeyen değer turu düşürür. Bu yüzden
yalnız örnekte `effortVariant=auto` seçiliyse ve model `openai/` (low|medium|high|xhigh)
ya da `anthropic/` (low|medium|high|max) ile başlıyorsa geçirilir; `DisableThinking`
hiç geçirmez. Diğer vendor'lar için ajanın düşünme seviyesi yok sayılır.

## 4. Canlı doğrulama (2026-10-13, scratch sunucu 8091)

1. `PUT /api/providers` ile `opencode-cli` örneği oluşturuldu; `configDir`
   otomatik `provider-homes/opencode-cli`; katalog `available=true, cliVersion=1.19.0`.
2. `opencode/big-pickle` modelli ajan + oturum; `POST /api/chat` "Reply with exactly:
   pong" → yanıt `pong`, kullanım `in=40350 out=3` (TionHarness sistem promptu +
   köprü araç kataloğu dahil).
3. İkinci tur: "todo_write çağır … sonra get_session_info" → köprü istekleri
   `/mcp/interaction/core` ve `/extended` 200 döndü, sunucu günlüğü `steps=3`,
   kullanım `in=983 out=102 cacheRead=81024` (resume ile sıcak önek).
4. Birim testleri: `opencodecli_test.go` (ayrıştırıcı fikstürleri canlı akıştan,
   args/variant, config üretimi, scope kaydı, sahte ikiliyle temiz çökme → tek
   yeniden deneme, auth hatası → kalıcı ve yeniden denenmez).

## 5. Kaldırılanlar (native Anthropic)

| Katman | Silinen / değişen |
|---|---|
| `internal/providers` | `anthropic.go`, `anthropic_claude55.go`, `anthropic_effort.go`, `claude_models.go`, `kind_anthropic.go`, `kind_anthropic_compat.go`, `kind_minimax_anthropic.go`, `kind_deepseek_anthropic.go`, `kind_zai.go` ve testleri; `TokenCounter` arayüzü; registry beta alanları (`SetAnthropicBetas`); `thinking.go` içindeki Anthropic'e özgü yüklemler; `webSearchName/webFetchName/renderWebToolResult` → `cli_webtools.go` (claude-cli akışı kullanıyor); zaman aşımı sabitleri → `timeouts.go` |
| `internal/agent` | `systemagent_route.go` (anthropic'e yardımcı çağrı yönlendirmesi, karantina, geri dönüş) ve `auxNativeRouting` tunable'ı; `nativeToolSearch`/`programmaticTools`/`webTools`/`serverCompaction`/`autonomousTaskBudget` tunable'ları ve `prepareNativeLoop` içindeki kapıları; `steerRoleFor` (steer artık her yerde user rolü) |
| `internal/api` | `prefix_calibration.go` (sunucu tarafı kesin token sayımı); `session_context` "anthropic" önbellek önizlemesi ve `?accurate=1`; `server.go` beta/tunable itmeleri; `market_install.go` "anthropic" türü paketleri açıklayıcı hatayla reddeder |
| `internal/settings` / `app` / `config` | `AnthropicKeyEnc`, `AnthropicKey` (patch), `AnthropicKeySet`, yedi Anthropic beta anahtarı, `AuxNativeRouting`, `AutonomousTaskBudgetTokens`; `ANTHROPIC_API_KEY` ortam tohumlaması; migrasyondaki `anthropic` örneği |
| Frontend | `AnthropicOptions.tsx`, ayar alanları/tipleri, `cli.auxRouting`, kaldırılan kind'ların metadata ve i18n girdileri, bütçe etiketi |

Davranış notları:

- Eski `settings.json` dosyalarındaki Anthropic alanları yok sayılır; eski
  `providers.json` içindeki `anthropic`/`zai`/… örnekleri "unregistered kind" olur ve
  bu örneklere bağlı ajanlar sağlayıcı seçene kadar çalışmaz (kataloğa düşmezler).
- Claude 5.5/Fable aile tabloları (`claude55.go`, `thinking.go`) kaldı: claude-cli ve
  opencode üzerinden Anthropic modelleri için pencere/tier/fiyat tahmini üretirler.
- `Request`/`Response` üzerindeki Anthropic'e özgü alanlar (`ThinkingBudget`,
  `OutputSchema`, `ProgrammaticTools`, `WebTools`, `ContainerID`, `RawContent`,
  `InputTransformations`, `StopPauseTurn/StopRefusal`) **şimdilik duruyor**; native
  döngü OpenAI-uyumlu kind'lar için hâlâ çalıştığından ayrı bir sadeleştirme turuna
  bırakıldı (§6).

## 6. Açık kararlar ve sonraki adımlar

1. **OpenAI-uyumlu HTTP ailesi — yapıldı (2026-10-13, §7).** Karar katmanı kendi
   anahtarına geçince `openrouter`/`deepseek`/`minimax`/`lmstudio`/`openai-compat`
   kind'ları ve `minimax.go`/`transport.go` istemcileri silindi.
2. **Native tool loop.** Üretimde hiçbir kind artık `Transport: api` değil; yine de
   `toolloop_phases.go` içindeki `prepareNativeLoop`/`runNativeLoop`, builtin dosya
   araçları, `recovery.go`/`toolloop_prune*.go` ve `Request`'in Anthropic alanları
   duruyor. Sebep: ajan katmanının yüzlerce testi sahte native sağlayıcılarla bu
   döngüyü kullanıyor ve köprü araçları aynı registry'den besleniyor; silmek ayrı,
   büyük bir temizlik turudur (testlerin CLI sahtesine taşınması gerekir).
3. **İzin modu — yapıldı (2026-10-13).** `ask` modunda yerleşik edit/bash otomatik
   reddedilir; gerçek onay yalnız köprü araçlarında vardır. Ajan formu codex-cli ve
   opencode-cli için Sor seçiliyken "Köprü onayı" notu gösterir
   (`AgentSettingsForm.tsx`, `settings.bridgeApprovalNote`).
4. **Efor varyantı doğrulaması.** `--variant` eşlemesi dokümana dayanır; anthropic/
   openai anahtarlı bir evde canlı doğrulanmadı (ücretsiz model varyant almıyor).
5. **Fiyat tahmini.** Yalnız `anthropic/` ve `openai/` önekleri liste fiyatına
   eşlenir; `openrouter/...` ve diğer vendor'lar "fiyatsız" görünür.

## 7. İkinci adım: HTTP ailesi ve karar katmanı (2026-10-13)

Kullanıcı "önerdiğin en sade şekilde" deyince §6'daki 1. karar uygulandı:

- **Karar katmanı kendi anahtarıyla.** `CredentialsProvider`, `ProviderInstanceID`,
  `EndpointSource`/`InstanceInfo`, `Hub.ProviderCandidates`, `Backend.Accepts`,
  `Manifest.ProviderKinds` ve `api/decider_wiring.go` silindi. Her karar modeli
  `baseUrl` + şifreli `key` taşır; anahtar gerektiren bir backend anahtarsız
  kaydedilebilir ama "no API key" durumunda kalır ve çağrılmaz (`clientFor`
  `ErrNoEndpoint`). Eski `providers.json`/`models.json` kayıtlarında `credentials:
  "provider"` olan modeller `own` olarak yüklenir ve anahtar girilmesini bekler.
  Hazır şablonlar (`jev-openrouter`, `jev-openrouter-systemone`) artık OpenRouter
  anahtarını doğrudan ister. Frontend: ödünç alma seçimi, aday listesi ve
  `viaProvider` etiketi kalktı; kurulum rehberinin 1. adımı "API anahtarı".
- **HTTP kind'ları silindi.** `kind_openrouter/deepseek/minimax/lmstudio/openai_compat`,
  `minimax.go`, `openai_compat_*.go`, `transport.go` (yalnız `retryafter.go` kaldı),
  `think.go`, `coalesce.go`, `local_health.go`, `registry_http.go`,
  `context_window_local.go`, `timeouts.go`; `Instance.Reasoning/PromptCache`,
  `Registry.Reachable`, `Manifest.NeedsBaseURL/TemplateOnly/RequestTimeoutSecs`,
  `FieldKeyAPIKey/BaseURL`. API: `/api/providers` artık `reachable` alanı
  döndürmez; market "provider" paketleri (`openai` türü dahil) açıklayıcı hatayla
  reddedilir; oturum önbellek önizlemesinde yalnız `claude-resume`/`none` modu var.
  Fiyat tabloları (`pricing.go`) veri olarak kaldı (opencode üzerinden gelen
  `anthropic/`, `openai/` modelleri ve decider OpenRouter fiyatı için).
- **Kalan kind'lar:** `claude-cli`, `codex-cli`, `opencode-cli`. Eski `providers.json`
  içindeki `openrouter` vb. örnekler "unregistered kind" olur; onlara bağlı ajanlar
  Ayarlar'dan bir CLI örneğine taşınmalıdır (en kolayı: aynı vendor'ı opencode evinde
  `opencode auth login` ile tanımlayıp `opencode-cli` örneği seçmek).
- **Açılış tohumlama ve anahtar taşıma (2026-10-17).** `providers.json` tek seferlik
  göçten sonra oluşan `opencode-cli` türünü içermiyordu. Artık her açılışta
  `opencode` ikilisi PATH'te bulunursa ve o türden hiç örnek yoksa
  `ProviderStore.EnsureKindInstance` varsayılan `opencode-cli` örneğini ekler.
  Ardından `ExportLegacyKeysToOpenCode` kayıtsız kind örneklerinin `key` sırrını
  `<dataDir>/opencode-home/data/opencode/auth.json` içine `{"type":"api","key":…}`
  biçiminde yazar (eşleme: `anthropic→anthropic`, `openrouter`/`openai-compat`+
  openrouter.ai→`openrouter`, `deepseek(-anthropic)→deepseek`,
  `minimax(-anthropic)→minimax`, `zai→zai`). Depoda zaten bulunan bir vendor hiç
  üzerine yazılmaz; eski örnekler silinmez, kullanıcı listeden kaldırır.

## İlgili dokümanlar

- [69-CODEX-CLI-SAGLAYICI.md](69-CODEX-CLI-SAGLAYICI.md) — ikinci CLI köprüsünün
  kalıbı (ev izolasyonu, JSONL ayrıştırma, MCP köprüsü); opencode aynı kalıbı izler.
- [71-SAGLAYICI-ORNEKLERI-PLANI.md](71-SAGLAYICI-ORNEKLERI-PLANI.md) — taslak→örnek
  modeli; `anthropic` migrasyon satırı artık geçersiz.
- [74-SISTEM-AJANLARI.md](74-SISTEM-AJANLARI.md) — yardımcı çağrı yönlendirmesi
  (`auxNativeRouting`) kaldırıldı; yardımcı çağrılar sistem ajanının kendi sağlayıcısında koşar.
- [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) — "native yolda parite" kararı
  bu değişiklikle kapanmıştır; Anthropic'e yalnız claude-cli ve opencode üzerinden ulaşılır.
