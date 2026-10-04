# 68 — Özet Haritası (Workspace Explorer / semantic-zoom drill-down)

> **Durum:** Faz 1-3 tamamlandı ✅ (2026-08-06) + **TSK66 genişletmesi** ✅ (2026-08-06:
> single-expand accordion + Artifacts/Otomasyonlar/Skill'ler/İçgörüler/Günlükler kovacıkları) +
> **TSK487 odak grafiği tasarımı** ✅ + **TSK492 klavye, responsive ve erişilebilirlik** ✅
> (2026-08-30) + **vis-network ağ görünümü** ✅ (2026-09-04: React Flow üç-kolon odak grafiği
> kaldırıldı; tüm workspace tek çağrıyla (`GET /api/views/graph`) yüklenir ve Ağ ekranındaki
> aynı fizik motoruyla merkezde workspace, çevresinde 11 grup, her üye kendi grubuna bağlı
> olarak çizilir — bkz. §7.0).
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

| İhtiyaç               | Mevcut karşılığı                                                      |
| --------------------- | --------------------------------------------------------------------- |
| Düğüm adresi          | `view.Ref{Kind, ID, Sub}` (`session:SES1`, `board:board#in_progress`) |
| Düğüm özeti           | `Projector.Project(ctx, ref, level) → View`                           |
| Çocuk düğümlere kenar | `View.Handle{Label, Ref, Level}` (drill-down işaretçisi)              |
| Semantic zoom (bütçe) | `Level` = `tiny/card/full`                                            |
| Ajan erişimi          | `get_view` aracı (Ref çözer, Handle takip eder)                       |
| Kırılma izi           | `ViewPanel.trail` (frontend'te zaten var)                             |
| Kök                   | `ProjectWorkspace` (`workspace` roll-up)                              |

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
├── Oturumlar (category)      → category:skind:<tür> (oturum türü grubu, 2026-09-05)
│                              → session:SES*  → (coordinator/worker alt düğümleri)
├── Akışlar (category)        → flowrun:RUN*  → (node alt düğümleri, Sub)
├── Pano (board)              → board#<sütun> → kart (task) düğümleri
├── Ajanlar (category)        → agent:AGT*    → o ajanın oturumları
├── Artifacts (category)      → artifact:ART* (yaprak — metadata projeksiyonu)
├── Otomasyonlar (category)   → automation:AUT* (yaprak — tetik/durum/hata)
├── Skill'ler (category)      → skill:<slug> (yaprak — katalog girişi)
├── İçgörüler (category)      → insight:FND* (yaprak — bulgu özeti)
├── Günlükler (logs, yaprak)  → process log kuyruğu inline
├── Bütçe (budget)            → budget#provider:<ad> (sağlayıcı başına yaprak, 2026-09-05)
└── Araçlar (tools)           → tools#group:<kategori> (yerleşik araç grubu) +
                                tools#mcp:<id> (MCP sunucusu) — yapraklar, 2026-09-05
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
   - `agent.go` — `ProjectAgent`: ajan kimliği + aktif oturum sayısı + son etkinlik;
     handle'lar → ajanın oturumları. Harcama **yok** (TSK374) — para yalnız `budget`
     view'ında.
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
   Odak grafiği için `GET /api/views/{kind}/{id}/neighborhood?sub=...` tek çağrıda
   `focus`, doğrudan `parents`, doğrudan `children`, `hiddenParentCount` ve
   `hiddenChildCount` döndürür. Bu endpoint backend'de parent/child cap uygulamaz;
   görsel taşma istemcide `+N` düğümü/drawer ile yönetilir. Mevcut `/children` ve
   ajan `expand` yolundaki güvenlik sınırı geriye uyumluluk için değişmez.
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

### 7.0 Güncel tasarım (2026-09-04): tek fizik ağı

Harita artık Ağ ekranıyla aynı `VisNetworkGraph` bileşenini (vis-network, sürekli
forceAtlas2 fiziği) kullanır; React Flow ve üç-kolon odak modeli kaldırıldı.

- **Veri:** `GET /api/views/graph` (`Projector.Graph`, `internal/view/graph.go`) kökten
  BFS ile ulaşılabilen **tüm** düğümleri ve parent→child kenarları tek seferde döndürür;
  cap yok, ziyaret kümesi döngü/self-loop'u keser, çıktı `Ref.String` sırasıyla
  deterministiktir. Skill/insight kaynağı yoksa o kovacık boş kalır, harita düşmez.
  `session → rota` ve `rota → bağlı varlık` kenarları da dahildir.
- **Yerleşim:** kök düğüm fizik için sabittir (`fixed`; sürüklenebilir) — sabit çapa
  olmayınca tüm alan sürekli kayıp dönüyordu. Harita `VisNetworkGraph`'ı `settle`
  prop'uyla açar: Ağ ekranının kalıcı hareketi (`minVelocity: 0`) yerine vis'in dinlenme
  eşiği (`0.75`) + daha sert sönüm (`damping 0.55`); simülasyon oturunca kendiliğinden
  durur, sürükleme veya yeni veri yeniden uyandırır. `explorerSeed.ts` radyal tohum verir (kök 0,0; 11 grup 340px halkada;
  her alt ağaç kendi grubunun açısal dilimi içinde, derinlik başına +190px). Fizik bunu
  düzeltir; `explorer:<workspaceId>` anahtarıyla konum + kamera `localStorage`'a yazılır
  (`VisNetworkGraph` `layoutId` prop'u — Ağ ekranının kayıtları ile çakışmaz).
- **Görsel dil (`explorerVis.ts`):** derinlik 0 büyük accent daire (kütle 12), derinlik 1
  gruplar renkli orta daire (kütle 4), üyeler Ağ'daki şekillerle (session=box,
  flowrun/rota=diamond, skill=star, automation=triangle, artifact=square,
  insight=hexagon). Kenar uzunluğu hub'a yakınlıkla artar; döngü/self-loop kesik +
  warning rengi. Etiketten `kind:ID` öneki soyulur, ref tooltip'te kalır.
- **Fizik modu `tree` (2026-09-05):** `VisNetworkGraph` `mode="tree"` = vis
  `repulsion` çözücüsü: çekim yalnız kenar yayları (parent↔child), itme yalnız
  `nodeDistance` (140/yoğunluk) yarıçapı içinde; merkezî çekim 0. Bir alt ağaç yalnız
  kendi ebeveynini ve komşularını hisseder. **Dinlenme kapalı (2026-09-05, kullanıcı
  kararı):** Harita `settle` geçmez; fizik Ağ'daki gibi sürekli akar (`minVelocity 0`).
  `settle` prop'u `VisNetworkGraph`'ta duruyor (stabilized → fizik kapalı, dragStart →
  açık) ama şu an kullanıcısı yok.
- **Alt düğüm rolleri (`nodeRole`, 2026-09-05):** pano sütunu = kare (sütun rengi),
  kart = kesik çerçeveli kutu (oturum kutusu düz kalır), araç grubu = altıgen,
  MCP sunucusu = üçgen (Ağ ile aynı), sağlayıcı = yeşil nokta. Araç grubu etiketi
  `toolMeta.ts` `CATEGORY_LABELS` ile Türkçeleşir; tooltip/arama listesi rol adını
  gösterir (Pano sütunu / Kart / Araç grubu / MCP sunucusu / Sağlayıcı).
- **Etkileşim:** **tek tık = seç + kameraya odakla + sağ panelde `◱ Özet`**
  (`focusNodeId`/`focusTick`: aynı düğüme ikinci tık da odaklar; veri yenilenmesi
  kamerayı geri çekmez; zoom asla düşürülmez). **Çift tık** oturum düğümünde sohbeti
  açar. Boş tuvale tık seçimi bozmaz. Yoğunluk kaydırıcısı fizik sıkışıklığını ayarlar.
- **Canlı katman (2026-09-05):** `GET /api/views/graph` `live[]` (çalışan oturumlar +
  worker bekleyen koordinatörler, sürücü ajanın ad/emoji/renk'i; kaynak `Sources.Live`
  = runtime liveness `RunningSet`) ve `meta{}` (oturum kind/agentId/tags/archived) taşır.
  `explorerLive.augmentLive` her canlı oturum için `agent:<AGT>#live:<SES>` avatar
  düğümü (circularImage, ajan rengi) + oturum→avatar kısa bağ üretir; oturum düğümü
  ajan renginde gölge ile **parlar** (`running` tam, `awaiting-workers` warning tonu).
  Oturum durunca bir sonraki yenilemede kayıt gider, avatar ve parlama silinir.
  Avatar tıklanınca panel ajan kartını gösterir (`panelRefFor`).
- **Filtre çubuğu (2026-09-05, Ağ'dan taşındı):** `ExplorerFilters` + `explorerFilter.ts`:
  katman çipleri (kökün 11 çocuğu; gizlenen kovacığın yalnız onun ulaştığı alt ağacı
  düşer), **Canlı** (yalnız çalışan oturumlar), tür / ajan / etiket (oturum `meta`'sı).
  Fasetler AND, faset içi OR; kökten ulaşılamayan düğüm atılır. `localStorage`
  (`tionharness.explorerFilter`) ile kalıcı. Kalıcılık GC'si (`canonicalNodeIds`)
  filtrelenmemiş grafı görür.
- **Alt düğümleri gizle/göster (2026-09-05):** sağ panelin üstünde, çocuğu olan her
  düğüm için katlama düğmesi (`useExplorerCollapse`; workspace başına `localStorage`).
  Katlanan düğüm kalır, yalnız onun üzerinden ulaşılan alt ağaç kaybolur (başka bir
  yoldan ulaşılan düğüm — ör. ajanı üzerinden bir oturum — görünmeye devam eder);
  tuvalde etiketine `[+N]` rozeti gelir. `applyExplorerFilter` `collapsed` kümesi alır,
  `childCounts` tam grafın çocuk sayısını verir.
- **Ekranında aç (2026-09-05):** sağ panelin üstündeki buton seçili düğümü sahibi
  olan ekranda açar (`explorerNavigation.screenForRef` → `App` `applyRoute`).
  Yoğunluk kaydırıcısı `useStoredDensity` ile kalıcıdır.
- **Korunanlar:** harita-içi arama (eşleşmeyen düğüm/kenar solar, sonuç listesi tık =
  odak), URL deep-link (`?node=`; haritada olmayan ref görünür hata + "köke dön"),
  dar ekranda `ExplorerDetailDrawer` (modal dialog semantiği). Klavye ok-gezinmesi ve
  `+N` taşma listesi kaldırıldı (canvas tabanlı; tüm düğümler zaten görünür).
- **Dosyalar:** `ExplorerView.tsx` (kabuk), `ExplorerSearch.tsx`, `ExplorerDetailDrawer.tsx`,
  `explorerVis.ts`, `explorerSeed.ts`, `useExplorerGraph.ts` (+ testler).

### 7.1 Tarihsel odak grafiği

TSK487/TSK492 React Flow odak grafiği, erişilebilirlik kararları ve eski bileşen
planı [tarihsel tasarım ekine](arsiv/68-OZET-HARITASI-ODAK-TASARIMI.md) taşındı.
Güncel ekran §7'deki vis-network ağıdır; `ExplorerGraph.tsx`, `ExplorerNode.tsx`
ve `SummarySidePanel.tsx` eski planın adlarıdır, canlı kaynak listesi değildir.

### 7.2 Canlı durum katmanı (2026-10-04)

Haritanın amacı ajana grafik vermek değil, **kullanıcının workspace'te şu an ne olup
bittiğini tek bakışta görmesi**. Farkındalık katmanı ([94](94-FARKINDALIK-VE-NOTLAR.md))
bu bilginin kaynağı, harita canlı yüzeyi. İlk dilim üç parça:

- **Olay → delta, yeniden çekme değil.** `eventToRefreshSignals` iki sinyal üretir:
  `explorer` (düğüm/kenar eklenip silinmiş olabilir: `session create/delete/tags…`,
  `board`, `notes`, `spawned`, `agent`, `flow`, `schedule`) tüm haritayı çeker;
  `explorer-live` (`chat`, `worker`, `task`, `awareness_digest`, meta-veri oturum
  op'ları) yalnız `GET /api/views/graph/live` ile parıltı, halka ve durum şeridini
  çeker. Düğüm kümesi yerinde kalır, fizik yeniden yerleşmez. `explorerAttention.ts`
  `mergeLive` ile katmanı mevcut haritanın üstüne bindirir.
- **Dikkat halkaları.** `GET /api/views/graph` ve `/graph/live` yanıtına
  `attention` (ref → `{level, reasons, at}`) ve `status` eklendi. Backend
  (`api/views_attention.go`) `awareness.CollectOpenLoops` ile aynı taramayı yapar:
  bekleyen soru ve engellenmiş koordinatör **warn**, takılmış oturum ve başarısız
  kart **danger**, son özette araç hatası **notice**. `explorerVis.ts` düzeye göre
  halka çizer (danger: kalın kenarlık + geniş gölge, warn: aynısı uyarı renginde,
  notice: kesik kenarlık); araç ipucu nedenleri listeler. Halka canlı parıltısını
  kenarlıkta geçer: koşan ama takılmış oturum takılmış okunur.
- **Durum şeridi.** `ExplorerStatusStrip`: çalışan · seni bekleyen · takılan ·
  başarısız kart · eskiyen kart sayaçları, bugün yazılan not sayısı ve son özetin
  yaşı. Her sayaç bir süzgeç: basınca harita o düğümlere daralır
  (`ExplorerFilter.attention`, `applyExplorerFilter` oturum ve kartları süzer,
  yapıyı korur). Sıfır sayaç soluk ve basılamaz; boş harita "yüklenemedi" okunurdu.
- **Spot ışığı.** `changedKeys` iki yük arasında halkası veya parıltısı değişen
  düğümleri bulur; 10 sn geniş hale (`FLASH_MS`), sonra sönme. Kamera kendiliğinden
  kaymaz (takip modu kapalı, karar 4).

Veri akışı: `/graph/live` her istekte depodan ve kalıcı özet indeksinden türetilir;
ayrı bir dikkat dosyası yoktur, yenileme sonrası da doğrudur. Akış koşuları harita
düğümü olmadığından yalnız sayılır (`status.failedRuns`), halka almaz.

**Etkileşimli sorular (canlı LLM testinde bulundu).** Sohbet turundaki `ask_user`
kalıcı `SessionAsk` tablosuna yazmaz; yalnız oturum hub'ında `interaction_open`
olarak yaşar ve haritanın baktığı açık döngü taraması onu görmez. Bu yüzden
`interactionStore.pendingSessions` da "seni bekleyen" kaynağıdır ve soru açılınca,
cevaplanınca veya iptal edilince yük taşımayan `interaction` olayı
(`events.TypeInteraction`) yayınlanır; `eventToRefreshSignals` onu `explorer-live`'a
yönlendirir, toast üretmez (kartı oturum akışı zaten gösterir).

Sonraki dilimler (onaylı plan): köken kenarları (not → yazan oturum, oturum →
artifact) ve yan panelde akış sekmesi; ardından tazelik ısısı ve "değişti" noktası.

## 8. Dahil edilen iyileştirmeler (tasarım gereği)

1. **Döngü koruması:** lazy gezinmede visited-set + max derinlik sonsuz açılımı
   önler. Odak komşuluğu recursive traversal yapmaz; sonlu workspace snapshot'ında
   ref bazlı tekilleştirilmiş tek-hop kenarları döndürür. Kullanıcı görünür node
   sayısına backend ürün limiti uygulanmaz.
2. **Tek veri kaynağı:** her şey View katmanı üstüne kurulur; paralel "özet toplayıcı"
   YAZILMAZ (iki kaynak zamanla çelişir — bu projenin tekrarlanan dersi).
3. **DOI pruning:** ekran dolunca "ilgisiz" dalları soldur/katla (focus+context).
4. **Sessiz kesme yok:** elision düğümde görünür.
5. **Maliyet görünürlüğü:** her düğümde `~N tok`; pahalı dal ajana/kullanıcıya belli.
6. **Stabil kimlik + deep-link:** `Ref.String()` cache anahtarı + URL + "aynı düğüm mü?".

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
  backend + testler), **URL deep-link** (`#/w/{ws}/explorer/{refString}` — odak düğümü
  geri-yüklenir; geçersiz/workspace dışı ref açık hata + workspace kökü dönüşü sunar;
  `useDeepLinks`/`useAppNavigation`/`url.ts`), **ekran-içi arama** (eşleşmeyeni soldurur;
  sonuçta tek tık yalnız seçer, çift tık odaklar), **DOI pruning** (kök-dışı seçimde
  odak = seçili + ataları + doğrudan çocukları,
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
