# 46 — Etiketler + Etiket/Pano-Tetikleyicili Otomasyonlar

> Durum: **Uygulandı + canlı doğrulandı** (2026-07-02/03, WS2, sonnet/claude-cli).
> Pano (kart) tetikleyicisi eklendi 2026-07-06.

Üç özellik: (1) oturum/flow/schedule kayıtlarına **etiket** (tag), (2) bir olay
gerçekleşince hedef ajanı/akışı çalıştıran **otomasyonlar** — iki tetik türü:
**etiket** (etiketli oturum bir turu bitirince) ve **pano** (bir kanban kartı
değişince, §2.5), (3) tur olaylarına göre **otomatik etiketleme** (§3).

**Tetik türü (`Automation.TriggerKind`):** `""`/`"tag"` (varsayılan,
geriye dönük uyumlu) = etiket tetikleyicili; `"board"` = pano tetikleyicili (2026-07-06);
`"token"` = token-harcaması eşiği tetikleyicili (2026-08-03, §2.6); `"counter"` =
mesaj/tool sayacı aralığı tetikleyicili (2026-08-06, §2.7 — token'ın kararlı, cache'ten
etkilenmeyen alternatifi). Guardrail'ler
(MaxIterations/CooldownSec/ExpiresAt/Enabled), hedefleme (TargetAgentID **veya** FlowID)
ve iterasyon defteri **dört tür için ortaktır** (`guardsPass` paylaşılır).

## 1. Etiketler

`Session.Tags` / `Flow.Tags` / `Schedule.Tags` (hepsi `[]string`, omitempty;
`Task.Tags` zaten vardı). Serbest metin etiketler; hem kullanıcı (UI) hem ajan
(araçlar) düzenler. Session etiketleri ayrıca otomasyonları tetikler.

- **Store:** `SetSessionTags` / `SetFlowTags` / `SetScheduleTags`. Ortak
  `normalizeTags` (trim + boşları at + first-seen sıralı dedup, hepsi boşsa nil).
- **API:** `PUT /api/sessions/{id}/tags`, `PUT /api/flows/{id}/tags`,
  `PUT /api/schedules/{id}/tags` — body `{"tags":[...]}` (tam değiştirme). Session
  tag değişimi canlı UI refresh için `session` event'i yayınlar. `SessionInfo`
  yanıtına `tags` eklendi.
