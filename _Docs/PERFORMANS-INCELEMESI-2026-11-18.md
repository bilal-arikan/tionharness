# TionHarness arayüz performans incelemesi — 18 Kasım 2026

> **Özet:** Arayüzü en çok yoran şey, ajan yazarken her şeyin yeniden çizilmesi. Akan
> mesaj durumu `App` kökünde tutuluyor ve `src/app/` içinde hiç `memo` sınırı yok. Bu
> yüzden saniyede 20 kez kenar çubuğu, başlık ve oturum listesi dahil tüm ağaç yeniden
> çiziliyor. Her karede de adımların JSON gidiş-dönüşü, Markdown'ın baştan parse
> edilmesi ve zorunlu layout gibi işler yapılıyor. Ölçülen en ağır ekran Harita
> (Explorer): CPU 4 kat yavaşlatılınca 3,4 sn ana iş parçacığı harcıyor ve 740 ms'lik
> uzun görev üretiyor. Açılış paketinde de iki somut kaçak var: `@xyflow/react` yanlış
> chunk kuralıyla başlangıç paketine giriyor, iki dilin bütün çevirileri de baştan
> yükleniyor. Açılışta 8 uç nokta 2–3 kez çağrılıyor; sohbet ekranında bile 1,2–4,5 sn
> süren harici araç sürüm kontrolü çalışıyor. Bu inceleme ürün koduna dokunmadı.

Önceki inceleme: [PERFORMANS-INCELEMESI-2026-09-29.md](PERFORMANS-INCELEMESI-2026-09-29.md).

## Yöntem

- **Paket:** `vite build` scratch dizinine alındı; `index.html`'in yüklediği ve
  modulepreload ettiği dosyalar tek tek ölçüldü.
- **Canlı profil:**
  - Ortam: makinedeki Playwright Chromium (`chromium-1243`), headless, Node'un yerleşik
    WebSocket'i üzerinden CDP. Yeni paket indirilmedi.
  - Hedef: kullanıcının çalışan `:8090` sunucusu (gömülü üretim UI), WS1 workspace'i,
    SES687 oturumu.
  - Ölçülenler (`Performance.getMetrics` deltaları): her görünüm için 4 sn'lik
    pencerede ScriptDuration, LayoutDuration, RecalcStyleDuration, TaskDuration ve
    `longtask` girdileri; soğuk açılış; 15 sn boşta bekleme.
  - Normal CPU ve 4× CPU yavaşlatması (`Emulation.setCPUThrottlingRate`) ile iki tur.
  - Yalnız hash rotaları gezildi; hiçbir düğmeye basılmadı, form gönderilmedi.
- **Kod denetimi:** üç paralel okuma-yalnız inceleme yapıldı (sohbet akışı, uygulama
  kabuğu, ağır paneller). En kritik iddialar kaynakta ayrıca doğrulandı.
- **Sınırlar:**
  - Ajan turu çalıştırılmadı, yani akış (streaming) maliyeti **canlı ölçülmedi**.
    O bulgular kod okumasıyla doğrulandı, süreleri tahmindir.
  - Bu makine hızlı; 4× yavaşlatma orta sınıf bir dizüstüne yaklaşık bir karşılıktır.
  - Taze tarayıcı profili kullanıldı; localStorage boştu.

## Ölçümler

### Açılış (sohbet oturumuna doğrudan giriş)

| Ölçüm | Normal CPU | 4× CPU |
|---|---:|---:|
| DOMContentLoaded | 567 ms | 453 ms |
| First Contentful Paint | 1.584 ms | 1.156 ms |
| Ana iş parçacığı (TaskDuration) | 771 ms | 2.032 ms |
| Uzun görevler | 91 / 95 / 77 ms | 212 / 221 / 187 / 59 / 58 ms |
| JS aktarımı | 626 KB | — |
| API çağrısı | 32 | — |

