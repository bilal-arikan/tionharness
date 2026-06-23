# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-06-23**

## Flow graph hata mesajlarına "ne yapmalı" ipucu + paralel node belgelenmesi ✅ (2026-06-23)

**Sorun (WS2/SES2):** Bir agent paralel flow kurarken `parallel` node'unu yanlış
şemayla (`branches:["id"]` + `next`) kurdu → ham Go hatası
`cannot unmarshal string into ... Node.nodes.branches of type orchestration.Branch`.
Hata "ne yapmalı" demediği ve `swarmgo-flows` skill'i node JSON şemasını hiç
belgelemediği (sadece soyut "steps/edges" anlatıyordu) + `create_flow` örneklerinde
paralel örnek olmadığı için agent doğru şemayı bulamadı, sıralı flow'a düştü.

**Çözüm — kendini düzelten tool hataları:** `validGraphJSON` artık her graph
hatasına kısa, eyleme dönük bir ipucu ekliyor (`graphSchemaHint` + `nodeSchemaCheat`):
hatadaki imzaya göre ("Node.nodes.branches", "has no children", "must be an agent
node" vb.) doğru alanı 5-6 kelimeyle söyler — ör. *"parallel fan-out uses
parallel:[...],joinNext — not branches/next"*. Ayrıca `create_flow`'a paralel
fan-out+join örneği ve `swarmgo-flows` skill'ine node-tipi/alan tablosu + paralel
örnek eklendi. (`builtin_flowmgmt.go`, `skills/defaults/swarmgo-flows/SKILL.md`.)
Doğru paralel şema: `{type:"parallel","parallel":["a","b"],"joinNext":"merge"}`
(çocuklar agent node id'leri; `branches`/`next` DEĞİL).

> Not: SES2'de turn 19'daki `provider error: claude CLI failed: exit status 1`
> ayrı bir provider/CLI çöküşüdür (tool hatası değil, detay loglanmadı) — bu fix
> kapsamı dışında.

## 3 self-management iyileştirmesi: create_agent skills + run_schedule + autonomous artifacts ✅ (2026-06-23)

Üç kullanıcı isteği tek turda:

**1) `create_agent` artık skill atayabiliyor.** Yeni opsiyonel `skills` (slug dizisi)
parametresi; verilmezse yeni agent **default SwarmGo skill seti** ile tohumlanır
(`skills.DefaultSkillSlugs()` — embed'deki `defaults/` alt-dizinlerinden türetilir).
Sağlanan slug'lar skill store'a karşı doğrulanır (`r.skillExists`); bilinmeyenler atlanır
ve sonuçta `skippedUnknownSkills` olarak raporlanır. (`builtin_agentmgmt.go`,
`skills/defaults.go`, `toolsetup.go`.)

**2) `run_schedule` aracı eklendi — "şu schedule'ı şimdi fırlat".** Agent bir routine'i
cron zamanını/enabled durumunu beklemeden **manuel tetikler** (UI "Run now" eşleniği,
`Scheduler.RunNow`). Yıkıcı olmadığı için provenance aranmaz (herhangi bir schedule).
Wiring: `Runtime.runSched` + `SetScheduleRunner` (manager `sched.RunNow` bağlar) →
`tools.NewRunScheduleTool` (self-management). (`builtin_schedulemgmt.go`, `runtime.go`,
`toolsetup.go`, `workspace/manager.go`.)

**3) Artifact'lar her turda oluşturulabilir.** Önceki hata: autonomous turlarda
(scheduler/spawn/flow) artifact sink kurulmadığı için `create_artifact` →
*"artifacts are not available for this turn"*. Çözüm iki yolu da kapsar:
- **Native:** `completeTraced` chokepoint'inde, ctx'te sink yoksa ve sessionID varsa
  fallback sink kurulur (`tools.HasArtifactSink` + `Runtime.NewArtifactSink`).
- **claude-cli:** `api/autonomous_interaction.go` artık run'a `setArtifacts` çağırıyor
  (Interaction MCP köprüsünün gördüğü sink).
- Yeni emit'li sink `internal/agent/artifactsink.go`'da (db'ye yazar + `artifact`
  event'i yayar; chat yolundaki api sink'inin aynası). Artifact'lar artık otonom
  ajanların ürettiğinde de UI'da bildirilir.

Test: `tools` (create_agent skills + run_schedule), `agent` (NewArtifactSink persist),
mevcutlar uyarlandı. **`tools`+`agent`+`skills`+`api`+`workspace` 221 test yeşil**,
build+vet+gofmt temiz. Skill `swarmgo-self-management` güncellendi (+ on-disk senkron).

---

## Self-management araçları prompt'tan gizlendi → skill katalog oldu (hidden-lazy tier) ✅ (2026-06-23)

**Sorun:** Self-management araçları zaten lazy'di (şema yok), ama ~40+ aracın **isim+özet
satırı** her turun "Available Tools (load on demand)" bloğunda (cached prefix) yer alıyordu —
gereksiz token. **Çözüm:** lazy araçlara **hidden** alt-katmanı eklendi; self-management suite
artık blokta **listelenmez**, yerine `swarmgo-self-management` skill'ine yönlendiren tek satır
durur. Araçlar aktive-edilebilir ve aranabilir kalır.

- **`internal/tools/registry.go`:** yeni `hidden map[string]bool` (hidden ⊆ lazy) +
  `MarkHidden(names…)`; `VisibleLazyCatalog(allow)` (= LazyCatalog − hidden, blok için);
  `HiddenLazyCount(allow)`. `LazyCatalog` (activate_tools/tool_search kaynağı) **tüm** lazy'yi
  döndürmeye devam eder → hidden araçlar aktive/aranabilir.
- **`internal/agent/toolsetup.go`:** self-management suite (`builtins[selfManageStart:]`)
  `MarkLazy` yerine **`MarkHidden`**. `LazyToolsCatalogBlock` artık `VisibleLazyCatalog` +
  `HiddenLazyCount` kullanır; `renderLazyToolCatalog(visible, hiddenCount)` hiddenCount>0 ise
  "**N self-management tools … not listed here … load the `swarmgo-self-management` skill … or
  `tool_search`**" pointer satırını basar.
