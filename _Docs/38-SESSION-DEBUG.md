# TionSwarm — Oturum Debug Günlüğü (Paralel Gözlemlenebilirlik Akışı)

> Son güncelleme: **2026-07-10**
> Her oturum için `session.jsonl`'in **yanına** yapılandırılmış, append-only bir
> debug akışı (`debug.jsonl`) yazılır. Amaç: debug, token/gecikme optimizasyonu
> ve ajanın **kendi kendini geliştirebilmesi** için okuyabileceği veri.

## Neden?

`session.jsonl` kullanıcıya görünen sohbettir; içinde tur süreleri, çağrı-başına
token kırılımı, araç gecikmeleri, hook kararları, hata/sıkıştırma/kurtarma
olayları **yapılandırılmış** biçimde yoktu. Bu sinyaller parçalıydı:

| Sinyal | Eskiden | Şimdi |
|--------|---------|-------|
| Loglar | süreç-geneli RAM ring buffer (`logbuf`) | + oturum-başına kalıcı `debug.jsonl` |
| Token | yalnız ömür-boyu rollup (`SessionUsage`) | + çağrı-başına `llm_call` olayı (model+token) |
| Araç | yalnız UI iz adımı | + `tool` olayı (gecikme, boyut, hata) |
| Hook | yalnız TurnStep | + `hook` olayı (karar + süre) |
| Compaction/recovery | yalnız log | + `compaction`/`recovery` olayı |

Mimari buna zaten çok uygundu: her şey dosya-tabanlı append-only. Debug akışı da
**inflight sidecar deseniyle** ayrı dosyaya O(1) eklenir, store kilidi gerektirmez.

## Disk yapısı

```
store/sessions/{id}/
├── session.jsonl      # mevcut: header + mesajlar (kullanıcıya görünen)
├── inflight.json      # mevcut: tur-içi crash sidecar
└── debug.jsonl        # YENİ: yapılandırılmış debug olayları (append-only, O(1))
```

`DeleteSession` zaten oturum dizinini `os.RemoveAll` ile sildiğinden `debug.jsonl`
de oturumla birlikte temizlenir (ek kaskad gerekmez).

## Olay şeması (`db.DebugEvent`)

Her satır bir JSON; alanlar seyrek (`omitempty`), her olay yalnız tipiyle ilgili
alanı taşır:

```jsonc
{"ts":1719..., "type":"turn",       "durMs":4210, "stop":"end_turn"}
{"ts":1719..., "type":"llm_call",   "model":"claude", "in":1200, "out":340, "think":180, "cacheRead":8000}
{"ts":1719..., "type":"tool",       "name":"Bash", "durMs":120, "outBytes":4200}
{"ts":1719..., "type":"hook",       "name":"PreToolUse", "detail":"Bash:allow", "durMs":12}
{"ts":1719..., "type":"error",      "detail":"provider_error: 429 rate limit", "err":true}
{"ts":1719..., "type":"compaction", "detail":"context_overflow"}
{"ts":1719..., "type":"recovery",   "detail":"max_output_tokens"}
{"ts":1719..., "type":"cache_break", "name":"ttl-or-server-eviction", "detail":"Önek değişmedi ama cache okunmadı → 1s TTL doldu…", "cacheWrite":9000}
```

Tip sabitleri (`internal/db/debug_journal.go`): `turn`, `llm_call`, `tool`,
`hook`, `error`, `compaction`, `recovery`, `cache_break`, ve self-healing/epoch
katmanlarıyla gelenler: `repair` (mesaj-dizisi onarımı), `guardrail` (tool-loop
guardrail kararı), `lesson` (hata→ders damıtıldı), `epoch` (prompt-epoch yaşam
döngüsü: created/adopted/stale/refreshed). Detay: `56-SELF-HEALING.md`,
`57-PROMPT-EPOCH.md`.

Ek alanlar (2026-07-10): `HookID` — `type=hook` olayında ateşleyen hook'un id'si
(rtk/sqz gibi token-optimizer hook aktivitesinin hook-başına atfı için);
`Calls` — `llm_call`'ın temsil ettiği alt-tur sayısı (claude-cli kümülatif
faturalamayı per-call bağlama bölmek için).

