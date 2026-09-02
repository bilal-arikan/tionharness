# TionHarness — Oturum Debug Günlüğü (Paralel Gözlemlenebilirlik Akışı)

> Son güncelleme: **2026-09-01**
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
{"ts":1719..., "type":"tool",       "name":"Bash", "durMs":120, "outBytes":4200, "error":"exit status 1", "args":"{\"command\":\"go test ./...\"}"}
{"ts":1719..., "type":"hook",       "name":"PreToolUse", "detail":"Bash:allow", "durMs":12}
{"ts":1719..., "type":"error",      "detail":"provider_error: 429 rate limit", "err":true}
{"ts":1719..., "type":"compaction", "detail":"context_overflow"}
{"ts":1719..., "type":"recovery",   "detail":"max_output_tokens"}
{"ts":1719..., "type":"cache_break", "name":"ttl-or-server-eviction", "detail":"Önek değişmedi ama cache okunmadı → 1s TTL doldu…", "cacheWrite":9000}
{"ts":1719..., "type":"pressure",   "name":"context_pressure", "detail":"context 176000/200000 tokens · 88% of budget · fold at 100%"}
```

Tip sabitleri (`internal/db/debug_journal.go`) — **16 tanesi vardır ve hepsi
okunabilir**: `turn`, `llm_call`, `tool`, `hook`, `error`, `compaction`,
`recovery`, `cache_break`, `pressure`, `build` (journal'ı yaratan sürecin
commit'i, taze `debug.jsonl`'in ilk satırı), `cli_compaction` (Claude CLI native
compaction yaşam döngüsü), `lifecycle` (oturum yaşam döngüsü: teardown sırasında
iptal edilen tur, kuyrukta hiç çalışmadan düşen tur, teardown grace süresinin
aşılması), ve self-healing/epoch katmanlarıyla gelenler:
`repair` (mesaj-dizisi onarımı), `guardrail` (tool-loop guardrail kararı),
`lesson` (hata→ders damıtıldı), `epoch` (prompt-epoch yaşam döngüsü:
created/adopted/stale/refreshed). Detay: `56-SELF-HEALING.md`,
`57-PROMPT-EPOCH.md`.

Ayrıca dahili bir işaretçi vardır: `_cli_compaction_dedupe`. Bu bir olay **tipi
değildir** — sayımdan, okumadan ve budamadan bilinçli olarak dışlanır
(`readDebugFile`, `countDebugEvents`, `truncateDebugTail`), UI'da ve araçta hiç
görünmez.

**Tip → okuyucu tablosu (2026-09-01 hizalaması).** Her tip diskten okunur, özette
sayılır ve ham olay log'unda filtrelenebilir:

| Tip | `GetDebugSummary` alanı | `GetTurnDebug` alanı | Araç `type` enum | UI filtre çipi |
|-----|-------------------------|----------------------|------------------|----------------|
| `turn` | `turns`, `turnDurMs` | `durMs`, `stop` | ✅ | ✅ |
| `llm_call` | `llmCalls`, token alanları | aynı | ✅ | ✅ |
| `tool` | `toolCalls`, `byTool` | `tools[]` | ✅ | ✅ |
| `hook` | `hooks` | `hooks` | ✅ | ✅ |
| `error` | `errors`, `lastError` | `errors`, `lastError` | ✅ | ✅ |
| `compaction` | `compactions`, `savedBytes` | `compactions` | ✅ | ✅ |
| `recovery` | `recoveries` | `recoveries` | ✅ | ✅ |
| `cache_break` | `cacheBreaks`, `coolingWasteUsd`… | `cacheBreaks`, `cacheBreakReason`… | ✅ | ✅ |
| `repair` | `repairs` | `repairs` | ✅ | ✅ |
| `guardrail` | `guardrails` | `guardrails` | ✅ | ✅ |
| `lesson` | `lessons` | `lessons` | ✅ | ✅ |
| `epoch` | `epochs` | `epochs` | ✅ | ✅ |
| `pressure` | `pressureEvents` | `pressureEvents` | ✅ | ✅ |
| `build` | `buildCommit` | — (turId taşımaz) | ✅ | ✅ |
| `cli_compaction` | `cliCompactions` | `cliCompactions` | ✅ | ✅ |
| `lifecycle` | `lifecycleEvents` | `lifecycleEvents` | ✅ | ✅ |

Sayaç alanları `omitempty` taşır: sıfırsa JSON'a hiç yazılmaz.

`summarizeDebugDetail` de her tip için ayrı bir redaksiyon etiketi döndürür
(ör. `guardrail detail [redacted]`, `cli compaction detail [redacted]`); serbest
metin hiçbir tipte olduğu gibi saklanmaz.

Ek alanlar (2026-07-10): `HookID` — `type=hook` olayında ateşleyen hook'un id'si
(rtk/sqz gibi token-optimizer hook aktivitesinin hook-başına atfı için);
`Calls` — `llm_call`'ın temsil ettiği alt-tur sayısı (claude-cli kümülatif
faturalamayı per-call bağlama bölmek için).

Ek alanlar (2026-08-20): `Error` ve `Args` — yalnız başarısız `tool`
olaylarında hata metninin ilk 500 karakterlik ve araç argümanlarının ilk 200
karakterlik tek-satır özeti. Native araç döngüsü, CLI trace'i ve code-mode köprüsü
aynı alanları yazar; başarılı çağrılarda alanlar boş bırakılır. Böylece hata
ayıklama için tüm araç çıktısını saklamadan hangi çağrının neden kırıldığı görülür.

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

Ek alanlar (2026-08-28): `FoldIndex` ve `SummaryBytes` — yalnız `compaction`
olaylarında anlamlıdır ve **`omitempty` taşımaz**, yani sıfır değer de JSON'a
yazılır (sıfır burada gerçek bir sinyaldir, eksik veri değil).

| Alan | Anlam |
|------|-------|
| `foldIndex` | Bu oturumdaki fold'un **1-tabanlı sırası** (fold sonrası `Session.CompactionCount`). Anlamsal aşınma birikimlidir — 5. fold'un özeti 1. fold'un kalitesinde değildir — sıra bilgisi token rakamlarının yanında bu eğriyi okunur kılar. Upgrade öncesi başlamış oturumlarda sayaç geriye dönük doldurulmadığı için `0`'dan başlayabilir; `0` = "ordinal tespit edilemedi". |
| `summaryBytes` | Fold **sonrasındaki** rolling-summary'nin bayt uzunluğu. `foldIndex` ile birlikte özetin büyüdüğünü, sabit kaldığını mı yoksa eridiğini mi gösterir: fold başına küçülen `summaryBytes` = **"özet eriyor"** sinyali (bağlam kaybı). `0` = boş özet yazıldı — kendi başına bir anomali. |

## Emit noktaları (tek huni: `Runtime.emitDebug`)

`internal/agent/debugjournal.go` tek yardımcı (`emitDebug`) tüm noktaları besler;
oturum id + çağrı kökenini (`callKind`) ctx'ten çözer, `DebugJournalEnabled`
kapalıysa veya oturum yoksa no-op'tur (best-effort, hata yutulur).

Runtime'a erişemeyen katmanlar (`internal/conversation` — import döngüsü olurdu —
ve `internal/api`'nin kuyruk/özet dayanıklılık yolları) doğrudan store'a yazar.
Bunlar `DB.AppendDebugEventGated` kullanır: aynı ayarı store üzerinde tutan ikinci
kapı (`internal/db/debug_journal_policy.go`, `DB.SetDebugJournal`). Ayar hem
tunables'a hem her canlı store'a `Server.applySettings` içinde tek noktadan
basılır, böylece iki huni de aynı `debugJournalEnabled` bayrağına ve aynı
`debugJournalCap` değerine uyar. Runtime'da **oluşturulan veya attach edilen**
workspace'ler store'larını `applySettings`'ten sonra açtığı için aynı politikayı
açılış anında devralır (`api/workspaces.go` → `Server.applyDebugJournalToStore`);
aksi hâlde bir sonraki ayar güncellemesine kadar sıfır-değer politikayla
(journal açık, varsayılan cap) kalırlardı. Ham `AppendDebugEvent` (cap argümanı alan, her
zaman yazan biçim) yalnız bu iki kapının içinden çağrılmalıdır. Yazma hatası
sessizce yutulmaz; ilgili katmanın logger'ıyla `Error` seviyesinde loglanır.

| Olay | Kaynak |
|------|--------|
| `turn` | `toolloop.go` — `completeTraced` wrapper'ı turu zamanlar (native+cli+plain hepsi buradan geçer) |
| `llm_call` | `budget.go RecordUsage` — her provider çağrısı (chat/reflect/summary/title dahil) |
| `tool` | `toolloop.go` native döngü — `reg.Call`/`CallStream` çevresi (sıkıştırma öncesi boyut) |
| `hook` | `hooks.go` — her eşleşen Pre/PostToolUse hook'u kararıyla |
| `error` | `toolloop.go fail()` + permission/budget hataları **ve** başarısız slash komutları (`api/summary.go recordSummaryFailure` — `name="/compact"`, `kind="command"`, `err=true`, `detail`/`error` sağlayıcı hatasını birebir taşır) |
| `compaction` | İki yol: (a) `toolloop.go` — reaktif (bağlam-taşması kurtarması) compact; (b) `conversation/manager.go` — rutin bütçe-tabanlı rolling-summary fold'u (`Prepare`, `Name="auto"`) **ve** manuel `/compact` (`ForceCompact`, `Name="manual"`). (b) `SavedBytes` + `Detail`("folded N msgs · before→after tokens") taşır; her tur-türünde (chat/spawned/wake/koordinatör) tek noktadan yazılır — böylece spawned turda katlanan bir fold da görünür olur. **`before`/`after` aynı formülden gelir:** mesajlar (özet + gönderilen turlar) + o turun mesaj-dışı overhead'i. `after` tarafında overhead **fold'a göre düzeltilir** — sıcak CLI thread'inde overhead'e giren kalıcı `Steps` izinin katlanan mesajlara ait kısmı düşülür (`WithContextOverheadStepBase`), yoksa katlanmış turlar hem özette hem izde iki kez sayılır ve `after` bütçenin üstünde görünür |
| `pressure` | `conversation/manager.go recordPressureDebug` — tur **fold'a girmedi** ama bağlam kullanımı etkin bütçenin `pressureWarnRatio`=**%85**'ini geçti; fold'un kendisi değil, fold'dan önceki erken uyarıdır (`Name="context_pressure"`, `Detail`: "context kullanılan/bütçe tokens · %N of budget · fold at 100%") |
| `recovery` | `toolloop.go` — çıktı-cap resume kurtarması |
| `cache_break` | `cachebreak.go noteCacheOutcome` — sıcak prompt-cache öneki kaybolup soğuk yeniden yazıldığında (yalnız ana konuşma turları: chat/task/schedule/flow/spawn); sebep atıflı (`model-changed`/`prompt-or-tools-changed`/`ttl-or-server-eviction`). Claude Code `promptCacheBreakDetection` muadili — veri zaten `Usage.Cache*`'te, bu yalnız atıf ekler. **Soğuma israfı:** yalnız `ttl-or-server-eviction` (geç gelen tur öneki soğuttu — model/prompt değişimi meşru geçersizleşmedir, israf değil) durumunda olay `wasteUsd`/`wasteEst` taşır = yeniden yazılan öneğin (native Anthropic `cache_creation`) yazma-tier'ı eksi zamanında okunsa ödenecek okuma-tier'ı (`providers.CoolingWaste`; abonelik sağlayıcıda tahmini). OpenRouter soğuk öneği input'a katıp write saymadığı için orada ~0 |

### Adlandırılmış `error` olayları — fail-closed konfigürasyon (2026-09-01)

`type=error` olaylarının bir kısmı `Name` alanı taşır: bunlar tur akışının
kendisinden değil, **bozuk bir konfigürasyon belgesinin fail-closed
karşılanmasından** doğar. Hepsi `Err=true`'dur ve `Error` alanında altta yatan
çözümleme hatasını birebir taşır.

| `name` | Kaynak | Koşul | Görüldüğünde ne anlama gelir |
|--------|--------|-------|------------------------------|
| `mcp_server_gate_malformed` | `climcp.go` (`writeCLIMCPConfig`), `codexmcp.go` — `mcpServerGate` | Ajanın `AllowedTools`/`ToolOverrides`/`BlockedTools` belgesi çözülemedi | **Hiçbir MCP sunucusu mount edilmedi** ve CLI turu başlamadan durdu. Ajanın araç kısıtı hesaplanamadığı için kapı deny-all'a düştü; MCP araçlarının "kaybolması" ajan konfigürasyonunun bozuk olmasındandır, sunucunun düşmesinden değil. Detay: `52-MCP-GATEWAY.md` |
| `tool_permission_config_malformed` | `toolsetup.go` — `toolFilter` | Ajanın araç izin belgesi çözülemedi | Filtre **her aracı reddediyor**. Ajan araçsız kalır; düzeltme yeri ajanın izin konfigürasyonudur. Detay: `19-LAZY-TOOL-LOADING.md` |
| `inbox_corrupt` | `api/inbox_durability.go` — `quarantineInbox` | `inbox.json` okunamadı/çözülemedi | Sidecar `inbox.json.corrupt-<unix>` olarak karantinaya alındı; **kuyruktaki mesajlar dağıtılmadı ve kaybedildi**. Bekleyen bir mesajın hiç işlenmemiş görünmesinin sebebi budur; karantina dosyası incelenebilir. Detay: `58-QUEUE-SENKRON.md` |
| `orphan_recovery_failed` | `coordination.go` — `RecoverOrphanedTurns` | Açılışta `ListSessions` hata verdi | Öksüz (yarım kalmış) turların kurtarma taraması **hiç çalışmadı**. Çöküş sonrası yeniden dispatch edilmesi beklenen turlar `running` takılı kalmış olabilir |
| `sidecar_corrupt` | `db/sidecar.go` — `Sidecar[T].Load` | Oturuma ait tipli bir sidecar (ör. `trajectory.json`) çözülemedi | Dosya `<ad>.corrupt-<unix>` olarak karantinaya alındı, sonraki okuma "yok" döner; çağıran işlem ilk seferde `SidecarCorruptError` alır. Rota için: indeks satırı düşer, kök oturum sıfırdan rota açabilir. Detay: `77-ROTA-ALTYAPI-PLANI.md` R4 |

### Adlandırılmış yaşam döngüsü / sıkışma olayları (2026-09-01)

Aşağıdaki olaylar bozuk konfigürasyondan değil, **normal akışın sessiz
dallarından** doğar: iptal edilen bir tur yalnızca yayın yapmayı bırakır, kuyruktan
düşen bir mesaj hiç çalışmaz, atlanan bir native compaction geride yalnız rolling
fold bırakır. Hiçbirinin başka bir izi yoktur; `debug.jsonl` tek kayıt yeridir.

| `name` | `type` | Kaynak | Koşul | Görüldüğünde ne anlama gelir |
|--------|--------|--------|-------|------------------------------|
| `turn_cancelled_by_teardown` | `lifecycle` | `api/session_teardown.go:162` (`recordTeardownLifecycle`) | Oturum silinirken **canlı bir chat turu** vardı ve `stopInflightTurn` onu iptal etti | Turun yarıda kesilme sebebi sağlayıcı/model değil, kullanıcının silme isteğidir. `Phase="inflight_turn"`, `DurationMs` = turun tamamen sönmesi için geçen süre |
| `autonomous_cancelled` | `lifecycle` | `api/session_teardown.go:165` (`recordTeardownLifecycle`) | Silme anında oturumda **aktif bir otonom çalışma** vardı (`autonomousRunsOf(wsp).IsSessionActive`) | Otonom döngü kendi kararıyla değil, teardown tarafından durduruldu. Aynı `Phase`/`DurationMs` alanları |
| `teardown_grace_exceeded` | `lifecycle` | `api/session_teardown.go:333` (`noteTeardownGrace` → `recordTeardownLifecycle`) | Teardown fazlarından biri grace penceresini doldurdu; silme **iptal edildi**, oturum yaşamaya devam ediyor | Hangi fazın takıldığını `Phase` söyler: `inflight_turn` (tur sönmedi), `mcp_calls` (admitted MCP çağrıları drenaj olmadı), `worker` (oturum worker'ı durmadı). `DurationMs` = grace penceresinin kendisi. Kullanıcının gördüğü HTTP hatasının makine-okunur karşılığıdır |
| `queued_turn_dropped` | `lifecycle` | `api/inbox_debug.go:24` (`recordQueuedTurnDropped`); çağrı yerleri `api/inbox.go:589`, `api/inbox.go:603` | Kuyruktaki bir mesaj **hiç çalışmadan** silindi | `Phase` hangi yolun sildiğini verir: `user_cancel` (tek mesajın iptali) veya `queue_cleared` (kuyruğun tamamen boşaltılması). Kendi kendini boşaltan bir kuyruk ile kullanıcının bilinçli budamasını ayırt etmeyi sağlar |
| `native_skipped` | `compaction` | `conversation/manager.go:386` → `nativecompact.go:104` (`recordNativeCompactDebug`) | `claimNativeAttempt` reddetti — bu rolling-summary sınırı için native denemesi zaten harcanmıştı | Native compaction denenmedi bile; bütçe aşımı bir sonraki adımda rolling fold ile karşılanır. Sebep bir hata değil, tek-deneme kuralıdır |
| `claim_consumed` | `compaction` | `conversation/manager.go:383` | Native denemesi çalıştı ve **hata verdi**; deneme hakkı bu hatayla tükendi | Sonraki turun `native_skipped`'ının görünür sebebidir. `ErrorKind` = `nativeCompactErrorKind(nativeErr)` (ör. `compaction_failed`) |
| `native_fallback_rolling` | `compaction` | `conversation/manager.go:391` | Mod `auto` (native-önce + rolling emniyet ağı) ve native **gerçekleşmedi** | Aşağıdaki rolling fold, `auto` modunun emniyet ağıdır — ayrı/beklenmedik bir davranış değil |
| `fold_idle_floor` | `pressure` | `conversation/foldtimeout.go:63` (`recordFoldIdleFloorDebug`, `summarizeTimed` üzerinden) | Fold'un duvar-saati süresi normal stdout-sessizlik penceresini (`providers.StdoutIdleWindow()`) aştı; pencere kapalıysa (`<=0`) veya zaten `FoldIdleOutputFloor`'dan büyükse olay yazılmaz | Bu fold, **yalnızca `FoldIdleOutputFloor` sayesinde** hayatta kaldı: taban olmasaydı watchdog onu öldürecekti. `DurationMs` = fold'un toplam süresi |
| `failed_turn_billed` | `llm_call` | `agent/toolloop.go:336` (`recordFailedUsage`) | Tur hata ile bitti ama sağlayıcı hatanın üzerinde kullanım taşıyor (`providers.UsageError`); `OutputTokens == 0` ve input+cache toplamı `> 0` | Çıktı üretmeyen bir tur **yine de faturalandı**. Olay `Model`/`In`/`Out`/`CacheRead`/`CacheWrite`/`Calls` alanlarını ve `Err=true` taşır |

**`fold_idle_floor` bir ÜST SINIRDIR.** Ölçüt fold'un duvar-saati süresidir, gerçek
sessizlik boşluğu değil: bir fold yayın yaparken de pencereyi aşabilir, yani
**yanlış pozitif mümkündür**. Buna karşılık taban sayesinde kurtulan her fold
mutlaka ölçütü tetikler — **yanlış negatif yoktur**. Watchdog'un kendisi
`internal/providers` içinde yaşar ve store'a erişmez, bu yüzden ölçüm bir üst
katmandan yapılır. `BuildHandoff` yolundaki fold'lar **kapsam dışıdır**: o imza
session id taşımaz, dolayısıyla yazılacak journal de yoktur.

**`queued_turn_dropped` yalnız iki yolu kaydeder.** Kullanıcı iptali
(`Phase="user_cancel"`) ve kuyruk temizleme (`Phase="queue_cleared"`). Poison-guard
düşüşü buraya girmez; o `recordQueueTurnFailure` üzerinden `error` tipinde ayrıca
journal'lanır (ve kalıcı bir hata kartı yazar). Wake / scheduled / automation /
peer / coordinator-drain turları `sessionInbox`'a **hiç girmez** — o kuyruk yalnız
chat turlarını taşır — dolayısıyla "kuyrukta düşen otonom tur" diye bir durum yoktur.

**`failed_turn_billed` aggregate'lere girmez.** Aynı token'lar, o denemenin
`RecordUsage` tarafından yazılan olağan `llm_call` olayında zaten sayılıdır; bu
olay okunabilirlik için yazılır ve `GetDebugSummary`/`GetTurnDebug` içindeki çağrı
ve token toplamlarında **atlanır** (`debugNameFailedTurnBilled`, çift sayımı
önlemek için). Ham olay log'unda ise normal şekilde görünür.

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
claude-cli ajanı da `mcp__tionharness_interaction__read_session_debug` olarak görür ve
çağırır. Köprü çağrı closure'ı build ctx'inden yakalanan oturum id'yi enjekte eder
(interaction server request ctx'inde olmadığından). Varsayılan **özet** döndürür;
`summary=false` ile ham olay listesi (`type` filtresi + `limit`). `session_id`
verilmezse mevcut oturumu okur (ctx'teki `tools.CurrentSessionID`).

Özet (`db.DebugSummary`): turlar, llm çağrıları, token toplamları, `byTool`
(çağrı/hata/süre/bayt), `byModel` (token), `topTools` (en yavaş araçlar), hata
sayısı + `lastError`, compaction/recovery sayıları ve yukarıdaki tabloda listelenen
yaşam döngüsü sayaçları (`hooks`/`repairs`/`guardrails`/`lessons`/`epochs`/
`pressureEvents`/`cliCompactions` + `buildCommit`).

### 2) API — `GET /api/sessions/{id}/debug`

`internal/api/session_debug.go`.
- `?summary=1` (varsayılan) → `{ summary: DebugSummary }`
- `?summary=0&type=tool&limit=200` → `{ events: []DebugEvent }`

### 3) UI — Debug modalı ("Oturum bilgisi" panelindeki "Debug" butonu)

`frontend/src/features/sessions/SessionDebugCard.tsx` (ayrı, self-contained
bileşen). İçerik hâlâ panele gömülü değil: **2026-08-25'ten beri** "Oturum
bilgisi" panelinin en altındaki **Araçlar** blokunda yer alan
"Debug / gözlemlenebilirlik" butonuyla (önceden chat header'daki "Debug"
butonu) açılan ayrı `SessionDebugModal` içinde
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

`settings.json` (default skill `tionharness-settings`'te de belgeli):

| Alan | Vars. | Açıklama |
|------|-------|----------|
| `debugJournalEnabled` | `true` | Debug akışını yaz (kapalıyken hiç olay yazılmaz) |
| `debugJournalCap` | `5000` | Oturum başına saklanan en yeni olay (0 = varsayılan) |

Tunables: `Tunables.SetDebugJournal/DebugJournalEnabled/DebugJournalCap`;
`api/server.go applySettings` canlı uygular. UI: Ayarlar ▸ Uygulama ▸ (kalıcı
ilerleme bölümünün altında) "Debug günlüğü".

## Default skill — `tionharness-self-debug`

`internal/skills/defaults/tionharness-self-debug/SKILL.md`. Ajana `read_session_debug`
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
- **Cache kırılımı (2026-08-11):** rollup artık `cache_break` olaylarını da topluyor →
  `CacheBreaks` + `CacheBreakReason`/`CacheBreakDetail` (atıflı sebep) +
  `CoolingWasteUSD`/`CoolingWasteEstimated` (yalnız TTL/eviction dalında dolu).
  Sıcak/soğuk ayrımı token sayılarından zaten türetilebiliyordu; panelde eksik olan
  **sebep** buradan gelir. Bkz. `_Docs\50` P7.
- **claude-cli araçları:** CLI kendi tool-loop'unu sürdüğü için per-tool olaylar
  normalde yayılmaz; `Runtime.emitCLIToolDebug` stream-json trace'inden her araç
  için bir `DebugTool` olayı yayar (boyut + hata; gecikme yalnız native yolda).
- **Codex collab araçları:** `Name` gerçek collab operation (eksikse
  `collab_tool_call`), `Detail` yalnız operation + alıcılar + durumdan oluşan
  güvenli özet, `DurMs` CLI item yaşam döngüsünde ölçülen süredir. Ham collab
  prompt'u debug günlüğüne yazılmaz.
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
  TionHarness sıkışmamış baseline'ı hiç görmez; ayrıca yalnız **native** turlar
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

## Compaction raporu vs bağlam ölçeri neden farklı sayı gösterir (2026-08-31)

Debug günlüğündeki `compaction` olayı (`X→Y token`) ile "Oturum bilgisi"
panelindeki bağlam ölçeri **aynı sayıyı göstermez ve göstermesi de beklenmez**.
İkisi farklı anları ve farklı bileşen kümelerini ölçer.

**Kapı nerede.** Rolling fold `internal/conversation/manager.go` içindeki
`(*Manager).Prepare` kapısında tetiklenir. Koşul:

```go
if before := EstimateTokens(summary, pending); before+overhead > maxTokens {
```

**İki terim, iki farklı içerik.**

| Terim | Ne sayar | Nerede |
|---|---|---|
| `EstimateTokens(summary, pending)` | yalnız özet metni + `Message.Text` + mesaj başına sabit çerçeve (`msgOverhead`) | `internal/conversation/tokens.go` |
| `overhead` | mesaj olmayan, her tur gönderilen sabit yük | `(*Server).contextOverheadTokens`, `internal/api/session_info.go` |

`EstimateTokens` **`Steps`'i saymaz**. Bu yüzden büyük rakamların çoğu "sohbet
metni" değil, `overhead` içindeki trace payıdır. `overhead` bileşenleri: statik
system prefix, skill kataloğu, talep-üzerine (lazy) araç kataloğu, gönderilen
(eager) araç şemaları, son araç etkinliği özeti (recent tool recap), artifact
bloğu ve **warm CLI thread'in hâlâ tuttuğu persisted `Steps` trace'i**
(`EstimatePersistedStepTokens(history[stepBase:])`).

**`stepBase` nedir.** `warmCLIStepBaseline` (`internal/api/cli_compaction.go`)
hesaplar: `max(SummaryMsgCount, CLICompactMsgCount)` — yani TionHarness'in kendi
özet sınırı ile CLI'ın kendi compaction sınırının **geç olanı**. Warm thread hiç
yoksa `-1` (soğuk tur trace tutmaz, hiçbir şey sayılmaz); sınır transkriptin
sonunu aşarsa `0` (muhafazakâr: tüm pending pencere sayılır). Bu değer
`conversation.WithContextOverheadStepBase` ile ctx'e damgalanır; çağrı yerleri
`chat_stream.go`, `chat_btw.go`, `wake_turn.go`.

**Tarihsel hata (düzeltildi).** Fold sonrası `AfterTokens`, fold **öncesi**
ölçülmüş `overhead` ile hesaplanıyordu. Katlanan mesajlar böylece iki kez
sayılıyordu: bir kez yeni özet metni olarak, bir kez de artık modele hiç
gönderilmeyen ölü trace olarak. Somut vaka: journal `75989→67182` derken bağlam
ölçeri ~30k gösteriyordu.

**Düzeltme.** `foldedStepOverhead` (`manager.go`) overhead'in
`history[stepBase:newCount]` aralığına düşen step terimini hesaplar ve
`AfterTokens` bu **düşülmüş** overhead ile raporlanır. `BeforeTokens` ise düşüm
**öncesi** overhead ile (`overheadBefore`) raporlanır — böylece her iki taraf da
kendi anındaki gerçek yükü gösterir; ikisini de düşülmüş değerle raporlamak tam
olarak fold'un kaldırdığı payı gizler ve "X→Y" oranını olduğundan küçük
gösterirdi. `Pressure` ise bilinçli olarak **düşülmüş** (fold sonrası) overhead'i
kullanır: basınç, bu tur sıkıştırmadan sonra bütçenin ne kadar dolu olduğudur.

**Kalan gerçek fark.** Düzeltmeden sonra bile iki sayı çakışmaz:

- Compaction journal satırı **geçmişin** tahminidir: fold anında ölçülmüş
  before/after.
- Bağlam ölçeri **o anda** modele gidecek yükü ölçer.

Farklı zamanlarda ve farklı bileşen kümesiyle hesaplanırlar. Okuma kuralı: aynı
sayıyı bekleme, hangisinin neyi ölçtüğüne bak.

> **Not.** `ForceCompact` (manuel `/compact`) ve `internal/conversation/reactive.go`
> (bağlam-taşması kurtarma) `overhead`'i **hiç** katmaz. İki tarafa da katmadıkları
> için kendi içlerinde oran tutarlıdır, ama mutlak sayıları `Prepare` yolundan
> gelen satırlarla karşılaştırılamaz.

## `autoCompactMode` — otomatik sıkıştırma modu (2026-08-31)

Otomatik sıkıştırma tetiklendiğinde **ne** yapılacağını seçen ayar:
`autoCompactMode`, değerler `rolling` | `native` | `auto`, varsayılan `rolling`
(`internal/settings/settings.go`; `store.go` bilinmeyen değeri `rolling`'e
normalize eder, `validate.go` açık geçersiz değeri 400 ile reddeder). Yayılım:
`(*Server).applySettings` → `Manager.SetAutoCompactMode`
(`internal/conversation/autocompact.go`).

| Mod | Davranış | Bedeli |
|---|---|---|
| `rolling` (varsayılan) | TionHarness kendi rolling özetini üretir — bugünkü davranış | Yok; transkript küçülür |
| `native` | Sağlayıcının kendi CLI compaction'ı tetiklenir (`runNativeCompact(..., nativeCompactAuto)`, `internal/api/summary.go`) | Warm CLI oturumu düşer, sonraki tur cold start (tam history yeniden gider) |
| `auto` | Yetenek varsa native, yoksa rolling | Native'in bedeli + fallback |

Native yolun önkoşulları sağlanmazsa (sağlayıcı desteklemiyor, CLI sürümü
compaction lifecycle olaylarını bilmiyor, resume edilebilir bir oturum yok, çağrı
başarısız) `errNativeCompactUnavailable` sentinel'i döner ve kapı doğrudan
rolling fold'a düşer. Varsayılanın native olmamasının sebebi bu tablodaki
"bedeli" sütunudur: warm oturumu düşürmek bir sonraki turu tamamen soğuk yapar.

**Anti-loop.** Native compaction CLI'ın penceresini küçültür ama TionHarness'in
kendi `pending` transcript'ini küçültmez — yani `EstimateTokens` aynı kalır ve
kapı bir sonraki turda yine aşımı görür. `Manager.claimNativeAttempt`
(`internal/conversation/nativecompact.go`) bu yüzden oturum kimliği başına son
denemenin **rolling sınırını** tutar: `Prepare`'in native'e geçtiği yerde
argüman `start`, yani clamp'lenmiş `SummaryMsgCount`'tır
(`internal/conversation/manager.go`). Kayıt yoksa **veya** güncel
`SummaryMsgCount` önceki denemeninkinden ileriyse claim verilir; aksi hâlde
reddedilir ve o tur doğrudan rolling fold'a düşer.

Sınır olarak history uzunluğu değil `SummaryMsgCount` kullanılmasının sebebi:
her gerçek tur transkripte user+assistant mesajı ekler, yani `len(history)` her
turda büyür — bu ölçüte bağlı bir kapı hiçbir zaman reddetmez, native her turda
ateşler ve rolling fold hiç koşmaz. `SummaryMsgCount` ise yalnız gerçek bir fold
olduğunda ilerler, yani ilerleme kanıtıdır.

Yakınsama zinciri: native turu transkripti katlamaz → `SummaryMsgCount` sabit
kalır → bir sonraki turda claim reddedilir → o tur gerçek bir rolling fold olur →
`SummaryMsgCount` ilerler → native tekrar serbest kalır. Sürekli bütçe aşan bir
oturumda native ve rolling turlar dönüşümlü koşar, transkript her rolling turda
kısaldığı için sistem yakınsar.

Claim, callback çalışmadan **önce** alınır: başarısız bir native denemesi zaten
aynı turda rolling'e düşer, tekrar claim edilmesi bir şey kazandırmaz.

Bilinen sınırlama: `lastNativeCompactAt` map'i hiç temizlenmez — oturum başına
bir giriş yazılır ve süreç ömrü boyunca durur (kodda silme yolu yoktur).

**`NativeCompacted` neden `Compacted`'ten ayrı.** Çağıranlar `Prepared.Compacted`
görünce `DropWarmCLISession` + cold resume yapar. Native compaction sonrası bu,
CLI'ın az önce kurduğu pencereyi çöpe atmak olurdu — bu yüzden native sonuç
ayrı bir bayrakla (`Prepared.NativeCompacted`) ve `Compaction.Mode` = `native`
ile raporlanır.

**Debug journal.** Native otomatik sıkıştırma `recordNativeCompactionDebug` ile
`Name: "auto-native"` adıyla, fold rakamları olmadan (hiçbir mesaj transkriptten
çıkmadı) journal'a düşer; Debug modalı ikisini tek seride listeler.

## Sırada (Faz 4+ fikirler)

- Anomali eşiklerinin ayarlanabilir olması (settings).
- Workspace-geneli "en pahalı oturumlar" / araç ısı haritası panosu.
- Gerçek byte-tasarrufu ölçümü istenirse: sqz'yi PostToolUse output-rewrite
  moduna alıp `runPostToolHooks`'ta `len(önce)−len(sonra)` ölçmek (ayrı iş).
