# SwarmGo — `schedule_wake`: Ajanın Kendi Sohbetine Geri Dönmesi

## Neden gerekti?

SwarmGo'nun `claude-cli` sağlayıcısı `claude -p --output-format stream-json` ile çalışır — her tur bir **tek-seferlik alt süreç**; tamamlanınca ölür. Claude Code'un yerleşik `ScheduleWakeup` aracı yalnızca `claude /loop` harness bağlamında anlamlıdır; bu harness SwarmGo'da yoktur. Sonuç: ajan "bekliyorum, 5 dakika sonra devam edeceğim" deyip `ScheduleWakeup` çağırıyordu, fakat hiçbir şey olmuyor, sohbet orada bitiyordu.

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
| `internal/agent/climcp.go` | `"schedule_wake"` → `interactionToolNames`; `"ScheduleWakeup"` → disallowed |
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

**claude-cli yolu:** `mcp__swarmgo_interaction__schedule_wake` → `mcp_interaction.go callWake` → `run.wakeScheduler()` → aynı `Runtime.ScheduleWake`. CLI'ın kendi `ScheduleWakeup` aracı `disallowed` listesinde (işe yaramaz, kafa karıştırır).

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
1. SwarmGo yeni oturumunda `schedule_wake` 10s ile tetiklendi
2. Agent "kurdum, bekliyorum" yanıtı verdi
3. 10 saniye sonra wake prompt aynı sohbet oturumuna otomatik enjekte edildi
4. Agent yeniden yanıt verdi — sohbet ekranda görünür şekilde devam etti

## Bilinen davranışlar

- `ScheduleWakeup` (Claude Code harness aracı) devre dışı bırakıldı. Model bazen hâlâ bu aracı çağırmayı dener; disallowed listesi sayesinde araç çağrısı reddedilir, model `schedule_wake`'e yönelir.
- Sohbet oturumu `ListSchedules` ve `list_schedules` aracından filtrelenir — yalnız aktif (henüz tetiklenmemiş) wake satırları gözükür ve bunlar da UI'da gösterilmez.
- Çok uzun gecikmeler (1h+) clamp edilir; kullanıcıya araç çağrısı yanıtında belirtilir.