FCP, DOM hazır olduktan yaklaşık 1 sn sonra geliyor. İlk boyamanın veri veya font
bekleyip beklemediği bu turda incelenmedi (açık soru).

Paket (scratch build): ilk açılışta zorunlu JS **1,55 MB ham / ~490 KB gzip**. Parçalar:

| Parça | Ham | Gzip |
|---|---:|---:|
| `components-*.js` | 649 KB | 210 KB |
| `vendor-react-*.js` | 390 KB | 121 KB |
| `index-*.js` | 193 KB | 57 KB |
| `vendor-markdown` | 162 KB | 48 KB |
| `vendor-highlight` | 154 KB | 52 KB |

CSS 132 KB. 29 Eylül'de 1,566 MB'tı; neredeyse değişmedi.

### Görünüm geçişleri (4 sn pencere, 4× CPU)

| Görünüm | Ana iş parçacığı | Script | Layout | Uzun görevler |
|---|---:|---:|---:|---|
| **Harita (explorer)** | **3.437 ms** | 2.359 ms | 156 ms | 740, 410, 101, 100, 81, 67 |
| **Artifacts** | **1.405 ms** | 485 ms | 516 ms | 574, 298 |
| Sohbet | 828 ms | 415 ms | 97 ms | 270, 120, 103 |
| Ajanlar | 808 ms | 222 ms | 302 ms | 159, 129 |
| Workspace | 688 ms | 169 ms | 97 ms | 76, 54 |
| Araçlar | 664 ms | 324 ms | 65 ms | 79, 73 |
| Skills | 607 ms | 180 ms | 188 ms | 58 |
| Diğerleri (panel, board, bütçe, rota, notlar, market…) | 273–488 ms | — | — | ≤58 |

Normal CPU'da Harita 643 ms, Ajanlar 266 ms, Artifacts 264 ms; diğerleri 47–199 ms.

Ağ tarafı:
- Ajanlar ekranında `/api/agents/{id}/tools` 2,5 sn sürdü.
- Araçlar ekranı 439 KB, Ajanlar ekranı 210 KB API verisi çekti.
- 15 sn boşta: yalnız üç yedek sorgu (`activity`, `executions/runtime`,
  `workspaces/activity`), ana iş parçacığı 74 ms. Boşta sorun yok.

## Bulgular — etkiye göre sıralı

### 1. Ajan yazarken tüm uygulama saniyede 20 kez yeniden çiziliyor — yüksek

Kaynak: `messages` durumu `useSessionsController` → `useTranscript` içinde, `App`
kökünde (`app/useSessionsController.ts:154`). Canlı balon 50 ms'de bir senkronlanıyor
(`features/chat/chatStreamHub.ts:172`); sohbet görünmüyorken bu aralık 500 ms.

`src/app/` içinde ve `SessionsSidebar`'da hiç `memo` yok. Bu yüzden her karede şunlar
baştan çiziliyor:
- `NavRail`, `AppHeader`, `MobileNavBar`;
- `SessionsSidebar`: sanallaştırılmamış 100+ satır; her satırda `resolveSessionOwner`,
  `relativeTime`, `AgentAvatar`;
- açıksa `SessionDetailPanel`;
- 1.041 satırlık `Composer`.

Aynı karede yapılan ek işler:

