# TionSwarm — `schedule_wake`: Ajanın Kendi Sohbetine Geri Dönmesi

## Neden gerekti?

TionSwarm'nun `claude-cli` sağlayıcısı `claude -p --output-format stream-json` ile çalışır — her tur bir **tek-seferlik alt süreç**; tamamlanınca ölür. Claude Code'un yerleşik `ScheduleWakeup` aracı yalnızca `claude /loop` harness bağlamında anlamlıdır; bu harness TionSwarm'da yoktur. Sonuç: ajan "bekliyorum, 5 dakika sonra devam edeceğim" deyip `ScheduleWakeup` çağırıyordu, fakat hiçbir şey olmuyor, sohbet orada bitiyordu.

## Mimari

```
Ajan turunda (native veya CLI)
   └─ schedule_wake(delay, prompt, reason)
         │  (Interaction MCP → mcp_interaction.go → Runtime.ScheduleWake)
         │
         ▼
   DB: Schedule { OneShot=true, FireAt, SessionID, AgentID, Prompt }
         │
         ▼
   Scheduler.armWakeLocked → time.AfterFunc(delay, fireWake)
         │
         ▼  (delay sonra)
   fireWake → deliverWake(sc)
         ├─ DB: user mesajı yaz (sc.Prompt)
         ├─ invokeTraced(sc.SessionID) → asistan yanıtı yaz
         ├─ emitWakeEvent(phase=start) / emitWakeEvent(phase=done)
         └─ DB: one-shot satırını sil
```

## Dosyalar

| Dosya | Değişiklik |
|---|---|
| `internal/db/models_task.go` | `Schedule`: `OneShot`, `FireAt`, `SessionID`, `Reason` alanları |
| `internal/agent/scheduler.go` | `wakeTimers`, `rebuildLocked`, `armWakeLocked`, `fireWake`, `deliverWake`, `emitWakeEvent`, `Stop` |
| `internal/agent/runtime.go` | `ScheduleWake()`, `MinWakeDelaySec=5`, `MaxWakeDelaySec=3600` |
| `internal/tools/builtin_wake.go` | `WakeFunc`, `WithWakeScheduler`, `wakeFrom`, `ScheduleWakeTool` |
| `internal/agent/toolsetup.go` | `NewScheduleWakeTool()` built-in listesine eklendi |
| `internal/api/chat_stream.go` | `wakeFn` closure; `WithWakeScheduler` + `setWakeScheduler` |
| `internal/api/chat_control.go` | `chatRun.wake` alanı + getter/setter |
| `internal/api/mcp_interaction.go` | `schedule_wake` araç tanımı + `callWake` dispatch |
| `internal/api/mcp_interaction.go` | `schedule_wake`, `interactionToolSpecs`'e eklenir (extended tier); allowlist `inter.Core/ExtendedToolNames`'ten otomatik türer (tek kaynak). CLI'da `mcp__tionswarm_extended__schedule_wake` |
| `internal/agent/climcp.go` | `"ScheduleWakeup"` (CLI native) → disallowed (TionSwarm'nun schedule_wake'i yerine geçer) |
| `internal/api/schedules.go` | One-shot satırları liste filtresi |
| `internal/tools/builtin_schedulemgmt.go` | One-shot satırları `list_schedules` filtresi |
| `frontend/src/App.tsx` | `chat` event: `phase=start` → `markPending`, `phase=done` → `clearPending` |
| `internal/agent/wake_test.go` | 3 birim testi |

## Araç parametreleri

```json
{
  "name": "schedule_wake",
  "description": "Schedule a delayed self-wakeup in the current chat session.",
  "parameters": {
    "delaySeconds": { "type": "integer", "description": "Delay in seconds (5–3600)" },
    "prompt":       { "type": "string",  "description": "Message to deliver back into this chat" },
    "reason":       { "type": "string",  "description": "Human-readable reason for the wakeup" }
  }
}
```

