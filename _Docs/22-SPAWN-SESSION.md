# Faz / Özellik 22 — Spawn Session (Fire-and-Forget Paralel İşçi)

> `spawn_session` deseni: bir prompt'tan **yeni,
> bağımsız bir oturum** başlatıp **beklemeden** bırakmak. Çıktı, Faz U "Birleşik
> Yürütme/Aktivite" feed'inde canlı görünür.

> **2026-06-19 güncel durumu:** `spawn_session` native ajan aracı olarak **kaldırıldı**.
> Yerine geçen: `run_subagent` (tek generic agent-to-agent primitifi; `wait:"async"`
> modu SpawnSession altyapısını kullanır). `SpawnSession` runtime metodu ve
> `POST /api/sessions/spawn` HTTP uç noktası **korunuyor** — (a) UI "Başlat" butonu,
> (b) claude-cli ajanlarının Interaction MCP köprüsü, (c) `run_subagent` async
> modunun iç motoru olarak aktif. Bu doküman orijinal tasarımı ve CLI köprü
> wiring'ini tarihsel kayıt olarak tutar.

## Amaç ve Konum (tarihsel tasarım bağlamı)

Tasarım sırasında elimizde üç tetikleme yolu vardı:

| Yol | Doğası | Çekirdek |
|-----|--------|----------|
| Schedule (cron / wake) | zamana bağlı | `scheduler.deliverPrompt` |
| `call_agent` | **senkron**, aynı tur (delegasyon) | `delegate.go::withDelegation` |
| `send_agent_message` | async inbox mesajı (mevcut ajana kuyruk) | `delegate.go::SendAgentMessage` |

> **Sonraki durum:** `call_agent` ve `send_agent_message` kaldırıldı; ikisi de
> `run_subagent` altında birleşti (sırasıyla `wait:sync` ve `wait:async` modu).

