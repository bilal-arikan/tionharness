# 77 — Rota Altyapı Hazırlık Planı

> **Durum:** **UYGULANDI** — R1–R10 tamamı (2026-09-02, her kalem kendi
> dalında; sapmalar kalemlerin "Gerçekleşen" notlarında) ve üstüne Rota F0–F5
> (2026-09-02/03; §2'deki notlar). Bu doküman, "Rota" (oturumları dinamik,
> çatallanan akış grafiğine hizalama + workspace canlı görünümü + koşu-sonu
> optimizer) özelliği **başlamadan önce** altyapıda kapatılan boşlukları ve
> refactor'ları sıralar; Rota'nın kendisi `78-ROTA-EKRANI.md`'de.
>
> **Önkoşul okuma:** `47-KOORDINATOR-COKLU-AJAN.md`, `15-FLOW-CANVAS.md`,
> `46-ETIKET-OTOMASYON.md`, `58-QUEUE-SENKRON.md`, `66-VIEW-KATMANI.md`,
> `68-OZET-HARITASI.md`, `08-DEPOLAMA.md`.

## 0. Neden önce altyapı

Keşif (2026-09-02, altı paralel tarama) Rota'yı doğrudan kurmayı engelleyen beş
yapısal boşluk buldu. Hepsi "Rota'yı bir projeksiyon olarak türet" tezinin
dayandığı temel verinin ya dağınık ya da sunucuda hiç olmamasından kaynaklanıyor:

| # | Boşluk | Bugün | Sonucu |
|---|--------|-------|--------|
| 1 | **Köken (lineage) tek kaynak değil** | `ParentSessionID` handoff, `CoordinatorSessionID` worker, `RetryOfSessionID`, `Kind+SourceID` flow'a işaret eder run'a değil, `FlowRun.SessionID` run **bitince** damgalanır, automation-run için yalnız `Automation.LastSessionID`, `run_flow`'dan doğan child run'da `ParentNodeID` yok | Rota kenarlarının çoğu tahminle çizilmek zorunda kalır |
| 2 | **Canlılık sunucuda tek kaynak değil** | `runningSessionIDs` üç registry'nin API katmanında birleşimi; interaktif tur API'de koşar (`SetExternalActiveSessions` köprüsü); `running`/`awaiting-workers` çipleri istemci tarafında çünkü "sunucu bilmez" | Workspace canlı görünümü sunucudan sorulamaz |
| 3 | **Global olay yolu sırasız ve replay'siz** | `/api/events` bus'ında seq/cursor yok, yavaş abone düşer, reconnect'te her panel "resync" ile yeniden çeker | 50 şeritlik canlı görünüm her kopuşta sıfırdan yüklenir |
| 4 | **Oturum sidecar'ları elle** | `inbox.json`, `inflight.json`, prompt-epoch, `progress/`, `cli-reply.wal.json` her biri kendi atomik yazım/yükleme/karantina kodunu taşır | `trajectory.json` altıncı kopya olur |
| 5 | **Otomasyon çekirdeği kapalı ve arşivsiz** | 4 tetik `automation.go` içinde switch; `ValidateAutomationShape` dallı; ateşleme defteri yok; `Archived` yok; tükenmiş/expired satırlar sonsuza kadar kalır | `phase`/`trajectory_end` tetikleri ve küratör budaması eklenemez |

Tasarım ilkeleri, mevcut kararlarla uyumlu:

- Dosya tabanlı store, migration yok: yeni alanlar `omitempty`, eski dosyalar sıfır
  değerle okunur, backfill **bellek içi** (diski yeniden yazmaz).
- `Session.Kind` taksonomisine **dokunulmaz** (frontend `sessionKindMeta`, çipler,
  writable listesi, testler). Köken **ek** alanla gelir.
- Pasif pano ilkesi Rota'ya da uygulanır: altyapı hiçbir şeyi çalıştırmaz, yansıtır.
- Prompt-cache'e dokunan hiçbir değişiklik yok (system prompt/araç şeması sabit).
- God-dosyalara (`coordination.go` 123 KB, `automation.go`) kod **eklemek yerine**
  gözlemci arayüzü çıkarılır; Rota bir abonedir.
- Mevcut seam'ler genişletilir, yenisi icat edilmez: `LaunchRun`, `SetBoardHook`,
  `SetActivityHook`, `SessionHub`, `FlowObserver`, `view.Sources`.

> ⚠️ **Eşzamanlı çalışma.** Ana ağaç şu an başka bir oturum tarafından da
> düzenleniyor (network fiziği, `05-ILERLEME`). Her kalem **ayrı worktree + kısa
> PR**; `05-ILERLEME.md` yalnız kalem kapanınca güncellenir. `internal/skills/store.go`
> son üç commit'te değişti — R6 rebase ile başlar.

## 1. Refactor kalemleri

Her kalem: neden → ne → dosyalar → etki alanı → test → boyut (S/M/L) → hangi Rota
fazını açar. Rota fazları brifteki numaralarla: F0 projeksiyon, F1 varlık+ilan,
F2 otomasyonlar grafikte, F3 metrik+küratör, F4 optimizer, F5 kapılar.

### R1 — Oturum kökeni (`SessionOrigin`), tek damgalama noktası, oturum hook'u

**Neden.** Boşluk 1. Rota'daki `spawned`, `fired`, `forked_from`, `next` (handoff)
kenarlarının tamamı "bu oturumu kim, hangi varlıktan, hangi düğümden başlattı"
sorusudur; bugün altı yerde altı farklı cevap var.

**Ne.**

```go
// internal/db/models.go
type SessionOrigin struct {
    Kind             string `json:"kind"`             // user|coordinator|subagent|flow|schedule|automation|handoff|wake|insight|peer
    EntityID         string `json:"entityId,omitempty"` // AUT… / SCH… / FLW… / AGT…
    RunID            string `json:"runId,omitempty"`    // RUN… (flow) — run BAŞLARKEN damgalanır
    NodeID           string `json:"nodeId,omitempty"`   // flow node / faz id
    TriggerSessionID string `json:"triggerSessionId,omitempty"` // tetikleyen oturum (tag/counter/coordinator)
    RootSessionID    string `json:"rootSessionId,omitempty"`    // ağaç kökü; boş = kendisi
    At               int64  `json:"at"`
}
// Session:
Origin *SessionOrigin `json:"origin,omitempty"`   // SchemaVersion 3 → 4
```