**Native provider yolu:** `ScheduleWakeTool.Call` → `wakeFrom(ctx)` → `Runtime.ScheduleWake`.

**claude-cli yolu:** `mcp__tionswarm_extended__schedule_wake` (NameOnly/extended tier → ToolSearch ile lazy gelir) → `mcp_interaction.go callWake` → `run.wakeScheduler()` → aynı `Runtime.ScheduleWake`. CLI'ın kendi `ScheduleWakeup` aracı `disallowed` listesinde (işe yaramaz, kafa karıştırır).

## Gecikmeler ve sınırlar

- Minimum: `MinWakeDelaySec = 5` saniye (1s altı istekler sıkıştırılır)
- Maksimum: `maxWakeDelay = 1 saat`
- Timer çakışması: `rebuildLocked` yeniden çağrıldığında eski timer iptal edilir
- One-shot tüketimi: `fireWake` timer teti sonrasında ilgili DB satırını siler
- Hata yönetimi: `invokeTraced` başarısız olursa, hata metni asistan mesajı olarak sohbete yazılır (sohbet askıda kalmaz)

## Frontend thinking göstergesi

`emitWakeEvent` iki kez çağrılır:
1. `phase=start` — `deliverWake` başlamadan önce → `chat.markPending([sessionId])`
2. `phase=done` — yanıt yazıldıktan sonra → `chat.clearPending(sessionId)`

Bu sayede kullanıcı, wake tetiklenip yanıt gelene kadar oturumda "düşünüyor…" göstergesi görür.

## Testler

```
internal/agent/wake_test.go
  TestScheduleWake_ArmsOneShot          — DB satırı, OneShot=true, delay sıkıştırma, boş sessionId reddi
  TestDeliverWake_TargetsOriginalSession — wake orijinal sohbet oturumuna enjekte edilir; hata mesajı inline
  TestScheduler_StartSkipsOneShotCron   — cron tablosuna eklenmez; wakeTimers'da 1 timer kurulur
```

## Canlı test özeti (2026-06-18)

Playwright ile doğrulandı:
1. TionSwarm yeni oturumunda `schedule_wake` 10s ile tetiklendi
2. Agent "kurdum, bekliyorum" yanıtı verdi
3. 10 saniye sonra wake prompt aynı sohbet oturumuna otomatik enjekte edildi
4. Agent yeniden yanıt verdi — sohbet ekranda görünür şekilde devam etti

## Bekleme durumu UI'ı + iptal (2026-06-19)

`schedule_wake` çağıran turun **bittiğini** ama oturumun **otomatik devam etmeyi
beklediğini** kullanıcıya göstermek için bir yaşam-döngüsü eklendi. Önceden tur
bitince oturum "tamamlanmış" gibi görünüyordu; artık bir **bekleme banner'ı** +
**Durdur** kontrolü çıkar.

**Olay yaşam döngüsü** (`chat` event, `target.phase`):

| phase | Ne zaman | Frontend |
|---|---|---|
| `armed` | `Runtime.ScheduleWake` arming sonrası | Bekleme banner'ı (reason + canlı geri sayım) + Durdur |
| `start` | Wake tetiklendi (`deliverWake` başında) | Banner kalkar, "düşünüyor" göstergesi yükselir |
| `done` | Wake yanıtı yazıldı | Her şey temizlenir |
| `cancelled` | Kullanıcı Durdur'a bastı (`CancelWake`) | Banner temizlenir |

`armed`/`cancelled` olayları `Runtime.emitWakePhase` ile yayılır; `target` içinde
`reason` ve `fireAt` (unix sn) taşınır. `start`/`done` hâlâ `scheduler.emitWakeEvent`'ten gelir.

**İptal yolu:** `POST /api/chat/wake/cancel {sessionId}` → `Runtime.CancelWake`
oturuma ait bekleyen one-shot satır(lar)ı siler, scheduler'ı yeniden kurar
(timer iptal) ve `phase=cancelled` yayar.

