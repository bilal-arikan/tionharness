# Harita odak grafiğinin tarihsel tasarımı

> **Özet (2026-10-03):** TSK487/TSK492 tek-hop React Flow odak grafiği ve ilk bileşen planıdır. Bu tasarımın yerini 2026-09-04'te vis-network aldı; aşağıdaki eski bileşen adları canlı dosya listesi değildir. Kaynak tasarım dosyaları ve erişilebilirlik kararları tarihsel referans olarak korunur. Güncel davranış [ana Harita rehberindedir](../68-OZET-HARITASI.md).

### 7.1 Eski tasarım (TSK487/TSK492, 2026-08-30 — tarihçe)

TSK487 ile ekran tek-hop odak grafiğine geçti: parent solda, focus ortada, child
sağda deterministik üç kolondur. Tek tık yalnız seçer ve mevcut `ViewPanel` detayını
günceller; çift tık odağı değiştirir. Odak kartı accent yüzey/ring ile, seçili kart
warning ring ile ayrılır. Cycle, self-loop ve iki yönlü ilişkiler kesik kenar yanında
metin rozeti taşır; anlam yalnız renge bağlı değildir.

TSK492 erişilebilirlik sözleşmesi:

- React Flow düğümleri klavye odağı alır. Yukarı/aşağı aynı parent/focus/child
  katmanında, sol/sağ komşu katmandaki en yakın satıra gider. `Enter`/`Space` tek-tık
  sözleşmesi gibi seçer; `Shift+Enter` çift-tık sözleşmesi gibi graf odağını değiştirir.
- Graf odağı değişince düğüm adı ve alt bağlantı sayısı `aria-live` ile bildirilir.
  Odak/seçim kart üstünde metin rozeti ve `aria-label` ile de aktarılır; renk tek sinyal
  değildir. Döngü/self-loop/iki yönlü kesik kenarlar Türkçe görünür etiket ve açıklayıcı
  edge `ariaLabel` taşır.
- `+N` listesi modal drawer semantiği, ilk odak, `Escape`, Tab odağı çevrimi ve
  tetikleyiciye odak dönüşü sağlar. Dar ekranda seçili düğümün mevcut `ViewPanel` detayı
  aynı davranışlı drawer olur; kalıcı sağ panel graf alanını daraltmaz. MiniMap dar
  ekranda gizlenir.
- Global `prefers-reduced-motion: reduce` kuralı kart/drawer geçişlerini ve spinner
  animasyonlarını yaklaşık anlık hâle getirir.

Aktif/focus node açıldığında veya graf odağı değiştiğinde, node ölçümü tamamlandıktan
sonra viewport bu node'u merkeze alır; mevcut zoom değeri aynen korunur. Selection
olayı tek başına merkezlemeyi tetiklemez.

Canvas görünürlük eşiği yön başına `VISIBLE_RELATIONS_PER_SIDE = 15` olarak sabittir.
Child odağına ilerlerken aktif ancestor zinciri root'a kadar görünür kalır; zincir
parent taşma sınırına dahil edilmez. Ancestor'a dönünce eski suffix kaldırılır.
Derin URL ile açılışta backend yalnız doğrudan parent döndürdüğü için istemci parent
neighborhood'larını köke ulaşana kadar seri çözer. Ref-bazlı path guard döngüleri
sonlandırır; URL/focus değişiminde ayrı AbortController ve nesil kontrolü eski çözümün
yeni lineage'ı ezmesini önler. Köke bağlanamayan zincir görünür hata üretir.
Bu bir API veya veri sınırı değildir: model tam, sıralı remainder listesini korur;
parent ve child tarafında ayrı `+N` sentetik düğüm üretir. Düğüm tüm kalan handle'ları
scroll edilebilir drawer içinde açar; listedeki tek tık seçer, çift tık odağı değiştirir.
Self-loop yeni bir düğüm üretmez. Tasarım kaynağı `_Docs/design/explore-focus-graph.js`,
Forge belgesi `_Docs/design/explore-focus-graph.op`, görsel kanıt
`_Docs/design/explore-focus-graph.png` altındadır.

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
