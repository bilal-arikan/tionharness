# 23 — İlişki Grafiği (Relation Graph)

Agent-MCP'nin "Multi-Agent Collaboration Network" görselleştirmesinden esinlenen,
entity'ler arası ilişkileri tek bakışta gösteren **salt-okunur** workspace ağ görünümü.
Görselleştirme, Agent-MCP'nin de kullandığı **`vis-network` (vis.js)** ile yapılır —
gerçek sürekli fizik motoru (sürükle/hover, canlı denge, forceAtlas2 ile homojen yayılım).

## Kütüphane seçimi

İlk sürüm React Flow + saf-TS force simülasyonuyla yapıldı; ancak ölçek/yoğunluk
ayarı kırılgandı. Agent-MCP dashboard'unun `package.json`'ında graf için
**`vis-network` v9.x** kullanıldığı doğrulandıktan sonra biz de **`vis-network`
(v10.1.0) + `vis-data` (v8.0.4)**'ya geçtik. vis-network'ün yerleşik fizik
çözücüsü düğüm aralığını otomatik dengeler — referanstaki homojen görünümün
kaynağı budur. Maliyet: ~515KB ek (yalnız grafik görünümleri açılınca lazy yüklenen
ayrı chunk; ana bundle'a binmez).

## Workspace Ağı ("Ağ" — NavRail)

> ### Ajanlar = çalışan **örnekler** (2026-08-01)
> Ağda ajan **tanımı** değil, **canlı kapsamdaki oturum** gösterilir: her çalışan
> oturum ile kendisi çalışmadığı hâlde doğrudan çalışan child worker'ı bulunan
> koordinatör oturumu (`liveScope: running | awaiting-workers`) için ajanın **bir
> kopyası** çizilir. Kapsanan türler: chat / task / flow / flow-coordinator /
> schedule / spawned (otomasyon+spawn) / worker / inbox. Aynı ajan üç oturum
> sürüyorsa **üç düğüm** olur; hiçbir şey çalışmıyorsa **hiç ajan düğümü olmaz**.
> Bu yüzden ajan düğüm id'si oturumu taşır: `agent:<agentID>#<sessionID>`.
> Düğüm `sessionId`/`agentId`/`runKind`/`runTarget` alanlarını da döndürür ve
> `sub` alanı örneği ayırt eden alt-başlıktır ("Görev · <oturum başlığı>").
> **Tıkla-aç:** bir ajan örneğine (veya Geçmiş run kartına) tıklamak o örneğin sürdüğü
> **oturumun transkriptini açar** (`NetworkPanel.onSelect` → `sessionIdFromNodeId` → App
> `onOpenSession` = `setView('chat')`+`selectSession`); tooltip'te "Oturum: SESxxx" +
> "↗ oturumu açmak için tıkla" ipucu → hangi ajanın hangi oturumda olduğu tek tıkla.
> Ajana bağlı tüm kenarlar (`owns`/`created`/`uses`/`skill`/`mcp`) **her canlı
> kopyaya** çoğaltılır; canlı kopyası olmayan ajanın kenarı hiç çizilmez. Skill ve
> MCP düğümleri de yalnız çalışan bir ajan onları kullanıyorsa görünür.
> Çalışan durum bekleyen-worker durumuna önceliklidir. `stats.agents` = canlı örnek
> sayısı, `stats.agentsTotal` = canlı kapsamdaki benzersiz ajan sayısı
> (başlıkta "N aktif ajan / M").

Tüm workspace'in işbirliği ağı. **Düğüm türleri / şekilleri:** ajan (renkli disk),
görev (**durum-renkli kare**; başlık altında etiket, **hover'da açıklama tooltip'i**),
akış (mor baklava), **beceri/skill** (sarı **yıldız**), **MCP sunucusu** (teal **üçgen**).
Tooltip'ler vis-network `title`'a verilen HTMLElement ile zengin (başlık + durum +
açıklama); görev açıklaması backend'de `graphNode.Desc` (`Task.Description`) olarak gelir.

- **Kenarlar (ilişki türleri, oklu):**
  - `owns` (yeşil) — ajan → sahip olduğu görev (`Task.OwnerAgentID`)
  - `created` (amber, kesik çizgi) — ajan → oluşturduğu görev (`Task.CreatedBy`)
  - `runs` (mor) — görev → çalıştırdığı akış (`Task.FlowID`)
  - `uses` (mavi) — akış → içindeki ajan node'ların ajanları (multi-agent sinyali)
  - `skill` (sarı) — ajan → kullandığı beceri (`Agent.Skills`); paylaşılan beceriler
    hangi ajanların örtüştüğünü gösterir
  - `mcp` (teal) — MCP-enabled ajan → etkin MCP sunucusu (kaba erişim sinyali;
    TionHarness araçları ajan başına allowlist ile geçer, sunucu başına değil)
- **Katman chip'leri (toolbar):** Görevler / Akışlar / Beceriler / MCP — her düğüm
  türü açılıp kapatılabilir (ajanlar her zaman görünür); gizli düğüme değen kenarlar
  da düşer. Varsayılan açık: Görevler + Akışlar + Geçmiş; Beceriler/MCP kapalı (sade
  başlangıç).
- **Facet filtre çubuğu (boards-benzeri, toolbar 2. satır — `NetworkFilters.tsx`):**
  arama (`/` ile odaklanır, TR-uyumlu fold) + **Ajan** / **Tür** (run-kind) / **Durum**
  (board sütunu) / **Etiket** açılır facet'leri (`FacetDropdown` yeniden kullanılır) +
  **Arşiv** toggle'ı. **Katman chip'leri ve yoğunluk kaydırıcısı aynı satıra birleşti**
  (ayrı satır değil): `NetworkFilters` bir `children` slotu alır, `NetworkPanel` katman
  chip'leri + yoğunluğu bu slota geçirir → tek toolbar satırı `[arama · facet'ler · Arşiv]
  | [katmanlar · Yoğunluk] ——— sayaç]`. Facet'ler AND, facet içi değerler OR ile birleşir. Filtreleme
  **istemci-tarafı saf fonksiyon** (`networkFilter.ts` → `filterGraph`): düğümü elerken
  ona değen kenarlar ve **kenarsız kalan skill/MCP** düğümleri de düşer; stats
  workspace toplamı olarak korunur. Sağda "N / M düğüm" sayacı + "N filtre ✕" temizle.
  Filtre hem İlişki hem Canlı modda çalışır; hiç eşleşme yoksa "Filtreye uyan düğüm yok"
  + temizle butonu. **Arşiv toggle varsayılan kapalı** → arşivlenmiş oturumlar gizli,
  açılınca görünür (arşiv run kartları kesik-kenar + 🗄 rozetiyle işaretli).
- **Yoğunluk kaydırıcısı (0.4×–2×):** sonraki yeni-düğüm stabilizasyonunda fizik
  itme + yay uzunluğunu ölçekler — yüksek değer = daha sıkı paketleme, düşük = daha
  geniş yayılım. Yoğunluk, tema ve lite değişiklikleri mevcut fiziği yeniden başlatmaz
  veya restore edilmiş düğümleri hareket ettirmez.
- **Yerleşim — fizik (forceAtlas2):** vis-network `forceAtlas2Based` çözücüsü.
  Bağsız/seyrek graflarda bile düğümleri **eşit/organik (homojen)** bir buluta
  yayar (barnesHut'ın aksine kümeye çökmez/dağılıp uçmaz). `avoidOverlap` geniş
  görev kutularının üst üste binmesini engeller. Kenar varken bağlı düğümler
  doğal kümeler oluşturur.
  > **Neden hiyerarşik/"Ağaç" modu yok?** İşbirliği ağı genel bir grafik:
  > çok sayıda bağsız görev + döngüsel kenarlar (`uses`: akış→ajan, `owns`'a ters) +
  > çok-ebeveynli düğümler içerir. vis-network'ün `hierarchical` düzeni temiz bir
  > DAG/ağaç ister; bu veride her bağsız görev ayrı kök olup üst sırayı doldurarak
  > "bozuk" görünür. Hiyerarşi gereken yer **Akışlar** ekranıdır (gerçek DAG). Bu
  > yüzden ağ yalnız fizik düzeni kullanır.
- **Kalıcı yerleşim:** Düğüm koordinatları workspace kapsamı ve yerleşim sürümü
  içeren `localStorage` anahtarında saklanır. Okuma sırasında kayıt şeması ile tüm
  `x`/`y` değerlerinin finite sayı olduğu doğrulanır; bozuk veya eski sürümlü kayıt
  kullanılmaz. Açılışta bütün düğümlerin kayıtlı konumu varsa fizik kapalı başlar.
  Yeni düğüm geldiğinde kayıtlı düğümler geçici sabitlenir ve yalnız yeni düğüm için
  kısa bir stabilization çalışır. Sürükleme ve fizik hareketleri açık sayfa boyunca
  storage'a yazılmaz; görünür düğümlerin son koordinatları ağ ekranından çıkışta,
  workspace değişiminde veya `pagehide` sırasında tek snapshot olarak kaydedilir.
  Geçici filtre/katman gizleme kayıtları silmez; stale koordinatlar yalnız kanonik,
  filtrelenmemiş workspace düğüm kümesine göre budanır. Writer'ın son okumasında
  gözlenen diğer-tab ekleme, güncelleme ve silmeleri three-way merge ile korunur.
  `localStorage` atomik compare-and-swap sunmadığından tam eşzamanlı
  read-modify-write/`setItem` işlemleri last-writer-wins olabilir.
- Başlık çubuğunda istatistik (ajan/görev/akış/beceri/MCP sayısı) + Yenile. Ayrı bir
  mod/gösterge satırı yok — **İlişki/Canlı mod geçişi ve "canlı" gösterge satırı UI'dan
  kaldırıldı**; ağ artık **daima Canlı** (animasyonlu,
  olaylarda kendini yenileyen board akışı). `NetworkPanel`'de `mode` sabit `'live'`;
  `relationGraph.workspaceToVis` hâlâ `mode` üzerinde dallanır (ilişki modu ileride geri
  gelebilir diye kod korundu), ama statik ilişki web'ine giden buton yok.

#### Canlı (live) modu
Ağ ekranının tek güncel yerleşimi, board akışını canlı gösterir:
- **Workspace board sütun başlıkları** üstte (`physics:false` — solver taşımaz ama
  kullanıcı sürükleyebilir; `fixed` kullanılmaz, bkz. "Sabit alanlar sürüklenebilir").
  Özel sütun tanımı varsa aynen kullanılır; tanım yoksa beş yerleşik sütun
  (Yapılacak→Başarısız) fallback olur.
- Her görev **kendi durum sütununa** yaylanır (`task→col` kenarı, kesik çizgi) →
  görevler durumlarına göre sütun altlarında kümelenir.
- **Aktif bağ:** bir ajan yalnız **şu an çalıştığı** göreve bağlanır — `owns` kenarı
  **ve** görev `in_progress` ise. Parlak accent kenar + gölge. Görev durum değişince
  (board'da taşınınca) bağ kopar, ajan serbest kalır; başka görev `in_progress`
  olunca yeni bağ kurulur. Skill/MCP bağları ajanla kalır (onunla sürüklenir).
- **Gerçek zamanlı:** `App.tsx` içindeki merkezi event dispatcher `/api/events` SSE
  akışını dinler; ilgili workspace olaylarını panelin refresh sinyaline dönüştürür ve
  panel grafiği yeniden çeker. `VisNetworkGraph` DataSet'i **artımlı**
  (diff ekle/güncelle/sil, konum sıfırlamadan) güncellediği için fizik motoru ajanı yeni
  bağına **kaydırarak animasyon** yapar — "canlı akış" hissi buradan gelir.
- **Sabit alanlar sürüklenebilir:** Sütun başlıkları, "Boşta" ve "Geçmiş" çekirdekleri
  `physics:false` (fizik solver'ı onları **hareket ettirmez**) ama `fixed` **kullanılmaz**
  → kullanıcı **sürükleyebilir** ve bıraktığı yerde kalır. Artımlı güncelleme mevcut
  node'ların x/y'sini koruduğu için (yalnız yeni node'a konum verilir) canlı yenileme
  sürüklenen konumu **eski yerine sıçratmaz**.
- **"Çalışıyor" çekirdeği:** alt-soldaki çekirdek; bağlanacak task/flow hedefi
  olmayan çalışan örnekler (chat / schedule / spawned / worker / inbox) zayıf bir
  yayla buraya çekilir, böylece boşlukta savrulmazlar. Hedefli örnekleri güçlü
  aktif bağ kendi kartına çeker. Hiç hedefsiz örnek yoksa çekirdek hiç çizilmez.
  (Eskiden "Boşta" lobisiydi; ağda artık boşta ajan bulunmadığı için amacı değişti.)
- **Ajanın akışı/skill/MCP'si:** Canlı modda `uses` (akış→ajan), `skill` ve `mcp`
  bağları korunur → ajanın bağlı olduğu akış/beceri/sunucu onunla birlikte sürüklenir.
  Görev/sütun yapısaldır; flow/skill/MCP katmanları chip'lerle açılıp kapatılır.
- **Canlı kapsam:** Tamamlanmış/geçmiş run düğümleri grafiğe alınmaz. Ajan ve oturum
  düğümleri aynı eligible session kümesinden üretilir; silinmiş ajan için orphan
  düğüm oluşmaz. `running` örnekler **parlak "live" glow**, `awaiting-workers`
  koordinatörler "Bekleyen" durumunu alır. Çalışan oturum kümesi
  `s.runs.activeSessionIDs()` → bu workspace'in oturumlarına join edilir. Örneğin
  oturumu task/flow-kind ise o **task/flow düğümüne aktif (accent) bağ** kurulur;
  aktif bağ ayrıca `in_progress` görev sahipliğiyle de (fallback) kurulur. Bağlar
  örnek bazlıdır → aynı ajanın iki kopyası aynı anda iki farklı göreve bağlanabilir.
  > Çalışan kümesi iki kaynaktan birleşir: `s.runs.activeSessionIDs()` (chat
  > streaming turn'leri) **+** `Runtime.ActiveSessionIDs()` (otonom + flow run'ları).
  > **Flow run'ları artık tracker'a kaydoluyor:** `RunFlowRecorded` flow oturumunu
  > çalışmadan önce `GetOrCreateSourceSession` ile çözüp `trackSession`/`defer
  > untrackSession` ile sarmalıyor → flow çalışırken ilgili ajan `runKind=flow`,
  > `runTarget=flow:<id>` ile **aktif accent bağ + glow** alır, bitince temizlenir.
  > (Doğrudan task çalıştırma yolu henüz yok — task'lar flow-backed ya da otonom
  > çalışır; doğrudan run path eklenince aynı tracker'la otomatik kapsanır.)

> **Not (2026-07-05):** Bu doküman eskiden ikinci bir görünüm (**Hafıza Bilgi
> Grafiği**) daha anlatıyordu; memory alt sistemi projeden tamamen kaldırıldığında
> o bölüm ile ilgili backend (`memory-graph`) ve frontend (`MemoryGraphView`) de çıktı.
> Aşağıdaki mimari yalnız **Workspace Ağı**nı kapsar.

## Backend

- `internal/api/graph.go` — `registerGraphRoutes`:
  - `GET /api/graph` → workspace ağı (`workspaceGraph{nodes,edges,stats}`).
    Düğüm id'leri tür-önekli: `agent:<agentID>#<sessionID>` / `task:` / `flow:` /
    `skill:<slug>` / `mcp:<id>` (türler arası benzersiz). Ajan düğümleri
    `buildAgentInstances(agents, sessions, liveScope)` ile üretilir — canlı kapsamdaki
    oturum başına bir düğüm + `agentID → örnek id'leri` indeksi; `addAgentEdges`
    ajana bağlı her kenarı bu indeks üzerinden tüm kopyalara çoğaltır. Akış→ajan
    kenarları `orchestration.ParseGraph` ile akış graf'ından; skill kenarları
    `Agent.Skills`'ten; mcp kenarları etkin `ListMCPServers` + `Agent.MCPEnabled`'dan
    çıkarılır (ikisi de yalnız canlı örneği olan ajanlar için).
    `stats` skills/mcp + `agents` (canlı örnek) / `agentsTotal` (canlı kapsamdaki
    benzersiz ajan) içerir ve yalnız filtrelenmiş canlı grafiği sayar.
    **Filtre için ek düğüm alanları:** `agentId` artık ajan örneklerine ek olarak
    **görev** (sahip = `OwnerAgentID`) ve **run-history** (oturumun ajanı) düğümlerinde de
    var; `archived` (backing oturum arşivli mi) ve `tags` (oturum/görev etiketleri) tüm
    ilgili düğümlerde döner → istemci-tarafı facet filtresi bunlarla süzer.
    Testler: `graph_instances_test.go`.

