# Bağlam Yönetimi Karşılaştırması — External Context Agent ↔ TionHarness

Kaynaklar (bu analizin okunduğu andaki durum):

| Depo | Yol | HEAD |
|------|-----|------|
| external-context-agent | `C:\Users\user\Desktop\Projects\external-context-agent` | `32fe129324` |
| TionHarness | `C:\Users\user\Desktop\Projects\TionHarness` | `3734f65c` |

> Not: codebase-memory indeksi `254158f453` üzerinde alınmıştı; yapısal sorgular
> (satır numaraları, karmaşıklık metrikleri) o commit'e aittir. Alıntılanan kod
> gövdeleri çalışma ağacından, yani `32fe129324`'ten okundu.

---

## 1. Katman haritası

Kullanıcı tarafından "iki katman" olarak adlandırılan yapı aslında **üç**
katmandır; her iki projede de üç katman var ama katmanların *rolleri* farklı.

### the external agent

| # | Katman | Dosya | Ne yapar | Varsayılan |
|---|--------|-------|----------|------------|
| 1 | **Native (sunucu taraflı)** | `agent/native_compaction.py` (569 satır, 14 fonksiyon) | OpenAI Responses API'ye `context_management=[{"type":"compaction","compact_threshold":N}]` gönderir; sunucu eski bağlamı şifreli (`encrypted_content`) opak bir `compaction` item'ına özetler | **kapalı** (`codex_responses_native: false`) |
| 2 | **Batch sıkıştırma** | `agent/context_compressor.py` → `ContextCompressor.compress()` (satır 7672–8539) | Beş fazlı istemci taraflı sıkıştırma: araç sonucu budama → head koruma → token bütçesiyle tail kesimi → LLM özeti → önceki özetin iteratif güncellenmesi | **açık** (`compression.enabled: true`, eşik `0.50`) |
| 3 | **Micro-compaction** | aynı dosya → `_micro_compact()` (satır 7223–7372) | Her turda **tek bir exchange**'i yuvarlanan (rolling) bir özet işaretine emer; imleç + splice ile ilerler | **kapalı** (`compression.micro_compact: false`) |

### TionHarness

