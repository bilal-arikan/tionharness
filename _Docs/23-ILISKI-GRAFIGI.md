# 23 — İlişki Grafiği (Relation Graph)

Agent-MCP'nin "Multi-Agent Collaboration Network" görselleştirmesinden esinlenen,
entity'ler arası ilişkileri tek bakışta gösteren iki **salt-okunur** ağ görünümü.
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

## İki görünüm

### 1. Workspace Ağı ("Ağ" — NavRail)
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
    SwarmGo araçları ajan başına allowlist ile geçer, sunucu başına değil)
- **Katman chip'leri (toolbar):** Görevler / Akışlar / Beceriler / MCP / Geçmiş — her düğüm
  türü açılıp kapatılabilir (ajanlar her zaman görünür); gizli düğüme değen kenarlar
  da düşer. Varsayılan açık: Görevler + Akışlar + Geçmiş; Beceriler/MCP kapalı (sade
  başlangıç). ("Geçmiş" katmanı yalnız Canlı modda etkindir.)
- **Yoğunluk kaydırıcısı (0.4×–2×):** fizik itme + yay uzunluğunu canlı ölçekler —
  yüksek değer = daha sıkı paketleme, düşük = daha geniş yayılım.
- **Hafıza neden burada yok?** Bir ajanın hafızaları yüzlerce düğüm olabilir;
  workspace ağını boğmamak için hafıza ayrı **Hafıza → Ağ** grafiğinde gösterilir.
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
- Toolbar'da istatistik (ajan/görev/akış/beceri/MCP sayısı) + ilişki türü lejantı +
  **İlişki / Canlı** mod geçişi + Yenile. **Varsayılan mod: Canlı** (animasyonlu,
  olaylarda kendini yenileyen board akışı birincil görünüm; İlişki web'ine tek tıkla geçilir).

#### Canlı (live) modu
Toolbar'daki **İlişki | Canlı** geçişiyle açılan, board akışını canlandıran ikinci yerleşim:
- **5 sabit sütun başlığı** üstte (Yapılacak→Başarısız, `physics:false` — solver taşımaz
  ama kullanıcı sürükleyebilir; `fixed` kullanılmaz, bkz. "Sabit alanlar sürüklenebilir").
- Her görev **kendi durum sütununa** yaylanır (`task→col` kenarı, kesik çizgi) →
  görevler durumlarına göre sütun altlarında kümelenir.
- **Aktif bağ:** bir ajan yalnız **şu an çalıştığı** göreve bağlanır — `owns` kenarı
  **ve** görev `in_progress` ise. Parlak accent kenar + gölge. Görev durum değişince
  (board'da taşınınca) bağ kopar, ajan serbest kalır; başka görev `in_progress`
  olunca yeni bağ kurulur. Skill/MCP bağları ajanla kalır (onunla sürüklenir).
- **Gerçek zamanlı:** Canlı modda panel `/api/events` SSE'ye abone olur; görev/zamanlama
  olaylarında grafiği yeniden çeker. `VisNetworkGraph` DataSet'i **artımlı**
  (diff ekle/güncelle/sil, konum sıfırlamadan) güncellediği için fizik motoru ajanı yeni
  bağına **kaydırarak animasyon** yapar — "canlı akış" hissi buradan gelir.
- **Sabit alanlar sürüklenebilir:** Sütun başlıkları, "Boşta" ve "Geçmiş" çekirdekleri
  `physics:false` (fizik solver'ı onları **hareket ettirmez**) ama `fixed` **kullanılmaz**
  → kullanıcı **sürükleyebilir** ve bıraktığı yerde kalır. Artımlı güncelleme mevcut
  node'ların x/y'sini koruduğu için (yalnız yeni node'a konum verilir) canlı yenileme
  sürüklenen konumu **eski yerine sıçratmaz**.
- **Boşta (idle) lobi:** alt-soldaki "Boşta" çekirdeği; aktif görevi olmayan
  her ajan zayıf bir yayla buraya çekilir. Görev `in_progress` olunca güçlü aktif bağ
  ajanı kartına çeker (idle yayını ezer), görev bitince ajan lobiye geri döner.
- **Ajanın akışı/skill/MCP'si:** Canlı modda `uses` (akış→ajan), `skill` ve `mcp`
  bağları korunur → ajanın bağlı olduğu akış/beceri/sunucu onunla birlikte sürüklenir.
  Görev/sütun yapısaldır; flow/skill/MCP katmanları chip'lerle açılıp kapatılır.
- **Geçmiş (arşiv) çekim noktası:** Sağ-alttaki sabit "Geçmiş" çekirdeği; **Aktivite
  (executions) ekranının birebir aynısı** — çalışmayan **tüm** oturumlar
  (chat/task/flow/schedule) buraya toplanır. Her run **başlıklı kart** olarak
  gösterilir (kanban kartı gibi, tür-renkli kenar: chat=mavi, flow=mor, schedule=cyan,
  task=slate; hover'da tür+ajan). Backend `graphNode` tip `run` olarak
  son ~30 bitmiş oturumu döndürür (`runKind`+başlık+ajan adı; **agent zorunlu değil** —
  ajansız flow oturumları da dahil, executions feed'iyle aynı). Yalnız Canlı modda
  görünür ("Geçmiş" katman chip'i). Çalışan run aktif bağ alır, bitince "Geçmiş"e kart
  olarak düşer — iş akışı görünür biçimde arşive akar.
  > Doğrulandı: MINIMAX'te `/api/executions` (non-running)=13 ↔ graph `run` node=13
  > (schedule 2 / chat 7 / flow 4) — birebir eşleşme.
- **Canlı run / "şu an çalışıyor":** Backend `/api/graph` ajan düğümüne `running`+
  `runKind`+`runTarget` ekler (process-wide çalışan oturum kümesi `s.runs.activeSessionIDs()`
  → bu workspace'in oturumlarına join). Canlı modda: çalışan ajan **parlak "live"
  glow** alır; oturumu task/flow-kind ise o **task/flow düğümüne aktif (accent) bağ**
  kurulur. Aktif bağ ayrıca `in_progress` görev sahipliğiyle de (fallback) kurulur.
  > Çalışan kümesi iki kaynaktan birleşir: `s.runs.activeSessionIDs()` (chat
  > streaming turn'leri) **+** `Runtime.ActiveSessionIDs()` (otonom + flow run'ları).
  > **Flow run'ları artık tracker'a kaydoluyor:** `RunFlowRecorded` flow oturumunu
  > çalışmadan önce `GetOrCreateSourceSession` ile çözüp `trackSession`/`defer
  > untrackSession` ile sarmalıyor → flow çalışırken ilgili ajan `runKind=flow`,
  > `runTarget=flow:<id>` ile **aktif accent bağ + glow** alır, bitince temizlenir.
  > (Doğrudan task çalıştırma yolu henüz yok — task'lar flow-backed ya da otonom
  > çalışır; doğrudan run path eklenince aynı tracker'la otomatik kapsanır.)

### 2. Hafıza Bilgi Grafiği ("Hafıza" → Ağ sekmesi)
Bir ajanın hafızalarının benzerlik grafiği: her hafıza bir düğüm, lexical-cosine
benzerliği **eşik** üstündeki çiftler bağlanır.

- Hafıza ekranında **Liste / Ağ** geçişi (Ağ sekmesi vis-network'ü lazy yükler).
  **Varsayılan görünüm: Ağ** (bilgi grafiği birincil; Liste'ye tek tıkla geçilir).
- Yerleşim: vis-network `forceAtlas2Based` fizik — benzer hafızalar birbirini çeker,
  loose hafızalar eşit yayılır.
- **İçerik etiketi:** her düğüm kısa bir içerik önizlemesi gösterir (hangi hafıza
  olduğu bir bakışta okunur) — eskiden etiketsiz gri noktalardı.
- **Zengin tooltip:** tür rozeti + içerik + tarih (`tip()` HTMLElement).
- **Türe göre şekil/boyut:** yansıma (üst-düzey özet) = **yıldız + büyük**;
  belge/günlük = disk; boyut ayrıca bağ derecesiyle (degree) ölçeklenir → hub'lar büyük.
- Düğüm rengi hafıza türü (belge mavi / günlük slate / yansıma yeşil).
- Kenar kalınlığı/opaklığı benzerlik skoruyla orantılı; kenar hover'da "benzerlik: %X".
- **Benzerlik eşiği** kaydırıcısı (0.05–0.60): yoğun ağdan yalnız en güçlü bağlara süzme.
- **Yoğunluk kaydırıcısı (0.4×–2×):** fizik itme + yay uzunluğunu canlı ölçekler
  (workspace ağındaki ile aynı mekanik; `VisNetworkGraph density` prop'u).
- **Hover komşu vurgusu:** bir düğümün üstüne gelince yalnız o düğüm, doğrudan
  komşuları ve aralarındaki kenarlar parlak kalır; geri kalan her şey soldurulur
  (odak + bağlam). `VisNetworkGraph highlightNeighbors` prop'u; blur'da geri döner.
- **Tıkla → detay paneli:** bir hafızaya tıklayınca sağ üstte bir kart açılır —
  tür rozeti, **tam içerik** (kaydırılabilir), bağ sayısı ve tarih; ✕ ile kapanır.
  Düğüm seçimi `onSelect` → `selectedId` ile sürülür.
- **Tür çapaları (toggle):** Belge/Günlük/Yansıma için sürüklenebilir, fizik-bağışık
  çapa kutuları; her hafıza kendi tür çapasına yaylanır → bulut türe göre kümelenir
  (canlı moddaki sütun mekaniğiyle aynı). `memoryToVis({ kindAnchors })`.
- **Küme rengi (toggle):** benzerlik kenarları üzerinden union-find ile bağlı
  bileşenler (konu grupları) hesaplanır; ≥2 üyeli her bileşen altın-açı ile üretilen
  ayrı bir tonla boyanır (tekil düğümler tür rengini korur). `memoryToVis({ clusterColor })`.

## Backend

- `internal/memory/graph.go` — `Store.Graph(ctx, agentID, threshold, maxEdges)`:
  tüm hafızalar arası O(n²) pairwise cosine; eşik üstü kenarlar, skora göre
  sıralı + `maxEdges` ile cap; düğüm degree'leri hesaplanır. (`graph_test.go`)
- `internal/api/graph.go` — `registerGraphRoutes`:
  - `GET /api/graph` → workspace ağı (`workspaceGraph{nodes,edges,stats}`).
    Düğüm id'leri tür-önekli: `agent:` / `task:` / `flow:` / `skill:<slug>` / `mcp:<id>`
    (türler arası benzersiz). Akış→ajan kenarları `orchestration.ParseGraph` ile
    akış graf'ından; skill kenarları `Agent.Skills`'ten; mcp kenarları etkin
    `ListMCPServers` + `Agent.MCPEnabled`'dan çıkarılır. `stats` skills/mcp sayılarını da içerir.
  - `GET /api/agents/{id}/memory-graph?threshold=&max=` → hafıza grafiği
    (varsayılan threshold 0.18, max 400).

## Frontend

- `types/graph.ts` — `WorkspaceGraph`/`MemoryGraph` DTO'ları (barrel: `types.ts`).
- `api/graph.ts` — `graphApi.workspaceGraph()` / `memoryGraph()` (barrel: `api.ts`).
- `lib/relationGraph.ts` — DTO → vis-network `{nodes, edges}` eşleyiciler
  (`workspaceToVis(graph, visible, mode)`, `memoryToVis(graph, { kindAnchors, clusterColor })`)
  + kenar/lejant/tür renk sabitleri + yardımcılar (`connectedComponents` union-find,
  `clusterHue` altın-açı, `tip`/`fmtDate`/`truncate`).
- `components/graph/VisNetworkGraph.tsx` — vis-network sarmalayıcı: `Network`+`DataSet`
  yaşam döngüsü, forceAtlas2 fizik düzeni. Prop'lar: **`mode`** (`relation`|`live` —
  canlı modda merkez-çekimi düşük), **`density`** (itme/yay uzunluğunu ölçekler — canlı
  `setOptions`), **`highlightNeighbors`** (hover'da komşu-dışı düğüm/kenarları soldurur),
  **`onSelect`** (düğüm seçim callback'i). Artımlı DataSet güncellemesi (sürüklenen/fizik
  konumlarını korur), stabilize sonrası `fit`.
- `components/panels/NetworkPanel.tsx` — workspace ağı paneli (App'te lazy);
  İlişki/Canlı mod, yoğunluk kaydırıcısı, katman chip'leri, Canlı modda SSE aboneliği.
- `components/graph/MemoryGraphView.tsx` — hafıza grafiği (MemoryPanel'de lazy);
  eşik + yoğunluk kaydırıcıları, Tür çapaları / Küme rengi toggle'ları, tıkla→detay paneli.

> Eski React Flow tabanlı `RelationGraph.tsx`/`EntityNode.tsx` ve saf-TS force
> layout fonksiyonları (`forcePositions`/`workspaceLayout`/`memoryLayout`) vis-network
> geçişinde kaldırıldı.

## Code-split

Her iki giriş noktası (NetworkPanel, MemoryGraphView) vis-network'ü ayrı bir lazy
chunk olarak yükler (~515KB); ana bundle (~870KB) etkilenmez. Akışlar (React Flow)
hâlâ kendi ayrı chunk'ında.

## Durum

`go build`/`go vet`/`go test ./internal/...` + frontend `tsc`/`vite build` yeşil.
Canlı görsel (Chrome, MINIMAX workspace 4 ajan/32 görev/0 kenar): **Fizik modunda
forceAtlas2 ile homojen dağılım** doğrulandı — 4 ajan diski merkeze yakın, 32 görev
kutusu tuvale eşit aralıklarla yayıldı, üst üste binme yok. vis-network canvas
(1650×803) render edildi.

> Not: Kenarlar yalnızca veri varsa görünür — sahip ajanı/akışı/createdBy'ı
> atanmamış görevler ağda bağsız düğüm olarak çıkar (veri özelliği, hata değil).
> Fizik motoru bağsız düğümleri yine de eşit dağıtır.