**Steer/Sıraya alma:** Bekleme sırasında çalışan bir tur olmadığından canlı
steer/queue uygulanmaz; kullanıcı normal bir mesaj yazarsa bu yeni bir tur olur ve
bekleyen wake otomatik düşürülür (`sendMessage` banner'ı temizler).

| Dosya | Değişiklik |
|---|---|
| `internal/agent/runtime.go` | `emitWakePhase`, `CancelWake`; `ScheduleWake` armed yayar |
| `internal/api/chat_control.go` | `handleCancelWake` |
| `internal/api/server.go` | `POST /api/chat/wake/cancel` route |
| `frontend/src/api/chat.ts` | `cancelWake(sessionId)` |
| `frontend/src/hooks/useChatStream.ts` | `wakeWaits` durumu, `setWakeWait`/`clearWakeWait`/`cancelWake`, `activeWakeWait` |
| `frontend/src/components/chat/WakeWaitBanner.tsx` | bekleme banner'ı + geri sayım + Durdur |
| `frontend/src/App.tsx` | `phase` armed/start/cancelled/done dallanması + banner render |

## Async sohbette `ask_user` / `request_confirmation` (2026-06-19)

Wake ile teslim edilen tur `KindSchedule` (autonomous) bağlamında koşar; bu yüzden
`ask_user` eskiden `"...proceed without asking"` hatası veriyordu — oysa kullanıcı
**asenkron olarak orada** (sohbette okuyup yanıtlayabilir). `deliverWake`'in turu
artık `tools.WithAsyncChat` ile işaretlenir; `ask_user`/`request_confirmation` bu
durumda modele **"sorunu normal yanıtın olarak yaz ve turu bitir; kullanıcı sohbette
yanıtlar"** yönergesini döndürür (tahmine zorlamak yerine). Tam headless koşular
(flow) eski "proceed without asking" davranışını korur.

| Dosya | Değişiklik |
|---|---|
| `internal/tools/ask.go` | `WithAsyncChat`/`IsAsyncChat` marker |
| `internal/tools/builtin_ask.go` | async-chat dalında yönlendirici mesaj |
| `internal/tools/builtin_confirm.go` | async-chat dalında yönlendirici mesaj |
| `internal/agent/scheduler.go` | `deliverWake` turu `WithAsyncChat` ile damgalanır |

## Araç döngüsü iterasyon limiti (2026-06-19)

Native tool döngüsü sınırı `maxToolIters` **8 → 24** (3 kat) yükseltildi —
schedule_wake güdümlü çok-adımlı async akışlar limit'e takılmadan tamamlansın
diye. `TIONSWARM_MAX_TOOL_ITERS` env değişkeniyle (pozitif tamsayı) override edilebilir
(`internal/agent/toolloop.go`).

## Geçmiş-duyarlı wake turu (2026-06-23)

Önceden `deliverWake` → `invokeTraced` turu **yalnızca wake prompt'unu**
gönderiyordu (`Messages: [{user, prompt}]`) — uyanan ajan sürdürmesi gereken
**sohbeti hiç görmüyordu**. Artık wake, tam bir sohbet turu gibi kurgulanıyor:

- `Runtime.WakeTurnFunc` hook'u (api server kurar; `runtime.SetWakeTurnRunner`,
  manager `SetWakeTurnRunner` ile her runtime'a dağıtır — `autonomousInteraction`
  ile aynı desen).
- `api/wake_turn.go` → `wakeTurnRunner`: oturum geçmişini yükler, yazar
  etiketlemesi uygular (`labelMultiAgentHistory`), `convo.Prepare` + `composeTurnRequest`
  ile **tam zengin isteği** (geçmiş + özet + hafıza + goal + workdir + cwd) kurar,
  `CompleteWithToolsTraced(autonomous=true)` ile çalıştırır.