| # | Katman | Dosya | Ne yapar | Varsayılan |
|---|--------|-------|----------|------------|
| 1 | **Native (CLI sağlayıcı taraflı)** | `internal/conversation/nativecompact.go` (117 satır) | Aktif CLI sağlayıcısından kendi penceresini sıkıştırmasını ister (`WithNativeCompact` ctx callback'i); TionHarness transkripti dokunulmadan kalır | **kapalı** (`AutoCompactMode: "rolling"`) |
| 2 | **Rolling fold** | `internal/conversation/manager.go` → `Manager.Prepare()` | Bütçe aşıldığında eski geçmişi `session.Summary` içine katlar, `SummaryMsgCount` filigranını ilerletir; **DB'ye kalıcı yazılır** | **açık** |
| 3 | **In-flight budama** | `internal/conversation/prune.go` → `PruneInFlightToolResults()` | Tur ortasında pencere taşarsa **önce** eski/büyük araç sonucu gövdelerini işaretçiye indirger; LLM çağrısı yok | tur taşmasında otomatik (2026-09-02) |
| 4 | **Reactive fold** | `internal/conversation/reactive.go` → `CompactInFlightMessages()` | Budama yetmezse uçuş hâlindeki mesaj dizisini özete katlar; DB'ye hiç dokunmaz | budama yetersizse |

**Yapısal eşleme:**

```
hermes native_compaction        ↔  TionHarness AutoCompactNative
hermes compress()               ↔  TionHarness Manager.Prepare() rolling fold
hermes _prune_old_tool_results  ↔  TionHarness PruneInFlightToolResults (tur içi)
hermes _micro_compact()         ↔  (karşılığı YOK)
(karşılığı YOK)                 ↔  TionHarness CompactInFlightMessages() reactive fold
```

İki asimetri var: hermes'te turdan sonra çalışan artımlı bir emme katmanı,
TionHarness'te tur *içinde* taşma anında devreye giren bir kurtarma katmanı.

---

## 2. En kritik fark: tetikleme sinyali

Bu, mimarideki tek en büyük ayrım.

**the external agent gerçek `prompt_tokens` üzerinden tetikler.** `update_from_response()`
her yanıttan sonra sağlayıcının bildirdiği gerçek token sayısını
`last_prompt_tokens`'a yazar; `should_compress()` doğrudan onu eşikle
karşılaştırır:

```python
def should_compress_info(self, prompt_tokens: int = None) -> "tuple[bool, str | None]":
    tokens = prompt_tokens if prompt_tokens is not None else self.last_prompt_tokens
    if tokens < self.threshold_tokens:
        return False, None
```

**TionHarness tahmin üzerinden tetikler.** Tokenizer bağımlılığı yoktur;
`internal/conversation/tokens.go` karakter sayımı yapar:

```go
const charsPerToken = 3          // 2026-07-23'te ~1.15M karakterlik gerçek metinle kalibre edildi
const denseMinRunes      = 256   // yoğun/kodlanmış içerik eşiği
const denseWhitespacePct = 3     // %3'ten az boşluk → ~1.5 karakter/token
```

Sonuçlar:

- the external agent sapmasız ama **reaktiftir**: eşiğin aşıldığını ancak taşmış bir isteğin
  yanıtı geldikten sonra öğrenir. Bu yüzden ayrıca bir preflight tahmin yolu
  (`note_request_rough_estimate`, `should_defer_preflight_to_real_usage`)
  taşımak zorunda kalmış.
- TionHarness **proaktiftir** ama sistematik hataya açıktır. Kalibrasyon notu
  bunu bilinçli olarak güvenli yöne çekmiş: 3 karakter/token gerçek oranın
  (~2.9–3.0) hafif üstünde sayar, yani katlama erken tetiklenir, geç değil.
- TionHarness'in tahmin yaklaşımı **mesaj dışı yükü de bütçeye katabilmesini**
  sağlıyor — hermes'te bunun doğrudan karşılığı yok (bkz. §4).

---

## 3. Bütçe hesabı

### the external agent

```
threshold_tokens = effective_input_budget × threshold_percent
```

- `compression.threshold` varsayılan **0.50** (pencerenin %50'si).
- Küçük pencereli modellerde taban: `_SMALL_CTX_THRESHOLD_PERCENT = 0.75`
  (yalnız yükseltir — `_effective_threshold_percent`).
- Model başına `compression.model_thresholds` sözlüğüyle ezilebilir.
- `compression.threshold_tokens` ile mutlak tavan konabilir.
- Tail bütçesi: `tail_token_budget = threshold_tokens × target_ratio`
  (`compression.target_ratio` varsayılan **0.20**).

### TionHarness

```go
// internal/conversation/budget.go
EffectiveBudget = clamp(window × fraction, MaxContextTokens, ContextBudgetCeil)
```

| Ayar | Varsayılan | Anlamı |
|------|-----------|--------|
| `maxContextTokens` | `800000` | **taban** (bütçe bunun altına inmez) |
| `contextBudgetCeil` | `262144` | tavan — 1M pencereli modeller için operatif sınır |
| `contextBudgetFraction` | `0` = auto | `providers.AdaptiveBudgetFraction` |
| `keepRecentMsgs` | `8` | katlamadan sonra aynen tutulan kuyruk |
| `reactiveKeepRecent` | `6` | reactive fold kuyruğu |

`AdaptiveBudgetFraction` aile başına **0.35–0.45** döner (opus/sonnet/fable
0.45; gemini/minimax/deepseek/glm 0.35). Gerekçe kodda açıkça yazılı: bağlam
çürümesi (context rot) — büyük ham pencere n² dikkat yüzeyini büyütür ve geri
çağırma kesinliğini aşındırır; katlanan detayın dayanıklılığı ham pencere
boyutuyla değil **erişim katmanıyla** (memory / `conversation_search` / core
blokları) taşınır.

**Karşılaştırma:** hermes'in %50'lik varsayılanı ile TionHarness'in %35–45'i
aynı büyüklük mertebesinde; TionHarness biraz daha muhafazakâr ve bunu bilinçli
bir bağlam-çürümesi argümanıyla gerekçelendiriyor. harici ajanin avantajı model
başına eşik sözlüğü; TionHarness'in avantajı aile-farkında otomatik oran.

---

## 4. Mesaj dışı yük (overhead) muhasebesi

TionHarness'in hermes'te doğrudan karşılığı olmayan bir mekanizması.

`Manager.Prepare` kapıyı **mesajlar + sabit yük** üzerinden geçirir:

```go
overhead := contextOverheadFrom(ctx)   // statik prefix + araç/skill katalogları
                                       // + eager araç şemaları + artifact bloğu
if before := EstimateTokens(summary, pending); before+overhead > maxTokens {
```

Yorumda gerekçe net: büyük bir sabit prefix, yalnız-mesaj tahminini sonsuza
kadar bütçe altında tutar, gerçek ayak izi ise bütçeyi aşar — o zaman hiç
katlama olmaz. `PredictCLIOverhead` (`clioverhead.go`) CLI sağlayıcıları için
bu yükü yüklenen araç sayısından tahmin eder.

Katlamadan sonra da düzeltme yapar: `foldedStepOverhead` ile artık silinmiş
mesajların persisted-Steps payı yükten düşülür, ama raporlanan "önce" değeri
kapının ateşlendiği andaki ayak izini korur (yoksa X→Y oranı olduğundan küçük
görünür).

Ayrıca `pressureWarnRatio = 0.85`: tur sığıyor ama kıl payı sığıyorsa debug
günlüğüne erken uyarı düşer. Bu, "kapı çalışıyor ve ateşlemek üzere" ile "kapı
gerçek ayak izini hiç görmüyor" durumlarını ayırt etmenin tek yolu.

the external agent tarafında bunun muadili dağınık: `_estimate_msg_budget_tokens`,
`_reasoning_details_text_chars`, `_stale_thinking_on_wire()` gibi
mesaj-içi düzeltmeler var ama sistem promptu / araç şeması gibi mesaj dışı
sabit yük için tek bir muhasebe noktası görünmüyor.

---

## 5. Kayıp politikası — ne korunur, ne atılır

### the external agent `compress()` faz sırası

1. **Faz 1 — LLM'siz ön geçiş.** `_prune_old_tool_results()` (satır 4033–4334,
   siklomatik 53 / bilişsel 135) eski araç sonuçlarını tek satırlık bilgilendirici
   özete indirger:
   ```
   [terminal] ran `npm test` -> exit 0, 47 lines output
   [read_file] read config.py from line 1 (3,400 chars)
   ```
   Kuyruk koruması hem token bütçesi hem mesaj sayısı ile; token bütçesi
   önceliklidir, mesaj sayısı `_MAX_TAIL_MESSAGE_FLOOR` ile sınırlı bir taban
   olarak davranır. Korunan bölge bile yumuşak bütçeyi (`tail × 1.5`) aşarsa
   *içeride* bir basınç geçişi yapar.
   Aynı fazda platform-echo boş kullanıcı satırları da temizlenir — ve bu faz
   **özet iptal edilse bile hayatta kalır**, yani iptal edilen bir sıkıştırma
   dahi değişmiş bir liste döndürebilir.
2. **Faz 2 — head koruma.** `protect_first_n` (varsayılan 3) + `_align_boundary_forward`.
3. **Faz 3 — token bütçesiyle tail kesimi.** `_find_tail_cut_by_tokens`.
4. **Faz 4 — LLM özeti.** `_generate_summary` (satır 4958–5637, siklomatik 41).
   Orta bölge eşiğin `_FEASIBILITY_SKIP_MIDDLE_FRACTION`'ından küçükse ve daha
   önce bir etkisizlik striki varsa LLM çağrısı **atlanır**.
5. **Faz 5 — iteratif özet güncellemesi** (yeniden sıkıştırmada).

Ek koruma mekanizmaları (hepsi `context_compressor.py` içinde):

- `_sanitize_tool_pairs` — tool_call / tool_result çiftlerinin kopmasını engeller.
- `_retire_stale_tool_result_images`, `_strip_historical_media` — geçmiş görsel
  içeriği emekliye ayırır.
- `_truncate_tool_call_args_json` — büyük araç argümanı JSON'unu kırpar.
- `_collect_protected_skill_names`, `_reinject_pruned_skill_markers` — budanan
  skill'lerin işaretlerini geri enjekte eder.
- `_validate_summary_user_provenance`, `_is_synthetic_compression_user_turn` —
  özetin sahte bir kullanıcı turu olarak kimliğini doğrular.
- `salvage_grown_transcript`, `_salvage_reduce_todo_snapshot` — sıkıştırma
  transkripti büyüttüyse kurtarma.
- `_redact_compaction_text` — özet metninden hassas içerik temizliği.

### TionHarness `foldBoundary()`

```go
func foldBoundary(history []db.Message, start, keepRecent int) (fold, keepTail []db.Message, newCount int, ok bool)
```

Tek bir sınır kararı: `history[start:len-keepRecent]` katlanır, kalanı aynen
kalır. Reactive fold ayrıca kesimi bir **assistant mesajında** yapar, böylece
rol dönüşümü (user→assistant) korunur ve bir `tool_use` kendi `tool_result`'ından
ayrılmaz.

Araç çıktısında iki ayrı kontrol vardır ve ikisi de fold'un dışındadır:

1. **Yazma anında sınır** — `MaxToolOutputKB = 100`
   (`internal/tools/registry.go`, `SetMaxToolOutputBytes`).
2. **Turlar arasında tam düşürme** — araç sonuçları yalnız `db.Message.Steps`
   içinde ekran için saklanır; `toProviderMessages` sağlayıcıya yalnız `Text`
   gönderir. Bir sonraki tur araç çıktısını hiç görmez. Bu, hermes'in
   budamasından daha agresiftir: hermes özet satırını korur, TionHarness gövdeyi
   tamamen bırakır.

Geriye kalan gerçek şişme **tur içidir**: bir araç döngüsünün `req.Messages`
içinde biriken sonuçları. harici ajanin Faz-1 budaması buraya karşılık gelir ve
2026-09-02'de oraya uygulanmıştır (§11-Ö1): `PruneInFlightToolResults`, reactive
fold'dan önce çalışan LLM'siz geçiş.

---

## 6. Dayanıklılık guard'ları

| Guard | the external agent | TionHarness |
|-------|--------|-------------|
| **Anti-thrashing (etkisizlik)** | ✅ Son iki sıkıştırma da <%10 kazandıysa geri çekilir; `should_compress_info` `"ineffective"` döner | ❌ Yok |
| **Başarısızlık cooldown'u** | ✅ 429/geçici hata sonrası 30–60 sn; `"cooldown:<saniye>"` sebebi çağırana döner | ❌ Yok |
| **Yapısal no-op backoff** | ✅ Etkisizlik strikinden **ayrı** sayaç (`_record_structural_no_op`); kısa oturumlarda breaker'ı kalıcı disarm etme hatasının düzeltmesi | ❌ Yok |
| **Statik fallback özeti** | ✅ `_build_static_fallback_summary` — LLM özeti düşerse LLM'siz özet | ❌ Yok |
| **Manuel geçersiz kılma** | ✅ `force=True` cooldown'u ve feasibility-skip'i atlar | ✅ manuel `/compact` ayrı yoldan |
| **Native anti-loop** | (gerek yok — eşik yerel tetiğin altına sabitli) | ✅ `claimNativeAttempt` — rolling-summary sınırı başına tek deneme |
| **Fold zaman aşımı** | ✅ `CompressionCommitFence` + iptal kontrolü (`conversation_compression.py`) | ✅ `foldCtx` + `FoldIdleOutputFloor = 10 dk` |
| **Gözlemlenebilirlik** | ✅ telemetri sözlüğü + `failure_class` | ✅ `debug.jsonl` (`DebugCompaction`) + basınç uyarısı + ekranda compaction adımı |

**TionHarness'te fold başarısız olursa tur tamamen düşer.** Doğrulandı —
`internal/api/chat_turn_phases.go:479`:

```go
prep, cerr := t.s.convo.Prepare(t.ctx, t.database, provider, t.session, agentRow, history)
if cerr != nil {
    t.failTurn(agentRow.ID, "compaction_failed", "compaction failed: "+cerr.Error())
    return out, false
}
```

the external agent aynı durumda özet üretmeyi bırakır, sebebi çağırana bildirir ve tur
sıkıştırılmamış bağlamla devam eder (sonunda sağlayıcı sınırına çarpar, ama
tek bir 429 yüzünden tur ölmez). harici ajanin kod yorumu bunun bilinçli olduğunu
söylüyor: cooldown erken sıfırlanırsa "yıkıcı statik-fallback"e düşülür ve veri
kaybı olur (#29559).

---

## 7. Native katman karşılaştırması

### the external agent — kasıtlı olarak dar

`native_compaction.py` modül docstring'i canlı doğrulama tarihi (Ağustos 2026)
ile birlikte kapıları sayıyor:

- **Yalnız gpt-5.6 ailesi.** gpt-5.1/5.2'ye alan gönderilirse sunucu tarafı
  güvenilir biçimde düşer: bloklayan yolda HTTP 500, akış yolunda kalıcı stall
  (90 sn watchdog × 3 retry = ölü tur). Yapılandırılmış "unsupported" reddi
  olmadığı için tek güvenli kapı açık model-ailesi kontrolü.
- **Yalnız doğrudan OpenAI rotaları**: `api.openai.com` veya ChatGPT Codex
  backend. Diğer her Responses yüzeyi (xAI, GitHub/external CLI agent, röleler, yerel
  sunucular) alanı hiç görmez.
- **Sahiplik modeli**: yerel sıkıştırma tam kurulu fallback sahibi olarak kalır.
  Native eşik yerel tetiğin **8192 token altına** sabitlenir
  (`LOCAL_TRIGGER_SAFETY_MARGIN`), böylece sunucu her zaman ilk atışı alır.
  Otomatik mod yerel tetiği inceleyemezse `DEFAULT_COMPACT_THRESHOLD = 200_000`.
- `compression.checkpoint_required: true` ise native **bastırılır** — sunucu
  taraflı sıkıştırma sağlayıcının sahip olduğu kayıplı bir sınırdır ve öncesinde
  hiçbir checkpoint çalıştırılamaz.

### TionHarness — sağlayıcıya delege

`WithNativeCompact` bir ctx callback'i taşır; `conversation` paketi hatayı
**sınıflandırmaz** (kullanılamazlık sentineli `api` paketinde yaşar), kural
basittir: `nil` = native oldu, `non-nil` = rolling fold'a düş.

Kritik tasarım notu (`nativecompact.go`):

> Native compaction shrinks the CLI's OWN window; it does not shrink the pending
> transcript TionHarness estimates, so `EstimateTokens(summary, pending)` is
> exactly the same on the next turn.

Bu yüzden `claimNativeAttempt` gerekiyor: bir oturum rolling-summary sınırı
başına yalnız **bir kez** native deneyebilir. Kalıcı olarak bütçe üstü turlar
native ↔ rolling arasında dönüşümlü ilerler ve yakınsar.

harici ajaninte bu sorun yoktur çünkü hermes gerçek `prompt_tokens`'a bakar — sunucu
gerçekten sıkıştırdıysa bir sonraki turun `prompt_tokens`'ı **düşer** ve kapı
kendiliğinden kapanır. TionHarness'in tahmin tabanlı kapısı bu geri beslemeyi
göremediği için ayrı bir anti-loop sayacı taşımak zorunda. **Bu, §2'deki
tetikleme sinyali farkının doğrudan bedeli.**

Ayrıca `AutoCompactMode: "auto"` (native-first + rolling emniyet ağı) modunda
native gerçekleşmediğinde `native_fallback_rolling` olayı günlüğe düşer —
"auto native vaat etti, ne oldu?" sorusunun cevabı.

---

## 8. Micro-compaction (TionHarness'te karşılığı yok)

`_micro_compact()` her turda **tek bir exchange** işler:

```
head_size → _align_boundary_forward → _find_tail_cut_by_tokens
          → _resolve_compact_cursor  (imleç)
          → _find_one_exchange       (tek alışveriş)
          → _micro_summarize_one     (küçük LLM çağrısı)
          → _splice_micro_compact_result
          → _sync_micro_compact_to_db
```

Tasarım detayları:

- **Cadence kapısı**: `micro_compact_every_n_turns` (varsayılan 1). Bir geçiş
  zaten gönderilmiş geçmişi yeniden yazar, yani **bir prompt-cache kırılmasına
  mal olur**; bu ayar operatörün geri kazanım sıklığını o maliyete karşı takas
  ettiği yerdir. Sayaç *çağrı başına* artar (geçiş başına değil), böylece emecek
  bir şey bulamayan bir tur da cadence'i ilerletir ve kilitlenemez.
- **Kümülatif özet**: `_micro_compact_rolling_summary` birikir; sonraki
  işaretler öncekileri `supersede` eder.
- **Defrag**: `_needs_defrag` / `_defrag_rolling_summary`, yuvarlanan özet
  `micro_compact_defrag_threshold_tokens` (varsayılan 2000) sınırını aşınca
  yeniden sıkıştırır.
- **Takılma koruması**: `_MICRO_COMPACT_MAX_CONSECUTIVE_FAILURES` aşılırsa imleç
  sıkışan exchange'in ötesine atlar; atlanan mesajlar transkriptte kalır ve bir
  sonraki batch sıkıştırma veya defrag tarafından emilir.
- `turn_finalizer.py:449` üzerinden **turdan sonra** çalışır, tur bütçe kapısında
  değil.
- `compression.checkpoint_required` açıksa zorla kapatılır
  (`agent_init.py:2888`).

Mimari değeri: bağlamı **uçurum kenarı** yerine **düz eğim** ile küçültür.
TionHarness'te tek bir büyük fold anı vardır (`Prepare` kapısı geçilince
`keepRecentMsgs=8` dışındaki her şey bir seferde özete gider) — micro-compaction
bunu sürekli, küçük, öngörülebilir adımlara böler.

---

## 9. Yapılandırma yüzeyi

**the external agent — ~25 `compression.*` anahtarı** (`agent/agent_init.py`):

```
enabled, threshold, threshold_tokens, model_thresholds, target_ratio,
protect_first_n, protect_last_n, tail_mode, min_tail_user_messages,
max_attempts, abort_on_summary_failure, checkpoint_required, in_place,
proactive_prune_tokens, proactive_prune_min_result_chars,
proactive_prune_min_reclaim_tokens, micro_compact,
micro_compact_every_n_turns, micro_compact_defrag_threshold_tokens,
codex_app_server_auto, codex_responses_native,
codex_responses_compact_threshold, codex_gpt55_autoraise,
codex_gpt55_autoraise_notice, idle_compact_after_seconds
```

**TionHarness — 5 ayar + 2 ortam değişkeni:**

```
maxContextTokens, keepRecentMsgs, contextBudgetFraction,
contextBudgetCeil, autoCompactMode
TIONHARNESS_CONTEXT_BUDGET_FRACTION, TIONHARNESS_CONTEXT_BUDGET_CEIL
```

Bu, felsefe farkı: hermes her davranışı operatöre açar, TionHarness sağlam
varsayılanlar seçip yüzeyi küçük tutar. TionHarness'in `contextBudgetFraction: 0`
= auto → aile-farkında oran çözümü, hermes'in `model_thresholds` sözlüğünün
elle bakım gerektiren muadilini otomatikleştiriyor.

---

## 10. Ölçek ve karmaşıklık

| Metrik | the external agent | TionHarness |
|--------|--------|-------------|
| Ana sıkıştırma dosyası | `context_compressor.py` — **8873 satır** | `manager.go` — **753 satır** |
| Destek dosyası | `conversation_compression.py` — 5997 satır (eşzamanlılık/fence/lease) | `nativecompact.go` 117 + `reactive.go` 124 + `tokens.go` 133 + `repair.go` 279 + `budget.go` |
| Ana fonksiyon | `compress()` — 868 satır, siklomatik **68**, bilişsel **127** | `Prepare()` — ~130 satır |
| En karmaşık yardımcı | `_prune_old_tool_results` — siklomatik 53, bilişsel 135 | — |
| Sembol sayısı (sıkıştırma dosyası) | 174 (1 sınıf, ~110 metot, 63 modül fonksiyonu) | ~15 |
| Toplam `internal/conversation` | — | 4507 satır (testler dâhil) |

harici ajanin karmaşıklığı büyük ölçüde **sağlayıcı çeşitliliğinden** geliyor:
reasoning item custody, encrypted replay, cross-issuer stamping, gateway session
replay, xAI/GitHub/Codex/Anthropic yol farkları. TionHarness bu yükün çoğunu
CLI sağlayıcısına delege ediyor (`AutoCompactNative` = "sen hallet").

---

## 11. TionHarness için somut çıkarımlar

Değer/maliyet sırasına göre:

### Ö1 — Geriye dönük araç sonucu budaması (LLM'siz) ✅ UYGULANDI (2026-09-02)

the external agent Faz 1'in muadili.

**Uygulama sırasında düzeltilen tespit.** Bu bölümün ilk hâli budamanın yeri
olarak `Prepare` kapısını gösteriyor ve "transkripte giren araç çıktısı oturum
boyunca o boyutta kalıyor" diyordu. Kod bunu doğrulamadı: `toProviderMessages`
yalnız `db.Message.Text` gönderir (`manager.go:764`), `Steps` alanı yalnız ekran
için saklanır. Yani TionHarness araç sonucunu **bir sonraki tura hiç taşımaz** —
bu noktada hermes'ten zaten daha agresiftir. Şişme her zaman **tur içindedir**:
uzun bir araç döngüsünün `t.req.Messages` içinde biriken sonuçları
(`internal/agent/toolloop_phases.go`). Budama da oraya kondu.

Uygulanan kapsam:

- `conversation.PruneInFlightToolResults` (`internal/conversation/prune.go`):
  koruma kuyruğunun (`ReactiveKeepRecent`) dışındaki, 4 KB'den büyük
  `ToolResult.Content` gövdelerini tek satırlık bir işaretçiyle değiştirir
  (`[tool result pruned … N lines dropped …]`). `CallID`/`IsError` ve mesaj
  yapısı korunur → `RepairSequence`'in `tool_use`↔`tool_result` eşleşme
  değişmezi bozulmaz. Girdi dilimi mutasyona uğratılmaz; işaretçi kendi çıktısını
  tanır (idempotent).
- `RawContent` taşıyan mesaj atlanır: o mesaj sağlayıcıya birebir echo edilir,
  `ToolResults` tele hiç çıkmaz — budamak hayali bir kazanç raporlamak olurdu.
- Devreye giriş noktası `compactAndRetry`'nin **önü**
  (`internal/agent/toolloop_prune.go`): önce bedava budama, yetmezse LLM fold.
  `PruneSufficient` kararı model penceresinin %70'ine göre verilir; pencere
  bilinmiyorsa budama yeterli sayılır (fold bir sonraki taşmada hâlâ elde,
  çünkü `ls.compacted` işaretlenmez).
- CLI-wrapper sağlayıcıda budama **atlanır**: taşan transkript CLI'ın kendi
  thread'idir, `req.Messages` yalnız delta'dır — bizim tarafı budamak pencereyi
  küçültmez. Doğrudan fold'a gider (o da CLI oturumunu özetle yeniden başlatır).
- Görünürlük: `StepCompaction` kartı (`Trigger: "prune"`, `FoldedMsgs` 0) +
  `debug.jsonl` olayı. Budama yetersiz kalsa bile kart basılır — sessiz yeniden
  yazma yasak.
- Testler: `internal/conversation/prune_test.go` (8), agent döngüsünde
  `internal/agent/recovery_prune_test.go` (2: budama tek başına yettiğinde
  özetleyici hiç çağrılmıyor; budanacak şey yoksa eski fold yolu aynen koşuyor).

**Kapsam dışı:** `EstimatePersistedSteps`'in ölçtüğü CLI persisted-thread yükü.
Orası TionHarness'in yazamadığı bir transkript; küçültmesi ancak CLI'ın kendi
native compact'iyle olur (§7).

### Ö2 — Fold başarısızlığında turu düşürmemek ✅ UYGULANDI (2026-09-02)

`chat_turn_phases.go:479` `failTurn` çağırıyordu; artık çağırmıyor. Uygulanan
kapsam:

- Fold gövdesi `applyRollingFold`'a taşındı (`internal/conversation/fold.go`);
  yalnız özetleyici LLM çağrısının hatası `errFoldSummary` ile işaretleniyor.
  Diğer her hata (özeti oturuma yazma, persisted-step yükünü yeniden ölçme)
  ölümcül kaldı — turun bağlı olduğu yerel durum.
- `Prepared.FoldFailed` + `FoldError`; ekranda `foldFailedLeadStep`, journal'da
  `fold_failed` (`ErrorKind: compaction_failed`).
- Oturum başına 45 sn stand-down (`foldFailureCooldown`,
  `internal/conversation/foldfailure.go`) + `fold_cooldown` olayı. Manuel
  `/compact` stand-down'a bakmaz.
- Testler: `internal/conversation/foldfailure_test.go` (6 test).

**Uygulanmayan (bilinçli):** hermes'in `_build_static_fallback_summary`
muadili — LLM'siz statik özet. Ayrı iş; TSK838 kapsamı dışında bırakıldı.
Ayrıca `chat_btw.go` ve `wake_turn.go` da `Prepare` çağırıyor; oralarda fold
hatası artık turu düşürmüyor ama **ekran uyarısı yok** (o yollarda lead-step
kanalı yok) — journal olayı ikisini de kapsıyor.

### Ö3 — Etkisizlik breaker'ı

`Prepare` bugün bütçe üstü her turda katlamaya çalışır. Fold %10'dan az
kazandırdıysa (`BeforeTokens` / `AfterTokens` zaten hesaplanıyor) art arda
ikinci seferde geri çekilmek, hermes'in #40803 için eklediği korumanın aynısı.
Veri hazır — yalnız oturum başına iki sayaç gerekiyor.

### Ö4 — Micro-compaction

En büyük mimari ekleme, en yüksek risk. Prompt-cache kırılma maliyeti gerçek ve
TionHarness'in cache stratejisiyle (prompt-epoch dondurma,
`internal/agent/promptepoch`) etkileşimi ayrıca incelenmeli. Ö1–Ö3'ten sonra
değerlendirilmeli.

