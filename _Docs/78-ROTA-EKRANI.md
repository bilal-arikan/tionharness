# Rota Ekranı — F0 (projeksiyon) + F1a (rota varlığı) + F1b (görünürlük) + F2 (otomasyonlar grafikte) + F3 (metrik + küratör)

**Durum:** F0 uygulandı (2026-09-02); F1a (rota varlığı + ilan, backend)
uygulandı (2026-09-02, §5); F1b (rota-içi faz görünümü + backend'in her
yeniliğinin UI karşılığı) uygulandı (2026-09-02, §6). Altyapı planı:
`77-ROTA-ALTYAPI-PLANI.md` (R1–R10). Tasarım brifi: oturum artefaktı "Rota
Tasarım Brifi" (§8 ekran, §11 fazlar). F2 (otomasyonlar grafikte, "neden
ateşlenmedi") uygulandı (2026-09-02, §7); F3 (deterministik metrik + reçete
istatistikleri + LLM'siz küratör + pin) uygulandı (2026-09-02, §8). Sonraki
fazlar F4 (optimizer), F5 (kapılar / kanvastan müdahale).

## 1. Ne gösterir

NavRail **Rota** girişi (`#/w/WS/rota`). Kök yakınlaştırma düzeyi **workspace**:
son etkinliği pencere içinde kalan her oturum bir **şerit**, zaman soldan sağa.

- **Kök oturum** grubun üst şeridi; worker / handoff / otomasyon oturumları
  altında içe girintili şeritlerdir (`Session.origin.rootSessionId`).
- **Çubuk** = oturumun ömrü (`createdAt → updatedAt`; canlıysa "şimdi"ye kadar
  uzar ve ucunda yanıp sönen nokta). Renk: canlı = vurgu, `completed` = yeşil,
  `failed/killed/timeout` = kırmızı, arşiv = soluk.