| Ne | Nerede | Maliyet |
|---|---|---|
| Canlı adımlar `JSON.stringify` ile gönderiliyor, sonra `parseSteps` ile geri açılıyor. Her adım yeni nesne olduğu için `ActivityCard` memo'su hiç tutmuyor. | `chatStreamHub.ts:158`, `AssistantTurn.tsx:113`, `ActivityCard.tsx:108` | Turun toplam boyutuyla büyür; tur boyunca karesel |
| Edit adımlarında LCS diff (≤400k hücre) iki kez hesaplanıyor; `DiffCard` memo'suz. | `TurnSteps.tsx:95`, `DiffCard.tsx:44` | Edit adımı × kare × 2 |
| `latestTodos` son todo'yu bulana kadar transkripti geriye doğru parse ediyor (150 mesaja kadar `JSON.parse`). `transcriptPaging` her render'da yeni nesne olduğundan `useMemo` da tutmuyor. | `ChatView.tsx:133`, `todos.ts:30`, `useTranscript.ts:204` | Her kare |
| Canlı yanıtın Markdown'ı her karede baştan parse ediliyor (react-markdown + remark-gfm + yol regex'leri). Dili belirtilmemiş açık kod bloğunda `hljs.highlightAuto` bütün dilleri deniyor. | `Markdown.tsx:169`, `CodeBlock.tsx:34` | Yanıt başına O(n²) |
| `agentById` her çağrıda yeni nesne döndürüyor (`resolveAgent`). Bu, `AssistantTurn`/`PeerTurn` satır memo'sunu bozuyor; ekrandaki her satır her karede yeniden çiziliyor. | `MessageList.tsx:154`, `shared/lib/agentLookup.ts:44` | Her kare ve her kaydırma |
| Büyüyen satır `ResizeObserver` → `setRevision` ile ikinci bir liste render'ı tetikliyor. `updateActivePinned`, `scrollTop` yazımından hemen sonra `getBoundingClientRect` okuyup zorunlu layout yapıyor; commit başına 2 kez. | `useTranscriptWindow.ts:100`, `MessageList.tsx:240`, `:316` | Kare başına ~4 zorunlu layout |
| `useReferencedAgents` transkript üzerinde `flatMap`, `Set`, sıralama ve `JSON.stringify` yapıyor. | `useSessionsController.ts:156`, `useReferencedAgents.ts:13` | Her App render'ı |
| Taslak rozetleri için `getSnapshot` her render'da bütün localStorage anahtarlarını tarıyor; her tuş vuruşunda da çalışıyor. | `shared/lib/sessionDrafts.ts:70`, `shared/hooks/useDraftSessionIds.ts:17` | Her render ve tuş |

Öneri sırası:
1. Akan transkripti küçük bir dış store'a taşı (`useSyncExternalStore`) ve yalnız
   `ChatView`/canlı satır abone olsun. Kabuk bileşenlerini `memo` ile sar ve
   `App.tsx:615-651, 951-955`'teki inline ok fonksiyonlarını `useStableCallback` ile
   sabitle.
2. Canlı `TurnStep[]` dizisini kalıcı olmayan bir alanla doğrudan geçir; JSON gidiş-dönüşünü
   kaldır. `synthDiffData` sonucunu adım nesnesine göre WeakMap'te önbellekle ve
   `DiffCard`'ı `memo` yap.
3. `agentById` için `useMemo`'lu Map; `transcriptPaging` nesnesini memo'la; `latestTodos`
   sonucunu mesaj başına önbellekle.
4. Canlı Markdown'ı bitmiş bloklara böl ve yalnız kuyruğu yeniden parse et. Açık kod
   bloğunda vurgulamayı atla.
5. Alttaki satır büyürken `setRevision`'ı rAF'a ertele; tekrarlanan `updateActivePinned`
   çağrısını kaldır.

### 2. Harita (Explorer) en ağır ekran — yüksek, ölçüldü

4× CPU'da 3,4 sn ana iş parçacığı ve 740 ms'lik uzun görev; normal CPU'da 643 ms.

- `settleNewNodes` mevcut her düğüm için ayrı `nds.update(...)` çağırıyor. vis-network her
  güncellemede kenarları yeniden bağladığından bu O(N×E) olay demek
  (`features/network/VisNetworkGraph.tsx:585-590`). Aynı dosyadaki
  `restoreTemporaryFixed` zaten tek toplu `update([...])` kullanıyor. **En kolay kazanç bu.**