- Wake prompt'u `deliverWake` zaten **son kullanıcı mesajı** olarak yazdığı için
  geçmiş onu taşır — ayrıca eklenmez.
- Hook kurulu değilse (api server bağlamadan önce) eski prompt-only invoke'a düşer.

| Dosya | Değişiklik |
|---|---|
| `internal/agent/runtime.go` | `WakeTurnFunc` tipi, `wakeTurn` alanı, `SetWakeTurnRunner`, `WorkspaceID()` |
| `internal/agent/scheduler.go` | `deliverWake` hook'u kullanır (yoksa fallback) |
| `internal/workspace/manager.go` | `wakeTurnFactory` + `SetWakeTurnRunner` + openWorkspace uygulaması |
| `internal/api/wake_turn.go` | `wakeTurnRunner` (geçmiş-duyarlı tam tur) |
| `internal/api/server.go` | `manager.SetWakeTurnRunner(s.wakeTurnRunner)` |

## Bilinen davranışlar

- `ScheduleWakeup` (Claude Code harness aracı) devre dışı bırakıldı. Model bazen hâlâ bu aracı çağırmayı dener; disallowed listesi sayesinde araç çağrısı reddedilir, model `schedule_wake`'e yönelir.
- Sohbet oturumu `ListSchedules` ve `list_schedules` aracından filtrelenir — yalnız aktif (henüz tetiklenmemiş) wake satırları gözükür ve bunlar da UI'da gösterilmez.
- Çok uzun gecikmeler (1h+) clamp edilir; kullanıcıya araç çağrısı yanıtında belirtilir.

---

# Tekrarlayan Zamanlamalarda Son Tarih (`expires_at`)

> Bu bölüm **one-shot wake**'ten ayrıdır. `schedule_wake` tek seferlik bir
> timer'dır; burada anlatılan ise **tekrarlayan cron zamanlamalarına** eklenen
> opsiyonel bir bitiş tarihidir.

## Neden gerekti?

Kullanıcı bir cron zamanlamasını yalnızca belirli bir tarihe kadar çalıştırmak
isteyebilir (ör. "her sabah 09:00, ama yalnız bu ayın sonuna kadar"). Önceden
zamanlamalar süresizdi; kullanıcının elle pasifleştirmesi gerekiyordu.

## Davranış

- `Schedule.ExpiresAt` (unix saniye, `json:"expiresAt,omitempty"`) **opsiyonel**
  bir son tarihtir. `0` = son tarih yok (süresiz çalışır).
- Son tarih **geçtikten sonra**:
  - O ana denk gelen cron tick'i **atlanır** (ajana prompt teslim edilmez),
  - Zamanlama **otomatik pasifleşir** (`enabled=false`),
  - Bir `Reload` ile cron tablosundan tamamen düşürülür.
- Scheduler **yeniden kurulurken** (boot / `Reload`) süresi çoktan geçmiş bir
  zamanlama hiç eklenmez ve aynı şekilde pasifleştirilir — restart sonrası
  süresi dolmuş bir zamanlama asla bir kez daha tetiklenmez.
- **Manuel "▶ Çalıştır"** son tarihten etkilenmez: kullanıcı isterse süresi
  dolmuş bir zamanlamayı elle bir kez daha koşturabilir (`RunNow` → trigger
  `manual`). Son tarih yalnızca otomatik cron tetiklemesini durdurur.

## Karar katmanı

`scheduleExpired(sc)` saf bir yardımcı: `sc.ExpiresAt > 0 && now >= sc.ExpiresAt`.
Hem `rebuildLocked` (kurulumda) hem `fire` (tick anında) bunu kullanır — tek
doğruluk kaynağı.

## API

- `POST /api/schedules` ve `PUT /api/schedules/:id` gövdesinde opsiyonel
  `expiresAt` (unix saniye) alanı kabul edilir; `0`/eksik = son tarih yok.