Ek alan (2026-07-23): `Think` — `llm_call`'ın `out` token'ının **gizli akıl
yürütmeye (extended thinking)** giden tahmini kısmı. API ayrı bir thinking alanı
vermiyor; `out − görünür_token` (görünür = text+tool_use, kalibre tahminci)
olarak **agent katmanında** türetilir (`budget.go deriveThinkingTokens`); yalnız
native provider + `Calls<=1` (claude-cli `out`'u kümülatif → 0). **`out`'un
içinde zaten sayılı** — atıf amaçlı, faturaya eklenmez. `GetDebugSummary` /
`GetTurnDebug` bunu toplayıp `thinkingTokens` + `thinkingShare` (çıktının oranı)
olarak sunar; Debug kartı ve mesaj panelinde "Düşünme %N · tok" olarak görünür.
Ölçüm: thinking açık turlarda çıktı token'ının ~%40'ı (Sonnet 5 ~%59) gizli
akıl yürütme.

## Emit noktaları (tek huni: `Runtime.emitDebug`)

`internal/agent/debugjournal.go` tek yardımcı (`emitDebug`) tüm noktaları besler;
oturum id + çağrı kökenini (`callKind`) ctx'ten çözer, `DebugJournalEnabled`
kapalıysa veya oturum yoksa no-op'tur (best-effort, hata yutulur).

| Olay | Kaynak |
|------|--------|
| `turn` | `toolloop.go` — `completeTraced` wrapper'ı turu zamanlar (native+cli+plain hepsi buradan geçer) |
| `llm_call` | `budget.go RecordUsage` — her provider çağrısı (chat/reflect/summary/title dahil) |
| `tool` | `toolloop.go` native döngü — `reg.Call`/`CallStream` çevresi (sıkıştırma öncesi boyut) |
| `hook` | `hooks.go` — her eşleşen Pre/PostToolUse hook'u kararıyla |
| `error` | `toolloop.go fail()` + permission/budget hataları |
| `compaction` | `toolloop.go` — reaktif compact başarılı olduğunda |
| `recovery` | `toolloop.go` — çıktı-cap resume kurtarması |
| `cache_break` | `cachebreak.go noteCacheOutcome` — sıcak prompt-cache öneki kaybolup soğuk yeniden yazıldığında (yalnız ana konuşma turları: chat/task/schedule/flow/spawn); sebep atıflı (`model-changed`/`prompt-or-tools-changed`/`ttl-or-server-eviction`). Claude Code `promptCacheBreakDetection` muadili — veri zaten `Usage.Cache*`'te, bu yalnız atıf ekler. **Soğuma israfı:** yalnız `ttl-or-server-eviction` (geç gelen tur öneki soğuttu — model/prompt değişimi meşru geçersizleşmedir, israf değil) durumunda olay `wasteUsd`/`wasteEst` taşır = yeniden yazılan öneğin (native Anthropic `cache_creation`) yazma-tier'ı eksi zamanında okunsa ödenecek okuma-tier'ı (`providers.CoolingWaste`; abonelik sağlayıcıda tahmini). OpenRouter soğuk öneği input'a katıp write saymadığı için orada ~0 |

## Dosya yönetimi (cap + budama)

- Cap aşımı **tembel budanır**: `cap + cap/4`'ü geçince dosya en yeni `cap`
  olaya yeniden yazılır (atomik `tmp→rename`). Rewrite maliyeti çok sayıda append'e
  yayılır.
- Satır sayısı bellekte (`DB.debugCount`, kendi mutex'i) tutulur; bir oturumun
  ilk append'inde dosya satırları bir kez sayılır → cap restart'lar arası da
  uygulanır.
- Bozuk/yarım son satır okuma sırasında sessizce atlanır (`session.jsonl` toleransı).

## Okuma yolları

### 1) Ajan aracı — `read_session_debug` (self-improvement)

`internal/tools/builtin_debug.go`. **Her zaman açık** core araç (debug günlüğü
etkinse; `conversation_search`'ün gözlemlenebilirlik kardeşi) — hidden-lazy
self-manage tier'ında **değil**, böylece kutudan çıkar çıkmaz çalışır. **claude-cli
köprüsü:** `Runtime.BridgeTools` aracı `extra` listesine ekler (yalnız `r.db`
gerektiren eager builtin, `conversation_search` gibi) → varsayılan keyless
claude-cli ajanı da `mcp__tionswarm_interaction__read_session_debug` olarak görür ve
çağırır. Köprü çağrı closure'ı build ctx'inden yakalanan oturum id'yi enjekte eder
(interaction server request ctx'inde olmadığından). Varsayılan **özet** döndürür;
`summary=false` ile ham olay listesi (`type` filtresi + `limit`). `session_id`
verilmezse mevcut oturumu okur (ctx'teki `tools.CurrentSessionID`).

Özet (`db.DebugSummary`): turlar, llm çağrıları, token toplamları, `byTool`
(çağrı/hata/süre/bayt), `byModel` (token), `topTools` (en yavaş araçlar), hata
sayısı + `lastError`, compaction/recovery sayıları.

### 2) API — `GET /api/sessions/{id}/debug`

`internal/api/session_debug.go`.
- `?summary=1` (varsayılan) → `{ summary: DebugSummary }`
- `?summary=0&type=tool&limit=200` → `{ events: []DebugEvent }`

### 3) UI — Debug modalı (chat header "Debug" butonu)

`frontend/src/features/sessions/SessionDebugCard.tsx` (ayrı, self-contained
bileşen). **2026-07-08'den beri "Oturum bilgisi" panelinde DEĞİL** — chat
header'daki "Debug" butonuyla açılan ayrı `SessionDebugModal` içinde
`SessionDebugCard alwaysOpen` olarak render edilir. Metrik
ızgarası + sağlık rozetleri (hata/compaction/recovery/süre) + en yavaş araçlar +
modele göre token + tembel yüklenen **ham olay** log'u (tip filtreli).

**Cache metrikleri (2026-07-05):** metrik ızgarası artık **Cache oku** + **Cache yaz**
+ **Cache isabet** (`cacheRead / (input + cacheRead + cacheWrite)`) gösterir — mesaj-başına
paneldeki (`MessageDebugPanel`) sıcak/soğuk göstergesiyle aynı formül, oturum geneline
toplanmış. Veri hattı zaten mevcuttu (`budget.go RecordUsage` → `llm_call` olayı `CacheRead`/
`CacheWrite` ile, tüm yollar + **claude-cli** dahil; `GetDebugSummary` toplar); bu değişiklik
yalnız session kartında write + isabet oranını görünür kıldı (önceden sadece "Cache tok"=read).

**Soğuma israfı hücresi (2026-08-03):** sağlık satırındaki "Cache kırılması" pill'inin yanında,
`ttl-or-server-eviction` kırılmalarının izole USD maliyeti — `Soğuma israfı: ~$0.0123 · 3×`
(abonelik sağlayıcıda `~` tahmini işareti + kırılma sayısı). Kaynak `DebugSummary.CoolingWasteUSD`/
`CoolingBreaks`/`CoolingWasteEstimated` (yalnız `wasteUsd` taşıyan cache_break'lerden toplanır).
Ham olay satırında da `· israf ~$…` görünür. Bu, "geç yanıt sıcak öneği soğuttu → yeniden yazım
parası" farkını toplam maliyetten ayırıp tek hücrede gösterir (fiili para zaten `CacheWrite` olarak
faturaya işleniyordu; bu kalem yalnız **önlenebilir** kısmı izole eder). Test: `providers.TestCoolingWaste`
+ `db.TestDebugSummaryCoolingWaste`.

## Ayarlar

`settings.json` (default skill `tionswarm-settings`'te de belgeli):

| Alan | Vars. | Açıklama |
|------|-------|----------|
| `debugJournalEnabled` | `true` | Debug akışını yaz (kapalıyken hiç olay yazılmaz) |
| `debugJournalCap` | `5000` | Oturum başına saklanan en yeni olay (0 = varsayılan) |

Tunables: `Tunables.SetDebugJournal/DebugJournalEnabled/DebugJournalCap`;
`api/server.go applySettings` canlı uygular. UI: Ayarlar ▸ Uygulama ▸ (kalıcı
ilerleme bölümünün altında) "Debug günlüğü".

## Default skill — `tionswarm-self-debug`

`internal/skills/defaults/tionswarm-self-debug/SKILL.md`. Ajana `read_session_debug`
ile kendi metriklerini okuyup (token/gecikme/araç/hata) davranışını nasıl
optimize edeceğini öğretir. `read_logs` (kaba, süreç-geneli) ile per-session
budget (para) arasındaki yeri netleştirir.

## Test

- `internal/db/debug_journal_test.go` — append→read round-trip, tip filtresi,
  özet agregasyonu (turlar/token/byTool/byModel/hata/compaction/topTools) ve cap
  budama (en yeni `cap` olay korunur, en eskiler düşer).

## Faz 3 — Anomali tespiti + zaman serisi + reflektör self-improvement ✅ (2026-06-26)

`GetDebugSummary` artık türetilmiş üç alan döndürür (hepsi `read_session_debug`
özetinde + API'de + UI'da):

- **`anomalies` (`[]DebugAnomaly`):** saf, test edilebilir heuristikler
  (`computeDebugAnomalies`, no-I/O) — `tool_time_dominant` (tek araç araç-süresinin
  ≥%60'ını yiyor, toplam >2s), `tool_failing` (çağrı≥3, hata oranı >%30),
  `tool_large_output` (ort. çıktı >64KB), `frequent_compaction` (≥3),
  `cache_breaks`/`cache_break` (kırılım olayları), `error_burst` (≥3),
  `slow_turns` (ort. tur >45s), **`low_cache_hit`** (cache koçu: ≥3 çağrı +
  prompt>20K token + sıcak-isabet <%50 → sıcak önek tutmuyor), **`high_thinking`**
  (thinking koçu: çıktı ≥2K token + `thinkingShare` ≥%50 → ThinkingLevel'i düşür).
  Her biri `severity` (warn/info) + `code` + Türkçe `message`. Sağlıklı oturumda boş.
- **Zaman serisi:** `turnDurSeries` (tur süreleri) + `tokenSeries` (çağrı-başına
  in+out), en yeni `debugSeriesCap=40` nokta. UI'da bağımlılıksız SVG sparkline.
- **Reflektör entegrasyonu (self-improvement):** ~~`reflect()` dream-cycle'da
  `r.debugPerfNotes(agentID)` ajanın en yeni ≤5 oturumunun anomalilerini
  toplar ve reflect prompt'una ekler → kalıcı reflection belleğine yedirir.~~
  **KALDIRILDI (2026-07-05):** memory alt sistemiyle birlikte dream-cycle
  reflektörü de çıkarıldı. Hata-odaklı self-improvement artık **hata→ders
  döngüsünden** geçer (`internal/agent/lessons.go`, `lesson` debug olayı;
  bkz. `56-SELF-HEALING.md` Faz F). UI'da kartta anomaliler
  (warn=kırmızı/info=gri rozet) + iki sparkline (tur süresi / çağrı token) gösterilir.
- Test: `db/debug_journal_test.go` (`TestDebugSummaryAnomaliesAndSeries`,
  `TestDebugSummaryNoAnomaliesOnHealthy`, `TestDebugSummaryCacheAndThinkingCoach`).

## Mesaj başına debug paneli (2026-06-29)

Debug olayları artık **tur (mesaj) kimliği** ile korele: `DebugEvent.TurnID`,
chat giriş noktasında `agent.WithTurnID(ctx, replyID)` ile damgalanır (replyID =
asistan yanıt mesajının id'si). `emitDebug` her olaya bunu basar, böylece bir
yanıtın tüm olayları (llm_call/tool/error/recovery/compaction) o mesaja bağlanır.

- **Rollup:** `db.GetTurnDebug(session, turnId)` → o yanıtın token/süre/model +
  per-tool listesi (ad, gecikme, çıktı boyutu, hata). Maliyet API katmanında
  `modelRowsFor` ile (Bütçe ekranıyla aynı fiyatlandırma).
- **API:** `GET /api/sessions/{id}/turn-debug?turn={replyMessageId}`.
- **claude-cli araçları:** CLI kendi tool-loop'unu sürdüğü için per-tool olaylar
  normalde yayılmaz; `Runtime.emitCLIToolDebug` stream-json trace'inden her araç
  için bir `DebugTool` olayı yayar (boyut + hata; gecikme yalnız native yolda).
- **UI:** her asistan mesajının sol üstünde küçük buton (`MessageDebugPanel`) →
  açılır panelde o mesajın token/maliyet/süre + araç kırılımı (lazy fetch).
- **cliOverhead iyileştirme:** context-preview projeksiyonu artık lifetime
  ortalaması yerine debug'daki **son llm_call**'ın gerçek girdisini kullanır.

## İş akışı görselleştirmeleri (2026-07-08 → 2026-07-10)

Debug modalında `debug.jsonl`'den **salt-frontend** türetilen görselleştirme
bölümü (`frontend/src/features/sessions/viz/`, kabuk `SessionFlowViz.tsx` +
veri hazırlığı `flowVizData.ts`):

- **Araç yürütme Sankey'i (`ToolSankey.tsx`):** `Ajan → Araç → Tamam|Hata`
  akışı, mermaid `sankey-beta` ile.
- **Eşzamanlılık zaman çizelgesi (`ConcurrencyTimeline.tsx`):** ajan-şeritli
  bağımsız SVG Gantt; şerit çakışması = eşzamanlılık.
- **Prompt-cache olayları (`PromptCacheEvents.tsx`, 2026-07-08):** `epoch`
  (önleme) + `cache_break` (tespit) olayları rozetli listede; adopt'suz
  kırılım = araştırılacak sinyal. Detay `57-PROMPT-EPOCH.md`.
- **Düşünme payı (`ThinkingShareChart.tsx`, 2026-07-24):** çağrı-başına `think`/
  `out` oranı bağımsız SVG bar-serisi + ortalama pay; thinking-off oturum sessiz.
  Gizli akıl yürütme maliyetini tur-tur görünür kılar (_Docs/17 türetim).
- **Self-healing olayları (`SelfHealingEvents.tsx`):** `repair`/`guardrail`/
  `lesson` olayları. Detay `56-SELF-HEALING.md`.
- **Hook / token-optimizer aktivitesi (`HookActivity.tsx`, 2026-07-10):**
  `type=hook` olayları `DebugEvent.HookID` ile hook-başına atfedilir;
  `\brtk\b`/`\bsqz\b` (backend probe aynası) ile rtk/sqz/other sınıflanır;
  hook başına ateşleme sayısı + araç dağılımı + hata sayısı. **Byte tasarrufu
  DEĞİL, aktivite göstergesi** — rtk/sqz PreToolUse komut-rewrite olduğundan
  TionSwarm sıkışmamış baseline'ı hiç görmez; ayrıca yalnız **native** turlar
  sayılır (claude-cli turlarında hook'lar CLI içinde çalışır, journal'a düşmez).

## Anomali → bildirim ✅ (2026-07-24)

Tur-sonu tek huni `AutoTagTurn` (her tamamlanma yolu: chat/spawn/schedule/wake/
auto-continue) artık **auto-tag'den bağımsız** olarak `notifyNewAnomalies` çağırır:
`GetDebugSummary`'nin **warn** anomalilerini (`low_cache_hit`, `cache_breaks`,
`tool_failing`, `frequent_compaction`, `error_burst`, `tool_time_dominant`)
events bus'a `type:"anomaly"` olayı olarak yayar → SSE → masaüstü bildirimi,
tıklama session'a deep-link. **Info** bulgular (high_thinking/slow_turns/…) yalnız
kartta kalır. Yayım koşulsuz (task/flow gibi); toast'ı frontend geçitler:
device-local `anomaly` notify-tipi (`NOTIFY_TYPES`, Ayarlar ▸ Bildirimler) +
ana masaüstü-bildirim toggle'ı. Dedup **per (session, code)** in-memory
(`Runtime.anomalyNotified`) → kalıcı anomali süreç başına bir kez bildirir, her
tur değil. Test: `agent/anomaly_notify_test.go`.

## Sırada (Faz 4+ fikirler)

- Anomali eşiklerinin ayarlanabilir olması (settings).
- Workspace-geneli "en pahalı oturumlar" / araç ısı haritası panosu.
- Gerçek byte-tasarrufu ölçümü istenirse: sqz'yi PostToolUse output-rewrite
  moduna alıp `runPostToolHooks`'ta `len(önce)−len(sonra)` ölçmek (ayrı iş).
