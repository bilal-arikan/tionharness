# 78 — Rota: Sistem ve Ekran

> **Durum:** Brifin §11 yol haritası **tamamlandı** (F0–F5, 2026-09-02/03).
> Altyapı planı ve gerçekleşen notları `77-ROTA-ALTYAPI-PLANI.md` (R1–R10).
> Tasarım brifi: oturum artefaktı "Rota Tasarım Brifi" (§8 ekran, §11 fazlar).
> Bu doküman faz faz büyüdü: §1–§4 F0'ın kanvasını anlatır, §5–§10 her fazın
> **ne eklediğini** ve **neyi bilinçli bıraktığını**. Bugünkü davranışı okumak
> için önce aşağıdaki haritaya, sonra ilgili faz bölümüne git.

**Rota nedir.** Bir koordinatör ağacının *ilan edilen* planı (reçetenin
`phases:` bloğu ya da ajanın `trajectory{plan}` çağrısı) ile *gözlenen*
gerçeklerin (worker'lar, akış koşuları, otomasyon ateşlemeleri, insan
kapıları) tek grafta buluşması. Rota hiçbir şeyi **çalıştırmaz**; runtime
gözlemcileri yazar, ekran ve ajan okur. Kök oturum başına bir rota
(`sessions/<root>/trajectory.json` + `trajectories/index.json`).

## 0. Fazlar ve harita

| Faz | Ne getirdi | Bölüm | Kod (başlıca) |
|-----|------------|-------|---------------|
| F0 | Workspace kök zaman-eksenli kanvas, `GET /api/trajectories` | §1–§4 | `frontend/src/features/rota/rotaLayout.ts`, `RotaCanvas.tsx`, `RotaPanel.tsx`; `internal/api/trajectories.go` |
| F1a | Rota varlığının üretilmesi (reçeteden tohum, gözlemci bağlama), `trajectory` aracı, durum bloğu | §5 | `internal/agent/trajectory_graph.go`, `trajectory_binder.go`, `trajectory_queue.go`, `trajectory_funcs.go`; `internal/tools/builtin_trajectory.go` |
| F1b | Rota-içi faz-sütunlu görünüm, derin bağlantı, sohbet başlığı şeridi, köken çipi, reçete çipleri, ateşleme defteri, `ws:*` → toast | §6 | `features/rota/trajectoryLayout.ts`, `RotaTrajectoryView.tsx`, `RotaStrip.tsx`; `app/useWorkspaceSignals.ts` |
| F2 | `phase` / `trajectory_end` tetikleri, reçete izleyicilerinin ateşlenmesi, grafta "neden ateşlenmedi" | §7 | `internal/db/automation_trigger_traj.go`, `internal/agent/trajectory_transitions.go`, `automation_trajectory.go` |
| F3 | Rota bitiş özeti, reçete istatistikleri, LLM'siz küratör, pin | §8 | `internal/agent/trajectory_summary.go`, `trajectory_recipe_stats.go`, `curator.go`; `features/schedules/CuratorPanel.tsx` |
| F4 | `recipe-optimizer` sistem ajanı, `recipe-opt` içgörü kanalı (yalnız öneri) | §9 | `internal/agent/recipe_optimizer.go`, `internal/prompts/defaults/recipe-optimizer.md` |
| F5 | Faz kapıları (artifact / verdict / human = Durable Ask), kanvastan müdahale, buradan çatalla, RunView → Rota; F4-v2 `auto_prune` | §10 | `internal/agent/trajectory_gate.go`, `internal/api/trajectory_actions.go`; `features/rota/PhaseActions.tsx`, `ForkModal.tsx`; `internal/skills/recipe_edit.go` |

**Sözlük.** *Faz* — ilan edilmiş adım (`p:<id>`, declared). *Şerit* — çizim
satırı; kök 0, her worker/koşu yeni şerit. *Hayalet* — ilan edilip henüz
gerçekleşmemiş düğüm (kesik çizgi). *Kapı* — fazın çıkış koşulu. *İzleyici*
— faz/rota sonunda ateşlenecek otomasyon (`watchers:`). *Küratör* — LLM'siz
haftalık budama. *Optimizer* — nadir LLM geçişi, yalnız öneri.

## 1. Workspace kök görünümü (F0) — ne gösterir

NavRail **Rota** girişi (`#/w/WS/rota`). Kök yakınlaştırma düzeyi **workspace**:
son etkinliği pencere içinde kalan her oturum bir **şerit**, zaman soldan sağa.

- **Kök oturum** grubun üst şeridi; worker / handoff / otomasyon oturumları
  altında içe girintili şeritlerdir (`Session.origin.rootSessionId`).
- **Çubuk** = oturumun ömrü (`createdAt → updatedAt`; `updatedAt` **son
  aktivitedir** — başlık/etiket yazımı onu ilerletmez, 2026-09-06; canlıysa "şimdi"ye kadar
  uzar ve ucunda yanıp sönen nokta). Renk: canlı = vurgu, `completed` = yeşil,
  `failed/killed/timeout` = kırmızı, arşiv = soluk.
