# 11 — Interaction MCP: Tarihsel Bölümler (Arşiv)

> **Özet (2026-09-22):** `11-INTERACTION-MCP.md`'den 2026-09-22 doküman temizliğinde taşınan
> plan/tarihsel metinler: hiç yazılmayan tahmini `RunSession` arayüzü, özgün "dosya
> değişiklikleri (tahmini)" listesi ve 2026-07-28'de kaldırılan oturum hedefi araçları
> (`set_session_goal`/`complete_goal`). Güncel davranış için ana doküman esastır.

## §2 — Tahmini `RunSession` arayüzü (yazılmadı)

Ortak çekirdek soyutlama — **`RunSession`** (aktif sohbet turunu temsil eder):

```go
type RunSession interface {
    Emit(step TurnStep)                              // SSE'ye adım it (todo, artifact kartı, ...)
    Ask(ctx context.Context, askID, question string, options []string) (string, error) // AskPrompt aç + cevabı bekle
    Artifacts() ArtifactSink                         // artifact üret/güncelle
    // ileride: Confirm, Notify, Pick, ...
}
```

`chatRun` bu arayüzü uygular. Hem native built-in'ler (context ile), hem MCP
sunucusu (token → run lookup ile) aynı `RunSession` üzerinden iş görür → **davranış
birebir aynı**.

## §9 — Dosya değişiklikleri (tahmini, özgün plan)

## 9. Dosya değişiklikleri (tahmini)

**Yeni:**
- `internal/api/mcp_interaction.go` — MCP-over-HTTP server (initialize/tools/list/tools/call), `internal/mcp` mesaj tiplerini reuse eder, bearer→run doğrulama, tool dispatch → `RunSession`.
- `internal/interaction/session.go` — `RunSession` arayüzü; tool tanımları `tools.Registry`'den enumerate edilir (yeniden tanımlanmaz).
- `internal/api/mcp_interaction_test.go` — protokol (initialize/list/call) + ask/answer round-trip + bearer reddi + emit mutex testi.

**Değişen:**
- `internal/api/chat_control.go` — `chatRun`'a `mu`/`write`/`token`/`answers` + `emit`; `register` token üretir; `answer` aksiyonu `askId`'ye yönlenir.
- `internal/api/chat_stream.go` — `run.write = sse` kur; `OnEvent`/asker → `run.emit`; interaction endpoint+token'ı context'e koy.
- `internal/climcp/climcp.go` — interaction server entry'si **her zaman** eklenir + allowedTools; boş-dönüş davranışı kalkar.
- claude-cli çağrısı (`providers/claudecli.go` / `agent/toolloop.go`/`climcp`) — `--disallowedTools` + system-prompt notu.
- `internal/tools/ask.go` benzeri — `WithInteractionEndpoint` context köprüsü.
- `_Docs/09-CLAUDE-AGENT-SDK.md` + `SKILL.md` — yeni mimari notu.

**İleride (CLI başına, ayrı iş):**
- `internal/providers/codexcli.go` · `vibecli.go` — her biri kendi MCP config yazıcısı + döngü shell-out'u; Interaction MCP server'ı ortak kullanır.

---

## §19 — `set_session_goal` / `complete_goal` (2026-06-26, kaldırıldı 2026-07-28)

## 19. Faz 3 — `set_session_goal` / `complete_goal` (oturum hedefi) (2026-06-26 — TAMAMLANDI ✅)

> **⚠️ BU BÖLÜM TARİHSELDİR.** Oturum-hedefi mekanizması **2026-07-28'de tamamen
> kaldırıldı** (araçlar, `db.Session.Goal`/`GoalDone`, prompt enjeksiyonu, HTTP
> endpoint'i, UI kartı, `goal`/`goal-done` etiketleri). İleride Claude Code'un
> `/goal`'üne benzer — ayrı bir değerlendirici modelin tur sonunda durma koşulunu
> yargıladığı — bir döngü olarak yeniden ele alınacak. Aşağısı ne yapıldığının kaydıdır.

Ajan artık oturumun kalıcı **"north star" hedefini** kendisi koyabilir/tamamlayabilir —
**mevcut `db.Session.Goal`/`GoalDone` ile paylaşımlı** (ayrı bir ajan-goal açılmadı). Daha
önce yalnız kullanıcı UI'dan set ediyordu; ajan hedefi sistem prompt'unda (`goalContextBlock`)
**görüyor** ama yazamıyordu. Artık iki araçla yazabilir; aynı alan, tek north-star.

**Araç sözleşmesi:**
- `set_session_goal(goal*)` → `Goal=goal, GoalDone=false`. Mevcut hedefi **ezer** ama yanıt
  "replaced the previous goal: …" diye **şeffaf** bildirir (paylaşımlı alan, kullanıcının
  hedefini sessizce ezmemek için). maxlen 2000.
- `complete_goal()` (argümansız) → mevcut metni koruyup `GoalDone=true`; hedef kalır ama
  context'e enjekte olmaz. Hedef yoksa/zaten done ise graceful mesaj (hata değil).
- Sink yoksa (oturumsuz tur) graceful no-op.

**Mekanizma:** `goalContextBlock` (`api/goal.go`) hedefi zaten her turun dinamik (cache-dışı)
ekine enjekte ediyor; done olunca düşüyor. Araçlar yalnız bu mevcut alana yazar → ajanın
koyduğu hedef **sonraki turdan** itibaren onu yönlendirir.

**Dosyalar:**
- `internal/tools/goalsink.go` — `GoalSink` (`Goal`/`SetGoal`) + `GoalState` + `WithGoal` köprüsü.
- `internal/tools/builtin_goal.go` — `SetSessionGoalTool` + `CompleteGoalTool`.
  `builtin_goal_test.go` (7 test: yaz/replace-flag/boş-red/sink-yok/complete/no-goal/already-done).
- `internal/agent/goalsink.go` — concrete `Runtime.NewGoalSink(sessionID)` → `db.GetSession`/
  `db.SetSessionGoal` (`artifactsink.go` deseni; "boş hedef done olamaz" guard'ı mirror).
- `internal/agent/toolsetup.go` — native registry'ye iki eager built-in.
- `internal/api/chat_control.go` — `chatRun.goal` + `setGoal`/`goalSinkFor()`.
- `internal/api/chat_stream.go` + `autonomous_interaction.go` — `wsp.Runtime.NewGoalSink` / `rt.NewGoalSink` ile bağlanır.
- `internal/api/mcp_interaction.go` — spec ×2 + `Call` case + `callGoal` (set/complete dispatch).
- Skill: `tionharness-progress` (north-star satırı genişletildi) + `tionharness-guide` (interaction bölümü).

**Yetki nüansı:** Hedef kullanıcıyla **paylaşımlı**. `set_session_goal` üzerine yazabilir ama
değişikliği yanıtta açıkça bildirir (provenance ayrımı veri modelinde yok; şeffaflık yeterli
görüldü). İleride sıkı guard istenirse `Session`'a "goal owner" alanı eklenebilir.

**UI notu:** Goal kartı `SessionDetailPanel` yan panelinde; ajan set edince **canlı refresh
event'i bu fazda eklenmedi** (panel yeniden açılınca/oturum değişince tazelenir). Kalıcılık +
context enjeksiyonu anında çalışır. Canlı refresh istenirse ayrı küçük iş (SSE + panel handler).

**Test:** `go build`/`vet`/`go test` (tools/agent/api) yeşil; 7 yeni birim test.

> **Faz 3 kalan:** `set_session_title` / cwd / archive_session — ayrı iş (§8 tablosu son satır).
