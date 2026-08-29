# 36 — Kalıcı Todo / PROGRESS Dosyası (Structured Note-Taking)

> **Durum (2026-06-25): UYGULANDI.** Anthropic'in *"Effective harnesses for
> long-running agents"* + *"Effective context engineering for AI agents"*
> makalelerindeki **kalıcı not dosyası** (`claude-progress.txt` + `feature_list.json`
> / `NOTES.md`) konvansiyonunun TionHarness karşılığı.
>
> İlişkili: [`arsiv/31-MEMGPT-CORE-MEMORY.md`](arsiv/31-MEMGPT-CORE-MEMORY.md) (core memory — KALDIRILDI),
> [`17-TOKEN-OPTIMIZASYON.md`](17-TOKEN-OPTIMIZASYON.md) (compaction),
> [`24-SELF-MANAGEMENT.md`](24-SELF-MANAGEMENT.md) (todo aracı).

## Sorun

`todo_write` aracı (`internal/tools/builtin_todo.go`) **stateless** idi: her çağrı
tüm listeyi taşır, sunucuda kalıcı durum tutmaz. Liste yalnızca dolaylı kalıcıydı —
çağrı bir `StepTodo` trace adımı olarak mesajın `Steps` alanına (`session.jsonl`)
yazılır, `todoContextBlock` (`internal/api/todos.go`) en son listeyi bu trace'ten
yeniden inşa edip her turda `SystemDynamic`'e enjekte eder (compaction'a dayanıklı).

**Boşluk:** Bu mekanizma **tek oturuma** bağlıydı. Oturum yeniden başlatıldığında /
yeni oturum açıldığında / ajan değiştiğinde liste görünmez olurdu. Anthropic modeli
ise tersi: ajan her oturum başında **diskteki** ilerleme dosyasını okuyup devralır
(*"read the progress files to get up to speed on what was recently worked on"*).

Core memory (MemGPT blokları) bu boşluğu doldurmaz: o serbest yapılı persona/human
belleğidir, yapılandırılmış görev durumu değil. Session `Goal` da oturum-başına tek
hedeftir, adım listesi değil.

## Tasarım

`todo_write` listesi **oturuma özel** bir progress dosyasına yazılır ve fresh
oturum açılışında geri yüklenir. (2026-07-11'e kadar çalışma-dizini paylaşımlıydı;
artık her oturum kendi listesini tutar — aynı projedeki iki oturum birbirinin
ilerlemesini ezmez.)

**Kompakt `set` formu (2026-07-10):** `todo_write` artık iki giriş şekli alır —
`todos` (tam liste replace, eskisi gibi) veya `set` (`{"set":{"1":"completed"}}`,
1-tabanlı indeks → status). `set` yolunda sunucu önceki listeyi `TodoSink.LoadTodos`
ile progress dosyasından yükler, durumları birleştirir, geri persist eder; birleşik
tam liste tool sonucunda JSON döner ki trace katmanı (`todoStepItems`) StepTodo
kartını input yerine output'tan kurabilsin. Sink yoksa/liste yoksa açık hata döner.
Dinamik "Active todo list" bloğu numaralıdır ve `set` formunu öğretir. Amaç: durum
güncellemelerinde değişmeyen içerikleri yeniden göndermemek (~400 → ~40 char).

