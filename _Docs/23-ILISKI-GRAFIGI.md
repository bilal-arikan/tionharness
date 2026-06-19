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
- **Katman chip'leri (toolbar):** Görevler / Akışlar / Beceriler / MCP — her düğüm
  türü açılıp kapatılabilir (ajanlar her zaman görünür); gizli düğüme değen kenarlar
  da düşer. Varsayılan: Görevler + Akışlar açık, Beceriler/MCP kapalı (sade başlangıç).
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
- Toolbar'da istatistik (ajan/görev/akış/beceri/MCP sayısı) + ilişki türü lejantı + Yenile.

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
  (`workspaceToVis`, `memoryToVis`) + kenar/lejant/tür renk sabitleri.
- `components/graph/VisNetworkGraph.tsx` — vis-network sarmalayıcı: `Network`+`DataSet`
  yaşam döngüsü, forceAtlas2 fizik düzeni, **`density` prop'u** (itme/yay uzunluğunu
  ölçekler — canlı `setOptions`), seçim olayı, stabilize sonrası `fit`.
  `workspaceToVis(graph, visible)` katman filtresi alır.
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