- `graphToVis` arama, seçim, yanıp sönme ve dikkat durumuna bağlı. Arama kutusu debounce
  edilmediği için her tuş vuruşunda bütün düğüm/kenar dizisi baştan kuruluyor; sync
  efekti de tüm konumları okuyup hepsini güncelliyor (`features/explorer/useExplorerGraph.ts:266-293`,
  `ExplorerView.tsx:59`, `VisNetworkGraph.tsx:516-660`).
- Öneri: toplu `update`; aramaya ~200 ms debounce; yapısal eşlemeyi (görünürlük, layout,
  tema) ucuz stil yamasından (seçim, arama, flash) ayır ve yalnız değişen id'leri güncelle.

### 3. Açılış paketinde iki kaçak — orta/yüksek, ölçüldü

- **`@xyflow/react` başlangıç paketinde.** `vite.config.ts`'teki `manualChunks` kuralı
  `id.includes('/react/')` diyor; bu `node_modules/@xyflow/react/...` yolunu da yakalıyor.
  Sonuç: FlowsPanel bilerek lazy olduğu halde React Flow (kaynakta 228 KB + 152 KB
  `@xyflow/system`) ve CSS'i her açılışta modulepreload ediliyor.
  `vendor-react-*.js` içinde 47 ayrı `react-flow__*` sınıfı var. Düzeltme: tam paket yolu
  (`/node_modules/react/`, `/node_modules/react-dom/`, `/node_modules/scheduler/`).
- **İki dilin bütün çevirileri baştan yükleniyor.** `i18n/catalog.ts:18`
  `import.meta.glob('./locales/*/*.json', { eager: true })`: en 283 KB + tr 295 KB ham JSON,
  29 namespace. Bunlar 649 KB'lık `components-*.js` parçasının büyük kısmı. Dosyadaki
  yorum "birkaç yüz KB'ı geçince lazy'ye geç" diyor; eşik aşıldı. Düzeltme: yalnız açılış
  dilini eager yükle, diğerini `addResourceBundle` ile dil değişiminde getir.
- Hâlâ eager olan küçük parçalar: `SessionsSidebar` (32,7 KB), `OnboardingScreen`,
  `ClaudeAuthGate`, `WorkspaceRecommendations`.

### 4. Açılışta tekrarlanan ve yavaş istekler — orta, ölçüldü

Soğuk açılışta 32 API çağrısı yapılıyor. Tekrar edenler:

| Uç nokta | Kaç kez |
|---|---:|
| `/api/sessions` (37 KB) | 3 |
| `/api/workspace-settings` | 3 |
| `/api/agents` (63 KB) | 2 |
| `/api/artifacts` (44 KB) | 2 |
| `/api/sessions/{id}/messages` | 2 |
| `/api/settings` | 2 |
| `/api/sessions/active` | 2 |
| `/api/stt/status` | 2 |

`/api/live/stream` de iki kez açılıyor: ilki 22 ms sonra daha geniş kanal listesiyle
yenileniyor.

Sohbet ekranında bile ~1,8 sn sonra `/api/external-tools` ve `/api/external-tools/check-updates`
çağrılıyor. Biri 1,2–1,4 sn, gizli panelli canlı oturumda 4,56 sn sürdü. 29 Eylül
incelemesinin 2. maddesi (yerel sürüm alt süreci önbelleksiz) hâlâ geçerli.

Öneri: açılış veri yüklemelerini tek bir önyükleme adımında birleştir (ya da istek
birleştirme/dedup ekle). Öneriler ekranı açık değilken sürüm kontrolünü çalıştırma.

### 5. Değişmeyen veride de render — orta

