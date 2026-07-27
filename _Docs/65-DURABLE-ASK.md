# 65 — Durable Ask (MVP): native `ask_user` kalıcı suspend/resume

> Session'ların insan-döngüsü sorularını, flow `await-input` durability'sine (`_Docs/62`)
> kavuşturur: native tool-loop'ta `ask_user` çağıran bir chat turu, goroutine'i
> **bloklamak yerine diske park edilir**; cevap endpoint'ten gelince tur kalıcı
> state'ten devam eder → restart/crash/deploy'a dayanıklı. Tüm davranış
> `WithDurableAsk` ctx flag'iyle **gate'li**; yalnız interaktif chat turu bunu set eder,
> diğer her yol (otonom/flow/claude-cli) bugünkü ephemeral davranışı korur.

## Durum

- **Faz 1 — DB katmanı ✅** (`internal/db`, canlı+test): `SessionAsk` entity'si
  (`models_session_ask.go`) + store (`store_session_ask.go`): `CreateSessionAsk` /
  `GetSessionAsk` / `ListWaitingSessionAsks` / **`ClaimSessionAsk`** (waiting→resolved
  CAS, tek-kazanan) / `CloseSessionAsk` (cancelled/timeout, idempotent) /
  `DeleteSessionAsk`. Dosya-store'a `session-asks/` dizini + `SAK` id prefix'i + boot-load.
  Test: `store_session_ask_test.go` (CAS çift-cevap, waiting-list terminal hariç, reopen persist).
- **Faz 2 — suspend/resume çekirdeği ✅** (`internal/agent`, canlı+test): `ask_suspend.go`
  — `WithDurableAsk`/`durableAskEnabled` gate, `askSuspend` sentinel, `askSuspendState`
  (replay snapshot: Model/System/SystemDynamic/OutputSchema/Messages/Steps/CallID),
  `persistAskSuspend`, `ResumeAsk` (+ provider-enjekte `driveResumedAsk`). `toolloop.go`:
  **temiz suspend noktası** tespiti — `durableAskEnabled && call.Name=="ask_user" &&
  len(ToolCalls)==1 && subFutures==nil && ContainerID==""` → `*askSuspend` döner (bloklamaz).
  Temiz olmayan noktada / gate kapalıyken eski bloklama yolu birebir korunur. Resume: cevap
  bekleyen `ask_user`'ın `tool_result`'ı olarak folded, native loop kaldığı yerden re-drive;
  pre-suspend trace prepend'lenir. Test: `ask_suspend_test.go` (temiz-nokta suspend,
  gate-kapalı-suspend-etmez, persist→claim→resume round-trip). Backend 426 test yeşil (agent+db+api).

