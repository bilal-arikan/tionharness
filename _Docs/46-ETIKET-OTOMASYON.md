# 46 — Etiketler + Etiket/Pano-Tetikleyicili Otomasyonlar

> Durum: **Uygulandı + canlı doğrulandı** (2026-07-02/03, WS2, sonnet/claude-cli).
> Pano (kart) tetikleyicisi eklendi 2026-07-06.

Üç özellik: (1) oturum/flow/schedule kayıtlarına **etiket** (tag), (2) bir olay
gerçekleşince hedef ajanı/akışı çalıştıran **otomasyonlar** — iki tetik türü:
**etiket** (etiketli oturum bir turu bitirince) ve **pano** (bir kanban kartı
değişince, §2.5), (3) tur olaylarına göre **otomatik etiketleme** (§3).

**Tetik türü (`Automation.TriggerKind`, 2026-07-06):** `""`/`"tag"` (varsayılan,
geriye dönük uyumlu) = etiket tetikleyicili; `"board"` = pano tetikleyicili. Guardrail'ler
(MaxIterations/CooldownSec/ExpiresAt/Enabled), hedefleme (TargetAgentID **veya** FlowID)
ve iterasyon defteri iki tür için **ortaktır** (`guardsPass` paylaşılır).

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
| `MaxIterations` | Toplam tetik üst sınırı (0=sınırsız — dikkat). Vars. 50. Aşılınca otomatik pasifle. |
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

`boardMatches(a, ev)`: op (boş→move; `any`→hepsi) **ve** from/to sütun filtreleri (boş→herhangi)
eşleşince ateşler.

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
- **UI — birleşik 3 SEKME (`Schedules.tsx`, 2026-07-07):** Otomasyon ekranı tek bir **tab bar**
  altında toplandı: **⏰ Zamanlamalar (cron)** · **🏷 Etiket otomasyonları** · **🗂 Pano
  otomasyonları** (her sekmede canlı sayaç rozeti). Tab state Schedules'ta tutulur; `schedules`
  sekmesinde cron başlık+form+liste, diğerlerinde `Automations` bileşeni ilgili bölümü gösterir.
  `Automations` artık **kontrollü**: `activeKind` prop'u (`'tag'|'board'|null`) hangi
  `AutomationSection`'ı render edeceğini belirler (`null` → hiçbir şey; ama bileşen mount kalır ki
  öğe fetch'i + `onCounts` ile bildirilen sayaçlar canlı kalsın). Her `AutomationSection`'ın kendi
  oluşturma formu/listesi/edit state'i var; tür toggle'ı yok (kind sekmeye göre sabit). Board
  bölümünde olay + kaynak/hedef sütun seçicileri; ortak `PromptVarsField` türe göre değişken
  listesi; board satırında `🗂 <op> (kaynak→hedef)` çipi. Board bölümünde etiket kutusu yerine **olay +
  kaynak/hedef sütun** seçicileri (`BoardTriggerFields`; sütunlar `getWorkspaceSettings().
  boardColumns`'tan, yoksa default). `BoardTriggerFields` ayrıca **Sıra** (sayı, `boardPriority`)
  ve **Tek sahip** (checkbox, `boardExclusive`) kontrollerini içerir — hem oluşturma formunda hem
  satır-içi editörde aynı bileşenden gelir. Liste satırında `boardExclusive` → `🔒 tek sahip`,
  sıfırdan farklı `boardPriority` → `sıra N` rozeti (editörü açmadan görünür). Prompt textarea + ℹ️ değişken popover'ı ortak `PromptVarsField`
  bileşeninde, türe göre `BOARD_PROMPT_VARS`/`PROMPT_VARS` gösterir. Board satırında `#tag` yerine
  `🗂 <op> (kaynak→hedef)` çipi; spawn-etiket editörü gizli (yerine bilgi notu). Parent `Automations`
  tek `listAutomations` çeker + `columns`'ı yükler, listeyi `kind`'e göre iki bölüme böler ve ortak
  `setItems`/`reload` ile senkron tutar (optimistic toggle/sil/spawnTags tam liste üzerinde çalışır).

### Test
- `internal/agent/automation_test.go` — `boardMatches` (op/from/to matrisi), `boardVars` (ikame).
- `internal/db/store_task_hook_test.go` — hook create/move/no-op-move/update/delete olaylarında
  doğru `Op`/`From`/`To` ile ateşliyor.

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
| `goal` | Oturumda kalıcı hedef var |
| `goal-done` | Hedef tamamlandı |
| `archived` | Oturum arşivlendi |

**`auth-error` — terminal, onarılamaz (2026-07-06):** Bir tur claude-cli kimlik
doğrulaması (login/token süresi/geçersiz anahtar) yüzünden başarısız olduğunda `error`'a
**ek olarak** `auth-error` atanır (`isAuthErrorText` bir `StepError` metniyle eşleşir).
Auto-repair, login'i düzeltemez — `error` tarayan bir onarım otomasyonu `auth-error`
etiketli oturumları **dışlamalı** (aksi halde MaxIterations/Cooldown'a kadar boşuna
döner). Çözüm: ilgili workspace'te `claude /login` (bkz. `tionswarm-session-debug`
skill "I) authentication_failed") ya da ajanı API-key sağlayıcıya al.

**"disallowed tool" istisnası:** claude-cli izin verilmeyen bir tool'u denerse
`is_error` sonucu döner ama bu bir **politika reddi**, tool hatası değil — `tool-error`
atanmaz. Native yolda red zaten `StepError{Reason:"permission_denied"}` (StepTool
değil) olduğu için sayılmaz; claude-cli yolunda `isPermissionDenyError` metin
işaretleriyle (`permission_denied`, "requested permissions", "haven't granted",
"not allowed", "disallowed" …) hariç tutar (`permissionDenyMarkers`).

**Bare-name mis-address istisnası (2026-07-06):** Model bir köprülü tool'u **çıplak
adıyla** çağırırsa (ör. `PowerShell`, allowlist'teki `mcp__tionswarm_interaction__PowerShell`
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
- Limit dolunca otomasyon otomatik pasifleşir (sonsuz döngü imkânsız, 0=sınırsız
  hariç — UI'da uyarı title'ı).

## Test
- `internal/db/automation_test.go` — CRUD, sayaç, aç/kapa-sıfırla, reset, reload
  kalıcılığı; `SetSessionTags` normalizasyonu.
- `internal/agent/automation_test.go` — `renderAutomationPrompt` (ikame + append +
  no-op), `containsTag`.

## Sıradaki
- Canlı loop doğrulaması (gerçek sağlayıcıyla uçtan uca; token maliyeti nedeniyle
  unit testlerle ayrıldı).
- Opsiyonel: `tionswarm-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi;
  flow/schedule etiketlerini de tetikleyiciye açma (şimdilik yalnız session).