- **`useAsync`:** her yoklamada `loading=true` ve `error=null` set ediyor, sonucu her
  zaman yeni referansla yazıyor (`shared/hooks/useAsync.ts:62-65`). Sonuçta istek başına
  ~2 App render'ı oluyor; `busyViews`/`runtimeById`/`busyWorkspaceIds` yeniden kuruluyor
  ve NavRail/kenar çubuğu memo'ları yeniden hesaplanıyor. Öneri: arka plan yenilemesinde
  `loading` gösterme, eşit sonucu yazma.
- **SSE dağılımı:**
  - Her kalıcı mesaj bir `message_activity` olayı üretiyor
    (`internal/workspace/manager.go:527-531`). Bu olay aktif oturum için `meterRefresh`'i
    birleştirmeden artırıyor (`app/useAppEvents.ts:134`). Sonuç: her mesajda tam
    `api.listArtifacts()` (`useSessionsController.ts:443-453`); detay paneli açıksa ayrıca
    `sessionInfo` + `sessionUsageDetail` (`SessionDetailPanel.tsx:102-125`).
  - Sohbet olayı da üç sinyali birden artırıyor ve oturum listesini + transkripti yeniden
    çekiyor (`eventToRefreshSignals.ts:79`).
  - `useWorkspaceActivity.ts:51` iki ayrı debounce'lu sinyali tek anahtarda birleştirdiği
    için bir olay 2 istek yapabiliyor.
  - Öneri: `meterRefresh`'i ~500 ms debounce et; artifacts'ı yalnız `artifact` olayında ya
    da tur sonunda çek.
- **Gizli pencere:** yoklama zamanlayıcıları gizliyken duruyor, ama sinyalle tetiklenen
  yenilemeler görünürlüğe bakmıyor (`useSignalRefresh.ts:13`, `useExecutionRuntime.ts:39-43`).
  Birden fazla pencere açıkken her olay her pencerede 3+ GET demek. Öneri: gizliyken "kirli"
  işaretle, görünür olunca tek yenileme yap.

### 6. Görünürlüğe bakmayan zamanlayıcılar — düşük/orta

| Yer | Aralık | Sorun |
|---|---|---|
| `workspace/ProcessPanel.tsx:236` | 1 sn | Çalışan süreç varken 300 satırlık tablonun tamamını yeniden çiziyor. Sayaç yalnız çalışan satırın hücresine inmeli. |
| `sessions/SessionDetailPanel.tsx:131-144` | 3 sn | Ham `setInterval`, görünürlük kontrolü yok. |
| `sessions/SessionDetailPanel.tsx:150-160` | 1 sn | Önbellek ısıdayken (her turdan sonra dakikalarca) bütün paneli yeniden çiziyor. |
| `logs/LogsPanel.tsx:206` | 30 sn | Ham `setInterval`; her yüklemede 1.000 satırı yeniden uzlaştırıyor (satırlar memo'suz). |
| `insight/InsightPanel.tsx:116` | 4 sn | Tarama sürerken çalışıyor, görünürlük kontrolü yok. |
| `SearchIndexPanel.tsx:84` | 2 sn | İndeksleme sürerken çalışıyor, görünürlük kontrolü yok. |

Hepsi için mevcut `useVisiblePoll` kullanılabilir.

### 7. Ölçülen ama nedeni çıkarılmayan ağır ekranlar — orta

- **Artifacts:** 4× CPU'da 1,4 sn ana iş parçacığı, 516 ms layout, 574 ms uzun görev;
  3.415 DOM düğümü. Bu turda kod nedeni incelenmedi; sıradaki profil adayı.
- **Ajanlar:** 808 ms, 302 ms layout; ayrıca arka uçta `/api/agents/{id}/tools` 2,5 sn.
  Ekran bu çağrıyı bekliyor.

### 8. Akış (Flow) editörü sürükleme — orta, yalnız editör açıkken

- Her sürükleme karesi bütün düğümleri klonluyor (`flows/FlowCanvas.tsx:342-360`,
  `flowGraph.ts:191`).
- `toReactFlow` her düğüme yeni `data` veriyor; bu yüzden `FlowNodeCard` memo'su hiç
  tutmuyor (`flowGraph.ts:164`).
- `FlowsPanel.tsx:146-147` her render'da `canonicalKey(draft)` (sıralama + bütün
  prompt'ların `JSON.stringify`'ı) ve `lint(draft)` çalıştırıyor.