- **Faz 3 — API wiring ✅** (`internal/api`, canlı+test): interaktif chat turu runner'ı
  (`chat_stream.go`) `turnCtx`'e **`WithDurableAsk`** ekler; `CompleteWithToolsStream` sonrası,
  hata dalından **önce**, `Runtime.SuspendAskFromError` ile `*askSuspend` yakalanır → suspend
  snapshot (lead+loop trace) persist + `ClearInflight` (artık orphan değil, durable) +
  `openDurableAskCard` (hub `interaction_open`, `id=askID`, `durable:true`) + turu **temiz döndür**
  (`sse ask_suspended`). Yeni `durable_ask.go`: `answerDurableAsk` (mevcut
  `POST /sessions/{id}/interactions/{iid}/answer` `resolveInteraction` başarısızsa **durable
  route**: `ClaimSessionAsk` CAS → `interaction_resolved` yayını → arka planda
  `driveDurableAskResume`), `ResumeAskAndRecord` (fold→re-drive→asistan mesajı kaydı; tekrar
  sorarsa yeni kart), `restoreWaitingAsks` (session stream subscribe'ında bekleyen kartları
  **ephemeral** yeniden yayınla → restart'ta ring boş olsa da disk satırı kaynak). Test:
  `ask_suspend_test.go` (+re-suspend). Backend 427 test yeşil (agent+db+api).
- **Faz 4 — Frontend ✅ (tasarımca):** durable kart yükü mevcut ask kartıyla **aynı şekle** sahip
  (`question`/`options`/`questions` + `id`); frontend cevabı zaten kartın `id`'siyle (=askID) aynı
  `/interactions/{id}/answer` endpoint'ine POST ediyor ve backend durable route ediyor → **frontend
  değişikliği gerekmedi**. `durable:true` bayrağı ileride rozet için mevcut.
- **Faz 5 — Doküman ✅:** bu dosya; `62` "Sonraki" maddesi kapatıldı.

## Faz 2+ genişletme (2026-07-27, canlı+test)

- **Permission onayları da durable ✅.** Native `permGate`'in bloklayacağı bir write/exec çağrısı
  (mode `ask`, RiskRead değil, standing grant yok, prompter var) **temiz noktada** artık suspend olur
  (`askSuspend.Kind="permission"`, tam `Call` taşınır). Resume'da karar araca dönüşür
  (`resolveResumeResult`): **allow/always** → onaylanan çağrı `buildRegistry`+`reg.Call` ile
  **çalıştırılır** (gerçek tool_result + tool kartı), **deny** → canlı gate'in beslediği ret metni.
  Suspend seam: `toolloop.go` `permGate` öncesi `wouldPromptPermission` (permGate'in "ask" dalını
  aynalar). Kart yükü `permission` kind'ı ile açılır (`openDurableAskCard` `ask.Kind` kullanır); cevap
  aynı `/interactions/{id}/answer` → durable route. Test: `TestDurablePermission_SuspendAtCleanPoint`,
  `TestResolveResumeResult_PermissionDeny`.
  **MVP sınırı:** "always" durable yolda **allow-once** gibi davranır (detached resume session'ın
  bellek-içi grant'larından ayrık → standing rule persist edilmez); resume turundaki **sonraki**
  permission'lar prompter yokluğunda reddedilir (otonom-benzeri, model uyarlanır).
- **Plan onayı — kapsam DIŞI (sınır, tercih değil).** Plan modu (`EnterPlanMode`/`ExitPlanMode`)
  **yalnız claude-cli**'de var; native loop'ta plan aracı yok. claude-cli alt-süreci tool call'ı açık
  tutarak bloklar → durable askıya alınamaz (ask_user'ın CLI Tier-2 sınırıyla aynı). Native bir plan
  aracı eklenirse aynı seam'e girer.
- **Timeout sweeper ✅.** `Runtime.StartWaitingAskSweeper` (30s ticker, `manager.open`'da başlar) →
  `sweepWaitingAsksAt(now)` (testlenebilir): `TimeoutSec` aşan bekleyen kartları **CAS-close**
  (`CloseSessionAsk(timeout)`; canlı cevaba karşı tek-kazanan). `TimeoutSec` varsayılan **0 (süresiz)**
  → meşru uzun beklemeler ölmez; bir workspace ileride bound set edebilir. Test: `TestSweepWaitingAsks`
  (bound timeout kapanır, deadline öncesi + `TimeoutSec=0` hayatta kalır). Backend 433 test yeşil.

## Bilinçli sınırlar (MVP)
- **claude-cli** değişmez (CLI alt-süreci tool call'ı açık tutar → durable askıya alınamaz;
  Tier 2 ephemeral). **permission/plan** onayı Faz 2+ (şimdilik yalnız `ask_user`).
- **Temiz olmayan nokta** (paralel batch / canlı subagent / PTC container) → eski bloklama
  fallback'i. Aktive edilmiş lazy-tool seti resume'da katalogdan yeniden ship'lenir (persist edilmez).
- Crash-during-resume: cevap `ClaimSessionAsk`'te persist edilir; claim sonrası çökmede tur
  kaybolur ama cevap diskte kalır (kabul edilir; ileride re-drive kurtarma eklenebilir).
