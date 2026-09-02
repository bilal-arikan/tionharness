# Rota Ekranı — F0 (projeksiyon)

**Durum:** F0 uygulandı (2026-09-02). Altyapı planı: `77-ROTA-ALTYAPI-PLANI.md`
(R1–R10). Tasarım brifi: oturum artefaktı "Rota Tasarım Brifi" (§8 ekran, §11
fazlar). Sonraki fazlar F1 (rota varlığı + ilan), F2 (otomasyonlar grafikte),
F3 (metrik + küratör), F4 (optimizer), F5 (kapılar / kanvastan müdahale).

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

- **Rota varlığı henüz üretilmiyor.** `Trajectory` (R4) deposu ve okuma uçları
  hazır ama hiçbir gözlemci `CreateTrajectory` çağırmıyor; bu F1'in işi. Kanvas
  bir rota varsa şerit etiketine ◈ koyar ve tooltip'te revizyonu gösterir; faz
  bantları (dikey PLAN/KOD/İNCELEME sütunları) F1 ile gelir.
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
