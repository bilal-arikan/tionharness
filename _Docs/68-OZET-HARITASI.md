# 68 — Özet Haritası (Workspace Explorer / semantic-zoom drill-down)

> **Durum:** Faz 1-3 tamamlandı ✅ (2026-08-06) + **TSK66 genişletmesi** ✅ (2026-08-06:
> single-expand accordion + Artifacts/Otomasyonlar/Skill'ler/İçgörüler/Günlükler kovacıkları).
> Kalan opsiyonel: MCP resource tree + büyük-workspace performansı (sigma.js) — ihtiyaç
> kanıtlanınca.
> **Önkoşul okuma:** `_Docs/66-VIEW-KATMANI.md` (bu özelliğin motoru), `internal/view/*`,
> `frontend/src/features/network/*` (ayrışacağımız komşu ekran).

## 1. Amaç

Workspace'in durumunu **kök düğümden başlayıp tıkladıkça bir katman derinleşen**
bir düğüm haritası olarak gezmek. Root'a tıklayınca en temel workspace özeti; oradan
kategori düğümlerine (Oturumlar, Akışlar, Pano, Pano-sütunu, Ajanlar, Bütçe, Araçlar)
dallanır; bir kategoriye tıklayınca üyeleri açılır; bir üyeye tıklayınca onun alt
düğümleri (ör. oturum → koordinatör/worker'lar). Aynı sistemi **bir ajan da** workspace
özetlerini gezmek için kullanabilmeli.

Literatürdeki karşılığı: **progressive disclosure (kademeli açılım) + semantic zoom +
degree-of-interest (DOI) tree**. Ekranı doldurmadan yalnız ilgili dalı açık tutmak.

## 2. Neden sıfırdan değil — mevcut temel: View katmanı

`internal/view/` bu haritanın motorudur. Zaten hazır olan primitifler:

| İhtiyaç | Mevcut karşılığı |
|---|---|
| Düğüm adresi | `view.Ref{Kind, ID, Sub}` (`session:SES1`, `board:board#in_progress`) |
| Düğüm özeti | `Projector.Project(ctx, ref, level) → View` |
| Çocuk düğümlere kenar | `View.Handle{Label, Ref, Level}` (drill-down işaretçisi) |
| Semantic zoom (bütçe) | `Level` = `tiny/card/full` |
| Ajan erişimi | `get_view` aracı (Ref çözer, Handle takip eder) |
| Kırılma izi | `ViewPanel.trail` (frontend'te zaten var) |
| Kök | `ProjectWorkspace` (`workspace` roll-up) |

**Sonuç:** ajan-tarafı zaten gezilebilir; eksik olan (a) kategori/eksik-Kind düğümleri,
(b) "problem-odaklı" değil "yapısal" çocuk listesi, (c) görsel harita ekranı.

## 3. Ayrı ekran kararı — "Harita", Ağ'dan bağımsız

Mevcut **Ağ ekranı** (`features/network`, vis-network) bir **ilişki grafiği**: "hangi
ajan-örneği hangi görev/oturumda çalışıyor" (çalışan runtime instance'ları). Bizimki
farklı: **durumu katman katman açan özet-drill-down**. Bu yüzden **ayrı bir ekran**
(`features/explorer`, NavRail'de yeni "Harita" girişi). `VisNetworkGraph` yeniden
kullanılabilir ama veri modeli ve etkileşim farklı → ayrı feature klasörü.

## 4. Düğüm hiyerarşisi

```
workspace (root)
├── Oturumlar (category)      → session:SES*  → (coordinator/worker alt düğümleri)
├── Akışlar (category)        → flowrun:RUN*  → (node alt düğümleri, Sub)
├── Pano (board)              → board#<sütun> → kart (task) düğümleri
├── Ajanlar (category)        → agent:AGT*    → o ajanın oturumları
├── Artifacts (category)      → artifact:ART* (yaprak — metadata projeksiyonu)
├── Otomasyonlar (category)   → automation:AUT* (yaprak — tetik/durum/hata)
├── Skill'ler (category)      → skill:<slug> (yaprak — katalog girişi)
├── İçgörüler (category)      → insight:FND* (yaprak — bulgu özeti)
├── Günlükler (logs, yaprak)  → process log kuyruğu inline (budget gibi)
├── Bütçe (budget)            → gün/model kırılımı
└── Araçlar (tools)           → workspace-aktif araç seti / MCP sunucuları
```

TSK66 notları:
- Kök **11 düğüm** (eskiden 6); boş kovacık da görünür (harita şekli içerikle değişmez).
- Yeni kovacıkların üyeleri **yapraktır** — tıklayınca yan panelde metadata projeksiyonu
  açılır, içerik (artifact body / skill body / bulgu detayı) ilgili ekranda kalır.
- `logs` yapraktır: ring-buffer kuyruğu inline render edilir.
- skills/findings/logs db'de değil → `Projector.WithSources(Sources{Skills, Findings,
  Logs})`; `internal/insight` view'i import ettiği için findings view-local `InsightFinding`
  tipine api-adapter'ıyla bağlanır (cycle yok).

**Dikkat: bu bir AĞAÇ değil GRAF.** `agent → session`, `session(coordinator) →
session(worker)` kenarları döngü üretebilir → lazy-expand'de **visited-set** ve
derinlik/çocuk cap'i şart (§8.1).

## 5. Backend değişiklikleri (`internal/view`, `internal/api`)

1. **Yeni Kind'ler:** `KindAgent`, `KindBudget`, `KindTools`, `KindCategory`
   (grup düğümü; ID = `sessions|flows|agents|...`). Board-sütunu zaten `Sub`.
2. **Yeni projeksiyonlar (deterministik, LLM yok):**
   - `agent.go` — `ProjectAgent`: ajan kimliği + bugünkü token/maliyet + aktif oturum
     sayısı + son etkinlik; handle'lar → ajanın oturumları.
   - `budget.go` — `ProjectBudget`: `billing.RollupOf` roll-up'ını View'a sar (yeniden
     hesaplama yok); gün/model kırılımı handle'ları.
   - `tools.go` — `ProjectTools`: workspace-aktif araç seti + MCP sunucu havuzu durumu.
   - `category.go` — `ProjectCategory`: bir kategori altındaki üyeleri sayar + üst-N'i
     handle olarak verir, kalanı `Elided` ile bildirir.
3. **`Children(ctx, ref) []Handle`** — Projector'a **yapısal çocuk** metodu
   (özet `Project`'ten ayrı; harita gezinirken her düğüm için tam `card` render
   etmeden çocukları almak için). `workspace` → 11 kategori/yaprak; `category:sessions` →
   session handle'ları; `board` → sütun handle'ları; vb.
4. **Endpoint:** `GET /api/views/{kind}/{id}/children` → `[]Handle`. Özet için
   mevcut `GET /api/views/{kind}/{id}` korunur.
5. **Elision & cost sözleşmesi korunur:** kategori/harita düğümü kaç öğe gizlediğini
   (`Elided`+birim) ve `~N tok`'u taşır.
6. **TSK66 ek Kind'ler:** `artifact`/`automation`/`skill`/`insight`/`logs` +
   `artifacts`/`automations`/`skills`/`insights` kategorileri. Store-dışı kaynaklar
   (`Sources`) opsiyoneldir; yoksa ilgili düğüm "yok" der, harita çökmez.

## 6. Agent tarafı

- `get_view` **zaten** handle döndürüyor → ajan BFS/DFS ile gezebiliyor. **✅ `expand`
  aracı canlı** (`internal/tools/builtin_expand.go`): `expand{kind,id,sub}` →
  `Projector.Children`'ı sarar, haritayla **birebir aynı backend**. Her çocuğu doğru
  sonraki çağrıya yönlendirir (çocuğu olan düğüm → `expand`, yaprak → `get_view`).
  Salt-okunur, her ajana açık, kategori `diagnostics`. Ajanın doğal akışı:
  `get_view workspace` → `expand workspace` → ilgili kategoriyi `expand` → gereken dalı
  `get_view full`.
  Çıktı **4KB'a cap'li** (`view.CapLines`, 2026-08-10): olgun bir workspace'te
  `expand{category:sessions}` her oturumu satır satır dökerdi. Toplam sayı başlıkta,
  elenen sayı ise açıkça yazılır — sessizce kısalmış bir liste eksiksiz sanılırdı.
- **Opsiyonel (ertelendi):** haritayı **MCP resource tree** olarak sun (`view://workspace`
  → alt resource'lar) → herhangi bir MCP istemcisi (Claude Code dahil) gezer.

## 7. Frontend (`features/explorer`)

- **Kütüphane kararı:** yeni ağır bağımlılık YOK. **React Flow** (Akış builder'da zaten
  kurulu — `@xyflow/react`) yeniden kullanılır; otomatik hiyerarşik yerleşim gerekirse
  küçük **`elkjs`** eklenir (impl sırasında karar; dagre alternatifi). vis-network de
  fallback.
- **Bileşenler:**
  - `ExplorerView.tsx` — ekran kabuğu + NavRail girişi ("Harita").
  - `ExplorerGraph.tsx` — React Flow canvas; lazy expand/collapse.
  - `ExplorerNode.tsx` — özel düğüm kartı (başlık + `~N tok` + durum ikonu + elision).
  - `SummarySidePanel.tsx` — seçili düğümün `getView card` özeti (mevcut `ViewPanel`
    embedded yeniden kullanılabilir).
  - `useExplorerGraph.ts` — düğüm/kenar state, lazy children fetch, visited-set.
- **Etkileşim:**
  - Düğüme tıkla → `children` çek → alt düğümleri ekle (bir katman derinleş); tekrar
    tıkla → collapse.
  - **Single-expand (accordion, TSK66):** bir node açılınca aynı parent'ın diğer açık
    node'ları kapanır (`nextExpandedSet`; `parentByKey` fetchChildren'da tutulur) — harita
    fan-out değil drill-down okur. Her seviyede çalışır.
  - Düğümü seç → yanda `getView card` özeti.
  - **Semantic zoom:** uzak zoom'da tiny satır, odakta card (zoom eşiğine göre içerik).
  - **Breadcrumb + level seçici** üst barda (ViewPanel kontratıyla aynı).
- **Canlı güncelleme:** `useRefreshTrigger('network')` benzeri bir `'explorer'` tick +
  merkezi SSE → açık düğümlerin çocukları tazelenir (tüm harita değil).
- **Deep-link:** düğüm id = `Ref.String()` → URL'de açık düğüm izi (paylaşılabilir).

## 8. Dahil edilen iyileştirmeler (tasarım gereği)

1. **Döngü koruması:** graf olduğu için visited-set + max derinlik (öner: 6) + düğüm
   başına çocuk cap'i (öner: 50, kalan `Elided`). Sonsuz açılımı önler.
2. **Tek veri kaynağı:** her şey View katmanı üstüne kurulur; paralel "özet toplayıcı"
   YAZILMAZ (iki kaynak zamanla çelişir — bu projenin tekrarlanan dersi).
3. **DOI pruning:** ekran dolunca "ilgisiz" dalları soldur/katla (focus+context).
4. **Sessiz kesme yok:** elision düğümde görünür.
6. **Maliyet görünürlüğü:** her düğümde `~N tok`; pahalı dal ajana/kullanıcıya belli.
7. **Stabil kimlik + deep-link:** `Ref.String()` cache anahtarı + URL + "aynı düğüm mü?".

## 9. Fazlar

- **Faz 1 — Backend:** ✅ (2026-08-06) yeni Kind'ler (`agent/budget/tools/category`),
  `ProjectAgent/Budget/Tools/Category`, `Children(ref)` metodu, `GET
  /api/views/{kind}/{id}/children` endpoint + testler (`view/*_test.go` fixture deseni +
  `fakeStore`). `go build ./... && go vet ./... && go test ./internal/view/...
  ./internal/api/...` ✅.
- **Faz 2 — Frontend:** ✅ (2026-08-06) `features/explorer` ekranı — React Flow lazy-expand
  (deterministik katmanlı yerleşim, elkjs eklenmedi), gömülü `ViewPanel` yan-özet paneli,
  semantic zoom (uzak zoom → tek satır), canlı SSE (`'explorer'` tick, yalnız
  açık dallar). NavRail "Harita" girişi + `api.viewChildren`. `tsc --noEmit` ✅.
  **Deferred → Faz 3:** URL deep-link (`useAppNavigation` entegrasyonu) ve `expand` ajan aracı.
- **Faz 3 — Agent/MCP + cila:** ✅ (2026-08-06) **`expand` aracı** (ajan, haritayla aynı
  backend + testler), **URL deep-link** (`#/w/{ws}/explorer/{refString}` — seçili düğüm
  geri-yüklenir; `useDeepLinks`/`useAppNavigation`/`url.ts`), **ekran-içi arama** (eşleşmeyeni
  soldurur), **DOI pruning** (kök-dışı seçimde odak = seçili + ataları + doğrudan çocukları,
  gerisi solar), **kök otomatik-açılım**. `go test` + `tsc --noEmit` + vitest ✅.
  **Ertelendi (opsiyonel):** MCP resource tree, sigma.js (büyük-workspace performansı).
- **TSK66 — Yeni kovacıklar + accordion:** ✅ (2026-08-06) root 6 → **11 node**
  (Artifacts/Otomasyonlar/Skill'ler/İçgörüler/Günlükler eklendi; Bütçe zaten vardı).
  Backend: `artifact/automation/skill/insight/logs` Kind'leri + 4 kategori, yeni
  projeksiyonlar (`artifact.go`/`automation.go`/`skill.go`/`insight.go`/`logs.go`),
  `Projector.WithSources(Sources{Skills,Findings,Logs})` (insight cycle'ı için view-local
  `InsightFinding` + api adapter), `Store`'a 4 salt-okunur metot, `views.go`'da
  `s.viewProjector(r)` kaynak bağlama. Frontend: `nextExpandedSet` accordion (sibling
  kapatma), `parentByKey` izleme, 5 yeni ikon+renk, `ViewKind` genişletmesi.
  `go build/vet/test ./internal/...` + `tsc -b` + `npm run build` + vitest ✅ + canlı
  API smoke (11 node, artifact/automation/skill/logs projeksiyonları).

## 10. Kabul kriterleri

- Root'tan başlayıp Oturumlar → bir oturum → koordinatör/worker'a **tıklayarak** inilebiliyor.
- 11 kök düğümü + `agent/budget/tools` özetleri deterministik (LLM yok), sayılar
  dashboard/billing ile tutarlı.
- Bir node açılınca aynı parent'ın diğer açık node'ları kapanır (accordion); farklı
  dallar açık kalır; collapse diğerlerine dokunmaz.
- Döngülü workspace'te (koordinatör↔worker) açılım sonsuza gitmiyor; elision doğru.
- Ajan `get_view`/`expand` ile aynı ağacı gezebiliyor (aynı backend).
- Yeni ağır bağımlılık yok (React Flow mevcut; elkjs eklendiyse gerekçeli).

## 11. Riskler

- Büyük workspace'te React Flow düğüm sayısı → lazy expand + cap ile sınırla; gerekirse
  Faz 3'te sigma.js.
- İki grafın (Ağ vs Harita) kullanıcı kafasında karışması → net adlandırma + farklı ikon;
  Ağ = ilişki, Harita = durum-drill-down.
- `tools`/`budget` projeksiyonu için veri erişimi → `Store` arayüzüne yeni metot
  gerekebilir (billing/settings). Leaf-package disiplini korunmalı.

## 12. Dosya haritası (özet)

- **Yeni (backend):** `internal/view/agent.go`, `budget.go`, `tools.go`, `category.go`,
  `children.go` (+ `*_test.go`). `view.go`'ya yeni Kind sabitleri. `api` view handler'ına
  `/children` route'u + `get_view`/`expand` aracı `internal/tools`.
- **TSK66 ek (backend):** `internal/view/artifact.go`, `automation.go`, `skill.go`,
  `insight.go`, `logs.go` (+ testler); `project.go`'da `Sources`/`WithSources` +
  view-local `InsightFinding`; `views.go`'da `s.viewProjector(r)`.
- **Projector kurulumu tek yerde** (2026-08-10): `internal/tools/viewprojector.go` →
  `tools.ViewProjector(db, wsName, ViewSources{Skills, Logs})`. Findings adapter'ı
  (`insight.FindingStore` → `view.FindingsSource`) da buraya taşındı; `api/views.go`,
  `api/dashboard.go`, `get_view` ve `expand` dördü de bunu çağırır.
  **Neden `tools` paketi:** skills + insight + logbuf + view'ı birlikte import eden tek
  paket o; `view` bunu kendi içinde yapamaz (insight → view, ters kenar döngü olur).
  **Kapattığı hata:** dört çağrı yerinden yalnız biri kaynakları bağlıyordu, dolayısıyla
  ajanın `get_view{kind:'skill'|'insight'|'logs'}` çağrısı "kaynak yok" derken kullanıcı
  aynı düğümü Harita'da dolu görüyordu. `ViewSources` bilerek **somut pointer** tutar:
  nil `*skills.Store` doğrudan arayüz alanına atanırsa arayüz nil OLMAZ ve projeksiyonun
  `== nil` koruması ıskalar.
- **Bütçe kademesi kategoride gerçek oldu** (2026-08-10): `ProjectCategory` yalnız
  `tiny`'yi ayırıyordu; `card` ve `full` **birebir aynı** DSL'i üretiyordu çünkü üyeler
  yalnız `Handle` olarak veriliyordu (panelde tıklanabilir, metinde görünmez). Artık
  `full` üyeleri satır satır yazar (`categoryFullMaxBytes` = 2400B ≈ 600 tok,
  `CapLines` + `Elided`), `card` handle-only kalır. Cap bilerek `categoryTopN`
  satırının toplam boyutunun (~3.1KB) **altında**: en büyük meşru girdinin
  ulaşamadığı bir cap dekordur, koruduğu elision yolu hiç çalışmaz.
  **Kalan yapraklar** (`skill`/`artifact`/`insight`/`schedule`/`agent`) `card` = `full`
  — küçük varlıkların söyleyecek fazlası yok, bu kasıtlı.
- **Kopyala butonu** artık aynı çıkan kademeleri **birleştirir** (`[CARD = FULL — …]`).
  Eskiden üç başlık altında üç özdeş blok yapıştırıyordu; okuyan "seviyeler yok
  sayılmış" diye okuyordu, oysa varlık gerçekten tek kademeye sığıyordu.
- **`get_view` kind listesi genişledi:** `expand`'in ref verdiği her düğüm artık
  `get_view` ile de okunabiliyor (`agent`, `budget`, `tools`, `logs`, `artifact`,
  `automation`, `skill`, `insight`, `category`). Eskiden şema beş kind'a kapalıydı;
  ajan `expand`'den aldığı ref'i açamıyordu.
- **Yeni (frontend):** `frontend/src/features/explorer/*`, `types` (ViewRef zaten var,
  yeni Kind literalleri), NavRail + viewRegistry girişi.
- **Doküman:** bu dosya + `00-GENEL-BAKIS.md` index satırı + iş bitince `05-ILERLEME.md`.