- **Bekleme aralıkları (2026-09-04):** bir koordinatörün çubuğunda worker'a iş
  verip beklediği aralıklar soluk/çizgili çizilir; çubuk aynı çubuk kalır,
  durum renkleri anlamını korur. Yalnız **spawn edilen** worker'lar için —
  `run_subagent` ayrı oturum yaratmadığından görünmez. Ayrıntı §13.
- **Kenarlar:** `spawned` (turuncu, koordinatör → worker, worker'ın
  `createdAt`'inde), `reported` (mor kesik, tamamlanmış worker → koordinatör,
  `updatedAt`'te), `forked_from` (mor, handoff/spawn/otomasyon → yeni oturum).
- **Akış koşuları** başlatan oturumun şeridinde ince alt çubuk.
- **İşaretler:** ⚡ ateşlenen otomasyon (tetikleyen oturumun şeridinde), ↷
  atlanan (sebep tooltip'te), ✕ koordinatör stall halt.
- **Gelecek şeridi:** "şimdi" çizgisinin sağında kurulu zamanlayıcılar (⏰,
  `ws:schedule_armed`), ufuk 30 dk; ötesi kenara kırpılır.
- **Süre log ekseni (2026-09-04, varsayılan açık):** zaman → piksel eşlemesinde
  süre logaritmik harcanır; kısa oturumlar okunur genişlikte kalır, çok uzun
  olanlar paneli yemez. **Eksenin kendisi** dönüştüğü için çubuk, kenar, işaret
  ve bekleme dilimi aynı eşlemeyi izler: her başlangıç ve bitiş tam
  zamanındadır. Üst çubuktaki "süre log" anahtarıyla kapatılır. Ayrıntı §14.
- **Boşluk kırpma (2026-09-04, varsayılan açık):** geçmiş penceresinde hiçbir
  şeritte çubuk/kenar/işaret olmayan aralıklar birkaç piksellik (4 px) sönük
  bir dilime indirilir; zaman → piksel eşlemesi böylece **parçalı doğrusal**
  olur. Bir
  gecelik boşluk yoksa görünüm eskisi gibi düz doğrusaldır. Üst çubuktaki
  "boşluk kırp" anahtarıyla kapatılır; kapalıyken 12 günlük boşluk ekseni
  yutup asıl işi birkaç piksele sıkıştırır. Ayrıntı §11.
- **Üst çubuk:** bağlantı noktası (yeşil = akış bağlı), sayaçlar, spawn/kuyruk
  kapasitesi (`/api/workspace/liveness`), **yakınlık** kontrolü, boşta-şerit
  penceresi (1 sa / 6 sa / 24 sa / tümü), `#seq · r<revizyon>`.
- **Yakınlaştırma (2026-09-04):** zaman ekseni panelin 1x–8x'i kadar geniş
  çizilir, kanvas yatay kaydırılır. Etiket sütunu ve gelecek şeridi ölçeklenmez.
  Ctrl/⌘ + tekerlek imlecin altındaki anı sabit tutarak yakınlaştırır; `+`/`-`
  adımlar, `0` panele sığdırır. Ayrıntı §15.
- **Çip süzgeci (2026-09-04):** başlığın altında, Sohbet listesindekiyle **aynı
  çipler** (`SESSION_CHIPS`) ve aynı tıklama sözleşmesi — düz tık değiştirir,
  Ctrl yalnız onu seçer, Shift diğerlerini tersler. Ayrıntı §12.
- **Sağ panel:** seçim yoksa tetik / koordinasyon / kurulu zamanlayıcı listesi;
  seçim varsa üstte **"Sohbette aç"** (seçili şeridin kendi oturumu) ve worker
  şeridinde ek olarak **"↰ Asıl oturum"** (bağlı olduğu kök), altında
  `ViewPanel` (get_view card: `session` veya `flowrun`).

Etkileşim: tık = seç, çift tık = oturumu Sohbet'te / akış koşusunu Akışlar'da
aç, ↑↓ şeritler arasında gez, Enter aç, Esc seçimi bırak (Harita sözleşmesi).
Çift tık ve Enter keşfedilir değildi; sağ paneldeki iki düğme aynı işi görünür
kılar (§12).

## 2. Veri yolu (F0)

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

## 3. F0'ın bilinçli sınırları ve kapanışları

| F0'da bırakılan | Kapandı mı | Nerede |
|-----------------|------------|--------|
| Rota varlığı üretilmiyor | ✅ F1a | §5 |
| Faz sütunu yok, yalnız zaman | ✅ F1b (◈ → rota-içi görünüm) | §6 |
| Mini rota (sohbet başlığı), RunView gömme | ✅ F1b / F5 | §6, §10 |
| Maliyet / süre | ✅ F3 (özet çipleri; ayrı alt şerit yok) | §8 |
| Şerit patlaması için demet katlama (`+N`) | ❌ açık | pencere süzgeci + çip süzgeci + son-200 seed |
| Boş zaman aralıkları ekseni yutuyor | ✅ 2026-09-04 | §11 |
| Worker alt-ağacı katlama | ❌ açık | — |
| Mobilde sağ panel gizli (`md:` altı), kanvas yatay kaydırır | tasarım kararı | — |

## 4. Testler (F0 + F1a)

- `frontend/src/features/rota/rotaZoom.test.ts` — aralık sınırlama ve bozuk
  girdi, adımların iki yönde yürümesi ve uçlarda durması, tekerlekten gelen ara
  değerden adıma oturma, tüm adımların erişilebilirliği, imleç sabitleme
  (yakınlaşma/uzaklaşma, sol kenarı geçmeme, **ölçeklenmeyen etiket sütununun
  hesaba katılması**), `formatZoom`.
- `frontend/src/features/rota/rotaWaits.test.ts` — tek worker'ın aralığı,
  örtüşenlerin birleşmesi + peak eşzamanlılık, bitişik worker'ların tek sürekli
  bekleme okunması, aradaki boşluğun ikiye bölmesi, canlı worker'ın "şimdi"ye
  açık kalması, eşik altı ve worker'sız kökler, şerit süzgecine uyum,
  `clipWaits`, `layoutRota`'nın beklemeyi yalnız köke koyması.
- `frontend/src/features/rota/rotaChips.test.ts` — çip yüklemi (worker'ın hem
  tür hem kapsam çipini istemesi, subagent'ın `origin.kind`'dan sınıflanması,
  arşiv/çalışan kapsamları), ağaç kuralı (eşleşen üyesi olan kök kalır, hiç
  eşleşmeyen kök düşer), `layoutRota` satırlarının daralması, çip sayaçları.
- `frontend/src/features/rota/rotaTimeScale.test.ts` — boşluk bulma (eşiğin
  altındakiler kalır, baş/son ölü hava sayılır, işaret/kenar anı canlı tutar),
  parçalı ölçek (dilim genişliği sabit, tek yönlü artan, pencere dışı kırpılır),
  dilimler paneli uçtan uca döşer, dilimler panelin yarısını yiyecekse doğrusala
  düşüş, `formatGapSpan`; **log ekseni** (pencere uçlarının sabitlenmesi, tek
  yönlü artış, uzun aralığın kısaya göre çok daha fazla sıkışması, boşluk
  kırpmayla birlikte çalışma, kapalıyken doğrusal eşleme).
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

## 9. F4 — LLM reçete optimizer, yalnız öneri (2026-09-03)

Dal `rota/f4-optimizer` (F3'ün üstüne). Brif §7.2–7.4. Rota işinin tek LLM
geçişi ve nadir olanı: reçete başına, **≥3 yeni özetlenmiş terminal koşu**
biriktiğinde, bir koşu **failed** bittiğinde ya da kullanıcı istediğinde.
v1 reçeteyi düzenlemez; öneriler mevcut içgörü yaşam döngüsüne düşer.

**Sistem ajanı** `recipe-optimizer` (`systemagents.go`, ✦, araçsız —
`AllowedTools: []`; prompt `prompts/defaults/recipe-optimizer.md`, kayıt
`prompts.go`, workspace `config/prompts/recipe-optimizer.md` ile ezilebilir).
Model/sağlayıcı `resolveAnalysisSystemAgent` ile içgörü analizcisi gibi
çözülür; temel ajan `pickInsightAgent`.

**Geçiş** (`internal/agent/recipe_optimizer.go`):
- `MaybeOptimizeRecipe(slug, trigger)` rota iş kuyruğunda, rota-sonu
  kurallarından sonra çağrılır; eşik `optimizer/state.json`'daki
  (`db.OptimizerSlugState`: `lastAt`, `runsSeen`, `trigger`, `proposals`,
  `skipped`) son geçişe göre ölçülür; LLM çağrısı ayrı goroutine'de, reçete
  başına tek uçuş.
- `RunRecipeOptimizer(slug, trigger)`: reçete (frontmatter planı + gövde,
  6 KB'a kırpılır), sürüm başına istatistik, son 5 özet, küratörün reçete
  önerileri → `guardedComplete` (JSON şeması). Cevap `{"proposals":[…]}`;
  her öneri `action` (`prune_phase | make_optional | prune_watcher |
  change_profile | add_gate | bind_watcher | split_phase | merge_phase |
  rollback_version`), `target`, `value`, `removes`, `title`, `rationale`,
  `evidence`, `severity`.
- **Kodda uygulanan değişmezler** (`optimizerFinding`): sayı içeren
  `evidence` yoksa **atılır**; genel olumsuz yargı ("güvenilmez",
  "çalışmıyor", "useless" …) **atılır**; ekleme sınıfı (`add_gate`,
  `bind_watcher`, `split_phase`) reçete büyüme bütçesini
  (`skills.RecipeGrowthBudget = 9`, faz + izleyici) aşacaksa `removes`
  adlandırmalı; bilinmeyen eylem atılır. Bütçe ayrıca `RecipeSpec.Validate`
  ile **yüklemede** de zorlanır — tavanın üstündeki reçete yüklenmez.
- Kabul edilen öneri `insight.Finding` olur: kanal **`recipe-opt`** (yeni;
  `Channel.Valid()`), `LensID recipe-optimizer`, imza
  `recipe-opt:<slug>:<action>:<target>` (tekrar = `Occurrences` artar),
  `FilePointer skill/<slug>`, `RootCause` = kanıt, `ProposedFix` = gerekçe +
  değer + kaldırır, `EvidenceSessionIDs` = son koşuların kök oturumları,
  yapısal `Proposal` alanı (`insight.RecipeProposal`). Öneri varsa `insight`
  bildirimi ("✦ Reçete optimizer — slug için N öneri").
- Uygulama: **insan**. Kullanıcı reçeteyi Beceriler'den düzenler, bulguyu
  `applied` + kanıt `skill/<slug>` ile kapatır (mevcut
  `ErrAppliedNeedsEvidence` kuralı); kapanan bir bulgunun sonraki geçişte yeniden
  gelmesi mevcut `Regressed` mekanizmasıyla işaretlenir.

**API.** `POST /api/recipes/{slug}/optimize` (elle; `OptimizerResult`: ran,
skipped, proposals, dropped), `GET /api/recipes/{slug}/optimizer` (son geçiş).
Öneriler `GET /api/insight/findings?channel=recipe-opt`.

**Ekran.** İçgörü: kanal süzgeci ve özet çipi `✦ recipe-opt`, rozet, bulgu
modalında "Reçete önerisi" bloğu (eylem/hedef/değer/kaldırır/kanıt + "reçeteyi
sen düzenlersin" notu). Beceriler ▸ reçete istatistikleri: **"Şimdi optimize
et"**, açık öneri sayısı, son geçiş satırı. Rota-içi görünüm başlığı:
"optimize et".

**Testler.** `recipe_optimizer_test.go` (değişmezler: kanıt/sayı, olumsuz
yargı, bütçe + `removes`, bilinmeyen eylem; bulgu şekli; büyüme bütçesi
doğrulaması; prompt içeriği; ajansız/koşusuz geçişlerin durum kaydı),
`prompts_test` kayıt senkronu, `skills` doğrulama.

**Bilinen durum.** `internal/insight` paketinin `scanner_test.go:231` testi
bu değişiklikten bağımsız olarak main'de de düşüyor (analizci zaman aşımı
beklentisi); F4 pakete yalnız kanal sabiti + `Proposal` alanı ekler.

**Kalanlar.** Budama sınıfı için reçete başına opt-in oto-uygulama (F4-v2:
`applier` benzeri, yalnız `prune_*`/`make_optional`); "oku-sonra-yaz" reçete
düzenleme aracı; regresyonda otomatik sürüm geri alma; F5 kapılar ve
kanvastan müdahale.

## 10a. E2E sonrası düzeltmeler (2026-09-03)

Gerçek LLM e2e'sinin (claude-cli, ardından codex-cli) ortaya çıkardığı üç
davranış ve kapanışları — hepsi bu belgenin kurallarını değiştirir:

| Bulgu | Kural (yeni) | Kod |
|-------|--------------|-----|
| Yaprak worker `report_to_coordinator{completed}` deyip turunu bitirince koordinatöre 4 sn arayla **iki** "completed" bildirimi düşüyordu (biri tutulan rapor, biri terminal bildirim) | Turu sonucu olan düğümde (yaprak worker / delege etmemiş alt-koordinatör) tutulan rapor terminal bildirime **katlanır**: yalnız terminal not gider; ajan daha zayıf bir durum bildirdiyse (`incomplete`/`failed`) o durum notun durumu olur. Ertelenen alt-koordinatör (`deferWorkerReport`) eskisi gibi yalnız raporunu gönderir | `coordination_tree.go` `pendingUpwardReport.foldIntoTerminal`, `coordination.go` terminal yol |
| "fazsız" sütunu (rota-sonu izleyicisi ve oturumu gibi faza bağlı olmayan düğümler) 1440 px'te kaydırma dışında kalıyor, ipucu yoktu | Min sütun genişliği 150 px (beş sütun 912 px panele sığar); sığmazsa başlıkta `N sütun · sağa kaydır ⇢ <son sütun>` çipi | `features/rota/trajectoryGeometry.ts` (+test), `RotaTrajectoryView.tsx` |
| Kapı yanıtının transkript kaydı (`<gate …>`) user-rollü olduğundan yeniden başlatmada `RecoverOrphanedTurns` bitmiş kök oturumu "yarım kalmış tur" sayıp yeniden kuyrukluyordu; koordinatör bitmiş işe yeni worker'lar açtı | Kayıt `Origin: gate` ile yazılır (`noteOriginGate`); kurtarma, transkripti bu kayıtla biten oturumu **bitmiş** sayar | `trajectory_gate.go`, `coordination.go` `RecoverOrphanedTurns`, test `TestRecoverSkipsTrailingGateNote` |
| Otomasyonun açtığı oturum reçeteli koordinatör ajandaysa listede ayrı bir "planlandı" rota olarak da beliriyordu | Başka bir rotanın üyesinin başlattığı kök oturum önce o rotaya **düğüm** olarak bağlanır; bağlandıysa create'te tohumlanmaz. Gerçekten worker açarsa `bindSpawn` → `EnsureTrajectory` o anda kendi rotasını kurar | `trajectory_binder.go` `onSessionChangeForTrajectory` / `bindForkedSession` (bool döner) |

## 10. F5 — Kapılar ve kanvastan müdahale + F4-v2 oto-budama (2026-09-03)

Dal `rota/f5-gates-canvas` (F4'ün üstüne).

**Faz kapıları** (`internal/agent/trajectory_gate.go`). Bir faz `done`'a
taşınırken (ajanın `trajectory{phase}` aracı ya da kanvas) fazın ilan edilmiş
kapısı çalışır; `force` atlar:

| Kapı | Koşul | Geçmezse |
|------|-------|----------|
| `artifact` | ağaçtaki oturumlardan birinde başlığı değeri içeren (ya da türü değere eşit) artifact var | `ErrGateBlocked` — araç hata döner, kanvas 422 + "zorla?" onayı |
| `verdict` | kök transkriptin son 60 mesajında değer geçiyor ("VERDICT: PASS") | aynı |
| `human` | kök oturuma **Durable Ask** parklanır ("Rota RTA · plan fazı bitti sayılsın mı?" Onayla / Reddet); faz aktif kalır, rota **waiting**, grafta `g:<ask>` düğümü fazın altında (`kapı: plan`) | `ErrGatePending` — araç "kart açıldı, turunu bitir" der, kanvas 202 |
| `schema` | v1'de doğrulanmaz, notla geçer | — |

İnsan kapısı: `openPhaseGateAsk` `SessionAsk{Kind: ask, Payload: {question,
options, gate:{trajectoryId, phase, kind, value}}}` yaratır (faz başına tek
açık kapı), `ws:ask` (yeni tür, `sessionhub.KindWSAsk`) yayınlar; API köprüsü
(`bridgeWorkspaceEvent`) kartı oturum hub'ında açar (`openDurableAskCard`) —
yeniden bağlanan pencere `restoreWaitingAsks` ile aynı kartı görür. Yanıt
mevcut `answerDurableAsk` yolundan gelir; `agent.GateAskRef` ile kapı olduğu
anlaşılınca tur yeniden sürülmez, `ResolvePhaseGate` fazı kapatır (onay →
done, "kapı onaylandı") ya da aktif bırakır ("kapı reddedildi: …"), kök
transkripte `<gate …>` notu düşer, ask satırı silinir.

**Ortak graf düzenleyiciler.** `Runtime.SetTrajectoryPhase / PlanTrajectory /
FinishTrajectory` (CAS `expectedRev`, 0 = ajan aracı) — araç ve kanvas aynı
yolu kullanır; `trajectory` aracına `force` alanı eklendi.

**Kanvas API'leri** (`internal/api/trajectory_actions.go`): `POST
/api/trajectories/{id}/plan|phase|finish` (409 eski revizyon, 422 kapı, 202
insan kapısı), `POST /api/sessions/{id}/workers` (**buradan çatalla**:
`SpawnWorker`, koordinatör olmayan oturuma 400), `GET
/api/trajectories/by-node?run=&node=` (akış koordinatör düğümü → rota).
İstemci hataları artık `status` taşır (`api/client.ts`).

**Ekran.** Rota-içi görünümde faz sütununa tıklayınca **PhaseActions** çubuğu
(aktif yap / tamamlandı (kapı) / atla; "faz ekle" mini formu; "bitir"); seçili
oturum için **"buradan çatalla"** → `ForkModal` (profil/ajan, görev,
alt-koordinatör); RunView'da koordinatör düğümünün başlığında **"Rota"**
düğmesi (`#/w/WS/rota/RTA` hash'i ile derin bağlantı, prop zinciri yok).

**F4-v2 oto-budama.** Reçete frontmatter'ında `auto_prune: true`
(`RecipeSpec.AutoPrune`) varsa optimizer'ın **budama sınıfı** önerileri
(`prune_watcher`, `prune_phase`, `make_optional`) `skills.ApplyRecipeProposal`
ile anında uygulanır: yalnız `phases:` / `watchers:` / `version:` /
`optimizer:` satırları yeniden yazılır (diğer frontmatter ve gövde bayt bayt
korunur), sürüm artar, yazmadan önce yeniden ayrıştırılıp doğrulanır; bulgu
`applied` + kanıt `skill/<slug>` ile kapanır (`OptimizerResult.applied`).
Eklemeler (`add_gate`, `bind_watcher`, `split/merge`, `rollback`) hep insan
onayı bekler. Beceriler'de `✂ oto-budama` çipi.

**Testler.** `trajectory_gate_test.go` (artifact/verdict kapıları, force, CAS
409, insan kapısı: tek ask, waiting graf, red → aktif + sebep, onay → done,
transkript notu), `skills/recipe_edit_test.go` (budama/profil değişimi, sürüm
artışı, korunan satırlar, ret durumları, render), araç testi `force`.

**Kalanlar (brif dışı / sonraki).** Faz ↔ todo bağı (faz düğümüne tıklayınca
todo listesi), worker alt-ağacı katlama, maliyet/süre alt şeridi, `schema`
kapısının gerçek doğrulaması, reçete sürüm geçmişinin seed ledger'ında
tutulması, küratör önerisini tek tıkla uygulama, zamanlama/hook kartlarında
pin düğmesi, faz kuralı modalında faz kimliği seçici, `rollback_version`
uygulaması.

## 11. Boşluk kırpma — boş zaman aralıklarını daralt (2026-09-04)

**Sorun.** F0 kanvasının geçmiş penceresi düz doğrusaldı: `t0 → şimdi` arası
piksellere orantılı bölünüyordu. "tümü" penceresinde 12 günlük bir depoda
oturumların çoğu son birkaç saatte olduğu için bütün çubuklar sağ kenarda
birkaç piksele sıkışıyor, aradaki ölü hava tuvalin %90'ını yiyordu.

**Çözüm.** Zaman → piksel eşlemesi parçalı doğrusal oldu.
`frontend/src/features/rota/rotaTimeScale.ts` (saf, DOM'suz):

- `findGaps(layout, minGapSec)` — çubuk aralıklarını, işaret ve kenar anlarını
  birleştirip `[t0, şimdi]` içindeki boş aralıkları çıkarır. Eşik altı
  (`MIN_GAP_SEC` = 5 dk) aralıklar dokunulmaz; her etkinliğin iki yanına
  30 sn tampon konur ki çubuk ucu ile kırpma işareti bitişmesin. Pencerenin
  başındaki ve sonundaki ölü hava da sayılır.
- `buildTimeScale(layout, {x0, width, collapse})` — her boşluğa sabit
  `GAP_PX` = 4 px verir (dilime harcanan her piksel gerçek etkinlikten
  gider; 4 px kesintiyi göstermeye yetiyor), kalan genişliği gerçek saniyeler paylaşır ve
  `x(t)` ile `segments` döndürür. Kırpılacak bir şey yoksa (ya da anahtar
  kapalıysa) bu tam olarak eski doğrusal eşlemedir. **Emniyet:** dilimlerin
  toplamı panelin yarısını aşacaksa doğrusala düşer — çok sayıda kısa boşluk
  asıl etkinliği sıfıra sıkıştırmasın.
- `formatGapSpan(sn)` — "2 sa 15 dk" / "1 gün 1 sa".

`RotaCanvas` ölçeği `useMemo` ile kurar; kırpılan her aralık sönük bir dilim +
kesik dikey dikiş çizgisi olarak çizilir (4 px'te tarama deseni okunmuyor) (tooltip: ne kadarın gizlendiği), üst
çubuktaki `t0` etiketine "· 12 gün 1 sa kırpıldı" eklenir. Gelecek şeridi
doğrusal kalır — 30 dk ufkunda kırpacak boşluk yok.

`RotaToolbar`'a `role="switch"` bir "boşluk kırp" düğmesi geldi; durum
`RotaPanel`'de `useState(true)` — **varsayılan açık**, pencere süzgeciyle aynı
kalıp (kalıcı değil, oturum başına).

**Kapsam.** Yalnız F0 workspace kanvası. Rota-içi görünüm (`RotaTrajectoryView`)
faz sütunlarına göre yerleşir, zaman ekseni yoktur; orada kırpılacak bir şey yok.

## 12. Çip süzgeci ve oturuma gitme düğmeleri (2026-09-04)

**Çip süzgeci.** F0 kanvasının tek süzgeci boşta-şerit penceresiydi; 80 şeritli
bir depoda "yalnız otomasyonlar" ya da "yalnız worker'lar" demenin yolu yoktu.
Başlığın altına Sohbet listesindekinin **aynısı** bir çip şeridi geldi:

- Çip sözlüğü ve yüklemi paylaşılıyor — `features/sessions/sessionKindMeta`'nın
  `SESSION_CHIPS` / `sessionMatchesChips` / `nextChipsOff`'u. Bir çip iki ekranda
  aynı şeyi demek zorunda; kopyalanmış ikinci bir liste ayrışırdı.
- `frontend/src/features/rota/rotaChips.ts` yalnız Rota'ya özgü iki şeyi taşır:
  **şekil adaptörü** (`laneChipShape` — lane deposu `category`/`executionType`
  taşımaz, o yüzden subagent `origin.kind`'dan, worker `rootSessionId`'den,
  çalışan `live.state ?? runState`'ten türetilir) ve **ağaç kuralı**
  (`laneChipFilter` — eşleşmeyen bir kök, üyelerinden biri eşleşiyorsa yine
  çizilir; yoksa tutulan worker'ın spawn kenarı çizilmemiş bir şeride bakardı).
- `laneChipCounts` rozetleri seçim uygulanmadan sayar, yani kapalı bir çip neyi
  gizlediğini söylemeye devam eder. Sayım sınıflandırmadan doğrudan yapılır:
  `laneMatchesChips`'i tek çiple yoklamak her worker'ı sıfır sayardı, çünkü
  yüklem tür **ve** kapsam çipini birlikte ister. Bunun sonucu olarak `worker`
  anahtarı çift rol oynar (hem tür hem kapsam) ve rozeti ikisinin toplamıdır.
- `layoutRota` yeni bir `laneFilter` seçeneği alır; pencere süzgeciyle
  **VE**'lenir. Ağaç kuralını kuran çağıran taraftır, layout değil.
- Seçim `useRotaChips` ile `localStorage`'ta (`tionharness.rotaChipsOff`).
  Sohbet listesinden **ayrı anahtar**: Rota'da Worker'ı kapatmak Sohbet listesini
  değiştirmemeli. Sidebar'daki gibi **kapalı** çipler saklanır, böylece sonradan
  eklenen bir çip seçili başlar. Bir çip kapalıyken "süzgeci sıfırla" belirir.

**Oturuma gitme.** Kanvastan oturuma geçmenin tek yolu çift tık / Enter'dı.
Seçim yapılınca sağ panelin üstünde iki düğme çıkar: **"Sohbette aç"** seçili
şeridin kendi oturumunu açar; şerit bir worker ise ayrıca **"↰ Asıl oturum"**
çıkar ve `rootSessionId`'nin gösterdiği kök oturuma gider — worker'ın kendi
thread'i koşunun bir parçası, asıl konuşma kökte.

**Kapsam.** Yalnız F0 workspace kanvası; rota-içi görünümde (`RotaTrajectoryView`)
çip şeridi gizlenir, orada süzülecek şerit yok.

## 13. Bekleme aralıklarını çubukta göstermek (2026-09-04)

**Sorun.** Bir koordinatörün çubuğu kesintisiz tek parça çiziliyordu, oysa uzun
bir koordinatör oturumunun büyük kısmı worker'lardan cevap beklemekle geçer.
Çubuğa bakınca "bu oturum 4 saat çalıştı" okunuyordu; gerçekte 4 saatin çoğu
bekleme olabilir.

**Ne yapıldı.** `frontend/src/features/rota/rotaWaits.ts` (saf, DOM'suz)
koordinatörün bekleme aralıklarını **üye şeritlerin ömründen** türetir:

- `waitSpans(state, rootId, now, keep)` — üyelerin `[createdAt, bitiş]`
  aralıklarını birleştirir. Örtüşenler tek aralık olur; uç uca değen iki worker
  da **tek sürekli bekleme**dir, çünkü koordinatör arada turu geri almamıştır.
  `peak` (o aralıktaki en yüksek eşzamanlı worker sayısı) birleştirmeden **ayrı**
  bir geçişte ölçülür: birleştirme sweep'inden saymak, uç uca devri anlık 2
  eşzamanlılık gibi gösterirdi. Eşik `MIN_WAIT_SEC` = 60 sn. Süzgeç `keep` ile
  paylaşılır, böylece bir bekleme hiç çizilmemiş bir şeride işaret etmez.
- `clipWaits` beklemeleri koordinatörün kendi çubuğuna kırpar (bir worker
  arşivlenmiş koordinatörden uzun yaşayabilir).
- `layoutRota` beklemeyi `RotaBar.waits`'e koyar, **yalnız kök şeritlerde**;
  bir üye şeridin kendi worker'ı yoktur.
- `RotaCanvas` bunları çubuğun üstüne çizgili bir örtü olarak çizer
  (`#rota-wait-hatch`). Taban genişlik `WAIT_MIN_PX` = 3 px: 10 günlük bir
  pencerede 19 dk'lık bekleme piksel altı kalır, oysa eşiği geçtiği için
  görünmeyi hak eder. Taban çubuğun sonunu taşıracaksa içeri kaydırılır.
  Tooltip: "2 worker · 18 dk bekleme".

**Neden liveness'tan değil.** `liveness.AwaitingWorkers` yalnız "şu an"ı
yanıtlar; dünkü bir çubuğu boyayamaz. Üye şeridin ömrü ise depoda duruyor ve
geriye dönük çalışıyor.

**Bilinçli sınır — `run_subagent`.** Bir `run_subagent` çağrısı koordinatörün
**kendi turunun içinde** senkron çalışır ve ayrı oturum yaratmaz; dolayısıyla ne
`awaiting_workers` olur ne de lane deposunda bir üye şerit belirir. Örnek:
`WS5/SES906` — 14 `run_subagent`, 8 tur (411–1568 sn), **sıfır** çocuk oturum.
Bu oturumun çubuğu tek parça kalır. Bu aralıkları da boyamak için step
zamanlarının (`messages.jsonl`'deki `steps[].tool == 'run_subagent'`) oturum
başlığına ya da yeni bir uca yayınlanması, oradan lane deposuna taşınması
gerekir — `_Docs/77` R10 veri yolu sözleşmesine dokunan ayrı bir iş.

## 14. Süre log ekseni (2026-09-04)

**Sorun.** Piksel başına saniye sabitti. Çok günlük bir pencerede bu kanvası
uçlarda okunamaz yapıyor: çubukların çoğu 2-3 piksele yuvarlanırken tek bir uzun
oturum paneli yiyor. Ölçüldü (WS1, "tümü", 83 çubuk): p75 **7.9 px**, p90
**33.5 px**, en geniş **438 px**.

**Neden çubuk genişliği değil, eksen.** İlk yaklaşım çubuk genişliklerini ayrıca
sıkıştırmaktı. Bu, çubuğun **ucunun** `end` zamanına oturmaması demekti: kenarlar
ve işaretler gerçek eksende kalırken çubuk kalmıyordu, ve büyüyen çubuklar hem
"şimdi" çizgisini hem birbirini aşabiliyordu (ölçüldü: 4 çubuk taşıyordu, en
kötüsü 172 px). Tavan koymak taşmayı çözdü ama hizasızlığı çözmedi. Doğru çözüm
eksenin kendisini dönüştürmek: o zaman **her şey** aynı eşlemeyi izler.

**Ne yapıldı.** `rotaTimeScale.ts`'e `logDuration` seçeneği eklendi;
`rotaBarScale.ts` tamamen kaldırıldı (artık çubuk-içi normalizasyon yok).

- `measure(sec, log)` — süreyi eksenin birimlerine çevirir.
  `LOG_KNEE_SEC * log1p(sec / LOG_KNEE_SEC)`: sıfırda sonlu ve tek yönlü artan,
  yani sıra ve "sonraki daha sağda" her zaman korunur.
- `LOG_KNEE_SEC` = **5 dk**. Diz noktasının altı fiilen doğrusaldır. İlk deneme
  60 sn'ydi; çoğu oturum zaten dakikalarca sürdüğü için sıradan şeritler de uzun
  olanlarla birlikte sıkışıyordu — amacın tam tersi.
- **Dilimleme.** Canlı bir aralık, `eventInstants` ile her olay anında (çubuk
  başı/sonu, işaret, kenar) kesilir ve **her dilim kendi süresiyle** ölçülür.
  Bütün bir aralığı tek seferde ölçmek paneli erken saniyelere harcar, pencere
  sonundaki kısa bir oturum gerçekte ne kadar sürdüğünden bağımsız ezilirdi
  (ilk sürümde tam bu oldu: p75 7.9 → 4 px'e **düştü**).
- Boşluk kırpma (§12) ile birlikte çalışır: kırpılan dilimler sabit `GAP_PX`
  genişliğini korur, log yalnız canlı dilimlerin içinde işler.

**Ölçülen sonuç** (aynı kanvas): p75 **7.9 → 12.7 px**, p90 **33.5 → 54.4 px**,
en geniş **438 → 400 px**. Yani kısa ve orta şeritler okunur hale geldi, en uzun
olan geri çekildi. Zaman ekseninden taşan çubuk **0**, çubuk çakışması **0**.

**Hiza.** Çubuğun iki ucu da aynı eksenden gelir, dolayısıyla başlangıç ve bitiş
noktaları tam zamanındadır. Kenarlar, işaretler, bekleme dilimleri (§13) ve
"şimdi" çizgisi aynı eşlemeyi paylaşır. Anahtarı kapatmak doğrusal eksene döner.

## 15. Yoğunluk ve yakınlaştırma (2026-09-04)

**Sıkılaştırma.** Kanvas bir yoğunluk görünümü; satır başına harcanan her piksel
ekranda bir eksik şerit demek. Ölçüler daraltıldı: satır adımı `ROW_H` 30 → 22,
etiket sütunu 220 → 180, gelecek şeridi 170 → 140, üst boşluk 26 → 20, sağ
boşluk 12 → 8; oturum çubuğu 12 → 11, akış koşusu çubuğu 6 → 5 px. Etiket
kırpma sınırı de yeni sütun genişliğine göre düşürüldü (26/22 → 22/19 karakter).
Aynı 900 px yükseklikte görünen şerit sayısı **27 → 37**.

**Yakınlaştırma.** `rotaZoom.ts` (saf) + `RotaZoomControl.tsx`:

- Zoom **yalnız geçmiş eksenini** çarpar (`pastW * zoom`). Etiket sütunu, gelecek
  şeridi ve satır adımı piksel boyutlarını korur, böylece metin her seviyede
  okunur kalır ve etiketler çubuklarından ayrılmaz. Kanvas panelden genişleyince
  mevcut kaydırma kabı onu yatay kaydırır.
- Adımlar `ZOOM_STEPS` = 1, 1.5, 2, 3, 4, 6, 8. `stepZoom` tekerlekten gelen ara
  bir değerden (örn. 2.4) doğru adıma oturur.
- **Ctrl/⌘ + tekerlek** sürekli yakınlaştırır ve `anchoredScrollLeft` ile
  imlecin altındaki anı sabit tutar. Klavye: `+`/`-` adımlar, `0` panele sığdırır.
- `anchoredScrollLeft` **ölçeklenmeyen etiket sütununu** (`fixedLeft`) hesaba
  katar: oranı ham kanvas konumuna uygulamak, etiket genişliğiyle büyüyen bir
  kayma bırakıyordu (ölçüldü: 23 px, düzeltmeden sonra 7 px — kalanı ölçüm
  sırasındaki etiket yuvarlaması).

**Uygulama notu.** Tekerlek işleyicisi kaydırma düzeltmesini `setZoom`
güncelleyicisinin **içinde** yapmıyor: güncelleyici saf olmak zorunda (React onu
iki kez çağırabilir), oysa düzeltme tam bir kez çalışmalı. Zoom bir ref'ten
okunur, hedef kaydırma `pendingScroll` ref'ine yazılır ve `useLayoutEffect`
içinde uygulanır — o noktada kanvas yeni genişlikte commit edilmiştir, yoksa
`scrollLeft` eski (dar) `scrollWidth`'e kırpılır. İlk sürümde bu yüzden kaydırma
hiç uygulanmıyordu.

**Açık kalan.** Yakınlaştırılıp sağa kaydırıldığında etiket sütunu da kayar;
sabitlemek (sticky) ayrı bir iş.
