# Rota Ekranı — F0 (projeksiyon) + F1a (rota varlığı, backend)

**Durum:** F0 uygulandı (2026-09-02); F1a (rota varlığı + ilan, backend)
uygulandı (2026-09-02, §5). Altyapı planı: `77-ROTA-ALTYAPI-PLANI.md`
(R1–R10). Tasarım brifi: oturum artefaktı "Rota Tasarım Brifi" (§8 ekran, §11
fazlar). Sonraki fazlar F1b (rota başına faz-sütunlu ekran, derin bağlantı,
mini rota), F2 (otomasyonlar grafikte), F3 (metrik + küratör), F4 (optimizer),
F5 (kapılar / kanvastan müdahale).

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

- ~~**Rota varlığı henüz üretilmiyor.**~~ F1a ile kapandı (§5): koordinatör
  ağaçları artık rota üretir, kanvas şerit etiketine ◈ koyar ve tooltip'te
  revizyonu gösterir. Faz bantları (dikey PLAN/KOD/İNCELEME sütunları) hâlâ
  yok — F1b ekran işi.
- **Faz sütunu yok, yalnız zaman.** Brifteki `x = faz sütunu` yerleşimi rota
  başına görünümdür; workspace kökü zaman eksenlidir. Rota-içi yakınlaştırma
  (tek rota, faz bantları, worker alt-ağacı katlama) F1/F2.
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
