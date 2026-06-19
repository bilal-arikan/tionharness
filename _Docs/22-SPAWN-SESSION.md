# Faz / Özellik 22 — Spawn Session (Fire-and-Forget Paralel İşçi)

> the external agent project/SwarmClaw'daki `spawn_session` benzeri: bir prompt'tan **yeni,
> bağımsız bir oturum** başlatıp **beklemeden** bırakmak. Çıktı, Faz U "Birleşik
> Yürütme/Aktivite" feed'inde canlı görünür.

## Amaç ve Konum

Zaten elimizde üç tetikleme yolu var:

| Yol | Doğası | Çekirdek |
|-----|--------|----------|
| Schedule (cron / wake) | zamana bağlı | `scheduler.deliverPrompt` |
| `call_agent` | **senkron**, aynı tur (delegasyon) | `delegate.go::withDelegation` |
| `send_agent_message` | async inbox mesajı (mevcut ajana kuyruk) | `delegate.go::SendAgentMessage` |

**Spawn bunlardan farklı:** elle (kullanıcı/ajan) **yeni bağımsız bir oturum**
açar, ajan turunu **arka planda** koşar ve `sessionID`'yi hemen döndürür —
çağıran beklemez. Bu, "swarm" (otonom işçi filosu) yeteneğinin temel taşıdır.

Kalıp aslında `scheduler.deliverPrompt` ile birebir aynı: session aç →
`AddMessage(user)` → ajan turu (`invokeTraced`) → `AddMessage(assistant, steps)`.
Spawn bunu **genelleştirir** ve fire-and-forget (goroutine) yapar.

## Tasarım — İki Katman

### 1. Çekirdek: `Runtime.SpawnSession`  (`internal/agent/spawn.go`)

Tek çalıştırma noktası. İmza:

```go
func (r *Runtime) SpawnSession(ctx context.Context, agentRef, prompt string, opts SpawnOptions) (SpawnResult, error)

type SpawnOptions struct {
    ModelOverride string // "" → ajanın kendi modeli
    Title         string // "" → prompt'tan üretilir
    CreatedBy     string // provenance: ajan id (otonom spawn) ya da "" (kullanıcı/API)
}
type SpawnResult struct { SessionID, AgentName string }
```

Akış:
1. `prompt` boşsa hata. `resolveAgent(agentRef)` (id veya isim).
2. `opts.ModelOverride` doluysa `agent.Model` üzerine bin (ajan-başına provider
   korunur — sadece model değişir).
3. **Eşzamanlılık guard'ı:** `acquireSpawnSlot()` — atomik sayaç, üst sınır
   `Tunables.SpawnMaxConcurrent` (vars. 16). Dolu ise hata döner (spawn-storm
   freni). Slot, arka plan goroutine bitince `releaseSpawnSlot()` ile bırakılır.
4. **Bağımsız oturum:** `db.CreateSession{Kind:"spawned", SourceID:uuid, …}` —
   her spawn taze bir `sourceID` ile **ayrı** bir session (GetOrCreate **değil**;
   dedup istemiyoruz). Başlık `✨ <kısa prompt>`.