- Öneri: `applyNodeChanges` ile yalnız taşınan düğümü değiştir; `dirty`/`layoutDirty`
  hesabını `useMemo`'la ya da sürükleme bitince hesapla.

### 9. Küçük maddeler — düşük

- **Sohbet kaydırması:** her scroll olayı rAF sınırlaması olmadan `setViewport` çağırıyor;
  1. bulgudaki kırık satır memo'suyla birlikte tur yokken bile ekrandaki her satır yeniden
  çiziliyor (`useTranscriptWindow.ts:49-77`).
- **Rota:** `layoutRota` her render'da memo'suz çalışıyor; `rootLanes` O(kök×oturum)
  (`rota/RotaPanel.tsx:169-181`, `shared/lib/laneModel.ts:72-91`).
- **Oturum araması:** sunucu sorgusu debounce'lu, ama yerel filtre her tuşta tüm listeyi
  yeniden çiziyor.
- **Kod küçükleri:**
  - `useRefreshTrigger.ts:21` her render'da yeniden abone oluyor (inline `subscribe`).
  - `useChatStream` her render'da yeni nesne döndürüyor.
  - `activeAsks` her render'da yeni `[]` döndürüyor.
  - Her `MermaidDiagram` kendi MutationObserver'ını `documentElement`'e bağlıyor.
- **Geliştirme modu:** `<StrictMode>` render'ları iki katına çıkarır; yavaşlık Vite
  (`:5173`) üzerinde değerlendiriliyorsa olduğundan kötü görünür. 29 Eylül ölçümünde Vite
  süreci 611–758 MiB tutuyordu.

## 29 Eylül bulgularının durumu

| Madde | Durum |
|---|---|
| Günlük kullanımda Vite | Değişmedi (kullanım tercihi; `dev.ps1` hâlâ Vite açar) |
| `check-updates` yerel sürüm önbelleği | **Hâlâ var**; sohbet açılışında da çalışıyor (4. bulgu) |
| İlk açılış JS'i | **Hâlâ büyük** (1,55 MB); yeni nedenleri 3. bulguda |
| Sohbet/oturum detayı/modalların eager import'u | **Düzeltildi** (`lazyChatPanels.ts`) |
| `useActivity` çift ilk yükleme | **Düzeltildi** (`useSignalRefresh`) |
| Sinyal yenilemelerinin görünürlüğü atlaması | **Hâlâ var** (5. bulgu) |
| Logs 30 sn / SessionDetail 3 sn / Insight 4 sn ham interval | **Hâlâ var** (6. bulgu) |
| `executions/runtime` 20 sn | Değişti: `useExecutionRuntime`, görünürlükte duruyor, 400 ms settle |

## Korunması gerekenler

Aşağıdakiler bilerek yapılmış ve iyi çalışıyor; kaldırılmamalı:

- **Sohbet akışı:**
  - 50/500 ms canlı balon birleştiricisi;
  - 150 mesajlık transkript penceresi;
  - ölçülen yüksekliklerle sanallaştırma (30+ mesajda) ve paylaşılan ResizeObserver;
  - steps string'ine bağlı `parseSteps` memo'su;
  - `Markdown`/`TurnSteps`/`CodeBlock` memo'ları.
- **Paketleme:**
  - ağır panellerin lazy yüklenmesi (`lazyPanels.ts`, `lazyChatPanels.ts`);
  - mermaid'in dinamik import'u ve elle gruplanmaması;
  - ikon kütüphanelerinde `import *` olmaması.