- `db.CreateSession` **tek damgalama noktası**: `Origin == nil` ise `user` yazar.
  Altı çağrı yeri güncellenir: `agent/spawn.go` (coordinator/subagent/handoff/peer,
  `SpawnOptions.Origin`), `agent/flow.go` ×2 (flow, `RunID` **oluşturmada**),
  `agent/flow_coordinator.go`, `agent/insightsteps.go`, `api/sessions.go`.
  `GetOrCreateSourceSession` / `GetOrCreateKindSession` (automation continue,
  schedule reuse) yaratma dalında Origin alır.
- `automation.go dispatchFire` → `LaunchRun` çağrısına `Spawn.Origin{Kind:
  automation, EntityID, TriggerSessionID}`; `scheduler.run` → `schedule`;
  `handoff.go` → `handoff` (+ `ParentSessionID` aynen kalır).
- `FlowRun.SessionID` run yaratılırken damgalanır (bugün `SetFlowRunSession`
  bitişte; 5 sn `createdAt` toleransı fallback'i **silinir**).
- `run_flow` aracı ctx'teki node id'yi child run'ın `ParentNodeID`'sine yazar
  (`flow_steps.go` `WithNodeID` zaten var; `builtin_flowmgmt.go` okur).
- `Session.Lineage()` türetilmiş okuyucu: Origin varsa onu, yoksa eski alanlardan
  (`CoordinatorSessionID` → coordinator, `Kind=flow` → flow …) **bellek içi**
  türetir. Boot `reconcileHeader` aynı türetimi bir kez yapar, diske yazmaz.
- `db.SetSessionHook(fn SessionChangeFn)` — `SetBoardHook`'un ikizi:
  `create|state|runstate|archive|origin` op'ları, **kilit dışında** ateşlenir
  (`fireBoardHook` deseni). R3 olay yayını ve Rota bağlayıcısı buna abone olur.

**Etki alanı.** Orta-büyük: db + 9 dosya; frontend `Session` tipine opsiyonel
`origin`. Mevcut alanlar silinmez; hiçbir okuma yolu bozulmaz.

**Test.** Her create yolu için Origin assert; boot backfill (eski header, Origin
yok → `Lineage()` doğru); `TestRunFlowStampsSessionAtStart`;
`TestRunFlowChildCarriesParentNode`; hook sırası (kilit dışı) testi.

**Boyut:** L. **Açar:** F0 (Ağ kenarları), F1.

**Gerçekleşen (2026-09-02).** Plan büyük ölçüde aynen; sapmalar:

- Köken türleri: `wake` ve `peer` **eklenmedi** (wake mevcut oturuma teslim eder, peer
  oturum yaratmaz); ajan/API kaynaklı bağımsız spawn için `spawn` eklendi.
- `Session.Origin` işaretçi (`omitempty`), `SessionSchemaVersion` 3→4. Yeni dosyalar:
  `internal/db/models_session_origin.go`, `store_session_hook.go`,
  `internal/agent/flow_origin.go` (launcher kökeni + koşu transkript oturumu için ctx
  anahtarları). `mutateSession` (post-mutasyon satırı döndüren) `mutateSessionLocked`'ın
  altına çekildi; hook create/state/runstate/origin/delete op'larıyla ateşleniyor.
- `FlowRun.SessionID` artık `RunFlow` içinde satır oluşur oluşmaz damgalanıyor; koşu
  sonundaki `SetFlowRunSession` fallback yolu (oturum geç açılırsa) korunuyor.
- Frontend'deki 5 sn `createdAt` eşleştirmesi (`SessionFlowInline.tsx`) tamamen
  silinmedi: yalnız akışın **hiçbir** koşusu `sessionId` taşımıyorsa (link öncesi
  mağaza) devrede; tek bir bağlı koşu varsa kesin eşleşme şart.
- `run_flow` child run'ının `ParentNodeID`'si: `runLineage` zaten düğüm id'sini ctx'ten
  okuyor; ayrı düzeltme gerekmedi, doğrulama R8'de test edilecek.
- Testler: `models_session_origin_test.go` (türetim tablosu, damga, eski header
  backfill + dosya yeniden yazılmıyor, `SetSessionOriginRun`),
  `store_session_hook_test.go` (op sırası, kilit-dışı yeniden giriş, başarısızlıkta
  sessizlik), `session_origin_test.go` (ilk düğüm başlarken run↔oturum bağı,
  otomasyon kökenli flow koşusu, worker kökeni, handoff kökeni).

### R2 — Canlılık kaydı (`liveness.Registry`), sunucuda tek kaynak

**Neden.** Boşluk 2. Workspace canlı görünümü "şu an ne koşuyor, ne kuyrukta,
ne bekliyor ve neden" sorusunu sunucuya soracak; bugün bu bilgi dört yerde ve
bir kısmı yalnız istemcide.

**Ne.**

```go
// internal/liveness/registry.go
type State string // running | queued | waiting_ask | waiting_input | waiting_workers | idle
type Entry struct {
    SessionID string; State State; Since int64
    Reason    string // "turn:chat" | "turn:autonomous" | "spawn-queue#3" | "ask:ASK12" | "flow:RUN88@await" | "coord:drain"
}
type Registry interface { Set(sessionID string, e Entry); Clear(sessionID string); Snapshot() []Entry; Get(id) (Entry, bool) }
```

- Tüm giriş noktaları yazar: tur slotu claim/release (`BusyTurn` kaynağı),
  spawn kuyruğu (`spawnqueue.go`), Durable Ask park/claim (`session_ask`),
  `MarkFlowRunWaiting`/`ClaimWaitingFlowRun`, koordinatör slot `pending`/`driving`,
  interaktif tur (`api/chat_turn*.go`) — böylece `SetExternalActiveSessions`
  köprüsü **kalkar**.
- `api/running_sessions.go` silinir; Ağ, executions, in-flight listesi, sidebar
  çip sayımları `Registry.Snapshot()` okur.
- `sessions_chips.go`'daki "canlılık çipleri istemcide kalır" istisnası kalkar:
  `running` ve `awaiting-workers` sunucu tarafında sayfalanır.
- Kapasite: `Registry.Capacity()` → `{spawnUsed, spawnMax, queueDepth, queueMax,
  autonomyPaused}`; `SpawnMaxConcurrent`/`SpawnQueueMax` ve otonomi freni tek zarfta.
- Olay: her `Set/Clear` R3 üzerinden `session_lifecycle` yayar.

**Etki alanı.** Orta-büyük; davranış değişmez, kaynak değişir. Regresyon riski
sidebar çipleri ve Ağ canlı kapsamı.

**Test.** Her giriş noktası için Set/Clear çifti; çip sayımı sunucu = istemci
eşitliği (mevcut `TestListSessionsChip*` genişler); köprü kaldırıldıktan sonra
`AgentBusy` testi.

**Boyut:** M-L. **Açar:** workspace şeritleri, kapasite şeridi, gelecek ekseni.

**Gerçekleşen (2026-09-02, dal `rota/r2-liveness`).** Plan "her giriş noktası yazan
bir registry" istiyordu; uygulanan, **aynı tek-kaynak sonucunu veren hesaplanmış
anlık görüntü** oldu — yeni bir mutable registry ve giriş noktalarına yayılmış yazımlar
yerine, zaten otoriter olan kaynakların tek yerde katlanması:

- `internal/liveness` (`State`, `Entry{SessionID, State, Reason, Since, Waiting}`,
  `Capacity`, `Snapshot`, `Builder` — bir oturum en aktif durumuyla bir kez görünür).
- `Runtime.Liveness(ctx)` (`internal/agent/liveness.go`): tur slotları (turnqueue,
  kind + since), izlenen otonom invoke'lar, api chat-run probe'u, koordinatör slotları
  (owed drain → `queued`; canlı worker'lı boşta koordinatör → `awaiting_workers`),
  Durable Ask (`waiting_ask`), bekleyen flow koşuları (`waiting_input`); kapasite =
  spawn aktif/maks, kuyruk derinliği/maks, meşgul tur sayısı, otonomi freni.
- API: `running_sessions.go` **silindi**; dört tüketici `s.liveSessions(wsp).RunningSet()`
  okur (RunningSet = running ∪ awaiting_workers, eski kapsam sözleşmesi korunur).
  Yeni uç `GET /api/workspace/liveness`. `sessions_chips.go`'daki "canlılık çipleri
  istemcide kalır" istisnası kalktı: `running`/`awaiting-workers` sunucuda sayfalar
  ve sayar. **Düzeltme (aynı gün):** ilk kesim bu iki chip'i "işaretliyse yalnız
  canlıları göster" diye daraltıcı uygulamıştı; kenar çubuğu tüm chip'leri
  gönderdiğinden liste canlı oturumlara indi. Frontend'deki gibi kapsam kapısına
  çevrildi (bkz. `05-ILERLEME.md` aynı tarihli kayıt).
- `SetExternalActiveSessions` köprüsü **kaldırılmadı**: chat turları slot claim ettiği
  için büyük ölçüde gereksiz ama istek gelişi ile slot arası pencereyi kapatıyor;
  ayrıca `liveSessions` `s.runs`'ı doğrudan da katlıyor (manager wiring'i olmayan
  test/araç sunucuları için).
- `ws:liveness` olayı: tur slotu claim/release/kuyruk değişiminde
  (`publishTurnQueue` → `emitLiveness`) workspace akışına `{sessionId, busy, kind,
  since, waiting}`.
- Frontend: `types/liveness.ts`, `workspaceStream.ts` `LivenessData`; sidebar'ın
  istemci tarafı canlılık daraltması **korundu** (anlık tepki), sunucu artık aynı
  yüklemi sayfalamadan önce uyguluyor. `liveness_mismatch` debug olayı eklenmedi.
- Testler: `internal/liveness/liveness_test.go`, `internal/agent/liveness_test.go`.

### R3 — Workspace olay günlüğü (sıralı, epoch'lu, replay'li)

**Neden.** Boşluk 3. `SessionHub` oturum başına seq + ring + epoch + `since`
cursor'ını zaten çözmüş (`58`). Aynı sözleşme workspace kapsamına gerekiyor;
`notify` biçimli (başlık/gövde) olaylar canlı şerit için yeterli değil, yapısal
payload lazım.

**Ne.**

- `internal/sessionhub` → kapsam anahtarı genelleşir: `(ws, "session", id)` ve
  `(ws, "workspace")`. Ring/seq/epoch/`since` kodu **aynen** yeniden kullanılır.
- Yeni yapısal olay türleri (JSON payload, `notify` değil):
  `session_lifecycle {sessionId, rootId, origin, state, reason}`,
  `spawn {parentId, childId, depth}`, `report {childId, parentId, status}`,
  `automation_fire {automationId, triggerSessionId, sessionId|runId, outcome:
  fired|skipped, reason}`, `schedule_armed {scheduleId, fireAt}`,
  `flow_run {runId, rootRunId, status}`, `trajectory {id, revision}` (F1'de).
- `GET /api/workspace/stream?since=&epoch=` — `session_stream.go` ikizi.
  Eski `/api/events` **kalır** (toast/bildirim yolu); `refreshSignals` kademeli
  olarak yapısal olaylara geçer, önce yalnız yeni ekran kullanır.
- Kaynaklar: R1 oturum hook'u, R2 registry, R5 ateşleme defteri, R7 gözlemci,
  scheduler `armWakeLocked`/`rebuildLocked`.

**Etki alanı.** Büyük ama eklemeli: mevcut hub tüketicileri değişmez. Cutover
riski `58`'de yaşandı; epoch-reset ve gap-detection mantığı birebir korunur.

**Test.** Seq monotonluğu, `since` gap-fill, epoch mismatch → reset, yavaş abone
düşürme, ring'in uçuştaki olayı asla evict etmemesi (mevcut test genişler).

**Boyut:** L. **Açar:** workspace canlı görünüm, çoklu pencere tutarlılığı.

**Gerçekleşen (2026-09-02, dal `rota/r3-workspace-eventlog`).** Sapmalar:

- `sessionhub` **genelleştirilmedi**, rezerve kapsam id'siyle (`"\x00workspace"`)
  aynı ring/seq/epoch üzerinden `PublishWorkspace`/`SubscribeWorkspace`/
  `ReplayWorkspace` eklendi; `Publish` gövdesi `publish(key, ev, ephemeral,
  ringCap, autoCommit)` çekirdeğine çekildi. Workspace yayınları anında commit
  (taze abone hiçbir şey replay etmez), ring 4×.
- Runtime → API köprüsü yeni bir arayüz yerine **mevcut bus** üzerinden:
  `events.Event.Data` alanı + `ws:` ön ekli türler (`events.TypeWS*`),
  `Runtime.emitWorkspaceEvent` tek yayın noktası (`wsevents.go`); `api.bridgeBusToHub`
  bunları workspace kapsamına köprüler, `/api/events` (`sseEventName`) atlar.
- Uç: `GET /api/workspace/stream` (`workspace_stream.go`), oturum akışıyla aynı
  hello/reset/hub sözleşmesi.
- Kaynaklar: `db.SetSessionHook` + `SetTrajectoryHook` → `Runtime.OnSessionChange`/
  `OnTrajectoryChange` (manager'da wire), flow koşusu (start/waiting/resume/finish/
  timeout), schedule arm (cron + wake), otomasyon `fired`. `spawn`/`report` R7'ye
  bırakıldı; `automation_fire skipped` R5'e.
- Frontend: `api/hubStream.ts` ortak döngü (sessionStream refactor edildi, dış API
  aynı), `api/workspaceStream.ts`. `refreshSignals` geçişi yapılmadı (yeni ekran
  gelince). Detay: `58-QUEUE-SENKRON.md` son bölüm.

### R4 — Oturum sidecar soyutlaması + Rota deposu iskeleti

**Neden.** Boşluk 4. `inbox_durability.go`'daki karantina deseni
(`.corrupt-<unix>`) ve `inflight.json`'daki throttle/atomik yazım genelleşmeli;
`trajectory.json` ilk müşteri olur.

**Ne.**

- `db.Sidecar[T any]` (`sidecar.go`): `Load() (T, bool, error)`, `Save(T) error`
  (tmp+rename), bozuk dosya → karantina + `DebugError` olayı, oturum kilidi
  `transcript_lock.go` ile aynı sıra kuralı (sidecar kilidi → `mu` asla tersi).
- `store_trajectory.go`: `Trajectory` CRUD, `Revision` CAS (`UpdateTrajectory(id,
  expectedRev, fn)` → 409), `trajectories/index.json` (id, root, templateRef,
  status, metrik özeti; yalnız optimizer/liste okur), `RTA` ön eki `counters.go`.
- `inbox.json` ve `progress/` fırsat buldukça `Sidecar`'a taşınır (ayrı PR, davranış
  aynı).
- `internal/db` dizin haritasına (`CLAUDE.md`) `sidecar.go`, `store_trajectory.go`
  satırları eklenir.

**Test.** Karantina Windows'ta açık dosya (`drainSpawns` deseni), CAS çakışması,
index ile sidecar tutarlılığı.

**Boyut:** M. **Açar:** F1.

**Gerçekleşen (2026-09-02, dal `rota/r4-sidecar-trajectory`).** Sapmalar:

- `Sidecar[T]` karantinada `DebugError` olayının adı `sidecar_corrupt`; debug günlüğü
  okuyucusunun ad allowlist'ine bu adla birlikte 38'de belgelenmiş ama okuyucuda
  eksik olan dört ad (`mcp_server_gate_malformed`, `tool_permission_config_malformed`,
  `inbox_corrupt`, `orphan_recovery_failed`) da eklendi — aksi halde okurken parmak
  izine dönüşüyorlardı.
- Trajectory modeli brifteki şekle ek olarak `Meta map[string]string` (projeksiyon
  katmanının şerit sayacı gibi küçük verisi) ve düğümde `RefKind/RefID` (view.Ref
  köprüsü, import döngüsü olmadan) taşıyor. `Validate()` yapısal: benzersiz id,
  kenar uçları, `PhaseID` yalnız faz düğümüne.
- `UpdateTrajectory` iki modlu: `expectedRev>0` CAS (UI/ajan), `0` koşulsuz ekleme
  (runtime gözlemcisi, kilit altında). Kimlik alanları `fn`'den korunur.
- İndeks yoksa boot taramaz (ilk yazım indeksi kurar); **bozuksa** karantina +
  sidecar taraması. `RebuildTrajectoryIndex()` onarım aracı olarak dışa açık.
- Kök oturum silinince indeks satırı `deleteSession` içinde, kilitler bırakıldıktan
  sonra düşer (kilit sırası: rota kilidi → indeks → `mu`).
- inbox/progress'in `Sidecar`'a taşınması bu dalda **yapılmadı** (ayrı PR).
- HTTP uçları yok; F1'de gelir. Testler: `sidecar_test.go`, `store_trajectory_test.go`.

### R5 — Otomasyon çekirdeği: tetik kaydı, ateşleme defteri, arşiv

**Neden.** Boşluk 5. `phase`/`trajectory_end` tetikleri switch'e iki dal daha
eklemek yerine kayıt olabilmeli; "neden ateşlenmedi" UI'da görünmeli; küratör
budaması için arşiv gerekli.

**Ne.**

- `automation_trigger.go`: `TriggerSpec{Kind, Validate(a), Match(ev), Context(ev)
  FireContext}` registry; mevcut dört tetik dosya başına ayrılır
  (`trigger_tag.go`, `trigger_board.go`, `trigger_token.go`, `trigger_counter.go`),
  `ValidateAutomationShape` registry'ye delege eder. `guardsPass` ve `dispatchFire`
  ortak kalır.
- `automations/<id>/fires.jsonl`: her ateşleme **ve atlama** (`cooldown`,
  `max_iterations`, `expired`, `disabled`, `autonomy_paused`, `target_missing`)
  sebep + tetikleyen oturum + üretilen oturum/run ile. `LastSessionID` türetilmiş
  hale gelir. Cap: son 500 kayıt.
- `Archived bool` + `SetArchived` → `Automation`, `Schedule`, `Hook`; liste uçları
  `archived` filtresi; UI kartlarında arşiv düğmesi; tükenmiş/expired satırlar
  yine yalnız **devre dışı** kalır (arşiv kararı küratöre, F3).
- `Hook.FireCount/LastFiredAt` telemetri (`hooks.go RunLifecycleHooks`).
- `dispatchFire` → `LaunchRun(Spawn.Origin{automation…})` (R1).

**Test.** Registry ile mevcut 4 tetik testleri değişmeden geçer; defter kaydı her
atlama nedeni için; arşivli kural ateşlenmez; şekil doğrulaması registry üzerinden.

**Boyut:** M. **Açar:** F2, F3 küratör.

**Gerçekleşen (2026-09-02, dal `rota/r5-automation-core`).** Sapmalar:

- Tetik registry'si **yalnız doğrulama** tarafını kapsıyor: `db.TriggerSpec{Kind, Label,
  Validate, NoTarget}` + `RegisterTrigger`/`TriggerKinds`/`ValidTriggerKind`
  (`automation_trigger.go`); `ValidateAutomationShape` switch yerine registry'ye
  delege ediyor, API'deki sabit `triggerKind` listesi de buradan. Match/dispatch
  tarafı motorda kind başına mevcut giriş noktalarında kaldı (dört tetik dosya
  başına **bölünmedi**); `phase`/`trajectory_end` F2'de `RegisterTrigger` + motora
  yeni `On*` metoduyla girer.
- Ateşleme defteri `automation-fires/<id>.jsonl` (`store_automation_fires.go`):
  `fired`/`skipped`/`failed`, sebep sabitleri (`archived`, `expired`, `cooldown`,
  `max_iterations`, `absolute_backstop`…), 600'ü geçince en yeni 500'e budama,
  silmede temizlenir; `GET /api/automations/{id}/fires`. `guardsPass` →
  `guardReason` + `recordSkip` (defter + `ws:automation_fire skipped`);
  `notifyFired`/`recordFailure` de deftere yazar.
- `Archived` alanı Automation/Schedule/Hook'ta; `Set*Archived` + `POST
  /api/{automations|schedules|hooks}/{id}/archive`; enabled-listeler arşivliyi
  eler; liste uçları arşivliyi varsayılan gizler (`?archived=true` yalnız
  arşivliler). `Hook.FireCount/LastFiredAt` her komut koşusunda güncellenir
  (CLI'ın kendi koşturduğu hook'lar sayılmaz).
- UI: Otomasyon kartında "Arşivle" eylemi; arşivlileri listeleyen ekran yok (F3
  küratör ekranına bırakıldı), schedule/hook için yalnız API + tipler.
- Testler: `db/automation_core_test.go`, `agent/automation_ledger_test.go`.

### R6 — Reçete frontmatter'ı tipli şema + sürüm

**Neden.** `coordinator-wf-*` skill'leri serbest frontmatter (`worker_targets`,
`stop_condition`, `max_turns`). `phases:` bloğu doğrulanmadan giremez; sürüm yok;
`Session.CoordinatorWorkflow` yalnız slug.

**Ne.**

- `skills.RecipeSpec{Version int, WorkerTargets, StopCondition, MaxTurns,
  Phases []PhaseSpec, Watchers []string, Optimizer string}`; `PhaseSpec{ID,
  Profile, Gate *GateSpec, Watchers, MaxRounds, Optional}`. Yüklemede parse +
  validate; bozuk → skill yüklenir, `RecipeSpec=nil`, `warn` olayı + UI rozeti.
- `Session.CoordinatorWorkflow` "slug@version" biçimine geçer (okuyucu her iki
  biçimi kabul eder).
- Seed ledger (`.shipped-versions.json`) reçete sürümünü izler; kullanıcı
  düzenlemesi `edited` durumu (lens deseni).
- `CoordinatorWorkflowPicker` sürümü gösterir.

**Test.** Şema doğrulama tablosu; eski slug-only oturum okuma; seed sürüm bump.

**Boyut:** M. **Açar:** F1 tohumlama, F3 sürümleme. **Not:** `internal/skills`
eşzamanlı değişti; rebase ile başlanır.

**Gerçekleşen (2026-09-02, dal `rota/r6-recipe-schema`).** Sapmalar:

- `RecipeSpec` **eski alanları kapsamıyor**: `WorkerTargets/StopCondition/MaxTurns`
  `Skill` üzerinde kaldı (mevcut tüketiciler); `RecipeSpec{Version, Phases,
  Watchers, Optimizer}` + `PhaseSpec{ID, Label, Profile, Gate, Watchers,
  MaxRounds, Optional}` yalnız yeni yapısal kısmı taşır. `Skill.Recipe` /
  `Skill.RecipeError`.
- Frontmatter ayrıştırıcısına `blocks` (ham iç içe blok yakalama) eklendi;
  `phases` için özel `parsePhaseBlock` (skaler / `[..]` / `{ k: v }`). YAML
  bağımlılığı **eklenmedi**.
- Sürüm damgası `SpawnWorker` ve flow koordinatör düğümünde; `createSessionLocked`
  ajan varsayılanı çıplak slug bırakır (db skills'i göremez), okuyucular ikisini
  de kabul eder. Seed ledger'a ayrı sürüm izi **eklenmedi**: gönderilen dosyanın
  değişmesi mevcut hash-ledger ile `default/tuned/edited` durumuna zaten düşer.
- Picker: "Fazlar: plan (planner) → …" satırı, `(vN)` etiketi ve geçersiz blok
  uyarısı. `_Docs/47` §16.
- Testler: `internal/skills/recipe_test.go` (blok sızıntısı yok, şema, ref, store
  yükleme + geçersiz işaretleme, gönderilen reçeteler geçerli).

### R7 — Koordinasyon gözlemci arayüzü

**Neden.** Rota'nın `observed` düğümleri `SpawnWorker`, `NotifyCoordinator`,
`ReportToCoordinator`, `drainCoordinator`, stall-halt içinde doğar. `coordination.go`
içine bir katman daha kod koymak yerine gözlemci çıkarılır.

**Ne.**

```go
type CoordinationObserver interface {
    OnSpawn(parent, child db.Session, spec WorkerSpec)
    OnReport(child, parent db.Session, status string, noteLen int)
    OnDrain(coordinator string, pending int, phase string) // start|end
    OnStall(coordinator string, tier string)
}
```

- Runtime'da observer listesi; mevcut `emit*` çağrıları observer'a taşınır. İlk
  abone R3 olay yayını, ikinci abone Rota bağlayıcısı (F1).
- Aynı desen: `FlowObserver` genişler (`OnRunStart/End`, mevcut `NodeEvent` kalır),
  `scheduler.run` → `ScheduleObserver{OnArm, OnFire, OnSkip}`.

**Test.** Observer sırası ve panik izolasyonu (bir abone panikse diğerleri çalışır);
mevcut olay testleri observer üzerinden aynı çıktıyı üretir.

**Boyut:** M. **Açar:** F1.

**Gerçekleşen (2026-09-02, dal `rota/r7-coordination-observer`).** Sapmalar:

- Arayüz olay yapılarıyla: `OnSpawn(SpawnEvent)`, `OnReport(ReportEvent)`,
  `OnDrain(DrainEvent{turn_start|turn_end, Turn})`, `OnStall(StallEvent)`
  (`coordination_observer.go`); `Runtime.AddCoordinationObserver`. Yerleşik
  workspace-stream yayını abone listesinde değil, `observe*` içinde koşulsuz önce
  çalışır; aboneler panic-izole.
- Yayınlar: `ws:spawn`, `ws:report` (durum `<status>` etiketinden okunur,
  `lastWorker`, araç sayısı), `ws:coordination` (drain turn_start/turn_end,
  stall_halt) — planda `coordination` türü yoktu.
- `FlowObserver`/`ScheduleObserver` genişletmesi **yapılmadı**: flow ve schedule
  olayları R3'te doğrudan yayınlandığı için ayrıca gözlemci gerekmedi.
- Mevcut `emit*` çağrıları taşınmadı; gözlemci mevcut yayınlara **ek** olarak
  çağrılıyor (kademeli geçiş).
- Test: `coordination_observer_test.go` (spawn + rapor abone ve akışa ulaşır,
  panikleyen abone izole, not etiketi ayrıştırma).

### R8 — Flow ↔ oturum bağlantı düzeltmeleri

**Neden.** `15`/`62` belgelenmiş sınırlar: `SessionID` bitişte; `run_flow` child
run'da `ParentNodeID` yok (ağaçta rozet/iniş yok); flow-run GC yok;
`executions.go lastStatusFor` eski run-oturumuna yeni run'ın çipini gösterir.

**Ne.** R1 ile `SessionID` yaratmada + `ParentNodeID`; `lastStatusFor` Origin'deki
`RunID`'yi kullanır; `DELETE /api/flow-runs/{id}` + retention sweeper
(`FlowRunRetention`, insight `RunSessionRetention` deseni); koordinatör düğümü
oturumu `Origin{Kind: flow, RunID, NodeID}` alır (Kind'a dokunulmaz).

**Boyut:** S-M. **Açar:** F1 flowrun düğümleri.

**Gerçekleşen (2026-09-02, dal `rota/r8-flow-linkage`).** Sapmalar:

- `SessionID` yaratmada ve koordinatör düğümü kökeni R1'de yapılmıştı; burada
  `lastStatusFor` `Origin.RunID` → koşu durumu (`flowRunStatusByID` indeksi) +
  eski oturumlar için akışın en yeni koşusuna düşüş.
- `DELETE /api/flow-runs/{id}` **ağaç** siler (üye adı yeter), canlı üyede 409
  (`db.ErrConflict`); adım sidecar'ları runtime'da (`Runtime.DeleteFlowRun`),
  `ws:flow_run status=deleted` yayını.
- Saklama uygulama ayarı olarak (`Settings.FlowRunRetention`, 0 = sınırsız,
  0..1000 clamp), `Tunables.FlowRunRetention`, süpürücü aynı 10 dk kadansında.
  Insight'ın oturum retention deseni gibi ayrı bir "run session" saklaması
  **eklenmedi**; koşu transkript oturumlarına dokunulmuyor.
- `run_flow` child run'ının `ParentNodeID`'si test edilmedi (araç çağrısı
  üreten sahte sağlayıcı gerektiriyor); `runLineage` düğüm id'sini ctx'ten okuyor.
- Testler: `db/store_flow_gc_test.go`, `agent/flow_gc_test.go`,
  `api/executions_originrun_test.go`.

### R9 — View katmanı + Graph API

**Ne.** `view.KindTrajectory`, `ProjectTrajectory` (tiny/card/full, `Elided`),
`Children(session)` faz handle'ları, `Sources.Liveness` (R2). `graph.go`:
`spawned` (coordinator→worker) ve `forked_from` kenarları, `?scope=live|recent`;
Ağ ekranı efsanesine iki kenar. Bu, Ağ'daki "koordinatör ağacı görünmüyor"
boşluğunu R1 sonrası bir günde kapatır.

**Boyut:** S-M. **Açar:** F0.

**Gerçekleşen (2026-09-02).** `view.KindTrajectory` + `ProjectTrajectory`
(`internal/view/trajectory.go`): tiny yalnız başlık (id, kök oturum, şablon,
durum, revizyon); card faz zinciri (`✓ plan → ● code → ○ review`), gözlenen
sayılar (oturum/akış koşusu/otomasyon/kapı/hayalet), ilan/gözlem/kenar sayısı;
full her faz için kapı+sebep+süre ve her bağlı düğüm için `⌘ session W1 [active]
⊂ p:code` satırı. Handle'lar bağlı varlıklar (session/flowrun/automation ref'i),
12'den fazlası `Elided` (birim "düğüm"). `Store` arayüzü `GetTrajectory` +
`GetTrajectoryByRoot` aldı; `Children(session)` kök oturumda rota handle'ını
worker'ların **önüne** ekler, `Children(trajectory)` bağlı varlıkları listeler,
`IsExpandable(trajectory)=true`. **Sapma:** "faz handle'ları" yerine tek bir
rota handle'ı — fazlar ayrı projeksiyon değil, rotanın gövdesi; `Sources.Liveness`
eklenmedi (R2 anlık görüntüsü `view`e değil `graph`/API'ye akıyor, F0'da
gerekirse eklenir). `graph.go`: canlı kapsamdaki `run:` düğümleri arasında
`sess.Lineage()` ile `spawned` (coordinator/subagent → worker) ve `forked_from`
(handoff/spawn/automation → yeni kök) kenarları, iki uç da payload'da yoksa
çizilmez; `?scope=recent` son 1 saatte güncellenen arşivsiz oturumları
`liveScope: "recent"` ile ekler (`addRecentGraphScope`); `stats.lineage`.
Frontend: `WorkspaceGraphEdge.kind` iki yeni tür, `EDGE_COLOR`/`EDGE_LEGEND`
(turuncu "worker açtı", mor "çatallandı"), `LIVE_SCOPE_LABEL.recent`. Yan
bulgular: R6'dan kalan `Skill.version` tip eksiği (tsc hatası) ve R8'den kalan
`flowRunRetention`'ın settings golden listesinde olmaması düzeltildi. Testler:
`view/trajectory_test.go`, `api/graph_lineage_test.go`.

### R10 — Frontend altyapısı

**Ne.** `api/workspaceStream.ts` (`sessionStream.ts` ikizi: cursor, epoch, gap,
backoff), `shared/lib/laneStore.ts` (artımlı şerit deposu; `useSyncExternalStore`,
revizyon ile stale reddi), yeni view kaydı (`url.ts VIEWS`, `viewRegistry`,
`navItems`, `lazyPanels`), `Session.origin`, `Automation/Schedule/Hook.archived`
tipleri, `workspace.ts` `BoardFilter.review` düzeltmesi (yan bulgu).

**Boyut:** M. **Açar:** F0 ekran iskeleti.

**Gerçekleşen (2026-09-02).** `api/workspaceStream.ts` R3'te gelmişti; burada
taşıma-bağımsız sözlük `api/workspaceEvents.ts`'e ayrıldı (kind sabitleri,
payload tipleri, `dataOf`) — `api/client` import anında `localStorage` okuduğu
için saf reducer ve node ortamlı testler artık transport'a dokunmadan çalışıyor;
`workspaceStream.ts` bunları yeniden dışa aktarır. Şerit deposu üç dosya:
`shared/lib/laneModel.ts` (`LaneState`: sessions/trajectories/flowRuns/armed
Map'leri, fires + activity halkaları [200], `revision`/`head`/`connected`/
`stale`, `rootLanes`/`laneMembers`/`trajectoryForRoot`), `laneReducer.ts` (saf,
immutable: `seedSessions` REST satırından eskiyse akıştan gelen başlığı korur,
`seedLiveness`, `applyLaneEvent` — oturum/akış koşusu `updatedAt`, rota
`revision` ile stale reddi; silme her zaman uygulanır; reddedilen olay AYNI
nesneyi döndürür, `revision` artmaz), `laneStore.ts` (modül deposu +
`useSyncExternalStore`; `connectLanes` ref-sayımlı akış, `seedLanes`,
`resetLanes`, `dispatchLaneEvent`). Görünüm kaydı: `View` birliği + `url.ts
VIEWS` + `VIEW_TITLE`/`HEADERLESS_VIEWS` + `NAV` (`Waypoints` ikonu) +
`lazyPanels.RotaPanel` + `App.tsx` blok; `features/rota/` iskeleti (RotaPanel:
akış → seed → şerit listesi, RotaLane, RotaActivity: tetik/koordinasyon/kurulu
zamanlayıcı şeridi, rotaLabels) — F0 kanvası bu veri yolunu değiştirmeden gövdeyi
değiştirecek. `Session.origin` (R1) ve `Automation/Schedule/Hook.archived`
(R5) tipleri zaten vardı. `BoardFilterBar` `dep`/`review` alanlarındaki
`as never` dökümleri `BoardDepFilter`/`BoardReviewFilter` ile değiştirildi (yan
bulgu). Testler: `laneReducer.test.ts` (seed/gruplama, stale reddi, revizyon,
liveness, halkalar, bağlantı bayrakları), `rotaLabels.test.ts`.

## 2. Dalgalar

```
Dalga A (seri, temel)      R1 ──► R4
Dalga B (paralel, A'ya bağlı)  R2   R3   R5   R6   R8
Dalga C (B'ye bağlı)       R7 (R3)   R9 (R1+R2)   R10 (R3)
──────────────────────────────────────────────────────────
Rota F0 (projeksiyon ekranı) buradan sonra başlar.
```

> **F0 uygulandı (2026-09-02):** workspace-kök zaman ekseni kanvası
> (`features/rota/`), `GET /api/trajectories[/{id}]` okuma uçları,
> `ws:session_lifecycle.createdAt`. Ayrıntı ve bilinçli sınırlar:
> `78-ROTA-EKRANI.md`.
>
> **F1a uygulandı (2026-09-02, dal `rota/f1a-trajectory-entity`):** rota
> varlığı artık üretiliyor — reçeteli kök koordinatör yaratılışta (R6 `phases:`
> → declared faz/ghost watcher/optimizer düğümleri), reçetesiz ilk spawn'da;
> R7 gözlemcisi (`trajectoryBinder`) spawn/rapor/stall'ı, `emitFlowRunEvent`
> akış koşularını, oturum hook'u çatallanan kökleri ve arşivi, Durable Ask
> insan kapılarını bağlar. `trajectory` aracı (`get|plan|phase|finish`) +
> `<trajectory>` durum bloğu. Model eki: `TrajectoryNode.Optional`; `Gate`
> alanı artık `gate` JSON anahtarıyla yazılır. Ayrıntı: `78-ROTA-EKRANI.md` §5.
>
> **F1b uygulandı (2026-09-02, dal `rota/f1b-visibility`):** R1 (köken), R3/R7
> (`ws:*`), R4/F1a (rota grafı), R5 (ateşleme defteri), R6 (reçete şeması), R9
> (`KindTrajectory`) yeniliklerinin ekran karşılıkları — rota-içi faz görünümü,
> derin bağlantı, sohbet başlığı şeridi, köken çipi, reçete çipleri, ateşleme
> listesi, `ws:*` → toast köprüsü. Ayrıntı: `78-ROTA-EKRANI.md` §6.
>
> **F2 uygulandı (2026-09-02, dal `rota/f2-automations-on-graph`):** `phase` ve
> `trajectory_end` tetik türleri (R5 registry'sine kayıt, `trajPhase/trajRecipe/
> trajEvent/trajStatus`), rota geçiş algılayıcısı + `SetTrajectoryTransitionHook`,
> reçete `watchers:` hayaletlerinin gerçekten ateşlenmesi ve her denemenin grafa
> "neden ateşlenmedi" sebebiyle yazılması; otomasyon panosunda iki yeni şerit.
> Ayrıntı: `78-ROTA-EKRANI.md` §7.
>
> **F3 uygulandı (2026-09-02, dal `rota/f3-metrics-curator`):** rota bitiş
> özeti (`TrajectorySummary`, indekste kopya, `…/summarize`), reçete
> istatistikleri (`GET /api/trajectories/recipes`), LLM'siz haftalık küratör
> (`RunCurator`: arşivle/öner, provenance kapısı, pin muafiyeti, rapor) +
> `Pinned` alanları ve pin uçları; ekranda özet çipleri, reçete istatistik
> bloğu, Küratör paneli, 📌. Ayrıntı: `78-ROTA-EKRANI.md` §8.
>
> **F4 uygulandı (2026-09-03, dal `rota/f4-optimizer`):** `recipe-optimizer`
> sistem ajanı + promptu, eşik/başarısızlık/elle tetikli `RunRecipeOptimizer`,
> kodda zorlanan değişmezler (kanıt, olumsuz yargı yasağı, büyüme bütçesi
> `RecipeGrowthBudget=9` — yüklemede de), öneriler `recipe-opt` içgörü kanalında
> (`insight.RecipeProposal`); v1 uygulamaz. Ayrıntı: `78-ROTA-EKRANI.md` §9.
>
> **F5 + F4-v2 uygulandı (2026-09-03, dal `rota/f5-gates-canvas`):** faz kapıları
> (artifact/verdict/human — insan kapısı Durable Ask kartı, `ws:ask` köprüsü,
> `ResolvePhaseGate`), ortak `SetTrajectoryPhase/PlanTrajectory/FinishTrajectory`
> (CAS), kanvas API'leri + PhaseActions/ForkModal, RunView → Rota, `auto_prune`
> ile budama önerilerinin otomatik uygulanması (`skills.ApplyRecipeProposal`).
> Brif §11 yol haritası tamam. Ayrıntı: `78-ROTA-EKRANI.md` §10.

| Dalga | Kalemler | Paralellik | Kapı |
|-------|----------|------------|------|
| A | R1, R4 | seri | Origin her create yolunda; eski header'lar okunur; `Sidecar` inbox testlerini geçer |
| B | R2, R3, R5, R6, R8 | 5 ayrı worktree | `runningSessionIDs` silindi; `/api/workspace/stream` gap-fill; 4 tetik registry'de; reçete şeması; run GC |
| C | R7, R9, R10 | 3 ayrı worktree | Ağ'da koordinatör kenarı görünür; observer üzerinden aynı olaylar; yeni view iskeleti boş açılır |

Her dalga sonunda: `go test ./... -count=1` (`TIONHARNESS_ENABLE_SHELL=1`),
`cd frontend && npm test && npx tsc --noEmit`, `scripts\e2e-smoke.ps1 -SkipLLM`,
`git diff --check`. Ek regresyon listesi: sidebar çip sayıları, Ağ canlı kapsamı,
RunView canlı statü, koordinatör tur drain'i, Durable Ask devamı, çoklu pencere
senkronu.

Kaba büyüklük: A ≈ 1 hafta, B ≈ 2 hafta (paralel), C ≈ 1 hafta. Toplam 4 hafta
tek kişi; paralel ajanlarla B sıkışır.

## 3. Riskler ve karşılıkları

| Risk | Karşılık |
|------|----------|
| Eşzamanlı oturum çakışması | Kalem başına worktree; `05-ILERLEME` yalnız kapanışta; `internal/skills` için rebase önce |
| Session header `v` 3→4 | Yalnız yeni alan; eski dosya sıfır değerle okunur; backfill bellek içi |
| `SessionHub` genelleştirmesi (58 cutover'ı) | Kapsam anahtarı ekleme, mevcut yol değişmez; epoch-reset birebir |
| Canlılık kaynağı değişince çip/Ağ regresyonu | Eski ve yeni kaynağı bir sürüm boyunca karşılaştıran debug olayı (`liveness_mismatch`) |
| Windows'ta sidecar karantina testleri | `drainSpawns` deseni; `t.TempDir` temizliğinden önce goroutine bekleme |
| Otomasyon registry'sinde davranış kayması | Mevcut testler değişmeden geçmek zorunda; `fires.jsonl` yalnız ek |
| Maliyet | Sıfır: bu plan tamamen deterministik, LLM çağrısı yok |

## 4. Kapsam dışı (bilinçli)

- Flow motoru sınırları (`maxSteps=50`, loop resume, paylaşımlı `Iter`) — Rota'yı
  engellemez, ayrı iş.
- `Session.Kind` sadeleştirme / yeniden adlandırma.
- `App.tsx` (995 satır) bölme; `coordination.go` tam bölme (yalnız gözlemci çıkarımı).
- Kanban dispatcher (pano pasif kalır).
- LLM optimizer, küratör LLM geçişi (F3/F4'te, bu plan sonrası).

## 5. Kabul ölçütleri

1. Her oturumun `Lineage()` cevabı var; Ağ ekranı koordinatör→worker kenarını çizer.
2. `GET /api/sessions?chips=running` sunucuda doğru sayar; istemci tarafı canlılık
   istisnası kodda yok.
3. `/api/workspace/stream` kopuşta `since` ile boşluğu kapatır; iki pencere aynı
   olay sırasını görür.
4. `trajectory.json` `Sidecar` üzerinden yazılıp bozulunca karantinaya düşer;
   revizyon çakışması 409 döner.
5. Dört mevcut tetik registry'den geçer, `fires.jsonl` atlama nedenlerini yazar,
   arşivli kural ateşlenmez.
6. Reçete `phases:` şeması doğrulanır; bozuk şema skill'i düşürmez.
7. Tüm test setleri ve smoke yeşil; `git diff --check` boş.
