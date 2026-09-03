# 62 — Birleşik Run (C+D) · `await-input` keystone

> **Özet (2026-09-03):** Flow'ların çalışırken kalıcı olarak girdi bekleyebilmesini (durable await-input) sağlayan uygulanmış bir özelliktir; goroutine bloklamayan, restart-safe, CAS korumalı bir suspend/resume mekanizmasıdır (`orchestration.NodeAwaitInput`, `db.MarkFlowRunWaiting`/`ClaimWaitingFlowRun`, `POST /api/flow-runs/{id}/input`). Doküman zamanla genişletilmiş: peer/ajan köprüsü, timeout/GC süpürücü, subflow/spawn/join async kompozisyonu, koşu soyağacı (lineage) katman 1-4 ve ilgili UI (RunTreeView) hep buraya eklendi. En önemli kararlar: session interaction (ask/permission) ile flow await'i bilinçli olarak ayrı durability modelleri olarak tutuldu (birleştirilmedi), reuse-continuation launcher'lar (`deliverPrompt`/`schedule_wake`) `LaunchRun`'a bilinçli olarak katlanmadı. Dayandığı ana dosyalar: `internal/orchestration/engine.go`, `internal/agent/flow.go`, `internal/agent/launch.go`, `internal/db/store_flow*.go`.

> Session ⇄ flow birleşiminin (Model C: ortak Run substratı, Model D: çapraz-driver
> kompozisyon) **yürütülebilir çekirdeği**: flow'un durable olarak **girdi bekleyebilmesi**.
> Bir flow `[agent] → [await-input] → [agent]` artık **scripted bir konuşmadır** =
> "her session bir flow"un ilk somut adımı.

## Neden keystone

Dört seam paralel Explore ajanlarıyla haritalandı. **Kritik bulgu (ask/interaction):**
mevcut `openInteraction`/`waitInteractionCLI` primitifi **bellek-içi, kalıcı değil, goroutine
bloklar, 15dk timeout, otonom run'da bail** → uzun-ömürlü await için yanlış. Doğru yol
**durable**: engine suspend + persisted `waiting` statüsü + resume-via-endpoint.

## Mekanizma (backend)

- **Node:** `orchestration.NodeAwaitInput` (`"await-input"`), alanlar: `Next` + `Title`
  (beklerken etiket). `Validate` `Next` ref'ini denetler; paralel çocuğu **olamaz** (zaten
  Validate paralel çocuğu agent'a zorluyor).
- **State:** `State.WaitingAt` (await node id; "" = çalışıyor/terminal).
- **Suspend (`engine.go` `NodeAwaitInput` case):** ilk gelişte `st.WaitingAt = node.ID`,
  `notify("waiting")`, `save(st)`, `Run` **`(st, nil)` ile döner** (hata değil). `Current`
  await node'da **park** kalır. Döngü gövdesindeki suspend `runLoop`'tan yukarı propage olur
  (`Current` await'te; **loop resume sınırı:** resume'da döngü devam etmez, gövde kalanı bir
  kez koşup çıkar — dokümante).
- **Resume:** aynı state ile `Run` yeniden girilince `WaitingAt == Current == await` → enjekte
  edilen `st.Last` tüketilir, trace'e yazılır, `Current = Next`. Girdi `{{last}}` ile sonraki
  node'a taşınır (accumulate modda sonraki agent'ın prompt'u onu user turn'e çevirir).
- **DB (`db`):** `FlowWaiting = "waiting"` statüsü; `MarkFlowRunWaiting(id, state)` (state+status
  atomik); `ClaimWaitingFlowRun(id)` **CAS waiting→running** (çift-resume koruması);
  `ListRunningFlowRuns` **waiting'leri hariç tutar** → boot'ta orphan-diriltme yok.
