# 62 — Birleşik Run (C+D) · `await-input` keystone

> Session ⇄ flow birleşiminin (Model C: ortak Run substratı, Model D: çapraz-driver
> kompozisyon) **yürütülebilir çekirdeği**: flow'un durable olarak **girdi bekleyebilmesi**.
> Bir flow `[agent] → [await-input] → [agent]` artık **scripted bir konuşmadır** =
> "her session bir flow"un ilk somut adımı. Tam tasarım: `plans/unified-run-CD-design.md`.

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

## Sonraki (daha ileri)
- Join ilerlemesini canvas node'unun kendisinde de göstermek (şu an yalnız adım-izi satırında).