- `UpdateSchedule` `ExpiresAt`'i de günceller. **Not:** update `enabled`
  bayrağına dokunmaz — süresi dolup pasifleşmiş bir zamanlamanın son tarihini
  ileri çekip tekrar çalıştırmak için düzenle → kaydet → toggle ile etkinleştir.

## Frontend

`ScheduleModal.tsx` (oluşturma **ve** düzenleme aynı popup) içinde **"Son tarih
(ops.)"** `datetime-local` alanı (✕ ile temizlenebilir, gelecekte olma
doğrulaması). Pano kartı (`ScheduleCard`) son tarihi gösterir; geçmişse kırmızı
**"(süresi doldu)"** etiketi.

## Dosyalar

| Dosya | Değişiklik |
|---|---|
| `internal/db/models_task.go` | `Schedule.ExpiresAt int64` |
| `internal/db/store_schedule.go` | `UpdateSchedule` `ExpiresAt`'i persist eder |
| `internal/agent/scheduler.go` | `scheduleExpired()`; `rebuildLocked` + `fire` expiry guard'ı |
| `internal/api/schedules.go` | create/update `expiresAt` alanı |
| `frontend/src/types/task.ts` | `Schedule.expiresAt?` |
| `frontend/src/api/tasks.ts` | `createSchedule`/`updateSchedule` `expiresAt` |
| `frontend/src/features/schedules/ScheduleModal.tsx` | son tarih input (popup) |
| `frontend/src/features/schedules/ScheduleCard.tsx` | pano kartında son tarih gösterimi |

## Testler

```
internal/agent/scheduler_test.go
  TestScheduleExpired                  — 0 süresiz, gelecek canlı, geçmiş süresi dolmuş
  TestExpiredScheduleSkippedOnReload   — süresi dolmuş zamanlama Start'ta otomatik pasifleşir
```

## Düzeltme: bekleme banner'ı erken kayboluyordu (2026-06-19)

**Belirti:** `schedule_wake` çağrıldıktan sonra sohbet "bitti" gibi görünüyor; "X sn
sonra devam edecek" bekleme banner'ı görünmüyor, sonra zamanlanmış prompt düşüyordu.

**Kök neden (frontend):** Backend `armed` fazlı eventi doğru yayıyordu (banner'ı
kaldırır). Ama `schedule_wake` turu hemen ardından **başarıyla biterken**
`chat_stream.go` phase'siz bir `chat` eventi ("Yanıt hazır") yayıyor; `App.tsx`
event handler'ı bunu `else` dalında "tur bitti" sayıp `clearWakeWait` ile banner'ı
**anında siliyordu**. Yani banner bir an görünüp kayboluyordu.