- **Runtime (`flow.go`):** `driveFlow` terminal'den önce `final.WaitingAt != ""` ise
  `MarkFlowRunWaiting` + return (FinishFlowRun **yok**). `ResumeWaitingFlow(runID, input)`:
  `ClaimWaitingFlowRun` (CAS) → flow/graph/state yükle → `st.Last = input` → state persist
  (crash-during-resume'da girdi kaybolmasın) → `go driveFlow(...)`. Silinen flow/bozuk state →
  temiz `FinishFlowRun(failure)`.
- **API:** `POST /api/flow-runs/{id}/input` (`handleResumeFlowRun`) → `ResumeWaitingFlow`;
  not-waiting/already-resumed → **409**.

## Frontend (Model D UI)

- Palet 8. tip **"Girdi Bekle"** (`await-input`, sarı `MessageCircleQuestion`), `AwaitInputNode`
  (agent gibi tek-next), inspector açıklaması. `NodeStatus += 'waiting'` + sarı ring.
- `FlowRun.status += 'waiting'`, `FlowState.waitingAt`, `FlowNodeEvent.phase += 'waiting'`.
- **RunView waiting composer:** `run.status==='waiting'` iken sarı bar + input + "Gönder" →
  `api.resumeFlowRun(runId, input)`; `onResumed` runs listesini tazeler. await node canvas'ta
  waiting ring; statü çipi "⏳ girdi bekleniyor".
- Editörden "Çalıştır" zaten Koşular tab'ına yönlendiriyor → suspend olan koşu orada composer'la
  görünür.

## İstisna senaryoları (karşılanan)

1. **await'te crash/restart:** waiting persist; boot yalnız running'leri resume eder → uykuda kalır (orphan yok). ✅
2. **Bitmiş/başarısız run'a input:** `ClaimWaitingFlowRun` reddeder → 409. ✅
3. **Çift input (çok pencere/peer):** CAS waiting→running, ilk kazanır; kalan 409. ✅
4. **Sonsuz bekleme:** durable; goroutine bloklamaz (interaction'ın 15dk timeout'u YOK). Timeout/temizlik ileride (`ListWaitingFlowRuns` hazır).
5. **Parallel içinde await:** Validate paralel çocuğu agent'a zorluyor → imkânsız. ✅
6. **Resume çift-tetik goroutine yarışı:** CAS tek kazanan. ✅
7. **Silinen flow/agent:** resume'da temiz `failure`. ✅
8. **Otonom/bütçe:** resume interaktif kabul edilir (budget-gated değil); await'e ulaşan otonom koşu suspend olur, herhangi biri (insan/peer) input verebilir.
9. **Crash-during-resume:** girdi launch'tan ÖNCE persist → boot running'i state'te girdiyle resume eder. ✅
10. **UI:** waiting composer + sarı ring + statü çipi. ✅

## Test / doğrulama

- `orchestration/await_test.go`: suspend→resume ({{last}} tüketimi), Validate Next ref.
- `db/flow_waiting_test.go`: CAS çift-resume koruması, MarkWaiting, boot-no-orphan.
- Backend **992 test yeşil**; `tsc -b` + `vite build` yeşil.
- **Not:** backend değişikliği; canlı görmek için backend yeniden derlenip başlatılmalı.

## Faz 3 — Birleşik `LaunchRun` launcher seam'i (2026-07-26)

Tetik-tabanlı launcher'ların `if FlowID != "" { flow } else { session-spawn }` **dallanma kararı**
tek girişte toplandı: `internal/agent/launch.go` → `Runtime.LaunchRun(ctx, RunSpec) (LaunchResult, error)`.

- **RunSpec:** `{Trigger, Input, Autonomous, FlowID (flow driver), AgentID+Spawn (session driver)}`.
  `RunTrigger` sabitleri: `manual|schedule|automation:tag|automation:board`.
- **LaunchRun:** hedefi doğrular (flow→`GetFlow`, session→`GetAgent`), flow ise `RunFlowRecorded`
  + **flow-failure'ı error'a normalize eder** (her çağıran aynı şekilde `recordFailure`), session ise
  `SpawnSession`. `LaunchResult{Driver, SessionID, FlowRun}`.
- **Refactor edildi:** `automation.fire` + `automation.fireBoard` artık LaunchRun kullanıyor →
  `automation.fireFlow` **silindi** (dupe dispatch eritildi); bildirim driver-farkında (flow'da
  " (akış)" / " (pano·akış)" son eki). `scheduler.deliverFlow` → LaunchRun. Davranış korundu
  (aynı dispatch/records/notify; tek kozmetik: board+flow bildirimi artık 🗂 ile).
- **Foldlanmayan (bilinçli):** session-**reuse** launcher'lar — scheduler'ın `deliverPrompt`
  (schedule-kind session'ı yeniden kullanır) ve `schedule_wake` (mevcut session'a teslim) — yeni
  session spawn etmedikleri için LaunchRun dışında bırakıldı. Bu, C-Faz3'ün "birleşik launcher"
  tohumudur: gelecekteki tek bütçe/pause/telemetri hook'u burada uygulanır.
- **Test:** `launch_test.go` (driver routing + validation, provider'sız). Backend **993 test yeşil**.

## Genişletmeler (2026-07-26, canlı + test)

### #1 — Peer/ajan → bekleyen flow'a input köprüsü
`ResumeWaitingFlow` araç katmanına açıldı (`internal/tools/builtin_flowmgmt.go`): **`list_flow_runs`**
(status='waiting' ile bekleyen koşuları bul; id/flowId/status/waitingAt/input/output döner) +
**`deliver_flow_input`** (runId+input → resume köprüsü). Böylece bir koordinatör/peer ajan/otomasyon
bekleyen flow'u besleyebilir — yalnız UI'daki insan değil. Wiring: `toolsetup_selfmanage.go`
(`r.ResumeWaitingFlow`). Kategori: Automation. Test: `builtin_flowmgmt_test.go` (waiting filtresi +
resume köprüsü). Bekleyen koşuyu bulma: `list_flow_runs status='waiting'` → `waitingAt` node id'si.

### #3 — await timeout / GC (sweeper)
`await-input` node'a opsiyonel **`TimeoutSec`** (0 = süresiz). `db.ListWaitingFlowRuns` + arka plan
süpürücü: `Runtime.StartWaitingFlowSweeper` (30s ticker, `workspace.Manager.open`'da başlar) →
`sweepWaitingFlowsAt(now)` (testlenebilir): her bekleyen koşunun await node'unun `TimeoutSec`'i
`now-suspendTime` ile aşıldıysa **CAS-claim** (canlı resume'a karşı) + `FinishFlowRun(failure,
"await-input timed out after Ns")`. Test: `flow_sweep_test.go` (timeout fail + no-timeout skip).
UI: inspector'da "Zaman aşımı (saniye)".

### #4 — Reuse-launcher fold: **tasarımca yapılmadı** (gerekçeli)
`scheduler.deliverPrompt`/`schedule_wake` incelendi: bunlar "fresh launch" değil, **mevcut session'a
continuation** runner'ları (slot claim + user-turn kaydı + `invokeTraced` + `maybeAutoContinue` +
`maybeAutoHandoff` + tagging). `LaunchRun`'a foldlamak, launcher'ı session-continuation politikasına
**coupler** ve en yük-taşıyan otonom yolu riske atar — kötü değer/risk. Trigger atıflandırması zaten
`RunTrigger`/call-kind sistemiyle var. Karar: **reuse-continuation ≠ fresh launch**, ayrı kalır.
`LaunchRun` fresh-launch (flow + spawn-session) seam'idir; continuation'lar bilinçli olarak dışında.

### #2 — `subflow` node (senkron flow kompozisyonu)
`orchestration.NodeSubflow` (`"subflow"`): `FlowRef` (çocuk flow id) + `Template` (girdi; boşsa
`{{last}}`). Motor `NodeSubflow` case → `ChildFlowRunner.RunChildFlow(flowID, input)` çocuğu **baştan
sona koşar** (kendi FlowRun'ı), çıktısını `{{node.<id>}}`/`{{last}}` olarak yakalar. Runner:
`flowRunner.RunChildFlow` = `RunFlow` + **recursion depth guard** (ctx `subflowDepthKey`, `maxSubflowDepth=5`).
Çocuk `failure` → hata. `Validate`: `FlowRef` zorunlu. Test: `subflow_test.go`. Frontend: palet
"Alt-Akış" (`SubflowNode`, indigo `Workflow`), inspector flowRef+şablon. **Çocuk `await-input`'ta
askıya alınırsa artık propagate edilir → aşağıya bak.**

## Async spawn/join + subflow await-propagasyonu ✅ (2026-07-27, canlı + test)

Önceki iki "Sonraki" maddesi tamamlandı (temiz kurulum; `flow.go` bu oturumda tek elden yazıldı).

**Subflow await-propagasyonu.** Çocuk `await-input`'a düşerse artık hata değil — **parent da askıya
alınır**. `orchestration.SuspendableChildFlowRunner` (`RunChildFlowResumable` → `waiting`+childRunID;
`ResumeChildFlow` → sync devam). `State.SubflowRun` = park edilen child run id. Motor `NodeSubflow`
case'i suspend/resume çift-fazlı: ilk gelişte child beklerse `WaitingAt=subflow, SubflowRun=child` →
parent `waiting`; parent'a input gelince `ResumeChildFlow` child'ı **sync** sürer
(`resumeWaitingFlowSync` = `prepareResume` + inline `driveFlow`), child biterse advance / tekrar
beklerse park kalır. Suspendable yoksa eski fail-on-suspend fallback korunur.

**Async spawn/join.** `NodeSpawn` (`SpawnFlows[]` + `Template`) çocuk flow'ları **bloklamadan**
başlatır (`spawnChildFlow`: precheck + `CreateFlowRun` + goroutine `driveFlow`), run id'lerini
`State.Spawned[nodeID]`'e yazar (restart-safe) → `Next`. `NodeJoin` (`SpawnRef` — boş=tümü) bir
**bariyer**: `JoinChildFlows` child run'ları **block-poll** eder (motor DB'ye dokunmaz; runtime
poll'lar), hepsi terminal olunca çıktıları `{{last}}`'e birleştirir. Çocuk `failure`→join fail; çocuk
`waiting`→join fail (async çocuklar interaktif olmamalı) → bariyer **her zaman sonlanır**.
`AsyncFlowRunner` arayüzü. Recursion `subflowDepthKey` ile guard'lı (değer `WithoutCancel`'da yaşar).

**Test:** `orchestration/spawn_join_test.go` (fake runner: happy path, child-fail, Validate),
`subflow_propagate_test.go` (fake: suspend→resume, no-suspend), `agent/spawn_join_flow_test.go`
(gerçek runtime: transform-child spawn/join; subflow-child await → parent waiting → resume → success).
**Canlı E2E:** spawn(`[child,child]`)→join → `child:go\n\nchild:go`; parent subflow→await-child →
`waiting` → input → `resumed:answer`. Frontend: palet "Spawn (Async)" (macenta `Rocket`) + "Join
(Bariyer)" (teal `GitMerge`), `SpawnNode`/`JoinNode`, inspector spawnFlows/spawnRef + subflow flowRef.

### Genişletme (2026-07-27): join timeout + kısmi mod, flow-picker UI
`NodeJoin` alanları **`JoinTimeoutSec`** (0=süresiz) + **`JoinPartial`**. `JoinChildFlows(ctx, runIDs,
timeoutSec, partial)`: `partial=false` (varsayılan) → herhangi bir fail/suspend veya timeout tüm
join'i düşürür; `partial=true` → fail/suspend/timeout olan çocuk **düşürülür** (çıktısı hariç), join
başarılı çocuklarla devam eder (sonuçlar orijinal sırada, düşenler çıkarılmış). Test:
`spawn_join_test.go` (forward timeout/partial) + `spawn_join_flow_test.go` (`TestJoin_PartialDropsFailedChild`:
OK-child + schema-fail-child → partial "ok", strict fail). Canlı E2E: partial join fail eden çocuğu
atlayıp `ok` döndü. **Flow-picker UI:** subflow/spawn artık akış id metni yerine seçici (subflow=select,
spawn=çoklu-checkbox, join spawnRef=graf'taki spawn node select'i) + join timeout/partial alanları;
`flows` listesi `FlowsPanel → FlowEditorView → NodeInspector` zinciriyle geçer (mevcut flow hariç).

### Genişletme (2026-07-27): continuation turn recording paylaşımı
Session-reuse (continuation) yolları — scheduler `deliverPrompt` + `deliverWake` — sync'ten kaçan
ortak parçayı artık paylaşıyor: `Runtime.recordAssistantReply` (boş→placeholder + `meta.apply` +
`encodeSteps` + agent id) ve `recordTurnError` (prefix + hata metni, best-effort) — `turn_record.go`.
**Bilinçli karar:** hepsini tek "generic turn-runner"a katlamak ~12 knob'lu bir god-function
gerektirir (overflow+autoContinue+autoHandoff vs asyncChat+wake-event, prompt-only vs history-aware) →
iki okunur fonksiyondan kötü olurdu; yalnız drift-prone mesaj-kurma bloğu paylaşıldı, yaşam-döngüsü
çağıranda kaldı. `agentmsg`/`coordination`/`spawn` aynı bloğu kullanıyor → ileride bu helper'lara
geçebilir. Test: `turn_record_test.go` (boş-substitution, error shape). Backend 1015 test yeşil.

### Genişletme (2026-07-27): tüm continuation siteleri + async örnek şablon + join canlı ilerleme
- **Paylaşımın yayılması:** `recordAssistantMessage` çekirdeği eklendi (`recordAssistantReply` boş→placeholder
  ekleyip onu çağırır; `recordTurnError` prefix+hata ile çağırır). `agentmsg` (inbox), `spawn`,
  `coordination` (worker + coordinator; metin/kill mantığı çağıranda kalır) artık bu helper'ları
  kullanıyor — 5 continuation sitesi tek yerden. Test: `turn_record_test.go`.
- **Async örnek şablon:** gallery'ye "Async Fan-out (Spawn/Join)" eklendi. `FlowTemplate.companions`
  (opsiyonel) — instantiate'te companion alt-akışlar oluşturulur ve `spawn.spawnFlows`'taki
  `companion:<i>` yer-tutucuları gerçek id'lerle değişir → şablon kutudan çıkar çıkmaz çalışır
  (`flowActions.instantiateTemplate`).
- **Join canlı ilerleme:** `JoinChildFlows(..., onProgress func(done,total))`; engine join case'i her
  poll'da `"progress"` faz'lı NodeEvent yayar (`done/total`), RunView bunu çalışan satırda
  "N/M tamamlandı…" olarak gösterir (`FlowNodeEvent.phase += 'progress'`). Canlı SSE doğrulandı:
  `start → 0/2 → 2/2 → done`. Backend 1029 test yeşil.

### Genişletme (2026-07-27): #5 son continuation sitesi + #1 canlı-step emit seam'i
- **#5 — `autocontinue` de paylaşımda.** Kalan tek elle-kayıt noktası (`autocontinue.go` hata + yanıt
  dalları, `db.Message`+`encodeSteps` elle kuruyordu) `recordTurnError`/`recordAssistantReply`'a
  geçirildi → 6. continuation sitesi tek yerden (`strings` importu düştü). **Bilinçli ayrı kalan:**
  `recordInterruptedReply` (crash-recovery, `Interrupted:true`, steps/meta yok) ve `recordFlowSessionTurn`
  (flow→session reify, `flowStateToSteps`) — ikisi de helper'a knob eklemeyi gerektirir, ayrı okunur.
- **#1 — tek canlı-step emit seam'i.** `emitSessionStep`(`session_step`) ve `emitFlowNodeStepCtx`
  (`flow_node_step`) ortak marshal+bus-envelope kuyruğunu tek `publishStep(evType, target, step)`
  (`sessionstep.go`) üzerinden yapıyor → Level/Step alanları tek yerden, drift yok; `flow_steps.go`
  `events` importundan kurtuldu. `busForwardable` filtresi çağırana özel kalır (session delta-flood eler;
  flow node adımları iri-taneli — davranış birebir korunur). **Bilinçli ayrı kalan (persistence):** chat
  adımı `Message.Steps` (session.jsonl), flow adımı per-node sidecar (State şişmesin diye) — bir flow
  node ≠ mesaj turu olduğundan iki depolamayı tek şemaya katlamak büyük/riskli, düşük görünür kazanç →
  yalnız bus emit yolu birleşti. Test: `internal/agent`+`internal/api` 353 yeşil.

### Genişletme (2026-07-27): #4 LaunchRun pause/telemetri gate + #2 durable-wait değerlendirmesi
- **#4 — `launchGate` (Faz3 tohumu somutlaştı).** LaunchRun'ın en başına tek dispatch-öncesi gate:
  `launchGate(spec, driver)` → **otonom** launch'larda workspace otonomi freni (`r.Paused()`) burada
  uygulanır, böylece duraklatılmış workspace **hiç** session/flow-run yaratmadan `ErrAutonomyPaused` ile
  fail-fast döner (önceden yalnız `guardedComplete` per-LLM-call gate'liyordu → junk row + async hata
  oluşuyordu). **Manuel** launch freni baypaslar. Ayrıca run başına tek launch-telemetri satırı
  (trigger/driver/autonomous). `launchDriver(spec)` yardımcı. Per-launcher guard'lar (cooldown, iterasyon
  cap) çağırandan önce çalışmaya devam eder. Test: `launch_test.go` `TestLaunchRun_PauseGate`
  (paused+autonomous→refuse, manual→bypass). Backend `internal/agent`+`internal/api` 354 yeşil.
- **#2 — durable-wait merge'i BİLİNÇLİ YAPILMADI (gerekçeli).** İki "bekle" sistemi **kasıtlı olarak
  farklı durability modeli** taşıyor: (a) **flow `await-input`** = durable (persist `waiting` statü +
  DB-katmanı CAS `ClaimWaitingFlowRun` + endpoint'ten resume, goroutine bloklamaz, restart-safe, timeout
  sweeper); (b) **session interaction** (ask/permission/plan) = bellek-içi `pendingInteraction` +
  `atomic.CompareAndSwapInt32` resolve-once, **canlı turu bloklar**, restart'ta ölür. Doc 62 flow yolunu
  zaten bu primitifi "durable için yanlış" bulduğu için **ayrı** kurmuştu. Üçüncüsü session **send-queue**
  = durable *mesaj kuyruğu* (teslim), süspansiyon değil. Bunları tek `DurableWait` arayüzüne katlamak ya
  (i) flow await'i yeniden goroutine-bloklar (doc 62'nin düzelttiği regresyon), ya (ii) session
  interaction'ı durable yapar = büyük *feature*, refactor değil. Durable substrat (flow) **zaten** tek
  temiz implementasyon ve peer/tool/endpoint yüzeyi de tek (`deliver_flow_input` / `POST
  /flow-runs/{id}/input` → `ResumeWaitingFlow` → `prepareResume`). → Güvenli/değerli bir kod-merge'i yok;
  yapılmadı.

## Koşu soyağacı (run lineage) — Katman 1 (2026-07-28)

Kompozisyon (subflow/spawn/`run_flow`) çalışıyordu ama **çocuk koşu ile ebeveyni arasında
kalıcı bir bağ yoktu**: parent'ın `State.SubflowRun`/`State.Spawned` alanları yukarıdan
aşağı iniyordu, tersi yoktu → "beni kim başlattı" sorulamıyor, ağaç tek sorguda çekilemiyordu.

- **`db.FlowRun` + 3 alan:** `ParentRunID` (ağacın kenarı), `ParentNodeID` (parent
  grafiğinde hangi node doğurdu — aynı grafikte birden çok subflow node'u varsa atfetmek
  için şart), `RootRunID` (ağacın tepesi; **boş = kendisi** kodlaması, böylece kök koşu
  id'si üretildikten sonra ikinci bir yazma gerekmez). Okuma: `RootOf()` / `IsRootRun()`.
  Hepsi `omitempty` → eski koşular kök olarak okunur, **migration yok**.
- **Tespit:** yeni taşıma mekanizması eklenmedi — `driveFlow` zaten
  `withFlowRunID(ctx, run.ID)` yapıyordu (step sidecar'ı için). Ctx'te flow run id
  **varsa** yeni koşu onun çocuğudur, yoksa köktür (`runLineage`, `flow_steps.go`).
  Yanına `withFlowRootID` eklendi → çocuk, kökü öğrenmek için parent'ı **DB'den okumaz**.
- **Node atfı:** `orchestration.WithNodeID` yalnız `runAgentNodeSafe` içinde uygulanıyordu;
  artık `NodeSubflow` ve `NodeSpawn` dalları da runner'ı kendi node id'leriyle çağırıyor.
  **Hiçbir runner arayüzü imzası değişmedi** (aksi halde 3 arayüz + tüm sahte test
  runner'ları etkilenirdi).
- **Bedava kazanç:** agent node'unun `run_flow` aracıyla başlattığı flow da node'un ctx'ini
  miras aldığı için ağaca çocuk olarak düşer → daha önce "canvas'ta görünmez" olan dinamik
  çağrılar en azından **soyağacında** görünür.
- **Sorgu:** `ListRootFlowRuns` (liste subflow çocuklarıyla dolmasın; çocuklar birinci sınıf
  kalır, id ile hâlâ çekilebilir) + `ListFlowRunTree(rootID)` — üyelik `RootRunID` üzerinden
  **tek tarama**, sıralama ise **ebeveyn bağlarıyla breadth-first**.
  **Neden zamana göre değil:** `now()` **saniye** çözünürlüğünde; bir composed flow ebeveyni
  ve çocuğunu aynı saniyede yarattığı için `CreatedAt` ikisini ayırt edemiyor → ilk yazdığım
  "eskiden yeniye" sıralama **keyfi sonuç veriyordu** (test yakaladı). Kardeşler
  `flowRunBefore` ile sıralanır: `CreatedAt`, eşitse id sayacı (`flowRunSeq`; id'ler sıfır
  dolgusuz olduğundan sözlüksel karşılaştırma `RUN10`'u `RUN2`'den önce koyardı).
  Ulaşılamayan üye (ebeveyn satırı silinmiş) **sessizce düşürülmez**, sona eklenir; döngüye
  karşı `seen` guard'ı var.
- **Kapsam dışı (bilinçli):** API uçları ve ağaç UI'ı (Katman 3–4) bu adımda yapılmadı.
- Test: `orchestration/lineage_test.go` (node-id etiketi subflow/spawn'da doğru, komşu
  node'a sızmıyor) + `db/flow_lineage_test.go` (boş=kendisi kodlaması, kalıcılık, iki
  seviye derinlikte kök hâlâ tepe, kök-filtresi çocuğu gizler ama silmez).

## Koşu soyağacı — Katman 2: olay katmanı (2026-07-28)

Her koşu (çocuklar dahil) node yaşam-döngüsünü **kendi run id'si** altında yayınlıyordu, bu
yüzden parent'ın RunView'ı child koşarken ölü duruyordu. Çocuk id'lerine tek tek abone olmak
çözüm değil: çocuklar **koşu ortasında doğuyor**, id'leri abone olurken bilinmiyor.

- **Backend:** `emitFlowNode(runID, flowID, rootRunID, ev)` → `Target`'a `rootRunId`.
  Kök `driveFlow`'da zaten ctx'te (Katman 1) → ek okuma yok. Kök koşuda `rootRunId == flowRunId`.
- **Frontend bus (`flowNodeBus.ts`):** aynı frame **iki kapsama** dağıtılır —
  `subscribeFlowNode(runId)` (mevcut API, **imzası değişmedi**, RunView aynen çalışır) ve yeni
  `subscribeFlowTree(rootRunId)` (kök + tüm alt koşular, **sonradan doğanlar dahil**).
- **Kritik:** ağaç kapsamındaki payload `{runId, ev}` taşır. Aksi halde parent, child'ın node
  id'lerini kendi grafiğine boyardı (`flow_node_step` zaten `{nodeId, step}` ile aynı deseni
  kullanıyordu). Per-run kapsamda gerek yok — orada anahtar zaten run id.
- **Geriye dönük:** `rootId` opsiyonel; etiketsiz frame "kendi kökü" sayılır → eski backend
  frame'leri kaybolmaz.
- **Kapsam dışı (bilinçli):** `flow_node_step`'e dokunulmadı (node adımları yalnız inspector
  açıkken gerekir, ağaç için gereksiz trafik). Replay/reconnect yok — bus'ta replay yok,
  kopuşta resync **Katman 3'ün** ağaç endpoint'iyle yapılacak (mevcut RunView de aynı durumda).
- Test: `agent/flow_treeevent_test.go` (frame hem kendi run'ını hem kökü taşır; kök koşu
  kendini kök etiketler) + `frontend/src/shared/lib/flowNodeBus.test.ts` (6 senaryo: kardeş
  ağaca sızma yok, child frame'i köke yönlenir, etiketsiz frame kendi kökü, çift teslim yok).

## Koşu soyağacı — Katman 3: sorgu ve API (2026-07-28)

Katman 1 veriyi, Katman 2 canlı nabzı üretmişti; ikisi de UI'ın erişemediği yerdeydi.
Bu katman store fonksiyonlarını HTTP'ye açar — yeni sorgu mantığı yazılmadı.

- **`GET /api/flow-runs?rootOnly=true`** → `ListRootFlowRuns`. **Opt-in**: parametresiz
  çağrı eskisi gibi her koşuyu döndürür, yani mevcut çağıranlar (ör. `SessionFlowInline`)
  sessizce davranış değiştirmez. Yalnız `"true"` filtreler; `rootOnly=1` filtrelemez.
  `flowId` filtresi bunun üstüne biner.
- **`GET /api/flow-runs/{id}/tree`** → `ListFlowRunTree`. Id **ağacın herhangi bir üyesi**
  olabilir, yalnız kök değil: UI'da seçili koşu çoğu zaman bir çocuktur ve "bu koşunun
  ağacı" isteği hangi üyenin tıklandığına bağlı olmamalı. Handler tek okumayla `RootOf()`
  ile köke normalize eder. Bilinmeyen id → **404** (boş liste değil; böylece bayat bir
  derin-bağlantı, gerçekten çocuğu olmayan bir koşudan ayırt edilir).
- **Resync yolu:** bus'ta replay yok. SSE koptuğunda (sekme uyudu, ağ gitti) ağacın anlık
  hali bu uçtan tek çağrıyla geri alınır — Katman 4 paneli buna dayanacak.
- **UI:** Koşular sekmesinde "alt koşuları göster" onay kutusu (`FlowsListPane`),
  varsayılan **kapalı** → liste yalnız kök koşular. Durum `useSessionState` ile kalıcı;
  değişince poll etkisi yeniden koşar, 3sn'lik aralık beklenmez.
  **Yan kazanç:** FlowsPanel'in derin-bağlantısı (`runs.find(r => r.flowId === openFlowId)`)
  aynı `flowId`'yi taşıyan **daha yeni bir çocuğu** seçebiliyordu; kök-only listeyle
  doğru kök seçilir.
- **İstemci sözleşmesi:** `FlowRun` TS arayüzü Katman 1'in üç alanını taşımıyordu — Go
  bunları JSON'da gönderse de tip bilmediği için `flowRunTree()`'nin çıktısından
  `parentRunId` okunamıyordu (kimse tüketmediği için derleme patlamamıştı). Üçü de
  **opsiyonel** eklendi (`omitempty` ile birebir). "Boş = kök" kodlaması her tüketicide
  yeniden türetilmesin diye `shared/lib/flowRunTree.ts` → `flowRunRootOf` /
  `isRootFlowRun` (Go'daki `RootOf` / `IsRootRun` karşılığı). **Kök olma kararı
  `parentRunId`'ye bakar, `rootRunId`'ye değil**: kökte ikisi de boştur ama yalnız
  ebeveyn bağı tam olarak kökler için boş kalır.
- Test: `api/flow_runs_tree_test.go` (rootOnly iki yön + flowId ile birleşim + gevşek
  değer filtrelemiyor; ağaç kökten/çocuktan/torundan aynı sırayı verir, komşu ağaç
  sızmaz, bilinmeyen id 404) + `shared/lib/flowRunTree.test.ts` (5 senaryo: kök kendi
  kökü, torun tepeyi gösterir, `""` de kendisi sayılır, kök kararı ebeveyn bağıyla).

## Koşu soyağacı — Katman 4: UI (2026-07-28)

Composed bir koşunun asıl işi, ana canvas'ın gösteremediği koşularda oluyor: her subflow/spawn
çocuğu **kendi grafiğine sahip ayrı bir koşu**. Bu katman onları görünür kılar.

- **Olaya iki etiket daha:** `emitFlowNode` artık `run db.FlowRun` alıyor (4 pozisyonel string
  yerine) ve `Target`'a `parentRunId` + `parentNodeId` ekliyor — ikisi de zaten koşu satırında,
  **ek okuma yok**. **Neden ikisi birden:** node id'leri yalnız *kendi grafiği içinde* tekil;
  aynı ağaçtaki iki koşu da `n1`'e sahip olabilir, node-only anahtar birinin ilerlemesini
  diğerinin canvas'ına boyardı. Etiketler olmasa parent, yeni doğmuş bir çocuğu yerleştirmek
  için önce ağacı çekmek zorunda kalır ve o istek dönene kadar hiçbir şey göstermezdi.
- **Saf mantık ayrı (`features/flows/runTree.ts`):** `buildRunTreeRows` (derinlik; **backend
  sırasını koruyor, yeniden sıralamıyor** — istemcide ikinci bir doğruluk kaynağı üretmemek
  için), `applyChildFrame` (canlı frame → `(parentRunId → nodeId → ChildProgress)` iki
  seviyeli harita), `runTreeBreadcrumb` (döngü guard'lı ata zinciri).
- **`RunTreeView` sarmalayıcı:** ağaç durumu, canlı abonelik ve hangi koşunun *görüntülendiği*
  burada. **RunView'a dokunulmadı** (3 çağrı yeri var, yalnız biri bu boyutu istiyor): RunView
  hâlâ tek koşu render ediyor, sarmalayıcı hangisi olduğuna karar veriyor. Ağaç, içinde koşan
  bir şey kaldığı sürece 3sn'de bir tazelenir; biten ağaç istek üretmeyi bırakır. Tanınmayan
  `runId` taşıyan frame = çocuk yeni doğdu → 250ms debounce ile ağaç yeniden okunur.
- **Rozet — `3/7` yerine çocuğun o anki node'u.** Payda (child akışın node sayısı) çalışan node
  sayısının **üst sınırı değil**: loop node'ları yeniden girer, branch atlar → oran hem yanlış
  olur hem 1'i aşabilir. `ChildRunBadge` bunun yerine `▶ 3. Kod incelemesi` gösterir.
- **Nested canvas:** subflow/spawn node'una **çift tık** → o node'un başlattığı koşuya inilir,
  üstte breadcrumb. `FlowCanvas.onNodeDoubleClick` bilerek `editable` kapısına takılı **değil**
  — run inspector zaten salt-okunur ve inişin geçerli olduğu tek yer orası.
- **Tarayıcı testinde yakalanan hata — rozet yalnız canlıydı.** İlk yazımda `childProgress`
  *sadece* canlı SSE çerçevelerinden doluyordu. Bitmiş bir composed koşu açıldığında hiç
  çerçeve gelmediği için **ne rozet çıkıyordu ne de çift tık çalışıyordu** — ve koşu listesine
  normalde iş bittikten *sonra* bakılır, yani bu kenar durum değil asıl durum. Düzeltme:
  `childProgressFromTree` ilerlemeyi **kalıcı ağaçtan tohumlar** (çocuğun `status`'ü → phase,
  kendi trace'inin son node'u → başlık/indeks), `mergeChildProgress` canlı çerçeveleri
  üstüne bindirir (canlı kazanır, çünkü daha yeni). Trace'i olmayan/bozuk state'li çocuk da
  kaydedilir: girdinin asıl işi **hangi koşunun o node'a asılı olduğunu** söylemek, iniş
  bunu kullanıyor.
- **Bilinçli sınır:** `run_flow` aracıyla ajan içinden başlatılan çocuklar ağaç panelinde
  listelenir ama **rozetleri yoktur** — onları doğuran bir node yok, asılacakları yer yok.
  Tek koşuluk ağaç paneli hiç render edilmez (çocuğu olmayan akış chrome ödemesin).
- Test: `agent/flow_treeevent_test.go` (iki yeni etiket, kökte ikisi de boş) ·
  `shared/lib/flowNodeBus.test.ts` (etiketler yalnız ağaç kapsamına geçer) ·
  `features/flows/runTree.test.ts` (19 senaryo: derinlik, backend sırası korunur, kayıp
  ebeveynli üye düşürülmez, **iki koşu aynı node id'sinde çakışmaz**, parent'sız frame
  yok sayılır, ağaçtan tohumlama + canlı bindirme, bozuk state ağacı düşürmez, breadcrumb
  döngüde durur).
- **Tarayıcı doğrulaması:** gerçek Chrome'da (playwright-core, system Chrome kanalı) üç
  seviyeli deterministik bir akış koşturulup **7/7** senaryo geçti — `rootOnly` listesi,
  toggle, ağaç paneli, üç seviye, rozet, çift tıkla iniş, breadcrumb; konsol hatasız.
  Not: MCP tarayıcı kaynakları bu oturumda araç yüklemediği için otomasyon doğrudan
  `playwright-core` ile sürüldü. `waitUntil: 'networkidle'` **kullanılamaz** — uygulama
  `/api/events` SSE bağlantısını kalıcı açık tuttuğu için ağ hiç boşa çıkmaz.

## Sonraki (daha ileri)
- Gantt görünümü (`TraceEntry.StartMs`/`EndMs` hazır) — ağaç + rozet oturduktan sonra.
- Join ilerlemesini canvas node'unun kendisinde de göstermek (şu an yalnız adım-izi satırında).
- (İstenirse) step *persistence* katmanını birleştirmek: flow sidecar + chat `Message.Steps`'i tek "step
  store" arayüzü ardına almak — yüksek risk (run inspector + crash-recovery + SSE routing regresyon
  yüzeyi), yalnız net ihtiyaç doğarsa.
- #2'nin gerçek ihtiyaç doğarsa doğru kapsamı: session interaction'ı **durable** yapmak (flow waiting
  altyapısını devralarak) — bir *feature* olarak planlanmalı, mevcut ephemeral yolla merge değil.
  **→ Yapıldı (2026-07-27): "Durable Ask" (MVP), native `ask_user` için kalıcı suspend/resume —
  `_Docs/65-DURABLE-ASK.md`.**