5. `AddMessage(user, prompt)` — thread gerçek bir konuşma gibi okunsun.
6. **Fire-and-forget:** `go r.runSpawn(agent, sessionID, prompt)` ve hemen
   `SpawnResult` döner. Çağıranın `ctx`'i goroutine'i **iptal etmez**
   (`context.WithoutCancel` + kendi timeout'u, vars. 10 dk) — sayfa/HTTP isteği
   kapanınca spawn ölmesin.
7. `runSpawn`: `trackSession` (Aktivite feed'inde canlı "çalışıyor" rozeti) →
   `invokeTraced(WithCallKind(ctx, KindSpawn), agent, prompt, true)` (autonomous →
   günlük bütçe guardrail'i geçerli) → `AddMessage(assistant, output, steps)` →
   `untrackSession` → `releaseSpawnSlot` → tamamlanma event'i yayınla.

> **Neden streaming değil de `invokeTraced`?** Aktivite feed'i mesajları poll
> eder ve canlı "running" bayrağını `ActiveSessionIDs()`'ten alır; ara adımlar
> ancak asistan mesajı persist edilince görünür hale gelir (canlı SSE tüketicisi
> yok). Bu yüzden schedule/heartbeat ile **aynı** kanıtlanmış kalıbı kullanırız:
> `trackSession` + `invokeTraced`. Running rozeti goroutine boyunca yanar,
> transkript bitince dolar.

### 2. Yüzeyler

**a) HTTP — `POST /api/sessions/spawn`** (`internal/api/spawn.go`)
- Body: `{agentId, prompt, modelOverride?}` → `{sessionId, agentName}`.
- `registerSessionRoutes`'a eklenir. UI "Yeni oturum başlat" butonu bunu çağırır.

**b) Ajan aracı — `spawn_session`** (`internal/tools/builtin_spawn.go`)
- Built-in, **`SelfManageEnabled`** ile gated (self-manage suite içinde, lazy).
- Parametre: `agent` (hedef ajan id/isim), `prompt`, `modelOverride?`.
- **Per-tur guard:** tur başına en fazla `Tunables.SpawnMaxPerTurn` (vars. 4)
  spawn — tool örneği başına sayaç (registry her turda yeniden kurulur).
- Provenance: `CreatedBy = <çağıran ajan id>`.
- Otonom "swarm": bir ajan, alt görevleri paralel işçilere dağıtıp beklemeden
  devam edebilir (call_agent senkron beklerken bu etmez).

**c) UI**
- `ExecutionsPanel`: `spawned` kind metadata (✨ "Spawn") + filtre sekmesi;
  başlıkta **"+ Başlat"** butonu → `SpawnSessionModal` (ajan seçici + prompt +
  opsiyonel model). Başarıda yeni yürütme seçilir, feed yenilenir.
- `api.spawnSession(agentId, prompt, modelOverride?)`.

## Guard'lar (özet)

| Guard | Değer | Nerede |
|-------|-------|--------|
| Eşzamanlı spawned-session üst sınırı | `SpawnMaxConcurrent` (16) | `SpawnSession` atomik slot |
| Tur başına spawn sayısı | `SpawnMaxPerTurn` (4) | `spawn_session` tool örneği |
| Workspace içinde kal | — | `resolveAgent` zaten workspace-scoped; çapraz-ws yok |
| Otonomi bütçesi | günlük bütçe | `invokeTraced(autonomous=true)` → `guardedComplete` |
| Tool gating | `SelfManageEnabled` | `toolsetup.go` |

## Dosyalar

- `internal/agent/spawn.go` — `SpawnSession`/`runSpawn`/`SpawnOptions`/`SpawnResult`,
  slot sayacı, `KindSpawn`.
- `internal/api/spawn.go` — `handleSpawnSession` + route.
- `internal/tools/builtin_spawn.go` — `SpawnSessionTool` + `SpawnResult`.
- `internal/agent/tunables.go` — `spawnMaxConcurrent`/`spawnMaxPerTurn` + `SetSpawnLimits`.
- `internal/settings/settings.go` — `SpawnMaxConcurrent`/`SpawnMaxPerTurn` (DTO+Patch+defaults).
- `internal/api/server.go::applySettings` — `tun.SetSpawnLimits(...)`.
- `internal/agent/toolsetup.go` — self-manage bloğunda `spawn_session` kaydı.
- Frontend: `api/sessions.ts` (spawnSession), `components/panels/ExecutionsPanel.tsx`
  (kind + buton), `components/sessions/SpawnSessionModal.tsx` (yeni).

## Test

- `go build`/`go vet`/`go test ./internal/...` yeşil.
- Frontend `tsc`/`vite build` yeşil.
- Canlı (Playwright/Chrome veya API E2E): spawn → Aktivite feed'inde `spawned`
  yürütmesi belirir, "çalışıyor" → transkript (prompt + ajan yanıtı) dolar.