**Çözüm (`App.tsx`):** Generic (phase'siz) tur-sonu eventi artık banner'ı silmiyor;
yalnız `start` (uyandı) ve `cancelled` (iptal) fazları siliyor. Banner, uyanış
gerçekleşene veya iptal edilene kadar kalıyor (geri sayım + İptal düğmesiyle). Hem
native hem claude-cli yolu için geçerli (`armed` her iki yolda da yayılıyor).

## Düzeltme: otomatik devam mesajı "kullanıcı balonu" gibi görünüyordu (2026-06-20)

**Belirti:** Wake süresi dolup ajan kendi kendine devam edince, uyanış prompt'u
sohbette **mor kullanıcı balonu** olarak çıkıyordu — sanki kullanıcı tekrar
soruyormuş gibi; bu da bekleme UI'ını "bozuyordu".

**Kök neden:** `deliverWake` (ve zamanlanmış teslim) prompt'u `Role:"user"` ile
kaydediyordu → UI onu UserBubble olarak çiziyordu.

**Çözüm (display-only, model bağlamı değişmez):**
- `db.Message`'a `Origin` alanı eklendi: `"wake"` (schedule_wake devamı) /
  `"schedule"` (zamanlanmış görev). Rol "user" kalır → modele giden geçmiş aynı.
- `scheduler.go` wake/schedule prompt'larını `Origin` ile işaretler.
- Frontend (`MessageList.tsx`): `role==='user' && origin` olan mesaj, mor balon
  yerine **ortalanmış "⏰ Otomatik devam / Zamanlanmış görev — <prompt>" notu**
  olarak çizilir (`Message.origin` tipi + `types/message.ts`).

**Not:** Backend değişikliği (Origin yazımı) için Go sunucusunun yeniden
başlatılması gerekir; frontend vite HMR ile anında güncellenir.

---

# Akış Tabanlı Zamanlamalar + Otomasyonlar (`flowId`) (2026-07-04)

## Neden

Bir zamanlama/otomasyon tetiklendiğinde tek bir ajana prompt teslim etmek yerine,
seçilen bir **orkestrasyon akışını** (Flow) başlatabilmek istendi — böylece
çok-adımlı (agent/branch/parallel/transform) iş akışları cron ile veya etiket
tetikleyicisiyle otomatik koşabilir. Model olarak, Task'taki mevcut `FlowID`
("flow-backed task") deseni Schedule ve Automation'a taşındı.

## Model

- `db.Schedule.FlowID` + `db.Automation.FlowID` (`json:"flowId,omitempty"`).
  Set ise **akış tabanlı**: Prompt/PromptTemplate akışın **girdisi** olur;
  `AgentID`/`TargetAgentID` opsiyonel hale gelir. Ajan-hedef ile karşılıklı dışlar.
- Store `Update*` fonksiyonları `FlowID`'yi round-trip eder.

## Dispatch

- **Schedule** (`agent/scheduler.go` `run`): `sc.FlowID != ""` → `deliverFlow` →
  `RunFlowRecorded(FlowID, Prompt, autonomous)`. Akış kendi transcript oturumunu +
  bildirimini yönetir; bu yüzden akış yolunda `emitPromptDelivery` çağrılmaz.
  `FlowFailure` → schedule `lastDeliveryStatus=failure`. Cron tick + "şimdi çalıştır"
  ikisi de aynı yoldan geçer.
- **Automation** (`agent/automation.go` `fire`): render sonrası `a.FlowID != ""` →
  `fireFlow` → `RunFlowRecorded(FlowID, prompt)`. Spawn/agent-lookup atlanır.
  Akış oturumları tetik etiketi taşımadığından **kendini döngülemez** (per-tetik
  dispatch); guardrail'ler (MaxIterations/Cooldown/ExpiresAt) yine tetik sıklığını
  sınırlar, `SpawnTags` yok sayılır.

## API + Araçlar + UI

- **API:** `POST/PUT /api/schedules` + `POST/PUT /api/automations` `flowId` alır.
  Doğrulama gevşetildi: cron/triggerTag + promptTemplate zorunlu; hedef = **ya**
  `flowId` (akış varlığı doğrulanır) **ya** ajan (+ prompt). Automation update'te
  `flowId` set → agent temizlenir, `targetAgentId` set → flow temizlenir (kısmi
  spawnTags güncellemesi hedefi ellemez).
- **Araçlar:** `create/update_schedule` + `create/update_automation` `flowId`
  parametresi (+ `list_*` çıktısında `flowId`). Flow varlığı `GetFlow` ile doğrulanır.
- **UI:** `ScheduleModal.tsx` + `AutomationModal.tsx` — ortak `TargetModeToggle`
  (Ajan/Akış) + `FlowPicker` (`pickers.tsx`; workspace akışları `api.listFlows`).
  Akış modunda prompt "Akış girdisi (opsiyonel)"; pano kartında akış hedefi
  `Workflow` ikonu + `🔀 <akış adı>` ile gösterilir; akış otomasyonunda
  spawn-etiket editörü yerine bilgi notu.

## Test

- `internal/db/automation_test.go` — `TestAutomationFlowIDRoundTrip`,
  `TestScheduleFlowIDRoundTrip` (create/update round-trip + hedef değiştirme).
