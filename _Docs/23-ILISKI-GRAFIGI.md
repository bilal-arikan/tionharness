# 23 — İlişki Grafiği (Relation Graph)

Agent-MCP'nin "Multi-Agent Collaboration Network" görselleştirmesinden esinlenen,
entity'ler arası ilişkileri tek bakışta gösteren iki **salt-okunur** ağ görünümü.
Görselleştirme, Agent-MCP'nin de kullandığı **`vis-network` (vis.js)** ile yapılır —
gerçek sürekli fizik motoru (sürükle/hover, canlı denge).

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
Tüm workspace'in işbirliği ağı: ajanlar (renkli disk), görevler (durum-renkli kutu),
akışlar (mor baklava); kenarlar ilişkileri gösterir.

- **Kenarlar (ilişki türleri, oklu):**
  - `owns` (yeşil) — ajan → sahip olduğu görev (`Task.OwnerAgentID`)
  - `created` (amber, kesik çizgi) — ajan → oluşturduğu görev (`Task.CreatedBy`)
  - `runs` (mor) — görev → çalıştırdığı akış (`Task.FlowID`)
  - `uses` (mavi) — akış → içindeki ajan node'ların ajanları (multi-agent sinyali)
- **Yerleşim — iki mod (toolbar'da "Fizik / Ağaç" geçişi):**
  - **Fizik (varsayılan):** vis-network `forceAtlas2Based` çözücüsü. Bu çözücü
    bağsız/seyrek graflarda bile düğümleri **eşit/organik (homojen)** bir buluta
    yayar (barnesHut'ın aksine kümeye çökmez/dağılıp uçmaz). `avoidOverlap` geniş
    görev kutularının üst üste binmesini engeller. Kenar varken bağlı düğümler
    doğal kümeler oluşturur.
  - **Ağaç:** `layout.hierarchical` (UD, directed) — fizik kapalı, hiyerarşik dizilim.
- Toolbar'da istatistik (ajan/görev/akış/bağ sayısı) + ilişki türü lejantı +
  **Fizik/Ağaç** + Yenile.

### 2. Hafıza Bilgi Grafiği ("Hafıza" → Ağ sekmesi)
Bir ajanın hafızalarının benzerlik grafiği: her hafıza bir düğüm, lexical-cosine
benzerliği **eşik** üstündeki çiftler bağlanır.

- Hafıza ekranında **Liste / Ağ** geçişi (Ağ sekmesi vis-network'ü lazy yükler).
- Yerleşim: vis-network `forceAtlas2Based` fizik — benzer hafızalar birbirini çeker,
  loose hafızalar eşit yayılır.
- Düğüm rengi hafıza türü (belge mavi / günlük slate / yansıma yeşil); boyut bağ
  derecesi (degree) ile ölçeklenir → hub'lar büyük görünür.
- Kenar kalınlığı/opaklığı benzerlik skoruyla orantılı.
- **Benzerlik eşiği** kaydırıcısı (0.05–0.60): yoğun ağdan yalnız en güçlü bağlara süzme.

## Backend

- `internal/memory/graph.go` — `Store.Graph(ctx, agentID, threshold, maxEdges)`:
  tüm hafızalar arası O(n²) pairwise cosine; eşik üstü kenarlar, skora göre
  sıralı + `maxEdges` ile cap; düğüm degree'leri hesaplanır. (`graph_test.go`)
- `internal/api/graph.go` — `registerGraphRoutes`:
  - `GET /api/graph` → workspace ağı (`workspaceGraph{nodes,edges,stats}`).
    Düğüm id'leri tür-önekli: `agent:` / `task:` / `flow:` (türler arası benzersiz).
    Akış→ajan kenarları `orchestration.ParseGraph` ile akış graf'ından çıkarılır.
  - `GET /api/agents/{id}/memory-graph?threshold=&max=` → hafıza grafiği
    (varsayılan threshold 0.18, max 400).

## Frontend

- `types/graph.ts` — `WorkspaceGraph`/`MemoryGraph` DTO'ları (barrel: `types.ts`).
- `api/graph.ts` — `graphApi.workspaceGraph()` / `memoryGraph()` (barrel: `api.ts`).
- `lib/relationGraph.ts` — DTO → vis-network `{nodes, edges}` eşleyiciler
  (`workspaceToVis`, `memoryToVis`) + kenar/lejant/tür renk sabitleri.
- `components/graph/VisNetworkGraph.tsx` — vis-network sarmalayıcı: `Network`+`DataSet`
  yaşam döngüsü, fizik/ağaç seçenekleri, `physics`/`tree` layout prop'u, seçim olayı,
  stabilize sonrası `fit`.
- `components/panels/NetworkPanel.tsx` — workspace ağı paneli (App'te lazy).
- `components/graph/MemoryGraphView.tsx` — hafıza grafiği (MemoryPanel'de lazy).

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