**Spawn bunlardan farklıydı:** elle (kullanıcı/ajan) **yeni bağımsız bir oturum**
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
    ModelOverride   string // "" → ajanın kendi modeli
    Title           string // "" → prompt'tan üretilir
    CreatedBy       string // provenance: ajan id (otonom spawn) ya da "" (kullanıcı/API)
    ParentSessionID string // context-reset soyağacı (handoff); "" → bağsız spawn
}
type SpawnResult struct {
    SessionID, AgentName string
    Queued bool
    QueuePosition int
}
```

> **Context reset motoru (2026-06-25):** `SpawnOptions.ParentSessionID` eklendi.
> Context reset / handoff (`_Docs/35`) SpawnSession'ı **temiz pencere** motoru
> olarak kullanır: handoff artifact yazıldıktan sonra `ParentSessionID = eski`
> ile taze bir oturum spawn edilir, yeni oturum handoff'u inline taşıyan bir
> continuation prompt'la açılır. Soyağacı `db.Session.ParentSessionID`'de tutulur.

Akış:
1. `prompt` boşsa hata. `resolveAgent(agentRef)` (id veya isim).
2. `opts.ModelOverride` doluysa `agent.Model` üzerine bin (ajan-başına provider
   korunur — sadece model değişir).
3. **Eşzamanlılık guard'ı:** `acquireSpawnSlot()` — atomik sayaç, üst sınır
   `Tunables.SpawnMaxConcurrent` (vars. 16). Doluysa iş, shallow/deep önceliğini
   koruyan in-memory kuyruğa girer ve `Queued=true`, `SessionID=""` döner. Kuyruk
   `SpawnQueueMax` (vars. 16) sınırına da ulaşırsa hata döner. Slot bırakıldığında
   tek tüketici kuyruğu otomatik ilerletir. Workspace shutdown sırasında bekleyen
   işler başlatılmaz; düşürülür ve koordinatör işi ise başarısızlık bildirimi yazılır.
4. **Bağımsız oturum:** `db.CreateSession{Kind:"spawned", SourceID:uuid, …}` —
   her spawn taze bir `sourceID` ile **ayrı** bir session (GetOrCreate **değil**;
   dedup istemiyoruz). Başlık `✨ <kısa prompt>`
   — koordinatör fan-out'uyla açılan worker oturumları (`Role == worker`) aynı
   yerde `🤖 <kısa prompt>` alır, böylece oturum listesinde düz spawn'dan ayrılır.
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
> yok). Bu yüzden schedule ile **aynı** kanıtlanmış kalıbı kullanırız:
> `trackSession` + `invokeTraced`. Running rozeti goroutine boyunca yanar,
> transkript bitince dolar.

### 2. Yüzeyler

**a) HTTP — `POST /api/sessions/spawn`** (`internal/api/spawn.go`)
- Body: `{agentId, prompt, modelOverride?}` →
  `{sessionId, agentName, queued, queuePosition?}`.
- `registerSessionRoutes`'a eklenir. UI "Yeni oturum başlat" butonu bunu çağırır.

**b) Ajan aracı — (kaldırıldı)**
- `spawn_session` built-in tool'u **native ajan yüzeyinden kaldırıldı** (2026-06-19).
- Yerine geçen: `run_subagent` ile `wait:"async"` — aynı `SpawnSession` altyapısını
  kullanır; daima kurulu (görünürlük araç-bazlı), profil/agent target destekler.
- claude-cli ajanları async arka plan çalışması için Interaction MCP köprüsünden
  `run_subagent` kullanabilir.

**c) UI**
- `ExecutionsPanel`: `spawned` kind metadata (✨ "Spawn") + filtre sekmesi;
  başlıkta **"+ Başlat"** butonu → `SpawnSessionModal` (ajan seçici + prompt +
  opsiyonel model). Başarıda yeni yürütme seçilir, feed yenilenir.
- `api.spawnSession(agentId, prompt, modelOverride?)`.

## Guard'lar (özet)

| Guard | Değer | Nerede |
|-------|-------|--------|
| Eşzamanlı spawned-session üst sınırı | `SpawnMaxConcurrent` (16) | `SpawnSession` atomik slot |
| Bekleyen spawn üst sınırı | `SpawnQueueMax` (16) | `spawnQueue` shallow/deep kuyrukları |
| Tur başına spawn sayısı | `SpawnMaxPerTurn` (4) | `spawn_session` tool örneği |
| Workspace içinde kal | — | `resolveAgent` zaten workspace-scoped; çapraz-ws yok |
| Otonomi bütçesi | günlük bütçe | `invokeTraced(autonomous=true)` → `guardedComplete` |
| Tool gating | `SelfManageEnabled` | `toolsetup.go` |

## Dosyalar

- `internal/agent/spawn.go` — `SpawnSession`/`runSpawn`/`SpawnOptions`/`SpawnResult`,
  slot sayacı, `KindSpawn`.
- `internal/agent/spawnqueue.go` — sınırlı in-memory shallow/deep kuyruk ve shutdown drop.
- `internal/api/spawn.go` — `handleSpawnSession` + route.
- `internal/tools/builtin_spawn.go` — `SpawnSessionTool` + `SpawnResult`.
- `internal/agent/tunables.go` — `spawnMaxConcurrent`/`spawnQueueMax`/`spawnMaxPerTurn` + `SetSpawnLimits`.
- `internal/settings/settings.go` — `SpawnMaxConcurrent`/`SpawnQueueMax`/`SpawnMaxPerTurn` (DTO+Patch+defaults).
- `internal/api/server.go::applySettings` — `tun.SetSpawnLimits(...)`.
- `internal/agent/toolsetup.go` — self-manage bloğunda `spawn_session` kaydı.
- Frontend: `api/sessions.ts` (spawnSession), `components/panels/ExecutionsPanel.tsx`
  (kind; "✨ Başlat" butonu 2026-06-19'da kaldırıldı), `components/sessions/SpawnSessionModal.tsx`
  (artık bağlı değil, öksüz).

## CLI köprüsü (2026-06-19) — claude-cli ajanları da spawn edebilir

`spawn_session` bir **native Go builtin**'di; sadece native-API provider'ları
(anthropic, minimax-anthropic, openai) Go tool-loop'unda görüyordu. claude-cli
ajanları (Coder, Fasty …) araçlara **Interaction MCP köprüsü** üzerinden ulaştığı
için `spawn_session`'ı bulamıyordu (Coder testinde "No such tool" → doğaçlama).

Çözüm — Interaction MCP köprüsüne eklendi (bkz. `11-INTERACTION-MCP.md §8`):

- `internal/api/mcp_interaction.go` — `interactionBackend.tun` ile gate; `Tools()`
  self-manage açıkken `spawn_session` ilan eder; `Call()` → `callSpawn`.
- `internal/api/chat_control.go` — `chatRun.spawn` (`*tools.SpawnSessionTool`) +
  `setSpawnTool`/`spawnTool`.
- `internal/api/chat_stream.go` — her ajan turunda taze spawn tool örneği kurulur
  (per-turn bütçe sıfırlanır), `wsp.Runtime.SpawnSession`'a bağlı.
- `internal/api/server.go` — köprüye `tun` geçilir.

Sonuç: hem native (Minimax3) hem claude-cli (Coder) ajanları "@Ajan, X'e şu konuda
session başlat" diyince çalışır — ikisi de canlı doğrulandı.

## Test

- `go build`/`go vet`/`go test ./internal/...` yeşil.
- Frontend `tsc`/`vite build` yeşil.
- Canlı (Playwright/Chrome veya API E2E): spawn → Aktivite feed'inde `spawned`
  yürütmesi belirir, "çalışıyor" → transkript (prompt + ajan yanıtı) dolar.
