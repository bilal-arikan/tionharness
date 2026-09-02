# TionHarness — İlerleme Takibi

## Oturum kökeni tek kaynağa bağlandı — Rota altyapısı R1 (2026-09-02) ✅

**Belirti.** "Bu oturumu kim başlattı" sorusunun altı ayrı cevabı vardı:
`ParentSessionID` handoff, `CoordinatorSessionID` worker, `Kind+SourceID` flow'a
işaret eder run'a değil, `FlowRun.SessionID` koşu **bitince** damgalanır,
automation-run için yalnız `Automation.LastSessionID`. Rota (dinamik akış grafiği,
`_Docs/77`) projeksiyonunun kenarları bu veriden türeyecek; dağınık haliyle tahmin
gerekirdi.

**Ne.** `Session.Origin *SessionOrigin` (`kind`: user/spawn/coordinator/subagent/
flow/schedule/automation/handoff/insight + `entityId`/`runId`/`nodeId`/
`triggerSessionId`/`rootSessionId`/`at`), tek damgalama noktası
`createSessionLocked`, `Lineage()`/`RootSession()` okuyucuları, boot'ta eski
header'lar için bellek-içi backfill (dosya yeniden yazılmaz), `SessionSchemaVersion`
3→4. Açık köken veren yollar: `SpawnOptions.Origin` (handoff, schedule spawn,
automation one-shot), `RunSpec.Origin` (automation/schedule → flow, ctx ile
`runFlowRecorded`'a taşınır), flow-coordinator düğümü (run + node), insight koşusu.
`FlowRun.SessionID` artık `RunFlow` içinde satır oluşur oluşmaz damgalanıyor ve
`SetSessionOriginRun` aynı anda `origin.runId`'yi tamamlıyor. Yeni
`db.SetSessionHook` (create/state/runstate/origin/delete; kilit dışı, `SetBoardHook`
ikizi) R3 workspace olay günlüğünün besleme noktası. Frontend: `Session.origin`
tipi; `SessionFlowInline` 5 sn `createdAt` eşleştirmesini yalnız link-öncesi
mağazalarda kullanıyor.

**Dosyalar.** `internal/db/models_session_origin.go` (yeni), `store_session_hook.go`
(yeni), `models.go`, `store.go`, `db.go`; `internal/agent/flow_origin.go` (yeni),
`flow.go`, `flow_coordinator.go`, `spawn.go`, `launch.go`, `automation.go`,
`scheduler.go`, `handoff.go`, `insightsteps.go`; `frontend/src/types/session.ts`,
`features/flows/SessionFlowInline.tsx`; `_Docs/02`, `_Docs/77`, `CLAUDE.md`.

**Doğrulama.** `go build ./...` ✅; `go test` db/api/workspace/view/tools/agent
paketleri ✅ (yeni: `TestDeriveOrigin`, `TestCreateSessionStampsOrigin`,
`TestLegacyHeaderOriginBackfilledOnLoad`, `TestSetSessionOriginRun`,
`TestSessionHookOpsAndLockFreedom`, `TestSessionHookNotFiredOnFailure`,
`TestRunFlowStampsSessionAtStart`, `TestLaunchRunFlowCarriesLauncherOrigin`,
`TestSpawnWorkerOriginIsCoordinator`, `TestHandoffContinuationOrigin`);
`npx tsc --noEmit` ✅; vitest flows+sessions ✅.

## Ağ fiziği yeniden açılışta kaldığı yerden sürüyor (2026-09-02) ✅

**Sorun:** Ağ yerleşimi düğümlerin `x/y` koordinatlarını saklıyordu; ekran hareket
hâlindeyken kapatılsa bile yeniden açılış bütün kayıtlı düğümleri durağan kabul edip
fiziği kapatıyordu. Görsel konum korunuyor, hareketin hızı ve aktif simülasyon bilgisi
kayboluyordu.

**Fix:** Yerleşim snapshot'ı artık `physicsActive` ile düğüm başına `vx/vy` hızlarını
da taşıyor. Aktif snapshot açıldığında koordinatlar constructor öncesi uygulanıyor,
vis-network velocity tablosu geri yükleniyor ve stabilization aynı fizik durumundan
devam ediyor. Durağan snapshot davranışı değişmedi. Tema, yoğunluk ve lite değişimleri
simülasyonu zorla kapatmak yerine mevcut aktiflik durumunu koruyor. Eski, hız içermeyen
v1 kayıtları geriye uyumlu okunuyor; bozuk hızlar atılıp geçerli konum korunuyor.

**Dosyalar:** `frontend/src/features/network/networkLayoutStorage.ts`,
`networkPhysicsState.ts`, `VisNetworkGraph.tsx` ve testleri; ayrıntı `_Docs/23`.

**Doğrulama:** `npm test` 93 dosya / 670 test ✅, `npm run format:check` ✅,
`npm run build` ✅. Canlı Playwright testinde aktif çıkış 88 düğüm kaydetti; 79
düğümde finite `vx/vy` bulundu ve yeniden açılış sonrası aynı 79 düğüm kayıtlı
koordinatından ilerledi. Konsol hatası yok, test öncesi kullanıcı snapshot'ı geri yüklendi.

## Workspace arka plan işleri Close'da drenaj ediliyor — `activity-inbox` yarışı kapandı (2026-09-02) ✅

**Doğrulama:** `go test ./internal/api -count=3` ✅ (111s, üç koşu da temiz),
`go test ./... -count=1` ✅ **sıfır FAIL** — bu depoda uzun süredir ilk kez.
`go vet ./internal/workspace` ✅, `git diff --check` ✅.

**Sorun:** `internal/api` tam koşuda rastgele bir test
`TempDir RemoveAll cleanup: ...store\activity-inbox: Dizin boş değil` ile düşüyordu
(son iki tam koşuda `TestRewindRejectedOnReadOnlySessions` ve
`TestRewindAllowedOnWritableKinds`; tek başına koşunca ikisi de geçiyordu). Rewind ile
ilgisi yoktu — `AddMessage` çağıran **herhangi** bir test aday.

**Kök neden:** `manager.go`'daki store hook'ları işi **takip edilmeyen** `go func()`
ile fırlatıyordu: her mesaj eklemesinde bir `DrainActivityInbox`, ayrıca
`OnActivityRecorded`, açılışta bir drenaj ve board hook'unda iki tane daha. Test bitip
`t.TempDir()` ağacı silmeye başladığında bu goroutine'ler hâlâ
`store/activity-inbox` altına yazıyordu; Windows açık/yeni dosya yüzünden silmeyi
reddediyor. POSIX'te görünmez.

**Nasıl:** `Manager`'a `bg sync.WaitGroup` + `bgClosed` bayrağı ve `goBackground`
yardımcısı eklendi; beş fırlatma noktası buna taşındı. `Close()` artık **önce** bayrağı
kaldırıp `bg.Wait()` çağırıyor, **sonra** scheduler/runtime/DB'yi kapatıyor — drenaj
canlı store'a karşı bitiyor, kapanış başladıktan sonra kuyruğa girecek iş ise hiç
başlatılmıyor. Sıra önemli: DB'yi önce kapatmak, bitiremeyeceği iş için hata logu
üreten bir drenaj bırakırdı.

**Dosyalar:** `internal/workspace/manager.go`.

## Shell prompt bloğu CLI turunda araçları namespace'li adıyla duyuruyor (2026-09-02) ✅

**Doğrulama:** `go test ./internal/agent -run 'ShellToolsContextBlock'` ✅ (7/7),
`go build ./...` ✅.

**Sorun:** `ShellToolsContextBlock` "Call Bash by that exact name" diyordu. claude-cli
yolunda TionHarness kendi shell'ini köprülediğinde CLI'ın **native** `Bash` ailesi
bilerek `--disallowedTools`'a giriyor (`climcp.go`), yani çıplak `Bash` çağrısı
`No such tool available: Bash. Bash is disabled for this session` ile ölüyor ve model
namespace'i tahmin ederek toparlanmak zorunda kalıyor. Bu oturumda birebir yaşandı;
`INSIGHT-BACKLOG.md` de aynı uyumsuzluğu iki oturumda kaydetmiş.

**Nasıl:** `shellToolNamesFor(provider, names)` — `skillToolNameFor` ile aynı desen ve
aynı `isCLIProviderKind` kapısı: CLI sağlayıcısında adlar `interactionToolPrefix` ile
öneklenir, native sağlayıcıda çıplak kalır. Allow/denylist geçidi **çıplak** adlar
üzerinde kalıyor — araç filtresi onlarla anahtarlanmış durumda.

**Dosyalar:** `internal/agent/runtime_prompt.go`, `internal/agent/shellcontext_test.go`.

## `get_session_info` başka bir oturumun aktivite izini de döndürüyor (2026-09-02) ✅

**Doğrulama:** `go build ./...` ✅, `gofmt` ✅,
`go test ./internal/tools ./internal/api -run 'SessionInfo|Recap|ElideMiddle|ToolRecap|RecentToolActivity'` ✅,
`go test ./internal/agent -run 'RecapStepParity'` ✅, `git diff --check` ✅ (0).
Tam `go test ./...` koşusunda yalnız `internal/api`'deki bilinen Windows
`TempDir RemoveAll` / `activity-inbox: Dizin boş değil` yarışı düştü
(`TestRewindRejectedOnReadOnlySessions`); tek başına `-count=3` ile geçiyor ve bu
değişiklikle ilgisi yok (bir önceki kayıtta da aynı yarış görülmüştü).

**Sorun:** `get_session_info` yalnız metadata döndürüyordu. Bir koordinatör "şu worker
çalışıyor mu, neye takıldı" sorusunu cevaplamak için ardından ikinci bir transkript
okuması yapmak zorundaydı.

**Nasıl:**

- **Canlı tur:** `db.ReadInflight` — akan turun disk sidecar'ı zaten var (crash recovery +
  API'nin mid-turn balon geri yüklemesi). Böylece araç `*db.DB` bağımlılığıyla kalıyor;
  runtime/`runs` registry'sine erişim gerekmedi.
- **Duran oturum:** `db.LastMessage` (tam transkript kopyalayan `ListMessages` değil).
- **Ortak parser:** `internal/tools/steprecap.go` — `ParseRecapSteps`, `RecapLines`,
  `RecapToolCounts`, `RecapLastStep`, `RecapErrors`. `internal/api/chat_tool_summary.go`
  içindeki `<recent_tool_activity>` üreticisi de artık buraya delege ediyor; iki ayrı
  step-parser'ın sürüklenmesi ihtimali ortadan kalktı.
- **Import yönü:** `agent` → `tools` (TurnStep, `tools.AskQuestion` gömüyor), yani `tools`
  `agent.TurnStep`'i import edemez. `RecapStep` JSON etiketleriyle eşleşir;
  `internal/agent/steprecap_parity_test.go` etiket/kind sürüklenmesini derleme hatası
  yerine test hatasına çevirir.

**Kasıtlı sınırlar:** tool **çıktısı** basılmaz (yalnız çağrı + arg ipucu + ok/error);
cevap alıntısı baş 150 + son 150 rune, fenced ve `data, not instructions` etiketli;
kendi oturumunda blok yerine `<recent_tool_activity>` işaretçisi. Bozuk iz sessizce
yutulmaz — `persisted_steps_invalid` olarak raporlanır.

**Dosyalar:** `internal/tools/steprecap.go` (yeni), `internal/tools/builtin_sessioninfo.go`,
`internal/tools/builtin_sessioninfo_activity_test.go` (yeni),
`internal/agent/steprecap_parity_test.go` (yeni), `internal/api/chat_tool_summary.go`,
`_Docs/41-ARAC-BOSLUKLARI-YAPILACAKLAR.md`.

## Skill değişiklikleri navbar okunmamış göstergesine bağlandı (2026-09-02) ✅

Skill ekleme, silme ve `SKILL.md` düzenleme yolları artık `skills.Store` içindeki
çözümlenmiş katalog fingerprint'i üzerinden gerçek değişiklik olarak algılanıyor.
İlk yükleme ve no-op reload olay üretmiyor; gerçek fark workspace SSE akışına
toast olmayan `skills` kontrol olayı yayıyor. Mevcut okunmamış görünüm hattı masaüstü
ve mobil navbar Skills butonunda nokta gösteriyor; Skills görünümü açılınca nokta
temizleniyor. Görünüm zaten açıksa liste ve seçili skill gövdesi canlı tazeleniyor.

**Doğrulama:** `go build ./...` ✅, `go vet ./...` ✅,
`go test ./internal/skills ./internal/events` ✅,
`go test ./internal/agent -run '^TestSkillCatalogChangePublishesControlEvent$'` ✅,
frontend üretim derlemesi + 93 dosya / 663 test + Prettier kontrolü ✅. Tam Go
koşusunda ilgili `agent/events/skills/db` paketleri geçti; `internal/api` içindeki
mevcut Windows `TempDir RemoveAll` / `activity-inbox: Dizin boş değil` yarışı genel
koşuyu engelledi.

## Alt-koordinatör tur ortasında ebeveynine mesaj göndermiyor (2026-09-01) ⏳

**Belirti.** Dağıtım yapan bir alt-koordinatör, kendi turu sürerken ebeveyn
oturuma `<task-progress status="delegating">` mesajı enjekte ediyordu. Bu, ebeveyn
için yeni bilgi taşımayan ama turu kesen bir gürültüydü; aynı bilgi ebeveynin canlı
worker görünümünde zaten mevcuttu.

**Nasıl:**

- `notifyDelegating` kaldırıldı — alt-koordinatör dağıtım sırasında ebeveyne
  hiçbir ara mesaj göndermiyor.
- Bilgi kaybı yok: ebeveynin canlı worker görünümünde `subCoordinatorBusy()`
  (rapor borcu olan **veya** canlı worker'ı bulunan düğüm) artık
  `Running+Delegating` olarak görünüyor.
- `report_to_coordinator` notu tur ortasında değil, **tur sonunda** gönderiliyor:
  not `pendingUpwardReport` zulasına yazılır, `flushUpwardReport` turun bitiminde
  boşaltır. Failed/killed/panik dahil her terminal yol kapsanır, böylece rapor
  turun nasıl bittiğinden bağımsız olarak yukarı ulaşır. Worker turu **dışında**
  yapılan çağrıda senkron gönderim korunur.

**Dosyalar:** `internal/agent/coordination.go`, `internal/agent/coordination_tree.go`,
`internal/agent/coordination_situation.go`, `internal/agent/workernotesteps.go`,
`internal/tools/builtin_coordination.go`, `internal/prompts/defaults/coordinator.md`,
`internal/skills/defaults/tionharness-coordinator/SKILL.md`,
`_Docs/47-KOORDINATOR-COKLU-AJAN.md`.

## Oturum listesi sayfalaması çip seçimine göre sunucuda filtreleniyor (2026-09-01) ⏳

**Belirti.** Sidebar'da 52 aktif sohbet varken listede yalnız ~3'ü görünüyordu ve
"Daha fazla yükle" bir işe yaramıyor gibiydi. Neden: sayfalama TÜM türler üzerinde
yapılıyordu (`/api/sessions?limit=100`), çip filtresi ise sayfa geldikten SONRA
istemcide uygulanıyordu. Worker/subagent oturumları listenin başını doldurduğu için
100'lük pencerede yalnız birkaç sohbet kalıyor, `total`/`hasMore` ise kullanıcının
göremediği satırları sayıyordu.

**Ne:** Çip seçimi artık liste isteğinin bir parçası — sunucu aynı çip yüklemini
sayfalamadan ÖNCE uygular, dolayısıyla `total`/`hasMore` ve "Daha fazla yükle"
gerçekten gösterilebilen satırları anlatır.

**Nasıl:**

- `internal/api/sessions_chips.go` (yeni): sidebar çip yükleminin sunucu ikizi —
  `sessionChipKey` (kategori → executionType → legacy kind sırası),
  `sessionIsWorker`, `sessionMatchesChips`, `sessionChipCounts`. İki canlılık çipi
  (`running`, `awaiting-workers`) kasıtlı olarak dışarıda: canlılık istemci
  durumudur, sunucu bilmez; onlar sayfa üzerinde istemcide daraltmaya devam eder.
- `internal/api/sessions.go`: `chips` sorgu parametresi (virgülle ayrılmış seçili
  çipler). Parametrenin **hiç verilmemesi** "filtre yok", **boş verilmesi** ise
  "hiçbir çip seçili değil" demektir ve hiçbir şeyle eşleşmez. Zarfa `chipCounts`
  eklendi: filtre uygulanmadan ÖNCE tüm workspace üzerinden sayılır, böylece
  işaretsiz bir çip kaç satır sakladığını göstermeye devam eder.
- `frontend/src/features/sessions/useSessionChips.ts` (yeni): çip seçimi
  sidebar'dan yukarı taşındı (istek onu taşıdığı için tek kaynak gerekiyordu);
  localStorage kalıcılığı ve Ctrl/Shift tıklama semantiği burada.
- `useSessionsController.ts`: her `listSessions` çağrısı `chips` gönderir; seçim
  değişince ilk sayfa yeniden çekilir (`loadedPageSizeRef` sunucu sıralamasındaki
  offset'i tutar). `withActiveSession`: AÇIK oturum çip filtresine takılsa bile
  state'te tutulur — başlık/composer/transkript onu bu listeden çözüyor, düşerse
  okunan sohbet boşalırdı; sidebar aynı yüklemi istemcide de uyguladığı için satır
  yine listede görünmez.
- `SessionsSidebar.tsx`: çip durumu prop'tan gelir; rozet sayıları sunucunun
  `chipCounts`'undan okunur (canlı çipler yüklü satırlardan). Başlık "Yüklenenlerde"
  yerine "Filtreler".

**Doğrulama:** `go build ./...` ✅, `go test ./internal/api/ -count=1` ✅ (yeni:
`TestListSessionsChipFilterPagesFilteredSet`, `TestListSessionsChipScopeChips`,
`TestListSessionsChipsEmptyVersusAbsent`, `TestSessionChipKeyClassification`);
`npx tsc --noEmit` ✅, `npm test` ✅ (83 dosya / 613 test).

## Oturum akışlarındaki sessiz olaylar journal'a bağlandı (2026-09-01) ⏳

**Belirti.** Bir turun teardown tarafından iptal edilmesi, kuyruktan hiç
çalışmadan düşen bir mesaj, atlanan bir native compaction ve çıktısız kalıp yine
de faturalanan bir tur hiçbir iz bırakmıyordu: olay ya "kendiliğinden ölmüş" bir
tur ya da açıklamasız bir rolling fold olarak okunuyordu.

- **Yeni `lifecycle` olay tipi.** Oturum yaşam döngüsü olayları için ayrı tip
  (`internal/db/debug_journal.go` `DebugLifecycle`); okuyucularda
  `lifecycleEvents` sayacı, araç `type` enum'u ve UI filtre çipiyle temsil edilir.
- **9 yeni adlandırılmış olay.** `lifecycle`:
  `turn_cancelled_by_teardown`, `autonomous_cancelled`, `teardown_grace_exceeded`
  (`internal/api/session_teardown.go`), `queued_turn_dropped`
  (`internal/api/inbox_debug.go`). `compaction`: `native_skipped`,
  `claim_consumed`, `native_fallback_rolling`
  (`internal/conversation/manager.go` + `nativecompact.go`). `pressure`:
  `fold_idle_floor` (`internal/conversation/foldtimeout.go`) — fold yalnız
  `FoldIdleOutputFloor` sayesinde watchdog'dan kurtulduğunda. `llm_call`:
  `failed_turn_billed` (`internal/agent/toolloop.go` `recordFailedUsage`) —
  çıktısız ama faturalanan tur; çift sayımı önlemek için aggregate'lere girmez.
- **`emitDebug` bypass'ı kapandı.** Runtime'a erişemeyen katmanlar (conversation
  manager, API kuyruk/özet dayanıklılık yolları) artık
  `internal/db/debug_journal_policy.go` içindeki `AppendDebugEventGated`
  üzerinden yazar; `DebugJournalEnabled` bayrağı ve kullanıcı cap'i bu ikinci
  huniye de uygulanır. Ham `AppendDebugEvent` yalnız kapıların içinden çağrılır.
- **Runtime workspace'leri politikayı devralıyor.** Çalışırken oluşturulan veya
  attach edilen workspace'ler store'ları açılır açılmaz aynı ayarı alır
  (`internal/api/workspaces.go` → `applyDebugJournalToStore`); önceden bir sonraki
  ayar güncellemesine kadar journal açık + varsayılan cap ile kalıyorlardı.
- Tablo ve operatör notları: `_Docs/38-SESSION-DEBUG.md`.

## Araç izin katmanı ve inbox dayanıklılığı fail-closed oldu (2026-09-01) ⏳

**Belirti.** Bozuk bir izin/konfigürasyon belgesi sessizce "kısıt yok" anlamına
geliyordu: çözülemeyen `AllowedTools`/`ToolOverrides`/`BlockedTools` belgesi
kısıtsız bir araç yüzeyi, bozuk `inbox.json` ise boş bir kuyruk üretiyordu.

- **`mcpServerGate` / `allowFunc` / `blockFunc` artık deny-all döner.** Belge
  çözülemediğinde kapı hiçbir aracı ve hiçbir MCP sunucusunu geçirmez; tur
  denetlenmemiş bir araç yüzeyiyle başlamak yerine durur
  (`internal/agent/mcpservergate.go`, `climcp.go`, `codexmcp.go`,
  `toolsetup.go`). Detay: `_Docs/19-LAZY-TOOL-LOADING.md`,
  `_Docs/52-MCP-GATEWAY.md`.
- **Bozuk `inbox.json` karantinaya alınıyor.** Sidecar okunamadığında dosya
  `inbox.json.corrupt-<unix>` olarak yeniden adlandırılır, kuyruktaki mesajlar
  **dağıtılmaz** ve durum hem log'a hem oturum debug günlüğüne yazılır
  (`internal/api/inbox_durability.go` `quarantineInbox`). Detay:
  `_Docs/58-QUEUE-SENKRON.md`.
- **`RecoverOrphanedTurns` sessiz erken dönüşü bitti.** `ListSessions` hatası
  artık loglanır ve debug günlüğüne düşer (`internal/agent/coordination.go`).
- **Görünürlük.** Dört yol da `db.DebugError` tipinde adlandırılmış olay yazar:
  `mcp_server_gate_malformed`, `tool_permission_config_malformed`,
  `inbox_corrupt`, `orphan_recovery_failed` — tablo:
  `_Docs/38-SESSION-DEBUG.md`.

## Debug journal enum drift'i kapatıldı (2026-09-01) ⏳

**Belirti.** Yeni debug olay tipleri eklendikçe okuma tarafı geride kaldı: bazı
tipler diskte vardı ama özet/tur görünümünde sayılmıyor, araç şemasında
filtrelenemiyor ve UI'da çipi bulunmuyordu — yani yazılan olay pratikte
görünmezdi.

- **Okuyucu arm'ları tamamlandı.** `GetTurnDebug`, `GetDebugSummary` ve
  `summarizeDebugDetail` eksik tipleri karşılıyor; her tipin ayrı redaksiyon
  etiketi var.
- **15 tipin tamamı karşılanıyor.** Her tip artık en az bir sayaç/özet alanı,
  `read_session_debug` araç şemasındaki `type` enum'u ve UI filtre çipi ile
  temsil ediliyor; tip → okuyucu tablosu `_Docs/38-SESSION-DEBUG.md`'de.
- **Frontend hizalaması.** Debug olay tipi union'ı ve çip listesi aynı 15 tipi
  içeriyor.

## Codex native compaction'ı idle watchdog'u tetiklemiyor (2026-09-01) ⏳

**Belirti (SES2570, TSK693).** Codex `context_compaction` item'ını duyurup
compaction model çağrısı boyunca susuyordu. `codexIdleOutputTimeout` bu sessizliği
takılma sayıp süreç ağacını öldürüyor, hata **non-retryable** dönüyor ve oturum
`blocked` kalıyordu — bağlam %92 dolduğunda, yani tam da compaction'ın gerekli
olduğu anda.

- **Uçuştaki compaction sayacı.** `codexStreamParser` artık `item.started` ile
  açılıp `item.completed` ile kapanan compaction'ları sayıyor
  (`compactionActive`, `compactionInFlight()`). Sayaç yalnız read-loop
  goroutine'inden yazılıp okunduğu için kilit gerekmiyor.
- **Sınırlı grace penceresi.** `runAttempt` read loop'unda `case <-idle.C`,
  compaction uçuştaysa `codexCompactionIdleGrace` (2) ek pencere bağışlayıp
  timer'ı sıfırlıyor. Hiç tamamlanmayan bir compaction en geç 3× idle
  penceresinde ölüyor; bu tur idle watchdog'unun (20 dk) altında kalıyor.
- **Idle takılması artık koşullu retryable.** `retryable = !p.ranTool()`: hiç
  araç çalışmadıysa turun yan etkisi yok, bir kez yeniden koşulabilir. Retry
  döngüsü bunu `codexIdleHangError` / `isCodexIdleHang` ile tanıyıp **tek** yeniden
  koşuyla sınırlıyor — her denemenin tam bir idle penceresi harcaması yüzünden.
  Hata metni kullanılan grace penceresi sayısını ve retryable etiketini taşıyor.
- **Testler.** `internal/providers/codexcli_hang_test.go`:
  `TestCodexIdleWatchdogWaitsOutNativeCompaction`,
  `TestCodexIdleHangAfterToolIsNotRetryable`; mevcut
  `TestCodexRunAttemptIdleOutputTimeoutWhileGrandchildHoldsPipe` beklentisi
  "non-retryable" yerine "araç çalışmadı → retryable" olarak güncellendi.
- **Fold yolu ayrı katman.** Tek atışlık fold/handoff çağrılarının idle tabanı
  (`conversation.FoldIdleOutputFloor` + `providers.WithMinIdleOutputTimeout`)
  bağımsız durmaya devam ediyor; bu değişiklik tur içindeki native compaction'ı
  kapsıyor. Detay: `_Docs/69-CODEX-CLI-SAGLAYICI.md`.

## `run_subagent` senkron-tek moda indi, `stop_subagent` silindi (2026-09-01) ⏳

**Karar.** `run_subagent` artık her zaman senkron çalışır. Şemadaki `wait` alanı
tek sürümlük bir geçiş için kabul edilmeye devam ediyor ama **kullanımdan
kaldırıldı ve etkisiz**: `wait:"sync"` (veya alanın hiç verilmemesi) çalışır,
`wait:"async"` artık hata döndürür. `stop_subagent` aracı tamamen silindi.

- **Neden.** Durdurulabilir bir alt-ajan koşusunun tek üreticisi async daldı.
  Senkron çağıran, çocuk koşarken araç çağrısının içinde bloklu bekler; o turda
  ikinci bir araç çağrısı — yani bir iptal çağrısı — yayınlayamaz. Async gidince
  `stop_subagent`'ın hedefleyebileceği erişilebilir bir koşu kalmadı: araç dar
  değil, ölü hale geldi. Koordinatör işçileri `Kind="worker"` taşır ve ayrı
  `stop_worker` ile durdurulur; onlar etkilenmedi.
- **Yerine ne var.** Turdan uzun sürecek iş için iki yol kaldı: işi kendi içinde
  tamamlanan birkaç küçük `run_subagent` çağrısına böl, ya da koordinatör moduna
  geç (`set_coordinator_mode`) ve `spawn_worker` ile arka planda koştur. Spawn guard'ları ve `launchSpawn`'ın
  iptal kaydı yerinde — hâlâ `spawn_worker`, köprülenen `spawn_session`, peer
  mesajları, otomasyonlar ve flow'lara hizmet ediyorlar.
- **Metin temizliği.** Async'i öneren tüm prompt/skill/doküman/arayüz metinleri
  güncellendi (`default-instructions.md`, `tionharness-self-management` ve
  `tionharness-autonomous-ops` skill'leri, `25-SUBAGENT-ISOLATION.md`,
  `22-SPAWN-SESSION.md`, `24-SELF-MANAGEMENT.md`, `47-KOORDINATOR-COKLU-AJAN.md`,
  `types/settings.ts`, `AppToolsPanel.tsx`). Hiçbir talimat artık modele `wait`
  göndermesini söylemiyor; alan yalnız bayat çağrıları yumuşak karşılamak için
  duruyor. Geçmiş kayıtlar (`03-YOL-HARITASI`,
  bu dosyanın eski girdileri) tarihsel olarak olduğu gibi bırakıldı.

## CLI olay çözümlemesi toleranslı, kalıcı oturumda watchdog (2026-08-31) ⏳

**Belirti.** `api_error_status` alanı bazı CLI sürümlerinde slug ("rate_limit"),
bazılarında çıplak HTTP durumu (429) geliyordu. Alan `string` olduğu için sayı
geldiğinde `encoding/json` **tüm olayı** reddediyor, result zarfı ile birlikte
`session_id` (→ `--resume` kırılıyor), `usage` (→ tur 0 token faturalanıyor),
`is_error` ve `result` metni kayboluyordu; `sawResult` false kaldığı için tur
"no result in stream" ile düşüyordu.

- **Toleranslı çözümleme.** Yeni `flexString` tipi (`claudecli_stream.go`) string,
  sayı, bool ve null kabul eder; `cliEvent.APIErrorStatus` bu tipe geçti. Katı
  çözümleme yine de düşerse `salvageCLIEvent` olayı **alan alan** çözer, başaranları
  tutar, düşenleri isimleriyle döner. Kayıp asla sessiz değil: ilk olay
  `[claude-cli field drop]` notu alır.
- **Sayısal durum sınıflandırması.** `classifyAPIErrorStatusCode` 429'u rate limit,
  401/403'ü auth hatası sayar — 429 artık genel hata gibi boşuna retry edilmiyor.
- **Hata turunda muhasebe.** Result zarfının `usage` + `num_turns` katlaması
  `is_error` dalının **üstüne** taşındı; rate-limit/auth turlarının ödenmiş input
  token'ları artık kayıt altında.
- **Düşürme sayacı.** Tur başına tek ayrıntılı not korunuyor, kalanlar sayılıp
  `finish()`'te `+N more ...` özeti olarak ekleniyor (claude ve codex). Codex
  ayrıştırıcısı bozuk satırları artık tamamen sessiz düşürmüyor
  (`[codex parse drop]`).
- **Kalıcı oturum watchdog'u.** `CLISession.Turn` okumayı goroutine'e aldı ve
  `ctx.Done()` / startup (90 sn) / idle (varsayılan 15 dk,
  `SetCLISessionIdleTimeout` ile ayarlanır) üzerinden `select` ediyor. Bloklayan
  `bufio.ReadString` yüzünden iptal yalnız satır aralarında görülüyordu; asılan
  bir CLI `s.mu`'yu süresiz tutup aynı (oturum, ajan) anahtarının sonraki tüm
  turlarını kilitliyordu. Watchdog süreci `proc.KillTree` ile indiriyor, oturumu
  kapalı işaretliyor (havuz düşürüyor) ve `WatchdogKill` raporluyor.

Testler: `TestCLIParserKeepsResultEnvelopeWithNumericAPIErrorStatus`,
`TestCLIParserSalvagesEventAndReportsDroppedField`,
`TestCLIParserSummarizesRepeatedParseDrops`, `TestCLIParserRecordsUsageOnErrorResult`,
`TestCodexParserReportsMalformedLineOnceAndCountsTheRest`,
`TestCodexParserIgnoresNonJSONLines`,
`TestCLISessionTurnHonoursContextCancellation`,
`TestCLISessionTurnIdleWatchdogReclaimsSilentStream`.

## Sidebar yerel aramasında session ID desteği (2026-08-31) ⏳

- **TSK570.** `SessionsSidebar` yerel filtresi kapsam çiplerinden sonra başlık yanında
  session ID üzerinde de case-insensitive substring eşleşmesi yapıyor. Mesaj gövdesi
  aramasının debounce/backend akışı değişmedi. Arama placeholder'ı başlık, ID ve mesaj
  kapsamını açıkça belirtiyor; saf `sessionMatchesQuery` yardımcısı başlık, kısmi ID,
  alakasız sorgu, ID büyük/küçük harf ve boş sorgu vakalarıyla test ediliyor.

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-08-31**

## Görsel anotasyon canlı kabul ve mobil modal katmanı (2026-08-31) ✅

Clipboard görseli, bozuk kaynak, serbest kalem, undo/redo/clear, kaydet/iptal,
pointer'ın canvas dışına çıkması, resize/piksel sınırı ve klavye/odak/ARIA akışları
izole veri dizini ve portta gerçek Chromium ile doğrulandı. Mobil viewport'ta
workspace öneri kartlarının anotasyon modalındaki **Kaydet** düğmesini örttüğü
bulundu. `ImageAnnotator` body portalına taşındı; modal artık üst seviye stacking
context'te kalıyor. Aynı hit-test düzeltme sonrası Kaydet düğmesini döndürdü;
konsol ve ağ hatası oluşmadı. Ayrıntı: `_Docs/07-CHAT-UX.md`.

## Auto-compact gate'i kalıcı CLI izini sayıyor (2026-08-31) ⏳

**Belirti (ölçüldü).** SES2230 (codex-cli, gpt-5.6-sol, etkin bütçe 70000): panel
257272 token gösterirken fold gate'inin gördüğü sayı 25190'da kalıyordu — fark
232083 (185 kalıcı step, 32 asistan mesajı). Sonuç: `session.json` içinde
`summary:""`, `summaryMsgCount:0`, `debug.jsonl`'in 237 satırında `foldIndex:0`.
Panel bütçenin 3.7 katını gösterirken hiç compaction tetiklenmiyordu.

**Kök neden.** `EstimateTokens` `Steps`'i bilinçli olarak saymaz (o iz sağlayıcıya
yeniden gönderilmez), ama sıcak bir CLI thread'inde sağlayıcı onu **kendi
tarafında tutmaya devam eder**. Panel bu izi ayrı bir kovada sayıyordu, gate ise
hiç görmüyordu.

- **Tek pay kaynağı.** Yeni `conversation.EstimatePersistedStepTokens(msgs []db.Message) (int, error)`
  (`internal/conversation/persisted.go`) yalnız asistan mesajlarının `Steps`
  izini toplar. Bozuk iz sessizce 0 sayılmaz, hata olarak döner.
  `EstimateTokens` **değişmedi** — `TestStepsAreNotSentAndNotEstimated` aynen
  yeşil; iz gate'e ayrı bir terim olarak eklenir.
- **Sağlayıcı adı yerine sıcak-thread testi.** `buildFillers`'ın `retainSteps`
  koşulu `agentRow.Provider == "codex-cli"` idi; iki yönden yanlıştı — claude-cli
  de aynı delta gönderimini yapıyor (`claudeResumeDecision`) yani orada hata hiç
  sayılmıyordu, codex dalı ise **soğuk** turu da sayıp doluluğu şişiriyordu. Artık
  tek yardımcı: `hasWarmCLIThread(session)` = `CLISessionID != "" && CLISentMsgCount > 0`
  (`internal/api/session_info.go`). Provider string karşılaştırması tamamen kalktı.
- **Gate aynı paya oturdu.** `contextOverheadTokens` artık `(int, error)` döner ve
  sıcak thread durumunda `EstimatePersistedStepTokens(pending)` terimini ekler; üç
  çağrı sitesi (`chat_stream`, `chat_btw`, `wake_turn`) hatayı yutmadan raporlar.
  Böylece `Prepare`'in `before+overhead > maxTokens` gate'i panelle **aynı** sayıyı
  görür.
- **`Prepared.Pressure` erken uyarıya bağlandı.** Alan hesaplanıyor ama üretimde
  hiç okunmuyordu. `Prepare` artık fold etmeyen ama `pressure >= 0.85` olan turda
  `debug.jsonl`'e yeni `db.DebugPressure` ("pressure") olayı yazar; fold olan tur
  zaten kendi `compaction` olayını üretiyor, ikinci uyarı yazılmaz.

Kapsam dışı bırakıldı (ayrı iş): codex native compaction'ı otomatik yola bağlamak,
auto-compact sınırını geri beslemek — bkz. bir sonraki bölüm.

Testler: `TestEstimatePersistedStepTokensCountsAssistantOnly`,
`TestEstimatePersistedStepTokensSurfacesMalformedTrace`,
`TestPrepareFoldsOnPersistedTraceOverhead` (SES2230'un birim karşılığı: metin
bütçe altında, iz dahil üstünde), `TestPrepareJournalsContextPressure`,
`TestHasWarmCLIThread`, `TestBuildFillersCountsRetainedTraceOnlyWhenWarm`.
Doğrulama: `go test ./internal/conversation/... ./internal/api/... ./internal/db/... -count=1` ok,
`git diff --check` sıfır.

## CLI kendi kendine compact edince sınır geri besleniyor (2026-08-31) ⏳

Bir önceki bölümün kapsam dışı bıraktığı yarım: CLI sağlayıcılar bağlamları
dolunca **kendi içlerinde** compaction yapar ve bunu bir yaşam-döngüsü olayı
olarak bildirir (claude-cli: `compact_boundary` / `PostCompact` / `status`,
codex-cli: `context_compaction` item'ı, `Trigger:"auto"`). TionHarness bu sinyali
**algılıyordu ama ölçüme geri beslemiyordu**: sağlayıcı izi kendi penceresinden
attıktan sonra da `contextOverheadTokens` ve panel kovası aynı `Steps`'i saymaya
devam ediyor, meter şişik kalıyor ve gate gereksiz erken fold tetikleyebiliyordu.

- **Yeni sınır alanı.** `db.Session.CLICompactMsgCount` — sağlayıcının kendi
  bağlamını en son hangi transkript uzunluğunda sıkıştırdığı.
  `DB.SetSessionCLICompactBoundary(ctx, sessionID, msgCount)` **monotondur**: geç
  gelen küçük bir değer sınırı geri sarmaz.
- **Tek eşik hesabı.** `warmCLIStepBaseline(session, historyLen)`
  (`internal/api/cli_compaction.go`) iki tabanın **geç olanını** döner: rolling
  summary sınırı (`SummaryMsgCount`) ve CLI'nin kendi compaction sınırı. Sıcak
  thread yoksa `-1` döner (hiçbir iz sayılmaz). Sınır transkriptin dışına
  düşerse (mesaj silme/düzenleme) 0'a düşülür — sessizce sıfır saymak yerine
  eskisi gibi tümü sayılır.
- **İki ölçüm sitesi de aynı tabana oturdu.** `contextOverheadTokens` artık
  `history[base:]` sayıyor; `buildFillers`'ın `retainSteps bool` parametresi
  `stepsFrom int` oldu (negatif = soğuk thread, iz sayılmaz). Böylece gate ile
  panelin tek sayı görme sözleşmesi auto-compact sonrasında da korunuyor.
- **Otomatik yol bağlandı.** `cliCompactionBoundary(steps, rawLen)` turun kendi
  izinde tamamlanmış bir CLI-native compaction arar; bulursa `chat_stream`
  sınırı `len(rawHistory)` yapar (`+1` değil: compaction bu turun yanıtından
  **önce** oldu, yanıtın kendi izi hâlâ sıcak). Ortak yüklem
  `isCompletedNativeCompaction(kind, source, running)` — açık `/compact` yolu
  (`nativeCompactSession`) da aynı yüklemi kullanıyor, ikisi ayrışamaz.
- **Açık yol da sınırı yazıyor.** `nativeCompactSession` `SetSessionCLIResume`'un
  yanında `SetSessionCLICompactBoundary(..., len(history)+2)` çağırıyor; hata
  yutulmuyor, `summaryResult{}, error` olarak dönüyor.

**codex-cli durumu:** sinyal **var**. `feedContextCompaction`
(`internal/providers/codexcli_events.go:351`) normal tur akışında
`Kind:"compaction", Source:"cli-native", Trigger:"auto"` üretiyor, yani yukarıdaki
sağlayıcıdan bağımsız yüklem codex'i de kapsıyor. Manuel yol (`CompactNative`,
`Trigger:"manual"`) ayrı bir çağrıda koştuğu için tur akışına hiç düşmüyor —
tetik filtresine gerek yok.

Testler: `TestCLICompactionBoundaryDetectsCompletedAutoCompaction` (iki yönlü:
tamamlanmış claude/codex sinyali sınırı taşır; running compaction, sıradan tur ve
TionHarness fold adımı taşımaz), `TestSetSessionCLICompactBoundaryIsMonotonic`,
`TestWarmCLIStepBaselineFollowsCLICompaction`,
`TestBuildFillersDropsTraceBeforeCLICompactionBoundary`. Mevcut
`claudecli_compaction_test.go` ve `summary*_test.go` gevşetilmedi.
Doğrulama: `go build ./...` ok,
`go test ./internal/providers/... ./internal/api/... ./internal/db/... ./internal/conversation/... -count=1` ok,
`git diff --check` sıfır.

## Oturum listesi canlı filtreleri + CLI yeniden başlangıç ayracı (2026-08-31) ⏳

Panodan iki kart; ikisi de "olan biteni görünür kıl" ekseninde.

- **TSK513 — beşli oturum kategorisi.** `SessionsSidebar` kapsam çipleri
  `Worker` + `Arşiv` ikilisinden dörde çıktı: `Çalışan` (oturumun kendi turu akıyor)
  ve `Bekleyen` (kendisi boşta, altındaki en az bir doğrudan worker canlı)
  eklendi. Sınıflandırma `sessionKindMeta.ts` içindeki saf `sessionLiveScope()`
  fonksiyonunda; iki değer karşılıklı dışlayıcıdır (kendi turu da akan bir
  koordinatör `Çalışan` sayılır), böylece bir satırı görünür tutmak için iki çipin
  birden açık olması gerekmez. Sidebar'da tek bir `chipShapeOf()` hem çip
  sayaçlarını hem görünür listeyi besler — rozet sayısı ile filtrenin tuttuğu satır
  sayısı ayrışamaz. `sessionMatchesChips`'te canlı alanlar opsiyonel: runtime anlık
  görüntüsü olmayan çağıranlar eski davranışı korur. Detay: `_Docs\07-CHAT-UX.md`.
- **TSK514 — yeni CLI oturumu çizgisi.** Transkriptte `ColdCacheDivider`'ın yanına
  `CLIColdStartDivider` geldi: `--resume` etkinken sıcak thread taşınamadığında
  (oturumun ilk CLI turu, compaction fold'u, ya da saklanan thread id'sinin CLI
  home'unda bulunmaması) turun üstüne çizgi çizilir. Cache ayracı zaman
  damgasından türetilir, bu yeni ayraç ise backend atıflıdır
  (`db.Message.CLIColdStart` ← `cliResumePlan.coldStart`) ve ikisi aynı turda
  üst üste binebilir. `coldStart` `claudeResumeDecision` içinde **true başlar**,
  yalnız sıcak dalda temizlenir; ayrıca iki provider doğrulama yolu
  (`CanResume`, `CanResumeScoped`) kararı reddettiğinde yeniden `true` yapılır —
  sonradan eklenen bir soğuk yol sessizce ayracı kaçıramaz.

Doğrulama: `go test ./internal/api/... ./internal/db/... -count=1` ok, frontend
`npm test` 510+ test geçti (yeni: `sessionKindMeta.test.ts` canlı kapsam blokları,
`MessageList.test.tsx` ayraç blokları), `npx tsc --noEmit` ve
`npm run format:check` temiz, `git diff --check` sıfır.

Kartlar `review`'da; aynı turda `internal/agent` tarafında TSK505/TSK508 (worker
bildirim yolu) ayrı bir oturumda paralel sürüyor.

## TSK507 — Peer mesajları normal sohbete taşındı, worker yazılabilir oldu (2026-08-31) ✅

Ajanlar arası iletişimin arayüzdeki üç pürüzü giderildi:

- **`inbox` oturum türü kaldırıldı.** `send_message` teslimi artık alıcının ayrı,
  salt-okunur `📥 Inbox` oturumuna değil, **sıradan `chat` thread'ine** düşer
  (`agent/agentmsg.go`). Tür bir izin kapısıydı: yalnız
  `db.WritableSessionKinds` içindeki türler yeni kullanıcı turu kabul ettiğinden,
  kendi türüne sahip olmak thread'i salt-okunur yapan tek şeydi. Thread ajan
  başına tektir ve `GetOrCreateSourceSession(kind="chat",
  sourceID="agent-messages:<agentID>")` ile aranır — **`(agentID, kind)` ile
  değil**: `"chat"` insanın açtığı ad-hoc oturumların da türü olduğundan o arama
  kullanıcının kendi sohbetini bulup peer mesajlarını oraya sızdırırdı
  (`TestDeliverAgentMessage_DoesNotHijackExistingChat`).
- **Eski `inbox` oturumları göç ettirildi.** `db.migrateLegacyInboxSessions`
  açılışta (`Open`) eski build'lerin yazdığı `inbox` oturumlarını `chat`'e çevirir
  ve `PeerThreadSourceID(agentID)` damgalar. SourceId damgası şart: onsuz oturum
  teslim tarafındaki aramaya görünmez ve bir sonraki mesaj yanına ikinci bir
  thread açarak geçmişi ikiye bölerdi. Idempotent, oturum başına best-effort
  (tek bir yazılamayan header boot'u düşürmez), dolu `sourceId`'ye dokunmaz.
  Ortak sabit `db.PeerThreadSourceID` runtime ile migration'ın ayrışmasını
  engeller. Sidebar çipi + `graph.go` etiketi legacy olarak korunur.
  **Canlı doğrulandı:** gerçek WS27 kopyasında `converted=2` (2ms), ikinci boot
  no-op, `GetOrCreateSourceSession` göç eden SES122/SES123'ü buldu (yeni thread
  açmadı), mesaj geçmişi korundu. API matrisi: göç eden peer thread + `worker`
  → HTTP 200; `task`/`flow`/`flow-coordinator`/`insight` → HTTP 403.
  **Üretim store'unda koştu (2026-08-31 22:06).** 18 workspace tarandı, 9 legacy
  oturum çevrildi (WS1 `converted=4`, WS19/WS24/WS5 birer, WS27 `converted=2`);
  diskte kalan `inbox` yok, 9/9 `kind:"chat"` + `agent-messages:<agentID>`
  damgası ajanla eşleşiyor, `messages.jsonl` dosyalarına dokunulmadı (toplam 336
  mesaj, mtime'lar göç öncesi tarihte kaldı). Migration başlığı değiştirmez —
  göç eden thread'ler `📥 Inbox` adıyla görünmeye devam eder.
- **`worker` oturumları yazılabilir.** Worker transkripti bitmiş bir koşu kaydı
  değil, koordinatörün `SendToWorker` ile zaten içine tur enjekte ettiği canlı bir
  konuşmadır; izleyen insan da yanıtlayabilmeli, rotayı düzeltebilmeli.
  `writableSessionKindList` + frontend aynası `isWritableSessionKind` birlikte
  güncellendi. Çakışma `schedule` ile aynı şekilde sınırlı — insan turu ile
  enjekte tur aynı `turnqueue` slot'unu talep eder, sıraya girer.
  `task`/`flow`/`flow-coordinator`/`insight` **bilinçli olarak salt-okunur kaldı**:
  onlar gerçekten orkestratörün yazdığı koşu kayıtlarıdır ve yeni bir kullanıcı
  turunun bağlanacağı koşu yoktur. Testler:
  `TestEnqueueMessageAcceptedOnWorkerSession`,
  `TestWorkerKindIsWritableButNotImmutable`.
- **Ajanın yazdığı enjekte turlar artık atfediliyor.** Peer teslimindeki katılımcı
  damgası (`AuthorKind=agent`/`AuthorID`/`RecipientID`) ortak bir yardımcıya
  alındı (`Runtime.recordAgentAuthoredNote`) ve iki yeni yere bağlandı: spawn
  açılış promptu (yazar = `SpawnOptions.CreatedBy`) ve koordinatörün worker'a
  follow-up'ı (yazar = koordinatör oturumunun ajanı). Bu mesajlar artık
  `MessageList`'te `PeerTurn` (soldan gelen balon) olarak çizilir; öncesinde
  insanın kendi turu gibi görünüyorlardı. Damga **yalnız gerçek bir ajana çözülen**
  yazar için basılır: `CreatedBy` ajan olmayan köken de taşır (`"automation:<id>"`)
  ve frontend `authorId`'yi ajan listesinde arayıp isim yazdığından, çözülemeyen id
  isimsiz bir balon üretirdi.

## Haftalık özellik denetimi: teardown context sızıntısı ve refactor (2026-08-31) ✅

Hafta boyunca eklenen özelliklerin denetiminde bulunan ve düzeltilen üç konu:

- **Teardown context sızıntısı (`internal/api/chat_control.go`).** Her tur
  `register`'da `context.Background()` kökünden bir teardown context'i yaratıyordu,
  fakat `teardownCancel` yalnız silme yolunda (`cancelSessionCalls`) çağrılıyordu.
  Silinmeyen — yani turların ezici çoğunluğu — her tur, süreç ömrü boyunca canlı bir
  cancel func sızdırıyordu. `unregister` artık bekleyen bridge call yoksa context'i
  hemen serbest bırakıyor; bekleyen call varsa serbest bırakma son call'un
  `endCall`'ına erteleniyor. `cancelSessionCalls` ve `reopenSessionCalls` serbest
  bırakılmış (nil) cancel'a karşı güvenli. Regresyon:
  `TestUnregister_ReleasesTeardownContext`,
  `TestUnregister_DrainingRunReleasesTeardownContextOnLastCall`,
  `TestCancelSessionCalls_AfterRelease`.
- **Silinen oturumun MCP alt süreçleri kapatılmıyordu (canlı testte bulundu).**
  `applyMCPScratchpadRoot` (`internal/agent/mcp_playwright.go`) dosya yazan bir MCP
  sunucusunun cwd'sini oturumun scratchpad'ine ayarlar. Teardown'da bu scoped
  bağlantıları kapatan bir faz yoktu — yalnız 300 sn'lik idle reaper topluyordu.
  Windows canlı bir sürecin cwd'si olan dizini silmediği için oturum silme
  `unlinkat ...\scratchpad: Dosya başka bir işlem tarafından kullanıldığından`
  hatasıyla fail-closed düşüyordu (oturum korunuyordu — davranış doğruydu, eksik
  olan kapatma fazıydı).
  - `internal/mcp/pool.go` → `CloseSession(sessionID)`: yalnız o oturuma ait
    scoped bağlantıları kapatır; paylaşılan slot ve diğer oturumlar korunur.
    Eşleşme `"<sessionID>|"` sınırındadır (SES1 silinince SES11 kapanmaz), boş id
    hiçbir şey kapatmaz. Kapatma havuz kilidi dışında yapılır (`reapScoped` deseni).
  - `internal/agent/runtime.go` → `CloseSessionMCP` geçidi.
  - `internal/api/session_teardown.go` → Faz 7, worker durdurmadan **sonra**
    (worker turları o ana kadar MCP aracı çağırıyor olabilir). Tersinir değil ve
    kasten fatal değil: bağlantı talep üzerine yeniden kurulur.
  - Testler: `TestPoolCloseSessionClosesOnlyThatSession`,
    `TestPoolCloseSessionDoesNotMatchIDPrefix`, `TestPoolCloseSessionEmptyIsNoOp`.
- **Sessizce yutulan handoff bağlaması (`internal/agent/handoff.go`).**
  `SetSessionHandoffArtifact` hatası `_ =` ile yutuluyordu; başarısızlıkta handoff
  artifact'ı yazılmış ama oturumun handoff'u olarak bulunamaz hâlde kalıyordu.
  Artık hata sarmalanıp döndürülüyor.
- **Refactor — büyük dosyaların bölünmesi.** Davranış değişikliği yok; yalnız
  dosya sınırları. Her parça kendi kaygısını taşıyan ayrı bir dosyaya alındı,
  testler yerinde bırakıldı:

  | Önce | Sonra |
  |------|-------|
  | `internal/agent/coordination.go` 2303 | 2167 + `coordination_teardown.go` 178 |
  | `internal/providers/claudecli.go` 1776 | 971 + `claudecli_stream.go` 819 |
  | `internal/agent/runtime.go` 1720 | 1174 + `runtime_skills.go` 243 + `runtime_prompt.go` 336 |

  - `coordination_teardown.go` — tersinir worker teardown (`StopWorkerForTeardown`,
    `FinishWorkerTeardown`, `markTreeTeardown`, `clearTreeTeardown`,
    `spawnBlockedByTreeTeardown`). `coordination.go` yalnız spawn/queue/notify
    akışında kaldı. Testleri: `coordination_tree_test.go`, `worker_queue_test.go`.
  - `claudecli_stream.go` — `cliStreamParser` ve stream-json olay tüketimi
    (trace adımları, usage, native-compaction yaşam döngüsü, rate-limit/auth
    sınıflandırması). `claudecli.go` alt süreç yönetiminde kaldı.
  - `runtime_skills.go` — skill kataloğu prompt bloğu ve `agentSkillLib` /
    `agentSkillWriter` sink'leri. `runtime_prompt.go` — persona birleştirme
    (`BuildSystemPrompt`) ve context blokları (environment, tarih/saat, shell
    araçları, workdir confine) ile otonom tur varyantları.

Doğrulama: `go build ./...`, `go vet ./...`, `go test ./... -count=1` (39 paket, 0
hata), `npm test` (71 dosya / 510 test), `npx tsc --noEmit`, `git diff --check`.
`ArtifactPreviewModal.test.tsx`'teki `act(...)` sarmalanmamış state güncellemesi
uyarısı da giderildi.

## TSK500 — Dosya seçicisinde görsel anotasyon kuyruğu (2026-08-31)

- Dosya seçicisindeki destekli görseller, clipboard ile aynı doğrulama sınırlarından geçer ve
  FIFO kuyruğuyla tek tek otomatik `ImageAnnotator` içinde açılır. Görsel olmayan dosyalar mevcut
  normal upload yolunu kullanır; doğrulama hataları composer hata yüzeyinde görünür.
- Kaydetme ve iptal, aktif bitmap kaynağını tek sefer kapatıp sıradaki görsele geçer. Aktif kayıt
  invariant'ı eksikse işlem sessizce yutulmaz.
- Annotator erişilebilir kalem boyutu kontrolü kazandı. Kaynak ölçeğine bağlı varsayılan korunur;
  seçim yalnız yeni stroke'lara uygulanır. Yalnız çizim viewport'u şeffaftır; modal, toolbar,
  kaynak görsel çizimi ve export davranışı korunur.
- TSK474/TSK475/TSK476 kapsamları değiştirilmedi; TSK500 yalnız dosya seçicisi entegrasyonu,
  kalem boyutu ve viewport görünümünü kapsar.

## Worktree git hata çıktısı ve log buffer sınırları (2026-08-30) ✅

Worktree git komutlarının sınırsız `CombinedOutput` hata zincirine, DB'deki
`WorktreeLastError` alanına ve loglara taşınması engellendi. Git hata çıktısı 8 KiB
bütçede anlamlı başlangıç + mümkünse son satırı korur; kesilen bayt sayısını yazar.
Ek savunma olarak log ring buffer message ve attr string değerlerini 16 KiB ile sınırlar.
CRLF uyarı seli, kalıcı lifecycle hatası ve büyük log alanları regresyon testleriyle kapsandı.

## Model etiketlerinden "Varsayılan" kalktı (2026-08-29) ✅

UI artık boş model id'si için "Varsayılan" yazmıyor; ne çalıştığını söylüyor.

- **Katalog etiketleri.** claude-cli ve codex-cli'nin boş-ID model girdisi
  `Varsayılan` yerine `claude oturum modeli` / `codex oturum modeli` etiketiyle
  geliyor (`internal/providers/kind_claudecli.go`, `kind_codexcli.go`). Boş model
  bir "varsayılan model" değildir: seçim CLI'nin kendi oturumuna bırakılır.
- **App-global çözümleme.** Öğrenilen istek-model → gerçekte servis edilen model
  eşlemesi workspace store'unun yanında uygulama veri kökünde de tutuluyor
  (`internal/db/store_model_resolution_global.go`). Katalog, workspace değeri boş
  olduğunda buna düşer → **yeni bir workspace ilk turunu koşturmadan önce de**
  somut model adını gösterebiliyor. Ayrıntı: `_Docs/08-DEPOLAMA.md`.
- **Frontend.** `modelDisplayName` boş id için `(model belirtilmemiş)` döner;
  `modelLabel` boş modelde önce katalogun boş-ID etiketini, sonra `resolvedModel`
  değerini kullanır (`frontend/src/shared/lib/modelLabel.ts` +2 test). Aynı dil
  `PackPreview.tsx` ("Sağlayıcı modeli", "sağlayıcı belirtilmemiş") ve
  `ProviderInstanceForm.tsx` placeholder'ında da kullanılıyor.
- **Sistem ajanları artık açık sağlayıcı ile tohumlanıyor** — bkz.
  `_Docs/74-SISTEM-AJANLARI.md`.

## Başlık için ayrı sağlayıcı/model seçimi kaldırıldı (2026-08-28) ✅

Otomatik başlık üretimi artık **yalnız** `titler` sistem ajanı üzerinden çalışıyor
(`resolveTitleConfig`); prompt ve model o ajandan gelir. `titleModel` /
`titleProviderId` ayarları, karşılık gelen `Tunables` alanları
(`SetTitleModel`/`TitleModel`/`SetTitleProviderID`/`TitleProviderID`), API'deki
uygulama çağrıları ve ayarlar ekranındaki sağlayıcı+model alanları silindi — geriye
tek bir `autoTitleEnabled` anahtarı kaldı. Aynı override'ı ucuz-model seçici olarak
ödünç alan iki yer de (`resolveCompactorConfig` fallback'i,
`judgeCoordinatorStalled`) artık çağıran ajanın kendi modelini kullanıyor.

## Marka ikonları simple-icons'a taşındı (2026-08-28) ✅

UI build'i `AboutPanel.tsx(2,20): error TS2305: Module 'lucide-react' has no
exported member 'Github'` ile kırıldı: lucide-react v1 marka ikonlarını kaldırdı.
`simple-icons` zaten bağımlılık ve zaten projenin marka-glyph kaynağı
(`shared/lib/programIcons.ts`), ama **path verisi** yayınlıyor, React bileşeni değil
— yani lucide arayüzü bekleyen bir ikon yuvasına doğrudan takılamıyordu. Araya
`shared/components/BrandIcon.tsx` adaptörü kondu: lucide'ın `size` prop'unu
karşılar, varsayılan olarak `currentColor` ile boyar (link/buton hover durumları ve
light/dark temalar çalışsın diye; marka rengi isteyen için `brandColor`). Böylece her
ikon yuvası marka ikonu alabilir, çağıran başına elde `<svg>` yazılmaz.
Kalan `lucide-react` importları tarandı — başka marka ikonu kullanan yer yok.

## Güvenlik sertleştirme: sandbox sınırı, alt süreç ortamı, gizli konsol (2026-08-28) ✅

Sandbox artık NT namespace (`\\?\`, `\\.\`) ve ADS (`dosya:stream`) yazımlarını
açıkça reddediyor (`internal/tools/sandbox.go`, `5f7db6b1`), ve kök yolu bir kez
`EvalSymlinks`'ten geçiriliyor: `Root` yalnız sözdizimsel `Abs+Clean` olduğu için
kökün kendisi symlink olduğunda kök-içi bir dosya gerçek mutlak adıyla verilince
`underRoot` yanlışlıkla reddediyordu; çözüm `internal/api.underDir` ile de aynı
sınırı paylaşıyor. `EvalSymlinks` hatasında sözdizimsel yol korunuyor (henüz var
olmayan çalışma dizini meşru); Windows'ta junction'ları izlemediği ölçülüp yeni
junction testine yazıldı (`9fb6d3bf`).

Alt süreç ortamından kimlik bilgileri soyuluyor: shell aracı (`849257c7`,
`internal/tools/builtin_shell.go` + ayar/hook yüzeyi) ve CLI sağlayıcı süreçleri
(`c4b240db`, yeni `internal/providers/cli_env.go` + testi) yalnız kendi
kimlik-doğrulama değişkenlerini taşıyor. `codexBaseEnv`'in neden iç içe geçme
filtresi taşımadığı yorumla açıklandı (`d7b32df8`).

İç içe CLI torunları artık görünür konsol açmıyor (`e5ca55db`,
`internal/proc/*` + `claudecli.go`/`codexcli.go`). Dokümanda iki düzeltme:
sınırlandırılmış (confined) turların shell'i yol bazında kısıtlamadığı yazıldı
(`08496bb8`, `_Docs/26-CALISMA-DIZINI.md`) ve transform verisinin sınırlandırılması
netleştirildi (`5dda71fd`).

## Araç katmanı: bundle/tier tabloları, kategori sözleşmesi, şema maliyeti (2026-08-28) ✅

Varsayılan araç görünürlüğü elle yazılmış listelerden veri katmanına taşındı. Önce
parite çıpası olarak her aracın etkin tier'ını dondurun golden tablo eklendi
(`983dda1f`), sonra paylaşılan bundle soyutlaması + `TierDefaults`/`DefaultTiers`/
`TierFor` tabloları geldi (`e9915668`), en son `buildRegistry` içindeki
`MarkNameOnly` (25 ad) ve `MarkHidden` (4 ad) blokları silinip yerlerine
`ApplyToolDefaults` / `ApplyBundleDefaults` çağrıları kondu; `AttachMCP` name-only
tier'ı artık sabit yerine `mcp:*` bundle satırından okuyor (`4aca9372`). Golden
tablo tek satır değişmeden yeşil kaldı.

`builtinCategory` tek doğruluk kaynağı olduğunu iddia ediyordu ama 18 yerleşik araç
(koordinatör/worker ailesi, otomasyon CRUD, insight üçlüsü, lessons ikilisi,
`apply_patch`, `run_code`, `render_template`) haritada yoktu ve sessizce "other"a
düşüyordu. Hepsi mevcut kategorilere eklendi ve haritadan değil paketin kendi
`Def()` gövdelerinden AST taramasıyla türeyen bir regresyon testiyle kapatıldı
(`32f72ea6`); tarama sessizce skip'e düşmek yerine yüksek sesle fail ediyor
(`ea800320`). Rune-sınırı kırpma testleri de boş-geçer olmaktan çıkarıldı
(`e9ad68da`).

Aşırı büyük araç çıktısı artık kırpılıp kaybolmuyor: `capToolOutputOffload` tam
çıktıyı metin artifact'ine yazıyor, modele head + tail + artifact handle gidiyor
(`67b72c18`); çıktı üst sınırının gövdeyi sınırladığı, işaretçiyi sınırlamadığı
dokümana yazıldı (`c380c590`). Yeni `tools.TierTokens`/`FullSchemaTokens`/
`CurrentTokens` yardımcıları bir araç grubunun şema token maliyetini tahmin ediyor
ve UI'da grup satırında gösteriliyor (`ed08f968`). Özetlenmiş araç kataloğunda
`ToolSearch` için `select:` sözdizimi açıkça yazıldı (`bcf85ad0`).

## Bağlam bütçesi, otonom turlar ve codex-cli düzeltmeleri (2026-08-28) ✅

`EffectiveBudget` `settings.MaxContextTokens` değerini üst sınırsız bir taban gibi
kullanıyordu: varsayılan 800000, hem auto tavanını (262144) hem de modelin gerçek
penceresini eziyordu — 200K'lık bir modele 800K transkript tutması söyleniyor,
compaction buna asla ulaşamıyor ve tur bağlam-taşması kurtarma yolunda ölüyordu.
Sonuç artık gerçek pencerenin %80'iyle sınırlanıyor (`dcbba21f`).

Koordinatör stall guard'ı taze bir worker notunda tamamen devre dışı kalıyordu —
tam da phantom spawn'ların en sık olduğu turda. Muafiyet asimetrik yapıldı: judge
ve düzeltici dürtme her zaman koşuyor, yalnız halt eskalasyonu bastırılıyor;
guard'ın kendi yazdığı notlar ve `<coordination-status>` artık worker sonucu
sayılmıyor (`91f70e58`). Guard ayrıca yalnız runtime tetikli koordinatör
turlarında değil, kullanıcı sohbet turlarında da koşuyor ve sweeper hiç worker
doğurmamış bir koordinatörü de aday sayıyor. Otonom (spawn/scheduler/flow) turları `steerable` alanını
ve oturum grant'lerini kaydediyor, böylece ask/read-only modda bir claude-cli
otonom turuna "Yönlendir" reddedilmiyor (`602c1578`).

codex-cli'nin kendi çoklu-ajan collab araçları kapatılmaya çalışıldı (`[features]
multi_agent=false, multi_agent_v2=false`) — model `spawn_worker` yerine onları
seçip `--ephemeral` yüzünden thread store'u olmadığından hata alıyordu
(`4b60540b`). **Bu anahtarlar 2026-08-31'de etkisiz ölçüldü**: codex 0.148.0'da
davranışı değiştiren tek anahtar `[agents] enabled = false`, ve araç açıkken
kırık spawn modelin sonucu uydurmasına yol açıyor — ayrıntı ve ölçüm yöntemi
`_Docs/47-KOORDINATOR-COKLU-AJAN.md` §18. Buna karşılık `WebSearch`/`WebFetch` yerleşikleri interaction MCP
köprüsünden codex-cli ajanlarına açıldı (`384ca6c5`) ve ajan başına "sağlayıcının
native web araması" anahtarı eklendi (model, API, market pack, şablon ve ajan
ayar formu; `2f8c5731`).

## View sadeleştirmesi ve sohbet/panel arayüzü (2026-08-28) ✅

Token ve maliyet raporlaması yalnız budget view'ında kaldı: ajan, oturum ve
workspace başlıklarından harcama düşürüldü (`0f354381`), geride kalan bayat
doküman/yorum iddiaları temizlendi (`693996fb`, `3f3cb70c`).

Sohbet tarafında: girdisinde komut/yol/mesaj taşımayan araçlar (`get_view`,
`expand`, `read_artifact`, `list_agents`, `list_workers`,
`set_coordinator_mode`) için adım kartı özeti artık boş kalmıyor — kimlik satırı
veya en fazla iki skaler `key=value` gösteriliyor (`7e1866dc`); tekil view'larda
`board · board` gibi tekrarlar tek başlığa çöküyor (`c1e6b8a2`); gelen peer/inbox
balonları kendi tema renk token'larını aldı (WCAG AA kontrast testiyle,
`1614b878`). Insight bulgular kanban'ı `min-h-0` zinciriyle panel içinde kalıyor
ve lens checkbox'ı switch oldu (`437f5109`). Yazılabilir oturum-türü kontrolü
`shared/lib/sessionKind` modülüne taşındı, salt-okunur rozeti `kind === 'insight'`
yerine bu kontrolden türeyerek inbox dahil her salt-okunur türde görünüyor
(`cdaea59b`). Vite build'i artık `dist/.gitkeep`'i silmiyor (`69eb60ec`).

## Tanıtım sitesi: /docs bölümü ve release-host www kökü (2026-08-28) ✅

Siteye `src/content/docs/**/*.md` üzerinden bir docs koleksiyonu eklendi: kenar
çubuğu, breadcrumb, h2/h3 içindekiler, önceki/sonraki gezinme, üç grupta
(getting-started/concepts/reference) yedi placeholder sayfa ve `/docs` iniş
sayfası; `docsUrl` artık `null` değil, uygulamadaki "Docs" bağlantısı canlı
(`8d0d2786`). `sync-www.sh` `website/dist`'i release-host'un `/srv/www` köküne
kopyalıyor — 8081 portu boş kök yüzünden 404 dönüyordu. `RELEASE_WWW_PATH`
bilinçli olarak `RELEASE_PATH`'ten ayrı: aynı olsaydı `rsync --delete`
yayımlanmış release'leri silerdi (`0196691b`).

**Devam eden (henüz commit'lenmedi):** dashboard commit heatmap'i
(`frontend/src/features/dashboard/CommitHeatmap.tsx`,
`internal/api/dashboard_commit_activity.go`) ve insight ajan seçimi
(`frontend/src/features/insight/insightAgentSelection.ts`) ile view DSL
sadeleştirmesi çalışılıyor.

## Public yayın (2026-08-27) ✅

Proje **public** oldu: `https://github.com/bilal-arikan/tionharness` (tek branch
`main`, 984 commit), repo açıklaması + 12 topic + `homepage: https://tionharness.com`
dolduruldu. Tanıtım sitesi GitHub Pages üzerinden canlıya alındı
(`.github/workflows/pages.yml`), custom domain + zorunlu HTTPS onaylı.

**VPS deploy hattı tamamen kaldırıldı:** `.gitea/workflows/deploy.yml`,
`deploy/README.md`, `deploy/deploy.sh`, `deploy/tionharness.service` silindi.
`deploy/release-host/` kaldı ama artık yalnız yerel Docker önizlemesi, üretimde
kullanılmıyor. Yayın hattının sahibi `.github/workflows/` (release + Pages);
`.gitea/workflows/ci.yml` + `release.yml` yalnız doğrulama yapar. Detay
`_Docs/75-YAYIN-SURECI.md` (zaten güncel).

**Apache-2.0 lisansı eklendi:** kökte `LICENSE`, `NOTICE`, `THIRD-PARTY-NOTICES.md`;
`website/src/site.config.ts`'te `license: 'Apache-2.0'` (eskiden `null` + TODO).

**Doküman sanitizasyonu (public yayın için):** `claude-code-audit` deposuna
atıflar kaldırıldı → "Claude Code'un gözlemlenen davranışı" gibi ifadelerle
değiştirildi; `mcp-chrome` → `browser-mcp`, `heimdall` → `mcp-alpha` olarak
yeniden adlandırıldı (dokümanlar + `internal/db/store_mcp_test.go` fixture'ları);
kişisel MCP envanteri/port listeleri `_Docs/52-MCP-GATEWAY.md`'den çıkarıldı;
yerel disk yolları placeholder'landı; `internal/workspace/defaults/default-instructions.md`'den
"Document tools" tablosu kaldırıldı.

**Yeni araç:** `gitleaks` 8.30.1 kuruldu (`<progs>/gitleaks/gitleaks.exe`, PATH'te
değil) — commit öncesi sır taraması için (bkz. `tionharness-commit` skill'i).

**Git temizliği:** tüm `task/*` worktree branch'leri ve `main-rename-v1` silindi;
commit mesajlarındaki `Co-Authored-By` trailer'ları (External Agent/Claude/TionSwarm/
TionHarness botları) geçmişten temizlendi.

Bu oturumda ayrıca `tionharness-project` (public/lisans/yayın bilgisi) ve
`tionharness-commit` (gitleaks adımı) skill'leri güncellendi.

## Sistem ajanlarına kanonik görsel kimlik + varsayılan ajan kısıtı (2026-08-27) ✅

11 yerleşik ajanın avatarı ve rengi 14 workspace'in hepsinde boştu, yani roster'da
varsayılan/rastgele görünüyorlardı. Artık `internal/agent/systemagents.go` içindeki
tek `systemAgentVisuals` tablosu her `SystemKey` için kanonik emoji + hex taşıyor
(analiz ajanları mor/kurşuni, salt-okunur worker'lar turkuaz/mavi, yazan worker'lar
kehribar/kiremit) ve `SystemAgentDefinition` bunları `Avatar`/`Color` alanlarıyla
persistence katmanına aktarıyor. `EnsureSystemAgents` bu iki alanı **yalnız boşsa**
dolduruyor — mevcut workspace'ler boot'ta düzeliyor, kullanıcının seçtiği değer
ezilmiyor. `POST /api/agents/{id}/restore-default` avatar/rengi de geri yüklüyor.

İkinci kural: sistem ajanı artık workspace'in `defaultAgentId` değeri olamıyor.
Kapı `Manager.UpdateSettings` içinde (tek yazma yolu, HTTP dahil her çağıranı
kapsar), HTTP tarafında 400'e eşleniyor; boot'ta `sanitizeDefaultAgent` eski bozuk
bir değeri temizleyip `Warn` logluyor (tarama sonucu: 14 workspace'in hiçbirinde
böyle bir değer yoktu). UI'da sistem ajanı için "Varsayılan yap" düğmesi hiç render
edilmiyor. Sistem ajanının kendi işlevi (titler/compaction/worker) etkilenmiyor.
Detay: `_Docs/74-SISTEM-AJANLARI.md`.

## MCP sunucu stderr'i artık yutulmuyor (2026-08-27) ✅

`DialStdio` çocuk sürecin stderr'ini `io.Discard`'a veriyordu ("chatty server pipe'ı
doldurup bloke etmesin" gerekçesiyle). Endişe haklı ama bedeli ağır: stdio MCP
sunucusunun kendini açıklayabildiği **tek kanal** buydu. 2026-08-27'de
`codebase-memory-mcp` her yeni istemciyi _"CBM daemon is active or starting but could
not accept this client within 30000 ms"_ diyerek reddedip çıktı; logda görünen tek
şey `mcp initialize: mcp read: EOF` oldu — "bir pipe kapandı" der, sebebini demez.
WS5'teki bütün ajan turları ilk LLM çağrısına gelmeden durdu ve log 32 saniyede bir
aynı boş uyarıyı tekrarladı.

Çözüm `internal/mcp/stderrtail.go`: sınırlı (8 KB), asla bloklamayan, asla büyümeyen
halka tampon. Doluysa en eskiyi atar, kapasiteyi aşan tek yazımda **sonu** saklar
(hata mesajı oradadır) ve her zaman tam uzunluğu "yazıldı" olarak raporlar — kısa
write bildirmek `os/exec`'in kopyalayıcısını durdurup tam da kaçınılan bloklamayı
geri getirirdi. Handshake başarısız olursa tampondaki eksiksiz stderr hataya iliştirilir
(`... (server stderr: ...)`). Close önce çağrılır
ki ölmekte olan sunucunun son satırı tampona yetişsin. Testler
`stderrtail_test.go`; düzeltme kaldırılınca regresyon testi tam olarak eski
`mcp initialize: mcp read: EOF` metniyle kırılıyor (doğrulandı).

Tail okuması `Close()`'un 2 sn'lik bekleme tavanına bağlıydı ve yüklü makinede
(tam test paketi + ajan build'leri koşarken) kaçırılabiliyordu — `waitNonEmpty` ile
sınırlı ve deterministik hale getirildi. `os/exec` stderr'i kendi goroutine'inde
kopyalar ve bunun bittiğini yalnız `Wait` döndüğünde garanti eder; bu bekleme
yalnızca hata yolunda çalışır.

## stdio package-runner başlangıçları koordine ediliyor (2026-08-30) ✅

`bunx`, `npx` ve `npm exec` aynı npm/bun paket önbelleğine eşzamanlı yazarken Windows'ta
`EBUSY`, `failed copying files from cache` ve `could not determine executable to run`
hataları üretebiliyordu. `internal/mcp/package_runner.go`, sürüm/tag ve harf farkını
ayıklayan normalize paket kimliğiyle yalnız aynı paketin **dial + initialize** penceresini
tekilleştiriyor. Farklı paketler paralel kalıyor; kilit canlı MCP sürecinin ömrü boyunca
tutulmuyor. Başlangıç hatası en fazla bir kez yeniden deneniyor; bilinen geçici desenlerde
context-aware kısa backoff/jitter uygulanıyor, `context.Canceled` hiç retry edilmiyor.
Kalıcı hata son denemenin sınırlı stderr tamponunu eksiksiz koruyor.

Playwright'ın `(session, agent)` scope'u, oturum scratchpad `cwd`'si ve `--output-dir`
izolasyonu değişmedi. Testler aynı paketin farklı scope kataloglarında azami launcher
eşzamanlılığını 1'e sabitliyor; farklı paket paralelliğini, transient başarıyı, kalıcı
hata retry sınırını ve canceled yolunu doğruluyor (`package_runner_test.go`).

## MCP katalog arızası WARN'da boğulmuyor (2026-08-27) ✅

Aynı sunucu için **ardışık** katalog hatası sayılıyor (`internal/agent/mcpescalate.go`,
`Runtime.mcpFailStreaks`); eşiği (3) geçen ilk hata tek seferlik **ERROR** basar,
sonrakiler yine WARN kalır ve başarılı bir katalog sayacı sıfırlar. Neden: 2026-08-27'de
`codebase-memory-mcp` ~9 saat boyunca her istemciyi reddetti; log 32 saniyede bir
aynı WARN satırını yazdı, hiçbir şey yükselmedi ve **hiçbir ajanın tur açamadığı** bir
workspace, log seviyesine bakıldığında normal işleyişten ayırt edilemedi. Eşik
"mevcut kesinti"yi ölçer, ömür boyu toplamı değil — böylece kısa kesintide gürültü
olmaz, ama tekrar tekrar düşen sunucu her kesintide yeniden yükselir. Testler
`mcpescalate_test.go` (tek seferlik geçiş, kurtarma sonrası yeniden geçiş,
sunucu-başına izolasyon).

**Kapsam notu:** native tool loop katalog arızasını zaten `StepRecovery` kartı olarak
gösteriyor (`toolloop.go` → `mcpnotice.go`). **claude-cli yolu (`BridgeTools`)
collector'ı bağlamıyor**, dolayısıyla o yolda hâlâ yalnız log var — olayın yaşandığı
yol da buydu. ERROR eskalasyonu her iki yolu da kapsar; CLI kartı açık iş.

## Kendi reposunu build eden ajan döngüsü kesildi (2026-08-27) ✅

Dev ortamı tekrar tekrar kapanıyordu. Zincir: backend boot'ta WS5'teki üç öksüz
koordinatörü (`SES1543/1545/1558`, cwd = TionHarness reposu) yeniden kuyruğa alıyor →
bunlar `cd frontend && npm run build` yapan worker'lar spawn ediyor → `dev.ps1`
temizliği backend **ağacını** `taskkill /T /F` ile öldürünce ajanın `npm`'i paket
açarken kesiliyor → `@rolldown/pluginutils/dist/index.mjs` gibi dosyalar hiç
oluşmuyor → Vite `ERR_MODULE_NOT_FOUND` ile ölüyor → script her şeyi kapatıyor →
başa dön. Yani uygulama, kendisini çalıştıran dev sunucusunun repo'sunu build
ediyordu.

Çözüm: `node_modules` sıfırdan `npm ci` ile kuruldu ve o koordinatör ağacının 28
oturumu (3 koordinatör + 25 worker) `state="archived"` yapıldı —
`RecoverOrphanedTurns` arşivli oturumu diriltmiyor. Doğrulandı: yeniden başlatmada
`orphaned coordinator re-enqueued` satırı yok, Vite temiz kalkıyor. Yedek:
`<store>\_archive-backup-20260827\`.

**Kalıcı düzeltme:** `enqueueCoordinatorTurn` artık uyandırma girişinde arşiv
kontrolü yapıyor — arşiv, otomatik turlar için `stallHalted` gibi sert bir dur.
Önceden yalnız `RecoverOrphanedTurns` bakıyordu, bu yüzden koordinatörü arşivlemek
yetmiyordu: kurtarılan her worker `NotifyCoordinator` çağırıp arşivli oturumda
yeni bir drain başlatıyordu (bu olayda alt ağacın tamamı elle arşivlenmek zorunda
kaldı). Not zaten kalıcı yazıldığı için hiçbir şey kaybolmuyor; arşivden çıkarınca
bir sonraki bildirim onu işliyor. Slot'a dokunulmadan dönülüyor, yani arşivden
çıkan oturum `driving` takılı kalmıyor. Regresyon: `coordination_archived_test.go`
(guard kaldırılınca kırmızı olduğu doğrulandı). Detay:
[47](47-KOORDINATOR-COKLU-AJAN.md), [58](58-QUEUE-SENKRON.md).

## dev.ps1 frontend ön-kontrolü vite'ı gerçekten yüklüyor (2026-08-27) ✅

Ön-kontrol artık dosya varlığına değil **`Test-FrontendDeps`**'e dayanıyor: `.bin\vite.cmd`
var mı + `node -e "import('vite')"` sıfırla çıkıyor mu. Dosya bakmak yetmiyordu, çünkü
aynı sabah ağaç iki farklı şekilde bozuldu ve ikisi de `Test-Path`'e görünmüyor:
(1) `.bin\` kayboldu → npm `vite`'ı PATH'ten çözüp alakasız bir Python static-site
generator'ı çalıştırdı (exit 2); (2) bir ajanın `npm install`'u paket açarken öldü →
`@rolldown/pluginutils` package.json'lı ama `dist\index.mjs`'siz kaldı →
`ERR_MODULE_NOT_FOUND`. Ölçüldü: bozuk ağaçta `vite --version` hâlâ **0 ile çıkıyor**
(eksik modülü hiç import etmiyor), yani ikinci vakayı yalnız gerçek bir import yakalar.

Onarım kademeli: `npm install` → hâlâ bozuksa `node_modules` silinip `npm ci`
(lockfile yoksa `npm install`) → hâlâ bozuksa açık hata. Kademe şart, çünkü
**`npm install` yarım açılmış paketi onaramaz** — package.json orada olduğu için npm
paketi kurulu sayıp atlar; ilk guard'ın çöküş döngüsünü kıramama sebebi buydu.
Canlı testte üç basamak da sırayla çalıştı. Ayrıca exit 2 açıklaması iki çocuğu da
anıyor (backend → Go panic/OOM, frontend → npm/vite başlatma hatası).

## Sohbet listesi kategori çiplerinde Ctrl/Shift tıklama (2026-08-27) ✅

Sidebar'daki oturum türü çipleri artık modifier tuşlarını anlıyor: düz tıklama tek
çipi açıp kapatıyor, **Ctrl/Cmd + tıklama** yalnız tıklanan çipi seçili bırakıyor
(solo), **Shift + tıklama** ise tıklanan çipi olduğu gibi bırakıp diğerlerinin
seçimini tersine çeviriyor. Mantık `sessionKindMeta.ts` içindeki saf
`nextChipsOff(chipsOff, key, mode)` fonksiyonunda; seçim yine `chipsOff` olarak
localStorage'da saklanıyor. Testler: `sessionChipClick.test.ts`.

## Blank workspace CEO + PM kontrol döngüsüyle açılıyor (2026-08-26) ✅

`workspace-blank` artık tek genel "Asistan" yerine iki ajan tohumluyor: salt-okuma,
mesajlaşma ve delegasyon araçlarıyla sınırlı **CEO** yalnız tetikleyici/gözlemci;
tam araç erişimli **PM** ise kart yönetimi, delegasyon, insight taraması ve
prompt/skill/otomasyon/akış optimizasyonunun yürütücüsü. CEO'nun etkin `*/20 * * * *`
zamanlaması kalıcı `kind="schedule"` oturumunda board'u denetleyip duran, başarısız
veya ilerlemeyen işte PM'i dürtüyor; işi kendisi yapmıyor. Kart `failed` ya da
`review` durumuna taşındığında etkin iki pano otomasyonu PM'in kalıcı
`sessionMode="continue"` oturumunu tetikliyor.

Workspace schedule/automation şablonlarına geriye uyumlu `enabled` alanı eklendi:
alan verilmezse eski pasif varsayılan korunuyor, açıkça `true` veren paket davranışı
hemen çalıştırabiliyor. Kurulum bu değeri DB'ye aktarıyor; `agentKey`/`flowName`
referansları seed edilen gerçek kimliklere çözülüyor. `templates_test.go`, blank
paketin CEO araç sınırını, PM'i, 20 dakikalık etkin schedule'ı ve iki etkin pano
otomasyonunu regresyon testiyle kilitliyor. Detay: [06](06-WORKSPACES.md),
[21](21-MARKET.md), [46](46-ETIKET-OTOMASYON.md).

## Stuck schedule tekrar döngüsü engellendi (2026-08-25) ✅

Prompt tabanlı scheduler, ortak `schedule` oturumu stuck eşiğine ulaşmışsa yeni
prompt yazmadan ilgili zamanlamayı otomatik pasifleştiriyor ve cron tablosundan
düşürüyor. Böylece aynı guard reddi, bildirim ve transkript kaydı her tick'te
tekrarlanmıyor; `StuckTurns` sabit kalıyor. Regresyon testi transcriptin büyümediğini,
sayacın değişmediğini ve schedule'ın pasifleştiğini doğruluyor. Detay: [20](20-SCHEDULE-WAKE.md).

## UI lokalizasyon altyapısı eklendi (2026-08-25) ✅

Arayüz dili artık birinci-sınıf bir ayar. Detay: [73](73-LOKALIZASYON.md).

**Eksen ayrımı (en kritik karar):** `Settings.Language` = **ajan yanıt dili** (anlamı
değişmedi), yeni `Settings.UILanguage` = **arayüz dili**; `""` → arayüz ajan dilini izler,
yani mevcut kurulumların gördüğü dil değişmedi. Ayrı olmalarının nedeni kozmetik değil:
`Language` değişimi prompt epoch'unu yeniden dondurup cache'i soğutuyor, arayüz dili ise
hiçbir prompt'a dokunmuyor. Ayarlar → Profil'de ikisi yan yana duruyor.

**Frontend:** `i18next` + `react-i18next`; kataloglar `src/i18n/locales/<dil>/<namespace>.json`
(namespace = feature klasörü), `import.meta.glob` ile eager toplanıyor. Dil değişimi
`I18nRoot` ile ağacı yeniden bağlıyor — çünkü metnin büyük kısmı `useTranslation` dışından
(formatter'lar, comparator'lar, `useMemo`) üretiliyor ve aksi hâlde ekran yarı çevrili kalıyor.
Backend ayarı SSE `settings` olayıyla geldiği için diğer pencereler de canlı geçiyor;
ilk kareyi boyamak için `localStorage` aynası var.

**Gerçek locale işi (kelime çevirisi değil):** 60+ hardcoded `'tr-TR'` / `localeCompare(…,'tr')`
çağrısı `shared/lib/intl.ts` üzerinden Intl'e taşındı (denetlenebilir codemod +
`SKIP` listesi: URL parametre sırası ve BCP-47 kod sıralaması bilerek locale-bağımsız kaldı).
`format.ts` (`usd`/`count`/`decimal`/`percent`) ve `time.ts` (göreli zaman, süre, kova
başlıkları) locale-duyarlı hâle geldi.

**Yol boyunca düzelen hatalar:** (1) `<html lang>` yazılmadığı için CSS `text-transform:
uppercase` Türkçe i/İ kuralını uygulamıyordu (59 dosya etkileniyordu) — artık doğru.
(2) Modül seviyesindeki `BUCKET_LABELS` sabiti ilk yüklemedeki dilde donuyordu → fonksiyona
çevrildi. (3) i18next'te `count` geçilen anahtar `_one`/`_other` ile aranıyor; eksik ek
tip kontrolünden ve parite testinden geçip çalışma anında ham anahtar basıyordu —
`time.test.ts` artık render çıktısını doğruluyor.

**Guard'lar:** katalog parite/boşluk/placeholder testi, render-seviyesi çözümleme testi,
migre klasörler için ESLint `i18next/no-literal-string` allowlist kapısı (feature bittikçe
büyür), `uiLanguage` için backend↔frontend altın liste testi. `npm run i18n:extract`
eksik anahtarları boş değerle çıkarıyor; parite testi boşluğa kırmızı veriyor.

**Kalan:** ~2.2k Türkçe literal (450+ dosya) feature-feature taşınacak; backend hata
mesajları `code` tabanlı olacak; LLM'e giden `internal/view/*` projeksiyonları İngilizce'ye
sabitlenecek (bu bir maliyet işi — Türkçe token ~2 kat pahalı).

## Statik tanıtım sitesi eklendi (2026-08-25) ✅

Açık kaynak kullanıcıya yönelik tek-sayfa tanıtım sitesi `website/` altında kuruldu —
**Astro 5 + Tailwind v4**, İngilizce, statik çıktı (`npm run build` → `website/dist/`).
Go tarafı etkilenmedi (modül dışı, `go:embed` ağacına girmiyor). Detay: [72](72-TANITIM-SITESI.md).

**Placeholder politikası omurga:** repo/release/docs/lisans/sürüm henüz yok. Hepsi tek
dosyada (`website/src/site.config.ts`) toplandı ve `null` = "henüz yok" anlamına geliyor;
`CTAButton` pasif + "Coming soon" rozeti, `SmartLink` "(soon)", `Screenshot` build anında
`public/` altını kontrol edip placeholder çerçeve çiziyor. Site bugün **ölü link üretmeden**
eksiksiz görünüyor; repo açılınca tek dosya düzenlenip canlıya geçecek.

**Bölümler:** Hero (screenshot'suz çalışan CSS koordinatör paneli) · sayılar şeridi ·
"No Docker" karşılaştırması · 9 kartlık özellik grid'i · 3 derin-dalış şeridi ·
canlı tema showcase'i (6 renk × açık/koyu) · sekmeli quickstart · "bu ne DEĞİLDİR"
bölümü · footer. Son bölüm **auth yokluğunu + wildcard CORS'u açıkça yazıyor** —
pazarlama değil güvenlik gereği (aksi hâlde kullanıcı `0.0.0.0`'a açıyor).

**Screenshot hattı:** `scripts\shots.ps1` (ASCII-only) → `website/scripts/shots.mjs`
(Playwright). Çalışan örneğe bağlanıp `#/w/{ws}/{view}` rotalarını 2560×1440 koyu temada
çekiyor. Playwright bilerek `package.json`'a konmadı (Chromium ~150 MB), `-InstallDeps`
ile talep üzerine kuruluyor. Görseller gitignore'da — üretilen çıktı, kaynak değil.

**Doğrulama:** `npm run build` temiz (2 sayfa, 33 KB CSS), `npm run check` **0 hata**.
Görsel doğrulama yapılmadı (tarayıcı MCP kaynakları pasif).

**Doküman ayak izi:** `72-TANITIM-SITESI.md` (yeni) · `00-GENEL-BAKIS.md` doküman dizini +
script tablosu (`shots.ps1`) · kök `README.md` proje yapısı + "Tanıtım Sitesi" bölümü +
doküman listesi · `CLAUDE.md`'ye "website/ — tanıtım sitesi (frontend/ ile karıştırma)"
bölümü (iki ayrı npm projesi, prettier kapsamı yalnız `frontend/`, placeholder kuralı,
elle senkronlanan iki tema dosyası) · `website/README.md`.

**Yan bulgu — kök `README.md` bayat:** kaldırılmış Hafıza alt sistemini (2026-07-05)
hâlâ özellik olarak sayıyor, provider listesi 3 diyor (gerçekte 7 kind), tema sayısını
8 sanıyor (gerçekte 6 renk × 2 mod). Site metni bu yüzden README'den değil
`00-GENEL-BAKIS.md` + skill'den yazıldı. **README tazeleme açık iş olarak kaldı.**

## Proje yeniden adlandırıldı: TionSwarm → TionHarness (2026-08-24) ✅

Proje `Desktop\Projects\TionSwarm`'dan **`Desktop\Projects\TionHarness`**'e kopyalandı
(git geçmişi korundu, `origin` remote'u kaldırıldı) ve ad değişimi baştan sona uygulandı:
**3342 eşleşme / 795 dosya**, üç casing formu (`TionSwarm`/`tionswarm`/`TIONSWARM`).

**Değişenler:**

- Go modülü `github.com/bilal-arikan/tionharness`; `cmd/tionharness`, `cmd/tionharness-desktop`.
- Veri dizini `~/.tionswarm` → **`~/.tionharness`**; env öneki `TIONSWARM_*` → **`TIONHARNESS_*`** (49 değişken).
- Frontend `localStorage` önekleri `tionswarm.*` → `tionharness.*` (~35 anahtar).
- Gömülü 13 skill klasörü `tionswarm-*` → `tionharness-*`.
- Deploy birimi `deploy/tionharness.service`; başlatıcı `TionHarness-Baslat.cmd`.

**Göç şimi YOK — kasten.** Kodda `TIONSWARM_*` fallback'i veya otomatik dizin göçü yok;
temiz kesim yapıldı. Bunun yerine canlı veri dizini **elle taşındı** (aşağı bak).

**Canlı veri dizini taşındı (2026-08-24):** `~/.tionswarm` → **`~/.tionharness`**
(~2.4 GB, 19k dosya, 14 workspace). Dizin adı değişiminin yanında:

- İçerideki 260 yol yeniden adlandırıldı — `skills/tionswarm-*`, çalışma-dizini durum
  klasörleri `.tionswarm/`, `logs/tionswarm.log`, `claude-home/projects/C--…--tionswarm-workspaces-*`.
- 1105 metin dosyasında (json/md/py/ps1/txt/toml/html) referanslar düzeltildi; `.jsonl`
  transkriptleri (6559 dosya) **bilerek dokunulmadan bırakıldı** — append-only tarihsel kayıt.
- Dokunulmayan korumalı desenler: `bench-tionswarm` (diskte duran harici repo),
  `C--Users-user-Desktop-Projects-TionSwarm` (claude-home proje dizini — eski proje
  klasörü hâlâ var, resume'u kırmamak için), cbm-store index adları, transkriptlerden
  referanslı `mcp-tionswarm_interaction-*.txt` tool-result dosyaları.
- Doğrulama: 5111 JSON parse edildi, **0 bozulma**; backend taze dizinle açıldı,
  14 workspace yüklendi, log'da 0 ERROR/WARN.
- **İkinci geçiş gerekti:** ilk sed 239 dosyayı atlamıştı (`xargs` toplu iş kaybı — aynı
  hata depoda da yaşandı, orada da tekrar koşarak çözüldü). Kalan `prompt_epoch.json`
  (87 adet, donmuş sistem promptu + araç şemaları) ve insight raporları düzeltildi.
  Ders: bu tür süpürmelerde **sıfıra kadar tekrar tara**, tek geçişe güvenme.
- Geri dönüş: `~/.tionswarm-config-backup-20260824\` (kök configler),
  `~/.tionharness-textfiles-backup-20260824.tgz` (1105 dosya) ve
  `~/.tionharness-textfiles-backup2-20260824.tgz` (ikinci geçişin 239 dosyası).

**Türkçe ek uyumu:** düz sed `TionSwarm'ın`/`'da`/`'a` gibi ekleri olduğu gibi bıraktığı
için ~380 yerde uyum bozuldu (`Harness` ince ve sessiz-sert biter). Hepsi düzeltildi:
`'in` · `'e` · `'te` · `'ten` · `'i` · `'teki`. İngilizce iyelik `TionHarness's` (121 yer,
Go/TS yorumları) bilerek korundu.

**Market markası da değişti (2026-08-24):** `SwarmPack` → **`HarnessPack`**. Bu yalnız
metin değil, disk/tel formatı: `SchemaV1 = "harnesspack/v1"`, `packFileSuffix =
".harnesspack.json"`, gömülü 6 default paket dosyası ve `go:embed` deseni. Kullanıcının
`<dataDir>/market/` klasöründeki **54 kurulu paket** de göç ettirildi (dosya adı + `schema`
alanı; yedek `market-backup-swarmpack-20260824/`) — aksi halde `scanDir` eski soneki
bulamayıp market'i boş gösterirdi. Uyumluluk şimi yok: eski `.swarmpack.json` artık okunmaz.

**`swarmregistry/v1` → `harnessregistry/v1` (2026-08-24):** uzak index protokolü
(`RegistrySchemaV1`, `21-MARKET.md` §7.1). Ayrı bir karardı çünkü paket zarfı değil bir
**tel formatı**; ama bu şemayı üreten tek şey bizim connector adaptörlerimiz (SkillsMP,
CrossAITools site API'lerini bu şekle çeviriyor), dışarıda yayınlayan sunucu yok →
kırılacak sözleşme yok. `<dataDir>/market/.remote-cache/` altındaki iki önbellek de
göç ettirildi, aksi halde eski şemayla reddedilirlerdi.

**Bayat cbm indeksleri silindi (2026-08-24):** `<dataDir>/workspaces/*/cbm-store/`
altında, artık var olmayan yolları indeksleyen **153 MB** ölü veritabanı vardı
(silinen `Projects\TionSwarm` ×3, yeniden adlandırılan `Progs\bench-tionswarm-bare`).
Yol yeniden kurma sezgisel olduğu için (klasör adındaki tire ile yol ayracı ayırt
edilemiyor — `Desktop-city-cleaner` önce yanlışlıkla ölü sanıldı) her aday tek tek
`Test-Path` ile doğrulandı; yalnız kesin ölü olanlar silindi. Ortak store'daki
`...-TionSwarm` projesi de `delete_project` ile kaldırıldı.

**Depo dışı yüzeyler de çevrildi (2026-08-24):**

- **Craft workspace:** 5 skill `tionharness-*`, source slug'ı `tionharness`, görünen ad
  ve `workingDirectory`. Workspace klasörü/slug'ı (`ws_tionswarm`) uygulama açıkken
  taşınamaz → bekliyor.
- **`Progs\tionharness-obsidian-sync`** (eski `tionswarm-obsidian-sync`): veri dizini
  taşınınca **bozulmuştu** — `config.json`'daki `data_dir` hâlâ `~/.tionswarm`'ı,
  `project_name` alanları da yeniden adlandırılmış workspace/vault adlarını göstermiyordu.
  Klasör, `sgsync/tionharness.py`, `TionHarnessClient`, `state/*.json` ve config anahtarları
  (`tionharness_to_pm`/`pm_to_tionharness`) çevrildi; üç eşleme de doğrulandı.
- **Windows Startup:** `CodebaseMemory-Watch-TionHarness.vbs` ve
  `TionHarness-Obsidian-Sync.vbs` — ikisi de yeni yollara çevrildi ve yeniden başlatıldı.
  Artık kod indeksleme TionHarness'i takip ediyor.
- **`Progs\bench-tionharness{,-bare}`, `Progs\tionharness-dev`** çevrildi. Not: bench
  klasörleri aslında genel algoritma benchmark'ı (AlgoBench), ürünle ilgisi yok — yalnız
  adı öyleydi. Yol değiştiği için eski cbm indeksleri öksüz kaldı (yeniden üretilebilir).

**Kasten dokunulmayanlar:** uzak depo ve dağıtım tarafındaki eski adlandırma, Claude Code'un
kendi `swarm/teammate` alt sistemine yapılan doküman atıfları, tarihsel `_Docs/05-ARSIV.md`
anlatıları.

**Not:** ad değişimi, kaynak projedeki `f544edf2` (tool-approval ses ipucu) commit'i ve
o an duran WIP ağacı senkronlandıktan **sonra** yeniden uygulandı; kopya kaynakla birebir.

## claude-cli 2.1.238 result zarfı: auth/terminal_reason/izin reddi/hook (2026-08-21) ✅

CLI 2.1.237 → 2.1.238 güncellemesi sonrası canlı `-p --output-format stream-json
--verbose` çıktısı ölçüldü. Olay **tipleri** değişmemiş (`system`, `assistant`,
`user`, `result`, `rate_limit_event`) ama `result` zarfı yeni alanlar taşıyor ve
bir tanesi gerçek bir hata sınıflandırma boşluğu açığa çıkardı.

**Gözlemlenen gerçek başarısızlık:** `{"subtype":"success","is_error":true,
"stop_reason":"stop_sequence","terminal_reason":"api_error","result":"Failed to
authenticate: OAuth session expired and could not be refreshed"}`. Yani `subtype`
"success" derken `is_error` true — sınıflandırmanın `is_error`'a bakması şart,
`subtype`'a asla. Parser zaten öyle yapıyordu; asıl kusur `isAuthErrorText`
listesinde bu ifadenin olmamasıydı → oturum düşmesi jenerik hata sayılıp boşuna
retry ediliyordu.

**Değişiklikler (`internal/providers/claudecli.go`).**

- `isAuthErrorText`: `failed to authenticate` ve `oauth session expired` eklendi
  (eski girdiler korundu).
- `cliEvent`: `stop_reason`, `terminal_reason`, `permission_denials` (yeni
  `cliPermissionDenial{tool_name, tool_use_id}`) ve hook alanları
  (`hook_name`, `hook_event`, `exit_code`, `outcome`, `stderr`) parse ediliyor.
- `case "result"`: hata ne auth ne rate-limit ise metne `(terminal_reason: X)`
  — yoksa `(stop_reason: X)` — ekleniyor; artık çıplak mesaj yerine sınıf görünür.
- İzin reddi sessiz kalmıyor: `[permission] N permission denial(s): Bash, Write`
  biçiminde bir **text** adımı yazılıyor. `TraceStep.Kind` yalnız
  `text|thinking|tool` olduğu için sahte bir "tool" adımı üretmek yerine
  `codexcli_events.go`'daki `[codex error] …` konvansiyonu izlendi.
- Yeni `case "system"` + `subtype == "hook_response"`: `exit_code != 0` (ya da
  fail/error/block/deny içeren `outcome`) olduğunda `[hook] …` uyarı adımı,
  stderr 500 karakterde kırpılarak. Başarılı hook hiçbir iz bırakmıyor.

Doğrulama: `gofmt -l` temiz, `go build ./...` temiz, `go test ./internal/providers/
-count=1` → `ok … 6.4s`. Yeni testler `internal/providers/claudecli_result_test.go`.

Sıradaki muhtemel adım: `permission_denials`'ı `Response`'a yapısal alan olarak
taşıyıp frontend'de metin adımı yerine ayrı rozet göstermek.

## Piper kurulumu wheel'e göçtü + sürüm probu esnetildi (2026-08-20) ✅

Harici araç turunda ortaya çıktı: **upstream Windows'a artık standalone piper
arşivi yayınlamıyor.** `OHF-Voice/piper1-gpl` v1.7.0'ın Windows varlığı yalnız
`piper_tts-1.7.0-cp39-abi3-win_amd64.whl`. Bu bir sürüm atlama değil, **çalışma
zamanı göçü** — o yüzden sessizce yapılmadı, kullanıcıya sorulup onaylandı.

**Kurulum.** `Progs\piper\.venv` (Python 3.13) + `pip install piper-tts==1.7.0`.
Eski standalone kurulum (1.2.0, 37 MB) silindi; `voices\tr_TR-dfki-medium.onnx`
yerinde bırakıldı — modeller pakete bağlı değil.

**Kod (`internal/tts/tts.go`).** `piperExe()` aday listesinin **başına**
`.venv/<Scripts|bin>/piper` eklendi; eski layout listede kaldı, dolayısıyla
rhasspy-dönemi kurulumu olan bir host bozulmuyor. `voiceDirs()` iki seviye yukarıyı
da tarıyor (binary `.venv/Scripts`'te ama modeller `<install>/voices`'ta; bir
seviye yukarı yalnız `.venv/voices`'a ulaşırdı — oraya kimse yazmıyor).
`Synthesize` **değişmedi**: piper1-gpl `--model`/`--output_file` alt-çizgili
yazımları takma ad olarak koruyor, yani sürüm forku gerekmiyor.

**Sürüm probu (`internal/exttools/versionprobe.go`, yeni).** Yeni CLI'da
`--version` **yok** ve bilinmeyen bayrağı sürüm-şeklinde token içermeyen usage
metniyle reddediyor → `LocalVersion` panelde kalıcı "sürüm okunamadı" bırakırdı.
Yeni `Tool.VersionProbe(path)` prob hedefini çözüyor: venv console script'i ise
o venv'in python'ına `importlib.metadata.version('piper-tts')` sorar, değilse
aracın kendi `--version`'ına düşer. Mandal **`pyvenv.cfg`** — yalnız klasör adına
bakmak `/usr/bin/piper`'ı venv sanıp yanındaki sistem python'ına sorardı ve
kurulu olmayan bir dağıtım için hata döndürürdü. `internal/api/external_tools.go`
iki çağrı yerinde de probu kullanıyor. Katalogdaki `VersionArgs` **korundu** —
eski standalone kurulumun tek prob yolu o.

**Update spec'i.** `manual` kaldı ama gerekçesi değişti: kilit sorunu yok (pip
paketi), fakat çalıştırılacak interpreter host'a özel venv python'ı ve statik
katalog mutlak yolunu bilemez. `Note` artık gerçek komutu gösteriyor.

Doğrulama: `go build ./...` temiz, `internal/{tts,exttools,api}` 330 test + 4 yeni
prob testi geçti, canlı uçtan uca sentez `.venv` piper'ıyla 245 KB WAV üretti.

## Eager araç maliyeti: `get_view` kısaltıldı + `run_subagent` özet tier'a indi (2026-08-20) ✅

2026-08-15 taramasının bilerek ertelenen iki maddesi kapandı.

**`get_view` (en pahalı eager araç).** Ertelenmedi — 51 gerçek çağrısı var ve
alternatifi (ham state okumak) daha pahalı. Bunun yerine açıklaması sıkıştırıldı
(kind başına paragraf → tek satır) ve `Examples` 7→4'e indi; örnekler
`foldExamples` ile **gönderilen şemaya** katıldığı için her biri tur maliyetidir.
Kalan dört örnek şemanın anlatamadığı konvansiyonları (singleton id, `sub`
drill-down, `level`) kapsıyor. `expand` referansı korundu — name-only olan
`expand`'in tek keşif yolu o cümle. **~1087 → ~858 token.**

**`run_subagent` (Strateji B).** Name-only YAPILMADI: delegasyon davranışsal, aracı
göremeyen model işi kendisi yapar. Onun yerine **özet tier** (`MarkLazy`) + açıklamanın
ilk satırı tek cümlelik nudge olacak şekilde yeniden yazıldı (`lazyDescription` ilk
satırı alıp 200 karakterde keser) → şema (791 tok) turdan kalktı, katalogda tek satır
hatırlatma kaldı. claude-cli etkilenmez (`coreInteractionTools` üyesi → orada eager).

**Üç davranışsal araçta şema sıkıştırması (tier değişmedi).** `ask_user`
**684 → 422**: şişkinlik açıklamada değil, `options` içindeki string/`{label}`
`oneOf` bloğundaydı — üstelik `questions[]` dalında ikinci kez tekrarlanıyordu.
`flexOptions` zaten tüm bu şekilleri çözdüğü için `oneOf` parser'ın kabul
ettiğinden fazlasını anlatmıyordu → `"items": {}` + tek cümle prose (claude-cli
`AskUserQuestion` uyumu aynen duruyor). `transform_data` **528 → 412** (argv
sözleşmesi + "çıktıyı yazmak zorundasın" + "dosya editörü değil" korundu),
`create_artifact` **497 → 422** (davranış kuralları korundu, alan açıklamaları
kısaldı), `todo_write` **533 → 471** (`category`/`steps` alanları silinmedi —
progress dosyasına kadar taşınıyorlar; yalnız açıklamalar kısaldı).

**Ölçüm (shell AÇIK, örnekler dahil, HEAD baseline `git worktree` ile alındı):**
eager **21 → 20** araç, **~8022 → ~6487 token**, katalog ~429 → ~485 →
**net ~1479 token/tur (%17).** Geriye 500 token'ı aşan tek eager araç `get_view`
(858) kaldı → bu tarama kolu burada bitti.

## Statik prefix blok taraması: skill kataloğu kırpılıyor (2026-08-20) ✅

Araçlardan sonra sıra prompt bloklarına geldi. WS5'te ölçüm: eager şemalar ~6487,
araç kataloğu ~485, **"# Available Skills" ~1399**. Yani en pahalı tek parça
araçlar değil, **skill kataloğuydu** — hiçbir aracın yaklaşamadığı boyutta.

**Kök sebep.** Araç kataloğunda lazy özet 200 karakterde kırpılıyor
(`lazyCatalogDescMaxChars`), skill satırı ise kırpılmıyordu. `description` +
`when_to_use` kullanıcı-yazımı serbest metin → tek uzun skill her ajanı, her turda,
süresiz vergilendiriyordu (en pahalı satır 240 token; ilk dört skill bloğun %54'ü).

**Düzeltme.** `renderCatalog` artık `catalogLine` ile kırpıyor (`description` 200,
`when_to_use` 160 karakter; ilk boş-olmayan satır, rune sınırında kesim + `…`).
Bilgi kaybı yok — `skill_search` tam frontmatter'ı, `use_skill` gerçek gövdeyi verir;
zaten blok'un altbilgisi modeli `skill_search`'e yönlendiriyor.
**WS5 1399→1026, WS1 1163→820, WS17 1372→1029** (~%25–30). Kırpma sonrası hiçbir
skill satırı 110 token'ı geçmiyor.

**Araç kataloğu.** Zaten yalındı; yalnız iki prose paragrafı (blok'un ~%36'sı)
mekanizmayı koruyarak sıkıştırıldı → **~485 → ~432**. Yan bulgu: self-management
işaretçisi hâlâ "memory" diyordu — hafıza alt sistemi 2026-07-05'te kaldırılmıştı,
yani blok var olmayan araçları reklam ediyordu; silindi.

Detay: `_Docs/19`. Düzeltme: 2026-08-15'teki "~2311 token" rakamı örnekleri
saymıyordu ve baseline'ı zaten insight sonrası alınmıştı (insight'ın ~700'ü o
sayının dışında).

Detay: `_Docs/19`.

## Sağlayıcı göçünün canlı-veri onarımı (2026-08-19) ✅

Sağlayıcı örnekleri + paylaşılan CLI login evleri devreye alındıktan sonra
mevcut kurulumda üç kırık ortaya çıktı; hepsi hem kodda hem diskte kapatıldı.

- **Ölü `--resume` id'leri (bildirilen hata).** claude-cli transkriptleri config
  evinin içinde durduğu için, ev `<workspace>/claude-home` → `<dataDir>/claude-home`
  taşınınca oturumların `cliSessionId` alanı eski evi gösteriyordu: `No conversation
found with session ID: …` + exit 1, üstelik "yeniden denenebilir" göründüğü için
  aynı ölü id ile 3 tur harcanıp oturum `stuck` etiketleniyordu.
  → `ClaudeCLI.CanResume(id)` ön-kontrolü (`claudecli_resumecheck.go`) +
  `planClaudeResume` bulunmayan id'de **soğuk** başlar (tam transkript korunur);
  CLI yine reddederse hata artık NON-retryable ve id + ev adını söyler.
- **Örnek id'si çalışma zamanında hiç kullanılmıyordu.** 13 çağrı yeri
  `providers.Get(agent.Provider)` (kind id) diyordu → ikinci bir örneğe bağlı ajan
  sessizce kind'ın varsayılan örneğine düşüyordu. → tek seam `db.Agent.ProviderRef()`.
- **`minimax-anthropic` / `deepseek-anthropic` örneği hiç üretilmemişti** (kendi
  legacy ayar anahtarları yok, temel sağlayıcının kimliğini paylaşıyorlar) → bu
  kind'a bağlı 5 ajan "unknown provider instance" ile ölüydü.
  → `MigrateFromSettings` iki varyantı da üretir (`baseUrl` boş = kind'ın kendi ucu).
- **Onarım aracı:** `cmd/repair-provider-migration` (kuru çalışma varsayılan,
  `-apply`, idempotent). Canlı sonuç: 2 örnek eklendi, 1 ajan yeniden bağlandı
  (`PRV1` → `claude-cli`), 114 transkript yeni eve taşındı, 28 ölü resume kaydı
  temizlendi, bu kesintinin bıraktığı `stuckTurns`/`stuck` izi silindi.
- **Eski per-workspace CLI evleri silindi (2026-08-20):** `claude-home` 14
  workspace / 1.16 GB (önce onarım aracının kuru çalışması "0 taşınacak
  transkript" dediği doğrulandı — hiçbir oturum artık eski eve bağlı değil) +
  `codex-home` 14 workspace / 167 MB (codex thread'leri oturumlarda tutulmuyor,
  `ResumeSessionID`'yi yalnız claude yolu yazıyor → bağımlılık yok). Ardından
  canlı turlar yeşil: claude soğuk+sıcak, codex `gpt-5.6-sol`. Per-workspace ev
  tohumlamasını anlatan ölü yorumlar (`workspace.Manager.open`, `toolloop`) da
  kaldırıldı — kod 65c58e9'da gitmişti.
- Detay → `71-SAGLAYICI-ORNEKLERI-PLANI.md` §3.1 ve `51-CLAUDE-CONFIG-BIRLESIK.md`.

## codex-cli sağlayıcısı: katalog + fiyatlandırma + frontend yüzeyi (2026-08-18) ✅

Çok-worker turunun (W1-W7) son ayağı — backend `codex-cli` transport'u
(`internal/providers/codexcli*.go`, `kind_codexcli.go`) diğer worker'larca
tamamlandıktan sonra kalan yüzey işi:

- **`internal/api/catalog_codexcli.go`** (yeni) — `codex --version` sürüm
  probe'u (`claudeCLIVersion`'ın aynısı, 10 dk/1 dk TTL) + `codexSubscriptionTier`
  (workspace `codex-home/auth.json` var/okunabilir mi → `"chatgpt"`/`""`; Codex'in
  auth dosyası claude'unki gibi plan adı taşımıyor, bu yüzden zengin tier yok).
  `catalog.go`'ya `codex-cli` dalı eklendi.
- **`internal/providers/pricing.go`** — yeni `"openai"` fiyat tablosu (yalnız
  `EstimateFor`'un `codex-cli` case'ini besliyor; `priceTable`'da `codex-cli`
  YOK, abonelik = fiyatsız). `gpt-5.6-sol/terra/luna`, `gpt-5.5`, `gpt-5.4-mini`
  eklendi (iki bağımsız kaynaktan doğrulandı). `gpt-5.4` (ChatGPT hesabıyla
  kullanılamıyor) ve `gpt-5.2` (yayınlanmış fiyat yok) **bilerek atlandı**.
- **`internal/exttools/catalog.go`** — `CodexToolName = "codex"` girişi
  (Harici Araçlar panelinde claude'un kardeşi). `server.go:applySettings`'e
  eksik kalan `exttools.SetPathOverride(CodexToolName, …)` satırı eklendi.
- **`internal/api/workspace_settings.go`** — `codexHomeDir` alanı
  (`claudeHomeDir`'in kardeşi, `<workspace>/codex-home`).
- **Frontend** — `ProvidersPanel.tsx`'e ikinci bir kart: CLI yolu + salt-okunur
  workspace config dizini + **login butonu yerine** `CODEX_HOME=… codex login`
  komutunu gösteren yardım metni (backend login endpoint'i bu turda yok,
  sessizce çalışmayan buton yerine gerçek komut tercih edildi).
- **Doğrulama:** `go build ./...` temiz; `go test ./internal/providers/
./internal/exttools/ -count=1` yeşil (`internal/api` paketi, bu işin dışında
  başka bir worker'ın bıraktığı `TestSteerableForTurn` çift tanımından derleme
  hatası veriyor — ayrıca raporlandı, bu turun kapsamı değil). Frontend `tsc
--noEmit` ve `vitest run` (245+ test) temiz; dokunulan dosyalar prettier'e
  uygun (config'li).
- Detay ve plan-vs-gerçek farkları → `_Docs/70-CODEX-CLI-UYGULAMA-PLANI.md` §8.

## `tionharness-terse` skill'i kaldırıldı (2026-08-18) ✅

- **Sorun:** Terse yanıt stili artık `WSSettings.TerseMode` + registry promptu
  `terse` ile koşulsuz enjekte ediliyor. Gömülü `tionharness-terse` skill'i aynı
  metnin ikinci kopyasıydı ve her oturumda skill katalog listesinde bir satır
  yer kaplıyordu — enjekte edilmiş bir stili "gerekirse yükle" diye ilan etmek.
- **Yapılan:** `internal/skills/defaults/tionharness-terse/` silindi (dizin
  `//go:embed defaults` ile gömülüyordu, ayrı kayıt listesi yok) ve global
  tier'a seed edilmiş kopya (`~/.tionharness/skills/tionharness-terse`) kaldırıldı.
- **Tek kaynak:** stil metni yalnız `internal/prompts/defaults/terse.md`
  (workspace override: `<workspace>/config/prompts/terse.md`).
- **Not:** `delete_skill` gömülü/global tier'ı korur — yalnız workspace skill'ini
  siler; gömülü bir default'u kaldırmak depodan silmeyi gerektirir.

## Store yüklemesi paralelleştirildi — boot 3–6× (2026-08-17) ✅

Bir önceki maddede ölçülen "boot'un maliyeti parse değil, **soğuk dosya açma**
(15 ms/dosya)" bulgusunun doğrudan karşılığı. Maliyet CPU değil **gecikme**
olduğu için worker'lar çekirdek değil bekleyen syscall harcıyor; havuz açmayı
üst üste bindirince gecikme gizleniyor.

**Yeni: `internal/db/loadpar.go`** — `parallelLoad[In, Out]`, sınırlı worker
havuzu (`min(2×CPU, 16)`). İki garanti **kasıtlı**:

- Sonuçlar **giriş sırasında** döner (çağıranlar konuma göre indeksliyor).
- Hata **giriş sırasına göre ilki** seçilir. Boot bozuk bir entity dosyasını
  ölümcül sayıyor; hangi dosyanın suçlanacağı — ve boot'un düşüp düşmeyeceği —
  goroutine zamanlamasına bağlı olamaz. Aksi hâlde ayda bir tekrarlanan,
  testte hiç görünmeyen bir hata olurdu.

**Paralelleştirilen yollar:**

| Yol                                | Ne okuyordu                                                                                                                              |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `loadJSONDir` (db.go)              | Tüm entity dizinleri: agents, tasks, schedules, mcp, flows, flow-runs, session-asks, automations, artifacts, hooks, usage, session-usage |
| Artifact içerikleri (db.go `load`) | Metin artifact'lerinin ayrı gövde dosyaları (WS1'de 156 artifact)                                                                        |
| `loadSessions` (store.go)          | Oturum başına `session.json` + `messages.jsonl` — en büyük kalem                                                                         |
| `recoverInflight` (inflight.go)    | Oturum başına `inflight.json` sondası; temiz kapanışta hepsi ıska ama **ıska da soğuk bir dosya açması**                                 |

`recoverInflight` ayrıca yeniden yapılandırıldı: sondalar kilit **dışında** ve
eşzamanlı koşuyor, mutasyon geçişi seri ve kilitli kalıyor. Oturum id'leri
sıralanıyor ki kurtarma sırası (dolayısıyla üretilen mesaj id'leri) boot'lar
arasında tekrarlanabilir olsun. Okunamayan sidecar artık **sessizce atlanmıyor**,
`slog.Warn` ile loglanıyor (dosya diskte kalıyor, sonraki boot tekrar deniyor).

**Kontrollü ölçüm** (aynı ağacın yarısı seri / yarısı 16 worker — iki yarı da
eşit soğuk):

| Store            | Seri           | Paralel       | Hızlanma |
| ---------------- | -------------- | ------------- | -------- |
| WS5 (701 dosya)  | 13,48 ms/dosya | 2,13 ms/dosya | **6,3×** |
| WS16 (265 dosya) | 25,67 ms/dosya | 8,69 ms/dosya | **3,0×** |

Gerçekçi bant **3–6×** → 59 sn'lik toplam boot ~10–20 sn'ye iner.

**Testler:** `loadpar_test.go` — giriş sırası korunumu, deterministik hata seçimi
(50 tekrarlı), 0/1/15/16/17/500 öğede havuz doğruluğu, `loadWorkers` sınırları,
bozuk entity dosyasının hâlâ boot'u düşürmesi, ve 60 oturumluk bir store'un
yeniden açılışta birebir aynı gelmesi (transkriptlerin çapraz bağlanmadığı dahil).

**Not:** Paralelleştirme 15 ms/dosya maliyetini **gizler, kaldırmaz**.
`~/.tionharness` için bir Defender istisnası hâlâ en büyük tek kazanç olabilir.

## Mesaj belleği — Aşama 0+1: ölçüm + dar transkript okuyucuları (2026-08-16) ✅

Madde 5 (tüm mesajların RAM'de tutulması) planının ilk iki, risksiz aşaması.
Lazy yükleme **henüz yok**; bu adım ölçümü kuruyor ve "tam transkript kopyala"
israfını kaldırıyor.

**Aşama 0 — ölçüm.** `db.Stats()` (`store_stats.go`) store'un RAM ayak izini
döndürür: `sessions`, `loadedSessions`, `messages`, `messageBytes` + entity
sayıları. İki yerden okunur:

- **Boot log'u** — workspace başına `store opened workspace=… ms=… sessions=…
messages=… message_mb=…` (`internal/workspace/manager.go`). Yavaş açılışta
  sorumlu workspace doğrudan görünür.
- **`GET /api/debug/store-stats`** — workspace başına satır + `totals`
  (`debug_store_stats.go`). Global uç; workspace-scoped değil.

`loadedSessions` bugün `sessions`'a eşit (yükleme eager). **Lazy yükleme
geldiğinde hareket edecek metrik budur** — aynı uç, sözleşmesi değişmeden
"ne kadar fayda etti?" sorusunu yanıtlar. Hiç yazılmamış oturum `nil` slice
tuttuğu için "loaded" sayılmaz, aksi hâlde ayak izi olduğundan büyük görünürdü.

**Aşama 1 — dar okuyucular** (`store_messages_read.go`). `ListMessages` her
çağrıda oturumun **tüm** mesaj slice'ını kopyalıyordu; çağrı yerlerinin çoğu
aslında son 1–64 mesajı istiyordu.

| Yeni okuyucu                    | Ne döner                                        |
| ------------------------------- | ----------------------------------------------- |
| `ListMessagesTail(ctx, sid, n)` | Son n mesaj **+ kuyruğun başlangıç indeksi**    |
| `LastMessage(ctx, sid)`         | Son mesaj, `ok=false` = boş oturum              |
| `FindMessage(ctx, sid, mid)`    | Tek mesaj; `ErrMessageNotFound` ≠ `ErrNotFound` |
| `StreamMessages(ctx, sid, fn)`  | Kopyasız gezinti; `fn false` dönerse durur      |

Taşınan çağrı yerleri:

| Yer                                              | Önce                              | Sonra                                                    |
| ------------------------------------------------ | --------------------------------- | -------------------------------------------------------- |
| `api/session_stream.go` `publishAutonomousReply` | tam kopya → son mesaj             | `LastMessage`                                            |
| `view/project.go` `loadSession`                  | tam kopya → son 40                | `ListMessagesTail`; `TailFrom` artık ikinci dönüş değeri |
| `api/sessions.go` `handleMessageSteps`           | tam kopya → tek mesaj             | `FindMessage`                                            |
| `api/session_changes.go`                         | tam kopya → sadece diff step'leri | `StreamMessages`                                         |
| `api/chat_queue.go` `writeChatReply`             | tam kopya → son assistant         | `ListMessagesTail(16)` + kuyruk kesikse tam tarama       |
| `api/todos.go` `todoContextBlock`                | tam kopya → son checklist         | `ListMessagesTail(64)` + kuyruk kesikse tam tarama       |

Son iki satırdaki geri düşüş **kasıtlı ve gerekli**: kuyruk kesilmişse
"bulunamadı" henüz kesin değildir, sessizce boş dönmek yalan olurdu.
`ListMessagesTail`'in ikinci dönüş değeri (`from > 0`) tam olarak bu kararı verir.

`view.Store` arayüzünde `ListMessages` yerine artık `ListMessagesTail` var —
projeksiyon zaten kuyruktan fazlasını render etmiyordu.

**Davranış farkı:** yeni okuyucular bilinmeyen oturumda `ErrNotFound` döner
(`ListMessages` boş slice dönüyordu). Yanlış oturum id'sinin "boş sohbet" gibi
görünmesi bu şekilde biter.

**Testler:** `store_messages_read_test.go` (pencere aritmetiği tablo testi, boş
vs. bilinmeyen oturum ayrımı, kopya-değil-alias güvencesi, erken durdurma,
`Stats` ölçümü), `debug_store_stats_test.go` (workspace başına + toplam,
manager'sız sunucuda panik yerine boş rapor).

### Aşama 0'ın ilk sonucu: planın gerekçesi kısmen çürüdü (2026-08-17)

İlk gerçek ölçüm iki tahmini bozdu:

|                          | Tahmin           | Ölçülen                           |
| ------------------------ | ---------------- | --------------------------------- |
| Mesajların RAM maliyeti  | 250–400 MB       | **95 MB** (300 MB RSS'in ~%32'si) |
| Boot yavaşlığının sebebi | 93 MB JSON parse | **Soğuk dosya açma, 15 ms/dosya** |

`Message.Steps` diskte zaten JSON **string** olduğundan struct'a dönüşte 3–4×
şişme yok. Boot ise (10 workspace toplamı **59,1 sn**) mesaj MB'ıyla değil
**dosya sayısıyla** ölçekleniyor — WS1: 2 MB mesaj / 104 oturum / 13,2 sn.

WS1 store'unda (720 dosya, 3,9 MB): soğuk seri okuma **15,02 ms/dosya**, sıcak
**0,14 ms/dosya** — 107× fark, yani dosya-açma başına bir filtre sürücüsü
(Defender) maliyeti. Maliyet CPU değil gecikme olduğu için paralel okuma
neredeyse doğrusal kazanıyor: WS17 (1315 dosya) 16 worker ile **15,9 sn → 1,9 sn
(8,5×)**.

Bu yüzden `load()`'a **faz bazlı ölçüm** eklendi (`timeLoadPhase`/`markLoadPhase`,
`db.go`); boot log'u artık `phases_ms="sessions=… entities=… …"` taşıyor ve
`Stats().LoadPhaseMs` ile `/api/debug/store-stats`'ten de okunur.

**Sırada (öncelik değişti):** (1) store yüklemesini paralelleştir — boot ~59 sn →
~7 sn, format değişikliği yok; (2) `~/.tionharness` için Defender istisnası (kod
dışı); (3) sonra plan'ın Aşama 2 (watermark) → Aşama 3 (lazy yükleme) — artık
gerekçesi boot değil **RAM (95 MB)** + `d.mu` çekişmesi + pagination UX'i.
Ayrıntı: `_Docs/16-PROFILLEME.md` → "Vaka: 59 sn'lik boot".

## `Run` entity'si kaldırıldı (2026-08-16) ✅

Bir önceki maddede "ölü map" olarak tespit edilen `db.Run` tamamen silindi. Kanıt
netti: `d.runs`'a **boot dışında hiçbir yerde yazılmıyordu** — `CreateRun`/`FinishRun`
diye bir fonksiyon depoda yoktu, gerçek store'daki `runs/` klasörü **0 dosya**
içeriyordu. Board görev çalıştırmıyor; entity yalnızca kendi taramasının maliyetini
üretiyordu.

**Silinenler:** `db.Run` struct'ı + `RunPending/RunRunning/RunSuccess/RunFailure`
sabitleri (`models_task.go`), `d.runs` map'i + `dirRuns` + `load()` bloğu +
`runningRuns` sayacı (`db.go`), `store_run.go` dosyasının tamamı
(`ListRunningRuns`, `HasRunningRuns`, `deleteRunLocked`), `DeleteAgent` ve
`DeleteTask` içindeki cascade döngüleri, `AgentBusy`'nin board-run dalı
(`agent_busy.go`), frontend `Run` arayüzü (`types/task.ts` — hiçbir yerden import
edilmiyordu).

**`activityState.Task` de kaldırıldı.** Tek kaynağı silinen run taramasıydı;
oturum-kind switch'inde `"task"` case'i yok ve production'da `"task"` kind'lı oturum
da üretilmiyor (`GetOrCreateSourceSession` yalnız automation için kullanılıyor).
Yani alan kalsaydı **kalıcı olarak `false`** dönecekti — çalışan bir gösterge gibi
okunan ölü bir alan, hiç alan olmamasından kötüdür. `GET /api/activity` yanıtından
`task` alanı ve frontend'deki `if (a.task) s.add('board')` satırı gitti; board nav
noktası zaten hiç yanmıyordu. `schedule` etkilenmedi — canlı kaynağı var
(`sess.Kind == "schedule"`).

**Korunanlar (bilinçli):** `Task.LastRunID/LastRunStatus/LastRunAt`. Bunlar Run
entity'sinin parçası değil, Task'ın alanları; onları da hiçbir kod yazmıyor ama eski
build'lerin yazdığı task dosyaları gerçek değerler taşıyor ve board görünümü +
`list_tasks` hâlâ gösteriyor. Alanları düşürmek, her task'ın bir sonraki yazımında o
geçmişi sessizce silerdi. Legacy oldukları model içinde yorumla işaretlendi.

Diskteki eski `runs/*.json` dosyalarına **dokunulmuyor** — artık okunmuyorlar,
kullanıcı verisini sessizce silmek doğru değil.

✅ `go build`/`vet` + `go test ./...` + frontend `tsc -b`/vitest yeşil.

## Boşta CPU: activity polling O(1) sayaca indi + poll'lar visibility-kapılı (2026-08-16) ✅

Boşta duran backend **249 s CPU / 116 dk** yakıyordu (~%3,6). Suçlu
`GET /api/workspaces/activity`: `workspaceRunning()` her workspace için
`ListRunningRuns` + `ListRunningFlowRuns` çağırıp `d.runs`/`d.flowRuns` map'lerini
**tamamen tarıyor**, sıralıyor ve bunu `d.mu.RLock()` altında yapıyordu. 5 pencere ×
4 sn × 10 workspace ≈ **saniyede ~25 tam-store taraması**. Beteri: `appendMessageLocked`
aynı `d.mu`'yu **senkron dosya yazımı boyunca** yazma modunda tutuyor ve Go'da bekleyen
yazar yeni okurları bloklar → poll ile canlı tur birbirini serileştiriyordu.

**Sayaç.** `internal/db`'ye `runningRuns` / `runningFlowRuns` `atomic.Int64` eklendi
(`db.go`), `load()`'da diskten tohumlanır. Flow tarafında tek boğaz noktası:
`persistFlowRunLocked(prev string, r FlowRun)` — `prev` **zorunlu parametre**, çünkü
derleyici böylece 6 çağrı yerinin hepsini işaretliyor ve ileride eklenecek bir
durum-geçiş yolu sayacı sessizce atlayamıyor. Okuma (`HasRunningRuns` /
`HasRunningFlowRuns`) `d.mu`'ya **hiç dokunmaz**.

Sayaç _cevap_ değil **hızlı-negatif kapısı**: 0 ise tarama hiç koşmaz (boşta hâli),

> 0 ise tarama koşar — o an makine zaten meşgul. Böylece fazla-sayma zararsız, tek
> gerçek risk eksik-sayma. Ona karşı `ReconcileRunCounters` (`store_runcount.go`) mevcut
> flow sweeper'ının 20. tick'inde (10 dk) çalışır, sapmayı düzeltir **ve `Error`
> seviyesinde loglar** — sessiz self-heal hatayı görünmez kılardı.

**Ölçüm** (`BenchmarkWorkspaceRunning`, 2000 flow-run'lı store):
272.932 ns/op + 385 KB/çağrı → **27,8 ns/op, 0 alloc**. ~9.800×. 125 çağrı/s × 273 µs
= saniyede 34 ms CPU = **%3,4** — ölçülen %3,6 ile birebir örtüşüyor.

**Yan bulgu 1 — `d.runs` ölü map.** Boot dışında hiçbir yerde yazılmıyor; `CreateRun`
diye bir fonksiyon yok. Yani `st.Task` pratikte hiç `true` olmuyor ve o tarama tamamen
boşaydı. → Takip eden maddede entity tamamen **silindi**.

**Yan bulgu 2 — `executions.go` O(oturum × flowRun).** `lastStatusFor` her flow
oturumu için `ListFlowRuns` çağırıyordu (tam tarama + sort), 5 sn'de bir, her pencere
için. Artık poll başına **tek tarama** ile `newestFlowRunStatus` indeksi kuruluyor ve
yalnız feed'de gerçekten flow oturumu varsa.

**Yan bulgu 3 — `ListFlowRuns` non-determinizmi (gerçek hata, test yakaladı).**
Yalnız saniye-granülerlikli `CreatedAt` ile sıralıyordu; aynı saniyede oluşan iki
koşunun sırası map iterasyonuna kalıyor, yani her çağrıda değişiyordu → `runs[0]`
("en yeni koşu") rastgele seçiliyordu. Depoda tam bu iş için duran `flowRunBefore`
(id sayacıyla tie-break) artık `ListFlowRuns` ve `ListRootFlowRuns`'ta kullanılıyor.

**Frontend.** `useAsync`'e `pauseWhenHidden` (varsayılan açık) + `setInterval` tabanlı
paneller için `useVisiblePoll` eklendi: gizli pencere hiç poll etmez, görünür olunca
bir yakalama koşusu yapar. 5 pencerenin 4'ü arka plandaysa trafiğin ~%80'i gider.
SSE yedeği olan interval'lar gevşetildi: activity 3→15 sn, workspaces/activity 4→20 sn,
executions 5→20 sn, flow runs 3→15 sn, automation stats 5→15 sn, MCP pool 5→10 sn.
`useActivity` `useAsync`'e taşındı (kapıyı bedava aldı). `LogsPanel` incelendi —
zaten SSE-birincil + 30 sn uzlaştırma, dokunulmadı.

**Tam SSE'ye taşıma (event-driven `workspaces/activity`) bilinçli olarak YAPILMADI:**
SSE zaten bağlı (`SIGNAL_WORKSPACE_ACTIVITY`) ve poll onun yedeği; sayaçtan sonra bir
poll'un maliyeti workspace başına 4 atomik yükleme. Yeni event tipi + ikinci bir "her
geçişi yakala" yükümlülüğü + reconnect senaryosu bu kazanca değmiyor.

Testler: `internal/db/store_runcount_test.go` (yaşam döngüsü, cascade, reload,
500-op rastgele dizi, drift tespiti), `internal/api/activity_test.go`,
`executions_flowstatus_test.go`, `activity_bench_test.go`.
✅ `go build`/`vet` + `go test ./...` + frontend `tsc -b`/vitest yeşil.

## Kuyruk watchdog'u boşta-farkında + her kesinti artık görünür (2026-08-16) ✅

Yukarıdaki düzeltmenin devamı; iki kalan açık kapatıldı.

**1) Boşta izleyicisi.** Duvar saati tek başına "asılı tur" ile "yavaş tur"u ayıramaz —
bu yüzden tavan cömert olmak zorunda, cömert tavan da gerçek bir wedge'in saatlerce
kuyruğu tutmasına izin veriyordu. Ayırt eden ölçü **sessizlik**: `runQueuedTurn` artık
15 sn'de bir yoklar ve **ya** tavanı aşan **ya da** `TurnIdleWatchdogMin` (varsayılan
**20 dk**) boyunca hiç olay üretmeyen turu keser. Adım yayan tur ne kadar uzun koşarsa
koşsun dokunulmaz.

Canlılık sinyali `sessionhub`'dan geliyor: `sessionState.lastPublish` her `Publish`'te
damgalanır — **ephemeral token delta'ları dâhil** (yalnız durable olayları saysaydık,
araç çağırmadan uzun metin üreten tur asılı sanılırdı) — ve `Hub.LastActivity()` ile
okunur. Sıcak yolda ek kilit yok: damga zaten alınan `h.mu` altında. Ölçüm
`(workspace, session)` kapsamlı, yoksa WS2/SES trafiği WS1/SES'i canlı gösterirdi.

**2) Görünürlük açığı.** `turn_error` yalnızca tur iptali **30 sn içinde
yanıtlamazsa** yazılıyordu; yaygın durumda (iptal gelir, tur hemen çözülür) transkriptte
**hiçbir iz kalmıyordu** — SES76'da sohbetin sebepsiz yarıda kalmış görünmesinin sebebi
buydu. Artık `recordWatchdogCut` **her** kesintide çalışır: kalıcı `kind=error` kartı +
`debug.jsonl` olayı + canlı `turn_error`. Sebep alanı ayrık: `watchdog` (tavan),
`watchdog-idle` (sessizlik), iptali de yanıtlamayan tur için `-detached` soneki.

**3) Panelde sessizlik göstergesi.** Aynı sinyal Oturum Bilgisi'ndeki süreç kartına
bağlandı: `running` DTO'su artık `lastActivityAt` + `idleLimitSec` + `hardLimitSec`
taşır, kart "N sessiz — sınır M" satırını gösterir (pencerenin %25'ini geçince görünür,
yarısını geçince `warning` rengine döner). **Süre değil mutlak zaman damgası
gönderiliyor:** panel yalnız konuşma değişince yeniden çeker, yani sessiz oturumda
hazır hesaplanmış bir süre donup kalırdı — tam da önemli olan durumda. İstemci
damgayı sunucu saatine karşı saniyede bir işler. İlk olay gelmeden önce ölçüm turun
başlangıcına düşer (kurulumda asılan tur da sayılır).

Testler: `internal/sessionhub/activity_test.go` (ephemeral damga + workspace kapsamı),
`internal/api/inbox_watchdog_test.go` (boşta ölçümü), `internal/agent/turnwatchdog_test.go`
(idle penceresi tavanı aşamaz). UI: Ayarlar → Araçlar'da "Tur boşta süresi (dk)" +
Oturum Bilgisi süreç kartında sessizlik satırı.

## Kuyruk watchdog'u 20 dk sabitinden ayara taşındı (2026-08-16) ✅

**Bulgu (WS15/SES76).** Sağlıklı bir sohbet turu tam `19m57s`'de kesildi:
`queued turn exceeded watchdog; force-cancelling after=20m0s`. Tur asılı değildi —
134 tool çağrısıyla ilerleyen bir Rust build/clippy döngüsüydü; iptal anında son
`Bash` çağrısı hâlâ çalışıyordu (`debug.jsonl`'de `durMs` yok).

**Kök neden.** 2026-07-28'de `spawnTimeoutMin` **120 dk**'ya çekilmişti (aşağıdaki
SES17 kaydı), ama sonradan eklenen kuyruk watchdog'u (`internal/api/inbox_durability.go`,
`_Docs/58`) **sabit 20 dk** idi ve ayarı hiç okumuyordu. Yani güvenlik ağı, korumak
istediği işin tavanından **dört kat dardı** — tıkanma freni, üretken turu kesiyordu.

**Düzeltme.** Sabit kaldırıldı; `TurnWatchdogMin` ayarı eklendi (varsayılan **120**,
1–1440 dk, Ayarlar → Araçlar'da "Tur izleyicisi (dk)"). `agent.Tunables.TurnWatchdog()`
değeri **spawn/zamanlama tavanlarının altına inemeyecek şekilde tabanlar** (aynı taban
`settings.normalize`'da da var) → bu sınıf hata yapısal olarak tekrarlayamaz. Ayar
global `~/.tionharness/settings.json`'da olduğu için **tüm workspace'ler** için geçerli;
mevcut dosyalarda alan yoksa `Open` default'tan 120 alır. Regresyon testi:
`internal/agent/turnwatchdog_test.go`.

## Insight araçları varsayılan NAME-ONLY (2026-08-15) ✅

`insight_scan` / `insight_list_findings` / `insight_apply_finding` şimdiye kadar
**eager** idi: üç tam şema her turun cache prefix'inde taşınıyordu, oysa bu araçlar
yalnız retrospektif tarama/triyaj turlarında kullanılıyor. `buildRegistry`'deki
varsayılan NAME-ONLY setine eklendiler (`internal/agent/toolsetup.go`) — katalogda
yalnız adlarıyla listelenir, şema `tool_search`/`activate_tools` ile çekilir;
claude-cli yolunda `tionharness_extended` köprüsünden ToolSearch ile gelir, yani
yetenek aynı. Yan düzeltme: `insight_scan`'i "eager araç örneği" olarak kullanan
yorumlar/test fixture'ları `todo_write`/`create_artifact` ile değiştirildi.
Detay: `_Docs/19` (Default NameOnly seti), `_Docs/60`.

## Düşük-frekanslı eager araç taraması → 5 araç daha NAME-ONLY (2026-08-15) ✅

**Yöntem.** Tahmin yerine ölçüm: (a) geçici bir audit testi `ShippedToolCatalog`
ile eager şemaları döküp token maliyetini çıkardı; (b) `~/.tionharness` altındaki
**355 `debug.jsonl`** günlüğü taranarak her aracın gerçek çağrı sayısı sayıldı.
Maliyet × nadirlik kesişimi seçildi.

**Sonuç.** `archive_sessions` (~820 tok / 4 çağrı), `expand` (~761 / 12),
`apply_patch` (~403 / 1), `read_lessons` (~181 / 2), `delete_lesson` (~146 / 0)
NAME-ONLY oldu. Eager 23→18 araç, **~9031 → ~6720 token** (≈ **%26**, tur/ajan
başına ~2311 token cache prefix'inden düştü).

**Bilerek eager bırakılanlar.** `get_view` (994 tok) tek başına en pahalısı ama
51 çağrıyla aktif ve alternatifi (ham state okuma) daha pahalı — doğru hamle
şemayı **kısaltmak**, ertelemek değil. `run_subagent` (707/13) `_Docs/19`'daki
"Strateji B": ertelemek delegasyon davranışını söndürebilir, önce prompt'a tek
satır nudge ister. `ask_user`/`request_confirmation`/`todo_write`/`create_artifact`
davranışsal dürtü. `WebFetch` 2026-06-26'da **bilerek** NameOnly'den eager'a
alınmıştı (commit 97ebf39) → dokunulmadı.

Detay + tablo: `_Docs/19` ("Düşük-frekanslı eager taraması").

## Workspace-kapsamlı oturum durumu — oturumlar workspace'ler arasına sızıyordu (2026-08-14) ✅

**Olay.** İki yeni workspace açıldı (`WS18` TionFramework, `WS19` FalciBaci),
birinde sohbet başlatıldı; UI'dan ikincisine geçilince orada **önceki
workspace'in oturumu** göründü.

**Kök sebep.** Oturum id'leri her workspace store'unun kendi `counters.json`'ından
gelir → `SES1` her workspace'te vardır. Süreç-geneli beş yapı ise yalnız session
id ile anahtarlanıyordu: `sessionhub.Hub`, `inboxStore`, `chatRuns`
(`sessionRunInfo`/`interactionToken`), `interactionStore`, `permGrantStore`. Yani
WS18/SES1 ile WS19/SES1 tek hub ring'ini, tek gönderi kuyruğunu ve tek izin/
etkileşim kaydını paylaşıyordu. Diskte bulaşma yok — sızıntı tamamen bellek
içindeki bu ortak anahtarlardan.

**Düzeltme.** Hepsi `scopeKey(wsID, sessionID)` ile anahtarlandı; `sessionhub`
metotları `(wsID, sessionID)` alır (derleyici tüm çağrı yerlerini yakalasın diye
imza değiştirildi, ~160 nokta). Ayrıca aynı sınıf hataya yol açan
**default-workspace geri dönüşleri kaldırıldı** (`flushInbox`,
`publishAutonomousReply`, `recordQueueTurnFailure` → `workspaceByID`, çözülemezse
log + `nil`); `teardownSessionRuntime` workspace'siz çağrıda hata döner; bus
köprüsü `e.WorkspaceID` kullanır. Geriye dönük uyum: `recoverInboxes` her
`inbox.json` kalemine bulunduğu store'un workspace id'sini yeniden damgalar.
Frontend: taslak anahtarı `tionharness:draft:<ws>:<session>`, hub aboneliği
`activeWorkspaceId`'ye de bağlı.

Detay + tablo: `_Docs8-QUEUE-SENKRON.md` → "Workspace kapsamı".
Testler: `internal/api/workspace_session_scope_test.go`, güncellenen
`session_teardown_test.go`.

## dev.ps1: Vite heap tavanı + exit koduna sebep etiketi (2026-08-14) ✅

**Olay.** 16:29:51'de web arayüzü gitti. `lifecycle.log`: `frontend exited on its
own (pid=43292 exit=-1)` → ardından dev.ps1 kendi kuralı gereği backend'i de
indirdi. Backend son milisaniyeye kadar sağlıklıydı (16:29:50'de 200 dönen istek,
stderr yakalaması boş). Aynı ölüm 2026-08-12'de de olmuştu (`exit=1073807364`).
Yani **frontend düştü, backend onunla birlikte götürüldü**; 16:25:33'te açılan
`WS18` workspace'i ile ilgisi yok (4 dakika sorunsuz çalıştı).

**İki değişiklik (`scripts/dev.ps1`):**

- **`Get-ExitReason`** — ham exit kodu `lifecycle.log`'a artık `reason=<...>` ile
  çözümlenmiş yazılır (`-1`, `0x40010004`, `0xC0000409`, `0xC0000005` …). Çıplak
  sayı aylar sonra okunmuyordu. .NET `ExitCode`'u **signed Int32** verdiği için
  0x7FFFFFFF üstü NTSTATUS değerleri negatif geliyor; unsigned hex string'e
  normalize edip tek tablodan bakıyoruz. **Tuzak:** maske `0xFFFFFFFFL` olmalı —
  WinPS 5.1 `0xFFFFFFFF`'i Int32 `-1` parse ettiği için maske etkisiz kalıyor ve
  `[uint32]` cast'i tam da çözmek istediğimiz negatif kodlarda patlıyordu.
- **`NODE_OPTIONS=--max-old-space-size=4096`** — Vite çocuğuna heap tavanı. node'un
  varsayılan old-space'i toplam RAM'den türer; HMR modül grafiğini + source map'leri
  canlı tuttuğu için günlerce açık kalan dev server yukarı sürükleniyor (30 saat ve
  5 saat uptime'lı iki ölüm). Tavan, süreç abort'a gitmeden GC'yi zorlar. Zaten set
  edilmiş `NODE_OPTIONS` korunur, yalnız flag eksikse eklenir.

Not: "frontend ölünce backend de ölsün" davranışı **bilerek korundu** (teşhis için
tek kapanış noktası). Frontend-only otomatik restart ayrı bir iş olarak açık.

## DeepSeek fiyatları zam sonrasına güncellendi (2026-08-14) ✅

DeepSeek 2026-08-06'da zammı duyurdu; yeni tarife **2026-08-16 16:00 UTC**'de
yürürlüğe giriyor ve düz oran yerine **peak/off-peak** ikili tarifeye geçiyor
(peak: 01:00–04:00 ve 06:00–10:00 UTC — günde 7 saat; off-peak yarı fiyat).

| Model    | off-peak (tabloda bu var) | peak (2×)     | cache-hit input (off-peak) |
| -------- | ------------------------- | ------------- | -------------------------- |
| V4 Flash | $0.22 / $0.66             | $0.44 / $1.32 | $0.007 (~0.032× input)     |
| V4 Pro   | $0.66 / $1.98             | $1.32 / $3.96 | $0.022 (~0.033× input)     |

Eskisi flash $0.14/$0.28, pro $0.435/$0.87 idi → output tarafında peak'te ~4.5–4.7×,
off-peak'te ~2.3× artış. Cache-hit indirimi de sığlaştı ($0.0028 → $0.007).

**Karar:** `Price` zaman-bağımsız olduğu için tablo **off-peak (normal) oranı**
tutar — günün çoğunluğu. Peak penceresine düşen turlar 2× eksik raporlanır; doğru
çözüm zaman-farkında `Price` (henüz yok, bilinçli borç).

**Dokunulan yerler:** `pricing.go` (`deepseek` tablosu + açıklama bloğu),
`kind_deepseek.go` ve `kind_deepseek_anthropic.go` model açıklamalarındaki fiyat
metinleri. `go build ./...` + provider fiyat testleri yeşil.

## Otomatik artifact yakalama tamamen kaldırıldı (2026-08-14) ✅

Tur sonunda ajanın yazdığı dosyaları taramaca artifact'a çeviren mekanizma
(`AutoCaptureArtifacts` toggle'ı + tarama kodu) **tümüyle silindi**. Artifact
bundan sonra yalnız **bilerek** oluşur: ajan `create_artifact`/`update_artifact`
araçlarını çağırır ya da artifacts API/UI kullanılır. Dosya yazmak (Write/Edit,
shell, script) artifact üretmez.

**Kaldırılanlar.** Backend: `internal/api/artifacts_auto.go` (+ iki test dosyası)
— `captureFileArtifacts`, `parseFileWrite`, `artifactKindForPath`,
`extractProducedMediaPaths` ve yardımcıları; `chat_stream.go`'daki iki çağrı
yeri (normal tur sonu + kesilen/durdurulan tur); artık çağrılmayan
`db.SaveFileArtifact` (source-path'e göre upsert eden depo fonksiyonu);
`workspace.WSSettings.AutoCaptureArtifacts` alanı + default + patch + `UpdateSettings`
kolu; `api.workspaceSettingsDTO.AutoCaptureArtifacts`. Frontend: WorkspacePanel'deki
"Artifact yakalama" toggle'ı ve `WorkspaceSettings(+Patch).autoCaptureArtifacts`
alanı ile WorkspaceView'ün save payload'u.

**Prompt tarafı.** İki varyantlı yönlendirme (`artifactGuidanceFor` +
`artifactDeliverableGuidanceManual`) tek sabite indi: `artifactDeliverableGuidance`
artık koşulsuz "dosya yazmak artifact YAPMAZ, deliverable'ı `create_artifact` ile
bilerek kaydet" der (statik prefix, `chat_turn.go`). `tionharness-deliverables`
skill'indeki "auto-capture kapalı olabilir" notu da bu tek gerçeğe göre yazıldı
(seed hash-ledger'ı düzenlenmemiş kopyaları tazeler).

**Dokunulmayan.** Araç yolu (`agent/artifactsink.go`, `tools` create/update
artifact), chat eki → artifact (`UpsertAttachmentArtifact`), plan artifact'ı,
medya/binary `sourcePath` desteği, koordinatörün taşan worker çıktısını artifact'a
yazması. Diskteki eski `origin:"agent"` artifact'lar olduğu gibi kalır.

`go build ./...` + `go vet ./...` + `go test ./internal/api ./internal/workspace ./internal/db`
yeşil; frontend `tsc --noEmit` temiz.

## "Klasörü aç" (Explorer reveal) butonları tamamen kaldırıldı (2026-08-12) ✅

Uygulama genelinde yol eylemleri **yolu kopyala + klasörü aç** çifti olarak
duruyordu. Açma tarafı artık gereksiz görüldü ve tek seferde silindi; kopyalama
(`CopyPathButton`) her yerde aynen kaldı.

**Kaldırılanlar.** Frontend: paylaşılan `RevealButton` bileşeni ve 8 kullanım
yeri — sohbet başlığı (`AppHeader`), oturum ⚙ menüsü (`SessionsSidebar`), Ajanlar,
Akışlar, Skill'ler, Artifact'lar, Loglar, Workspace dosyaları, Komutlar
(`PromptDetails`) ve Ayarlar'daki "rtk config dosyasını aç". Backend: dokuz
`handleReveal*` handler'ı + route'ları (`agents`/`sessions`/`flows`/`artifacts`/
`skills`/`prompts`/`logs`/`workspace-config`/`rtk-config`) ve karşılık gelen
`api/*.ts` istemci fonksiyonları. Böylece `explorer.exe` çağıran tek yüzey de
kalmadı.

**Dokunulmayan.** `GET .../path` uç noktaları (kopyalama hâlâ onları kullanır) ve
adı benzeşen ama tamamen farklı olan **`GET /api/secrets/{name}/reveal`** (sır
değerinin sahibe gösterilmesi).

**Devamı — `CopyPathButton` ikon-only.** Çift buton gidince etiket taşıma
mekanizmasının (`label` + `labelClassName`) da anlamı kalmadı: her çağrı yeri zaten
`labelClassName="hidden"` ile etiketi gizliyordu. İki prop kaldırıldı, bileşen tek
`<Copy>` ikonu render eder; yol bilgisi başlıkta (tooltip) durur. `PATH_ACTION_CLS`
artık `gap` taşımıyor.

`go build ./...` + `go test ./internal/api` + `npm run build` yeşil.

## Koordinasyon araçları allowlist'ten muaf — canlı testte bulunan sessiz delegasyon kaybı (2026-08-11) ✅

**Canlı koşuda bulundu.** "Ürün Ekibi" şablonu uçtan uca denenirken CTO,
`spawn_worker(agent:"explore", coordinator:true, workflow:"coordinator-wf-plan-dev-test")`
çağırdı. Oturum koordinatör doğdu, prompt'u "worker aç" dedi — ama `worker:explore`
ajanının profil allowlist'i (`Read/LS/Glob/Grep/WebFetch`) koordinasyon yüzeyinin
**tamamını** süzmüştü: 5 araç, ne `spawn_worker` ne `report_to_coordinator`.
Alt-koordinatör ne delege edebildi ne rapor verebildi, **işi kendisi yaptı** — ve
hiçbir şey hata vermedi, çünkü claude-cli yolunda `Read`/`Edit`/`Bash` CLI-native'dir
ve bu süzgeçten geçmez. Delegasyon sessizce buharlaştı.

**Kök neden.** İki gate karıştırılmıştı. Allowlist = profil personasının **iş**
araçları (ajan özelliği). Worker sürme / yukarı rapor borcu = **oturum** özelliği,
zaten `CoordinationFuncs` ile kapılı. Kayıt katmanı o kapıya göre doğru davranıyordu;
allowlist ikinci ve yanlış bir kapıydı.

**Düzeltme.** `toolFilter` koordinasyon araçlarını **yalnız allowlist'ten** muaf tutar
(`tools.IsCoordinationTool`). Workspace anahtarı ve ajanın açık denylist'i aynen
geçerli — "bu ajanda spawn_worker'ı kapat" bilinçli bir karardır. Tek noktada
düzeltildi, dolayısıyla native ve CLI köprüsü birlikte kapsandı (`BridgeTools` da
aynı `toolFilter`'ı kullanır). `spawn_worker` açıklamasına ayrıca "alt-koordinatör
hedefi olarak profil değil var olan bir ajan seç" notu eklendi.

**Testler.** `toolfilter_coordination_test.go` (3): allowlist muafiyeti + iş
araçlarının hâlâ kısıtlı kalması, denylist'in hâlâ kazanması, `CoordinationToolNames`
↔ CLI köprü dispatch'i tutarlılığı (listede eksik bir ad, o araç için hatayı sessizce
geri getirirdi). Muafiyet elle geri alınıp testin gerçekten kırmızıya döndüğü
doğrulandı. Tüm `go test ./internal/...` yeşil. Detay: `_Docs/47` §15.5.

## Koordinatörlük artık bir AJAN varsayılanı + "Ürün Ekibi" şablonu (2026-08-11) ✅

**Teşhis.** `CoordinatorMode` yalnızca `db.Session` alanıydı. Bir workspace şablonu
"bu ajan koordinatördür" diyemiyordu: şablonla gelen ajanla açılan her yeni sohbet düz
bir ajan olarak doğuyor, kullanıcının UI'dan (veya ajanın `set_coordinator_mode` ile)
oturum başına anahtarı açması gerekiyordu. Yani "workspace açılır açılmaz çalışan bir
PM/CTO ekibi" kurulamıyordu.

**Düzeltme — varsayılan/canlı ayrımı.** `db.Agent`'a `CoordinatorMode` +
`CoordinatorWorkflow` eklendi; oturum bunları **doğuşta** alır. Tohumlama
`createSessionLocked`'da, `Model` anlık görüntüsünün hemen yanında (aynı desen: ajan
varsayılanı tutar, oturum canlı değeri). Oturum alanı ve `set_coordinator_mode`
aynen duruyor — bu bir varsayılan, taşıma değil.

- **Ağaç içinde tohumlama YOK.** `CoordinatorSessionID != "" || CoordinatorDepth > 0`
  ise db katmanı karışmaz: orada kararı spawn eden verir, çünkü derinlik bütçesini
  yalnız o bilir.
- **`SpawnWorker` OR kuralı.** Ajan varsayılanı yeteneği **ekler**, açıkça istenmiş
  olanı asla kaldırmaz. Derinlik tavanında **sessizce** düz worker'a düşer — açık
  `coordinator: true` orada hâlâ _hata_ verir; asimetri bilinçli: açık istek cevap
  bekleyen bir taleptir, varsayılan yalnızca bir tercih.
- **Reçete doğrulaması katmana göre.** API create/update bilinmeyen slug'ı **reddeder**
  (etkileşimli düzenleme = yazım hatası görünür olmalı); seed/install'da `resolvableRecipe`
  ile **düşürülür** (paket kendi skill'ini getiremediyse oturum serbest koordinasyonla
  çalışır, kurulum patlamaz).

**`planner` profili (G4).** `explore/coder/reviewer/validator/config` yanına beşincisi:
salt-okunur, çıktısı GOAL/FILES/STEPS/VERIFY/RISKS. `subagent-planner` prompt anahtarı +
`spawn_worker`/`run_subagent` şemalarında ilan. Yanına
`coordinator-wf-plan-dev-test` reçetesi (gömülü skill): planner → coder → validator,
en fazla 2 onarım turu, PASS'siz commit yok.

**Şablon otomasyon taşıyor (G2).** `WorkspacePayload.Automations` +
`seedTemplateAutomations` (6 adımlı seed sırası) + publish simetrisi
(`publishInclude.AutomationIDs`, `Seed != ""` gömülü default'lar hariç). Kural: ajan
**key**'iyle, akış **adıyla** referans verir; çözülmeyen referans seed'lenmez, **atlanır**
— hedefi boş bir pano kuralı her kart taşımasında ateşleyip başarısız olurdu. Hepsi
**kapalı** gelir (starter schedule'larla aynı sözleşme). Bu, gömülü
`EnsureDefaultBoardAutomations` kurallarının şablon ekipleri için neden yetmediğini de
kapatıyor: onlar workspace açılışında, yani şablon ajanları var olmadan önce tohumlanır
→ `TargetAgentID` boş kalır.

**Yeni gömülü şablon.** `workspace.productteam.harnesspack.json` — PM + CTO, ikisi de
koordinatör; `product-team-charter` skill'i; pano sütunları; CTO'ya bağlı
`boardExclusive` bir "kart → geliştiriliyor" kuralı (gömülü default'u bastırır, kapalı
gelir). Zincir: `kullanıcı → PM → CTO → özellik koordinatörü → planner/coder/validator`.

**Testler.** `coordination_agent_default_test.go` (4): yeni oturum tohumu, ağaç-içi
dokunulmazlık, `SpawnWorker` OR kuralı, derinlik tavanında düşürme.
`templates_test.go`: paket bütünlüğüne otomasyon referansı + pinlenmiş reçete kontrolü,
ayrı `TestProductTeamTemplateShape`. Frontend: ajan formunda koordinatör anahtarı +
reçete seçici (`CoordinatorWorkflowPicker` yeniden kullanıldı), roster'da 🕸 rozeti.
`go build ./...` + `go vet` + tüm `go test ./internal/...` + `tsc` + 167 vitest yeşil.
Detay: `_Docs/47` §15, `_Docs/21` §2.1.

## codebase-memory per-workspace store kaldırıldı — cbm 0.10 tek-root kuralı (2026-08-11) ✅

**Teşhis.** PC'deki `codebase-memory-mcp` 0.9.0 → **0.10.1**'e güncellendi. 0.10 ile gelen
koordinasyon daemon'u **hesap başına TEK cache root** dayatıyor: farklı bir root talep eden
ikinci istemci `active account daemon uses a different cache directory` ile reddediliyor.
TionHarness'in workspace-başına store'u (`<workspace>/cbm-store`) bu kuralla bağdaşmıyordu —
uygulama açıkken WS5 ve WS17 birbirini, ayrıca dışarıdaki tüm CBM istemcilerini (CLI,
watcher, External Agent source'u) kilitliyordu; süreç öldürmek yarışı çözmüyordu çünkü
TionHarness sunucuyu anında yeniden doğuruyor.

**Düzeltme.** `CBMStoreDir` + `applyCBMStore` tamamen kaldırıldı; ne native tool döngüsü
(`toolsetup.go`) ne claude-cli config yazıcısı (`climcp.go`) artık `CBM_CACHE_DIR`
enjekte ediyor — sunucu kendi store'unu kullanır, operatörün MCP satırına elle yazdığı
env aynen taşınır. Sistem promptundaki "ISOLATED index store" cümlesi, UI'daki
"izole store" rozeti ve `codebase_workspace_search`'ün `storeDir` parametresi silindi.
`EnsureCodebaseIndexed` artık `--repo-path` bayrağını kullanıyor (0.10 raw-JSON arg'ı
deprecate etti) ve guard'ı `(cwd,store)` yerine `cwd`.

**Testler.** `TestWriteCLIMCPConfigRoutesCodebaseMemoryStore` → tersine çevrilip
`TestWriteCLIMCPConfigKeepsCodebaseMemoryEnv` oldu (yazıcı env uydurmaz, operatör değerini
taşır); `TestCBMStoreDir`/`TestApplyCBMStore` silindi. `go build ./...` +
`go test ./internal/tools ./internal/agent` yeşil. Detay: `_Docs/54`.

## Tek tur kuyruğu: `internal/turnqueue` (2026-08-11) ✅

**Teşhis (SES612).** Koordinatör workerlarını sürerken sıraya eklenen kullanıcı mesajı
UI'dan kayboldu, dakikalar sonra bir worker cevabı gelince sohbet akışına düştü. Aynı
oturumu iki serileştirici yönetiyordu: görünür/kalıcı send-queue (`internal/api`) ve
görünmez `coordSlot` mutex'i (`internal/agent`). Worker head'i **önce** pop ediyor
(tray'den düşüyor), **sonra** slot'ta bloke oluyordu → mesaj ne kuyrukta ne
transkriptte, iptal de edilemez. Kanıt: mesajın `createdAt`'i önceki asistan turunun
bitiş saniyesiyle birebir aynı.

**Refactor.** Kabul sırası bağımsız bir alt katmana taşındı: **`internal/turnqueue`**
— FIFO baton devri, ctx-farkında claim, bekleyen varken barging yok, `Snapshot` ile
gözlemlenebilir; her giriş yolu kendini adlandırır (`user`/`command`/`coordinator`/
`worker`/`wake`/`peer`/`spawn`/`automation`). `coordSlot`'tan `running` kalktı; artık
yalnız koordinatör politikası (`driving`/`pending`/`ackedIdle`/cap/stall). En kritik
sonuç: `drainCoordinator` slot'u döngü boyunca tutmak yerine **her turda yeniden
kuyruğa giriyor** → bekleyen kullanıcı mesajı mevcut turdan sonra koşar; adalet özel
durum değil, yapısal.

API tarafında: worker head'i pop etmeden **önce** slot'u alır (`turnSlotHeld` ile
devreder) → mesaj WAITING'de görünür/iptal edilebilir kalır; ikinci serileştirici
(`acquireInboxSlot` + idle sinyali) tamamen **silindi**, `/compact`+`/handoff` aynı
kuyruğa girer; `queue_update` artık `queue`+`inflight`+`turns` taşır ve runtime
tarafı değişince bus üzerinden yeniden yayınlanır. UI tray'i üç satır gösterir:
"Şu an" (oturumu tutan otonom tur), "Gönderiliyor", "Sırada #N".

**Testler.** `internal/turnqueue/queue_test.go`,
`internal/agent/coordination_fairness_test.go`, `internal/api/inbox_turnslot_test.go`
(sonuncusu eski sırayla kırmızı, doğrulandı). Detay: `_Docs/58`.

## dev.ps1: `go run` sarmalayıcısı kaldırıldı — sahte "backend çöktü" bitti (2026-08-11) ✅

**Teşhis.** 17:29:32'de `lifecycle.log` "backend exited on its own (exit=1)" yazdı, stderr
yakalaması **0 bayt**tı, uygulama logunda `shutting down` **yoktu** ve log normal trafiğin
ortasında kesiliyordu. Ama sunucu ölmemişti: `tionharness.exe` 20 saniye daha istek işledi ve
ancak sonraki koşunun port pre-flight'ında öldürüldü. Sebep yapısal — `go run` araya bir
**go.exe sarmalayıcısı** koyuyor; dışarıdan gelen zorla-sonlandırma sarmalayıcıyı öldürünce
script "çocuk öldü" sanıp Vite'ı indiriyor, gerçek sunucu ise :8090'da **öksüz** kalıyor.
Aynı imza 02.08–11.08 arası **beş kez** tekrarlamış; hepsinde `go run` çocuğun ölümünü çıplak
`exit status 1`'e indirgediği için sebep hiç görülememiş (Go panic olsaydı exit 2 + stack trace
olurdu). Panic/OOM değil, hepsi dış zorla-sonlandırma.

**Düzeltme (`scripts/dev.ps1`):**

- **Derle-sonra-çalıştır:** `go build -o bin\tionharness-dev.exe ./cmd/tionharness` + exe'yi doğrudan
  başlat. Tek süreç → öksüz sunucu ve sahte "çöktü" imkânsız; **gerçek exit kodu ve panic
  trace** stderr yakalamasına düşer. Derleme hatası da Vite başlamadan, net biçimde patlar.
- **Öksüz süpürme:** backend kendi kendine ölmüşse teardown'da `Free-Port 8090` koşar
  (Stop-Tree ölü PID üzerinden torunlara ulaşamaz). Normal Ctrl+C kapanışında **çalışmaz** —
  o an porttaki dinleyici başkasına aittir.
- **Ölüm bağlamı:** dış kill stderr'e hiçbir şey yazmadığından, teardown artık uygulamanın
  kendi log aynasının (`~/.tionharness/logs/tionharness.log`) son 25 satırını da ekrana basar.

`bin/` zaten gitignored. Not: "kim öldürdü" sorusu Windows'ta geriye dönük cevaplanamıyor —
süreç oluşturma/sonlandırma denetimi (4688/4689) kapalı; altıncı olay olursa artık exit kodu
ve son loglar elimizde olacak.

## Derin-çalışma / maksimum düşünme tiyerleri claude-cli'ye geçirildi (2026-08-11) ✅

"Maksimum düşünme modunu aç, saatlerce derin çalış" isteğinin çekirdek engeli: UI zaten
`xhigh`/`max` sunuyordu (`THINKING_OPTIONS`) ama backend `cliEffortLevel` bunları `high`'a
**kırpıyordu** — claude-cli tavanı `high`'da kalıyordu. Düzeltildi:

- **`cliEffortLevel`** (`internal/agent/climcp.go`): `xhigh`→`xhigh`, `max`→`max` artık geçer;
  `""`/`off`/`high`/bilinmeyen yine `high` (batch-koruma). Bu tiyerlerde thinking açık →
  paralel araç batch'i kapanır ("think XOR batch"), derin akıl yürütme için kabul edilen takas.
- **`max` özel yolu:** Claude Code'un `settings.json` enum'u `max`'i reddedip sessizce `high`'a
  düşürür (web-doğrulandı: anthropics/claude-code #65651, #52247). Bu yüzden `writeCLISettings`
  `max` turunda dosyaya `xhigh` (taban) yazar ve provider `runAttempt` turu
  `CLAUDE_CODE_EFFORT_LEVEL=max` env'i ile `max`'a yükseltir. Taşıyıcı: yeni
  `Request.CLIEffortLevel` (`toolloop.go` `isCLI` seam'inde set edilir).
- **Testler:** `TestCLIEffortLevel` (xhigh/max passthrough), yeni `TestWriteCLISettingsClampsMaxToXhigh`.
- **UI/doküman:** AgentSettingsForm yardım metni + `_Docs/07` güncellendi.

> Not: uzun-soluklu deliberate döngü (plan→araştır→eleştir→rafine→artifact) için ayrı bir
> "Deep Work" skill/flow preset'i henüz eklenmedi — mevcut coordinator+validator+todo ile
> yapılabilir; effort passthrough o presetin ön-koşuluydu.

## Pending balonunda ajan başlığı (2026-08-11) ✅

Asistanın ilk token'ı/`AgentStart`'ı gelene kadar gösterilen standalone "çalışıyor"
balonu (WorkingDots) ajan başlığını (avatar+ad+model) taşımıyordu → kimlik yalnız yanıt
akmaya başlayınca görünüyordu (worker session'larında en belirgin). `ChatView` artık
`MessageList`'e `pendingAgentId={activeAgentId}` geçiyor (session seçilince
`sess.agentId`'e set edilir, worker dahil) → pending balon `AgentHeader`'ı `AgentStart`
öncesinde de çizer. Ghost balon zaten `AgentStart`'tan `agentId` alıyordu; boşluk yalnız
pending penceresiydi. `frontend/src/features/chat/ChatView.tsx`.

## Built-in tool tanımları denetimi + düzeltmeler (2026-08-11) ✅

Tüm built-in tool `Def()`'leri (description/InputSchema/default) `Call()` gerçek
davranışına karşı iki turda denetlendi (`internal/tools/builtin_*.go`). Bulunan
tutarsızlıklar düzeltildi. **Fonksiyonel hatalar:**

- **`shell_manage` yeniden adlandırma boşluğu:** Bash/PowerShell açıklamaları ve arka-plan
  shell hata/durum mesajları hâlâ artık var olmayan `shell_output`/`shell_kill`/`shell_list`
  araçlarını söylüyordu (ajan çağırınca "no such tool"). Hepsi `shell_manage (action=…)`
  olarak düzeltildi (`builtin_shell.go`, `builtin_shell_bg.go`).
- **`Read` satır kırpması byte kesiyordu:** 2000 sınırında `line[:2000]` çok-baytlı UTF-8
  karakterini ortadan bölüp geçersiz bayt üretiyordu (tool'un "unicode aynen" garantisini
  çiğniyor) → rune-tabanlı kırpmaya çevrildi.
- **`update_automation.expiresAt`** şemada yoktu → `additionalProperties:false` yüzünden
  erişilemezdi; şemaya eklendi.
- **`read_lessons.limit`** `0=all` belgesine aykırıydı (`0→20`); `*int` ile "atlandı→20,
  0→hepsi" yapıldı.
- **`update_schedule.cronExpr`** boş set edilebiliyordu (create ile tutarsız) → boş reddedilir.
- **`render_template.data`** şemada `required` ama zorlanmıyordu → nil kontrolü eklendi.
- **`update_session.working_dir`** "absolute" diyordu ama relative kabul ediyordu →
  `filepath.IsAbs` kontrolü.

**Belge/şema tutarsızlıkları** (davranış değişmez): `glob` .git/no_ignore ifadesi;
`list_sessions`/`list_artifacts`/`read_session_debug` enum'larına eksik türler;
`update_flow`(emoji)/`update_schedule`(name)/`update_automation`(alan listesi)/`create_task`
açıklama eksikleri; `create_automation` "Three→Four" + counter `{{scope}}` + board
placeholder + spawnTags `[]` yanlışı; `request_confirmation` `ambiguous:`; `run_flow`
await-input; `list_flow_runs` status enum; `insight_apply_finding` new/triaged; `read_logs.q`
kapsamı; `apply_patch` tüm-patch atomikliği; `create_mcp_server` shared scope.

**Belgelenmemiş sessiz limitler** ilgili açıklamalara eklendi (grep 200, glob 500, Read
2000-char/satır, shell timeout ayar-bağımlı, LS 1000, codebase 40-proje, list_schedules
one-shot eleme, tool_search 30); grep `files_with_matches`/`count` modlarına da **yalnız
cap aşılınca** çıkan truncation işareti eklendi.

Kalan bilinçli-bırakılan: `create_automation` spawnTags `[]` loop-break'i gerçekten çalışsın
diye db katmanı değişikliği (nil↔empty ayrımı) ayrı iş olarak bırakıldı; `spawn_worker`'ın
gizlice kabul ettiği `config` profili (fan-out için tasarlanmadığından belgelenmedi).

## Salt-okunur oturumlarda worker izleme yüzeyleri (2026-08-11) ✅

Salt-okunur oturumlar (task/flow/schedule **ve worker** günlükleri) composer'ın tüm
alt yığınını gizliyordu; bu yığında canlı `TodoPanel`, park edilmiş `ask`/permission ve
worker roster'ı da vardı — dolayısıyla worker ekranında yalnız transkriptteki donmuş
inline `TodoCard`'lar görülüyor, worker sessizce girdi bekleyebiliyordu. `ChatView.tsx`'in
`readOnly` dalı flex-col yığına çevrilip aşağıdaki yüzeyler eklendi (çoğu mevcut
component'in yeniden kullanımı):

- **Görünürlük (A):** `TodoPanel` (canlı `todo_write` ilerlemesi) + park edilmiş
  `AskPrompt`/`PermissionPrompt`/`PlanPrompt` (yanıt = suspend çözümü, yeni tur değil) +
  streaming sırasında yeni **`WorkerStatusStrip`** ("Worker çalışıyor").
- **Navigasyon (B):** mevcut **`CoordinatorBreadcrumb`** (üst-koordinatör zinciri;
  root/sıradan oturumda kendini gizler). Yeni **`floating`** varyantı chat yığınında
  `ComposerCard` zemini kullanır (kardeş panellerle aynı opak yüzey; yan panel düz kalır).
- **Kontrol (C):** `WorkerStatusStrip` içindeki **Durdur** (`chat.stopTurn`, run'ı sunucu
  çözer; yalnız koordinatör-ağacı üyeleri) + `WakeWaitBanner`'ın yeni **`hideCancel`**
  varyantı (schedule self-wake'ini salt-okunurdan iptal ettirmez).
- **Navigasyon (B#4):** flow koşu log'unda **"Koşu geçmişini aç"** linki →
  `useDeepLinks.openFlowRun(sourceId)` (flow session'ın `sourceId`'si = flow id;
  Flows ekranını o flow'un koşularında açar). `openFlowRun` tanımlıydı ama hiç
  çağrılmıyordu; App tek `activeSession.find` ile `onOpenRunHistory`'yi türetip
  `ChatView`'e geçiriyor, link yalnız flow oturumunda çözülür.
- **`WorkerWaitBanner`** self-gated (roster yalnız oturum kendisi koordinatörse dolu).
- Dışarıda: `PendingTray`, `CacheWarmthStrip`, `Composer`.

## Chat ekranında prompt-cache kırılım görünürlüğü (2026-08-11) ✅

Kırılım tespiti/atfı (`cachebreak.go`, `_Docs\50` P4) 2026-07-05'ten beri vardı ama
yalnız oturum-seviyesi Debug kartında görünüyordu — **mesaj başına hiç yoktu**
(`GetTurnDebug` `cache_break` olayını hiç toplamıyordu, oysa olaylar `TurnID`
damgalı). Sohbete beş yüzey eklendi; her biri farklı bir soruya cevap verir:

- **`CacheWarmthStrip`** (composer üstü) — tek ÖNLEYİCİ yüzey: sıcak pencerenin geri
  sayımı + oturumun soğuk tur sayısı. Backend alanı yok (son mesajın `createdAt`).
- **`ColdCacheDivider`** — transkriptte 1sa TTL'i aşan boşluğa ayraç ("❄️ cache soğudu · 3 sa ara").
- **`CacheWarmthDot`** — tur altbilgisinde 🔥/❄, `Message.usage`'tan; transkript taranabilir olur.
- **`CacheBreakCard`** (`cache_break` TurnStep) — sebep + gerekçe + aksiyon. **Yalnız**
  `model-changed` / `prompt-or-tools-changed` kart olur (`inlineCacheBreak`); TTL soğuması
  normaldir, her molada kart basmak kullanıcıyı karta kör ederdi.
- **`MessageDebugPanel`** — atıflı sebep + kaçınılabilir fazla ödeme (`coolingWasteUsd`).

Tasarım kararları: (1) kart **canlı SSE ile yayılmaz**, kalıcı izin BAŞINA eklenir —
kırılım turun başında ödenir ama ancak provider yanıtından bilinebilir; geç yayınlamak
kartı akışın altına çizip reload'da yukarı zıplatırdı. (2) **Kanıt yoksa iddia yok**:
🔥/❄ ve soğuk-tur sayacı yalnız `cacheRead`/`cacheWrite` varken konuşur (OpenRouter
soğuk öneki düz `input` olarak faturalar → orada sessiz). (3) İlk tur soğuk sayılmaz
(backend `warmed` koşuluyla aynı). (4) `prompt-or-tools-changed` kartı şüpheli tonda:
prompt epoch açıkken bu oturum ortasında olmamalı → epoch regresyonu artık Debug kartı
açılmadan fark edilir (`_Docs\57`).

Dosyalar: `internal/db/debug_journal.go` (TurnDebug += 5 alan + `DebugCacheBreak` dalı),
`internal/agent/{cachebreak,trace,runtime}.go` (`CacheBreak` + `pendingCacheBreaks` +
`StepCacheBreak`/`ColdTokens`), `internal/api/{chat_turn,chat_stream}.go`
(`consumeCacheBreakLead`), frontend `features/chat/{CacheBreakCard,CacheWarmthStrip}.tsx`

- `MessageMeta`/`MessageList`/`TurnSteps`/`AssistantTurn`/`MessageDebugPanel`/`ChatView`
- `shared/stepKinds.ts`. Testler: `TestGetTurnDebugCacheBreak`, `TestInlineCacheBreak`,
  `TestConsumeCacheBreak`, `TestCacheBreakStep`. Go suite + tsc + vitest (167) + build yeşil.
  Detay: `_Docs\50` P7, `_Docs\07` "Prompt-cache görünürlüğü".

**Devamı — Insight cache lensleri (aynı gün):** mevcut `context-cache-opt` lensi
`cache_break ≥ 2` ile prefilter'dan geçiyor ama `buildSlice` cache olaylarını **hiç
yazmadığı** için analize kanıtsız dilim gidiyordu; ayrıca `Lens.Scope` frontmatter'da
vardı ama hiçbir yerde kullanılmıyordu. Üçü birden düzeltildi: (1) `ScopeCache`
("cache") ile opt-in "## Prompt-cache events" bölümü — `cause`/`coldTokens`/`wasteUsd`

- `at=` damgası + araya `epoch` olayları (adopt'suz kırılım kuralı ancak böyle
  uygulanabilir); (2) `extractSignals` sebebi ayrı sinyal olarak indeksler
  (`cache_break:<cause>`, `cooling_waste`) — bu da `parseMinCount`'un ilk-iki-noktadan
  bölme hatasını açığa çıkardı (bileşik anahtar bozulup eşik sessizce düşüyordu → lens
  her oturumu eşlerdi), son-iki-noktadan bölmeye geçildi; (3) yeni **`cache-cooling-waste`**
  lensi TTL soğumasını _tempo_ problemi olarak ele alır (schedule aralığı, oturum ömrü,
  prefix boyu), `context-cache-opt` ise yalnız `prompt-or-tools-changed`'e bakar. Testler:
  `TestBuildSliceCacheScope`, `TestExtractSignalsCacheCause`, `TestParseMinCountCompoundKey`,
  `TestEmbeddedDefaultsAllParse` (7 lens + scope + bileşik minCount). Detay: `_Docs\60` Faz 6.3.
  **Devamı — shipped-defaults tazeleme (`internal/seed`, aynı gün):** yukarıdaki iş bir üst
  sorunu açığa çıkardı — `insight.EnsureDefaults` "dosya varsa dokunma" dediği için lens
  düzeltmeleri mevcut kurulumlara **hiç ulaşmıyordu**; uygulamayı güncellemek lensleri
  güncellemiyordu. Skills'te zaten çalışan "shipped-hash ledger" deseni paylaşılan
  **`internal/seed`** paketine çıkarıldı ve insight ikinci tüketici oldu.

Çekirdek fikir: _"kullanıcı düzenledi mi?"yi tahmin etmek yerine ne gönderdiğimizi kaydet._
`.shipped-versions.json` her dosya için tüm-dosya (`Files`) ve yalnız-gövde (`Bodies`) sha256'sı
tutar → "dokunulmamış" kanıtlanabilir olgu olur, güvenle tazelenir. `Bodies` şart: uygulamanın
KENDİSİ frontmatter'ı yerinde yazıyor (skills'te görünürlük, lenste `enabled` toggle'ı), tüm-dosya
hash'i bir daha tutmaz; gövde ledger'ı olmasa **tek toggle dosyayı sonsuza dek dondururdu.**

Lens merge politikası skills'ten **kasıtlı olarak farklı**: skills tüm frontmatter'ı korur, lens
yalnız `enabled` + `model`'i taşır, `prefilter`/`scope`/`channel`/gövdeyi gönderilenden alır —
çünkü lens frontmatter'ı ağırlıkla *mekanik*tir ve onu korumak bugünkü prefilter/scope
düzeltmelerini kalıcı dondururdu. `SetFrontmatterEnabled` → `SetFrontmatterScalar` (yalnız
üst-seviye anahtar; girintili satır nested `prefilter:` bloğunun).

Kaçınılmaz sınır: ledger'dan ÖNCE gönderilmiş ve o gün bugün değişmiş dosya kullanıcı
düzenlemesinden ayırt edilemez → `Ensure` dokunmaz. Çıkış kapısı: `seed.Restore` +
`POST /api/insight/lenses/{id}/restore` + lens satırında iki adımlı **"Varsayılan"** butonu
(yalnız `hasDefault` olanlarda). Restore manifest'e de yazar → **otomatik tazelemeyi yeniden
kurar.**

**Ledger görünür + skill paritesi:** `seed.Status` üç durum döner — `default` (dokunulmamış) ·
`tuned` (yalnız config farklı, **yine otomatik tazelenir**) · `edited` (içerik değişmiş →
**donmuş**). Lens ve skill DTO'larında `defaultState`; paylaşılan `SeedDefaultBadge` **yalnız
`edited`**'i rozetler — diğer ikisi güncelleme almaya devam ettiği için onları rozetlemek her
satıra bilgi vermeyen bir çip koyardı. Skill listesine de aynı "Varsayılan" butonu geldi
(`POST /api/skills/{slug}/restore`, paylaşılan `RestoreDefaultButton`), yalnız **global tier**
için: workspace-tier override başka bir dosyadır, onun "varsayılanını" yazmak kullanıcının
bakmadığı dosyaya yazmak olurdu.

Testler: `internal/seed/seed_test.go` (9 senaryo — merge sonrası donmama + **`Status`'ün
`Ensure`'ün gerçekte yaptığıyla tutarlılığı**, aksi halde rozet yalan söyler), insight'ta 5,
skills'te 2 test. Detay: `_Docs\60` Faz 6.4.

## codebase-memory: ölü onarım guard'ı + izolasyon kaçağı + otomatik onarım (2026-08-11) ✅

Bir worker oturumunun tek `get_code_snippet` çağrısı `project` argümanı olmadan
gitti, sunucu `"project not found or not indexed"` döndürdü, ajan MCP'yi arızalı
sanıp 10 kez Grep'e düştü — oysa repo indeksliydi. İz sürünce üç ayrı arıza çıktı.

- **`mcpRepair` hiç tetiklenmiyordu.** `mcpToolPrefix = "mcp__"` arıyordu; yerel
  ajan döngüsünde araç adları `mcp.NamespaceTool` ile `<server>__<tool>` üretiliyor.
  `precheck`/`repair` ilk satırda her çağrıyı eliyordu. Testler yeşildi çünkü adı
  elle `mcp__…` diye yazıyorlardı — üretimde var olmayan bir ad. Artık
  `isMCPToolCall` (`__` ayracı) her iki biçimi eşliyor ve testler adı
  `mcp.NamespaceTool` ile üretiyor.
- **claude-cli yolunda store izolasyonu yoktu.** `CBM_CACHE_DIR` yalnız
  `toolsetup.go`'da enjekte ediliyordu; `climcp.go` (`--mcp-config`) etmiyordu.
  Sonuç: claude-cli sağlayıcılı her oturum sunucunun global default store'una
  bağlanıyor, workspace'in `cbm-store`'undaki indeksi göremiyordu — sistem promptu
  ise "ISOLATED index store" diyordu. Enjeksiyon tek kaynağa alındı
  (`(*Runtime).applyCBMStore`), iki launch yolu da onu çağırıyor.
- **Hata mesajı iki farklı arızayı aynı cümleyle anlatıyordu.** Eksik argüman ile
  indekslenmemiş repo ayrıldı; eksik argüman artık "repo indeksli değil" demiyor.

Eklenen koruma:

- **Şema kapısı (`internal/agent/mcpargs.go` — yeni):** giden MCP çağrısı,
  sunucunun `inputSchema.required` listesine karşı **gönderilmeden önce** kontrol
  edilir. Eksik `project` oturumun working directory'sinden doldurulur; başka bir
  eksik alan varsa çağrı hiç gönderilmez ve model yerel bir şema hatası alır.
  Şema/`required` yoksa kapı hiçbir şey yapmaz — tahminle bloklama yok.
- **Otomatik onarım:** `repair()` artık `repairPlan` döndürür. Düzeltilebilir
  kimlik (çıplak repo adı, yol, harf farkı) çağrı **bir kez yeniden koşturularak**
  onarılır; belirsizlikte tahmin edilmez. Çağrı başına tek düzeltme.
- **Otomatik indeksleme:** oturumun reposu `available_projects` içinde yoksa
  `EnsureCodebaseIndexed` tetiklenir; yönerge indeksin bu tur içinde hazır
  olmayacağını açıkça söyler (turu bloklayıp beklemek yerine).
- **Sınır:** claude-cli sağlayıcısında araç döngüsünü CLI koşturur, çağrılar
  TionHarness'ten geçmez — şema kapısı ve otomatik onarım orada devreye giremez.
  O yolda kazanılan tek şey doğru store'a bağlanmaktır.

Dosyalar: `internal/agent/mcprepair.go` · `mcpargs.go` (yeni) · `toolloop.go` ·
`capabilities.go` (`applyCBMStore`) · `climcp.go` · `toolsetup.go` ·
`internal/tools/registry.go` (`MCPSchema`). Testler: `mcprepair_test.go` (yeniden
yazıldı), `mcpargs_test.go`, `registry_mcpschema_test.go` (yeni),
`climcp_test.go` + `capabilities_test.go` (regresyon kalkanları).
Doğrulama: `go build ./...` ✅ · `go vet ./...` ✅ · `go test ./... -count=1` ✅
(33 paket, 0 FAIL). Teknik not: `_Docs/11-INTERACTION-MCP.md`, `_Docs/54-CAPABILITY-PROBE.md`.

## View katmanı: tek projektör kurulumu + tek sayım (2026-08-10) ✅

Explorer ekranı ile `get_view` zaten aynı motoru (`internal/view`) kullanıyordu;
ortaklaştırılmamış olan **kurulum** ve **sayım** idi. Dört çağrı yeri kendi
projektörünü kuruyor, biri hariç hiçbiri opsiyonel kaynakları bağlamıyordu.

- **`tools.ViewProjector(db, wsName, ViewSources{Skills, Logs})`** — projektörün
  kurulduğu tek yer (`internal/tools/viewprojector.go`); findings adapter'ı da
  buraya taşındı. `api/views.go`, `api/dashboard.go`, `get_view`, `expand`
  dördü de bunu çağırır. `tools` paketi seçildi çünkü skills+insight+logbuf+view'ı
  birlikte import eden tek paket o (`view` yapamaz: insight → view döngüsü).
  **Kapattığı iki hata:** ajanın `get_view{skill|insight|logs}` çağrısı "kaynak
  yok" derken kullanıcı aynı düğümü Harita'da dolu görüyordu; Panel ekranı
  workspace başlığını isimsiz basıyordu.
- **`get_view` kind listesi** `expand`'in ref verdiği her düğümü kapsıyor
  (`agent`/`budget`/`tools`/`logs`/`artifact`/`automation`/`skill`/`insight`/
  `category`). Şema beş kind'a kapalıyken ajan `expand`'den aldığı ref'i açamıyordu.
- **`view.CountWorkspace` + `Projector.Workspace`** — Panel'in stat kutuları
  `dashboardCounters`'ın ayrı döngüsüyle sayılıyordu; silindi. Metin ve sayaçlar
  artık **tek yüklemeden, tek saatle** üretiliyor.
- **Kategori düğümünde `full` işe yaramıyordu:** `ProjectCategory` üyeleri yalnız
  `Handle` olarak veriyordu, dolayısıyla `card` ile `full` aynı metni üretiyordu ve
  Harita'daki `full` düğmesi hiçbir şey değiştirmiyordu. Artık `full` üyeleri satır
  satır yazar (2400B cap + `Elided`). Kalan küçük yapraklarda `card` = `full` kasıtlı.
- **Kopyala butonu** özdeş kademeleri birleştirir (`[CARD = FULL — …]`) — üç özdeş
  blok "seviyeler yok sayılmış" gibi okunuyordu.
- **Regresyon testleri:** `TestDashboardSummaryMatchesWorkspaceView` (`asOf`
  hariç bayt-eşitlik), `TestDashboardCountersComeFromTheProjection`,
  `TestCategoryFullListsMembersCardDoesNot`,
  `TestCategoryFullElisionAddsToTopNOverflow`.

**Doğrulama:** `go build ./...` ✅, `go test ./internal/... -count=1` ✅ (33 paket),
`tsc --noEmit` ✅, `npm test` ✅ (167 test).

## Fullstack review düzeltmeleri (2026-08-10) ✅

Son 17 commit'in (schedule adları, AI başlık üretimi, per-workspace varsayılan
ajan, `expand`, tombstone yayını) uçtan uca incelemesi. Bulunan 9 hata düzeltildi.

**P0 — `anthropic` provider'da her araç turu 400 riski.** `contentBlock` tek bir
etiketli birlik; `Text` alanından `omitempty` kaldırılınca **tool_use ve
tool_result** blokları da `"text":""` göndermeye başlamıştı ve API tanınmayan
alanı reddeder. Alan `omitempty`'ye geri alındı; bunun yerine tip-farkındalı
`contentBlock.MarshalJSON` eklendi — `text` bloğunda alan (boş olsa da) **her
zaman** yazılır, diğer tiplerde **hiç** yazılmaz (`internal/providers/anthropic.go`).

**P1 — Sayfalama eşit anahtarlarda satır atlıyordu.** `SortByField`'da tiebreak
yoktu; `db.dbList` map üzerinde dolaştığı için eşitlikler her çağrıda farklı
sıralanıyordu ve zaman damgaları saniye çözünürlüklü olduğu için eşitlik yaygın.
`offset=0` ve `offset=20` ayrı çağrılar olduğundan okuyucu satır kaçırıp
başkalarını iki kez görebiliyordu. Artık **zorunlu** `id` tiebreak parametresi var
(20 çağrı yeri güncellendi); asc/desc birbirinin tam tersi.

**P1 — AI başlık üretimi hatayı yutup çöp veriyi yazıyordu.** `TitleFor` asla ""
döndürmez, hatada `FallbackTitle(source)` döner — yani LLM patlayınca ham
"Schedule: cron=… prompt=…" metni isim olarak diske yazılıyor, UI ise başarı
gösteriyordu. Artık hata 502 ile yüzeye çıkıyor. Ayrıca uç noktalar **öneri-only**
oldu (`{title}` döner, yazmaz): buton bir düzenleme modalının içinde, yazmak
kullanıcının onaylamadığı ismi kalıcı kılıyor ve İptal ile geri alınamıyordu.
`SetScheduleName`/`SetAutomationName` kaldırıldı.

**P2 — Zamanlama ismi silinemiyordu:** `UpdateSchedule`'daki `if sc.Name != ""`
guard'ı ve frontend'in `|| undefined`'ı birlikte "ismi temizle"yi sessiz no-op
yapıyordu. İkisi de kaldırıldı.

**P2 — Ayarlarda aktif `titleModel` görünmez olmuştu:** model alanı yalnız
sağlayıcı override'ı seçiliyken render ediliyordu, ama ayar tek başına yürürlükte.
Artık her durumda görünür. Ek olarak select `models[0]`'ı seçili gösterirken kayıtlı
değer "" idi (form durumu yanlış anlatıyordu) — artık değer birebir; sağlayıcı
değişince model sıfırlanıyor.

**P2 — Okuma hatası kullanıcı tercihini eziyordu:** `getWorkspaceSettings`
başarısız olunca self-heal devreye girip varsayılan ajanı `agents[0]` ile kalıcı
olarak değiştiriyordu. `wsSettingsLoaded` bayrağı ile geçici okuma hatası artık
yazmaya dönüşmüyor; workspace değişiminde seçim de sıfırlanıyor.

**P3:** `expand` çıktısı sınırsızdı (olgun bir workspace'te her oturum bir satır) →
`view.CapLines` ile 4KB'a bağlandı, elenen sayı açıkça yazılıyor. View
başlıklarındaki kırpma kaybı geri alındı (kart başlığı 100, schedule/workspace adı
60). `internal/web/dist/.gitkeep` takibe alındı — takip edilmediği için temiz bir
clone'da `go:embed all:dist` derlemeyi kırıyordu.

**Doğrulama:** `go build` ✅, `go vet` ✅, `go test ./...` ✅, `tsc --noEmit` ✅.
Yeni testler: `providers/anthropic_toolsearch_test.go` (blok tipi başına `text`),
`tools/listpage_test.go` (tiebreak determinizmi + geliş sırasından bağımsızlık +
zorunlu `id`), `tools/builtin_list_test.go` (gerçek `Name` sıralaması).

## TSK68: Liste araçlarına pagination + filtreleme + sıralama (2026-08-06) ✅

**Ne:** Tüm liste araçları (`list_agents`, `list_tasks`, `list_flows`,
`list_automations`, `list_schedules`, `list_workers`, `list_workspaces`) artık ortak
`limit`/`offset`/`sort` sözleşmesini kullanıyor ve her yanıt standart
`{items,total,offset,limit,hasMore}` zarfında geliyor. Agent'lar kalabalık listeleri
context'i tüketmeden sayfalayabilir (`hasMore` → `offset += limit`).

**Nasıl:**

- **Paylaşılan kontrat** (`internal/tools/listpage.go`): `PageArgs` (limit≤0 → 20,
  > 100 → 100, offset<0 → 0), `SlicePage` (filtre-sonrası sayfalama), `SortOrder`
  > (updated/created/name × asc/desc; bilinmeyen anahtar = **açık hata**, sessiz düşüş yok),
  > `SortByField` (per-entity getter'larla tip güvenli karşılaştırıcı), `SplitTags`/
  > `HasAllTags` (AND semantiği). `pageResult` zarfı marshal'lar.
- **7 tool** aynı desende: filtre → sırala → sayfala → zarf. Filtreler:
  `list_agents` (provider/model substring + state enabled|disabled), `list_tasks`
  (boardState/priority/ownerAgentId/tags), `list_flows` (tags; **flow modelinde
  Enabled yok** — flow'lar disable edilmez, bu yüzden enabled filtresi uygulanamaz),
  `list_automations` (enabled/triggerKind/targetAgentId), `list_schedules`
  (enabled/agentId; one-shot wake'ler gizlenir; name_* gerçek `Schedule.Name`
  alanına göre sıralar — 2026-08-10'dan beri),
  `list_workers` (state running|idle|stuck; yalnız koordinatör oturumunda),
  `list_workspaces` (yalnız pagination; updated_* → created_* belgelenmiş vekil).
- **API tarafı** (`internal/api/listparams.go`): `listQueryParams` (limit/offset/sort
  - `listing` bayrağı — parametre yoksa **legacy davranış: düz dizi**, UI bozulmaz),
    `boolQuery` (enabled=1/0/true/false), `pageJSONResponse`. Uygulandığı endpoint'ler:
    `/api/agents`, `/api/tasks`, `/api/flows`, `/api/automations`, `/api/schedules`,
    `/api/workspaces`. (`list_workers` bir REST kaynağı değil — koordinatör oturum içi
    durum; HTTP endpoint'i yok.)
- **Bug fix:** `list_workers` sıralaması filtrelenmemiş `rows` üzerinden karşılaştırıcı
  kuruyordu — filtre aktifken (örn. `state=running&sort=name_asc`) yanlış elemanlar
  karşılaştırılıyordu. Artık `SortByField(matches, …)` + regresyon testi.
- **Ek düzeltme (review sonrası):** (1) API listing'lerinde `sort` verilmezse
  artık araç katmanıyla aynı varsayılan (`updated` desc) dönüyor — aynı müşteri hem
  API'den hem tool'dan sayfalarsa sıralama tutarlı kalıyor (`internal/api/listparams.go`).
  (2) `list_workers` girdiyi iki kez parse ediyordu (önce Scope-only struct, sonra
  ayrı struct); tek struct + tek `json.Unmarshal`'a indirildi
  (`internal/tools/builtin_coordination.go`) — boş/whitespace girdi artık tutarlı
  şekilde varsayılan demek.

**Doğrulama:** `go build` ✅, `go vet` ✅, `go test ./... -count=1` ✅ (yeni:
`tools/listpage_test.go` kontrat birim testleri, `tools/builtin_list_test.go` 7 tool
testi — filtre+sort kombinasyonu dahil, `api/listparams_test.go`). Frontend etkilenmedi
(paramsız çağrılar legacy düz-dizi alıyor).

### Kapsam genişletmesi (2026-08-06, TSK68 sonrası) ✅

**Ne:** Aynı `PageArgs`/`pageResult` kontratı kalan 4 list aracına da taşındı +
frontend'de büyük listeler için gerçek "load more".

**Nasıl:**

- **Tools — 4 araç kontrata geçti:**
  - `list_artifacts` (`internal/tools/builtin_artifactmgmt.go`): `limit/offset/sort`
    - `sessionId`/`kind`/`origin` filtreleri; name = başlık.
  - `list_hooks` (`internal/tools/builtin_hookmgmt.go`): `limit/offset/sort` +
    `event` filtresi; hook'lar değişmez olduğundan `updated_*` → oluşturma zamanı
    (belgelenmiş vekil), `name_*` açık hata (name alanı yok).
  - `list_mcp_servers` (`internal/tools/builtin_mcpmgmt.go`): `limit/offset/sort` +
    `transport` (substring)/`enabled` filtreleri; `updated_*` → oluşturma zamanı vekili.
  - `list_sessions` (`internal/tools/builtin_sessions.go`): mevcut `state`/`kind`
    filtreleri + pagination korundu; `sort` eklendi; limit artık `PageArgs`
    normalizasyonundan geçiyor (max 100). Çıktı bilinçli olarak plain-text kaldı
    (satır başı `[kind·state]` + `Showing X–Y of Z` + `offset:` ipucu — mevcut
    ajanların alışık olduğu format; JSON zarfına geçilmedi).
- **API — 3 endpoint kontrata bağlandı** (`internal/api/`): `/api/artifacts`
  (ek filtreler: `kind`, `origin`, `q`=başlık substring, `archived` bool),
  `/api/sessions` (`kind`, `state`), `/api/executions`. Hepsi `listQueryParams` +
  `pageJSONResponse`; parametre verilmezse legacy düz-dizi davranışı korunur → UI bozulmadı.
- **Frontend — Artifacts ekranında "load more"** (`frontend/src/features/artifacts/ArtifactsPanel.tsx`):
  filtreler (başlık araması, origin, arşiv görünümü) **server'a taşındı** (pagination
  ile tutarlılık için), 50'şer sayfa yüklenir, `Daha fazla yükle (X/toplam)` butonu
  `offset += 50` ile ekler; `total`/`hasMore` server'dan. Arşiv rozet sayısı ayrı hafif
  istekten (`archived=true&limit=1` → total). Yeni `shared/hooks/useDebouncedValue.ts`
  (300ms) — arama kutusu her tuşta istek atmaz. `api/artifacts.ts` her çağrıyı
  `ArtifactPage` zarfına normalleştirir (paramsız çağrı legacy diziyi client'ta sarar);
  `useSessionsController`/`TaskFormModal` çağrıları `r.items` ile güncellendi.
- **Dokunulan dosyalar:** `internal/tools/builtin_{artifactmgmt,hookmgmt,mcpmgmt,sessions}.go`,
  `internal/tools/builtin_{list,sessions}_test.go`, `internal/api/{artifacts,sessions,executions}.go`,
  `frontend/src/{api/artifacts.ts, features/artifacts/ArtifactsPanel.tsx,
app/useSessionsController.ts, features/tasks/TaskFormModal.tsx, shared/hooks/useDebouncedValue.ts}`.
  (Not: ilk `tsc -b`'de `App.tsx:678` tip hatası göründü — working tree'deki BAŞKA işin
  `api/agents.ts` (P1.3, `{agent, warning?}` dönüşü) değişikliği `AgentsView` prop'uyla
  uyumsuzdu; o iş `AgentsView.tsx`'i eşzamanlı güncellediği için ikinci koşuda temizdi —
  bu kapsamda o dosyaya dokunulmadı.)

**Doğrulama:** `go build` ✅, `go vet` ✅, `go test ./internal/... -count=1` ✅
(yeni: `TestListArtifactsPaginationAndSort`, `TestListHooksPaginationAndSort`,
`TestListMCPServersPaginationAndSort`, `TestListSessionsToolSort`); frontend
`npx tsc -b` ✅ + `npm run build` ✅.

### Sessions load-more (TSK68 kapsam genişletmesi devamı, 2026-08-07) ✅

**Ne:** SessionsSidebar + SessionsOverview artık tam listeyi tek seferde çekmiyor;
sidebar `/api/sessions` üzerinden 100'erlik sayfalarla yükleniyor (backend
`maxPageLimit` = 100, sessiz klamp olmaması için istemci aynı değeri kullanır),
altta `Daha fazla yükle (X/toplam)` butonu sonraki sayfayı ekliyor. Overview
tablosu render-side sayfalıyor (veri zaten tam bellekte — runtime map'i için
gerekli): filtre/sıralama tüm listede çalışır, yalnız DOM satırları 50'şerlik
pencerede, `Daha fazla yükle (X/Y)` pencereyi büyütür; filtre değişince pencere
sıfırlanır.

**Nasıl:**

- `useSessionsController.ts`: `SESSIONS_PAGE_SIZE = 100`; ilk yükleme + refresh
  aynı pencereyi (`sessionsLimitRef`) yeniden çeker — sayfa derinliği olan
  kullanıcı her event'te ilk sayfaya çökmez; `loadMoreSessions` offset'ten
  sonraki sayfayı `id`-dedupe ile ekler, `total`/`hasMore` zarfından gelir.
  `sessionApi.listSessions` artık `Promise<SessionPage>` döner (paramsız legacy
  dizi istemci tarafında `asSessionPage` ile normalize edilir) — `r.items`
  kullanımına geçildi.
- `SessionsSidebar.tsx`: `totalSessions`/`hasMoreSessions`/`onLoadMore` prop'ları;
  buton arama aktifken (mesaj eşleşmeleri kendi bölümünde) veya liste tamamen
  yüklendiğinde gizlenir.
- `SessionsOverview.tsx`: `OVERVIEW_RENDER_PAGE = 50`; `visibleRows` + load-more
  butonu; `query/kind/sortKey/asc` değişince `useEffect` pencereyi sıfırlar.
- `App.tsx`: controller'dan yeni üç prop'u sidebar'a bağlar.

**Doğrulama:** `npx tsc -b` ✅ + `npm run build` ✅ (backend'e dokunulmadı — bu
adım yalnız frontend). Backend kontratı bir önceki bölümdeki `b529649` ile
commit'liydi.

**Dokunulan dosyalar:** `frontend/src/api/sessions.ts`,
`frontend/src/app/useSessionsController.ts`, `frontend/src/app/App.tsx`,
`frontend/src/features/sessions/SessionsSidebar.tsx`,
`frontend/src/features/sessions/SessionsOverview.tsx`.

## TSK66: Harita — yan-yana node kapatma + eksik node tipleri (2026-08-06) ✅

**Ne:** Workspace Explorer'da iki iyileştirme: (1) bir node açılınca aynı parent'ın
diğer açık node'ları kapanıyor (accordion — tek seferde bir sibling açık), (2) root'a
yeni kovacıklar: **Artifacts, Otomasyonlar, Skill'ler, İçgörüler, Günlükler** (Bütçe zaten
vardı). Kök 6 → **11 node**.

**Nasıl:**

- **Single-expand (accordion):** `explorerModel.nextExpandedSet(expanded, key, parentByKey)`
  saf fonksiyonu — genişletilen node'la aynı parent'a sahip diğer açık node'ları kapatır.
  `useExplorerGraph` artık `parentByKey`'i `fetchChildren`'da tutar (her çocuğun parent'ı
  kaydedilir); `toggle` accordion kuralını uygular. Her seviyede çalışır: root kovacıkları,
  board sütunları, ajan oturumları, koordinatör worker'ları.
- **Yeni backend Kind'leri** (`internal/view`): `artifact`, `automation`, `skill`,
  `insight`, `logs` + 4 yeni kategori (`artifacts/automations/skills/insights`). Yeni
  projeksiyonlar (deterministik, LLM yok): `artifact.go` (metadata: origin/session/grup/yaş),
  `automation.go` (tetik/hedef/durum/ateşleme sayacı + hata), `skill.go` (katalog: erişim/
  grup/açıklama), `insight.go` (bulgu: severity/durum/oluşum/kanıt), `logs.go` (process log
  ring-buffer kuyruğu — budget/tools gibi yaprak).
- **Kaynaklar:** skills/logs/findings db'de değil → `Projector.WithSources(Sources{Skills,
Findings, Logs})` (narrow interface'ler; nil = "yok" satırı, asla sessiz boşluk değil).
  `internal/insight` zaten `view`'i import ettiği için (cycle!) findings view-local
  `InsightFinding` tipine bir adapter'la bağlanır (2026-08-10'dan beri
  `tools/viewprojector.go`; önce `api/findingsSource` idi). `Store` arayüzüne
  `GetArtifact/ListArtifacts/GetAutomation/ListAutomations` eklendi.
- **API:** `views.go` → `s.viewProjector(r)` runtime'dan skills + findings + logs kaynaklarını
  bağlar. `expand` aracının açıklaması 11 kovacığa güncellendi.
- **Frontend:** `ViewKind`'e 5 yeni literal; `ExplorerNode`'da ikon+renk haritası (artifact
  FileText/success, automation Zap/warning, skill GraduationCap/info, insight Lightbulb/
  warning, logs ScrollText/dim); `parseRef` yeni kind'ları tanır.

`go build/vet ./...` ✅; `go test ./internal/...` ✅ (yeni: `artifact/automation/skill/
insight/logs_test.go`, `children_test.go`'ya kategori+leaf+source testleri, `views_test.go`
uçtan uca); `tsc -b` + `npm run build` ✅; vitest 167 ✅ (`nextExpandedSet` accordion
senaryoları). Canlı smoke: kök 11 node, artifact/automation/skill/logs projeksiyonları ve
kategori çocukları API'den doğrulandı.

## UX: Ajan klonlama butonu "Kopyala" → "Klonla" (2026-08-06) ✅

**Ne:** Ajanlar ekranının detay başlığındaki ajan çoğaltma butonu artık **Klonla**
olarak adlandırılıyor (`AgentSettingsForm` başlığı, `data-testid=agent-duplicate`).
Tooltip ve yükleme durumu da aynı dilde: "bir klonunu oluştur" / "Klonlanıyor…".
Davranış değişmedi — hâlâ `POST /api/agents/{id}/duplicate` ile ajanın tüm
ayarlarının birebir kopyasını oluşturur. `tsc -b` ✅.

## `create_agent` provider mirası (2026-08-06) ✅

**Ne:** Koordinatör bir ajan oluşturduğunda (provider belirtmeden) yeni ajanlar sabit
`claude-cli`'ye düşüyor, kullanıcı her birini elle deepseek'e çeviriyordu (SES207).

**Nasıl:** `create_agent` (`internal/tools/builtin_agentmgmt.go`) provider boşsa
**oluşturan ajanın** provider+model'ini eşleşik çift olarak devralır (`GetAgent(actorID)`),
yoksa `claude-cli`'ye düşer. Model yalnız hem provider hem model boşken miras alınır → miras
model asla farklı bir caller-provider'ıyla eşleşmez. Kaldırılan soyut workspace-default'a
dönmeden agent-bazlı felsefeye uygun. Test: `TestCreateAgentInheritsCreatorProviderModel`.
Not: ID sayacı (silinen ajanlar sonrası yüksek AGT numaraları) **bilinçli** monotonik —
silinen satır+geçmiş korunduğu için ID geri-kullanımı yapılmaz.

## Özet Haritası — Faz 3 (agent + cila): `expand` aracı, URL deep-link, arama, DOI (2026-08-06) ✅

**Ne:** Haritanın son fazı: ajan tarafı + gezinme cilası. Ajan artık haritanın gezdiği
grafı **aynı backend'le** dolaşabiliyor; harita URL ile paylaşılabiliyor, aranabiliyor ve
odak+bağlam ile soldurma yapıyor.

**Nasıl:**

- **`expand` aracı** (`internal/tools/builtin_expand.go`) — `expand{kind,id,sub}` →
  `Projector.Children`'ı sarar (haritayla birebir aynı). Her çocuğu doğru sonraki çağrıya
  yönlendirir: çocuğu olan → `expand`, yaprak → `get_view`. Singleton id defaulting
  (workspace/board), bilinmeyen kind/kategori = hata. `toolsetup.go`'da her ajana açık,
  kategori `diagnostics`. View paketine `IsExpandable(ref)` helper'ı. Testler:
  `builtin_expand_test.go`.
- **URL deep-link** — `#/w/{ws}/explorer/{refString}`: odak düğümü URL'ye yazılır ve
  girişte/yenilemede geri-yüklenir. Geçersiz veya workspace dışında kalan ref açık hata
  gösterir ve workspace köküne güvenli dönüş sunar. `url.ts` (VIEWS'a `explorer`,
  `routeIdForView`), `useDeepLinks`
  (`explorerNode` state), `useAppNavigation` (applyRoute + route.id). `parseRef` (refString
  → ViewRef; "col:" kategori kolonu ve "#sub" doğru ayrışır).
- **Ekran-içi arama** — başlıkta arama kutusu; eşleşmeyen görünür düğümleri soldurur;
  sonuçta tek tık yalnız seçimi, çift tık odağı değiştirir.
- **DOI pruning** — kök-dışı bir düğüm seçilince odak = seçili + ataları + doğrudan
  çocukları; gerisi (ve kenarları) solar. Küçük/odaksız harita soldurmaz. Soldurma
  gizlemez — düğüm hâlâ tıklanabilir.
- **Kök otomatik-açılım** — Harita açılınca kök yerine 6 kovacık hemen görünür.

Yeni bağımlılık yok. `go build/vet/test ./internal/{tools,view,api}` ✅; `tsc --noEmit` +
vitest (`explorerModel.test.ts` — buildGraph dimming/cycle/layout + parseRef) ✅; explorer
dosyaları lint-temiz. **Ertelendi (opsiyonel):** MCP resource tree, sigma.js.

## Özet Haritası — Faz 2 (frontend): "Harita" ekranı, React Flow lazy-expand (2026-08-06) ✅

**Ne:** Faz 1 backend'inin üstüne **"Harita"** ekranı (NavRail'de Ağ'dan sonra, `Map`
ikonu). Kök `workspace` düğümünden başlayıp tıkladıkça bir katman açılan semantic-zoom
drill-down; seçili düğümün tam projeksiyonu (ajanın gördüğü ham DSL) yanda gösterilir.
Bu ekran **Ağ'dan ayrıdır** — Ağ ilişki grafiği, Harita durum-drill-down.

**Nasıl:** Yeni `features/explorer/`:

- `explorerModel.ts` — graf modeli + **deterministik katmanlı yerleşim** (derinlik→sütun,
  kardeş→satır) ve döngü-kırıcı visited-set (her düğüm bir kez yerleşir, DAG/döngü
  geri-kenarı çizilir). elkjs/dagre eklenmedi.
- `useExplorerGraph.ts` — durum: `expanded`/`childrenByKey`/`selected`, lazy children
  fetch (`api.viewChildren`).
- `ExplorerNode.tsx` — özel React Flow düğümü: kind ikonu + etiket + çocuk sayısı +
  chevron; **semantic zoom** (uzak zoom → tek satır).
- `ExplorerGraph.tsx` — React Flow canvas (pan/zoom + tıkla; sürüklenemez).
- `ExplorerView.tsx` — ekran kabuğu: graf + gömülü `ViewPanel` yan-özet;
  session düğümünde "Sohbeti aç".

Yeni React Flow bağımlılığı YOK (Akış builder'dakini yeniden kullanır); lazy chunk
(`lazyPanels.ts`). `api.viewChildren` + `ViewChildrenResult` tipi + `ViewKind`'e
`agent/budget/tools/category`. Canlı güncelleme: `eventToRefreshSignals`'a `'explorer'`
sinyali (Ağ ile aynı yaşam-döngüsü olayları) → yalnız açık dallar tazelenir.
`tsc --noEmit` ✅; explorer dosyaları lint-temiz. **Faz 3'e ertelendi:** URL deep-link
(`useAppNavigation`) ve ajan `expand` aracı.

## Özet Haritası — Faz 1 (backend): Workspace Explorer drill-down (2026-08-06) ✅

**Ne:** View katmanının (66) üstüne **Özet Haritası / Workspace Explorer** motorunun
backend'i eklendi (bkz. [68](68-OZET-HARITASI.md)): kök `workspace` düğümünden başlayıp
tıkladıkça bir katman açılan semantic-zoom drill-down için yapısal çocuk grafı ve dört
yeni deterministik projeksiyon. Faz 2 (frontend `features/explorer`) ve Faz 3'e
dokunulmadı.

**Nasıl:** `internal/view`'a dört yeni **Kind** (`agent`/`budget`/`tools`/`category`) ve
her biri için ayrı dosyada bir projeksiyon:

- `agent.go` — `ProjectAgent`: ajan kimliği + bugünkü token/maliyet (`billing.RollupOf`,
  yeniden fiyatlama yok) + oturum sayısı (aktif) + son etkinlik; handle'lar ajanın
  oturumları, `agentSessionHandles` cap + elision.
- `budget.go` — `ProjectBudget`: `billing.Rollup`'ı **sarar** (loader `UsageForDay`'i
  birleştirip `RollupOf`'u bir kez çağırır); gün/model kırılımı, üst-N + `Elided`.
- `tools.go` — `ProjectTools`: MCP sunucu havuzu (aktif önce) + kapalı araç sayısı.
- `category.go` — `ProjectCategory`: grup düğümü, üyeleri sayar (dürüst toplam), üst-N'i
  handle verir, kalanı `Elided`. Bilinmeyen kategori id = hata.
- `children.go` — `Projector.Children(ctx, ref) []Handle`: **yapısal** çocuk grafı
  (özet `Project`'ten ayrı). `workspace`→6 kategori; `category:sessions/flows/agents`→üye
  handle'ları; `board`→sütun düğümleri; `category:col:<key>`→kart handle'ları;
  `agent:X`→oturumları; `session:COORD`→worker'ları;
  `categoryTopN` (50) döngü/patlama cap'i. Bilinmeyen Kind = hata, sessiz boş liste değil.

`Store` arayüzüne dört salt-okunur metot (`GetAgent`, `GetUsageToday`, `ListMCPServers`,
`GetWorkspaceToolConfig`); leaf-paket disiplini korundu. API: `handleGetView`'in yanına
`GET /api/views/{kind}/{id}/children` route'u (`views.go`). Testler mevcut
`view/*_test.go` fixture desenini izler (hand-built `db.*` + `fakeStore`); her projeksiyon

- `Children` + endpoint için kapsam. `go build ./... && go vet ./... && go test
./internal/view/... ./internal/api/...` ✅.

## UI: Chat header'da "◱ Özet" → "Coord"; koordinasyon kendi drawer'ına, projeksiyon detay paneline (2026-08-06) ✅

**Ne:** Sohbet başlık çubuğundaki `◱ Özet` (session `ViewButton`) kaldırıldı; yerine
**"Coord"** butonu (Network ikonu) geldi. Yeni buton, koordinasyon UI'ını (koordinatör
modu toggle + workflow seçici + canlı worker roster + koordinatör ağacı) sağdan açılan
bir yan-sheet'te gösterir. Bu blok (`CoordinatorSection`) daha önce **"Oturum bilgisi"**
detay panelinin içinde duruyordu; oradan çıkarıldı → tek yerde, kendi butonuyla. Boşalan
yere, eski `◱ Özet` drawer'ının içeriği (ajanla aynı `get_view` projeksiyonu) **satır-içi**
taşındı.

**Nasıl:** Yeni `features/sessions/CoordinatorPanel.tsx` — `api.sessionInfo` çekip
`CoordinatorSection`'ı `ModalOverlay` (right-anchored) içinde render eden drawer. `App.tsx`
`coordOpen` state + `onOpenCoord` prop'u ile bağlar (kalıcı değil, on-demand overlay).
`ViewPanel`'e `embedded` prop'u eklendi (opsiyonel `onClose`): drawer kromu/sabit yükseklik
yerine host panelin akışında self-contained blok — `SessionDetailPanel`'de "Özet" bölümü
olarak gömülü. `AppHeader`'dan `ViewButton` importu/kullanımı silindi (bileşen Görevler /
Dashboard / Akışlar ekranlarında kalıyor). `SessionDetailPanel`'in artık kullanılmayan
`onOpenSkill` prop'u temizlendi. Detay panelindeki "Özet" bölümü **katlanabilir**
(başlık chevron toggle, `tionharness.sessionSummaryOpen` ile kalıcı, varsayılan açık;
katlıyken `ViewPanel` mount edilmez → gereksiz `get_view` çağrısı yok). Gömülü
`ViewPanel`'in `target`'ı `useMemo(sessionId)` ile stabil — satır-içi obje her render'da
kimlik değiştirip 1s timer/3s poll tick'lerinde `getView`'i (deterministik, LLM'siz)
gereksizce yeniden çağırıyordu; artık yalnız oturum/seviye değişince yükler.
`tsc --noEmit` ✅.

## Fix: Kullanıcı "Durdur" sonrası koordinatör idle-reconcile turu kaçağı (2026-08-06) ✅

**Ne:** Bir oturumda kullanıcı koordinatörü durdurunca — o an worker koşmasa bile —
koordinatör `<coordination-status>All workers ... have finished ...</coordination-status>`
notunu görüp session devam etmeye başlıyordu. Kök neden: `run.cancel` koordinatörün
o anki turunu `context.Canceled` ile bitiriyor (`runCoordinatorTurn` "⏹️ durduruldu"
yazıyor) ama ayrı goroutine'deki `drainCoordinator` dönmeye devam edip **idle-reconcile**
dalına düşüyor; koordinatörün workerları önceden bittiyse (`hadWorkers && workers==0`)
notu enjekte edip bir tur daha koşuyordu.

**Nasıl:** `coordSlot`'a tek-seferlik `stopRequested` bayrağı eklendi. `runCoordinatorTurn`
insan-stop dalında (watchdog kesintisinden `classifyTurnContext`'in zaten ayırdığı plain
`context.Canceled`) bayrağı set eder; `drainCoordinator` **yalnızca no-pending
idle-reconcile'ı** bastırır. Önemli incelik (kullanıcı isteği): pending bir worker
bildirimi varsa (çalışan/yeni-biten worker) o Stop'u geçersizler ve tur yine koşar —
**çalışan worker'lar Durdur'a rağmen koordinatörü sürdürebilir**; yalnız hiç worker
yokken tam durur. `scheduleSettleBackstop` korunur (mid-node üst-rapor borcu). Durdurma
kalıcı değil: koşan worker bitince `enqueueCoordinatorTurn` loop'u yeniden başlatır.
Detay `_Docs/47` §14.9. Testler: `TestUserStopSkipsIdleReconcile`,
`TestUserStopHonoursPendingWorker`; `go build ./...` + `go vet` + `go test ./internal/agent/...` ✅.

## Fix: DeepSeek V4 Pro `context deadline exceeded` @120s (2026-08-06) ✅

**Ne:** OpenAI-uyumlu client (`OpenAICompat`) her isteğe **sabit 120s** `http.Client.Timeout`
uyguluyordu ve model-sınıfından habersizdi. Akıl-yürütme ağırlıklı `deepseek-v4-pro`
tek isteği 120s'yi aşınca `context deadline exceeded` ile kesiliyordu (SES446/WS17: 3 kez
tam 120.000 ms; 2 retry toparladı, 3.'de retry bütçesi bitti → tur öldü). Ayrıca native
Anthropic yolundaki `requestCtx`, "uzun bütçe" kararını **thinking wire-format**
sınıflandırıcısı `UsesAdaptiveThinking`'e bağlıyordu; deepseek orada olmadığı için
`deepseek-anthropic` kind'ı da 120s alıyordu.

**Nasıl:** İstek bütçesi wire-format'tan **ayrıldı** → yeni `internal/providers/timeouts.go`:
`LongRequestModel` (adaptive sınıf + deepseek reasoning) + `RequestTimeoutSecsFor` +
`effectiveRequestTimeoutSecs`. Her iki HTTP client artık **per-request ctx deadline**'ı
model-sınıfı-farkında uygular (`OpenAICompat.requestCtx`; `http.Client.Timeout` yalnız
630s güvenlik ağı). **Manifest-driven override:** `Manifest.RequestTimeoutSecs` (0 =
model-sınıfı varsayılanı) → `Registry.Get` Build sonrası `requestTimeoutConfigurable`
(her iki client) üzerinden uygular; hiçbir kind build fonksiyonu timeout plumbing'i
taşımıyor. Testler: `timeouts_test.go` (deepseek-v4-pro→uzun, flash→120s, override kazanır).
`go build ./...` + `go test ./internal/providers/` (118) ✅.

## Feature: Token otomasyonları kalıcı oturumda devam ediyor (2026-08-06) ✅

**Ne:** Token-eşiği tetikleyicili otomasyonlar, her geçişte **yeni oturum spawn
etmek yerine** otomasyona ait tek **kalıcı bakım oturumunda** devam ediyor — cron
schedule'ın stabil `schedule` oturumuyla aynı model. Kullanıcı isteği: "token'lı
otomasyonlar her çalıştığında önceki sessiondan devam etsin, cronjob'lı otomasyonlar
gibi".

**Nasıl:** Yeni `Runtime.deliverAutomationTurn` (`internal/agent/automation_deliver.go`)
promptu `GetOrCreateSourceSession(kind="automation", sourceID=otomasyon.ID)` oturumuna
**history-aware** (`runSessionTurn`) teslim eder; scheduler `deliverPrompt`
yaşam-döngüsünü yansıtır (turn slot, watchdog + idle-resume, reply/error record,
bounded auto-continue). `fireToken` (`agent/automation.go`) session/agent driver'ı
bu yola, flow driver'ı yine `LaunchRun`'a yönlendirir; reuse yolu launch-gate'i
atladığı için `Paused()` elle kontrol edilir.

**Self-amplification guard:** Bakım oturumu her fire'da token harcadığından
session-kapsam kuralı kendini sonsuz döngüde tetikleyebilirdi. `OnUsageRecorded`
session-kapsam dalı `Kind=="automation"` oturumlara atfedilen geçişleri **atlar**
(lazy, bellek-içi lookup). Workspace kapsamı guard'lanmaz (gerçek harcama).

`go build`/`vet` + `go test ./internal/agent/ ./internal/db/` ✅. Detay `_Docs\46 §2.6`.

## Fix: `update_automation` token eşiğini kaydetmiyordu (2026-08-05) ✅

**Ne:** `db.UpdateAutomation` (`internal/db/store_automation.go`) gelen struct'tan
alanları tek tek kopyalarken **`TokenScope` ve `TokenThreshold`'u atlıyordu** →
`update_automation` aracı `"updated"` dönüp başarı raporluyor ama token eşiği eski
değerinde kalıyordu (araç katmanı doğru işliyordu, DB katmanı yutuyordu). Bir
ajanın canlı workspace'te (SES353) kara-kutu keşfettiği hata.

**Düzeltme:** `UpdateAutomation`'a `cur.TokenScope = a.TokenScope` +
`cur.TokenThreshold = a.TokenThreshold` iki satırı eklendi. Regresyon testi:
`automation_test.go > TestAutomationTokenThresholdUpdate` (eşik + scope güncellemesi
persist + reload doğrular). `go build ./...` + `go test ./internal/db/` ✅.

## Fix: Self-management "provenance guard" kaldırıldı — workspace hariç (2026-08-05) ✅

**Ne:** Ajanların, kullanıcının UI/API'de oluşturduğu varlıkları düzenleyip/
silmesini engelleyen köken (provenance) filtresi kaldırıldı. Eskiden
`CreatedBy == ""` (veya artifact'te `AgentID == ""`) olan varlıklara ajan
dokunamıyordu; artık **her varlık** (kullanıcı- veya ajan-oluşturduğu) ajan
araçlarıyla düzenlenip silinebilir. Kapsam: **agents, flows, schedules,
automations, hooks, mcp_servers, artifacts** (tasks'ta zaten kaldırılmıştı).
`CreatedBy`/`AgentID` bu türlerde yalnız köken/görüntü için tutulur.

**Tek istisna — workspaces:** Workspace silme benzersiz yıkıcı olduğu için (tüm
workspace verisini geri dönüşsüz siler) köken guard'ı **workspace silmede
korundu** — ajan yalnız kendi oluşturduğu workspace'i silebilir. (Kullanıcı
isteği, 2026-08-05: "sadece workspace silmeyi devre dışı bırak, gerisi dursun".)

**Korunan operasyonel guard'lar (köken filtresi değil):** ajan **kendini**
silemez (`delete_agent`); **mevcut çalıştığı** ve **son kalan** workspace
silinemez; tüm silme/düzenleme yollarında "bulunamadı" hatası aynen döner
(sessiz no-op yok).

**Dokunulan yerler:** `internal/tools/builtin_{agentmgmt,flowmgmt,schedulemgmt,
automationmgmt,mcpmgmt,hookmgmt,artifactmgmt}.go` — `require*` helper'ları köken
kontrolünden arındırıldı ve dürüst adlara (`requireAgent`, `requireFlow`,
`requireSchedule`, `requireAutomation`) yeniden adlandırıldı; inline delete
kontrolleri (mcp/hook/artifact) kaldırıldı; tool açıklama ve yorumları güncellendi.
`builtin_workspacemgmt.go` köken guard'ını **korur** (yalnız yorum/açıklama netleşti).
`internal/agent/toolsetup_selfmanage.go` + `autotag.go` (ölü deny-marker temizliği)
yorumları. Testler: `builtin_selfmanage_test.go`, `builtin_controlgaps_test.go`
yeni davranışa (izin verilir) çevrildi; `builtin_workspacemgmt_test.go` guard'lı
kaldı. Doküman: `24-SELF-MANAGEMENT.md`, `02-VERI-MODELI.md`. `go build ./...` +
`go test ./internal/tools/` ✅.

## Feature: DeepSeek sağlayıcı kind'ı (V4 Flash/Pro) eklendi (2026-08-05) ✅

**Ne:** Yeni birinci-parti `deepseek` provider kind'ı — DeepSeek'in OpenAI-uyumlu
uç noktası (`https://api.deepseek.com`), paylaşılan `OpenAICompat` istemcisiyle
(tool-use + streaming, native ajan döngüsü). Modeller: `deepseek-v4-flash`
(varsayılan, hızlı/çok ucuz) + `deepseek-v4-pro` (akıl-yürütme). Legacy
`deepseek-chat`/`deepseek-reasoner` 2026-07-24'te kaldırıldığı için dahil edilmedi;
`AllowCustomModel` açık.

**Dokunulan yerler:** `kind_deepseek.go` (yeni, self-registering `init`),
`registry.go` (alanlar + `SetDeepSeek`/`DeepSeekConfigured` + `resolve` case),
`pricing.go` (V4 fiyatları: flash $0.14/$0.28, pro $0.435/$0.87, cache-hit read
mult ~0.02/0.008), `settings.go`+`store.go` (KeyEnc/BaseURL + DTO + Patch +
write-only sır + `reservedProviderIDs`), `server.go applySettings`, frontend
(`settings.ts`, `SettingsPanel.tsx`, `ProvidersPanel.tsx` yeni kart). Bağlam
penceresi/max-output tabloları `deepseek` ailesini zaten kapsıyordu → değişmedi.
`context_window.go` dokunulmadı.

**Yan düzeltme:** `SettingsPanel` ana Kaydet gövdesi `zaiBaseUrl`'i göndermiyordu
(pre-existing eksik) → `deepseekBaseUrl` ile birlikte eklendi. `reservedProviderIDs`
listesine eksik olan `zai` de eklendi. `go build ./internal/...` + frontend
`tsc --noEmit` ✅.

**Ek (2026-08-05, resmi doküman doğrulaması):** DeepSeek'in **Anthropic-uyumlu ucu**
(`https://api.deepseek.com/anthropic`) resmi dokümandan doğrulandı → `minimax-anthropic`
deseninde ikinci bir `deepseek-anthropic` kind'ı eklendi: DeepSeek anahtarını yeniden
kullanır (ayrı kart/anahtar yok), `NewAnthropic().WithEndpoint` ile native tool-use +
**düşünme** sürer (özellikle reasoning-ağırlıklı V4 Pro için). `resolve()` case,
`PriceFor` (`deepseek-anthropic`→`deepseek` eşlemesi), `reservedProviderIDs` ve
katalog testi (7→8) güncellendi. Fiyatlar resmi sayfayla birebir doğrulandı; DeepSeek
gerçek max-output 384K olduğundan `maxOutDeepSeek` 8K→**32K** yükseltildi (reasoning
çıktısının erken kesilip resume döngüsüne girmesini azaltır, tavanın hâlâ ~12× altında).
DeepSeek yakında peak/off-peak (2×) fiyatlandırmasına geçecek — henüz yürürlükte değil,
not düşüldü. Toplam 8 kind. `go build ./...` + `go test ./internal/providers/...
./internal/settings/...` + frontend `tsc` ✅.

## Fix: bütçeli fold sistem-prompt/şema yükünü hesaba katmıyordu — "%127 ama özetlenmiyor" (2026-08-05) ✅

**Belirti:** SES319 (WS17, claude-cli koordinatör) "Bağlam penceresi · 88.8k/70.0k
(%127)" gösterdi ama hiç compact olmadı (`summary:""`, `debug.jsonl`'de 0 `compaction`
olayı). İlk şüphe "spawned koordinatör turları `Prepare`'i atlıyor" idi — **yanlış**:
`runCoordinatorTurn` → `runSessionTurn` → `wakeTurn` → `Prepare` zaten çalışıyor.

**Kök neden:** İki taraf farklı temel sayıyordu. UI metresi (`session_info.go`)
`ContextTokens`'ı **mesaj+özet + sistem promptu + skill/tool katalogları + eager tool
şemaları + artifacts** (`systemFillers`) olarak toplar → 88.8k. Ama `Manager.Prepare`'in
fold kararı yalnız **mesaj+özet** (`EstimateTokens(summary, pending)`) tokenine bakıyordu;
her turda gönderilen sabit mesaj-dışı yükü (bir claude-cli koordinatöründe 20-40k) hiç
saymıyordu. Sonuç: mesaj-only tahmin 70k floor'unun altında kaldıkça fold **hiç
tetiklenmiyor**, oysa gerçek footprint bütçenin çok üstünde.

**Fix:** `conversation.WithContextOverhead(ctx, tokens)` ctx-seam'i eklendi; `Prepare`
fold gate'ini `before+overhead > maxTokens`'e ve pressure'ı `(contextTokens+overhead)/
maxTokens`'e taşıdı (overhead=0 → eski davranış birebir korunur, testler/e2e harness
değişmez). Üç çağrı sitesi (`chat_stream`/`chat_btw`/`wake_turn`) overhead'i metrenin
kullandığı **aynı** `systemFillers` üzerinden (`s.contextOverheadTokens`) hesaplayıp
ctx'e koyar → metre ve motor artık oranın **iki tarafında da** aynı temeli kullanır.
Testler: `TestPrepareOverheadRaisesPressure`, `TestWithContextOverheadNoOp`. `go build`

- `go test ./internal/conversation/... ./internal/api/...` ✅.

**Tradeoff:** Fold artık gerçek footprint bütçeyi aşınca (daha erken) tetiklenir → biraz
daha sık özetleyici (summarize) LLM çağrısı; istenen düzeltme tam da bu. Büyük sabit yük
bütçeye yakınsa `keepRecent` yine taban olduğu için aşırı fold olmaz.

## UX: Ajan detay ekranında "Kopyala" (ajan çoğaltma) butonu (2026-08-05) ✅

**Ne:** Ajanlar ekranının detay (sağ pane) başlığına, seçili ajanın **tam kopyasını**
oluşturan bir **Kopyala** butonu eklendi (Bağlam ile Sil arasında). Kopya, kaynağın
_bütün_ ayarlarını taşır: profil (ad/avatar/renk/soul/identity), sağlayıcı+model,
thinking + izin modu, yetenekler (skills) ve tam araç yapılandırması (MCPEnabled +
ToolOverrides + allow/block listeleri). Yalnız kimlik alanları sıfırlanır — kopya yeni
bir ID, `"<ad> (kopya)"` adı ve temiz created/updated/deleted durumu alır. `CreatedBy`
boş bırakılır → kullanıcı-sahipli (agent-created değil).

**Uygulama:** Backend `POST /api/agents/{id}/duplicate` → `handleDuplicateAgent`
(`GetAgent` → struct kopyala → kimlik alanlarını temizle → `CreateAgent`; `CreateAgent`
zaten tüm alanları literal'den kopyaladığı için araç/skill/görsel yapılandırma birebir
korunur). Frontend: `api.duplicateAgent`, controller `duplicateAgent` (roster'a ekle +
klonu seç, varsayılana dokunmaz), `AgentSettingsForm` başlığında `data-testid=
agent-duplicate` buton. `go build ./...` + `tsc --noEmit` ✅.

## Fix: claude-cli `allowed_warning` rate-limit yanlış sınıflandırması (2026-08-05) ✅

**Belirti:** SES189'da manuel `/compact` sessizce başarısız oldu (asistan yanıtı yok,
`debug.jsonl`'de kayıt yok). App log: `summary command failed … "claude CLI usage/rate
limit reached: seven_day window, status=allowed_warning, resets 2026-08-10"`.

**Kök neden:** Ham claude-cli dump'ı → `rate_limit_event` `status="allowed_warning"`,
`utilization=0.64`, `isUsingOverage=false` (istek İZİNLİ, kotanın %36'sı boş). Ama
`internal/providers/claudecli.go` `rate_limit_event` işleyicisi yalnız tam `"allowed"`
string'ini geçerli sayıyor, `!EqualFold(status,"allowed")` ile `allowed_warning`'i
ölümcül rate-limit olarak işaretliyordu → `p.rateLimited=true`, tur non-retryable
hataya düşüp gerçek çıkış nedenini (init sonrası model çıktısı olmadan exit) maskeliyordu.

**Düzeltme:** Guard `strings.HasPrefix(lower(status),"allowed")` oldu → `allowed` +
`allowed_warning` ailesi tur ilerlemesine izin verir; yalnız `rejected`/`blocked` gibi
"allowed" dışı statüler `rateLimited` set eder. Böylece uyarı statüsünde tur, gerçek
sebebe göre (çoğunlukla `!sawModelTurn` → tool çalışmadıysa **retryable**) sınıflanır.
`cliRateLimit` doc-comment'i güncellendi. Regresyon testi eklendi
(`claudecli_ratelimit_test.go`: `allowed`/`allowed_warning`/boş status → rateLimited
DEĞİL; `rejected`/`blocked` → rateLimited). `go build ./...` ✅.

## UX: Bağlam bütçesi ayarında taban/tavan netleştirme + canlı önizleme (2026-08-05) ✅

**Sorun:** "Maks. bağlam token" bir MAKSİMUM gibi duruyor ama aslında TABAN (floor) —
`EffectiveBudget = clamp(pencere × oran, taban, tavan)`. Kullanıcı pencereyi küçültmek
için tabanı 50K'ya çekti ama etkin bütçe `contextBudgetCeil=63000` tavanına kırpılı
olduğundan (1M×0.6=600K→63K) değişmedi; taban yalnız YÜKSELTİR.

**Düzeltme (`frontend/.../settings/ContextPanel.tsx`):** Etiketler netleşti —
"Maks. bağlam token — TABAN" (hint: yalnız yükseltir, küçültmek için tavanı düşür) ve
"Bütçe tavanı — ÜST SINIR" (hint: pencereyi küçültmek için değiştireceğin ayar budur).
Ayrıca **canlı önizleme bloğu** eklendi: girilen taban/oran/tavan ile iki temsili pencere
(1M Opus/Sonnet/Fable auto-0.45, 200K Haiku auto-0.40) için etkin bütçeyi ve HANGİ sınırın
aktif olduğunu (`tavana kırpıldı` sarı · `tabana yükseltildi` accent · `pencere × oran`)
gösterir. `effectiveBudget()` helper'ı backend `EffectiveBudget` mantığını birebir
yansıtır (default'lar 12000/262144 mirror'landı). `npx tsc --noEmit` ✅.

## Handoff: koordinatör + workflow ayarları devam oturumuna taşınıyor (2026-08-05) ✅

**İstek:** `/handoff` ile açılan temiz-pencere devam oturumu, kaynak oturumun koordinatör
ve workflow (recipe) ayarlarını da alsın.

**Değişiklik:** `HandoffSession`'daki inline `SpawnOptions` kurulumu saf, test edilebilir
`handoffContinuationSpawnOpts(session, opts)` helper'ına çıkarıldı. Devam oturumuna
taşınanlar: `CoordinatorMode` (koordinatör promptu + `spawn_worker`/… araç seti),
`CoordinatorWorkflow` (seçili recipe), `CoordinatorMaxTurns` (recipe'in notify-loop cap'i)
— ayrıca mevcut `WorkingDir`. **Taşınmayanlar** (bilinçli): ağaç/worker lineage'ı
(`Role`/`CoordinatorSessionID`/`Root`/`Depth`) → devam oturumu kendi taze **kök**
koordinatörü olur, eski (artık handoff'lanmış) ağaca rapor veren bir worker değil.
Alt-koordinatör SPAWN'ından farkı: orada recipe kasıtlı miras alınmaz (özyinelemeli
recipe ağaçta tekrarlanmasın diye); handoff ise aynı mantıksal oturumun devamı → miras
doğru. Üç handoff yolu (manuel `/handoff`, `handoff_session` aracı, oto-handoff) aynı
helper'dan geçtiği için hepsi kapsanır. Test: `handoff_test.go` (koordinatör ayarları
taşınır + lineage sızmaz; koordinatör-olmayan ebeveyn bayrak uydurmaz). `go build ./...`

- `go vet` + `go test ./internal/agent` ✅.

## Fix: compact sırasında kuyruk paneli kaybolması (seri slot tutulmuyordu) (2026-08-05) ✅

**Belirti:** `/compact` çalışırken yeni mesaj gönderilince UI'daki "Bekleyenler" (sıra)
paneli anında kayboluyordu; mesaj compact'in arkasında beklemek yerine onunla eşzamanlı
koşuyordu (claude-cli'de aynı `cliSessionId`'de iki subprocess → transkript bozulma riski).

**Kök neden:** `/compact` (ve diğer summary slash komutları) `handleSessionSummary`'de
doğrudan `provider.Complete` ile HTTP goroutine'inde koşuyor, `runInboxWorker`'dan
GEÇMİYOR. Bu yüzden inbox seri slot'u boş kalıyor; mesaj enqueue edilince `kickInbox`
worker'ı hemen başlatıp mesajı pop ediyor → `ib.items` boşalıyor → boş `queue_update`
→ tray kayboluyor.

**Düzeltme (v2 — kuyruğa tam uyum):** `internal/api/inbox.go`'ya
`acquireInboxSlot(ctx, sessionID, wsID) (release, err)` eklendi: kuyruk-dışı slash
komutlarını (`/compact`, `/refresh-context`, `/handoff`) seri send-queue'ya HER İKİ
yönde de uydurur. Slot meşgulse (bir tur akıyorsa) `idleSignal` broadcast kanalında
**bloklayarak bekler** (poll değil), tur bitince slot'u kapar; komut sürerken gelen
mesaj tray'de WAITING kalır (`kickInbox` running iken no-op), `release()` flag'i temizler

- idle broadcast + worker'ı kick eder (birikeni drenajlar). `ctx` iptali (istemci
  kopması) yan etki öncesi temiz çıkar. `running=false` set eden iki yer (worker
  drain-tamam + release) `signalInboxIdleLocked` ile bekleyeni uyandırır. İlk sürümdeki
  `holdInbox` (tur akıyorsa `held=false` ile pas geçen band-aid) bununla değiştirildi —
  artık komut, önündeki turu bekler, arkasındakileri de bloklar.
  `handleSessionSummary` + `handleSessionHandoff` `acquireInboxSlot` + `defer release`
  ile sarıldı (compact/refresh-context/handoff kapsanır). Testler: `inbox_hold_test.go`
  (held iken dispatch engellenir; worker koşarken beklenir + uyanır; ctx iptalinde
  worker'a dokunmadan çıkar). `go build ./...` + `go vet` + `go test ./internal/api
./internal/providers` ✅ (race detector bu makinede gcc yokluğundan çalıştırılamadı;
  kilitleme tek `inbox.mu` altında, capture-then-select lost-wakeup'a karşı güvenli).

## View: board tek-kart + schedule projeksiyonu + aksiyon-kuyruğu drill-down (2026-08-05) ✅

Aksiyon kuyruğundaki her satır artık ekrandan çıkmadan **◱ ile o varlığın `get_view`
projeksiyonunu** yandan açabiliyor. İki boşluk kapatıldı:

- **Board tek-kart `sub`:** `ProjectBoard`'a `Sub` → `projectCard` (durum/öncelik/
  sahip/termin/bağımlılık ⛔/gecikme ⚠/son koşu; bilinmeyen id hata). `card` aksiyonu
  `{kind:'board', sub:id}`.
- **Schedule projeksiyonu (yeni `KindSchedule`):** `schedule.go` — armed/disabled,
  son-çalışma statüsü+hatası, sıradaki, hedef agent/flow, prompt. L0/L1. Roll-up'ın
  aksine devre-dışı zamanlamanın hatası burada gösterilir (kullanıcı ona inmişse).
- **Araç + UI:** `get_view` kind enum'una `schedule` + board `sub`=kart eklendi.
  `ViewButton`'a opsiyonel `label`; aksiyon satırına ◱ tetikleyicisi (iç içe buton
  olmaması için satır yeniden yapılandırıldı); workspace özeti altında
  `summary.handles` tıklanabilir ◱ çipler. `schedule` için projeksiyon yok değil artık
  → dört aksiyon türü de drill-down. API değişmedi (routing generic).
- **Test:** `schedule_test.go` (tek-kart sub + bilinmeyen-id hatası + schedule son-hata).
  view/api/tools yeşil, FE build geçer.

## Panel: CEO kokpiti — maliyet + aksiyon kuyruğu + delta + sonuçlar (2026-08-04) ✅

Dashboard (`66`) dört yeni blokla CEO görünümüne yaklaştı; hepsi backend'de
hesaplanır (tarayıcı workspace'i indirmez), fiyatlama `billing.RollupOf` ile Bütçe
ekranıyla tek kaynaktan.

- **Maliyet (item 1):** workspace projeksiyonu başlığına `$X bugün` eklendi
  (`internal/view` artık `billing` import eder; fiyatlanmış harcama yoksa satır
  yazılmaz). Panelde 💰 blok: bugün / bu ay / günlük ort. (burn) / ay-sonu tahmini +
  önlenebilir cache-israfı notu. Abonelik sağlayıcı `~$`. Ayrıca maliyet trendi +
  "en maliyetli ajanlar ($)" sıralaması.
- **Aksiyon kuyruğu (item 2):** projeksiyon sinyallerinin **tıklanabilir** hâli
  (`dashboard_actions.go`) — takılmış oturum / başarısız koşu-kart (danger) +
  bekleyen soru / hatalı zamanlama / hareketsiz kart (warn). Her satır ilgili ekrana
  gider (`DashboardNav`: oturum→chat, koşu→flows, kart→board, zamanlama→schedules).
- **Dönem-üstü delta (item 3):** açılan oturum · koşu · token · maliyet trendleri
  önceki eşit-uzunluk pencereye göre ▲/▼% rozeti taşır (`dashboard_cost.go`;
  maliyette artış kırmızı, baseline yoksa "yeni").
- **Sonuç metrikleri (item 4):** biten kart/gün (velocity) · ort. tamamlanma süresi
  (cycle time) · koşu başarı oranı (`dashboard_outcomes.go`) — hacim değil tamamlanma
  tarafı. Her metrik kendi "veri yok" durumunu kelimeyle söyler.
- **Test:** `dashboard_new_test.go` dört bloğu + aksiyon kuyruğunun gerçek sorunları
  yakaladığını sabitler. `internal/view` + `internal/api` yeşil, FE prod build geçer.

## UI tema-hizası + a11y + Toast/Komut-Paleti (2026-08-04) ✅

- **Tema-hizası:** Ham renkler tema token'larına taşındı — `RunView` waiting durumu
  (`#eab308`→`--color-warning`, buton yazısı `--color-bg` ile iki-temada kontrast),
  `TodoPanel` completed (`green-400`→`--color-success`), `ThinkingShareChart`
  (violet→`--color-accent`). viz olay-rozetleri (`SelfHealing`/`PromptCache`/
  `HookActivity`) tümüyle token'landı; yeni **`--color-info`** (sky) token'ı eklendi
  (dark `#38bdf8` / light `#0284c7`). 5 chat kartında `shadow-xl shadow-black/40`→
  `--shadow-lg`. Hepsi preset + light temada re-theme oluyor.
- **a11y:** Global `prefers-reduced-motion` (animasyon/transition kısılır); toggle
  switch'lerine `role="switch"`+`aria-checked` (`settings/Toggle`, `ScheduleCard`,
  `AutomationCard`).
- **Tutarlılık:** 3 ad-hoc modal (`ArtifactPreviewModal`/`RewindDialog`/`LensList`)
  paylaşılan `ModalOverlay`'e; 6 tam-panel yükleyici `LoadingState`'e taşındı.
  (`PromptEditor` tam-ekran + `ServerManagement` testid'li bilinçli ad-hoc bırakıldı.)
- **Toast (yeni):** Hata-özel `ErrorToast` → çok-tonlu **`Toast`** (error/success/info)
  - `Toaster` host + modül-seviye `toast.*` API (context'siz, her yerden çağrılır).
    Mevcut `onError` sink'leri `toast.error`'a yönlendi (geriye-uyumlu).
  * **Pozitif toast'lar (yeni):** Başarıyla tamamlanan kullanıcı aksiyonlarına kısa
    `toast.success`/`toast.info` eklendi. **Ayarlar:** ayar kaydet (`SettingsPanel`),
    görünüm kaydet/genele sıfırla (`AppearancePanel`), workspace dosyaları kaydet
    (`WorkspaceFilesPanel`), sır ekle/güncelle/sil + panoya kopyala (`SecretsPanel`),
    hook ekle/güncelle/sil (`HooksPanel`), MCP sunucusu ekle/güncelle/sil
    (`useToolsPanelState`). **Workspace:** oluştur/ekle/sil (`useWorkspaces`), genel
    ayar kaydet (`WorkspaceView`). **Akışlar:** oluştur/şablondan oluştur/kaydet/sil
    - toplu sil (`flowActions`). **Görevler:** oluştur/güncelle/sil (`TaskFormModal`).
      **Zamanlama/otomasyon:** oluştur/güncelle (`ScheduleModal`/`AutomationModal`),
      sil (`AutomationBoard`). **İçgörü:** ayar kaydet (`SettingsTab`), lens kaydet
      (`LensList`), ders sil (`LessonsList`). **Sağlayıcı/kimlik:** özel sağlayıcı sil
      (`ProvidersPanel`), claude-cli kimlik kaydet/sil (`ClaudeAuthDialog`).
      **Market/registry:** paket kur (`MarketPanel`), registry ekle/sil
      (`RegistryManager`), katalogdan MCP ekle (`useToolsPanelState.addImportable`).
      **Artifact:** kaydet/sil (`ArtifactsPanel`). Yalnız onaylanmış başarıda (await
      çözülüp hata fırlamadıysa) tetiklenir; hata yolu değişmedi, gürültü (her
      tuş/otomatik-kayıt/toggle) yok. **Ton düzeltmesi:** `MarketPanel` kurulum
      başarısını `onError('✓ …')` ile KIRMIZI hata toast'ında gösteriyordu →
      `toast.success`'e alındı. Bilinçli atlananlar: zaten inline sonuç gösteren
      `BackupPanel`, MCP import, `ExternalToolsPanel` bakım aksiyonları.
  * **Kopyala geri bildirimi → tek kanal:** Tüm kopya butonlarındaki inline
    "Kopyalandı"/✓-checkmark swap'ları (yerel `copied`/`done` state + `setTimeout`)
    kaldırılıp tek `toast.info('Panoya kopyalandı')` kanalına taşındı: `CopyPathButton`,
    `PromptEditor`, markdown `CodeBlock`/`MermaidDiagram`, `AgentContextModal`,
    `SessionContextModal`, `ClaudeAuthDialog`, `ServerManagement`, `ExternalToolsPanel`,
    `TaskFormModal` (kart-id), `ViewPanel`, `ArtifactsPanel`, `ChangesModal` +
    `RunNodeInspector` `CopyButton`'ları. Buton artık her zaman `Copy` ikonu gösterir;
    başarı geri bildirimi tek yerden (toast) gelir. Not: `CodeBlock`/`MermaidDiagram`
    sohbette sık kopyalanır → gürültü olursa bu ikisinde inline ✓ geri getirilebilir.
- **Komut paleti (yeni):** **⌘K / Ctrl+K** → generic `CommandPalette` (substring
  filtre + klavye nav); komutlar = her view'a git + workspace değiştir. App shell'de
  tek yerden beslenir (`setView`/`switchWorkspace`).
- **Doğrulama:** `tsc -b` + `vite build` temiz.

## Bağlam metresi ↔ compaction eşiği tutarsızlığı (2026-08-04) ✅

- **Sorun:** `session_info.go` bağlam metresi pencereyi **ham `MaxContextTokens`
  tabanı** (örn. 63K) olarak gösteriyordu; oysa `Prepare`'ın gerçek katlama eşiği
  `EffectiveBudget` (büyük-pencere modelde tabanı `window*fraction`, `ceil` ile
  sınırlı). claude-cli **opus** ajanda (1M pencere, `fraction=0.6`, `ceil=512K`)
  gerçek eşik ~512K'ydı → metre 81.8K/63K = %130 gösterip "sonraki turda sıkışır"
  derken motor 512K'ya kadar hiç katlamıyordu. Kullanıcı "bağlam sıkışmıyor" olarak
  gördü (SES189/WS17).
- **Çözüm (kod):** `session_info.go` `resp.ContextWindow` artık `MaxContextTokens`
  yerine `conversation.EffectiveBudget(provider, model, floor, fraction, ceil)`
  raporluyor → metre penceresi motorun gerçek eşiğiyle birebir. Ajan çözülemezse
  ham tabana düşer.
- **Not:** Metre "kullanılan" toplamı sistem-promptu/araç-şeması gibi ~sabit
  overhead'i de içerir; `Prepare` yalnız mesajları sayar → küçük kalıcı fark
  (erken uyarı yönünde, güvenli).

### claude-cli resume ↔ compaction: fold'da cold-start (2026-08-04) ✅

- **Sorun:** claude-cli **warm-resume** modunda (`claudeResume=on`) `--resume` CLI'nin
  tam sunucu-taraflı geçmişini yükler; TionHarness bir fold yapsa bile `planClaudeResume`
  yalnız **ham delta** gönderir ve özeti prompt `[Context]` bloğuna **eklerdi** → fold
  CLI penceresine hiç ulaşmaz, hatta özet kadar **büyütürdü** (mükerrer). Compaction
  yalnız `native` yol ve CLI **cold-start** için etkiliydi.
- **Çözüm:** `claudeResumeDecision` artık `compacted` parametresi alıyor; bir fold olan
  turda (geçerli warm boundary olsa bile) **cold-start**'a zorlar → `llmReq.Messages`
  Prepare'ın compacted kuyruğu (özet + `keepRecent`) olarak kalır ve **taze** bir CLI
  oturumuna gönderilir. `plan.sentCount=rawLen` korunur → sonraki turlar compacted
  temelden normal warm-resume eder. Maliyet: fold turunda tek seferlik cache-cold.
  `planClaudeResume` çağrısı `prep.Compacted`'i geçirir; testte yeni vaka
  ("compacted forces cold despite warm boundary"). Persistent yol etkilenmez
  (resume gate zaten kapalı).
- **Log:** Mevcut bir warm CLI oturumunu düşüren fold, Loglar'a
  `component=conversation` altında `cli resume reset: fold re-baselined the
claude-cli session` (prev_cli_session + raw_msgs) satırıyla işaretlenir —
  "context compacted" fold logunun yanında; tek cache-cold turu açıklar. İlk-tur /
  edit kaynaklı zaten-cold turlar loglanmaz (gürültü değil).

## Koordinatör ağaç bütçesi: görünürlük + reclaim (2026-08-04) ✅

- **Sorun (FND-23385d87 · FND-dad4f7be · FND-4240b836):** `CoordinatorMaxSubtreeSessions`
  tavanı GÖRÜNMEZDİ — koordinatör doluluğu ancak `tree budget exhausted (N/N)` ile
  duvara çarpınca öğreniyordu; kalan kota önceden görünmüyordu. Dahası bütçe
  `len(tree)-1` ile **tüm** oturumları (bitenler dahil) sayıyordu → uzun ömürlü
  koordinatör bitirdiği işin oturumlarıyla **kalıcı brick** oluyor, hata mesajı
  "conclude existing workers…" derken concluding kotayı boşaltmıyordu.
- **Çözüm:** (1) `agent.SpawnResult`/`tools.SpawnResult`'a `TreeBudgetUsed/Total`
  eklendi; her `spawn_worker` sonucu `Tree budget: N/M live … (K remaining)` satırı
  - %75/%90 `⚠️` uyarısı gösterir (`treeBudgetLine`). (2) `checkCoordinatorTreeBudget`
    → `evalCoordinatorTreeBudget` yalnız **CANLI** worker sayar
    (`countsAgainstTreeBudget` = koşan tur veya dallanan alt-koordinatör); bitenler
    otomatik **reclaim**. (3) Tükenme hatası artık hâlâ aktif sayılan worker'ları
    listeler (`coordTreeBudget.activeList`). Mutex/kilit mantığı (per-tree spawn lock)
    değişmedi.
- **Not (semantik):** Tavan artık _lifetime-toplam_ değil, _eşzamanlı-canlı_ worker
  tavanıdır. Üstel eşzamanlı patlama korumasını korur; ardışık batch üretimi ajan
  başına günlük token bütçesiyle sınırlanır.
- **Dosyalar:** `internal/agent/coordination.go`, `internal/agent/spawn.go`,
  `internal/tools/builtin_spawn.go`, `internal/tools/builtin_coordination.go`
  (+ testler: `coordination_test.go`, `coordination_race_test.go`,
  `builtin_coordination_budget_test.go`).
- **Doğrulama:** `go build ./internal/agent/... ./internal/tools/...` ✅,
  `go test ./internal/agent/... ./internal/tools/...` ✅ (588 test;
  `TestSpawnWorkerSubtreeBudget`, `TestSubtreeBudgetHoldsUnderConcurrentSpawns`,
  `TestTreeBudgetLine`).

## Koordinatör stall — sert-halt eskalasyonu + prompt/eager teyidi (2026-08-04) ✅

- **Sorun (FND-99caeb31 · FND-4fd06a80 · FND-c70cf8a7 · FND-8ea05c42):** Fantom-spawn
  stall koruması (`coordination_stall.go`) nudge bütçesi bitince yalnız gözlemlenebilir
  `DebugError` bırakıyordu — koordinatör hâlâ donuksa kullanıcı bilgilendirilmiyor ve
  otomatik-turlama katmanları sessizce vazgeçiyordu. Ayrıca koordinatör prompt'unda
  "düz metinde worker'dan bahsetmek spawn etmek değildir" kuralı açıkça yoktu.
- **Çözüm:**
  - **Sert-halt eskalasyonu (katman 3, `escalateCoordinatorStallHalt`):** nudge bütçesi
    tükendiği hâlde yargıç stall'ı **hâlâ** doğruluyorsa `slot.stallHalted` set edilir →
    drain döngüsü koordinatörü otomatik-turlamayı bırakır (re-arm YOK, idle-reconcile
    turu YOK) ve **kullanıcıya tek-seferlik** `coordination` bildirimi (sebep + nasıl
    devam edileceği) yayınlanır. Turn-end guard hem sweeper aynı yola girer; gerçek bir
    koordinasyon aracı çağrısı bayrağı temizler; sweeper backstop olarak açık kalır.
    `CoordinatorStallGuard` ayarına saygı gösterir.
  - **Prompt kuralı:** `prompts/defaults/coordinator.md` + `tionharness-coordinator/SKILL.md`
    altın kurallarına "worker'dan bahsetmeden ÖNCE `spawn_worker` çağır; mevcut worker'lara
    atıf öncesi `list_workers`; düz metinde 'worker başlattım' demek stall'a düşürür" eklendi.
  - **Eager teyit:** `spawn_worker`/`list_workers` registry'de varsayılan `Full` → CLI
    `core`/alwaysLoad tier'ında (deferred değil) olduğu doğrulandı; regresyon testi eklendi.
  - **UI — kalıcı "durduruldu" rozeti/CTA (2026-08-04):** halt durumu
    `session_info.coordinatorStallHalted` + koordinatör-ağacı düğüm `stallHalted`
    alanıyla sunulur (`Runtime.CoordinatorStallHalted`, in-memory slot). Koordinasyon
    panelinde kırmızı **"Koordinatör durduruldu"** rozeti + **"Devam ettir"** butonu
    (POST `/api/sessions/{id}/coordinator/resume` → `ResumeCoordinatorFromStall`:
    halt+streak temizler, bir tur kickler); ağaç görünümünde OctagonAlert işareti.
    Live güncelleme: `coordination` SSE olayı `workerBus`'a köprülenip panelin
    `session_info` refetch'ini tetikler. Restart'ta rozet sweeper penceresinde geri gelir.
    Testler: `TestCoordinatorStallHaltedReflectsState`, `TestResumeCoordinatorRejectsNonCoordinator`.
- **Doğrulama:** `go build ./internal/agent/... ./internal/prompts/... ./internal/skills/...` ✅,
  `go test ./internal/agent/...` (335) ✅ ve `./internal/api/...` tier testleri (yeni
  `TestCoordinationToolsAreEager`, `TestEscalateCoordinatorStallHaltIsOneShot`,
  `TestStallHaltStopsIdleReconcile`, `TestToolCallClearsStallHalt`) ✅.

## Boşta-zaman-aşımı tek-atımlık resume (2026-08-04) ✅

- **Sorun (FND-708844f8):** boşta gözcüsü (`ErrTurnIdleTimeout`) turu döngü DIŞINDA
  kesiyor; `recovery.go` kurtarma bütçeleri (`decideRecovery`) yalnız döngü-içi
  hatalar için tasarlı olduğundan, döngü-dışı boşta zaman aşımı hiçbir bütçeye
  girmiyor → kısmi iş kalıcı yarım kalıyordu. Faz E (2026-08-04, `1613777`/`6e0b1a8`)
  bu kesintiyi **görünür** kıldı (unfinished notu + koordinatöre "timeout" sinyali)
  ama **otomatik yeniden deneme** eklemedi.
- **Çözüm (`internal/agent/turnoutcome.go`):** `runTurnWithIdleResume` helper'ı
  turu, **yalnız `ErrTurnIdleTimeout`** ile kapandıysa taze boşta penceresiyle **bütçe
  kadar** otomatik yeniden başlatır; her resume önceki denemenin kurtarılan fragmanını
  `resumeContinuationPrompt` ile alır (kaldığı yerden sürer). **Sert tavan
  (`ErrTurnHardTimeout`) resume edilmez.** Bütçe tükenince tur yine unfinished
  raporlanır (Faz E) → koordinatör re-task (çift kurtarma değil, yerinde ilk savunma).
  Worker'da `workerCtl` `setCancel`/metot-`cancel` ile thread-safe; `stop_worker` her
  denemede uçuştaki turu keser.
- **Ayarlanabilir bütçe (adım 2):** `Tunables.IdleResumeMax` (ayar `idleResumeMax`,
  varsayılan `DefaultIdleResumeMax=1`, 0 = kapalı, [0,5] clamp) — settings.go/store.go/
  api server + frontend "Boşta yeniden başlatma" input'u ile uçtan uca bağlandı.
- **Koordinatör drain turu da kapsandı (adım 1):** `runCoordinatorTurn` de resume
  kullanır; drain/stall makinesine şeffaf (`lastTurnUnix` tur sonrası,
  `guardCoordinatorStall` yalnız temiz turda). Bağlı yollar: spawn / worker / inbox /
  wake / schedule / koordinatör.
- **Doğrulama:** `go build ./...` ✅, `go test ./internal/agent/... ./internal/settings/...
./internal/api/...` ✅ (533 test, `idleresume_test.go` 8 senaryo dâhil), frontend
  `tsc -b` ✅ + prettier ✅.

## send_to_worker meşgul-worker kuyruğu (2026-08-04) ✅

- **Sorun (FND-befa7846 · FND-c28c48d0 · FND-4ff2cecc):** Koordinatör bir worker'a
  `send_to_worker` çağırdığında worker hâlâ önceki turunu işliyorsa çağrı
  `worker ... is still running its previous turn` ile **reddediliyordu**; mesaj
  kayboluyor, tek çare yıkıcı `stop_worker` oluyordu. Worker başına backpressure yoktu.
- **Çözüm (`internal/agent/coordination.go` + `runtime.go` + `tools/builtin_coordination.go`):**
  worker başına **tek-slotluk bekleyen-mesaj kuyruğu** (`workerQueueMu` +
  `workerQueue`). `SendToWorker`: worker boşsa hemen teslim (`Delivered`), meşgulse
  mesajı kuyruğa park eder (`Queued`, çalışan turun süresi + "meşgul, tıkalı değil"
  ipucu ile) → gereksiz `stop_worker`ı önler; kuyruk doluysa ikinci mesaj **net hata**.
  Teslim `runWorker`'ın en-son çalışan `defer drainWorkerQueue`'una bağlı: tüm slot
  release + `untrackSession`'dan sonra kuyruğu `workerQueueMu` altında pop edip
  sıradaki turu başlatır (teslim edilemezse koordinatöre `failed` bildirimi). Busy-check
  - enqueue tek kritik bölümde → lost-update/TOCTOU yok. `Send`/`SendToWorker` imzası
    artık `(tools.SendResult, error)`.
- **Doğrulama:** `go build ./internal/agent/... ./internal/tools/...` ✅,
  `go test ./internal/agent ./internal/tools` ✅ (585 test; yeni `worker_queue_test.go`:
  busy→queued, ikinci mesaj→hata, tur bitince teslim, boş→hemen teslim).
- **UI:** `WorkerInfo.Queued` + `GET /sessions/{id}/workers` `queued` alanı; koordinasyon
  panelinde çalışan+bekleyen worker'a **"kuyrukta"** rozeti (`CoordinatorSection.tsx`).
  Backend+frontend: `go test ./internal/agent ./internal/api ./internal/tools` → 772 test ✅.
- Ayrıntı: `_Docs/47` (§10 "send_to_worker meşgul-worker kuyruğu"). Prompt/SKILL notu:
  `internal/prompts/defaults/coordinator.md` + `internal/skills/.../tionharness-coordinator/SKILL.md`.

## Grep çoklu-path desteği (2026-08-04) ✅

- **Sorun (FND-02391b62 · FND-be8c85b7 · FND-5253471e):** `Grep` `path` alanı tek
  string olduğu için, model birden çok dosyayı tek çağrıda taramak isteyip yolları
  virgülle birleştirince (`a_test.go,b_test.go`) tüm dize tek yol sanılıp
  `os.Stat`'ta `path does not exist` ile düşüyordu — üstelik hata devasa birleşik
  dizeyi geri yazdığı için hangi yolun geçersiz olduğu anlaşılmıyordu.
- **Çözüm (`internal/tools/builtin_grep.go` + `grep_rg.go`):** `path` artık
  virgül/noktalı-virgülle ayrılmış çoklu hedefi destekler. Yeni `resolveGrepTargets`
  her parçayı ayrı çözer (önce **tüm-dize** dener → virgül içeren gerçek yol da
  çalışır), geçerlileri toplar; rg hızlı-yolu sandbox kökünden **çoklu positional
  target** ile koşar (Go fallback ile bayt-uyumlu), Go motoru dizinleri gezip
  sonuçları kök-göreli birleştirir. Eksik parça(lar) varsa `grepMissingPathErr`
  **yalnız geçersiz yolları** (en çok 3) + "ortak üst dizini `path` yapıp `glob` ile
  daralt / her yol için ayrı çağrı" yönergesini döndürür (birleşik dizeyi yazmaz).
- **Doğrulama:** `go build ./...` ✅, `go test ./internal/tools -run Grep` ✅
  (+ `TestGrepMultiPath`, `TestGrepMultiPathMissing`, `TestGrepMultiPathRGParity`).

## Playwright MCP izinli-kök = oturum scratchpad (2026-08-04) ✅

- **Sorun (FND-a96f35e0/f45b51ab/0a4871a6/bb25e7e3):** Playwright MCP dosya
  yazımını izinli köklerine (bu client MCP root ilan etmediği için de cwd'sine)
  kısıtlıyor; TionHarness ise ajana çıktı yolu olarak oturum scratchpad'ini
  veriyordu → `browser_take_screenshot`/PDF her çağrı `File access denied:
outside allowed roots`. Ek: paylaşılan havuz bağlantısı bayat oturuma çözülüyordu
  (SES4'te SES1 scratchpad'i).
- **Fix:** `mcp.ServerConfig`'e `Dir` (stdio alt-sürecin cwd'si; `DialStdio`
  `cmd.Dir`) + yeni `internal/agent/mcp_playwright.go`
  (`applyMCPScratchpadRoot`): dosya-yazan sunucu için scratchpad **her build'de
  aktif oturumdan yeniden çözülür**, `cfg.Dir`=scratchpad + `--output-dir=` +
  (session,agent) scope. `Dir` parmak izinde olduğu için bayat kök yerine yeniden
  dial. Çözülemezse uyarı loglar (sessiz yutmaz). `toolsetup.go` döngüsünde tek
  satır çağrı — codebase-memory bloğuyla çakışmayacak biçimde ayrı dosyada.
- **Doküman:** `_Docs/52-MCP-GATEWAY.md` + workspace `CLAUDE.md` ("## Playwright
  MCP": izinli köke kaydet, scratchpad'e mutlak yol verme, `Read` ile oku).
- **Doğrulama:** `go build ./internal/mcp ./internal/agent` ✅, `go vet` ✅,
  `go test ./internal/mcp ./internal/agent` ✅ (+ `TestIsFileWritingMCP`,
  `TestEnsureOutputDirArg`).

## Shell-kapalı farkındalığı: ölü-araç kuralı (2026-08-04) ✅

- **Sorun (FND-9c9a52aa · FND-6095a777 · FND-e9c79d9a · FND-495575b8):** Kabuk
  araçları (Bash/PowerShell) kapalı oturumlarda ajana bu bildirilmiyordu. Ajan
  bare `PowerShell`/`Bash` çağırıp _"No such tool available … not enabled in this
  context"_ alıyor, muadiline geçmeyip **aynı çağrıyı tekrarlıyor** ve 5 dk'lık tur
  zaman aşımına düşüyordu. Ayrıca terminal varsayan rehberlik (`rtk`, `go test`,
  `npm`) shell'siz ajanlara da telkin ediliyordu.
- **Çözüm (`internal/agent/runtime.go` → `ShellToolsContextBlock`):** Blok artık
  **hiç boş dönmez** ve shell yeteneğinin **tek kaynağıdır**. Gate açık + backing
  shell varsa eskisi gibi Bash/PowerShell'i adıyla duyurur; **kapalıysa** (gate
  kapalı VEYA `tools.ShellToolNames()` boş) "Shell execution is DISABLED" +
  **ölü-araç kuralını** enjekte eder: bir araç "not enabled in this context" derse
  onu yok kabul et, aynı çağrıyı tekrarlama, hedefe fs araçlarıyla (Read/Glob/Grep/
  Edit/Write) ulaş ya da durumu raporla; terminal varsayan rehberlik bu bağlamda
  geçersizdir. Kaynak tek → chat (`composeTurnRequest`) ve headless
  (`autonomousDynamicSuffix`) yolları aynı metni paylaşır; volatile dinamik suffix
  (gate mid-session değişebilir), epoch snapshot ile cache-güvenli.
- **Doküman:** [53-CRAFTAGENT-PROMPT-PARITE.md](53-CRAFTAGENT-PROMPT-PARITE.md)
  shell-gate satırı güncellendi.
- **Doğrulama:** `rtk go build ./...` ✅.

## View katmanı göç 2/4: handoff — ölü alan + üç kopya (2026-08-04) ✅

- **Bulunan hata:** `conversation.HandoffEnv.Todos` alanı **tanımlıydı, render'ı
  ve testi vardı — ama hiç kimse doldurmuyordu.** Her handoff, devralan ajanın en
  çok ihtiyaç duyduğu şey olmadan üretiliyordu: neyin bitmiş, neyin açık olduğu.
  `agent.handoffEnv` artık zaten yüklü transkriptten (ikinci okuma yok) dolduruyor.
- **`internal/view/todo.go`** — `TodoRollup` (Items + Done + Active) +
  `LatestTodos(msgs)` + `RenderChecklist()`. "Transkriptteki en yeni checklist'i
  bul" **üç yerde** ayrı yazılmıştı; tek uygulamaya indi:
  - `view/session.go`: `latestTodos` silindi → `LatestTodos` + yerel Türkçe satır
  - `api/todos.go`: `latestSessionTodos` + `parseMessageSteps` + `stepTodos`
    **silindi** → `LatestTodos` + `RenderChecklist`
  - `agent/handoff.go`: _(hiç yoktu — alan ölüydü)_ → artık dolduruyor
- **Adım çözme artık tek evde.** `agent.TurnStep`'i yapısal çözen kod dört yerde
  tekrarlıyordu; üçü silindi, geriye `view/step.go` kaldı.
- **Bilinçli fark:** sistem-prompt bloğu bitmiş listeyi **gizler** (izlenecek şey
  kalmadı), handoff **gösterir** — "bunlar zaten yapıldı", taze ajanın işi baştan
  yapmasını engelleyen şeyin ta kendisi. `RenderChecklist()` her şeyi basar,
  gizleme kararı çağırana bırakılır (`AllDone()`).
- **1-tabanlı indeksler korunur:** `todo_write {"set":{"3":"completed"}}` onları
  kullanıyor; düşüren bir renderer ajanı listeyi güncelleyemez hâle getirirdi.
- **Testler:** `view/todo_test.go` (7 — en yeni liste, boş≠bitmiş ayrımı, AllDone,
  indeks korunumu, tamamlanmış listenin korunması, legacy input) +
  `conversation/handoff_env_test.go` (2 — checklist render'ı, olmayan checklist'in
  boş başlık üretmemesi) + `api/todos_test.go` bu yüzeye özgü sözleşmeye daraltıldı.
- **Durum: 2/4 göç etti.** Kalan: `conversation` summarizer.

## View katmanı göç 2 (kısmi): insight paylaşılan primitifler (2026-08-04) ✅

- **Önce bir düzeltme:** [66](66-VIEW-KATMANI.md)'da aday olarak "insight
  prefilter" yazıyordu — **yanlış adaydı**. `Prefilter.Match` bir _filtre_
  (`SessionSignals` → `bool`), hiç metin üretmiyor; view katmanıyla paylaşacağı
  bir şey yok. Asıl duplicate özetleyici **`Scanner.buildSlice`**. Doküman düzeltildi.
- **`buildSlice` bilerek `View` YAPILMADI.** Farklı soru ("bu hipotez için kanıt
  ne?" vs "bunun durumu ne?"), farklı kapsam (**tüm transkript** vs son 40 mesaj —
  300 mesaj önceki hata tam da insight'ın aradığı şey), farklı içerik (yalnız
  hata/recovery vs sağlıklı durum + sinyaller). Zorlamak kabul testini harfiyen
  geçirir ama ruhunu ıskalardı: amaç dosya taşımak değil **tekrarı yok etmek**.
- **`view.Step` + `DecodeSteps` (`step.go`)** — kalıcı `agent.TurnStep`'i yapısal
  çözen **tek ev**. `view/session.go`'daki `stepLite` ile
  `insight/scanner.go`'daki `rawStep` birbirinin kopyasıydı (ikisi de aynı
  cycle'dan kaçmak için vardı); **ikisi de silindi**. `Step.TodoItems()` legacy
  `todo_write` input formunu tolere eder.
- **`view.CapLines` (`cap.go`)** — bütçeyi **kayıt sınırında** uygular, düşeni
  **sayar**. `buildSlice` eskiden `out[:sliceCap]` ile **bayttan** kesiyordu: son
  kayıt satır ortasından bölünüp **eksik ama tam görünen** bir şeye dönüşüyordu ve
  `…(truncated)` ne kadar düştüğünü söylemiyordu. Artık
  `…(%d more error/recovery step(s) omitted for size)`; ayrıca **adımlar olaylara
  önceliklidir** (birincil kanıt onlar, debug olayları büyük ölçüde tekrar).
- **Testler:** `view/cap_test.go` (8 — kayıt bütünlüğü, bütçe muhasebesi, bozuk
  trace toleransı, legacy todo) + `insight/slice_test.go` (4 — düşenin
  raporlanması, sınırda kayıt bütünlüğü, temiz dilimde yanlış omission iddiası
  olmaması, adım önceliği). Mevcut insight testleri değişmeden geçti.
- **Durum: 1 tam + 1 kısmi göç.** Kalan gerçek adaylar: **handoff**
  (`ProjectSession(full)` ile örtüşüyor) ve `conversation` summarizer.

## View katmanı kabul testi: coordinator worker-state bloğu göçtü (2026-08-04) ✅

- **Neden:** [66](66-VIEW-KATMANI.md)'nın kabul testi — katman kurulup eski ad-hoc
  özetleyiciler yerinde kalırsa bu, beşinci bir özetleyici olurdu. İlk göç:
  `Runtime.coordinatorWorkerStatusBlock`. **36 satırlık render mantığı silindi**,
  geriye 16 satırlık veri toplama + `view.ProjectWorkers` çağrısı kaldı.
- **`internal/view/workers.go`** — koordinatörün canlı worker filosu. Girdi
  runtime state olduğu için (`agent` → `tools` → `view` cycle'ı) `WorkerInfo`
  import edilmez; `agent` tarafında `toViewWorkers` map'ler.
- **Kazanç 1 — elision cap.** Blok her koordinatör turuna enjekte edilen bir
  **push** kanalı ve **sınırsızdı**; 40 worker'lı filo her turda 40 satır basardı.
  Artık 20 ile sınırlı, **çalışanlar önce** korunur (cap'in koordinatörün beklediği
  satırı düşürmesi en kötü sonuç olurdu) ve **özet satırı tüm filoyu** sayar →
  gizlenenler aritmetiği bozmaz.
- **Kazanç 2 — geçen süre.** `WorkerInfo.StartedAt` vardı ama yalnız UI banner'ı
  kullanıyordu. Artık prompt'ta `RUNNING for 14m00s`; başlangıç bilinmiyorsa süre
  **basılmaz** (uydurma yerine sessizlik).
- **Korunan davranış:** "trust THIS over the notifications in history" çerçevesi,
  `DELEGATING` durumunun açık ifadesi, filo boşalınca verilen kapanış dürtüsü —
  üçü de coalesced-notification stall'ını engellediği için kelimesi kelimesine
  taşındı ve teste bağlandı.
- **Bilinçli istisna:** bu, paketteki tek **İngilizce** projeksiyon — tek tüketicisi
  koordinatörün sistem prompt'u ve komşu blokların hepsi İngilizce. Bunun için
  `View.ElidedNote` eklendi: yapısal `Elided` yine set edilir (sessiz kesme yasağı
  bozulmaz), yalnız render edilen cümle override edilir.
- **Testler:** `view/workers_test.go` (5 — çerçeve korunumu, geçen süre + bilinmeyen
  başlangıç, boş filo dürtüsü, cap altında aritmetiğin bozulmaması, boş filo).
  `internal/agent` paketi ve tüm suite yeşil.
- **Durum: 1/4 göç etti.** Kalan adaylar: insight prefilter, handoff,
  `conversation` summarizer.

## Panel (dashboard) ekranı + workspace projeksiyonu — Faz 6 (2026-08-04) ✅

- **İstek:** "Workspace'in genel durumunu grafiklerle görebilmek ve genel
  workspace özetini oradan da okuyabilmek."
- **`internal/view/workspace.go`** — roll-up projeksiyonu (Faz 6). Başlık:
  ajan/oturum(aktif)/kart/koşu sayıları + bugünkü token. Gövde: pano histogramı,
  koşu durum satırı ve L1 sinyaller — takılmış oturumlar, cevap bekleyen sorular
  (**en eskisi** raporlanır, çünkü bekleme süresini o sınırlar), başarısız koşular,
  bozuk zamanlamalar, başarısız/yaşlanmış kartlar, koordinatör oturumları.
  **Tamamı bellek-içi store okuması** → disk I/O yok, poll edilebilir.
  **Devre dışı** bir zamanlamanın son hatası raporlanmaz: bilerek duraklatılmış
  şeyi "bozuk" göstermek okuyucuya satırı yok saymayı öğretir.
- **`GET /api/dashboard?days=N`** (`internal/api/dashboard.go`) — sayaçlar +
  seriler + projeksiyon metni tek çağrıda. Seriler **backend'de** toplanır;
  tarayıcının sayabilmek için tüm workspace'i indirmesi, bu katmanın önlemek için
  var olduğu maliyetin ta kendisi olurdu. Gün ekseni **her zaman tam pencere** →
  sessiz bir gün trendden silinmez, boşluk olarak görünür. `days` 1..90 arası
  clamp'lenir. Token serisi Bütçe ekranıyla **aynı** usage rollup'ından okunur →
  iki ekran farklı rakam söyleyemez.
- **UI `features/dashboard/`** — sol navigasyonda **Panel**. Stat kutuları
  (sorunlu olanlar renkli), **◱ Workspace özeti** bloğu (ham DSL, `~N tok`,
  `asOf`), 3 günlük trend (oturum/koşu/token, 7·14·30·90g), 3 kompozisyon çubuğu
  (pano/koşu/oturum türü), ajan sıralaması. Grafikler **elle yazılmış SVG/CSS**
  (`charts.tsx`) — yeni bağımlılık yok, `sessions/viz` idiomu; tema değişkenleri
  bedavaya geliyor. Her grafik **kendi boş durumunu kelimeyle söyler** (boş alan
  "veri yok" mu "yüklenemedi" mi belli olmaz).
- **Temel kurgu:** üstteki metin bloğu projeksiyonun **kendisi**, altındaki
  grafikler aynı gerçeklerin çizilmiş hâli — ikinci bir bağımsız hesap değil.
  Çelişirlerse kullanıcının görebildiği bir bug olur.
- **`get_view` artık `workspace` kind'ını da alır** (id boşsa `"workspace"`a düşer).
- **Testler:** `view/workspace_test.go` (8) + `api/dashboard_test.go` (4: seri
  şekli, pencere clamp'i, gün ekseni, pencere-dışı damga elenmesi). İzole bir
  instance'ta (ayrı port + geçici store) uçtan uca doğrulandı.

## View katmanı — Faz 4: board + session projeksiyonları (2026-08-04) ✅

- **`internal/view/board.go`** — pano projeksiyonu. Sütun histogramı + L1
  sinyaller: çalışan sütunda (`in_progress`/`review`) >3g hareketsiz kart,
  başarısız kartlar, bağımlılıkla bloke kartlar, gecikmiş kartlar, Δ24s hareket,
  sahipsiz kart sayısı. **8 kart → 62 token.** Sütunlar workspace ayarından
  DEĞİL kartlardan türer → paket settings bağımlılığı almaz ve view var olan
  panoyu raporlar (yapılandırılmış-ama-boş sütun görünmez). Yerleşik sütunlar
  önce, özel sütunlar alfabetik sonra. Kart listesi card seviyesinde bilerek
  yok → `Elided = tüm kart sayısı`.
- **`internal/view/session.go`** — oturum projeksiyonu, **transkript okumadan**:
  header + usage bellek-içi, geri kalanı yalnız son 40 mesajlık kuyruktan
  (`tiny` seviyede hiç dosya okunmaz). Rolling summary, todo ilerlemesi
  (`2/4 tamam · şu an: …`), L1 sinyaller (StuckTurns, durable ask beklemesi, son
  hata adımı, alarm etiketleri, handoff soyağacı, compaction). Coordination
  soyağacı **başlıkta** — bir worker'ın "stuck"ı koordinatörünün de sorunu.
  **214 mesajlık oturum → 60 token.** Okunmayan mesaj sayısı `Elided`'e yazılır.
- **Import cycle'dan kaçınma:** `view` paketi `internal/agent`'ı import edemez
  (agent → tools → view). `TurnStep`/`TodoItem` yapısal olarak `stepLite`/
  `todoLite` ile çözülür — okunan alanlar zaten kalıcı oturum formatının parçası.
- **`Elided` artık birimli** (`ElidedUnit`: kart / eski mesaj / node). Çıplak
  sayı belirsizdi: 174 gizli mesaj ile 174 gizli kart okuyucu için aynı şey değil.
- **`get_view` üç kind'ı da alır** (`flowrun|session|board`); board'da `id`
  boşsa `"board"`a düşer (panonun kendi id'si yok, bu bir hata değil).
- **UI:** `◱ Özet` butonu **chat header**'ına (→ session) ve **Görevler**
  PaneHeader'ına (→ board) eklendi. **İsim değişti: "Bağlam" → "Özet"** — chat
  header'ında zaten bir "Bağlam" butonu var ve o sonraki turun **ham prompt'unu**
  önizler; aynı çubukta iki "Bağlam" kötü olurdu.
- **Testler:** `board_test.go` (5) + `session_test.go` (6) + genişletilen
  `api/views_test.go` (board/session uçtan uca). Tüm suite + frontend build yeşil.
- **Uyarı — kabul testi hâlâ sağlanmadı:** katman kuruldu ama mevcut ad-hoc
  özetleyicilerden (coordinator worker-state bloğu, insight prefilter, handoff,
  `conversation` summarizer) hiçbiri henüz göç etmedi. Göç olmadan bu, beşinci
  bir özetleyici olarak kalır — sıradaki iş bu.

## View (projeksiyon) katmanı — Faz 1-3 (2026-08-04) ✅

- **İstek:** "Büyük verilerin toplandığı yerlerin (uzun session, flow akışı, çok
  kartlı board) o anki durumunu bağlamı şişirmeden, bütün ajanlara verilebilecek
  şekilde çıktı veren bir sistem" + "bu değerleri UI'da bir butonla/panelle
  görebilelim". Tasarım notu: [66-VIEW-KATMANI.md](66-VIEW-KATMANI.md).
- **Yeni paket `internal/view`** (leaf; `db`+`orchestration` okur):
  `Project(ref, level) → View{Header, Body, Handles, AsOf, Source, Elided,
Tokens}`. Üretim **deterministik**: L0 sayım + L1 kural-tabanlı sinyal, LLM yok
  → sayılar uydurulamaz. Bütçe tier'ları `tiny`/`card`/`full`. Çıktı JSON değil **satır-bazlı kompakt
  DSL** (JSON'un tekrar eden anahtarları bu ölçekte saf token israfı).
- **İlk entity: flow run** (`flowrun.go`). Paralel node'un çocukları ebeveyne
  katlanır (`parallel:fan[2/2✓ 48s]`); ardışık node süresi trace damgalarının
  farkından türer (motor sırayla koştuğu için bu gerçek duvar saati); koşunun
  park ettiği node (running/waiting/failed) trace'te olmasa da gösterilir.
  7 node'luk bir koşu **47 token**, 40 node'luk koşu iki satır. `sub` ile
  tek-node drill-down'ı.
- **Sessiz kesme yok:** `View.Elided` her zaman render edilir; zincir 24
  segmenti aşarsa ortası katlanır ve kaç node atlandığı yazılır.
- **Hata yutulmaz:** bilinmeyen kind → 400, olmayan koşu → 404, bozuk graph/state
  JSON → hata. Boş view "sağlıklı boş entity" gibi okunacağı için asla
  döndürülmez.
- **API** `GET /api/views/{kind}/{id}?level&sub` (`internal/api/views.go`);
  yanıt `text` alanını taşır — ajanın aldığı baytların aynısı.
- **Araç** `get_view` (`internal/tools/builtin_view.go`) — pull kanalı, her ajana
  açık, kategori `diagnostics`. Push (dinamik suffix) bilerek yapılmadı: canlı bir
  özeti her tura enjekte etmek prompt cache'ini kırar ([57](57-PROMPT-EPOCH.md)).
- **UI `frontend/src/features/view/`** — `◱ Özet` butonu (Akışlar ▸ Koşular
  başlığı + `RunView` özet satırı) → sağdan `ViewPanel` sheet'i: level
  seçici, `asOf` + `~N tok`, **ham DSL monospace** (güzelleştirilmiş kart değil →
  projeksiyon yanlışsa kullanıcı görür), `elided` satırı, tıklanabilir handle'lar
  - breadcrumb, Kopyala.
- **Testler:** `internal/view/flowrun_test.go` (7 senaryo) +
  `internal/api/views_test.go` (200/400/404). Tüm backend suite + frontend build
  yeşil.
- **Sırada:** faz 4 = `board` + `session` view'ları ve mevcut ad-hoc
  özetleyicilerin (conversation summarizer, coordinator worker-state bloğu)
  bu katmana göçü — kabul testi eskisinin **silinmesi**.

## Edit/Read eşleştirme sağlamlığı (2026-08-04) ✅

- **İstek (FND-84c0981b + unicode bulguları):** `old_string` hafızadan yeniden
  yazıldığında dosyadaki gerçek metinle (unicode « » ✅ ⏳, boşluk/hizalama) birebir
  eşleşmiyor ve Edit "String to replace not found" ile düşüyordu.
- **Kod (`internal/tools/builtin_fs.go`, `computeEdit`):** eşleştirme tek bir
  yardımcıya toplandı ve kademelendi — (1) birebir → (2) cat -n satır-no / CRLF
  normalizasyonu → (3) **boşluğa toleranslı, satır-tabanlı fallback** (önce satır-sonu
  boşluk, sonra girinti; yalnız **tekil** konumda ateşler, `replace_all` yoksa çoklu
  adayda hata; CRLF satır-sonu korunur — `fuzzyLineMatch`/`applyRanges`). Hiç eşleşme
  yoksa `diagnoseNoMatch` **en yakın dosya satırını + ilk farklılaşan sütunu** gösteren
  ve "kısa BENZERSIZ ASCII parça hedefle" diyen tanılayıcı hata döner; asla no-op yok.
- **Prompt/doküman:** `Read`/`Edit` tool açıklamalarına "old_string'i birebir kopyala,
  normalize etme, unicode/hizalamayı koru; tutmazsa benzersiz kısa ASCII parça" kuralı;
  `CLAUDE.md` "Edit aracı — eşleşme" bölümü; `56-SELF-HEALING.md` girdisi.
- **apply_patch'e taşındı:** `applyHunks` de aynı kademeli toleransı kullanır
  (`blockMatchesNorm`, cursor'dan sonra tekil konum; `diagnoseHunkMismatch`). Tool
  açıklaması güncellendi. Testler `builtin_patch_match_test.go`.
- **Doğrulama:** `go build ./...` ✅, `go test ./internal/tools` 246 ✅ (yeni
  `builtin_edit_match_test.go` + `builtin_patch_match_test.go`: fuzzy trailing-ws /
  girinti / CRLF-koruma / ambiguity / no-match tanılama).

## MCP "indekslenmemiş proje" onarım kuralı (2026-08-04) ✅

- **İstek:** `mcp__codebase-memory-mcp__search_code` indekslenmemiş projeyle
  çağrılınca "project not found or not indexed" dönüyor; ajan hatayı
  `list_projects`'e / otomatik indekslemeye çevirmeden aynı çağrıyı tekrarlıyordu.
- **Backend (`internal/agent/mcprepair.go` — yeni, izole):** tur-ömürlü
  `mcpRepair` guard'ı. `repair()` post-execution'da not-indexed gövdesini
  `available_projects` yönergesine çevirip sonuca ekler + çağrıyı poison'lar;
  `precheck()` aynı araç+argümanlı ikinci çağrıyı (hard-stop'tan bağımsız)
  sunucuya gitmeden reddeder. Yönerge doğru `list_projects` sibling aracını,
  `C-Users-user-Desktop-<repo>` formatını ve repo listede yoksa Glob/Grep
  fallback'ini söyler.
- **Wiring (`toolloop.go`):** `guard` yanında `newMCPRepair()`; `guard.check`
  öncesi `precheck`, `guard.observe` sonrası `repair` kancası. Debug:
  `mcp_repair_block` / `mcp_repair`.
- **Doküman kuralı:** kök `CLAUDE.md` → "codebase-memory-mcp kullanımı" (önce bir
  kez `list_projects`, `project`'i birebir kopyala, yoksa Glob/Grep). Teknik not:
  `_Docs/11-INTERACTION-MCP.md`.
- **Doğrulama:** `go build ./internal/...` ✅, `go test ./internal/agent ./internal/tools`
  ✅ (+ yeni `TestMCPRepair_*`, `TestParseAvailableProjects`).

## Hit-limit hata kartı + salt-okunur retry (2026-08-04) ✅

- **İstek:** "Session'lar hit-limit hatası dönerse hata mesajı gibi göster ve
  Yeniden Dene ile sürdürelim; salt-okunur oturumlarda da hatadan sonra tekrar
  denemeyi mümkün kıl."
- **Backend (`internal/agent/errclass.go` + `toolloop.go`):** yeni
  `limitErrorText(errClass)` yardımcı fonksiyonu — `rate_limit`/`overloaded`/
  `billing` sınıfları için net Türkçe, retry-odaklı mesaj (diğerlerinde "").
  `fail()` closure'ı terminal hatayı sınıflandırır; limit sınıfıysa ham
  "anthropic HTTP 429…" metnini açıklamayla değiştirir ve error step `Reason`'ını
  spesifik tag'e çeker (ham detay altta kalır). Muhafazakâr sınıflandırma →
  guardrail/max-iters hataları etkilenmez.
- **Frontend `ErrorStep.tsx`:** `rate_limit`/`overloaded`/`billing` reason'larını
  tanıyıp ⛔ başlık + "Yeniden dene ile sürdür" ipucu gösterir.
- **Salt-okunur retry:** `performRetry` artık `preserve` parametresi alır;
  read-only olmayan chat eskisi gibi hatalı çifti silip yeniden gönderir,
  read-only run-log'larda (`retryMessagePreserve`) transkripti **silmeden**
  tetikleyici prompt'u yeniden kuyruğa alır (audit izi korunur; backend enqueue
  her oturum türünü kabul ettiği için tur gerçekten yeniden koşar). ChatView
  read-only iken preserve varyantını geçer; banner metni retry'ı belirtir.
- **Doğrulama:** `go build ./...` ✅, `go test ./internal/agent ./internal/providers` ✅
  (+ yeni `TestLimitErrorText`), `tsc -b` ✅, `vitest` 151 ✅.

## Sohbet listesi sekme belirteçleri + sekme sırası (2026-08-04) ✅

- **İstek:** "Aktif, Arşiv ve Workers sekmelerinde tamamlanan ve devam eden
  oturum belirteçlerini görelim; ayrıca Arşiv sekmesini en sona al."
- **Yapılan (`frontend/src/features/sessions/SessionsSidebar.tsx`):**
  - Eski `archivedCount`/`workerCount` memo'ları tek `tabStats` memo'suyla
    değiştirildi — her sekme kapsamı (active/workers/archived) için
    `{ongoing, completed}`. `ongoing` = satır-başı canlılık ölçütünün aynısı
    (`streamingSessionIds` ∪ `runtimeById.running`), `completed` = total−ongoing.
  - Yeni `TabActivity` bileşeni: yeşil nabız-nokta + devam eden sayısı, soluk
    tamamlanan sayısı; her yarım sıfırken gizlenir (boş sekme hiçbir şey göstermez).
    Üç sekmeye de (Aktif dahil) uygulandı.
  - Sekme sırası **Aktif → Workers → Arşiv** (Arşiv en sona alındı); tüm sekmeler
    aynı `flex items-center justify-center gap-1` düzenine hizalandı.

## Z.ai GLM sağlayıcısı (6. kind, Anthropic modu) ✅ (2026-08-03)

- **İstek:** "Z.ai GLM-5.2 gibi modellerin desteğini ekleyebilir miyiz?" — Z.ai,
  GLM ailesini hem OpenAI- hem **Anthropic-uyumlu** uçtan (`https://api.z.ai/api/anthropic`,
  istemci `/v1/messages` ekler) sunuyor; Anthropic yolu araç kullanımı + streaming +
  düşünmeyi native taşıdığı için `minimax-anthropic` kalıbıyla eklendi.
- **Yeni kind:** `internal/providers/kind_zai.go` — self-registering manifest
  (`Kind:"zai"`, Order 5, `AllowCustomModel`), `NewAnthropic(key).WithEndpoint("zai", …, "glm-5.2")`.
  Kendi API anahtarı (OpenRouter komisyonu yok, GLM Coding Plan aboneliğini kullanır).
- **Bağlantılar (OpenRouter kalıbının aynısı):** `registry.go` (zaiKey/zaiBaseURL +
  `SetZAI`/`ZAIConfigured` + resolve `case "zai"`), `settings/settings.go`
  (`ZAIKeyEnc`/`ZAIBaseURL` struct+DTO+patch) + `store.go` (`ZAIKey()` accessor +
  patch şifreleme + BaseURL applyString), `api/server.go` applySettings `SetZAI`,
  `providers/pricing.go` (`zai` GLM fiyat tablosu — **gerçek z.ai fiyatları** 2026-08:
  GLM-5.2 $1.40/$4.40, 5.1 $0.97/$3.04, 5 $0.60/$1.92, 4.7 $0.60/$2.20, 4.7-flash
  $0.06/$0.40 · 1M; cache read mult ~0.19). Market girdileri de tazelendi
  (`pricing_market.go` `glm-anthropic` + `zhipu-glm` → güncel GLM-5.x lineup; not:
  dosya "gen_providers.py generated" başlığı taşır ama generator+`data/` repoda yok →
  fiilen elle bakılıyor, doğrudan düzenlendi). Frontend: `types/settings.ts`,
  `SettingsPanel.tsx` (keyPatch/clearKey/applyKey union'a `'zai'`), `ProvidersPanel.tsx`
  (**Z.ai GLM** BuiltinProvider kartı). Test: `kind_test.go` katalog 5→6.
- **Not:** GLM-5.2 ayrıca OpenRouter kataloğunda (`z-ai/glm-5.2`) ve Özel Sağlayıcı ile
  zaten erişilebiliyordu; bu native kind doğrudan/ucuz yolu ekler. Detay `01` §6.

## Token-eşiği tetikleyicili otomasyonlar (3. tetik türü) ✅ (2026-08-03)

- **İstek:** "Belli token geçilince kendi kendine optimizasyon/temizlik/bakım
  otomasyonu çağır." Otomasyon motoruna **üçüncü tetik türü** (`TriggerKind="token"`)
  eklendi — etiket ve pano'nun yanına.
- **Semantik — "her-N" (tekrarlı, stateless):** `TokenThreshold` bir *aralık*tır;
  kümülatif harcama her katını geçince ateşler (100k → 100k/200k/300k…). Geçiş,
  `RecordUsage` içinde _önceki_ vs _yeni_ toplamdan `crossedMultiple`'la stateless
  tespit edilir → **per-scope defter yok**. `TokenScope`: `session` (oturum ömrü,
  `SessionUsage`) veya `workspace` (bugünkü toplam, `WorkspaceTokensToday`). Token =
  `input+output+cacheRead+cacheWrite`. Guardrail'ler (cooldown/maxIter/expiry) ortak.
- **Akış:** `agent/budget.go` `RecordUsage` → `Runtime.FireUsageRecorded`
  (`UsageRecorded{SessionID,DeltaTokens,SessionNewTotal}`, detached) → `rt.AddUsageHook`
  (workspace manager) → `AutomationEngine.OnUsageRecorded` → `fireToken` (guardsPass +
  LaunchRun; session spawn'ı geçiş oturumunu `ParentSessionID` yapar, kendini döngülemez).
- **Dosyalar:** `db/models_automation.go` (TriggerToken + TokenScope/TokenThreshold +
  ValidTokenScope), `db/automation_limits.go` (ValidateTokenThreshold, min 1000),
  `db/store_usage.go` (WorkspaceTokensToday) + `store_session_usage.go` (TotalTokens),
  `agent/runtime.go` (usageHooks + FireUsageRecorded), `agent/budget.go`, `agent/automation.go`
  (OnUsageRecorded/fireToken/tokenVars/crossedMultiple), `agent/launch.go`,
  `workspace/manager.go`, `api/automations.go`, `tools/builtin_automationmgmt.go` + testler.
  Frontend: `types/task.ts`, `api/tasks.ts`, `schedules/automationMeta.ts` + `AutomationFields`/
  `AutomationModal`/`AutomationCard`/`AutomationBoard` (**4. şerit ⚡ Token**). Detay `46` §2.6.

## Handoff continuation "Sohbet" filtresinde görünmüyordu → chat kind mirası ✅ (2026-08-03)

- **Sorun:** Handoff (context reset) sonrası taze devam oturumu sidebar'da
  görünmüyordu. Kök neden: `HandoffSession` → `SpawnSession` continuation'ı **her
  zaman `kind="spawned"`** damgalıyordu; sidebar'ın **"Sohbet"** kind-filtresi ise
  yalnız `kind ∈ {"", "chat"}`'i gösteriyor (`matchesKindFilter`). `selectSession`
  transcript'i açıyor ama kind filtresini değiştirmiyor → satır gizli kalıyor.
  Çelişki: `isWritableSessionKind` yorumu spawned/handoff çocuklarının **bilerek
  insan-devamlı sohbetler** olduğunu söylüyor, ama "Sohbet" sekmesi onları dışlıyordu.
- **Çözüm:** `SpawnOptions.Kind` override alanı (worker branch'ini ezmez) +
  `continuationKind(parentKind)`: **chat ebeveyn (veya legacy `""`) → `chat`**
  continuation, diğer tüm türler yazılabilir `spawned`'a düşer (task/flow/schedule
  non-writable, worker/flow-coordinator back-link taşıyan tree kind'ları sızmasın).
  Böylece sohbetten yapılan handoff "Sohbet" sekmesinde ebeveyninin yanında belirir.
- **Dosyalar:** `internal/agent/spawn.go` (`SpawnOptions.Kind` + override),
  `internal/agent/handoff.go` (`continuationKind` + spawn'a geçiş),
  `internal/agent/handoff_test.go` (yeni). Detay `35`.

## Koordinatör donma koruması: prose-regex → yargıç + gecikme tarayıcısı ✅ (2026-08-03)

- **Sorun (WS17/SES101 nüksü):** 2026-08-02'de eklenen `guardSpawnHallucination`
  prose-regex'i (`coordSpawnClaimRe`) uzun koşuda **sözlük-kaymasını kaçırdı**.
  Koordinatör 15 tur sonra fantom spawn'ı "worker" demeden anlattı: _"Round 15
  açıldı — 2 kol · SES144 (Kol AM) · SES145 (Kol AN)"_. Regex yalnız `worker`+fiil
  ya da `[running]` aradığı için eşleşmedi → guard hiç ateşlenmedi (debug'da 0
  guardrail), koordinatör olmayan SES144/145'i bekleyip dondu. 42 önceki
  `spawn_worker` gerçekti; yalnız son tur `steps=[]`.
- **Kök karar:** prose-regex kırılgan (determinizm değil, anlam sorunu). **Kaldırıldı.**
  Yerine `internal/agent/coordination_stall.go`:
  - **Deterministik kapı** (regex yok): tur hiç koordinasyon aracı çağırmadı **ve**
    0 çalışan worker → ancak o zaman sınıflandırıcı çalışır (paid çağrı yalnız gerçek
    boş turda).
  - **Ucuz-model yargıcı** (`judgeCoordinatorStalled`): title-model override, yoksa
    koordinatörün modeli (lessons/summary ile aynı politika); son mesaja bakıp
    `{"stalled":true|false}` döner (`parseStallVerdict` fail-safe: bozuk → false).
  - **Katman 1 — tur-sonu** (`guardCoordinatorStall`): pozitif verdikte
    `<coordination-guard>` notu + `slot.pending` ile aynı batch'te bir tur zorlar.
    `CoordinatorStallMaxNudges` (vars. 2) ile sınırlı; gerçek araç çağrısı streak'i
    sıfırlar; yargıç hatasında nudge YOK (sweeper'a bırakılır).
  - **Katman 2 — gecikme tarayıcısı** (`StartCoordinatorStallSweeper`, 60 sn tick):
    canlı `coordSlots`'u gezer; `slotIsStallCandidate` (idle + hadWorkers + 0 worker +
    `lastTurnUnix` `CoordinatorStallSweepMin` (vars. 5 dk) öncesinden eski) olanları
    yargılar, `enqueueCoordinatorTurn` ile uyandırır. Bütçe biterse dırdır yerine
    gözlemlenebilir `DebugError`. Restart/kaçırma horizonunu kapatır.
- **Ayarlar (Ayarlar ▸ Araçlar ▸ "Koordinatör donma koruması"):** `CoordinatorStallGuard`
  (master on/off, vars. açık) · `CoordinatorStallSweepMin` (0=vars.5dk, −1=tarayıcı
  kapalı) · `CoordinatorStallMaxNudges` (0=vars.2). Tam plumbing: settings.go/store.go
  (clamp) → tunables.go → api/server.go applySettings → manager.go boot; frontend
  types/settings.ts + AppToolsPanel + SettingsPanel patch.
- **Kod/Test:** `coordination_stall.go`, `coordination.go` (`coordSlot.lastTurnUnix`,
  guard çağrısı agent taşır), sweeper `manager.go`'da başlatılır. Test:
  `coordination_hallucination_test.go` → `parseStallVerdict` + `slotIsStallCandidate`
  - `turnCalledCoordinationTool` (regex testi kaldırıldı). go build/vet/test yeşil.

## API workspace kapsamı: `?workspace=` sessizce yok sayılıyordu ✅ (2026-08-02)

## API workspace kapsamı: `?workspace=` sessizce yok sayılıyordu ✅ (2026-08-02)

- **Sorun:** `withWorkspace` yalnız `X-Workspace-Id` header'ı ve `?ws=` kabul
  ediyordu. `?workspace=WS17` gibi bir parametre **sessizce yok sayılıp** default
  workspace'e düşüyordu → çağıran, **başka bir workspace'in verisini** kendi
  istediği id'nin cevabı sanıyordu. Canlı örnek: `GET /api/mcp-servers?workspace=WS17`
  7 workspace için de WS1'in sunucularını (`MCP31 browser-mcp`) döndürdü; WS17'nin
  gerçek listesi diskte `MCP1 codebase-memory` + `MCP2 playwright`. Teşhis sırasında
  bu, "WS17'de browser-mcp var" şeklinde **yanlış bir bulguya** yol açtı. Dış-ajan
  otomasyonu için (`_Docs/33`) asıl risk okuma değil **yanlış store'a yazma**.
- **Çözüm** (`internal/api/server.go`):
  - `workspaceQueryKeys` = `ws` · `workspace` · `workspaceId` · `workspace_id`
    (alias'lar bilerek geniş: sessizce yok sayılan bir kapsam parametresi,
    bilinmeyen bir parametreden daha kötüdür).
  - `workspaceIDFromRequest` id'yi **ve açıkça query ile mi geldiğini** döndürür.
    Query, header'ı ezer (per-request kapsam > istemcinin ambient seçimi).
  - **Bilinmeyen id politikası ikiye ayrıldı:** query ile gelen (kasıtlı kapsam) →
    **400 `unknown workspace <id>`**, asla sessiz yönlendirme. Header ile gelen
    (localStorage'da bayat kalmış olabilir) → eskisi gibi default'a düşer ama artık
    **warn loglanır** — silinen workspace sonrası UI brick olmasın.
  - Her yanıta **`X-Workspace-Id` response header'ı**: çağıran, isteğine hangi
    workspace'in cevap verdiğini varsaymak yerine görebilir.
- **Test:** `workspace_scope_test.go` (tüm alias'lar, query>header önceliği,
  header'ın explicit sayılmaması, boşluk-only kapsam sayılmaz).

## Koordinatör spawn halüsinasyon guard'ı ✅ (2026-08-02)

- **Sorun (SES1 donması):** Uzun/ağır-compact bağlamda koordinatör turu prose'da
  "3 worker başlattım, `list_workers` ile doğruladım hepsi `[running]`" **yazıyor
  ama o turda hiç `spawn_worker`/`list_workers` çağırmıyordu** — debug.jsonl'de
  sıfır `tool` olayı, iddia edilen SES95/96/97 diskte yok. Sonuç: hiç worker
  yaratılmadan koordinatör hayalî worker'ları bekleyip donuyor ("worker başladı
  deniyor ama başlamıyor"). Teşhis: 23:38 turu gerçek `spawn_worker`×2 yaptı
  (SES93/94 çalıştı, 23:55-23:57'de raporladı); 23:58 turu aynı biçimi taklit etti
  ama araç çağırmadı (cache okuması 1.34M→55K, arada ağır compaction).
- **Çözüm:** `runCoordinatorTurn` başarılı tur sonrası `guardSpawnHallucination`
  çağırır: turun metni spawn/verify iddia edip (`coordSpawnClaimRe`: `[running]`
  ya da worker↔başlat/launch/spawn yakınlığı, bilingual) ama `steps`'te hiç
  koordinasyon aracı (`spawn_worker`/`list_workers`/`send_to_worker`/`stop_worker`,
  bare + namespaced CallName, subStep'lere iner) çağrılmadıysa → düzeltici
  `<coordination-guard>` notu (origin `coordination-guard`) enjekte eder ve
  `slot.pending=true` ile **aynı batch'te bir tur daha** zorlar; nudge:
  "betimlemek spawn etmek değildir — gerçekten aracı çağır ya da bitir".
- **Sınır:** per-koordinatör `coordSpawnHallucStreak` (`coordSlot`), tavan
  `coordSpawnHallucMaxCorrections=2` → wedged model tüm notify-loop bütçesini
  düzeltmeye harcayamaz; **gerçek bir koordinasyon aracı çağrısı streak'i sıfırlar.**
  Warn log + `debug.jsonl` `error` anomalisi.
- **Kod:** `internal/agent/coordination.go` (`guardSpawnHallucination`,
  `turnCalledCoordinationTool`, `coordSpawnClaimRe`, `coordSlot.spawnHallucStreak`).
  Test: `coordination_hallucination_test.go` (gerçek SES1 mesajları trip eder,
  conclude/status turları etmez; bare + namespaced + nested tool tespiti).
- **Tamamlayıcı:** aynı kök nedenin (koordinatör bağlam şişmesi — 93
  `<task-notification>` ≈ 103k token) görüntü tarafı bugün ayrıca ele alındı
  (bkz. bir alttaki "Bağlam metresi" girdisi). Kalan sertleştirme fikri: worker
  raporlarını `NotifyCoordinator` enjeksiyonundan önce özetlemek.

## Bağlam metresi: "Kullanıcı" kovası origin'e göre bölündü ✅ (2026-08-02)

- **Sorun:** Koordinatör oturumunda bağlam penceresi "Kullanıcı" kovasını devasa
  gösteriyordu. Ölçüm (WS17/SES1): 121 `user` mesajının **yalnız 9'u** insan
  metni (~143 token); **93'ü** `<task-notification>` (~103k token, %98.7), 19'u
  `<coordination-status>`. Yani panel "kullanıcı 400 KB yazdı" diyordu; gerçekte
  kullanıcı birkaç satır yazmış, gerisi makine enjeksiyonuydu.
- **Çözüm:** `buildFillers` artık kovayı `db.Message.Origin` ile ayırıyor —
  ayırt edici alan **zaten veride vardı** (`origin: "worker-note"`), sadece
  panelde kullanılmıyordu. Yeni sentetik filler rolleri: `worker-note`
  ("Worker sonuçları", turuncu) ve `auto-prompt` ("Otomatik dürtme", sarı,
  `wake`/`schedule` kaynaklı). Wire'daki `role` değişmedi — model bunları hâlâ
  user turu olarak replay ediyor; bölünme yalnız **görüntüleme** katmanında.
- **Bilinmeyen origin** düz `user` kovasına düşer (yeni bir Origin değeri
  etiketsiz kova üretmesin diye); test bunu pinliyor.
- **Renk seçimi:** accent bilerek yalnız insanın payında kaldı — turuncu/sarı
  bakışta "bu benim yazdığım değil" diyor.
- **Kod:** `internal/api/session_info.go` (`fillerRoleFor`, `roleLabel`),
  `frontend/src/shared/lib/palette.ts`. Test: `session_fillers_test.go`
  (kova sayıları + `Role` tekilliği — frontend legend'ı `key={f.role}` kullanıyor,
  çakışma iki kovayı tek React key'inde eritirdi).

## Harici araçlar: `winget` bağımlılığı GOOS'a bağlandı (Linux sunucu hatası) ✅ (2026-08-03)

- **Bulgu:** "Bu komutlar Ubuntu sunucuda da çalışır mı?" sorusuyla yapılan denetim
  gerçek bir hata ortaya çıkardı — **aynı gün eklenen `bun` girdisi Linux'ta bozuktu**.
  `oven-sh/bun` release akışı Linux'ta da çalıştığı için statü `outdated` olabiliyor,
  `canOfferUpdate` düğmeyi gösteriyor, düğme ise `winget` çağırıp hata veriyordu.
- **Daha sinsi olan:** `git` ve `ffmpeg`'de akış olmadığı için düğme çıkmıyordu ama
  panel her `command` spec'i için **komut kopyalama çipi** de render eder → Ubuntu
  kullanıcısına otoriter görünen, asla çalışamayacak bir `winget upgrade --id …`
  satırı sunuluyordu. Yanlış talimat, talimatsızlıktan kötüdür.
- **Çözüm:** `wingetSpec(goos, id, note)` + `bunUpdateSpec(goos)`. Windows'ta winget,
  değilse: `git`/`ffmpeg` → `manual` + apt notu, `bun` → **`bun upgrade`** (bun kendi
  güncelleyicisini taşır, paket yöneticisi gerekmez). Karar `runtime.GOOS`'u doğrudan
  okumak yerine **parametreli** verilir → her iki dal da Windows'tan test edilebilir.
- **Kalıcı test:** `platform_test.go` — Linux dalında Kind manual/`bun`, komut winget
  değil ve `UpdateCommandLine()` winget satırı döndürmüyor; ayrıca
  `TestCatalogHasNoWingetOffWindows` canlı katalogu Linux host'ta tarar.
- **Zaten doğru olanlar (denetlendi):** `exec.LookPath` Linux PATH'i, `--version`
  probe'ları, `proc.PythonCandidates()` (Linux'ta `python3` önce), `IsWindowsAppAlias`
  (Linux'ta daima false), `tts`/`stt` çözücüleri (`exeName()` `.exe`'yi düşürür,
  PATH fallback), `gitProjectURL`. `GOOS=linux go build ./...` temiz.
- **Devamı — `node`/`python` notları da bağlandı:** İkisi her platformda `manual`
  KALIR (kurulumun sahibi bilinemez: nvm/dağıtım paketi/pyenv/brew/conda/installer —
  yanlış seçmek gerçek sahiple kavga eder); platforma bağlanan yalnız **not
  metnidir**, çünkü not kullanıcının uygulayacağı talimattır. Windows: nvm/installer/
  winget · Linux: nvm/**NodeSource** (apt'taki node çok eski) ve python için
  **deadsnakes/pyenv + venv** · macOS: `brew upgrade`. Linux python notu ayrıca
  **Debian/Ubuntu'da sistem `python3`'ünü yerinde yükseltmenin apt araçlarını
  bozabileceği** uyarısını taşır. Testler: notlar o platformda **var olmayan** paket
  yöneticisini anamaz (Windows'ta `apt`/`brew`, Linux'ta `winget`/`brew` yasak) ve en
  az bir geçerli yol göstermek zorundadır. `GOOS=windows/linux/darwin` üçü de derlendi.
- **Dokunulan:** `internal/exttools/catalog.go` · `internal/exttools/platform_test.go` (yeni) · `_Docs/54`.

## Harici araçlar: `bun` + "Güncelle" butonu kuralının isimlendirilmesi ✅ (2026-08-03)

- **`bun` eklendi** (kategori `dev`, `Wire: "cli"`): `transform_data`'nın kabul ettiği
  üçüncü çalışma zamanı (python3/node/bun). node/npm'in aksine **kullanılabilir bir
  release akışı var** — `oven-sh/bun` release yayımlıyor ve tag'i `bun-v1.3.14`;
  `semverRe` içinden `1.3.14`'ü okuyor, karşılaştırma gerçek. Ölçüldü: yerel `1.3.1`
  → verdict `outdated` ✓. Güncelleme `command` (winget `Oven-sh.Bun`) — bu makinede
  zaten winget ile kurulu (`WinGet\Links\bun.exe`).
- **`canOfferUpdate` (asıl değişiklik):** "Güncelle" butonunun kuralı JSX içinde
  satır-içi bir koşuldu ve akışsız araçlarda butonun çıkmaması **emergent** bir yan
  etkiydi — kimse bunu kural olarak yazmamıştı. Tek bir isimlendirilmiş yardımcıya
  taşındı: `kurulu ∧ updateKind==='command' ∧ akış "geride" dedi`. Yani `ffmpeg`,
  `npm`, `node`, `python` (ve Windows dışı `git`) için buton **hiç render edilmez**;
  statüleri ancak `unknown` olabilir ve "bilmiyorum" kullanıcının makinesinde paket
  yöneticisi koşturmak için gerekçe değil. Komut kopyalama çipi manuel çıkış kapısı
  olarak kalır. İsimlendirmenin amacı: statü mantığı ileride değişirse gerekçesiz
  güncelleme önerisi sessizce geri gelmesin.
- **Dokunulan:** `internal/exttools/catalog.go` · `ExternalToolsPanel.tsx` · `_Docs/54`.

## Harici araçlar: `python` + yorumlayıcı çözümünün tek kaynağa taşınması ✅ (2026-08-02)

- **Neden:** `python` opsiyonel değil **gerçek bağımlılık** — `run_code` ve
  `transform_data` ona shell ediyor, code-mode binding'leri onda koşuyor. Panelde yoktu.
- **Windows tuzağı (ölçüldü, varsayılmadı):**
  `lookPath("python3")` → `...\AppData\Local\Microsoft\WindowsApps\python3.exe`, yani
  Store **app-execution-alias stub'ı** (0-baytlık reparse point; sadeleştirilmiş env'de
  `Python was not found` yazıp **9009** ile çıkar). Naif tespit "kurulu ✓" der, sürüm
  probe'u patlar → gayet çalışan bir makinede "sürüm okunamadı".
- **Çözüm — kopyalama değil, taşıma:** kural (`aday sırası` + `WindowsApps atla`) zaten
  `tools/builtin_transform_data.go`'da vardı. Kopyalamak yerine `internal/proc/interp.go`'ya
  taşındı (`PythonCandidates` / `LookInterpreter` / `IsWindowsAppAlias`); `tools` ve
  `exttools` **aynı** fonksiyonu çağırıyor → panelin gösterdiği ikili ile `run_code`'un
  çalıştırdığı ikili ayrışamaz. `proc` ikisinin de zaten bağımlı olduğu leaf paket.
- **Doğrulandı:** naif `lookPath(python3)` → stub; `Detect(python)` →
  `<python>/python.exe` → `3.13.7` ✓. `python/cpython` `releases/latest` → **404**
  (tag var, release yok — `git/git` ile aynı) → akış bağlanmadı.
- **Dokunulan:** `internal/proc/interp.go` (yeni) · `internal/tools/builtin_transform_data.go`
  (kopya kalktı) · `internal/exttools/catalog.go` · `_Docs/54`.

## Harici araçlar: `node` + `npm` katalogda (release akışı bilerek yok) ✅ (2026-08-02)

- **Neden:** `mmdc` güncellemesi `npm`'e dayanıyor ama npm'in kendisi listede yoktu;
  npm yoksa o "Güncelle" düğmesi sessizce başarısız oluyordu.
- **İki release akışı da denendi, ikisi de kullanılamaz çıktı:**
  - `nodejs/node` → `v26.5.1 "(Current)"`. Endpoint tarihe göre en yenisini verir =
    **Current** hattı. Yerel `v20.20.2` (LTS) "outdated" gösterilip kullanıcı
    **LTS'ten itilirdi**. Node'un LTS bilgisi `nodejs.org/dist/index.json`'da,
    GitHub release'inde değil.
  - `npm/cli` → `libnpmpack-v10.0.2`, yani npm CLI değil **monorepo alt paketi**.
    `semverRe` içinden `10.0.2` çekip yerel `10.8.2` ile karşılaştırır → sessizce
    "güncel" der. Kendinden emin ve anlamsız.
- **Karar:** GitHub olmayan URL (`nodejs.org` / `npmjs.com`) → `Repo()` boş →
  "release akışı yok". Uydurmaktansa bilmediğini söyle.
- **`Path` alanı burada asıl değer (düzeltme):** İlk yazdığım "bu makinede ikisi de
  nvm dizininden çözülüyor" ifadesi **eksikti** — ölçüm Bash tool'unda yapılmıştı.
  Gerçekte **iki ayrı Node** var ve hangisinin görüneceği sürecin PATH'ine bağlı:
  PowerShell → `C:\Program Files\nodejs` (winget, LTS v24), Git Bash → nvm-sh'ın
  `.bashrc`'den öne aldığı `~\.nvm\versions\node\...`. Yani `/api/external-tools`
  çıktısı **backend'in nasıl başlatıldığına** göre değişir (`dev.ps1` → PowerShell →
  Program Files). `exec.LookPath`'in doğru davranışı, ama panelin `Path` alanını
  vazgeçilmez kılan da tam olarak bu.
- **`node` → `manual`** (kurulumun sahibi nvm mi installer mı bilinemez; winget
  nvm'in üstüne kurarsa çakışır), **`npm` → `command`** (`npm i -g npm@latest`).
- **Windows'ta ölçüldü:** `npm` PATH'te `npm.cmd`'ye çözülüyor ve Go'nun `exec`'i
  batch dosyasını sorunsuz çalıştırıyor → `10.8.2` okundu, `cmd /c` sarmalayıcısı
  gerekmedi.
- **Dokunulan:** `internal/exttools/catalog.go` · `_Docs/54`.

## Harici araçlar: `git` katalogda (GOOS-farkındalı release akışı) ✅ (2026-08-01)

- **Neden:** TionHarness git'e üç yerde dayanır — oturum bağlamına branch enjeksiyonu,
  `scripts\worktree.ps1`, `internal/proc`'un non-interactive git env'i — artı ajanın
  kendi shell komutları. "Kurulu mu / hangi sürüm" sorusu panele ait.
- **Tuzak:** İlk akla gelen `git/git` URL'si **çalışmaz**. O depo GitHub'da salt-okunur
  ayna: tag yayımlar ama **release yayımlamaz** → `releases/latest` **404** → araç
  sonsuza dek "sürüm karşılaştırılamadı" gösterirdi. Denendi, doğrulandı.
- **Çözüm:** `gitProjectURL` çalışma anında seçer — Windows'ta
  `git-for-windows/git` (release yayımlar; tag'i inşa ettiği **upstream** sürümü
  adlandırır: `v2.55.0.windows.3` → `2.55.0`, yani karşılaştırma anlamlı), diğer
  platformlarda `git-scm.com` → GitHub slug'ı yok → akış kapalı. Linux kullanıcısına
  Windows build numarası göstermektense "bilmiyorum" demek doğru.
- **Güncelleme `command`** (winget `Git.Git`) — ffmpeg ile aynı gerekçe: kurulum
  dizinini ve çalışan ikiliyi paket yöneticisi yönetir.
- **Uçtan uca ölçüldü:** `git` → `C:\Program Files\Git\mingw64\bin\git.exe`,
  yerel `2.50.1` ↔ `v2.55.0.windows.3` → `outdated` ✓ ·
  `claude` → yerel `2.1.220` ↔ `v2.1.220` → `up-to-date` ✓
- **Dokunulan:** `internal/exttools/catalog.go` · `_Docs/54`.

## Harici araçlar: `claude` (Claude Code CLI) katalogda + yol geçersiz kılma ✅ (2026-08-01)

- **Sorun:** Ayarlar ▸ Harici Araçlar paneli TionHarness'in yanında kullanılabilecek
  _opsiyonel_ CLI'ları listeliyordu, ama en kritik ikili — anahtarsız `claude-cli`
  sağlayıcısının çalıştırdığı `claude` — listede yoktu. "Hangi sürüm kurulu, güncel
  mi, nerede?" soruları model seçicideki rozete ve Sağlayıcılar ekranına dağılmıştı;
  bir claude-cli ajanı bozulduğunda tam da bu panele bakılıyordu.
- **Çözüm:** `exttools.Catalog`'a `claude` girdisi (`ClaudeToolName` sabiti — ikilinin
  adı, sağlayıcı id'si `claude-cli` **değil**; `Detect`/`LocalVersion` bu adı kullanır).
  Kategori `provider` (yeni grup: "LLM sağlayıcı CLI'ları"), `Wire: "provider"` → panelde
  **Sağlayıcı** rozeti. Sürüm `claude --version` (`2.1.220 (Claude Code)` → `2.1.220`),
  release akışı `anthropics/claude-code` (`releases/latest` → `v2.1.220`, mevcut
  `Tool.Repo()` URL türetmesiyle bedava geldi).
- **Güncelleme `manual`, bilinçli:** Claude Code kendini arka planda zaten günceller;
  ayrıca **süren bir claude-cli turu ikiliyi kilitler** → yarım kalan güncelleme
  workspace'teki tüm claude-cli ajanlarını durdururdu. Note `claude update` (native)
  ve `npm i -g @anthropic-ai/claude-code` (npm) yollarını söyler.
- **`SetPathOverride` (asıl doğruluk düzeltmesi):** `Detect` yalnız PATH'e bakıyordu,
  oysa `Settings.ClaudeCLIPath` sağlayıcının çalıştırdığı ikiliyi değiştirebiliyor →
  panel, ajanların kullandığından **farklı** bir `claude`'un sürümünü gösterebilirdi.
  `applySettings` artık her ayar değişiminde yolu `exttools`'a da iter. Override
  varsa PATH'e **düşülmez**: yol geçersizse dürüst cevap "bulunamadı"dır.
- **Dokunulan:** `internal/exttools/catalog.go` · `internal/api/server.go` (import +
  applySettings) · `ExternalToolsPanel.tsx` (kategori etiketi + `provider` rozeti +
  giriş metni artık "önce override, yoksa PATH" diyor) · `_Docs/54`.

## Oturum Bilgisi ▸ Bağlam penceresi: Skill kataloğu ayrı segment ✅ (2026-08-01)

- **Sorun:** Oturum Bilgisi panelindeki "Bağlam penceresi" kırılımı (`session_info.go`
  → `systemFillers`) sistem promptunu, araç şemalarını ve artifact bloğunu sayıyor
  ama **Available Skills kataloğunu hiç saymıyordu**. `composeTurnRequest` bu bloğu
  statik prefix'e ekliyor, yani her turda bağlamda — ölçer onu eksik raporluyordu.
  ("Bağlam önizle" ekranı, `session_context.go`, Skills'i zaten ayrı segment olarak
  gösteriyordu; iki ekran ayrışmıştı.)
- **Çözüm:** `systemFillers` artık `Runtime.SkillsCatalogBlockForAgent`'ı ayrı bir
  filler olarak ekliyor — `role:"skills"`, etiket **"Skill kataloğu"**, `Count` =
  ilan edilen skill sayısı (`countCatalogSkills`, `- \`slug\``satırlarını sayar).
Frontend'de`ROLE_COLORS.skills`(sky`#0ea5e9`) ile kendi rengi var; çubuk ve
legend başka değişiklik istemedi (ikisi de `info.fillers` üzerinden generic).
- **Neden ayrı segment:** şişmiş bir skill kütüphanesi, sistem promptundan bağımsız
  olarak kullanıcının **küçültebileceği** bir maliyet (ajandan skill kaldır /
  name-only tier). Aynı kovada gizlenince aksiyon alınamıyordu.
- **Kapsam notu:** Skill **gövdesi** (`use_skill` çıktısı) bu ölçerde görünmez ve
  görünmemeli — tool sonuçları tur-içi; kalıcı geçmişe yalnız user/assistant
  mesajları yazılır (`historyToPreviewMessages`), yani bir sonraki turun penceresine
  taşınmaz. Ayrı segment olan tek şey katalogdur.

### Devamı: ölçer tek kaynağa bağlandı (drift kalıcı kapandı)

Yukarıdaki eksik, tek bir skill kataloğundan ibaret değildi: `systemFillers`
statik prefix'i **elle yeniden kuruyordu**, yani `chat_turn.go`'nun gerçek
kompozisyonundan bağımsız bir kopyaydı ve zamanla ondan uzaklaşmıştı.

- **Kök neden:** statik prefix'in üç eli vardı — gerçek yol (`buildStaticPrefix`),
  ajan önizlemesi (`buildAgentStaticPrompt`, "keep in sync" yorumuyla) ve ölçer
  (`systemFillers`). Üçüncüsü en çok sapanıydı: skills kataloğu, lazy-tool
  kataloğu, artifact rehberi ve capability bloğu hiç sayılmıyordu.
- **Çözüm:** `systemFillers` artık prefix'i `s.buildStaticPrefix(...)`'ten alıyor —
  `composeTurnRequest`'in prompt epoch'a dondurduğu **aynı** üretici. Tek elden
  gelen metinden iki blok `stripBlock` ile kendi kovasına ayrılıyor
  (`carve` yardımcısı): **Skill kataloğu** (`role:"skills"`) ve **Araç kataloğu
  (talep üzerine)** (`role:"lazy-tools"`, `#c084fc`). Blok birebir bulunamazsa
  tokenlar sistem kovasında kalır — asla iki kez sayılmaz.
- **`EpochStaticSystem` bilerek KULLANILMADI:** o fonksiyon mutasyon yapar (epoch
  dondurur + diske yazar + debug olayı üretir); burası salt-okunur bir panel GET'i.
  Canlı prefix zaten bir sonraki adopt noktasında gönderilecek olandır.
- **İkinci doğruluk düzeltmesi — "Araçlar" fazla sayıyordu:** kova
  `Runtime.ToolCatalog` (izin verilen **tüm** araçlar) kullanıyordu, oysa tur
  başında yalnız **eager** tier'ın şeması gider. Artık `ShippedToolCatalog`.
  Lazy araçların gerçek maliyeti = katalog bloğu, ki o da artık ayrı kovada.
- **`multiAgent` doğru besleniyor:** statik prefix çok-ajanlı oturumda bir not
  bloğuyla değişiyor → `labelMultiAgentHistory`'nin bayrağı ölçere de veriliyor.
- **Testler** (`session_info_fillers_test.go`): `countCatalogSkills` **gerçek**
  renderer çıktısına karşı doğrulanır (marker değişirse sayım sessizce 0'a düşerdi);
  `stripBlock` iki ardışık blok eklendikten sonra ikisini de birebir bulabiliyor mu
  — carve'ın sessiz no-op'a düşmesini yakalayan asıl guard budur.
- **Kalan (bilinçli, kapsam dışı):** `session_context.go` önizleme GET'i hâlâ
  `composeTurnRequest` üzerinden `EpochStaticSystem`'i çağırıyor → bir önizleme
  epoch'u erken dondurabilir.

### Devamı 2: `buildAgentStaticPrompt` de tek kaynağa indirgendi

Ajan önizlemesinin kendi statik-prefix kopyası (`agent_context.go`) kaldırıldı;
artık tek satır: `s.buildStaticPrefix(ctx, wsp, db.Session{}, agent, false)`.

- **Kopya gerçekten sapmıştı** (varsayım değil, ölçüldü): ajan-adı notu hâlâ eski
  metni taşıyordu — _"In this chat, the user picks which agent should answer by
  starting a message with `@<AgentName>`"_ — oysa `buildStaticPrefix` bu notu çoktan
  yeniden yazmıştı (@name artık **yönlendirme değil, salt isim referansı**; ayrıca
  `spawn_session` sonucunun bu sohbete dönmeyeceği uyarısı eklenmişti). Yani önizleme,
  ajanın **artık almadığı** bir metni gösteriyordu. Kopya ayrıca capability bloğunu
  (`CapabilityContext`) hiç öğrenmemişti → `SystemTokens` eksik raporlanıyordu.
- **Sıfır `db.Session` bir kılıf değil:** `buildStaticPrefix` oturumdan yalnız iki
  şey okur — `IsCoordinator()` (sıfır değerde `false`) ve `WorkingDir` (`""`).
  Yani bu, "cwd'si olmayan, koordinatör olmayan oturum" ile **birebir aynı** prefix'i
  üretir; taze bir oturumun gerçek durumu da budur. `multiAgent=false` çünkü taze
  oturumda başka yazar yok.
- **Neden test yazılmadı:** kopya silindiği için sapma artık bir test konusu değil,
  yapısal olarak imkânsız. Geriye kalan tek statik-prefix üreticisi
  `buildStaticPrefix`'tir.

## dev.ps1: crash kanıtı kalıcılaştırma (stderr + lifecycle logu) ✅ (2026-08-01)

- **Sorun:** 01.08.2026 05:26:55'te backend aniden öldü ve **neden olduğu tespit
  edilemedi**. Elde hiçbir kanıt yoktu: uygulama logunda graceful `shutting down`
  satırı yok (önceki tüm restart'larda var), panic izi yok, Windows WER raporu ve
  crash dump yok, Kernel-Power/uyku/reboot olayı yok. Son kayıt playwright MCP
  child'ının EOF vermesiydi — yani **önce çocuk süreç öldü**, sunucu bunu
  loglayacak kadar yaşadı, sonra kendisi öldü.
- **Kök neden (tanı boşluğu):** `internal/app/app.go:62` slog'u **stdout**'a yazar
  (+ `~/.tionharness/logs` aynası), ama Go runtime fatal'ları (`fatal error: out of
memory`, panic trace) **stderr**'e gider. `dev.ps1` backend'i `-NoNewWindow` ile
  başlatıp stderr'i **hiçbir yere yönlendirmiyordu** → crash metni konsolla
  birlikte yok oldu. Ayrıca script'in temizlik yolu `taskkill /T /F` kullanıyor;
  bu, uygulama logunda **hiç iz bırakmaz** → "biz öldürdük" ile "kendi öldü"
  olayları sonradan ayırt edilemiyordu.
- **Çözüm — iki kayıt (`_devlogs/`, gitignored):**
  - `backend-stderr-<stamp>.log` — `Start-Process -RedirectStandardError` ile
    backend stderr'i diske. **Yalnız stderr** yönlendirilir: stdout konsolda canlı
    kalır, yani dev deneyimi değişmez. Frontend bilerek dışarıda bırakıldı (Vite
    hataları etkileşimli raporlanıyor; yakalamak onları gizlerdi).
  - `lifecycle.log` — koşular arası **append**: launch, kendiliğinden çıkış
    (exit code ile), pre-flight orphan kill ve cleanup kill'leri. Kill satırı
    `taskkill`'den **önce** yazılır. Kural: backend kaybolmuş ve lifecycle.log'da
    ona ait cleanup satırı **yoksa**, ölüm bu script'in dışından gelmiştir.
- **Ayrıntılar:** boş stderr yakalamaları çıkışta silinir → duran bir dosya daima
  "gerçekten bir şey oldu" demektir; çıkışta son 40 satır konsola kırmızı basılır
  (yazılıp okunmayan log işe yaramaz); en yeni 10 yakalama saklanır.
  `Add-Proc` süreç `.Handle`'ına dokunur — bu olmadan `Start-Process -PassThru`
  nesnesi ölümden sonra `ExitCode`'u `$null` döndürüyor; exit code burada gerçek
  sinyal (Go fatal = 2, dışarıdan `taskkill /F` = 1).
- **Doğrulama:** panic atan geçici bir Go programı aynı bayraklarla koşuldu →
  stdout konsolda kaldı, panic + goroutine izi + `exit status 2` dosyaya düştü.
  `Stop-Tree`/`Write-Lifecycle` gerçek fonksiyonlar AST ile çıkarılıp iki senaryoda
  test edildi (canlı süreç öldürülür ve loglanır; ölmüş süreç öldürülmez, `exit=7`
  doğru okunur). Çalışan 8090 sunucusuna dokunulmadı.
- **Not (çözülmedi):** bu değişiklik 05:26 ölümünü _açıklamaz_, **bir dahakini
  açıklanabilir kılar**. O olayın iki adayı hâlâ ayrıştırılamıyor: (a) konsolun
  Ctrl+C/kapatılmasıyla `finally` → `taskkill /T /F` (çocuğun önce ölmesi bununla
  birebir uyuşuyor), (b) Go runtime fatal / OOM (03:20–05:26 arasında 36 opus
  worker spawn'ı + playwright Chromium; 31 GB RAM'de bugün 9 claude süreciyle bile
  yalnız 2.2 GB boş). Bir sonraki olayda `lifecycle.log` bu ikisini kesin ayıracak.

## Model sürümü: takma adın arkasındaki gerçek model (gözlemlenerek) ✅ (2026-08-01)

- **Sorun:** claude-cli model id'leri takma ad (`opus`, `sonnet`, hatta boş = "oturum
  varsayılanı"). Arayüz her yerde "Opus" yazıyordu; **hangi Opus** (Opus 5 mi 4.8 mi)
  hiçbir yerde görünmüyordu. Bilgi aslında elimizdeydi — CLI her turda gerçek id'yi
  bildiriyor (`Response.Model` → `Message.Model`, ör. `claude-opus-5`) — ama takma adla
  ilişkilendirilmediği için seçicilerde kayboluyordu.
- **Çözüm — hardcode değil gözlem:** `<store>/model-resolutions.json` singleton'ı
  `"<provider>|<istenen>" → {resolved, seenAt}` tutar. Statik bir `opus → claude-opus-5`
  tablosu **bilinçli olarak yazılmadı**: Anthropic yeni model çıkardığında çürür — bu tam
  olarak `ContextWindowFor`'un aile-bazlı kalarak kaçındığı hata. Doğru cevabı zaten her
  tur söylüyor, o yüzden öğrenilir.
- **Yazma yolu:** `Runtime.noteResolvedModel` → `DB.NoteModelResolution`, mevcut
  `noteCacheOutcome` dikişlerinin yanında (guardedComplete + recordedComplete ×2 +
  recordedStream). **Yeni bilgi yoksa diske yazmaz** → ısındıktan sonra dosya yalnız
  sağlayıcı gerçekten farklı bir model sunmaya başlayınca değişir. Native sağlayıcılar
  istedikleri id'nin aynısını döndürdüğü için store'da filtrelenir (`resolved == requested`
  → no-op), yani harita yalnız gerçek takma adlarla dolar.
- **Okuma yolu:** `GET /api/catalog` her `ModelInfo`'ya `resolvedModel` ekler
  (`ws.DB.ResolvedModelFor`). Alan providers manifest'ine değil **API katmanına** ait —
  bir takma adın nereye işaret ettiği sürümle değil çalışma-anıyla değişir.
- **Etiketleme:** `formatModelVersion` id'yi **ayrıştırır**, tablo tutmaz —
  `claude-opus-5`→`Opus 5`, `claude-sonnet-4-5-20250929`→`Sonnet 4.5`,
  `claude-3-5-haiku-20241022`→`Haiku 3.5`. Rakamlar aile token'ından ayrı toplandığı için
  token **sırası önemsiz**; aynı kod hem güncel hem eski (sürüm-önce) şemayı okur.
  Bilinmeyen ailede `""` döner → çağıran ham id'ye düşer, **uydurma isim yok**.
- **Nerede görünür:** `resolveModelLabel` artık gözlemlenen sürümü tercih ediyor → ajan
  kimlik satırı (`AGT3 · Opus 5`), sohbet balonu, katılımcılar, pano — hepsi tek
  fonksiyondan. Model seçicide ayrıca `Opus — en güçlü → Opus 5 · 1M` seçenek metni +
  ham id'yi tooltip'te taşıyan rozet.
- **Test edilebilirlik:** saf etiket mantığı `shared/lib/modelLabel.ts`'e ayrıldı
  (`catalog.ts` re-export eder). Sebep: `catalog.ts` api client'ı import ediyor, o da
  import anında `localStorage`'a dokunuyor → repo'nun `node` vitest ortamında suite
  hiç yüklenemiyordu. Testler: `modelLabel.test.ts` (8) + `store_model_resolution_test.go` (3).
- **Somut id'ler için tek giriş: `modelDisplayName`** (aynı dosya). Elde zaten somut bir id
  varken (turun modeli, harcama satırı, debug özeti) katalog sorgusuna gerek yok — yalnız
  biçimlendirme. Kullanıldığı yerler: Bütçe provider▸model tablosu, Oturum Debug "Modele göre
  token", sohbet balonu altındaki tur meta çipi. **Ham id her yerde `title`'da kalır** —
  faturayla/hata raporuyla eşleşmesi gereken değer odur, varsayılan okuma olmaması onu
  erişilemez yapmamalı. Tanınmayan id (MiniMax, OpenRouter rotaları) **aynen** gösterilir:
  harcama satırı ne gizleyebilir ne yaklaşık söyleyebilir.
  İkisi arasındaki sınır: `resolveModelLabel` = ajanın **yapılandırılmış** modeli (takma ad
  olabilir → katalog gerekir), `modelDisplayName` = elde **zaten somut** id.
- **Kapsam (uygulama geneli tarandı):** Bütçe provider▸model tablosu · Oturum Debug "Modele
  göre token" · sohbet balonu tur-meta çipi · mesaj Debug panelindeki "Model" satırı · Market
  paket önizlemesindeki ajan modeli (iki yer) · Sağlayıcı test rozeti (`✓ bağlandı (Opus 5)`).
  `MessageDebugPanel`'in `Row`'una bunun için `title` prop'u eklendi ve model satırından
  `mono` kaldırıldı (artık insan adı, id değil).
- **İki bilinçli istisna — ham kalır:**
  1. Debug kartındaki **raw event listesi** (`eventLabel`, `llm_call`): kullanıcı orada model
     _hakkında_ değil **günlüğün kendisini** okuyor; çevirmek görünümün tek işini bozar.
  2. Market'teki **`ModelList`** (sağlayıcı paketinin model id kataloğu): orada id **içeriğin
     kendisi** — kopyalanıp config'e yazılır ve fiyat tablosu tam id ile anahtarlanır. İki
     farklı id aynı isme düşebileceği için çevirmek eşlemeyi bozar; mono biçim doğru.
- **Sınır:** rozet **son gözlem**tir. Takma ad yeni bir modele taşındıysa, o workspace'te
  bir tur koşana kadar eski değer görünür (tooltip ham id'yi verir).

## claude-cli: modelin yanında Claude Code sürümü + plan rozeti ✅ (2026-08-01)

- **Sorun:** `claude-cli` sağlayıcısının modelleri çıplak takma ad (`sonnet`, `opus`).
  Ekranda yazan "Sonnet" her makinede aynı görünüyordu; onu gerçekte çalıştıran **yerel
  Claude Code sürümü** ve **hangi Max/Pro planıyla** koştuğu hiçbir yerde görünmüyordu.
- **Çözüm:** katalog DTO'suna iki alan — `cliVersion` + `subscription` (yalnız `claude-cli`
  girdisinde dolu, diğerlerinde boş). Model seçicide ve Sağlayıcılar ekranındaki claude-cli
  kartında `Claude Code v2.1.220 · Max` rozeti olarak çıkar.
- **Sürüm nereden:** `exttools.LocalVersion(claude, --version)` — halihazırda Harici Araçlar
  panelinin kullandığı, 3 sn timeout'lu, süreç-ağacı reap eden probe. Katalog her sayfa
  açılışında çekildiği için sonuç **TTL'li memo**: başarıda 10 dk, başarısızlıkta 1 dk
  (bozuk kurulum her istekte 3 sn yakmasın, düzelen kurulum da 10 dk görünmez kalmasın).
- **Plan nereden:** `claudeauth.ReadCredentials(<workspace>/claude-home)` →
  `claudeAiOauth.subscriptionType` (`max`/`pro`). Login **per-workspace** olduğu için rozet
  de per-workspace. Uygulama ayarı `claudeCliAuthKind == "apikey"` ise plan **yazılmaz**
  (API anahtarı token-başı faturalanır, abonelik değil) — home'daki bayat OAuth dosyası
  yanıltıcı bir "Max" göstermesin.
- **Tahmin yok:** sürüm okunamazsa veya `subscriptionType` yoksa ilgili parça çıkmaz;
  rozet kısalır ya da hiç görünmez. Katalog isteği bundan dolayı hata döndürmez.
- **Ajan kimlik satırında DEĞİL** (2026-08-01, aynı gün geri alındı): kısa süre
  `AgentIdentity`'ye de eklendi, sonra kaldırıldı — Claude Code kurulumu **makinenin**
  özelliği, turun değil. O satır cevabı veren modeli adlandırır ve artık gerçek sürümü
  gösteriyor (üstteki bölüm). `compact` seçeneği `resolveRuntimeBadge`'de duruyor,
  ileride dar bir yere gerekirse hazır.
- **Asistan balonu:** `AgentHeader` artık `subtitle="model"` geçiyor — balonda daha önce
  yalnız isim vardı; model satırı composer'daki ajan tetikleyicisiyle hizalandı, böylece
  turu hangi modelin ürettiği cevabın yanında yazıyor.
- **Kod:** `internal/api/catalog.go` + `catalog_claudecli.go`, `internal/claudeauth/read.go`,
  `Registry.ClaudeCLIPath()`, `shared/lib/modelLabel.ts → resolveRuntimeBadge`,
  `ProviderModelSelect.tsx`, `ProvidersPanel.tsx`, `chat/AgentHeader.tsx`.

## Terse (caveman) mod — workspace toggle + registry promptu ✅ (2026-08-01)

- **Sorun:** `tionharness-terse` skill'i katalogda yalnız slug + tek satır özet olarak
  duruyordu; gövdesi `use_skill` çağrılınca yükleniyordu. Yani yanıt stilinin yürürlükte
  olması modelin tetik kelimeyi fark edip aracı çağırmasına kalıyordu — pratikte kullanıcı
  "terse" demedikçe hiç ateşlenmiyordu.
- **Çözüm:** `WSSettings.TerseMode` (bool) + **registry promptu `terse`**. Anahtar açıkken
  prompt her ajanın statik system prefix'ine, workspace instructions'ın ardından eklenir
  (sıra bilinçli — workspace kuralı stili ezebilsin). Kapalıyken tek bayt gitmez.
- **Neden skill değil prompt:** yanıt stili yalnız koşulsuz olduğunda çalışır; skill yolu
  tanım gereği modelin takdirine bağlı. Prompt yolu ayrıca `capabilities.go`'daki
  codebase-memory deseniyle aynı: workspace toggle → cachelenebilir statik blok.
- **Düzenlenebilirlik:** metin `internal/prompts/defaults/terse.md` (gömülü) →
  `<workspace>/config/prompts/terse.md` ile override edilebilir; boş/bozuk override
  varsayılana düşer, kötü bir edit modu sessizce kapatamaz.
- **Tek kaynak:** `Runtime.TerseModeBlock()` — headless (`systemPrompt`) ve api tarafı
  (`chat_turn`, `agent_context`, `session_info`) aynı anahtarı + aynı metni okur. Dört
  yerde ayrı ayrı kurulsa sessizce kayardı.
- **İlk denenen ve geri alınan:** `alwaysSkills []string` — seçilen skill'lerin gövdesini
  prompta basan genel liste. Çalışıyordu ama istenen tek anahtardı; liste UI'ı gereksiz
  genellemeydi ve `skills.Store`'a `exclude` parametresi taşıyordu. Geri alındı.
- **Kod:** `internal/agent/tersemode.go`, `internal/prompts/prompts.go` (+`defaults/terse.md`),
  `internal/workspace/settings.go`, `internal/api/workspace_settings.go`,
  `frontend/.../WorkspacePanel.tsx`. Test: `internal/agent/tersemode_test.go`.
- Detay: `_Docs\06-WORKSPACES.md` ▸ "Terse (caveman) mod", registry `_Docs\61`.

## Sınırsız derinlikte koordinatör ağacı ✅ (2026-08-01)

- **Sorun:** Koordinatör/worker tek seviyeydi. `withCoordination`
  `Role=="coordinator"` bakıyordu; worker `Role=="worker"` olduğu için koordinasyon
  araçlarını asla göremiyordu (bilinçli recursion guard'dı). Bir agent, işini
  kendi içinde bölecek bir agent'ı görevlendiremiyordu.
- **Taşıyıcı karar — rol ≠ ebeveynlik:** `Session.Role` artık yalnız **soyağacı**
  (`"worker"` / `""`), koordinatörlük ayrı bir **yetenek** alanı
  (`CoordinatorMode`). Ara düğüm ikisine birden sahip. Yanına
  `RootCoordinatorSessionID` + `CoordinatorDepth` (aynen `FlowRun.RootRunID`
  deseni). Tüm çağrı yerleri `Session.IsCoordinator()`/`IsWorker()`/
  `RootCoordinator()`'a çevrildi; legacy `Role=="coordinator"` okumada kabul edilir
  (migrasyon yok), legacy worker'da kök = ebeveyn (eski ağaçlar zaten tek
  seviyeydi → paylaşılan scratchpad yolu kaymaz).
- **En kritik semantik — ara düğüm ne zaman "bitti" der:** turunun bitmesi işinin
  bittiği anlamına gelmez (tur, işi worker'larına dağıttığı anda biter). Üç parça:
  `deferWorkerReport` (dal canlıyken `<task-notification>` yerine bir kerelik
  `<task-progress status="delegating">`), `report_to_coordinator` aracı (düğüm
  görevini kendi kapatır; kendi worker'ları çalışırken `completed` reddedilir) ve
  `settleReportBackstop` (dal sustuğu hâlde 30 sn rapor gelmezse otomatik rapor —
  **daima `incomplete`**, runtime işin bittiğini bilemez). `WorkerInfo.Delegating`
  ile roster ve canlı worker-state bloğu "dağıtıyor" der.
- **Guard'lar:** `CoordinatorMaxDepth` (5, `-1`=sınırsız) + **`CoordinatorMaxSubtreeSessions`**
  (64). İkincisi kritik: düğüm-başına worker limiti düğüm bazında olduğu için
  derinlikle **çarpılır** (8 × derinlik 4 ≈ 4096 oturum); üstel dallanmayı durduran
  tek sınır bu. Limite takılan `spawn_worker(coordinator:true)` **hata verir**,
  sessizce düz worker'a düşmez — delegasyon yaptığını sanan koordinatör gelmeyecek
  bir raporu sonsuza kadar beklerdi.
- **Mod seçimi:** `spawn_worker(coordinator, workflow)` (reçete miras alınmaz —
  özyinelemeli reçete ağaç boyunca kendini tekrarlardı) + `set_coordinator_mode`
  aracı ve `PUT /sessions/{id}/role` aynı runtime yolunu kullanır. İki kural:
  çalışan worker varken **kapatma reddedilir**, ve toggle **prompt epoch'unu
  tazeler** — bu olmadan araç şemaları donuk kaldığı için (`_Docs/57`) agent "mod
  açıldı" yanıtını alır ama `spawn_worker`'ı hiç göremezdi.
- **Dayanıklılık:** `stopSubtree` cascade (durdurulan alt-koordinatörün torunları
  bütçe yakıp zombi tur uyandırmasın; teardown + flow düğümü de kullanır),
  `RecoverOrphanedTurns` **BFS** (sığdan derine; bu taramada kurtarılan bir düğüme
  notify atılmaz) ve `waitCoordinatorIdle` artık `activeSubtreeWorkers==0` da ister
  (slot sayacı yalnız doğrudan çocukları izliyordu, ara düğüm kendi turu bitince
  sayacı düşürüyordu → flow düğümü yarım sonuç alabilirdi).
- **Uçlar/UI:** `GET /sessions/{id}/coordinator-tree` (ağacın herhangi bir üyesiyle
  çağrılıp köke normalize edilir) + `/coordinator-ancestors`;
  `list_workers(scope:"subtree")`; `CoordinatorSection` worker'da artık erken
  dönmüyor (üst-zincir breadcrumb'ı + altında koordinatör kontrolleri); sidebar
  Workers filtresi `isWorkerSession()` (aksi hâlde ara düğümler ve tüm dalları
  sekmeden düşerdi). Araç kaydı fonksiyon-varlığına göre; CLI köprüsü aynı alt
  kümeyi ilan eder → native/CLI araç seti ayrışamaz.
- **Takip (aynı gün):**
  - **Ağaç görünümü** `CoordinatorTreeView` — seviyeye göre girintili tüm ağaç,
    düğüm başına + toplam maliyet, katlanır ve kapalıyken fetch edilmez. Düz roster
    yalnız doğrudan çocukları gösteriyordu; bir alt-koordinatörün dalı görünmüyordu.
  - **Maliyet rollup'ı** — `coordinator-tree` per-model istatistikleri birleştirip
    tek seferde fiyatlıyor (`mergeModelStats` + `modelRowsFor`) → Bütçe ekranıyla
    birebir aynı aritmetik. Per-session dolar toplamak yuvarlama kaydırırdı ve
    prompt-cache tasarrufu token kırılımı olmadan hesaplanamaz.
  - **Derinlik-farkında spawn slotu** (`acquireSpawnSlotAtDepth`) — `depth>=2`
    spawn'ları global havuzun dörtte biri boş kalmak şartıyla slot alır. Deadlock
    yoktu (otomatik tur slot almıyor) ama meşgul bir derin dal kardeşlerini ve
    alakasız chat/schedule spawn'larını aç bırakabiliyordu.
  - **Rapor borcu kalıcı** (`Session.CoordinatorReportPending`) — bellekteki bayrak
    tam da kapatması gereken boşluğu açık bırakıyordu: dalını bekleyen ara düğüm
    diskte sağlıklı görünür (son mesajı kendi yanıtı), yetim-tur kurtarması ona
    dokunmaz, bayrak restart'ta kaybolur ve koordinatörü sonsuza kadar beklerdi.
    Boot'ta `RecoverPendingReports` (orphan taramasından **sonra**) backstop'u
    yeniden kurar → kurtarılan worker'lardan tur alan düğüm kendisi rapor verir.
  - **Backstop grace ayarlanabilir** (`CoordinatorSettleGraceSec`, vars. 30 sn,
    5–1800 clamp) — doğru değer modele bağlı: kısa olursa yavaş sentez turu yarışı
    kaybedip gereksiz `incomplete` gönderir, uzun olursa takılmış dal bekletir.
  - **Ağaçta hatalı dal vurgusu** — düğümün `health` alanı (`stuck` > `error` > ``)
    mevcut oto-etiketlerden türer (yeni "bozuk" kavramı yok → oturum listesi ve
    onarım otomasyonlarıyla aynı sinyal). Kırmızı isim/ikon/zemin + **katlanmış
    başlıkta sayaç**, çünkü derindeki hata kimsenin açıp bakmayacağı şeydir;
    `reportPending` ayrı kum saati rozeti.
  - **Canlı duman testi:** izole store'da backend açılıp `coordinator-tree`
    (rollup alanları dahil), `coordinator-ancestors`, rol toggle'ı, bilinmeyen
    oturumda 404 ve derinlik/alt-ağaç ayarlarının round-trip'i (`-1` = sınırsız
    korunuyor) doğrulandı.
- **Canlı LLM denemesi ✅ (claude-cli/sonnet, 3 seviye):** ertelenmiş rapor sahada
  çalıştı (alt-koordinatörün ilk turu `completed` gitmedi, `<task-progress
delegating>` gitti; kök "bu bir sonuç değil" dedi), alt-koordinatör
  `report_to_coordinator`'ı kendisi çağırdı, kök `ALPHA+BETA` sentezini aldı.
  Ayrı koşuda backstop **kasıtlı tetiklendi** ("asla rapor etme" talimatıyla) →
  otomatik `status=incomplete` + "doğrulanmış sonuç değildir" uyarısı gitti,
  `completed` iddia edilmedi.
- **Denemede çıkan auth bug'ı + düzeltmesi:** koşu ortasında turlar
  `authentication_failed` verip workspace credential'ı sıfırlandı. Zincir:
  `~/.tionharness/claude-home` (self-heal'in 1. tercihi) **19 gün önce ölmüş** bir
  credential tutuyordu, `credentialUsable` yalnız "token boş mu" baktığı için
  geçerli saydı → CLI ölü refresh token ile `invalid_grant` aldı → credential'ı
  temizledi → self-heal yalnız workspace açılışında koştuğu için o workspace
  restart'a kadar öldü. Eşzamanlılık büyüttü (refresh token'ları tek kullanımlık,
  ağaç tek claude-home'a 4+ süreç koşuyor). Üç parça: **aday sıralaması**
  (`credentialRank`, birincil ölçüt _canlı access token_ — diskte "refresh token
  harcanmış mı" bilgisi yok; ilk denemem sadece expiry damgalarına bakıp tuzağa
  düşmüştü, bayat home'un `refreshTokenExpiresAt`'i 4 gün ileridedir), **her tur
  self-heal** (`toolloop.go` — kayıp tüm oturumu değil tek turu götürür) ve
  **refresh serileştirme** (`claudeauth/refreshgate.go` — yalnız yenileme gerekli
  pencerede, sağlıklı token'da hiç kilit yok). Doğrulama: aynı koşu tekrarlandı →
  **auth hatası 0**, credential bozulmadı.
- **İstisna/yarış denetimi (aynı gün):** ağaç eşzamanlılığı tasarım gereği ürettiği
  için "check-then-act" desenleri tarandı; **üç gerçek açık** bulunup kapatıldı —
  (a) rapor gönderimi atomik değildi (iki backstop veya backstop⇄açık rapor aynı
  görevi çelişkili statülerle iki kez raporlayabilirdi) → `ClaimCoordinatorReport`
  CAS; (b) alt-ağaç bütçesi say-sonra-yarat idi, eşzamanlı fan-out limiti aşabilirdi
  → kök başına spawn kilidi; (c) `copyFile` truncate-sonra-stream yapıyordu ve
  credential heal'i her tura taşımak eşzamanlı CLI'ye **yarım dosya** gösterebilirdi
  → tmp+rename + per-home kilit + kopyalamadan önce dst'nin yeniden sıralanması
  (taze login eski kaynakla ezilmesin). Kapsam dışı olduğu **açıkça yazılanlar**:
  `SerializeRefresh` süreç-içidir (iki TionHarness süreci aynı claude-home'a koşarsa
  korumaz) ve `-race` bu makinede koşturulamadı (cgo/gcc yok). Detay `_Docs/47` §14.9.
- Detay: **`_Docs/47` §14**. Test: `coordination_tree_test.go`, `coordination_race_test.go`,
  `coordination_test.go`, `claudehome_credential_test.go`,
  `claudeauth/refreshgate_test.go`.

## Toplu dosya-farkı görüntüleyici (chat) ✅ (2026-08-01)

- **Sorun:** Bir turun dosya değişiklikleri yalnız araç izine dağılmış tekil
  `DiffCard`'lardan izlenebiliyordu. "Bu ajan neye dokundu?" sorusunun tek bir
  cevabı yoktu; oturum geneli için hiç yoktu.
- **UI:** ajan balonunun altındaki aksiyon satırına `⧉ N dosya +A −R` çipi
  (`ChangesButton.tsx`); açtığı popup (`ChangesModal.tsx`) **master-detail** —
  solda dosya listesi, sağda yalnız seçili dosyanın yaması. Sekmeler: **Bu tur**
  ve **Tüm oturum**. Popup yalnız açıkken mount edilir; canlı (streaming) balonda
  çip gösterilmez — iz her delta'da büyürken çıkarım yapmanın anlamı yok.
- **Yeni endpoint `GET /api/sessions/{id}/changes`** (`internal/api/session_changes.go`):
  transkripti bir kez gezip **yalnız dosya değiştiren adımları** kırpılmamış
  döndürür. Alternatif — her turun tam izini tek tek çekmek — bir avuç edit
  bulmak için tüm Read/Grep/Bash çıktısını da taşırdı. Adımlar **ham TurnStep
  JSON'u** olarak döner: claude-cli yolunda yama araç girdisinden sentezlenir ve
  o sentez zaten frontend'de (`shared/lib/diff.ts`) yaşıyor; Go'da kopyalamak
  popup ile sohbet kartına iki ayrı doğruluk kaynağı verirdi.
- **Çıkarım** `shared/lib/fileChanges.ts`'te toplandı ve `TurnSteps`'in `DiffCard`
  seçimiyle **birebir** aynı kuralı uygular (yoksa popup'ın sayısı görünen
  kartlarla çelişir): `kind:diff` ya da başarılı `tool`+edit aracı; hatalı adım
  atlanır; `subSteps` içine inilir (alt-ajan edit'i de gerçek, `nested` rozetiyle).
- **Kırpma dürüstlüğü:** okuma yolu yamaları `stepFieldCap`'e kırpıyor. Çipte `≥`
  işareti + popup açılışta tam izi çeker. Sentezlenen yamada kırpılan şey
  **girdi** olduğundan satır sayıları da eksik kalır → `inputTruncated` de
  "kırpılmış" sayılır.
- **Büyük dosya stratejisi** (`DiffView` yeni `variant="panel"`): bağlam katlama
  (`foldableRanges`/`diffRows`) → 400+ satırda sanallaştırma
  (`useVirtualRows.ts`) → 20k satırda sert tavan (ilk 2k + gerçek sayıyla açık
  buton, **sessiz kırpma yok**) → yeni dosya varsayılan kapalı (`Write`'ın
  "farkı" dosyanın tamamıdır). Panelde satırlar bilerek **sarmaz**: sarma satır
  yüksekliğini değiştirir, sabit-yükseklik varsayımı bozulunca sanal spacer'lar
  içerikten kayar.
- **Bilinçli sınır:** aynı dosyanın çoklu edit'i sıralı listelenir, birleştirilmiş
  tek yama üretilmez — dosyanın öncesi/sonrası içeriği saklanmıyor, uydurulmuş
  bir birleşim yanlış olurdu. Popup altında sabit not: gösterilen fark
  değişiklik anındaki halidir, dosyanın şu anki içeriği değil.
- Testler: `internal/api/session_changes_test.go` (filtre/rekürsiyon/ham geçiş),
  `frontend/src/shared/lib/fileChanges.test.ts` (14 vaka: sentez, hata atlama,
  gruplama, katlama). Detay `_Docs/07-CHAT-UX.md`.

## Harici araçlar: sürüm + güncelleme kontrolü ✅ (2026-08-01)

- **Sorun:** `/api/external-tools` yalnız "kurulu mu" diyordu. Kullanıcı hangi
  sürümün kurulu olduğunu ve yenisinin çıkıp çıkmadığını uygulamadan göremiyordu.
- **Yeni paket `internal/exttools`** — katalog `internal/api`'den buraya taşındı
  (api yalnız HTTP kaldı). Her girdi artık `VersionArgs` + `UpdateSpec` taşır;
  GitHub repo'su URL'den **türetilir** (`Tool.Repo()`) → ikinci alanla sapamaz.
- **Sürüm okuma** (`version.go`): aracın kendi `--version` çıktısı, 3 sn timeout +
  `proc.TreeKill`. Bu, katmanın "hiçbir şeyi çalıştırma" kuralından **bilinçli**
  sapmasıdır: sürüm bayrağı yan etkisizdir ve timeout, bayrağı tanımayıp stdio
  sunucusu olarak beklemeye geçen `codebase-memory-mcp` gibi bir aracı da keser.
  Çıktı stdout+stderr birleşik okunur (ffmpeg banner'ı stderr'e yazar).
- **Release kontrolü** (`release.go`): `releases/latest`, **6 saat disk cache**
  (`<dataDir>/cache/exttools-releases.json`, atomik tmp→rename). Kimliksiz GitHub
  limiti saatte 60 istek; 7 araç × birkaç tık limiti dakikalar içinde bitirirdi.
  Ağ/limit hatasında **fail-open**: bayat cache `stale=true` ile döner, cache hiç
  yoksa hata. `Compare` ayrıştıramadığı sürümde **`unknown`** der — asla tahmin
  etmez; yerel sürüm ileri ise `up-to-date` (nightly build'i "güncelleme var" diye
  göstermez).
- **Güncelleme uygulama** (`update.go`) yalnız **paket-yöneticisi destekli**
  araçlarda: `mmdc` → `npm install -g @mermaid-js/mermaid-cli`, `ffmpeg` →
  `winget upgrade --id Gyan.FFmpeg`. Diğer hepsi `UpdateManual` — Windows'ta
  çalışan bir alt-süreç (MCP stdio sunucusu kendi .exe'sini, piper sentezi kendi
  dll'ini) dosyayı kilitler ve yarım kalan kopyalama aracı geri dönüşsüz bozar.
  Komut istekten değil **katalogdan** gelir → enjeksiyon yolu yok.
- **API:** `GET /api/external-tools` (+`version`/`versionError`/`updateKind`/
  `updateCommand`/`updateNote`, probe'lar paralel) · `POST .../check-updates`
  (`?refresh=1` cache atlar) · `POST .../{name}/update` (manual araçta **409** +
  talimat). Güncelleme **ajan aracı olarak açılmadı** — ajan koştuğu makineyi
  sessizce değiştirmesin.
- **UI:** `ExternalToolsPanel` satırında `v0.8.1` çipi, güncelleme varsa release'e
  giden `↑ <tag>` rozeti, `command` araçlarda **Güncelle** butonu + canlı çıktı
  kutusu, `manual` araçlarda talimat callout'u; her `command` aracın altında
  komutu panoya kopyalama. Kontrol butonu **açılışta otomatik çalışmaz** (ağa
  çıkar) — tespit anında, release kontrolü istek üzerine.
- **Öneri kuralı `tool-update`** (`recommendations.ts`): güncelleme varsa uyarı
  kartı + kaç tanesinin tek tıkla güncellenebildiği. Yalnız `outdated` sayılır
  (`unknown` kart çıkarmaz — manual araçta yanlış tahmin riskli ikili değişimine
  iter). Probe `fetchRecommendationData`'nın **tek ağ çağrısı** olduğu için
  `.catch(() => [])` ile sarıldı: çevrimdışı makine `Promise.all`'u reddedip
  **tüm** önerileri düşürmesin; boş dizi "fikrim yok" demek, "hepsi güncel" değil.
- Test: `exttools/{version,compare,release}_test.go` (banner ayrıştırma,
  karşılaştırma tablosu, cache TTL + fail-open + soğuk-cache hatası, katalog
  bütünlüğü) + `recommendations.test.ts` (30 test, yeni kural dahil).
  Canlı doğrulama: 7/7 araç tespit + sürüm; codebase-memory-mcp 0.8.1→v0.9.0 ve
  piper 1.2.0→v1.6.0 gerçekten "eski" çıktı. Detay `54-CAPABILITY-PROBE.md`.

## Ağ ekranı: ajanlar artık çalışan **örnekler** ✅ (2026-08-01)

- **Sorun:** Ağ ekranı ajan **tanımlarını** listeliyordu — workspace'te hiçbir şey
  çalışmasa bile tüm ajanlar tuvalde duruyordu, aynı ajan aynı anda üç oturum
  sürerken tek düğüm olarak görünüyordu. Ekran "kim ne yapıyor" değil "kimler var"
  sorusunu cevaplıyordu.
- **Çözüm:** `/api/graph` ajan düğümlerini **çalışan oturum başına** üretiyor
  (`buildAgentInstances`): id `agent:<agentID>#<sessionID>`, düğümde
  `sessionId`/`agentId`/`runKind`/`runTarget` + örneği ayırt eden alt-başlık
  ("Görev · <oturum başlığı>"). Kapsanan kindler: chat / task / flow /
  flow-coordinator / schedule / **spawned (otomasyon + spawn)** / worker / inbox.
  Hiç çalışan yoksa hiç ajan düğümü yok.
- Ajana bağlı kenarlar (`owns`/`created`/`uses`/`skill`/`mcp`) `addAgentEdges` ile
  **her canlı kopyaya** çoğaltılır; canlı kopyası olmayan ajanın kenarı çizilmez.
  Skill/MCP düğümleri de yalnız çalışan bir tüketicisi varsa eklenir (yoksa tuvalde
  bağsız düğüm olarak asılı kalıyorlardı).
- Frontend: ajan etiketi iki satır (ad + çalıştırma türü) → aynı ajanın kopyaları
  ayırt edilir; aktif bağ haritası **örnek** id'siyle anahtarlanır (bir ajanın iki
  kopyası iki farklı göreve bağlanabilir); "Boşta" lobisi **"Çalışıyor"** çekirdeğine
  dönüştü (task/flow hedefi olmayan chat/schedule/spawn/worker örnekleri buraya
  yaylanır) ve hedefsiz örnek yoksa hiç çizilmez. Başlıkta "N aktif ajan / M".
- `eventToRefreshSignals.ts`: `task` olayı da artık `SIGNAL_NETWORK` bumpluyor —
  çalıştırma başlayınca/bitince düğüm sadece renk değiştirmiyor, **eklenip siliniyor**.
- Test: `internal/api/graph_instances_test.go` (boşta → 0 düğüm, oturum başına bir
  kopya, silinmiş ajanın oturumu düşer, alt-başlık üretimi). Detay `_Docs\23`.

## Glob `**/` sınır hatası + inline eşiği 32 KB + composer'da dizin/branch alt alta ✅ (2026-07-29)

- **`globToRegexp` `**/` dizin-sınırını kaybediyordu (gerçek hata).** `**/` şu
  şekilde çevriliyordu: `.*(?:/)?`. `.*` açgözlü ve `/` opsiyonel olduğu için
  desen **segment ortasında** da tutuyordu → `**/x.go` deseni `barx.go`'yu
  eşleştiriyordu. Somut etki: bir ajan `**/log_2026*` ile dosya ararken
  `d81c8e90-log_20260729.txt`'i **yanlışlıkla** bulur (ya da tersi senaryolarda
  alakasız dosyalar sonuç listesini kirletir). Doğru çeviri `(?:.*/)?` — grubun
  `/` ile bitmek zorunda olması sınırı korur; boş geçebildiği için `**/x` yine
  kökteki `x`'i de tutar. Çıplak `**` (sonda `/` yok) eskisi gibi `.*`.
  Etki alanı `Glob` + `Grep`'in `glob` filtresi + `.gitignore` kuralları
  (`ignore.go` aynı fonksiyonu kullanıyor — orada da doğru gitignore semantiğine
  yaklaştı). Test: `TestGlobToRegexp`'e 10 vaka eklendi.
  > Not: SES59'daki başarısız aramalar bu hatadan **değildi** — onlar claude-cli'ın
  > native (ripgrep tabanlı) Glob'uydu ve sonuçları doğruydu; dosyanın diskteki
  > adı `<id>-<isim>` olduğu için kullanıcının gördüğü isimle prefix araması
  > tutmuyordu. Bu hata TionHarness'in kendi Glob'unda ayrıca duruyordu.
- **`maxInlineTextBytes` 16 KB → 32 KB** (`api/uploads.go`). Eşiğin altındaki
  text/code ekleri prompt'a gömülür (tek turda okunur), üstündekiler yola göre
  `Read` edilir. Takas bilinçli: gömülü ek oturumun **her turunda** yeniden
  gönderilir, listelenen ek yalnız gerektiğinde bir `Read` maliyeti çıkarır.
  32 KB (~8K token) tipik "config/stack-trace yapıştır" vakasını kapsar.
- **Composer'da dizin ve branch alt alta** (`WorkDirBadge`): 1. satır klasör adı, 2. satır `⎇ branch`. Yan yanayken ikisi aynı genişlik için yarışıyordu; artık
  yanındaki ajan seçicisiyle aynı iki-satırlı yapıda, composer satırı hizalı.
  Telefonda yine yalnız ikon.

## Attachment'lar artık MUTLAK yolla veriliyor (ajan dosyasını aramıyor) ✅ (2026-07-29)

**Bulgu (WS10/SES59):** kullanıcı 4.9 MB'lık bir Unity log'u ekledi; ajan dosyayı
**okumaya başlayana kadar 15 araç çağrısı** harcadı — ikisi 20 sn'lik ripgrep
timeout'u. Kök neden tek satırdı (`conversation/manager.go`):

```
- log.txt (text, 4942432 bytes) — read_file path: artifacts/SES59/d81c8e90-log.txt
```

Üç ayrı kusur:

1. **Yol göreliydi ve neye göreli olduğu söylenmiyordu.** `Attachment.RelPath`
   workspace sandbox kökü (`<DataDir>/workspace`) altındadır; ama oturumun cwd'si
   `Session.WorkingDir` (rastgele bir proje klasörü, burada `Desktop/city-cleaner`).
   Ajan yolu cwd'ye göre çözünce dosya yok; sonra `Glob`/`Grep` ile dosyayı kendi
   ekinde aramaya başladı.
2. **`read_file` diye bir araç yok** — fs araçları claude-cli ile aynı isimde
   (`Read`/`Grep`). Prompt olmayan bir aracı işaret ediyordu.
3. **Boyut ham byte'tı ve strateji önerisi yoktu.** 4.9 MB'ı `Read` ile baştan
   okumak anlamsız; ajan bunu ancak deneyip görüyordu.

**Düzeltme** — attachment mantığı `conversation/attachments.go`'ya ayrıldı:

- **Yol mutlaklaştı.** Yeni `WithAttachmentRoot(ctx, root)` (ctx-taşımalı, çünkü
  `Manager` workspace'ler arasında paylaşımlıdır — `WithCompactPrompt` ile aynı
  gerekçe) sandbox kökünü turun bağlamına koyar; satır artık host'un native yol
  stilinde tam yol basar. Kök `workspace.Workspace.SandboxRoot()`'tan gelir (5 yerde
  elle yazılan `filepath.Join(DataDir, "workspace")` da bu helper'a çevrildi).
- **Gerçek araç adı:** `— Read/Grep this exact path: C:\…\artifacts\SES59\…`.
- **Okunabilir boyut + strateji ipucu:** `4.7 MB`; 256 KB üstü ek varsa bloğun
  sonuna "önce Grep'le, sonra offset/limit ile çevresini Read'le" notu eklenir.
- **Kök yoksa sessizce yanıltmıyor:** ctx kök taşımıyorsa yol "workspace sandbox
  kökine göreli" diye **etiketlenir** — açılabilir gibi sunulmaz.

Enjeksiyon noktaları: `chat_stream` · `chat_btw` · `summary` · `wake_turn` ·
`chat_resume` (claude-cli delta) · `flows` ×2. `ToProviderMessages`/
`InlineAttachments` artık `ctx` alır. Test: `attachments_test.go` (5 test —
mutlak yol, köksüz etiket, büyük-dosya ipucu, boyut birimleri).

## Ajan kimliği: ID ikinci satıra + sohbet listesi sekmeleri URL'de ✅ (2026-07-29)

İki küçük ama uygulama-geneli UX düzeltmesi.

- **Ajan ID'si ikinci satırda, modelin solunda.** `AgentIdentity` (`shared/components/
agents/`) `showId` ile ID'yi ismin **yanına** basıyordu; uzun isimlerde ikisi aynı
  satırda yarışıyordu. Artık ikinci satır `ID · model` düzeninde: ID sola sabitlenmiş
  ve asla kırpılmıyor (`shrink-0`), model kalan genişliği alıyor (`truncate`). Tek
  bileşen olduğu için composer ajan seçici, sohbet balonu başlığı, oturum katılımcıları,
  pano kartları ve ajan listeleri **hepsi birden** aynı yerleşimi aldı. `showId` var
  ama `subtitle="none"` olduğunda ikinci satır yalnız ID'den oluşur.
- **Sohbet listesi sekmeleri artık URL'de.** `SessionsSidebar`'ın Aktif/Arşiv/Workers
  görünümü ve tür filtresi bileşen-içi `useState`'ti → paylaşılamıyor, geri/ileri
  tuşunu tanımıyordu. Hash routing'e **query desteği** eklendi
  (`#/w/{ws}/{view}[/{id}][?k=v]`): `parseRoute` '?'ten sonrasını `Route.query`'ye
  ayrıştırır, `buildRoute` anahtarları sıralı + boşları atarak geri yazar (aynı state
  → byte-aynı string, `useUrlSync` karşılaştırması bozulmasın diye). Sohbetin tek
  entity yuvası `sessionId`'ye ait olduğundan sekmeler query'de:
  `?list=archived&kind=task`. Varsayılanlar (`list=active`, `kind=''`) yazılmaz →
  gündelik URL değişmedi. State `useDeepLinks`'e taşındı (diğer sekmelerle aynı yer),
  `SessionsSidebar` kontrollü bileşen oldu. Tür filtresinin localStorage kalıcılığı
  korundu; URL'de açık `?kind=` varsa storage'ı ezer. URL otoritedir: query taşımayan
  bir link sekmeleri varsayılana döndürür. Detay `33-DIS-AJAN-OTOMASYONU.md` §B.1.

## Sohbet süre/zaman bilgileri artık sunucu-otoriter ✅ (2026-07-28)

Balon altbilgisindeki "⏱ süre" ve canlı sayaç frontend'de türetiliyordu; ikisi de
sunucuya taşındı.

- **Tamamlanmış tur süresi:** `MessageList` `asistan.createdAt − önceki kullanıcı
mesajı.createdAt` hesaplıyordu. Oysa backend turu zaten ölçüp `Message.DurationMs`
  olarak kaydediyordu (`chat_stream.go` `agentStart`; otonom yollarda `turnmeta.apply`)
  ve frontend bu alanı **hiç kullanmıyordu**. Artık `durationMs` gösteriliyor; saniye
  yuvarlaması yerine ms hassasiyeti (`formatDurationMs`, <10 sn'de "3.4 sn"). Eski
  türetim yalnız bu alandan önce yazılmış mesajlar için fallback ve "~" ile
  **yaklaşık** işaretleniyor (sessizce doğruymuş gibi gösterilmiyor).
- **Canlı sayaç + tur başlangıcı:** `LiveTimer` istemci `Date.now()` kullanıyor,
  tur başlangıcı da istemcide damgalanıyordu (`chatStreamHub` `AgentStart`). Sonuç:
  (a) saati kaymış istemcide süre yanlış, (b) tur ortasında açılan/yenilenen pencere
  turu **gördüğü ilk adımdan** saymaya başlıyordu. Artık başlangıç `agent_start` hub
  olayının sunucu `time` damgası (durable/ringed → replay'de gerçek başlangıç gelir).
- **Yeni `shared/lib/serverClock.ts`:** hub frame'lerinin `time` alanı + `hello`
  frame'ine eklenen `now` alanından skew tahmini (≥2 sn farkta adopte → ağ jitter'ı
  sayacı zıplatmaz); `serverNow()` tek "şimdi" kaynağı. Müşteriler: `LiveTimer`,
  `WorkerWaitBanner`, `WakeWaitBanner`, prompt-cache sıcaklık geri sayımları
  (`SessionDetailPanel`/`SessionContextModal`). Mutlak saat etiketleri (`MessageTime`)
  bilerek yerel saat diliminde kalır. Test: `serverClock.test.ts` (6/6).

Detay `07-CHAT-UX.md` + `58-QUEUE-SENKRON.md`.

## Spawn/zamanlama süre ayarları gerçekten kaydediliyor + tavan 2 saat ✅ (2026-07-28)

**Bulgu (SES17):** bir worker turu tam `1.199.905 ms` = 20 dk'da kesildi. Sebep
`SpawnTimeout` hard-cap'i; idle watchdog (5 dk) tetiklenmemişti çünkü worker sürekli
araç çağırıyordu (68 tool call). Kök neden ayarın **hiç uygulanamaması**:

- **Plumbing açığı:** `spawnTimeoutMin` / `spawnIdleTimeoutMin` / `scheduleTimeoutMin`
  `Settings` ve `DTO`'da vardı, Ayarlar UI'ında alanları da vardı ve PUT gövdesinde
  gönderiliyordu — ama `settings.Patch` struct'ında **yoktu** → sunucu değeri sessizce
  yutuyordu. UI alanı boot'tan beri no-op'tu. Düzeltildi: `Patch` alanları +
  `applyPatch` `applyInt` çağrıları + `normalize` clamp'leri (süreler 1–1440 dk;
  `spawnIdleTimeoutMin` hard-cap'i **aşamaz**, yoksa watchdog hiç ateşlemez).
- **Yeni değer:** `spawnTimeoutMin` = **120 dk** (global `~/.tionharness/settings.json`
  → tüm workspace'ler; `internal/settings` app-seviyesi, workspace-scoped değil).
  Kod default'u (`DefaultSpawnTimeoutMinutes = 20`) değişmedi — yalnız fallback.

## Kesilen worker turu artık "completed" diye raporlanmıyor ✅ (2026-07-28)

Yukarıdaki SES17'nin ikinci yarısı: tur 20 dk'da kesildiği hâlde koordinatöre
`<status>completed</status>` + yarım cümlelik `<result>` gitti, koordinatör işi
bitmiş sandı. **Sebep:** kesilme hiçbir yerde hata olmuyor — claude-cli
`salvage()` kısmi metni `err=nil` ile döndürüyor, native döngü de iterasyon
limiti/guardrail halt/bağlam tükenmesinde `StepRecovery` ekleyip `nil` dönüyor.

- **`withActivityTimeout` → `context.WithCancelCause`** (`activity_timeout.go`):
  yeni sentinel'ler `ErrTurnHardTimeout` / `ErrTurnIdleTimeout`. Öncesinde
  watchdog iptali ile kullanıcının "Durdur"u **ayırt edilemiyordu** (ikisi de
  `context.Canceled`), yani süre dolması "killed" olarak da raporlanabilirdi.
- **Yeni `internal/agent/turnoutcome.go`** — `classifyTurnOutcome(ctx, steps,
hard, idle)`: ctx cause + trace'teki **terminal** `termReason` işaretlerini
  okur (mid-turn `contReason`'larla çakışmaz) → `timeout` / `incomplete` /
  `completed`. `applyTurnOutcome` kurtarılan metnin **önüne** ne olduğunu
  anlatan Türkçe not koyar (metin atılmaz — işin nereye kadar geldiğinin tek
  kaydı).
- **`runWorker`** artık bu verdikti kullanıyor; deadline kontrolü
  `context.Canceled` kontrolünden **önce** gelir. Kesik turda ayrıca
  `debug.jsonl`'e `error` olayı düşer ve log'a `worker: turn truncated` yazılır.
- **Kapsanan durumlar:** süre tavanı, boşta watchdog'u, **araç iterasyon limiti**
  (`maxToolIters`, vars. 500), guardrail halt, bağlam penceresi ve çıktı-token
  tükenmesi.
- **`runSpawn` de aynı verdikti kullanıyor** (worker'sız detached spawn'lar):
  kesik turda (a) kurtarılan metnin önüne not, (b) trace'e `turn_timeout`
  `StepRecovery` (`appendOutcomeStep` — watchdog döngünün DIŞINDA iptal ettiği
  için başka hiçbir şey iz bırakmıyordu; loop'un kendi `term*` işaretleri
  tekrarlanmaz), (c) `spawn` olayı **başarısız** seviyesinde yayınlanır,
  (d) `ClearParentTagsOnSuccess` **çalıştırılmaz** — yarıda kalan bir onarım
  ajanı ebeveynin `error` etiketini temizleyemez, sınırlı onarım döngüsü
  tekrar denesin. Hata dalında ham `context canceled` yerine hangi tavanın
  dolduğunu söyleyen not kaydedilir.
- **Testler:** `turnoutcome_test.go` (8 vaka: temiz tur, hard/idle timeout,
  kullanıcı-stop'un timeout sayılmaması, 4 terminal işaret, mid-turn retry'ın
  turu bozmaması, iz adımı tekrarlanmaması, not birleştirme). Full suite +
  `go vet` yeşil.

Detay `_Docs\47` ▸ "task-notification formatı".

## Transkript açılış maliyeti: iz kırpma + off-screen render atlama ✅ (2026-07-28)

**Şikâyet:** bir worker oturumunu her açtığında adımlar/tool kullanımları "baştan
yükleniyor" gibi geliyordu. **Ölçüm sonucu varsayım yanlıştı:** sunucu zaten
cache'liyor — `db.loadSessions` boot'ta tüm `session.jsonl`'leri belleğe alıyor,
`ListMessages` disk'e hiç gitmiyor. Gerçek maliyet (a) wire payload'ı, (b) React
render'ıydı. İkisi de hedeflendi:

- **Sunucu-tarafı iz kırpma** — yeni `internal/api/steps_trim.go`: okuma yolunda
  `output`/`text`/`patch` + `input` yaprakları 2 KB'a kesilir, `*Truncated`/`*Len`
  bayrağıyla işaretlenir, `subSteps` özyinelemeli. `input` anahtarları korunur
  (diff sentezi + tool etiketi onlara bağlı). Disk ve model bağlamı **tam kalır**.
  `handleListMessages` + `publishHub(KindReply)`'a bağlandı. Gerçek 11-oturumlu
  workspace'te iz payload'ı **~%45** küçüldü.
- **Talep üzerine tam iz** — `GET /api/sessions/{id}/messages/{msgId}/steps` +
  turun 🔧 satırındaki **"⤓ tam iz"** çipi.
- **Off-screen render atlama** — `MessageList` satırlarına `content-visibility:auto`.
  Virtualizer bilerek kullanılmadı: scroll/deep-link mantığı `data-msg-id` ile
  gerçek DOM'u sorguluyor, unmount hepsini bozardı. 30 satırdan kısa transkriptte
  devre dışı; son 3 satır eager; atlamalar rAF'ta ikinci kez hizalanır.
- **Memoizasyon** — `AssistantTurn`/`UserTurn`/`PeerTurn`/`TurnSteps`/`ActivityCard`
  `React.memo`, `parseSteps` `useMemo`'ya alındı (her delta'da yüzlerce KB JSON.parse
  ediyordu), handler kimlikleri `shared/lib/useStableCallback.ts` ile sabitlendi.

Detay `_Docs\07-CHAT-UX.md` ▸ "Transkript yükleme maliyeti".

Yan bulgu: `SkillImportDialog.tsx`'te `KIND_LABEL` haritası `hook` ingest türünü
kaçırdığı için `npm run build` kırıktı (bu değişiklikle ilgisiz) — eklendi.

## Oturum-hedefi (goal) mekanizması tamamen kaldırıldı ✅ (2026-07-28)

Kaldırıldı çünkü **yarım bir özellikti**: bir kuzey-yıldızı metnini her turun
dinamik suffix'ine enjekte ediyordu, ama "bitti"ye işi yapan modelin kendisi karar
veriyordu (`complete_goal`) ve hiçbir şeyi sürmüyordu — `autocontinue.go` goal'den
tamamen habersizdi. Claude Code'un `/goal`'ü ise bir **kontrol akışıdır**: ayrı bir
değerlendirici model her turdan sonra transkripti okuyup durma koşulunu yargılar ve
karşılanmadıysa yeni tur başlatır. İleride bu şekilde ayrı bir oturumda yeniden
kurulacak; o zamana kadar yarı-uygulanmış hali beklenti yaratıp karşılamıyordu.

- **Silinen dosyalar:** `internal/agent/goal.go` (+test), `internal/api/goal.go`,
  `internal/tools/goalsink.go`, `frontend/src/features/sessions/SessionGoalSection.tsx`.
- **Veri modeli:** `db.Session.Goal` / `GoalDone` + `SetSessionGoal` kalktı. Diskteki
  eski `goal` alanları JSON'da öylece kalır ve **yok sayılır** (migration gerekmez).
- **Prompt:** `goalContextBlock` enjeksiyonu (chat + otonom yol) ve statik prefix'teki
  `GoalUsageHint` kalktı — `BuildSystemPrompt` artık salt persona (soul + identity).
- **Araç yüzeyi:** `update_session`'ın `goal` / `goal_done` alanları kalktı;
  `tools.SessionSink` artık `GoalSink`'i gömmüyor; `get_session_info` goal satırı yok.
- **Yan sistemler:** `PUT /api/sessions/{id}/goal` route'u, `SessionInfo.goal/goalDone`,
  bağlam ölçerdeki "Hedef" kovası, handoff'un `HandoffEnv.Goal` alanı ve
  **`goal` / `goal-done` auto-tag'leri** kalktı → bu etiketlere bağlı bir otomasyon
  varsa artık tetiklenmez (detay `46`).
- **Seed skill'ler:** `tionharness-guide` ve `tionharness-progress` artık ölü API'yi
  öğretmiyor; hedefin yeri `todo_write`/progress olarak yazıldı.
- Doğrulama: `go vet ./...` temiz; `go test` agent/tools/api/db/conversation/skills
  paketleri yeşil. `npx tsc --noEmit` bu değişikliklerde temiz.

  worker onu alınca boyanıyordu; yukarı kaydırmış kullanıcı o ana kadar hiçbir tepki
  görmüyordu.

- **Bekleme göstergesi:** bağımsız `WorkingDots` balonu artık `pending`'in yanı sıra
  `streaming` ile de çıkar, ve `App` oturum açılışında `GET /api/sessions/active`'ten
  o oturumu `markPending` ile tohumlar (yalnız EKLER — kuyruğa yeni atılmış tur
  sunucuda henüz kayıtlı olmayabilir). Çalışan bir oturum açıldığında transkript artık
  kendi mesajımızda bitip ajan susmuş gibi görünmüyor.
- **Coordinator workflow picker → satır başına (ⓘ) + 📖:** `<select>` bir radio
  listesine çevrildi (`<option>` buton taşıyamaz). Her recipe satırında kendi
  açıklama balonu (`recipeHelp` — frontmatter'daki `pattern`/`workerTargets`/
  `stopCondition`/`maxTurns`'ten üretilir) ve kendi skill'ini açan 📖 butonu var;
  etiket yanındaki (ⓘ) genel "workflow nedir" notu olarak kalır. Skills'e geçiş
  yeni `setSessionState` seam'i ile — seçim tohumlanır, Skills ekranı mount olurken
  okur → URL şeması değişmedi. Balonlar `fixed` modda: oturum paneli
  `overflow-hidden` olduğu için `absolute` balon kırpılıyordu; ayrıca
  `InfoPopover`'ın fixed dikey clamp'i artık uydurma 180px yükseklik yerine gerçek
  bir tavan (`max-h` + scroll) kullanıyor, uzun not viewport dışına taşmıyor.

## `shellOutputCompression=on` sessiz kalıyordu: DTO + ajan uyarısı ✅ (2026-07-28)

WS16'da sıkıştırmayı açarken iki ayrı sessiz boşluk çıktı:

- **API DTO'su alanı hiç taşımıyordu.** `PUT` diske doğru yazıyordu ama `GET` boş
  dönüyordu → Ayarlar ▸ Workspace seçici (`WorkspacePanel.tsx`, `ws.shellOutputCompression || ''`)
  gerçek değer `on`/`off` olsa bile **daima "auto"** gösteriyordu. `workspaceSettingsDTO`'ya
  alan eklendi.
- **Ajan `on` modunda bilgilendirilmiyordu.** `tokenOptimizerCapability` yalnız hook
  tarıyordu; `on` hook gerektirmediği için sıkıştırma çalışıp ajan uyarısız kalıyordu —
  sqz'nin kısaltmalı çıktısını truncation sanma riski. `Runtime.effectiveTokenOptimizers`
  artık in-process filtreyi de sayıyor (`*` matcher, daraltıcı kapsam notu yok); `off` ise
- `ShellTool` artık `preArgs` taşıyor, böylece `-c`'den önce ek argüman verilebiliyor.
- Git-bash tercih ediliyor çünkü Windows dosya sistemi + ağ yığınını paylaşır; diğer
  araçların (Read/Write/cwd) Windows yollarıyla tutarlı kalır.
- Testler: `builtin_shell_posix_test.go` — launcher tespiti, resolver'ın launcher'ı
  reddetmesi ve seçilen kabukta `X=..; Y=$(..)` genişletmesinin gerçekten çalışması.

## Bash aracı WSL launcher'ına düşünce `$VAR` sessizce boşalıyordu ✅ (2026-07-28)

Windows'ta `resolvePOSIXShell` PATH'teki ilk `bash`'i alıyordu; bu çoğu makinede
`C:\Windows\System32\bash.exe`, yani WSL launcher'ı. Launcher `-c` payload'ını gerçek
bash'e vermeden önce **bir dış kabukta expand ediyor** → script'in kendi atadığı
değişkenler ve `$(...)` sonuçları boş geliyor. Pratikte `T=$(cat tok.txt)` sonrası
`curl -H "Bearer $T"` boş kimlik gönderip 401 alıyordu; ajan bunu "kabuk değişkeni
genişletmesi bozuk" diye raporlayıp Python'a kaçıyor, orada da WSL'in ayrı network
namespace'i yüzünden Windows `127.0.0.1`'ine ulaşamıyordu (`Errno 111`).

- Resolver ayrı dosyaya alındı: `internal/tools/builtin_shell_posix.go`. Sıra artık
  PATH'teki bash (launcher değilse) → git-bash (PATH'te olmasa da git.exe'den ve standart
  kurulum yollarından bulunur) → son çare `wsl.exe -e bash` (argv'yi bozmadan geçirir).
  altına taşındı** → dolu şeritte yanlış tıklamayla kural silinmiyor.
- `data-testid`'ler korundu (`schedule-row`, `schedule-enable-toggle`,
  `schedule-run-now`, `schedule-edit`, `schedule-delete` — sonuncusu artık popup
  içinde, `schedule-create-*`); kartlara `automation-row`/`data-automation-id` eklendi.
- Detay: `46-ETIKET-OTOMASYON.md` ▸ "UI — 3 SÜTUNLU PANO".

## Frontend format otomasyonu (Prettier + pre-commit hook) ✅ (2026-07-29)

Frontend'in tek bir format otoritesi yoktu; elle tutarlı yazılıyordu ve config'siz bir
`npx prettier` çağrısı dosyaları çift tırnak/noktalı virgüle çevirip sahte diff üretebiliyordu.

- `frontend/.prettierrc.json` — `singleQuote` · `semi:false` · `printWidth:100` ·
  `trailingComma:all` · **`endOfLine:auto`** (CRLF çalışma kopyası + LF depo; `lf` deseydik
  106 dosya yalnızca satır-sonu yüzünden "farklı" görünürdü). `prettier` devDependency,
  `.prettierignore` (dist/node_modules/public/lock).
- Script: `npm run format` / `npm run format:check`.
- `.githooks/pre-commit` — **yalnız stage'lenmiş** dosyaları formatlar (frontend→prettier,
  `*.go`→gofmt) ve yeniden stage'ler; prettier **mutlak yolla** çağrılır (npm `.bin` hook'un
  düz `sh`'ında PATH'te değil). Etkinleştirme: `git config core.hooksPath .githooks` (yapıldı).
- `.vscode/settings.json` — kaydet-formatla + prettier/gopls eşlemesi.
- **Toplu format ATILMADI:** ağaçta 111 commit'lenmemiş dosya vardı, repo-geneli yeniden
  format gerçek diff'i gömerdi. `format:check` şu an ~248 dosyada uyarıyor; dokunulan dosya
  hook ile kendiliğinden dönüşür. Toplu geçiş temiz ağaçta kendi commit'inde yapılmalı.
- Kural CLAUDE.md'ye yazıldı ("config'siz prettier çalıştırma").

## Composer'da araç müfettişi — "ajan neyi kullanabiliyor?" ✅ (2026-07-28)

Sohbetten çıkmadan görülemiyordu: ajanın o an hangi araçlara sahip olduğu, hangilerinin
şemasının her tur gönderildiği (pahalı), hangilerinin yalnız katalogda durup gerektiğinde
açıldığı ve MCP gateway'de nelerin bağlı olduğu. Composer toolbar'ına 🔧 butonu eklendi;
açtığı panel **salt bilgi** — hiçbir ayarı değiştirmez (değişiklik yine Araçlar ekranından).

- Backend: `GET /api/agents/{id}/tool-access` (`internal/api/agent_tool_access.go`) —
  eager/lazy katalog + tier + ajan `blocked` listesi + MCP sunucu envanteri
  (etkin mi, transport/kapsam, canlı bağlantı sayısı, sunucu başına aktif/hazır adet).
- Frontend: `features/chat/composer/ToolAccessPanel.tsx` (üç sekme: Aktif · Talep üzerine ·
  MCP, arama kutusu), `ToolAccessList.tsx` (gruplu satırlar + sunucu listesi),
  `toolAccessGroups.ts` (saf grup/filtre). Built-in'ler kategoriye, MCP araçları sunucuya

## Otomasyon ekranı 3 sütunlu panoya çevrildi ✅ (2026-07-28)

Sekmeli görünüm (⏰ Zamanlamalar / 🏷 Etiket / 🗂 Pano) yerine **üç şeritli pano**:
üç kural türü artık aynı anda yan yana görünüyor, sekme değiştirmeye gerek yok.
Her şeridin başlığında sayaç + kısa açıklama + **`+`** butonu var; `+` o türün
**oluşturma popup'ını** açar. Kurallar salt-okunur kart; düzenleme de aynı

- Doğrulama: `go build ./...` + `go vet` + `go test ./internal/market/... ./internal/ingest/...`
  (19 test) yeşil, `npx tsc --noEmit` yeşil.
- **Kalan (içerik kararı):** şablonlar hâlâ yalnız agents + lineer flow + schedule taşıyor;
  automations (etiket/pano) için payload alanı bile yok, koordinatör rolü ve
  `await-input`/`subflow`/`spawn-join` düğümleri şablonla dağıtılamıyor.

## Git worktree izolasyonu kaldırıldı ✅ (2026-07-28)

Otonom oturuma per-session git worktree + dal veren `gitWorktreeIsolation`
çalışma-zamanı özelliği tamamen kaldırıldı — ileride kapsamlı biçimde yeniden
eklenecek (bkz. `41` madde 11: `EnterWorktree`/`ExitWorktree` ile birleşik tasarım).

- **Kaldırılanlar:** `internal/agent/worktree.go` (dosya silindi: `ensureWorktree` /
  `RemoveSessionWorktree` / `worktreesDir` / `gitRepoToplevel`); `Tunables`
  `gitWorktreeIsolation` alanı + `GitWorktreeIsolation()` getter; `SetWorkdirGuards`
  imzasından parametre düştü; `toolloop.go` otonom worktree dallanması; `sessions.go`
  oturum-silme temizlik çağrısı; `settings.go` (Settings/View/Patch) + `store.go`
  gerçek bir sqz hook'unu gizlemiyor. Testler: `capabilities_tokenopt_test.go`.

Ölçüm notu: WS16'nın üç worker oturumunda shell çıktısı toplamı ~40K char, sqz kazancı
**%24.8** (~2.5K token). Bağlamın asıl yükü `Write` (%39–44) ve `Read` (%16–34) — sqz
ikisini de kapsamıyor, yani shell sıkıştırma tek başına belirleyici değil.

artık paketle taşınabiliyor** (önceden sessizce düşüyordu).

- **`NODE_ICON` 5 → 12 node tipi** (start/end/loop/await-input/subflow/spawn/join eklendi);
  workspace önizlemesindeki akış çipleri tüm non-lineer tipleri gösterir.
- **Ölü kod/tip temizliği** — kaldırılmış `memory` türüne ait yorumlar, `AgentPayload.capabilities`,
  kullanılmayan `SourceWorkspace` tier'ı (frontend `PackSource` + `SOURCE_LABEL` dâhil).
  Eklenenler: `toolOverrides` (TS), `prompts`/`readme` (TS + workspace önizlemesinde bölüm).
- **Gömülü 5 şablona `version: 1.0.0`** → `updateAvailable()` artık gömülülerde de çalışır.
- **Varsayılan kategori `skill` → `workspace`** (gömülü skill paketi 0 olduğu için market
  ilk açılışta boş ızgara gösteriyordu).
  `applyBool` alanları; frontend `types/settings.ts` + `AppToolsPanel` toggle'ı +
  `SettingsPanel` gönderimi.
- **Korunanlar:** `autonomousConfine` freni ve **geliştiriciye ait** `scripts\worktree.ps1`
  yardımcısı (tamamen ayrı) yerinde.
- Doğrulama: `go build ./...` yeşil, `npx tsc --noEmit` yeşil.

## Sohbet gezinme + bekleme geri bildirimi ✅ (2026-07-28)

Transkriptte dört küçük ama sürtünme yaratan boşluk kapatıldı (detay `07`, `47`).

- **Yapışkan soru başlığı tıklanabilir:** üstte sabitlenen kullanıcı sorusuna
  tıklayınca o mesaja dönülür. Sarmalayıcı `pointer-events-none` kalır (gradyan
  üzerinden scroll geçmeye devam eder), yalnız balon `pointer-events-auto` olur;
  overlay scroll konteynerinin dışında olduğu için wheel elle iletilir. Satır üstten
  8px aşağıya oturur — aksi halde sticky başlık jump ettiğimiz mesajın üstüne binerdi.
- **Gönderimde dibe in:** `ChatView` composer submit'inde (send + queue)
  `scrollBottomSignal`'ı bump eder. Gönderim yalnız kuyruğa aldığı için mesaj ancak
- Doğrulama: `npx vitest run` 42 test yeşil. `npx tsc --noEmit` bu değişikliklerde
  temiz; repoda o sırada **başka bir çalışmanın** yarım kalan `IngestKind` düzenlemesi
  vardı (`SkillImportDialog.tsx` güncellenmemiş) — dokunulmadı.

## Navbar'da hayalet "Sohbet çalışıyor" noktası ✅ (2026-07-28)

Boştaki bir workspace'in nav rail'inde `Sohbet` nabız noktası (ve yan etkileri:
sidebar spinner'ı, composer'daki "Durdur" butonu) hiçbir tur çalışmadığı halde
yanıp kalıyordu; yalnız sayfa yenilemek geçiriyordu. Backend suçsuzdu —
`GET /api/activity` ilgili workspace için `chat:false` dönüyordu.

- **Kök neden:** `useChatStream.streamingSessions`, poll gecikmesini gizleyen
  istemci-tarafı bir mandal. Yalnız olayla temizleniyor, tamamlanma olayları ise
  `useAppEvents`'te `e.workspaceId === aktif workspace` koşuluyla filtreleniyor.
  Kullanıcı tur bitmeden başka workspace'e geçerse temizleme olayı düşüyor ve
  mandal sekme ömrü boyunca asılı kalıyor. Oturum ID'leri **store başına**
  sayaçla üretildiğinden (`db.nextID` → her workspace kendi `SES1`, `SES2`…
  serisini verir) bu yetim ID, ekrandaki workspace'in gerçek bir oturumuyla
  çakışıyor ve `App.tsx`'teki `chatBusyLocal` kesişim testini yanlış yere
  geçiriyordu.
- **Düzeltme:** kurtarma yolu artık **mutabakat** yapıyor, yalnız eklemiyor.
  `useChatStream.reconcileActive(ids)` pending'i `GET /api/sessions/active`
  listesiyle değiştirir, streaming'i o listeye daraltır (yeni saf yardımcı
  göre gruplanır. Panel akış sürerken de açılabilir (read-only), ajan değişince remount.
- Not: `/tools` slash komutu (LLM'e özet yazdıran, token harcayan yol) duruyor; bu buton
  aynı bilgiyi **sıfır token** ile verir.
- Ek tur: panel **boşluğa tıklayınca** kapanır (mousedown; 🔧 toggle'ı `data-tool-access-toggle`
  ile hariç tutulur, aksi halde kapat→anında-aç yanıp sönmesi olurdu) ve MCP sekmesindeki her
  sunucu **"bağlamda mı"** verdikti taşır (`in-context` / `hidden-only` / `disabled` /
  `agent-mcp-off` / `no-tools`) + aktif/katalog/gizli sayaçları. Özel (custom) MCP'lerin
  bağlama girip girmediği artık tek bakışta görünüyor.
- Terminoloji düzeltmesi: `hidden-only` rozeti **"bağlam dışı" değil "katalog dışı"**.
  Gizli tier promptta ~40 token'lık bir işaretçi bırakır ("N araç daha var, `tool_search`
  ile bul"), yani araçlar bilinmez değil — yalnız isim listesi çıkarılmıştır (49 self-
  management aracı için ~800–1000 token/tur tasarruf). Gizli sayaç rozeti tooltip'inde
  kabaca tasarruf gösteriliyor. Gerekçe: `_Docs\19` son bölüm.

Detay: `_Docs\19-LAZY-TOOL-LOADING.md` + `_Docs\07-CHAT-UX.md`.

## Akış `coordinator` node tipi — dinamik worker fan-out ✅ (2026-07-28)

`parallel`/`spawn` fan-out genişliği tasarım anında sabitti; "bulunan her bulgu için bir
worker" gibi sayısı **çalışma anında** belli olan işler ifade edilemiyordu. Yeni
`coordinator` node'u seçilen ajanı kendi koordinatör oturumunda çalıştırır, ajan kaç
worker açacağına anlık karar verir, düğüm hepsi bitene kadar bloklar ve son yanıtı
`{{last}}`'e koyar. Graf deterministik motorda kalır — düğüm motor açısından atomiktir,
yarıda kalan koşu resume'da düğümü baştan (yeni oturumla) çalıştırır.

- Motor: `orchestration.NodeCoordinator` + opsiyonel `CoordinatorRunner` arayüzü; panic-safe,
  accumulate modunda sonucu `parallel` gibi tek sentetik user/assistant çifti olarak katlar.
- Runtime: `internal/agent/flow_coordinator.go` — `Kind="flow-coordinator"` +
  `Role="coordinator"` oturum, `enqueueCoordinatorTurn` (slot'u senkron claim eder → erken
  "boşta" okuması imkansız), `waitCoordinatorIdle` yoklaması, timeout'ta `StopWorker`.
- `RecoverOrphanedTurns` bu oturumları **atlar** (ve bağlı yetim worker'lar için
  `NotifyCoordinator` yapmaz), yoksa `ResumeRunningFlows`'un yeniden çalıştırmasıyla yarışıp
  işi iki kez yapardı.
- **Koordinasyon türü (recipe) düğümden seçilebilir:** node'un `workflow` alanı Oturum
  Bilgisi panelindeki listenin aynısını sunar; slug oturumun `CoordinatorWorkflow`'una
  yazılır (reçete gövdesi normal yoldan prompt'a girer), reçetenin `max_turns`'ü düğüm
  kendi tavanını vermediyse uygulanır. Doğrulama `skills.ResolveCoordinatorWorkflow`'a
  (leaf paket; `api.ResolveCoordinatorRecipe` artık alias) taşındı ve hem precheck'te hem
  düğümde koşar — bilinmeyen reçete sessizce serbest koordinasyona düşmez.
- UI: `CoordinatorNode` (pusula, `#ea580c`), palet + (ⓘ), inspector formu
  (ajan/görev/workflow/maxTurns/timeoutSec), koşu görüntüleyicide hedef + rapor kartı,
  Oturumlar'da "Akış Koordinatörü" kind rozeti. Workflow seçici tek paylaşılan bileşen
  (`shared/components/CoordinatorWorkflowPicker`); `CoordinatorSection` de ona taşındı.
- Not: bu iş sırasında `internal/agent` test paketi zaten derlenmiyordu — `bootseq_test.go`
  popup'tan (kalem ikonu) yapılır — ekranın yarısını yiyen satır-içi form ve
  satır-içi editör kaldırıldı.

- `Schedules.tsx` (795 satır) + `Automations.tsx` (911 satır) ikilisi silindi; yerine
  12 odaklı dosya: `AutomationBoard.tsx` (ekran + veri + mutasyonlar),
  `BoardColumn.tsx` (şerit kabuğu), `ScheduleCard.tsx`/`AutomationCard.tsx` (kartlar),
  `FormModal.tsx` (ortak popup kabuğu), `ScheduleModal.tsx`/`AutomationModal.tsx`
  (oluştur+düzenle), `AutomationFields.tsx` (`BoardTriggerFields`+`PromptVarsField`),
  `pickers.tsx` (`TargetModeToggle`/`FlowPicker`/`Field`), `automationMeta.ts`,
  `cronPresets.ts`, `timeUtils.ts`.
- `App.tsx` artık `AutomationBoard`'u render ediyor (`Schedules` yerine).
- Etiket otomasyonu modalına **Ad** alanı eklendi (eskiden yalnız şablon set ediyordu).
- **Dikey/dar ekran:** `md` altında üç şerit `snap-x snap-mandatory` karuseli
  (`w-[85vw]`, şerit başına bir ekran, her şeridin kendi dikey scroll'u); `md`+ üçü
  yan yana `flex-1`. Popup'lar `ModalOverlay` sayesinde telefonda bottom-sheet.
- Kart aksiyonları çerçeveli ikon buton (`CardAction`): ▶ çalıştır + ✏️ düzenle
  (+ limit dolduysa ↺ sıfırla). **Sil karttan kaldırılıp düzenleme popup'ının sol
  kaldırılmış `gitWorktreeIsolation` parametresini geçiyordu; çağrı 2 argümanlı yeni imzaya
  güncellendi.

Detay: `_Docs
-FLOW-CANVAS.md` + `_Docs'-KOORDINATOR-COKLU-AJAN.md`.

## Market ekranı + item'ları bayatlık tazelemesi ✅ (2026-07-28)

Market, son alt sistemlerin gerisinde kalmıştı. Detay tablo: **`_Docs\21-MARKET.md` §8**.

- **`hook` türü UI'a bağlandı** — backend'de kurulabiliyordu ama `PackKind`/`IngestKind`'da
  yoktu: sekme yok, `KIND_LABEL['hook']` boş (İçe Aktar diyaloğunda başlıksız grup),
  önizleme yok. Artık **Hooks** kategorisi + hook önizleme kartı (olay/matcher/timeout +
  komut + güvenlik uyarısı) var; kurulum dedup yapmadığı için "zaten kurulu" işareti yok.
- **MCP paketi hibrit kapsama uyduruldu** — `MCPPayload`'a `description`/`headersConfig`/
  `scope`; `installMCPPack` sabit `Scope:"shared"` yerine payload'ı aktarır (allow-list'li).
  `mcp_adapter` `.mcp.json`'daki `headers` bloğunu okur → **auth başlıklı HTTP MCP sunucusu

## Flow paleti: node butonlarında (ⓘ) açıklama balonu ✅ (2026-07-27)

Flow editöründe sol paletteki node tipleri yalnız ad + ikon gösteriyordu; ne işe
yaradıkları görünmüyordu. Her palet satırına bir **(ⓘ) bilgi butonu** eklendi;
tıklayınca o node tipinin ne yaptığını anlatan balon açılıyor.

- Metinler: yeni `frontend/src/features/flows/nodeTypeHelp.ts` (`NODE_TYPE_HELP`,
  12 node tipinin tamamı; `internal/orchestration/model.go` yorumlarıyla hizalı).
- `shared/components/InfoPopover`'a opsiyonel **`fixed`** modu: balon viewport
  koordinatlarında çizilir (kenarlara clamp'li). Palet kolonu `overflow-y-auto`
  olduğu için mutlak konumlu balon kırpılıyordu. Varsayılan kapalı → mevcut
  kullanım yerleri etkilenmedi.
- `FlowEditorView` palet satırı `flex` sarmalayıcıya alındı (buton `flex-1`,
  etiket `truncate`); sürükle-bırak + tıkla-ekle davranışı korundu.
- Doğrulama: `npx tsc --noEmit` yeşil.

## Per-ajan araç override'ları — "Yasaklı Araçlar" 5. tier oldu ✅ (2026-07-27)

Araç görünürlük tier'ları (Tam/Özet/İsim/Gizli) yalnız **workspace** seviyesinde
ayarlanabiliyordu; ajan seviyesinde ise ayrı bir **yasaklı araç** denylist'i vardı —
iki ayrı model, iki ayrı UI, ajan başına ince ayar imkânsız. Artık tek bir 5 değerli
ajan override haritası var: `full | summary | name-only | hidden | **blocked**`.

- **Öncelik zinciri:** kod default < workspace `ToolVisibility` < ajan `ToolOverrides`.
  `blocked` registry'ye **girmez** — bir katman yukarıda `toolFilter`'da çözülür ve
  aracı katalogdan tamamen düşürür; görünürlük semantiği kirlenmez.
- **Persistans:** yeni `Agent.ToolOverrides` (JSON object; anahtar tam ad **veya**
  `prefix*` deseni). `BlockedTools` silinmedi, **türetilmiş ayna**'ya dönüştü —
  `UpdateAgentTools` her yazışta `blocked` girdilerinden sıralı üretip yazar, böylece
  market paketleri/şablonlar/eski ajan dosyaları bozulmaz.
- **Migrasyon okuma tarafında:** `agent.ParseToolOverrides` iki kaynağı birleştirir
  (legacy denylist → `blocked`), açık override daima kazanır. Bozuk JSON fatal değil →
  override'sız duruma düşer, ajan çalışmaz hale gelmez.
- **Desen genişletmesi:** `SetVisibility` tek isim aldığı için `prefix*` anahtarları
  katalog üzerinde genişletilir; anahtarlar sıralı işlenir → daha özel (uzun) desen kazanır.
- **UI = DIFF:** ajan Araçlar ekranı ikinci bir katalog değil; yalnız override'lı araçlar
  `varsayılan → seçili` rozet çiftiyle listelenir (+5'li tier seçici, "Varsayılana dön",
  toplu uygulama). Varsayılana eşit seçim override'ı **siler**. Katalogda karşılığı
  olmayan anahtarlar (desen / o an kapalı araç) kesikli çerçevede korunur — sessizce
  düşürmek bir yasağı kaldırırdı.
- Kod: `agent/tooloverrides.go` (yeni), `agent/toolsetup.go` (`buildRegistry` iki-katmanlı
  zincir, `blockFunc`, `ActiveToolCatalogWithState`), `db/models.go`+`db/store_mcp.go`,
  `api/agent_tools.go`, frontend `AgentToolsSection.tsx`+`AgentToolOverrideRow.tsx`+
  `VisibilityControls.tsx`+`toolMeta.ts`. Testler: `tooloverrides_test.go` (6) +
  `tooloverrides_integration_test.go` (4). Detay: **`_Docs/19`**.

## SSE kopması toleransı: yeniden-bağlanınca resync ✅ (2026-07-27)

Poll'ler SSE'ye taşındıkça (bir alttaki giriş) yeni bir kırılganlık doğdu: **feed
kopup geri geldiğinde arada yayınlanan frame'ler kalıcı olarak kayıp** — backend bus'ı
fire-and-forget, replay yok, `Last-Event-ID` cursor'ı yok. Yalnız event'lerle render
eden bir görünüm bunu **kendi başına fark edemez**; en kötü hâli: outage sırasında biten
worker yüzünden banner çoktan rapor vermiş bir worker'da asılı kalır.

`api/system.ts` zaten CLOSED olan kaynağı 2sn backoff'la yeniden kuruyordu, ama kimseye
"kaçırdın, tazele" demiyordu. Eklenenler:

- **`sharedES.onopen`** → ilk açılış değilse `reconnectSubs`'a sinyal (`everConnected`
  bayrağı ilk açılışı ayırt eder; feed idle'dan kapanınca sıfırlanır → sonraki açılış
  yine "ilk" sayılır, çünkü yeni aboneler kendi başlangıç fetch'ini yapar).
- **`subscribeReconnect(cb)`** — `subscribeEvents`/`subscribeLogs` ile aynı desen
  (multiplex + `closeIfIdle` muhasebesine dahil).
- **`useAppEvents.onReconnect`** — SSE-only kümeyi tazeler: `refreshSessions()` +
  `publishWorkerChangeAll()` (yeni; hangi koordinatörün etkilendiği bilinemediği için
  tüm roster aboneleri) + açık oturumun transkriptini yeniden yükler.

Tarayıcının kendi auto-reconnect'i (readyState CONNECTING) ve modül-seviyesi rebuild
(CLOSED → backoff) **ikisi de** aynı `onopen` yolundan geçtiği için tek kanca yeterli.
`tsc --noEmit` temiz, `npm run build` + vitest (38) yeşil.

## Worker banner'ı: canlı süre + poll→SSE ✅ (2026-07-27)

Banner'ın (bir alttaki giriş) iki eksiği kapatıldı.

**1) Canlı süre.** `WorkerInfo`'ya `StartedAt` (unix sn) eklendi; kaynak
`workerCtl.startedAt` (runWorker turu başlatırken damgalar), API'de `startedAt`
alanı. Banner her çipte 1sn tick ile `Xsn` / `Xdk Ysn` yazar. Start zamanı
bilinmediğinde (ctl yok: worker oturumunda doğrudan açılmış tur veya restart'ı
atlatmış tur) alan 0 kalır ve UI süreyi **gizler** — uydurma süre göstermez.

**2) Poll kaldırıldı, SSE geldi.** `runWorker` başında yeni `emitWorkerStartEvent`
bir `worker` event'i yayınlar (`Target.phase="start"`; spawn + `send_to_worker`
ikisini de kapsar). `useAppEvents` her `worker` event'ini `coordinatorId` anahtarıyla
yeni `shared/lib/workerBus.ts`'e fanlar; `useRunningWorkers` **ve**
`CoordinatorSection` abone olup roster'ı tazeler — ikisindeki 3sn poll silindi.
Boşta trafik sıfır; worker başlayınca/bitince banner anında güncellenir.

**Kritik ayrıntı — `phase="start"` gate'i.** `useAppEvents`'in otonom-tamamlanma dalı
`worker` event'ini "tur bitti" sayıyor: ghost balonu siler, transkripti yeniden yükler,
`publishTurnEnd` fanlar ve masaüstü toast'ı atar. Bunların hepsi yeni **başlayan** bir
tur için yanlış olurdu (üstelik 8'li fan-out 8 toast demekti), o yüzden start event'i
hem o daldan hem toast funnel'ından hariç tutuldu; `refreshSessions`/panel sinyalleri
akmaya devam eder (yeni worker oturumu sidebar'a düşsün).

Yeni: `frontend/src/shared/lib/workerBus.ts`. `go build` + `go test ./...` (1029)
yeşil, `tsc --noEmit` temiz, `npm run build` başarılı.

## Sohbette çalışan worker banner'ı ✅ (2026-07-27)

**Belirti:** Bir koordinatör oturumu `spawn_worker` ile worker başlatıp turunu bitirdiğinde
sohbet **bitmiş gibi** görünüyordu — ilk `<task-notification>` düşene kadar hiçbir işaret yok.
Canlı worker roster'ı yalnız sağ **SessionDetailPanel ▸ Koordinasyon** bölümünde vardı, yani
panel kapalıysa konuşmanın worker sonucu beklediği belli olmuyordu.

**Düzeltme:** composer'ın üstündeki alt-yığına `WorkerWaitBanner` eklendi (`WakeWaitBanner` ile
aynı desen): "N worker çalışıyor — sonuçları bekleniyor · M/T bitti" + her çalışan worker için
tıklanabilir çip (worker oturumunu açar; worker'lar birinci-sınıf oturum). Veri `useRunningWorkers`
hook'undan — `GET /api/sessions/{id}/workers`; **yalnız** `role==='coordinator'` oturumlarda etkin,
poll da yalnız (tur streaming || en az bir worker çalışıyor) iken 3sn'de bir; boşta oturum başına
tek fetch. Oturum değişiminde roster anında boşaltılır (başka koordinatörün listesi yanıltmasın).

Yeni: `frontend/src/features/chat/WorkerWaitBanner.tsx`, `useRunningWorkers.ts`;
`ChatView` yeni `sessionRole`/`onSelectSession` prop'ları, `App.tsx` ikisini de besler.
Yeni backend/API yok. `tsc --noEmit` temiz.

## Navbar kayboldu: Tailwind utilities cascade layer'dan çıkarıldı ✅ (2026-07-27)

**Belirti:** Sol `NavRail` masaüstünde hiç görünmüyordu; alt `MobileNavBar` de gizliydi, yani
1920px'te iki gezinme çubuğu birden yoktu. Kod tarafında hiçbir değişiklik yoktu (`tsc` temiz,
`App.tsx` her ikisini de koşulsuz render ediyor) — bozulan CSS'ti.

**Kök neden — Chrome 150 regresyonu.** Tarayıcıda ölçüldü: `hidden md:flex` sınıf çiftinde
`display` `none` kalıyordu, oysa `.md\:flex` kuralı üretilen CSS'te `.hidden`'dan ~700 kural
SONRA geliyor (CSSOM'da doğrulandı: `@layer utilities` içinde sırasıyla #111 ve #807), aynı
specificity, `!important` yok. Tailwind v4 responsive utility'leri **nested** yazar
(`.md\:flex { @media (width >= 48rem) { display: flex } }`); Chrome 150 bu nested `@media`
declaration'larının kaynak sırasını **büyük bir `@layer` bloğu içinde** kaybediyor. Kesin kanıt:
aynı CSS'te tek kelimeyi değiştirip (`@layer utilities {` → `@media all {`) ölçüm `flex`'e döndü.
Küçük bir layer'da tekrarlanmıyor, yani kural sayısına bağlı bir Blink hatası. Tailwind 4.2 de
aynı nested çıktıyı ürettiği için sürüm düşürmek çözmüyordu.

**Etki alanı navbar'dan genişti:** `hidden sm:inline` / `hidden md:block` gibi TÜM
"mobilde gizle, geniş ekranda göster" kalıpları (AppHeader etiketleri, ~25 dosya) ölüydü.

**Düzeltme (`frontend/src/index.css`):** tek satırlık `@import 'tailwindcss'` üç parçaya bölündü —
`theme.css layer(theme)` + `preflight.css layer(base)` + `utilities.css` **layer'sız**. Layer'sız
utility'ler kaynak sırasını koruyor, kalıp yeniden çalışıyor. Cascade açısından güvenli: utilities
zaten en yüksek layer'daydı, layer dışına çıkınca yalnızca daha güçlü oluyor. Dev + prod build
doğrulandı (rail 208px `flex`, mobil bar `md`'de gizli). Chrome hatası düzelince tek-satır
import'a dönülebilir — gerekçe index.css'teki yorumda duruyor.

## Test altyapısı: CI tam kapsam + frontend testleri ✅ (2026-07-27)

Test **kodlarının** durumu iyiydi (kaldırılan özelliklerin testleri de silinmiş — `tools/compact`,
memory, günlük limitler için sıfır artık referans), asıl boşluk **koşturma** tarafındaydı. Dört düzeltme:

1. **CI hiç koşmuyormuş — crabbox kaldırıldı, gate Gitea'ya taşındı.** İki katmanlı sorun: (a) mevcut
   `ci` job'ı 30 paketten yalnız 4'ünü (`conversation/billing/orchestration/skills`) koşturuyordu —
   `agent`/`api`/`tools`/`db`/`insight`/`providers`, yani testlerin ~%85'i, kapsam dışıydı; (b) daha
   kötüsü, o job `.github/workflows/` altındaydı ama **repo'nun tek remote'u Gitea** — GitHub'a hiç
   push edilmiyor, dolayısıyla workflow hiç çalışmıyordu. Crabbox tamamen kaldırıldı (`.crabbox.yaml`,
   `.github/`, harici-araç kataloğu satırı); GitHub runner'ının içinde ikinci bir Docker katmanı zaten
   gereksiz dolaylılıktı ve `slug=` stdout-parse'ı kırılgandı. Yerine **`.gitea/workflows/ci.yml`**:
   native `setup-go`/`setup-node` (deploy.yml'in zaten kanıtladığı desen), `go vet` + `go build` +
   `go test ./... -race` (`TIONHARNESS_ENABLE_SHELL=1`) + frontend `npm test` + `npm run build`.
   `-race` yalnız burada gerçekten koşar — Windows geliştirme makinesinde CGO kapalı. Ayrıca
   `deploy.yml` artık `needs: test` ile geçide bağlı: kırmızı build deploy edilemiyor.
2. **BOM fix.** `internal/tools/builtin_workspacemgmt.go` UTF-8 BOM ile başlıyordu; `go build`/`go vet`
   tolere ediyor ama cover instrumentation dosyayı yeniden yazınca BOM ortada kalıp
   `invalid BOM in the middle of the file` ile **tüm `internal/tools` paketinin coverage ölçümünü**
   kırıyordu. BOM kaldırıldı → paket %57.8 ile ölçülebiliyor (repodaki tek BOM'lu `.go` dosyasıydı).
3. **Bildirim sözleşmesine drift testi.** `events.NotifyKinds` ↔ `frontend/.../notifyTypes.ts` senkronu
   yalnız iki yorum satırıyla korunuyordu. `internal/events/notifyparity_test.go` TS dosyasını parse edip
   iki listeyi karşılaştırır: backend-only kind = Ayarlar'da susturulamaz bildirim, frontend-only kind =
   ölü toggle. `prompt` bilerek frontend-only (backend olayı yok). Yanında kontrol/stream tiplerinin
   `NotifyKinds`'e sızmadığı testi (sızarsa her log satırı toast olurdu).
4. **Frontend testleri sıfırdan.** Vitest kuruldu (`vitest.config.ts`, node env, `@` alias; `npm test` /
   `npm run test:watch`); kullanılmayan `@playwright/test` devDependency'si kaldırıldı (config yok,
   script yok, tek test yok). İlk 38 test: `notifyTypes` (cue/badge eşlemesi + `task`→`board` legacy
   alias'ı) ve `recommendations` (öneri kural motoru: token-conflict önceliği, sqz/rtk hook tespiti,
   `shellOutputCompression` explicit-tercih saygısı, cbm add/enable ayrımı, kart sırası). `ci.yml`'de
   ayrı bir `frontend` job'ı olarak `npm test` + `npm run build` koşar (Go gate'ine paralel).

Ek olarak `internal/tools/readtracker_test.go`: dosya-tazelik guard'ının unit sözleşmesi — nil tracker
no-op, mtime-değişti-içerik-aynı (guard tripmemeli) vs içerik-değişti-mtime-aynı (tripmeli), never-read
ile stale hata mesajlarının ayrışması, `recordWritten` sonrası ardışık yazım, path-scope, eşzamanlılık.
Araç seviyesindeki happy-path'ler zaten `builtin_fs_test.go`/`builtin_patch_test.go`'daydı.

Durum: **937 test yeşil, 0 fail, 11 skip** (harici araç/ağ geçitli) + 38 frontend testi.

Bilinen kalan boşluklar (öncelik sırasıyla): `internal/api` %17 — `chat_stream.go` (37KB),
`chat_control.go` (27KB), `session_context.go` (26KB), `session_stream.go` testsiz;
`internal/workspace` %4.3 (`manager.go` 23KB); `internal/app` %15.5.

## Flow: async spawn/join + subflow await-propagasyonu ✅ (2026-07-27)

İki yeni yürütme yeteneği (temiz kurulum; `flow.go` tek elden). **Async spawn/join:** `spawn`
node child flow'ları bloklamadan başlatır (`State.Spawned`), `join` node bariyer olarak block-poll
ile bekleyip çıktıları birleştirir; interaktif çocuk join'i fail eder → hep sonlanır. **Subflow
await-propagasyonu:** subflow çocuğu `await-input`'a düşerse parent da askıya alınır (`State.SubflowRun`),
parent'a input verilince child sync resume edilir. `AsyncFlowRunner` + `SuspendableChildFlowRunner`
arayüzleri; frontend palet "Spawn"/"Join" + inspector. **Genişletme:** join'e `joinTimeoutSec` +
`joinPartial` (kısmi mod: fail/suspend/timeout çocuğu düşür), ve subflow/spawn/join için **flow-picker
UI** (id metni yerine seçici). Ayrıca continuation turn'lerinin (scheduler `deliverPrompt`/`deliverWake`)
ortak reply/error kaydı `recordAssistantReply`/`recordTurnError`'a çıkarıldı (god-function'dan bilinçli
kaçınıldı). **Devam:** paylaşım 5 continuation sitesine yayıldı (`agentmsg`/`spawn`/`coordination` +
`recordAssistantMessage` çekirdeği); gallery'ye **Async Fan-out (Spawn/Join)** örnek şablonu (companion
alt-akışlarla runnable); **join canlı ilerleme** (`onProgress` → `progress` NodeEvent → RunView "N/M").
Backend 1029 test yeşil; canlı E2E hepsi (spawn/join, propagasyon, partial-drop, SSE progress `0/2→2/2`).
Detay: [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md).

## Claude Opus 5 model desteği ✅ (2026-07-27)

Anthropic **Opus 5** (`claude-opus-5`, 24 Tem 2026; 1M bağlam, Opus fiyatı sabit
$5/$25) katalog + fiyat tablolarına eklendi. Açık giriş gereken 3 yer:
`kind_anthropic.go` katalog (yeni "en yetenekli", Opus 4.8 → "önceki nesil"),
`pricing.go` (anthropic + openrouter tabloları), `kind_openrouter.go`
(`anthropic/claude-opus-5` önerisi). `context_window.go`/`maxoutput.go` model-ailesi
("opus") eşleştiği için 1M pencere + çıktı tavanını otomatik verir; claude-cli `opus`
alias'ı CLI güncellenince otomatik çözülür (değişiklik yok). Frontend model dropdown'ı
API-güdümlü → değişiklik gerekmedi. Varsayılan model değişmedi (anthropic hâlâ Sonnet 5).
Backend `go build` + `internal/providers` 107 test yeşil.

## Flow Start / End node'ları ✅ (2026-07-27)

İlk-sınıf **Start** (zorunlu giriş markeri; per-node "başlangıç işaretle" kalktı) + **End**
(opsiyonel terminal; `Template` çıktıyı şekillendirir, `OutputSchema` nihai çıktıyı JSON-Schema'ya
karşı doğrular → uymuyorsa `failure` = **çıktı sözleşmesi**). Temiz kurulum: geri uyumluluk yok,
eski flow'lar `MigrateAddStart` + `MigrateFlowsStartEnd` ile workspace açılışında migrate edildi;
default flow + gallery/harnesspack templates + frontend `ensureStartNode` yeni formatta. Detay:
[15-FLOW-CANVAS.md](15-FLOW-CANVAS.md). Backend 1004 test yeşil; canlı: start→agent→end +
eski FLW15 migrate doğrulandı.

## Birleşik Run (C+D): await-input keystone + genişletmeler ✅ (2026-07-26)

Session ⇄ flow birleşiminin yürütülebilir çekirdeği ve çevresi. Detay: [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md).

- **`await-input` keystone (durable suspend/resume):** flow bir node'da **durup girdi bekleyebilir**.
  `orchestration.State.WaitingAt` + `db.FlowWaiting` statüsü; suspend `Run`'dan `(st,nil)` ile döner
  (Current park), `MarkFlowRunWaiting` persist; `ClaimWaitingFlowRun` **CAS** çift-resume korur;
  waiting'ler boot-resume dışı (orphan yok). `ResumeWaitingFlow` + `POST /api/flow-runs/{id}/input`
  (409 guard). Girdi `{{last}}` ile devam eder. UI: RunView waiting composer + sarı ring. Canlı
  gerçek-LLM E2E ✅.
- **`LaunchRun` (Faz 3):** tetik-launcher'ların `FlowID?flow:session` dalı tek seam'de
  (`internal/agent/launch.go`, `RunTrigger`/`RunSpec`); `automation.fire`/`fireBoard` + `scheduler.deliverFlow`
  buradan geçer, `fireFlow` silindi. Reuse-continuation launcher'ları (`deliverPrompt`/wake) **tasarımca
  dışında** (continuation ≠ fresh launch).
- **Peer-bridge (#1):** `list_flow_runs` (status='waiting' → bekleyeni bul) + `deliver_flow_input`
  (resume köprüsü) araçları → peer ajan/koordinatör bekleyen flow'u besler.
- **await timeout/GC (#3):** node `TimeoutSec` + 30s sweeper (`StartWaitingFlowSweeper`) deadline
  geçeni CAS-claim + `failure`.
- **`subflow` node (#2):** bir flow başka flow'u baştan sona koşup çıktısını yakalar (kompozisyon);
  `RunChildFlow` + recursion depth guard (5). Canlı E2E ✅.
- Backend 1000 test yeşil; frontend `tsc`+`vite build` yeşil; backend restart + canlı doğrulama.

## Flow motoru: accumulate cache + loop + session↔flow köprüsü ✅ (2026-07-25/26)

Detay: [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md).

- **Accumulate (cache'li bağlam):** `Graph.Accumulate` (flows ekranında default açık) — ardışık agent
  node'ları büyüyen tek konuşma thread'ini paylaşır (`State.Thread` + `ThreadAgentRunner`) → prompt-cache
  düğümler arası; `Node.Fresh` opt-out; paralel copy-on-fork + join sentetik-turn katlama.
- **`loop` node:** `Body`/`LoopNext`/`MaxIters`/`Until` — gövde alt-zincirini yineler (`{{iteration}}`),
  global `maxSteps` frenler.
- **Session → Flow:** chat header "Akış" toggle → oturumu **tamamlanmış bir koşu** olarak inline RunView'de
  gösterir (flow-run oturumunun gömülü step'leri çok-node'a açılır); node inline çıktı önizlemesi; dikey
  auto-layout.
- **Per-run flow oturumu:** her koşu kendi session'ı (`CreateSession`, `SourceID=flow.ID`).
- **Editörden çalıştır → Koşular tab'ına yönlendir** (editör canvas'ı değişmez).

## Kuyruk mesajı iptal edilince gözlemci kapanışı ✅ (2026-07-25)

`/chat` + `/chat/stream` kendi turunun terminal olayını bekler; mesaj çalışmadan başka
pencereden (`DELETE .../queue/{id}` veya `.../queue`) silinirse terminal hiç gelmez →
gözlemci asılırdı. `queue_update` payload'ına `inflightClientMsgId` eklendi (dispatched↔
cancelled ayrımı); gözlemci mesajını kuyrukta **canlı gördükten sonra** kaybolursa iptal
sayar → stream `error{reason:"cancelled"}`, non-stream **409** döner. Enqueue-öncesi yarışa
karşı "önce canlı görülmeli" guard'ı. Test `TestQueueHasMsg`; app+api+agent **348 test** yeşil.
Detay: `_Docs/58`.

## Tek-instance DataDir kilidi (multi-process guard) ✅ (2026-07-25)

Doc 58'in tüm serileştirmesi (hub/inbox-worker/coordSlot/interaction-CAS/scheduler)
**tek process belleğinde**; aynı store'a iki server process = cross-process eşzamanlı
tur + çift schedule + boot çift re-dispatch + entity ezmesi. Masaüstü `:0` portu
bağladığından double-launch'ta port çakışması yok, store kilidi de yoktu → iki birincil
aynı store'u bozardı. **Sert ret eklendi:** `app.Bootstrap` store'a girmeden DataDir'de
process-ömürlü exclusive advisory kilit alır (`internal/app/instancelock*.go`; Windows
`CreateFile` share=0, Unix `flock` — yeni bağımlılık yok, OS process çıkışında bırakır →
stale kilit yok). Tutuluysa net hatayla reddeder; connect-only ikincil pencereler
etkilenmez. Test `instancelock_test.go`; app+api+agent **341 test** yeşil. Detay: `_Docs/30`.

## Kuyruk cutover — nadir-senaryo sağlamlaştırması ✅ (2026-07-25)

Cutover sonrası adversarial gözden geçirmede 3 nadir boşluk kapatıldı: (1) **düşen
terminal frame → asılma** — hub fan-out'u terminal `turn_done`'u düşürürse gözlemci
sonsuza bekliyordu → her ping tick'te `drainReplay` ile ring reconcile; (2)
**`handleChat` yanlış/boş yanıt** — ardışık turlarda "son assistant" yarışı → gördüğü
hub `KindReply` payload'ını kullanır (DB fallback sondan geriye tarar); (3) **asılan
wake/scheduled tur → kuyruk head-of-line bloğu** — `deliverWake`/`deliverPrompt` slotu
alıyor ama `withActivityTimeout`'u yoktu → spawn/worker paritesiyle sarıldı. Yeni test
`TestDrainReplayRecoversDroppedTerminal`; agent+api **340 test** yeşil. Detay: `_Docs/58`.

## Legacy `/chat/stream` + `/chat` durable kuyruğa taşındı ✅ (2026-07-25)

Bu iki endpoint (frontend hiçbirini çağırmıyor — yalnız dış otomasyon/eski istemci)
turu **inline** koşup `inbox.json`'a yazmıyordu → mid-turn crash'te mesaj kayboluyor

- kuyruğu atlıyordu. Artık ikisi de serial send-queue'ya enqueue eder (kalıcı,
  crash-recoverable, tek-tur garantili) ve per-session hub'ı gözler: `handleChatStream`
  hub'ı legacy SSE frame şekline çevirip relay eder (streaming sözleşmesi korunur),
  `handleChat` terminal olayı bekleyip kalıcı yanıtı DB'den döndürür. Korelasyon: taze
  `clientMsgId` → `runChatTurn` + kuyruk bariyerleri terminal hub olaylarına (turn_done/
  turn_error) damgalar (öndeki turun terminal'i erken kapatmaz); slow-drop'a karşı
  `Replay` gap-fill. `failTurn` artık hub'a da turn_error yayınlıyor (önceden yalnız SSE
  sink → hub istemcileri reload'da görüyordu). Yeni dosya `internal/api/chat_queue.go`
  (iki handler + relay helper'ları); eski inline gövdeler `chat.go`/`chat_stream.go`'dan
  silindi (`inflightRecorder` testte kullanıldığı için korundu). `go build`/`go vet`
  temiz, agent+api **339 test** yeşil. Detay: `_Docs/58`.

## Per-session tur kilidi TÜM oturumlara genelleştirildi ✅ (2026-07-25)

**Sorun:** "tek oturumda tek tur" garantisi yalnız koordinatör oturumlarındaydı;
düz oturumda `claimTurnSlotIfCoordinator` no-op dönüyordu. Kullanıcı chat yazarken
(inbox worker) aynı oturuma zamanlanmış **wake** / **peer teslimi** düşerse ya da
legacy `/chat/stream`·`/chat` inbox worker koşarken çağrılırsa **eşzamanlı iki tur**
açılabiliyordu (doc 58'in kapatmayı hedeflediği yarış, düz oturumda açıktı).

**Ne yapıldı:** koordinatör-only geçit kaldırıldı, `coordSlot` her oturumun tek tur
kilidi oldu. `BeginCoordinatorUserTurn`→`BeginSessionUserTurn` (chat_stream.go +
chat.go koşulsuz claim); `claimTurnSlotIfCoordinator`→`claimSessionTurnSlot` (her
zaman claim, resetCap=false) → wake/scheduled/peer (scheduler.go + agentmsg.go) aynı
slotta serileşir. Düşük seviye `claimCoordinatorSlot` korundu. Testler:
`TestClaimSessionTurnSlot` (yeniden yazıldı) + yeni
`TestPlainSessionSerializesConcurrentTurns`. Ayrıca `runWorker` +
`runSpawn` kendi oturum slotlarını almıyordu ve `SendToWorker` eşzamanlı turu yalnız
`isSessionActive` (UI göstergesi, kilit değil → TOCTOU) ile kontrol ediyordu; ikisi
de `claimSessionTurnSlot`'a bağlandı → worker/spawn turları da her turla serileşir.
`go build`/`go vet` temiz, agent+api **333 test** yeşil. Detay: `_Docs/47` §13, `_Docs/58`.

## Workspace oluşturmada "veri klasörü" → "proje dizini" ✅ (2026-07-25)

**İstek:** Workspace oluştururken "Veri klasörü" seçimi kalksın (hep app default kullanılsın);
yerine opsiyonel "Proje dizini (path)" girilebilsin.

**Ne yapıldı:**

- **Backend:** `createWorkspaceReq.Path` → `ProjectDir`. Data dir daima app varsayılanı
  (`workspaces.Create(name, "", "")`). `ProjectDir` doluysa identity patch'iyle workspace'in
  `DefaultWorkingDir`'ine yazılır.
- **Frontend:** `WorkspaceCreateModal` — "Veri klasörü" alanı "Proje dizini (path)" oldu
  (state `path`→`projectDir`, Gözat/pickFolder korundu). `NewWorkspaceData.path`→`projectDir`,
  `api.createWorkspace` body alanı da `projectDir`.
- **Test:** `go build` + `go test ./internal/api ./internal/workspace` (120) yeşil, `tsc --noEmit` temiz.

## Soyut "varsayılan sağlayıcı/model/ajan" kaldırıldı ✅ (2026-07-25)

**İstek:** Workspace ayarlarında "varsayılan model/provider/ajan" diye bir özellik olmasın;
sağlayıcı/ajan net belirtilsin ya da mevcut ajanlardan ilki seçilsin. Aynısı İçgörü
ekranındaki ajan seçiminde de geçerli olsun.

**Ne yapıldı:**

- **Workspace ayarları:** `WSSettings.DefaultProvider/DefaultModel` (struct + patch + DTO +
  frontend `WorkspaceSettings`/patch) tamamen kaldırıldı; `WorkspacePanel`'deki
  "Varsayılan sağlayıcı + model" bloğu + banner metni silindi (`ProviderModelSelect` importu da).
- **App-geneli varsayılan da kaldırıldı:** `settings.Settings.DefaultProvider/DefaultModel`
  (struct + DTO + patch + `Default()` + normalize + `Validate`) ve `ProvidersPanel`'deki
  "Varsayılan sağlayıcı + model" kontrolü + `SettingsPanel` save payload'ı + `AppSettings`
  tipi silindi.
- **Registry temizliği:** `providers.Registry.defaultModel` alanı + `SetDefaultModel` metodu +
  `ResolvedConfig.Model` alanı tamamen söküldü (`server.go` çağrısı da). claude-cli artık modelini
  ajanın `req.Model`'inden alır (`kind_claudecli` `NewClaudeCLI(..., "", ...)`), boşsa CLI kendi
  oturum varsayılanını kullanır.
- **Yeni ajan çözümü:** `handleCreateAgent` boş sağlayıcı/modeli workspace'in **ilk (en yeni)
  ajanından** miras alır (`firstAgentProviderModel`) → `claude-cli` + provider yerleşik modeli.
  Şablon tohumlama (`defaultProviderModel`) → `claude-cli`, model "". `handleTestProvider`
  boş modeli provider'ın kendi varsayılanına bırakır.
- **İçgörü:** `SettingsTab` AgentPicker artık `clearable` değil; açılışta ajan seçili değilse
  **ilk ajan** otomatik seçilir. "Varsayılan (…)" placeholder'ı kalktı.
- **Migration:** `ws-settings.json` yüklemede eski `defaultProvider/defaultModel` anahtarları
  görülürse dosya bir kez temiz yeniden yazılır (`loadSettings` → `saveSettings`). App
  `settings.json`'daki dead anahtarlar unmarshal'da yok sayılır, sonraki kayıtta düşer.
- **config_validate:** `settings.json` için beklenen anahtar `defaultProvider` → `defaultPermissionMode`.
- **Test:** `go vet ./...` temiz, `go test ./...` (974) yeşil, `tsc --noEmit` temiz.

## Her workspace'e varsayılan flow tohumlama ✅ (2026-07-25)

**İstek:** Her workspace'te kullanılabilecek bir flow; yeni workspace'te otomatik eklensin,
istenirse silinebilsin, eski workspace'lere de eklensin, şablonlar arasına da eklensin.

**Ne yapıldı:**

- **Model:** `db.Flow` += `Seed string` (shipped default işaretçisi).
- **Seed mantığı:** `internal/agent/flow_defaults.go` — `defaultFlows` (tek doğruluk kaynağı;
  "Yanıtla & Doğrula" akışı) + `EnsureDefaultFlows(ctx, db, storeDir)`. İdempotent
  (DB'de aynı `Seed` varsa atlar) + **silme kalıcı** (store kökünde `.seeded-flows.json`
  ledger; silinen tohum geri gelmez). Agent node'lara ilk ajan atanır (yoksa boş).
- **Tetikleme:** `workspace.Manager.open()` her açılışta çağırır → yeni workspace kurulumda,
  eski workspace'ler bir sonraki başlangıçta backfill.
- **Şablon galerisi:** `flowTemplates.ts` += `default-starter` (aynı graph).
- **Test:** `flow_defaults_test.go` (4 vaka: seed/idempotens, silme kalıcılığı, ilk-ajan
  ataması, ajansız seed). `go build`/`vet` + `tsc -b` yeşil.

## Worker oturumunda "Koordinatöre dön" butonu ✅ (2026-07-24)

**İstek:** Worker oturumundan koordinatör oturumuna dönme kısayolu.

**Ne yapıldı (yalnız frontend):** `SessionInfo.coordinatorSessionId` back-link'i
`SessionDetailPanel`'den `CoordinatorSection`'a geçirildi; worker branch'indeki
pasif notun altına ArrowLeft ikonlu "Koordinatöre dön" butonu eklendi →
`onSelectSession(coordinatorSessionId)` ile koordinatör oturumunu açar. Yalnız
`onSelectSession` + back-link mevcutsa görünür. `tsc` yeşil. Detay `_Docs\47`.

## Koordinasyon roster'ı: worker satırına tıklayınca oturumu açılır ✅ (2026-07-24)

**İstek:** Oturum bilgisi ekranındaki worker'a tıklayınca o worker'ın oturumuna
gitsin.

**Ne yapıldı (yalnız frontend):** Her worker zaten kendi oturumu (`w.sessionId`).
`SessionDetailPanel` mevcut `onSelectSession`'ı `CoordinatorSection`'a zincirledi;
roster satırı `onSelectSession` verildiğinde tıklanabilir butona dönüşüp
`onSelectSession(w.sessionId)` ile o oturumu açar (hover accent kenarlık). Prop
yoksa satır eski düz div olarak kalır. `tsc` yeşil. Detay `_Docs\47`.

## Composer: dar ekranda tur-ayarı butonları toggle ile gizlenir ✅ (2026-07-24)

**İstek:** Sohbet input alanı dikey/dar ekrana geçince Düşünme seviyesi, İzin modu
ve Çalışma dizini butonları bir toggle ile gösterilip gizlenebilsin.

**Ne yapıldı (yalnız frontend, `Composer.tsx`):**

- Üç per-turn kontrolü (`ComposerPicker` düşünme + `ComposerPicker` izin +
  `WorkDirBadge`) `display:contents` sarmalayıcıya alındı → toolbar gap'i bozulmaz.
  Sarmalayıcı dar ekranda `hidden`, `md:contents` ile **`md:`'den itibaren daima
  görünür**.
- Yeni `SlidersHorizontal` toggle butonu (`md:hidden`, yalnız dar ekran) kontrolleri
  aç/kapat yapar; açıkken accent kenarlık. Tercih `localStorage`
  (`tionharness.composerControlsOpen`) ile kalıcı; varsayılan gizli. `tsc` yeşil.
- **2026-08-01:** 🔧 **Araçlar** butonu da bu gruba alındı (dördüncü üye) —
  toggle tooltip'i `düşünme · izin · çalışma dizini · araçlar` oldu. Grup
  kapanınca açık bir araç paneli de kapanır: ⚙ toggle'ında
  `data-tool-access-toggle` yok, dolayısıyla panelin dışarı-tıklama kapanışına
  takılır — ek kod gerekmedi.

## Görev listesi tamamlandıktan sonra kalıcı kullanıcı kapatması ✅ (2026-08-29)

- `latestTodos`, tamamlanmış listeyi sonraki kullanıcı mesajından sonra artık
  otomatik gizlemiyor; en yeni `todo_write` oturumun güncel listesi olarak kalıyor.
- Composer üstü `TodoPanel` bitmemişken kapatılamaz. Tümü tamamlanınca erişilebilir
  etiketli X görünür ve katla/aç tıklamasından bağımsız çalışır.
- Kapatma `localStorage`'da session ID + todo occurrence ID + liste imzası bazında
  kalıcıdır: aynı occurrence reload sonrası gizli kalır; aynı session'da aynı içerikle
  yeni bir `todo_write` occurrence'ı eski kapatma kaydından etkilenmeden görünür. Farklı
  liste ve başka session da görünürdür. Kapatma geçmişi en yeni 100 kayıtla sınırlıdır;
  limit aşılınca en eski kayıtlar atılır. Bozuk/engelli storage güvenli biçimde görünür
  panel davranışına düşer. Inline `TodoCard` değişmedi.
- Vitest kapsamı: tamamlanmış listenin sonraki kullanıcı turunda kalması, en yeni
  liste, legacy `todo_write`, session/liste izolasyonu ve bozuk storage.

## Görev listesi UX: bilgi panelinden kaldırıldı + TodoPanel başlangıçta kapatılamaz ✅ (2026-07-24)

**İstek:** Sohbet bilgisi panelinde görev listesi görünmesin; sohbet sırasında
açılan görev listesi küçültülmüş başlasın ama kapatılamasın.

**Ne yapıldı (yalnız frontend):**

- **Bilgi paneli:** `SessionDetailPanel.tsx`'ten `ProgressCard` (Görev Listesi)
  render'ı + `progress` state + `sessionProgress` fetch effect'i (`executions`
  sinyaliyle tazeleme) + ilgili importlar kaldırıldı. `SessionProgressCard.tsx`
  artık kullanılmayan ölü bileşen; API/tip korunur.
- **`TodoPanel.tsx` (tarihsel, 2026-08-29'da güncellendi):** varsayılan
  **küçültülmüş** (`open=false`) başlar. Bu değişiklikte eski genel dismiss mantığı
  kaldırılmıştı. Güncel davranışta panel bitmemişken kapatılamaz; yalnız tüm maddeler
  tamamlandığında session + liste imzalı X ile kapatılabilir. Detay `_Docs\36`.

## 👍/👎 geri bildirimi tur bağlamına enjekte ✅ (2026-07-24)

Mesaj puanları saklanıyordu (`db.MessageFeedback`, `session.jsonl`) ama hiç geri
okunmuyordu — puan vermek sonraki cevabı değiştirmiyordu. Yeni
`internal/api/chat_feedback_summary.go` → `recentFeedbackBlock`: en yeni **6**
puanlı turdan kompakt bir `<user_feedback>` bloğu üretir ve **volatile dinamik
suffix'e** enjekte eder — history'ye katlanmaz, puan vermek rolling prompt
cache'i **bozamaz**. Tüm konuşma taranır (puan seyrek ama uzun ömürlü),
`rating: 0` (geri alınan) atlanır; 5 `composeTurnRequest` çağıranının hepsine
bağlı (stream, blocking, btw, wake, context-preview). Test:
`chat_feedback_summary_test.go`. Detay `_Docs\07`.

## Chat yüzey rötuşları: mesaj altbilgisi + bloke-tur uyarısı + canlı panel ✅ (2026-07-24)

Üç UI düzeltmesi: **(1)** Per-mesaj meta + aksiyonlar balon altında **footer
satırına** taşındı — solda pasif meta (saat/süre/model/token), sağda her zaman
görünür aksiyon çipleri (🔊 oku, 👍/👎, yeniden dene, sil; eskiden hover-only
ghost'tu). Ortak stil `chat/messageActions.ts`. **(2)** Tur kullanıcıya bloke
olunca (ask_user / izin / plan) **ayrı chime + masaüstü toast** — interaction
id başına bir kez, reconnect/replay güvenli (`chatStreamHub` + `sounds.ts`).
**(3)** Açık Oturum Bilgisi paneli mesaj girişinde canlı yenilenir —
`bumpMeter` nonce'u bağlanmamıştı, panel mount anında donuyordu; Faz 3 queue
cutover'dan kalan 11 ölü `SendContext` alanı da temizlendi. Ek: oturum
başlığındaki AI-başlık + yeniden adlandırma butonları artık hover'sız her zaman
görünür (`SessionTitleBlock`). Detay `_Docs\07`.

## Workspace listesinde çapraz-workspace "çalışıyor" nabzı ✅ (2026-07-24)

Session "devam ediyor/tamamlandı" göstergeleri sohbet listesinde (yeşil nabız +
`StatusPill`) ve navbar per-view `busy` noktasında vardı, ama bunlar yalnız
**aktif** workspace-scoped (`/api/activity`). Workspace **listesinde** aktif
olmayan workspace'lerde canlı koşu görünmüyordu. Yeni çapraz-workspace sinyal:
backend `GET /api/workspaces/activity` (`handleWorkspacesActivity` + `workspaceRunning`
— her workspace için tek `running` bayrağı; global route, aktif ws gerektirmez),
frontend `useWorkspaceActivity` hook (4sn poll + **iki SSE sinyali**: aktif ws için
`executions`, çapraz-ws için yeni `workspace-activity` — `useAppEvents` çapraz-ws
dalında run-lifecycle olaylarında `bumpWorkspaceActivityForEvent` ile bumlar, aktif
olmayan ws'te başlayan/biten koşu **anlık** yansır → `busyWorkspaceIds`).
`WorkspaceSwitcher`/`MobileWorkspaceButton` her
satırda yeşil nabız (oturum "yazıyor…" görseliyle aynı, `unread`'in önünde);
aktif olmayan bir workspace çalışıyorsa switcher trigger + collapsed rail ikonu
accent noktayı pulse eder. "Tamamlandı" ayrı sinyal değil — mevcut SSE `unread`
accent noktası. `App.tsx` → `NavRail`/`MobileNavBar` boyunca `busyWorkspaceIds`
taşındı. `go vet`, `go test ./internal/api`, `npx tsc --noEmit` yeşil. Detay `_Docs\29`.

## Pano otomasyonlarında sıralama + tek sahip (çift-tetik yarışı) ✅ (2026-07-24)

**TSK59:** Aynı sütunu izleyen iki pano otomasyonu (Kart Sınıflandırıcı AUT7 ve
Board Planner AUT4, ikisi de `move → todo`) aynı olayda ateşliyor, üstelik sıra
`ListEnabledAutomations`'ın map kaynaklı dönüş düzenine bağlı olduğu için
**belirsiz** kalıyordu → iki otonom oturum aynı kart üzerinde yarışıyordu.
Çakışma o güne dek Planner elle kapatılarak önlenmişti. `Automation`'a iki alan
eklendi: **`BoardPriority`** (aynı olaya uyanlar arasında ateşleme sırası, küçük
önce; eşitlikte `ID` ile stabil) ve **`BoardExclusive`** (eşleşen olayı tek
başına sahiplenir, diğer tüm eşleşmeler bastırılır = "sütun başına tek sahip").
Karar mantığı ayrı dosyada — `internal/agent/automation_board_order.go`
`selectBoardAutomations`; `OnBoardChange` artık eşleşmeleri doğrudan gezmek
yerine bu seçiciden geçirip **sırayla** ateşliyor (ikinci kural birincinin
bıraktığı kart durumunu görür). Alanlar db/API/araç/UI boyunca taşındı; API ve
araç tarafında **pointer** oldukları için kısmi patch saklı değeri korur. UI:
`BoardTriggerFields` içine "Sıra" + "Tek sahip" kontrolleri (hem oluşturma formu
hem satır-içi editör), liste satırında `🔒 tek sahip` / `sıra N` rozetleri.
7 yeni birim testi (sıralama, ID tie-break, exclusive bastırma, exclusive
kazanan, sütun-içi izolasyon, filtreleme, boş küme) + `go build/vet`,
`go test ./internal/...`, `npx tsc -b`, `npm run build` yeşil. Detay `_Docs\46`.

## PromptEditor içerik-boyutlu yükseklik (autoSize) ✅ (2026-07-23)

Promptlar & Dosyalar ekranındaki editörler sabit 26rem'lik kutu yerine **içeriğe
göre boyutlanıyor**: `PromptEditor`'a opsiyonel `autoSize` + `autoSizeMax`
(vars. 320px, min 72px) prop'ları eklendi. Edit görünümünde textarea scrollHeight
ölçümüyle, split'te satır yüksekliği imperatif set edilerek (iki pane eşit kalır),
tek-pane önizlemede max-height + shrink-wrap ile çalışır; tavana ulaşan içerik
içten kaydırılır, tam ekran etkilenmez. **Uygulama-geneli VARSAYILAN** (aynı gün
ikinci adım): `autoSize` default `true` — tüm PromptEditor yüzeyleri (workspace
promptları, ajan soul/identity, skill gövdesi `autoSizeMax=560`, flow node
promptu, profil notları, oturum hedefi) kurala uyar; taban `rows`-farkındalı
(`max(72, rows*20+18)` — 12 satırlık authoring alanı boşken 72px'e çökmez);
`autoSize={false}` ile eski sabit kutuya dönülebilir. Görsel doğrulama: kısa
promptlar 72–132px, uzunlar 320px tavanında; ajan formu 416→98/72px; skill
gövdesi içerikle 518px.
`tsc` + prod build + gömülü binary üzerinde Playwright kontrolü yeşil.

## Sesli giriş + sesli okuma (STT/TTS) ✅ (2026-07-23→24)

Composer'a **dikte** geldi: `MicButton` + `useSpeechToText` (Web Speech API,
Chromium-bağımlı; API yoksa gizli), mikrofon dili Ayarlar ▸ **Ses**'ten, aktif
dil tooltip'te. **UI sesleri** merkezi `shared/lib/sounds.ts` (mic blip + yanıt
bitiş chime'ı + onay-bekleyen cue). **TTS** (`shared/lib/tts.ts` + balonda 🔊 +
"Yanıtları sesli oku"): kod/tablo/link ayıklanır; iki motor — tarayıcı
`speechSynthesis` veya **sunucu Piper CLI** (`internal/tts` + `/api/tts`;
telefon/thin client'ta da çalar, yoksa tarayıcıya düşer). **STT'de de iki
motor:** tarayıcı Web Speech veya **sunucu whisper.cpp** (`internal/stt` +
ffmpeg + `/api/stt`; offline/Türkçe, yoksa Web Speech'e düşer). Tüm ses
ayarları tek alt-sayfada: Ayarlar ▸ **Ses** = `SoundPanel` (efektler + STT +
TTS). Detay `_Docs\07`.

## İçgörü kokpiti: kanban Bulgular + reset + Dersler sekmesi + büyüme sınırı ✅ (2026-07-23)

Dört adım: **(1)** Bulgular sekmesi görev panosu gibi **kanban** oldu — 5 sabit
yaşam-döngüsü sütunu, sürükle = statü, kart tıkla = `FindingModal` detay
popup'ı, Ctrl/Shift çoklu seçim + toplu bar (`useMultiSelect`/`SelectionBar`
reuse); bulgu **silme** eklendi (`FindingStore.Delete` +
`DELETE /api/insight/findings/{id}`). **(2)** **Insight reset, iki mod**
(`internal/insight/reset.go` + `POST /api/insight/reset` + Ayarlar'da tehlike
bölgesi): varsayılan ledger'ı korur (eski oturumlar yeniden taranmaz),
`deep=true` sıfırdan (uyarılı). **(3)** **Dersler sekmesi**: reaktif lesson
tarafı (toggle + `LessonsList`) kokpitte — Insight tek öz-iyileştirme ekranı.
**(4)** Tema hizalaması (tanımsız `--color-text-muted` → `--color-text-dim`) +
`FindingModal` opak yüzey. Ayrıca **birikme önleme**: `ledger.jsonl` her
taramada `Compact()`, `runs.jsonl` 1000 kayıtla cap'li. Detay `_Docs\60`.

## Araç güvenilirlik düzeltmeleri (Insight bulgularından) ✅ (2026-07-23)

Insight taramasının yüzeye çıkardığı, koda karşı doğrulanmış dört düzeltme:
**(1) CRLF eşleşmesi** — `Edit`/`apply_patch` çok-satırlı `old_string`'i CRLF
(Windows) dosyada hiç yakalayamıyordu (Read LF verir, disk CRLF); Edit iğneyi
dosyanın satır sonuna hizalar, apply_patch hunk eşleşmesinde `\r` soyar, ikisi
de yazarken CRLF'i korur (`builtin_crlf_test.go`). **(2) `activate_tools`
namespace toleransı** — uydurma `mcp__server__` önekli ad, son `__`-segmenti
katalogda tekil ise çözülür (belirsizse unknown kalır). **(3) bg-shell şema
kapısı** — `Bash`/`PowerShell` şeması `run_in_background`'ı yalnız arka-plan
shell yöneticisi bağlıyken ilan eder (reddedeceğini teklif etme). **(4)
Artifact yol hatası** — çözülmeyen mutlak yol için hata artık "MCP sunucusu
container/uzak host yolu döndürmüş olabilir; içerik döndürün" ipucunu verir
(`artifact_content.go`). Detay `_Docs\19` (2). Testler:
`builtin_crlf_test.go`, `builtin_activate_ns_test.go`,
`builtin_shell_bg_gate_test.go`, `artifact_media_test.go`.

Uygulamaya dağılmış 15 gömülü LLM promptu (summary/title/compact + btw×2, lesson,
insight-analyzer, auto-continue, handoff, continuation, coordinator, subagent×4)
yeni **leaf paket `internal/prompts`**'ta toplandı: default'lar `defaults/*.md`
(`//go:embed`), her anahtar Spec metadata'lı (Türkçe label/hint, zorunlu
`{{yerTutucu}}` listesi, `EpochAffecting`). Çözümleme her yerde tek disiplin:
workspace `config/prompts/<key>.md` override → gömülü default; boş/eksik-yer-tutuculu
dosya sessizce default'a düşer (eski compact-%s guard'ının genellenmişi).
Konumsal `%s` → adlandırılmış `{{...}}` geçişi yapıldı (legacy iki-%s compact
override'ı okuma anında otomatik dönüştürülür). **Seed politikası değişti:**
prompt default'ları artık dosya olarak seed edilmez; eski default-aynısı seed
artıkları temizlenir → default iyileştirmeleri edit'lenmemiş workspace'lere
otomatik ulaşır. Subagent allowlist'leri bilinçli olarak kodda kaldı (güvenlik
sözleşmesi); btw araçsızlığı yapısal zorlamada. **Prompt izi:** `WithPromptTrace`
→ `debug.jsonl` `llm_call` olaylarına `promptKey`+`promptHash` (düzenlenmiş
prompt ≠ default hash'i → etki ölçülebilir). API: `workspace-config` DTO'suna
`promptMeta`; UI: Promptlar & Dosyalar ekranı registry-güdümlü ("özelleştirildi",
"yeni oturumlarda etkili" epoch rozeti, eksik-yer-tutucu uyarısı). Drift guard:
`TestRegistryConsistency` (yetim dosya/anahtar = test kırılır). Detay `61`.
`go build`+`vet`+ilgili paket testleri + frontend `tsc` yeşil.

## claude-cli hook matcher'ı: bridged-shell genişletme (tüm mevcut workspace'lere uygulandı) ✅ (2026-07-14)

Bir önceki (2026-07-13) regex-çevirisi düzeltmesinin tamamlayıcısı. **Kalan boşluk:** CLI'da
built-in shell açıkken araç adı **köprülü** görünür (`mcp__tionharness_interaction__PowerShell`),
düz `PowerShell` değil. Yani matcher'ı yalnız `Bash,PowerShell` olan hook'lar (regex-çevirisi
sonrası `^(Bash|PowerShell)$` bile) köprülü ismi kapsamadığından CLI'da **hâlâ ateşlenmezdi**.
Tüm workspace'ler tarandı: WS1/WS5/WS10'un enabled sqz hook'ları matcher'da köprülü isimleri
zaten taşıyordu (elle eklenmiş — çalışıyordu); **WS15'in enabled sqz hook'u yalnız
`Bash,PowerShell`** taşıyordu → açık kurbandı.

**Çözüm (kod, evrensel — veri düzenlemesi YOK):** `cliMatcherRegex` artık matcher'daki
`Bash`/`PowerShell` alternatiflerine köprülü formu (`interactionToolPrefix+ad`) **otomatik
ekler** (deduplu). Düz `Bash,PowerShell` → `^(Bash|mcp__tionharness_interaction__Bash|PowerShell|
mcp__tionharness_interaction__PowerShell)$`. Böylece WS15 + gelecekteki her workspace + tek-tık
"Bağla" şablonu CLI'da veri düzenlemeden ateşlenir; zaten köprülü ismi olanlar deduple aynı
kalır. Native yol etkilenmez (olmayan araç zaten eşleşmez).

Test: `climcp_matcher_test.go` güncellendi (WS15-şekli `Bash,PowerShell` düz matcher köprülü
tools'a ateşler; superset'e uymaz; entegrasyon: writeCLISettings çıktısı genişletilmiş regex).
`go build`+`vet`+`go test ./internal/agent` yeşil (200).

**Canlı E2E (uçtan uca kanıt):** WS15'e geçici bir marker PreToolUse hook'u (matcher düz
`PowerShell`) eklendi, AGT1'e (claude-cli) gerçek bir PowerShell turu tetiklendi. Ajan cevabı:
hook **`mcp__tionharness_interaction__PowerShell` çağrısında ateşlendi** → düz `PowerShell`
matcher'ı köprülü CLI aracına eşleşti (fix'ten önce eşleşmezdi). **Bonus keşif:** Claude Code
PreToolUse hook'larını Windows'ta **bash/sh ile** çalıştırıyor (TionHarness native yol PowerShell
ile) — marker ham-PS sözdizimindeydi, bash altında `syntax error` verdi. **Etki:** sqz hook'u
`powershell -NoProfile … -File sqz-bridge-hook.ps1` (bash-geçerli) olduğu için ateşlendiğinde
sorunsuz çalışır; ama **ham-PS sözdizimli rtk hook'ları** (`$j=[Console]::In.ReadToEnd()|…`,
şu an her workspace'te DISABLED) CLI'da bash altında kırılır — etkinleştirilirse sqz gibi
`powershell -File` sarmalayıcısına çevrilmeli. Marker+test-session temizlendi.

## claude-cli hook matcher'ı: virgül-glob → regex çevirisi (sqz/rtk CLI'da sessiz çalışmıyordu) ✅ (2026-07-13)

**Kök neden (bir oturum incelemesinde yakalandı):** TionHarness'in **native** hook matcher'ı
(`hookMatches`) virgül-ayrık **filepath.Match glob listesi** (`Bash,PowerShell`,
tam-eşleşme). Ama `climcp.go writeCLISettings` matcher'ı claude-cli'ın `--settings`'ine
**verbatim** yazıyordu. **Claude Code matcher'ı REGEX sayar** (alternation `|`, virgül
literal, ankraj yok) → `Bash,PowerShell,mcp__tionharness_interaction__PowerShell,…` virgüller
dahil o literal diziyi arar, hiçbir tekil araç adına uymaz → **hook claude-cli turlarında
SESSİZCE hiç ateşlenmez** (ajanların çoğunun kullandığı yol). Sonuç: virgüllü matcher'lı
sqz/rtk optimizerları CLI ajanlarında ölüydü — bir WS10 oturumunda 23 PowerShell/git komutu
ham çalışmış, sqz sıfır optimizasyon yapmış (komut+çıktı ham, "sqz" 0 kez).

**Çözüm:** yeni `internal/agent/climcp_matcher.go` — `cliMatcherRegex` matcher'ı iki lehçe
arası köprüler: virgülle böl → her glob'u regex'e çevir (`*`→`.*`, `?`→`.`, diğer metachar'lar
escape) → `|` ile birleştir → `^…$` ankraj (native `filepath.Match`'in tam-eşleşme semantiğini
aynala, `Bash` artık `BashOutput`'a uymasın). `writeCLISettings` artık `cliMatcherRegex(h.Matcher)`
yazıyor. Boş matcher boş kalır (iki motor da "tüm araçlar" sayar). Native yol değişmedi.

Test: `climcp_matcher_test.go` — dönüştürme tablosu + davranışsal ateşleme (bridged
`mcp__…__PowerShell`'e uyar, superset'e uymaz) + **entegrasyon** (`writeCLISettings` çıktısında
dönüştürülmüş regex, verbatim virgül-liste YOK). `go build ./...` + `go vet` + `go test
./internal/agent` yeşil (203). Backend rebuild+restart edildi; artık her CLI turunda doğru
matcher yazılıyor → sqz/rtk gerçekten ateşlenir.

## Loglama revizyonu: kaynak alanları + SSE canlı akış + Loglar ekranı yükseltmesi ✅ (2026-07-13)

- **Entry modeli:** `logbuf.Entry`'ye birinci sınıf **`component` / `session` /
  `agent` / `workspace`** alanları eklendi — handler aynı-isimli slog attr'larını
  bu alanlara terfi ettirir (attrs map'inden çıkarır). Kablolama: runtime
  `component=agent`+`workspace=<id>`, scheduler/automation/api/backup/workspace/
  mcp-pool/cli-pool kendi component etiketlerini alır (`runtime.go`,
  `workspace/manager.go`, `app/app.go`).
- **SSE canlı log akışı:** `logbuf.Buffer.SetNotify` → her kayıt `events.Bus`'a
  `Type="log"` olarak yayınlanır (`app.Bootstrap`), `/api/events` `log` SSE
  event'iyle iletir. Loglar ekranı artık 2.5sn poll yerine **canlı tail** yapar
  (30sn'de bir mutabakat poll'u SSE kopmalarını kapatır).
- **API filtreleri:** `GET /api/logs`'a `component`, `session`, `since`, `until`
  (unix ms) parametreleri; `entryMatches` terfi eden alanlarda da arar.
- **`read_logs` aracı:** `q` artık attrs + kaynak alanlarında da arar (önceden
  yalnız mesajdı — API ile tutarsızdı); `component` ve `session` filtreleri eklendi.
- **Gürültü:** `toolloop.go` "tool call" logu INFO→**DEBUG** (yoğun oturumda tur
  başına 100+ satır tamponu domine ediyordu; block/deny INFO'da kaldı).
- **Sessiz bölge logları:** `db.atomicWriteBytes` (mkdir/tmp/rename) ve `nextID`
  best-effort counter yazımı hataları WARN (`component=db`, slog default tee'li
  olduğundan Loglar ekranına düşer); `tools/registry.go` MCP çağrı transport
  hatası WARN (`component=mcp`) — önceden yalnız model'e dönen IsError'dı.
- **Loglar ekranı (UI):** bileşen filtresi (satırdaki rozet tıklanabilir),
  zaman aralığı seçici (15dk/1sa/24sa), 300ms debounce'lu arama + `<mark>`
  vurgusu, satır kopyalama (hover), filtrelenmiş JSON indirme, takip kapalıyken
  "N yeni kayıt — Yenile" rozeti, `content-visibility:auto` ile ucuz
  virtualization, session/agent/workspace alanları ayrı gösterim. Gruplama
  imzasına kaynak alanları dahil edildi (`logGroup.ts`).
- **Testler:** `go build ./...` + 561 test (logbuf/api/tools/db/events/workspace/
  agent/app) ve frontend `npm run build` temiz.
- **Doküman:** `_Docs/12-LOGLAMA.md` güncellendi.

## `archive_sessions` tüm oturum tiplerini süpürebiliyor (`kinds`) ✅ (2026-07-13)

- **Sorun:** Araç `s.Kind != "chat"` ile sabit filtreliyordu → otonom çalıştırmaların
  ürettiği oturumlar (`spawned`, `worker`, `flow`, `task`, `schedule`, `inbox`)
  **hiçbir toplu araçla arşivlenemiyordu**. Board otomasyon zinciri her kart için
  `spawned` oturum üretiyor, bunlar birikiyordu. Geriye tek yol ham REST kalıyordu —
  ki bu workspace scope'unu kaybettiren footgun (bir kez yanlış workspace arşivlendi).
- **Çözüm:** Opsiyonel `kinds` alanı. Geçerli tipler `chat`/`spawned`/`worker`/`flow`/
  `task`/`schedule`/`inbox` + hepsi için `["*"]`. **Verilmezse varsayılan `["chat"]`** →
  mevcut çağrılar birebir aynı davranır (geri uyumluluk kritikti: varsayılan genişletilseydi
  eski "temizlik" çağrıları aniden flow/schedule/inbox'ı da süpürürdü).
- **Nasıl:** Tip mantığı ayrı dosyada — `internal/tools/builtin_sessionkinds.go`
  (`archivableSessionKinds` otoritatif liste, `resolveArchiveKinds` çözümleme +
  trim/lowercase). Bilinmeyen tip **hata döner, sessizce yutulmaz** (`"spawn"` gibi bir
  typo aksi halde "arşivlenecek bir şey yok" diye okunurdu). `dry_run` ve uygulanan
  çıktı her satırda tipi gösterir (`- SES12 · [flow] · "..."`), başlıkta süpürülen tip
  seti yazar. Mevcut korumalar (`currentSessionID` hariç tutma, `exclude`, `idle_days`,
  `title_contains`, `limit` 100) `["*"]` altında da geçerli.
- **Not:** `worker` tipi plandaki 6 tipe ek olarak dahil edildi — coordinator spawn'ları
  (`spawn.go:120`) bu tipi üretiyor, listede olmasa süpürülemez kalırdı.
- **Testler:** `builtin_sessionkinds_test.go` (7 yeni test: chat-only varsayılan,
  seçili tip, `["*"]`, bilinmeyen tip hatası, dry-run tip gösterimi, `["*"]` altında
  guard'lar, helper birim testi). Mevcut 4 test **değiştirilmeden** geçiyor → geri
  uyumluluk kanıtı. `go build ./...`, `go vet ./...`, `go test ./internal/...` temiz.
- **Doküman:** `_Docs/24-SELF-MANAGEMENT.md` satır 60 güncellendi.

## Global skill değişiminde "tüm workspace'leri etkiler" toast'ı ✅ (2026-07-13)

- **Ne:** Skills ekranında **global** tier bir skill'in görünürlüğü, erişimi (shared)
  veya gövdesi değiştiğinde sağ-altta bilgilendirici bir toast çıkar: değişikliğin
  tüm workspace'lerde geçerli olduğunu (workspace override'ları hariç) belirtir.
- **Neden:** Global skill dosyası paylaşımlı global dizinde (`~/.tionharness/skills`) →
  bir workspace'te yapılan düzenleme sessizce diğerlerini de etkiliyordu; kullanıcı
  bunu görmüyordu.
- **Nasıl:** Yeni `shared/components/InfoToast.tsx` (ErrorToast'ın nötr/accent kardeşi,
  `bottom-20 right-4` → error toast'ıyla çakışmaz). `SkillsPanel` üç mutasyon yolunda
  (`toggleAccess`/`setVisibility`/`onEditorSaved`) `active.source === 'global'` ise
  tetikler. Yalnız frontend, self-contained (App error plumbing'e dokunulmadı).
  `npx tsc --noEmit` temiz.

## Yeni default skill `tionharness-terse` (caveman-esinli terse mod) ✅ (2026-07-13)

- **Ne:** Gömülü skill `internal/skills/defaults/tionharness-terse/SKILL.md` (`🪨 TionHarness
Terse Mode`, `access: shared`). Caveman skill'inin (github.com/JuliusBrussee/caveman)
  özünü TionHarness'e uyarlar: dolgu/nezaket/hedge at, teknik özü koru; **kod/komut/yol/
  hata string'leri byte-for-byte aynen**. 3 seviye (`lite`/`full`/`ultra`), dil-koruyan
  (çeviri yok), oturum-sürekli, "normal mode" ile kapanır.
- **Neden:** Yalnız **output token** kısar (caveman ölçümü ort. %65). Prompt-seviyesi,
  opt-in, cache-dostu — TionHarness'in güvenlik/izin/araç davranışını değiştirmez.
- **Nasıl:** `//go:embed defaults` yeni klasörü otomatik seed eder (kod değişikliği yok).
  Caveman'in destructive/security kalıplarında caveman'i kapatan **auto-clarity** kuralı
  TionHarness izin-gate'leriyle hizalı biçimde korundu. `go build/test ./internal/skills/`
  yeşil (37 test). **Not:** çalışan binary için yeniden derleme + restart gerekir.

## `list_sessions` artık tüm kind'leri listeler + `kind` filtresi ✅ (2026-07-13)

- **Ne:** `list_sessions` aracı varsayılan olarak **her kind'i** döndürüyor (chat +
  spawn/worker/flow/task/schedule). Eski sabit `Kind=="chat"` eleme kaldırıldı;
  opsiyonel `kind` argümanı tek kind'e daraltır. Her satır artık `[kind·state]`
  ön ekiyle başlar (ör. `[spawned·active]`).
- **Neden:** Otonom koşular (board otomasyon spawn'ları vb.) UI'nın sidebar +
  SessionsOverview'ında görünürken ajanın `list_sessions`'ında **hiç** görünmüyordu
  → ajan "session listesi eksik geliyor" durumu yaşıyordu (WS5/SES158 teşhisi).
- **Nasıl:** `internal/tools/builtin_sessions.go` — filtre `kindFilter != "" &&
!sessKindMatches(...)` oldu; `sessKindMatches` (legacy `""`→chat) + `sessKindLabel`
  yardımcıları eklendi; şema/açıklama güncellendi. Test `builtin_sessions_test.go`
  yeni davranışa göre yazıldı; self-management SKILL.md + `27-CROSS-SESSION-SEARCH.md`
  senkron. `go build ./...` + `go test ./internal/tools/` yeşil. **Not:** çalışan
  binary'nin görmesi için yeniden derleme + restart gerekir.

## Otomatik artifact yakalama toggle'ı (`AutoCaptureArtifacts`) ✅ (2026-07-13)

> **GEÇERSİZ (2026-08-14):** bu toggle ve arkasındaki yakalama mekanizması tamamen
> kaldırıldı — bkz. dosyanın başındaki "Otomatik artifact yakalama tamamen kaldırıldı".

- **Ne:** Yazılan dosyaların tur sonunda otomatik artifact yapılması artık **workspace
  ayarı** ile açılıp kapanabiliyor ve **varsayılan KAPALI**. Kapalıyken yalnız ajanın
  **bilerek** `create_artifact` çağırdığı içerikler artifact olur — proje kaynak
  dosyalarını düzenlemek Artifacts'ı kirletmez. Açıldığında eski davranış: ajanın
  `Write`/`create_file` ile yazdığı her dosya `captureFileArtifacts` ile Artifacts
  ekranına düşer.
- **Neden:** Kaynak-kodu düzenleyen ajan turları (ör. `ART30.cs`) istenmeden artifact
  üretiyordu. Varsayılan bilerek-artifact'a çekildi; isteyen workspace toggle'ı açar.
- **Nasıl:** `WSSettings.AutoCaptureArtifacts` (varsayılan `false`) + patch/DTO. `chat.go`
  ve `chat_stream.go`'daki 4 `captureFileArtifacts` çağrısı bu ayara koşullandı. Prompt
  yönlendirmesi de takip ediyor: `artifactGuidanceFor(auto)` — kapalıyken ajana "dosya
  yazımı artifact üretmez, deliverable'ı `create_artifact` ile kaydet" der
  (`artifactDeliverableGuidanceManual`). Frontend WorkspacePanel'de "Artifact yakalama"
  toggle'ı.
- **Doğrulama:** `go build`/`go test ./internal/api ./internal/workspace` temiz (default
  testi genişletildi); frontend `tsc --noEmit` temiz. WS10 için ayar `false`'a alındı.

## Shell adımında program ikonunun yanına program adı ✅ (2026-07-13)

- **Ne:** Sohbetteki Bash/PowerShell (ve `transform_data`/`run_code`) araç adımında,
  marka ikonunun yanında artık programın **adı** da yazıyor: `<ikon> (curl) Bash`.
  Böylece adımın hangi programı çalıştırdığı ikonu tanımadan da okunabiliyor.
- **Nasıl:** `shared/lib/programIcons.ts`'e `resolveProgram(command)` eklendi — ilk
  tanınan programın **adını + ikonunu birlikte** döndürür (yalnız-ikon döndüren
  `resolveProgramIcon` kaldırıldı). Yeni `features/chat/CommandProgramTag.tsx` ikonu +
  `(ad)` etiketini render eder; `ActivityCard` eski `CommandProgramIcon` yerine bunu
  kullanır (eski bileşen dosyası silindi).
- **Sınır:** Ad yalnız program **tanındığında** (ikonu varsa) gösterilir; tanınmayan
  komutlarda (PowerShell cmdlet'i, düz `ls`…) davranış eskisi gibi — ne ikon ne ad.
- **Doğrulama:** `go build ./...` + `go vet ./...` + `go test ./internal/...` temiz;
  frontend `npx tsc -b` + `npm run build` temiz.

## Shell adımında komut-programı marka ikonu ✅ (2026-07-12)

- **Ne:** Bir Bash/PowerShell araç adımında, komuttaki programı (git/npm/docker/python
  /go/cargo/kubectl…) tespit edip tool ikonunun yanında küçük **marka SVG'si** gösterir
  (external-agent-oss'taki "hangi programı kullandığına göre ikon" davranışının frontend-only
  uyarlaması).
- **Nasıl:** `shared/lib/commandProgram.ts` komutu parse eder (env/`sudo`/pipe/chain/
  path/`bash -c`+`pwsh -Command` sarmalayıcıları). `shared/lib/programIcons.ts`
  program adını (**~90 alias**, ~75 marka) `simple-icons` glyph'ine eşler; ilk eşleşen
  program kazanır. `features/chat/CommandProgramIcon.tsx` 13px SVG'yi brand-hex + `title`
  ile render eder; eşleşmezse (cmdlet/`ls`…) hiçbir şey.
- **Kapsam:** git/gh, npm/pnpm/yarn/bun/node/deno, tsc/vite/webpack/esbuild/rollup/turbo/
  next/astro/prisma/eslint/prettier/biome/jest/vitest/cypress, python/pip/poetry/uv/pytest/
  conda, go/cargo, docker/podman/kubectl/helm/terraform/pulumi/ansible/vagrant, java/dotnet/
  gradle/maven/ant/scala/kotlin, ruby/php/composer, dart/flutter/swift/elixir/julia/r/perl/lua,
  clang/cmake/make, psql/mysql/mongo/redis/sqlite, nginx, gcloud/vercel/netlify/supabase/
  firebase/wrangler, nvim/vim/brew/pacman/wasmer, curl. (aws/azure/playwright simple-icons'ta yok.)
- **Bonus:** `ActivityCard`'ta yalnız Bash/PowerShell (`input.command`) **ve** `transform_data`
  /`run_code` (`input.language` → python/node/bun…) adımlarında `<meta.icon>` yanına eklenir.
- **Bağımlılık/bundle:** `simple-icons` (named import → tree-shake doğrulandı, yalnız
  kullanılan ikonlar bundle'a girer). `tsc --noEmit` + `vite build` temiz; parser 12
  örnek vaka ile node sanity-check'ten geçti (frontend'de unit-test runner yok).

## TurnStep ikonları tek kaynağa çekildi (sohbet ↔ ayar ekranı) ✅ (2026-07-12)

- **Sorun:** `stepKinds.ts` "tek doğruluk kaynağı" olduğunu iddia etse de yalnız ayar
  ekranı (`StepKindsPanel`) onu tüketiyordu (emoji); sohbet step bileşenleri ikonları
  ayrı ayrı hardcode ediyordu (lucide SVG). Drift vardı (ör. `thinking` ayarda 🧠,
  sohbette 💭).
- **Çözüm:** Metadata `@/shared/stepKinds.ts`'e taşındı; her kind artık bir **lucide
  `Icon`** taşır (+ `STEP_KIND_MAP` O(1) lookup). Ayar ekranı ve **tüm** sohbet step
  bileşenleri (TextStep/ThinkingBlock/ErrorStep/RecoveryStep/SteerStep/HookStep/
  SubagentStep/ContextChangeCard/TodoCard/DiffCard/ToolDeltaStep) aynı ikonu oradan
  render eder → görsel birebir aynı, drift imkânsız. `tsc --noEmit` + `vite build` temiz.

- **Teşhis:** Aynı tura düşen birden fazla `<task-notification>`'dan biri koordinatör
  LLM'i tarafından gözden kaçırılınca (biten worker'ı "hâlâ çalışıyor" sanması), o tur
  `pending` olmadan bitip drain döngüsü çıktığı için atlanan completion bir daha
  ziyaret edilmiyordu → koordinatör zaten biten bir worker'ı sonsuza dek bekliyordu.
  Kuyruk mekanizması bildirimi kaybetmiyor; açık **reasoning + liveness** katmanında.
- **Fix 1 — otoriter worker-state bloğu:** `coordinatorWorkerStatusBlock` her koordinatör
  turunun dinamik system suffix'ine (`autonomousDynamicSuffix`, coordinator-only) canlı
  `ListWorkers` durumunu enjekte eder → model biten worker'ı "çalışıyor" sanamaz.
- **Fix 2 — idle reconciliation sweep:** `drainCoordinator` çıkışta, koordinatör worker
  spawn etmişse (`hadWorkers`) ve **tüm** worker'lar bitmişse (`workers==0`) ve bu batch
  için henüz yapılmadıysa (`ackedIdle`), tek-seferlik `<coordination-status>All workers
finished…</coordination-status>` notu ekleyip bir otoriter tur daha koşar. `ackedIdle`
  bir sonraki bildirimde re-arm olur; `CoordinatorMaxTurns` cap'i sınırlar → sonsuz döngü
  yok. LLM bir bildirimi atlasa bile stall imkânsız.
- **Testler:** `TestIdleReconcileSweepRunsFinalTurn` (process+reconcile, one-shot, re-arm),
  `TestIdleReconcileSkippedWithoutWorkers`; mevcut coalesce/user-turn testleri yeşil.
  (`coordination.go`, `runtime.go`, `coordination_test.go`.)

## MCP sunucu satırına "Kopyala" butonu ✅ (2026-07-12)

- **İstek:** Araçlar & MCP ekranında custom MCP eklendikten sonra Test/Aç-Kapat/Sil
  yanına, sunucu yapılandırmasını başka yerlere yapıştırabilmek için bir **Kopyala**
  butonu.
- **Çözüm:** `ServerManagement.tsx`'e Test'ten hemen sonra **Kopyala** butonu — sunucuyu
  standart `mcpServers` JSON belgesi (Claude Code / .mcp.json biçimi; "JSON ile içe aktar"
  kutusunun kabul ettiği aynı şekil) olarak panoya kopyalar → başka workspace/araca
  yapıştırıp içe aktarılabilir. Tıklayınca 1.5sn "Kopyalandı" geri bildirimi verir.
  `navigator.clipboard` yoksa (güvensiz bağlam) gizli textarea + `execCommand('copy')`
  fallback'i. transport'a göre yalnız ilgili alanlar yazılır (stdio → command/args/env,
  http → url/headers).
- **Değişiklikler:** `toolMeta.ts` — `serverToImportJson(server)` + `parseJsonObject(raw)`
  yardımcıları (env/headers JSON string'lerini güvenli parse). `ServerManagement.tsx` —
  `copiedId` state + `copyServer`, `data-testid="mcp-server-copy"`.
- Doğrulama: `tsc --noEmit` temiz.

## archive_sessions: kendi oturumunu da arşivleyebilir (include_current) ✅ (2026-07-11)

- **Karar:** `archive_sessions` artık mevcut oturumu **yalnız varsayılan olarak** hariç
  tutuyor; yeni `include_current: true` argümanıyla ajan **kendi çalıştığı oturumu da**
  arşivleyebilir. Arşivleme soft/geri-alınabilir ve çalışan turu durdurmaz (sadece
  aktif listeden çıkar). `exclude` listesi include_current ile de geçerli.
- **Değişiklikler:** `builtin_sessionarchive.go` — args'a `IncludeCurrent bool`, keep-set'e
  ekleme artık `!args.IncludeCurrent` koşullu; Def açıklaması + şema (`include_current`),
  struct/constructor yorumları ve "nothing to archive" mesajı güncellendi (koşullu ipucu).
  Not: şema açıklamasındaki `` `exclude` `` backtick'leri raw-string literal'i erken
  kapatıyordu → düz metne çevrildi. Test: `builtin_sessionarchive_test.go` →
  `TestArchiveSessionsIncludeCurrent`.
- Doğrulama: `go build ./...` + `go test ./internal/tools -run TestArchiveSessions` yeşil.

## Otonom-tur boot kurtarması + SpawnIdle Settings UI ✅ (2026-07-13)

- **Sorun:** Worker/spawn/koordinatör turları fire-and-forget goroutine; crash/restart
  onları sessizce öldürüyor → session yanıtsız `state=active` kalıyor VE worker'ın
  koordinatörü hiç bildirim almadığı için sonsuza dek bekliyor (SES28 donması). Inbox
  turları kurtarılıyordu ama otonom turlar için kurtarma YOKtu.
- **Çözüm — boot kurtarması** (`Runtime.RecoverOrphanedTurns`, `coordination.go`;
  boot'ta `server.go` → `recoverAutonomousTurns` her workspace runtime için çağırır):
  son mesajı **user** olan (yani yarıda kalmış) otonom session'lar için:
  - **worker** → interrupted assistant reply yaz + koordinatöre sentetik
    `<task-notification status="killed">` enjekte et → koordinatör beklemeyi bırakır,
    yeniden görevlendirebilir/sonuçlandırabilir;
  - **koordinatör** (yarıda ölmüş) → bir koordinatör turu re-enqueue → geçmişten devam;
  - **düz spawn** → interrupted reply (donuk görünmesin).
    Idempotent (reply eklenince son mesaj assistant olur, ikinci boot atlar), archived
    session'lara dokunmaz.
- **Testler:** `internal/agent/recover_orphan_test.go` (worker kurtarma + koordinatör
  bildirimi + idempotent + tamamlanmış worker'a dokunmama).
- **SpawnIdleTimeoutMin Settings UI:** AppToolsPanel'e "Spawn boşta süresi (dk)" alanı
  eklendi (spawn üst-sınır alanının yanına); tip + save payload + backend round-trip
  `spawnTimeoutMin` ile birebir. "Spawn süresi" etiketi "üst sınır" olarak netleştirildi.
- **Doğrulama:** full suite **847 test** yeşil, vet temiz, frontend tsc temiz.
- **Kalan:** durable spawn KUYRUĞU (N-bounded, "limit reached" yerine sıraya al) —
  kullanıcı isteğiyle şimdilik ertelendi.

## Otonom tur timeout'u: hard-cap tunable + idle watchdog ✅ (2026-07-13)

- **Sorun:** Worker + koordinatör turları **hardcoded 10 dk `spawnTimeout` const**'una
  takılıydı (sıradan spawn zaten 20 dk tunable `r.tun.SpawnTimeout()` kullanıyordu) →
  ağır keşif worker'ları (72-101 tool çağrısı) tam 600sn'de tur ortasında kesiliyordu
  (SES33). Ayrıca mutlak duvar-saati "asılı" ile "uzun ama üretken"i ayırmıyordu.
- **Çözüm — iki katmanlı süre sınırı** (`internal/agent/activity_timeout.go`
  `withActivityTimeout`): (1) **hard-cap** = `r.tun.SpawnTimeout()` (default 20 dk),
  (2) **idle watchdog** = yeni tunable `r.tun.SpawnIdleTimeout()` (default **5 dk**).
  Tur her adım yaydığında (`SessionStepEmitter` → `activityTouchFrom` → touch) idle
  timer resetlenir; adım akmayan (gerçekten asılı) tur idle penceresinde iptal edilir,
  üretken uzun tur hard-cap'e kadar koşar. Hangisi önce dolarsa ctx iptal.
- **Kapsam:** worker, koordinatör, spawn ve inbox-delivery **iş turları** artık
  hard-cap + idle kullanıyor (10 dk const yalnız turn-finished/failed **hook**
  dispatch'inde kaldı — iş turu değil).
- **Ayarlanabilir:** `SpawnIdleTimeoutMin` settings alanı (default 5) — `SpawnTimeoutMin`
  ile birebir aynı plumbing (settings.go + defaults + server.go applySettings).
- **Testler:** `internal/agent/activity_timeout_test.go` (idle iptal, touch canlı
  tutar, hard-cap, tunable). Full suite **844 test** yeşil, vet temiz.
- **Not:** Bu, süreç **restart**'ında öksüz kalan turları çözmez (o hâlâ ayrı bir iş:
  otonom-tur boot kurtarması). Bu değişiklik yalnız **asılı/uzun** turların timeout
  davranışını düzeltir.

## Otonom turlar için "çalışıyor" göstergesi + gerçek Durdur ✅ (2026-07-12)

- **İhtiyaç:** Canlı adım köprüsü düzeldikten sonra ghost balonu akıyordu ama otonom
  turlarda (koordinatör/scheduler/spawn) "çalışıyor" göstergesi + composer Durdur/
  Kes/Yönlendir kümesi çıkmıyordu — çünkü frontend `streamingSessions`'ı yalnız
  `KindUserMessage`'da işaretliyor, otonom tur bunu yayınlamıyor.
- **Frontend:** `chatStreamHub.ts` — ilk hub aktivitesinde (`KindAgentStart` ve
  `KindStep`) oturum `streamingSessions`'a eklenir → transkript göstergesi +
  composer busy-durumu interaktifle simetrik çıkar. `turn_done`/`turn_error` (ve
  global tamamlanma feed'i) temizler.
- **Backend:** `autonomousInteraction` (`autonomous_interaction.go`) artık **gerçek
  bir cancel** kaydediyor (eskiden no-op): `ctx, cancel := context.WithCancel(ctx)`
  → dönen iptal-edilebilir ctx CLI provider çağrısına akar, cleanup `cancel()`+
  `unregister`. Böylece izleyicinin "Durdur"/"Kes"i otonom claude-cli turunu
  gerçekten durdurur. `coordination.go` `runCoordinatorTurn`/`runWorker`:
  `errors.Is(err, context.Canceled)` → temiz "⏹️ … durduruldu" mesajı (hata değil).
- **Doğrulama:** api+agent **274 test** yeşil, vet temiz, frontend tsc temiz.

## Bug fix — otonom tur canlı adımları hub'a köprülenmiyordu ✅ (2026-07-11)

- **Teşhis:** Koordinatör (ve scheduler/spawn/worker) otonom turlarında ajanın
  düşünce/tool adımları canlı görünmüyor, yalnız tur bitince toptan geliyordu.
- **Kök neden:** `autonomousInteraction` her otonom claude-cli turu için Interaction
  MCP Bearer token'ını eşlemek üzere **token-only bir chatRun** kaydediyor
  (`run.autonomous=true`). `bridgeBusToHub` `session_step` guard'ı ise
  `if _, live := sessionRunInfo(sid); live { continue }` ile "canlı run'ı olan
  oturumu atla" yapıyordu — bu guard **interaktif** turlar için doğru (runChatTurn
  adımları zaten doğrudan hub'a yayınlar, çift yayını önler), ama otonom token-only
  run hub'a hiçbir şey yayınlamadığından adımlar **düşüyordu**. Tur bitince run
  unregister → completion event `turn_done`+reload → her şey birden.
- **Çözüm:** guard yalnız **interaktif** run'ı atlasın:
  `if info, live := sessionRunInfo(sid); live && !info.Autonomous { continue }`
  (`internal/api/session_stream.go`). Otonom run'ların adımları artık köprüleniyor.
- **Testler:** `internal/api/bridge_autonomous_test.go` — otonom adımlar köprüleniyor,
  interaktif adımlar çift yayınlanmıyor. api paketi 96 test yeşil.

## claude-cli canlı steer (Yönlendir) ✅ (2026-07-11)

- **Sorun:** "Yönlendir" yalnız native provider'da çalışıyordu; claude-cli için
  backend `unsupported` dönüp mesajı kuyruğa düşürüyordu (mid-turn rehberlik yok).
- **Çözüm (external-agent muadili, Doc 59):** claude-cli için steer mesajı
  `chatRun.pendingSteer`'a saklanır (`setSteer`/`takeSteer`, `chat_control.go`);
  `handleSessionControl` claude-cli → stash + `"steered"` (`inbox.go`);
  `callPermission` her **allow** (auto-allow RiskRead + prompt sonrası) sınırında
  mesajı `additionalContext` olarak enjekte eder (`permDecisionCtx`/`steerContext`,
  `mcp_interaction_tools.go`) → rehberlik bir sonraki **tool sınırında** turu
  yeniden başlatmadan bağlama girer. Tur tool'suz (yalnız metin) biterse
  `runChatTurn` bekleyen mesajı **sonraki tura enqueue** eder (steer_undelivered
  fallback, `chat_stream.go`).
- **Testler:** `internal/api/steer_cli_test.go` (stash consume-once, additionalContext
  enjeksiyonu, steer yokken plain karar). api paketi 94 test yeşil; frontend tsc temiz.
- **Açık doğrulama:** `additionalContext`'in claude-cli permission cevabında modele
  gerçekten girip girmediği CLI sürümüne bağlı (canlı test). Girmezse Doc 59 Faz 2
  seçenek (B) PreToolUse hook kanalına geçilir; fallback her hâlükârda güvenli.

## Shell sağlamlaştırma: non-interactive git + süreç-ağacı reap ✅ (2026-07-11)

- **Teşhis:** Bir ajan shell aracıyla `git commit` çalıştırınca tur tamamen
  kilitleniyordu. İki kök neden: (1) `core.editor=notepad` → git editör açmak
  isteyince headless/stdin'siz alt-süreçte **notepad GUI'si sonsuza dek bloke**
  oluyordu (env'de `GIT_EDITOR` yoktu); (2) timeout'ta `exec.CommandContext`
  yalnız doğrudan çocuğu (bash/powershell) öldürüp **spawn edilen `git.exe`+editör
  torununu zombi bırakıyordu** → `.git/index.lock` kalıyor, sonraki commit'ler
  bloke. Gözlemde ~28 zombi git süreci + 10 dakikalık boşa tur.
- **Çözüm (uygulama geneli):** `internal/proc` katmanına iki primitive:
  - `HardenedEnv(base)` → non-interactive guard'lar (`GIT_TERMINAL_PROMPT=0`,
    `GIT_EDITOR=true`, `GIT_SEQUENCE_EDITOR=true`, `GIT_PAGER=cat`/`PAGER=cat`,
    `GIT_OPTIONAL_LOCKS=0`, `GCM_INTERACTIVE=never`). Guard'lar **en sona
    eklenir** → os/exec last-wins dedup'ı ile miras `notepad` editörünü ezer.
  - `TreeKill(cmd)` → ctx iptal/timeout'ta **tüm çocuk ağacını** öldürür (Windows
    `taskkill /F /T`; Unix `Setpgid` + negatif-pid `SIGKILL`) + `WaitDelay` (5sn)
    ile takılan doğrudan çocuğu force-kill eder.
  - Bağlanan yerler: `builtin_shell.go` Bash+PowerShell `build` closure'ı ortak
    `hardenShellCmd` (foreground + `run_in_background` ikisi de) ve claude-cli
    `cliBaseEnv` (CLI ajanının kendi git'i de non-interactive).
- **Testler:** `internal/proc/env_test.go` — guard override (notepad→true),
  TreeKill Cancel/WaitDelay set; build+vet+tools/providers testleri yeşil.
- **Confined imza kapatma:** `DisableGitSigningEnv()` → `commit.gpgsign=false` +
  `tag.gpgsign=false` (git `GIT_CONFIG_COUNT/KEY/VALUE` env-injection). Yalnız
  **confined (otonom/spawn) shell'lerde** enjekte edilir (`hardenShellCmd(cmd,
t.sb.Confined)`); interaktif turlar imzayı korur (insan passphrase girebilir).
  Böylece nezaretsiz `git commit` GPG pinentry'de asılamaz.

## Çapraz-session farkındalığı: tamamen ayarsız → her zaman açık + list_sessions sayfalama ✅ (2026-07-11)

- **Karar:** Çapraz-session farkındalığı artık diğer pull araçları gibi **her zaman
  aktif ve hiç ayarı yok**. Önce iki toggle (`Session bağlamı` = `SessionContextEnabled`,
  master; `Her turda ver` = `SessionContextEveryTurn`), ardından son kalan
  `Listelenecek geçmiş session sayısı` (`SessionContextRecentCount`) input'u da
  kaldırıldı. Pushed özet bloğu daima yalnız session'ın **ilk turunda**, sabit **5**
  geçmiş session ile verilir; `list_sessions`/`archive_sessions`/`conversation_search`
  daima sunulur.
- **list_sessions sayfalama:** Araç artık `offset` argümanı alıyor; yanıt toplam
  sayıyı ("Showing X–Y of Z") ve daha varsa bir sonraki sayfanın offset'ini bildiriyor
  → **tüm** sessionlar (aktif veya geçmiş) sayfa sayfa gezilebilir (varsayılan sayfa
  20). (`builtin_sessions.go` + `builtin_sessions_test.go`.)
- **Değişiklikler:** `WSSettings`/`WSSettingsPatch` + API DTO'dan üç alanın hepsi
  silindi, `clampRecent` kaldırıldı (`settings.go`, `workspace_settings.go`).
  `Runtime`'dan tüm `sessionCtx*` durumu + `SetSessionContext`/`SessionContextRecentCount`
  silindi, `DefaultSessionContextRecent` sabiti kaldırıldı (`runtime.go`, `tunables.go`).
  Gate'ler koşulsuz (`toolsetup.go`, `runtime.go` CLI bridge, `agent_context.go`);
  `sessionsContextBlock` artık parametresiz (sabit 5), `chat_turn.go` yalnız
  `freshSession` koşuluyla enjekte ediyor. Frontend: `WorkspacePanel.tsx`'ten
  "Çapraz-session farkındalığı" bölümü **tamamen kaldırıldı** (başlık + açıklama; hiç
  ayar yok), tipler + `WorkspaceView` payload'ı güncellendi. Testler (`settings_test.go`, `sessionctx_test.go`, `sessions_context_test.go`,
  `builtin_sessions_test.go`) yeni imzalara uyarlandı.
- Doğrulama: `go build ./...` + `go test ./internal/{workspace,agent,api,tools}` yeşil;
  frontend `npx tsc --noEmit` yeşil.

## Artifact arşivleme + "Kalıcı ilerleme" oturum sızıntısı düzeltmesi ✅ (2026-07-11)

- **TSK46 (iki parça):**
  1. **Artifact arşivleme** — Artifact'ler artık oturumlar gibi _yumuşak,
     geri-alınabilir_ şekilde arşivlenebiliyor (silinmiyor). Model'e
     `Archived bool` alanı (`models_artifact.go`), store'a `SetArtifactArchived`
     (`store_artifact.go`, `SetArtifactGroup` desenini izler), API'ye
     `PUT /api/artifacts/{id}/archive` (`artifacts.go` + `server.go`) eklendi.
     Frontend: `setArtifactArchived` API çağrısı, `Artifact.archived` tipi,
     Artifacts ekranında filtre çubuğuna **Arşiv (N)** görünüm toggle'ı (aktif ⇄
     arşiv listeleri asla karışmaz), toplu **Arşivle/Arşivden çıkar** aksiyonu,
     detay toolbar'ında tekil arşiv butonu + "Arşivlendi" rozeti. Son arşivli
     artifact geri alınınca görünüm otomatik aktif listeye döner.
  2. **Bug** — "Kalıcı ilerleme · N/N" kartı bütün oturumlarda aynı görünüyor ve
     hiç kaybolmuyordu: `progress.json` **çalışma dizinine** göre anahtarlı
     (bkz. `36-KALICI-ILERLEME.md`), aynı proje dizinini paylaşan tüm oturumlar
     tek dosyayı okuyordu. Kayıt zaten **son yazan** oturumun `sessionId`'sini
     tutuyor (`todosink.go` `SaveTodos`); `SessionDetailPanel` artık kartı yalnız
     kaydı son yazan oturumda gösteriyor (sahipsiz legacy kayıt hâlâ görünür).
     `SessionProgressCard`'daki artık ulaşılamaz "foreign" uyarı bloğu kaldırıldı.
- Doğrulama: `go build ./...` + `go vet ./...` + `go test ./internal/...` yeşil;
  frontend `npx tsc -b` + `npm run build` yeşil.

## Canlı süre göstergesi her tool'da sıfırlanıyordu ✅ (2026-07-11)

- **Problem:** Sohbette asistan balonunun altındaki canlı süre (`LiveTimer`), agent'ın
  baştan beri çalışma süresini değil, **son tool'dan/step'ten beri geçen süreyi**
  gösteriyordu. Kök neden: `chatStreamHub.ts`'de `composeGhost()` her `syncGhost`
  çağrısında (her step/delta) `createdAt: Date.now()` ile **yeniden** damgalanıyordu;
  `LiveTimer startUnixSec={m.createdAt}` de bu sürekli-yenilenen zamandan sayıyordu.
  (SES162'de 5 dk görünmesinin sebebi: hung olduğu için hiç step gelmemiş, damga sabit
  kalmıştı — bug'ı maskeliyordu.)
- **Çözüm:** `ghostStartedAt` tur başında **bir kez** damgalanıp (`AgentStart`'ta;
  step'ler önce gelirse `syncGhost` fallback'iyle) tüm step/delta upsert'lerinde sabit
  tutuluyor, `dropGhost`'ta sıfırlanıyor. `composeGhost` artık `createdAt: ghostStartedAt`
  kullanıyor → canlı sayaç agent'ın tüm turunu sayıyor. Tamamlanan tur süresi
  (`workedSec = m.createdAt − tetikleyen user mesajı`) zaten doğruydu, dokunulmadı.
- Doğrulama: `tsc --noEmit` temiz. (Frontend değişikliği → gömülü dist rebuild gerekir.)

## sqz PreToolUse köprü uyumsuzluğu — teşhis + workaround (2026-07-11, kod değişikliği yok)

- **Bulgu:** WS5'te `sqz` (token sıkıştırıcı) HOK1 olarak **enabled** ve CLI
  `--settings`'ine doğru forward ediliyor, ama `sqz gain` → _son 7 günde 0 sıkıştırma_
  (tarihsel 25). Sebep: `sqz hook claude` **yalnız `tool_name == "Bash"`** olan çağrıları
  yeniden yazıyor; TionHarness shell'i claude-cli'ye **MCP-namespaced** isimle köprülüyor
  (`mcp__tionharness_interaction__PowerShell` — 463 çağrı — ve `__Bash` — 49), ayrıca
  CLI-native `Bash` gölgeleme korumasıyla deny listesinde. Sonuç: matcher `Bash,PowerShell`
  gerçek shell çağrılarını yakalamıyor → sqz hiç tetiklenmiyor. (Aynı sorun rtk/HOK2'de de
  var; HOK2 zaten disabled.)
- **Workaround (workspace config, TionHarness source'a dokunmaz):** köprü scripti
  `Progs/sqz/sqz-bridge-hook.ps1` — köprülü `tool_name`'i `Bash`'e normalize edip sqz'ye
  **byte-temiz** (temp dosya + `cmd` redirection; WinPS 5.1 pipe UTF-16 bozuyor) devreder,
  çıktıyı BOM'suz yazar. HOK1 matcher'ı köprülü isimleri de içerecek şekilde genişletildi
  ve komutu bu scripte bağlandı. Test: köprülü PowerShell/Bash → `<cmd> 2>&1 | sqz compress`
  olarak yeniden yazılıyor (native Bash da bozulmadı). **Tüm sqz-hook'lu workspace'lere
  uygulandı:** WS5/HOK1, WS1/HOK2, WS10/HOK4 (WS10'unki bir otonom ajan tarafından çift-escape'li
  bozuk yazılmıştı — `-File \"C:\\...\"` — düzeltildi). WS2/WS8/WS9'da sqz hook yok. **Etkin
  olması için TionHarness restart gerekir** (hook DB bellek-içi; dosya boot'ta yüklenir).
- **Olası kalıcı çözüm (gelecek kart):** ya sqz'nin köprülü tool-adı desteği, ya da
  TionHarness'in bridged-shell çıktısını doğrudan bir token-optimizer'dan geçiren native seam.

## claude-cli startup-hang watchdog ✅ (2026-07-11)

- **Problem:** Spawn edilen bir board-otomasyon turunun `claude.exe -p` subprocess'i
  MCP `initialize` handshake'inde kilitlenip **hiç stdout üretmeden ve çıkmadan**
  7+ dk askıda kaldı (WS5/SES162). `runAttempt`'in bloklu okuma döngüsü yalnız `ctx`
  iptaliyle biterdi; spawn/otonom turun ctx'inde deadline yoksa → `llm_call` yok,
  `error` yok, sohbetteki "yazıyor" göstergesi hiç temizlenmez, seri kuyruk kilitlenir.
- **Çözüm:** `runAttempt` okuma döngüsüne **startup-only watchdog** eklendi
  (`claudecli.go`, `cliStartupTimeout = 90s`). Reader goroutine + timer; **yalnız
  ilk-çıktıya-kadar** olan süre korunur — ilk stdout satırı gelince timer durur,
  sonraki uzun sessizlikler (meşru sync `run_subagent`, dakikalarca sessiz) CLI'nin
  `MCP_TOOL_TIMEOUT`'una + dış ctx'e bırakılır (yanlış-pozitif kill yok). Çıktısız
  hang'de subprocess öldürülür → `cmd.Wait` döner → **retryable** net hata döner →
  self-healing bir kez retry eder, ghost temizlenir, kuyruk açılır.
- Doğrulama: `go build ./...` + `go vet ./internal/providers` + `go test
./internal/providers -short` yeşil.

## `/compact` + `/handoff` claude-home seam düzeltmesi ✅ (2026-07-11)

- **Problem:** Manuel `/compact` (`api.compactSession`) ve `/handoff`
  (`Runtime.HandoffSession` → `conversation.BuildHandoff`) provider'ı
  `providers.Get` ile alıp **doğrudan** `provider.Complete` çağırıyordu —
  `guardedComplete` funnel'ını (dolayısıyla `SetConfigDir` seam'ini) atlayarak.
  claude-cli sağlayıcıda provider default `configDir` = **global**
  `~/.tionharness/claude-home`; oranın credential'ı boşsa `authentication_failed`
  dönüyordu, workspace'in kendi `claude-home`'u login olsa bile (WS5/SES130'da
  görüldü).
- **Çözüm:** Ortak seam tek yere alındı — `Runtime.PinClaudeHome(provider)`
  (`budget.go`), claude-cli provider'ının config evini bu workspace'in
  `claude-home`'una sabitler (diğerlerinde no-op). `guardedComplete` artık bunu
  çağırıyor; iki out-of-loop yol (`compactSession`, `HandoffSession`) da ham
  `Complete` öncesi çağırıyor. Diğer yardımcı çağrılar (title/summary/reflect)
  zaten `guardedComplete`'ten geçtiği için etkilenmemişti.
- Doğrulama: `go build ./...` + `go vet` + `go test ./internal/agent` yeşil.

## an external CLI agent `/chronicle` referans dokümanı ✅ (2026-07-11)

- **TSK30 (doküman-only):** an external CLI agent'nin `/chronicle` oturum-içgörü
  ailesini (tips / improve / standup / cost-tips / search / reindex) + yerel
  SQLite session store mekaniğini açıklayan ve TionHarness muadilleriyle
  (`session.jsonl`+`debug.jsonl`, ders döngüsü `lessons.jsonl`,
  `conversation_search`, Tasarruf Merkezi) kıyaslayan `_Docs/64-GITHUB-COPILOT-CHRONICLE.md`
  eklendi. Boşluk tespiti: proaktif `tips`/`standup` içgörü üreteci TionHarness'te yok
  (gelecek kart tohumları dokümanda). Kaynaklar dipnotlandı (GitHub Docs + changelog).
  `00-GENEL-BAKIS.md` dizinine 58 + 59 satırları eklendi. Kod değişikliği yok.

## Sohbet kuyruğu + çoklu-ekran senkronizasyonu (event-sourcing cutover) ✅ (2026-07-11)

- **Amaç:** Sohbeti "owner window kendi SSE'sini stream'ler + non-owner'lar
  inflight snapshot + polling ile kurtarır" ikiliğinden çıkarıp **sunucu-otoriter,
  tek total-order'lı, cursor tabanlı event akışı** modeline taşımak. Her pencere
  sadece abone; "sahip pencere" öldü. Tam tasarım + faz planı: `_Docs/58-QUEUE-SENKRON.md`.
- **Faz 1 — SessionHub + cursor SSE:** `internal/sessionhub` (per-session monoton
  `seq` + ring buffer + epoch + gap-aware `Replay` + `Commit` boundary);
  `GET /api/sessions/{id}/stream?since=&epoch=` (hello/reset/hub, `Last-Event-ID`
  uyumlu). İnteraktif tur tüm dayanıklı olayları eş-sıralı hub'a yayınlıyor;
  autonomous turlar `bridgeBusToHub` ile bus→hub (steps + turn_done). **Replay
  optimizasyonu:** fresh abonelik (`since<=0`) yalnız `committed`'dan sonraki
  in-flight tail'i replay eder → tamamlanmış turlar tekrar oynatılmaz.
- **Faz 2 — Interaction CAS (resolve-once):** `ask_user`/`permission`/`plan` tek
  `pendingInteraction` primitive'ine; `interaction_open`/`interaction_resolved`
  tüm pencerelere yayılır; cevap `POST .../interactions/{iid}/answer` → compare-and-
  swap, ilk yazan kazanır (200), diğeri 409 + kart kapanır. Native + CLI yolu.
- **Faz 3 — Durable send-queue:** `handleChatStream` → ince wrapper + `runChatTurn`
  (HTTP'siz, worker'ın çağırdığı çekirdek). `db/inbox.go` (durable `inbox.json`) +
  `api/inbox.go`: `POST /sessions/{id}/messages` (enqueue, `clientMsgId` idempotency),
  `DELETE .../queue/{msgId}` (cancel), `POST .../control` (session-scoped stop/steer),
  `queue_update` broadcast, boot `recoverInboxes`. Frontend: send = enqueue
  (optimistic yok — kuyruktaysa tray, çalışınca chat balonu); client-side flush
  kaldırıldı, backend serialize ediyor.
- **Faz 4 — Presence:** `hub.SubscriberCount` → efemer `presence` yayını her abone
  giriş/çıkışında → ChatView "Bu oturum N pencerede açık" rozeti.
- **Frontend cutover:** `api/sessionStream.ts` (cursor+epoch+gap-detect+reconnect),
  `features/chat/chatStreamHub.ts` (aktif session'ın tek otoriter render'ı);
  `useChatStream` hub aboneliği; inflight polling + bus-ghost + recoverInflight
  kaldırıldı.
- **Doğrulama:** `go build`/`go vet` yeşil, **815 test / 35 paket** (3 interaction
  testi yeni CAS'a göre güncellendi); `tsc --noEmit` + `vite build` temiz; runtime
  smoke (headless boot + hub stream hello/presence + queue/control endpoint'leri).
- **Bilinen sınır:** autonomous turlar reply mesajını hub'a yayınlamıyor
  (tamamlanma `bridgeBusToHub`'ın turn_done'u + listMessages reload ile geliyor).

## Mobil: interaktif kartlar taşınca kaydırılabilir ✅ (2026-07-10)

- **Sorun:** Telefon ekranında `ask_user` (çok-soru), plan onayı, izin ve görev
  listesi kartları viewport'u aşınca üst kısımları görünmüyordu — dikey kaydırma yoktu.
- **Çözüm (yalnız frontend, mantık değişmedi):** ortak `ScrollableCard`
  (`shared/components/ScrollableCard.tsx`) sarmalayıcısı — viewport-oransal
  `maxH` (varsayılan `max-h-[55vh]`) + `overflow-y-auto`. Büyüyen içerik bölgesi bu
  sarmalayıcıya alındı; aksiyon butonları dışarıda bırakıldı → her zaman görünür.
  - Kullananlar: `AskPrompt` (SingleAsk soru/seçenek + MultiAsk soru listesi, 55vh),
    `TodoCard` (50vh), `TodoPanel` (45vh), `PermissionPrompt` (komut `<pre>`, 40vh),
    `PlanPrompt` (plan markdown'ı, 55vh).

## Bash öncelikli, PowerShell gerektiğinde ✅ (2026-07-10, TSK43)

- **Sorun:** Windows'ta ajan koşulsuz PowerShell'e yönlendiriliyordu — üç katman
  (ortam bloğu, Bash aracı açıklaması, claude-cli köprüsü). bash.exe zaten kuruluysa
  `Bash` aracı native döngüde kayıtlıydı; sorun araç eksikliği değil **önceliklendirmeydi**.
- **Çözüm (davranış/prompt-policy refactor, execution core değişmedi):**
  - `EnvironmentContextBlock()` artık `tools.ShellToolNames()`'in ilk girdisine göre
    tercih edilen shell'i ilan eder — Windows'ta bash.exe varsa `shell="Bash"` +
    "prefer Bash; use PowerShell only for Windows-native tasks", bash.exe yoksa
    PowerShell'e düşer (guard'a bağlı, sessiz yutma yok).
  - `ShellTool.Def()` "PREFERRED shell — reach for it first, including on Windows";
    `PowerShellTool.Def()` "use ONLY when the Bash tool cannot do the job".
  - claude-cli köprüsü: `interactionToolSpecs()` Windows'ta `ShellToolNames()`'e göre
    hem Bash (varsa) hem PowerShell'i ilan eder; `NewShellRunner()` closure'ı artık
    `toolName`'e göre dispatch eder (Bash→POSIX, PowerShell→PS; Windows'ta bash yoksa
    PowerShell'e düşer); `callShell()` dispatch'e tool adını (`bare`) geçirir.
  - `climcp.go` shadowing yorumu ve `default-instructions.md` shell cümlesi
    Bash-öncelikliye güncellendi. Native döngü zaten iki aracı da kaydediyordu.
- **Bash mevcudiyeti guard'ı:** Tüm Bash-öncelikli ilan `resolvePOSIXShell()`/`Available()`
  üzerinden `ShellToolNames()`'e bağlı — bash.exe olmayan makinede "Bash" iddia edilmez.
- **Not (kullanıcı CLAUDE.md çelişkisi):** Bu makinenin `CLAUDE.md`'si Windows/PowerShell
  tercihi belirtir (kullanıcının; dokunulmadı). Bu değişiklik uygulamanın **varsayılan
  ajan yönlendirmesini** Bash-öncelikliye çevirir; kullanıcı workspace talimatı/CLAUDE.md
  ile bunu ezebilir.
- **Doğrulama:** `go build ./...`, `go vet ./...`, `go test ./internal/...` — hepsi yeşil
  (agent/api/tools dahil; frontend'e dokunulmadı). Detay `_Docs\51`, `_Docs\17`.

## Artifact detayında tekil grup düzenleme ✅ (2026-07-10, TSK44)

- **Sorun:** Bir artifact açıkken grubunu değiştirmenin yolu yoktu — yalnız çoklu-seçim
  (bulk) grup atama ve sürükle-bırak vardı. Tek bir artifact'i gruplamak için kullanıcı
  çoklu-seçime girmek veya DnD yapmak zorundaydı.
- **Çözüm:** `ArtifactsPanel` detay editörüne doğrudan grup input'u eklendi
  (`data-testid="artifact-edit-group-input"`, mevcut grup adları `artifacts-group-names`
  datalist'iyle önerilir; boş = grupsuz). `Draft`'a `group` alanı; `createNew`/`startEdit`
  onu doldurur; `dirty` grup farkını da içerir.
- **Kayıt yolu:** `save` önce içerik/meta patch'ini (`api.updateArtifact`), grup değiştiyse
  ardından `api.setArtifactGroup(id, group)` çağırır — backend `updateArtifact` handler'ı
  `group`'u yok saydığı için mevcut `PUT /api/artifacts/{id}/group` endpoint'i reuse edildi.
  **Backend değişikliği yok.** Detay `_Docs\45`.
- **Doğrulama:** `go build/vet`, `go test ./internal/...`, `npx tsc -b`, `npm run build` — hepsi yeşil.

## Spawn süresi ayarlanabilir + otomatik-devam deadline koruması ✅ (2026-07-10)

- **Sorun:** Otomatik-devam turu (`maybeAutoContinue`), spawn work-turn'ünün context'ini
  paylaşıyordu. İlk tur 10 dk'lık sabit `spawnTimeout`'u tükettiğinde devam turu **süresi
  dolmuş** context'te 80 ms'de patlayıp kullanıcıya ham `context deadline exceeded`
  gösteriyordu (SES147).
- **Fix 1 — ayarlanabilir süre:** Spawn work-turn deadline'ı artık Ayarlar'dan
  (`SpawnTimeoutMin`, **default 20 dk**). `Tunables.SpawnTimeout()` → `runSpawn` kullanır;
  applySettings ile canlı uygulanır. Frontend: AppToolsPanel "Spawn süresi (dk)" alanı.
  Diğer ayrık yüzeyler (worker/inbox/hook firing) sabit `spawnTimeout`'ta kalır.
- **Fix 2 — deadline guard:** `maybeAutoContinue`, her iterasyonda `ctx.Err()` kontrol eder;
  deadline dolmuşsa devam turunu atlar ve ham Go hatası yerine anlamlı "⏱️ Süre doldu…"
  mesajı yazar (`context.WithoutCancel` ile kalıcılaştırır, `auto_continue_error` etiketler).

## Ek sabit değerler Ayarlar'a taşındı ✅ (2026-07-10)

Daha önce kod-sabiti olan 4 değer settings-driven yapıldı (applySettings ile canlı,
0 → yerleşik default). Ayarlar → App/Tools panelinde:

- **Zamanlama süresi** (`ScheduleTimeoutMin`, default **60 dk**) — `scheduler.fire/fireWake`
  artık `s.rt.tun.ScheduleTimeout()` kullanır; spawn süresiyle aynı desen.
  **2026-08-16:** default 30 → 60 yükseltildi ve elle **"Şimdi çalıştır"** yolu
  (`handleRunSchedule`, `internal/api/schedules.go`) da aynı tunable'a bağlandı — orada
  sabit kodlu 10 dk vardı, yani ayar sessizce yok sayılıyordu. Araştırma tipi bir
  zamanlanmış prompt (WS1/SES286) tam bu yüzden `context deadline exceeded` ile 10.
  dakikada kesilmişti. `TurnWatchdogMin` (120) tavanın üstünde kaldığı için tıkanma
  freni hâlâ geçerli; `settings/store.go` zaten watchdog'u schedule süresinin altına
  düşürmüyor.
- **Kabuk varsayılan/maks. süre** (`ShellDefaultTimeoutSec`/`ShellMaxTimeoutSec`, 30/120 sn)
  — `tools.SetShellTimeouts`; per-call `timeout_sec` yine geçersiz kılar, maks. ile kırpılır.
- **Araç çıktı sınırı** (`MaxToolOutputKB`, default 100 KB) — `tools.SetMaxToolOutputBytes`;
  MCP dahil tüm araç çıktısının backstop kesme sınırı.
- **Maks. bağlam token** default `12000 → 800000`.

Not: `tools` paketinde bu değerler artık process-global `var` + setter (const değil).

## Executions ekranı kaldırıldı — birleşik Sohbet transkripti ✅ (2026-07-10, TSK45)

- **Karar:** Ayrı "Aktivite" (Executions) ekranı kaldırıldı. Executions ayrı bir veri
  kaynağı değildi — `GET /api/executions` ile `GET /api/sessions` **aynı** `DB.ListSessions`
  çağrısına dayanıyor; Executions yalnızca her satıra `running` + `lastStatus` ekliyordu.
  Artık **Sohbet** ekranı her oturum türü (chat/task/flow/schedule/spawned) için tek
  birleşik transkript görünümü.
- **Frontend — birleşik sidebar:** `SessionsSidebar` artık tüm oturum türlerini listeler:
  kind rozeti + ikon, kind filtre sekmeleri (`Tümü/Sohbet/Görev/Akış/Spawn/Zamanlama`,
  `localStorage`'da hatırlanır), canlı nabız noktası ve task/flow için pass/fail `StatusPill`.
  `running`/`lastStatus` bilgisi yeni `useExecutionRuntime` hook'undan (`/api/executions`
  poll + `executions` SSE sinyali) `runtimeById` haritası olarak beslenir.
- **Salt-okunur composer:** `ChatView`'e `readOnly` prop'u eklendi; task/flow/schedule
  transkriptlerinde composer + ask/todo/pending/wake yığını **hiç render edilmez**, yerine
  "Bu oturum salt-okunurdur" bandı gösterilir (`isWritableSessionKind` → `chat`/`spawned`
  yazılabilir). Böylece bağlam/debug/oturum-bilgisi panelleri **tüm** oturum türleri için
  açılabilir hâle geldi (önceden yalnız `view === 'chat'` altındaydı).
- **Toplu tablo:** `SessionsOverview` (`features/executions/`'tan `features/sessions/`'a
  taşındı) sidebar'daki "Oturumlar" (`Table2`) butonundan overlay olarak açılır; satıra
  tıkla → o oturumu Sohbet transkriptinde aç.
- **Silinen / taşınan dosyalar:** `features/executions/` klasörü tamamen kaldırıldı
  (`ExecutionsPanel.tsx`, `useLiveTranscript.ts` silindi; `executionsShared.tsx` →
  `features/sessions/sessionKindMeta.tsx`, `SessionsOverview.tsx` → `features/sessions/`).
  `NavRail`/`viewRegistry`/`url.ts`/`eventViews`/`useActivity`/`useDeepLinks`/
  `useAppNavigation`/`useSessionsController` içindeki `executions` referansları
  temizlendi; eski `#executions/<sid>` deep-link'i chat oturumuna düşer. `AgentActivityPanel`
  "yürütmeyi aç" bağlantısı artık chat transkriptine yönlenir.
- **Backend değişmedi:** `GET /api/executions` (`internal/api/executions.go`) korundu —
  `AgentActivityPanel` ve `useExecutionRuntime` onu tüketiyor; yalnız UI ekranı kaldırıldı.
- **Doğrulama:** `go build`/`go vet`/`go test ./internal/...` tümü yeşil; `npx tsc -b`
  temiz; `npm run build` başarılı.

## Spawn session'ı "çalışıyor" göstermiyordu 🐛 (2026-07-10)

- **Sorun:** `spawn` (veya `spawn_worker`) ile bir session oluşturulunca, sohbet ekranında o
  session "devam ediyor" (thinking) olarak gözükmüyordu; composer/input ajan çalışmıyormuş gibi
  görünüyordu.
- **Kök neden:** Spawn/worker turları runtime'da **detached** çalışır (`runSpawn`/`runWorker` →
  `trackSession` → `invokeTraced`); api sunucusunun `chatRuns` (`s.runs`) registry'sine **hiç
  kaydolmaz**. (a) Başlangıçta thinking göstergesini kaldıracak bir sinyal yoktu (scheduler'ın
  wake yolu `chat` phase=start olayı yayar, spawn yaymıyordu). (b) `handleActiveSessions`
  (`/api/active-sessions`, reload sonrası thinking restore kaynağı) yalnız `s.runs`'ı dönüyordu,
  `wsp.Runtime.ActiveSessionIDs()`'i kaçırıyordu → yenileme sonrası da restore edilemiyordu.
- **Çözüm:** (1) Yeni `Runtime.emitTurnStart(sessionID, title)` — wake desenini yansıtan `chat`
  phase=start olayı (frontend `markPending` → thinking + composer busy; `chat` olayları toast
  üretmez). `runSpawn` ve `runWorker` `trackSession`'dan hemen sonra çağırır; tamamlanma zaten
  `"spawned"`/`"worker"` olayıyla `clearPending` yapıyor. Flow/inbox/autocontinue completion'ları
  pending temizlemediği için `trackSession` **global** hook'lanmadı (cerrahi = yalnız spawn+worker).
  (2) `handleActiveSessions` artık `s.runs` **∪** `wsp.Runtime.ActiveSessionIDs()` (dedup) döner →
  reload sonrası spawn/worker/schedule/flow turları da restore edilir. Build + 266 test yeşil.

## Tur-ortası yenileme — eski araç adımları kurtarma yarışı 🐛 (2026-07-10)

- **Sorun:** Executions/Sohbet ekranında bir ajan turu akarken sayfa yenilenirse ajanın
  **reload öncesi** araç kullanımları listeden kayboluyor, yalnız reload **sonrası** yeni araç
  adımları büyümeye devam ediyordu.
- **Kök neden (frontend yarış koşulu):** Kurtarma mekanizması (`recoverInflightSnapshot`,
  `inflight.json` snapshot'ından adımları ghost bubble'a tohumlar) canlı `session_step` bus
  akışıyla yarışıyordu. (a) **Executions** (`useLiveTranscript.load`): snapshot tohumlaması
  `if (!autoLiveRef.has(sid))` ile korunuyordu; `listMessages` (async) çözülmeden önce bir bus
  adımı gelirse `foldAutoStep` boş entry (`steps:[]`) oluşturur → gate `true` olur → recover
  **hiç çağrılmaz** → tüm eski adımlar kaybolur. (b) **Chat** (`recoverInflightSnapshot`):
  mevcut entry'yi sorgusuz **overwrite** ediyordu → yarış anındaki adımlar kaybolur.
- **Çözüm:** (1) `recoverInflightSnapshot` artık **merge** ediyor — canlı entry zaten varsa
  snapshot adımlarını **başa ekler** (reload öncesi adımlar ⊕ reload sonrası bus adımları;
  bus geçmişi tekrar yollamadığı için çakışma yok), bubble id/text korunur. (2)
  `useLiveTranscript`'e `foldAutoStep`'ten bağımsız `seededRef` eklendi → snapshot seçim başına
  **tam bir kez** merge edilir (bus adımı yarışı kazansa bile). Seçim değişiminde
  `seededRef.clear()`. Backend zaten 600ms throttle ile `kept` (tool adımları dahil)
  snapshot'lıyor (`chat_stream.go` `snapshot()`), değişmedi. Frontend `tsc --noEmit` temiz.

## Sohbet & ekran geçişlerinde loading göstergesi ⏳ (2026-07-10, TSK41)

- **Sorun:** Açılışta bir an "Yeni sohbete başla" boş-durumu, sohbet değişince eski
  transkriptin ekranda kalması, panellerde ilk paint'te "boş" görünmesi.
- **Çözüm:** `useSessionsController`'a `bootstrapping` + `messagesLoading` bayrakları,
  transkript yüklemesine `msgSeqRef` in-flight guard'ı (hızlı A→B→A geçişinde geç gelen
  cevap ezmiyor) ve sessiz `.catch(() => {})` yerine `setError`. Yeni primitive'ler:
  `shared/components/Skeleton.tsx` (`Skeleton`/`LoadingState`), `shared/hooks/useDelayedFlag.ts`
  (≈140ms, iskelet titremesini önler), `features/chat/ChatSkeleton.tsx`. `useAsync`'te
  `loading` artık `enabled` ile başlıyor; `TaskBoard`/`Schedules`/`Automations`/`FlowsPanel`/
  `MarketPanel`/`ArtifactsPanel` bu desene taşındı. Lazy panel `Suspense` fallback'leri ve
  `SessionsSidebar` iskelete geçti. Detay → `07-CHAT-UX.md` "Loading & iskelet durumları".
- **Doğrulama:** `go build ./...`, `go vet ./...`, `go test ./internal/...`, `npx tsc -b`,
  `npm run build` temiz. Canlı UI doğrulaması review aşamasında.

## Debug Sankey — köprü ön ekleri temizlendi 🧹 (2026-07-10)

- **İstek:** "Debug / Gözlemlenebilirlik" popup'ındaki Araç yürütme akışı (Sankey)
  düğümlerinde araç adlarının başındaki `mcp__tionharness_interaction__` /
  `mcp__tionharness_extended__` ön ekleri okunurluğu bozuyordu.
- **Çözüm:** `flowVizData.ts`'e `toolDisplayName(name)` yardımcısı eklendi — yalnız bu iki
  **iç claude-cli köprü** namespace'ini soyar (araçlar zaten TionHarness'in kendi köprülü
  built-in'leri; ön ek bir transport detayı). `buildToolSankey` tool etiketini bundan geçirir;
  **"En yavaş araçlar" listesi** de (`SessionDebugCard.tsx` `topTools`) aynı yardımcıyı kullanır.
  Gerçek harici MCP sunucuları (`mcp__github__…`) ön eklerini korur (hangi sunucunun
  çağırdığını ayırt eder). Frontend `tsc --noEmit` temiz.

## Default skill seed'i frontmatter-farkındalı ✅ (2026-07-10)

**Sorun (global tier denetiminde bulundu):** `EnsureDefaults` sürüm-farkındalıydı ama
bütün-dosya hash'iyle çalışıyordu. Uygulama görünürlük frontmatter'ını yerinde yazdığı
için (`access`/`group`/`auto_summary`/`name_only`/`summary_only` — ör. Skills ekranında
tek bir paylaşım/tier değişikliği) dosya hem gömülüden hem son-sevk hash'inden ayrışıyor
→ "user edit" sayılıp **sonsuza dek donuyordu**; sevk edilen gövde güncellemeleri bir
daha ulaşmıyordu (canlı örnek: 12/12 global default donmuştu, `tionharness-settings`
kaldırılmış compaction ayarlarını belgelemeye devam ediyordu).

**Çözüm:** SKILL.md dosyaları için **gövde-ayrı muhasebe**:

- `.shipped-versions.json` v2: `{files: {...}, bodies: {...}}` — `files` bütün-dosya
  (eski semantik, frontmatter değişikliklerini de sevk eden tam-tazeleme yolu önce
  denenir), `bodies` yalnız SKILL.md gövdesi (frontmatter hariç, `splitFrontmatter`).
  Legacy düz map `files`'a katlanarak okunur (`loadShippedManifest`).
- Yeni kural: bütün-dosya eşleşmezse gövde karşılaştırılır — gövde gömülüyle aynıysa
  yalnız kayda geçirilir (unfreeze bootstrap); gövde son-sevk gövdesiyle aynıysa
  **frontmatter verbatim korunarak** yeni gövde altına yazılır (`rebuildSkillFile`);
  ikisi de değilse gerçek kullanıcı düzenlemesi → dokunulmaz.
- Dosyalar: `internal/skills/defaults.go` (algoritma), yeni
  `internal/skills/defaults_manifest.go` (manifest v2 + `skillBody`/`rebuildSkillFile`).
- Testler: yeni `defaults_test.go` — gövde-tazeleme (kullanıcı frontmatter'ı altında),
  kullanıcı gövde düzenlemesi korunur, yalnız-frontmatter değişikliğinde unfreeze
  kaydı, legacy düz manifest yükleme, rebuild round-trip; mevcut `store_test.go`
  seed/pristine testleri yeni API'ye uyarlandı. `go test ./internal/...` 811 yeşil.
- Bayat `tionharness-doc-improver` cümlesi ("EnsureDefaults never overwrites") repo
  default'unda + global kopyada düzeltildi.

**Etki:** Mevcut kurulumda gövdeler şu an gömülüyle eşit (elle senkronlandı) →
ilk çalıştırmada `bodies` kayıtları kendini tohumlar, sonraki her sevk gövdeyi
kullanıcı frontmatter'ına dokunmadan tazeler. Yeni davranış rebuild + restart ister.

## Doküman bakım turu ✅ (2026-07-10)

Git geçmişiyle (özellikle compactor kaldırma + memory kaldırma + debug-viz eklemeleri)
dokümanlar senkronlandı:

- **`38-SESSION-DEBUG.md`:** eksik "İş akışı görselleştirmeleri" bölümü eklendi
  (`viz/`: ToolSankey · ConcurrencyTimeline · PromptCacheEvents · SelfHealingEvents ·
  HookActivity); yeni olay tipleri (`repair`/`guardrail`/`lesson`/`epoch`) +
  `HookID`/`Calls` alanları; debug'ın ayrı `SessionDebugModal`'a taşındığı işlendi;
  kaldırılan dream-cycle reflektörü işaretlendi; "Sırada" listesi tazelendi.
- **`00-GENEL-BAKIS.md`:** dizine eksik 5 kayıt eklendi (50, 53-SOURCE-TEMPLATES,
  55, 57, MALIYET-DUSURME-PLANI); 17 açıklaması harici rtk/sqz devrini yansıtıyor;
  53 numara çakışması nota bağlandı; bayat "Sıradaki (07-03)" satırı 07-10 açık
  kalemleriyle yenilendi (P5 memory tool + 55 UI boşlukları, sqz byte-ölçümü,
  batching n≥5 kıyası, code-exec Faz 4/5, araç backlog'u).
- **Silinmiş compactor referansları:** `55-API-NATIVE` (P1 hedef listesi + uygulama
  notu) ve `50-CACHE-PARITE` (Sistem A/B "tamamlayıcı" cümlesi) kaldırma notuyla
  düzeltildi.
- **Kaldırılmış core-memory kalıntıları:** `11-INTERACTION-MCP` (CLI köprüsü bölümü)
  ve `41-ARAC-BOSLUKLARI` (madde 7 son cümle) tarihsel işaretlendi.
- **Skill `tionharness-project`:** `compact/`+`compactor`+`reflector`+memory araç
  referansları temizlendi (yerine `lessons`); token-optimizasyon maddesi "built-in
  sıkıştırma kaldırıldı → harici rtk/sqz" olarak yeniden yazıldı.
- **Default skill `tionharness-autonomous-ops`:** Guardrails bölümündeki bayat
  "per-agent budgets (`daily_call_limit`/`daily_token_limit`)" maddesi (limitler
  2026-07-01'de kaldırılmıştı) global otonomi-pause freni + kullanım ölçümü
  (`guardedComplete`) olarak düzeltildi. Not: `~/.tionharness/skills`'teki global
  kopyalar `access/group/summary_only` frontmatter eklendiği için EnsureDefaults
  tarafından "user edit" sayılıp bir daha tazelenmiyor — düzeltme global kopyaya
  elle de uygulandı.

## Debug: Hook / token-optimizer aktivite göstergesi ✅ (2026-07-10)

"rtk/sqz tasarrufu Bütçe/Debug'da gözükür mü?" sorusunun ürün cevabı. **Gerçek tasarruf
(byte) gösterilemez** çünkü rtk/sqz **PreToolUse** hook'u — komutu yeniden yazıyorlar,
araç zaten sıkışmış çıktı üretiyor; TionHarness sıkışmamış baseline'ı hiç görmüyor → delta
yok (built-in sıkıştırma muhasebesi de 2026-07-10'da kaldırıldı). Bunun yerine **aktivite
göstergesi** eklendi:

- **Backend:** `db.DebugEvent`'e `HookID` alanı; `hooks.go`'daki 6 hook emit noktası
  (pre/post/lifecycle · başarı+hata) artık ateşleyen hook'un id'sini yazıyor → `type=hook`
  debug olayları hook-başına atfedilebilir. Test: `hookdebug_test.go` (fire → journal → read).
- **Frontend:** yeni `sessions/viz/HookActivity.tsx` — `type=hook` olaylarını `hookId`'ye göre
  gruplayıp `\brtk\b`/`\bsqz\b` (backend probe aynası) ile rtk/sqz/other sınıflar; hook başına
  ateşleme sayısı + araç dağılımı (`tool×N`) + hata sayısı gösterir. `SessionFlowViz`'e
  "Hook / token-optimizer aktivitesi" bölümü olarak bağlandı (hooks listesini `/api/hooks`'tan
  çekip atıf için geçiriyor).
- **Dürüstlük:** UI açıkça "byte tasarrufu değil, aktivite" der; ayrıca **yalnız native turlar
  sayılır** (claude-cli turlarında hook'lar CLI içinde çalışır, journal'a düşmez) uyarısını taşır.

Canlı doğrulandı: yeni binary'de SES130 debug görünümünde bölüm render oldu, claude-cli
oturumu olduğu için dürüst empty-state gösterdi. `go test ./internal/agent ./internal/db
./internal/api` yeşil (316), `npx tsc -b` temiz. Not: gerçek byte tasarrufu istenirse yol,
sqz'yi PostToolUse output-rewrite moduna alıp `runPostToolHooks`'ta `len(önce)−len(sonra)`
ölçmek — ayrı iş.

## Token-optimizer UI callout + hook matcher düzeltmesi + API GET no-store ✅ (2026-07-10)

Token-optimizer capability'sinin (rtk/sqz) devamı — UI tarafı + bir gerçek matcher bug'ı

- canlı testte yakalanan bir cache bug'ı.

* **UI callout (`ExternalToolsPanel.tsx`):** "Token / bağlam optimizasyonu" grubunun altına
  codebase-memory callout'unun eşdeğeri "⚡ Token optimizasyonu entegrasyonu" bloğu eklendi —
  rtk/sqz hook olarak bağlıysa ajanın promptuna bilgi bloğu enjekte edildiğini, çıktının otomatik
  kısaltıldığını (kayıp değil) ve matcher'ın `Bash,PowerShell` olması gerektiğini açıklar. **Matcher
  onarımı:** bağlı bir token hook'unun matcher'ı PowerShell'i kapsamıyorsa (`coversPowerShell`,
  backend `hookMatches` aynası: boş/`*`/`PowerShell` → kapsar) ⚠ rozeti + tek-tık **"Matcher'ı
  düzelt"** butonu (`fixMatcher` → mevcut matcher'a `PowerShell`'i merge eder, diğer alanları korur,
  `updateHook` PUT). `data-testid="fix-matcher"`.
* **Gerçek matcher bug'ı (WS5 + WS1):** rtk/sqz hook'ları `matcher=Bash` ile kayıtlıydı; ajanlar
  Windows'ta `PowerShell` aracını kullandığı için hook'lar **hiç ateşlenmiyordu**. Canlı UI butonuyla
  düzeltildi → ikisi de `Bash,PowerShell`.
* **API GET no-store (`api/client.ts`):** canlı testte yakalandı — `req()` `fetch`'i default cache
  ile çağırıyordu, tarayıcı `/api/hooks` gibi GET'leri **stale servis edebiliyordu** (panel bir
  değişiklikten sonra eski veriyi gösteriyordu). `fetch(path, { cache: 'no-store', … })` eklendi;
  çağıran `init.cache` ile hâlâ override edebilir. Tüm GET domain modülleri için canlı-veri tazeliği.

Doğrulama: `npx tsc -b` temiz; Playwright ile WS5'te callout + ⚠ uyarı + "Matcher'ı düzelt" butonu
uçtan uca test edildi (tık → WS5 hook `Bash,PowerShell` → uyarı sıfırlandı). Not: canlı backend eski
binary — token-optimizer **prompt bloğunun** enjekte olması için backend yeniden derlenip başlatılmalı.

## todo_write kompakt `set` formu: durum güncellemesi tüm listeyi yeniden göndermiyor ✅ (2026-07-10)

AlgoBench dökümünde todo_write çağrıları koşu başına ~1.2k output-char tutuyordu — her
güncellemede tüm liste yeniden gönderiliyordu. Artık iki form var (tam olarak biri):

- **`todos`** — tam liste (oluşturma / metin değişikliği; eskisi gibi replace).
- **`set`** — `{"set":{"1":"completed","2":"in_progress"}}`: 1-tabanlı indeks → status.
  Sunucu önceki listeyi **todo sink'ten** yükler (`TodoSink.LoadTodos`, progress
  dosyası), birleştirir, geri persist eder. Sink yoksa veya henüz liste yoksa **yüksek
  sesle hata** verir ("send the full todos array"). Tipik güncelleme ~400 → ~40 char.
- **Trace/UI:** `set` çağrısının input'unda liste olmadığından birleşik tam liste tool
  SONUCUNDA JSON olarak döner; `todoStepItems` (trace.go + toolloop.go) input boşsa
  output'tan parse edip StepTodo kartına terfi ettirir — CLI ve native yol aynı.
- **Model tarafı:** dinamik prompt'taki "Active todo list" bloğu artık **numaralı**
  (`1. [x] …`) ve `set` formunu öğretiyor; araç açıklaması da güncellendi.
- Test: builtin_todo_test (merge + alan koruma + 6 hata senaryosu), todos_test numaralı
  format. `go build ./...` + `go test ./internal/...` yeşil (799 test).

## AlgoBench ~1.7× output farkı ayrıştırıldı: %81'i Write payload'ı, kaynak ajan personası ✅ (2026-07-10)

v6/v7 transkriptlerinde TS vs CA output-token dökümü (dedup usage; karakter bazında kategori):

- **Fark narasyon değil, üretilen dosya içeriği.** TS v7 38.0k out-char / CA v7 22.2k.
  Delta 15.8k char'ın dağılımı: **Write %81** (+12.8k), text +1.1k, `todo_write` +1.2k
  (CA hiç todo kullanmadı), shell +0.6k. Her iki tarafta da out-char'ın %94-96'sı tool_use.
- **TS aynı görevde ~%64 daha uzun kod yazıyor** (iki koşuda da tutarlı → sistematik, sampling değil):
  6 algoritma dosyası 9.9k vs 5.8k · testler 7.7k vs 4.6k (30 test vs 22) · Go portu 4 dosyaya
  bölünmüş 3.8k vs tek main.go 2.4k · compute.py 2.4k vs 1.3k (bağımsız çapraz-doğrulama kodu) ·
  REPORT.md 4.6k vs 2.6k.
- **Kök neden: benchmark ajanı AGT10'un personası** ("focused implementation engineer …
  running code to verify") + TR yanıt tercihi. Görev metinleri birebir aynı (diff'lendi);
  TS statik prefix'i (5k char, epoch sidecar'dan okundu) verbosity talimatı içermiyor.
  Runtime'ın kendi output ek yükü küçük: todo_write ~~1.2k char (~~%3).
- **Sonuç:** 1.7× fark büyük ölçüde persona-kaynaklı titizlik (daha çok test, cross-check,
  dosya bölme) — kalite/maliyet dengesi, runtime bug'ı değil. Adil kıyas için TS koşusu
  boş-persona ajanla (AGT9 CoderEmpty benzeri, thinking-off) tekrarlanmalı.
- **Adil tekrar koşu (aynı gece, AGT11 "BenchBare": boş soul/identity + thinking-off,
  v7 ile aynı sunucu binary'si, taze `bench-tionharness-bare`):** toplam 16.4k out-tok
  (0 thinking; batching var: [5,3,2,2,2,2,2]). Persona hipotezi **kısmen** doğrulandı:
  REPORT.md 4.6k→2.8k (CA seviyesi), compute.py 2.4k→2.1k, algoritma dosyaları kısaldı
  (örn. binary_search 1601→898) — ama **30 test yine yazıldı** ve Go yine dosyalara
  bölündü → bunlar persona değil. Koşu ortada bir kez kesildi (non-stream istemci koptu;
  **inflight paritesi çalıştı**, 32 adım kayıpsız persist edildi) ve devam turu ~3-4k
  char yeniden-bağlam maliyeti ekledi (8 Read + ekstra shell). Düzeltilmiş tahmin
  ~14.5-15k tok → CA'nın hâlâ ~1.4×'i. **Net:** persona ~%15-20'lik kısmı açıklıyor;
  kalan fark TS runtime append'i / model davranışı / örneklem gürültüsü — kesinleşme
  n≥5 istatistiksel seriye kaldı.

## Token-optimizer capability probe: rtk/sqz kuruluysa prompta bilgi bloğu ✅ (2026-07-10)

codebase-memory capability'siyle aynı desen: workspace'te `rtk`/`sqz` **hook ile bağlı**
ise ajanın statik prompt prefix'ine "# Token optimization active" bloğu enjekte edilir.

- **Yeni:** `internal/agent/capabilities_tokenopt.go` — `tokenOptimizerCapability`
  (registry'ye eklendi). `detectTokenOptimizers` enabled Pre/PostToolUse hook
  komutlarını `\brtk\b` / `\bsqz\b` ile tarar (kelime-sınırlı → `quirtky`/`sqzip`
  false-positive vermez). **Tespit hook komutundan**, PATH'ten değil (bağlanmamış ikili
  hiçbir şey optimize etmez → over-claim yok). Hata yutulmaz (loglanır, atlanır).
- **Blok içeriği:** hangi optimizer aktifse onu anlatır (rtk = komut proxy'si, sqz =
  çıktı sıkıştırıcı) + "kendisi çağırma, otomatik" + "çıktı kısaltılmış olabilir, kayıp
  değil" + **matcher kapsam notu**.
- **Kritik gerçek (SES130 kök nedeni):** WS5'in iki hook'u da `matcher=Bash`; ajanlar
  Windows'ta `PowerShell` aracını kullandığı için ikisi de **hiç ateşlenmiyordu**. Blok
  bunu açıkça söyler ("eşleşen `Bash` aracını token-ağır komutlar için tercih et").
- Test: `capabilities_tokenopt_test.go` (WS5-şekli iki-hook, kelime-sınırı, wildcard-no-scope).

Doğrulama: `go build ./...` + `go vet ./internal/agent` + `go test ./internal/agent ./internal/tools ./internal/api` yeşil (449 test). Detay `54-CAPABILITY-PROBE.md`.

## Oturum incelemesi düzeltmeleri: PowerShell UTF-8 + lazy-tool uyarısı + codebase-memory hint ✅ (2026-07-10)

Bir board-otomasyon oturumunun (SES130) debug/transkript analizinden çıkan üç somut sürtünme
noktası düzeltildi:

- **PowerShell 5.1 mojibake (kök neden + fix).** `internal/tools/builtin_shell.go` PowerShell
  aracını `powershell.exe -NoProfile -NonInteractive -Command …` ile hiçbir kodlama zorlaması
  yapmadan çalıştırıyordu. Windows PowerShell 5.1 (pwsh **yoksa** kullanılan fallback) konsol
  çıktısını + dosya-okuma cmdlet'lerini (Get-Content/Select-String/Import-Csv) legacy ANSI/OEM
  code-page'iyle çözer → **UTF-8-BOM'suz** bir Türkçe dosya cp1254 olarak okunur ve baytlar
  Go'ya geçersiz UTF-8 olarak ulaşır (ör. `talimatlar��`). Ajan bu oturumda ws-settings ve
  agent-soul'ları **bozuk** okumuştu. **Yeni:** `internal/tools/builtin_shell_encoding.go` —
  yalnızca legacy host'a (pwsh'e değil) bir UTF-8 prelude enjekte eder: `[Console]::OutputEncoding`
  - `$OutputEncoding` = UTF-8 (BOM'suz) ve `$PSDefaultParameterValues` ile Get-Content/Select-String/
    Import-Csv okuma default'u `utf8`. **Yazma davranışına dokunulmaz** (global `*:Encoding` set
    edilmez → BOM regresyonu yok). Canlı doğrulandı: prelude'suz mojibake, prelude'lu çıktı orijinal
    UTF-8 baytlarıyla birebir. Test: `builtin_shell_encoding_test.go`.
- **Lazy-tool same-batch reddi (#1).** İki-tier claude-cli köprüsünde extended (deferred) tier
  boş başlar, `activate_tools`'tan **sonraki** turda büyür; ajan `activate_tools`'u aktive edilen
  aracı **aynı yanıtta** çağırırsa CLI `No such tool available` döner. Araçları eager CORE'a taşımak
  bilinçli gateway-bütçe tasarımını (ve `TestInteractionTierSplit` sözleşmesini) bozacağı için
  **bütçe-nötr** çözüm: `activate_tools` açıklaması + çalıştırma sonucu artık "aktive edilenler
  yalnız **SONRAKI** adımda gelir, bu yanıtta çağırma" diye açıkça uyarır (`builtin_activate.go`).
- **codebase-memory hint güçlendirildi (#4).** `capabilities.go codebaseMemoryGuidance` artık raw
  shell grep'lerini (Select-String / Get-Content -Recurse / grep / findstr) ilk hamle olarak
  **yasaklar**, "yalnız indeks cevap veremezse son çare" der. (SES130'da ajan ~13 ardışık
  Select-String taraması yapıp indeksi az kullanmıştı.)

Ayrıca aynı analizde netleşen **sqz/rtk sorusu** (kod değişikliği gerektirmez): sqz TionHarness'e
gömülü değil, `token` kategorisinde bir **PostToolUse hook** olarak Ayarlar→Hooks'tan bağlanır;
rtk ise ajanın Bash ile komutu sarmalamasıyla çalışır. İkisi de bu workspace'te bağlı/çağrılmadığı
için devreye girmiyordu; ayrıca claude-cli per-workspace `claude-home` kullandığından kullanıcının
global `~/.claude` rtk hook'u **devralınmaz** (CLI hook'ları yalnız TionHarness'in kendi hook DB'sinden
`--settings`'e yazılır). Detay `17-TOKEN-OPTIMIZASYON.md`.

Doğrulama: `go build ./...` + `go test ./internal/tools ./internal/agent ./internal/api` yeşil (445 test).

## Skills + Artifacts: sürükle-bırak ile grup değiştirme ✅ (2026-07-10)

Skills ve Artifacts listelerinde bir kartı başka bir grubun üzerine sürükleyip bırakmak,
o kartın `group` alanını kalıcı olarak değiştiriyor. Backend'e **sıfır** değişiklik: mevcut
`PUT /api/skills/{slug}/group` ve `PUT /api/artifacts/{id}/group` uçları (ve `Artifact.Group`
alanı) zaten vardı — kart notundaki "veri modeline group eklenmesi gerekebilir" varsayımı
yanlış çıktı.

**Yeni:** `frontend/src/shared/hooks/useGroupDnD.ts` — iki panelin paylaştığı **native HTML5
DnD** hook'u (`useGroupedList` ile aynı desen; `dnd-kit`/`react-dnd` gibi yeni bağımlılık
**eklenmedi**, TaskBoard'ın kanban sürüklemesiyle aynı yaklaşım). Hook `itemProps(item)` +
`groupProps(groupName)` prop-paketleri döndürür; paneller bunları karta ve grup sarmalayıcısına
yayar.

Davranış kararları:

- **No-op garantisi:** sürüklenen kartların hepsi zaten hedef gruptaysa `dragover` üzerinde
  `preventDefault()` **çağrılmaz** → tarayıcı bırakmayı reddeder → `drop` olayı hiç doğmaz →
  API isteği de gitmez. (Bayrakla bastırmak yerine tarayıcıya reddettiriyoruz.)
- **Dosya yükleme regresyonu yok:** sürükleme kendi MIME tipini taşır
  (`application/x-tionharness-group-item`), ArtifactsPanel kökündeki upload drop zone ise
  `Files` tipine bakar. Ayrıca kart `onDrop`'u `stopPropagation()` çağırır → kart bırakması
  asla upload zone'una sızmaz.
- **Seçim farkındalığı:** sürüklenen kart mevcut çoklu seçimin içindeyse **tüm seçim** taşınır,
  değilse yalnız o kart.
- **Hata yutulmaz:** `onMove` hatasında `onError(...)` gösterilir ve her iki yolda `reload()`
  çalışır → liste yarım uygulanmış bir taşımada kalmaz, sunucu gerçeğine döner.
- **Katlı grup** başlığına bırakma çalışır (drop hedefi başlık + gövdeyi kapsayan sarmalayıcı).
- **Grupsuz** bucket'ına bırakmak `group=""` gönderir.

Test kancaları: grup sarmalayıcısında `data-drop-active`, kartta `data-dragging`
(mevcut `data-testid="skills-group-header"` / `artifacts-group-header` korundu).

Doğrulama: `go build ./...` + `go vet ./...` + `go test ./internal/...` temiz (backend'e
dokunulmadı, regresyon güvencesi); `npx tsc -b` + `npm run build` temiz. Not: repoda frontend
test altyapısı (vitest) **yok**, bu yüzden birim testi eklenmedi; `npm run lint` repo genelinde
zaten kırmızı (86 hata) — yeni hook sıfır bulgu veriyor, dokunulan iki panelin bulguları
değişikliğimizden önce de vardı.

## Built-in araç-çıktısı sıkıştırması TAMAMEN kaldırıldı → harici hook'lara devredildi ✅ (2026-07-10)

Az önce Sistem B (LLM özeti) kaldırılmıştı; bu adımda **Sistem A (deterministik) de
kaldırıldı** — TionHarness artık **hiçbir built-in araç-çıktısı sıkıştırması içermez**.
Sıkıştırma tamamen **harici araçlara** devredildi: `sqz` (PostToolUse hook,
`agent/toolloop.go`'daki tek shrink yolu) ve `rtk` (komut-katmanında agent tarafından
`rtk <cmd>`). Gerekçe: server-tarafı `clear_tool_uses`/API-native compaction + retrieval
transcript baskısını zaten karşılıyor; ham çıktı kırpmasını bakımı-kolay harici bir
hook/CLI katmanına taşımak built-in bir alt sistem tutmaktan temiz. **Uyarı:** artık
kullanıcı bir hook bağlamazsa araç çıktıları sıkıştırılmadan modele gider (tek koruma
tool'ların kendi 64 KB hard-cap'i; hata/boş sonuçlar aynen geçer).

**Silinenler:** `internal/tools/compact` paketi (compact.go + test); `agent/compactor.go`
tümüyle + `toolloop.go` çağrısı; tunables `compactDeterministic/MaxLines/MaxBytes` +
`SetToolCompaction`/`CompactDeterministic`/`CompactMaxLines`/`CompactMaxBytes(For)` +
tüm **bütçe-ölçekleme aparatı** (`contextBudgetTokens`/`SetContextBudget`/
`ContextBudgetTokens`/`budgetScaleFor`/`budgetScaleLocked`/`defaultContextBudgetTokens` +
`DefaultCompactMax*`) + `server.go` iki çağrı; settings `CompactToolOutput/MaxLines/MaxBytes`
(struct/DTO/patch/default/clamp); DB `Usage.CompactSavedBytes` + `SessionUsage.CompactSavedBytes`

- `AddCompactionSavings`/`AddSessionCompactionSavings`; API `budget/usage/session_usage`
  `compactSavedBytes`; frontend `ContextPanel` sıkıştırma bölümü + `BudgetPanel` Tasarruf Merkezi
  (tek Prompt-cache hücresi kaldı, "Sıkıştırma" trend metriği düştü) + `SessionUsageCard`/
  `SessionDetailPanel`/`SettingsPanel` + `types/{settings,usage,agent}.ts`; testler.
  `KindCompact`/`UsageKindCompact` **paylaşımlı** (ana konuşma compaction'ı) → korundu.
  Doküman `17-TOKEN-OPTIMIZASYON.md` harici-delegasyon anlatısına dönüştürüldü, `tionharness-settings`
  skill'i güncellendi. `go build ./...` + `go test` (agent/db/settings/api **312 test**) +
  frontend `tsc --noEmit` temiz.

## Araç-çıktısı LLM-özeti ("Sistem B") tamamen kaldırıldı ✅ (2026-07-10)

Araç çıktısı token optimizasyonundaki opt-in **LLM intent-aware özet** geçişi uçtan uca
silindi; yalnız **deterministik (Sistem A, ücretsiz kural tabanlı)** sıkıştırma kaldı.
Gerekçe: server-tarafı `clear_tool_uses` / API-native compaction (eskiyen/taşan sonuçlar)

- retrieval katmanı bu ihtiyacı zaten karşılıyordu; ekstra ucuz-model çağrısı + gecikme +
  tek-sağlayıcı sınırı değmiyordu (Sistem A ise sağlayıcıdan bağımsız giriş-noktası kalkanı,
  kalıyor). Silinenler: `compactor.go` B dalı + `summarizeToolOutput`/`toolSummarySystemPrompt`;
  tunables `CompactLLM*`/`CompactModel` + `SetToolCompaction` imzası (artık 3 arg); settings
  `CompactLlmSummary`/`CompactLLMThreshold`/`CompactModel` (struct/DTO/patch/clamp/default);
  DB `Usage.CompactSavedBytesLLM` + `AddLLMCompactionSavings`/`AddSessionLLMCompactionSavings`;
  API `budget.go`/`usage.go`/`session_usage.go` `compactSavedBytesLLM` alanları; frontend
  `ContextPanel` "Sistem B" bölümü + `BudgetPanel` Tasarruf Merkezi 3→2 hücre + trend tek seri
- `SessionUsageCard`/`SessionDetailPanel`/`SettingsPanel` + `types/{settings,usage,agent}.ts`.
  `KindCompact`/`UsageKindCompact` **paylaşımlı** (ana konuşma compaction'ı) → dokunulmadı.
  Doküman: `17-TOKEN-OPTIMIZASYON.md` tek-sisteme indirildi, `tionharness-settings` skill güncellendi.
  `go build ./...` + `go test` (agent/db/settings 226 test) + frontend `tsc --noEmit` temiz.

## Cross-session bloğu artık aktif oturumları OTOMATİK göndermiyor ✅ (2026-07-10)

Dinamik bağlama enjekte edilen "Other sessions in this workspace" bloğu
(`internal/api/sessions_context.go` `sessionsContextBlock`) artık **yalnız geçmiş
(non-active) chat oturumlarını** listeliyor. **Aktif (canlı) oturumlar otomatik
gönderilmiyor** — sürekli değişip gürültü ekledikleri için ajan bunları **kendi
inisiyatifiyle** `list_sessions` aracıyla çeker (manuel kontrol). Blok başlığı
buna göre güncellendi ("Active (live) sessions are NOT listed here; call the
list_sessions tool..."), `Active:` bölümü ve kullanılmayan `maxActiveSessionsInBlock`
sabiti kaldırıldı. Tek kaynak: chat yolu (`composeTurnRequest`) + headless
(`buildAgentDynamicPrompt`) aynı fonksiyonu çağırdığı için ikisi de otomatik
hizalı. Workspace ayarları (`SessionContextEnabled`/`EveryTurn`/`RecentCount`)
aynen geçerli — yalnız aktif oturum satırları düştü. Test `TestSessionsContextBlock`
güncellendi (aktif oturum yok, yalnız `Recent:`).

**Manuel kontrol için statik ipucu + frontend temizliği:** `list_sessions` aracının
açıklaması (`internal/tools/builtin_sessions.go`, statik/cache'li şema) artık aktif
oturumların bağlama otomatik enjekte EDİLMEDİĞİNİ ve devam eden işi görmek için bu
aracın çağrılması gerektiğini söylüyor. Frontend `WorkspacePanel` "Session bağlamı"
toggle hint'i "aktif + son" yerine "son (geçmiş) sessionlar; aktif olanları
list_sessions ile kendi çeker" olarak güncellendi.

## Batching bulgusu REVİZE: deterministik değil, olasılıksal ✅ (2026-07-10, v6/v7)

v6/v7 koşuları önceki iki keskin hipotezi (yalnız-sürüm, katı "think XOR batch")
**olasılıksal** bir resme çevirdi: TS v6 (2.1.205, 8 thinking bloğu) **batch'ledi**;
CA v6 (2.1.197, 0 thinking) **serial** koştu. 16 koşu/probe'luk toplam örneklem
yine güçlü eğilimler gösteriyor — 2.1.197: 6/7 batch · 2.1.203/205+thinking: 1/5
batch · 2.1.205+thinking-off: 2/2 batch — yani sürüm ve thinking batching
OLASILIĞINI kuvvetle etkileyen kovaryatlar, ama garanti değil. Maliyetin gerçek
belirleyicisi **batch⇔ucuz / serial⇔pahalı**; ikinci kalıcı bulgu: TS aynı görevde
CA'dan **~1.7× fazla output token** üretiyor (18k vs 10k — prompt/verbosite
kaynaklı, ayrı optimizasyon adayı). Kesinleşen tek şey: v7'de `thinkingLevel`
Kapalı → **0 thinking** (aşağıdaki DisableThinking bağlaması canlıda doğrulandı).
Sağlıklı kıyas için sıradaki adım: koşul başına n≥5 tekrarla batching-oranı +
maliyet dağılımı.

## ThinkingLevel "Kapalı" → CLI'da MAX_THINKING_TOKENS=0 (batch dönüşü) ✅ (2026-07-10)

"Think XOR batch" bulgusunun ürün çözümü: `Request.DisableThinking` (yeni alan) —
`completeTracedInner` `thinkingBudgetForLevel(agent.ThinkingLevel)==0` ise set
eder (UI'daki **Kapalı** = `""`, per-turn `off` dahil); claude-cli **her iki
yolda** (one-shot `Complete` + persistent-session launcher) subprocess'e
`MAX_THINKING_TOKENS=0` env'i enjekte eder → thinking tamamen kapanır, Claude
Code ≥2.1.203'te paralel araç batch'leri geri gelir (probe kanıtı: gerçek
AlgoBench 2.1.205'te `[6,6,1,1,4,1]`). Native sağlayıcılar alanı yok sayar
(orada budget 0 zaten kapalı) — yani bu aynı zamanda bir **parite düzeltmesi**:
"Kapalı" seçimi artık CLI'da da gerçekten kapalı (eskiden CLI kendi adaptive
default'una düşüyordu). `cliEffortLevel("off")` low→high düzeltildi (thinking
zaten kapalı; düşük effort basit-görev batch'ini riske atar). **Frontend:**
Ajan ayarlarında thinking seçicisinin altı güncellendi — seviyenin artık CLI
`effortLevel`'ına eşlendiği yazıyor; **claude-cli + Kapalı** seçiliyken ⚡'lı
bilgi kutusu "think XOR batch" davranışını, maliyet kazancını ve takası anlatır
(`AgentSettingsForm.tsx`). Test: `TestCLIEffortLevel` güncel; suite 791/29
paket + tsc yeşil. Benchmark ajanını **Kapalı**'ya alıp v6'yı 2.1.205'te koşmak
artık pin gerektirmiyor.

## Paralel araç batch'leri chat'te gruplu görünüyor ✅ (2026-07-10)

Tek provider cevabında gelen çoklu paralel tool çağrıları artık UI'da tek küme
olarak render ediliyor: `TurnStep.Batch` (tur içi 1-tabanlı grup id, `omitempty`)
— **native döngü** her çok-çağrılı cevaba grup id atar (`toolloop.go` `batchSeq`;
tool kartı + izin/guardrail hataları dahil), **claude-cli yolu** stream-json'da
aynı API mesaj id'sini paylaşan `tool_use` event'lerini gruplar (`cliMessage.ID`

- parser `curMsgID/curMsgTools/batchSeq`; CLI bir mesajı bloklara bölerek
  yayınladığı için 2. çağrıda geriye dönük damgalama). Frontend: `TurnSteps.tsx`
  ardışık aynı-batch adımları "⚡ N paralel araç çağrısı (tek istekte)" başlıklı
  vurgulu çerçevede kümeler (tekil render'a düşen grup düz satıra döner);
  `renderStep` yardımcıya çıkarıldı. Test: `claudecli_batch_test.go` (bölünmüş
  mesaj + tekil + tek-event'te 3'lü senaryosu); suite 801/30 paket + tsc yeşil.
  effortLevel düzeltmesiyle birlikte batching görünürlüğü de tamam — v4
  benchmark'ta gruplar chat'te doğrudan izlenebilir.

## Benchmark serileşmesinin GERÇEK kök nedeni: `effortLevel` ✅ (2026-07-09, akşam)

Kontrollü probe deneyleri (aynı CLI 2.1.205, aynı mini görev "4 dosya yaz"):
global `~/.claude` home altında **4'lü paralel Write batch**, WS1 workspace
claude-home altında **[1,1,1,1] serileşme**; WS1 settings'e `"effortLevel":"high"`
eklenince **batch geri geldi**. Yani fail sürümün kendisi değil: **2.1.203+
`effortLevel` ayarlanmamışken (default effort) opus-4-8 tool çağrılarını
serileştiriyor**; kullanıcının global home'unda `effortLevel: high` olduğu için
kendi Claude Code kullanımı etkilenmiyordu, TionHarness workspace home'ları minimal
settings ile default'a düşüyordu. `--disallowedTools`/`--allowedTools`/
`--dangerously-skip-permissions`/`stream-json` tek tek elendi (hepsi batch'li).
the external agent projects 2.1.197'ye pinli olduğu için hiç etkilenmedi. Maliyet zinciri:
default effort → serileşme → çağrı başına prefix re-read + cache-write primi →
$0.65→$1.51. **Uygulanan katman + SONRADAN DÜZELTİLEN beklenti (v5 sonrası,
2026-07-10 gece):** `ensureClaudeHomeEffortLevel` (workspace açılışında claude-home
settings'e eksikse `effortLevel`, kullanıcı `~/.claude`'undan kopya/`high`) +
`writeCLISettings` per-turn `effortLevel` (`cliEffortLevel`: ThinkingLevel eşlemesi)
uygulandı ve kalıcı (zararsız + ThinkingLevel'a CLI karşılığı kazandırır). ANCAK
v5 koşusu + belirleyici probe (gerçek AlgoBench promptu + `effortLevel:high` +
2.1.205, `--max-turns 6`) gösterdi ki **effortLevel yalnız basit/thinkingsiz
görevlerdeki serileşmeyi düzeltiyor; karmaşık (thinking tetikleyen) görevlerde
2.1.203+ effort değerinden bağımsız serileştiriyor** (ayarlar/hook/deny/MCP/
append-system-prompt tek tek probe'la aklandı; medium bile trivial görevde
batch'liyor). **NİHAİ KÖK NEDEN (aynı gece, probe'la kanıtlı):** 2.1.203+
**thinking aktifken paralel tool çağrısı yapmıyor** (think XOR batch; eski
CLI'lar — CA 2.1.197, TS 2.1.202 — hem düşünüp hem batch'liyordu).
`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` YETMİYOR (yine thinking üretti,
serial); **`MAX_THINKING_TOKENS=0`** thinking'i tamamen kapatınca gerçek
AlgoBench 2.1.205'te **[6,6,1,1,4,1] batch imzasına döndü** (v1 paritesi). Yani
iki geçerli çözüm: (a) CLI'ı ≤2.1.202'ye pinle (thinking + batch birlikte), veya
(b) 2.1.205'te `MAX_THINKING_TOKENS=0` env'i (batch var, thinking yok —
kalite/maliyet takası ajan bazında seçilmeli; TionHarness'e ThinkingLevel="off" →
env enjeksiyonu olarak bağlanabilir, henüz bağlanmadı). Kaynaklar: claude-code
issue #24131 (paralel Write sınırı, closed-not-planned), #65785 (`--thinking
disabled` dokümantasyonu), topluluk: MAX_THINKING_TOKENS/effort yazıları.
Test: `claudehome_effort_test.go`; suite 800/30 paket yeşil.

## Benchmark v1↔v2 maliyet farkı analizi: fail CLI sürümüydü ✅ (2026-07-09)

AlgoBench v2'de (SES125) maliyet artışının nedeni "file-mutation verifier yazımları
serileştirdi" sanılıyordu — **yanlış atıf**. CLI transkript karşılaştırması
(claude-home): v1'de 6+7 Write **tek API cevabında paralel tool_use** (usage
değerleri özdeş), v2'de her Write ayrı çağrı (usage monoton artan). Tek değişen:
**claude-cli 2.1.202 → 2.1.203** (otomatik güncellenmişti); verifier CLI-native
`Write`'ları hiç görmez (delegasyon modunda fs araçları köprülenmez), lessons boştu,
epoch stale notu yoktu. Cache tarafı kusursuz doğrulandı: read monoton 28k→46k,
sıfır break (Prompt Epoch CLI yolunda da çalışıyor). Yan bulgular: (a) v2 turu
TionHarness'e persist edilmedi — dev rebuild'i tam tur biterken geldi (kasıtlı
restart) ve **non-stream `/api/chat` inflight sidecar yazmıyordu** → süreç
ölümünde tur sessizce kaybolur; (b) makinedeki CLI 2.1.205'e güncellendi —
benchmark tekrarı aynı sürümle koşulmalı.

## Non-stream chat'e inflight paritesi ✅ (2026-07-09)

Yukarıdaki (a) bulgusunun düzeltmesi: `/api/chat` (non-stream) artık streaming
yoluyla aynı crash-korumasını taşıyor (`internal/api/chat.go` `inflightRecorder`):
reply id önceden ayrılır (`WithTurnID` debug paritesi dahil), tur
`CompleteWithToolsStream` ile koşup adımlar throttle'lı (600ms) `inflight.json`
snapshot'ına düşer → süreç ölümünde boot recovery kısmi turu materyalize eder.
Provider hatasında da stream paritesi: kısmi iz **interrupted mesaj** olarak
persist edilir (üretilmiş araç adımları/text kaybolmaz; HTTP sözleşmesi
değişmedi, 502), sidecar temizlenir; persist hatasında sidecar bilerek bırakılır
(recovery ağı). Test: `chat_inflight_test.go`; suite 797/30 paket yeşil.

## Board kartı: çoklu artifact referansı + dosya-drop ile ekleme ✅ (2026-07-09)

Kartlar artık workspace artifact'larına **birden fazla referans** taşıyabiliyor
(`Task.ArtifactIDs []string`, `models_task.go`; API create/update thread'lendi,
`store_task.go` UpdateTask kopyalar). Artifact yaşam döngüsünden bağımsız — kart
silmek referansı düşürür, artifact'ı silmez; çözülemeyen id'ler UI'da atlanır.

- **Frontend:** yeni `TaskArtifactRefs.tsx` bileşeni (chip listesi + "Var olan"
  arama-picker'ı + dosya sürükle-bırak alanı) TaskFormModal'a "Ekler" bölümü olarak
  eklendi. Dosya bırakınca: `/api/uploads` (bucket = kart id / `board`) → `createArtifact`
  (`sourcePath` + `origin:manual`) → id karta eklenir. Mevcut artifact upload→artifact
  akışı (`artifactKindForUpload`, ArtifactsPanel) yeniden kullanıldı.
- **Board kartı:** OS'ten dosya sürükleyip **doğrudan kart üzerine bırakma** → aynı
  upload+artifact+link akışı (`attachFilesToTask`, `TaskBoard.tsx`; `dataTransfer.files`
  olan drop dosya-ekleme, olmayan drop hâlâ sütuna taşıma). Kartta `📎 N` ek sayacı rozeti.
- **E2E doğrulama (canlı 8090):** mevcut-referanslı task create + multipart upload→artifact→link
  reload sonrası `artifactIds=['ART34','ART44']` olarak kalıcı.
- **Not:** frontend dist build'i şu an **ilgisiz** iki kullanıcı-WIP hatasıyla bloklu
  (`useAppEvents.ts:214`, `RunView.tsx:89`); bu özelliğin dosyaları tip-temiz.

## Navbar "çalışıyor" göstergesi workspace'ler arası sızıyordu 🐛 (2026-07-09)

- **Sorun:** Bir workspace'te oturum çalışırken navbar'daki "çalışıyor" (Aktivite/
  Executions) göstergesi doğru şekilde yanıyordu; ancak başka bir (boşta) workspace'e
  geçilince orada da iş devam ediyormuş gibi gösterge yanıyordu.
- **Kök neden:** `internal/api/activity.go` `handleActivity`, in-flight chat turlarını
  **server-geneli** `s.runs` registry'sinden (tüm workspace'ler ortak) topluyor. Döngüde
  `st.Executions = true`, session'ın bu workspace'e ait olup olmadığı `wsp.DB.GetSession`
  (workspace-scoped, izole store) ile doğrulanmadan **önce** set ediliyordu. Yabancı bir
  workspace'in akan turu global registry'de bulunduğundan, GetSession başarısız olsa (`continue`)
  bile bayrak zaten yanmış oluyordu.
- **Çözüm (kaynakta workspace-scope):** `chatRun`'a `workspaceID` alanı eklendi
  (`register` artık `id, sessionID, workspaceID, cancel` alır; chat yolu `ws(r).ID`, otonom
  yol `rt.WorkspaceID()` geçer). `chatRuns.activeSessionIDs(workspaceID)` verilen workspace'e
  filtreler (`""` = filtresiz, yalnız legacy/test). Böylece server-geneli registry'nin sızıntısı
  **kaynağında** kesilir; 4 çağıran (`activity`/`executions`/`graph`/`sessions.handleActiveSessions`)
  artık `wsp.ID` geçer → yabancı workspace'in turları hiçbir görünürlükte (busy dot, reload
  "thinking" restore, ağ grafiği, executions feed) görünmez. Diğer bayraklar (`Task`/`Flow`/
  `Schedule`) zaten `wsp.DB` üzerinden scoped'du. Kilit testi: `activity_scope_test.go`
  (`TestActiveSessionIDsWorkspaceScope`); tüm api paketi (90 test) yeşil.
- **Panel gözden geçirmesi — 2 ek frontend sızıntısı (aynı sınıf):** backend fix'i poll
  yolunu kapatsa da frontend'de iki yol daha aynı semptomu üretiyordu.
  (1) `App.tsx` `busyViews = useActivity(activeWorkspaceId, chat.streamingSessions.size > 0)`:
  `useChatStream` App seviyesinde yaşar, workspace switch'te **remount olmaz** → A'da başlayan
  stream'in session'ı Set'te kalır, B'ye geçince B'nin chat noktası yanardı. Düzeltme:
  `chatBusyLocal = ctl.chatSessions.some(s => chat.streamingSessions.has(s.id))` (yalnız aktif
  workspace'in oturumları; diğer aynı-workspace stream'leri zaten poll kapsar).
  (2) `useAppEvents.onEvent` → `bumpSignalsForEvent(e)` yorumu "workspace-match gate'inde" dese de
  koda göre gate'in **dışındaydı** → yabancı workspace olayı `activity`/`executions`/`network`
  panellerini boşuna re-fetch'e zorlar + göstergeyi kısa süre yakabilirdi. Düzeltme:
  `if (!e.workspaceId || e.workspaceId === getActiveWorkspace())` ile sarıldı (badge/toast yolu
  gate dışında kalır). **Kasıtlı global kaldı:** Logs ekranı (`logs.go` "application + all
  workspaces"), `events` SSE feed'i (her olay `workspaceId` taşır, frontend filtreler),
  `grants`/`sessionRunInfo` (session-ID anahtarlı). Frontend `tsc --noEmit` temiz.

## Flow-run görüntüleyicisi (RunView) canlı node ilerlemesi ✅ (2026-07-09)

- **Bağlam:** Aktivite ekranı canlı transkript aldıktan sonra "aynı canlılığı akış
  koşu görüntüleyicisine de" istendi. **Mimari bulgu:** akış node'ları session turu
  DEĞİL — her agent node'u `f.rt.complete(...)` ile session'sız tek provider çağrısı
  yapar (`session_step` yaymaz, tool-adım granülerliği yoktur); flow session'ı transkript
  turunu koşu **bitince** post-hoc yazar. Dolayısıyla stepBus/`FlowRun.SessionID` yolu
  buraya oturmuyordu. Doğru canlılık sinyali: engine'in zaten yaydığı `orchestration.NodeEvent`
  (per-node `start`/`done`+çıktı/`error`). RunView bunu kullanmıyordu → yalnız 3sn
  `listAllFlowRuns` polling ile canlı-kördü; ayrıca node olayları sadece koşuyu başlatan
  HTTP istemcisine gidiyordu (otonom/scheduled koşularda `obs=nil` → hiç canlı olay yok).
- **Çözüm (session_step deseninin flow eşi):** (a) `events.Event`'e `Node json.RawMessage`
  alanı; (b) `driveFlow` artık observer'ı **her zaman** sarmalar → her `NodeEvent`'i
  `emitFlowNode(runID, flowID, ev)` ile global bus'a yayınlar (`flow_node` tipi, `Target.flowRunId`),
  başlatıcının `obs`'u varsa yine çağrılır → **tüm** koşular (UI/otonom/scheduled) canlı olay
  yayar; (c) API SSE `flow_node` → ayrı `flownode` kanalı (notify/badge/toast yolunu kirletmez);
  (d) frontend: `AppEvent.node` + `FlowNodeEvent` tipi, `shared/lib/flowNodeBus.ts` (runId-keyed
  pub/sub), `useAppEvents.onFlowNode` → bus'a fan-out, `system.ts` üçüncü SSE callback'i;
  (e) `RunView` `run.id`'ye abone → canvas node durumu (asla geri sarmaz: pending→running→done/error)
  - "Adım izi" panelinde biten node çıktıları anında + çalışan node için "…çalışıyor" satırı;
    3sn poll caught-up olunca canlı ekler dedupe olur. `go build ./...` + flow/engine testleri
    (3/3) + `tsc` + `vite build` temiz.

## Aktivite (Executions) ekranı canlı transkript ✅ (2026-07-09)

- **Sorun:** Executions/Aktivite ekranı seçili yürütmenin transkriptini yalnız
  `api.listMessages` polling'iyle çekiyordu → **çalışan turda** araç adımları henüz
  diske yazılmadığından sadece "çalışıyor" noktaları görünüyordu; chat ekranı ise aynı
  turu SSE `session_step` bus'ından canlı gösteriyordu. Render katmanı zaten ortaktı
  (`MessageList`); ayrışma **canlı-veri katmanındaydı** (tek SSE beslemesi frame'leri
  yalnız `chat.applyAutoStep`'e veriyordu).
- **Çözüm — canlı-transkript katmanı birleştirildi:** (a) yeni `shared/lib/stepBus.ts`
  = `session_step` frame'lerini + `turn-end` sinyalini taşıyan session-scoped pub/sub
  (`refreshSignals.ts` desenini yansıtır, **ikinci EventSource açmaz**); (b) `useAppEvents`
  tek SSE beslemesinden `publishStep`/`publishTurnEnd` ile bus'a fan-out yapar (chat yolu
  aynen korunur); (c) yeni `features/executions/useLiveTranscript.ts` = chat'le **aynı**
  `chatStreamAutoLive` helper'larını (`foldAutoStep`/`recoverInflightSnapshot`/
  `clearAutoLiveEntry`) kullanan salt-okunur canlı-transkript hook'u — ghost balon + mid-turn
  inflight kurtarma dahil; (d) `ExecutionsPanel` kendi polling/tick transkript mantığını bu
  hook'la değiştirdi (liste polling'i kaldı). Sonuç: tek SSE → tek stepBus → N transkript
  görünümü aynı canlı kodu paylaşır. `tsc` + `vite build` temiz.
- **Executions canlı-transkript flicker fix (2026-07-10):** `useLiveTranscript.load`
  arka-plan poll'de (`RUNNING_POLL_MS` 2sn) + her `executions` sinyalinde `setMessages(m)`
  ile tüm listeyi kalıcı mesajlarla değiştirip **canlı ghost balonu düşürüyor**, ardından
  `recoverInflightSnapshot`'ı **her yüklemede** çağırıp akümülatörü snapshot'ın (≤600ms
  throttle) daha az adımına resetliyor + balon id'sini `live-auto-*`↔`messageId` arası
  değiştiriyordu → çalışan turda saniyede birkaç kez titreme ("N araç" oynaması, balonun
  çıplak working-dots'a düşüp geri gelmesi). Düzeltme: (a) ghost balonu kalıcı-liste
  takasında **koru** (id ile, yalnız `running` iken ve henüz persist olmamışsa); (b)
  `recoverInflightSnapshot`'ı yalnız **ghost yokken** çağır (gerçek mid-turn reload); (c)
  ghost persist olunca **veya** tur bitince akümülatörü sil (bir sonraki turun stale
  girdiye eklemesini önler). `runningRef` ile `load` kimliği stabil kalır. `tsc` temiz.

## Board otomasyon değişkenleri: `{{owner}}` + `{{priority}}` ✅ (2026-07-09)

`{{tags}}`'in ardından iki değişken daha: `BoardChangeEvent`'e `Priority` alanı
(4 fire noktası doldurur, `store_task.go`); `boardVars` artık ctx alıp `{{owner}}`'ı
`e.db.GetAgent` ile ajan **adına** çözer + `{{priority}}`'yi render eder
(`automation.go`). Araç şeması + frontend `BOARD_PROMPT_VARS` + testler güncellendi
(`TestBoardVarsSubstitution` tags/priority/owner kapsar). Canlı E2E: owner+priority+tags'li
kart → prompt `OWNER=[BenchOpus48] PRIO=[high] TAGS=[urgent]` render etti.

## Board UX: kart edit popup'ında kart ID'si (kopyalanabilir) ✅ (2026-07-10)

- **Card edit popup — kart ID'si (başlığın solunda)** (`TaskFormModal.tsx`): edit modunda
  header'da başlık input'unun **solunda** `tsk_…` chip'i; tıklayınca panoya kopyalar
  (Copy→✓ 1.5sn geri bildirim, `copyToClipboard`, `data-testid="task-id-copy"`). Amaç:
  kartı bağımlılık/otomasyon/prompt içinde referanslarken id'yi UI'dan almak. Yalnız edit
  modunda (create'te id yok).

## Board UX: create'te description focus + otomasyon `{{tags}}` ✅ (2026-07-09)

- **Create popup → description'a otomatik focus** (`TaskFormModal.tsx`): create modunda
  modal açılınca `descRef` ile açıklama alanına focuslanır (başlık zaten ondan üretiliyor
  → kullanıcı hemen yazmaya başlar). Edit modunda focus yok.
- **Board otomasyon prompt değişkeni `{{tags}}`**: kanban kartının etiketleri artık
  otomasyon promptuna parametre. `BoardChangeEvent`'e `Tags` alanı eklendi + dört fire
  noktası (Create/Move/Update/Delete, `store_task.go`) doldurur; `boardVars`
  (`automation.go`) `{{tags}}`'i virgülle-ayrık render eder. Araç şeması
  (`builtin_automationmgmt.go`) + frontend `BOARD_PROMPT_VARS` (`Automations.tsx`)
  güncellendi. Canlı E2E: board 'create' otomasyonu, etiketli kart → spawn oturumun
  prompt'u `CARD_TAGS=[urgent, backend]` olarak render etti.

## Prompt Epoch: oturum-başı donmuş bağlam snapshot'ı ✅ (2026-07-08)

external-context-agent "frozen snapshot" deseninin genellenmesi (yeni doküman
`57-PROMPT-EPOCH.md`): statik system promptu + araç şemaları per (session,
agent) oturum başında donar (`internal/agent/promptepoch.go` +
`prompt_epoch.json` sidecar'ı, restart-safe, fail-open) → oturum-ortası
skill/ayar/MCP-katalog/capability değişiklikleri prompt cache'ini artık kırmaz.
Adopt yalnız zaten-bust anlarında: compaction fold, 1h TTL soğuması,
model/workdir/katılımcı değişimi, `/refresh-context` komutu + `update_session
{refresh_context}` alanı. Tur içi `activate_tools` bilinçli kast: donmuş baza
canlı formuyla merge edilir (`mergeFrozenToolDefs`); kapatılan araç yürütmede
fail-closed kalır. Stale drift dinamik suffix notu + `epoch` debug olaylarıyla
görünür; workspace toggle `PromptEpochEnabled` (default açık) → Ayarlar →
Workspace paneli. Test: `promptepoch_test.go` (10 senaryo); suite 796/30 paket

- tsc yeşil. **Ek (aynı gün): Debug viz** — "Prompt-cache olayları" kartı
  (`PromptCacheEvents.tsx` + `buildPromptCacheSummary`): `epoch` + `cache_break`
  olayları rozetli zaman çizelgesinde (donduruldu/adopte/stale/yenilendi/kırılım);
  ham olay filtresi `epoch` tipini aldı; epoch emit'leri `WithSessionID` damgalı
  (damgasız ctx'te olaylar düşüyordu — düzeltildi). **Canlı E2E doğrulandı**
  (izole :8099, gerçek fable-5): `created → stale → refreshed → created` dizisi,
  donmuş sidecar drift'i almadı → refresh sonrası aldı, kararlı turda olay yok,
  `cache_break` 0. Yan bulgu: izole claude-home'da aralıklı `authentication_failed`
  (token rotasyonu şüphesi, retry'la geçiyor) — epoch-dışı, ayrı araştırma.

## Board kartı: collapsible bağımlılıklar + pbi-kayması kök-neden ✅ (2026-07-08)

- **Card edit popup — bağımlılıklar collapsible** (`TaskFormModal.tsx`):
  "Bağımlılıklar — önce tamamlanması gereken görevler" bölümü artık chevron'lu
  açılır/kapanır başlık; görevde bağımlılık varsa açık, yoksa kapalı başlar +
  kapalıyken sayaç rozeti. Yer kaplamayı azaltır.
- **"pbi sütununa taşınan kartlar todo'ya kayıyor" — kök neden TionHarness DIŞINDA:**
  harici `tionharness-obsidian-sync` aracı (`Progs\tionharness-obsidian-sync`,
  `watch --interval 30`) `status_map`'te olmayan custom board anahtarını her 30s'de
  `todo`'ya çeviriyordu (`obsidian.py` `_card_to_pm`/`_pm_to_card` `get(..,"todo")`
  fallback'i → conflict ping-pong). Düzeltme: bilinmeyen board anahtarı **pass-through**
  (todo'ya çevrilmez); custom sütun iki yönde kayıpsız round-trip eder. Canlı doğrulama:
  WS5'teki gerçek "Memory Provider Plugins Comparison" görevi 2+ sync döngüsü boyunca
  `pbi`'de sabit kaldı. **TionHarness çekirdeği bu konuda zaten doğruydu** (`IsValidBoardKey`
  custom anahtarları kabul eder, store coercion yapmaz) — TionHarness kodunda değişiklik yok.

## Board kartı: asenkron başlık + optimistic oluşturma ✅ (2026-07-08)

Board'da kart oluşturma iki iyileştirme aldı:

- **Asenkron AI başlık** (`internal/api/tasks.go` `handleCreateTask`): başlık boş
  bırakıldığında create artık LLM'i **beklemez**. Önce içerikten türetilmiş anlık
  bir placeholder (`placeholderTitle`: ilk satır, 60 rune + `…`) damgalanır ve kart
  hemen döner (~0.003s); gerçek AI başlığı **arka plan goroutine**'de üretilip
  (`TitleFor`, detached 60s ctx) yere iner ve `board` SSE olayıyla açık pencereler
  tazelenir. Guard: kart silinmiş veya kullanıcı başlığı elle değiştirmişse
  (placeholder ≠ mevcut başlık) AI sonucu **uygulanmaz** (kullanıcı düzenlemesi
  ezilmez). Başlık verilmişse davranış eskisi gibi.
- **Optimistic + seçili sütun düzeltmesi** (`TaskFormModal.tsx` + `TaskBoard.tsx`):
  create artık kartı **anında seçili sütunda** render eder (temp `temp-…` id →
  "başlık üretiliyor…" ipucu), sonra sunucu satırıyla `onReplaceTemp` ile
  upsert-uzlaştırır (eşzamanlı SSE reload'da tekilleştirir). Seçilen board sütunu
  (ToDo dışı sütunlar dahil) uçtan uca korunur. Placeholder başlık = açıklamanın
  kısaltılmış hali (`excerpt`).
- **Doğrulama (canlı 8090):** boş-başlık + `boardState=review` create → yanıt
  0.003s, doğru sütun, placeholder `"…"` ile; ~5s sonra AI başlığı
  ("TionHarness 2FA TOTP Doğrulama Akışı Ekleme") yerine indi.

## Prompt-cache denetimi: hedge breakpoint + blok-bazlı coalesce ✅ (2026-07-08)

Cache-kırılım denetiminin iki düzeltmesi (`internal/providers/anthropic.go`):

- **Hedge breakpoint:** kayan history breakpoint'ine ek, bir önceki taşıyabilen
  mesajın son bloğuna 4. breakpoint — >20 bloklu paralel tool batch'lerinde
  Anthropic'in ~20-blok lookback ufku yüzünden tüm geçmişin miss olmasını önler
  (Raw mesajlar atlanır, en yeni Raw-olmayan öncüle düşer). Bütçe 4/4 dolu.
- **Blok-bazlı coalesce:** çok-ajanlı art arda aynı-rol düz mesajlar artık önceki
  mesajın text'ine değil **ayrı text bloğu** olarak eklenir → cached prefix'in son
  mesajının baytları değişmez. `coalescePlainSameRole` yalnız minimax'ta kaldı.
- Test: 5 yeni/güncel `TestToAnthropicMessages_*` + `TestCacheBreakpointStability`;
  tüm suite 787/787 yeşil. Detay `17-TOKEN-OPTIMIZASYON.md` §Hedge breakpoint.

## Self-healing: dosya-mutasyon verifier'ı ✅ (2026-07-08)

the external agent analizindeki "gereksiz adım optimizasyonu" maddesinin son boşluğu:
`Write`/`Edit`/`apply_patch` başarılı yazım sonrası diski geri okuyup içeriğin
gerçekten yere indiğini doğrular (`internal/tools/verifymutation.go`; ≤1MB
bayt-bayt, üstü SHA-256; dosya-başına tek ekstra okuma). Uymuyorsa çağrı hatalı
tool_result'a döner → mevcut self-healing zinciri (guardrail, tool-error tag,
ders) devralır. Detay `56-SELF-HEALING.md`.

## Self-healing devam turu: 8 adım + canlı E2E doğrulaması ✅ (2026-07-08)

Detay: `_Docs/56-SELF-HEALING.md` "Devam turu" bölümü. Özet:
`read_lessons`/`delete_lesson` araçları · ders yaşlandırma (45g) + signature
normalizasyonu (yol/sayı) · ajan-öncelikli enjeksiyon · Retry-After hint'i
turn-retry backoff'unda · CLI turlarına tur-sonu guardrail analizi (`cli_warn`) ·
hatalı-tur dispatch'i (`FireTurnFailed`) + tek-tık "Stuck oturum onarıcısı"
otomasyon şablonu · Debug viz'e Self-healing olayları bölümü. **Canlı E2E**
(izole instance, gerçek fable-5): tool-error tag → guardrail → stuck zinciri →
otomasyon → fixer → etiket/sayaç temizliği → ders doğumu → sonraki tura
enjeksiyon uçtan uca doğrulandı; E2E'nin yakaladığı 4 bug düzeltildi
(non-stream chat parity, CLI aux claude-home, SpawnTags omitempty, NONE artefaktı).

## Bütçe/maliyet ekranı hesaplama düzeltmeleri (4 bulgu) ✅ (2026-07-08)

Bütçe · oturum bilgisi · sohbet-debug · debug popup'larının hesap denetimi. Dört düzeltme, hepsi ortak `billing.PriceStat` + fiyat tablosu + `session_info` filler yolunda (tek nokta → dört ekran):

- **claude-cli tahmini cache tasarrufu $0 idi → düzeltildi** (`billing.PriceStat`): estimated dalı maliyeti tahmin edip tasarrufu `0` bırakıyordu; artık `ep.CacheSavings(cacheRead)` da tahmin ediliyor (`priced=false` korunur). Varsayılan anahtarsız sağlayıcıda Tasarruf Merkezi/oturum kazancı/mesaj-debug artık gerçek cache ROI'yi gösteriyor.
- **claude-cli cache-write primi 2× → 1.25×** (`providers.EstimateFor`): 1s-TTL primi (`CacheWrite1hMult`) yalnız native anthropic client'a özgü; Claude Code CLI 5dk TTL kullanır. Override sıfırlandı → ~%60 fazla fiyatlandırma giderildi.
- **Bağlam penceresi çubuğu segment↔toplam** (`session_info.go`): "kullanılan" başlığı `+MsgOverhead` sayıyor, segmentler saymıyordu → çubuk %100'e ulaşmıyordu. `buildFillers` artık mesaj başına `conversation.MsgOverhead` ekliyor + `ContextTokens` filler toplamından türetiliyor (birebir). `MsgOverhead` dışa açıldı.
- **"Tasarrufsuz maliyet" kartı tam-doğru baseline'a çevrildi** (`Price.CostNoCaching` + `billing.NoCacheCost` + `cumulative.noCacheCostUSD`): eski `cost + savings` cache-write primini içeride bırakıp senaryoyu şişiriyordu; artık cacheRead+cacheWrite tümü taban girdi fiyatından hesaplanan gerçek "caching yokmuş" tutarı gösteriliyor.
- 260707-quick-pass benchmark verisi aritmetik olarak iç-tutarlı doğrulandı (1.25× ile); tek çelişki kod tarafındaki 2× regresyonuydu → yukarıda giderildi.
- Regresyon: `billing_test.go` + `pricing_test.go`; backend build + test + frontend tsc yeşil. Detay `17-TOKEN-OPTIMIZASYON.md` §Hesaplama düzeltmeleri.

## Shell-araç promptu artık gate'e dinamik bağlı (prompt ↔ katalog drift'i giderildi) ✅ (2026-07-08)

**Sorun:** Statik workspace talimatları (`default-instructions.md` → `config/instructions.md`)
`Bash`/`PowerShell`'i "core, always-available" diye koşulsuz reklam ediyordu. Ama shell
araçları `buildRegistry`'de `r.tun.ShellEnabled()` gate'inin (+ backing shell varlığının)
arkasında — gate kapalıyken hiç register edilmez. Sonuç: model çıplak `PowerShell` çağırıp
`No such tool available: PowerShell … not enabled in this context` alıyordu (autotag bunu
zaten self-recovered permission-deny sayıyor; fonksiyonel bug değil ama modeli yanıltıyordu).

**Çözüm — dinamik render (tek-kaynak):**

- Statik talimatlardan shell vaadi çıkarıldı; yalnız fs araçları (`Read/Write/Edit/LS/Glob/Grep`)
  "always-on" kaldı. Shell satırı "gated — enabled olunca environment context söyler" notuna dönüştü.
  (Hem embedded `internal/workspace/defaults/default-instructions.md` hem canlı WS10 `config/instructions.md`.)
- Yeni `tools.ShellToolNames()` — `resolvePOSIXShell`/`resolvePowerShell` (araçların kendi resolver'ları)
  ile hangi shell'lerin register edileceğini adıyla döner → advertised isim, kayıtla asla drift etmez.
- Yeni `Runtime.ShellToolsContextBlock()` — gate açık + backing shell varsa Bash/PowerShell'i adıyla
  duyurur, aksi halde boş string. **Volatile → dinamik suffix** (gate tur-ortası değişebilir):
  chat yolu `composeTurnRequest` (EnvironmentContextBlock'tan hemen sonra) + headless
  `autonomousDynamicSuffix`, `EnvironmentContextBlock` ile aynı OS/shell kimliğini paylaşır.
- Build + vet + `internal/tools`/`internal/agent` testleri yeşil. Detay `53-CRAFTAGENT-PROMPT-PARITE.md`.

## Self-healing: Lessons UI + guardrail eşikleri ayarlara açıldı ✅ (2026-07-07)

- **Lessons API/UI:** `GET /api/lessons` + `DELETE /api/lessons/{id}`
  (`internal/api/lessons.go`, workspace-scoped) → Ayarlar → Bağlam → Self-healing
  bölümünde `LessonsList` bileşeni (araç rozeti, görülme sayısı, tarih, satır-başı
  silme, yenile). Frontend: `api/lessons.ts` + `types/lesson.ts` barrel'lara eklendi.
- **Guardrail eşikleri:** 6 eşik ayara açıldı (`guardExactWarn/Block`,
  `guardSameToolWarn/Halt`, `guardNoProgressWarn/Block`; 0 = varsayılan 2/5, 3/8,
  2/5; clamp 0..50). `toolGuardConfig` eşikleri taşır, `newToolGuard` <=0'ı
  varsayılana çözer; UI'da Self-healing bölümünde 3'lü grid. Detay `56` (ayarlar tablosu).

## Sohbet boş-durum ekranı: "Yeni sohbete başla" ✅ (2026-07-07)

**İstek:** Sohbet ekranını ilk açtığımızda ve hiç sohbet yokken devre-dışı bir input alanı
görünüyordu ama ajan bile seçili olmuyordu. Bunun yerine düzgün bir "Yeni sohbete başla"
ekranı çıksın.

- Yeni `features/chat/ChatEmptyState.tsx`: `ChatView` artık `activeSessionId` yokken (aktif
  oturum yok) transcript+devre-dışı composer yerine ortalanmış bir başlangıç kartı gösterir.
  - Ajan varsa: başlık "Yeni sohbete başla" + (birden fazla ajan varsa) ajan seçici (default'u
    `pickDefaultAgent` ile ayarlar) + **"Yeni sohbet"** butonu (`ctl.newSession` → seçili
    ajanla taze oturum açar). Tek ajan varsa hangi ajanın kullanılacağı rozet olarak gösterilir.
  - Ajan yoksa: "Önce bir ajan oluştur" + Ajanlar ekranına kısayol (`selectView('agents')`).
- `ChatView` yeni prop'lar alır: `defaultAgentId`/`onNewSession`/`onSelectDefaultAgent`/
  `onGoToAgents`; erken-return tüm hook'lardan SONRA (rules-of-hooks güvenli).
- `App`, `ctl.defaultAgentId`/`ctl.newSession`/`ctl.pickDefaultAgent`/`selectView('agents')`
  ile bağlar.

## Frontend feature-bazlı refactor (3 aşama) ✅ (2026-07-07)

**İstek:** Frontend kodlarının daha düzenli bir yapıya refactor edilmesi.
Davranış eşdeğeri, 3 commit: yapı taşıma → App.tsx decompose → dev dosya bölme.
Her aşamada `tsc -b` + `vite build` yeşil; eslint problem sayısı 116 → 102
(yeni ihlal sıfır, kalanlar refactor öncesinden).

- **Aşama 1 — feature klasörleri + `@/` alias:** 223 dosya taşındı (git rename
  olarak). Yeni yerleşim: `app/` (kabuk: App, NavRail, MobileNavBar, url/event
  hook'ları), `features/<domain>/` (chat, sessions, agents, flows, tasks,
  schedules, executions, artifacts, skills, tools, market, budget, logs,
  network, settings, workspace), `shared/` (components [eski common + markdown
  - çapraz-feature agent widget'ları], hooks, lib). `api/` + `types/` barrel'ları
    değişmedi. `tsconfig.app.json` `paths` + vite `resolve.alias` ile `@/*` = `src/*`.
- **Aşama 2 — App.tsx decompose:** 1.446 → ~630 satır kompozisyon kökü. Yeni
  `app/` modülleri: `viewRegistry` (VIEW_TITLE/HEADERLESS + `isChatKind`),
  `lazyPanels`, `useAppearance`, `useDeepLinks`, `useSessionsController`
  (agents/sessions/transcript state + tüm aksiyonlar), `useAppEvents` (SSE
  dağıtımı), `useAppNavigation` (URL↔state), `AppHeader`, ve
  `features/chat/ChatView` (transcript + alt yığın).
- **Aşama 3 — dev dosya bölmeleri (saf kod taşıma):**
  `useChatStream` 1006→375 + 7 modül (`chatStreamSend/AutoLive/Commands/
Interventions/History/Types/Helpers`); `FlowsPanel` 1143→~450 +
  `FlowsListPane/FlowsHeader/FlowEditorView/flowActions/flowGraphOps/
flowsPanelShared`; `MarketPanel` 1089→370 + `MarketGrid/PackDetailModal/
PackPreview/previewParts/marketHelpers`; `ToolsPanel` 1033→314 +
  `useToolsPanelState/ServerManagement/ToolDetail/VisibilityControls`;
  `SessionDetailPanel` 897→429 + 9 bölüm modülü.
- **Eski→yeni yol eşlemesi (eski dokümanlardaki atıflar için):**
  `components/panels/X` → `features/<domain>/X` · `components/chat|sessions|
agents|flow|artifacts|workspace|settings/` → `features/<domain>/` ·
  `components/common/` → `shared/components/` · `components/markdown/` →
  `shared/components/markdown/` · `hooks/`+`lib/` → feature'a aitse
  `features/<domain>/`, genel ise `shared/hooks|lib/`, app-kabuğuysa `app/`.
  05 ve öncesi tarihli dokümanlardaki eski yollar bu tabloyla okunmalı.
- Ölü dosyalar silindi (0 import): `features/agents/AgentRoster.tsx`,
  `features/sessions/SpawnSessionModal.tsx`, `shared/hooks/useResizableWidth.ts`,
  `app/App.css` — gerekirse git geçmişinden geri alınabilir.

## Self-healing Faz F: hata→ders döngüsü + Ayarlar UI ✅ (2026-07-07)

**İstek:** Self-healing ayarlarının frontend'e eklenmesi + hermes `background_review`
karşılığı hata→ders döngüsü. Detay: `_Docs/56-SELF-HEALING.md` (Faz F bölümü).

- **Lesson reflector** (`internal/agent/lessons.go`): kötü biten tur → arka-plan ucuz
  model çağrısı (`KindReflect`) → tek genellenebilir ders; `AutoTagTurn` hunisinden
  tetiklenir, `lessonReflect` ayarıyla (vars. açık) gate'li. Policy denial/guardrail
  coaching/iptal/stuck-gate reddi kanıta girmez; `NONE` cevabı kaydedilmez.
- **Lessons store** (`internal/db/store_lessons.go`): workspace-geneli `lessons.jsonl`,
  signature dedupe (tekrar → `Count++` + metin tazelenir), cap 200, kendi mutex'i.
- **Enjeksiyon**: `Runtime.LessonsContextBlock` — en yeni 5 ders chat + headless
  dinamik suffix'ine girer (cache'li prefix bozulmaz). Debug olayı: `lesson`.
- **Frontend**: `ContextPanel` "Self-healing" bölümü (guardrail uyarı/devre kesici
  toggle'ları, stuck eşiği, lesson toggle) + "Tur kurtarma" grid'ine sağlayıcı retry
  bütçesi; `types/settings.ts` + `SettingsPanel.saveApp` patch'i. **Bugfix:** saveApp
  patch'inde handoff/progress/autoTag/debugJournal alanları eksikti (bu toggle'lar
  hiç kaydedilmiyordu) — eklendi.
- Testler: `store_lessons_test.go` + `lessons_test.go`; `tsc --noEmit` + vite build temiz.

## Fable 5 uyumluluk paketi (6 madde) ✅ (2026-07-07)

**İstek:** Fable 5 uyumluluk denetiminde bulunan 6 boşluğun kapatılması.

1. **Thinking-blok echo'su (kritik):** Fable/Mythos'ta thinking HER yanıtta var (araç
   döngüsü dahil) ve bloklar aynen geri gönderilmek zorunda. `rawEcho` kapısına
   `providers.AlwaysOnThinking(agent.Model)` eklendi — Fable'lı ajanlar hiçbir toggle'a
   bağlı olmadan verbatim echo alır.
2. **Refusal + server-side fallback:** `AnthropicRefusalFallback` ayarı (**varsayılan
   AÇIK**, Anthropic'in Fable rehberi) → Fable-sınıfı isteklere `fallbacks:
[claude-opus-4-8]` + `server-side-fallback-2026-06-01` beta; sınıflandırıcı reddi aynı
   çağrıda Opus 4.8'le karşılanır. `stop_details` parse edilir (`Response.StopDetails`);
   araç döngüsünde `refusal` artık boş balon yerine kategori+açıklamalı hata kartı üretir.
   Mid-output fallback echo kuralı için `sanitizeFallbackEcho` (sınır öncesi thinking/
   tool_use blokları düşülür). UI toggle + `fallback` iz adımı.
3. **Zaman aşımları:** model-sınıflı istek bütçesi — uzun bütçe (600s) adaptive sınıf
   (Fable/4.7+/Sonnet 5) **+ akıl-yürütme modelleri (deepseek-v4-pro)**, eskiler 120s.
   Sınıflandırıcı `LongRequestModel` (`timeouts.go`) — thinking wire-format'tan ayrı; hem
   native Anthropic hem OpenAI-uyumlu (`OpenAICompat.requestCtx`) client'ta uygulanır,
   client transport tavanı 630s. `Manifest.RequestTimeoutSecs` ile kind bazında override
   (`Registry.Get`). `scheduleTimeout` 120s → **30 dk** (Fable'ın dakikalarca süren tek
   istekleri + uzun araç döngüleri; kaçak koruması iterasyon/bütçe guard'larında). Detay 2026-08-06 fix.
4. **`model_context_window_exceeded` stop reason:** `decideRecovery`'ye dal eklendi —
   hata-şekilli taşmayla aynı tek-atım compact-and-retry; tekrarında tur artık
   "completed" değil `context_window_exhausted` olarak işaretlenir (iz kartı + kırpılma
   notu). Araç döngüsünün yanıt-tarafına compact uygulaması eklendi.
5. **Prompt ince ayarı:** `autonomousBootReminder` de-prescribe edildi ve genişletildi —
   "Autonomous operation": izin sorma/planla bitirme yok (aksiyon al), baseline doğrula,
   TEK iş, ilerleme iddiaları bu oturumdaki araç sonuçlarına dayansın, kapanışta kayıt.
   `notify` açıklamasına birebir-iletim (verbatim) tetiği eklendi (send_to_user deseni).
6. **Veri saklama notu:** Fable katalog açıklamasına 30-gün saklama + fallback notu;
   anthropic 400 hatasında "retention" geçiyorsa eyleme dönük ipucu ekleniyor
   (ZDR org'da isteğin değil org ayarının sorun olduğu).

Testler: `TestSanitizeFallbackEcho`, `TestRequestCtx`, `TestDecideRecovery_ContextWindowStop`,
güncellenen boot-reminder pinleri. ✅ build/vet temiz; 764 test / 35 paket; frontend `tsc` temiz.

## Kendi kendini onaran oturum akışları (self-healing, Faz A–D) ✅ (2026-07-07)

**İstek:** external-context-agent incelemesinden çıkan self-healing desenlerinin TionHarness'e
uyarlanması: tool hatalarını çözen, oturum akışını onaran, döngüleri kesen ve stuck
oturumları işaretleyen katman. Detay: `_Docs/56-SELF-HEALING.md`.

- **Faz A** `internal/agent/errclass.go`: provider hata taksonomisi
  (`rate_limit/overloaded/server_error/timeout` retry-edilebilir; `auth/billing/
cancelled/unknown` terminal) + `decideRecovery`'de `contProviderRetry` — jitter'lı
  backoff'la sınırlı retry (`maxProviderRetries` ayarı, vars. 2). İptal artık
  `termCancelled`.
- **Faz C** `internal/conversation/repair.go` `RepairSequence`: her provider çağrısı
  öncesi tur-içi tool_use↔tool_result eşleşme onarımı (orphan drop / duplicate dedup /
  eksik sonuç sentezi / bölünmüş assistant batch merge). Saf + idempotent; onarımlar
  `debug.jsonl` `repair` olayı.
- **Faz B** `internal/agent/toolguard.go`: tur-başına döngü tespiti (exact-failure 2/5,
  same-tool 3/8, no-progress 2/5; idempotent = `RiskRead`). Uyarı hint'i başarısız
  tool_result'a eklenir (`toolGuardWarnings` vars. açık); hard stop (`toolGuardHardStop`
  vars. kapalı) blok/`termGuardrailHalt`. `debug.jsonl` `guardrail` olayı.
- **Faz D** `db.Session.StuckTurns` + `stuck` auto-tag + `stuckGate`: ardışık kötü tur
  eşiği (`stuckTurnThreshold` vars. 3) aşınca otonom turlar reddedilir (manuel chat
  serbest); `stuck` etiketi kaldırılınca sayaç sıfırlanır. Etiket-otomasyonla onarım
  ajanına bağlanabilir.
- Testler: `errclass_test` / `toolguard_test` / `repair_test` / `stuck_test` (tablo
  testleri). Sıradaki: hata→ders döngüsü (background-review karşılığı, ayrı plan).

## Workspace oluşturunca claude-cli hazırlık kapısı (auth popup / Sağlayıcılar yönlendirme) ✅ (2026-07-07)

**İstek:** Yeni bir workspace oluşturulduğunda, claude-cli kuruluysa ama bu workspace'in
claude-home'u yetkilendirilmemişse bir bildirim çıksın ve oradan "claude-cli kimlik
doğrulama" popup'ı açılabilsin; claude-cli hiç yoksa kullanıcı Sağlayıcılar ekranına
yönlendirilsin.

**Backend:**

- `providers.ClaudeCLI.Installed()` (yeni): yapılandırılmış `binPath`'i `exec.LookPath` ile
  çözer — CLI binary'si mevcut mu (login'den bağımsız). "CLI yok" ile "CLI var ama login yok"
  ayrımını sağlar.
- `claudeAuthDTO`'ya `installed bool` alanı; `handleWorkspaceClaudeAuth` artık probe'dan
  ÖNCE `cli.Installed()` kontrolü yapıyor — binary yoksa `installed:false` + açıklayıcı detay
  döner (login probe'u boşa çalıştırmaz).

**Frontend:**

- `api.checkWorkspaceClaudeAuth` dönüş tipine `installed` eklendi.
- `useWorkspaces.createWorkspace` artık oluşturulan workspace'i (veya hatada `undefined`)
  döndürüyor → çağıran taraf oluşturma-sonrası kapı çalıştırabiliyor.
- Yeni `features/workspace/ClaudeAuthGate.tsx`: `App` bir `claudeGateNonce` sayacıyla her
  başarılı oluşturmadan sonra tetikler; gate yeni (aktif) workspace'in login durumunu
  problar. Sonuç: login var → sessiz; CLI var + login yok → eyleme dönüştürülebilir bildirim
  (buton `ClaudeAuthDialog`'u açar, kimlik doğrudan bu workspace'in claude-home'una yazılır);
  CLI yok → `onNavigateProviders` ile Sağlayıcılar ekranı (`setSettingsCat('providers')`).
- `App` tüm oluşturma yollarını (onboarding + rail + mobil) tek `handleCreateWorkspace`
  sarmalayıcısından geçirir. **Mevcut klasör bağlama** (`attachWorkspace`, onboarding) da
  aynı kapıyı tetikler (`handleAttachWorkspace`) — bağlanan workspace'in kendi
  yetkilendirilmemiş claude-home'u olabilir; `attachWorkspace`'in hata-fırlatma sözleşmesi
  korunur (inline doğrulama hataları).
- **Sohbet-açılışında tekrar-hatırlatma (`requireNoProvider`):** Kapı iki nedenle tetiklenir
  — (a) oluştur/bağla (`requireNoProvider=false`, her zaman probe/popup); (b) her sohbet
  ekranı girişinde + workspace değişiminde (`requireNoProvider=true`). (b) yalnızca
  workspace'te **hiçbir kullanılabilir provider yoksa** iş yapar: `hasNoUsableProvider()` =
  hiçbir built-in anahtar (anthropic/minimax/openrouter) + claude-cli token + custom provider
  yok. Bu **ucuz** ön-kontrol, **pahalı** `claude -p` login probe'undan ÖNCE çalışır →
  provider'ı olan kullanıcı asla rahatsız edilmez, hiç kurulumu olmayan kullanıcı her sohbet
  açılışında popup'ı yeniden görür (CLI kuruluysa) veya Sağlayıcılar'a yönlendirilir (CLI
  yoksa). `App`'te `claudeGate={nonce,requireNoProvider}` durumu + `view==='chat'` effect'i.

## API-native P2+P3: sunucu web search/fetch + server-side compaction (toggle'lı) ✅ (2026-07-07)

**İstek:** _Docs/55 P2 ve P3'ün eklenmesi, her ikisi de ayarlardan açılıp kapanabilir.
İkisi de ek sunucu/süreç GEREKTİRMEZ — mevcut /v1/messages isteğinin alanlarıdır.

**P2 — Sunucu-tarafı web search + web fetch (`AnthropicWebTools`, varsayılan kapalı):**

- `Request.WebTools` → `toAnthropicTools` (yeni `serverToolOpts` yapısı): `web_search` +
  `web_fetch` sunucu araçları isteğe eklenir; aramayı Anthropic yürütür, alıntılı sonuçlar
  aynı yanıtta döner. 4.6+ modellerde `_20260209` dinamik-filtreli sürüm
  (`SupportsDynamicWebTools`); eski modellerde VE PTC açıkken temel sürümler
  (`web_search_20250305`/`web_fetch_20250910`) — dinamik sürüm kendi code-execution
  ortamını taşıdığından PTC'yle çifte ortam oluşmaz.
- Maliyet freni: tur başına `max_uses` tavanları (arama 8, çekme 12, sabit).
- Sonuç blokları iz adımı olarak UI'a düşer (`web_*_tool_result` → sonuç sayısı/hata kodu);
  RawContent verbatim echo web modunda da açık (şifreli alıntı içeriği tur içinde korunur).
- Yalnız `provider.Name()=="anthropic"` + native tool loop (araçları açık ajanlar).

**P3 — Server-side compaction (`AnthropicServerCompaction`, varsayılan kapalı):**

- `WithBetas` üçüncü parametre → `compact-2026-01-12` beta başlığı +
  `context_management.edits`'e `{type:"compact_20260112"}` (sunucu-varsayılan ~150K tetik).
  Context-editing ile bağımsız; ikisi açıkken iki edit birden gönderilir.
- Compaction blokları TUR İÇİNDE RawContent verbatim echo ile aynen geri gönderilir
  (API şartı) — uzun tek turların (araç döngüsü) taşma sigortası. Turlar-arası transkripti
  istemci-tarafı compaction yönetmeye devam eder (çifte özetleme çakışması yok; tam
  turlar-arası server compaction, compaction bloklarının db persist'ini gerektirir — P3'ün
  ileride derinleştirilecek kısmı olarak _Docs/55'te not edildi).
- Kayıt zinciri: settings → `SetAnthropicBetas(cache, ctxEdit, serverCompact)` (registry →
  kind cfg → `WithBetas`) + tunables aynası (rawEcho kapısı için).

**UI:** Ayarlar → Bağlam → "Anthropic beta" altında iki yeni toggle (web araçları +
API-native compaction), Türkçe ipuçlarıyla.

Testler: `TestToAnthropicTools_WebTools` (dinamik/temel sürüm seçimi, PTC çakışma kuralı,
max_uses, breakpoint yerleşimi), `TestContextMgmt_ServerCompaction` (edit + beta başlığı,
bağımsızlık). ✅ build/vet temiz; `go test ./...` 734 test / 35 paket; frontend `tsc` temiz.

## API-native P1+P6+P7+P4: structured outputs, system mesajları, strict/effort, PTC ✅ (2026-07-07)

**İstek:** _Docs/55 yol haritasının P1, P6, P7, P4 maddelerinin uygulanması.

**P1 — Structured Outputs (`output_config.format`):**

- `Request.OutputSchema` → `applyOutputSchema` (yalnız destekleyen modeller: Fable/Mythos,
  Opus 4.8, Sonnet 5, Haiku 4.5, legacy 4.5/4.1 — Opus 4.6/4.7 ve Sonnet 4.6 matriste YOK).
- **Titler**: `{"title": string}` şeması + JSON-parse-else-fallback (claude-cli serbest metin
  dönmeye devam eder, sanitizer korunur).
- **Orchestration**: agent node `outputSchema` alanı (yeni `SchemaAgentRunner` opsiyonel
  arayüzü — mevcut runner mock'ları değişmedi); branch node `jsonField` alanı — son çıktı
  JSON'ından üst-düzey alan çekilip eşleştirilir (`{"verdict":"SHIP"}` → parse-proof karar).

**P6 — Mid-conversation system messages (Opus 4.8):**

- `toAnthropicMessages` artık RoleSystem'ı düşürmüyor: Opus 4.8'de `{"role":"system"}` olarak
  geçer (cache-safe operatör kanalı); diğer modellerde `foldSystemMessages` metni ÖNCEKİ user
  mesajına `<system-reminder>` bloğu olarak katlar (rol alternasyonu korunur — steer'in
  user-user 400 riskini de çözer). Tool-loop steer mesajları Opus 4.8 + anthropic'te
  system rolüyle gönderilir.

**P7 — küçükler:**

- **Strict tool use**: `ToolDef.Strict` → `strict:true` (fs süiti Read/Write/Edit/LS/Glob +
  Grep); registry `foldStrict` şemaya `additionalProperties:false` + `required:[]` enjekte
  eder; PTC'li (allowed_callers) araçlarda otomatik düşer (uyumsuz).
- **xhigh/max thinking**: ThinkingLevel'a iki yeni seviye (32K/64K bütçe eşlemesi) →
  adaptive sınıfta `effort: xhigh|max`; legacy enabled+budget yolunda 16384'e kırpılır.
  Ajan formu + composer picker seçenekleri eklendi.
- **CountTokens**: `providers.TokenCounter` + `Anthropic.CountTokens`
  (/v1/messages/count_tokens); oturum bağlam önizlemesi `?accurate=1` ile gerçek sayımı
  `accurateTokens` alanında döner (sezgisel tahmin compaction'ı sürmeye devam eder —
  davranış değişmez, yalnız drift görünür olur).

**P4 — Programmatic Tool Calling (`code_execution_20260120`):**

- `AnthropicProgrammaticTools` ayarı (varsayılan kapalı) → tunables → tool loop: uygun
  builtinler (`CodeModeEligible`, MCP hariç) `allowed_callers:["code_execution_20260120"]`
  ile işaretlenir; code-execution sunucu aracı listeye eklenir.
- `ToolCall.Caller` parse edilir; programatik batch'in yanıt mesajı `OnlyToolResults` —
  dinamik sonek o mesaja binmez (API şartı: saf tool_result). Steer, programatik sonuç
  beklerken ERTELENIR (sıradaki normal iterasyonda teslim edilir).
- Container zinciri: yanıttaki `container.id` sonraki isteklere `container` olarak taşınır
  (bekleyen programatik çağrı varken zorunlu). RawContent verbatim echo PTC modunda da açık.
- `code_execution_tool_result` stdout/stderr'ı iz adımı olarak UI'a düşer.
- UI: Ayarlar → Bağlam → "Programatik araç çağrısı (code execution)" toggle'ı.

Testler: `anthropic_native_test.go` (output schema, system fold/native, saf tool_result,
caller sınıflandırma, effort/clamp), PTC tool-marshal testleri. ✅ build/vet temiz;
`go test ./...` 732 test / 35 paket; frontend `tsc` temiz.

## API-native Task Budgets + Tool Search (beta) ✅ (2026-07-07)

**İstek:** Anthropic'in güncel API özelliklerinden Task Budgets ve native (sunucu-tarafı)
Tool Search'ün TionHarness'e eklenmesi (optimizasyon araştırması madde 1-2).

**Task Budgets (`task-budgets-2026-03-13` beta):**

- `providers.Request.TaskBudgetTokens` + `anthropic.go applyTaskBudget`: adaptive-sınıf
  modellerde (Opus 4.7/4.8, Sonnet 5, Fable 5 — `SupportsTaskBudget`) isteğe
  `output_config.task_budget {type:"tokens", total:N}` + beta başlığı eklenir; API
  minimumu 20K'nın altı otomatik yükseltilir. Model tüm ajan döngüsü için geri sayım
  görür ve kendini ona göre ayarlar — sert iterasyon caplerinin yumuşak, model-farkındalı
  tamamlayıcısı.
- Ayar: `AutonomousTaskBudgetTokens` (0 = kapalı, varsayılan) → tunables →
  `completeTracedInner` yalnız OTONOM turlarda `req.TaskBudgetTokens` doldurur
  (etkileşimli sohbet bütçesiz kalır). UI: Ayarlar → Bağlam → "Otonom görev bütçesi".

**Native Tool Search (`tool_search_tool_regex_20251119`):**

- `ToolDef.DeferLoading` + `tools.Registry.DeferredDefs`: TAM katalog gönderilir —
  eager araçlar normal, lazy/hidden/MCP araçları `defer_loading:true` ile; aktive
  edilenler sıcak kalır (deferred değil). Herhangi bir deferred def varsa Anthropic
  istemcisi arama sunucu-aracını listeye başa ekler (breakpoint asla sunucu araca
  binmez). Keşfedilen şemalar sunucuda EKLENEREK yüklenir → cache öneki bozulmaz;
  araç bloğu iterasyonlar arası bayt-stabil.
- **Raw passthrough:** `Response.RawContent` / `Message.RawContent` — native-search
  modunda asistan turları sunucu bloklarını (tool_search_tool_result / server_tool_use)
  bire bir geri yansıtır (`anthropicMessage.Raw` + özel MarshalJSON). Raw mesajlar
  coalesce edilmez, breakpoint/dinamik binmez. `pause_turn` stop-reason'ı döngüde
  otomatik devam ettirilir (asistan içerik verbatim eklenir, ek user mesajı yok).
- Kapı: `AnthropicNativeToolSearch` ayarı (varsayılan kapalı) + `provider.Name() ==
"anthropic"` (minimax-anthropic/custom uçlar sunucu araç tipini reddeder). Mevcut
  activate_tools/tool_search builtinleri yanında çalışmaya devam eder.

Testler: `anthropic_toolsearch_test.go` (task budget resolver, defer serileştirme,
raw marshal/passthrough), `deferred_defs_test.go`. ✅ build/vet temiz; 725 test yeşil;
frontend `tsc` temiz.

## Token/prompt optimizasyon turu: adaptive thinking + cache varsayılanı + headless split ✅ (2026-07-07)

**İstek:** Kapsamlı optimizasyon denetiminin 1,2,3,4,6,7,9 numaralı maddelerinin uygulanması.

**Ne yapıldı (backend):**

- **Adaptive thinking (kritik düzeltme):** `providers/anthropic.go` — `thinking:{type:"enabled",budget_tokens}`
  formatı Opus 4.7/4.8, Sonnet 5 ve Fable 5'te **400 döndürüyordu** (kataloğun tamamı). `thinkingFor`
  artık model-sınıf farkındalı: adaptive sınıfta `{type:"adaptive", display:"summarized"}` +
  `output_config.effort` (bütçe→low/medium/high eşlemesi); Fable/Mythos'ta "off" alanı tamamen
  atlar (explicit disabled da 400); eski modeller + MiniMax `enabled+budget` şeklinde kalır.
  `providers/thinking.go` yeniden yazıldı (`UsesAdaptiveThinking`/`AlwaysOnThinking`/
  `EffortForThinkingBudget`); `agent.resolveThinkingBudget` Fable tabanı kaldırıldı (çeviri
  provider'a taşındı). Testler güncellendi.
- **Prompt caching varsayılan AÇIK:** `settings.Default()` → `ExtendedPromptCache: true`
  (mevcut settings.json dosyaları kayıtlı değerini korur; yeni kurulum cache'li başlar).
- **Headless statik/dinamik ayrımı:** `autonomousSystemPrompt`'tan uçucu tarih/saat çıkarıldı
  (env satırı bayt-stabil olduğundan statikte kaldı); yeni `autonomousDynamicSuffix(ctx)`
  (saat + hedef bloğu) scheduler/spawn/subagent/flow yollarında `SystemDynamic`'e bağlandı →
  otonom turlar da artık statik prefix cache'inden yararlanır. `DateTimeContextBlock`
  agent paketine alındı; chat yolu aynı kaynağı kullanır.
- **Fiyat düzeltmeleri:** Fable 5 $3/$15 → **$10/$50** (anthropic + openrouter tabloları);
  native anthropic girdilerine `CacheWrite1hMult=2.0` override'ı (client daima 1h TTL ister —
  yazma primi 1.25× değil 2×).
- **`<recent_tool_activity>` dinamik soneke taşındı:** recap artık geçmiş asistan mesajlarına
  gömülmüyor (pencereden düşen turun baytları değişip rolling cache breakpoint'ini kırıyordu);
  `recentToolActivityBlock(history)` tek birleşik blok üretir, `composeTurnRequest` yeni
  `toolRecap` parametresiyle dinamik tarafa ekler (chat/stream/wake/preview 4 çağrı yolu).
- **Eager araç tanımları küçültüldü:** `run_code` 1577→~600 karakter (kullanım detayı keşif
  çıktısına taşındı), `run_subagent` açıklama+şema sadeleşti, `Grep` şemasındaki bayrak
  açıklamaları kaldırıldı (anlamları tanımda tek yerde). Tur başına ~1.5-2K token kazanç.
- **Tekilleştirme/temizlik:** `agent.BuildSystemPrompt` tek persona kaynağı (api kopyası
  delegasyona döndü); ölü `default-instructions_old.md` (~40KB) silindi; software harnesspack
  akışının Execute node'u artık `{{input}}` + `{{node.search}}` bulgularını da alıyor.

✅ `go build`/`vet` temiz; `go test ./...` 720 test / 35 paket yeşil.

## Agent config: yasaklı araç + atanan skill "chip"leri tıkla-kaldır ✅ (2026-07-07)

**İstek:** (1) Yasaklı araç chip'inde ayrı çarpı yerine chip'in kendisine tıklayınca
yasak kalksın. (2) Ajana atanan skiller liste değil chip olarak görünsün.

**Ne yapıldı (yalnız frontend):**

- `AgentToolsSection.tsx`: yasaklı araç `<li>` içindeki ayrı `X` butonu kaldırıldı;
  chip'in tamamı artık `unblock` butonu (`data-testid="agent-tool-blocked"` üstünde
  onClick). Hover'da `Ban` ikonu `X`'e döner + kırmızı vurgu. Eski `agent-tool-unblock`
  testid'i kaldırıldı (dış referansı yok).
- `AgentSkillsSection.tsx`: seçili skiller `<ul>` liste yerine `flex-wrap` **chip**
  (aşağıdaki ekleme picker'larıyla aynı görsel dil). Chip'in tamamı `remove` kontrolü
  (`data-testid="skill-remove"` korundu); hover'da kırmızı vurgu + `X`. Bulunamayan
  slug kırmızı chip. Frontend `tsc` yeşil.

## Sohbet iz kartları arka-plansız + gönderince en-alta kay ✅ (2026-07-07)

**İstek:** (1) Yeni mesaj gönderince transkript en alta kaysın. (2) Sohbetteki
"Görev Listesi" bubble'ının arka-plan dolgusu kalksın; benzer UI bileşenlerinde de.

**Ne yapıldı (yalnız frontend):**

- **Auto-scroll:** `MessageList.tsx` — kaydırma yalnız kullanıcı en-alta "pinned"
  iken çalışıyordu; yukarı kaydırıp mesaj gönderince gönderilen mesaj görünmüyordu.
  `prevLastId` ref'i eklendi: en yeni tur **insanın kendi** turu (role `user`,
  `authorKind !== 'agent'`) ise `pinnedRef` zorla `true` → en alta kayar. Peer/inbox
  mesajları + streaming asistan delta'ları eski pin davranışına saygı gösterir.
- **Pinned `TodoPanel` — kutu yok ama opak:** composer üstündeki pinned "Görev Listesi"
  tepsisinden **iç bubble-kutusu** kaldırıldı (`bg-surface-2`+`border` yok → düz görünür),
  ama **dış tray opak** tutuldu (`bg-[var(--color-bg)]`+`border-t`) → arkasındaki transkript
  **sızmaz** (şeffaf yapılınca içerik geçiyordu; opak zemin bunu keser). Transkript içindeki
  katlanabilir iz kartları (`TodoCard`/`DiffCard`/`ThinkingBlock`/`TextStep`) arka-planını
  korur + **`shadow-sm` gölge** eklendi. `tsc` + prod build yeşil.

## Artifact gruplama: Skills paritesi (katlanabilir gruplar + toplu grup atama) ✅ (2026-07-07)

**İstek:** Artifactları da skiller gibi gruplayabilelim.

**Ne yapıldı:**

- **Backend:** `Artifact` modeline first-class `Group string` alanı (`models_artifact.go`);
  `DB.SetArtifactGroup(ctx, id, group)` yalnız `group`'u yazar (`store_artifact.go`);
  `handleSetArtifactGroup` + rota `PUT /api/artifacts/{id}/group` (`artifacts.go`,
  `server.go`). Skill gruplamasından fark: skill'de grup frontmatter'da, artifact'ta
  entity JSON alanında (artifact'lar dosya-tabanlı entity).
- **Frontend:** `Artifact.group?` tipi + `api.setArtifactGroup`. `ArtifactsPanel`
  `useGroupedList` ile kovalanır (arama+origin filtresinden **sonra**): katlanabilir
  grup başlıkları, tümünü katla/aç, SelectionBar'da grup input'u (`datalist` önerili)
  - "Ata"/"Grupsuz" (Enter da uygular). `orderedIds` görünür (katlanmamış) sırayı
    izler → shift-aralık folded grupları atlar. Detay başlığında grup rozeti. Collapse
    durumu `tionharness.artifactsCollapsedGroups`'ta kalıcı.
- **Doğrulama:** `go build ./...` + `go test ./internal/db ./internal/api` (135 passed)
  - frontend `tsc` yeşil. Docs: `45-COKLU-SECIM.md` (tablo + not), `SKILL.md` artifact satırı.

## Composer odağı: yalnız yeni sohbet açılınca input'a odaklan ✅ (2026-07-07)

**İstek:** Bir sohbet penceresi açınca input'a odaklanma **yalnız yeni sohbet
oturumu açılınca** olsun; mevcut oturumlara tıklayınca input otomatik odaklanmasın.

**Ne yapıldı (yalnız frontend):**

- **Kök neden:** `Composer.tsx` odak effect'i `[sessionId, disabled]`'e bağlıydı →
  Composer oturum geçişinde remount olmadığı için (`composerKey` yalnız rewind'de artar)
  **her** oturum değişiminde `taRef.focus()` çalışıyordu (mevcut oturuma tıklayınca da).
- **Çözüm:** `App.tsx`'e `focusSessionId` state'i eklendi; **yalnız** `newSession()`
  bunu yeni oturum id'sine set eder. Composer'a `focusSessionId` prop'u geçildi; odak
  effect'i artık yalnız `sessionId === focusSessionId` iken odaklanır. Mevcut oturum
  seçimi (sidebar) bu sinyali değiştirmediğinden odak çalınmaz. İlk açılışta (mevcut
  oturum gösterilir) `focusSessionId=null` → odak yok.
- **Rewind korundu:** `handleRewind` de `setFocusSessionId(activeSessionId)` yaparak
  remount sonrası imleci input'a bırakır (eski davranış). Frontend `tsc` yeşil.

## Skills ekranı: son düzenleme tarihi + grup içi recency sıralaması ✅ (2026-07-06)

**İstek:** Skills ekranında her becerinin son düzenleme tarihi görünsün ve skill
grupları içinde güncelden eskiye doğru sıralansın.

**Ne yapıldı:**

- **Backend:** `skills.Skill`'e `ModifiedAt int64` (`json:"modifiedAt"`, Unix saniye)
  alanı eklendi; `store.go scanDir` her `SKILL.md`'yi `os.Stat` ile damgalar (best-effort,
  stat başarısızsa 0). API `Skill`'i doğrudan serialize ettiği için ek endpoint gerekmedi.
- **Frontend:** `types/skill.ts` `modifiedAt?: number`; `SkillsPanel.tsx` listeyi
  `modifiedAt` DESC sıralayıp `useGroupedList`'e verir (hook grup-içi giriş sırasını korur →
  her grup en yeni düzenlenenden eskiye sıralanır). Liste öğesinde "Düzenlendi: <relatif>"
  satırı (`relativeTime`, hover'da tam tarih), detay panelinde "Son düzenleme: <tam tarih>".
- **Not:** `store.List()` prompt kataloğu için hâlâ ada göre sıralı — yalnız UI sunum
  sırası değişti. `go build` + frontend `tsc` yeşil.

## Pano (kart) tetikleyicili otomasyonlar ✅ (2026-07-06)

**İstek:** Otomasyonlara cron + etiket türlerine ek olarak, **Board'daki kart
değişimlerinde** çalışan bir tetik türü ekle — genel kart değişimlerini dinlesin;
bir kart bir board'a taşınınca ajan veya flow çalışabilsin.

**Ne yapıldı:** Aynı `Automation` entity'sine ikinci bir tetik türü eklendi
(`TriggerKind`: `""`/`tag` varsayılan, `board` yeni). Board türü alanları:
`BoardOp` (`move` vars./`create`/`update`/`delete`/`any`), `BoardFromState`,
`BoardToState` (sütun filtreleri).

- **Tek nokta tetik (db hook):** UI ve ajan araçları kart mutasyonlarını hep
  `db` katmanından (`CreateTask`/`MoveTask`/`UpdateTask`/`DeleteTask`) geçirdiği için
  gözlemci `DB.SetBoardHook`/`BoardChangeEvent` ile **db seviyesine** kondu; kilit
  bırakıldıktan sonra çağrılır, manager onu ayrı goroutine'de `OnBoardChange`'e bağlar
  (mutasyon bloklanmaz). Move yalnız sütun **gerçekten** değişince ateşler; update
  sütun değiştiyse `move` aksi halde `update`.
- **Motor:** `AutomationEngine.OnBoardChange` + `boardMatches` + `fireBoard` +
  `boardVars` (`{{taskId}}/{{title}}/{{op}}/{{from}}/{{to}}/{{toLabel}}/{{board}}` …).
  Guardrail bloğu `guardsPass`'e çıkarılıp tag/board yolları paylaşır. Board otomasyonu
  kendini döngülemez (spawn'lanan oturum etiket taşımaz); MaxIterations/Cooldown sınırlar.
- **API + araçlar:** `automationReq` + `create/update/list_automation` yeni alanları alır;
  board türü `triggerTag` istemez, `boardOp` doğrulanır; update kısmi patch'te `triggerKind`
  verilmezse dokunulmaz (board→tag kazara dönüşümü önlenir).
- **UI — birleşik 3 SEKME (2026-07-07 güncelleme):** Otomasyon ekranı tek bir tab bar altında:
  **⏰ Zamanlamalar (cron)** · **🏷 Etiket otomasyonları** · **🗂 Pano otomasyonları** (canlı sayaç
  rozetleri). Tab state `Schedules.tsx`'te; `schedules` sekmesinde cron başlık+form+liste, diğer
  sekmelerde `Automations`. `Automations` **kontrollü** hâle geldi (`activeKind: 'tag'|'board'|null`;
  `null` → render yok ama mount kalır → `onCounts` ile sayaçlar canlı). Her `AutomationSection`'ın
  kendi formu/listesi/edit state'i; tür toggle'ı yok. Board bölümünde olay + kaynak/hedef sütun
  seçicileri; ortak `PromptVarsField`; board satırında `🗂 <op> (…→…)` çipi.
- **Test:** `boardMatches`/`boardVars` (agent) + `store_task_hook_test.go` (db hook 5 olay).
  `go build ./...` + `go test ./internal/db ./internal/agent` (175) + frontend `tsc` + prod build yeşil.
- **Dok:** `_Docs/46-ETIKET-OTOMASYON.md` §2.5 + intro; SKILL.md otomasyon maddesi.

## Dar ekranda liste drawer'ı seçim yoksa otomatik açılır (Aktivite + Akışlar) ✅ (2026-07-07)

**İstek:** Activities ekranına girince hiçbir aktivite seçili değilse dar
ekranlarda aktivite listesi paneli otomatik açılsın; aynısı Akışlar ekranında
flow listesi için.

**Ne yapıldı:** `ExecutionsPanel` ve `FlowsPanel`'e mount-once `useEffect` — seçim
yoksa (`!selectedId`) `useCollapsibleList.setOpen(true)` ile liste drawer'ı açılır.
`useCollapsibleList.open` yalnız dar ekranlarda etkili (md+ CSS ile hep görünür) →
otomatik açılma sadece dar ekranı etkiler, geniş ekranda no-op. Paneller view'e
göre remount olduğundan effect her girişte çalışır. `frontend tsc --noEmit` temiz.

## Ajanlar kullanıcının görevlerini de silebilir (delete_task köken kısıtı kaldırıldı) ✅ (2026-07-06)

**İstek:** Boards (kanban) ekranındaki görevleri ajanlar da silebilsin —
kullanıcının oluşturdukları dahil.

**Ne yapıldı:** `builtin_taskmgmt.go` `delete_task` aracındaki köken (provenance)
kısıtı kaldırıldı. Önceden `cur.CreatedBy == ""` (kullanıcı görevi) ise silme
reddediliyordu; artık **her görev** silinebilir (yalnız var-olma kontrolü kalır;
yok id → net hata, sessiz no-op değil). Açıklamalar + dosya-başı güvenlik yorumu +
`list_tasks` açıklaması güncellendi (`CreatedBy` yalnız provenance/gösterim için
damgalanmaya devam eder). Test `TestDeleteTaskGuard` → `TestDeleteTaskAny` (kullanıcı

- ajan görevi silinebilir + yok-id hatası). Self-management default SKILL + proje
  skill notu güncellendi. `go build ./...` + `go test ./internal/tools -run Task` (4)
  yeşil.

## Chat başlığı = oturum title + ayrı "Debug" paneli ✅ (2026-07-06)

**İstek:** (1) Chat header'ında "Sohbet · Manager" yerine oturumun **title**'ı
yazsın. (2) Sağdaki "Bağlam" butonunun yanına **"Debug"** butonu. (3) "Oturum
bilgisi" içindeki Debug parçalarını **yeni bir panele** taşı; title'daki Debug
butonuyla açılsın.

**Ne yapıldı (frontend):**

- **Başlık:** `App.tsx` chat header artık `VIEW_TITLE`+ajan yerine `sessions.find(
...).title` gösterir (fallback: ajan adı → "Yeni sohbet"). Diğer view'ler
  değişmedi.
- **Debug butonu:** header'da "Bağlam" ile "Detay" arasına `Bug` ikonlu buton →
  `setDebugOpen(true)`.
- **Yeni panel:** `SessionDebugModal.tsx` — `ModalOverlay` içinde başlık + kapat;
  gövdede `SessionDebugCard`'ı **`alwaysOpen`** modunda render eder (iç katlama yok,
  chrome'u modal verir). `SessionDebugCard`'a `alwaysOpen` prop'u eklendi (açık
  başlar, katlama başlığı gizlenir).
- **Taşıma:** `SessionDetailPanel`'den `SessionDebugCard` (+import) kaldırıldı;
  Debug artık yalnız modalda. Modal `agents`'tan agentId→name map'i alır (viz
  şerit/düğüm etiketleri). `frontend tsc --noEmit` temiz.

## "Oturum bilgisi" paneli genişletilebilir (drag-resize) ✅ (2026-07-06)

**İstek:** Sağdaki "Oturum bilgisi" paneli (SessionDetailPanel) sabit `w-80`
genişlikteydi; sürükleyerek genişletilebilir olsun.

**Ne yapıldı:**

- **`useResizableSidebar` hook'una `invert` seçeneği:** sağ-taraf paneli sol
  kenardan sürüklendiğinde (clientX azalırken) **büyümesi** için delta ters
  çevrilir. Deps'e eklendi.
- **`ResizeHandle`'a `side` prop'u:** `'right'` (varsayılan, sol-liste kolonları)
  veya `'left'` (sağ panel). `left-0`/`right-0` konumlandırma.
- **SessionDetailPanel:** sabit `w-80` → `useResizableSidebar({storageKey:
'tionharness.sessionInfoWidth', default 320, min 280, max 640, invert:true})` +
  inline `style.width`. Sol kenarda `ResizeHandle side="left"`. Handle içerik
  kaydırılınca kaymasın diye aside `overflow-hidden` yapıldı, scroll **iç
  sarmalayıcıya** taşındı (ListPane deseni). Mobil çekmece için genişlik
  `max-md:!w-[85vw] max-md:!max-w-sm` ile bağlandı (inline stili `!important`
  ezsin diye). Genişlik `localStorage`'da kalıcı. `frontend tsc --noEmit` temiz.

## İş akışı görselleştirmeleri: Araç Sankey + Eşzamanlılık zaman çizelgesi ✅ (2026-07-06)

**İstek:** CCAM'in Workflows ekranındaki "Tool execution Sankey" ve "Concurrency
timeline" görselleştirmelerini TionHarness'e ekle.

**Ne yapıldı (yalnız frontend; mevcut `debug.jsonl` verisinden, ekstra backend yok):**

- **Veri katmanı (saf):** `frontend/src/components/sessions/viz/flowVizData.ts` —
  `buildToolSankey` (tool olaylarını `Ajan → Araç → Tamam|Hata` mermaid `sankey-beta`
  koduna toplar; hata dalı yalnız hata varsa) + `buildConcurrencyTimeline` (zaman
  damgalı `llm_call`/`turn` olaylarını ajan-şeritli bar modeline; olay COMPLETION'da
  damgalandığı için bar = `[ts−durMs, ts]`; sweep-line ile şeritler-arası çakışma =
  `hasOverlap`). Yan-etkisiz → test edilebilir.
- **Sankey bileşeni:** `viz/ToolSankey.tsx` — mevcut `MermaidDiagram`'ı (lazy mermaid,
  tema-duyarlı, expand) `sankey-beta` koduyla besler.
- **Zaman çizelgesi:** `viz/ConcurrencyTimeline.tsx` — bağımlılıksız SVG Gantt
  (Sparkline desenine uygun); ajan başına şerit, zaman-eksenli barlar (hata=kırmızı),
  3 eksen tick'i, seri/eşzamanlı rozeti, `<title>` tooltip.
- **Kapsayıcı:** `viz/SessionFlowViz.tsx` — katlanabilir "İş akışı görselleştirmeleri"
  bölümü; açılınca olayları lazy çeker (limit 500, truncation notu), iki görseli üst
  üste render eder. Durum `localStorage`'da (`tionharness.flowVizOpen`).
- **Bağlama:** `SessionDebugCard`'a `agentNames` opsiyonel prop + model dökümünden
  sonra `SessionFlowViz` render; `SessionDetailPanel` `info.agents`'tan agentId→name
  map'i geçirir. Yer: **oturum detay panelinin Debug bölümü**.
- **Not:** Tek-ajan seri oturumda zaman çizelgesi "seri" görünür; koordinatör/çok-ajan
  oturumlarda şerit-çakışması gerçek eşzamanlılığı gösterir. `frontend tsc --noEmit`
  temiz (lint `set-state-in-effect` kuralı repo-genelinde mevcut, idioma uygun).

## Fix: Yürütme/oturum listesi sırası her poll'de değişiyordu ✅ (2026-07-06)

**Belirti:** Aktivite ekranındaki yürütme listesi (ve sol oturum listesi) "durduk
yere" sürekli yeniden sıralanıyordu.

**Kök neden:** `DB.ListSessions` kaynak `d.sessions` **map**'i üzerinde dönüyor →
Go map iterasyon sırası her çağrıda rastgele. Sıralama yalnız `(Pinned, UpdatedAt
desc)` idi; **eşit `UpdatedAt`** (saniye-hassasiyetli damga, aynı anda güncellenen
oturumlar) olan satırlar `SliceStable`'da rastgele gelen map sırasını koruyordu →
her 5 sn'lik poll'de yer değiştirme.

**Düzeltme:** İki sıralamaya da kararlı **ID tie-break** eklendi:
`store.go ListSessions` (`ID` desc) + `api/executions.go handleListExecutions`
(`SessionID` desc). Artık eşit-zamanlı satırlar sabit sırada. `go build ./...` yeşil.

## Oturumlar toplu tablo görünümü (Aktivite ekranı) ✅ (2026-07-06)

**İstek:** Claude-Code-Agent-Monitor'ün "Sessions" ekranı gibi tüm oturumları
tek tabloda topluca (aranabilir/filtrelenebilir/sıralanabilir) görebilmek. Aktivite
ekranında yürütme listesinin en üstünde bu tabloyu açan bir buton olsun.

**Ne yapıldı:**

- **Paylaşılan meta çıkarıldı:** `frontend/src/components/panels/executionsShared.tsx`
  — `KIND_META`/`FILTERS`/`kindMeta`/`shortId`/`StatusPill` `ExecutionsPanel`'den
  buraya taşındı; hem panel listesi hem yeni tablo aynı kaynağı kullanır (kind rozeti/
  filtre/kimlik-kısaltma/durum pill'i birebir aynı).
- **Yeni bileşen:** `SessionsOverview.tsx` — `ModalOverlay` içinde geniş tablo.
  Kolonlar: Başlık (canlı/okunmadı noktası + kopyalanabilir kısa kimlik), Tür, Ajan
  (avatar), Mesaj, Durum (çalışıyor/başarılı/hata), Süre (bitmiş=created→updated,
  canlı=created→now), Oluşturma, Güncelleme. Kolon başlığına tıkla → sırala (aynı
  kolon yön çevirir; sayısal/tarih kolonları azalan varsayılan). Arama başlık/kimlik/
  ajan üzerinde; kind filtre sekmeleri. Ekstra fetch YOK — panelin zaten yüklü
  `listExecutions()` verisini kullanır. Satıra tıkla → o yürütmeyi seç + overlay kapan.
- **Panel entegrasyonu:** `ExecutionsPanel` "Yürütmeler" başlığına `Table2` ikonlu
  buton (`data-testid="sessions-overview-open"`) + `overviewOpen` state + koşullu
  overlay render. `frontend tsc --noEmit` temiz.

## Capability Probe → Context Genişletme + per-workspace codebase-memory store ✅ (2026-07-06)

**İstek:** Cihazda `codebase-memory-mcp` **mevcutsa** yeni oturumların sistem
promptuna **kısa + cachelenebilir** bir bilgi bloğu enjekte et → ajan varlığını
bilsin ve workspace indeksleme/arama araçlarını kullansın. Tespit + genişletme
katmanı **generic** olsun (ileride başka tool'lar tek kayıtla eklenebilsin). Ayrıca
her workspace kendi **izole** codebase-memory store'unu kullansın. Tam tasarım: **\_Docs/54**.

**Ne yapıldı:**

- **Generic katman:** `internal/agent/capabilities.go` — `Capability{ID, Detect, Context}`
  - `[]capabilities` kaydı + `(*Runtime).CapabilityContext(ctx, cwd)`. `Detect` MCP-sunucu
    varlığı / on-PATH binary / settings-flag olabilir → yeni tool = slice'a bir kayıt.
- **İlk müşteri codebase-memory:** enabled stdio MCP sunucusunun `Command`'ında
  `codebase-memory-mcp` işareti aranarak tespit; kısa statik blok (araç-tercihi + izole
  store + cwd'den türetilen `project` id, `projectIDForPath`). Path→id kuralı
  codebase-memory ile birebir (iki gerçek örnekle test edildi), ıskalarsa `list_projects`
  hedge'i.
- **Enjeksiyon (iki senkron assembler):** chat `api.composeTurnRequest` statik prefix'e
  (cwd/project id'li) + headless `agent.autonomousSystemPrompt` (cwd-siz farkındalık).
  Cache breakpoint bozulmaz (presence sabit; sunucu yokken blok "" → no-op).
- **Per-workspace store (§C):** `(*Runtime).CBMStoreDir()` = `<workspace-container>/cbm-store`
  (skills/hook-scripts kardeşi). `toolsetup.go` codebase-memory sunucusunun stdio env'ine
  `CBM_CACHE_DIR`'i enjekte eder (user-set kazanır) → store = workspace sınırı, indeksler
  karışmaz.
- **Auto-index (§C):** `(*Runtime).EnsureCodebaseIndexed(ctx, cwd)` — best-effort, arka
  plan, `(cwd,store)` başına süreç-içi tek sefer (`cbmIndexed sync.Map`); `command cli
index_repository` + `CBM_CACHE_DIR`. Hata **loglanır** (yutulmaz), guard silinip sonraki
  tur retry olur.
- **Test:** `capabilities_test.go` — `projectIDForPath` (gerçek örnekler), `codebaseMemoryCommand`
  (stdio-match/http-skip/absent), `CBMStoreDir`. `go build ./...` + `go test ./internal/agent`
  yeşil.
- **Takip (aynı gün):** (1) **Headless project id** — `autonomousSystemPrompt(ctx,a)` +
  `sessionCwd(ctx)` → headless turlar da cwd/project id + auto-index alır (executor+subagent
  ctx'li çağrı). (2) **Workspace-geneli arama** — `codebase_workspace_search`
  (`builtin_codebase_search.go`): `list_projects`→her project'e `search_code` fan-out,
  project'e göre birleşik sonuç (`limit` 30 / `maxCodebaseProjects` 40); `RiskRead` +
  `CategorySearch`; yalnız enabled codebase-memory sunucusu varsa kayıtlı. (3) **Frontend** —
  `ToolsPanel.tsx` MCP sunucu satırında **"izole store"** rozeti+tooltip
  (`data-testid="mcp-server-isolated-store"`). `go build`/`go test ./internal/agent
./internal/tools` + `npm run build` yeşil.
- **Aç/kapa toggle + canlı duman testi (2026-07-07):** Tüm yetenek workspace ayarıyla
  açılıp kapanır (default açık): `WSSettings.CodebaseMemoryEnabled` → `Runtime` atomic gate →
  `codebaseMemoryCmd` tek choke-point (kapalı → hint/auto-index/tool/env hepsi kaybolur).
  UI: `WorkspacePanel` Toggle + `ExternalToolsPanel` callout ("Ayarlar ▸ Bu Workspace"). **Smoke:**
  CLI kontratı (izole store index→list→search fan-out) + binary boot (panic yok) + canlı toggle
  round-trip (default true→PUT false→re-GET false→PUT true). Test, `workspaceSettingsDTO`'da
  **eksik alan** bug'ını yakaladı → GET her zaman boş dönüyordu; DTO'ya eklenip giderildi.
  `go test ./internal/{agent,tools,workspace,api}` + `npm run build` yeşil.

## `archive_sessions` (workspace-scoped toplu oturum arşivleme) ✅ (2026-07-06)

**İstek:** Bir oturum incelemesinde ajanın "başka oturumları temizle" isteğinde
`update_session` yalnız **mevcut** oturumu arşivleyebildiği için **ham REST**'e
(`Invoke-RestMethod /api/sessions/{id}/state`) düştüğü, `X-Workspace-Id` header'ı
verilmeyince **yanlış (Default) workspace'i** arşivlediği tespit edildi. Bu boşluğu
kapatan workspace-scoped toplu-arşiv aracı gerekiyordu.

**Ne yapıldı:**

- **Araç:** `internal/tools/builtin_sessionarchive.go` `archive_sessions` — `r.db`'ye
  bağlı (fiziksel workspace scope), **mevcut oturumu daima hariç tutar**
  (`SessionIDFrom(ctx)` build anında). Filtreler: `idle_days` (N günden eski),
  `title_contains`; `exclude` (ek koru), `dry_run` (önizleme), `limit` (vars. 100
  güvenlik tavanı). Yalnız `Kind=="chat"` + `State=="active"` hedefler; arşiv
  soft/geri alınabilir (`SetSessionState(...,"archived")`).
- **Kayıt:** `toolsetup.go` — `list_sessions`'ın yanına, `SessionContextEnabled()`
  gate'i altında eager. `categories.go` → `CategoryAgents`. Risk sınıfı listelenmedi
  → varsayılan `RiskWrite` (mutasyon; "ask" modda onay ister).
- **Test:** `builtin_sessionarchive_test.go` (3 test): current+non-chat+archived
  hariç tutma & scope; `dry_run` değiştirmez; `title_contains` + `idle_days` filtre.
  `go build ./...` + `go test ./internal/agent ./internal/tools` yeşil.
- **Doküman/skill:** `24-SELF-MANAGEMENT.md` yeni "Oturum yönetimi (workspace-scoped)"
  satırı; `tionharness-project` + `tionharness-session-debug` skill'leri (yeni §7 kök-neden).

## `render_template` + `html-preview` (şablonlu HTML render) ✅ (2026-07-06)

**İstek:** the external agent project'ın "Source Templates / `render_template`" özelliğini TionHarness'e taşı —
motor markalı HTML şablonu doldurur, modele **yalnız dosya yolu** döner (HTML değil → token
tasarrufu), sohbette **inline izole iframe**'de gösterilir. Tam tasarım: **\_Docs/53**.

**Ne yapıldı:**

- **Motor:** Go `html/template` (auto-escape/XSS-güvenli). `internal/tools/render_template.go`
  (saf: render + sidecar `.meta.json` oku + missing-field), `builtin_render_template.go` (Tool),
  `builtin_render_template_test.go` (8 test: escape, soft-warn, hard-fail'ler, traversal, no-session).
- **Session çıktı dizini:** `internal/agent/renderdir.go` `SessionRenderDir` = `<db.Root>/render/<sid>`
  (`progress/`'in kardeşi). Session yoksa boş → araç hard-fail.
- **Kayıt:** `toolsetup.go` builtins + `MarkNameOnly("render_template")`. Shell gate GEREKTİRMEZ (saf render).
- **Servis:** `internal/api/files.go` `serveTextFile` — `GET /api/files?path&as=text` yalnız render
  kökü altını (`underDir` whitelist) `text/plain`+`nosniff` ile döndürür (asla `text/html`).
- **Inline UI:** `HtmlPreview.tsx` (`html-preview` → `<iframe srcDoc sandbox="allow-scripts">`,
  opaque origin, tab desteği) + `CodeBlock.tsx` dispatch + `attachments.tsx` `fileTextURL`.
- **Şablonlar:** skill-bundled `tionharness-templates` (report + email şablonu + meta). "Template store"
  alt-sistemi KURULMADI (source kavramı yok; `${SKILL_DIR}` yeterli).
- **Soft vs hard:** eksik `requiredField` → render + warning; bozuk template/JSON/sidecar → hata.
- **Prompt + guard:** "## Rendering"e `html-preview`/`render_template` eklendi; `defaults_test.go`
  bu ikisini yasak listesinden çıkardı (artık TionHarness-native).
- **Ömür/temizlik (aynı gün eklendi):** render çıktısı iki katmanda toplanır —
  `deleteSessionFilesLocked` session silmede `<render>/<sid>`'i siler; `DB.Open` → `cleanupRenders`
  startup'ta orphan dizinleri + `renderTTL` (14g) üstü eski dosyaları temizler, boş dizini budar.
  Yol tek kaynak `db.RenderDir(sid)`. Dosyalar: `internal/db/render_cleanup.go` + 3 test.

**Doğrulama:** `go build ./...` ✓, `go vet` ✓, tools+agent 304 test ✓, db+agent 169 test ✓ (render sweep 3/3),
workspace+skills 35 test ✓, `tsc` ✓.
**AÇIK:** UI'da gerçek bir render'ın görsel doğrulaması (iframe + `as=text` fetch) canlı denemeyle yapılmalı.

## Uygulama-içi claude-cli OAuth login (tarayıcıyla, popup senkron) ✅ (2026-07-06)

**İstek:** Terminal açmadan, popup'tan tıklayarak Max/Pro girişi — link verilir, tarayıcı
açılır, popup'a koddan yapıştırılır, arka planda kimlik senkron yazılır.

**Yaklaşım B (native PKCE):** `claude setup-token`/`auth login` interaktif TUI'sini sürmek
yerine OAuth authorization-code + PKCE akışını **Go'da kendimiz** kurduk. OAuth istemci
sabitleri sağlayıcının resmî CLI'ından ve public client-metadata dokümanından alınır
(domain'ler `platform.claude.com`/`claude.com/cai`'ye taşınmış — eski `console.anthropic.com`
değil):

- public client_id, authorize `claude.com/cai/oauth/authorize`,
  token `platform.claude.com/v1/oauth/token`, redirect `platform.claude.com/oauth/code/callback`, S256.

**Parçalar:**

- `internal/claudeauth/oauth.go` — `Begin()` (PKCE verifier/state + authorize URL),
  `Exchange()` (kod→credential, state doğrulama, `code#state` parse), `WriteCredentials()`
  (`<home>/.credentials.json`'a `claudeAiOauth{...}` atomik yaz, .bak yedek). Birim test 3/3.
- API: `POST /api/workspace-settings/claude-auth/oauth/{start,complete}` — start URL+flowId
  döner (verifier server-side stash, 10dk TTL), complete kodu exchange edip **aktif
  workspace'in claude-home'una** yazar (refresh token'lı → CLI kendi tazeler).
- Frontend: `ClaudeAuthDialog`'a **"Tarayıcıyla giriş"** sekmesi (varsayılan) — "Giriş başlat"
  → URL aç → `kod#state` yapıştır → "Girişi tamamla" → ✓. Manuel token-paste + API-key
  sekmeleri yedek kaldı. `api.startClaudeOAuth`/`completeClaudeOAuth`.

**AÇIK DOĞRULAMA:** Canlı token-exchange (platform.claude.com'a gerçek POST) yalnız gerçek
bir login ile doğrulanabilir — sabitler doğru ama scope/param ince ayarı gerekirse tek dosya
(`oauth.go` const bloğu). Go build+vet+78 test yeşil, tsc temiz.

**Loopback (paste'siz) varyant ✅ (2026-07-06):** İkinci akış eklendi —
`http://localhost:<port>/callback` redirect'ini kullanır (`LoopbackConfig`). Backend efemeral portta yerel dinleyici açar (`claude_oauth_loopback.go`);
tarayıcı yetkilendirmeden sonra doğrudan geri döner, callback handler kodu exchange edip
credential'ı yazar, tarayıcıya HTML başarı sayfası basar. Popup `.../oauth/loopback/status`'ı
poll eder → paste GEREKMEZ. `oauth.go` `Begin`→`BeginWith(FlowConfig)` refaktörüyle iki akış
tek çekirdeği paylaşır (`PendingLogin` client/redirect taşır). Popup'ta "Otomatik (önerilen)"
vs "Elle kod" alt-modu; otomatik varsayılan. Yalnız tarayıcı backend ile aynı makinedeyken
(masaüstü/yerel) çalışır — uzak/LAN'da manuel-paste'e düşülür.

**Loopback client_id fix ✅ (2026-07-06):** Otomatik (loopback) akış tarayıcıda "OAuth Request
Failed — client_id: Input should be a valid UUID, found `h` at 1" hatası veriyordu. Sebep:
`LoopbackConfig` client_id olarak metadata-doküman URL'i (`https://claude.ai/oauth/
claude-code-client-metadata`) gönderiyordu, ama `claude.com/cai/oauth/authorize` endpoint'i
client_id'yi **UUID** olarak doğrular ve URL'i (`https`'in `h`'sinden) reddeder. Düzeltme:
loopback artık manuel akışla **aynı public UUID client**'ı kullanır. Ayrıca
redirect_uri `127.0.0.1` → **`localhost`** yapıldı: authorize endpoint 127.0.0.1'i localhost'a
normalize ediyor; token-exchange redirect_uri'si normalize edilmiş biçimle eşleşmezse
"redirect mismatch" olurdu. Yerel dinleyici hâlâ 127.0.0.1'e bind (tarayıcı localhost'u ona
çözer). Canlı doğrulama: UUID+localhost authorize URL'i artık UUID hatası vermiyor (yalnız
oturumsuz 403). Tek dosya: `internal/claudeauth/oauth.go` (`loopbackClientID` const kaldırıldı).

**Rebuild+test (2026-07-06):** `go build ./cmd/tionharness` (binary + gömülü dist) ✓,
`go vet ./...` ✓, `go test ./...` **673 test / 34 paket** ✓, frontend `npm run build` ✓, tsc temiz.

## Oturum bilgisi panelinde arka-plan süreç kontrolü (gör/durdur/yeniden başlat/tazele) ✅ (2026-07-06)

**İstek:** Bir sohbetin arkasında çalışan claude-cli/provider işlemini "Oturum bilgisi"
panelinde görebilmek ve durdurma/yeniden başlatma yapabilmek. 3 tier uygulandı:

**Tier 1 — Gör + Durdur (backend-driven, detached/autonomous turlar için de sağlam):**

- `chatRun`'a `startedAt` + `provider` (setter `setProvider`, `chat_stream` agents[0]'dan
  doldurur); `chatRuns.sessionRunInfo(sid)` snapshot döndürür.
- `sessionInfoResp`'e `running {runId, startedAt, autonomous, provider}` + `warmCliProcess`.
- Panelde canlı "Tur çalışıyor" kartı (geçen süre sayacı, provider rozeti) + **Durdur** →
  mevcut `POST /api/chat/control {action:stop}` → `run.cancel()` → `exec.CommandContext`
  claude.exe'yi öldürür. Panel `running`/`warmCliProcess` varken 3sn'de bir poll eder.

**Tier 2 — Yeniden başlat:** chat hook'una `rerunLast()` (son asistan turunu `retryMessage`
ile tekrar; yoksa son user mesajını yeniden gönder) → App `onRerun` ile panele bağlar.
Panel önce backend runId ile durdurur, sonra yeniden gönderir (tek gerçek tur yolu korunur).

**Tier 3 — Persistent süreç tazeleme:** `CLISessionPool`'a `AliveForSession`/`DropSession`
(pool anahtarı `<sid>|<agentID>` prefix eşleşmesi) + Runtime `HasWarmCLISession`/
`DropWarmCLISession` + `DELETE /api/sessions/{id}/cli-process`. Panelde "Sıcak claude-cli
süreci" satırı + **Tazele** → sonraki tur cold-restart (konuşma korunur). Yalnız
persistent-pool modunda görünür.

**Dosyalar:** `internal/api/{chat_control,chat_stream,session_info,sessions,server}.go`,
`internal/agent/runtime.go`, `internal/providers/claudecli_session.go`,
`frontend/src/{types/session.ts, api/sessions.ts, hooks/useChatStream.ts,
components/sessions/SessionDetailPanel.tsx, App.tsx}`. Go build+vet+159 test yeşil, tsc temiz.

## claude-cli auth hatası: sınıflandırma + ön-uçuş probe + terminal etiket ✅ (2026-07-06)

**Problem:** claude-cli sağlayıcılı bir ajanın workspace claude-home'u giriş yapmamışsa
tur ilk LLM çağrısında `authentication_failed` / "Not logged in · Please run /login"
ile reddediliyordu. Bu hata **jenerik `claude CLI failed: exit status 1`'e** düşüyor,
`retryable=true` ile **boşuna 2. kez** deneniyor, kullanıcıya hangi claude-home'un login
gerektirdiği söylenmiyordu. (Rate-limit için çözülmüştü, auth atlanmıştı.)

**5 maddelik çözüm:**

1. **Sınıflandırma (retry-EDİLMEZ):** `claudecli.go` yeni `isAuthErrorText` +
   parser `notLoggedIn`/`authMsg` alanları; `feed()` hem standalone
   `{"error":"authentication_failed"}` satırını (yeni `cliEvent.Error`) hem result-error'ı
   yakalar. `runAttempt` rate-limit dalının üstünde net, retry-edilmez auth hatası döndürür.
2. **Aksiyon mesajı:** Hata metni ilgili `CLAUDE_CONFIG_DIR=<claude-home>` yolunu + "`claude
/login` çalıştır veya API-key sağlayıcıya al" önerisini gömer.
3. **Ön-uçuş probe:** `ClaudeCLI.ProbeAuth(ctx)` (araçsız minimal `claude -p`, aktif
   workspace'in claude-home'unu test eder) + `GET /api/workspace-settings/claude-auth`
   (`handleWorkspaceClaudeAuth`) + Ayarlar→Sağlayıcılar'da **"Bu workspace login doğrula"**
   butonu (`ProvidersPanel.tsx`, `api.checkWorkspaceClaudeAuth`). Not: jenerik "Test et"
   global config dir'i dener; bu probe workspace-özeldir.
4. **Terminal etiket:** `autotag.go` yeni `TagAuthError = "auth-error"` — auth hatası
   turlarına `error`'a ek olarak eklenir; auto-repair otomasyonu bunu **dışlamalı**
   (login onarılamaz, aksi halde MaxIterations'a kadar boşuna döner).
5. **Doküman:** `tionharness-session-debug` skill'ine "I) authentication_failed" deseni +
   ön-uçuş probe reçetesi eklendi.

**Dosyalar:** `internal/providers/claudecli.go`, `internal/agent/autotag.go`,
`internal/api/workspace_settings.go` + `server.go` (route), `frontend/src/api/workspaces.ts`,
`frontend/src/components/settings/ProvidersPanel.tsx`. Go `build ./...` + `tsc --noEmit` yeşil.

## Schedule manuel "Run" turu sayfa yenileyince kesiliyordu ✅ (2026-07-05)

**Bug:** Bir schedule'ı elle "Run" ile çalıştırıp (senkron `POST /api/schedules/{id}/run`)
tur devam ederken sayfayı yenileyince/başka yere gidince tur **yarıda kalıyordu**
(yarım asistan cevabı persist, `lastDeliveryStatus=success`, hata bayrağı yok).

**Kök neden:** `handleRunSchedule` → `Scheduler.RunNow(r.Context(), id)` turu **istek
context'ine** bağlıyordu. Tarayıcı yenileme POST'u abort eder → `r.Context()` iptal →
`deliverPrompt`/`invokeTraced`/claude-cli turu üretim ortasında iptal. (Cron-fire zaten
`context.Background()` ile detach; sadece manuel Run bağlıydı.)

**Fix:** `handleRunSchedule` artık turu istekten ayırıyor:
`context.WithTimeout(context.WithoutCancel(r.Context()), 10*dk)` — chat-stream detach'ı
gibi. Böylece yenileme/navigasyon turu kesmiyor; 10dk güvenlik timeout'u hung run'ı sınırlar.

**Doğrulama (empirik):** eski binary'de RunNow'ı 12sn'de abort → tur 12sn'de kesildi
(out=4, partial). Fix'li binary'de aynı abort → tur detached tamamlandı (2045 char, 197sn).
Kesintisiz RunNow zaten tam çalışıyordu (175sn, tam plan + artifact). Dosya:
`internal/api/schedules.go`.

**Flow tarafı (kontrol edildi, değişiklik gerekmedi):** 4 flow-run endpoint'i
(`handleRunFlow`/`handleRunFlowStream`/`handleSessionRunFlow`/`handleSessionRunFlowStream`)
zaten `context.WithoutCancel` ile detached (`flows.go`). **Flow-backed schedule** manuel
Run'ı aynı `Scheduler.run(ctx)` → `deliverFlow` yolundan geçtiği için bu fix onu da
kapsar. Cron/automation/agent-tool flow yolları zaten server-side (istemciye bağlı değil).
Tek boşluk prompt-backed schedule manuel Run'dı.

## İlk kurulum: "Mevcut Workspace Seç" butonu ✅ (2026-07-05)

Onboarding ekranına (hiç workspace yokken) "Workspace Oluştur"un yanına ikinci
buton eklendi: **Mevcut Workspace Seç** → native klasör seçici → seçilen klasör
geçerliyse (içinde `store/` var) taşınmadan kayıt defterine eklenip aktifleşir.

- **Backend:** `Manager.Attach(path)` + `isWorkspaceDir`/`workspaceNameFromDir`/`sameDir`
  yardımcıları (`internal/workspace/manager.go`); `POST /api/workspaces/attach`
  (`internal/api/workspaces.go` + route `server.go`). Geçersiz/zaten-ekli klasör 400 +
  Türkçe mesaj. Yeni `WS<n>` id, `Meta.Path` doğrudan seçilen klasör; `open()` mevcut
  içeriği yerinde kullanır. Ad klasör adından (`tionharness-` öneki soyulur); ikon/renk
  `ws-settings.json`'dan.
- **Frontend:** `api.attachWorkspace` (`api/workspaces.ts`),
  `useWorkspaces.attachWorkspace` (hata fırlatır → satır-içi göster),
  `OnboardingScreen` iki-buton düzeni + inline hata (`data-testid`:
  `select-existing-workspace` / `onboarding-error`), `App.tsx` `onAttach` prop.
- **Doğrulama:** `go build ./...` ve `tsc --noEmit` temiz. Detay: `06-WORKSPACES.md`
  "İlk Kurulum: Oluştur veya Mevcut Klasör Seç".

## UI: composer max-yükseklik + Budget dar-ekran uyumu ✅ (2026-07-05)

- **Composer textarea max-yükseklik:** `max-h-[5.5rem]` (~3 satır) → `max-h-[12rem]`
  (~7 satır); aşınca iç kaydırma. Doğrulandı: clientH=192px, scroll aktif.
- **Budget ekranı responsive:** özet kart satırları `flex` → `grid grid-cols-2
lg:grid-cols-4`; Tasarruf Merkezi `grid-cols-3` → `grid-cols-1 sm:grid-cols-3`
  (divide yönü de responsive); Köken+Trend `grid-cols-2` → `grid-cols-1
lg:grid-cols-2`; provider/ajan tabloları `overflow-x-auto` + `min-w` ile yatay
  kaydırılır; içerik dolgusu `p-5` → `p-3 md:p-5`. Doğrulandı: 430px'de yatay taşma
  0, özet 2 sütun; 1280px'de 4 sütun. Tema-uyumlu gradient de eklendi (composer).

## Sohbet: yüzen composer overlay + opak input + son mesaj görünürlüğü ✅ (2026-07-05)

Composer artık transkriptin **üstüne yüzen bir overlay** (App.tsx chat view'i
`relative` sarmalayıcı + `absolute bottom-0` bottom-stack). Böylece:

- **Gradient arka plan (tema-uyumlu):** composer sarmalayıcısı
  `bg-gradient-to-t from-[var(--color-bg)] via-[color-mix(...var(--color-bg)_85%...)]
to-transparent` — mesaj balonları alttan geçerken şeffaf üst kısımdan görünüp arka
  plana karışarak composer'ın **arkasına kayar** (açık/koyu temada tutarlı).
- **Opak input:** iç input kartı `bg-[var(--color-surface)]` + `shadow-lg` — okunur,
  gradient'in üstünde net durur.
- **Son mesaj görünürlüğü (bug):** overlay'in canlı yüksekliği `ResizeObserver` ile
  ölçülüp `MessageList`'e `bottomInset` (scroll padding) olarak verilir → en yeni
  kullanıcı/asistan mesajı **daima opak input'un üstünde** kalır, "agent bitene kadar
  son kullanıcı mesajı görünmüyor" belirtisi giderilir. Ask/todo/pending/wake
  banner'ları da bu yüzen yığına taşındı (ölçüme dahil, mesajları örtmez).
- **Doğrulama (standalone Playwright, sistem Chrome):** kısa+uzun cevap boyunca
  kullanıcı mesajı `everHidden=false`; seri tool (`Glob/Glob/PowerShell`), subagent
  (`run_subagent`), `schedule_wake` (kuruldu **ve tetiklendi**, takip turu üretti)
  turları geçti; hepsinde `userShown=true`. Detay: `07-CHAT-UX.md`.

## Sohbet: tur-ortası reload'da canlı cevap kaybolması düzeltildi ✅ (2026-07-05)

Başka sohbete geçip geri gelince (veya sayfa yenileyince) **kendi penceresinin**
stream ettiği asistan cevabının kaybolması giderildi. Kök neden: `App.tsx`
session-değişim effect'i `listMessages` ile in-memory `live-*` balonu siliyordu;
`recoverInflight` ise pencere turu sahiplenmeye devam ettiği için (`runsRef`) erken
dönüyordu ve yerel SSE handler'ları `prev.map(id===liveId)` ile güncellediğinden
balon silinince sonraki delta'lar + final `onReply` sessizce düşüyordu.

- **Fix A (self-heal / UPSERT):** `useChatStream.ts` canlı metin+iz artık closure
  var'larında biriktirilir; `syncLive` balonu yoksa tam içerikle yeniden yaratır
  (`onStep`/`delta`/`onReply` map-only yerine upsert). `onReply` kalıcı mesajı
  farklı id ile append eder (map yerine) → reload sonrası da hayatta kalır.
- **Fix B (reseed):** yeni `liveBubblesRef` (session→canlı balon) + `reseedLive`;
  App effect'i `listMessages` sonrası çağırır → dönüşte balon anında geri gelir.
- **Fix C (metin ilerletme):** sahiplenilmeyen/refresh sonrası ghost balonun cevap
  metni donuyordu (bus `delta`'yı düşürür). `useChatStream` artık aktif session'da
  sahiplenilmeyen tur pending iken `/inflight`'i saniyede bir çekip yalnız `text`'i
  ilerletir (iz bus'a ait kalır); tur bitince poll durur.
- **Temizlik:** bırakılmış `[DBG:delta]` debug console.log'u kaldırıldı.
- Mimari refactor **gerekmedi** — backend zaten detached tur + `inflight.json` +
  `session_step` bus ile tek doğru kaynak. Detay: `07-CHAT-UX.md` (tur-ortası
  reload/navigasyon kurtarma bölümü).

## Memory alt sistemi kaldırıldı ✅ (2026-07-05)

2026-07-05 — Memory alt sistemi (journal recall + core memory + hafıza grafiği +
ilgili tool/API/UI/veri) tamamen kaldırıldı. Çıkarılanlar: `internal/memory` paketi;
ajan araçları `Remember`/`Recall`/`core_memory_replace`/`core_memory_append`/`reflect`;
API uçları `/api/agents/{id}/memories`, `/core`, `/reflect`, `/recall`, `memory-graph`;
frontend Hafıza ekranı/MemoryPanel/CoreMemoryCard/hafıza grafiği; ayarlar
`journalMinLen`/`recallMinScore`/`journalCap`/`journalMaxLen`/`autoReflectThreshold`
(+ `memoryPressureWarn`). Workspace Ağı / İlişki Grafiği'nin workspace tarafı
(`/api/graph`) korundu. Doküman güncellemeleri: `31-MEMGPT-CORE-MEMORY.md` (kaldırıldı
notu), `23-ILISKI-GRAFIGI.md` (yalnız Hafıza Bilgi Grafiği bölümü çıkarıldı),
`17-TOKEN-OPTIMIZASYON.md` (recall/journal kapıları), `00-GENEL-BAKIS.md`.

## Claude Sonnet 5 desteği ✅ (2026-07-05)

Yeni **Claude Sonnet 5** (`claude-sonnet-5`) modeli katalog + fiyat tablosuna eklendi.

- **Katalog:** `kind_anthropic.go` (`claude-sonnet-5`) + `kind_openrouter.go`
  (`anthropic/claude-sonnet-5`) manifestlerine "dengeli" etiketiyle eklendi; Sonnet 4.6
  "önceki dengeli" olarak yeniden etiketlendi. Katalog context-window / max-output
  değerleri aile tablosundan otomatik türer (`sonnet` substring → 1M bağlam, 32K çıktı,
  0.45 adaptif oran) → `context_window.go`/`thinking.go` değişmedi.
- **Fiyat:** `pricing.go` anthropic + openrouter haritalarına **$3/$15** (standart liste;
  giriş fiyatı $2/$10, 31 Ağu 2026'ya kadar) eklendi. OpenRouter satırı Anthropic
  pass-through cache tier'ı (0.10× okuma / 1.25× yazma) taşır.
- **claude-cli:** ayrı işlem gerekmedi — `sonnet` alias'ı aboneliğin sunduğu güncel
  Sonnet'i zaten çözer.
- **Varsayılan model Sonnet 5'e taşındı:** `anthropic.go DefaultModel` ve
  `kind_openrouter.go openrouterDefaultModel` → `claude-sonnet-5`
  (`anthropic/claude-sonnet-5`). Ingest adapter `mapCCModel` `sonnet` ailesi →
  `claude-sonnet-5` (CC subagent import'u); örnek snippet'ler (Reviewer ajanı,
  settings patch örneği) da güncellendi.
- **Fast varyantı EKLENMEDİ (bilinçli):** Fast mode yalnızca Opus sınıfına özgü;
  Anthropic Sonnet 5 için Fast tarifesi yayınlamadı → uydurma model eklenmedi.
- `go build ./...` ✅ · providers/ingest/billing/tools/api/settings **349 test geçti**
  · yeni `TestPriceFor_Sonnet5` regresyon kilidi.

## Per-workspace claude-cli config evi (birleşik config) ✅ (2026-07-05)

claude-cli'nin config evi artık **per-workspace**: `<workspace>/claude-home` =
`CLAUDE_CONFIG_DIR`. Böylece TionHarness ve driver ettiği CLI **aynı skill/settings/
login** setini kullanır. Detay `51-CLAUDE-CONFIG-BIRLESIK.md`.

- **Faz 1:** `providers.ClaudeCLI.SetConfigDir` (turluk override) + `Runtime.claudeHomeDir()`;
  `toolloop.go` choke point'te (`provider.(*providers.ClaudeCLI)` sonrası) her CLI turu
  için set edilir. `agent.EnsureWorkspaceClaudeHome` workspace açılışında global
  `~/.tionharness/claude-home`'u per-workspace eve tohumlar (çalışma-anı dizinleri atlanır),
  idempotent.
- **Faz 2:** ~~`workspaceSkillsDir` → `<workspace>/claude-home/skills`~~ **GERİ ALINDI (2026-07-05)** —
  skill'ler `<workspace>/skills`'te kalıyor; taşınmış skill'ler (WS1:4, WS8:14) geri alındı,
  `claude-home/skills` silindi. claude-home skill içermez, yalnız login/settings.
- **Faz 3:** ~~native `Skill` disallow kaldırıldı~~ **GERİ ALINDI (2026-07-05)** — native `Skill`
  KAPALI kalıyor; tek skill yolu `use_skill` köprüsü (hem workspace hem global tier'ı sunar).
- Sınırda bırakılan (bilinçli): MCP/hooks/izinler/agent'lar DB'de kalır, `--mcp-config`/
  `--settings` ile enjekte edilmeye devam eder.
- `go build ./...` ✅ · etkilenen 4 paket testi 241 passed.

**Ek (UI + güvenlik):**

- Settings → Sağlayıcılar → "claude config dizini" alanı **salt-okunur** yapıldı
  (`ProvidersPanel.tsx endpoint2ReadOnly`; per-workspace türetiliyor, global yalnız
  fallback). `tsc --noEmit` temiz.
- **Yedek sızıntısı kapatıldı:** migration credential/login dosyalarını (`.claude.json`,
  `.credentials.json`) her workspace'e kopyaladığından, workspace dizinini tümüyle
  zip'leyen periyodik yedek bunları arşive sızdırabilirdi. `backup/archive.go`'ya
  `backupExcludeNames` eklendi → bu dosyalar hiçbir yedek zip'ine yazılmaz (credential
  diskte kalır, arşive girmez). Test `TestZipExcludesClaudeCredentials`. Mevcut zip'ler
  (2026-06-25, migration öncesi) tarandı — sızıntı yok.

## claude-cli kalıcı süreç varsayılan AÇIK + sistem-prompt teslim toggle'ı ✅ (2026-07-05)

İki değişiklik:

1. **`claudePersistentSession` deneysellikten çıkarıldı, varsayılan AÇIK.**
   `settings.Default()` → `ClaudePersistentSession: true`. `--resume` ile karşılıklı
   dışlar; ikisi de açıkken persistent süreç kazanır (tasarım gereği). Struct/DTO/UI
   yorumlarından "experimental/deneysel" ibaresi kaldırıldı; UI toggle etiketi
   "(deneysel)" → sade. Tunables/skill/17. dokümanı güncellendi.

2. **Yeni ayar `claudeSysPromptFile` (varsayılan KAPALI = doğrudan).** claude-cli'ye
   eklenen sistem promptunun nasıl verileceğini seçer:
   - **Kapalı (varsayılan):** `--append-system-prompt <metin>` — komut satırında doğrudan, geçici dosya yok.
   - **Açık:** temp dosya + `--append-system-prompt-file <yol>` — Windows ~32 KB komut satırı limitini (errno 206) aşan çok büyük promptlar için.

   Plumbing: `settings` (struct+DTO+Patch+default) → `store.applyBool` → `api/server.go`
   `SetClaudeSysPromptFile` → `agent.Tunables.cliSysPromptFile` (Set/Get) →
   `recordedComplete` `req.SysPromptFile` → `providers.Request.SysPromptFile` →
   `claudecli.go` (tek-atış) + `claudecli_session.go` (kalıcı) dallanır. Persistent
   fingerprint'e eklenmedi (teslim yöntemi değişse de CLI'ye giden metin aynı — sıcak
   süreç yeniden başlatma gerektirmez). Frontend `types/settings.ts` + `SettingsPanel`
   patch + `AppToolsPanel` toggle.

   ⚠️ **Not:** doğrudan mod, prompt ~32 KB'ı aşarsa süreci başlatmadan çöktürebilir —
   o durumda dosya toggle'ını açın. (Bilinçli tercih: sessiz guard eklenmedi; hata
   gerekiyorsa görünür versin.)

   `go build`/`go vet`/285 test yeşil; `tsc --noEmit` temiz.

## Fix: "claude-cli kalıcı süreç" toggle'ı kaydedilmiyordu 🐛 (2026-07-05)

Ayarlar ekranındaki **claude-cli kalıcı süreç (deneysel)** toggle'ı açık kalmıyordu.
Neden: `SettingsPanel.tsx` içindeki kaydetme (patch) yükü `claudeResume`'u gönderiyor
ama `claudePersistentSession` alanını atlıyordu → toggle draft state'i değişiyor,
kaydet'e basınca backend'e hiç ulaşmıyor, yeniden yüklemede eski (kapalı) değere dönüyordu.
Düzeltme: patch objesine `claudePersistentSession: draft.claudePersistentSession` eklendi
(backend `store.go`/`settings.go` alanı zaten kabul ediyordu). `tsc --noEmit` temiz.

## Claude Code cache paritesi: P2–P5 tamamlandı ✅ (2026-07-05)

`_Docs\50-CLAUDE-CODE-CACHE-PARITE.md` planının kalan tüm iş paketleri uygulandı
(P1+P6 önceki oturumda bitmişti):

- **P2 — Özet=compact-boundary mesajı:** Rolling özet volatile Dinamik'ten çıkıp
  `providers.Request.Summary` ile taşınıyor; native yollar (anthropic + openrouter) onu
  cache'li önekin başına **sentetik head mesajı** koyuyor (`prependSummaryMessage`) → iki
  katlama arası **cache-read**. Cache kapalıyken `system`'e katlanır. **claude-cli native-only
  kararı gereği değişmedi** (özet hâlâ `[Context]` tail'inde, her tur taze). Önizleme
  `SummaryCached` ile özet bölümünü yeşil gösterir.
- **P4 — Cache-break telemetrisi:** `internal/agent/cachebreak.go` — oturum-başına önek
  hash'i (statik System + araç şeması) + model + warmth; ana konuşma turlarında sıcak önek
  kaybını (warmed && cacheRead==0 && cacheWrite≥2000) `cache_break` debug olayı olarak sebep
  atıflı kaydeder (model / prompt-tools / TTL-server). Debug kartında "Cache kırılması" pill +
  anomali. Claude Code `promptCacheBreakDetection` muadili.
- **P3 — API-native context editing:** anthropic `context_management` beta (`clear_tool_uses_
20250919`), ayar `anthropicContextEditing` (vars. kapalı), ContextPanel toggle'ı. Microcompact
  muadili; client-side fold'a ek.
- **P5 — TTL/breakpoint kararlılığı:** tüm anthropic breakpoint'leri tek `cacheTTL="1h"`
  sabitinden türer (karışık-TTL drift'i imkânsız); `TestCacheBreakpointStability` tek stabil
  rolling marker'ı (son persist blokta) kilitler.

**Doğrulama:** `go test ./...` **680 yeşil**, `go vet` temiz, `tsc` temiz, `dist` yeniden
build edildi (tek-binary'e gömülü). Dokümanlar: `_Docs\50` (P2–P5 ✅), `_Docs\38` (cache_break
olayı), default skill `tionharness-settings`.

**Canlı doğrulandı ✅ (2026-07-05, OpenRouter `anthropic/claude-haiku-4.5`, gerçek API):**

- **P1** — statik System (~8.2k tok) + geçmiş, **dinamik her tur değişmesine rağmen** turn-2'de
  `cache_read=8197` HIT aldı (P1'den önce dinamik system'de olduğu için bu 0 olurdu).
- **P2** — stabil özet head'i turn-2'de `cache_read=8216` HIT → özet cache'li önekin parçası.
- **P4** — sistem öneki başından değişince `cache_read` 8216→0 çöktü → detektör kırılmayı yakalar.
  Canlı test bir eksik ortaya çıkardı ve düzeltildi: OpenRouter write sayacını raporlamadığından
  tetik `cacheWrite+input` üzerinden ölçülüyor (native Anthropic + OpenRouter ikisini de kapsar).

> Not: Native anthropic anahtarı store'da sahte (`asdfasdf…`) olduğu için doğrulama gerçek
> OpenRouter anahtarıyla yapıldı (aynı native cache kod yolu: OpenAICompat cacheSystem). Kullanıcının
> backend'i yeni kodu almak için `.\scripts\dev.ps1` ile yeniden başlatılmalı (şu an çalışan örnek yok).

## Ağ ekranı mobilde kasması giderildi (vis-network lite modu) ✅ (2026-07-05)

Neden: `vis-network` canvas'ında **node gölgeleri**, **eğri (continuous) kenarlar**,
**`improvedLayout`** ön-yerleşim geçişi ve yüksek stabilizasyon iterasyonu — mobil
GPU'da pan/zoom sırasında her karede yeniden çizim çok pahalı → kasma.

Çözüm: `VisNetworkGraph`'a `lite` prop'u eklendi; `NetworkPanel` bunu `useIsMobile()`
ile besliyor. Lite modda (`< md`): gölge kapalı, kenarlar düz (`smooth: false`),
`improvedLayout: false`, stabilizasyon 300→120, hover kapalı (touch'ta zaten yok).
Grafın kendisi (düğüm/kenar/fizik yerleşimi) aynı, sadece çizim ucuzladı. Masaüstünde
tam kalite korunur. Build temiz; mobil (390px) canvas temiz render ediyor.

> ⚠️ `VisNetworkGraph.tsx` bu oturumda eşzamanlı bir süreç tarafından bir kez geri
> alındı; değişiklikler yeniden uygulandı.

## Ağ ekranı: istatistik + yenile başlık çubuğuna taşındı ✅ (2026-07-05)

`NetworkPanel` App'in generic başlığını kullanıyordu ("Ağ"). Artık kendi başlık
çubuğu var: "Ağ" solda, **"9 ajan · 13 görev · 2 akış · 0 beceri · 2 MCP"** istatistiği
sağa dayalı (`ml-auto`), **Yenile** butonu en sağda. İstatistik + yenile eski
toolbar satırından kaldırıldı; `network` `HEADERLESS_VIEWS`'e eklendi (çift başlık
olmasın). Playwright: title "Ağ", stats sağda, refresh en sağda, App çift başlığı yok.
Build temiz.

## AgentPicker: seçili ajan artık kaldırılabilir ✅ (2026-07-05)

- `AgentPicker`'a `clearable` prop'u eklendi. Aktifken seçili bir ajanı temizlemek
  için iki yol var: **trigger'daki ✕** düğmesi (sağda, `▾` yerine) ve **açılır listenin
  başındaki "Seçimi kaldır"** seçeneği. İkisi de `onChange('')` çağırıp değeri boşaltır.
- Etkinleştirildiği yerler: **Board görev formu** (`TaskFormModal` — atama zaten
  opsiyonel) ve **Akış ajan node'u** (`flow/NodeInspector`). Kullanıcı her ikisinde de
  seçili ajanı silemiyordu; artık silinebiliyor.
- Dosyalar: `agents/AgentPicker.tsx`, `panels/TaskFormModal.tsx`, `flow/NodeInspector.tsx`.
  `tsc -b` temiz.

## Agents: aktivite paneli chat Detay gibi yan-drawer oldu ✅ (2026-07-05)

- `AgentsView` aktivite paneli artık `SessionDetailPanel` (chat "Detay") ile aynı
  desende açılıyor: **masaüstünde** sağ kolon (tam yükseklik), **mobilde** sağdan
  kayan drawer (`max-md:fixed inset-y-0 right-0 z-40 w-[85vw] max-w-sm shadow-xl`) +
  `md:hidden` karartma backdrop (tıklayınca kapatır).
- İçerik satırından `max-md:flex-col` kaldırıldı (panel artık mobilde altta yığılmıyor,
  drawer). Sarmalayıcı `flex` yapıldı ki `<aside>` (h-full'süz) tam yüksekliği doldursun.
- Doğrulama: masaüstünde panel sağ kenarda (x:1600/w:320/h:860), toggle `aria-pressed`
  çalışıyor. Mobil drawer CSS'i SessionDetailPanel ile birebir.
- Dosya: `agents/AgentsView.tsx`. `tsc -b` temiz.

## Modal (bottom-sheet) mobilde navbar arkasında kalması fix ✅ (2026-07-05)

`ModalOverlay` `z-50` → `z-[60]`. MobileNavBar de `z-50` ve DOM'da modaldan sonra geldiğinden,
mobilde bottom-sheet modalın (ör. Flows node-editör popup'ı) altı navbar'ın arkasında kalıyordu.
Modal artık navbar'ın **üstünde**; 13 `ModalOverlay` tüketicisinin hepsi tek yerden düzeldi.

## Skills/Artifacts başlığı: yan-bilgiler dar ekranda 2. satıra + min-h-0 fix ✅ (2026-07-05)

- **`PaneHeader`'a `secondary` prop'u:** üç-slot flex-wrap düzeni. Geniş ekranda tek
  satır (title · secondary · right); dar ekranda `secondary` grubu tam genişlikle
  (`order-last basis-full`) 2. satıra sarar, title + sağ eylemler 1. satırda kalır.
  Geniş: `md:order-2 md:flex-1 md:basis-auto`.
- **Skills:** chip'ler (grup/Kısıtlı/görünürlük/kaynak) + **Tam/Özet/İsim/Gizli**
  seçici `secondary`'ye taşındı (dar ekranda 2. satır). Title + Düzenle/Kısıtla/
  kopya/Aç/Sil 1. satırda.
- **Artifacts:** rozetler (origin/kind/creator) + **İçerik** (içerik-kopyala) butonu
  `secondary`'ye taşındı. Title + Düzenle/yol-kopyala/Aç/kaynak/Sil 1. satırda.
- **Navbar/yükseklik fix:** Skills + Artifacts içerik kolonuna (`flex flex-1 flex-col`)
  **`min-h-0`** eklendi — eksik olması `min-height:auto` yüzünden uzun içeriğin kolonu
  parent yüksekliğinin ötesine taşırıp body-scroll + navbar kaymasına yol açıyordu.
- Dosyalar: `common/PaneHeader.tsx`, `panels/SkillsPanel.tsx`, `panels/ArtifactsPanel.tsx`.
  `tsc -b` temiz; canlı DOM doğrulaması (secondary `basis-full order-last`, geniş inline).
- **Güncelleme:** `PaneHeader`'a `secondaryAlwaysWrap` prop'u eklendi — `secondary`
  grubu **her genişlikte** kendi 2. satırında kalır (`md:` inline override'ları
  kaldırılır). **Skills** bu modu kullanıyor (chip'ler + Tam/Özet/İsim/Gizli hep 2.
  satırda). Doğrulama: geniş ekranda header 2 satır, secondary title'ın altında.
- **Güncelleme 2:** **Artifacts** de artık `secondaryAlwaysWrap` (chip'ler + İçerik
  hep 2. satırda). Ayrıca 2. satırda hareketli kontrol **sağa dayandı** (`ml-auto`):
  Skills'te Tam/Özet/İsim/Gizli seçici, Artifacts'te İçerik butonu satırın sağ
  kenarında (chip'ler solda). Doğrulama: her ikisinde `rightGap: 0`.

## Çoklu seçim: Ctrl+Click ile aktif öğe de sete dahil ediliyor ✅ (2026-07-05)

Sorun: bir öğe açık/aktifken Ctrl+Click ile ikinci bir öğeye tıklanınca yalnız yeni
tıklanan seçime giriyor, ilk (aktif) öğe dahil edilmiyordu — çünkü multi-select seti
panelin "aktif detay" state'inden ayrı.

**Çözüm (`hooks/useMultiSelect.ts` — tek noktada, tüm ekranları kapsar):**
`handleClick` artık yeni bir çoklu seçim başlarken (set boş) aktif öğeyi **tohumluyor**:
Ctrl/Cmd+Click ikinci satırda → ikisi de seçilir. Aktif öğe kaynağı: yeni opsiyonel
`activeId` parametresi (varsa) → yoksa anchor (son düz-tıklanan satır). Böylece
tıklanmamış default-seçili satır bile dahil edilir.

**Çağrı yerleri:** paneller aktif id'lerini geçiriyor — Artifacts (`activeId`),
Executions (`selectedId`), Flows (`selectedId`), Skills (`activeSlug`), Tools
(`selectedName`), Sessions (`activeSessionId`), Agents (`selectedId`). Memory /
AgentTools / TaskBoard anchor fallback kullanır.

Playwright: Tools (tık→Ctrl+tık = 2 seçili), Artifacts (default-seçili + Ctrl+tık başka
satır = 2 seçili). Build temiz.

## Ekran seçimleri oturum boyunca korunuyor (reload'da sıfırlanır) ✅ (2026-07-05)

İstek: farklı ekranlar arasında gezerken seçili öğe (sohbet, aktivite, ajan, hafıza
ajanı, akış, artifact, skill, tool) korunsun; uygulama kapatılıp açılınca sıfırlansın.

**Mevcut zaten çalışanlar (App-seviyesi state):** Sohbet (`activeSessionId`),
Aktivite (`executionTarget` + `onSelectExecution`), Ajanlar/Hafıza (`activeAgentId`).
App unmount olmadığı için nav geçişinde korunuyorlardı.

**Kırık olanlar (panel-local `useState`, remount'ta sıfırlanıyordu):** Artifactlar,
Skills, Araçlar, Akışlar — paneller kendi seçimini tutup App'e geri yazmıyordu.

**Çözüm:** Yeni `useSessionState(key, initial)` hook'u (→ `hooks/useSessionState.ts`).
`useState` benzeri ama değeri **modül-içi bir Map**'te tutar → component unmount/remount
arası korunur, ama localStorage'a yazılmaz → tam sayfa reload / uygulama yeniden açılış
sıfırlar. Panellerde seçim state'i buna geçirildi:

- `ArtifactsPanel` → `artifacts.activeId`
- `SkillsPanel` → `skills.activeSlug`
- `ToolsPanel` → `tools.selectedName`
- `FlowsPanel` → `flows.selectedId` + `tab` + `templateId` + `selectedRunId`, ayrıca
  mount'ta seçili akışın editör state'ini yeniden yükleyen tek-seferlik restore effect.

Playwright: 8 ekranın hepsinde seç → başka ekrana git → geri dön → seçim korunuyor.
Build temiz.

## Hafıza (memory): bilgi inputu + Belge(ikon) + Yansıt üst-title'a ✅ (2026-07-04)

- `memory` `HEADERLESS_VIEWS`'e eklendi; `MemoryPanel` kendi `PaneHeader`'ını render
  ediyor: `titleSlot` = bilgi-girme inputu (büyüyen), `right` = **Belge** (yalnız
  `Paperclip` ikonu, metin kaldırıldı) + **Yansıt**. Eski "bilgi ekle + reflect" bar'ı
  ve `+ Belge` (`Button`) kaldırıldı. Kullanılmayan `Button` importu temizlendi.
- Memory chat-benzeri layout (sol AgentRoster sibling + App header) kullandığından,
  App header'ın mobil roster hamburger'ı kayboldu → `PaneHeader`'a `onToggleList`
  prop'u (`setMobileListOpen(true)`) App'ten geçirildi; null-ajan durumunda da
  PaneHeader ("Hafıza" + hamburger) render ediliyor.
- Dosyalar: `App.tsx`, `panels/MemoryPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (input header'da, Belge ikon-only, Yansıt header'da).
- **Ek:** `titleSlot`'ta inputun soluna seçili ajanın `AgentAvatar`'ı (22px) eklendi.

## Dar ekranda sohbet mesaj listesi yatay padding azaltıldı (1px) ✅ (2026-07-04)

Sohbet mesaj kaydırma konteyneri (`MessageList` `role="log"`) **ve** üstte sabitlenen
(pinned) soru overlay'i yatay padding'i dar telefonlarda **1px**'e düşürüldü:
`px-6` → **`px-[1px] md:px-6`** (ikisinde de). Dar (`<md`) = 1px, geniş (`md+`) = 24px
(değişmedi). Playwright: 390px→1px, 1280px→24px. Build temiz.

## Flow: daraltılabilir "Node ekle" paleti + yukarı büyüyen run input ✅ (2026-07-04)

- "Node ekle" başlığı chevron'lu toggle (`paletteOpen`, localStorage kalıcı); daraltınca node
  tip butonları gizlenir, "Görünüm" kalır.
- Çalıştır girdisi auto-grow (`runInputRef` effect, `min(scrollHeight,160)`, `resize-none`); run
  paneli bottom-anchored olduğundan **yukarı doğru** genişler (alt kenar sabit). Detay `15-FLOW-CANVAS.md`.

## Dar ekranda composer ↔ alt navbar boşluğu near-flush yapıldı ✅ (2026-07-04)

Sohbet ekranında composer ile alttaki `MobileNavBar` arasındaki boşluk fazlaydı;
kullanıcı composer'ın navbar'a bitişik olmasını istedi. `main` alt padding'i
sabit `pb-16/pb-20` yerine **`max-md:pb-[calc(3.25rem+env(safe-area-inset-bottom))]`**
yapıldı → navbar yüksekliğiyle (safe-area dahil) eşleşiyor, boşluk **~1px** (bitişik),
taşma yok. Safe-area büyüyen cihazlarda da padding aynı `env()` değerini içerdiği için
bitişiklik korunur, composer navbar arkasına kaçmaz. Playwright (390px): composer alt
728, navbar üst 729, gap 1, overlap yok. Build temiz.

## Bağlam modal header + cache info + otomasyon formu sadeleştirme ✅ (2026-07-04)

1. **Cache notu → InfoPopover:** SessionContextModal cache legend'ında "cache'li…"
   chip'inin yanındaki uzun `data.cache.note` metni, chip içine (i) `InfoPopover`
   olarak alındı.
2. **Header aksiyonları title yanına döndü:** iki modalda header tekrar tek satır
   (`nowrap`); kopyala + X butonları başlığın yanında (BulkButtons zaten örnek-mesaj
   satırına taşınmıştı). Alt yazı `truncate`.
3. **Otomasyon formu:** "Ad (ops.)" isim inputu **create + edit**'ten kaldırıldı;
   kart görünümündeki isim (`{a.name || '(adsız)'}`) kaldırıldı (artık `#etiket`
   birincil). Ajan-Akış geçişi (`TargetModeToggle`) forma **en sola** alındı
   (zamanlamada zaten soldaydı).
4. **FlowPicker görünür ikon:** native `<select>` artık solunda seçili akışın
   ikonunu (mojibake-safe emoji, yoksa `Workflow` glyph) gösteriyor — AgentPicker
   avatar'ına paralel; hem schedule hem otomasyon formunda.

Playwright: isim inputu 0, toggle en solda, iki FlowPicker'da da ikon, header nowrap

- copy/X title yanında, cache chip'inde (i). Build temiz.

## Bağlam modalı: token özeti + CLI ek yükü açılır-kapanır (default kapalı) ✅ (2026-07-04)

`SessionContextModal` ve `AgentContextModal`'da token istatistik satırı (Toplam,
Sistem, Skills, … ~token tahmini) ve "CLI ek yükü" bloğu tek bir açılır-kapanır
şeride alındı. **Default kapalı**; başlık çubuğu "Token özeti · Toplam N (~tahmini)"

- varsa "CLI ek yükü" rozetini gösterir, `ChevronRight` 90° dönerek durumu belli
  eder. Playwright: default `aria-expanded=false`, kapalıyken detay (`Mesajlar (…)`)
  gizli, tıklayınca açılıyor. Build temiz.

## Flow değişken popover'ı kırpılma fix + run input info butonu ✅ (2026-07-04)

- **Kırpılma fix** — `FlowVarsButton` paylaşılan bileşene taşındı (`flow/FlowVarsButton.tsx`),
  absolute popover yerine satır-içi (`w-full basis-full`, ebeveyn `flex flex-wrap`) panel; modal
  gövdesinin `overflow-y-auto`'su artık kesmiyor.
- **Run input info butonu** — flow başlatma girdisine ℹ️ (`context="seed"`): yalnız
  `{{date}}/{{time}}/{{datetime}}` (motorun sıralı `render` ikamesiyle çözülür) + "{{input}} olur" notu.
- Yalnız frontend; `tsc`+build temiz. Detay `15-FLOW-CANVAS.md`.

## Bağlam modal aksiyonları + claude-cli cache notu + schedule kartı ✅ (2026-07-04)

1. **"Tümünü aç/kapat" ikon-only + Compact Simüle yanına taşındı:** `SessionContextModal`
   ve `AgentContextModal`'da header'daki metinli `BulkButtons` kaldırıldı; örnek-mesaj
   satırına (Compaction simüle / Simüle et düğmesinin yanına), yalnız ikon (30×30)
   olarak taşındı. Satır `flex-wrap` yapıldı. Playwright: ikon-only, input satırında,
   header'da yok.
2. **claude-cli soğuk-tur cache notu kaldırıldı:** backend `session_context.go` içindeki
   uzun "claude-cli ilk/soğuk tur…" metni `Note: ""` yapıldı; frontend cache-legend
   boş note'u artık render etmiyor (`{data.cache.note && …}`). ⚠️ görünür etki için
   backend restart gerekir.
3. **Schedule/Automation kartı dikey:** açma-kapama toggle'ı **üstte**, agent/flow
   ikonu **altta** olacak şekilde dikey sütuna alındı (`Schedules.tsx` + `Automations.tsx`).
   Playwright: stacked column, toggle üstte, icon altta.

Frontend + Go build temiz.

## Dar ekran düzeltmeleri: bağlam modal başlığı + ayarlar min-w + artifact buton metni ✅ (2026-07-04)

Üç dar-ekran sorunu giderildi:

1. **Bağlam pencereleri başlığı sığmıyordu** (`SessionContextModal`, `AgentContextModal`):
   sağdaki geniş aksiyon butonları (Tümünü aç/kapat/kopyala/X) başlık kolonunu ~0'a
   sıkıştırıyor, alt yazı dikey kelime-kelime sarıyordu. Header artık `flex-wrap`;
   dar ekranda başlık tam ilk satırı alıyor (`basis-full sm:basis-0`), aksiyonlar
   tek grup halinde alt satıra sarıyor. Playwright (360px): başlık kolonu 319px,
   alt yazı 2 satır (dikey sıkışma yok), modal 360'a sığıyor.

2. **Ayarlar Hooks & Harici Araçlar bir noktadan sonra daralmıyordu:** içerik kolonu
   (`SettingsPanel` sağ pane) `min-w-0` içermiyordu → içindeki geniş bir öğe (kod
   örneği vb.) min-content genişliğinin altına inmeyi engelliyor, yatay taşma
   oluşuyordu. `flex flex-1 flex-col` → `flex min-w-0 flex-1 flex-col`. Bu tek
   düzeltme tüm ayar kategorilerini kapsıyor. Playwright (360px): `docScrollW 360`,
   taşma yok (hem hooks hem exttools).

3. **Artifacts "içeriği kopyala" butonu metni** "İçerik" olarak kısaltıldı.

Build temiz.

## Flow tarih/saat değişkenleri + tag filtreleme ✅ (2026-07-04)

- **Yeni flow değişkenleri** — `orchestration.render` (engine.go) artık `{{date}}`/`{{time}}`/
  `{{datetime}}`'i de çözer (otomasyon formatıyla aynı). Node inspector ℹ️ popover'ına eklendi.
  Test `TestRenderVars`. Oturum-bağlamlı otomasyon değişkenleri (`{{result}}` vb.) flow'da yok.
- **Flow tag filtreleme** — `FlowsPanel` Akışlarım sekmesinde etiket chip'leri; ANY-eşleşme
  süzme, ad aramasıyla AND. Boş → "Eşleşen akış yok." Detay `15-FLOW-CANVAS.md`.

## Alt navbar'a workspace seçme butonu eklendi ✅ (2026-07-04)

Dar ekranlarda (`< md`) masaüstü NavRail'in tepesindeki workspace seçici yoktu.
Alt `MobileNavBar`'ın **en başına sabit** bir workspace-seçme butonu eklendi (mobil
karşılığı `MobileWorkspaceButton` → `components/MobileWorkspaceButton.tsx`).

- **Yukarı açılan menü:** bar ekranın altında olduğu için popup `bottom-full` ile
  **yukarı** açılıyor; workspace'ler arasında geçiş + "Yeni workspace" (mevcut
  `WorkspaceCreateModal` yeniden kullanıldı).
- **Kırpılma çözümü:** `<nav>` artık kaydırmıyor (overflow visible), yalnız içteki
  görünüm şeridi yatay kayıyor. Böylece yukarı açılan menü şeridin `overflow` 'una
  takılmıyor. `useDragScroll` ref'i içteki şeride taşındı (fare-sürükleme korundu).
- **Playwright doğrulaması (360px):** workspace butonu en solda (left 0); menü
  yukarı açılıyor (bottom 706 ≤ nav-top 709), kırpılmıyor (top ≥ 0), 5 workspace +
  create listeleniyor; şerit hâlâ fare-sürükleme ile kayıyor (200px, navigasyon yok).
  Build temiz.

## Node türü sabit + değişken info butonu + flow tag chip'leri ✅ (2026-07-04)

Flow editörü UX (yalnız frontend). Ayrıntı: `_Docs/15-FLOW-CANVAS.md`.

- **Node türü sabit** — `NodeInspector`'dan "Tür" select kaldırıldı; yerine salt-okunur tür
  başlığı (monokrom ikon + ad + "(tür sabit)"). Tür artık yalnız palet'ten oluşturulurken belirlenir.
- **Değişken info butonu** — Prompt/Şablon alanları yanına ℹ️ popover (`FlowVarsButton`).
  Flow motorunun gerçekten desteklediği değişkenler: `{{input}}`, `{{last}}`, `{{node.<id>}}`
  (engine.go `render`). Popover, akıştaki her diğer node için dinamik `{{node.<id>}}` satırı üretir;
  tıklayınca alana ekler. (Otomasyonların `{{date}}` vb. flow motorunda yok.)
- **Flow tag chip'leri** — etiketler zaten Görünüm > Etiket'ten atanabiliyordu; artık sol flow
  listesinde chip olarak da görünür (canlı senkron).

## Akış emojisi + monokrom node ikonları ✅ (2026-07-04)

Akışlar (Flow) için iki görsel iyileştirme. Ayrıntı: `_Docs/15-FLOW-CANVAS.md`.

- **`Flow.emoji`** — her akışa opsiyonel emoji. Bağımsız kalıcı: yeni store metodu
  `SetFlowEmoji` + `PUT /api/flows/{id}/emoji`; `UpdateFlow` emojiye dokunmaz (ad/graph
  kaydı emojiyi silmez). `create_flow`/`update_flow`/`get_flow`/`list_flows` araçlarına
  `emoji` eklendi. UI: `FlowsPanel` başlığında ortak `EmojiField` (seçince anında kalıcı).
  Emoji her akış-seçim/gösterim yerinde: flow listesi, Koşular listesi + başlık, `RunView`,
  `FlowPicker` (Zamanlama/Otomasyon), zamanlama/otomasyon satır rozetleri, TaskBoard +
  `TaskFormModal`. Hepsi `normalizeAvatar` ile mojibake-güvenli.
- **Monokrom node ikonları** — çok renkli emoji (🤖🔀⚡⏱️🧩) yerine temaya uygun tek renkli
  lucide ikonlar (`Bot/Split/Zap/Timer/Puzzle`, `nodeStyles.NODE_ICONS`). `NodeChrome.icon`
  → `Icon: LucideIcon`; `NodeShell` başlıkta + "Node ekle" paletinde (node ağacı) aynı
  ikon seti render eder.
- **Test:** `TestFlowEmojiRoundTrip` (create/UpdateFlow-koruma/SetFlowEmoji-değiştir-temizle).
  Go build+vet temiz, `tsc`+`npm run build` temiz.

## Görevler (board): toolbar üst-title'a taşındı ✅ (2026-07-04)

- `board` `HEADERLESS_VIEWS`'e eklendi; `TaskBoard` board kolonunun tepesine
  `PaneHeader` (title="Görevler", `right` = ⊞ Sütunlar + "+ Görev" + 🔗 Sırala)
  yerleştirildi. Eski `border-b` toolbar bar'ı kaldırıldı. PaneHeader, sol sütun
  editörü panelinin sağındaki içerik kolonunun tepesinde (diğer ekranlardaki
  ListPane+PaneHeader deseniyle tutarlı). Sütunlar butonu zaten editörü toggle
  ettiğinden ayrı liste-toggle'a gerek yok.
- Dosyalar: `App.tsx`, `panels/TaskBoard.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Görevler · ⊞ Sütunlar · + Görev · 🔗 Sırala").

## Alt navbar fare-sürükleme ile kaydırılabilir oldu ✅ (2026-07-04)

Dar ekranlarda (`< md`) alttaki yatay `MobileNavBar` dokunmatikle native kayıyordu
ama fareyle sürüklenemiyordu. Yeni `useDragScroll` hook'u (→ `hooks/useDragScroll.ts`)
sadece **fare** için tıkla-sürükle panning ekliyor (touch'a dokunulmuyor — zaten
momentumlu native kaydırma var). 4px'lik ölü bölge + capture-fazında click bastırma
ile bir buton üzerinde sürükleme yanlışlıkla o görünüme geçmiyor. `cursor-grab` /
`active:cursor-grabbing` + `select-none` görsel/etkileşim ipuçları eklendi.

- **Playwright doğrulaması (360px):** 200px sola sürükleme → `scrollLeft` 0→200
  (birebir), görünüm değişmedi; düz tıklama → görünüm değişiyor. Build temiz.

## OpenRouter duplicate-key React uyarısı giderildi (katalog id çakışması) ✅ (2026-07-04)

**Belirti:** Sağlayıcı/model seçicilerinde React "encountered two children with the
same key, `openrouter`" uyarısı.

**Kök neden (backend, frontend değil):** `GET /api/catalog` yerleşik katalogu
(`providers.Catalog()`) custom sağlayıcılarla (`CustomCatalog()`) **id kontrolü
olmadan** birleştiriyordu. Kullanıcı, yerleşik `openrouter` ile aynı id'de bir
custom sağlayıcı eklemişti → katalog aynı id'yi iki kez döndürüyordu (canlı API'de
doğrulandı: 26 modelli yerleşik + 3 modelli custom). Bu sadece bir uyarı değil,
gerçek belirsizlik: `provider: "openrouter"` hangisini kastediyor?

**Düzeltme:** Yeni `providers.MergeCatalog(builtin, custom)` (→ `internal/providers/merge.go`).
Custom sağlayıcı aynı id'li yerleşiği **yerinde override eder** ("kullanıcı config'i
kazanır"), eşi olmayan custom id'ler sona eklenir; girdi dilimleri değişmez.
`handleCatalog` artık bunu kullanıyor. Birim test: `merge_test.go` (çakışma → tek
kayıt, sıra korunur, custom kazanır, girdi mutasyonu yok) — geçti. `go build ./internal/...` temiz.

> ⚠️ Etki için backend restart gerekir — çalışan `go run ./cmd/tionharness` (kullanıcı
> oturumu) yeniden başlayınca devreye girer.

## Bütçe: iç header üst-title'a (PaneHeader) taşındı ✅ (2026-07-04)

- `budget` `HEADERLESS_VIEWS`'e eklendi; `BudgetPanel` kendi `PaneHeader`'ını render
  ediyor (title="Bütçe", subtitle = gün, `right` = 7g/30g/90g aralık seçici + Yenile).
  Eski iç header (`Wallet` + "Bütçe" h1 + gün + kontroller) kaldırıldı → App header ile
  çift "Bütçe" başlığı sorunu giderildi. Kullanılmayan `Wallet` importu temizlendi.
  İçerik (özet kartlar + tablolar) `flex-1 overflow-y-auto p-5` gövdeye alındı.
- Dosyalar: `App.tsx`, `panels/BudgetPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Bütçe · 2026-07-04 · 7g/30g/90g · Yenile").

## Schedule + Automation: boş oturum yerine bir Flow başlatma (`flowId`) ✅ (2026-07-04)

Kullanıcı isteği: zamanlamalar ve otomasyonlar tek ajana prompt teslim etmek yerine
seçilen bir **orkestrasyon akışını** (Flow) da başlatabilsin. Task'taki mevcut
`FlowID` deseni Schedule + Automation'a taşındı.

- **Model:** `db.Schedule.FlowID` + `db.Automation.FlowID` (`flowId,omitempty`); set
  ise prompt = akış girdisi, agent/targetAgent opsiyonel (karşılıklı dışlar). Store
  `UpdateSchedule`/`UpdateAutomation` round-trip eder.
- **Dispatch:** `scheduler.go` `run` → `deliverFlow` (`RunFlowRecorded`, akış kendi
  bildirim + transcript oturumunu yönetir, `FlowFailure`→delivery failure).
  `automation.go` `fire` → render sonrası `fireFlow` (spawn atlanır; akış per-tetik,
  kendini döngülemez — guardrail'ler tetik sıklığını sınırlar, SpawnTags yok sayılır).
- **API:** create/update (schedules+automations) `flowId` alır; doğrulama gevşetildi
  (cron/triggerTag + promptTemplate zorunlu; hedef = flowId **ya da** ajan). Automation
  update hedef-değiştirme mantığı flowId↔agent (kısmi güncelleme hedefi ellemez).
- **Araçlar:** `create/update_schedule` + `create/update_automation` + `list_*`
  `flowId` (flow varlığı `GetFlow` ile doğrulanır).
- **UI:** ortak `TargetModeToggle` (Ajan/Akış) + `FlowPicker` (Schedules.tsx export;
  Automations import); create/edit formları + liste satırları (`Workflow` ikonu +
  `🔀 <akış>`); akış otomasyonunda spawn-etiket editörü yerine bilgi notu.
- **Test:** `db/automation_test.go` `TestAutomationFlowIDRoundTrip` +
  `TestScheduleFlowIDRoundTrip`. `go build ./...` + `go test` (409) yeşil, `tsc` temiz.
- Dosyalar: `db/models_task.go`, `db/models_automation.go`, `db/store_schedule.go`,
  `db/store_automation.go`, `agent/scheduler.go`, `agent/automation.go`,
  `api/schedules.go`, `api/automations.go`, `tools/builtin_schedulemgmt.go`,
  `tools/builtin_automationmgmt.go`, `frontend/{types/task.ts, api/tasks.ts,
panels/Schedules.tsx, panels/Automations.tsx}`. Detay `20-SCHEDULE-WAKE.md`,
  `46-ETIKET-OTOMASYON.md`.

## Logs: sayaç + Kopyala + Aç üst-title'a taşındı ✅ (2026-07-04)

- `logs` `HEADERLESS_VIEWS`'e eklendi; `LogsPanel` kendi `PaneHeader`'ını render ediyor
  (title="Loglar", `right` = "N satır · N kayıt" sayacı + `CopyPathButton` (ikon) +
  `RevealButton` **label="Aç"**). Bu üç öğe filtre toolbar'ından çıkarıldı; toolbar'da
  seviyeler/arama/grupla/canlı/Yenile kaldı.
- Reveal butonu artık "Aç" metnini gösteriyor (`labelClassName="hidden sm:inline"`).
- Dosyalar: `App.tsx`, `panels/LogsPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Loglar · 35 satır · 36 kayıt · [kopya] · Aç").

## Schedules toggle title'a + Flows minimap toggle + Market butonları panele ✅ (2026-07-04)

Üç ayrı UI isteği. Hepsi canlı doğrulandı (browser-mcp), `tsc -b` temiz.

- **Schedules — otonomi toggle başlığa taşındı:** `schedules` artık `HEADERLESS_VIEWS`
  içinde; `Schedules` kendi `PaneHeader`'ını render ediyor (title="Otomasyon",
  `right` = "Otonomiyi duraklat" switch). İçerikteki eski büyük duraklat kartı
  kaldırıldı. Dosyalar: `App.tsx`, `panels/Schedules.tsx`.
- **Flows — mini harita aç/kapa butonu:** `FlowCanvas` `CanvasInner`'a `showMinimap`
  state'i + `CanvasTools` içine "🗺 Mini harita" toggle butonu eklendi (`CanvasTools`
  artık salt-okunur modda da render oluyor; auto-layout yalnız editable). `<MiniMap>`
  koşullu. Dosya: `flow/FlowCanvas.tsx`.
- **Market — İçe Aktar + Kaynaklar sol panele:** iki buton üst header'dan çıkarılıp
  sol "Kategoriler" `ListPane`'inin altına (border-top'lu footer) taşındı. Header'da
  yalnız arama + Yenile kaldı. Dosya: `panels/MarketPanel.tsx`.

## Agents: aktivite paneli üst-title'daki butondan toggle ✅ (2026-07-04)

Kullanıcı isteği: ajan ekranında aktivite paneli, başlıktaki bir butonla açılıp
kapansın. Canlı doğrulandı (browser-mcp).

- `AgentsView` `PaneHeader` `right`'ına **"Aktivite"** toggle butonu eklendi
  (`data-testid="agent-activity-toggle"`, `Activity` ikonu, chat'teki "Detay"
  butonu deseni; açıkken accent kenarlık, `aria-pressed`). Copy/Aç butonları yalnız
  ajan seçiliyken; Aktivite butonu her zaman görünür.
- Eski **slim dikey reopen-rail** (kapalı durumda sağ kenardaki şerit) kaldırıldı;
  panel artık yalnız başlıktaki butondan açılıp kapanıyor (`{activityOpen && <AgentActivityPanel/>}`).
  Panelin kendi X (onClose) butonu korundu.
- Dosya: `agents/AgentsView.tsx`. `tsc -b` temiz.

## Artifacts + Skills başlıkları da üst-title'a birleştirildi ✅ (2026-07-04)

Flows/Agents desenini diğer detay ekranlarına uygulama. Canlı doğrulandı (browser-mcp).

- **Artifacts:** detay toolbar'ı (başlık + origin/kind/creator rozetleri + eylemler:
  Düzenle/İçerik-kopyala/Yol-kopyala/Aç/Kaynak-sohbet/Sil) tamamen üst `PaneHeader`'a
  taşındı → `titleSlot` = kimlik, `right` = eylemler. Düzenleme modunda `right` =
  Kaydet/İptal. "Artifactlar" başlığı + subtitle detay görünümünde kalktı.
- **Skills:** isim + rozetler (grup/kısıt/görünürlük/kaynak) `titleSlot`'a, eylem
  grubu (Düzenle/Kısıtla-Paylaş/Görünürlük-seçici/Yol-kopyala/Aç/Sil) `right`'a taşındı.
  Açıklama bloğu (slug, açıklama, ne-zaman, izinli araçlar, alt-beceriler) gövdede
  slim strip olarak kaldı. "Skills" başlığı detay görünümünde kalktı.
- **Uygulanmayan:** `Araçlar & MCP` (detay salt-içerik, ayrı toolbar/path yok) ve
  liste-ağırlıklı ekranlar (Market/Hafıza/Loglar/Bütçe) desene uymuyor — dokunulmadı.
- Dosyalar: `panels/ArtifactsPanel.tsx`, `panels/SkillsPanel.tsx`. `tsc -b` temiz.

## Fix: Schedules ekranında "Zamanlamalar" başlığı scroll dışında kalıyordu ✅ (2026-07-04)

- **Belirti:** Schedules ekranında "Zamanlamalar (cron / zaman tabanlı)" başlığı +
  yeni-zamanlama formu üstte sabit (pinli) kalıyor, yalnız alttaki liste (içine
  Automations da giriyor) kayıyordu → başlık/form dikey alan yiyor, scroll dışında.
- **Çözüm:** `Schedules.tsx` tek scroll kapsayıcısına alındı — kök `flex-col`
  (p-4'süz), içine `min-h-0 flex-1 overflow-y-auto p-4` sarmalayıcı; başlık + form +
  liste + `Automations` birlikte kayar. Liste div'i `flex-1 … overflow-y-auto` →
  `space-y-2`. Deep-link `scrollIntoView` çalışmaya devam eder. `tsc -b && vite build` temiz.

## Flows: Şablonlar + Koşular başlıkları da üst-title'a birleştirildi ✅ (2026-07-04)

Önceki birleştirmenin (flow editörü + agents) devamı — aynı desen Şablonlar ve
Koşular sekmelerine uygulandı. Canlı doğrulandı (browser-mcp).

- **Şablonlar:** şablon önizleme üstündeki ayrı toolbar kaldırıldı; içeriği üst
  `PaneHeader`'a taşındı → `titleSlot` = şablon adı + açıklaması; `right` =
  "salt-okunur önizleme" + "+ Bu şablondan akış oluştur". "Akışlar" başlığı kalktı.
- **Koşular:** `RunView`'e `hideSummary` prop'u eklendi (üst özet satırını gizler,
  Girdi/Hata satırları kalır). `STATUS_LABEL` + `statusColor` export edildi;
  `FlowsPanel` üst `PaneHeader`'da `titleSlot` = akış adı + durum + tarih, `right` =
  "Tekrar çalıştır" butonu. "Akışlar" başlığı kalktı, özet çift render olmuyor.
- `title` artık üç detay görünümünde de (editor/template/run) gizli; yalnız
  boş/liste durumunda "Akışlar".
- Dosyalar: `flow/RunView.tsx`, `panels/FlowsPanel.tsx`. `tsc -b` temiz.

## Flows + Agents başlıkları tek üst-title'a birleştirildi ✅ (2026-07-04)

Kullanıcı isteği: detay başlıklarını tek üst-bar'a topla (sohbet başlığı deseni).
Canlı doğrulandı (browser-mcp).

- **`PaneHeader` esnetildi:** `title` opsiyonel oldu + yeni `titleSlot?: ReactNode`
  (title/subtitle bloğunun yerine büyüyen özel içerik — ör. isim inputu). Sol
  konteyner `flex-1` aldı ki input genişleyebilsin. Hamburger zaten `md:hidden`
  (yalnız dar ekran).
- **Flows:** flow editöründe ayrı "meta toolbar" (alt title) kaldırıldı; içeriği üst
  `PaneHeader`'a taşındı → `titleSlot` = akış-adı inputu + ID chip; `right` =
  Yol-kopyala (ikon) + "Aç" + "Kaydet". "Akışlar" başlığı ve `· <akış adı>` subtitle
  editör görünümünde kaldırıldı (şablon/koşu/boş sekmelerde "Akışlar" korunur).
  Doğrulama: header'da input="akış adı", id="FLW…", butonlar [Listeyi göster, Yolu
  kopyala, Aç, Kaydet], "Akışlar" yok.
- **Agents:** `CopyPathButton` + `RevealButton` `AgentSettingsForm` header'ından
  `AgentsView` üst `PaneHeader`'ının `right`'ına taşındı; reveal etiketi "Klasörü aç"
  → **"Aç"**. Kullanılmayan importlar (`api`, `CopyPathButton`, `RevealButton`)
  AgentSettingsForm'dan temizlendi.
- Dosyalar: `common/PaneHeader.tsx`, `panels/FlowsPanel.tsx`, `agents/AgentsView.tsx`,
  `agents/AgentSettingsForm.tsx`. `tsc -b` temiz.

## Sol panellerdeki "listeyi gizle" butonları kaldırıldı (sohbet listesi gibi) ✅ (2026-07-04)

Sohbet oturum listesinde panel-kapat butonu yok; mobilde drawer'ı **boşluğa
(backdrop) tıklayarak** kapatıyorsun, masaüstünde ise sütun hep açık. Diğer liste
ekranlarındaki `PanelLeftClose` "Listeyi kapat" butonları bu davranışı gereksiz
kılıyordu — hepsi kaldırıldı.

- **`SidebarHeader` (`common/SidebarChrome.tsx`):** `onCollapse` prop'u ve kapat
  butonu tamamen kaldırıldı → Executions, Ajanlar, Artifactlar başlıklarındaki
  buton gitti (3 çağrı yeri güncellendi).
- **Liste-içi satır butonları kaldırıldı:** ToolsPanel, FlowsPanel, SkillsPanel,
  MarketPanel (`md:hidden` "Listeyi kapat" düğmeleri) + artık kullanılmayan
  `PanelLeftClose` importları temizlendi.
- **Korunan:** başlıktaki hamburger (Menu) açma butonu — mobilde drawer'ı açmak
  için gerekli (MarketPanel dahil), sohbetteki gibi.
- **Playwright doğrulaması (390px):** 7 ekranın hepsinde liste sütununda 0 gizle
  butonu; hamburger ile açılıyor, backdrop'a tıklayınca kapanıyor
  (drawer `left: 0 → -width`).

## Kopyala butonları sadece-ikon yapıldı ✅ (2026-07-04)

Kullanıcı isteği: kopyala butonlarındaki "Bağlamı kopyala" / "Copy" gibi metinler
kaldırılsın, yalnız kopyalama ikonu kalsın.

- **Metin→ikon (4 buton):** `markdown/CodeBlock` ("Copy/Copied" → `Copy`/`Check`),
  `markdown/MermaidDiagram` (aynı), `sessions/SessionContextModal` ("Bağlamı kopyala"
  → ikon), `agents/AgentContextModal` ("Promptu kopyala" → ikon). Erişilebilirlik
  için `title` + `aria-label` eklendi; CodeBlock/Mermaid'e `lucide-react` ikonları,
  Session/Agent modal'a `Check` importu geldi.
- **Zaten ikon-only:** tüm `CopyPathButton` örnekleri (`labelClassName="hidden"`),
  `SecretsPanel`, `PromptEditor`, `ClaudeAuthDialog`, `ArtifactsPanel` kopya butonları.
- **Bilinçli istisna (etiketli bırakıldı):** `SessionsSidebar` sağ-tık menüsündeki
  "Yolu kopyala" (menü öğesi — liste satırı, etiket gerekli) ve `ExecutionsPanel`
  seçim-çubuğundaki "Kimlikleri kopyala" (bulk-action; `SelectionBarButton` children
  zorunlu, ikon-only UX'i bozardı). İstenirse bunlar da dönüştürülebilir.
- `tsc -b` temiz; canlı doğrulama (browser-mcp) sorunsuz.

## Liste ekranı başlıkları tam sohbet paritesi: başlık artık listenin üstüne gelmiyor ✅ (2026-07-04)

Sorun: liste ekranlarında (Aktivite, Ajanlar, Artifactlar, Skills, Araçlar, Akışlar)
`PaneHeader` tüm genişliğe yayılıyordu — başlık, soldaki listenin **üzerine** de
geliyordu. Sohbet ekranında ise başlık yalnız içerik alanının üstünde; oturum
listesinin üzerine gelmez.

**Kök neden:** panel yapısı `flex-col > [PaneHeader(tam genişlik)] > [row: liste | detay]`
şeklindeydi. Sohbet ise `flex-row > [liste (tam yükseklik)] | [main: header + içerik]`.

**Düzeltme:** her liste ekranı sohbet düzenine geçirildi →
`flex-row > [ListPane (tam yükseklik kardeş sütun)] | [içerik-kolonu: PaneHeader + detay]`.
Böylece başlık çubuğu yalnız listenin **sağındaki** içerik kolonunun üstünde durur.
Playwright ile doğrulandı: 6 ekranın hepsinde `header.left === list.right` (üst üste
binme yok), 1280px'de hamburger gizli, 390px'de hamburger görünür + drawer varsayılan
kapalı, yatay taşma yok.

- **Değişen dosyalar:** `ExecutionsPanel`, `AgentsView` (3 kolon korundu:
  roster | ayarlar | aktivite), `ArtifactsPanel` (drop-zone kök satır oldu),
  `SkillsPanel`, `ToolsPanel`, `FlowsPanel`.
- **Path butonları sohbet stiline getirildi:** başlıklardaki `Yolu kopyala`
  (`labelClassName="hidden"` → sadece ikon) ve `Aç` (`labelClassName="hidden sm:inline"`)
  artık sohbet başlığındakiyle birebir aynı. Executions'ta path butonları detay
  alt-başlığından `PaneHeader.right`'a taşındı (sohbetteki gibi sağ üstte).

## Tüm kopyalama butonları merkezi `copyToClipboard`'a taşındı ✅ (2026-07-04)

Önceki düzeltmenin devamı: uygulamadaki tüm doğrudan `navigator.clipboard.writeText`
kullanımları tek merkezi metoda migrate edildi (güvensiz LAN/HTTP bağlamında hepsi
sessizce bozuktu).

- **Merkezi metod:** `lib/clipboard.ts` → `copyToClipboard(text, promptLabel?)`.
  İç akış: `copyText` (Clipboard API → `execCommand` fallback) başarısızsa
  `window.prompt` ile elle-kopya. Programatik kopya başarılıysa `true` döner →
  çağıran "Kopyalandı" onayını yalnız bunda gösterir. `navigator.clipboard`'a
  doğrudan dokunan tek yer artık bu dosya.
- **Migrate edilen 9 dosya (10+ buton):** `markdown/CodeBlock`, `markdown/MermaidDiagram`,
  `agents/AgentContextModal`, `sessions/SessionContextModal`, `panels/ArtifactsPanel`,
  `settings/ClaudeAuthDialog`, `common/PromptEditor`, `panels/SecretsPanel`,
  `panels/ExecutionsPanel` (toplu-id + oturum-id kopyala). Ayrıca daha önce düzeltilen
  `CopyPathButton` + `App.tsx` (`openFile`, `copySessionPath`) de artık merkezi metodu
  kullanıyor (inline prompt kaldırıldı).
- `tsc -b` temiz; canlı smoke testi (browser-mcp) sorunsuz.

## Claude Code cache paritesi P1+P6: native yolda dinamiği mesaj kuyruğuna taşı ✅ (2026-07-04)

`_Docs\50-CLAUDE-CODE-CACHE-PARITE.md` planının **P1** (en büyük kazanç) + **P6**'sı uygulandı.
**Teşhis:** native (anthropic + OpenAI-compat) yolda Tools + statik System zaten cache HIT
alıyordu, ama volatile **Dinamik `system` alanında** (tools+mesajların önünde) durduğu için
asıl büyüyen kısmın (mesaj geçmişi) rolling breakpoint'i **her tur ıskalıyordu** → pratikte
ölü. External Agents bunu yaşamıyor çünkü cache/compaction'ı native `claude` binary'ye (Claude
Agent SDK) devrediyor; TionHarness kendi yazdığı için boşluk oluşmuş.

**Yapılan (yalnız `extendedCache`/`cacheSystem` açıkken; cache-kapalı yol birebir korundu):**

- `anthropic.go`: `systemField` artık **statik-only** (tam cache'lenebilir); `toAnthropicMessages`
  yeni imza `(msgs, extendedCache, dynamic)` — rolling breakpoint son **persist** mesaj bloğunda,
  volatile dinamik onun **gerisinde** trailing text-blok olarak (request-time, persist edilmez).
- `minimax.go`: `buildSystemMessage` statik-only; `attachHistoryBreakpoint` işaretlediği
  **indeksi döndürür**; dinamik o mesaja breakpoint'ten sonra eklenir. Fallback: uygun mesaj
  yoksa trailing user mesajı.
- **Invariant:** cache öneki yalnız immutable içerik barındırır → dinamik hiç persist edilmez,
  sonraki tur önek byte-aynı kalır → HIT. (claude-cli'nin `withDynamic(lastUserText…)` deseninin
  native'e genellenmesi.)
- `session_context.go` `computeCachePreview` anthropic dalı: `CachedMsgCount=msgCount-1` (rolling),
  Araçlar+Sistem+geçmiş cache'li, dinamik "tail/taze" notu.
- Testler: `anthropic_test.go` + `minimax_test.go` yeni yerleşimi kilitler (dinamik breakpoint'in
  gerisinde, cache-kapalıyken system'de kalır). **`go test` 289 yeşil**, `tsc` temiz.
- **Kalan:** canlı `cache_read>0` ölçümü (anthropic/openrouter anahtarı + gerçek tur). Sıradaki:
  **P2** (özeti compact-boundary mesajına çevir → özet de cache'lensin), sonra P4 (cache-break
  telemetri), P3/P5 (opsiyonel). Detay: `_Docs\50`.

## claude-cli ek yükü: ajan bağlam önizlemesinde de + buton sohbet başlığına ✅ (2026-07-04)

CLI ek-yük bilgisi artık **iki bağlam penceresinde de** görünür ve "Bağlam önizle"
butonu sohbet başlığına taşındı. `go build/test` + `tsc -b --force` temiz.

- **Ajan bağlam önizlemesi (`AgentContextModal`):** `handleAgentContext` artık
  `computeCLIOverhead`'i **boş sessionID** ile çağırır → predicted-only (ölçüm yok,
  ajanın oturumu yok). `computeCLIOverhead` boş sessionID'de debug/usage okumasını
  atlar. UI'da "Beklenen (CLI, tahmini)" chip'i + "CLI ek yükü" uyarı kutusu. Canlı:
  AGT4 → predicted 27.275 (eager=5). `agentContextPreview.cliOverhead` alanı eklendi.
- **Buton taşındı:** `SessionDetailPanel` "Araçlar" kartındaki "Bağlam önizle (debug)"
  → sohbet başlığına (`App.tsx` header, `ScanEye` ikon). `SessionContextModal` artık
  App.tsx'ten render edilir; detay panelini açmadan erişilir. İlgili import/state/modal
  detay panelinden temizlendi.

## claude-cli ek yükü: ölçülmüş generic referans + önceden tahmin ✅ (2026-07-04)

Kullanıcı "Tahmin ↔ Gerçek" farkının (claude-cli vergisi) nereden geldiğini sordu →
bileşenler **empirik ölçüldü** ve uygulama geneli generic bir referansa dönüştürüldü.
`go build ./...` + `go test ./internal/conversation ./internal/api` temiz (95 test).

- **Ölçüm (claude-cli 2.1.201, gerçek API `usage`):** saf sistem promptu 0 araç =
  **17.067**; +dahili araçlar (~15) = **26.265** (dahili ≈ 9.198); köprülü araç başına
  ort. şema **~215** (42–710). Doğrulama: SES104 Tahmin 29.573 → Gerçek 88.425.
- **Generic kaynak `internal/conversation/clioverhead.go`:** `CLIBaseSystemTokens`,
  `CLIBuiltinToolsTokens`, `CLIBaseTokens`, `CLIAvgBridgedToolTokens` sabitleri +
  `PredictCLIOverhead(loadedTools)` = `26.200 + yüklü×215`. Token-hesap yapan her yer
  bu tek kaynaktan okur (native paket, import döngüsü yok).
- **`api/session_context.go`:** `computeCLIOverhead` artık `eagerTools` alır (call-site
  `interactionTier(d.Name)=="core"` sayımı — gerçek eager/deferred ayrımı, fs built-in'ler
  tabanda); ölçüm yokken (`measured==0`) 0 yerine `PredictCLIOverhead` ile **önceden
  tahmin (cold-start floor)** verir. `cliOverheadPreview.predictedOverhead` alanı (JSON)
  eklendi. Ölçüm-yok notu artık taban rakamları içerir.
- **Frontend (`SessionContextModal.tsx` + `types/session.ts`):** ölçüm yokken başlıkta
  "Beklenen (CLI, tahmini) = Tahmin + predictedOverhead" Stat'ı + "CLI ek yükü" kutusunda
  "Tahmin → beklenen ~X (+Y tahmini ek yük, henüz ölçülmedi)" satırı. Ölçüm gelince eski
  "Gerçek" görünümü. Benim dosyalarım `tsc` temiz.
- **Docs/skill:** `_Docs\17` "claude-cli ek yükü — ölçülmüş referans" bölümü;
  `tionharness-session-debug` skill'ine "Tahmin↔Gerçek farkı" ölçüm-referansı + tekrar-
  ölçüm komutu eklendi. `clioverhead_test.go` regresyon kilidi.

## Liste panelleri tam sohbet paritesi: masaüstü hep açık + hamburger sadece mobil ✅ (2026-07-04)

Kullanıcı: liste ekranları (Aktivite vb.) sohbet ekranı gibi olmalı — geniş ekranda
**sol panel hep açık**, **hamburger geniş ekranda gizli**, daralt-butonu yok. Önceki
davranış (masaüstünde de daraltılabilir + toggle her boyutta) sohbetten farklıydı.
Sohbet sessions-drawer modeline geçirildi. `tsc -b && vite build` temiz; Playwright
ile masaüstü (1280) + mobil (390) doğrulandı.

- **`common/CollapsibleListShell.tsx` yeniden yazıldı:** liste artık DAİMA DOM'da —
  `md+` statik sütun (hep görünür, daralma yok), `< md` sola kayan drawer (`open`
  yalnız mobil translate'i sürer) + backdrop. Eski "kapalıyken null / ince ray"
  mantığı kaldırıldı.
- **`useCollapsibleList` sadeleşti:** artık ephemeral `useState(false)` (persist yok)
  — sohbetin `mobileListOpen`'ı gibi; mobilde her açılışta drawer kapalı gelir
  (eski persist, drawer'ı açık açıyordu).
- **Tüm toggle'lar `md:hidden`:** PaneHeader hamburger, Market inline, App
  workspace/settings toggle + liste-içi daralt butonları (SidebarHeader onCollapse,
  Skills/Tools/Market/Flows) → geniş ekranda görünmez, sadece mobil drawer'ı sürer.
- **Executions başlığı bağlamlı:** `Aktivite · {seçili çalıştırma}` (sohbetteki
  "Sohbet · Manager" gibi). Doğrulama: masaüstü hamburger gizli + aside statik
  görünür; mobil hamburger görünür + drawer default kapalı, tıklayınca dolu-surface
  drawer + backdrop, taşma yok.

## Flow açıklaması (description) uygulamadan tamamen kaldırıldı ✅ (2026-07-04)

Akışların `description` alanı uçtan uca kaldırıldı. `go build/test` (413) + `tsc -b &&
vite build` temiz.

- **Backend model/store/API:** `db.Flow.Description` alanı silindi; `store_flow.UpdateFlow`,
  `api.flows` (`flowReq` + create/update), `summarizer` (SummaryFlows artık `Ad (id)`),
  `api/graph.go` (node `Sub` = flow id) güncellendi.
- **Agent araçları (`builtin_flowmgmt.go`):** `create_flow`/`update_flow` şemalarından
  `description` prop'u; `list_flows`/`get_flow` çıktılarından `description` alanı; ilgili
  tool açıklama metinleri temizlendi.
- **Market/şablon (tam temizlik — 2026-07-04):** HarnessPack format tiplerinden
  `FlowPayload.Description` ve `WorkspaceTemplateFlow.Description` alanları **da silindi**
  (Go `market/pack.go` + frontend `types/market.ts`); install/publish/seed zaten db.Flow
  ile bağ kurmuyordu. `MarketPanel` flow açıklaması render'ı kaldırıldı. `gen_examples.py`
  `flow_pack` artık flow'a description koymaz (pack-seviye `description` korunur).
  **Gömülü paketler temizlendi:** `internal/market/defaults/workspace.*.harnesspack.json`
  içindeki 5 flow-description anahtarı silindi (4 dosya; `blank`'te yoktu). Ev dizininde
  başka `.harnesspack` yok. Not: çalışan workspace store'larındaki `FLW*.json` dosyalarında
  kalan eski `description` anahtarları zararsız (yükte yok sayılır, ilk kayıtta düşer);
  uygulama çalışırken canlı store'a dokunulmadı.
- **Frontend:** `types/flow.ts` + `api/flows.ts` (createFlow/updateFlow imzaları)
  `description`'sız; `FlowsPanel` state/dirty/kaydet/şablon çağrıları temizlendi;
  `useChatStream` slash-komut açıklaması sadeleşti; `WorkspaceExportPanel` sub → flow id.
- **Sol flow listesi:** açıklama satırı yerine **flow id (mono) · N node** meta satırı
  (`flowNodeCount` helper). Detay `_Docs\15`.

## Yol kopyala butonu güvensiz bağlamda (LAN IP/HTTP) düzeltildi ✅ (2026-07-04)

Kullanıcı "Copy path / Open path çalışmıyor" bildirdi. Canlı tarayıcıda (browser-mcp,
`http://<lan-ip>:5173`) teşhis edildi:

- **Kök neden (Copy):** `window.isSecureContext === false` → `navigator.clipboard`
  **undefined**. Eski kod `navigator.clipboard?.writeText` (optional chaining) ile
  sessizce hiçbir şey yapmıyordu. Async Clipboard API yalnız güvenli origin'de
  (HTTPS veya `localhost`) açık; düz-HTTP LAN IP'de yok. Test: bu Chrome güvensiz
  bağlamda `document.execCommand('copy')`'yi de (gerçek tıklama gesture'ında bile)
  **false** döndürüyor.
- **Çözüm:** yeni `lib/clipboard.ts` → `copyText()` (Clipboard API → execCommand
  fallback, boolean döner). `CopyPathButton` ve `App.tsx` (`openFile`,
  `copySessionPath`) bunu kullanır. Her ikisi de başarısızsa **son çare**:
  `window.prompt(...)` yolu seçili gösterip kullanıcının Ctrl+C ile elle
  kopyalamasını sağlar → hiçbir bağlamda sessiz başarısızlık kalmaz.
- **Open (Aç) aslında çalışıyor:** backend `/api/sessions/{id}/reveal` →
  `explorer.exe <path>` host'ta klasörü açıyor (Shell.Application ile doğrulandı).
  Uzak cihazdan erişimde host'ta açılır (tasarım gereği), istemcide değil. Önceki
  manuel test 404'leri geçiciydi (hash chat route'unda değilken `sessionId=undefined`
  gidiyordu) — endpoint sağlam.
- Not: uygulamada ~13 yerde daha doğrudan `navigator.clipboard` kullanımı var;
  bunlar da güvensiz bağlamda çalışmaz — ileride `copyText`'e migrate edilebilir.
- Dosyalar: `frontend/src/lib/clipboard.ts` (yeni), `frontend/src/components/CopyPathButton.tsx`,
  `frontend/src/App.tsx`.

## Chat başlığından bütçe-yönlendiren harcama pill'i kaldırıldı ✅ (2026-07-04)

- **`ChatMeters` kaldırıldı:** chat üst-bar'ındaki "BUGÜNKÜ harcama" pill'i
  (`N çağrı · ~$X`, tıklayınca Bütçe ekranına gidiyordu) `App.tsx` başlığından
  silindi; import ve artık öksüz kalan `components/panels/ChatMeters.tsx` dosyası
  tamamen kaldırıldı. `meterRefresh` state'i korundu (artifact yenileme +
  `SessionDetailPanel` hâlâ kullanıyor). Bütçe verisi Bütçe ekranı + oturum detay
  panelinde zaten mevcut.
- Dosyalar: `frontend/src/App.tsx`, `frontend/src/components/panels/ChatMeters.tsx` (silindi).

## Ajan-yanı model etiketi + isim hizalaması ✅ (2026-07-04)

Kullanıcı isteğiyle 2 küçük UI düzeltmesi (canlı tarayıcıda browser-mcp ile doğrulandı).

- **Model etiketinden açıklama eki kırpıldı:** `resolveModelLabel` (`lib/catalog.ts`)
  artık `stripTagline` ile katalog label'ındaki boşlukla ayrılmış tire sonrası eki
  atar → "Sonnet — dengeli" yerine "Sonnet", "MiniMax M3 - guncel amiral" yerine
  "MiniMax M3". Regex `/\s+[—–-]\s+.*$/` boşluksuz tireleri ("GPT-5.5") ve parantezli
  varyantları ("Opus 4.8 (Fast)") korur. Tam açıklamalı label'lar model seçicilerde
  (`ProviderModelSelect`/`ProvidersPanel`) aynen kalır; yalnız ajan-yanı gösterim
  sadeleşir.
- **Ajan ismi hizalaması standartlaştı:** `AgentIdentity` kök span'ine `text-left`
  eklendi. `<button>` varsayılan `text-align:center` taşıdığından composer'ın
  `AgentSelect` tetikleyicisinde (ve `AgentPicker` trigger'ında — `text-left`
  class'ı yoktu) ajan ismi ortalanıyordu; artık her yerde sola hizalı.
- **Dar telefonda sadece avatar:** `AgentIdentity`'ye `mobileIconOnly` prop'u eklendi
  → metin sütunu `md` altında `hidden` (yalnız avatar). Composer `AgentSelect`
  tetikleyicisi bu prop'u kullanır (`md:max-w-[180px]`), böylece dar ekranda ajan
  seçici kompakt kalır.
- Dosyalar: `frontend/src/lib/catalog.ts`, `frontend/src/components/agents/AgentIdentity.tsx`,
  `frontend/src/components/chat/composer/AgentSelect.tsx`.

## Liste-toggle butonu tüm ekranlarda sohbet hamburger'ıyla eşitlendi ✅ (2026-07-04)

Kullanıcı geri bildirimi: Executions/Agents/… başlığındaki liste aç/kapa butonu
(turuncu çerçeveli `PanelLeft` kutu) sohbet başlığındaki hamburger'dan farklı
görünüyordu. Hepsi sohbetteki **çerçevesiz `Menu` (hamburger), dim renk** stiline
alındı. `tsc -b && vite build` temiz; Playwright ile doğrulandı (border 0px, dim renk).

- `common/PaneHeader.tsx` (Agents/Artifacts/Tools/Skills/Executions/Flows),
  `MarketPanel` inline toggle, `App.tsx` header workspace/settings toggle → hepsi
  `Menu` + `flex h-8 w-8 rounded-lg text-dim hover:bg-surface-2` (sohbet hamburger'ının
  birebir sınıfları). Eski `border-accent` (kapalıyken) / bordered kutu kaldırıldı.

## Flow editörü: popup boyutu + palet sürükle-bırak ✅ (2026-07-04)

- **Popup boyutu board popup'ına eşitlendi** (`TaskFormModal`): `max-h-[90vh] w-full
max-w-2xl rounded-xl` (önceki `w-80 max-h-[85vh]` yerine).
- **Palet sürükle-bırak ile node ekleme:** node listesindeki tipler `draggable`;
  `FlowCanvas` `CanvasInner`'a ayrıldı (ReactFlowProvider altında `screenToFlowPosition`
  erişimi için), `onDrop` bırakma noktasını graf uzayına çevirip `onDropNode` →
  `FlowsPanel.addNodeAt` ile node'u **o konumda** oluşturur. Tık ile ekleme korundu.
  MIME: `FLOW_NODE_DND_MIME`. `tsc -b && vite build` temiz.

## Flow editörü UX düzeni: popup node editörü + sadeleşmiş toolbar ✅ (2026-07-04)

Kullanıcı isteğiyle 4 UI değişikliği. `tsc -b && vite build` temiz.

- **Meta toolbar sadeleşti:** açıklama (`description`) alanı kaldırıldı; "yolu kopyala"
  **icon-only** (`CopyPathButton` `label` prop'u kaldırıldı); ad girişi genişledi.
- **Görünüm ayarları sol palete taşındı:** etiket (`TagEditor`), "Kablo" edge-style
  seçici ve "Animasyon" toggle artık "Node ekle" paletinin altında **Görünüm** bölümünde
  (palet `w-40`, mobil `w-32`).
- **Node editörü popup oldu:** sabit sağ panel → `ModalOverlay`. Node'a **tıklayınca**
  açılır; `FlowCanvas`'a `onNodeClick` prop'u + `nodeDragThreshold={4}` eklendi →
  sürükleme/tıklama karışmaz. Boş canvas/Escape/backdrop kapatır.
- **Node üstü toolbar kaldırıldı:** `NodeToolbar` yerine Başlangıç/Çoğalt/Sil eylemleri
  popup içindeki `NodeInspector` başlığında (`onDuplicate` prop'u eklendi). FlowsPanel
  artık `nodeActions` geçmiyor → `NodeActionsContext` null.
- Dosyalar: `FlowsPanel.tsx`, `flow/FlowCanvas.tsx`, `flow/NodeInspector.tsx`,
  `CopyPathButton.tsx` (değişmedi — zaten opsiyonel label). Detay `_Docs\15`.

## Portrait UI testi (Playwright) + taşma düzeltmeleri ✅ (2026-07-04)

Gerçek tarayıcıda (Playwright, 360px & 390px) 16 view tarandı; yatay-taşma
(docW > viewport) tespiti için clip-farkında JS detektörü kullanıldı. 4 gerçek
taşma bulundu ve düzeltildi; tümü "dar ekranda sarmalanmayan/`shrink-0` buton
satırı" desenindeydi. Yeniden tarama: 16/16 view temiz (docW=360), drawer açıkken
dolu surface + taşma yok.

- **Chat composer:** `px-6→max-md:px-3`, toolbar satırı `flex-wrap`, `AgentSelect`
  ad genişliği mobilde `max-w-[120px]` → "Gönder" artık taşmıyor.
- **Skills detay eylem çubuğu:** başlık + toolbar `flex-wrap` (eski `shrink-0`
  kaldırıldı) → ~238px taşma giderildi.
- **Artifacts viewer başlığı:** `flex-wrap` (başlık + aksiyon toolbar).
- **Memory ekle satırı:** `flex-wrap` + input `min-w-[10rem]`.
- Not: layout-dışı, pre-existing bir React uyarısı gözlendi — Sağlayıcılar
  listesinde çift `key="openrouter"` (veri kaynaklı; ayrı ele alınmalı).

## Standart ekran başlığı: PaneHeader + başlıktan liste aç/kapa (9 ekran) ✅ (2026-07-04)

Tüm liste ekranları sohbet ekranı gibi bir **başlık çubuğu + tıklanabilir liste
aç/kapa butonu** kazandı; kapalıyken liste tamamen gizlenir (sohbet gibi, ince ray
yok), başlıktan yeniden açılır. `tsc -b && vite build` temiz.

- **Yeni `common/PaneHeader.tsx`:** standart ekran başlığı (sol toggle + başlık +
  ops. subtitle + sağ aksiyonlar). **`CollapsibleListShell`/`ListPane` `hideRail`:**
  kapalıyken null döner (ray yerine başlık toggle'ı açar).
- **PaneHeader'lı 7 ekran:** Agents (roster→ListPane, subtitle = seçili ajan / "Ajan
  seçilmedi", boş-durum korunur), Artifacts, Tools, Market (toggle mevcut katalog
  başlığına), Skills, Executions, Flows.
- **App-header toggle'lı 2 ekran:** Workspace + Settings — kategori/sekme `<aside>`'ı
  `CollapsibleListShell hideRail` ile sarıldı; collapse state App'te
  (`workspaceNav`/`settingsNav` = useCollapsibleList), App header'ında PanelLeft
  toggle. Detay: `_Docs\49` §7.6.

## Refactor: iki-panelli liste ekranları tek `ListPane` standardında ✅ (2026-07-04)

Her iki-panelli ekran kendi liste-kolonu çözümünü uyguluyordu (kimi
`useResizableSidebar`, Skills özel resize, Tools/Market sabit genişlik; farklı
bg/border/handle; kimi CollapsibleListShell'li kimi değil). Tek standart bileşene
indirgendi. `tsc -b && vite build` temiz.

- **Yeni `common/ListPane.tsx`:** tek standart sol liste kolonu — `CollapsibleListShell`
  (daralt/rail + mobil drawer) + dolu surface `<aside>` + sağ border + `useResizableSidebar`
  (kalıcı sürükle-genişlet) + `ResizeHandle`. Ekran yalnız header + gövdeyi `children`
  olarak verir; genişlik/collapse/tema/handle ListPane'de.
- **Taşınan 6 panel:** Artifacts, Skills, Tools, Market, Flows, Executions. Kazanımlar:
  Skills'in **özel resize kodu silindi**; Tools/Market **artık resizable**; Executions
  **artık daraltılabilir**; hepsi aynı bg/border/genişlik/drawer davranışı.
- **Kapsam dışı:** `AgentsView` (roster | ayarlar | aktivite = 3-panel özel; mobil
  flex-col stack + aktivite paneliyle rail/drawer çakışması) mevcut paylaşılan
  primitiflerde bırakıldı. Detay: `_Docs\49` §7.5.

## Fix: Akış editörü yanlış "kaydedilmemiş değişiklik" ✅ (2026-07-04)

- **Belirti:** Flows ekranında bir akışa tıklayınca hiçbir düzenleme yapılmasa
  bile "değişiklik var" algılanıyor; ayrılırken/kapatırken uyarı çıkıyor
  (nav amber nokta + `beforeunload`).
- **Kök neden:** `FlowsPanel.flowDirty` canvas'tan yeniden kurulan graph'ı
  (`reactFlowToGraph` her zaman `next:""`, `x`, `y` üretir) backend'in stored
  JSON'u ile karşılaştırıyordu. Go `orchestration` modeli neredeyse tüm alanlarda
  `omitempty` kullandığından (`next`, `x`, `y`, `prompt`…) kayıtlı JSON boş alanları
  düşürüyor → iki taraf **her açılışta** farklı → sürekli dirty.
- **Çözüm:** normalize mantığı `flowGraph.ts` içinde tek `canonicalGraphKey()`
  helper'ına çıkarıldı — bir graph'ı editörün yüklemede kullandığı aynı round-trip'ten
  (`graphToReactFlow → reactFlowToGraph`) geçirip kararlı bir karşılaştırma anahtarı
  döndürür (cosmetic `edgeStyle`/`animated` hariç). `flowDirty` hem canlı canvas'ı
  hem stored graph'ı bu helper'dan geçirir → simetrik, yalnız gerçek düzenlemeler
  fark yaratır (`FlowsPanel.tsx` + `flowGraph.ts`). `tsc --noEmit` temiz.

## Sohbet UX küçük rötuşlar ✅ (2026-07-04)

- **Sohbet header "yolu kopyala" icon-only** (`App.tsx`, `labelClassName="hidden"`).
- **Sohbet listesinde oturum ID'si alt satıra taşındı** (`SessionsSidebar` — başlık
  satırından çıkıp meta satırında sağa hizalı; başlık artık daha geniş).
- **Koordinatör worker listesi daraltılabilir** (`CoordinatorSection` — "Worker'lar · N"
  başlığı + chevron, kalıcı `tionharness.coordWorkersOpen`).

`tsc -b && vite build` temiz.

## Panel UX: daraltılabilir listeler + otomasyon renk ayrımı ✅ (2026-07-04)

Kullanıcı isteğiyle 5 UI iyileştirmesi. `tsc -b && vite build` temiz.

- **Yeniden kullanılabilir daraltılabilir liste:** `hooks/useCollapsibleList.ts`
  (kalıcı, masaüstü açık / telefon kapalı) + `common/CollapsibleListShell.tsx`
  (açık: sütun / mobil drawer+backdrop; kapalı: ince yeniden-açma rayı) +
  `SidebarHeader` `onCollapse` (◀). Sessions sidebar deseninin genelleştirmesi.
- **Uygulandığı paneller:** Artifacts, Skills, Tools, Market (kategori rayı), Flows
  (sol akış listesi). Bu panellerde F3 mobil top-bottom stack **geri alındı** → drawer.
- **Flows node inspector:** editördeki sağ node paneli daraltılabilir
  (`PanelRightClose/Open`).
- **Agents aktivite paneli:** sohbet DetayPaneli gibi aç/kapa (X + "Aktivite" rayı).
- **Agents "yolu kopyala":** icon-only (`labelClassName="hidden"`).
- **Otomasyon ekranı renk ayrımı:** Zamanlamalar → sky sol şerit + "Zamanlamalar
  (cron)" başlığı; Otomasyonlar → violet sol şerit + violet başlık ikonu (form +
  satır + edit kartı). Hangisi ne, bir bakışta belli. Detay: `_Docs\49` §7.4.

## Üç optimizasyon: default-skill re-seed + skill sadeleştirme + run_code built-in binding'leri ✅ (2026-07-04)

1. **`EnsureDefaults` sürüm-farkında re-seed** (`internal/skills/defaults.go`): eskiden
   mevcut dosyanın üzerine hiç yazmıyordu → gömülü skill güncellemeleri mevcut
   kurulumlara yansımıyordu. Artık root'ta `.shipped-versions.json` sidecar'ı her
   default dosyanın son-shipped sha256'sını tutar; boot'ta: **eksik**→yaz, **gömülüyle
   aynı**→bırak+kaydet, **son-shipped ile aynı (kullanıcı dokunmamış)**→**tazele**,
   **ikisinden de farklı (kullanıcı edit'i)**→koru. İlk boot'ta manifest kendini
   seed'ler (senkronladığımız kopyalar gömülüyle eşit → hepsi kaydolur). Testler:
   `TestEnsureDefaultsSeeds` (edit korunur) + yeni `TestEnsureDefaultsRefreshesPristine`.
2. **`tionharness-project` workspace skill'i sadeleştirildi** (~194→~150 satır): "Mevcut
   Yetenekler" bölümü changelog seviyesi tarih/commit/test-ismi/env-minutiae'den
   arındırılıp "ne var + hangi `_Docs\NN`" özet haritasına indirildi (her-tur cache'li
   prefix küçüldü). Yetenek bilgisi korundu.
3. **`run_code`'a built-in araç binding'leri** (`_Docs\44`): code-execution artık yalnız
   MCP'yi değil, **TionHarness built-in araçlarını** da `tionharness` Python modülü olarak sunar
   (`from tionharness import <tool>`). `codemode.WriteBindings(dir, entries, builtins, allow)`
   - `Config.Builtin` dispatcher (reserved `tionharness__<tool>` namespace, bare-name
     allow/gate/dispatch); built-in'ler **tur ctx**'iyle `reg.Call`'a gider → sink/oturum
     davranışı direkt-çağrıyla birebir. Eligibility hard-exclude (`CodeModeEligible`:
     interaktif/exec-in-exec/delegasyon/meta/worker araçları hariç) + ajan tool-filter.
     run_code artık **MCP'siz** de kullanılabilir (built-in'ler yeter; `toolsetup.go`'da
     `len(entries)>0` koşulu kaldırıldı). Testler: bindings (built-in modül + collision
     guard), builtin_runcode (dispatch + discovery), bridge (routing). Tam suite 660 yeşil.

## Mobil/dikey ekran uyumu — F3 (liste panelleri tek-sütun) ✅ (2026-07-04)

İki-sütunlu paneller portrait telefonda **dikey stack** (üstte liste 45vh tavanlı,
altta içerik); memory roster sohbet gibi drawer. Masaüstü birebir korunur. `tsc -b
&& vite build` temiz.

- **Memory roster → drawer:** `mobileSessionsOpen` → `mobileListOpen` genellendi
  (sohbet+memory ortak); header hamburger `chat||memory`'de; `AgentRoster` sohbet
  sidebar'ıyla aynı drawer deseni.
- **6 headerless panel dikey stack:** kök `max-md:flex-col` + sol liste
  `max-md:!w-full max-md:max-h-[45vh] max-md:border-b` (`!important` inline
  resizable width'i ezer) — Executions/Agents/Tools/Skills/Artifacts/Market
  (Market kategori rayı `flex-row flex-wrap` chip). Detay: `_Docs\49` §7.3.

## Otonom self-completion: oto-devam (lazy-tool aktivasyon tuzağı) ✅ (2026-07-04)

**Sorun (canlı SES6/schedule):** Otonom (scheduler/spawn/wake) tek-atımlık tur,
ajan `ToolSearch`/`activate_tools` ile araç aktive edip todo yazdıktan sonra
`end_turn` ile bitiyordu. claude-cli'nin araç seti süreç başında sabit olduğundan
aktive edilen araçlar **ancak bir sonraki turda** kullanılabilir — ama otonom turda
sonraki tur yok → görev yarıda kalıyor, kullanıcı müdahale edemediği için asla
tamamlanmıyor. Error de yok (temiz `end_turn`, `result.subtype=success`).

**Çözüm — `internal/agent/autocontinue.go`:** Otonom tur bittikten sonra
`maybeAutoContinue` trace'i inceler; **tamamlanmamış iş sinyali** varsa
(`needsAutoContinue`: son anlamlı eylem lazy-tool aktivasyonu **veya** en güncel
`todo_write`'ta açık `pending`/`in_progress` madde) aynı oturumda history-aware bir
**devam turu** (`runSessionTurn` + Türkçe nudge, `Origin="auto-continue"`) tetikler,
yanıtı persist eder ve tekrarlar — **maks 10** (`DefaultAutoContinueMax`, ayar
`autonomousAutoContinueMax`). Devam turunun kalıcı MCP pool'u sayesinde aktive edilen
araçlar artık hazırdır. Duruş koşulları: iş bitti (`!needsAutoContinue`), tur araç
ilerlemesi yapmadı (`hasToolStep`=false → no-progress guard), sağlayıcı hatası veya
**günlük bütçe** stop'u (guardedComplete zaten enforce eder). Çağrı noktaları:
`runSpawn` (spawn.go) + scheduler deliver (scheduler.go), `FireTurnFinished`'ten önce.
Koordinatörün çalışan worker'ı varken genel oto-devam dürtmesi bastırılır; worker
sonucu zaten `<task-notification>` ile sonraki koordinatör turunu açar ve meşru
bekleme hali yanıltıcı “önceki tur yarım kaldı” mesajı üretmez.
Ayar `autonomousAutoContinue` (vars. açık) + `autonomousAutoContinueMax` (vars. 10) —
settings→tunables köprüsü `SetAutoContinue` (server.go applySettings), Tunables
`AutoContinue()`/`AutoContinueMax()`. settings paketi build+test yeşil; agent/api
derlemesi paralel codemode WIP'i (`builtin_runcode.go` ↔ yeni `WriteBindings` imzası)
yüzünden geçici bloke — kendi dosyalar gofmt-temiz, imzalar doğrulandı.

## Koordinatör: interaktif tur ↔ oto-tur kilit birleştirme ✅ (2026-07-04)

Stream/non-stream kullanıcı turu artık koordinatör oto-turlarıyla aynı kilidi
paylaşıyor: `BeginCoordinatorUserTurn` (`coordination.go`, sync.Cond'lu coordSlot)
interaktif tur boyunca slotu tutar; bu sırada gelen worker bildirimleri pending'e
düşüp release'te TEK coalesced oto-tur olarak koşar; kullanıcı turu cap'i
(`turns`/`capWarn`) resetler. Wiring: `chat_stream.go` + `chat.go`
(`Role=="coordinator"` → claim + defer release). Otonom yollar da kapsandı:
`claimTurnSlotIfCoordinator` (no-op release non-coordinator'da, cap RESETLEMEZ)
→ wake + scheduled prompt (`scheduler.go`) + inbox (`agentmsg.go`). 3 yeni test;
agent+api 191 test yeşil (tools/codemode test derlemesi paralel oturumun devam
eden run_code imza değişikliğinden kırık — bu işten bağımsız). Detay: `_Docs/47` §10.

## Koordinatör: canlı coalescing testi ✅ + stream/oto-tur kilit bulgusu (2026-07-04)

SES104'te 3 hızlı worker (ALPHA/BETA/GAMMA) aynı turda spawn edildi: 3 bildirim
→ **2 otomatik tur** (SES113+SES112 tek turda birleşti) — `coordSlot` coalescing
canlıda doğrulandı. Aynı testte bulgu: `handleChatStream`/`wake_turn` oturum
kilidi kullanmıyor, `drainCoordinator` da `isSessionActive`'e bakmıyor → kullanıcı
stream turu ile koordinatör oto-turu aynı oturumda **paralel** koşabiliyor
(canlıda gözlendi, zararsızdı; tasarım kararı bekliyor). Detay + zaman çizelgesi

- çözüm seçenekleri: `_Docs/47-KOORDINATOR-COKLU-AJAN.md` §10.

## Araç konsolidasyonu — birleşik built-in araçlar ✅ (2026-07-04)

Fazla/parçalı built-in araçlar tek çok-amaçlı araçlara indirildi (per-tur bağlam

- şema tekrarı azaldı, yetenek aynı). CRUD aileleri zaten standarttı, dokunulmadı.

* **`update_session`** (yeni, `tools/builtin_sessionupdate.go`): altı ayrı aracı
  birleştirir — `set_session_title` / `set_working_dir` / `archive_session` /
  `set_session_goal` / `complete_goal` / `set_session_tags` **kaldırıldı**. Tek
  çağrıda title/working_dir/goal/goal_done/tags(add,remove veya replace)/archive
  alanlarından verilenleri uygular; mutasyondan önce hepsini doğrular (yarım
  güncelleme yok). `sessionFrom` (SessionSink ⊇ GoalSink) tek sink'ten okur →
  CLI köprüsünde tek `sessionAttach`. Görünürlük: name-only.
* **`secret`** (birleşik, `tools/builtin_secret.go`): `secret_list`/`_get`/`_set`/
  `_delete` **kaldırıldı** → tek araç `action: list|get|set|delete`. `builtin_secretmgmt.go`
  silindi; artık yalnız `buildRegistry`'de vault varken kayıtlı (hidden). RiskWrite
  (eskiden de reads RiskWrite idi → regresyon yok).
* **`shell_manage`** (birleşik, `tools/builtin_shell_bg.go`): `shell_output`/`_kill`/
  `_list` **kaldırıldı** → tek araç `action: output|kill|list`. Görünürlük: name-only.
* **Tag editörleri folded**: `set_flow_tags`/`set_schedule_tags` **kaldırıldı** →
  `update_flow`/`update_schedule` artık `tags` alanı alıyor (ayrı `SetFlowTags`/
  `SetScheduleTags` ile persist, `SetScheduleEnabled` deseni gibi). Session tag'leri
  `update_session`'da.
* Wiring: `toolsetup.go`+`toolsetup_selfmanage.go` (kayıt+MarkNameOnly/MarkHidden),
  `mcp_interaction.go` (spec+sinkToolTable), `categories.go`, frontend `toolIcons.ts`,
  `default-instructions.md`. Tüm testler yeşil (650 passed / 34 paket). Detay:
  `_Docs\24-SELF-MANAGEMENT.md`.
* **Gömülü default skiller güncellendi (2026-07-04, ayrı tur):** 3 skill (`tionharness-guide`,
  `tionharness-progress`, `tionharness-self-management`) yeni araç isimlerine (`update_session`,
  `secret`) göre düzeltildi. **Bulgu:** `skills.EnsureDefaults` diske seed ederken
  **mevcut dosyanın üzerine yazmıyor** → önceden çalışmış kurulumlarda global skills
  dizini (`~/.tionharness/skills`, tüm workspace'ler paylaşır) **genel olarak bayat**
  kalmış (11 default skill'in hepsi farklı: self-management 361, settings 306, guide
  303 satır). On-disk kopyalar güncel gömülü içerikle **elle senkronlandı** (yedek:
  `~/.tionharness/skills-backup-20260704-preconsolidation`; `otonom-dispatch` gibi kullanıcı
  skill'lerine dokunulmadı). **Açık gap:** `EnsureDefaults` sürüm-farkında değil →
  ileride gömülü skill güncellemeleri mevcut kurulumlara otomatik yansımıyor; içerik-hash
  ile "kullanıcı düzenlememişse tazele" mantığı eklenebilir (ileride).

## Mobil/dikey ekran uyumu — F2 (modallar + header) ✅ (2026-07-04)

Modallar portrait telefonda **bottom-sheet**; header taşması giderildi. Masaüstü
birebir korunur (mobil sınıflar `max-md:`/`md:hidden` altında). `tsc -b && vite
build` temiz.

- **`common/ModalOverlay.tsx` (tek kaldıraç → 13 modal):** `< md`'de `items-end` +
  `p-0` + `max-md:[&>*]:!w-full !max-w-none !max-h-[92dvh] !rounded-b-none` → her
  modal tam-genişlik, düz-alt-köşe, 92dvh iç-scrolllu sheet. `!important` çocuğun
  sabit genişlik/yuvarlamasını ezer.
- **Elle yazılmış overlay'ler:** `ArtifactPreviewModal` (48-F2 önizleme ile
  örtüşür) + `RewindDialog` aynı desene; `PromptEditor` tam-ekran editör mobilde
  kenardan-kenara (`p-0` + çocuk `!rounded-none !max-w-none`).
- **Header (`App.tsx`):** `max-md:px-3`; sol grup `min-w-0` + ajan adı `truncate`;
  **ChatMeters `hidden md:flex`** (mobilde gizli). Detay: `_Docs\49` §7.2.

## Mobil/dikey ekran uyumu — F1 (shell + sohbet) ✅ (2026-07-04)

UI artık portrait telefonda (`< md` = 768px altı) kullanılabilir. Masaüstü
düzeni birebir korunur (tüm mobil sınıflar `max-md:`/`md:hidden` altında).

- **Yeni:** `hooks/useMediaQuery.ts` (`useIsMobile`, `max-width:767px`) +
  `components/MobileNavBar.tsx` — altta `fixed bottom-0` **yatay-kaydırılabilir**
  nav bar; tüm view'lar + Workspace/Ayarlar tek şeritte (taşanlar scroll ile),
  busy/unread/dirty noktaları, safe-area padding, `md:hidden`.
- **NavRail:** `NAV` export edildi (tek kaynak → mobil bar da tüketir); kök
  `hidden md:flex` (mobilde gizli).
- **App.tsx:** chat header'ında mobil hamburger → **SessionsSidebar** soldan
  slide-in drawer (backdrop + `translate-x`, seçimde kapanır); **SessionDetailPanel**
  sağdan slide-in drawer (`detailOpen` sürer); `<main>` `max-md:pb-16`; `<MobileNavBar>`
  render.
- Navigasyon deseni **kararlaştı**: 4-tab+drawer hibriti yerine tam yatay-scroll bar.
- `tsc -b && vite build` temiz. Kalan: F2 modallar (full-screen sheet) · F3 liste
  panelleri · F4 grafik/canvas · mobil workspace switcher. Detay: `_Docs\49` §7.1.

## Chat: worker task-notification'a özel katlanabilir kart ✅ (2026-07-04)

`Origin=worker-note` mesajlar (koordinatöre enjekte edilen `<task-notification>`
blokları) artık ham XML yerine özel bir kartla çiziliyor: yeni
`frontend/src/components/chat/TaskNotificationNote.tsx` + `MessageList`'te
worker-note dalı. Kart başlığı worker ajan adı, oturum id ve durum rozeti
(tamamlandı yeşil / başarısız kırmızı / durduruldu sarı), altında araç sayısı +
süre; result gövdesi varsayılan **katlı**, tıklayınca açılır. Parse edilemeyen
format ham metniyle katlı gösterilir (sessizce gizleme yok); DB metni değişmez.
SES104'te canlı doğrulandı (`_Docs/gorseller/coord-06-notification-card.png`),
`npx tsc --noEmit` temiz. Detay: 47 §10.

## Bağlam önizleme modalları: katlanabilir bölümler + ayrık Skills segmenti + tümünü aç/kapat ✅ (2026-07-04)

Bağlam önizleme pencerelerindeki (Oturum + Ajan) tüm bağlam segmentleri artık
tek tek **fold in/out** (katla/aç) edilebilir; ayrıca **Skills** kendi segmenti
olarak sistem promptundan ayrıldı ve header'a **Tümünü aç / Tümünü kapat**
eklendi. Canlı doğrulandı (Playwright, WS2/AGT9 + WS1/SES89).

- **Yeni primitif `common/CollapsibleSection.tsx`:** chevron + başlık gövdeyi
  açar/kapar, opsiyonel `right` node (cache etiketi, sayaç, Markdown/Ham geçişi)
  toggle düğmesinin DIŞINDA kalır (buton-içinde-buton geçersiz HTML'den kaçınır —
  kendi kontrolleri tıklanabilir), vars. açık. Yanında `useBulkToggle` hook'u +
  `BulkToggle` tipi: `{all,nonce}` sinyalini bump'layıp tüm abone bölümleri aynı
  anda açar/kapar (sonra tek tek toggle serbest); `useEffect([nonce])` ile senkron.
- **Backend — Skills segment ayrımı (`session_context.go` + `agent_context.go`):**
  önizleme yanıtına `skills`/`skillsTokens` alanları eklendi.
  `SkillsCatalogBlockForAgent(agent)` bloğu composeTurnRequest/
  buildAgentStaticPrompt'un ürettiği sistem promptundan `stripBlock` yardımcısıyla
  (tek verbatim occurrence + ayraç temizliği) çıkarılıp ayrı alana taşınır; toplam
  token korunur (sys+skills+dyn+msg+tools). Blok bulunamazsa duplikasyon yerine
  skills boş bırakılır. `List()/SharedList()` `s.order` slice tabanlı → deterministik,
  recompute-strip güvenli. Canlı: AGT9 systemTokens 11762→10689 + skillsTokens 973,
  `systemHasSkillsHeader=false`.
- **`SessionContextModal`:** bölümler katlanabilir + **bölüm sırası** (kullanıcı
  isteği, 2026-07-04): **Mesaj dizisi (modele gidecek)** en üstte → **Dinamik
  bağlam** → Sistem promptu → **Skills** (varsa) → Cache'li mesaj dizisi → Araçlar
  (değişken/model-bağlı içerik üstte, stabil cache'li prefix altta). Mesaj dizisi
  ikiye bölündü — cache öneki varsa **"Cache'li mesaj dizisi (sıcak önek)"** ayrı
  grup (vars. KAPALI, artık alt tarafta) + **"Mesaj dizisi (taze)"**; mesaj kartı
  `MessageCard`'a çıkarıldı, eski inline cache-sınırı çizgisi kaldırıldı. `Section`
  helper'ı `CollapsibleSection` sarar + `bulk` iletir. Token chip'e Skills +
  copy()'ye `# Skills` bölümü eklendi.
- **`AgentContextModal`:** bölüm sırası (kullanıcı isteği, 2026-07-04): **Dinamik
  bağlam** en üstte → Sistem promptu (Markdown/Ham `right`'ta) → **Skills** (varsa,
  Markdown/Ham'a saygılı) → lazy araçlar; hepsi katlanabilir + Skills token chip.
- Doğrulama: `go build ./...` + `go test ./internal/api/...` (73) + `npx tsc
--noEmit` temiz. Canlı: tek-toggle (aria-expanded true→false, 5→4), Tümünü
  kapat→0, Tümünü aç→geri; Skills bölümü AGT9 modalında chip 973 + başlık render.
- Not: kullanıcının 8090'daki backend'i (dün başlatılmış eski binary) bu
  değişiklikleri içermez → yeni davranış için backend yeniden başlatılmalı
  (`.\scripts\dev.ps1`).
- **Doğruluk uyarıları (`SessionContextModal`, 2026-07-04):** önizlemenin gerçek
  wire-payload'dan bilinçli saptığı yerler için yeni `HintNote` (soft-amber callout):
  (1) Mesaj dizisi başlığında — önizleme **bu-turun compaction'ını uygulamaz**
  (yan-etkisiz; bütçeye yakın oturumda modele gerçekte gidenden fazla mesaj
  görünebilir); (2) claude-cli'da (`data.cliOverhead != null`) Dinamik bölümünde —
  dinamik ayrı system bloğu değil **son kullanıcı mesajına dokunularak** gider;
  (3) claude-cli'da Araçlar bölümünde — araçlar TionHarness isteğinde şema olarak değil
  **CLI built-in + MCP köprüsüyle** iletilir, token yaklaşık. `npx tsc --noEmit` temiz.
- **Provider alanı + "Compaction'ı simüle et" toggle + AgentContextModal cli notu
  (2026-07-04):** her iki önizleme yanıtına `provider` alanı eklendi
  (`session_context.go`/`agent_context.go` → `agent.Provider`); `AgentContextModal`
  Dinamik bölümüne de #3 notu (`data.provider === 'claude-cli'`) taşındı.
  **Compaction simülasyonu:** yeni yan-etkisiz `conversation.Manager.SimulateCompaction`
  (Prepare'ın ön yarısı — aynı bütçe matematiği + `foldBoundary`, ama **LLM summarize
  YOK, persist YOK**) → API `?compact=1` (`compactionSimulated`/`foldedCount` alanları,
  `strconv.ParseBool`). UI'da input satırında **"Compaction simüle/açık"** toggle
  (`FoldVertical`), açıkken mesaj dizisi bu-turun katlamasını yansıtır ve #1 notu
  duruma göre değişir (kaç mesaj katlanırdı / katlanacak yok). `load(msg, compact)` +
  `simulate` state; `useEffect([simulate])` toggle'da anında refetch.
- **Canlı doğrulandı (Playwright, WS1, kendi backend 8095 + Vite 5174 → 8095):**
  SES89 (claude-cli) modalında üç not da render (`compactionOff`/`cliDinamik`/`cliAraclar`
  = true), toggle → "Compaction açık" + "simülasyon açık" + "katlanacak mesaj yok"
  (foldedCount 0, oturum küçük). AgentContextModal Holly (AGT8, claude-cli): Dinamik
  en üstte + cli notu true. Negatif: Minimax3 (minimax-anthropic) → cli notu gizli.
  Backend `go test ./internal/api/... ./internal/conversation/...` (94) + `tsc` temiz.
  Görseller: `session-ctx-cli-notes-compaction.png`, `agent-ctx-cli-dynamic-note.png`.
  Doğrulama sonrası verify-backend/Vite kapatıldı, `vite.config.ts` 8090'a geri alındı.
- **Katlanmış mesajlar ayrı grup + Özet kategorisi + varsayılan tutarlılık fix'i
  (2026-07-04):** `SessionContextModal` artık `/compact` sonrası gerçekle tutarlı.
  **Backend (`session_context.go`):** önizleme mesaj dizisi **her zaman** kalıcı özet
  sınırını (`session.SummaryMsgCount`) uygular → `liveHistory = history[start:]`
  (gerçekten gönderilen) ve `droppedHistory = history[:start]` (özete katlanmış, artık
  gönderilmeyen) ayrılır; `?compact=1` ile bu-turun ek bütçe katlaması da düşülür.
  Rolling summary `conversationSummaryBlock` `stripBlock` ile Dinamik'ten çıkarılıp
  ayrı `summary`/`summaryTokens` alanına taşınır (Dinamik'te çift sayım yok). Yeni
  alanlar: `summary`/`summaryTokens` (toplama DAHİL — katlananların yerine geçer),
  `droppedMessages`/`droppedTokens` (toplama DAHİL DEĞİL — wire'da yok). `authorsFor`
  helper'ı iki dilime de yazar rozeti verir.
  **UI:** yeni **"Özet (katlanmış mesajların yerine geçer)"** katlanabilir bölümü +
  **"Artık gönderilmeyen (özete katlanmış)"** açık-turuncu grup (vars. KAPALI);
  `MessageCard`'a `dropped` varyantı (turuncu border/bg + `DroppedTag "katlandı ·
gönderilmiyor"`), `Stat`'a `dropped` (turuncu chip), token chip'lerine Özet +
  Katlanmış, `copy()`'ye `# Summary (folded)`. **Bulk fix:** `CollapsibleSection`
  mount'ta (nonce değişmeden) artık `bulk`'u uygulamıyor (`seenNonce` ref) →
  `defaultOpen` korunuyor (dropped/cache grupları kapalı açılır), Tümünü aç/kapat
  hâlâ çalışıyor.
  **Canlı doğrulandı (Playwright, WS2/SES2, özetli oturum SummaryMsgCount=20):**
  20 canlı + 20 katlanmış mesaj, Özet 1130 tok (Dinamik'te yok), Katlanmış 3691 tok
  (toplama dahil değil), turuncu grup vars. kapalı → açınca 20 turuncu kart +
  conversation_search ipucu; Tümünü kapat→0/aç→7. `go test` (94) + `tsc` temiz.
  Görsel: `session-ctx-dropped-summary.png`.

## M2 koordinatör/worker: LLM-in-the-loop canlı görsel deneme ✅ + non-stream CLI köprü fix'i (2026-07-03)

`_Docs/47` §10: gerçek modelle (claude-cli/opus) koordinatör oturumu (WS5/SES104)
uçtan uca doğrulandı — `spawn_worker` ×2 tek turda, koordinatör turu bloklanmadan
bitti; Koordinasyon roster'ı UI'da canlı doldu (ÇALIŞIYOR→BITTI, 3 sn poll);
`<task-notification>`'lar otomatik koordinatör turlarını tetikledi ve sentez
yazıldı (çift tur yok). Görseller: `_Docs/gorseller/coord-0*.png`.

- **Bulgu+fix:** non-stream `/api/chat` + claude-cli turunda Interaction MCP hiç
  kurulmuyordu (`interaction=false`) → köprü araçları (spawn_worker dahil) yok ve
  CLI'nin native `Agent`/`Task`'ı disallow edilmiyordu; model kendi Agent'ıyla
  fan-out yapıp M2'yi bypass etti. `toolloop.go` on-demand `autoInteract`
  kurulumundaki `autonomous` şartı kaldırıldı (endpoint'siz her CLI turu sarılır;
  stream yolu etkilenmez). `go build ./...` + agent/tools 296 test yeşil.
- Not: model, prompt'taki meşru seçenek gereği ilk istekte M1'i (`run_subagent`
  sync) seçebiliyor; denemede async M2, "use spawn_worker (NOT run_subagent)"
  yönlendirmesiyle tetiklendi.

## Otonom turların canlı adım akışı — session-step bus ✅ (2026-07-03)

**Sorun:** Chat turunda ajanın adımları (thinking/tool) canlı görünüyordu; otonom
turlarda (scheduler/spawn/worker/wake/peer) **kalıcı olarak kaydediliyordu** ama
canlı görünmüyordu — çünkü chat'in canlı akışı `chat_stream`'in **isteğe-özel** SSE'si
(`sse("step")`) üzerindendi, otonom yollar ise `onStep=nil` ile
`CompleteWithToolsTraced` çağırıyordu (trace toplanır+persist edilir, ama hiçbir yere
yayınlanmaz). Süreç-geneli `/api/events` bus'ı yalnız kaba bildirim taşıyordu.

**Yapılan — süreç-geneli canlı adım köprüsü:**

- `events.Event`'e `Step json.RawMessage` alanı (opaque TurnStep JSON; events paketi
  agent'ı import etmez — `db.Message.Steps` deseni). `handleEvents` `Type=="session_step"`
  frame'lerini ayrı SSE event adı **`step`** ile yazar (bildirim/badge yolu `notify`
  dinler → step'ler oraya karışmaz).
- `internal/agent/sessionstep.go`: `emitSessionStep`/`EmitSessionStep` +
  `SessionStepEmitter(ctx)` (ctx'te session id yoksa nil → eski yol). `busForwardable`
  yüksek-frekanslı (delta/tool_delta) ve etkileşimli (ask/permission/plan/tombstone —
  yalnız turu **sahiplenen** pencere yanıtlayabilir) adımları eler; kalan anlamlı
  aktiviteyi (thinking/tool/todo/diff/recovery/error/subagent) yayınlar.
- Choke point'ler `CompleteWithToolsTraced`→`CompleteWithToolsStream(..., emitter)`
  ile değişti: `invokeTraced` (scheduler/spawn/peer-fallback) + `wakeTurnRunner`
  (worker/coordinator/wake/peer history-aware). Dönen `steps` slice'ı **değişmedi** →
  persistence birebir aynı; yalnız canlı yayın eklendi.
- `chat_stream` onStep'i de `EmitSessionStep` ile bus'a aynalanır → **çok-pencere**
  senkronu: aynı chat turunu başka pencerede izleyen de canlı görür.

**Frontend:** `subscribeEvents(onEvent, onStep?)` tek EventSource'ta `step` frame'lerini
de dinler. `useChatStream.applyAutoStep` aktif oturum için bir **ghost asistan balonu**
(`live-auto-<sid>`) büyütür (thinking merge + tombstone); turu bu pencere sahipleniyorsa
(`runsRef`) atlar (yerel SSE zaten render eder → çift balon yok), ekran-dışı oturumda
yalnız "düşünüyor" göstergesi. Tur bitince tamamlanma event'i (`chat`/`spawned`/`worker`/
`schedule`) ghost'u temizler + transcript'i reload eder → yetkili kalıcı mesaj yerine
geçer. `go build`+vet+agent testleri yeşil, frontend `tsc --noEmit` temiz.

**Ek — tur ortasında UI yenilenince adım/agent kaybı düzeltildi:** Yenileme
sırasında tur sunucuda detached sürüyor ama taze sayfa yalnız **kalıcı** mesajları
yüklüyordu → o ana kadarki adımlar + agent adı kayboluyor, yalnız son cevapla geri
geliyordu (asistan mesajı yalnız tur bitince persist edilir). `inflight.json` sidecar'ı
(partial text+steps+agentId, throttle'lı yazılır) zaten vardı ama yalnız **boot**'ta
crash kurtarma için okunuyordu (`recoverInflight`); canlı yenilemeye açık değildi.
Eklendi: `db.ReadInflight` (exported) + `GET /api/sessions/{id}/inflight` (snapshot
veya null). Frontend: mesaj-yükleme effect'i `chat.recoverInflight(sid, msgs)` çağırır →
snapshot varsa (ve henüz persist edilmemişse) `id=messageId` ghost balonu seed eder
(agent adı + o ana kadarki adımlar geri gelir); session-step bus'ı bu balonu **canlı
büyütmeye devam eder**, tur bitince aynı id'li kalıcı mesaj yerine geçer. Turu bu
pencere sahipleniyorsa (yerel SSE) no-op. `chatRef` (App'te chat hook'una canlı handle)
mesaj-yükleme effect'i chat tanımından ÖNCE geldiği için deps-dizisi TDZ'sini atlar.
`go build`+db+api testleri yeşil, `tsc --noEmit` temiz.

## Workspace default promptu TionHarness-native yeniden yazıldı ✅ (2026-07-03)

`internal/workspace/defaults/default-instructions.md` hâlâ başka bir ajan ürününden
devralınmış yönergelerin mekanik ad-değiştirilmiş uyarlamasıydı — TionHarness'te **olmayan**
onlarca yeteneği öğretiyor (`datatable`/`spreadsheet`, `html/pdf/markdown-preview`,
`render_template`, `call_llm`, `~/.external-agent/docs/*`, `_displayName` MCP meta,
External Sources+`guide.md` modeli), **gerçek** yüzeyi (run_subagent, use_skill,
set_session_goal, flows/self-management/handoff/plan modu, gerçek render seti) hiç
anlatmıyordu. Ayrıca ajana talimat olmayan devralınmış iç dokümantasyon (Dynamic
context / Complete user message / SDK config bölümleri + mini-agent promptu) ve
makineye özel sızıntı (gömülü kişisel tercihler + sabit `C:/Users/<user>/...` yolları)
içeriyordu.

**Yapılan:** dosya sıfırdan TionHarness-native olarak yeniden yazıldı (~750 → ~150
satır). Tasarım ilkesi the external agent project'ın "her şeyi inline et" (~37K token) yaklaşımı
yerine TionHarness'in **küçük cache'li prefix + skill'e devret** felsefesi (`_Docs/17`,
`_Docs/19`): skill kataloğu + `GoalUsageHint` zaten prefix'te enjekte edildiği için
prompt artık ansiklopedi değil, doğru araç yüzeyi + skill pointer'ları. İçerik
İngilizce (kod/prompt kuralı). Render fence'leri gerçek koda göre doğrulandı
(`frontend/.../CodeBlock.tsx`: yalnız `diff`/`mermaid`/`gallery`+`image-preview`).
Document Tools bölümü **kullanıcı kararıyla korundu** ("ileride eklenecek" notuyla).

**Regresyon kilidi:** `internal/workspace/defaults_test.go` —
`TestDefaultInstructionsAreTionHarnessNative` embed'in yasak the external agent project-ism string'leri
(`datatable`/`call_llm`/`render_template`/`~/.external-agent/docs`/`_displayName`/
`html-preview`…) içermemesini ve gerçek TionHarness terimlerini (`run_subagent`/
`use_skill`/`set_session_goal`/`mermaid`) içermesini garanti eder. Enjeksiyon yolu
`TestSystemPromptInjectsWorkspaceInstructions` ile zaten kilitli. `go build ./...` +
118 test (agent+workspace) yeşil. Not: yalnız **yeni** workspace'leri etkiler;
persisted `instructions` taşıyan mevcut workspace'ler seed'i override eder.

## Otomasyon UX: nav rename + inline edit + opsiyonel son tarih ✅ (2026-07-03)

`_Docs/46` devamı. (1) **NavRail "Zamanlamalar" → "Otomasyon"** (`NavRail.tsx` +
`App.tsx` başlık; ekran cron Zamanlamalar + Otomasyonlar'ı birlikte tutar). (2)
**Otomasyonlara inline düzenleme** (`Automations.tsx` kalem butonu → ad/tetik/hedef/
maks-iter/bekleme/son-tarih/prompt + ℹ️ değişken popover; `updateAutomation` API zaten
vardı). (3) **Opsiyonel son tarih** `Automation.ExpiresAt` (unix sn): `fire` başında
`time.Now >= ExpiresAt` → otomatik pasifle (Schedule `expiresAt` deseninin eşi);
create+update API + `create/update_automation` tool + UI datetime-local. Canlı
doğrulandı (create round-trip expiresAt saklandı; PUT edit name/expiresAt/cooldown
güncelledi). `go build ./...` + 406 test + tsc yeşil. **Ayrıca** WS5'te gerçek
**otomatik-onarım otomasyonu** kuruldu+test edildi (AGT24 Repairer sonnet, triggerTag
`tool-error`, spawnTags `["repair"]` loop-kırıcı): induced tool-error → Repairer spawn →
"false positive, no changes" doğru teşhis.

## Araç backlog P2 dalgası: `get_session_info` + `update_user_preferences` ✅ / labels-status ❌ kapsam dışı (2026-07-03)

`_Docs/41` madde 5-6-7 kapatıldı (kullanıcı kararı: 6'yı yapma, 5+7'yi yap):

- **`get_session_info` (YENİ, `tools/builtin_sessioninfo.go`):** ajan kendi oturumunun
  metadata'sını okur — id/title/state/kind/agent(ad+id)/mesaj sayısı/tags/goal/
  working_dir/role/coordinator_session/parent_session; `session_id?` ile başka oturum.
  Ctx'teki mevcut oturum (`CurrentSessionID`) default; oturumsuz turda zarif mesaj.
  Session-edit araçlarının (title/tags/goal) okuma eşi. Koşulsuz kayıt, `MarkNameOnly`
  tier, `RiskRead`, kategori `agents`; claude-cli köprüsü `BridgeTools` `extra`.
- **`update_user_preferences` (YENİ, `tools/builtin_userprefs.go`):** kullanıcıdan
  öğrenilen kalıcı bilgileri (ad/saat dilimi/şehir/ülke/tercih notları) mevcut
  **Settings ▸ Profil** alanlarına yazar (`SettingsBridge.Apply`, yalnız 5 profil
  alanı — dar sarmalayıcı). `notes` REPLACE / `notes_append` satır ekler (ikisi
  birlikte → hata; append Snapshot'tan mevcut notu okur). Profil zaten her turda
  "About the user" bloğu olarak enjekte → yeni prompt katmanı gerekmedi. Bridge
  varken kayıt, `MarkNameOnly`, `RiskWrite` (default), kategori `config`; CLI köprülü.
- **`set_session_labels`/`set_session_status` KAPSAM DIŞI:** etiketleri
  `set_session_tags` + etiket-otomasyonları (`_Docs/46`) zaten karşılıyor; durum için
  `State`+`archive_session`+Kanban yeterli. `_Docs/41` §6 gerekçesiyle işaretlendi.
- **Test:** `builtin_sessioninfo_test.go` (explicit id / ctx default / no-session /
  unknown id) + `builtin_userprefs_test.go` (alan patch, append/replace, guard'lar,
  nil bridge). `go build ./...` + tools+agent 289 test yeşil.

## Kaydedilmemiş-değişiklik belirteci: agents/artifacts kapsam + sayfa-değiştirme uyarısı ✅ (2026-07-03)

Ayarlar/Workspace/Flows ekranlarında zaten var olan "kaydedilmemiş değişiklik"
(dirty) nav belirteci **Ajanlar** ve **Artifactlar** ekranlarına da genişletildi;
ayrıca kirli bir ekrandan ayrılmaya çalışınca uyarı gösterilir.

- **Ortak altyapı (`lib/dirtySignals.ts`):** `useRegisterDirty(view, isDirty)` artık
  `view: View | undefined` kabul eder (undefined → no-op). Böylece aynı editör bir
  modalda tekrar kullanıldığında nav'ı yanlış ekrandan kirletmez.
- **Ajanlar (`agents/AgentSettingsForm.tsx`):** form alanları (name/avatar/color/soul/
  identity/provider/model/thinkingLevel/permissionMode/skills) ajanın kalıcı değerleriyle
  karşılaştırılıp `dirty` hesaplanır; yeni opt-in `dirtyView?: View` prop'u ile
  `AgentsView` `dirtyView="agents"` geçer (modal reuse geçmez). Tools bölümü anında
  kaydettiği için dirty'e dahil değil.
- **Artifactlar (`panels/ArtifactsPanel.tsx`):** açık `draft` kalıcı artifact'tan
  (title/kind/language/content) farklıysa `useRegisterDirty('artifacts', dirty)`.
- **Sayfa-değiştirme uyarısı (`App.tsx`):** `selectView` guard'ı — mevcut ekran dirty
  iken başka nav view'ine geçişte `window.confirm` onayı ister (NavRail `onSelectView`
  artık `selectView`). Ayrıca herhangi bir ekran dirty iken sekme kapatma/yenilemede
  `beforeunload` tarayıcı uyarısı. Not: workspace switch bu guard'ın dışında (kapsam
  yalnız nav view değişimi).

## Araç boşluk kapatma: arka-plan shell + apply_patch + CLI tool latency + built-in hook görünürlüğü ✅ (2026-07-03)

claude-cli built-in araç yüzeyi ile TionHarness native araçları arasındaki boşlukların
kapatılması (the external agent project↔TionHarness backlog `_Docs/41`).

- **Arka-plan / uzun-süren shell (`BashOutput`/`KillShell` paritesi):** `Bash`/`PowerShell`
  araçlarına `run_in_background` argümanı → detached süreç başlatıp shell id döner
  (`internal/tools/builtin_shell_bg.go`: `ShellManager` süreç kayıt defteri + `bgWriter`
  rolling ring 256KB + offset-takipli **destructive drain**; `bgShellMaxLive=16`,
  `bgShellKeepDone=16` budama). Üç yönetim aracı: `shell_output` (son okumadan beri YENİ
  çıktı + durum satırı; ring taşarsa "rolled off" uyarısı), `shell_kill`, `shell_list`
  (name-only tier). Yönetici **session-scoped** (`Runtime.shellMgrs sync.Map`,
  `shellMgrFor`), turlar arası yaşar; catalog/preview build'de nil → arka-plan devre dışı.
  Shell tool'lar `WithManager` ile bağlanır (value-type copy; eski call-site'lar
  değişmedi). Confined-git brake arka-planda da geçerli. Dev server/watcher senaryosu.
- **`apply_patch` (unified-diff, çok-hunk/çok-dosya):** `Edit`'in batch kardeşi
  (`internal/tools/builtin_patch.go`). `diff -u`/`git diff` çıktısını **context
  eşleştirmeyle** uygular (@@ satır no'ları ipucu, güvenilmez) → alakasız edit'ler satırı
  kaydırsa bile tutar; hunk eşleşmezse **o dosyanın tamamı reddedilir** (yarım uygulama
  yok). **İki-fazlı** (tüm dosyalar önce validate+freshness, sonra commit) → dosyalar
  arası all-or-nothing. `--- /dev/null` create, `+++ /dev/null` delete; `a/`,`b/` prefix
  ve `diff -u` tab-timestamp temizlenir. Freshness guard entegre (read-only ajanda lazy).
- **CLI tool latency → debug.jsonl (#3):** `emitCLIToolDebug` zaten tool olaylarını
  besliyordu ama `DurMs=0` idi. `cliStreamParser`'a `toolStart map[id]time.Time` eklendi:
  `tool_use` görülünce saat başlar, `tool_result` gelince `TraceStep.DurMs` = gerçek
  wall-clock (stream satır-satır `ReadString` ile real-time beslendiği için CLI-içi araç
  gecikmesi doğru). `provider.TraceStep.DurMs` yeni alan; native/CLI arası tek-tip tool
  metriği tamamlandı.
- **Built-in/auto-injected hook görünürlüğü (salt-okunur):** yeni `GET /api/hooks/builtins`
  (`api/hooks.go handleListBuiltinHooks`) app-settings snapshot'ından hesaplanan 8 yerleşik
  davranışı döner (freshness guard, CLI native-tool bridging, Bash→PowerShell, CLI hook
  passthrough, permission deny-list, plan-mode approval, autonomous git brake, hook
  fail-open) — `enabled` toggle'lanabilirler için `FileFreshnessGuard`/`EnableShell`/
  `EnableCLIHooks`/`AutonomousConfine`'dan. Frontend: `HooksPanel.tsx` altına dashed-border
  read-only bölüm (scope/event badge + ayar anahtarı + Aktif/Pasif); `types/hook.ts
BuiltinHook` + `api/hooks.ts listBuiltinHooks`.
- **#2 (freshness-guard'ı CLI yoluna taşı) YAPILMADI — bilinçli:** CLI'nin **native**
  Read/Edit/Write'ı zaten kendi read-before-write guard'ını uyguluyor (TionHarness'in
  `readtracker.go`'su bunu "mirrors Claude Code's readFileState guard" diye kopyaladı).
  Hook tabanlı ikinci guard redundant + kırılgan olurdu (hook executor'ı değiştiremez,
  yalnız deny/observe/updatedInput; `updatedInput` Edit'te bug'lı #47853).
- **Test/derleme:** `internal/tools/builtin_patch_test.go` (update/create/delete/mismatch/
  freshness/multi-hunk), `builtin_shell_bg_test.go` (ring drain/overflow, unknown-id, nil
  manager, gerçek arka-plan echo). `go build ./...` + 430 test (4 paket) + `tsc --noEmit` yeşil.

## Etiket-Otomasyon: genişletilmiş değişkenler + info popover + olay-bazlı otomatik etiketleme ✅ (2026-07-03)

`_Docs/46`'nın devamı (etiket + otomasyon çekirdeği 2026-07-02).

- **PromptTemplate değişkenleri 4→13:** `renderAutomationPrompt` + yeni `turnVars`
  (`agent/automation.go`) → {{result}}/{{title}}/{{tag}}/{{sessionId}}/{{iteration}}/
  {{maxIterations}}/{{agent}}(+{{agentName}})/{{prevPrompt}}/{{automation}}/{{date}}/
  {{time}}/{{datetime}}. Bilinmeyen `{{...}}` aynen kalır; {{result}} yoksa sona eklenir.
- **UI info popover:** `panels/Automations.tsx` prompt alanına ℹ️ butonu → 13 değişkeni
  açıklamalı listeler; satıra tıkla → şablona ekler (`PROMPT_VARS`).
- **Olay-bazlı otomatik etiketleme** (yeni `agent/autotag.go`): tur olaylarına göre
  well-known etiketler (ADD-only): `tool-error` (gerçek tool hatası; claude-cli
  **disallowed-tool** reddi `isPermissionDenyError` ile HARİÇ), `error` (tur-seviyesi
  hata, "stopped" hariç), `goal`/`goal-done`/`archived` (durum). `Runtime.AutoTagTurn`
  chat(başarı+cerr)/spawn/schedule/wake yollarında; `archived` ayrıca arşiv mutasyonunda
  (`sessionSink.Archive` + API state handler, geri yüklemede silinir). Amaç: bir
  otomasyonla hataları tarayıp otomatik onarmak. **Ayar toggle'ı** `AutoTagSessions`
  (vars. açık; Ayarlar ▸ Bağlam) → `AutoTagTurn`/`AutoTagEnabled`/`sessionSink.autoTag`
  guard'ları; kapalıyken hiç otomatik etiket yazılmaz. Canlı: OFF→yazmıyor, ON→yazıyor.
- **Canlı doğrulama (WS2, sonnet/claude-cli):** loop (sayaç 10→11→12, story zinciri,
  2 senaryo paralel, maks-iter'de auto-disable); genişletilmiş değişkenler render;
  autotag: archived ekle/sil, goal, var-olmayan dosya Read → tool-error. Testler:
  `db/automation_test.go`, `agent/automation_test.go`, `agent/autotag_test.go`;
  spawn testlerine `drainSpawns` (fire-and-forget goroutine'i TempDir cleanup'tan önce
  beklet → Windows dosya-kilidi flakiness giderildi). `go build ./...` + 635 test + tsc yeşil.

## Fix: `run_subagent` hedef gölgeleme + sync timeout (WS8/SES1 teşhisinden) ✅ (2026-07-03)

**Belirti:** Superpowers pipeline'ında (Orchestrator=claude-cli) `run_subagent`
"Reviewer"/"Verifier" fazlarında tutarlı başarısız: async → `async subagents require
an existing agent target, not a profile`; sync → `The operation timed out.`

**Kök neden 1 (gölgeleme):** `resolveSubagentTarget` önce built-in profillere
(`explore`/`coder`/`reviewer`) bakıyordu → kullanıcının gerçek "Reviewer" (AGT6) ajanı
`reviewer` profiliyle gölgelenip ephemeral çözülüyordu; ephemeral async'i reddettiği için
hata. **Fix:** önce mevcut ajana bak, bulamazsa profile düş (gerçek ajan kazanır).
Async+ephemeral reddi provider/bütçe işinden **önce** açıklayıcı mesajla (Guard 4).

**Kök neden 2 (timeout):** sync `run_subagent`'ta alt-ajanın tüm işi tool çağrısında
koşuyor ve claude-cli'nin ~60 sn MCP araç-çağrısı timeout'unu aşıyor → CLI `The operation
timed out.` verir (TionHarness işi arka planda bitirir). **Fix:** `claudecli.go runAttempt`
CLI process'ine `MCP_TOOL_TIMEOUT` + `MCP_TIMEOUT=60000` ms enjekte eder (kullanıcı
override kazanır → `ensureEnvDefault`).

**Güncelleme (2026-08-21):** 600000 ms (10 dk) de pratikte yetmedi — dosya düzenleyip
`go test ./...` koşturan bir alt-ajan rutin olarak aşıyor. `MCP_TOOL_TIMEOUT` **900000 ms
(15 dk)**, codex tarafında `tool_timeout_sec` **900 sn** yapıldı. Tavanı yükseltmenin
maliyeti yok: çağrıyı zaten çağıranın ctx'i (kullanıcı durdurması, tur iptali) sınırlıyor;
bu değer yalnız CANLI bir çağrının yavaşlık gerekçesiyle öldürüleceği anı belirler.

**Dokunulan:** `internal/agent/subagent.go` (çözümleme sırası + Guard 4), `internal/
providers/claudecli.go` (`ensureEnvDefault` + MCP timeout env), testler
`TestResolveSubagentAgentBeatsProfile`/`TestAsyncProfileRejected`. Doküman `_Docs\25` +
skill `tionharness-session-debug` (desen G/H). `go build`/`go test ./internal/agent
./internal/providers` (186) yeşil.

## Feature: Read/Grep/Glob araç-paritesi — offset/limit + satır no, ripgrep-stili Grep, mtime Glob, .gitignore ✅ (2026-07-03)

**İstek:** Claude Code'un fs araçlarında olup bizde olmayan per-tool özellikler
(`_Docs/41` Bölüm E): Read satır-aralığı + numaralama, Grep output-mode/context/-i/
type/multiline/head_limit, Glob mtime sıralama + path, ve .gitignore farkındalığı.

**Çözüm:**

- **`Read`** (`builtin_fs.go` `renderNumbered`): çıktı artık **satır-numaralı** (`%6d\t…`,
  cat -n stili; Edit için "önek+tab'ı sıyır" notu açıklamaya eklendi). **`offset`**
  (1-tabanlı başlangıç) + **`limit`** (satır sayısı, vars. 2000) ile büyük dosyanın
  penceresi okunur; satır-başı karakter cap'i (2000) + 256KB çıktı cap'i + "devam:
  offset=N" ipuçları. Tazelik hash'i tam içerik üzerinden (pencere kısmi görünüm).
- **`Grep`** (yeni `builtin_grep.go`): `output_mode` (content/files_with_matches/count),
  context `-A`/`-B`/`-C` (bitişik pencereler birleşir, `--` ayraç), `-i`, `-n` (vars.
  açık), `-o` (yalnız eşleşen), `type` (dil→uzantı haritası), `multiline` (`(?s)`),
  `head_limit`, `path` (dosya/dizin), `no_ignore`. Eşleşen satır `path:line:text`,
  bağlam `path-line-text` (ripgrep konvansiyonu).
- **`Glob`** (yeni `builtin_glob.go`): sonuçlar **mtime'a göre** (en yeni önce) sıralı;
  `path` (arama kökü) + `no_ignore` argümanları.
- **`.gitignore` farkındalığı** (yeni `ignore.go` `IgnoreSet`): Grep+Glob kök+iç-içe
  `.gitignore`'ları (lazy) + daima `.git`'i atlar; dizin eşleşince `SkipDir` ile tüm
  alt-ağaç elenir. `*`/`**`/`?`, `!` negasyon, dir-only `/`, anchored `/` desteklenir
  (byte-perfect Git değil; node_modules/.git/dist gürültüsünü keser). `no_ignore` ile
  kapatılır. Yeni bağımlılık yok (ripgrep binary'sine bağlanmadan saf-Go).
- Eski `FSGlobTool`/`FSGrepTool` `builtin_fs.go`'dan yeni dosyalara taşındı;
  `globToRegexp`/`isBinary` paylaşımlı kaldı.
- **Grep `rg` hızlı yolu (opsiyonel, 2026-07-03):** PATH'te `rg` (ripgrep) varsa Grep
  otomatik ona delege eder (`grep_rg.go` `tryRG`) — daha hızlı + native tip/ignore. Bayrak
  eşlemesi: `--no-require-git --hidden` (Go `IgnoreSet` semantiğiyle eşleşir: repo olmadan
  `.gitignore`'a uy + dotfile'ları ara, `.git` daima atlanır), `--path-separator /` (Windows
  `\`→`/` normalizasyon), output_mode→`--no-heading`/`--files-with-matches`/`--count-matches`,
  `-A/-B/-C`, `--ignore-case`, `--only-matching`, `--multiline --multiline-dotall`,
  `--glob`, tip→uzantı-glob'ları, `--no-ignore`, `--regexp` (dash-güvenli). Çıktı Go
  motoruyla **aynı şekle** normalize edilir (lider `./` sıyrılır, head-limit uygulanır).
  **Herhangi bir belirsizlikte** (rg yok / bilinmeyen mode/tip / rg exit≠0/1 / timeout)
  sessizce **Go motoruna düşer** → davranış her iki yolda birebir. `TIONHARNESS_GREP_NO_RG=1`
  ile kapatılır. rgExe env kontrolü `sync.Once` DIŞINDA (test-toggle edilebilir; LookPath cache'li).
- **Edit satır-no toleransı (Read numaralama davranış değişikliği için):** Read çıktısı artık
  `<no>\t<içerik>` numaralı; model bazen bu öneki `old_string`'e kopyalar. Doğrudan eşleşme
  0 olduğunda Edit, `old_string` VE `new_string`'den `cat -n` öneklerini (`^ *\d+\t`,
  `stripCatNPrefixes`) sıyırıp yeniden dener — eşleşirse temiz uygular (numaralı paste
  artefaktı dosyaya yazılmaz). Yalnız verbatim eşleşme başarısızsa devreye girer; başarılı
  eşleşmeyi asla değiştirmez.
- **Test:** `TestFSReadWindow`, `TestFSEditToleratesLineNumbers`, `TestGrepOutputModes/
Context/OnlyMatching` (Go motoruna sabitli), `TestGrepRGFastPath` (rg yoksa skip),
  `TestGlobMtimeSortAndIgnore/PathArg`; mevcut Read-eşitlik testleri `Contains`'e
  güncellendi. `go build`/`vet`/`test` (366) yeşil. (`run_in_background` paralel bir çalışmada
  `builtin_shell_bg.go` `ShellManager` + `shell_output`/`shell_kill`/`shell_list` ile
  ayrıca eklendi — tazelik guard'ı eşzamanlı düzenlemede duplikat yazımı engelledi.)

## Feature: Dosya tazelik guard'ı — Edit/Write "read-before-write" (Claude Code paritesi) ✅ (2026-07-03)

**İstek:** Claude Code'un Edit toolundaki _"File has been modified since read… Read
it again before attempting to write it"_ tespiti bizde yoktu; ekleyelim.

**Sorun:** TionHarness'in `Edit`/`Write` araçları önceki bir `Read`'i takip etmiyordu →
bir oturum dosyayı okuduktan sonra dosya dışarıdan (kullanıcı/linter/başka tool)
değişse bile edit **sessizce üzerine yazıyordu** (stale-write footgun).

**Çözüm:** Session-scoped **`ReadTracker`** (içerik-hash tabanlı tazelik temeli):

- **`internal/tools/readtracker.go`** (yeni): `ReadRecord{ModTime,Size,Sum(sha256),
Partial}` + concurrency-safe `ReadTracker` (nil = no-op, guard kapalı). `Read`
  aracı okuma anında (truncation ÖNCESİ, tam içerik hash'i → >256KB dosyalar da
  kapsanır) temeli kaydeder. `checkFreshness` = kayıt yok → _"file has not been read
  yet"_, içerik hash'i uyuşmuyor → _"file has been modified since it was last read"_.
  Karşılaştırma mtime değil **içerik-hash** (cloud-sync/AV kaynaklı sahte mtime
  bump'larında yanlış-pozitif yok, mtime değişmeyen gerçek edit'i de yakalar).
- **`builtin_fs.go`:** `Read` kaydeder; `Edit` mutasyondan önce `checkFreshness`;
  `Write` yalnız **var-olan** dosyada guard (yeni dosya prior-read istemez); ikisi de
  yazımdan sonra temeli yeni içeriğe tazeler (`recordWritten`) → aynı dosyada edit
  zinciri araya Read istemez. Tool açıklamalarına "önce Read" notu eklendi.
- **Ayar/gating:** `Tunables.fileFreshnessGuard` (default **açık**) + `settings.
FileFreshnessGuard` (Default/public/patch/store + `server.go applySettings`); default
  skill `tionharness-settings`'e belgelendi. `buildRegistry` session-id'yi (`SessionIDFrom`)
  çözüp `Runtime.readTrackers` (sync.Map, session-başına kalıcı) üzerinden tracker'ı
  3 fs tool'a bağlar; katalog/preview build'lerinde (session yok) veya ayar kapalıysa
  **nil** → guard devre dışı. Yalnız **native** yol; claude-cli'nin kendi karşılığı var.
- **Test:** `TestFSFreshnessGuard` (edit-before-read / write-before-read / after-read
  ok / external-change → stale / new-file-ok / nil-guard-off) + mevcut `TestFSWriteReadEdit`
  tracker'lı güncellendi. `go build`/`vet`/`test` (273) yeşil.

## Feature: Skiller için toplu "Grup ata" (bulk set-group) ✅ (2026-07-03)

**İstek:** Skilleri toplu seçince topluca gruplarını setleyebilelim.

**Çözüm:** Skills ekranı zaten çoklu-seçim (`useMultiSelect`) + `SelectionBar`
(toplu görünürlük + sil) taşıyordu; buna **toplu grup atama** eklendi.

- **Backend:** `Store.SetGroup(slug, group)` — SKILL.md'nin yalnız `group`
  frontmatter'ını `setFrontmatterFields` ile yeniden yazar (gövdeye/diğer alanlara
  dokunmaz), `category` alias'ını düşürür (ikisi çelişmesin), boş grup → grupsuz.
  API `PUT /api/skills/{slug}/group` (`handleSetSkillGroup`). Diğer `Set*` toggle
  endpoint'leriyle aynı desen.
- **Frontend:** `api.setSkillGroup(slug, group)`; `SkillsPanel` `bulkSetGroup` seçili
  her slug için paralel çağırır (`Promise.all`), sonra listeyi tazeler ve seçimi
  korur (zincirleme aksiyon). `SelectionBar`'a datalist'li (mevcut grup adları öneri)
  grup input'u + "Ata"/"Grupsuz" butonu; Enter da uygular.
- **Test:** `store_test.go TestSetGroup` (ata / category-alias düşür / boş=grupsuz /
  gövde-korunur / eksik-skill hata). `go build`/`test` + `tsc` yeşil.

## Antigravity CLI (`agy`) provider'ı KALDIRILDI ✅ (2026-07-03)

Deneysel `antigravity-cli` provider'ı (2026-06-29'da eklenmişti; tarihsel kayıt
aşağıda) **tamamen kaldırıldı**: upstream non-TTY bug'ı (#76) düzelmedi ve ConPTY
workaround'u istenmedi → ölü deneysel kod taşımak yerine temizlendi. Silinen:
`providers/antigravitycli.go` + `kind_antigravity.go` + live test;
`kind.go`/`registry.go`/`kind_test.go` arındırıldı. Artık **5 provider kind'ı**
(claude-cli/anthropic/minimax/minimax-anthropic/openrouter). Dış-ajan adaptörü
olarak sırada **Codex** (MCP delegasyonlu) duruyor.

## Feature: Çok-Ajan Koordinasyonu — M2 Koordinatör/Worker ✅ (2026-07-03)

**İstek:** Bir ajanın paralelde 4-5 ajanı koordine etmesi (Claude Code'un
koordinatör modu gibi) + farklı koordinasyon yöntemleri.

**Çözüm (M2 koordinatör/worker + M1/M3/M4 birleşik çatı):** Bir oturum
`Role="coordinator"` yapılınca koordinatör sistem promptu + dört araç açılır:
`spawn_worker` (async worker = mevcut ajan hedefli arka-plan oturumu), `send_to_worker`
(yüklü bağlamla devam), `stop_worker` (iptal→killed), `list_workers`.

- **Geri bildirim halkası:** worker turu bitince (`runWorker`, başarı/başarısız/killed)
  sonuç `<task-notification>` olarak koordinatör oturumuna enjekte edilir
  (`NotifyCoordinator`, `Origin="worker-note"`) ve **per-session tur kuyruğu**
  (`coordSlot` + `enqueueCoordinatorTurn`/`drainCoordinator`) bir koordinatör turu
  tetikler. Eşzamanlı bitişler **serileşir**; koordinatör meşgulken biriken
  bildirimler tek turda **coalesce** olur (çift-tur yarışı yok — kritik test yeşil).
- **Recursion engeli:** araçlar context-injection (`tools.WithCoordination`) ile YALNIZ
  koordinatör oturumunda kayıtlı → worker worker spawn edemez.
- **Guard'lar:** `CoordinatorMaxWorkers` (8) + `CoordinatorMaxTurns` (50) +
  `SetCoordinatorLimits`.
- **Session modeli:** `Role` + `CoordinatorSessionID`; worker `Kind="worker"`.
  `turnHook` → çoklu `turnHooks` (`AddTurnHook`), otomasyonu ezmeden.
- **Prompt/skill:** `api/coordinator_prompt.go` (`composeTurnRequest`'te koşullu enjekte,
  wake yolunu da kapsar) + gömülü default skill `tionharness-coordinator`.
- **API:** `session_info`'ya `role`+`coordinatorSessionId`; `PUT /api/sessions/{id}/role`
  - `GET /api/sessions/{id}/workers`.
- **UI:** `CoordinatorSection.tsx` (aç/kapa + canlı worker roster, running varken 3sn
  poll) SessionDetailPanel'de; `worker`/`coordination` SSE tipleri executions'a bağlı.
- **Sapma:** tasarımdaki ayrı `CoordinationEngine` turn-hook yerine geri bildirim
  `runWorker` içinden doğrudan (runSpawn başarısız turda FireTurnFinished çağırmıyor →
  hook yolu worker hatalarını iletemezdi).
- **Test:** `agent/coordination_test.go` (kuyruk serileştirme+coalescing, worker cap,
  coordinator-link, notification format). `go build ./...` + `tsc` yeşil.

### İkinci tur: kalan adımların tamamı ✅ (2026-07-03)

- **CLI köprüsü:** koordinasyon araçları `BridgeTools`'a eklendi → claude-cli
  koordinatör de `spawn_worker/...` sürebilir (advertise + `dispatchCoordinationBridge`;
  `autonomous_interaction` ctx `WithSessionID` stamp).
- **Ayar UI'si:** `settings.CoordinatorMaxWorkers`/`CoordinatorMaxTurns` (ana+maskeli+
  patch + clamp 1–64 / 1–500) → `applySettings`→`SetCoordinatorLimits`; frontend
  `AppToolsPanel` "Koordinatör limitleri". Canlı doğrulandı (8/50→5/42).
- **M3 scratchpad:** koordinatör + worker'lar için ortak dizin
  (`<SessionDir(coordID)>/scratchpad`), `coordinationScratchpadBlock` context'e enjekte.
- **Efemeral worker hedefi:** `spawn_worker` profil hedefi (explore/coder/reviewer)
  `resolveWorkerTarget` ile kalıcı `worker:<profile>` ajanına materyalize (base'den
  klon, `profileWorkerMu` dup guard). Test: `TestSpawnWorkerMaterializesProfile`.
- **Canlı doğrulama:** backend boot + API smoke (rol set/get, `/workers`, ayar
  round-trip) uçtan uca geçti. `go build ./...` + **624 test** + `tsc` yeşil.
- **Kalan (opsiyonel):** LLM-in-the-loop görsel deneme (dev'de). Detay: `_Docs\47`.

## Feature: İçe aktarılan skiller için "Grup" (import namespace) ✅ (2026-07-02)

**İstek:** Markette başka kaynaklardan/linklerden skiller indirilebiliyor; içe
aktarırken **bir grup içinde** aktaralım ki mevcut skiller'e karışmasın.

**Çözüm (ingest pipeline'a `Group` opsiyonu):**

- `ingest.Options.Group` eklendi — doluysa her içe aktarılan skill'in `group`
  frontmatter'ı olarak yazılır (Skills UI'da tek katlanabilir başlık altında toplanır;
  `Skill.Group` zaten vardı). Boşsa kaynağın kendi `group`'u **korunur** (eskiden
  tamamen düşüyordu), doluysa onu **ezer**.
- `skills.RenderImportedSkill`/`mapCCSkill` imzasına `group` parametresi; skill +
  command adapter'ları `opts.Group` geçiriyor. Tek-skill import (`ImportCCSkill`) yolu
  `""` geçerek kaynağın kendi grubunu korur.
- API: `POST /api/ingest/install` `group`, `installRequest.group` (source-ref/directory-
  site kurulumu) — her iki install yolu `Options.Group`'a bağlı.
- UI (`SkillImportDialog`): "Grup (opsiyonel)" alanı; tarama sonrası grup **kaynak
  adından otomatik ön-doldurulur** (`deriveGroup`: `owner/repo`/GitHub tree URL → repo,
  yerel yol → son klasör), kullanıcı düzenleyene kadar. Elle düzenlenince ön-doldurma
  durur.
- Test: `import_test.go TestMapCCSkillGroup` (açık grup yaz / kaynak grubu koru / açık
  grup ezer). `go build`/`test` + `tsc` yeşil.

## Fix: sqz/rtk hook'u PowerShell aracını kaçırıyordu ✅ (2026-07-02)

**Bulgu:** Ayarlar ▸ Dış Araçlar'daki tek-tık "Bağla", sqz/rtk hook'unu
`matcher: 'Bash'` ile kuruyordu. `shell` aracı 2026-07-01'de `Bash` + `PowerShell`
olarak bölününce, Windows'ta ajanlar shell komutlarını **`PowerShell`** aracıyla
çağırdığından hook **hiç eşleşmiyordu** → sqz/rtk sessizce devreye girmiyordu.
Örnek oturum (`WS6/SES1`) `debug.jsonl`'inde onlarca `PowerShell` çağrısı var ama
sıfır `hook` olayı; sqz/rtk'nin kendisi CLI testinde PowerShell komutlarını sorunsuz
sıkıştırıyor (sqz `tool_name`'i umursamıyor, rtk sadece `rtk ` ekliyor) — yani tek
kusur matcher'daydı.

**Çözüm:**

- `agent/hooks.go` `hookMatches` artık **virgülle ayrılmış alternatif** glob'ları
  destekliyor (`Bash,PowerShell` → herhangi biri eşleşirse tetiklenir; `filepath.Match`
  süslü parantez desteklemediği için). Boşluk-toleranslı.
- `ExternalToolsPanel.tsx` rtk+sqz template matcher'ları `Bash` → `Bash,PowerShell`.
- `_Docs/18-HOOKS.md` matcher bölümü + Windows uyarısı güncellendi.
- Regresyon: `hooks_test.go` `TestHookMatches`'e 5 virgül-alt vakası.

**Not:** Eski workspace'lerde matcher `Bash` kalmış hook'lar elle `Bash,PowerShell`
yapılmalı (veya kaldırıp yeniden "Bağla"). `go build`/`test` + `tsc` yeşil.

## Code Execution with MCP — Faz 3: canlı A/B ölçümü ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 3 — klasik tool-loop vs kod-modu, gerçek LLM + gerçek
MCP sunucusuyla uçtan uca karşılaştırma.

**Düzenek:** Geçici ikinci TionHarness instance'ı (ayrı port + temp data dir — canlı
örneğe dokunulmadı), native ajan **minimax-anthropic/MiniMax-M3** (anthropic
anahtarı geçersiz çıktı), gerçek `sqz-mcp`; görev: 5 dizinin girdi sayımı
(ground truth 336). Metrikler `debug.jsonl`.

**Sonuç (özet — tam tablo `_Docs/44` §12):**

- **A klasik:** ✅ 336 · 3 iterasyon · in+out 19.098 tok · bağlama 4.366 B araç çıktısı · 7,3 s
- **B1 kod (naif):** ❌ **533 — yanlış!** Script sıkıştırılmış dönüşü `len()` ile
  saydı; model veriyi görmediği için fark edemedi → **§7 doğruluk riski canlı
  doğrulandı** (ölçümün en değerli çıktısı). Doğru olsaydı: in+out −%13, bağlama
  giren araç verisi −%91 (373 B).
- **B2 kod (format-bilinçli):** ✅ 336 · 10 iterasyon · 7 run_code · 25 köprü çağrısı ·
  in+out 22.239 · 55 s — model sqz formatını script içinden keşfetti (`expand(hash)`),
  binding docstring'ini Read'le okudu; on-demand tanım okuma tasarımı sahada çalıştı.
- **Zincir uçtan uca doğrulandı:** Settings toggle canlı → run_code kaydı → binding +
  köprü + izin + `via run_code` debug olayları + katlanabilir trace kartı (5 alt satır).
- **Dürüst not:** sqz-mcp kod-moduna en aleyhte senaryo (çıktılar zaten sıkışık);
  büyük-çıktılı tekrar (browser-mcp/playwright) sıradaki hedef. `run_code`
  açıklamasına "opak dönüşte önce küçük örnek print et" nudge'ı **eklendi**
  (2026-07-03, `builtin_runcode.go` "ACCURACY:" paragrafı).

## Etiketler + Etiket-Tetikleyicili Otomasyonlar ✅ (2026-07-02)

**İstek:** (1) Sohbet/flow/schedule kayıtlarına etiket (tag) ekleyebilmek — hem
kullanıcı UI'dan hem ajan araçlarla düzenleyebilsin. (2) Schedules ekranına yeni
"otomasyon" türü: belirli bir etikete sahip oturum bir turu **bitirince**, o
oturumun sonucunu alıp yeni bir oturum başlatan → kendiliğinden süren döngüler.

**Tasarım kararı (kullanıcıyla netleşti):** Tetikleyici = etiketli oturumda bir tur
`end_turn` ile bitince (araçlar kullanılıp son cevap verilince). Ham "her tur"
sohbet selini **tag-gating** (yalnız tetik etiketi taşıyan oturumlar) +
guardrail'ler (maks. iterasyon, cooldown, aç/kapa) ile önlenir. Otomasyon ayrı bir
`Automation` entity'sidir ama UI'da Schedules ekranında ayrı bölümde gösterilir.

**Yapılan (backend):**

- **Etiket alanları:** `Session.Tags` / `Flow.Tags` / `Schedule.Tags` (`[]string`,
  omitempty; `Task.Tags` zaten vardı). Store setter'ları `SetSessionTags` /
  `SetFlowTags` / `SetScheduleTags` + paylaşımlı `normalizeTags` (trim/dedup/boş-at).
- **`Automation` modeli** (`db/models_automation.go` + `store_automation.go`): CRUD +
  `SetAutomationEnabled` (aç→sayaç sıfır) + `RecordAutomationFire` + `ResetAutomationCount`.
  Alanlar: TriggerTag, TargetAgentID, PromptTemplate (13 değişken: {{result}}/
  {{title}}/{{tag}}/{{sessionId}}/{{iteration}}/{{maxIterations}}/{{agent}}/
  {{prevPrompt}}/{{automation}}/{{date}}/{{time}}/{{datetime}} — `turnVars`, 2026-07-03
  genişletildi + canlı doğrulandı), SpawnTags (nil→[TriggerTag]=döngü), Enabled, MaxIterations (vars.
  50, 0=sınırsız), CooldownSec, IterationCount/LastFiredAt/LastSessionID/LastError.
  Yeni id prefix `AUT`, dir `automations`, `load()`'a eklendi.
- **Tur-tamamlanma hook'u:** `Runtime.turnHook` + `SetTurnHook` + `FireTurnFinished`
  (detached goroutine → turu bloklamaz). Çağrı yerleri: chat_stream (her yanıt
  sonrası), spawn (`runSpawn`), scheduler (`deliverPrompt` + `deliverWake`).
- **`AutomationEngine`** (`agent/automation.go`): `OnTurnFinished` → biten oturumu
  yükler, etiketsizse hızlı döner; eşleşen enabled otomasyonlar için cooldown +
  maks-iterasyon (aşılırsa otomatik pasifle + event) kontrolü → `renderAutomationPrompt`
  → `SpawnSession(Tags=spawnTags)` → `RecordAutomationFire`. `SpawnOptions.Tags`
  eklendi (spawn'lanan oturum oluşturulurken etiketlenir → race yok).
- **API:** `PUT /api/sessions|flows|schedules/{id}/tags` + automations CRUD
  (`GET/POST /api/automations`, `PUT/POST toggle/POST reset/DELETE /{id}`).
  `SessionInfo`'ya `tags` eklendi.
- **Araçlar (self-management):** `set_session_tags` (sink, add/remove/replace),
  `set_flow_tags`, `set_schedule_tags`, `create/update/delete/list_automation`
  (provenance: ajan yalnız kendi oluşturduğunu düzenler/siler). `SessionSink`
  arayüzüne `Tags`/`SetTags` eklendi.

**Yapılan (frontend):**

- Types: `Session/Flow/Schedule.tags`, yeni `Automation`, `SessionInfo.tags`.
- API client: `setSessionTags/setFlowTags/setScheduleTags` + automation CRUD.
- Yeniden kullanılabilir `common/TagEditor.tsx` (chip editörü, Enter/virgül ekler,
  Backspace son etiketi siler). Bağlandı: SessionDetailPanel (Hedef altına
  "Etiketler" bölümü), FlowsPanel (meta toolbar), Schedules (satır içi).
- Yeni `panels/Automations.tsx` — Schedules ekranında "Otomasyonlar" bölümü:
  oluşturma formu (ad/tetik-etiket/hedef-ajan/prompt/maks-iter/cooldown) + liste
  (aç-kapa, iterasyon sayacı, spawn-etiket editörü, limit dolunca sıfırla, sil).

**Doğrulama:** `go build ./...` + 143 test (db+agent, yeni automation_test'ler dahil)

- 73 api test + `npx tsc --noEmit` yeşil.

**Sıradaki:** Canlı loop doğrulaması (gerçek sağlayıcıyla uçtan uca); opsiyonel
`tionharness-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi.

## Code Execution with MCP — Settings toggle + UI trace kartları ✅ (2026-07-02)

**İstek:** Kod-modunu env-only olmaktan çıkarıp Settings'e almak + `run_code` içi
MCP çağrılarını sohbet trace'inde kart olarak göstermek (`_Docs/44` kalan işler).

**Yapılan:**

- **Settings toggle `enableCodeMode`:** `settings.go` (Settings/DTO/Patch/mapping) +
  `store.go` apply + `api/server.go` `applySettings → SetCodeMode` (canlı) +
  `app.go`'da `TIONHARNESS_CODE_MODE` artık `EnableShell` gibi **tek seferlik boot seed**
  (doğrudan tunable set kaldırıldı; source of truth Settings ekranı).
- **Frontend:** `types/settings.ts` + `SettingsPanel` patch'i + `AppToolsPanel`'e
  toggle ("Kod-modu (run_code + MCP binding'leri)"); kabuk kapalıyken sarı uyarı
  kutusu (run_code kabuk yetkisi olmadan kaydedilmez).
- **Trace kartları:** `codemode.CallObservation`'a `Args` alanı (yalnız UI trace'i —
  model bağlamına girmez); `toolsetup` observer'ı her script-içi çağrıyı call-ctx
  sub-step sink'ine `StepTool` olarak ekler (mutex'li — çok-thread'li script
  eşzamanlı çağırabilir) → tool loop'un mevcut generic promotion'ı `run_code`
  kartını katlanabilir `StepSubagent` yapar. **Frontend değişikliği gerekmedi**
  (run_subagent kartıyla aynı render). Girdi 2KB cap (`capStepInput`), çıktı satırı
  "N KB in M ms"; red `permission_denied` reason'lı hata satırı.
- **Doğrulama:** `go build ./...` + 349 test (codemode/tools/agent/settings/api) +
  `npx tsc --noEmit` yeşil.

**Sıradaki:** Faz 3 A/B ölçümü (`_Docs/44` §11 metriğiyle).

## Code Execution with MCP — Faz 0: baseline ölçümü ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 0 — kod-modu kazancını ölçebilmek için gerçek MCP
ortamında occupancy baseline'ı.

**Yapılan:**

- **Yeni ölçüm aracı `cmd/measure-codemode`** (main/servers/report): data dizinindeki
  tüm workspace'lerin etkin MCP sunucularına gerçekten bağlanır (`mcp.ListServerTools`),
  üç senaryoyu raporlar: eager-full / tier-lazy (bugünkü default) / code-mode.
  Token tahmini `conversation.EstimateText` (runtime'la aynı). Read-only, tekrar
  koşulabilir (Faz 3'te aynı araçla karşılaştırılacak).
- **Gerçek sonuçlar (4 sunucu, 72 araç):** eager-full **67.050 B ≈ 17.049 tok/tur** ·
  tier-lazy **≈69 tok/tur** (−%99,6; +~236 tok/aktive araç) · code-mode **≈374 tok/tur**
  (−%97,8; binding'ler diskte 101,7 KB = 0 bağlam). context-mode sunucusu bayat
  konfig (ulaşılamıyor).
- **Gerçek kullanım profili (28 oturum, 204 llm_call):** in p50 1.967 · cacheRead
  p50 68.299 · out p50 515 tok; 336 araç olayının yalnız ~5'i gerçek harici MCP.
- **Dürüst bulgu:** şema-occupancy'yi tier-lazy zaten çözmüş (69 tok < 374 tok!) —
  kod-modunun gerçek değeri **aktivasyon churn'ü + ara verinin bağlam dışında kalması
  - tur sayısı düşüşü**. Faz 3 ölçüm metriği buna göre güncellendi: görev-başına
    toplam token + tur sayısı + bağlama giren araç-çıktısı baytı (şema tokenı değil).
    Detay + tablolar: `_Docs/44` §11.

**Sıradaki:** Faz 3 — MCP-yoğun çok-adımlı senaryoda klasik vs kod-modu A/B
(`turn-debug` ile görev-başına toplam maliyet).

## Code Execution with MCP — Faz 2: per-call izin + gözlemlenebilirlik ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 2 — kod-modu köprüsünde per-call izin kancası ("ask"
modda script içi mutasyon çağrıları onay UI'ına düşsün) + per-call gözlemlenebilirliğin
geri kazanılması (debug.jsonl).

**Yapılan:**

- **`codemode.Bridge` genişletildi** (`bridge.go`): `Start` artık `Config` alıyor
  (`Call`/`Allow`/`Gate`/`Observe`). `Gate` dispatch'ten önce çalışır; red, script'e
  loud `MCPError` (isError=true) olarak döner ve özet "(N denied by permission gate)"
  sayacı içerir. `Observe` dispatch edilen VE reddedilen her çağrı için
  `CallObservation{Tool, DurMs, OutBytes, IsError, Denied}` üretir.
- **`RunCodeTool`** (`builtin_runcode.go`): `RunCodeGate` + `RunCodeObserver` hook'ları;
  Call ctx'i closure'a bağlanıp köprüye verilir (prompter/grants/session-id ctx'te).
  Araç açıklamasına "ask modda onay beklemesi timeout'a sayılır" uyarısı eklendi.
- **Agent bağlantısı** (`toolsetup.go`): gate = native loop'un aynı çağrıya uygulayacağı
  **`permGate`'in birebir kendisi** (aynı mod, ctx prompter/grants, audit logger —
  ayrı politika kodu yok, tam parite; standing "always allow" grant'ları script içi
  çağrılarda da geçerli). Observer = her çağrı için `debug.jsonl`'a `DebugTool` olayı
  (`Detail: "via run_code"` / `"permission denied (via run_code)"`).
- **Bilinçli karar:** shell'deki otonom `git push` guard'ının MCP karşılığı eklenmedi
  (annotation yok, semantik opak) — parite kuralı yeterli: otonom tur native yolda
  neyi çağırabiliyorsa köprüden de onu çağırır. Gerekçe `_Docs/44` §10'da.
- **Testler (+4):** gate reddi (dispatcher çalışmaz, observer Denied kaydeder, özet
  sayar) + gate izni & observer dispatch kaydı (bridge_test) · python ile uçtan uca
  red → `MCPError` ve observer/özet doğrulaması + dispatch gözlemi (runcode_test).
  270 test yeşil (codemode/tools/agent) + api 73 yeşil.

**Sıradaki:** Faz 0 baseline ölçümü → Faz 3 çok-server ölçümü → Faz 5 UI trace kartları.

## Code Execution with MCP — Faz 1 PoC (`run_code`) ✅ (2026-07-02)

**İstek:** `_Docs/44-CODE-EXECUTION-MCP.md` planının ilk uygulama fazı — MCP araçlarını
şema olarak değil, üretilmiş Python binding'leri olarak sunmak (occupancy düşürme).

**Yapılan:**

- **Yeni paket `internal/codemode`:** `bridge.go` (per-execution loopback HTTP köprüsü —
  127.0.0.1 rastgele port, rastgele Bearer token, agent tool-filter parity, 200 çağrı/koşu
  tavanı, 120s per-call timeout, çağrı sayacı/özeti) + `bindings.go` (`.tionharness/mcp/`
  altına `_bridge.py` + server-başına Python modülü; docstring = açıklama + input schema;
  Python identifier sanitizasyonu; her çağrıda sıfırdan regen — bayat stub kalmaz).
- **Yeni araç `run_code`** (`internal/tools/builtin_runcode.go`): boş script → binding
  regen + modül/fonksiyon listesi (discovery); script → stripped env
  (`minimalScriptEnv`) + `PYTHONPATH` + köprü URL/token env'i ile python çalıştırır;
  yalnız stdout/stderr (16KB cap) + MCP çağrı özeti döner — **veri context'e girmez**.
  RiskExec (`classify.go`), varsayılan 60s / max 300s timeout.
- **Gate'ler:** `Tunables.codeMode` (default KAPALI, `TIONHARNESS_CODE_MODE=1` ile boot'ta
  açılır — `codemode_tunable.go` + `app.go`) VE `ShellEnabled` VE MCP kataloğu dolu.
  Kayıt `toolsetup.go` AttachMCP bloğunda; CLI köprüsüne verilmez (`bridgeExcluded` +
  `cliLazyBridgeExcluded`).
- **Plandan sapmalar (gerekçeli):** (1) "yalnız read-only MCP araçları" yerine **tool-filter
  parity** — MCP `Tool` yapısında `readOnlyHint` annotation'ı yok (doğrulandı), heuristik
  isim filtresi kırılgan; bunun yerine köprü ajanın kendi filtresini uygular + `run_code`
  RiskExec olduğundan ask modunda bütünüyle onaya düşer, read-only modda bloklanır.
  (2) Köprü token'ı subprocess'e **env ile** geçer — host secret değil, koşu-başına
  rastgele, köprüyle birlikte ölür; diske yazmaktan (worktree'ye sızma riski) daha güvenli.
- **Testler (12 yeni):** `codemode/bridge_test.go` (auth/403/400/429, dispatcher hatası
  loud, özet), `codemode/bindings_test.go` (sanitizasyon, allow filtresi, regen temizliği),
  `tools/builtin_runcode_test.go` (discovery, gerçek python ile MCP çağrısı + veri
  sızmaması, loud failure, script-içi bypass'ın köprüde reddi). `go build ./...` +
  codemode/tools/agent/api testleri yeşil.

**Sıradaki:** Faz 0 baseline ölçümü (turn-debug ile şema token payı, klasik vs kod-modu
A/B) → Faz 2 (per-call izin + trace olayları). Detay: `_Docs/44` §5.

## run_subagent: app-settings delegasyon master toggle'ı kaldırıldı ✅ (2026-07-02)

**İstek:** Ayarlar ▸ Geçişli yetenekler'deki "Ajan→ajan delegasyon (run_subagent)"
toggle'ını kaldır — araç zaten Araçlar ekranından (ajan denylist) aktif/deaktif
edilebiliyor; ikinci bir master toggle gereksiz.

**Yapılan (self-management master toggle'ının 2026-07-01'de kaldırılmasıyla aynı desen):**

- **Backend gate kaldırıldı:** `run_subagent` artık **daima kurulu** (native
  `toolsetup.go`, CLI köprüsü `mcp_interaction.go`, `subagent.go RunSubagentRunner`
  koşulsuz). `Tunables.delegation`/`SetDelegationEnabled`/`DelegationEnabled` silindi;
  `settings.EnableDelegation` (3 struct + patch pointer + `applyBool` + `server.go`
  push + `app.go` env seed `TIONHARNESS_ENABLE_DELEGATION`) tamamen çıkarıldı. **Kalan
  frenler:** `delegationMaxDepth` (1–10, vars. 3) + `delegationMaxCalls` (1–100, vars. 8)
  — her delegasyon çağrısında geçerli güvenlik/bütçe guard'ları.
- **Frontend:** `AppToolsPanel` toggle'ı bilgi kartıyla değiştirildi (araç Araçlar
  ekranından yönetilir), derinlik/çağrı limitleri koşulsuz gösteriliyor.
  `types/settings.ts` + `SettingsPanel.tsx` payload'ından `enableDelegation` alanı kaldırıldı.
- **Testler:** `TestRunSubagentGate` → `TestRunSubagentAlwaysInstalled` (registry'de
  daima var); `TestDelegation_DisabledToolAbsent` → `TestDelegation_ToolAlwaysAvailable`
  (unknown-tool hatası ALMAZ); `store_test`/`subagent_test`/`delegation_e2e_test`
  `EnableDelegation`/`SetDelegationEnabled(true)` referanslarından temizlendi.
- **Dokümanlar:** `tionharness-settings`/`tionharness-self-management`/`tionharness-autonomous-ops`
  skill'leri + `_Docs/11/22/24/25` "gated" ifadelerinden "daima kurulu, görünürlük
  araç-bazlı"ya güncellendi.
- **Doğrulama:** `go build` ✅, tüm backend testleri ✅, frontend `tsc --noEmit` ✅.

## Native anthropic: konuşma geçmişi kayan cache breakpoint'i ✅ (2026-07-02)

**İstek:** Fable 5 cache analizinde tespit edilen fırsat — native anthropic yolunda
prompt-cache yalnız statik prefix'i (tools + system) kapsıyordu; uzun oturumlarda baskın
maliyet olan ham transkript her turda tam ücretleniyordu (OpenRouter yolunda geçmiş
breakpoint'i zaten vardı).

**Yapılan:**

- `contentBlock`'a `CacheControl` alanı eklendi; `toAnthropicMessages` artık
  `extendedCache bool` parametresi alıyor ve caching açıkken **son mesajın son bloğuna**
  kayan bir breakpoint (1h TTL) koyuyor. `Complete` + `Stream` çağrıları güncellendi.
- Breakpoint muhasebesi: `tools(1) + system-static(1) + history(1) = 3` (Anthropic limiti 4).
- Semantik: tur N cache yazımı → tur N+1 cache okuması (0.10×), breakpoint en yeni mesaja kayar.
  Trailing `tool_result` dahil her blok türünde çalışır.
- **Testler:** `TestToAnthropicMessages_RollingHistoryBreakpoint` (yalnız son blok işaretli),
  `_BreakpointOnLastBlockAcrossKinds` (tool_result kuyruğu), `_NoCacheWhenDisabled`.
  `_Docs/17`'ye bölüm + thinking-cache-invalidation uyarısı eklendi.
- **Doğrulama:** `go build` ✅, providers testleri ✅ (12/12 anthropic testi PASS).

## Claude Fable 5 tam desteği ✅ (2026-07-02)

**İstek:** TionHarness'e Fable 5 (claude-fable-5) desteği ekle.

**Mevcut durum (kısmi destek vardı):** thinking resolver (`RequiresAdaptiveThinking` —
Fable/Mythos `thinking:disabled`'ı 400 ile reddeder, off/low → min adaptif 1024) ve
anthropic pricing (3/15) zaten vardı; ingest `agent_adapter` fable→claude-fable-5
eşliyordu. Eksikler tamamlandı:

- **Kataloglar:** `kind_claudecli.go`'ya `fable` alias'ı eklendi (listede yoktu);
  `kind_anthropic.go`'daki yanlış tanım düzeltildi ("yaratıcı yazım odaklı" →
  "en yeni nesil; 1M bağlam, adaptif düşünme (daima açık), ajan görevleri", listenin
  başına alındı); `kind_openrouter.go`'ya `anthropic/claude-fable-5` eklendi.
- **Aile tabloları (`context_window.go`):** Fable ayrı katman oldu —
  `windowFable=1M` (eskiden genel-Claude 200K'ya düşüyordu), `MaxOutputFor` →
  `maxOutClaudeCapable` 32K (eskiden 16K), `AdaptiveBudgetFraction` → 0.45
  (opus/sonnet sınıfı; eskiden 0.40). Genel-Claude fallback'i 200K/16K/0.40 kaldı.
- **Pricing:** OpenRouter tablosuna `anthropic/claude-fable-5` (3/15, Anthropic
  cache tier 0.10×/1.25×) eklendi.
- **Metinler:** ContextPanel çıktı-tavanı ipucu, `tionharness-settings` default skill,
  `_Docs/17` adaptif-fraction tablosu ve `tionharness-project` skill'i yeni aile
  sınıflamasına güncellendi (opus/sonnet/fable 32K · haiku 16K).
- **Testler:** `context_window_test`/`maxoutput_test` yeni beklentilere güncellendi
  (+`fable` alias satırları). Doğrulama: `go build` ✅, providers/agent/conversation/
  billing testleri ✅, frontend `tsc --noEmit` ✅.

## Doküman bakımı: 40 numara çakışması + eksik index satırları ✅ (2026-07-02)

**Tespit (proje incelemesi):** `_Docs` içinde iki dosya 40 numarasını paylaşıyordu
(`40-PLAN-MODE.md` + `40-COKLU-SECIM.md`) ve `00-GENEL-BAKIS.md` index tablosunda
37–43 arası dokümanlar hiç listelenmiyordu (36'dan 44'e atlıyordu).

**Yapılan:**

- `40-COKLU-SECIM.md` → **`45-COKLU-SECIM.md`** (git mv; dosya başlığına numara notu,
  `05-ILERLEME` içindeki referans güncellendi). Plan modu 40'ta kaldı.
- `00-GENEL-BAKIS.md` index'ine 37/38/39/40/41/42/43/45 + `analiz-craftagent-arac-eslestirme.md`
  satırları eklendi; "Numara notu" 40→45 taşınmasını belgeliyor.
- `tionharness-project` skill'i (the external agent project workspace) düzeltildi: modül yolu
  `github.com/bilal-arikan/tionharness` (yanlış `bilal/tionharness` idi), go.mod'a `jchv/go-webview2`
  eklendiği bilgisi, klasör yapısına `cmd/tionharness-desktop` + `internal/app`, doc index'e
  39=Dizin-Site-Registry / 40=Plan-Modu / 44 / 45, kırık `39-PLAN-MODE.md` referansı →
  `40-PLAN-MODE.md`, bayat "Sırada: Faz 9 Wails" → native pencere zaten yapıldı (Wails'siz),
  Bash+PowerShell ayrımı + WebSearch + transform_data araçları eklendi.
- **Not (bir önceki oturum, commit `7ce98a4`, 2026-07-02 01:39):** persistent-pool
  gözlemlenebilirliği (`CLISessionPool.SetLogger` yaşam-döngüsü logları) + `resumeGateEnabled`
  saf fonksiyon + `TestResumeGateEnabled` + AppToolsPanel çifte-toggle uyarısı + `_Docs/17`
  düzeltmesi (canlı 3-tur ölçüm: resume ≈−43%, persistent ≈−58% cacheWrite) ve birikmiş WIP
  (rewind, code-execution MCP planı, bridge filter, sidebar chrome, error toast) commit'lendi.
- **Doğrulama:** `go build ./...` + `go vet ./...` + `go test ./...` (22 paket) + frontend
  `tsc --noEmit` tamamı yeşil.

## Otonomi duraklatma → yalnız workspace-özel + Zamanlamalar ekranına taşındı ✅ (2026-07-01)

**İstek:** Workspace ayarları ekranındaki "Otonomiyi duraklat" seçeneğini **Zamanlamalar**
ekranına taşı; gelişmiş uygulama ayarlarındaki "Tüm otonomiyi duraklat (uygulama geneli)"
anahtarını tamamen kaldır — pause artık yalnız workspace-özel olsun, Zamanlamalar
ekranından açıp kapatmak yeterli.

**Yapılan:**

- **App-geneli pause tamamen kaldırıldı (backend):** `settings.Settings/snapshot/patch`'ten
  `PauseAutonomy` alanı, `store.go` patch-apply'ı, `api/server.go`'daki
  `tun.SetAutonomyPaused(...)` çağrısı silindi. `agent.Tunables`'tan `pauseAutonomy`
  alanı + `SetAutonomyPaused`/`AutonomyPaused` metotları kaldırıldı. Otonomi freni artık
  yalnız workspace-düzeyi `Runtime.Paused()` (ws-settings `pauseAutonomy` → `SetPaused`):
  `budget.go guardedComplete` ve `reflector.go maybeAutoReflect` kontrolleri
  `r.tun.AutonomyPaused() || r.Paused()` → sadece `r.Paused()`.
- **Frontend:** Ayarlar ▸ Gelişmiş'ten "Otonomi" bölümü + `AutonomyPanel` bileşeni ve
  `settings.pauseAutonomy` tipi kaldırıldı. Workspace ▸ Genel'deki workspace pause toggle'ı
  kaldırılıp yerine Zamanlamalar'a yönlendiren not kondu. **Zamanlamalar ekranının üstüne**
  workspace-özel pause toggle'ı eklendi (`Schedules.tsx`: `getWorkspaceSettings` ile yüklenir,
  `updateWorkspaceSettings({pauseAutonomy})` ile optimistic toggle; `data-testid=workspace-pause-autonomy`).
- **Dokümanlar/skill:** `tionharness-settings` + `tionharness-autonomous-ops` skill'leri güncellendi
  (pause artık app-settings key'i değil, workspace-özel + Zamanlamalar ekranı); `06-WORKSPACES.md`
  tablosu not düştü. `update_settings` araç örnekleri `pauseAutonomy` yerine `autoTitleEnabled` kullanıyor.
- **Durum:** `go build ./...` ✅, ilgili paket testleri ✅ (agent/tools/settings 256 test), frontend `tsc` temiz.

## `/rewind` — sohbet checkpoint geri sarma (yalnız-sohbet MVP) ✅ (2026-07-01)

**İstek:** Sohbet ekranına Claude Code'daki `/rewind` benzeri bir komut ekle — bir tur
kodu bozunca ajanla tartışıp bağlamı kirletmek yerine, hatadan önceki temiz checkpoint'e
dönüp çarkı yeniden çevirmek için. Kapsam kullanıcı kararıyla **yalnız-sohbet** (dosya
geri-yükleme yok; kod için git zaten var).

**Yapılan:**

- **Backend truncate:** `db.DeleteMessagesFrom(sessionID, msgID)` — verilen mesaj + sonrasını
  siler, JSONL'i yeniden yazar; silinen sayıyı döndürür. Truncation noktası özet sınırından
  önceyse (`SummaryMsgCount > idx`) **artık geçersiz rolling-summary sessizce sıfırlanır**
  (dangling özet bırakılmaz). Test: `db/rewind_test.go` (removed sayısı + özet reset + reopen kalıcılığı).
- **API:** `POST /api/sessions/{id}/rewind` body `{messageId}` → `handleRewindSession`
  (`bindJSON` + `DeleteMessagesFrom`, Logs'a `session rewound` satırı). server.go route kaydı.
- **Frontend:** `/rewind` slash komutu (`useChatStream.chatCommands`, ⟲) → `RewindDialog`
  picker açar. Dialog oturumun kullanıcı promptlarını (checkpoint) yeniden-eskiye listeler
  (her satır: "Prompt #N" + "M mesaj silinir" + önizleme); seçilen checkpoint'e geri sarar.
  `rewindTo(msgId)` görünüm + sunucu tarafını atomik siler ve **silinen promptu composer
  draft'ına geri koyar** (düzenleyip yeniden göndermek için) — `writeSessionDraft` +
  Composer `key` bump ile remount. Yerel-only (persist edilmemiş) anchor'da sunucu çağrısı atlanır.
- **Balon hover aksiyonu (2026-07-02):** her kullanıcı balonunun altında ⟲ "Buraya geri sar"
  butonu (`RewindButton.tsx`, iki-adımlı onay) → picker açmadan doğrudan o mesaja geri sarar
  (`UserTurn`→`MessageList` `onRewind`→`App.handleRewind`, dialog ile ortak yol).
- **Sınır (Claude Code ile aynı):** yalnız transcript geri alınır; `Bash` yan etkileri
  (`git push`/`npm install`/`rm`) ve dosya değişiklikleri geri **gelmez**.
- **Durum:** db+api derlenir + testler geçer (db 35, api 66), frontend `tsc` temiz.
  Dosya-restore modu (kod geri-yükleme) ileride eklenebilir — snapshot altyapısı gerektirir.

## Compact (compaction) promptu editlenebilir + tüm workspace'lerde default ✅ (2026-07-01)

**İstek:** Compaction (bağlam sıkıştırma) promptunu da editlenebilir yap ve bütün
workspace'lerin default (seed) promptu yap — summary/reflect/title gibi.

**Yapılan:**

- **Editlenebilir 4. runtime prompt:** `agent.PromptKeys`'e `"compact"` eklendi →
  config API (`GET/PUT /api/workspace-config`) ve WorkspaceFilesPanel bu key üzerinden
  döndüğü için **otomatik editlenebilir** oldu (`config/prompts/compact.md`). Default
  `promptDefaults["compact"] = conversation.CompactPromptDefault()` (ham template, iki
  `%s` slotlu).
- **Per-workspace enjeksiyon (paylaşılan global Manager'a rağmen):** `conversation`
  paketine `WithCompactPrompt(ctx, tmpl)` + `compactPromptFromCtx(ctx)` eklendi;
  `summarizeRendered` template'i ctx'ten alır. **Güvenlik:** ctx template'i yalnız
  **tam iki `%s` ve başka `%` verb'ü yoksa** kullanılır — bozuk edit'te `fmt.Sprintf`'in
  `%!`-işaretli çıktısı yerine sessizce gömülü default'a düşer. Enjeksiyon 5 çağrı
  yerinde: chat / chat_stream / wake_turn (Prepare) + summary (ForceCompact) +
  toolloop (reaktif mid-loop, `r.CompactPromptTemplate()`).
- **Tüm workspace'lerde default:** yeni `"compact"` PromptKey olduğu için boot'ta
  `syncConfigFiles → SeedWorkspaceConfig → writeIfAbsent` her workspace'e
  `compact.md`'yi **otomatik** yazar (restart sonrası). README template'i iki-`%s`
  uyarısıyla güncellendi.
- **Temizlik:** eski read-only `wsConfigDTO.CompactionPrompt` alanı kaldırıldı (compact
  artık editlenebilir promptKeys'te; duplikasyon önlendi). `CompactionPromptText()`
  export'u referans için korundu. Frontend'de karşılığı salt-okunur textarea bloğu +
  `WorkspaceConfig.compactionPrompt` tipi de kaldırıldı; `PROMPT_LABELS`'a `compact` eklendi.
- **UI overflow fix (workspace prompt ekranı):** uzun promptlar (compact + 40KB instructions)
  ekran dışına taşıyordu — kök neden `WorkspaceView` sağ-içerik `flex-1` sütununda **`min-w-0`
  yokluğu** (flex item min-content genişliğinin altına küçülemiyordu). `min-w-0` eklendi
  (WorkspaceView sütunu + içerik container), `PromptEditor` root `w-full min-w-0` + split
  yarımları `min-w-0` + preview `break-words`. `<pre>` zaten `overflow-x-auto` taşıyordu.
  Frontend tam build ✅ (dist yazıldı).
- **Durum:** çekirdek paketler (`conversation`+`agent`) derlenir + `go vet` temiz + test
  yazılacak. **api paketi build'i, ilgisiz market_publish/export granular-selection
  WIP'i (`publishInclude` bool→ID-list refaktörü) nedeniyle bloklu** — o WIP tamamlanınca
  api tarafı + binary derlenir.

## Skill 4-tier görünürlük + default workspace prompt ✅ (2026-07-01)

**İstek:** (1) Skilleri de araçlardaki gibi 4 görünürlük kategorisinden birine
ayarlanabilir yap. (2) Yeni workspace'lerin default prompt'unu the external agent project'ın tam
sistem promptu gibi yap (TionHarness'te monolitik sistem promptu yok — workspace prompt
onun yerini tutar). (3) `ClaudeResume`'u default açık yap.

**Yapılan:**

- **Skill 4-tier görünürlük (araç muadili tek seçici):** skiller artık `full` /
  `summary` / `name-only` / `hidden` tier'larından **tam birini** taşır. Önceden 3
  durum vardı (full / name-only / `auto_summary:false`≈hidden); eksik **summary**
  (slug + açıklama, when bastırılır) eklendi. Türetilmiş `Skill.Visibility`
  (`skillVisibility()`) + `Store.SetVisibility(slug,tier)` üç frontmatter flag'ini
  tek yazımda kurar. Yeni `SummaryOnly` alanı + `isSummaryOnly` +
  `setFrontmatterSummaryOnly`; `renderCatalog` summary'de when'i atlar. API
  `PUT /api/skills/{slug}/visibility` (geçersiz tier 400). UI: `SkillVisibilitySelector`
  eski Özet/NameOnly toggle çiftini değiştirir, araçların `VISIBILITY_TIERS`'ini
  paylaşır. Detay: `_Docs\19` §Skill 4-tier.
- **Default workspace prompt:** `internal/workspace/defaults/default-instructions.md`
  (varsayılan ajan yönergeleri, ~40KB) `//go:embed` ile `defaultWSSettings().Instructions`
  seed'ine bağlandı → talimatı olmayan (yeni) workspace'ler bu baseline'la açılır.
  Persisted `instructions` bunu override eder (mevcut workspace'ler etkilenmez).
- **ClaudeResume:** zaten default açıktı (kod `settings.go` DefaultSettings + canlı
  `claudeResume=true`); değişiklik gerekmedi, doğrulandı.
- **Yan düzeltme:** `RevealButton.onReveal` tipi `() => void | Promise<unknown>`'a
  genişletildi (reveal endpoint'leri `{path}` döndürüyor; çağrı yerleri tek noktadan
  tip-uyumlu oldu).

**Cross-runtime cache benchmark (claude-cli resume vs the external agent project SDK):** aynı 3-mesajlık
konuşma iki runtime'da ölçüldü. TionHarness (claude-cli, resume açık) ilk turları **soğuk**
yazıp tur-başı ~70-90K cache **yeniden yazıyor** (warm-read tutarsız, yalnız bazı
turlarda); the external agent project SDK 1. turdan **istikrarlı sıcak** cache okuyor (cR≫cW) → aynı
konuşmada ~3× ucuz. Ağır (~40KB) workspace prompt eklemek TionHarness'te cache-write'ı
+42K büyüttü (input değişmez — prompt cache'e gider). Sonuç: darboğaz claude-cli'nin
sıcak prefix'i turlar arası **tutarlı** koruyamaması.

## Dışa aktarım: promptlar/README + flows/skills/schedules tek tek seçilebilir ✅ (2026-07-01)

**İstek:** Export'a "Promptlar & Dosyalar" ekranındaki diğer promptları da dahil et (varsayılan
değilse); Flows, Workspace skill'leri ve Schedules'ı ajanlar gibi **tek tek** seçilebilir yap.

**Yapılan:**

- **Payload:** `market.WorkspacePayload`'a `Prompts map[string]string` (yalnız varsayılandan
  farklı runtime prompt override'ları) + `Readme string` eklendi (`internal/market/pack.go`).
- **Export builder** (`buildWorkspaceTemplatePayload`): her `agent.PromptKeys` anahtarını okur,
  `PromptDefault` ile karşılaştırır, **yalnız farklı olanları** taşır; README boşsa atlanır.
  `Include` boolean yerine **id/slug set**'leriyle çalışır — `wantSet(all, ids)` (nil=tümü,
  boş=hiçbiri) + `sliceOrNil` ile agents/flows/skills/schedules bağımsız filtrelenir.
- **Include şeması:** `publishInclude` = `AgentIDs/FlowIDs/SkillSlugs/ScheduleIDs []string`
  (tri-state) + `Instructions/Prompts/BoardColumns bool`. Frontend `WorkspaceExportInclude` aynen.
- **Seed/install:** `seedTemplateConfigFiles` (`seedWorkspaceTeam` 5. adım) install'da non-default
  promptları `config/prompts/`, README'yi `config/README.md`'ye yazar. Dokunulmamış promptlar
  hedefteki güncel varsayılanı korur (`internal/api/templates.go`).
- **UI:** yeni yeniden kullanılabilir `ExportPickList.tsx` (checkbox liste) ile Ajanlar/Akışlar/
  Skill'ler/Zamanlamalar dört ayrı seçim listesi; Talimatlar/Promptlar&README/Pano toggle kaldı.
  Promptlar toggle'ı `getWorkspaceConfig`'ten non-default prompt + README sayısını gösterir.
  Önizleme + bağımlılık uyarısı seçili öğelere göre güncellendi.
- `go build ./...` ✅ · `npx tsc --noEmit` ✅. (Not: bu refactor, paralel "Compact prompt" WIP'inin
  beklediği api-build blokerini de çözer.)

## Dışa aktarıma canlı önizleme ✅ (2026-07-01)

**İstek:** Dışa aktarım paneline **canlı önizleme** ekle.

**Yapılan:**

- `WorkspaceExportPanel`'e, kategori toggle'larının altında **Önizleme** kartı eklendi:
  mevcut seçimin tam çıktısını pill'lerle gösterir (N ajan / akış / zamanlama / skill /
  talimat / pano sütunu). Kapalı veya 0 olan kalemler soluk + üstü çizili.
- Sayımlar backend kurallarını yansıtır: **zamanlama yalnızca seçili ajana bağlıysa** sayılır
  (orphan düşer); akış/skill/talimat/pano ilgili toggle'a uyar.
- Kart üstünde çözümlenen **ad · sürüm · pack id** ve aynı slug'a yeniden yayında
  **üzerine yazma** notu. Frontend `slugify`, Go `slugify` ile birebir eşleşir →
  önizleme sunucuyla aynı `workspace-<slug>` id'sini verir.
- `npx tsc --noEmit` ✅. Dosya: `frontend/src/components/workspace/WorkspaceExportPanel.tsx`.

## Dışa aktarıma metadata alanları + bağımlılık uyarısı ✅ (2026-07-01)

**İstek:** Dışa aktarıma **isim/açıklama/sürüm** alanı desteği ve **ajan bağımlılık uyarısı** ekle.

**Yapılan:**

- **Metadata alanları:** `WorkspaceExportPanel`'in üstüne **Şablon adı / Açıklama / Sürüm**
  girişleri kondu. Ad `ws.name`'den seed edilir; açıklama/sürüm boşsa sunucu varsayılan üretir.
  Frontend `WorkspaceExportMeta` (`name?/description?/version?`) `api.publishPack(...)`'ın yeni 4. argümanı olarak geçer; boş alanlar düşürülür.
- **Backend:** `publishRequest`'e `Name/Description/Version` (omitempty) eklendi;
  `handlePublishMarket` workspace kind'ında bunları trim edip `BuildWorkspacePack(slug, name,
desc, version, …)`'e verir. `BuildWorkspacePack` imzasına `version` parametresi eklendi
  (boş → `1.0.0`). Slug artık kullanıcı adından türetilir → farklı ad = farklı pack id.
- **Bağımlılık uyarısı:** panel, hariç bırakılan ajanlara bağlı akış/zamanlamaları uyarı
  kutusunda listeler. Flow bağımlılığı `flow.graph` JSON'u client'ta parse edilip
  `type==='agent'` düğümlerinin `agentId`'leri toplanarak hesaplanır. Etki net: akış → kopuk
  referans, zamanlama → dışa aktarımdan düşer.
- `go build ./...` ✅ · `npx tsc --noEmit` ✅. Dosyalar: `internal/market/publish.go`,
  `internal/api/market.go`, `internal/api/market_publish.go`, `frontend/src/api/market.ts`,
  `frontend/src/components/workspace/WorkspaceExportPanel.tsx`.

## Workspace dışa aktarımı ayrı sekmeye taşındı + seçilebilir içerik ✅ (2026-07-01)

**İstek:** Workspace ayarlarındaki "export aldığımız kısım" (Şablon olarak yayınla) ayrı bir
alt-panele taşınsın (feature detaylandırılacak); export alırken **neyin dahil edileceği** seçilebilsin.

**Yapılan:**

- **Yeni alt-sekme:** `WorkspaceView.tsx`'e `export` sekmesi ("Dışa Aktar", `PackageCheck` ikonu)
  eklendi (TAB_KEYS + TABS + render dalı). Kendi publish aksiyonu olduğu için header "Kaydet"
  butonu bu sekmede gizli (appearance gibi). Genel (`WorkspacePanel`) tab'ındaki eski
  "Şablon olarak yayınla" butonu **kaldırıldı**, yerine yeni sekmeye yönlendiren not kondu.
- **Yeni panel** `frontend/src/components/workspace/WorkspaceExportPanel.tsx`: ajanları
  (listAgents), akışları, workspace-tier skill'leri, zamanlamaları çeker; **ajan seçim listesi**
  (checkbox, varsayılan tümü seçili, ≥1 zorunlu) + kategori toggle'ları (Akışlar / Zamanlamalar /
  Skill'ler / Talimatlar / Pano sütunları, sayı rozetli, varsayılan açık). "Şablon olarak dışa aktar"
  → `api.publishPack('workspace', ws.id, include)`.
- **Backend:** `publishRequest`'e opsiyonel `Include *publishInclude` alanı (`AgentIDs []string`
  (nil=tümü) + `Flows/Schedules/Skills/Instructions/BoardColumns bool`). `buildWorkspaceTemplatePayload`
  artık `inc *publishInclude` alıyor — **nil = her şey** (geriye uyumlu), aksi halde ajanları filtreler
  ve kategori flag'lerini birebir onurlandırır (`market_publish.go`). En az bir ajan hâlâ zorunlu.
- **API tipi:** `market.ts`'e `WorkspaceExportInclude` tipi + `publishPack(kind, sourceId, include?)`.

**Doğrulama:** `go build ./...` ✅ · `npx tsc --noEmit` ✅.

## Harici araçlar listesine `codebase-memory-mcp` eklendi ✅ (2026-07-01)

**Yapılan:** Ayarlar ▸ Harici Araçlar ekranının kaynağı olan `knownExternalTools` slice'ına
(`internal/api/external_tools.go`) yeni entry: **codebase-memory-mcp** (DeusData) — kod tabanını
kalıcı bilgi grafiğine indeksleyen stdio MCP sunucusu (158 dil, sub-ms sorgu, ~%99 daha az token).
`category=dev`, `wire=mcp` (Market'te "Codebase Memory MCP" paketiyle kurulur). Tespit PATH'te
`exec.LookPath` ile; program `<progs>/codebase-memory-mcp/` altında ve PATH'te
olduğundan ekran **Found** gösteriyor. `go build ./internal/api` ✅. Not: yeni entry PATH'e o dizini
içeren bir süreçten görünür — backend yeni PATH ile yeniden başlatıldı.

## Seçili dil sohbet bağlamına enjekte ediliyor (profil zaten ediliyordu) ✅ (2026-07-01)

**Soru:** Profil bilgilerim ve seçtiğim dil bağlama ekleniyor mu?

**Bulgu:** Profil (Ad/Konum/Saat dilimi/Notlar) zaten **sohbet** turlarında "## About the
user" bloğu olarak enjekte ediliyordu (`api.userContextBlock` → `composeTurnRequest`).
Ama **dil (tr/en) hiçbir yere enjekte edilmiyordu** — blok yalnız profil alanlarını
içeriyordu.

**Yapılan:** `userContextBlock`'a **dil yönergesi** eklendi (`languageName` yardımcısı) —
"Preferred language: reply in Turkish (Türkçe) by default…". Profil alanı boş olsa bile
dil satırı çıkar (dil varsayılanı `tr`), yani her sohbet turu artık dili onurlandırıyor.

**Bilinen sınır:** Enjeksiyon **yalnız sohbet yolunda** — otonom/zamanlanmış/flow turları
(`agent.autonomousSystemPrompt`, farklı paket) bu bloğu hâlâ almıyor. Utility promptları
(reflect/title/summary) zaten "reply in the same language as the data" diyor. Otonom yola
taşımak istenirse Tunables köprüsü gerekir (backlog). `go build`/`api test` ✅.

## "Özet promptu" netleştirildi + asıl compaction promptu salt-okunur gösteriliyor ✅ (2026-07-01)

**Soru:** Promptlar & Dosyalar ekranındaki "Özet promptu" kullanılıyor mu? Özetleme
kapsamlı olmalı; gereksizse sil, veya asıl promptu göster.

**Bulgu:** "summary" runtime promptu **kullanılıyor ama konuşma özetlemesi değil** —
yalnız `/memory · /board · /flows` slash-komutlarının anlık genel-bakış sistem promptu
(`agent/summarizer.go`, composer'da hâlâ bağlı: `useChatStream.ts`). Asıl konuşma
özetlemesi ayrı ve **zaten kapsamlı**: `conversation/manager.go compactPrompt` (8 bölüm +
anti-decay), düzenlenemez.

**Yapılan:**

- Etiket "Özet promptu" → **"Genel bakış promptu"**, hint bunun slash-komut özeti olduğunu
  ve konuşma özetlemesi olmadığını açıkça belirtiyor (`WorkspaceFilesPanel.tsx`).
- **Asıl compaction promptu salt-okunur gösteriliyor:** `conversation.CompactionPromptText()`
  (yeni exported erişimci; `%s` slotları etiketle doldurulmuş) → `wsConfigDTO.compactionPrompt`
  (`api/workspace_config.go`) → panelde read-only textarea (`WorkspaceConfig.compactionPrompt`).
- Silinmedi (slash komutları hâlâ kullanıyor). `go build`/`tsc` temiz.

**İstek:** Ayarlardan self-management aç/kapa silinsin (araçlar diğerleri gibi olsun);
her araca skill'lerdeki gibi görünürlük seçilebilsin — **4 tier, biri seçili**:
`Tam` (context'in tamamı) / `Özet` / `İsim` / `Gizli`. Ayrıca araç bilgisinde olup
UI'da görünmeyenler (örnekler, when-to-use) gösterilsin.

**Yapılan:**

- **Backend:** `tools.Visibility{Full,Summary,NameOnly,Hidden}` + `SetVisibility`/
  `VisibilityOf` (registry). `WorkspaceToolConfig.ToolVisibility map[string]string`
  (eski `HiddenTools`/`ShownTools` listeleri yüklemede map'e migrate). `toolsetup`
  override'ları en son `SetVisibility` ile uygular. `workspace-tools` API `visibility`
  - `examples` döndürür, PUT `toolVisibility` map'i alır (`validVisibility` doğrular).
- **Self-manage:** master toggle kaldırıldı → paket daima kurulu, varsayılan `hidden`.
  **Tam sökme (aynı gün):** `settings.EnableSelfManage` alanı + `TIONHARNESS_ENABLE_SELFMANAGE`
  env + `Tunables.SelfManageEnabled`/`selfManage` + ayar UI toggle'ı **tamamen silindi**;
  `spawn_session` CLI köprüsünde koşulsuz ilan edilir. `TestInteractionAdvertisedNames`
  - `store_test` güncellendi.
- **UI:** `ToolsPanel` per-tool 4'lü segment (Tam/Özet/İsim/Gizli) + tier-renkli
  `VisibilityBadge`; toplu + per-server hızlı eylem 4 tier'a genişledi. Detay görünümü
  **örnek çağrıları** + tam (çok-paragraflı) açıklamayı gösterir.
- **CLI uyumu:** Native tüm tier'ları tam uygular; CLI'da tier katalog-bloğu metnini
  etkiler, gerçek yükleme CLI'nin ToolSearch/`alwaysLoad`'ıyla — `full` CLI'da "en
  fazla ilan", dış MCP'de "kesin eager" garantisi vermez (mimari sınır, dokümante).
- `go build ./...` + tüm testler yeşil, `tsc --noEmit` temiz. Detay: `_Docs/19`.

## Görünüm sadeleştirme: accent + temel mod kaldırıldı, renk=açık/koyu varyant ✅ (2026-07-01)

**İstek:** Görünüm ekranında "Vurgu rengi (accent)" ve "Temel mod (koyu/açık/sistem)"
kaldırılsın; onun yerine her tema **renginin** açık ve koyu karşılığı olsun.

**Yeni model:** Tek kontrol = `themePreset`. Bir preset id'i hem rengi hem modu kodlar
(`violet-dark` / `violet-light`). 6 renk ailesi (Mor/Mavi/Zümrüt/Gül/Kehribar/Nord) ×
2 varyant = 12 preset. Ayrı base-mode ve accent picker yok.

**Frontend:**

- `lib/themePresets.ts` yeniden yazıldı: `COLORS` (aile tanımı, dark+light accent/soft) →
  `THEME_PRESETS` (düz, aile başına 2) + `THEME_COLORS` (picker için aile+varyant) +
  `DEFAULT_PRESET='violet-dark'`. Neutrals (bg/surface/text) mod başına sabit, yalnız
  accent değişir.
- `lib/theme.ts` sadeleşti: `Appearance={themePreset}`, `applyTheme(preset)` (accent
  override + legacy dark/light/system yolu kaldırıldı; boş/bilinmeyen id → default preset).
- `AppearancePanel` (`settings/appPanels.tsx`): "Temel mod" + "Vurgu rengi" alanları
  kaldırıldı; tema paleti grid'i yerine **renk satırları** (her aile: Koyu + Açık swatch).
  Load/save yalnız `themePreset`.
- `App.tsx`: appearance ref/`applyClientPrefs`/ws-override yalnız `themePreset`.

**Backend:**

- `settings.Default().ThemePreset` `"midnight-violet"` → `"violet-dark"`; alan yorumu
  güncellendi. `Theme`/`Accent` alanları geriye-uyum için kaldı (artık UI'yı etkilemiyor).
- `cmd/tionharness-desktop/titlebar_windows.go`: preset→renk haritası kaldırıldı; titlebar
  artık id son-ekine (`-light`/`-dark`) göre mod-neutrals seçiyor (accent kullanılmıyor).

**Doğrulama:** `go build ./...` + desktop build ✅, frontend `tsc --noEmit` temiz ✅.
`tionharness-settings` skill'i güncellendi.

## Journal gürültü filtresi + recall eşiği ayarlanabilir ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu (4 paralel görevden 4.). Her turun **dinamik**
(cache-dışı) bağlamına enjekte edilen "Relevant memory" bölümü, önemsiz/düşük-bilgili journal
kayıtlarıyla kirleniyordu (ör. `Q: 2+2 kaç eder? A: 4 eder.`). Bu trivial turlar her tur taze token
harcatıyor ve dikkati dağıtıyordu.

**Çözüm — iki ayarlanabilir mekanizma:**

1. **Yazma-tarafı düşük-bilgi kapısı (`journalMinLen`)** — `Journal()` artık içeriği
   `journalMinLen` rune'dan kısa olan turları **hiç saklamadan** atar (boş-check'in yanında, uzunluk
   cap'inden önce). Böylece gürültü daha kaynakta, recall havuzuna girmeden kesilir.
   - Yeni tunable + settings alanı `journalMinLen`. **Default 40 rune** (muhafazakâr: kısa ama
     anlamlı notlar korunur). **0 = kapalı** (filtre devre dışı; geriye dönük tam uyum).
   - `0` anlamlı bir değer olduğu için cap deseninden farklı: getter `0`'ı default'a çevirmez,
     yalnızca negatifi 0'a normalize eder. Clamp: `0 ≤ minLen ≤ journalMaxLen`.
   - Test runtime'ları (`NewTunables`, `tun==nil`) kapıyı **kapalı** tutar — mevcut testler
     etkilenmez.

2. **Recall eşiği artık ayarlanabilir (`recallMinScore`)** — eskiden `memory.go` içinde
   sert-kodlu `const minScore = 0.04` idi. Artık `memory.Store` eşiği bir **canlı provider**
   üzerinden okur (`SetMinScoreProvider`); runtime bunu workspace'in `Tunables.RecallMinScore`'una
   bağlar, böylece ayar değişikliği **restart'sız** bir sonraki recall'da geçerli olur (import döngüsü
   yok — `memory` paketi `agent`'ı import etmez).
   - `recallMinScore` ayarı zaten settings/frontend'de **vardı ama ölü konfigdi** (hiçbir yere bağlı
     değildi, default 0.05). Artık gerçekten bağlandı; default `0.05 → **0.04**` düzeltildi (gerçekte
     yürürlükteki sabit değer 0.04'tü — davranış korunur). Kullanıcı gürültüyü kesmek için
     yükseltebilir.

**Etki:** Default 40-rune kapısı `Q: kısa? A: tek kelime` türü ultra-trivial turları kaynakta eler;
daha agresif filtreleme için `journalMinLen` yükseltilir (ör. 60–80) ve/veya `recallMinScore`
artırılır (ör. 0.08–0.12) — ikisi de dinamik segmentin token + dikkat maliyetini düşürür. Her iki
default da mevcut davranışı bozmaz (recall 0.04 sabit; kapı yalnızca en kısa turları eler).

**Değişen dosyalar:** `internal/agent/tunables.go` (+`journalMinLen`, +`recallMinScore`,
`SetJournalLimits` 3 parametre), `internal/agent/reflector.go` (`Journal()` kapı + `journalMinLen()`
helper), `internal/agent/runtime.go` (provider bağlama), `internal/memory/memory.go`
(`DefaultMinScore` + provider + `minScore()`), `internal/memory/graph.go` (floor provider),
`internal/settings/{settings,store}.go` (+alan, applyInt, clamp, default 0.04), `internal/api/server.go`
(applySettings wiring), `internal/agent/tunables_test.go` (yeni testler), frontend
(`types/settings.ts`, `SettingsPanel.tsx`, `appPanels.tsx`).

**Doğrulama:** `go build ./...` temiz; `go test ./internal/{memory,agent,settings,api,e2e}/...` →
201 test geçti; `tsc --noEmit` temiz.

## Statik prefix sadeleştirme — Deliverables skill'e taşındı + deferred-not tek yerde ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu (4 paralel görevden 3.). Sistem prompt'un statik
prefix'inde iki şişkinlik: (1) "deferred / ToolSearch ile yükle / unloaded ad → No such tool
available" açıklaması hem Skills hem Tools bloğunda tekrar ediyordu; (2) "Deliverables →
Artifacts" bloğu (inline media + gallery JSON + `![alt]` kuralları) her tur statik prefix'te —
token'dan çok dikkat/context-rot maliyeti.

**Yapılan:**

- **Adım 1 — Boilerplate birleştirme:** `internal/skills/store.go` `renderCatalog` içindeki
  skills `deferNote` tek kısa cümleye indirildi (`ToolSearch select:...` + "Available Tools
  notuna bak"). Mekanizmanın tam açıklaması (DEFERRED'ın anlamı, `"No such tool available"`
  cümlesi) artık **yalnız** `internal/agent/toolsetup.go` `renderLazyToolCatalog` CLI intro'sunda
  (tek canonical yer). Native vs CLI varyant farkı korundu (native eager → not yok).
- **Adım 2 — Deliverables skill'e taşındı:** Yeni shipped skill
  `internal/skills/defaults/tionharness-deliverables/SKILL.md` (`access: shared`, on-demand). Tüm
  detaylı kurallar (binary `sourcePath`, inline media, gallery, `update_artifact` by id) skill
  body'sine taşındı. `internal/api/artifacts.go` `artifactDeliverableGuidance` 2 satırlık özet +
  `use_skill tionharness-deliverables` pointer'ına indirildi. Bilgi **kaybolmadı** — sadece prefix'ten
  skill'e taşındı; `//go:embed defaults` deseni otomatik gömüyor (ek kayıt gerekmedi).

`go build ./...` temiz; `go test ./internal/skills ./internal/agent ./internal/api` (197) temiz.
Yeni skill'in seed + parse + shared-yüklenebilir olduğu geçici testle doğrulandı (sonra silindi).
Tahmini statik-prefix kazancı: deliverables ~250-300 token + her CLI tur deferred-tekrar ~40-60
token ≈ **~300-360 token/tur**; asıl kazanç deliverables bloğunun her turdan kalkmasıyla
**dikkat/context-rot azalması**.

## Eager araç şemalarını sadeleştirme (run_subagent / create_artifact / core_memory) ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu. Araç JSON şemaları en ağır segment (~%56).
Bilinçli **eager** (her tur tam şemayla giden, "behavioral nudge") üç araç şişkin:
`run_subagent` (şemada 3 örnek + uzun field açıklamaları), `create_artifact` (uzun açıklama),
`core_memory_append`/`replace` (neredeyse aynı uzun açıklamayı tekrarlıyor).

**Yapılan (Strateji A — eager kalır, sıfır davranış riski):**

- `internal/tools/subagent.go`: `run_subagent` açıklaması ~yarıya, field açıklamaları kısaltıldı;
  `Examples` **3 → 1** (örnekler `foldExamples` ile her tur eager şemaya katlanıyordu → en büyük kalem).
- `internal/tools/builtin_artifact.go`: `create_artifact` açıklaması kısaltıldı (image/binary + base64-etme uyarısı korundu).
- `internal/tools/builtin_memory_core.go`: ortak core-memory tanımı tek `coreMemoryDesc` const'una çıkarıldı; her açıklama ortak cümle + role özgü satıra indi. **Lazy yapılmadı** (bağlam-basıncı anında lazım).

`go build ./internal/tools/...` + `go test ./internal/tools/...` (152) temiz; şema/örnek JSON geçerliliği doğrulandı.
Tahmini tasarruf: ~1.3 KB / eager tur ≈ **~300-350 token**. Strateji B (lazy + prompt nudge) öneri olarak bırakıldı.
Detay: `_Docs/19-LAZY-TOOL-LOADING.md`.

## Dış MCP araçları katalogda yalnız-ad (NameOnly) ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu. "Available Tools (load on demand)" bloğunda
dış MCP araçları (ör. `mcp__browser-mcp__*`) tam açıklamalarıyla dökülüyordu; TionHarness'in
kendi `tionharness_extended` araçları ise zaten yalnız-ad. Tutarsızlık + her tur ölü token.

**Yapılan:** `internal/tools/registry.go` `AttachMCP` artık her MCP aracını `lazy` **VE**
`nameOnly` işaretliyor (tek satırlık ekleme: `r.nameOnly[e.NamespacedName] = true`).
Mevcut `VisibleLazyCatalog`/`writeLazyToolLine` mekanizması açıklamayı boşaltıp yalnız
`- \`mcp__server__tool\``basıyor. İsimler listede kaldığı için`tool_search`/`activate_tools`ve CLI`ToolSearch select:<name>`ile araçlar hâlâ keşfedilip yüklenir.`go build ./internal/tools/...`

- `go test ./internal/tools/...` temiz. Tahmini tasarruf: chrome ~30 araç için ~800–1200 token/tur.
  Detay: `_Docs/19-LAZY-TOOL-LOADING.md`.

**Manuel override (UI):** MCP araçları artık varsayılan NameOnly olduğundan, kullanıcının
bunu sunucu bazında geri alabilmesi için `ToolsPanel` MCP sunucu kartına her satırda
**"Tümü NameOnly" / "Tümü Göster"** hızlı eylemi (+ eager/total sayacı) eklendi. Backend
değişmedi — mevcut `setWorkspaceToolsVisibility` override'ı kullanılıyor. `tsc --noEmit` temiz.

## Built-in araçlar fonksiyonel kategorilere gruplandı ✅ (2026-07-01)

**İstek:** Tools ekranında ~85 built-in araç tek "Yerleşik" grubunda akıyordu (MCP'ler
sunucu başına gruplanırken). Tutarsız ve taranması zor.

**Yapılan (tek-kaynak, backend → frontend):**

- **Backend:** `internal/tools/categories.go` — `CategoryOf(name) string`, isim→kategori
  açık eşlemesi (10 fonksiyonel anahtar: `files`, `search`, `memory`, `agents`,
  `automation`, `interaction`, `artifacts`, `skills-mcp`, `config`, `diagnostics`;
  eşlenmemiş araç `other`). `internal/api/workspace_tools.go` her built-in'e `category`
  alanını basar; MCP araçlarında boş (onlar sunucuya göre gruplanır).
- **Frontend:** `WorkspaceTool.category` tipi; `toolMeta.ts`'te `toolCategory` +
  `CATEGORY_LABELS` (TR etiket) + `CATEGORY_ORDER` (sabit sıra). `ToolsPanel` `groups`
  memo'su built-in'leri kategoriye göre (sabit sırada), MCP'leri sunucuya göre
  (alfabetik) böler. Bilinmeyen kategori anahtarı ham haliyle sona düşer (graceful).
- `go build ./internal/tools/... ./internal/api/...` + `tsc --noEmit` temiz. Yeni araç
  eklenince `categories.go`'ya bir satır eklenmeli (yoksa "Diğer" altında görünür).
- **Güncelleme (2026-08-28):** Bu sözleşme artık testle zorlanıyor —
  `internal/tools/categories_coverage_test.go`, paketin `Def()` gövdelerini AST ile
  tarayıp her yerleşik aracın haritada olduğunu (`TestEveryBuiltinToolHasACategory`)
  ve haritada ölü girdi kalmadığını (`TestNoStaleCategoryEntries`) doğrular. Aynı
  turda haritada eksik olan 18 araç (coordinator/worker altılısı, automation CRUD,
  insight üçlüsü, lessons ikilisi, `apply_patch`, `run_code`, `render_template`)
  eklendi.

## Tam temizlik: bütçe/limit sistemi + ölü ayarlar backend'den söküldü ✅ (2026-07-01)

UI kaldırıldıktan sonra backend kalıntıları da tamamen temizlendi.

**Bütçe/limit sistemi (tamamen kaldırıldı — ajanlar artık koşulsuz sınırsız):**

- `db.Agent.DailyCallLimit`/`DailyTokenLimit` alanları (`db/models.go`) + `db.UpdateBudget`
  (`db/store_usage.go`) silindi.
- `agent/budget.go`: `ErrBudgetExceeded`, `ensureBudget`, `billableTokens` silindi;
  `guardedComplete` artık yalnız global `pauseAutonomy`/`Paused` frenini uyguluyor.
  `agent/toolloop.go`'daki iki per-iterasyon bütçe kapısı + `recovery.go termBudget`
  sabiti + `budget_test.go TestBillableTokens` kaldırıldı.
- API: `POST /api/agents/{id}/budget` endpoint'i + `handleSetBudget`/`setBudgetReq`
  (`api/usage.go`, route `server.go`) silindi; `handleAgentUsage` + `agentBudgetRow`
  (`api/budget.go`) artık limit alanı döndürmüyor; `api/agents.go` varsayılan-bütçe
  tohumlaması kaldırıldı; `api/templates.go` + `api/market.go` + `market/pack.go` template
  agent limit alanları söküldü, `subagent_test.go` güncellendi.
- Settings: `DefaultDailyCallLimit`/`DefaultDailyTokenLimit` (struct/Default/DTO/Patch +
  store apply/clamp) kaldırıldı.
- Frontend: `types/agent.ts` (AgentUsage), `types/usage.ts` (BudgetAgentRow),
  `types/market.ts`, `types/settings.ts` limit alanları + `ChatMeters.tsx` overBudget
  mantığı temizlendi (spend pill yalnız çağrı+maliyet gösteriyor). **Spend takibi
  (RecordUsage + Bütçe ekranı) korunur** — yalnız _limit_ kavramı gitti.

**Ölü ayarlar (`mcpGatewayUrl`, `logLevel`):** settings.go (struct/Default/DTO/Patch),
store.go (apply + logLevel clamp), validate.go (+ validate_test.go logLevel case) ve
frontend `types/settings.ts` + save payload'undan tamamen kaldırıldı.

**Doğrulama:** `go build ./...` ✅, `go vet ./...` temiz ✅, `go test` 208 test ✅,
frontend `tsc --noEmit` temiz ✅. Skill dökümanları (`tionharness-settings`,
`tionharness-autonomous-ops`) güncellendi.

## Antigravity CLI + Gemini CLI provider'ları tamamen kaldırıldı (2026-07-03)

Deneysel Google dış-ajan adaptörleri (Antigravity CLI `agy` ve daha önce denenen
Gemini CLI) **projeden ve dokümanlardan tamamen kaldırıldı.** Antigravity CLI, agy'nin
doğrulanmış non-TTY stdout bug'ı (google-antigravity/antigravity-cli#76 — pipe/subprocess
altında yanıtı sessizce düşürüyordu) nedeniyle hiçbir zaman üretim-hazır olamadı; Gemini
CLI ise daha önce OAuth yönlendirmesi yüzünden bırakılmıştı.

**Kaldırılanlar:**

- Dosyalar: `providers/antigravitycli.go`, `antigravitycli_live_test.go`, `kind_antigravity.go`.
- `registry.go`: `antigravityCLIPath`/`antigravityKey` alanları, `findAgy`,
  `SetAntigravityCLIPath`/`SetAntigravityKey`/`AntigravityCLIAvailable`, `NewRegistry`
  seeding'i ve `resolve()` doldurma; kullanılmayan `os` importu düştü.
- `kind.go`: `ResolvedConfig`'ten `AntigravityCLIPath`/`AntigravityKey`.
- `api/session_context.go`: CLI-overhead önizlemesi yalnız `claude-cli`'ye daraltıldı.
- `kind_test.go`: katalog artık **5 kind** (`claude-cli`/`anthropic`/`minimax`/
  `minimax-anthropic`/`openrouter`); yorumlar (`builtin_websearch.go`, `toolsetup.go`,
  `interaction/server.go`) ve frontend yorumları (`session.ts`, `SessionContextModal.tsx`)
  temizlendi.

**Doğrulama:** `go build ./...` ✅, `go test ./internal/providers` ✅ (72 test).
Not: OpenRouter üzerinden erişilen Gemini **modelleri** (pricing/context-window/katalog)
bir CLI provider'ı değil — meşru model referansları olarak korundu.

## Bağlam ölçeri: araç çağrısı/sonucu trafiği artık sayılıyor

- **Ölçüm:** tool-yoğun bir oturumda (4 mesaj, 197 araç çağrısı) mesaj metinleri
  4.689 karakter (~1.2k token) iken `db.Message.Steps` izleri 236.405 karakter
  (~59k token) tutuyordu. Panel bu farkın hiçbirini göstermiyordu.
- **Doğrulama (önce):** `Steps`'in tamamı sağlayıcıya **gönderilmiyor**.
  `conversation.toProviderMessages` (`internal/conversation/manager.go:496-504`)
  saklanan bir turu yalnız `Text` (+ ek dosyalar) olarak provider mesajına
  çeviriyor; iz `providers.Request.Messages`'a hiç girmiyor. Bu yüzden
  `EstimateTokens` **değiştirilmedi** — ham izi saymak, hiç harcanmamış tokenlar
  için sıkıştırmayı erken tetiklerdi.
- **Gerçekten gönderilen kısım:** `composeTurnRequest`
  (`internal/api/chat_turn.go:65-67`) son asistan turlarının izlerinden üretilen
  **sınırlı** `<recent_tool_activity>` özetini (`recentToolActivityBlock`, en fazla
  4 tur × 10 araç × 240 rune çıktı) volatil dinamik ekte gönderiyor. Bu blok
  hiçbir yerde sayılmıyordu — ne ölçerde ne de katlama eşiğinde.
- **Çözüm:** `systemFillers` artık `history` alıyor ve bu özeti kendi kovası olarak
  ekliyor: `role:"tool-activity"`, etiket **"Araç çağrıları ve sonuçları"**,
  `Count` = özetteki araç satırı sayısı (`countToolRecapLines`). Frontend'de
  `ROLE_COLORS['tool-activity']` (`#7c3aed`) ile kendi rengi var. `systemFillers`
  aynı zamanda `contextOverheadTokens`'ın tabanı olduğundan katlama eşiği de artık
  bu maliyeti görüyor — ölçer ile motor tek kaynakta kalmayı sürdürüyor.
- **Kapsam notu:** ham `Steps` izi bu ölçerde görünmez ve görünmemeli; o yalnız
  diskte/UI transkriptinde yaşar. Ölçerde çıkan tek şey modele giden özettir.
- **Testler:** `conversation.TestStepsAreNotSentAndNotEstimated` (izin provider
  mesajına sızmadığını ve tahmini şişirmediğini sabitler),
  `api.TestToolActivityRecapIsCounted` + `TestCountToolRecapLinesIgnoresNonToolTurns`.
  `go test ./internal/conversation/... ./internal/api/... -count=1` ✅

## CLI sağlayıcılarında MCP sunucu kapısı (2026-08-21)

- **Sorun:** `claude-cli`/`codex-cli` ajanları, ajan araç kısıtına (`allowedTools`
  / `blockedTools` / `toolOverrides`) bakılmaksızın **enabled olan her MCP
  sunucusunu** kullanabiliyordu; UI ise aynı araçları "blocked" gösteriyordu.
  Canlı örnek SES948 (Playwright + codebase-memory).
- **Çözüm:** `internal/agent/mcpservergate.go` — önek tabanlı sunucu kapısı;
  `writeCLIMCPConfig` ve `codexMCPSpec` artık `ag db.Agent` alıp sunucuyu mount
  etmeden önce kapıya sorar. Ayrıntı ve sınırlar: `_Docs/52-MCP-GATEWAY.md`.
- **Testler:** `internal/agent/mcpservergate_test.go`.
  `go test ./internal/agent/ -count=1` ✅
- **Politika (2026-08-21):** yerleşik worker profilleri built-in-only kalır;
  28 mevcut `worker:*` ajanına migration yapılmadı. `validator`'ın yanlış
  "browser for e2e" yorumu düzeltildi. Sözleşme testi:
  `TestProfileAllowlistsNameNoMCPServer`.
- **codebase-memory muafiyeti (2026-08-21):** kod grafı allowlist'ten muaf altyapı
  sayıldı (`allowlistExemptServer`); worker'lar dahil her ajan erişir. Açık denylist
  ve workspace anahtarı hâlâ geçerli. `Capability.Detect` ajan-farkında yapıldı —
  ajanın çağıramadığı araç için prompt bloğu artık basılmıyor.

## Sohbet listesi: tek akış + çoklu çip filtresi (2026-08-21)

- **Değişiklik:** Sidebar'daki `Aktif / Workers / Arşiv` sekmeleri ve "Tümü" çipi
  kaldırıldı. Artık tek düz liste var; üstteki çipler **çoklu seçim** ve
  **hepsi varsayılan olarak seçili**. Kind çiplerine ek olarak iki kapsam çipi
  geldi: **Worker** (koordinatör tarafından spawn edilen oturumlar) ve **Arşiv**.
  Bir çipi kapatmak o dilimi listeden gizler.
- **Kapsam (tam):** çipler her `Session.Kind`'i kapsar — eski sekmelerin dışarıda
  bıraktığı `flow-coordinator` ("Akış Koord.") ve `inbox` ("Inbox") için de çip
  var; tanınmayan bir kind **"Diğer"** çipine düşer (`kindChipKey` artık null
  dönmez), yani hiçbir oturum filtrelenemez durumda kalmaz.
- **Kalıcılık:** localStorage `tionharness.sessionChipsOff` **kapatılan** çipleri
  tutar (seçilenleri değil). Sonraki bir sürümde eklenen çip böylece açık başlar;
  kaydı olan kullanıcıda sessizce satır gizlemez.
- **Nerede:** `sessionKindMeta.tsx` (`SESSION_CHIPS`, `ALL_SESSION_CHIPS`,
  `normalizeChipsOff`, `kindChipKey`, `sessionMatchesChips`,
  `SESSION_CHIPS_OFF_KEY`), `SessionsSidebar.tsx` (yerel + localStorage'a yazılan çip
  durumu; `tabStats`/`TabActivity` silindi).
- **URL:** `?list=`/`?kind=` query'leri ve `routeQueryForView` tamamen kaldırıldı
  (`url.ts`, `useAppNavigation.ts`, `useDeepLinks.ts`, `App.tsx`). Çip seçimi
  deep-link değil, yerel görünüm durumudur.
- **Yan etki:** İçgörü oturumları artık varsayılan listede görünür (eski "sadece
  kendi çipinde" opt-in davranışı sidebar'da geçerli değil); istenmiyorsa İçgörü
  çipi kapatılır. Toplu "Arşivden çıkar" butonu, seçimin tamamı arşivliyse çıkar.
- **Doğrulama:** `npx tsc --noEmit` ✅, `npm test` (18 dosya / 190 test) ✅,
  `npm run format:check` ✅, canlı UI (5173) çipler basılı + worker/arşiv satırları
  listede.

## Toplu "Oturumlar" tablosu kaldırıldı (2026-08-21)

- **Ne:** Sohbet sidebar'ındaki `Oturumlar` butonu ve açtığı toplu tablo overlay'i
  tamamen kaldırıldı — çip filtreli tek liste ihtiyacı karşılıyor.
- **Silinen:** `features/sessions/SessionsOverview.tsx`, `App.tsx`'teki
  `overviewOpen` state + render bloğu, `SessionsSidebar` `onOpenOverview` prop'u ve
  `data-testid="sessions-overview-open"` butonu. `useExecutionRuntime` artık
  yalnız `runtimeById` döner (`executions` dizisi tablonun tek tüketicisiydi).
  Ölü kalan `FILTERS`, `matchesKindFilter`, `shortId` de `sessionKindMeta.tsx`'ten
  temizlendi; kind çipleri artık `SESSION_CHIPS` içinde doğrudan tanımlı.
- **Not:** Ajan/E2E senaryolarında `sessions-overview-open` seçicisi artık yok.
- **Doğrulama:** `npx tsc --noEmit` ✅, `npm test` 190/190 ✅, `format:check` ✅,
  canlı UI'da buton yok, konsol temiz.

## Debug butonu chat header'dan "Oturum bilgisi" paneline taşındı (2026-08-25)

- **Ne:** Sohbet başlığındaki `Bug` ikonlu "Debug" butonu kaldırıldı; aynı
  `SessionDebugModal`'ı açan aksiyon artık "Oturum bilgisi" panelinin en altındaki
  **Araçlar** bloğunda, "Debug / gözlemlenebilirlik" etiketiyle duruyor (sabitle /
  arşivle / sil butonlarının üstünde).
- **Değişen:** `AppHeader.tsx` — buton, `onOpenDebug` prop'u ve `Bug` importu
  silindi. `SessionDetailPanel.tsx` — opsiyonel `onOpenDebug` prop'u + `ActionBtn`
  eklendi. `App.tsx` — `onOpenDebug={() => setDebugOpen(true)}` artık
  `SessionDetailPanel`'e geçiliyor; `debugOpen` state ve modal render'ı aynen kaldı.
- **Not:** Panel gizliyken (Detay kapalı) Debug'a erişim de kapanır — modalın
  kendisi bağımsız render edildiği için açıkken panel kapatılsa bile çalışır.
- **Doğrulama:** `npx tsc --noEmit` ✅, `format:check` ✅.

## TSK270 — Sistem ajanları dokümantasyonu (2026-08-27)

- **Ne:** TSK260–TSK269 ve TSK271 ile uygulama içi LLM işleri için dört kararlı
  sistem ajanı (`titler`, `compactor`, `lesson-extractor`, `insight`) eklendi;
  workspace özelleştirmesi, yerleşik fallback, yönetim UI'ı ve usage ayrımı tek
  zincirde tamamlandı.
- **Değişen:** Derlenmiş registry ve idempotent workspace seed; etkin workspace
  profili/yerleşik tanım çözümlemesi; silme için HTTP 409 koruması; profil alanlarını
  derlenmiş değerlere döndüren restore-default endpoint'i; sistem oturumlarını ders
  ve içgörü adaylığından çıkaran özyineleme koruması;
  `system:<SystemKey>:<call-kind>` usage
  taksonomisi; frontend'de ayrı sistem ajanları bölümü, fallback rozeti, disable ve
  restore kontrolleri. Ayrıntı: `_Docs/74-SISTEM-AJANLARI.md`.
- **Not:** `insight` varsayılan olarak devre dışıdır. Sistem ajanını disable etmek
  işi durdurmaz veya kaydı silmez; derlenmiş fallback'i etkinleştirir. Doğruluk
  kaynağı `Agent.System`/`SystemKey` alanlarıdır; `Session` modeline alan eklenmedi.
- **Doküman:** Kavram, resolver/fallback, restore/disable, API, UI, seed,
  özyineleme koruması, usage taksonomisi ve kaynak dosya haritası
  `_Docs/74-SISTEM-AJANLARI.md` içinde toplandı; doküman dizinine eklendi.

- **Ne:** Yerleşik subagent/worker profilleri (`explore`, `planner`, `coder`,
  `reviewer`, `validator`, `config`) sistem ajanına çevrildi; `spawn_worker` artık
  `worker:<profil>` ajanı yaratmıyor, `subagent-<profil>` sistem ajanını hedefliyor.
- **Değişen:** `systemAgentDefaults`'a altı girdi (prompt registry + profil
  allowlist'i); `subagentProfile()` promptu `ResolveSystemAgent` ile çözüyor
  (hata → uyarı logu + registry fallback); `resolveWorkerTarget` sistem ajanının
  id'sini döndürüyor; `applyProfileAllowlist` spawn ve follow-up turlarında
  allowlist'i koddan yeniden dayatıyor; `EnsureSystemAgents` eski `worker:*`
  ajanlarını idempotent biçimde göç ettiriyor; UI'da bu ajanların araç bölümü
  kilitli.
- **Not:** Provider/model dinamik kaldı — sistem ajanı model pinlemiyor, worker
  koordinatörün provider/instance/model/permission değerlerini klonluyor.
- **Doğrulama:** `go build ./...` ✅, `go test ./internal/agent/... ./internal/api/...
./internal/prompts/... ./internal/db/...` ✅, `npm run format:check` + `npm test` ✅.
  Ayrıntı: `_Docs/74-SISTEM-AJANLARI.md`, `_Docs/47-KOORDINATOR-COKLU-AJAN.md` §17.

## TSK335 — Tema denetimi B+C: overlay token'ı + toggle knob (2026-08-27)

- **Ne:** Denetimin en düşük riskli dilimi uygulandı: sabit `bg-black/*` scrim'leri
  ve `bg-white` toggle knob'ları tema token'larına bağlandı, `ArtifactsPanel`
  "daha fazla yükle" hover regresyonu düzeltildi.
- **Yeni token — `--color-overlay`:** `frontend/src/index.css` içinde HEM koyu
  (`@theme`, `#000000`) HEM açık (`[data-theme='light']`, `#16202c`) bloğuna
  eklendi. Açık temada saf siyah sert okunduğu için scrim slate tonlu. Kullanım:
  `bg-[var(--color-overlay)]/50` (opaklığı tüketici belirler). `website/src/styles/theme.css`
  mirror'ına da eklendi (CLAUDE.md senkron kuralı).
- **Değişen:** 9 dosyada `bg-black/{25,40,50,55,85}` → `bg-[var(--color-overlay)]/…`;
  5 toggle knob'da `bg-white` → `bg-[var(--color-text)]` (knob hem accent hem
  nötr `--color-border` ray üzerinde görünür kalmalı — `--color-on-accent` açık
  temada nötr ray üzerinde kayboluyordu); `ArtifactsPanel.tsx:778` base
  `--color-surface`, hover `--color-surface-2` (dosyanın kendi hover idiyomu),
  böylece hover koyu temada koyulaşmak yerine yükseliyor.
- **Kapsam dışı:** `relationGraph.ts` ve `palette.ts` renk KAYNAĞI dosyalarıdır
  (`themePresets.ts` gibi), token'a çevrilmedi. `ArtifactView`/`HtmlPreview`
  içindeki `bg-white`/`bg-black` kullanıcı içeriği tuvalidir, kasıtlı.
- **Doğrulama:** `npx tsc --noEmit` ✅, `npm run format:check` ✅,
  `git diff --check` ✅.

### Ek dilim — composer buton stilleri (`buttonStyles.ts`)

- **Yeni token — `--color-on-danger`:** `--color-on-success`/`--color-on-warning`
  ile aynı desende, HEM `@theme` (koyu: `var(--color-bg)`) HEM
  `[data-theme='light']` (`var(--color-surface)`) bloğuna eklendi. `var()`
  referansı olduğu için preset'ler `--color-bg`/`--color-surface`'i inline
  ezdiğinde de doğru çözülür; `themePresets.ts`'e alan eklemek gerekmedi.
- **Yeni token — `--color-info-soft`:** dolgu (surface) amaçlı sakin bilgi rengi;
  koyu `#1e3a5f`, açık `#dbe7f7`. `--color-info` (canlı, ikon/kenarlık için)
  dolgu olarak kullanılamadığı için ayrı token. Yine iki blokta birden.
- **Değişen:** `buttonStyles.ts:13,18,23` — `BTN_DANGER` ve `BTN_STOP_COMPACT`
  `text-white` → `text-[var(--color-on-danger)]`; `BTN_QUEUE`
  `bg-[#1e3a5f] text-white hover:bg-[#264a75]` →
  `bg-[var(--color-info-soft)] text-[var(--color-text)] hover:opacity-90`
  (hover artık dosyadaki diğer butonlarla aynı idiyom).

## TSK386 — Shell alt proseslerinden credential sızıntısı kesildi (2026-08-28)

- **Teşhis:** `hardenShellCmd` (`internal/tools/builtin_shell_harden.go`)
  `cmd.Env`'i `nil` bırakıyor, `proc.HardenedEnv(nil)` de `os.Environ()`'ın
  tamamını alıyordu. Sonuç: ajanın çalıştırdığı **her** shell komutu ve alt
  prosesi `ANTHROPIC_API_KEY`, `CREDENTIAL_SECRET` ve kullanıcının tüm
  `*_TOKEN`/`*_KEY` değişkenlerini görüyordu; tek bir `env`/`printenv` hepsini
  LLM bağlamına, transkripte ve UI'a yazıyordu.
- **Çözüm — `internal/proc/env_credentials.go` (yeni):**
  - `IsCredentialEnvName(name)` — değişken **adına** bakan sınıflandırıcı
    (değer hiç incelenmez). Alt-dize kuralları (`SECRET`, `TOKEN`, `PASSWORD`,
    `PASSWD`, `PASSPHRASE`, `CREDENTIAL`, `APIKEY`, `API_KEY`, `ACCESS_KEY`,
    `PRIVATE_KEY`, `AUTH_KEY`, `SESSION_KEY`, `SIGNING_KEY`, `_KEY`) + sağlayıcı
    ön ekleri (`ANTHROPIC_`, `OPENAI_`, `OPENROUTER_`, `AZURE_OPENAI_`,
    `GEMINI_`, `GROQ_`, `MISTRAL_`, `DEEPSEEK_`, `XAI_`, `HUGGINGFACE_`, `HF_`)
    - tekil adlar (`CREDENTIAL_SECRET`, `GITHUB_TOKEN`, `GH_TOKEN`, `NPM_AUTH`,
      `PGPASSFILE`). Eşleşme büyük/küçük harf duyarsız.
  - `StripCredentialEnv(env)` / `CredentialSafeEnv(base)` — filtrelenmiş env +
    `TIONHARNESS_STRIPPED_ENV` işaretçisi.
- **Neden denylist (allowlist değil):** `transform_data`'nın katı allowlist'i
  (`transform_data_env.go`) tek bir yorumlayıcıyı başlatmaya yeter; shell ise
  keyfi komut çalıştırır ve `GOPATH`, `JAVA_HOME`, `NVM_DIR`, proxy, locale,
  toolchain değişkenlerine ihtiyaç duyar. Eksik kalan bir değişken komutu
  **sessizce** kırar; bu yüzden shell için bilinen credential desenleri
  reddedilir, gerisi geçirilir. Bedeli listelenmemiş bir sır şeklinin
  kaçabilmesidir — desenler bu yüzden bilinçli olarak geniş tutuldu.
- **Sessiz yutma yok:** her komutta log basmak yerine çocuğun env'ine
  `TIONHARNESS_STRIPPED_ENV=<soyulan adlar, virgüllü>` enjekte edilir. Bir komut
  kimlik doğrulayamadığında `echo $TIONHARNESS_STRIPPED_ENV` neyin esirgendiğini
  tam olarak söyler. Ebeveynden **miras alınan** işaretçi düşürülür (çocuğun
  işaretçisi yalnız kendi filtresini anlatır).
- **Bağlanan yer:** `builtin_shell_harden.go:26` →
  `proc.HardenedEnv(proc.CredentialSafeEnv(cmd.Env))`. Bash + PowerShell,
  foreground + `run_in_background` hepsi bu tek closure'dan geçtiği için
  kapsam tamdır.
- **Testler:** `internal/proc/env_credentials_test.go`
  (`TestCredentialSafeEnvStripsSecretsKeepsPath` — sır adı _ve değeri_ yok,
  `PATH`/`GOPATH` duruyor, işaretçi doğru; `TestCredentialSafeEnvDropsInheritedMarker`;
  `TestIsCredentialEnvName` — pozitif/negatif set) ve
  `internal/tools/builtin_shell_harden_test.go`
  (`TestHardenShellCmdStripsCredentials` — gerçek `exec.Cmd` env'inde credential
  yok, `PATH` ve `GIT_EDITOR=true` guard'ı duruyor).
- **Kapsam dışı — CLI sağlayıcı env'i:** `providers/claudecli.go` `cliBaseEnv` ve
  `codexcli.go` `codexBaseEnv` hâlâ `os.Environ()` üzerinden `ANTHROPIC_API_KEY`
  geçiriyor. Bu **kasıtlı olabilir** (CLI kendi auth'unu oradan alır); soymak
  sağlayıcıyı komple kırardı. Ayrı bir karar/kart gerektiriyor, bu iş kapsamında
  dokunulmadı.

## TSK388 — CLI sağlayıcı alt proseslerinde credential filtresi (2026-08-28)

TSK386'nın "kapsam dışı" bıraktığı kart: `cliBaseEnv` (`providers/claudecli.go`)
ve `codexBaseEnv` (`providers/codexcli.go`) `os.Environ()`'ın tamamını çocuğa
veriyordu — `CREDENTIAL_SECRET`, `GITHUB_TOKEN`, AWS anahtarları ve **diğer**
sağlayıcıların anahtarları dahil.

- **Tehdit modeli shell'den farklı:** env keyfi bir ajan komutuna değil, tek bir
  bilinen binary'ye (claude / codex) gidiyor ve o binary auth'unu kendi vendor
  namespace'inden alıyor. Bu yüzden körlemesine `CredentialSafeEnv` uygulanmadı.
- **Yeni yardımcı:** `proc.CredentialSafeEnvExcept(base, exempt)` — muafiyet
  yordamı alan `CredentialSafeEnv` varyantı (`internal/proc/env_credentials.go`).
  `StripCredentialEnv` içi `stripCredentialEnv(env, exempt)`'e taşındı; dış
  imza değişmedi.
- **Muafiyetler (`internal/providers/cli_env.go`), kanıtla:**
  - claude → `ANTHROPIC_` ön eki. Kanıt: CLI'nin auth önceliği
    `ANTHROPIC_API_KEY` > `CLAUDE_CODE_OAUTH_TOKEN` > config-dir
    `.credentials.json` (`_Docs/05-ARSIV.md`); TionHarness `authKind="apikey"`
    için zaten `ANTHROPIC_API_KEY` enjekte ediyor. `authKind` ayarlamayıp
    ortamdan `ANTHROPIC_API_KEY` export eden kullanıcı desteklenen bir kurulum,
    soymak onu kırardı. `ANTHROPIC_BASE_URL` / Bedrock-Vertex uçları da aynı
    gerekçeyle geçiyor.
  - codex → `OPENAI_` ön eki. Kanıt: `OPENAI_API_KEY` belgelenmiş codex login
    kanalı (`_Docs/69-CODEX-CLI-SAGLAYICI.md`) ve claude'un aksine codex için
    backend enjeksiyon kanalı **yok** (`_Docs/70-…`) — miras alınan env veya
    önceden yapılmış `codex login` tek yol.
- **Nesting sızıntısı geri gelmedi:** `cliBaseEnv` `ANTHROPIC_MODEL`,
  `ANTHROPIC_SMALL_FAST_MODEL`, `ANTHROPIC_DEFAULT_*` ve tüm `CLAUDE_CODE_*`
  ön ekini credential filtresinden **önce** düşürüyor; muafiyet yalnız filtreden
  sağ kalanı belirliyor.
- **Sessiz yutma yok:** `TIONHARNESS_STRIPPED_ENV` işaretçisi CLI çocuğuna da
  enjekte ediliyor, soyulan adlar oradan okunabiliyor.
- **Testler:** `internal/providers/cli_env_test.go` —
  `TestCLIEnvExemptTable` (muaf/soyulan adları pinleyen tablo) ve
  `TestCLIBaseEnvFiltersCredentials` (gerçek `cliBaseEnv`/`codexBaseEnv` çıktısı:
  kendi anahtarı duruyor, yabancı anahtarlar yok, işaretçi doğru).
- **Kapsam dışı — ayrı kart:** `internal/exttools/update.go:42` ve
  `version.go:53` hâlâ `proc.HardenedEnv(nil)` kullanıyor (update-check alt
  prosesi); bu turda dokunulmadı.

## Tek seferlik schedule/otomasyon ateşlemeleri "Spawn" yerine "Otomasyon" cipine düşüyordu (2026-08-28) ✅

Zamanlamanın `sessionMode: "spawn"` yolu (`deliverSpawnedPrompt`,
`internal/agent/scheduler.go`) ve bir otomasyonun tek seferlik ateşi
(`sessionMode != "continue"`, `dispatchFire`'ın spawn dalı,
`internal/agent/automation.go`) her ateşlemede `SpawnSession`'ı **Kind ayarlamadan**
çağırıyordu → varsayılan `Kind = "spawned"` ile oturum kenar çubuğunda genel
"Spawn" cipine düşüyordu, "Otomasyon" cipine değil (`kindChipKey`,
`frontend/src/features/sessions/sessionKindMeta.ts`).

- **Naif düzeltme (`Kind: "schedule"`) yanlış çıktı:** `getOrCreateKindSession`
  (`internal/db/store.go`) yalnız `(agentID, Kind)` ile eşleşiyor, `SourceID`'ye
  bakmıyor — bu tek seferlik spawn oturumunu ajanın **paylaşılan** cron
  thread'iyle çakıştırıp sonraki `reuse`-modu ateşlemelerinin mesajlarını bu
  spawn oturumuna yazdırdı (`TestDeliverPrompt_SpawnModeOpensFreshSession`
  bunu yakaladı).
- **Gerçek düzeltme — iki yeni ayrık kind:**
  - `Kind = "schedule-run"` — schedule spawn-modu ateşi
    (`internal/agent/scheduler.go` `deliverSpawnedPrompt`).
  - `Kind = "automation-run"` (`internal/agent.SessionKindAutomationRun`,
    `internal/agent/automation_deliver.go`) — otomasyon tek seferlik ateşi
    (`internal/agent/automation.go` `dispatchFire`, yalnız `spawn.Kind == ""`
    iken doldurulur).
  - Her ikisi de backend `writableSessionKindList`
    (`internal/db/models.go`) ve frontend `isWritableSessionKind`
    (`frontend/src/shared/lib/sessionKind.ts`) listesine eklendi — `"spawned"`
    gibi insan devam ettirebilsin diye yazılabilir.
  - `sessionKindMeta.ts`'te `KIND_META` + `kindChipKey`: ikisi de `"Otomasyon"`
    etiketiyle `automation` cipine gruplanıyor (ikon: `automation-run` → Zap,
    `schedule-run` → Clock, kaynağını ayırt etmek için).
- **`automation-run`'ın `"automation"`'ı DEĞİL, yeni bir kind olması bilinçli:**
  `OnUsageRecorded`'daki self-amplification guard (`crossingIsMaint`,
  `internal/agent/automation.go`) `s.Kind == SessionKindAutomation` kontrolüyle
  yalnız kalıcı bakım oturumunun kendi harcamasını session-scope token eşiğinden
  muaf tutuyor; tek seferlik bir spawn'ı da `"automation"` etiketlemek bu
  muafiyeti yanlışlıkla ona da uygulayıp meşru bir token geçişini yutardı.
- **Değişen dosyalar:** `internal/agent/scheduler.go`,
  `internal/agent/automation.go`, `internal/agent/automation_deliver.go`,
  `internal/db/models.go`, `frontend/src/shared/lib/sessionKind.ts`,
  `frontend/src/features/sessions/sessionKindMeta.ts`,
  `internal/agent/scheduler_sessionmode_test.go` (beklenen kind
  `"spawned"` → `"schedule-run"`), `_Docs/20-SCHEDULE-WAKE.md`.
- **Testler:** `go test ./...` (tam takım) + `frontend`: `tsc -b --noEmit`,
  `vitest run` (323/323) — hepsi yeşil.

## TSK466 — Navigasyon yerleşimi: Workspace Logları + üst seviye Promptlar & Dosyalar (2026-08-30)

- **Loglar** bağımsız NavRail öğesinden çıkarılıp `Workspace ▸ Loglar` sekmesine taşındı.
  `LogsPanel` burada kendi başlığı ve tam genişlikte kaydırma alanıyla render edilir.
- **Promptlar & Dosyalar** workspace alt sekmesinden çıkarılıp üst seviye NavRail görünümü oldu;
  mevcut `WorkspaceFilesPanel` ve kaydet/kirli-durum davranışı korundu.
- Eski `#/w/{workspace}/logs` bağlantıları `workspace/logs`'a,
  `#/w/{workspace}/workspace/files` bağlantıları üst seviye `prompts` görünümüne yönlenir.
- `focus_view` sözleşmesi canlı üst seviye görünümle eşitlendi: `prompts` eklendi, bağımsız
  `logs` kaldırıldı.

## TSK489 — Kart worktree base ref ve dirty koruması (2026-08-30)

- Koşulsuz `main` fallback kaldırıldı. Dolu `worktreeBaseRef`,
  `git rev-parse --verify <ref>^{commit}` ile doğrulanır; boş ayar gerçek HEAD dalını
  `git symbolic-ref --quiet --short HEAD` ile çözer. Detached/unborn HEAD açık hatadır;
  `main`/`master` tahmini yoktur.
- Çözülen ref karta `Task.WorktreeBaseRef` olarak yazılır; provision, merge ve discard
  aynı ref'i kullanır.
- Merge öncesi base dirty kapısı `git status --porcelain` ile staged, unstaged ve
  untracked değişikliklerin tümünü reddeder. Dirty durumda merge/worktree remove/branch
  delete, otomatik stash/reset veya force cleanup yapılmaz; görev worktree'si ve dalı korunur.
- Workspace worktree ayarları açılışta snapshot edilir. `worktreeBaseRef` değişikliği
  mevcut workspace yeniden açıldıktan sonra yeni yaşam döngüsünde geçerli olur.

## CLI compaction görünürlüğü ve summary-backed restart (2026-08-30)

- Compaction kartı `source`, `provider` ve `sessionAction` provenance alanlarını
  kalıcı taşır. Codex 0.148.0 `exec --json` wire sözleşmesindeki
  `context_compaction` item'ı parse edilir: `item.started` canlı çalışan kart,
  `item.completed` aynı ID ile tek kalıcı `cli-native/native-compact` karttır.
  App Server'ın `contextCompaction` camelCase sözleşmesi ayrı transporttur.
- Claude Code 2.1.238'de `--include-hook-events` etkinleştirilir. Native
  `PreCompact` hook başlangıcı çalışan karttır; `compact_boundary` birincil,
  `PostCompact` hook yanıtı ek tamamlanma kanıtıdır. İkisi aynı turda gelirse
  tamamlanma deduplicate edilir. Event token sayısı vermiyorsa tahmin yapılmaz.
- Claude `PreCompact` sonrasında iki dakika içinde completion kanıtı gelmezse
  çalışan kart tombstone ile geri çekilir; rolling-summary fallback değişmez.
- Claude Code 2.1.238 `system/status` lifecycle'ı da aynı korelasyona katılır:
  `status=compacting` çalışan kartı başlatır, `compact_result=failed` onu geri çeker,
  `compact_result=success` tek `cli-native/native-compact` tamamlanması üretir.
  Ardından gelen `compact_boundary`/`PostCompact` aynı lifecycle için yinelenmez;
  native lifecycle rolling summary veya `SummaryMsgCount` değiştirmez.
- Capability kurulu CLI sürümüne göre fail-closed açılır: Codex `>=0.148.0`,
  Claude Code `>=2.1.238`. Eksik/eski binary'de rolling-summary fallback korunur.
  Running frame inflight/persisted trace'e girmez; tamamlanmış native event debug
  journal'a bir kez yazılır.
- Codex normal tek-katılımcılı turları izole resume home + `exec resume` ile delta
  gönderir. Persona/provider/model/statik prompt scope değişimi veya rollout
  yokluğu full summary-backed cold start yapar.
- TionHarness fold'u sonrası Codex/Claude resume edilmez. Auto, wake, side-chat ve
  manuel `/compact` yolları Claude persistent warm sürecini düşürür; manuel yol
  ayrıca DB resume metadata'sını temizler.
- Manuel `/compact` başarı mesajındaki boş `Steps: "[]"` kaldırıldı. Gerçek fold,
  `trigger=manual`, `source=tionharness` ve CLI için
  `sessionAction=restart-summary` içeren kalıcı `compaction` TurnStep yazar.
- TSK501'in eski fallback sözleşmesi TSK512 ile değiştirildi: Claude Code 2.1.238
  print transportunda `/compact`, yalnız
  mevcut `--resume` oturumuna stdin'in tamamı olarak gönderildiğinde native komut
  olur. Provider'ın normal sistem/dinamik/history render'ı kullanılmaz. Başarı
  yalnız `cli-native/native-compact` TurnStep yazar; `DebugCompaction` debug
  olayı yazılmaz (o olay custom fold ve reactive compaction yollarına aittir),
  native yolda yalnız hata durumunda `DebugError` yazılır; rolling summary
  ve `SummaryMsgCount` değişmez. TSK512 sonrası resume/capability yokluğu ve
  native hata açıkça döner; rolling-summary fallback yapılmaz. Eski TionHarness
  fold davranışı yalnız `/compact-custom` komutundadır.
- Doğrulama: provider/agent paket testleri, `go build ./...`, `go vet ./...`,
  frontend `tsc` + build + ilgili kart testleri + Prettier ve `git diff --check`
  geçti. Tam `internal/api` paketi, bu görev dışındaki görsel-artifact dalında
  `IMAGE_DECODE_FAILED` veren iki test nedeniyle kırmızı kaldı.

## TSK503 — Claude CLI chat/worker gerçek bağlam ayrımı (2026-08-31)

- Session context preview, debug journal'dan en yeni chat ve en yeni non-chat
  ölçümlerini bağımsız raporlar; çağrı-başı hesap her iki tur tipi için korunur.
- Eski `measuredTokens/calls/overheadTokens` chat alias'ı olarak korunur. Usage
  fallback ve tahmini ek yük yalnız chat'e uygulanır; worker değerinde ek yük
  çıkarımı yapılmaz.
- UI, “Gerçek bağlam — chat” ve worker kind bilgisini taşıyan ayrı “Gerçek bağlam
  — worker” satırlarını gösterir.

## TSK568 — Terminal worker session arşivleme (2026-08-31)

- Worker terminal `<task-notification>` mesajı koordinatör geçmişine kalıcı
  yazıldıktan sonra worker session doğrudan `State="archived"` yapılır.
- Bildirim persist hatasında arşivleme ve koordinatör turu tetikleme yapılmaz;
  hata sessiz yutulmaz.
- Alt koordinatörün `delegating` sahiplik akışı ve `send_to_worker` archived
  session davranışı değiştirilmedi.
- Başarılı persist/arşiv ve persist-hatasında arşivlememe regresyon testleri eklendi;
  mevcut eşzamanlı idle-fold lifecycle testleri korunur.

## Sayısal ayar alanlarında stale-write koruması (`NumberField`) (2026-08-31)

**Sorun:** `onChange={(e) => set('x', Number(e.target.value))}` yazan bir sayı
alanı, kullanıcı geçersiz bir şey yazdığında (boş dize, `-`, `1e`, aralık dışı)
taslağa sessizce **eski/yanlış** bir değer işler ve Kaydet bunu kalıcılaştırır —
kullanıcı ekranda gördüğü metinden farklı bir değerin kaydedildiğini fark etmez.

**Desen** (`frontend/src/features/settings/primitives.tsx`):

- `NumberField` — yazılan metni **kendi local state'inde** tutar, böylece ara
  girdi (`""`, `-`, `1e`) ekranda kalır ama taslağa `0`/`NaN` olarak yazılmaz.
  Her tuş vuruşunda `min`/`max` ve `Number.isFinite` doğrulaması yapar,
  reddedilen değerin gerekçesini alanın altında `role="alert"` ile gösterir ve
  `onChange`'i **yalnız geçerli** değerde çağırır. Dışarıdan gelen değişiklik
  (ayar yeniden yüklendi, sıfırlandı) metni tazeler; alanın kendi commit'leri
  bununla çakışmaz (`committed` ref).
- `useNumberValidity()` + `NumberValidityProvider` — geçersiz alan kimliklerini
  bir küme olarak tutar; `hasInvalid` doğruyken panelin **Kaydet** butonu
  kapatılır. Alan unmount olurken kendi kaydını temizler, yoksa Kaydet sonsuza
  dek kapalı kalırdı.

**Her panel kendi provider'ını kurar.** Provider, alanları *ve* Kaydet butonunu
birlikte render eden ekranda çağrılır; tek global provider yoktur:
`features/settings/SettingsPanel.tsx` ve `features/insight/SettingsTab.tsx`
kendi `useNumberValidity()`'lerini kurar, `features/insight/LessonsTab.tsx` de
kendi provider'ıyla sarar. `features/flows/NodeInspector.tsx` `NumberField`
kullanır ama **bilerek provider dışındadır** — orada `onPatch` canlıdır (ayrı
bir taslak + Kaydet adımı yoktur), yani stale-write yolu hiç oluşmaz;
`NumberField` provider yokluğunda `useContext` context'in `null` varsayılanını
döndürdüğü için sorunsuz çalışır (`validity?.report`), yalnız satır-içi
doğrulama gösterir.

**Kalıntı (henüz desene geçmedi):**

- `frontend/src/features/settings/HooksPanel.tsx:272` — `timeoutSec` hâlâ ham
  `Number(e.target.value)` ile yazılıyor (alan `min=1`/`max=120` iddia ediyor
  ama bu yalnız tarayıcı ipucu; taslağa yazımı engellemiyor).
- `frontend/src/features/schedules/AutomationFields.tsx:134,192,259` —
  `priority`, `threshold` ve `interval` `Number(...) || 0` ile yazılıyor; bu
  kalıp geçersiz girdiyi sessizce `0`'a çeviriyor ve `MIN_TOKEN_THRESHOLD` /
  `MIN_COUNTER_INTERVAL` alt sınırlarını atlıyor.

## TSK760 — Run-scope semantic progress watchdog (2026-09-01)

- Normal chat inner ve queue outer 120 dk hard cap aktif karar yolundan çıkarıldı;
  legacy settings/API alanları 0=disabled uyumluluğuyla korundu.
- Yarış güvenli tracker yalnız aynı run'ın anlamlı assistant/tool/child/terminal
  ilerlemesini sayıyor. Heartbeat, empty/duplicate frame, tekrar running/append,
  tombstone ve başka run event'i idle süresini yenilemiyor.
- Non-stream opaque provider/tool çağrıları ayrı bounded operation lease kullanıyor;
  completion tracker progress'i. Session info progress zaman/tür/sequence ve idle
  limitini yayımlıyor; frontend hard-limit ayar/gösterimini kaldırdı.
- Kısa-süre testleri progress ile hard sınır ötesi yaşamı, gerçek idle iptalini,
  noise/duplicate filtrelemeyi, run izolasyonunu, operation lease'i ve timer
  reset/Stop yarışlarını kapsıyor. Cooperative cancel + 30 sn detach yolu korundu.

### Bağımsız inceleme düzeltmeleri

- Spawn/worker/coordinator/peer/automation ve schedule/wake yollarındaki kalan
  `SpawnTimeout`/`ScheduleTimeout` toplam-süre contextleri kaldırıldı. Deprecated
  hard alanlar wire/storage için duruyor; default/disabled 0 ve normal yürütme
  kararı üretmiyor.
- Operation lease önceden iptal edilmiş context'te kullanıcı/provider/tool kodunu
  çağırmıyor; operation daima buffered-result goroutine'de. Deadline/cancel sınırı
  result yarışını deterministik kazanıyor ve geç sonuç uygulanmıyor. Go in-process
  üçüncü taraf goroutine'ini zorla öldüremez; 64-slot admission timeout sonrası çağrı
  gerçekten dönene kadar slotu tutuyor, doluluk typed `ErrOperationLeaseBusy` ile
  hızlı reddediliyor ve detached sayaç sınırı görünür kılıyor. Child goroutine'deki
  provider/tool panic'i value + stack taşıyan `OperationPanicError` sonucuna çevrilir;
  süreç çökmez, admission/detached accounting temizlenir ve deadline/cancel yarışı
  yine sınır nedenini deterministik döndürür.
- Session-generation kayıtta değil gerçek turn-slot sahipliğinde aktive ediliyor;
  kuyruğa giren B çalışan A'yı stale yapmıyor. B slotu alınca eski A'nın lifecycle,
  block/fail, live step, durable reply/error, inflight ve terminal hub yazıları aynı
  generation primitive'iyle no-op. Inflight clear `(runID,generation,messageID)` CAS;
  yeni run'ın recovery kaydı korunuyor.
- Fence server-geneli `chatRuns.mu` değil, workspace/session anahtarlı kalıcı
  gate'tir. Global mutex yalnız run/gate metadata lookup-create için kısa tutulur;
  aynı session activation + fenced write atomik, farklı session'lar bağımsızdır.
  Gate tüm registered/detached run referansları bitince registry'den kaldırılır.
  Auto-title provider çağrısı gate dışında aday üretir; title persist ve terminal
  yan etkiler current generation yeniden doğrulanınca fenced commit'te çalışır.
  Arada generation değişirse stale title ve terminal event düşürülür.
  Queue panic bariyeri de orijinal run/generation gate referansını recovery bitene
  kadar taşır; unregister sonrası yeni generation aktive olmuşsa eski panic'in
  durable error/reply, debug, turn_error ve hub commit yan etkileri topluca no-op.
- Legacy hard-limit regresyonu `runFor > idle > legacyHardLimit` düzeninde, idle'dan
  belirgin kısa aralıklı benzersiz semantic progress ile gerçek idle penceresini
  aşar. Eş no-progress kontrolü aynı idle eşiğinde iptali ayrıca kanıtlar.
- Settings UI deprecated `spawnTimeoutMin`/`scheduleTimeoutMin` alanlarını değiştirilebilir
  hard-cap kontrolleri olarak sunmaz. Alanlar wire/storage uyumluluğunda kalır ve
  `0 = disabled`; UI yalnız semantic progress/idle kararını açıklar.
- Semantic dedup source/step kimliği başına tutuluyor; alternating A/B duplicate,
  repeated Running ve boş/running subagent snapshot ilerleme değil. Yeni child
  output, meaningful substep artışı ve terminal geçiş sayılır. State 512 kaynakla
  sınırlı.
## Uzun koordinatör oturumu maliyet düşürme paketi (SES2570 türevi) (2026-09-01) ✅

20.7 saatlik tek koordinatör oturumunun (`SES2570`, codex-cli) ölçümünden
türetilen altı düzeltme. Oturum ağacı: 27 oturum, 8.95M in+out token, 74.7M
cache-read, `messages.jsonl` 12.33 MB, 5 fold, sıfır commit. Tam analiz ve her
düzeltmenin gerekçesi: `_Docs\47-KOORDINATOR-COKLU-AJAN.md` §19.

1. **Oturum-kapsamlı `use_skill` dedupe** — 174 skill yüklemesinin 167'si
   tekrardı (~560K token). `tools.SkillLedger` + `SkillReloadPointer`; fold
   epoch'u (`Session.CompactionCount`) doğruluk kapısı, `force: true` kaçış yolu.
   Native araç ve CLI köprüsü aynı defteri paylaşır.
2. **Spawn-zamanı yetenek kontrolü** — salt-okunur bir ajana verilen dosya-yazma
   brief'i artık spawn anında reddediliyor (`checkWorkerCapability`). Ayrıca
   `Registry.unknownToolMessage` izin sınırını "bilinmeyen araç"tan ayırıyor.
3. **Worker adım izi bildirimden ayrıldı** — `digestWorkerSteps` yalnız kartın
   render ettiğini (dosya değişiklikleri + todo) saklıyor; 12.33 MB'ın 8.2 MB'ı
   buydu ve hiç görüntülenmiyordu.
4. **Durum sorguları push'landı** — `coordinatorSituationBlock` fleet + ajan
   roster'ı (yetenek etiketli) + panoyu her tura enjekte ediyor ve **chat** yoluna
   da bağlandı (daha önce yalnız headless yolda vardı). 194 durum çağrısını
   hedefliyor.
5. **Doğrulama kapısı tur bütçesi** — `Task.ReviewBounces` + `ReviewRoundBudget`
   (3) + `<review-gate-exhausted>` bloğu; 7 saatlik reviewer koşu bandını kesiyor.
6. **Kapsam sözleşmesi** — kart ne yapar / ne YAPMAZ; kapsam dışı bulgu kartı
   bloklayamaz, yeni kart açılır. Prompt düzeyinde kural (`coordinator.md` +
   `orchestrator-doctrine` §9).

**Doğrulama turu rozeti (aynı gün, takip):** `ReviewBounces` artık kullanıcıya da
görünür — `TaskCard`'da `↻ N/3` rozeti (bütçe dolunca kırmızı), `TaskFormModal`'da
ne yapılacağını söyleyen salt-okunur bant, pano filtre çubuğunda `Doğrulama`
facet'i (`bounced` / `exhausted`), `get_view board` projeksiyonunda sinyal satırı
ve kart drill-down'ında tur sayısı. Bütçe sabiti `agent`'tan `db`'ye taşındı
(`db.ReviewRoundBudget`) — runtime ve view aynı kaynağı okuyor; frontend'deki
`REVIEW_ROUND_BUDGET` elle senkronlanan aynası. Detay:
`_Docs\67-BOARD-GORUNUMLERI.md` § "Doğrulama turu rozeti".

**Canlı doğrulamada çıkan üç düzeltme (2026-09-01, aynı gün):**

1. `ReviewBounces` yalnız `MoveTask`'ta sayılıyordu; kart sürükleme ve kart formu
   `PUT /api/tasks/{id}` → `UpdateTask` yolundan geçiyor ve sayaç hiç artmıyordu —
   yani rozet tam olarak insanın kullandığı yola görünmezdi. Kural
   `db.countReviewBounce`'a çıkarıldı ve iki yol da çağırıyor. Alan sunucu-sahipli:
   `UpdateTask` istemcinin gönderdiği `reviewBounces` değerini yok sayar.
2. Koordinatörün pano ve doğrulama-kapısı blokları `ListTasks` okuyordu →
   arşivlenmiş kartlar da sayılıyordu; blok 117 kartlık panoyu 270 kart olarak
   bildiriyordu. `ListActiveTasks`'a çevrildi (`get_view` zaten onu kullanıyor).
3. `frontend/src/features/artifacts/artifactGrouping.test.ts:109` — `Draft` tipini
   doğrudan `Record<string, unknown>`'a cast eden satır `tsc -b`'yi kırıyor ve
   `npm run build`'i tamamen bloke ediyordu. Gömülü `internal/web/dist` bu yüzden
   eski kalmıştı, yani derlenen binary güncel arayüzü hiç servis etmiyordu.
   `as unknown as` ile düzeltildi (TypeScript'in kendi önerdiği çözüm). Hata
   `8f026b4a` commit'inden beri duruyordu ve CI'daki `npm run build` adımı
   (`.gitea/workflows/ci.yml:83`) de bunu kırmızıya düşürmüş olmalı.

## Otomatik devam dürtmesi: durdurulan ve yarıda kesilen turlarda susuyor (2026-09-01) ✅

- **Sorun:** "⏰ Otomatik devam — Önceki turda görevi tamamlamadan durdun…" notu, turun
  neden bittiğine bakmadan yazılıyordu. Watchdog kesmesi (hard/idle), araç-iterasyon
  tavanı, guardrail durdurması veya bağlam/çıktı tükenmesi ile **yarıda kesilen** bir tur
  `reconcileTurnOutcome`'dan `err == nil` + `truncated == true` ile döndüğü için
  `maybeAutoContinue` yine de çalışıyor; kullanıcı hem "iş bitmedi" outcome notunu hem de
  onun hemen ardından ajanı aynı duvara geri süren dürtmeyi görüyordu. Aynı şekilde
  insan "Durdur"a bastığında (`CancelSession` → sade `context.Canceled`) döngü,
  deadline'a özgü "⏱️ Süre doldu…" notunu yazıyordu — yanlış gerekçe.
- **Fix — `internal/agent/autocontinue.go`:** `maybeAutoContinue` artık `truncated`
  parametresi alıyor ve yarıda kesilen turda hiç devam turu açmıyor (yalnız log).
  Döngü başındaki context kontrolü ikiye ayrıldı: **bütçe** dolduysa (yeni
  `deadlineExpired`: `context.DeadlineExceeded` / `ErrTurnHardTimeout` /
  `ErrTurnIdleTimeout` cause'u) eski "⏱️ Süre doldu…" notu korunuyor; **elle durdurma**
  ise sessizce çıkıyor — kullanıcının bilerek durdurduğu iş yeniden başlatılmıyor.
  Ek olarak devam turunun kendisi bir tavana takılırsa (`classifyTurnSteps` terminal
  işareti) döngü orada bitiyor.
- **Çağrı noktaları:** `runSpawn` (`outcome.Truncated()`), scheduler `deliverPrompt` ve
  `deliverAutomationTurn` (`truncated`) — üçü de yeni argümanı geçiriyor. Hata dalları
  (`err != nil`) zaten erken dönüyordu, davranışları değişmedi.
- **Test:** `TestAutoContinueSkipsStoppedAndCutShortTurns` (store'suz runtime ile: dürtme
  yazılsaydı panik ederdi) + `TestDeadlineExpired`. `go test ./internal/agent/ -count=1`
  yeşil.
