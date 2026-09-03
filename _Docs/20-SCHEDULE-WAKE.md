# TionHarness — `schedule_wake`: Ajanın Kendi Sohbetine Geri Dönmesi

> **Özet (2026-09-03):** Tek-seferlik alt-süreç olarak çalışan `claude-cli` ajanının
> "5 dakika sonra devam edeceğim" deyip gerçekte hiç uyanmaması sorununu çözen
> `schedule_wake` aracı ve altyapısı — DB'ye one-shot `Schedule` satırı yazar,
> `time.AfterFunc` ile zamanlar, tetiklenince aynı sohbet oturumuna geçmiş-duyarlı
> tam bir tur olarak enjekte eder (at-most-once teslim garantili). Durum: uygulanmış
> ve olgun; bekleme banner'ı + iptal, async `ask_user` desteği, ve dokümanın devamında
> **ayrı** iki özellik daha var — tekrarlayan zamanlamalarda `expires_at` son tarihi
> ve zamanlama başına `sessionMode` (reuse/spawn). Dayandığı dosyalar:
> `internal/agent/scheduler.go`, `internal/agent/runtime.go` (`ScheduleWake`),
> `internal/tools/builtin_wake.go`, `internal/db/models_task.go` (`Schedule`).

## Neden gerekti?

TionHarness'in `claude-cli` sağlayıcısı `claude -p --output-format stream-json` ile çalışır — her tur bir **tek-seferlik alt süreç**; tamamlanınca ölür. Claude Code'un yerleşik `ScheduleWakeup` aracı yalnızca `claude /loop` harness bağlamında anlamlıdır; bu harness TionHarness'te yoktur. Sonuç: ajan "bekliyorum, 5 dakika sonra devam edeceğim" deyip `ScheduleWakeup` çağırıyordu, fakat hiçbir şey olmuyor, sohbet orada bitiyordu.

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
   fireWake
         ├─ DB: ConsumeOneShotSchedule (TESLİMDEN ÖNCE: enabled=false + lastRunAt)
         │     └─ zaten tüketilmişse teslim edilmeden çıkılır
         └─ deliverWake(sc)
               ├─ DB: user mesajı yaz (sc.Prompt)
               ├─ invokeTraced(sc.SessionID) → asistan yanıtı yaz
               ├─ emitWakeEvent(phase=start) / emitWakeEvent(phase=done)
               ├─ başarı → DB: one-shot satırını sil
               └─ hata  → satır kalır (devre dışı) + lastDeliveryStatus/Error yazılır
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
| `internal/api/mcp_interaction.go` | `schedule_wake`, `interactionToolSpecs`'e eklenir (extended tier); allowlist `inter.Core/ExtendedToolNames`'ten otomatik türer (tek kaynak). CLI'da `mcp__tionharness_extended__schedule_wake` |
| `internal/agent/climcp.go` | `"ScheduleWakeup"` (CLI native) → disallowed (TionHarness'in schedule_wake'i yerine geçer) |
| `internal/api/schedules.go` | Bekleyen one-shot satır liste filtresi + one-shot düzenleme reddi (400) |
| `internal/tools/builtin_schedulemgmt.go` | Aynı filtre + `update_schedule`'da one-shot reddi |
| `frontend/src/App.tsx` | `chat` event: `phase=start` → `markPending`, `phase=done` → `clearPending` |
| `internal/db/store_schedule.go` | `ConsumeOneShotSchedule` (at-most-once claim), `ErrNotOneShot` |
| `internal/agent/wake_test.go` | 5 birim testi |

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

**claude-cli yolu:** `mcp__tionharness_extended__schedule_wake` (NameOnly/extended tier → ToolSearch ile lazy gelir) → `mcp_interaction.go callWake` → `run.wakeScheduler()` → aynı `Runtime.ScheduleWake`. CLI'ın kendi `ScheduleWakeup` aracı `disallowed` listesinde (işe yaramaz, kafa karıştırır).