- **Konum (2026-07-11'den beri OTURUMA ÖZEL):** `<store>/progress/<sessionID>/…`
  — **her zaman** oturum-başına, çalışma dizininden bağımsız. Session'ın working
  dir'i olsa bile progress oraya YAZILMAZ (eski `<cwd>/.tionharness/progress.json`
  dizin-paylaşımlı davranışı kaldırıldı: aynı projedeki farklı oturumlar
  birbirinin checklist'ini eziyordu; kullanıcı isteğiyle oturuma özel yapıldı).
  Tek resolver: `Runtime.ProgressDir(sessionID)` (yaz=NewTodoSink + oku=resume
  bloğu + detay-panel hep onu kullanır → hep tutarlı). Not: eski cwd'lerde kalan
  legacy `progress.json`'lar artık okunmaz (yörüngesiz kalır, elle silinebilir).
- **Format:** `internal/progress` paketi, `Record{Version, UpdatedAt, SessionID,
  AgentID, Todos[], Log[]}`. `Todos[].Status` = `pending|in_progress|completed`;
  `completed` ≡ Anthropic `feature_list` `passes:true`. `Log[]` = rolling ilerleme
  günlüğü (`claude-progress.txt` karşılığı), son 50 girdiye budanır. Atomik yazım
  (`*.tmp`→`rename`).

```mermaid
graph TD
    AGENT["Ajan: todo_write"] --> TOOL["TodoWriteTool.Call"]
    TOOL -->|"ctx'te sink varsa"| SINK["TodoSink.SaveTodos"]
    SINK --> DISK["progress.Save<br/>&lt;cwd&gt;/.tionharness/progress.json"]
    TOOL --> STEP["StepTodo trace<br/>(session.jsonl)"]
    FRESH["Yeni/restart oturum"] --> CB["todoContextBlock"]
    CB -->|"oturum trace'i boş"| LOAD["progress.Load"]
    LOAD --> DISK
    LOAD --> INJECT["SystemDynamic:<br/>'Resumed progress' bloğu"]
    CB -->|"trace dolu"| OWN["mevcut davranış"]
    style DISK fill:#69d,stroke:#036
    style SINK fill:#2d6,stroke:#093
```

**Akış:** (1) Fresh oturum, ilk tur: oturumun todo trace'i yok → `progress.json`
diskten yüklenir → "## Resumed progress (from a previous session)" bloğu enjekte
edilir. (2) Ajan `todo_write` çağırır → adım oturuma yazılır **ve** sink dosyayı
günceller. (3) Sonraki turlar: oturum trace'i dolu → mevcut davranış, disk senkron
kalır. (4) Restart → (1)'e döner.

## Sink mekanizması (ArtifactSink ikizi)

Persist, context-tabanlı bir sink ile yapılır (artifact sink deseninin birebir
kopyası), böylece `todo_write` aracı bağımlılık-hafif kalır:

- `internal/tools/todosink.go` — `TodoSink` arayüzü + `WithTodoSink`/`HasTodoSink`
  (ctx). `builtin_todo.go::Call` ctx'te sink varsa `SaveTodos` çağırır (best-effort;
  hata todo dönüşünü bozmaz).
- `internal/agent/todosink.go` — `Runtime.NewTodoSink(sessionID, agentID, cwd)` →
  cwd çözer (boşsa store fallback), `progress.Load`+merge+`progress.Save`, `progress`
  event yayınlar.
- **Native yol:** `toolloop.go` artifact fallback'inin yanında todo sink fallback'i
  kurar (`SessionIDFrom(ctx)!="" && ProgressPersist && !HasTodoSink`); `workDir`
  zaten orada çözülü.
- **Chat (stream) yolu:** `chat_stream.go` her tur sink'i hem ctx'e hem run'a
  (`run.setTodoSink`) bağlar — native ctx üzerinden, CLI run üzerinden.
- **CLI yol:** `mcp_interaction.go::callTodo` run'ın todo sink'ini ctx'e takıp
  kanonik aracı çağırır; sink `chat_stream.go` (chat) + `autonomous_interaction.go`
  (scheduler/spawn/flow) tarafından run'a kurulur.