### Ö5 — Gerçek `prompt_tokens` geri beslemesi

Sağlayıcı `Usage` zaten `recordCompaction` içinde okunuyor. Bir önceki turun
gerçek `PromptTokens`'ını tahminle karşılaştırıp bir kalibrasyon katsayısı
tutmak, hem `charsPerToken` sapmasını hem de §7'deki native anti-loop
karmaşıklığını azaltır (native gerçekten sıkıştırdıysa gerçek token düşer,
`claimNativeAttempt`'e gerek kalmaz).

---

## 12. harici ajanin TionHarness'ten öğrenebileceği

Simetri için:

- **Mesaj dışı yük muhasebesi** (§4) — hermes'in kapısı sistem promptu / araç
  şeması yükünü tek noktadan görmüyor.
- **Aile-farkında otomatik bütçe oranı** — `model_thresholds` elle bakım
  gerektiren bir sözlük; `AdaptiveBudgetFraction` gerekçeli bir varsayılan.
- **Bağlam-çürümesi gerekçesi** — TionHarness bütçeyi bilinçli olarak pencerenin
  altında tutup dayanıklılığı erişim katmanına yıkıyor; hermes'te bu argüman
  yazılı değil.
- **Basınç erken uyarısı** (`pressureWarnRatio = 0.85`) — "kapı çalışıyor ama
  henüz ateşlemedi" ile "kapı gerçek ayak izini hiç görmüyor" ayrımı.