## Gecikmeler ve sınırlar

- Minimum: `MinWakeDelaySec = 5` saniye (1s altı istekler sıkıştırılır)
- Maksimum: `maxWakeDelay = 1 saat`
- Timer çakışması: `rebuildLocked` yeniden çağrıldığında eski timer iptal edilir
- One-shot tüketimi: **at-most-once**. `fireWake`, teslimi denemeden **önce**
  `db.ConsumeOneShotSchedule` ile satırı atomik olarak sahiplenir
  (`enabled=false` + `lastRunAt=now`). Satır o anda etkin listeden düştüğü için
  araya giren bir `Reload` ikinci timer kuramaz, teslim sırasında süreç ölse bile
  yeniden başlatmada tekrar teslim edilmez. Yarışı kaybeden çağrı
  (`claimed=false`) teslimi tümüyle atlar.
- Teslim sonucu: başarılıysa satır silinir; başarısızsa **silinmez** — devre dışı
  hâlde kalır ve hata `lastDeliveryStatus="failure"` + `lastDeliveryError`'a
  yazılır (hata yutulmaz, ayrıca Error seviyesinde loglanır). Tüketim
  bookkeeping'i teslim deadline'ından bağımsız bir `context.Background()` ile
  yazılır; teslim zaman aşımı işareti düşürmeyi engelleyemez.
- Bayat (stale) wake: `FireAt` üzerinden `staleWakeAge = 1 saat`'ten fazla geçmiş
  ya da `lastRunAt > 0` olan bir satır `rebuildLocked` içinde **teslim edilmeden**
  tüketilir (`lastDeliveryStatus="expired"`), yeniden kuyruğa girmez.
- Hata yönetimi: `invokeTraced` başarısız olursa, hata metni asistan mesajı olarak sohbete yazılır (sohbet askıda kalmaz)

## Tükenmiş wake satırının UI görünürlüğü (2026-08-28)

Teslimi denenmiş ama başarısız olmuş (`failure`) veya bayatlamış (`expired`) bir
wake satırı diskte kalır. Eskiden liste filtreleri **tüm** one-shot satırları
attığı için bu artık kayıt UI'da hiç görünmüyordu; silme yalnız düzenleme
modalinden yapılabildiği ve modal ancak listedeki karttan açıldığı için kayıt
silinemez hâle geliyordu.

Kural artık "one-shot ise gizle" değil, **"teslimi hiç denenmemiş one-shot ise
gizle"**:

- `internal/api/schedules.go` `handleListSchedules` ve
  `internal/tools/builtin_schedulemgmt.go` `list_schedules`:
  `sc.OneShot && sc.LastDeliveryStatus == ""` olan satır atlanır. Bekleyen wake
  (henüz ateşlenmemiş) gizli kalır; `lastDeliveryStatus` dolu satır listeye girer.
  Filtre `enabled`/`agentId` filtrelerinden **önce** uygulanır, yani sayfalama
  toplamlarını (`total`/`hasMore`) da doğru şekilde şekillendirir.
- Başarılı teslim satırı zaten sildiği için listede görünen her one-shot satır
  tanım gereği başarısız/bayat bir kalıntıdır.

**Düzenlenemez, yalnız silinebilir.** Wake satırının cron ifadesi yoktur ve fire
zamanı scheduler'a aittir; bir düzenleme yarı geçerli bir satır üretirdi. Bu
yüzden yazma yolları açık hata döndürür (sessiz yutma yok):