- **Native araç gölgeleme (2026-06-25, fix):** claude-cli kendi built-in checklist
  aracını sunar; eski sürümlerde `TodoWrite`, yenilerde **`TaskCreate`/`TaskUpdate`/
  `TaskList`/`TaskGet`** ailesi. Bu native araç TionHarness'in bridged `todo_write`'ını
  **gölgeler** → model native'i çağırır, sink'e hiçbir şey gitmez, progress kartı boş
  kalır. `climcp.go::writeCLIMCPConfig` artık `--disallowedTools` ile her iki ad
  ailesini de bastırır (CLI'da olmayan adı disallow etmek zararsız) ve
  `claudecli.go::interactionSystemNote` modele yalnız `todo_write` kullanmasını açıkça
  söyler. E2E doğrulandı: "checklist ile takip et" gibi araç-adı içermeyen doğal
  istemde bile artık `todo_write` çağrılıp progress.json yazılıyor.

## Geri yükleme

`internal/api/todos.go::todoContextBlock(ctx, db, sessionID, cwd, agentID, resume)`:
oturumun kendi todo trace'i varsa onu render eder (mevcut davranış); yoksa ve
`resume` açıksa `progress.Load(progressDir(...))` ile diskten yükleyip
`renderResumedBlock` ile "Resumed progress" başlıklı bloğu döner (tamamlanmamış
liste; hepsi tamamsa boş). `chat_turn.go::composeTurnRequest` cwd + agentID +
`s.tun.ProgressResume()` geçirir. `progressDir` çözümü `NewTodoSink` ile **aynı**.

## Diğer Kalıcılık Katmanlarıyla İlişki

| Katman | Ne tutar | Yapı | Kapsam | Kaynak |
|---|---|---|---|---|
| ~~Core memory~~ | ~~Ajan kim / kullanıcı kim~~ | — | — | **KALDIRILDI (2026-07-05)** — [arşiv](arsiv/31-MEMGPT-CORE-MEMORY.md) |
| ~~Session Goal~~ | ~~Tek kuzey-yıldızı~~ | — | — | **KALDIRILDI (2026-07-28)** |
| **Progress (bu doküman)** | Ne bitti / sırada ne var | Yapılı todo + log | **Oturum** | `<store>/progress/<sessionID>/` |

Kalan iki katman tamamlayıcıdır, çakışmaz; ikisi de `SystemDynamic`'e ayrı bloklar girer. Progress
recall'a girmez (disk dosyası, knowledge_source değil).

## Ayarlar

| Alan | Vars. | Açıklama |
|---|---|---|
| `progressPersist` | açık | `todo_write` listesini diske yaz (kapalı → eski efemeral davranış) |
| `progressResume` | açık | Fresh oturuma diskten "Resumed progress" bloğu enjekte et |

`settings.Settings`/`DTO`/`Patch` + `Tunables.SetProgress`/`ProgressPersist`/
`ProgressResume` + `applySettings` canlı push. UI: Ayarlar ▸ Bağlam ▸ "Kalıcı
ilerleme (progress)" (iki toggle). Migration yok; yeni dosya + yeni opsiyonel alanlar.

## `feature_list` zenginliği (opsiyonel alanlar)

`todo_write` öğeleri Anthropic `feature_list` paritesinde iki **opsiyonel** alan
taşır: `category` (gruplama etiketi, ör. `functional`/`tests`/`docs`) ve `steps`
(doğrulama alt-adımları, `[]string`). Boşken dosyadan tamamen düşer (`omitempty`).
Şema (`builtin_todo.go`), `tools.TodoSinkItem`, `progress.TodoItem` ve sink mapping
(`agent/todosink.go`) bu alanları uçtan uca taşır. `completed` statüsü `passes:true`.

## PROGRESS.md konvansiyon skill'i

Yeni default skill **`tionharness-progress`** (`internal/skills/defaults/tionharness-progress/
SKILL.md`, `access: shared`): ajana hem **otomatik** progress.json katmanını (her
`todo_write` diske yazılır, fresh oturum geri yükler) hem de **insan-okunur**
`PROGRESS.md` konvansiyonunu (mevcut kilitsiz `Read`/`Write`/`Edit` ile proje
kökünde tut: oturum başında oku → tek iş seç → bitince dated entry ekle) öğretir;
core memory/goal'dan ayrımı belirtir. `//go:embed` ile otomatik dahil; yeni
ajanların baseline skill setine girer.

## UI görüntüleyici (salt-okunur kart)

`GET /api/sessions/{id}/progress` (`api/progress.go::handleSessionProgress`) →
`{path, exists, record}` (sink ile **aynı** dizin çözümü). API + `SessionProgress`
tipi (`types/session.ts`) korunur.

**Bilgi panelinde görev listesi kaldırıldı (2026-07-24):** `SessionDetailPanel`
artık "Görev Listesi" kartını (`ProgressCard`) render etmiyor — kart + progress
fetch (`sessionProgress` + `executions` sinyaliyle tazeleme) paneiden çıkarıldı.
Kalıcı ilerleme yalnız composer üstüne iğnelenen `TodoPanel` üzerinden görünür;
`SessionProgressCard.tsx` **2026-07-27'de silindi** (ölü bileşendi). (`ProgressCard` eski
davranışı: katla/aç toggle + statü işaretçisi + son 3 log; dizin-scope uyarı notu
+ `executions` sinyaliyle self-heal — hepsi kaldırıldı.)