- **Graf ve paneller:**
  - VisNetworkGraph'ın Network'ü yeniden kurmadan DataSet diff'i ve unmount temizliği;
  - TaskBoard'ın memo'lu kartları ve debounce'lu araması;
  - Logs'un 1.000 kayıt sınırı ve `content-visibility`'si.
- **Boşta:** 15 sn'de yalnız üç küçük sorgu ve 74 ms iş yapılıyor; uygulama boştayken
  kaynak yemiyor.

## Önerilen uygulama sırası

1. **Hızlı ve düşük riskli** (her biri birkaç satır):
   - `manualChunks` yol düzeltmesi;
   - `settleNewNodes` toplu update ve Explorer aramasına debounce;
   - `agentById` Map'i ve `transcriptPaging` memo'su;
   - `meterRefresh` debounce'u;
   - ProcessPanel sayacını hücreye indirme.
2. **Akış yolu** (en büyük kazanç):
   - JSON gidiş-dönüşünü kaldırma, diff önbelleği, `latestTodos` önbelleği;
   - kabuk `memo` sınırları + sabit callback'ler;
   - sonra transkripti dış store'a taşıma.
3. **Açılış:**
   - yalnız aktif dilin çevirilerini yükleme;
   - açılış isteklerini tekilleştirme;
   - sürüm kontrolünü öneriler ekranına bağlama.
4. **Ölçüm:** Artifacts ve Ajanlar ekranlarını ayrıca profilleme; gerçek bir ajan turu
   sırasında React Profiler + CDP izi alıp 1. bulgunun etkisini sayıyla doğrulama.

Her adımdan sonra aynı CDP betiğiyle önce/sonra karşılaştırması yapılmalı.

## Uygulananlar (2026-11-18, 1. adım)

| Değişiklik | Dosya | Sonuç |
|---|---|---|
| `vendor-react` kuralı tam paket yoluyla eşleşir (`/node_modules/(react\|react-dom\|scheduler)/`) | `frontend/vite.config.ts` | React Flow ve CSS'i başlangıç paketinden çıktı; yalnız `FlowsPanel` parçasında. İlk açılış JS+CSS **1.683 KB → 1.489 KB ham, 512 KB → 453 KB gzip**; `vendor-react` 390 → 211 KB. |
| `settleNewNodes` sabitleme güncellemesi tek toplu `nds.update([...])` | `features/network/VisNetworkGraph.tsx` | Düğüm başına kenar yeniden bağlama döngüsü kalktı. |
| Harita vurgusu aramayı 200 ms gecikmeyle izler (sonuç listesi anında) | `features/explorer/ExplorerView.tsx` | Her tuşta tüm vis verisinin yeniden kurulması kalktı. |
| Ajan nesneleri kadro başına bir kez çözülür | `features/chat/MessageList.tsx` | Satır memo'su artık her karede kırılmıyor. |
| `transcriptPaging` memo'lu | `app/useTranscript.ts` | `ChatView`'daki todo memo'su her karede yeniden hesaplanmıyor. |
| `meterRefresh` saniyede en fazla bir kez (`useThrottledValue`) | `app/useSessionsController.ts`, `shared/hooks/useThrottledValue.ts` | Akan turda mesaj başına artifacts / oturum bilgisi yeniden çekilmiyor; son güncelleme yine geliyor. |
| Süreç tablosu sayacı yalnız çalışan satırın hücresinde, gizli sekmede durur | `features/workspace/ProcessDuration.tsx`, `ProcessPanel.tsx` | Saniyede bir 300 satırlık tablo yeniden çizilmiyor. |

Doğrulama: `tsc -b`, değişen dosyalarda eslint (yeni uyarı yok), vitest 1224/1224.
Harita ve sohbet üzerindeki etki canlı olarak yeniden ölçülmedi: çalışan `:8090` sunucusu
eski gömülü paketi sunuyor, yeni paket ancak backend yeniden derlenip başlatılınca devreye
girer.