## Frontend

- `types/graph.ts` — `WorkspaceGraph` DTO'su (barrel: `types.ts`).
- `api/graph.ts` — `graphApi.workspaceGraph()` (barrel: `api.ts`).
- `features/network/relationGraph.ts` — DTO → vis-network `{nodes, edges}` eşleyici
  (`workspaceToVis(graph, visible, mode, boardColumns)`) + kenar/lejant/tür renk
  sabitleri + yardımcılar (`tip`/`truncate`). Ajan düğümünün etiketi iki satır:
  ad + canlı kapsam durumu ("Çalışan" / "Bekleyen"); tooltip aynı Türkçe
  durumu gösterir ve aynı ajanın kopyalarını ayırt eder.
- `features/network/VisNetworkGraph.tsx` — vis-network sarmalayıcı: `Network`+`DataSet`
  yaşam döngüsü, forceAtlas2 fizik düzeni. Prop'lar: **`mode`** (`relation`|`live` —
  canlı modda merkez-çekimi düşük), **`density`** (itme/yay uzunluğunu ölçekler — canlı
  `setOptions`), **`highlightNeighbors`** (hover'da komşu-dışı düğüm/kenarları soldurur),
  **`onSelect`** (düğüm seçim callback'i). Artımlı DataSet güncellemesi (sürüklenen/fizik
  konumlarını korur), stabilize sonrası `fit`; workspace/sürüm anahtarlı kalıcı
  koordinatları uygular, tam kayıtlı açılışta fiziği kapatır ve yeni düğümlerde kısa
  stabilization çalıştırır. Kalıcı snapshot yalnız unmount/workspace değişimi/`pagehide`
  çıkışlarında yazılır; tema ve yoğunluk seçenekleri fiziği yeniden açmaz.
- `features/network/networkLayoutStorage.ts` — yerleşim anahtarı, şema/finite sayı
  doğrulamalı okuma-yazma ve stale düğüm koordinatlarını budama yardımcıları.
- `features/network/networkFilter.ts` — **saf facet filtresi**: `NetworkFilter` modeli
  (`text/agentIds/runKinds/statuses/tags/showArchived`) + `filterGraph(graph,f)` (düğüm
  eleme → kenar/kenarsız-attachment budama) + `isNetworkFilterActive`/
  `countActiveNetworkFacets` + `KIND_LABEL` + TR-uyumlu `foldForSearch`.
- `features/network/NetworkFilters.tsx` — boards-benzeri facet çubuğu: arama + Ajan/Tür/
  Durum/Etiket `FacetDropdown`'ları (tasks/views'ten yeniden kullanılır) + Arşiv toggle +
  "N/M düğüm" sayacı; facet seçenekleri/sayıları **filtresiz graf'tan** türetilir.