**Tamamlanan listeyi kullanıcı kapatır (2026-08-29):** En yeni `todo_write` listesi
tamamlandıktan ve sonraki kullanıcı mesajı geldikten sonra artık otomatik kaybolmaz.
Bitmemiş liste kapatılamaz; tüm maddeler `completed` olduğunda X görünür. Kullanıcının
kapatma tercihi `localStorage` içinde session ID + todo occurrence ID + görünür todo
alanlarından üretilen liste imzasıyla tutulur. Aynı occurrence reload sonrası gizli
kalır; aynı session'da aynı içerikle yeni bir `todo_write` occurrence'ı eski kapatma
kaydından etkilenmeden görünür. Yeni/farklı liste ile başka session da etkilenmez.
Kapatma geçmişi en yeni 100 kayıtla sınırlıdır; limit aşılınca en eski kayıtlar atılır.
Storage bozuk veya tarayıcı tarafından engelliyse panel
görünür kalıp çalışmayı sürdürür. Inline trace `TodoCard` bu tercihten bağımsızdır.

## Dosyalar

- **Yeni:** `internal/progress/progress.go` (+test), `internal/tools/todosink.go`,
  `internal/tools/builtin_todo_test.go`, `internal/agent/todosink.go` (+test),
  `internal/api/progress.go`, `internal/skills/defaults/tionharness-progress/SKILL.md`.
- **Değişen:** `internal/tools/builtin_todo.go` (sink kancası + category/steps),
  `internal/agent/toolloop.go` (native fallback), `internal/agent/tunables.go` (knob),
  `internal/agent/workdir_ctx.go` (`SessionWorkdir`), `internal/db/db.go` (`Root()`),
  `internal/api/{todos.go,chat_turn.go,chat_stream.go,chat_control.go,mcp_interaction.go,autonomous_interaction.go,server.go}`,
  `internal/settings/{settings.go,store.go}`, `frontend/src/types/{settings.ts,session.ts}`,
  `frontend/src/api/sessions.ts`, `frontend/src/features/settings/appPanels.tsx`,
  `frontend/src/features/sessions/SessionDetailPanel.tsx`.

## Geri-Uyumluluk & Felsefe

- Migration yok; tek binary korunur (sıfır yeni runtime bağımlılığı); offline (saf
  dosya I/O); C1 cache güvenli (tüm enjeksiyon `SystemDynamic`'e); opt-in
  (`progressPersist=false` → bugünkü davranış birebir).

## Doğrulama

```powershell
cd <repo>
go build ./...
go test ./internal/progress/... ./internal/agent/... ./internal/tools/... ./internal/api/... ./internal/settings/...
cd frontend; npm run build
```

## Sırada (olası devamlar)

İlk üç devam maddesi (feature_list zenginliği, PROGRESS.md skill'i, UI
görüntüleyici) **uygulandı** (yukarıdaki bölümlere bakın). Kalan fikirler:

- **Düzenlenebilir kart:** salt-okunur viewer'ı düzenlenebilir yap (madde durumu
  değiştir / sil) — şu an yalnız görüntüleme.
- **Log telemetrisi:** progress log girdilerinden "oturum başına tamamlanan madde"
  metriği (token-optimizasyon tasarruf kartı deseni).
- **Çoklu-ajan eşzamanlılık:** aynı cwd'de paralel ajanlar tek dosyayı paylaşır
  (Anthropic'in sıralı-oturum modeli); gerekirse ajan-başına dosya veya kilit.
