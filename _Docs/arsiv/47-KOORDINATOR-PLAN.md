# 47 — Koordinatör: tamamlanmış plan bölümleri

> Arşiv: `47-KOORDINATOR-COKLU-AJAN.md` dosyasından taşınan, tamamlanmış plan/tasarım metni. Güncel durum için asıl dokümana bak.

### 1.2 Eksik olan / riskli olan

1. **Async worker → koordinatör geri bildirimi yok.** Bugün `wait:async`
   ayrı bağımsız bir oturum açar; sonuç orada kalır, koordinatörün oturumuna
   dönmez. `FireTurnFinished` yalnızca **yeni** bir oturum spawn edebiliyor
   (automation), var olan koordinatör oturumuna besleme yapamıyor.
2. **Aynı oturumda eşzamanlı tur koruması YOK.** `activeSessions` (bugün
   `activeMu` altında oturum id → **kayıt listesi**; `trackSession` bir
   `*sessionRun` tutamacı döndürür, `run.release()` yalnız o tutamacı siler)
   yalnız UI "çalışıyor" göstergesi — **kilit değil**. 4 işçi aynı anda bitip
   koordinatöre `<task-notification>` yazıp tur
   tetiklerse: iç içe geçmiş mesajlar + çift tur = yarış. **Per-session tur
   kuyruğu şart.**
3. Koordinatör-farkında sistem promptu / işçi araç kısıtı yok.
4. Koordinatör/işçi ilişkisini modelleyen alanlar yok (`ParentSessionID` handoff
   için kullanılıyor, anlamı "devamı" — worker "tarafından-spawn-edildi" farklı).

---

## 4. Koordinatör Sistem Promptu / Skill

Yeni gömülü skill `tionharness-coordinator` (`internal/skills/defaults/`), Claude
Code'un `getCoordinatorSystemPrompt()`'undan uyarlanır (Türkçe doküman / İngilizce
prompt kuralına göre prompt İngilizce):

- Rolün = koordinatör; her mesajın kullanıcıya; worker bildirimleri iç sinyal,
  onlara teşekkür etme.
- Paralellik senin süper gücün: bağımsız worker'ları tek mesajda fan-out et.
- **Sentezi SEN yap** — "based on your findings" YASAK; dosya:satır içeren
  spesifik spec yaz.
- Continue-vs-spawn karar tablosu (bağlam örtüşmesi yüksek→continue,
  düşük→fresh).
- Fazlar: Araştırma(paralel)→Sentez(sen)→Uygulama(dosya-seti başına tek)→Doğrulama.
- Gerçek doğrulama: özelliği açıp test et, rubber-stamp etme.

Prompt yalnız `Session.Role=="coordinator"` iken enjekte edilir (workspace prompt
kompozisyonuna koşullu blok — `composeTurnRequest`).

İşçi profilleri: mevcut `explore`/`coder`/`reviewer` (subagent.go) yeniden
kullanılır; koordinatör bunları `spawn_worker(target=...)` ile hedefler ya da
gerçek workspace ajanı adı verir.

---

## 5. UI

- **Koordinasyon paneli** (yeni): koordinatör oturumu açıkken sağda worker
  kartları — ad, durum (⏳running / ✅done / ❌failed), süre, token, son özet;
  karta tık → worker transkripti (executions feed'e deep-link, zaten var).
- Composer'da **koordinasyon yöntemi rozeti** (M1–M4 seçimi; M2 için "Koordinatör"
  toggle → oturum `Role=coordinator` olur, prompt+araçlar açılır).
- SSE: mevcut `spawned`/`chat` event'lerine `worker`-tipli event (status geçişi)
  eklenir → panel canlı güncellenir. Backend `emitSpawnEvent` deseni kopyalanır.

---

## 6. Fazlama

| Faz    | Kapsam                                                                                                                                                                        | Dosyalar                                                                            |
| ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| **F0** | Bu tasarım dokümanı + koordinatör skill taslağı                                                                                                                               | `_Docs/47`, `skills/defaults/tionharness-coordinator`                               |
| **F1** | Çekirdek backend: session alanları + `NotifyCoordinator` + per-session tur kuyruğu + `CoordinationEngine.OnWorkerFinished` (workspace manager'a `SetTurnHook` zincirine ekle) | `db/models.go`, `agent/coordination.go`, `agent/runtime.go`, `workspace/manager.go` |
| **F2** | Araçlar: `spawn_worker`/`send_to_worker`/`stop_worker`/`list_workers` + worker araç kısıtı + guard'lar (`CoordinatorMaxWorkers`/`MaxTurns`)                                   | `tools/builtin_coordination.go`, `agent/subagent.go`, `agent/tunables.go`           |
| **F3** | Koordinatör sistem promptu (koşullu enjeksiyon) + `tionharness-coordinator` skill                                                                                             | `agent/prompts*`, `api/*compose*`, `skills/defaults/`                               |
| **F4** | UI: koordinasyon paneli + yöntem seçici + `worker` SSE event                                                                                                                  | `frontend/src/components/`, `agent/coordination.go` (emit)                          |
| **F5** | M3 ortak scratchpad + M1/M4 birleşik "yöntem" belgeleme + testler + doküman güncelleme                                                                                        | `_Docs/28`,`15`,`47`, `*_test.go`, `SKILL.md`                                       |

### 6.1 F1 için en kritik teknik detay

`FireTurnFinished` bugün **tek** hook'a gidiyor (`AutomationEngine.OnTurnFinished`,
`manager.go:245`). İki tüketici gerekiyor (automation + coordination). Çözüm:
`SetTurnHook`'u **çoklu-hook** yap (hook listesi) ya da manager'da tek bir
"dispatcher" hook kur; o hem `AutomationEngine.OnTurnFinished` hem
`CoordinationEngine.OnWorkerFinished` çağırsın. Basit ve geriye uyumlu:
`manager.go`'da bir kompozit fonksiyon.

---

## 7. Kabul Kriterleri (M2)

1. Koordinatör oturumunda `spawn_worker` ×3 (tek tur) → 3 worker paralel koşar,
   koordinatör turu **bloklanmadan** biter.
2. Worker'lar bitince koordinatör oturumuna `<task-notification>` düşer ve **tek**
   yeni koordinatör turu (hepsini gören) otomatik başlar.
3. İki worker aynı anda bitse bile koordinatörde **çift tur olmaz** (kuyruk).
4. `send_to_worker` biten worker'ı yüklü bağlamıyla devam ettirir; `stop_worker`
   çalışanı iptal eder.
5. Worker oturumu `spawn_worker` göremez (recursion engellendi).
6. `CoordinatorMaxTurns` aşılınca notify döngüsü durur + kullanıcı bilgilendirilir.
7. M1 (`run_subagent` sync) ve M3 (`send_message`/inbox) davranışları **değişmez**.

---

## 8. Açık Kararlar (ÇÖZÜLDÜ — kararlar §9'da)

1. **Kapsam:** F0–F5'in tamamı mı, yoksa önce yalnız F1+F2 (çalışan çekirdek M2,
   UI'sız) mı? (Öneri: F1+F2+F3'ü ilk PR, F4 UI ikinci PR.)
2. **`Role` alanı mı, yalnız `CoordinatorSessionID` mi?** (Öneri: ikisi de — Role
   prompt/araç koşulu, CoordinatorSessionID bağ.)
3. **Kuyruk uygulaması:** keyed-lock+flag (basit) vs per-session goroutine
   (temiz ama daha çok makine). (Öneri: keyed-lock+flag.)
4. **M3 scratchpad** bu turda mı yoksa sonraya mı? (Öneri: F5, opsiyonel.)

---