- `features/network/NetworkPanel.tsx` — workspace ağı paneli (App'te lazy);
  tek Canlı görünüm, yoğunluk kaydırıcısı, katman chip'leri, **facet filtre satırı**
  (`filter` state → `filterGraph` → `workspaceToVis`), merkezî SSE yenileme
  sinyali (`useRefreshTrigger('network')`). `app/eventToRefreshSignals.ts`'te
  `chat`/`flow`/`schedule`/`spawned`/`worker`/**`task`**/`board`/`agent` olayları
  ağ sinyalini tetikler — çalıştırma başlayınca/bitince ajan düğümü **eklenip
  silindiği** için `task` de bu listede.

> Eski React Flow tabanlı `RelationGraph.tsx`/`EntityNode.tsx` ve saf-TS force
> layout fonksiyonları (`forcePositions`/`workspaceLayout`) vis-network
> geçişinde kaldırıldı.

## Code-split

Giriş noktası (NetworkPanel) vis-network'ü ayrı bir lazy chunk olarak yükler
(~515KB); ana bundle (~870KB) etkilenmez. Akışlar (React Flow) hâlâ kendi ayrı
chunk'ında.

## Durum

`go build`/`go vet`/`go test ./internal/...` + frontend `tsc`/`vite build` yeşil.
Canlı görsel (Chrome, MINIMAX workspace 4 ajan/32 görev/0 kenar): **Fizik modunda
forceAtlas2 ile homojen dağılım** doğrulandı — 4 ajan diski merkeze yakın, 32 görev
kutusu tuvale eşit aralıklarla yayıldı, üst üste binme yok. vis-network canvas
(1650×803) render edildi.

> Not: Kenarlar yalnızca veri varsa görünür — sahip ajanı/akışı/createdBy'ı
> atanmamış görevler ağda bağsız düğüm olarak çıkar (veri özelliği, hata değil).
> Fizik motoru bağsız düğümleri yine de eşit dağıtır.