- `handleUpdateSchedule` → 400 `one-shot wake schedules cannot be edited (delete it instead)`
- `UpdateScheduleTool` (`update_schedule`) → benzer hata, `delete_schedule`'a yönlendirir
- `internal/api/market_publish.go` export'u `sc.OneShot` satırlarını atlar
  (boş `cronExpr`'li template kirliliği önlenir)

Silme yolu değişmedi: `DELETE /api/schedules/{id}` ve `delete_schedule` çalışmaya
devam eder.

**Frontend:**

- `ScheduleCard.tsx` — cron satırı yerine, `s.oneShot` true ise
  `Tek seferlik · <fireAt>` yazar (`fmtTime`, `timeUtils.ts`). Aksi hâlde boş cron
  yüzünden boş satır çıkıyordu.
- `ScheduleModal.tsx` — `editing.oneShot` durumunda form yerine salt-okunur özet
  (fire zamanı, prompt, `lastDeliveryError`) + "Tek seferlik uyandırma —
  düzenlenemez, silinebilir." notu gösterilir. Footer'daki `schedule-delete` silme
  düğmesi korunur; hiçbir `updateSchedule` isteği gönderilmez, dolayısıyla
  `cronExpr` için `*/5 * * * *` varsayılanı bu satıra sızmaz.
- `frontend/src/types/task.ts` `Schedule` tipine `oneShot?: boolean` ve
  `fireAt?: number` alanları eklendi (backend JSON adlarıyla).

Testler: `internal/api/schedules_oneshot_test.go`
(`TestListSchedulesOneShotVisibility`, `TestUpdateScheduleRejectsOneShot`).

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
  TestFireWake_DeliversAtMostOnce       — iki tick = tek teslim; başarısız teslim de tüketilir,
                                          satır devre dışı + lastDeliveryError dolu, reload re-arm etmez
  TestScheduler_RetiresStaleWake        — vadesi çok geçmiş wake teslim edilmeden "expired" tüketilir

internal/db/store_schedule_oneshot_test.go
  TestConsumeOneShotSchedule_AtMostOnce — ilk claim kazanır (enabled=false + lastRunAt),
                                          ikincisi hatasız ok=false; satır etkin listeden düşer;
                                          store yeniden açıldığında (restart) claim korunur
  TestConsumeOneShotSchedule_RejectsCron — cron satırı ErrNotOneShot, bilinmeyen id ErrNotFound
```

## Canlı test özeti (2026-06-18)

Playwright ile doğrulandı:
1. TionHarness yeni oturumunda `schedule_wake` 10s ile tetiklendi
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
| `frontend/src/features/chat/useChatStream.ts` | `wakeWaits` durumu, `setWakeWait`/`clearWakeWait`/`cancelWake`, `activeWakeWait` |
| `frontend/src/features/chat/WakeWaitBanner.tsx` | bekleme banner'ı + geri sayım + Durdur |
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
diye. `TIONHARNESS_MAX_TOOL_ITERS` env değişkeniyle (pozitif tamsayı) override edilebilir
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
- Bekleyen (henüz tetiklenmemiş) wake satırları `ListSchedules` ve `list_schedules` çıktısından filtrelenir, yani UI'da gösterilmez. Teslimi denenip başarısız olan satırlar listelenir — bkz. "Tükenmiş wake satırının UI görünürlüğü".
- Çok uzun gecikmeler (1h+) clamp edilir; kullanıcıya araç çağrısı yanıtında belirtilir.

## Stuck oturumlarda tekrarlayan zamanlama freni

Prompt tabanlı tekrarlayan zamanlama, ortak `schedule` oturumunun `StuckTurns`
sayacı ayarlardaki eşiğe ulaştığında yeni kullanıcı mesajını yazmadan durur.
Scheduler ilgili zamanlamayı otomatik pasifleştirir ve cron tablosunu yeniden
kurar. Böylece her cron tick'inde aynı guard hatası, bildirim ve transkript kaydı
üretilmez; guard reddi sayacı da artırmaz. Devam etmek için sorun giderilir,
oturumun `stuck` etiketi kaldırılarak sayaç sıfırlanır ve zamanlama kullanıcı
tarafından yeniden etkinleştirilir.

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

---

# Zamanlama Başına Oturum Modu (`sessionMode`)

## Neden

Ajan hedefli bir zamanlama, her ateşlemede ajanın tek ve uzun ömürlü
`"schedule"` oturumuna tur ekliyordu. Bu, "bir önceki turu hatırlasın" isteyen iş için
doğru; ama günde defalarca koşan bağımsız bir görev için transkripti sonsuza
kadar büyüten ve turları birbirine bulaştıran bir davranış. Karar artık
zamanlama başına verilir.

## Model

- `db.Schedule.SessionMode` (`json:"sessionMode,omitempty"`,
  `internal/db/models_task.go`).
- Kabul edilen değerler — `db.ScheduleSessionModeReuse` = `"reuse"`,
  `db.ScheduleSessionModeSpawn` = `"spawn"`. Boş string de geçerlidir
  (yazılmamış eski satırlar).
- `db.ValidScheduleSessionMode(mode)` yalnız `""` / `"reuse"` / `"spawn"`
  değerlerine `true` döner.
- `Schedule.EffectiveSessionMode()` **varsayılanı** çözer: boş değer `"reuse"`
  olur. Yani hiçbir mevcut zamanlamanın davranışı değişmez.

## Davranış

- **`reuse` (varsayılan):** `Scheduler.deliverPrompt` eski yoldan gider —
  `GetOrCreateKindSession(agentID, "schedule", "⏰ Schedule")` ile ajanın
  paylaşılan cron thread'ini açar, stuck-guard kontrolünü uygular, per-session
  turn slot'unu (`turnqueue`) alır ve prompt'u `origin="schedule"` kullanıcı turu
  olarak yazar.
- **`spawn`:** `deliverSpawnedPrompt` çağrılır; her ateşleme
  `LaunchRun(RunSpec{Trigger: TriggerSchedule, Autonomous: true, Spawn: …})` ile
  **kendi taze oturumunu** açar. Paylaşılan thread'e ait defter tutma (stuck
  gate, turn slot, ortak transkript) bu yolda hiç uygulanmaz. Oturum başlığı
  `scheduleSpawnTitle`: zamanlamanın adı varsa `⏰ <ad>`, yoksa prompt'tan
  türetilen `⏰ <kısa başlık>`. `Kind = "schedule-run"` (ne varsayılan `"spawned"`
  ne de paylaşılan thread'in `"schedule"`'ı): kenar çubuğunda yine "Otomasyon"
  cipiyle gruplanır (`kindChipKey`), ama `getOrCreateKindSession` yalnız
  `(agentID, Kind)` ile eşleştiği için `"schedule"` kullanmak bu tek seferlik
  oturumu paylaşılan cron thread'iyle çakıştırırdı — bkz.
  `internal/agent/scheduler.go` `deliverSpawnedPrompt`.
- **Akış tabanlı zamanlamada yok sayılır.** `FlowID` set ise ateşleme
  `deliverFlow` yolundan gider ve akış zaten kendi koşu-başına transkriptine
  kaydeder.

## API + UI

- `POST /api/schedules` ve `PUT /api/schedules/{id}` `sessionMode` alanını alır
  (`internal/api/schedules.go`). Bilinmeyen değer **saklanmaz**: her iki yol da
  `400` + `invalid sessionMode (want reuse or spawn)` döner — aksi hâlde
  `EffectiveSessionMode` bilinmeyen değeri sessizce `reuse` gibi çalıştırırdı.
- `ScheduleModal.tsx` — hedef "Ajan" iken görünen **Oturum** alanı
  (`data-testid="schedule-create-session-mode-select"`): "Aynı oturumu sürdür
  (varsayılan)" / "Her çalışmada yeni oturum aç". Akış hedefinde alan hiç
  render edilmez.

## Test

- `internal/agent/scheduler_sessionmode_test.go` —
  `TestScheduleEffectiveSessionMode` varsayılan tablosunu (`"" → reuse`)
  sabitler; spawn modunun ayrı oturum açtığı ateşleme yolu ile doğrulanır.