- **Kenarlar:** `spawned` (turuncu, koordinatör → worker, worker'ın
  `createdAt`'inde), `reported` (mor kesik, tamamlanmış worker → koordinatör,
  `updatedAt`'te), `forked_from` (mor, handoff/spawn/otomasyon → yeni oturum).
- **Akış koşuları** başlatan oturumun şeridinde ince alt çubuk.
- **İşaretler:** ⚡ ateşlenen otomasyon (tetikleyen oturumun şeridinde), ↷
  atlanan (sebep tooltip'te), ✕ koordinatör stall halt.
- **Gelecek şeridi:** "şimdi" çizgisinin sağında kurulu zamanlayıcılar (⏰,
  `ws:schedule_armed`), ufuk 30 dk; ötesi kenara kırpılır.
- **Üst çubuk:** bağlantı noktası (yeşil = akış bağlı), sayaçlar, spawn/kuyruk
  kapasitesi (`/api/workspace/liveness`), boşta-şerit penceresi (1 sa / 6 sa /
  24 sa / tümü), `#seq · r<revizyon>`.
- **Sağ panel:** seçim yoksa tetik / koordinasyon / kurulu zamanlayıcı listesi;
  seçim varsa `ViewPanel` (get_view card: `session` veya `flowrun`).

Etkileşim: tık = seç, çift tık = oturumu Sohbet'te / akış koşusunu Akışlar'da
aç, ↑↓ şeritler arasında gez, Enter aç, Esc seçimi bırak (Harita sözleşmesi).

## 2. Veri yolu

```
GET /api/sessions (son 200, updated_desc)      ┐
GET /api/workspace/liveness                     ├─ seed ─► laneStore (LaneState)
GET /api/trajectories (indeks satırları)        ┘              ▲
GET /api/workspace/stream (ws:*)  ─── applyLaneEvent ──────────┘
                                                               │
                              layoutRota(state, {now, cutoff}) ▼
                                   RotaCanvas (SVG) ── ViewPanel / RotaActivity
```

- `shared/lib/laneModel.ts` — `LaneState` (Map'ler + halkalar, `revision`,
  `head`, `connected`, `stale`), `rootLanes`, `laneMembers` (oluşturma sırası),
  `trajectoryForRoot`.
- `shared/lib/laneReducer.ts` — saf ve immutable. `seedSessions` /
  `seedLiveness` / `seedTrajectories`; `applyLaneEvent` stale reddi
  (`updatedAt`, rota `revision`), reddedilen olay aynı nesneyi döndürür.
- `shared/lib/laneStore.ts` — `useSyncExternalStore`, ref-sayımlı
  `connectLanes`, `seedLanes`, `resetLanes`.
- `features/rota/rotaLayout.ts` — deterministik yerleşim, saniye cinsinden;
  satır sırası: kökler son etkinliğe göre (yeni üstte), üyeler `createdAt`'e
  göre. Canlı şerit hiçbir pencerede düşmez.
- `features/rota/RotaCanvas.tsx` — düz SVG (React Flow değil; sabit zaman ×
  şerit ızgarası, sürükleme yok, her olayda ucuz yeniden çizim). `x(t)`: geçmiş
  penceresi konteyner genişliğine sığar, gelecek şeridi sabit 170 px.
- `features/rota/RotaPanel.tsx` — akış → seed → layout → kanvas; `useServerNow`
  5 sn'de bir "şimdi"yi ilerletir (sunucu saati).

Backend (F0'da eklenen): `GET /api/trajectories` (indeks; `root`, `template`,
`status`, `terminal`, `limit`) ve `GET /api/trajectories/{id}` (tam graf, 404)
— `internal/api/trajectories.go`. `ws:session_lifecycle` payload'ına
`createdAt` eklendi (akıştan ilk duyulan oturum REST turu olmadan yerleşir).
`view.KindTrajectory` frontend `ViewKind`'a eklendi; Harita ikon haritasında
`trajectory: Waypoints`.

## 3. F0'ın bilinçli sınırları

- ~~**Rota varlığı henüz üretilmiyor.**~~ F1a ile kapandı (§5).
- ~~**Faz sütunu yok, yalnız zaman.**~~ F1b ile kapandı (§6): workspace kökü
  zaman eksenli kalır, ◈ tıklanınca rota-içi faz-sütunlu görünüm açılır.
  Worker alt-ağacı katlama hâlâ yok (F2).
- **Şerit patlaması** (12 worker = 12 şerit) için demet katlama (`+N`) yok;
  pencere süzgeci ve son-200 seed'i şimdilik yeterli.
- **Maliyet / süre alt şeridi, mini rota (sohbet başlığı), RunView gömme** F3+.
- Mobilde sağ panel gizli (`md:` altı), kanvas yatay kaydırır.

## 4. Test

- `frontend/src/features/rota/rotaLayout.test.ts` — satır sırası ve derinlik,
  spawn/report/fork kenar zamanları, canlı/biten çubuklar, işaret şeridi, gelecek
  ufku kırpma, pencere kapalıyken tümü, boş depo.
- `frontend/src/shared/lib/laneReducer.test.ts` — seed, stale reddi, rota
  revizyonu (`seedTrajectories`), liveness, halkalar, `createdAt` düşüşü.
- `internal/api/trajectories_test.go` — liste, süzgeç (boş dizi), tekil, 404.
- F1a: `internal/agent/trajectory_graph_test.go` (reçeteden tohum, faz
  geçişleri + durum türetme, plan kuralları, metin render),
  `trajectory_binder_test.go` (yaratılışta tohum, spawn/rapor bağlama +
  durum bloğu + arşivde terk, araçla plan/faz/bitir + kayıt kapısı, Ask kapısı,
  akış koşusu, otomasyon çatalı), `internal/tools/builtin_trajectory_test.go`.

## 5. F1a — Rota varlığı + ilan (backend, 2026-09-02)

F0 salt projeksiyondu; F1a **`Trajectory` varlığını üreten ve dolduran** tarafı
ekler. Frontend'e dokunulmadı (yalnız `TrajectoryNode.optional` tipi); F0
kanvası aynı `ws:trajectory` + `GET /api/trajectories` yolundan ◈/revizyonu
zaten gösterir. Kod: `internal/agent/trajectory_graph.go` (saf graf
yardımcıları), `trajectory_binder.go` (gözlemci + oturum/akış/Ask bağlama),
`trajectory_funcs.go` (araç işlevleri + durum bloğu),
`internal/tools/builtin_trajectory.go` (araç tanımı).

**Kim yaratır.** Kök oturum başına bir rota (`EnsureTrajectory`):

- Kök koordinatör **reçeteyle** yaratılırsa (`CoordinatorWorkflow` dolu —
  oturum paneli, `RunCoordinatorNode`) oturum hook'unda (`OnSessionChange`
  create) hemen tohumlanır: reçetenin `phases:` bloğu `p:<id>` **declared**
  düğümleri (profil, kapı, `optional`), ardışık `next` kenarları, faz/rota
  watcher'ları ghost `a:<watcher>[@faz]` düğümleri, `optimizer` → `o:optimizer`
  ghost; `TemplateRef` = `slug@version`. Prose-only/bozuk reçete → yalnız kök
  düğüm + `TemplateRef`.
- Reçetesiz koordinatör: ilk `spawn_worker`'da (gözlemci `OnSpawn`) kök düğümle
  yaratılır; `trajectory{plan}` çağrısı da yaratır.
- Var olan bir köke sonradan reçete seçilirse (`PUT …/role` workflow) faz yoksa
  reçetenin fazları eklenir (`AdoptTrajectoryRecipe`).

**Ne gözlemlenir** (tümü `observed`, `UpdateTrajectory(expectedRev=0)` ile
ekleme; başarısızlık loglanır, işi asla düşürmez). R7 hook'ları koordinasyon
goroutine'inde koşar; bağlayıcı yazımları **sıralı, yol-dışı bir kuyruğa**
alınır (`trajectory_queue.go`: tembel başlayan tek drainer, varış sırası
korunur → aynı worker için `spawned` her zaman `reported`'dan önce). Aksi
hâlde spawn yolundaki 3-4 atomik dosya yazımı `spawn_worker` döner dönmez
worker'ın iptal edilebilir olması garantisini (`worker_stop_race_test`)
bozuyordu. Testlerde `drainSpawns` → `backgroundWorkPending` bu kuyruğu da
bekler.

| Kaynak | Düğüm / kenar | Durum |
|--------|---------------|-------|
| `OnSpawn` (R7) | `s:<worker>` (şerit `Meta.lanes` sayacından, `PhaseID` = aktif faz), `s:<coord> → s:<worker>` `spawned` | aktif; kuyruktaysa `pending` + sebep. Faz varsa ama hiçbiri aktif değilse **ilk pending faz otomatik aktif** olur |
| `OnReport` (R7) | `s:<worker> → s:<coord>` `reported` | `<status>` `completed` → done; failed/killed/timeout/incomplete → failed + `Reason` |
| `OnStall` (R7) | koordinatör düğümü `Reason: stall halt: …` | — |
| `emitFlowRunEvent` (tek yayın noktası) | transkript oturumunun `TriggerSessionID`'si ağaç üyesiyse `r:<run>` + `spawned` | running/waiting → active, success → done, failure → failed + hata; yeniden yayın düğümü çoğaltmaz |
| Oturum hook'u create | tetikleyicisi ağaç üyesi olan **yeni kök** oturum: `s:<id>` + `fired` (otomasyon) / `forked_from` (handoff, spawn_session …); akış transkripti atlanır (koşu düğümü onu temsil eder) | aktif |
| `persistAskSuspend` / `ReleaseAsk` | `g:<askID>` insan kapısı (`Gate{human}`), `s:<asker> → g:` `blocked_by` | açık → rota **waiting**; yanıt → done, timeout/iptal → skipped + sebep. `ReleaseAsk` üç claimer'dan çağrılır: `ResumeAsk`, sweeper, API `answerDurableAsk` |
| Oturum hook'u state | kök arşivlenince kök düğüm done; fazlar kapanmamışsa rota **abandoned**, faz yoksa done | — |

Yan değişiklik: `flowSessionOrigin` artık `TriggerSessionID`'yi, akış dışından
`run_flow` çağıran oturumdan da doldurur (yalnız tetikleyici; transkript
oturumu ayrı kalır).

**Durum türetme** (`trajDeriveStatus`): açık kapı → waiting; faz varsa: failed
faz → failed, aktif faz → running, zorunlu fazların tümü done/skipped → done,
aksi planned; faz yoksa: kök dışında aktif düğüm → running, tek kök → planned.
Terminal durumlar (done/failed/abandoned) türetmeyle geri açılmaz.

**`trajectory` aracı** (koordinasyon yüzeyinin parçası: oturum-kapılı,
allowlist-bağımsız; native kayıt + CLI köprüsü):

- `get` — metin render (fazlar glyph'li: ○ pending ● active ✓ done ✗ failed
  ↷ skipped; her fazın altındaki worker/koşular; açık kapılar). Her koordinatör.
- `plan {phases:[{id,label?,profile?,optional?,gate?{kind,value}}]}` — fazları
  ilan eder/yeniden planlar; reçete `id` kuralı (`skills.ValidPhaseID`); aktif
  veya bitmiş faz plandan **düşürülemez**, pending olanlar düşer, yeni eklenir,
  `next` zinciri yeniden kurulur. Yalnız **kök** koordinatör.
- `phase {id, state: active|done|skipped|failed, reason?}` — `active` önceki
  aktif fazı done kapatır. Yalnız kök.
- `finish {status: done|failed, reason?}` — aktif fazları ve kök düğümü kapatır,
  terminal durum yazar. Yalnız kök.

**Durum bloğu.** `coordinatorSituationBlock` filo bloğundan sonra `<trajectory>`
bölümü basar: rota id/durum/revizyon/reçete, faz satırı, aktif fazın altındaki
worker sayıları (running/done/failed), kök koordinatöre "faza geçince
`trajectory{phase}`, iş bitince `trajectory{finish}`" hatırlatması; faz yoksa
bir kerelik `plan` önerisi; açık kapı uyarısı. Ağaçta rota yoksa bölüm boş
(prompt-cache'e dokunmaz: dinamik son ek).

**Bilinçli sınırlar (F1b/F2'ye).** Faz-sütunlu rota görünümü, derin bağlantı
`#/w/WS/rota/RTA12`, sohbet başlığında mini rota; ghost watcher'ların gerçek
otomasyonlara çözülmesi ve ateşlenmesi/"neden ateşlenmedi" (F2); faz kapısının
(artifact/verdict) otomatik doğrulanması (F5); `todo_write` ilerlemesinin faza
yansıması; UI'dan düzenleme için `PUT /api/trajectories/{id}` (CAS) yok.

## 6. F1b — Görünürlük dalgası (2026-09-02)

Amaç: F0–F1a'ya kadar backend'e giren her yeniliğin ekranda bir karşılığı
olsun; kullanıcı uygulamada olan biteni hangi ekranda olursa olsun görsün.
Dal `rota/f1b-visibility`. Keşif özeti (öncesi): rota grafı hiç çekilmiyordu
(`getTrajectory` ölü kod), `#/w/WS/rota/RTA` ayrıştırılıp atılıyordu,
`VIEW_KINDS`'ta `trajectory` eksikti (`parseRef` null), sohbet başlığında
koordinatör/rota bilgisi yoktu, oturum kökeni hiçbir yerde görünmüyordu,
beceri ekranı reçete fazlarını/hatasını göstermiyordu, ateşleme defteri
API'de vardı UI'da yoktu, `ws:*` olayları Rota dışında hiçbir bildirime
dönüşmüyordu.

| Backend yeniliği | Ekran karşılığı (F1b) | Dosya |
|------------------|------------------------|-------|
| Rota grafı (R4/F1a) | **Rota-içi görünüm**: ◈ (şerit etiketi), sağ paneldeki "◈ Rotayı aç" ve "Rotalar" listesi → faz sütunları × şeritler; kök oturum sütunlar boyunca bant, worker/koşu/kapı düğümleri hücrelerde, hayaletler kesik çizgi; `spawned`/`reported`/`fired`/`blocked_by`/`forked_from` kenarları; aktif sütun vurgulu; başlıkta durum rozeti, rev, `n/m faz`, hayalet sayısı, "kök sohbet"; tek tık → sağ panel (`ViewPanel`: oturum/koşu/otomasyon/rota projeksiyonu), çift tık → sohbet/RunView, Esc → geri | `features/rota/trajectoryLayout.ts` (saf), `RotaTrajectoryView.tsx`, `useTrajectory.ts` (id **veya** kök oturumla; `ws:trajectory` revizyonu → `GET /api/trajectories/{id}` yeniden okuma; `connectLanes` ref-sayımlı) |
| Derin bağlantı | `#/w/WS/rota/RTA12` yazılır/okunur (`routeIdForView` `rota`, `useDeepLinks.rotaTrajectory` + `openTrajectory`, `useAppNavigation`) | `app/url.ts`, `useDeepLinks.ts`, `useAppNavigation.ts`, `App.tsx` |
| `view.KindTrajectory` (R9) | `VIEW_KINDS`'a `trajectory` eklendi → `parseRef('trajectory:RTA1')` çalışır; Harita derin bağlantısı kurtulur | `types/view.ts` |
| Koordinasyon ağacı (M2) + rota | **Sohbet başlığı**: rol rozeti (`coordinationLabel`: Koordinatör / Alt-koordinatör / Worker) + **mini rota şeridi** (`plan ✓ · kod ● · inceleme ○`, durum rozeti, `n/m`, worker'ın bağlı olduğu faz ◂) — tık → Rota ekranı yakınlaşmış | `features/rota/RotaStrip.tsx`, `app/AppHeader.tsx` |
| `SessionOrigin` (R1) | **Oturum bilgisi** panelinde köken çipi: "⇢ Otomasyon AUT4 tetikledi / Zamanlayıcı SCH2 başlattı / Akış FLW1 · koşu RUN7 · düğüm n2 / Koordinatör SES9 açtı / Devir · SES3 …"; tetikleyici oturum varsa tıkla-git. Backend: `SessionInfo.origin` alanı eklendi (`session_info.go`, `Lineage()` türetilmiş; `user` için boş) | `shared/lib/sessionOrigin.ts` (+test), `features/sessions/SessionTitleBlock.tsx` |
| Reçete şeması (R6) | **Beceriler** ekranı: listede `reçete · N faz` / `⚠ reçete` çipi; detay başlığında `vN`, desen, `◈ plan → kod → inceleme` (tooltip: profil/kapı/isteğe bağlı), `⚡ izleyiciler`, `✦ optimizer`, geçersiz blokta hata metni | `features/skills/RecipeChips.tsx`, `SkillsPanel.tsx` |
| Ateşleme defteri (R5) | Otomasyon kartında **"Ateşlemeler"** açılır listesi: her deneme ⚡/↷/✕, zaman, tetik türü, atlama sebebi (Türkçe), açılan oturum, iterasyon; `lastFiredAt` değişince yenilenir | `features/schedules/AutomationFires.tsx`, `fireMeta.ts`, `AutomationCard.tsx` |
| `ws:*` akışı (R3/R7) | **Uygulama geneli bildirim köprüsü**: aktif workspace'in şerit deposu her ekranda açık (`connectLanes`, tek SSE paylaşımlı); yeni ateşleme (`cooldown` atlamaları hariç), stall halt, rota başladı / bitti / başarısız / terk / insan bekliyor, akış koşusu başarısızlığı → toast. Ayarlar ▸ Bildirimler'de `automation`/`coordination`/`flow` anahtarları ve yeni **Rota** türü ile susturulur | `app/workspaceSignals.ts` (saf `diffSignals`, +test), `app/useWorkspaceSignals.ts`, `shared/lib/notifyTypes.ts` |
| Rota listesi | Rota ekranı sağ şeridi "Rotalar": kök başlığı · reçete · durum rozeti, tık → yakınlaş | `features/rota/RotaActivity.tsx` |

Kalanlar (F2+): faz düğümüne tıklayınca todo listesi (faz ↔ todo bağı yok),
hayalet otomasyona "neden ateşlenmedi" kartı, worker alt-ağacı katlama,
RunView içinde koordinatör düğümünden rota açma, maliyet/süre alt şeridi,
kanvastan müdahale (faz ekle/atla, buradan çatalla).

Testler: `features/rota/trajectoryLayout.test.ts` (sütun/hücre yerleşimi,
fazsız graf, özet/ilerleme), `shared/lib/sessionOrigin.test.ts`,
`app/workspaceSignals.test.ts` (yeni ateşleme/cooldown süzgeci, rota
başlangıç + terminal geçişler + susturma, stall/akış hatası). Doğrulama:
`tsc`, `vitest` 100 dosya / 703 test, `vite build`, prettier; eslint'teki 13
hata önceden vardı (main ile aynı sayı, dokunulan dosyalarda değil).

## 7. F2 — Otomasyonlar grafikte (2026-09-02)

Dal `rota/f2-automations-on-graph`. F1a'da reçetenin `watchers:` listesi
grafa **hayalet** `a:<izleyici>[@faz]` düğümleri olarak düşüyordu ama hiçbir
şey onları ateşlemiyordu. F2 bunu ve iki yeni tetik türünü ekler.

**Yeni tetik türleri** (`internal/db/automation_trigger_traj.go`, R5
registry'sine iki `RegisterTrigger`):

| Tür | Ne zaman | Süzgeçler (boş = hepsi) |
|-----|----------|-------------------------|
| `phase` (Rota fazı) | ilan edilmiş bir faz **bitince** (`exit`: done/skipped/failed, varsayılan) ya da **başlayınca** (`enter`: active) | `trajPhase` (faz id), `trajEvent`, `trajRecipe` (reçete slug'ı, sürümsüz) |
| `trajectory_end` (Rota sonu) | rota terminal duruma gelince | `trajStatus` (done/failed/abandoned), `trajRecipe` |

Her ikisi de otomasyon modelinde `trajPhase/trajRecipe/trajEvent/trajStatus`
alanlarını kullanır; API create/update bunları kabul eder; `SessionMode`
varsayılanı spawn (kendi kendini döngülemez, `spawnTags` nil = etiket yok).
Prompt değişkenleri: `{{trajectoryId}} {{rootSessionId}} {{sessionId}}
{{recipe}} {{phase}} {{phaseState}} {{event}} {{status}} {{phases}}` + ortak
olanlar. Üretilen oturumun `Origin.TriggerSessionID` = kök oturum, yani F1a
bağlayıcısı onu `fired` kenarıyla grafa asar.

**Geçiş algılama** (`internal/agent/trajectory_transitions.go`). Rota hook'u
yalnız sonraki anlık görüntüyü verir; `trajTransitionDiffer` rota başına faz
durumlarını ve statüyü hatırlar, ardışık görüntülerden `enter`/`exit`/`end`
geçişleri üretir ve bunları `Runtime.SetTrajectoryTransitionHook` ile bağlı
motora **rota iş kuyruğunda** (sıralı, yol-dışı) iletir. Boot'ta hafıza
boştur: var olan bir rotanın ilk görüntüsü taban çizgisidir (süreç kapalıyken
biten faz yeniden duyurulmaz — bilinçli).

**Ateşleme** (`internal/agent/automation_trajectory.go`,
`AutomationEngine.OnTrajectoryTransition`):

1. **Açık kurallar**: etkin + arşivsiz `phase`/`trajectory_end` otomasyonları
   süzgeçleriyle eşleşince ateşlenir.
2. **Reçete izleyicileri**: graftaki hayalet düğümler — faz çıkışında
   `PhaseID`'si o faz olanlar, rota sonunda rota-geneli olanlar — `RefID`
   (otomasyon id **veya** adı, büyük/küçük harf duyarsız) ile çözülür ve
   süzgeçsiz ateşlenir. Aynı geçişte hem kuralla hem izleyiciyle eşleşen
   otomasyon **bir kez** ateşlenir; izleyici düğümü o sonuca bağlanır.

Her deneme grafa yazılır (**"neden ateşlenmedi"**): düğüm `done` + `a:… →
s:<yeni oturum>` `fired` kenarı; ya da `skipped` + sebep (`cooldown`,
`disabled`, `archived`, `max_iterations`, `not_found` = izleyici adı hiçbir
otomasyonu bulmadı …); ya da `failed` + hata (hedef ajan yok, boş prompt).
Açık kural için düğüm yoksa `a:<id>[@faz]` gözlemlenmiş düğümü oluşturulur.
Defter (`/fires`) ve `ws:automation_fire` aynı `notifyFired`/`recordSkip`/
`recordFailure` yollarından beslenir; kart defteri ile graf birbirini tutar.

**Ekran karşılıkları.** Otomasyon panosunda iki yeni şerit ("Rota fazı",
"Rota sonu") ve modalda `TrajectoryTriggerFields` (faz, olay/bitiş, reçete);
kartta `◈ faz code · bitince · plan-dev` / `⚑ rota sonu · başarısız` çipi;
rota-içi görünümde otomasyon düğümü hücresinde `docs · bekleme süresi`,
`nobody · otomasyon bulunamadı` gibi sebep metni (`fireMeta.SKIP_REASON_LABEL`),
hayaletlerde `· bekliyor`; ateşlenen düğümden yeni oturuma `fired` kenarı.

**Testler.** `trajectory_transitions_test.go` (taban çizgisi, enter/exit,
tek bitiş, silme, boot), `automation_trajectory_test.go` (kural süzgeçleri,
prompt değişkenleri, gerçek depo üzerinde faz çıkışı → izleyici + açık kural
düğümleri failed+sebep, defter kaydı, rota sonu → not_found / disabled),
`db/automation_core_test.go` (yeni türlerin doğrulaması).

**Kalanlar.** Optimizer düğümü hâlâ hayalet (F4); `phase` kuralı için faz
kimliği seçici (reçete fazlarından açılır liste) yok — metin alanı; hayalet
düğüme tıklayınca kural kartı (ViewPanel `automation` projeksiyonu zaten
açılıyor, ama `not_found` için bir "otomasyon oluştur" kısayolu yok).

## 8. F3 — Deterministik metrik + küratör (2026-09-02)

Dal `rota/f3-metrics-curator`. Brif §7.1, §7.4–7.5: her koşunun LLM'siz
özeti, reçete başına istatistik ve "ekleme değil budama" yapan haftalık
küratör. Optimizer (F4) hâlâ yok; `o:optimizer` düğümü hayalet.

**Rota özeti** (`db.TrajectorySummary`, `Trajectory.Summary` + indeks
satırında kopyası; `internal/agent/trajectory_summary.go`, saf
`summarizeTrajectory`). Rota terminal duruma gelince rota iş kuyruğunda,
`trajectory_end` kurallarından **önce** yazılır (kural prompt'u ve küratör
son sayıları görür); `POST /api/trajectories/{id}/summarize` canlı bir rota
için de yeniden hesaplar. Alanlar: süre (kök oturum açılışı → son düğüm
bitişi / şimdi), token + maliyet (bağlı oturumların `SessionUsage` rollup'ı,
`billing.RollupOf`; fiyatsız model varsa `priced=false`), worker sayısı ve
başarısızlar, akış koşuları, faz sayısı / biten, **hayalet fazlar** (ilan
edilip hiç başlamayan), **plansız** (faza bağlanmamış oturum/koşu), ilan
edilen izleyiciler ve **sessiz** kalanlar, kapı sayısı + bekleme süresi, faz
başına worker/başarısızlık/süre. `view.ProjectTrajectory` özet satırı basar
(`özet: 12 dk · 40k token · $0.31 · 3 worker (1 ✗) · sessiz izleyici: docs`).

**Reçete istatistikleri** (`GET /api/trajectories/recipes[?slug=]`,
`agent.RecipeStatsFromIndex`): indeks satırlarından `TemplateRef`
(slug@sürüm) başına terminal koşu sayısı (bitti / başarısız / terk), canlı
sayısı, özetli koşular üzerinden ortalama süre / token / maliyet / worker,
izleyici başına "kaç koşuda ateşlenmedi", faz başına "kaç koşuda başlamadı",
son rota. Yalnız indeks okunur; sidecar açılmaz.

**Küratör** (`internal/agent/curator.go`, `Runtime.RunCurator(trigger,
apply)`). Saat başı kontrol, son geçiş 7 günden eskiyse **ve** workspace
boştaysa (`Liveness` girişleri sıfır) `apply=true` ile koşar; API'den elle
(`POST /api/curator/run?apply=`), rapor `curator/last.json`
(`GET /api/curator/report`). Kurallar, hepsi depodaki durumdan:

| Varlık | Koşul | Ajan yapımı (`CreatedBy` dolu) | Kullanıcı yapımı |
|--------|-------|-------------------------------|------------------|
| otomasyon | iterasyon tavanına ulaştı / son tarihi geçti | **arşivle** | öneri |
| zamanlama | tek seferlik çalıştı (kapalı + LastRunAt) / son tarihi geçti | **arşivle** | öneri |
| hook | 30 gündür hiç tetiklenmedi | öneri | öneri |
| reçete | ≥3 özetli koşunun **hepsinde** ateşlenmeyen izleyici → "reçeteden çöz"; hepsinde başlamayan faz → "optional yap / kaldır" | öneri (kanıt: koşu sayısı + son rota) | öneri |

Değişmezler (brif §7.4): **asla silme, arşivle** (arşiv geri alınabilir);
**provenance kapısı** (yalnız ajan/otomasyon yapımı varlıklara dokunur);
**pin** — `Automation/Schedule/Hook.Pinned` (`POST …/{id}/pin {pinned}`)
her otomatik geçişten muaf. Geçiş sonunda `events.TypeAutomation` bildirimi
("🧹 Küratör geçti — N arşivlendi, M öneri").

**Ekran karşılıkları.** Rota-içi görünümün başlığında özet çipleri (⏱ süre,
token · $, worker (✗), koşu, ⏸ kapı bekleme, plansız, ◌ hayalet fazlar,
⚡ sessiz izleyiciler); Beceriler'de koordinatör reçetesi detayında "Rota
istatistikleri" bloğu (sürüm başına satır, rozetler, ortalamalar, sessiz
izleyici / hayalet faz çipleri, "son: RTA…" → Rota ekranı); Otomasyon
panosunda **Küratör** düğmesi → panel (son geçiş, arşivlendi/öneri sayıları,
eylem listesi, "Kuru çalıştır" / "Şimdi çalıştır"); otomasyon kartında
📌 sabitle/kaldır ve "sabit" çipi.

**Testler.** `trajectory_summary_test.go` (özet alanları, canlı koşu süresi,
reçete rollup'ı), `curator_test.go` (ajan/kullanıcı provenance, pin muafiyeti,
tek seferlik zamanlama, sessiz hook, rapor kaydı + haftalık saat, ≥3 koşu
eşiğiyle reçete önerileri; `db.SetHookCreatedAtForTest` test kancası),
`trajectoryFormat.test.ts`.

**Kalanlar (F4/F5).** LLM optimizer (`recipe-optimizer` sistem ajanı,
`recipe-opt` bulgu kanalı, net büyüme bütçesi, kanıt zorunlu uygulama,
regresyonda geri alma); reçete sürüm geçmişinin seed ledger'ında tutulması
(bugün yalnız `version:` + `TemplateRef`); küratör önerisini tek tıkla
uygulama (arşivle / reçeteyi düzenle) ve zamanlama/hook kartlarında pin
düğmesi (API hazır, UI yalnız otomasyon kartında); faz kapılarının otomatik
doğrulanması.