- **Araçlar:** `set_session_tags` (mevcut oturum, `SessionSink` üzerinden;
  `tags` tam-değiştir **veya** `add`/`remove` artımlı), `set_flow_tags`,
  `set_schedule_tags` (id ile, db üzerinden — tag'leme yıkıcı değil, provenance yok).
- **UI:** `common/TagEditor.tsx` — chip editörü (Enter/virgül ekler, × kaldırır,
  boş kutuda Backspace son etiketi siler). Bağlı: SessionDetailPanel (Hedef altında
  "Etiketler"), FlowsPanel (meta toolbar; `setFlowTags` ile anında kalıcı), Schedules
  (satır içi).

## 2. Otomasyonlar (`Automation`)

Ayrı entity (`db/models_automation.go`, id prefix `AUT`, dir `automations`). Cron
Schedule'dan **ayrı** tutuldu (Schedule zaten cron + one-shot wake ile yüklü) ama
UI'da aynı Schedules ekranında ayrı bölümde gösterilir.

### Alanlar
| Alan | Anlam |
|------|-------|
| `TriggerTag` | İzlenen session etiketi. Bu etiketi taşıyan oturum bir turu bitirince tetiklenir. |
| `TargetAgentID` | Spawn'lanan oturumu çalıştıracak ajan. `FlowID` set ise opsiyonel. |
| `FlowID` | Set ise **akış tabanlı** otomasyon: tetikte ajan oturumu spawn etmek yerine render edilen prompt, o orkestrasyon akışının **girdisi** olarak çalıştırılır (`RunFlowRecorded`). Ajan-hedef ile karşılıklı dışlar. Akış oturumları tetik etiketi taşımaz → **kendini döngülemez** (per-tetik dispatch); yine de guardrail'ler (MaxIterations/Cooldown/ExpiresAt) tetik sıklığını sınırlar, `SpawnTags` yok sayılır. |
| `PromptTemplate` | Yeni oturumun promptu (veya akış girdisi). Placeholder'lar aşağıda. |
| `SpawnTags` | Spawn'lanan oturuma uygulanan etiketler. `nil` → `[TriggerTag]` (döngü). `[]` → döngüyü kırar. |
| `Enabled` | Kill-switch. Aç→iterasyon sayacı sıfırlanır. |
| `MaxIterations` | Toplam tetik üst sınırı. **Pozitif olmalı ve en fazla 500 (`db.MaxIterationsHardCap`).** `0` (eski "sınırsız" değeri) ve negatifler create/update yollarında **reddedilir** — bkz. §5.1. Vars. 50. Aşılınca otomatik pasifle. |
| `CooldownSec` | İki tetik arası min. saniye. |
| `IterationCount` / `LastFiredAt` / `LastSessionID` / `LastError` | Çalışma-zamanı defteri (`RecordAutomationFire`). |

### PromptTemplate değişkenleri
`renderAutomationPrompt` + `AutomationEngine.turnVars` (`agent/automation.go`) şu
`{{...}}` placeholder'larını ikame eder (biten tur başına):

| Değişken | Değer |
|----------|-------|
| `{{result}}` | Biten oturumun son yanıtı (asistan metni) |
| `{{title}}` | Biten oturumun başlığı |
| `{{tag}}` | Tetikleyici etiket |
| `{{sessionId}}` | Biten oturumun ID'si |
| `{{iteration}}` | Bu ateşlemenin sıra no'su (1-tabanlı, `IterationCount+1`) |
| `{{maxIterations}}` | Üst sınır (`0` → `∞`) |
| `{{agent}}` / `{{agentName}}` | Sonucu üreten ajanın adı |
| `{{prevPrompt}}` | Biten oturumun son **user** mesajı (sonucu üreten girdi) |
| `{{automation}}` | Otomasyonun adı |
| `{{date}}` / `{{time}}` / `{{datetime}}` | `2006-01-02` / `15:04` / `2006-01-02 15:04` |

Şablonda hiç `{{result}}` yoksa, sonuç yine de "--- Önceki sonuç ---" başlığıyla
sona eklenir (döngü çıktıyı taşımadan kalmasın). Bilinmeyen `{{...}}` aynen kalır.
Yeni değişken eklemek = `turnVars` haritasına bir satır. **Canlı doğrulandı**
(2026-07-03, WS2): `{{iteration}}/{{agent}}/{{date}}/{{prevPrompt}}/{{result}}` doğru
render oldu (sayaç 10→11→12, AUT4 2/2'de otomatik durdu).

### Tetikleyici: tur bitişi
Karar: tetik = **etiketli oturumda bir tur `end_turn` ile bitince** (araçlar
kullanılıp son cevap verilince). Bu, `Runtime.FireTurnFinished(sessionID, agentID,
output)` ile sinyallenir — **detached goroutine**, turu asla bloklamaz/iptal etmez.

Çağrı yerleri (tüm tamamlanma yolları):
- `api/chat_stream.go` — her yanıt sonrası (`resp.Text`).
- `agent/spawn.go` `runSpawn` — spawn turu bitince (döngünün doğal adımı).
- `agent/scheduler.go` `deliverPrompt` (zamanlanmış) + `deliverWake` (uyandırma).
- `agent/insightsteps.go` `insightStepRecorder.finish` — içgörü taraması başarıyla bitince
  (2026-08-27). Tarama transkriptini kendi sürdüğü için sohbet/spawn yolundan geçmiyordu;
  eklenmeden önce tarama oturumlarında **hiçbir** etiket otomasyonu tetiklenmiyordu.

**İçgörü tarama oturumu `insight-scan` etiketini taşır.** `openInsightSession` oturumu
`Tags: []string{insightScanSessionTag}` ile açar (sabit: `agent/automation_defaults.go`).
Etiket + yukarıdaki `FireTurnFinished` çağrısı birlikte, "tarama bitti" olayını etiket
tetikleyicisine bağlanabilir hâle getirir.

### Motor (`agent/automation.go`)
`AutomationEngine.OnTurnFinished`:
1. Biten oturumu yükle; **etiketsizse hızlı dön** (etiketsiz sohbet trafiği neredeyse
   bedava).
2. Enabled otomasyonlardan `TriggerTag ∈ session.Tags` olanlar için `fire`:
   - **Cooldown:** `now - LastFiredAt < CooldownSec` → atla (logla).
   - **Maks. iterasyon:** `IterationCount >= MaxIterations` (>0) → **otomatik pasifle**
     + `automation` event, dur.
   - `renderAutomationPrompt` (placeholder ikamesi; `{{result}}` yoksa sonucu
     "--- Önceki sonuç ---" ile ekler).
   - **Akış tabanlı (`FlowID` set):** `fireFlow` → `RunFlowRecorded(FlowID, prompt,
     autonomous)`; akış transcript oturumuna tur olarak yazılır + kendi bildirimini
     yükseltir. Ajan lookup + spawn atlanır. `RecordAutomationFire(sessionID)`.
   - **Ajan tabanlı (varsayılan):** `SpawnSession(TargetAgentID, prompt, {Tags:
     spawnTags, ParentSessionID: biten, CreatedBy: "automation:"+id})`.
     **`SpawnOptions.Tags`** → spawn'lanan oturum **oluşturulurken** etiketlenir
     (arka plan turu tag konmadan bitse bile race yok).
   - `RecordAutomationFire` (sayaç++, spawned/flow session id, hata).

Döngü: A(#loop) biter → B(#loop) spawn → B biter → C spawn … MaxIterations'a kadar.
Sayaç tek otomasyon üzerinde birikir (tüm spawn'lar aynı etiketi → aynı kural).

### Gönderilen kural: `insight-apply-workspace-opt` (2026-08-27)

Tohumlanan tek **etiket** otomasyonu (`defaultAutomations`,
`internal/agent/automation_defaults.go`): içgörü taraması bitince `workspace-opt`
bulgularını uygulatır.

| Alan | Değer |
|------|-------|
| `TriggerKind` / `TriggerTag` | `tag` / `insight-scan` |
| Hedef | `insight-applier` **sistem ajanı** — seed anında `AgentSystemKey` ile çözülür (`FindAgentBySystemKey`), "ilk ajan" fallback'i kullanılmaz |
| `SessionMode` | `spawn` (her tarama kendi uygulama oturumunu alır) |
| `SpawnTags` | `["insight-applied"]` |
| `MaxIterations` / `CooldownSec` | 20 / 300 |
| `Enabled` | **`false` — opt-in** |

- **Döngü kırıcı `SpawnTags`:** `nil` bırakılsaydı spawn edilen oturum tetik etiketini
  (`insight-scan`) alır ve kural kendini yeniden ateşlerdi; boş dilim de bunu ifade edemez
  çünkü `normalizeTags` `[]` değerini `nil`'e indirger. Bu yüzden **farklı** bir etiket
  verilir — ayrıca uygulayıcı oturumları filtrelemeyi kolaylaştırır.
- **Neden kapalı geliyor:** diğer shipped otomasyonlarla aynı sözleşme — düz bir workspace'te
  sürpriz maliyet/mutasyon olmaz. Açmak: **Otomasyon** ekranı ▸ 🏷 Etiket otomasyonları şeridi ▸
  kartın aç-kapa düğmesi. Kullanıcı silerse `.seeded-automations.json` defteri sayesinde
  yeniden tohumlanmaz.
- `insight-applier`'ın izin sınırı (repo dosyalarına erişemez, yalnız workspace varlıklarını
  düzenler) `_Docs/74-SISTEM-AJANLARI.md`'de; zincirin tamamı `_Docs/60-RETROSPEKTIF-TARAMA.md`
  §9.2'de.

### API + Araçlar + UI
- **API:** `GET/POST /api/automations`, `PUT /api/automations/{id}`,
  `POST /api/automations/{id}/toggle`, `POST /api/automations/{id}/reset`,
  `DELETE /api/automations/{id}`. Create'te hedef ajan doğrulanır; vars. maks=50.
- **Araçlar:** `create/update/delete/list_automation` (self-management suite,
  provenance: ajan yalnız kendi oluşturduğunu düzenler/siler).
- **UI:** `panels/Automations.tsx` — **Otomasyon** ekranında (NavRail'de eski
  "Zamanlamalar" → **"Otomasyon"**, `NavRail.tsx` + `App.tsx`; ekran hâlâ cron
  Zamanlamalar + Otomasyonlar bölümlerini birlikte tutar) "Otomasyonlar" bölümü:
  oluşturma formu (ad/tetik/hedef/maks-iter/bekleme/**son tarih (ops.)**/prompt) +
  liste (aç-kapa, iterasyon sayacı `n/max`, spawn-etiket editörü, **inline düzenle**
  (kalem → tüm alanlar + prompt), limit dolunca "sıfırla", sil). **Son tarih
  (`ExpiresAt`, opsiyonel unix sn):** geçtikten sonra otomasyon bir sonraki tetik
  denemesinde otomatik pasifleşir (`fire` başında `time.Now >= ExpiresAt` kontrolü,
  Schedule `expiresAt` deseninin eşi); create+update API/tool + UI datetime-local.
  Prompt şablonu alanında **ℹ️ info butonu** →
  13 değişkeni açıklamalı listeleyen popover (satıra tıkla → şablona ekle;
  click-away ile kapanır; `PROMPT_VARS` sabiti `turnVars` ile senkron).
  Etiket-tetikleyici event'leri `automation` tipiyle
  yayınlanır (deep-link: başarı→executions, limit/hata→schedules).

## 2.5 Pano (Kart) Tetikleyicili Otomasyonlar (2026-07-06)

Aynı `Automation` entity'si, `TriggerKind="board"` ile bir **kanban kart değişiminde**
tetiklenir. Böylece "bir kart _İnceleme_'ye taşınınca kod-gözden-geçir ajanını çalıştır"
gibi akışlar kurulur. Etiket türüyle **aynı** hedefleme (ajan **veya** flow) ve guardrail'leri
paylaşır; farkı yalnızca **tetik** ve **prompt değişkenleri**dir.

### Ek alanlar (yalnız `board` türünde anlamlı)
| Alan | Anlam |
|------|-------|
| `BoardOp` | Hangi kart değişimi tetikler: `move` (varsayılan, boş=`move`), `create`, `update`, `delete`, `any`. |
| `BoardFromState` | Kartın **çıktığı** sütun filtresi (boş=herhangi). |
| `BoardToState` | Kartın **girdiği** sütun filtresi (boş=herhangi). |
| `BoardPriority` | **Aynı** kart değişimine uyan otomasyonlar arasında ateşleme sırası; küçük olan **önce** (vars. 0). |
| `BoardExclusive` | Eşleşen değişimi **tek başına** sahiplenir; aynı olaya uyan diğer tüm pano otomasyonları bastırılır (vars. false). |
| `BoardAction` | Ateşleyince ne yapılır: `spawn` (varsayılan, boş=`spawn`) → hedef ajanı/akışı başlatır (**board yürütmeyi sürer**); `archive` → kartı arşivler (`SetTaskArchived`), **LLM çağrısı yok, hedef gerekmez** — ucuz "done → archive" temizliği. `ValidBoardAction`. |

`boardMatches(a, ev)`: op (boş→move; `any`→hepsi) **ve** from/to sütun filtreleri (boş→herhangi)
eşleşince ateşler.

### Board = yürütmenin kaynağı (generic, per-workspace) — 2026-08-04

Board zaten bir kart değişiminde iş **başlatabiliyordu** (`fireBoard`); iki eksik kapatıldı:

1. **Arşiv aksiyonu (`BoardAction=archive`)** — `fireBoard` artık dallanır: `archive` ise
   `LaunchRun` yerine `db.SetTaskArchived(taskID, true)` çağırır (LLM yok). Arşivlenen kart
   `Task.Archived=true` alır; **geri alınabilir** (silme değil). Sunum katmanı arşivi varsayılan
   gizler: `list_tasks` aracı, `GET /api/tasks` (yeni `?archived=1` ile tümü) ve `get_view board`
   projeksiyonu `db.ListActiveTasks`'e geçti (`ListTasks` bütünlük yolları için tümünü döndürmeye
   devam eder). `UpdateTask` `Archived`'a dokunmaz → arşiv yalnız `SetTaskArchived`'den değişir.
2. **Generic tohum (`EnsureDefaultAutomations`, `internal/agent/automation_defaults.go`;
   2026-08-27'ye kadar `EnsureDefaultBoardAutomations` — artık pano dışı kurallar da
   tohumladığı için yeniden adlandırıldı)** —
   `EnsureDefaultFlows` desenini (silme-defterli `.seeded-automations.json`, `Automation.Seed`)
   birebir taklit eder; her workspace store'una iki pano kuralı tohumlar: **`board-run-in-progress`**
   (`move → in_progress`, `spawn`, hedef=ilk ajan) ve **`board-archive-done`** (`move → done`,
   `archive`). Manager `open()` yolunda çağrılır → mevcut workspace'ler bir sonraki açılışta
   backfill olur. **`Enabled:false` tohumlanır** (opt-in): board yürütmenin kaynağı olsun diye
   Otomasyonlar ekranından tek toggle ile açılır — düz sohbet workspace'inde sürpriz maliyet yok.

Bu generic çift, workspace açılış backfill'idir. Gömülü `workspace-blank` şablonu
buna ek olarak PM hedefli iki **etkin** pano otomasyonu kurar: kart `failed` veya
`review` durumuna taşınınca (`move`) PM'in `sessionMode="continue"` kalıcı oturumunu
uyandırır. Template `enabled` alanı yoksa kural yine pasif kurulur; yalnız açıkça
`true` veren paketler bu varsayılanı aşar. Blank paketindeki CEO cron gözetimi ve
PM yürütmesi için `_Docs/21-MARKET.md` bölümüne bakın.

`BoardAction` create/update yolları: REST (`api/automations.go`) + agent aracı
(`create_automation`/`update_automation`) `ValidBoardAction` doğrular; `archive` kuralı hedef
ajan/akış **istemez**. UI: Otomasyon popup'ında pano tetikleyicisine "Aksiyon" seçici
(`BoardTriggerFields`); arşiv seçilince hedef/prompt alanları gizlenir, kart bir arşiv rozeti gösterir.

Koordinatöre (`prompts/defaults/coordinator.md`) board'ı **tek iş defteri** olarak kullan +
durumu tekrar tekrar `list_tasks` yerine ucuz **`get_view board`** ile oku yönergesi eklendi.

**Arşiv görünümü (UI + REST) — 2026-08-04.** Görevler ekranında başlık çubuğunda **"🗄 Arşiv"**
toggle'ı (`TaskBoard.tsx`, `task-board-archived-toggle`): aktif pano ↔ yalnız arşivlenmiş kartlar.
Arşiv modunda kart oluşturma/sütun editörü gizli; her kartta **"↩ Geri al"** (`TaskCard.onUnarchive`,
memo-güvenli stable callback) + seçim çubuğunda toplu **Arşivle/Geri al**. Manuel uç:
`POST /api/tasks/{id}/archive {archived:bool}` → `SetTaskArchived`, board olayı yayınlar
(`op:archive/unarchive`); `GET /api/tasks?archived=1` arşiv dahil tümünü döndürür (varsayılan hariç).
Frontend `taskApi.listTasks(includeArchived)` + `taskApi.archiveTask(id, archived)`.

### Aynı sütunda çoklu tetik: sıralama + tek sahip (2026-07-24)

**Sorun (TSK59):** İki kural aynı sütunu izlediğinde (ör. Kart Sınıflandırıcı ve Board Planner,
ikisi de `move → todo`) ikisi de ateşliyordu ve **sıra belirsizdi** — `ListEnabledAutomations`
map üzerinden döndüğü için işlem sırası yeniden başlatmalar arasında değişebiliyor, iki otonom
oturum aynı kart üzerinde yarışıyordu. Çakışma o güne kadar Planner'ı **elle kapatarak** önlenmişti.

**Çözüm — `selectBoardAutomations` (`internal/agent/automation_board_order.go`):**
`OnBoardChange` artık eşleşmeleri doğrudan gezmez; önce bu seçici üzerinden geçirir.

1. **Sıralama** — eşleşmeler `BoardPriority` artan, eşitlikte `ID` ile sıralanır. Böylece sıra
   hem **deterministik** (yeniden başlatmadan bağımsız) hem de **yapılandırılabilir** olur.
2. **Tek sahip (exclusive)** — eşleşmelerden herhangi biri `BoardExclusive` ise **yalnız kazanan**
   döner (sıralama sonrası ilk exclusive; yani en düşük `BoardPriority`). Bu, "sütun başına tek
   sahip" garantisidir — kaybedeni elle devre dışı bırakmaya gerek kalmaz.

Ateşleme **sıralı**dır (`for` içinde arka arkaya): ikinci kural, birincinin bıraktığı kart
durumunu görür — yarışmaz. Exclusive bir kural olayı sahiplendiğinde bu, `automation: exclusive
owner claimed board event` satırıyla loglanır.

**Kullanım (iki seçenek):**
- **Zincirleme istiyorsan:** ikisini de açık bırak, `BoardPriority` ver (ör. Sınıflandırıcı 10,
  Planner 20) → önce sınıflandırma, sonra planlama; aynı olayda ama sırayla.
- **Tek sahip istiyorsan:** kazanan kurala `BoardExclusive=true` ver → diğeri açık kalsa bile
  o olayda ateşlemez (başka sütunlardaki kuralları etkilemez).

> Not: exclusive yalnız **kendi eşleştiği olayı** sahiplenir; `todo`'yu sahiplenen bir kural
> `in_progress`'i izleyen kuralı etkilemez.

### Tetik: db board hook (çift-yol tek nokta)
Kart mutasyonları hem UI (`api/tasks.go`) hem ajan araçları (`move_task`/`update_task`/
`create_task`/`delete_task`) üzerinden gelir; ikisi de **`db` katmanındaki** aynı
`CreateTask`/`MoveTask`/`UpdateTask`/`DeleteTask` fonksiyonlarından geçer. Bu yüzden tetik
**db seviyesinde** bir gözlemci hook'a bağlandı (`DB.SetBoardHook`, `BoardChangeEvent`):
mutasyon fonksiyonu kilidi bıraktıktan sonra hook'u çağırır; workspace manager onu
`autoEngine.OnBoardChange`'e (ayrı goroutine → kart mutasyonu **hiç bloklanmaz**) bağlar.
- `CreateTask` → `create` (ToState=başlangıç sütunu)
- `MoveTask` → `move` (yalnız sütun **gerçekten** değişince; From/To dolu)
- `UpdateTask` → sütun değiştiyse `move`, aksi halde `update`
- `DeleteTask` → `delete` (FromState=son sütun)

### Prompt değişkenleri (`boardVars`, `agent/automation.go`)
`{{taskId}}` · `{{title}}` · `{{op}}` · `{{from}}` · `{{to}}` · `{{fromLabel}}` ·
`{{toLabel}}` (sütun anahtarı→ad; özel sütun için anahtar) · `{{board}}` (=`{{to}}`) ·
`{{tags}}` (kartın etiketleri, virgülle ayrık — `BoardChangeEvent.Tags` üzerinden;
tüm op'larda dolu) · `{{owner}}` (atanan ajanın **adı**, `e.db.GetAgent` ile çözülür;
boş = atanmamış) · `{{priority}}` (`critical/high/medium/low`, boş olabilir) ·
ortak: `{{iteration}}` · `{{maxIterations}}` · `{{automation}}` · `{{date}}` · `{{time}}` ·
`{{datetime}}`. `{{result}}` **yoktur** (oturum sonucu yok → append yapılmaz).

### Döngü/güvenlik
Pano otomasyonu **kendini etiketle döngülemez** (spawn'lanan oturum tetik etiketi taşımaz;
`SpawnTags` yok sayılır). Ateşleyen ajan bir kartı taşırsa dolaylı yeniden-tetik olabilir;
MaxIterations (vars. 50) + Cooldown bunu sınırlar. Bildirim tipi yine `automation`
(başlık `🗂`, başarı→executions).

### API / Araç / UI
- **API:** `automationReq`'e `triggerKind`/`boardOp`/`boardFromState`/`boardToState`. Create'te
  board türü `triggerTag` **istemez**, `boardOp` doğrulanır (`ValidBoardOp`); tag türü hâlâ
  `triggerTag` ister. Update'te `triggerKind` **verilmezse** dokunulmaz (kısmi patch — ör.
  yalnız-spawnTags — board otomasyonunu tag'e çevirmesin); verilirse board filtreleri onunla
  birlikte (yeniden) uygulanır. `boardPriority`/`boardExclusive` **pointer** alanlardır: kısmi
  patch'te gönderilmezse **saklı değer korunur** (yalnız-spawnTags düzenlemesi bir sütunun
  sahibini sessizce 0/false'a döndüremez).
- **Araçlar:** `create/update/list_automation`'a aynı alanlar (`list` çıktısına `triggerKind`/
  `boardOp`/`boardToState`/`boardPriority`/`boardExclusive`). `TriggerKind` kısmi patch'te
  pointer ile korunur; `boardPriority`/`boardExclusive` de aynı şekilde.
- **UI — 3 SÜTUNLU PANO (`AutomationBoard.tsx`, 2026-07-28):** Otomasyon ekranı sekmeli görünümden
  **kanban benzeri üç şeride** çevrildi — **⏰ Zamanlamalar (cron)** · **🏷 Etiket otomasyonları** ·
  **🗂 Pano otomasyonları**. Her şerit bir `BoardColumn`: renkli üst şerit + sayaç + kısa açıklama +
  **`+` butonu** (o türün oluşturma popup'ını açar) ve altında kendi kaydırılan kart listesi. Kurallar
  artık **salt-okunur kart** (`ScheduleCard` / `AutomationCard`); satır-içi form ve satır-içi editör
  kaldırıldı — **oluşturma ve düzenleme popup'ta** (`ScheduleModal` / `AutomationModal`, ortak
  `FormModal` kabuğu; Esc/backdrop kapatır). Kart aksiyonları belirgin **çerçeveli ikon butonlar**
  (`CardAction`, `pickers.tsx`): zamanlamada ▶ çalıştır (yeşil hover) + ✏️ düzenle, otomasyonda
  ✏️ düzenle (+ limit dolduysa ↺ sıfırla). **Sil karttan kaldırıldı** → düzenleme popup'ının sol
  altında `🗑 Sil` (birincil aksiyondan uzakta, `confirm` + popup kapanışı board'da). **Dikey/dar
  ekran:** `md` altında üç şerit `snap-x snap-mandatory` karuseli olur (`w-[85vw]`, şerit başına bir
  ekran); `md`+ üçü yan yana `flex-1`. Modal'da kind
  sabittir (hangi sütunun `+`'sına basıldıysa); etiket otomasyonu modalında ayrıca **Ad** alanı ve
  yalnız oluşturmada 🩹 stuck-onarıcı şablon butonu vardır. Board modalında etiket kutusu yerine
  **olay + kaynak/hedef sütun** seçicileri (`BoardTriggerFields`; sütunlar `getWorkspaceSettings().
  boardColumns`'tan, yoksa default) + **Sıra** (`boardPriority`) ve **Tek sahip** (`boardExclusive`).
  Board kartında `#tag` yerine `🗂 <op> (kaynak→hedef)` çipi, `boardExclusive` → `🔒 tek sahip`,
  sıfırdan farklı `boardPriority` → `sıra N` rozeti; spawn-etiket editörü gizli (yerine bilgi notu).
  Prompt textarea + ℹ️ değişken popover'ı ortak `PromptVarsField`'da (`AutomationFields.tsx`), türe
  göre `BOARD_PROMPT_VARS`/`PROMPT_VARS`. `AutomationBoard` tek `listSchedules`+`listAutomations`
  çeker, `columns`/`flows`/`pauseAutonomy`'yi yükler ve tüm mutasyonları (optimistic toggle/sil/
  tags/spawnTags) sahiplenir. Yardımcılar ayrı dosyalarda: `automationMeta.ts` (sabitler + sütun
  renkleri), `cronPresets.ts`, `timeUtils.ts`, `pickers.tsx` (`TargetModeToggle`/`FlowPicker`).

### Test
- `internal/agent/automation_test.go` — `boardMatches` (op/from/to matrisi), `boardVars` (ikame).
- `internal/db/store_task_hook_test.go` — hook create/move/no-op-move/update/delete olaylarında
  doğru `Op`/`From`/`To` ile ateşliyor.

## 2.6 Token-Eşiği Tetikleyicili Otomasyonlar (2026-08-03)

Aynı `Automation` entity'si, `TriggerKind="token"` ile **kümülatif token harcaması**
bir eşik katını geçince tetiklenir. Amaç: "belli token geçilince kendi kendine
optimizasyon/temizlik/bakım çağır" — periyodik öz-bakım. Etiket/pano türleriyle
**aynı** hedefleme (ajan **veya** flow) ve guardrail'leri paylaşır.

### Ek alanlar (yalnız `token` türünde anlamlı)
| Alan | Anlam |
|------|-------|
| `TokenScope` | İzlenen kapsam: `session` (vars., boş=`session`) = bir oturumun **ömür-boyu** tokenı (`SessionUsage`); `workspace` = tüm workspace'in **bugünkü** toplamı (tüm ajan `Usage` satırları toplamı, gün-bazlı sıfırlanır). `db.ValidTokenScope`. |
| `TokenThreshold` | Token **aralığı**: kümülatif her bu kadarın katını geçince ateşler (ör. 100000 → 100k, 200k, 300k…). En az `db.MinTokenThreshold` (1000). Token = `input+output+cacheRead+cacheWrite`. |

### Semantik: "her-N" (tekrarlı), stateless geçiş tespiti
Eşik bir kez geçilip üstünde kalacağından naif kontrol her turda tetiklerdi. Bunun
yerine **her-N** modeli: `TokenThreshold` bir *aralık*tır. Geçiş, kayıt anında
*önceki* vs *yeni* kümülatif toplamdan **stateless** hesaplanır —
`crossedMultiple(prev, now, interval) = prev/interval < now/interval`. Böylece
**per-scope defter (ledger) tutulmaz**; boundary tam olarak onu aşan tek çağrıda
tespit edilir. Cooldown + MaxIterations + ExpiresAt sıklığı yine sınırlar.

### Tetik: usage hook (tek huni `RecordUsage`)
Her sağlayıcı çağrısının tokenı `agent/budget.go` `RecordUsage`'tan geçer (session
+ gün rollup'ı buraya yazılır). Kayıttan sonra `Runtime.FireUsageRecorded` bir
`UsageRecorded{SessionID, DeltaTokens, SessionNewTotal}` sinyalini **detached
goroutine**'lerde dağıtır (turu bloklamaz). Workspace manager `autoEngine.OnUsageRecorded`'ı
`rt.AddUsageHook` ile bağlar.
- **Session kapsam:** `SessionNewTotal − DeltaTokens = prev`; oturum yoksa (detached
  aux çağrı) atlanır.
- **Workspace kapsam:** `db.WorkspaceTokensToday()` (tüm ajan bugünkü toplamı) yeni
  toplam; `prev = yeni − DeltaTokens`. Yalnız bir workspace-kapsam kuralı varsa lazy hesaplanır.

Ateşleme — **kalıcı bakım oturumu (2026-08-06):** token fire artık her seferinde
yeni oturum **spawn ETMEZ**; render edilen promptu otomasyona ait **tek kalıcı
oturuma** (`Kind="automation"`, `SourceID=otomasyon.ID`, `GetOrCreateSourceSession`)
**history-aware** tur olarak teslim eder (`Runtime.deliverAutomationTurn`,
`agent/automation_deliver.go`). Böylece her geçiş **önceki turdan devam eder** —
cron schedule'ın stabil `schedule` oturumuyla aynı model (compaction büyümeyi
sınırlar). Farklı tetikleyen oturumların hepsi aynı bakım defterine akar. Akış
tabanlı (`FlowID`) kural yine `LaunchRun` ile çalışır (akış zaten kendi transkriptini
biriktirir). Reuse yolu `LaunchRun`'un fren kapısını atladığından, otonom teslimden
önce workspace autonomy-pause `e.rt.Paused()` ile elle kontrol edilir.
- **Self-amplification guard:** bakım oturumu her fire'da token harcar; session-kapsam
  kuralında bu upkeep tokenları oturumu bir sonraki eşik katına iter → kendini sıkı
  döngüde yeniden tetikler (yalnız cooldown/maxIterations sınırlar). `OnUsageRecorded`
  session-kapsam dalı bu yüzden `Kind=="automation"` oturumlara atfedilen geçişleri
  **atlar** (bir bellek-içi lookup, lazy). Workspace kapsamı bilerek guard'lanmaz —
  o tokenlar gerçek workspace harcamasıdır ve gün toplamına aittir.

### Prompt değişkenleri (`tokenVars`, `agent/automation.go`)
`{{tokens}}` (eşiği geçen kümülatif toplam) · `{{threshold}}` · `{{scope}}` ·
`{{sessionId}}` (workspace kapsamında boş) · ortak `{{iteration}}` · `{{maxIterations}}` ·
`{{automation}}` · `{{date}}` · `{{time}}` · `{{datetime}}`. `{{result}}` **yoktur**.

### API / Araç / UI
- **API:** `automationReq`'e `tokenScope`/`tokenThreshold` (pointer). Create'te token türü
  `triggerTag` istemez; scope doğrulanır, threshold zorunlu + `db.ValidateTokenThreshold`.
  Update'te `tokenThreshold` kısmi patch (pointer); token türüne dönen kural geçerli eşik taşımalı.
- **Araçlar:** `create/update/list_automation`'a aynı alanlar (üç tetik türü artık `token` içerir).
- **UI:** Otomasyon panosuna **4. şerit "⚡ Token"** (`AutomationBoard.tsx`, `COLUMN_ACCENT.token`).
  `AutomationModal` token dalı: kapsam seçici + eşik girişi (`TokenTriggerFields`); kart rozeti
  `⚡ oturum/workspace · her N token`. Olaylar `automation` tipiyle (başlık `⚡`, başarı→executions).

### Test
- `internal/agent/automation_test.go` — `crossedMultiple` (boundary matrisi), `tokenVars` (ikame).
- `internal/db/automation_limits_test.go` — `ValidateTokenThreshold` (floor), `ValidTokenScope`.

## 2.7 Sayaç (Mesaj/Tool) Tetikleyicili Otomasyonlar (2026-08-06)

Aynı `Automation` entity'si, `TriggerKind="counter"` ile bir **aktivite sayacı**
(bir oturumun **veya** tüm workspace'in — `CounterScope`) bir aralık katını geçince
tetiklenir. Token tetikleyicisinin **kararlı
alternatifi**: token sayısı prompt-cache (`cacheRead`) yüzünden turda 200k–1M
zıplayıp öngörülemez ateşlerken, sayaçlar yavaş ve öngörülebilir büyür — "her N
mesajda/araç çağrısında bir özet/kontrol/bakım" gibi **tempo-bazlı** kurallar için
doğru primitif. Token bütçe bekçisi olarak kalır; tempo için `counter` tercih edilir.

### Ek alanlar (yalnız `counter` türünde anlamlı)
| Alan | Anlam |
|------|-------|
| `CounterMetric` | İzlenen sayaç: `message` (vars., boş=`message`) = `Session.MessageCount` (her user/assistant mesajı); `tool` = `Session.ToolCallCount` (yürütülen araç çağrıları). `db.ValidCounterMetric`. |
| `CounterScope` | İzlenen kapsam: `session` (vars., boş=`session`) = geçişi yapan oturumun kendi sayacı; `workspace` = tüm workspace'in **kümülatif** sayacı (her oturumun toplamı, `db.WorkspaceCounterTotal`). Gün-reset değil: her `interval` **yeni** aktivitede bir ateşler — çok-ajanlı iş temposu için doğru. `db.ValidCounterScope`. |
| `CounterInterval` | Sayaç **aralığı**: izlenen sayaç her bu kadarın katını geçince ateşler (ör. 10 → 10, 20, 30…). En az `db.MinCounterInterval` (2). |

`Session.ToolCallCount`, `AddMessage`'ta asistan mesajının `Steps` içindeki
`kind=="tool"` adımları sayılarak beslenen monotonik bir sayaçtır (yükleme/rewind'de
mesaj satırlarından yeniden hesaplanır — `reconcileHeader` + truncate yolu).

### Semantik: token ile aynı stateless "her-N" geçiş tespiti
`crossedMultiple(prev, now, interval)` **yeniden kullanılır** — per-scope defter
tutulmaz. Prev, sinyalin taşıdığı deltadan çıkarılır (`now − delta`).

### Tetik: activity hook (tek huni `AddMessage`)
Mesaj eklemenin tek choke-point'i `db.AddMessage`; sayaçları güncelledikten sonra
**kilit serbest bırakılıp** `ActivitySignal{SessionID, MessageTotal/Delta,
ToolTotal/Delta}` gönderir (`internal/db/store_activity.go`, `SetActivityHook`).
Workspace manager bunu detached goroutine'de `autoEngine.OnActivityRecorded`'a
bağlar. Board hook'un aynısı: append asla bloklanmaz. `OnActivityRecorded` metrik +
kapsama göre `(total, delta)` seçip geçişi değerlendirir (`agent/automation_counter.go`):
- **Session kapsam:** `total` = sinyalin taşıdığı oturum toplamı (`sig.*Total`).
- **Workspace kapsam:** `total` = `db.WorkspaceCounterTotal(metric)` (tüm oturumların
  toplamı, token'ın `WorkspaceTokensToday` deseni — yalnız bir workspace-kuralı varsa
  lazy hesaplanır). `prev = total − delta`.

Ateşleme token ile aynı: **kalıcı bakım oturumuna** history-aware teslim
(`deliverAutomationTurn`) veya `FlowID` varsa `LaunchRun`. **Self-amplification
guard:** bakım oturumunun kendi mesaj/tool'ları sayacı ittiğinden `Kind=="automation"`
oturumlara atfedilen geçişler **her iki kapsamda da** atlanır (token'da yalnız session
guard'lıydı; sayaçta bakım turu her fire'da mesaj+tool eklediğinden workspace de guard'lı —
upkeep toplama SAYILIR ama kendisi fire tetiklemez).

### Prompt değişkenleri (`counterVars`, `agent/automation_counter.go`)
`{{count}}` (aralığı geçen toplam) · `{{interval}}` · `{{metric}}` · `{{scope}}` ·
`{{sessionId}}` (workspace kapsamında boş) · ortak
`{{iteration}}`/`{{maxIterations}}`/`{{automation}}`/`{{date}}`/`{{time}}`/`{{datetime}}`.
`{{result}}` **yoktur**.

### API / Araç / Test
- **API:** `automationReq`'e `counterMetric`/`counterScope`/`counterInterval` (pointer).
  Create'te metrik+kapsam doğrulanır, interval zorunlu + `db.ValidateCounterInterval`.
  Update kısmi patch.
- **Araçlar:** `create/update/list_automation`'a aynı alanlar (tetik türü artık `counter` içerir).
- **Test:** `db/activity_test.go` (ToolCallCount + hook deltaları + `countToolSteps` +
  `WorkspaceCounterTotal`), `db/automation_limits_test.go`
  (`ValidateCounterInterval`/`ValidCounterMetric`/`ValidCounterScope`),
  `agent/automation_counter_test.go` (`counterVars` + workspace scope).
- **UI (2026-08-06):** Otomasyon ekranına **5. şerit "Sayaç"** eklendi
  (`AutomationBoard`, `COLUMN_ACCENT.counter`, `Hash` ikonu). `AutomationModal` sayaç
  dalı + `CounterTriggerFields` (**kapsam** seçici session/workspace + metrik seçici
  mesaj/tool + aralık girişi, min 2), `AutomationCard` rozeti
  (`# oturum|workspace · her N mesaj/tool`), `task.ts` tipine
  `counterMetric`/`counterScope`/`counterInterval`, `automationMeta` sabitleri
  (`COUNTER_METRICS`/`COUNTER_SCOPES`/`COUNTER_PROMPT_VARS`/`MIN_COUNTER_INTERVAL`/`DEFAULT_COUNTER_INTERVAL`).
- **Canlı şerit istatistiği (2026-08-06):** `GET /api/automations/live-stats`
  (`handleAutomationLiveStats`) `{tokensToday, messages, tools}` döner
  (`WorkspaceTokensToday` + `WorkspaceCounterTotal`). `AutomationBoard` 5 sn'de bir
  çekip **token** şerit başlığına `bugün N token`, **sayaç** şeridine `N mesaj · N tool`
  rozeti (`BoardColumn.stat`) basar — yalnız o şeritte **workspace-scope kural** varsa
  gösterilir (session-scope kuralların workspace-seviyesi tek değeri yok). Amaç: bir
  sonraki ateşlemeye ne kadar kaldığını görüp aralığı kalibre etmek.

### Ortak kod (refactor 2026-08-06)
Dört tür büyüdükçe biriken kopya-kod tek kaynağa toplandı (davranış değişmedi, testler koruyor):
- **`deliverContinuity(a, prompt, trigger)`** (`agent/automation.go`) — token+counter fire'ın
  birebir aynı olan flow/session sürücü seçimi (+pause-guard) tek yerde. `fireToken`/`fireCounter`
  ~30→~8 satır.
- **`notifyFired(a, sessionID, icon, suffix, prompt)`** — dört fire yolunun (tag/board/token/counter)
  `RecordAutomationFire` + success event `publish` boilerplate'i ortak; her tür yalnız ikon+suffix verir.
- **`commonVars(a)`** — dört `*Vars` fonksiyonunun ortak kuyruğu (`iteration`/`maxIterations`/
  `automation`/`date`/`time`/`datetime`, "∞" mantığı dahil) tek yerde; her tür kendi anahtarını ekler.
- **`Automation.EffectiveTokenScope()` / `EffectiveCounterScope()`** (`db/models_automation.go`) —
  `scope=="" → session` normalizasyonu accessor'a; dağınık fallback'ler kaldırıldı.
- **Doğrulama tekilleştirme** — **dört yazma yolundaki** (REST + ajan aracı × create + update)
  tür-bazlı `Valid*` tekrarları kaldırıldı; format/aralık/hedef doğrulaması artık **yalnız**
  `db.ValidateAutomationShape`'te (yollar ayrışamaz). Create handler'larında kalan tek özel
  kontrol: "zorunlu interval atlandı" (pointer nil). Update'te yalnız alan-atama + son shape freni.
- **Not:** frontend `TokenTriggerFields` vs `CounterTriggerFields` **bilerek ayrı** bırakıldı —
  counter'a `metric` alanı eklenince şekiller ayrıştı; zorlama ortak bileşen daha karmaşık olurdu.

### Oturum modu (`SessionMode`, 2026-08-06)
Ajan-hedefli otomasyonlar artık **her tetikte yeni oturum mu / aynı kalıcı oturumu mu** kullanacaklarını
seçebiliyor. Eskiden bu tür-başına sabitti (tag/board = yeni spawn, token/counter = kalıcı bakım thread'i);
artık kullanıcı seçer, varsayılan eski davranışı korur.
- **Alan:** `Automation.SessionMode` ∈ `spawn` | `continue` | `""`. `EffectiveSessionMode()` boşu
  tür-başına çözer (token/counter → `continue`, diğerleri → `spawn`) → eski kayıtlar aynı çalışır.
  `ValidSessionMode` + `ValidateAutomationShape` değeri doğrular.
- **Dispatch (`dispatchFire`, `agent/automation.go`):** flow-backed → her zaman `LaunchRun` (flow kendi
  transcript'i); `continue` → `deliverAutomationTurn` (kalıcı per-otomasyon thread, **geçmiş-farkında**,
  pause guard burada); `spawn` → `LaunchRun` + taze oturum + `SpawnOptions`. Dört fire yolu da bunu kullanır.
- **`continue` semantiği:** ajan her tetikte önceki thread'i görür (cron benzeri). Spawn'a özel şeyler
  (parent-tag temizliği, tag self-loop tohumu) `continue`'da **yok sayılır** — tıpkı token/counter'da olduğu gibi.
  Yalnız ajan-hedefli için anlamlı; flow'da yok sayılır.
- **"Geçmişi oku" ayrı seçenek DEĞİL:** `continue` zaten geçmiş-farkında; ayrı bir toggle gereksiz olurdu.
- **UI:** `AutomationModal`'da ajan hedefi seçiliyken "Oturum: Yeni / Aynı·sürdür" seçici; `AutomationCard`'da
  `🧵 sürdür` rozeti. `create/update_automation` şeması + REST `automationReq` alanı taşır.
- **Test:** `db/automation_limits_test.go` → `TestValidSessionMode` + `TestEffectiveSessionMode` +
  shape'te geçerli/geçersiz mode.

## 3. Otomatik Etiketleme (olay → etiket)

Tur olaylarına ve oturum durumuna göre well-known etiketler otomatik atanır
(`agent/autotag.go`), böylece bir otomasyon (veya kullanıcı) bunları tarayıp
işleyebilir — hedef akış: "tool-error"/"error" etiketli oturumları bulup onaran
tag-tetikleyicili otomasyon. **ADD-only**: etiket, bir onarıcı `set_session_tags`/
API ile silene kadar kalır (onarım sinyali budur).

**Parent-tag temizliği — auto-repair fix (2026-07-06):** Onarıcı spawn, hatalı **parent**
oturumu (`ParentSessionID`) düzeltmek için açılır ama kendi ayrı oturumunda çalışır; `update_session`
tool'u **yalnız içinde bulunduğu oturumu** düzenler → onarıcı parent'ın etiketine ulaşamaz.
Eski davranışta ajan etiketi (yanlışlıkla) **kendinden** kaldırıp parent'ı sonsuza dek işaretli
bırakıyordu. Fix: `SpawnOptions.ClearParentTagsOnSuccess` — motor (`automation.go`), tetikleyici
tag error-sınıfıysa (`tool-error`/`error`; `auth-error` **hariç**, terminal) bunu `[tetik-tag]`
yapar; onarıcı spawn'ın turu **başarıyla** bitince (`runSpawn`, `err==nil`) framework parent'tan
o tag'i `Runtime.RemoveSessionTags` ile siler (başarısızlıkta silmez → sınırlı döngü tekrar
deneyebilir). Böylece etiket temizliği ajanın `update_session` çağrısına bağlı değil.

Atanan etiketler (`agent/autotag.go` sabitleri):
| Etiket | Ne zaman |
|--------|----------|
| `tool-error` | Turda **gerçek** bir tool hatası (`StepTool.IsError`) |
| `error` | Tur-seviyesi hata (provider/aksiyon hatası; kullanıcı "stopped" hariç) |
| `auth-error` | Tur bir **kimlik doğrulama** hatasında bitti (claude-cli login/token) — **TERMINAL** |
| `archived` | Oturum arşivlendi |

> `goal` / `goal-done` etiketleri **2026-07-28'de kaldırıldı** (oturum-hedefi
> mekanizmasıyla birlikte). Bu iki etikete dayanan bir otomasyonun varsa artık
> tetiklenmez — kuralı `stuck` / `error` gibi yaşayan bir etikete taşı.

**`auth-error` — terminal, onarılamaz (2026-07-06):** Bir tur claude-cli kimlik
doğrulaması (login/token süresi/geçersiz anahtar) yüzünden başarısız olduğunda `error`'a
**ek olarak** `auth-error` atanır (`isAuthErrorText` bir `StepError` metniyle eşleşir).
Auto-repair, login'i düzeltemez — `error` tarayan bir onarım otomasyonu `auth-error`
etiketli oturumları **dışlamalı** (aksi halde MaxIterations/Cooldown'a kadar boşuna
döner). Çözüm: ilgili workspace'te `claude /login` (bkz. `tionharness-session-debug`
skill "I) authentication_failed") ya da ajanı API-key sağlayıcıya al.

**"disallowed tool" istisnası:** claude-cli izin verilmeyen bir tool'u denerse
`is_error` sonucu döner ama bu bir **politika reddi**, tool hatası değil — `tool-error`
atanmaz. Native yolda red zaten `StepError{Reason:"permission_denied"}` (StepTool
değil) olduğu için sayılmaz; claude-cli yolunda `isPermissionDenyError` metin
işaretleriyle (`permission_denied`, "requested permissions", "haven't granted",
"not allowed", "disallowed" …) hariç tutar (`permissionDenyMarkers`).

**Bare-name mis-address istisnası (2026-07-06):** Model bir köprülü tool'u **çıplak
adıyla** çağırırsa (ör. `PowerShell`, allowlist'teki `mcp__tionharness_interaction__PowerShell`
yerine) claude-cli `"No such tool available: PowerShell. PowerShell exists but is not
enabled in this context."` ile reddeder ve model **hemen doğru adla yeniden dener**. Bu
kendi kendine toparlanan bir yanlış-adresleme, onarılabilir bir hata değil — bu yüzden
`permissionDenyMarkers`'a `"no such tool available"` + `"not enabled in this context"`
eklendi; artık `tool-error` **atanmaz** (sahte auto-repair spawn'ı tetiklemez). Birim
testi: `autotag_test.go` (bare-`PowerShell` reddi deny kümesinde).

**Tetik noktaları:** `Runtime.AutoTagTurn(ctx, sessionID, steps, turnErr)` — chat
(başarı + cerr hata yolu), spawn (başarı + hata), scheduler `deliverPrompt` +
`deliverWake` (başarı + hata). `archived` ayrıca mutasyon anında: `sessionSink.Archive`
+ API `handleSetSessionState` (arşivde ekle, geri yüklemede sil). Yazım add-only +
değişiklik varsa persist + `session` event (canlı UI refresh).

**Ayar toggle'ı:** `settings.AutoTagSessions` (vars. **açık**) → `Tunables.autoTagSessions`
(`Set/AutoTagSessions`), `applySettings` ile canlı uygulanır. `AutoTagTurn` başında
`r.tun.AutoTagSessions()` guard'ı; tur-dışı arşiv etiketlemesi de `Runtime.AutoTagEnabled()`
(API) + `sessionSink.autoTag()` (agent tool) ile aynı toggle'a bağlı. UI: Ayarlar ▸
Bağlam ▸ "Otomatik etiketleme" (`ContextPanel.tsx`). Kapalıyken hiçbir otomatik etiket
yazılmaz (elle + ajan `set_session_tags` çalışmaya devam eder).

**Canlı doğrulama (2026-07-03, WS2):** archived ekle/sil ✅, goal→`['goal']` ✅,
var-olmayan dosya Read → `is_error` → `tool-error` ✅; toggle OFF→etiket yazılmadı,
ON→yazıldı ✅. Birim testi: `autotag_test.go` (`isPermissionDenyError` gerçek-hata
vs politika-reddi ayrımı).

## Güvenlik / Runaway Freni
- Etiketsiz sohbet asla tetiklenmez (tag-gating).
- Cooldown + MaxIterations + per-otomasyon Enabled + workspace "otonomi duraklat"
  (mevcut) + per-ajan günlük bütçe (`ensureBudget`, spawn autonomous).
- Limit dolunca otomasyon otomatik pasifleşir.

### 5.1 `maxIterations` sözleşmesi (TSK60, 2026-07-29)

Eskiden `MaxIterations = 0` "sınırsız" demekti ve create/update yollarının hiçbiri
bunu engellemiyordu. Kartlar ajanın kendi `move_task` çağrısıyla hareket
edebildiğinden, pano tetikleyicili bir otomasyon insan müdahalesi olmadan sonsuza
kadar dönebiliyordu. Artık üç katmanlı savunma var:

1. **Giriş noktası doğrulaması** — `db.ValidateMaxIterations`
   (`internal/db/automation_limits.go`) `<= 0` ve `> 500` değerlerini reddeder.
   Hem REST (`internal/api/automations.go`, create + update → `400`) hem de ajan
   araçları (`internal/tools/builtin_automationmgmt.go`, `create_automation` +
   `update_automation` → tool hatası) **aynı** fonksiyonu çağırır; iki yolun
   birbirinden ayrışması mümkün değil.
2. **Sert tavan** — `db.MaxIterationsHardCap = 500`. `0` kapatıldıktan sonra
   "pratikte sınırsız" bir sayı geçirerek kuralın etrafından dolaşmayı engeller.
3. **Mutlak güvenlik freni** — `db.AbsoluteIterationBackstop = 1000`. Diskte
   `maxIterations <= 0` ile **zaten kayıtlı** otomasyonlar (bu kural öncesinde
   yazılmış, market paketinden import edilmiş veya JSON'u elle düzenlenmiş)
   giriş doğrulamasına hiç uğramaz. `guardsPass` bu kayıtları backstop'ta durdurur:
   otomasyonu pasifleştirir ve `warn` seviyesinde bir olay yayınlar.

Yani doğrulama yeni kayıtları, backstop ise eski/ithal kayıtları kapatır — 1 ve 2
olmadan 3 yetmez, 3 olmadan 1 ve 2 mevcut veriyi kurtarmaz.

> **Uygulama notu (2026-07-31):** Bu sözleşme 2026-07-29'da **yazıldı ama kodlanmadı**.
> UI `MAX_ITERATIONS_HARD_CAP`'i kullanıyordu, sabit hiçbir yerde tanımlı değildi →
> `tsc -b` kırık (frontend build'i iki gün boyunca derlenmiyordu; `tsc --noEmit` ile
> doğrulandığı için fark edilmemişti). Backend'de ise **hiçbir doğrulama yoktu**: REST de
> ajan araçları da `0`'ı sorunsuz kabul ediyordu ve diskte **dördü `maxIterations=0`,
> üçü aktif** otomasyon duruyordu. Üç katman da bu tarihte hayata geçirildi; frontend
> sabitleri `automationMeta.ts`'te `db` değerlerini aynalar (sunucu otoriter, sapma
> yalnız formun `max` niteliğini etkiler).
>
> **Ders:** sözleşmeyi dokümana yazmak onu yürürlüğe koymaz. Bu bölüm iki gün boyunca
> var olmayan bir korumayı anlatıyordu.
>
> **Şema metni düzeltmesi (2026-08-05):** Doğrulama `0`'ı reddediyordu ama `create_automation`
> aracının şema açıklaması hâlâ "0 = unlimited; default 50" diyordu. Bir ajan bu metne
> güvenip tekrarlayan bir token otomasyonuna `maxIterations=0` yazdı ve tool hatası aldı —
> araç kabul etmediği bir değeri reklam ediyordu. Şema metni (`builtin_automationmgmt.go`
> create + update), `models_automation.go` yorumu ve `frontend/.../task.ts` tipi "1-500;
> 0 reddedilir" olacak şekilde gerçekle hizalandı. Ders: doğrulama eklerken onu duyuran
> tüm metinleri (tool şeması, tip yorumu, UI ipucu) aynı commit'te güncelle.

### 5.2 `ValidateAutomationShape` — create/update şekil sözleşmesi (2026-08-05)

`maxIterations` denetimini ararken bulunan ikinci asimetri: **update yolu** yalnız token
threshold'u yeniden doğruluyordu. Bir ajan `update_automation` ile bir kuralın türünü
değiştirip (veya bir alanı boşaltıp) `create`'in reddedeceği bir şekle sokabiliyordu:
- `triggerKind='tag'` ama `triggerTag` boş → tetik-anında `TriggerTag==""` guard'ıyla
  atlanır → otomasyon **sessizce hiç tetiklenmez** (hata dönmez).
- spawn kuralı ama hedef (`targetAgentId`/`flowId`) boş → yalnız tetik-anında `recordFailure`.

Çözüm: `db.ValidateAutomationShape(a)` (`automation_limits.go`) — birleştirilmiş
(merged) otomasyon üzerinde tür-bazlı zorunlulukları uygular: tag→`triggerTag` zorunlu,
token→geçerli scope+threshold, board→geçerli action; archive dışı her kural bir hedef
ister. **Dört yazma yolu** da (REST create+update, ajan `create_automation`+
`update_automation`) DB'ye yazmadan hemen önce bunu çağırır → create ve update artık
ayrışamaz. Alan-formatı denetimleri (`ValidBoardOp`/`ValidTokenScope`) anında geri-bildirim
için çağrı yerlerinde kalır; bu, birleşik sonucun son freni. `RANGE` denetleyicileri
(`maxIterations`, `tokenThreshold`) ayrı kalıp yanında çağrılır. Test:
`automation_limits_test.go` → `TestValidateAutomationShape` (tür-switch geçerli/geçersiz tablosu).

**Seed'ler de aynı sözleşmeden geçer (2026-08-05):** `EnsureDefaultAutomations`
artık her seed'i `CreateAutomation`'dan önce `ValidateAutomationShape`'ten geçirir.
`board-run-in-progress` spawn seed'i, workspace'te henüz ajan yokken **boş hedefle**
gelir → shape'i geçmez → **deftere yazılmadan atlanır**; bir sonraki açılışta (ajan
varken) backfill olur. `board-archive-done` hedef istemez, hemen tohumlanır. Test:
`automation_defaults_test.go` → `TestEnsureDefaultBoardAutomationsDefersSpawnWithoutAgent`.

**UI inline doğrulama (`AutomationModal.tsx` + `ScheduleModal.tsx`):** zorunlu-alan
ihlalleri artık ilk kaydetme denemesinden (`attempted`) sonra **alan altında inline**
gösterilir (yalnız toast değil). AutomationModal: tag→triggerTag, token→eşik, spawn→hedef
(arşiv board kuralında prompt/hedef aranmaz) → sunucunun `ValidateAutomationShape`'i
istekten önce ayna. ScheduleModal: cron→ifade, hedef→ajan/akış, ajan modunda→prompt
(akış girdisi opsiyonel). Ortak desen: hesaplanmış `*Error` string'leri + `attempted`
gate + hatalı input'ta kırmızı kenarlık.

## Test
- `internal/db/automation_test.go` — CRUD, sayaç, aç/kapa-sıfırla, reset, reload
  kalıcılığı; `SetSessionTags` normalizasyonu.
- `internal/agent/automation_test.go` — `renderAutomationPrompt` (ikame + append +
  no-op), `containsTag`.
- `internal/db/automation_limits_test.go` — `ValidateMaxIterations` sınır tablosu
  (0/negatif reddedilir, 1..500 kabul, 501 reddedilir).
- `internal/agent/automation_backstop_test.go` — `guardsPass` mutlak freni:
  `MaxIterations=0` + sayaç backstop'ta → pasifleşir; backstop altında → çalışır;
  pozitif limit davranışı değişmemiş.

## Sıradaki
- Canlı loop doğrulaması (gerçek sağlayıcıyla uçtan uca; token maliyeti nedeniyle
  unit testlerle ayrıldı).
- Opsiyonel: `tionharness-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi;
  flow/schedule etiketlerini de tetikleyiciye açma (şimdilik yalnız session).