- **Keşif yolu:** Available Skills bloğu `swarmgo-self-management` skill'ini zaten ilan ediyor
  (giriş noktası). Skill **kataloğun kendisi** oldu; metni güncellendi ("bu skill araçların
  listesidir; isimleri buradan/`tool_search`'ten al, `activate_tools` et"). On-disk seed kopya
  da güncel kaynakla senkronlandı (EnsureDefaults üzerine yazmadığı için).
- **Kapsam dışı (şimdilik):** secret_*/list_sessions/WebFetch/*_config hâlâ görünür-lazy
  (self-management değil, az sayıda, çapraz-kesen). claude-cli Interaction MCP köprüsü
  (`BridgeableDefs`) tam şema göndermeye devam ediyor (ayrı yol) — istenirse ayrıca kısılır.
- Test: `tools` (MarkHidden/VisibleLazyCatalog/HiddenLazyCount + aktive-edilebilirlik) +
  `agent` (render pointer + boş durum). **`tools`+`agent` 150 test yeşil**, build temiz.

---

## Generic bildirim sinyalleri: busy / unread / dirty (nav + workspace) ✅ (2026-06-23)

"Haber verme" parçaları (flow/schedule/sohbet/board/artifact değişimleri +
kaydedilmemiş ayar) tek bir generic sisteme toplandı. Her nav görünümü için 3 dik
sinyal: **busy** (accent nabız), **unread** (accent dolu), **dirty** (amber). Hepsi
workspace etiketine yukarı toplanır.

- **Backend**: `api/notify.go` `publishEntityChange` + yeni `board` (task taşıma)
  ve `artifact` (agent sink + UI) event'leri — chat/flow ile aynı SSE borusu.
- **Frontend**: `lib/eventViews.ts` (event→view), `hooks/useUnreadViews.ts`
  (workspace-başına, cross-window persist), `lib/dirtySignals.ts`
  (`useSyncExternalStore` modül store + `useRegisterDirty`). `NavRail` `NavDots`
  ile 3 durumu çizer; `WorkspaceSwitcher`/collapsed ikon aktif workspace'i toplar.
- **Kayıtlı dirty ekranlar**: Settings, WorkspaceView, FlowsPanel.
- **Pencere dışı**: tab başlığı `(N) SwarmGo` (odak dışıyken) + taskbar/dock
  rozeti (`navigator.setAppBadge`, Edge/WebView2'de native taskbar). Toplam
  görülmemiş sayısı `App.tsx` `unreadTotal`. `lib/appBadge.ts`,
  `hooks/useUnreadBadge.ts`.

Detay: `_Docs/29-BILDIRIM-SINYALLERI.md`. Build + tsc yeşil.

## Fix: "Aktivite" nav göstergesi arka plan oturumlarında yanmıyordu ✅ (2026-06-23)

**Sorun:** Bir flow/schedule/agent **ayrı bir oturum** başlattığında (spawn, inbox
teslimi, schedule wake, flow node'ları) sol navbar'daki **Aktivite** öğesinde "işlem
sürüyor" göstergesi çıkmıyordu. Çünkü `handleActivity` yalnızca chat-stream registry'sini
(`s.runs.activeSessionIDs()`) + çalışan task/flow run'larını sayıyordu; otonom invoke'ları
izleyen `Runtime.ActiveSessionIDs()`'i (executions feed'in kullandığı sinyal) **hiç
kullanmıyordu**. Ayrıca `executions` görünümü için bir bayrak yoktu.

**Çözüm:** `activityState`'e `executions` bayrağı eklendi. `handleActivity` artık
chat-stream + `Runtime.ActiveSessionIDs()` oturumlarını birleştiriyor; **herhangi** biri
varsa `executions` yanıyor, ek olarak oturum kind'ı (`chat`/`flow`/`schedule`) ilgili
görünümü de yakıyor. Frontend: `useActivity` → `executions` → `'executions'` View;
`getActivity` tipi güncellendi. `api/activity.go`, `useActivity.ts`, `api/system.ts`.

## UI: ID görünürlüğü + disk yolu erişimi (sohbet / akış / zamanlama / log) ✅ (2026-06-23)

Ajan ekranındaki "ID + klasörü aç/kopyala" deseni diğer ekranlara da yayıldı:

1. **Sohbet listesi (SessionsSidebar)** — her oturum başlığının yanında küçük
   mono **oturum ID'si** (ajanlardaki gibi).
2. **Akışlar (FlowsPanel)** — editör araç çubuğunda **akış ID'si** + **yolu kopyala**
   (`CopyPathButton`) + **klasörü aç** (Explorer `/select`). Yeni backend:
   `GET /api/flows/{id}/path`, `POST /api/flows/{id}/reveal`, `db.FlowPath`.
3. **Zamanlamalar (Schedules)** — her satırda cron ifadesinin yanında mono
   **zamanlama ID'si**.
4. **Loglar (LogsPanel)** — kontrol çubuğunda **log dosyası yolunu kopyala** +
   **klasörü aç**. Loglar artık disk dosyasına da yazılıyor: `SetupLogging`
   stdout + `io.MultiWriter` ile `<dataDir>/logs/swarmgo.log` (append, best-effort).
   Yeni: `config.DefaultDataDir()`, `config.LogFilePath()`, `GET /api/logs/path`,
   `POST /api/logs/reveal` (`api/logs_path.go`).

**Not (workspace rengi):** "kullanılmıyorsa kaldır" istendi ama renk **kullanılıyor** —
NavRail (daraltılmış workspace ikonu) ve WorkspaceSwitcher ikon arkaplan tonu. O yüzden
ayar korundu. API: `api/flows.ts`, `api/system.ts`. Build + tsc yeşil.

## Sıradaki-tur bağlam önizleme (debug) + peer mesajlaşma Faz 2–3 ✅ (2026-06-23)

1. **Sıradaki-tur bağlam önizleme** — Agent ekranındaki bağlam önizlemesinin oturum
   karşılığı: SessionDetailPanel'de **"Bağlam önizle (debug)"** → `SessionContextModal`,
   `GET /api/sessions/{id}/context-preview?message=`. Ajanın bu oturumda sonraki turda
   alacağı tam isteği (sistem + dinamik + **mesaj dizisi** + araçlar, ~token'larla) gösterir.
   **Yan etkisiz** (compaction/persist/provider çağrısı yok; `Prepared` elle kurulur).
   `api/session_context.go`, `SessionContextModal.tsx`, `types/session.ts`, `api/sessions.ts`.
2. **Peer mesajlaşma Faz 2** — yanıt ergonomisi: alıcı `from` adını `to` yapıp yanıtlar
   (araç açıklamasında talimat). Faz 1'de inbox turu zaten geçmiş-duyarlıydı.
3. **Peer mesajlaşma Faz 3 (kısmi)** — **broadcast `"*"`** (`broadcastAgentMessage`,
   best-effort + slot guard, test). Kalan (UI inbox göstergesi + grafik `messaged` kenarı)
   ve **Faz 4** (yapısal protokol) ertelendi.

Testler: `sendmessage_test.go` (+broadcast), `chat_tool_summary_test.go`. Build +
131 test yeşil (cmd/swarmgo-desktop WIP hariç). Detay: `_Docs\07-CHAT-UX.md`,
`_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## Medya/binary artifact desteği (create_artifact sourcePath + auto-capture) ✅ (2026-06-23)

**Sorun (SES30'da görüldü):** Ajan Chrome MCP ile ekran görüntüsü aldı ama PNG'yi
artifact yapamadı; `create_artifact` yalnız inline metin `content` kabul ediyordu,
ajan da binary'yi base64 olarak context'ten geçirmeye zorlanıp token sınırına çarptı
ve "yapısal engel, çözülemez" sonucuna vardı. Oysa depolama (`models_artifact.go`
`image/video/audio/file` kind'ları, `SourcePath`) ve frontend (`ArtifactView`
medya render) zaten hazırdı — eksik olan **ajana açık araç** + **auto-capture'da
medya farkındalığı**ydı.

**Çözüm — iki parça:**

1. **`create_artifact` genişletildi** — `kind` enum'una `image/video/audio/file`
   eklendi + yeni `sourcePath` parametresi. Medya kind'larında bytes context'e hiç
   girmez: dosya yolu verilir, `ArtifactSink.CreateArtifact` artık `CreateArtifactSpec`
   struct'ı alır, sink `db.ImportMediaSource` ile yolu workspace-göreli hale getirir
   (workspace dışındaki dosyayı — ör. Downloads'taki screenshot — `artifacts/<session>/`
   altına **kopyalar**, içindekini olduğu yerden referanslar). Doğrulama: text kind →
   `content` zorunlu, media kind → `sourcePath` zorunlu.
2. **Auto-capture genişletildi** (`artifacts_auto.go`) — (a) `artifactKindForPath`
   medya uzantılarını (`.png/.jpg/.gif/.webp/.mp4/.mp3/.pdf/.zip/...`) doğru media
   kind'a eşler (artık `text`'e düşmez); (b) `Write`/`create_file` medya dosyası
   yazarsa binary-as-text yerine `SourcePath` ile yakalanır; (c) **herhangi bir aracın
   çıktısı** taranır (`extractProducedMediaPaths` + regex) — screenshot/export araçları
   kaydettikleri dosya yolunu döndürünce o dosya da medya artifact'ı olarak yakalanır.
   Var olmayan yol stat'ta elenir → sahte artifact üretilmez.

Her zaman enjekte edilen `artifactDeliverableGuidance` promptu, ajana "binary dosyayı
`sourcePath` ile ver, base64 gömme" talimatıyla güncellendi.

Dosyalar: `tools/artifact.go` (`CreateArtifactSpec`), `tools/builtin_artifact.go`,
`db/artifact_content.go` (`ImportMediaSource`+`copyFileContents`), `api/artifacts.go`
(sink + guidance), `api/artifacts_auto.go`. Testler: `db/artifact_media_test.go`,
`api/artifacts_auto_media_test.go` (+ güncellenen `mcp_interaction_test.go` fakeSink).
Build + tüm api/db/tools testleri yeşil.

## Peer mesajlaşma Faz 1 (send_message/mailbox) + araç I/O geçmiş özeti ✅ (2026-06-23)

İki iş birlikte yapıldı:

1. **`send_message` (Faz 1)** — Claude Code mailbox deseninin uyarlaması: bir ajan
   başka ajana **adresli, kimlikli** mesaj atar; mesaj alıcının kalıcı **inbox**
   oturumuna (`GetOrCreateKindSession` kind="inbox") `<agent_message from="…">`
   etiketiyle düşer ve alıcının **geçmiş-duyarlı turu** arka planda çalışır
   (fire-and-forget, `SpawnMaxConcurrent` guard, kendine-mesaj reddi). self-manage
   gated. `run_subagent` (izole görev) ile birlikte durur; bu "süregelen peer
   işbirliği" yolu. Dosyalar: `agentmsg.go`, `builtin_sendmessage.go`, `toolsetup.go`;
   `runSessionTurn` ile wake/inbox ortak geçmiş-duyarlı runner. Plan/detay:
   `_Docs\28-PEER-MESAJLASMA-PLANI.md`.
2. **Araç I/O geçmiş özeti (#5)** — geçmişte araç çağrı/sonuçları düşüyordu; artık
   son N=4 asistan turunun `Steps` izinden kompakt `<recent_tool_activity>` bloğu
   (kopya üzerinde) eklenir → ajan "az önce ne yaptın / ne döndü"yü yanıtlar
   (`api/chat_tool_summary.go`). Wake/inbox turları da dahil.

Testler: `sendmessage_test.go`, `chat_tool_summary_test.go` (+ mevcutlar). Build +
241 test yeşil (cmd/swarmgo-desktop'taki ilgisiz WIP hariç). Detay:
`_Docs\07-CHAT-UX.md`, `_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## MCP kalıcı bağlantı havuzu (persistent pool) ✅ (2026-06-23)

Daha önce belgelenen 🔴 kısıt (dinamik MCP araç ekleme / `tools.listChanged` yok +
dial-per-operation) **kalıcı olarak çözüldü**. Önceki ara çözüm (60sn katalog TTL cache,
`mcpcatalog.go`) **kaldırıldı**, yerini sunucu başına **canlı oturum havuzu** aldı.

**`internal/mcp/client.go` (yeniden yazıldı):** stdio istemci artık **kalıcı + eşzamanlı
kullanıma güvenli**. Tek arka-plan **read loop** yanıtları JSON-RPC `id`'ye göre per-call
kanallara demux eder; sunucu bildirimleri (`notifications/tools/list_changed`) bir
callback'e yönlenir. `initialize` artık `capabilities.tools.listChanged=true` ilan eder.
Yeni API: `SetOnToolsChanged`, `Alive`. (`proc.Command` süreç-grubu kill korundu.)

**`internal/mcp/pool.go` (yeni):** `Pool` her enabled MCP server için **tek canlı
`StdioClient`** tutar (sanitized ada göre).
- **Catalog reuse:** tur-başı tekrarlı `buildRegistry` çağrıları aynı canlı oturumu
  kullanır → gateway'de **session churn yok** (eski burst sorunu da tamamen biter).
- **Oturum-durumu korunur:** gateway'in `activate_tools` etkisi oturum kapanmadığı için
  **sonraki çağrıda da yaşar** → dinamik araç ekleme artık çalışır (tur-ötesi).
- **listChanged → invalidate:** sunucu araç listesi değişince entry stale işaretlenir,
  sonraki `Catalog` aynı canlı oturumda yeniden listeler. Ek emniyet: TTL
  (`SWARMGO_MCP_POOL_TTL_SEC`, vars. 60sn) — listChanged göndermeyen sunucular için.
- Config (command/args/url/env fingerprint) değişiminde veya bağlantı ölümünde şeffaf
  re-dial; çağrı ölü bağlantıda bir kez retry eder.

**Wiring:** `Runtime.mcpPool *mcp.Pool` (+ `CloseMCP()`, workspace teardown'da çağrılır:
`workspace/manager.go` delete + Close). `buildRegistry` → `pool.Catalog`. `Registry`
artık `mcpCaller` taşır (`AttachMCP(..., caller)`); MCP çağrıları pool üzerinden gider,
nil ise dial-per-call'a düşer (testler). Tek-seferlik test endpoint'i
(`api/mcp.go handleTestMCPServer`) + `live_test.go` hâlâ dial-per-op `ListServerTools`/
`CallNamespaced` kullanır (kasıtlı).

**Kalan nüans:** Aynı tur içinde `activate_tools` sonrası yeni araçlar **bir sonraki turda**
çağrılabilir (tur başı `reg` sabit). Pre-load için preset hâlâ en pürüzsüz yol; ama artık
zorunlu değil. Test: `internal/mcp/pool_test.go` (sahte stdio MCP sunucusu: persistent
reuse, listChanged refresh, config-change re-dial, Close, hata yolları). **`internal/mcp`
+ `internal/agent` + `internal/tools` + `internal/workspace` 153 test yeşil**; build+vet
temiz (ilgisiz `internal/e2e` MemGPT WIP build hatası hariç).

---

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴→✅ (2026-06-23)

Çok-ajan "kim ne dedi" çözümünü iki referansla karşılaştırdık:
- **external-agent-oss:** sorunu *yaşamıyor* — bir oturum = tek ajan; çok-ajan ayrı oturum.
  Mesajlarda per-mesaj yazar alanı yok; Claude Agent SDK döngüyü sürüyor.
- **Claude Code (`observed-behavior` swarm/teammate):** çok-ajanı **izole bağlam + adresli
  mailbox** ile çözüyor — `SendMessage({to,message,summary})`, alıcının inbox'ına `from`
  kimliğiyle `<teammate_message teammate_id>` etiketiyle düşer; plain çıktı diğer ajana
  görünmez. Kimlik **doğuştan**; ardışık-rol çakışması hiç oluşmaz.

SwarmGo iki modeli birden taşıyor: paylaşılan-thread (etiketleme+coalesce ile sağlamlaştırıldı)
ve izole `run_subagent`. Eksik olan "akran ajana adresli DM" için **uyarlama planı** yazıldı:
`_Docs\28-PEER-MESAJLASMA-PLANI.md` (mevcut `GetOrCreateKindSession` inbox + `SpawnSession`
üzerine). Kavramsal not: `_Docs\10-KAVRAMSAL-TASARIM-NOTLARI.md` §10. **Uygulama kullanıcı
onayı bekliyor** (tetik/inbox modeli/ayrı-araç kararları planda).

## Çok-ajanlı bağlam sağlamlığı: yazar kimliği + ardışık-rol + geçmiş-duyarlı wake ✅ (2026-06-23)

**Bağlam:** SES29'da iki ajana soru soruldu ama ajanlar "kim ne dedi"yi göremedi.
Kök neden: `toProviderMessages` geçmişi çevirirken `AgentID`'yi düşürüyordu → tüm
asistan turları tek ayrımsız "assistant" sesine karışıyordu. Düzeltme + aynı sınıftan
diğer kusurlar tarandı; en kritik 3'ü (+ kullanıcı-hedefi) kapatıldı:

1. **Yazar etiketleme** (önceki tur) — çok-yazarlı geçmişte her asistan turu yazarıyla
   ön-eklenir (`api/chat_authors.go`); kendi turlarına `(you)` markerı. **Güncelleme
   (2026-06-23):** etiketleme eşiği "2+ farklı yazar"dan "geçmişte **yanıtlayan ajandan
   farklı** bir yazar var mı"ya genişletildi → ajanı değiştirilmiş (devredilen) oturumda
   da (tek önceki yazar A, şimdi B yanıtlıyor) A'nın turları etiketlenir, B onları
   kendisininki sanmaz. Saf tek-ajan oturumu (yalnız yanıtlayan konuşmuş) hâlâ etiketsiz
   (doğal transkript + prompt cache korunur).
2. **Geçmiş-duyarlı wake** — `schedule_wake` ile uyanan ajan eskiden yalnız wake
   prompt'unu görüyordu (`invokeTraced`, geçmiş yok). Artık `WakeTurnFunc` hook'u
   (`api/wake_turn.go`) tam sohbet turunu (geçmiş+özet+hafıza+goal) kurar. Detay:
   `_Docs\20-SCHEDULE-WAKE.md`.
3. **Ardışık aynı-rol birleştirme** — bir kullanıcı mesajına 2 ajan ardışık yanıtlarsa
   `user→assistant→assistant` oluşuyor, Anthropic "roles must alternate" ile reddediyordu.
   `providers/coalesce.go` ardışık aynı-rol düz-metin turları birleştirir (araç turlarına
   dokunmaz); anthropic + minimax çeviricilerinde uygulanır.
4. **Kullanıcı mesajının hedef ajanı** — kullanıcı mesajı artık yönlendirildiği ajanla
   (`AgentID = agents[0]`) damgalanır; geçmişte `"[User → Ada]: …"` etiketlenir → "hangi
   soru kime" de görünür. `@Ad` yalnız bilgi amaçlı (yönlendirme değil).

Testler: `chat_authors_test.go`, `providers/coalesce_test.go` (+ mevcutlar). Tüm
build + 167 test yeşil. Detay: `_Docs\07-CHAT-UX.md`, `_Docs\20-SCHEDULE-WAKE.md`.

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴 (2026-06-23)

**Bulgu (gerçek vaka):** Bir ajan MCP Gateway üzerinden `mcp-chrome`'u kullanmak istedi.
Gateway'in `activate_tools('mcp-chrome')` çağrısı **"✅ 29 tools activated"** döndü ama
ardından `chrome_navigate` çağrısı **`No such tool available`** verdi. Gateway'in kendisi
uyardı: *"Your client did not advertise tools.listChanged support… reconnect with a preset."*

**Kök neden — iki birleşen mimari gerçek:**
1. **`tools.listChanged` yok:** istemci `initialize`'da `capabilities:{}` gönderir
   (`internal/mcp/client.go`), yani sunucu "araç listem değişti" bildirimini gönderse bile
   SwarmGo `tools/list`'i yeniden çağırmaz.
2. **Dial-per-operation (havuzsuz):** `BuildCatalog`/`CallNamespaced` her işlemde **yeni
   session** açıp kapatır. Gateway'in `activate_tools`'u **oturum-kapsamlıdır** → araçları
   o anlık session'a ekler, session `Close()` ile kapanınca kaybolur. Eklenen araçlar
   SwarmGo'nun kataloğuna hiç girmez → çağrılamaz.

→ Sonuç: **runtime'da araç ekleyen/çıkaran MCP sunucularıyla SwarmGo uyumsuz.**

**Geçici çözüm (uygulandı):** İstenen araçlar sunucunun bağlantı URL'indeki **preset'e**
konur; preset her taze session'da başlangıçta yüklendiği için dial-per-operation modeliyle
sorunsuz çalışır. MCP Gateway `swarmgo` preset'ine `mcp-chrome` eklendi
(`mcp-server/config.json`: `swarmgo: [<remote-service>, mcp-chrome]`); `?preset=swarmgo` artık
47→**76 araç** döndürüyor. Doğrulandı.

**Kalıcı çözüm (Sırada / öneri):** ya (a) `initialize`'da `tools.listChanged` ilan edip
**kalıcı session** tut + bildirimde `tools/list`'i yenile, ya da (b) gateway gibi
dinamik sunucular için kalıcı bağlantı havuzu (Seçenek 2) — böylece `activate_tools`
etkisi sonraki çağrıda da yaşar. İkisi de aynı `internal/mcp` yeniden tasarımına bağlanır.

---

## HA-1: human bloğu otomatik kullanıcı modelleme (MemGPT Parça 4b) ✅ (2026-06-23)

`human` çekirdek bloğu artık dream-cycle ile **otomatik** doldurulur. `reflect()`
her çalıştığında (manuel/auto), yansımadan sonra journal'lar silinmeden önce
`updateUserModel` çağrılır: model mevcut profili + journal'ı alıp kullanıcı
hakkındaki kalıcı çıkarımları kısa satırlar olarak merge eder, `human`'a yazar.

- **Yeni dosya** `internal/agent/user_model.go` (`updateUserModel`/`writeUserModel`
  + prompt); `reflector.go`'ya tek `if r.tun.UserModel()` satırı. `runtime.go`
  **dokunulmadı**. Usage `KindReflect`'e yazılır (dream-cycle maliyeti).
- **Best-effort**: değişiklik yoksa no-op, limit aşılırsa truncate, hata yansımayı
  bozmaz. **Ayar** `AutoUserModel` (varsayılan açık) — settings + Tunables +
  applySettings + frontend toggle.
- `go build` ✅, **206 test** ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 4b.

## Çekirdek bellek: adlandırılmış bloklar + karakter limiti (MemGPT Parça 5) ✅ (2026-06-23)

Letta'nın **memory blocks** modeli native getirildi. Sabit persona/human ikilisi,
ajanın istediği etikette tanımlayabildiği **dinamik bloklar**a genelleşti; her blok
**karakter limiti + açıklama + salt-okunur** taşır.

- **Encoding** `kind="core:"+label` (`db.CoreKind/IsCoreKind`); tanım ajan dosyasında
  (`Agent.CoreBlocks`), içerik `knowledge_sources`'ta. `CoreBlocks` boşsa varsayılan
  persona+human (2000 char) → migration yok.
- **Store**: `WriteCore/ReadCore/AppendCore(label)` + limit (`*CoreBlockFullError`),
  `ErrUnknownCoreBlock`, `ReadCoreBlocks→[]BlockView`, `DefineCoreBlock`/`DeleteCoreBlock`.
- **Araç**: `section`→`label` (alias korundu); bilinmeyen/read-only/limit hatası ajana
  net döner. Constructor imzaları sabit → `runtime.go`/`toolsetup.go` **dokunulmadı**.
- **API**: `GET /core→{blocks}`, `PUT /core {blocks:{label:content}}`, `POST/DELETE
  /core/blocks[/{label}]`. **Frontend**: `CoreMemoryCard` dinamik + limit çubuğu +
  blok ekle/sil + read-only kilit.
- Doğrulama: `go test` 141 ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 5.

## Composer: oturum-başına taslak + ikon-tabanlı kontroller ✅ (2026-06-23)

İki UX iyileştirmesi (kullanıcı isteği). Yalnız frontend, `tsc --noEmit` temiz.

1. **Oturum-başına taslak.** Yeni `useSessionDraft` hook'u (`hooks/useSessionDraft.ts`):
   composer'a yazılıp **gönderilmeyen** metin `localStorage`'da oturum-id ile saklanır
   (`swarmgo:draft:<sessionId>`). Oturum değiştirip dönünce ve sayfa yenilenince korunur;
   gönderme/temizleme taslağı siler (boş taslak saklanmaz). Composer `useState('')` yerine
   bu hook'u kullanır — tüm mevcut `setText` çağrıları otomatik kalıcı. **Yan fayda:** eskiden
   metin oturumlar arası sızıyordu (Composer `key`'siz, monte kalıyor); artık her oturum kendi taslağını taşır.
2. **İkon-tabanlı composer kontrolleri.** Ajan seçici yalnız avatar (ad tooltip'te); düşünme
   seviyesi yoğunluk ikonuyla (◌○◔◑●); izin modu emoji ikonuyla (🛡🔒✋⚡). `ComposerPicker`'a
   `iconOnly` prop'u eklendi; `THINKING_OPTIONS`'a seviye ikonları eklendi.

---

## E2E test paketi — uçtan uca ajan davranışları ✅ (2026-06-23)

Yeni **`internal/e2e`** test paketi: tam kablolu bir `agent.Runtime` (gerçek dosya-store,
memory, skills, sandbox, `conversation.Manager`) `api/chat_stream`'in sürdüğü tur hattının
**aynısıyla** sürülür; yalnız LLM, ağsız-deterministik bir **`scriptedProvider`** ile
değiştirilir. Provider sıraya konmuş yanıtları kuyruktan tüketir (metin turu veya `tool_use`
turu → native araç döngüsü gerçek araçları çalıştırır), `conversation.Manager`'ın rolling-summary
özetleme çağrısını ise script'i bozmadan yakalayıp yanıtlar. Harness (`harness_test.go`) her turda
kullanıcı mesajını kalıcılaştırır, geçmişi bütçeleyici üzerinden tekrar oynatır, sistem+memory
bağlamını dizer, akışlı araç döngüsünü koşar, yanıtı kalıcılaştırır + journal'lar.

**Kapsam (16 test, hepsi yeşil):**
- **Konuşma:** çok-turlu geçmiş kalıcılığı + ikinci tura ilk alışverişin tekrar oynatılması (`conversation_e2e_test.go`).
- **Araç kullanımı:** tek turda `Write`→`Read` çok-adımlı döngü (dosya gerçekten diske düşer, sonuç cevaba akar, `StepDiff` izi), shell-gate (`tools_e2e_test.go`).
- **Skill kullanımı:** workspace skill oluştur → katalogta slug+özet (gövde lazy) → `use_skill` ile gövde yükleme; ayrıca per-agent allowlist ile kısıtlı skill erişilemezliği (`skills_e2e_test.go`).
- **Hafıza:** `core_memory_replace` ile kalıcı çekirdek bellek + sonraki tura tekrar enjeksiyon; `memory_recall` ile uzun-dönem hatırlama (`memory_e2e_test.go`).
- **Uzun oturum:** bütçe aşımında compaction tetiklenmesi (özet kalıcı, yalnız son tur'lar verbatim) (`longsession_e2e_test.go`).
- **İzin modu:** read-only modda yazma engellenir/okuma serbest; ask modunda interaktif prompter ile onay/red (yazma diske düşer ya da engellenir, `permission_denied` izi) (`permission_e2e_test.go`).
- **Self-wake:** `schedule_wake` ctx'teki scheduler'a doğru delay/prompt ile ulaşır; scheduler yokken (headless) net hata (`wake_e2e_test.go`).
- **Delegasyon:** delegasyon kapalıyken `run_subagent` kayıtlı değil (unknown tool); açıkken bilinmeyen hedef target-çözümleme guard'ıyla reddedilir (`delegation_e2e_test.go`). Not: alt-ajan provider'ı registry'den çözüldüğü için (anahtarsız) mutlu-yol e2e'si üretim kodu değişmeden test edilemez; gate+guard yolları kapsandı.
- **Çok-ajanlı tur:** tek kullanıcı mesajı, iki ajan sırayla yanıtlar; ikinci ajan birincinin cevabını geçmişte görür (`multiagent_e2e_test.go`).

Harness genişletildi: `decorate` ctx-kancası (prompter/grants/wake enjeksiyonu) + `sendMulti` (çok-ajanlı tur sürücüsü).
İzolasyon: `SWARMGO_DATA_DIR` temp'e yönlendirilir → gerçek `~/.swarmgo` skill/market seed'ine dokunulmaz.
✅ `go test ./internal/e2e/` 16/16 yeşil, `go vet` temiz.

## Native pencere — konsol penceresi yanıp sönmesi düzeltildi ✅ (2026-06-23)

Konsolsuz desktop binary (`-H windowsgui`) bir konsol alt-süreci başlattığında Windows'un
çocuk için açtığı terminal penceresi yanıp sönüyordu (claude CLI/PowerShell/git/MCP). Yeni
**`internal/proc`** paketi (`Hide(cmd)` → Windows'ta `CREATE_NO_WINDOW`, diğer platformlarda
no-op) konsol açan tüm `exec.Command` çağrılarına eklendi: `claudecli.go` (ana suçlu),
`builtin_shell.go`, `agent/hooks.go`, `agent/worktree.go`, `mcp/client.go`, `api/git.go`,
`workdir_context.go`, `workspaces.go`. `explorer.exe` (GUI) dokunulmadı. ✅ build/vet +
305 test yeşil. Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native pencere — başlık çubuğu temaya uyumlu ✅ (2026-06-23)

WebView2 penceresinin native başlık çubuğu (caption + küçült/büyüt/kapat butonları + kenarlık)
artık uygulama temasına boyanıyor — beyaz Windows frame'i koyu temayla çelişmiyor. **DWM** ile
(`dwmapi.dll` `DwmSetWindowAttribute`, salt `syscall`, yeni bağımlılık yok): `DWMWA_USE_IMMERSIVE_DARK_MODE`
(Win10 1809+) + `DWMWA_CAPTION_COLOR`/`TEXT_COLOR`/`BORDER_COLOR` (Win11 22000+). `app.App.Appearance()`
çözülen `ThemePreset`/`Theme`/`Accent`'i verir; `cmd/swarmgo-desktop/titlebar_windows.go` 8 curated paletin
bg/text/border'ını (`themePresets.ts` ile elle senkron) COLORREF'e (`0x00BBGGRR`) çevirir. Bilinmeyen
preset → yalnız dark/light frame (caption rengi atlanır); eski Windows'ta desteklenmeyen attribute'lar
sessizce yok sayılır (pencere yine çalışır). **Canlı güncelleme:** `watchTitleBar` 1.5sn poll ile tema
değişince `w.Dispatch` üzerinden yeniden uygular (Ayarlar'dan palet değiştirince başlık anında uyar).
✅ `go build ./...`/`vet` yeşil; desktop canlı (varsayılan midnight-violet, pencere açıldı, health 200,
panik/hata yok). Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native masaüstü penceresi — WebView2 (CGO'suz) ✅ (2026-06-22)

SwarmGo artık tarayıcı yerine **kendi masaüstü penceresinde** açılabiliyor. Plan:
[32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md). **Ön koşul refactor (davranış-korumalı):**
`cmd/swarmgo/main.go`'nun boot dizisi yeni **`internal/app`** paketine taşındı
(`SetupLogging` + `Bootstrap`/`Serve`/`Shutdown`/`Addr`/`URL`); `Bootstrap` artık listener'ı
önden açar (`net.Listen`, `:0` boş port desteği) ve `SetBaseURL`'i çözülen adresle çağırır.
`main.go` ~130→~50 satır. **Yeni giriş noktası** `cmd/swarmgo-desktop` (`//go:build windows`,
[`jchv/go-webview2`](https://github.com/jchv/go-webview2) — **saf Go, CGO yok**; Win11'de
yerleşik WebView2 runtime): sunucuyu `127.0.0.1:0`'da başlatır, `waitForHealth` ile hazır olunca
1280×800 WebView2 penceresi açar, pencere kapanınca graceful `Shutdown`. WebView2 yoksa →
varsayılan tarayıcıya fallback (`rundll32 url.dll`). `!windows` stub mevcut. `scripts/build.ps1`
`-Desktop` bayrağı (`-H windowsgui` → konsolsuz). Bağımlılık: `go-webview2` (direct) +
`go-winloader`/`x/sys` (indirect) — yalnız desktop hedefinde derlenir; **başsız `swarmgo`
hâlâ saf-Go/çapraz-derlenebilir**. ✅ `go build ./...`/`vet` yeşil; başsız smoke (refactor sonrası
`/`+`/health` 200, boot logları aynı); desktop canlı (rastgele port 60385'te boot, `/health`+`/`
200, pencere açıldı); `-H windowsgui` build 12 MB. README "Native Masaüstü Uygulaması" eklendi.

## Prompt saatine saniye eklendi (SES28 zaman-ölçüm hatası) ✅ (2026-06-22)

**Sorun:** SES28'de bir ajan "30 sn bekle, farkı ölç" görevinde ilk saati **uydurdu**
(`18:18:48`). Kök neden: sistem-prompt saati yalnız **dakika** hassasiyetindeydi
(`15:04`), `get_current_time` aracı da kaldırılmıştı → saniye gereken ölçümde ajanın
gerçek saati yok, uyduruyor. (Not: `schedule_wake` timer'ı doğru — tam 30 sn tetikledi;
hata zamanlayıcıda değil, uydurmadaydı.)

- **Çözüm (geçici):** `dateTimeContextBlock` (chat) + `autonomousSystemPrompt` (headless)
  artık `15:04:05` (saniyeli) yazıyor ve satır "tur başında yakalandı, tur içinde ilerlemez"
  diye etiketli — ajan ölçüm görevinde bunu **baseline** alsın, uydurmasın.
- **Kalıcı (sırada):** hafif, her-zaman-açık `get_current_time` aracı (saniye + unix epoch)
  — bilerek kaldırılmıştı; kullanıcı onayı bekliyor.
- **Doğrulama:** `go build ./...` + `go vet` temiz.

---

## Ajan path/reveal + oturum yolu ~ gösterimi + context-mode dedektörü ✅ (2026-06-22)

Üç küçük UI/UX iyileştirmesi (kullanıcı isteği). Build + `tsc --noEmit` temiz.

1. **Ajan dosya yolu / klasör aç.** Ajan ayarları formuna (`AgentSettingsForm`) sağ
   üstte **Yolu kopyala** + **Klasörü aç** butonları eklendi. Backend: `db.AgentPath`
   (ajanın `agents/<id>.json` mutlak yolu) + `GET /api/agents/{id}/path` +
   `POST /api/agents/{id}/reveal` (`explorer.exe /select,<path>` ile dosyayı vurgular).
   Frontend api: `agentApi.agentPath`/`revealAgent`. Session reveal deseninin ajan eşleniği.
2. **Oturum yolu `~` gösterimi.** `SessionDetailPanel` "Klasör" bölümü `info.path`'i ham
   gösteriyordu; artık `displayPath()` ile `~\...` kısaltmasıyla gösterir (title'da tam yol;
   "Yolu kopyala" hâlâ tam yolu kopyalar).
3. **context-mode dedektörü.** Hooks "Harici token araçları" listesine (`external_tools.go`)
   `context-mode` eklendi (rtk/sqz/headroom yanında).

---

## MCP katalog önbelleği — gateway'de session birikmesi düzeltildi ✅ (2026-06-22)

**Sorun:** Yerel MCP Gateway'de saniyeler içinde 4 ayrı `swarmgo` session açılıyordu
(her biri `requestCount:3`). Kök neden: native MCP istemcisi **havuzsuz** (`manager.go`
dial-per-operation) ve `buildRegistry` tek bir sohbet turunda birden çok kez çağrılıyor
(tur girişi `runtime.go`, native döngü `toolloop.go`, UI/araç önizleme endpoint'leri).
Her çağrı `BuildCatalog` ile **her enabled MCP server'ı yeniden dial ediyordu**
(`initialize` + `notifications/initialized` + `tools/list` = requestCount 3), ardından
`Close()`. Gateway HTTP-köprülü olduğu için her dial yeni bir session açıyor; stdio
istemci HTTP `DELETE` göndermediğinden gateway session'ları idle olarak birikiyordu.

**Çözüm — workspace başına katalog önbelleği** (`internal/agent/mcpcatalog.go`):
- `Runtime.mcpCat *mcpCatalog` — dial edilmiş katalogu (entries) bellekte tutar.
- **Fingerprint-tabanlı geçersizleme:** anahtar = enabled server config'lerinin
  SHA-256 fingerprint'i (sıra-bağımsız). Server toggle/ekle/sil/düzenle → fingerprint
  değişir → otomatik rebuild. **Ayrı invalidation hook'u gerekmez.**
- **TTL:** varsayılan **60 sn** (`SWARMGO_MCP_CATALOG_TTL_SEC` ile override; `0` =
  önbellek kapalı, eski davranış). Config'in göremediği dış değişiklikleri (server
  farklı tool sunması) sınırlar.
- Sadece pahalı dial sonucu (entries) önbelleklenir; ucuz dispatch haritası
  (`cfgByServer`) her çağrıda canlı server listesinden yeniden hesaplanır → asla
  drift etmez. Nil-receiver toleranslı (bare `&Runtime{}` testleri önbeklsiz çalışır).
- **Etki:** tur-başı dial burst'ü her server için **1**'e iner → gateway session
  birikmesi ~%90 azalır.
- Test: `mcpcatalog_test.go` (cache hit/miss, fingerprint geçersizleme, TTL süresi,
  ttl=0, nil-receiver, env parse). `internal/agent` + `internal/mcp` **81 test** yeşil.

> **Kalan (tam çözüm değil):** `CallNamespaced` (gerçek tool çağrısı) hâlâ dial-per-call.
> Asıl tool kullanımı seyrek olduğu için burst kaynağı değil; kalıcı bağlantı havuzu
> (Seçenek 2) veya native HTTP transport + `Mcp-Session-Id` reuse ileride değerlendirilebilir.

---

## Bütçe ceil/fraction artırıldı + eskimiş 1M-token ayarı kaldırıldı ✅ (2026-06-22)

**Hedef:** (1) 1M modelleri daha çok kullan, (2) eskimiş 1M-token beta ayarını temizle.

- **Ceil/fraction (1M kullanımını artır):** `budgetWindowFraction 0.10→0.20`,
  `budgetAutoCeil 32K→128K`, tool-eşik scale clamp `[1,5]→[1,12]`. Artık 1M modeller
  **128K** transcript bütçesi (≈%12.8), Haiku **40K**. 1M'de ceil operatif knob.
  `budget_test.go` + `tunables_compact_test.go` güncellendi. **124 test** yeşil.
- **Eskimiş 1M-token ayarı kaldırıldı:** Web doğrulaması — Anthropic 1M'i **13 Mart 2026**
  GA yaptı (header gerekmez), `context-1m-2025-08-07` beta header'ı **30 Nisan 2026**
  kapatıldı. Bugün 22 Haziran → tamamen işlevsiz.
  - **Provider:** `anthropic.go` artık header'ı **göndermiyor** (const + field + append
    silindi; `WithBetas(_, extendedCache)`). 43 test + vet temiz.
  - **Frontend:** Ayarlar → Anthropic beta'daki "1 milyon token bağlam" toggle'ı +
    `oneMillionContext` tipi/payload referansları kaldırıldı (appPanels + types +
    SettingsPanel). `npm run build` yeşil.
  - **Kalan (entangle):** backend `settings.OneMillionContext` alanı + `server.go`
    `SetAnthropicBetas` argümanı + registry/ResolvedConfig plumbing vestigial kaldı
    (zararsız, header gitmiyor). `api` paketi paralel MemGPT WIP'inden kurtulunca
    tek temiz commit'te purge edilecek (server.go o dosyada entangle).

---

## `http_get` → `WebFetch` zengin fetch ✅ (2026-06-22)

`http_get` (düz GET, 64KB) **`WebFetch`'e yükseltildi** — claude-cli'nin WebFetch'inin
native eşleniği, son web-parite boşluğu kapandı. Build+vet temiz, **300 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart.

- **HTML→Markdown converter** (`internal/tools/htmltomarkdown.go`, **stdlib-only** —
  go.mod minimal kalır, `x/net/html` yok): regexp geçişleriyle script/style/nav/form
  blokları atılır; başlık/link/liste/bold/italic/code/pre → Markdown; göreli linkler
  base URL'e çözülür; `html.UnescapeString` ile entity çözme; whitespace temizliği.
- **`WebFetchTool`** (`builtin_http.go`): SSRF-guard'lı dialer korunur (loopback/
  private/link-local + 169.254 metadata + CGNAT engeli, redirect/DNS-rebind kapsanır).
  HTML→Markdown; metinsel içerik (md/plain/json/xml) verbatim; ikili içerik özetlenir
  (dump edilmez). `raw=true` ham gövde döndürür. Ham indirme 3MB, çıktı 96KB cap.
  Yönlendirme sonrası final URL başlıkta.
- **İsim:** `http_get`→`WebFetch` (classify zaten `WebFetch=RiskRead` taşıyordu;
  `bridgeExcluded` anahtarı güncellendi — CLI kendi WebFetch'ini kullandığından bizimki
  köprülenmez). Referanslar: `toolsetup`/`subagent`/`registry`/frontend `tools.ts` +
  default skill + testler (`tooltier`/`lazyload`/`builtin_http`). Yeni test:
  `htmltomarkdown_test.go` (4 senaryo). Dokümanlar: `09-SDK`, `11-INTERACTION`, `19-LAZY`.

## Tool eşikleri per-model bütçeye hizalandı + model picker rozeti ✅ (2026-06-22)

**Hedef:** (1) §5 tool eşiklerini de per-model bütçeye bağla (zinciri tutarlı kıl),
(2) frontend model picker'da context-window'u göster.

- **Tool eşikleri per-model:** `Tunables`'a `CompactMaxBytesFor(budget)` /
  `CompactLLMThresholdFor(budget)` + `ContextBudgetTokens()` getter + saf
  `budgetScaleFor(budget)`. Compactor her tur `conversation.EffectiveBudget(
  provider, model, ContextBudgetTokens())` hesaplayıp For-varyantlarına geçiyor.
  No-arg getter'lar process-geneli bütçeyle geriye-uyumlu. Zincir: model penceresi
  → EffectiveBudget → eşik ölçeği. `tunables_compact_test.go` (For). Agent **71 test**.
- **Model picker rozeti:** `CatalogModel.contextWindow?` (TS) + `ProviderModelSelect`
  `formatContextWindow` (200000→"200K", 1000000→"1M"); dropdown option'larında
  "· 200K" + seçili modelde "200K bağlam" rozeti. `tsc` + `npm run build` yeşil.
- **Not:** Hepsi agent/conversation/providers/frontend'de — `api` paketine dokunulmadı.

---

## claude-cli `use_skill` isim uyuşmazlığı düzeltmesi ✅ (2026-06-22)

**Hedef:** SES21'de açık kalan konu — claude-cli ajanı ilk turda `use_skill`'i çağırınca
"No such tool available: use_skill" alıyordu (ikinci turda kendini toparlıyordu).

- **Kök neden (yarış değil, isim uyuşmazlığı):** "# Available Skills" prompt bloğu modele
  **çıplak** `use_skill` adını söylüyordu (`skills/store.go renderCatalog`). Native
  ajanlarda araç gerçekten `use_skill`; ama **claude-cli** ajanlarında SwarmGo built-in'leri
  Interaction MCP köprüsünden **namespaced** geliyor: `mcp__swarmgo_interaction__use_skill`.
  Model prompt'u harfiyen izleyip çıplak adı deniyor → CLI reddediyor. (`trace.go` namespace'i
  soyduğu için başarılı 2. çağrı izde yine `use_skill` görünüyor — kafa karıştırıcı.)
- **Çözüm:** Katalog bloğu artık aracı **ajanın göreceği adla** yazıyor. `renderCatalog`
  skill-araç-adı parametresi aldı; `skills.DefaultSkillTool` sabiti + yeni
  `CatalogBlockForAgentTool(assigned, skillTool)`. `runtime.go skillToolNameFor(provider)`:
  provider `""`/`claude-cli` → namespaced, diğerleri (anthropic/minimax/openrouter/custom)
  → çıplak. `SkillsCatalogBlockForAgent` bunu kullanıyor (chat + autonomous yolları).
- **Doğrulama:** `TestSkillToolNameFor` (6 vaka) + `TestCatalogBlockForAgentTool`;
  `internal/agent` & `internal/skills` **88 test** yeşil, `go vet` temiz.

---

## SES21 hava-durumu oturumu hata düzeltmeleri ✅ (2026-06-22)

**Hedef:** Bir kullanıcı oturumunda (SES21, claude-cli ajanı) çıkan üç somut hatayı gider.

- **Shell `$` değişkeni bozulması (kök neden):** Ajan komutunu zaten-PowerShell olan
  `Bash` aracının içinde tekrar `powershell -Command "..."` ile sarmaladı → dış kabuk
  `$geo` vb. değişkenleri iç kabuğa geçmeden boşaltıp sildi ("An empty pipe element").
  Çözüm: (1) araç açıklamasına "tekrar `powershell -Command` ile sarmalama" uyarısı,
  (2) `unwrapRedundantPowershell` ile gereksiz dış sarmalı savunmacı olarak açma (yalnız
  tüm komut sarmalsa ve iç tarafta kaçışlı tırnak yoksa). `builtin_shell.go` + test.
- **Artifact geri okunamıyor:** `list_artifacts` yalnız id/başlık veriyordu; ajan
  içeriği göstermek için dosya yolunu (`.artifacts\...`) tahmin edip "File does not exist"
  aldı. Çözüm: yeni **`read_artifact`** aracı (id → içerik, bellekten; yol tahmini yok) +
  `list_artifacts` artık `contentFile` yolunu da döndürüyor. İkisi de `RiskRead`.
  `builtin_artifactmgmt.go`, `classify.go`, `toolsetup.go`.
- **Veri uydurma (skill davranışı):** `daily-weather-report` skill'i WebFetch başarısız
  olunca uydurma tablo yazıyordu. Skill'e eklendi: çok-şehir için Open-Meteo geocoding,
  weather.com kazımayı yasakla, **asla veri uydurma**, forecast≠iklim ortalaması,
  alt-ajana da aynı kuralı geçir.
- **Doğrulama:** `go build ./...` + `internal/tools` & `internal/db` **85 test** yeşil.
- **Açık kalan:** claude-cli köprüsünde `use_skill` ilk çağrıda "No such tool available"
  (MCP aracı ilk turda çözülmüyor) — ayrı, daha derin bir köprü konusu; not edildi.

---

## Modele göre akıllı varsayılan bütçe — Option B ✅ (2026-06-22)

**Hedef:** Flat 12K transcript bütçesi büyük modelin (200K–1M) penceresini boşa
harcıyordu. Pencere metadata'sını (önceki commit) gerçekten kullan.

- **`conversation.EffectiveBudget(provider, model, configured)`**: pencere biliniyorsa
  bütçe = `clamp(window × 0.10, configured, 32K)` — yapılandırılmış değer **taban**
  (asla altına inmez), 32K **tavan** (1M modelde maliyet guard'ı), bilinmeyen → değişmez.
- **`Manager.Prepare`** artık compaction tetiğini + pressure oranını model-aware bütçeyle
  hesaplıyor. `maxTokens≤0` (bütçe kapalı) dokunulmaz — `TestPrepareZeroPressure...`
  semantiği korundu (ilk denemede bu testi kırdım, `if maxTokens>0` guard'ıyla düzelttim).
- **Sonuç:** Opus 4.8/Sonnet 4.6 1M → 32K · Haiku 4.5 200K → 20K · MiniMax/DeepSeek/Gemini 1M → 32K · bilinmeyen → 12K.
- **Doğrulama:** `budget_test.go` + conversation/providers **51 test** yeşil, `go vet` temiz.
- **Follow-up:** §5 tool eşikleri hâlâ process-geneli bütçeyle (dormant); per-model
  `EffectiveBudget`'a bağlamak temiz sonraki adım. Detay: `17-TOKEN-OPTIMIZASYON.md` §7.

---

## Per-model context-window metadata ✅ (2026-06-22)

**Hedef:** Modellerin context-window boyutunu metadata olarak taşı (UI + gelecekteki
tokenLimitFor zemini). Kullanıcı isteği.

- **`ModelInfo.ContextWindow int`** (token, `contextWindow,omitempty`). `Catalog()`
  build-time'da merkezi **`ContextWindowFor(provider, model)`** aile-tablosundan
  doldurur → manifest'ler churn'den uzak kalır, yine her modelde değer görünür.
- **Aile-bazlı, muhafazakâr:** Opus 4.8/Sonnet 4.6 **1M**, Haiku 4.5 **200K**,
  MiniMax/DeepSeek/Gemini 1M (web'le doğrulandı: M3 = 1,048,576; Opus/Sonnet 1M,
  Haiku 200K), gerisi 0 = "bilinmiyor" → fallback. 40+ third-party OpenRouter
  modelini elle yanlış doldurmaktansa emin olunanlar.
  > **Düzeltme (2026-06-22):** İlk sürümde Claude ailesine düz 200K verilmişti; Opus
  > 4.8 ve Sonnet 4.6 aslında **1M**, sadece Haiku 200K. Per-tier eşlemeyle düzeltildi.
- **Test:** `context_window_test.go` (aile eşleme + Catalog dolduruyor mu) — providers
  paketi **43 test** yeşil, `go vet` temiz. `api`'ye dokunulmadı (JSON tag otomatik akar).
- **Phase 2 (tokenLimitFor) bilinçle ertelendi:** model penceresine ölçekleme SwarmGo'nun
  12K transcript bütçesiyle çelişir (bir tool sonucu tüm bütçeyi aşar); doğru hamle
  "modele göre akıllı varsayılan bütçe". Detay: `17-TOKEN-OPTIMIZASYON.md` §6.

---

## CG-9 ikinci yarı — bütçe-orantılı tool eşikleri ✅ (2026-06-22)

**Hedef:** the external agent project'ın `tokenLimitFor` (tool-result eşiği context window'a göre)
deseninin SwarmGo karşılığı. Model context-window metadata'sı yok (`ModelInfo`
sadece ID/Label), o yüzden mevcut **transcript bütçesine** (`MaxContextTokens`)
orantıladım — kullanıcının zaten modeline göre ayarladığı knob.

- **`Tunables.budgetScaleLocked()`** = `budget/12000`, clamp **[1×,5×]**.
  `CompactMaxBytes()` ve `CompactLLMThreshold()` artık base × scale döndürüyor.
  **Aynı faktör** → A-cap(16384) > B-eşik(12288) değişmezi her ölçekte korunur.
- **5× tavan** → B≈60KB, the external agent project'ın ~60KB özet tavanıyla örtüşür.
- **Default bütçe (12000) → 1×** → değerler birebir mevcut → **regresyon yok**.
- **`SetContextBudget`** setter + `applySettings` wiring (`s.convo.SetLimits` yanında).
- **Doğrulama:** `tunables_compact_test.go` + agent paketi **71 test** yeşil, `go vet` temiz.
- **Not:** wiring satırı (`server.go`) paralel oturumun MemGPT Parça-4 `/api/agents/{id}/core`
  route'larıyla aynı dosyada uncommitted → o paket bütünleşince commit'lenecek.
  Wiring olmadan `contextBudgetTokens=0` → 1× → güvenli no-op (dormant).

---

## Ayarlar ekranı kaydetme tutarsızlığı düzeltildi ✅ (2026-06-22)

**Hedef:** Ayarlar ekranında gösterilen ama Kaydet'e basınca diske yazılmayan
("sessizce kaybolan") alanları onar — UI/kaydetme tutarsızlığı denetimi.

- **Kök neden:** `SettingsPanel.tsx` `saveApp()` patch nesnesini elle alan-alan
  kuruyordu; panellerde render edilen 10 kontrol bu listede yoktu. Kullanıcı
  değiştirip Kaydet'e basınca patch alanı içermiyor, backend değişmemiş değeri
  döndürüyor ve `setOriginal(updated)` kontrolü eski haline geri alıyordu (hatasız).
- **Onarılan 10 alan:** `defaultPermissionMode` (Sağlayıcılar) + `reactiveCompact`,
  `maxTokenRetries`, `reactiveKeepRecent`, `compactToolOutput`, `compactMaxLines`,
  `compactMaxBytes`, `compactLlmSummary`, `compactLlmThreshold`, `compactModel`
  (Bağlam — Tur kurtarma + Sistem A/B sıkıştırma bölümlerinin tamamı). Hepsi
  `saveApp()` patch'ine eklendi.
- **İkincil:** `spawnMaxConcurrent`/`spawnMaxPerTurn` backend (Patch+store clamp+
  server canlı uygulama) ve skill dokümanında vardı ama frontend `AppSettings`
  tipinde, UI'da ve patch'te **yoktu**. Tipe eklendi, Tools paneline kontrol
  (1–128 / 1–64) eklendi, patch'e eklendi → uçtan uca bağlandı.
- **Doğrulama:** `tsc --noEmit` temiz. Skill `swarmgo-settings` zaten tüm alanları
  doğru belgeliyordu (değişiklik gerekmedi).

**Diğer düzenleme ekranlarının denetimi (aynı tur):** Workspace (`WorkspaceView`),
Ajan (`AgentSettingsForm`), Hooks (`HooksPanel`), Skill (`SkillEditor`), Görev
(`TaskDetailPanel`) ve Akış (`FlowsPanel`) ekranları tek tek denetlendi — **hepsi
temiz**: her biri render ettiği tüm alanları kendi create/update payload'una
gönderiyor (elle-omit yok). Workspace'te `instructions`/`boardColumns`,
SkillEditor'da `color` bilinçli/dökümante şekilde ayrı yüzeyde. Asıl hata yalnız
App Settings'teydi.

**Ölü alan temizliği:** `db.Agent.Capabilities` (`models.go`) kaldırıldı — hiçbir
yerde okunmuyordu (yalnız `store.go`'da `"[]"` default'lanıp market install'da
yazılıyordu, geri-publish yolu yok). 3 nokta: `models.go` alan, `store.go` default
bloğu, `api/market.go` atama. Ardından pack formatı
`market.AgentPayload.Capabilities` (SwarmPack v1) alanı da kaldırıldı — `omitempty`
olduğu için eski pack JSON'ları sorunsuz parse olur (alan varsa yok sayılır). Eski
agent JSON'larında migrasyon gerekmez. `go build` (db/api/market) temiz, db testleri
20/20, market testi geçti.

---

## CG-9 density-aware estimator + Sistem B varsayılan açık ✅ (2026-06-22)

**Hedef:** the external agent project kıyaslamasında çıkan iki açığı kapat — (1) yoğun içerikte token
undercount ("session poisoning"), (2) büyük araç-sonucu özetinin kutudan-kapalı olması.

- **CG-9 (birinci yarı):** `conversation/tokens.go` `estimateText` density-aware oldu.
  Tek geçişte rune+whitespace sayar; uzun & `<%3` boşluklu (≥256 rune) içerik **~1.5
  chars/token** (`runes*2/3`), düz metin **~4**. `utf8` importu düştü. `tokens_test.go`.
  Transcript bütçesi (12K) ve UI meter artık base64/hex'i doğru sayıyor. **Kalan:**
  tool-result eşiğini context window'a göre ölçekleme (CG-9 ikinci yarı).
- **Sistem B varsayılan açık:** `settings.Default()` + `tunables` sabitleri —
  `CompactLLMSummary: false→true`, `CompactLLMThreshold: 8192→12288`,
  `CompactMaxBytes: 12288→16384`. **Kritik:** A'nın cap'i B eşiğinin üstüne çıkarıldı,
  yoksa A çıktıyı B eşiğinin altına kırpıp B'yi pre-empt ediyordu. Doc 17 güncellendi.
- **Doğrulama:** conversation/settings/agent **89 test** yeşil; `TestEstimateTextDensity`
  ayrıca tek tek geçti. **Not:** `go build ./...` şu an paralel oturumun yarım MemGPT
  Parça-4 işinden (`ReadCore`/`WriteCore` 3-arg, `coreMemoryBlock`) **kırık** — benim
  paketlerim (api'ye bağımsız) izole derlenip test edildi; bozuk dosyalara dokunulmadı.

---

## Araç hizalama + tarih enjeksiyonu + bellek/arama CLI köprüsü ✅ (2026-06-22)

Üç bağımsız iyileştirme (kullanıcı isteği). Build+vet temiz, **289 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart (127.0.0.1:8090).

1. **`get_current_time` kaldırıldı → tarih sistem prompt'unda.** Ajan saati artık
   tool round-trip yerine bağlamdan okur. `composeTurnRequest` dinamik bloğuna ve
   `autonomousSystemPrompt`'a tek satır eklendi (`dateTimeContextBlock`,
   `"Current date and time: Monday, 2006-01-02 15:04 (-07:00)"`). `builtin_time.go`
   + testi silindi. Bu, claude-cli'nin zaten yaptığının native eşleniği.

2. **Çekirdek araç isimleri claude-cli ile hizalandı.** `read_file→Read`,
   `write_file→Write`, `edit_file→Edit`, `list_dir→LS`, `glob→Glob`, `grep→Grep`,
   `shell→Bash` (`builtin_fs.go`, `builtin_shell.go`). Model bu isimlere yoğun
   eğitimli → daha güvenilir tool-use. `classify.go`/`permpattern.go` zaten her iki
   isim setini taşıyordu → tek sete indirgendi (`execArgTools={"Bash"}`). Yan
   referanslar güncellendi: subagent profilleri, `artifacts_auto.fileWriteTools`,
   hook şablonları, MarkLazy, bridge dispatch case, frontend (`DiffCard`/`tools.ts`/
   `stepKinds`). `http_get` kasıtlı korundu (CLI WebFetch'ten farklı; düz GET).

3. **`core_memory_replace/append` + `conversation_search` CLI'ye köprülendi.** Bu
   eager built-in'ler native-loop ctx bağımlılığı taşımadığından `BridgeTools` def
   listesine doğrudan eklendi (gate'leri `CoreMemoryTools()`/`SessionContextEnabled()`);
   dispatch zaten `bridgeCallFor()`→`reg.Call` ile çalışır, ekstra case yok. Native'de
   eager kalırlar. Artık CLI ajanı gördüğü core-memory bloğunu **düzenleyebilir** ve
   geçmişte derin arama yapabilir. (Stale `bridgeExcluded` run_subagent yorumu da
   düzeltildi.) Dokümanlar: `11-INTERACTION-MCP`, `26-MEMGPT`, `27-CROSS-SESSION`,
   `19-LAZY`, `09-SDK`, `25-SUBAGENT`, `06-WORKSPACES`.

## CG-16 — Oturumlar-arası tam-metin arama ✅ TAM (çekirdek+araç+API+UI, 2026-06-22)

**Hedef:** Workspace'in tüm oturum mesaj geçmişinde anahtar-kelime araması (bugün
yalnız başlık+summary üzerinden farkındalık vardı). Plan: `_Docs/27-CROSS-SESSION-SEARCH.md`.

**Kilit karar (RG-6 disiplininin ürünü):** Oturumlar boot'ta `d.messages`'a (RAM)
yükleniyor → aranacak veri zaten bellekte. Varsaymak yerine depolama modelini
okuyup **ripgrep/FTS5'i eledim**; saf-Go tarama hem en basit hem yeterli.

Yapılan:

- **`db.SearchMessages`** (`internal/db/store_search.go`): saf-Go RAM-içi tarama,
  boşlukla bölünmüş terimler AND-eşleşir (case-insensitive), `score = matchCount +
  recency` (C5 felsefesi), rune-sınırlı Türkçe-güvenli snippet. `SearchHit`/`SearchOpts`.
- **`conversation_search` aracı (N5)** (`tools/builtin_conversation_search.go`):
  `ListSessionsTool` deseni; `toolsetup.go`'da `SessionContextEnabled()` gate'i.
  `exclude_current` plandan düşürüldü (tools→agent import döngüsü); API'de `exclude`
  query param'ı karşılıyor.
- **API** `GET /api/sessions/search?q=&limit=&role=&exclude=`
  (`api/sessions_search.go` + `server.go` route, `active` yanında).
- **Frontend contract:** `SearchHit` tipi + `sessionApi.searchMessages(...)`.
- **Doğrulama:** `go build ./...` + db/tools/api/agent testleri **200** yeşil;
  `tsc --noEmit` temiz. Yeni testler: `store_search_test.go`,
  `builtin_conversation_search_test.go`.
- **Görsel arama ✅ (aynı gün):** `SessionsSidebar` arama kutusu çift işlevli —
  başlık filtresi + `api.searchMessages` mesaj araması (≥2 char, 250ms debounce,
  `cancelled` guard). "Mesajlarda (N)" bölümü rol-rozeti+snippet+yaş; tıkla →
  `onSelectSession(sessionId, messageId)`. `selectSession` opsiyonel `messageId` →
  `scrollToMsgId` → `MessageList`: her satır `data-msg-id`, hedefe `scrollIntoView`
  (center) + 1.6sn accent-ring flash, sonra `onHighlightConsumed`. `tsc` + `npm run
  build` + `go build` yeşil. **CG-16 tam kapandı.**

---

## RG-6 — Implement-öncesi keşif guard'ı ✅ (2026-06-22)

**Hedef:** Ajanın var olan bir özelliği "yok" sanıp sıfırdan yeniden yazma (veya
çalışan bir uygulamanın üzerine yazma) riskini önle. Faz R oturum-analizindeki en
büyük yanlış kararın mekanizma karşılığı; kod değil, skill/system-prompt düzeyi.

Yapılan:

- **`swarmgo-guide` SKILL.md → "Before you build: discover first" bölümü:** uygulamadan
  ya da "bu yok" demeden önce **search → read → confirm → extend** disiplini;
  absence iddiası ancak gerçekten arandıktan sonra ("Y ve Z için grepledim, bulamadım"),
  ve sıfırdan yazmak yerine mevcudu genişletme kuralı. Kod/konfig/agent/flow/skill/
  memory — hepsine uygulanır.
- **`swarmgo-self-management` → "Prefer reading first" güçlendirildi:** entity
  (agent/flow/skill/schedule/hook/MCP) oluşturmadan önce mevcudu kontrol et,
  duplicate yerine genişlet; guide bölümüne çapraz-referans.
- **Doğrulama:** `go build ./...` ✅ + `go test ./internal/skills/...` (14 test) yeşil.
  Skill testleri yalnız seed varlığı/non-overwrite kontrol ediyor; içerik değişimi
  güvenli.

---

## MemGPT/Letta Tarzı Self-Editing Bellek — Parça 1–3 ✅ (2026-06-22)

**Hedef:** Letta'yı (Docker+Postgres+Python) koşmadan, fikirlerini native Go'da:
bağlam-basıncı sinyali + ajanın in-place düzenlediği kalıcı **çekirdek bellek**.
Tek binary / offline / dosya-tabanlı kimliği korunur, migration yok. Plan +
sapmalar: `_Docs/31-MEMGPT-CORE-MEMORY.md`.

Yapılan (3 parça):

- **(1) Bağlam-basıncı sinyali:** `conversation.Prepared.Pressure`
  (`ContextTokens/maxTokens`, `maxTokens<=0 → 0`); `composeTurnRequest` eşik
  (`memoryPressureWarn`, vars. 0.75) aşılınca `SystemDynamic` başına "önemliyi
  şimdi yaz" uyarısı koyar — sessiz compaction'dan *önce*. Sıfır yeni model çağrısı.
- **(2) `core` bellek türü + Store API:** `db.MemoryCore` + `db.UpsertKnowledgeByKind`
  (ajan başına tek satır, ID korunur); `memory.Store.WriteCore/ReadCore/AppendCore`.
  Recall artık `recallKinds` allowlist'iyle core'u hariç tutar (her turda zaten
  sabit enjekte edildiğinden tekrar çıkmaz).
- **(3) `core_memory_*` araçları:** `tools.CoreMemoryTool` (replace/append) — eager,
  `coreMemoryTools` ayarıyla (vars. açık) gated; core bloğu her turda recall'ın
  üstünde `SystemDynamic`'e enjekte edilir. Letta'nın "core memory always in
  context" davranışı.
- **Ayar/UI:** `memoryPressureWarn` + `coreMemoryTools` → Settings/DTO/Patch/
  Tunables + `applySettings` canlı push; frontend ContextPanel "Çekirdek bellek
  (MemGPT)" bölümü.
- **Doğrulama:** `go build ./...` + ilgili paket testleri (conversation/memory/
  tools/api/agent/db/settings) yeşil; frontend `tsc --noEmit` temiz.
- **UI/API genişletmesi (ikinci tur):** Ayarlarda basınç eşiği **sürgü** + canlı
  yüzde + tetikleme-token'ı + "kapalı" durumu + canlı durum kartı (yeni `Slider`
  primitifi). Hafıza panelinde **çekirdek bellek kartı** (`CoreMemoryCard` —
  göster/düzenle) + `GET|PUT /api/agents/{id}/core` uç noktaları.
- **Parça 4a — persona/human ayrımı ✅ (2026-06-22):** çekirdek bellek iki bağımsız
  bölüme ayrıldı — persona + human, her biri ajan başına tek satır.
  > ⚠️ **2026-06-23'te Parça 5 ile geçersiz kılındı:** `core_persona`/`core_human`,
  > `section` parametresi, `ReadCoreSections` ve `{persona,human}` API'si artık
  > yok → `core:<label>`, `label`, `ReadCoreBlocks`, `{blocks}`. Bu girişin üstündeki
  > **"adlandırılmış bloklar"** ve **"HA-1"** girişlerine bak.
- **Devamı:** Parça 5 (adlandırılmış bloklar + limit) ve Parça 4b (HA-1
  oto-modelleme) **2026-06-23'te tamamlandı** — dosyanın başındaki güncel girişler.

## Oturum-başına Çalışma Dizini (cwd) + otonomi frenleri ✅ (2026-06-22)

**Hedef:** the external agent project'taki "working directory" mekaniği — her oturumun, ajanın
dosya/kabuk araçlarının çalışacağı bir cwd'si olsun; UI'dan değiştirilebilsin;
bağlama enjekte edilsin; ajan git ile repo editleyebilsin. Kilitsiz fs/shell'in
otonom yolda güvenli kalması için frenler.

Yapılan (4 adım uçtan uca):

- **(1) cwd çözümleme:** `Session.WorkingDir` (db) + `SetSessionWorkingDir`;
  `agent/workdir_ctx.go` (`effectiveWorkDir` + ctx taşıyıcı); `completeTraced`
  her turda çözüp `req.WorkDir` + ctx'e koyar; `buildRegistry` sandbox kökünü
  ctx'ten alır. Chat yollarına `WithSessionID` eklendi.
- **(2) Bağlam enjeksiyonu:** `api/workdir_context.go` — "Working directory"
  bloğu (cwd + git branch + CLAUDE.md ipucu) dinamik bağlama (chat_turn.go).
- **(3) Otonom fren:** `autonomousConfine` ayarı (varsayılan açık) — otonom
  turlar `NewConfinedSandbox` ile çalışma dizinine kilitlenir; `shell` confined
  modda `git push`'u reddeder (`isNetworkMutatingGit`).
- **(4) Worktree izolasyonu:** `gitWorktreeIsolation` ayarı (varsayılan kapalı) —
  `agent/worktree.go` otonom oturuma `<workspace>/worktrees/<sid>` worktree+dal
  verir; oturum silinince `RemoveSessionWorktree` temizler.
- **API:** `GET/PUT /api/sessions/{id}/workdir`, `GET /api/fs/browse` (`api/workdir.go`).
- **Frontend:** `chat/WorkDirBadge.tsx` (klasör rozeti + dizin gezgini); Ayarlar ▸
  iki yeni toggle; types/api alanları.
- **Doğrulama:** `go build ./...` + `go test ./...` + frontend `tsc -b` + `npm run
  build` yeşil. Detay: `_Docs/26-CALISMA-DIZINI.md`.

### Workspace varsayılan çalışma dizini + canlı doğrulama (2026-06-22)
- `WSSettings.DefaultWorkingDir` (ws-settings.json) + Runtime `SetDefaultWorkDir`/
  `WorkspaceDefaultDir`; `effectiveWorkDir` artık oturum → workspace-default →
  fiziksel workDir sırasıyla çözüyor. Ayarlar ▸ Bu Workspace ▸ "Varsayılan çalışma
  dizini" alanı (`WorkspacePanel.tsx`, `workspace_settings.go` DTO/patch).
- **Canlı duman testi (8088):** `GET /api/fs/browse` ✓; bir oturuma SwarmGo deposu
  set edildi → `{exists:true,isGitRepo:true,branch:"main"}` ✓ (git branch tespiti),
  sonra sıfırlandı. **Not:** varsayılan 8080 portu mcp-for-unity backend'iyle
  çakıştığı için bu örnek `SWARMGO_ADDR=127.0.0.1:8088` ile çalışıyor.

## fs/shell sandbox kilidi kaldırıldı (kilitsiz dosya/komut erişimi) ✅ (2026-06-22)

**Hedef:** Built-in dosya/komut araçlarının workspace dizinine kilitli olma
güvenlik kısıtını kaldırmak — the external agent project (external-agent-oss) gibi araçların makinedeki
herhangi bir yola erişebilmesi, güvenliği tek başına **izin moduna** bırakmak.

Yapılan:


---

> **Daha eski kayitlar (2026-06-19 ve oncesi) arsivlendi:** [05-ARSIV.md](05-ARSIV.md).
