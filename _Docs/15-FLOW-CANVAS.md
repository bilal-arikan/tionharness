# 15 — Görsel Flow Builder (React Flow Canvas)

> Faz 7 orchestration akışlarının düzenleyicisi, form/liste editöründen **sürükle-bırak
> node-graph canvas**'a yükseltildi. Referans: React Flow tabanlı node-graph builder deseni,
> ComfyUI/LiteGraph bağlantı UX'i. Bkz. `_Docs/arsiv/14-PROVIDER-MIMARISI-INCELEME.md` çizgisi.

## Neden

Eski `FlowsPanel` node'ları üst üste kartlar olarak gösteriyor, bağlantıları `next`
dropdown'larıyla kuruyordu — graf topolojisi görünmüyordu. Yaygın node-graph builder'lar
React Flow canvas kullanıyor. Aynı kütüphaneyle, kodu kopyalamadan aynı deneyim kuruldu.

## UX düzeni güncellemesi (2026-07-04)

Editör yerleşimi sadeleştirildi:

- **Meta toolbar** yalnız: akış adı (geniş) + ID rozeti + **icon-only** "yolu kopyala" +
  "Aç" + **Kaydet**. **Açıklama (`description`) desteği uygulamadan tamamen kaldırıldı**
  (backend model/API/agent-araçları/tipler dahil — bkz. `_Docs\05` ilgili kayıt). Sol
  flow listesi açıklama yerine **flow id · N node** meta satırı gösterir.
- **Sol palet** iki bölüm: **Node ekle** (5 tip) + **Görünüm** (akış-bazlı etiket
  `TagEditor`, "Kablo" edge-style seçici, "Animasyon" toggle) — hepsi toolbar'dan
  buraya taşındı. Palet genişliği `w-40` (mobilde `w-32`).
- **Node editörü artık popup** (`ModalOverlay`), sabit sağ panel değil. Bir node'a
  **tıklayınca** (sürükleme değil) açılır: `FlowCanvas.onNodeClick` → `openNodeEditor`.
  Sürükleme/tıklama karışmaması için `nodeDragThreshold={4}` (birkaç px hareket = sürükleme,
  düz tık = popup). Boş canvas'a tık veya Escape/backdrop → kapanır. **Boyut board
  popup'ıyla (`TaskFormModal`) aynı:** `max-h-[90vh] w-full max-w-2xl rounded-xl`.
- **Palet sürükle-bırak:** node ekleme listesindeki her tip hem tıklanabilir hem
  **canvas'a sürüklenebilir**. `dataTransfer` MIME `FLOW_NODE_DND_MIME`
  (`application/tionharness-flow-node`); `FlowCanvas` içindeki `CanvasInner` (artık
  `ReactFlowProvider` altında ayrı bileşen) `onDrop`'ta `screenToFlowPosition` ile
  ekran noktasını graf uzayına çevirir → `onDropNode(type, pos)` → `FlowsPanel.addNodeAt`
  node'u **bırakılan konumda** oluşturur. Tık ile ekleme origin yakınına kaskad bırakır.
- **Node üstü `NodeToolbar` kaldırıldı** (FlowsPanel artık `nodeActions` geçmiyor →
  `NodeActionsContext` null → toolbar render edilmez). **Başlangıç / Çoğalt / Sil**
  eylemleri popup içindeki `NodeInspector` başlığına taşındı (`onDuplicate` prop'u eklendi).

## Mimari — backend'e neredeyse dokunmadan

Veri modeli (`orchestration.Graph{Start, Nodes[]}`) zaten yönlü graf olduğundan React Flow'un
`nodes[] + edges[]` modeline saf bir TS adapter katmanıyla eşlenir. Tek backend dokunuşu:
`orchestration.Node`'a opsiyonel **`X,Y float64`** (`json:"x,omitempty"`) — salt kozmetik
layout kalıcılığı; motor ve `Validate` bunları yok sayar.

```
FlowGraph JSON  --graphToReactFlow-->  nodes[] + edges[]   (canvas)
            ^                                   |
            +---------reactFlowToGraph----------+
```

### Edge semantiği (tek doğruluk kaynağı: kenarlar)

| Node tipi | Kaynak handle | Edge → hedef |
|-----------|---------------|--------------|
| `agent` | bottom (varsayılan) | `next` |
| `branch` | `b<i>` (sağda her dal) | `branches[i].next` (`contains` = inspector'da düzenlenir) |
| `parallel` | `fan` (bottom) | `parallel[]` çocuklar (çok hedefli) |
| `parallel` | `join` (sağda) | `joinNext` |

`reactFlowToGraph` rotalamayı kaynak handle'a göre kenarlardan geri kurar. `onConnect`:
tek-hedefli handle'lar (agent/branch/join) yeni bağlantıda eskisini siler; `fan` çok hedefe izin verir.

### Auto-layout

Pozisyonu olmayan node'lar (`x/y` undefined) `autoLayout` ile yerleşir: sütun = start'tan
BFS derinliği, satır = o derinlikteki sıra. Döngü guard'lı (ziyaret seti), her zaman sonlanır.
Canvas ile bir kez kaydedilince `x/y` kalıcılaşır; sonraki açılışlarda korunur.

## Dosyalar

**Frontend**
- `src/lib/flowGraph.ts` — adapter (`graphToReactFlow`/`reactFlowToGraph`), `autoLayout`,
  `nextNodeId`, `blankNode`.
- `src/components/flow/FlowCanvas.tsx` — `<ReactFlow>` + `Background`/`MiniMap`/`Controls`,
  `onConnect` bağlantı kuralları, `AgentsContext` sağlayıcı.
- `src/components/flow/NodeShell.tsx` — ortak node kabuğu (tip-renkli başlık + başlangıç
  rozeti + run-status ring).
- `src/components/flow/{AgentNode,BranchNode,ParallelNode}.tsx` — custom node tipleri (handle'lar).
- `src/components/flow/NodeInspector.tsx` — seçili node'un alanlarını düzenler.
- `src/components/flow/nodeStyles.ts` — `AgentsContext`, tip→renk/ikon, `statusRing`.
- `src/components/panels/FlowsPanel.tsx` — canvas + inspector + çalıştır entegrasyonu
  (`useNodesState`/`useEdgesState`).
- `src/types/flow.ts` — `FlowNode` += opsiyonel `x,y`.
- `package.json` — `@xyflow/react`.

**Backend**
- `internal/orchestration/model.go` — `Node` += `X,Y` (kozmetik) + `Graph` += `EdgeStyle`/`Animated`.

## UX cilası (2026-06-18)

- **Başlangıç/bitiş tonlaması:** başlangıç node'u hafif yeşil + "başlangıç" rozeti, terminal
  node'lar (giden kenarı yok) hafif mavi + "bitiş" rozeti. Bitiş tespiti `useIsEndNode` ile
  React Flow store'undan canlı okunur (`START_TINT`/`END_TINT`, `nodeStyles.ts`).
- **Düzen:** _(güncel: açıklama alanı kaldırıldı — 2026-07-04)_ node'lar **sol palet**ten
  tıklanarak/sürüklenerek eklenir (ikonlu liste); inspector'da ajan seçimi avatarlı
  `AgentPicker`, prompt alanı yüksek + monospace.
- **Node üzerinde ajan avatarı** (`AgentAvatar`).
- **Bağlantı noktaları (handle):** 12px, accent dolgu (`flowCanvas.css`); hover'da
  **tooltip** (`title`): "Giriş" / "Çıkış → sonraki node" / "Dal → <koşul>" / "Paralel dallar" / "Join".
- **Kablo (edge) sunumu — akış bazında kalıcı:** `Graph.EdgeStyle` (Eğri=default / Yumuşak=smoothstep /
  Basamak=step / Düz=straight) + `Graph.Animated` (animasyonlu akış). Toolbar'da "Kablo" seçici +
  "Animasyon" toggle; `FlowsPanel.saveFlow` graf'a yazar, `selectFlow` graf'tan okur (localStorage
  yalnız yeni/akışta-yok durumunda varsayılan edge stili için). `FlowCanvas` `styledEdges` ile
  her kenara `type`+`animated` uygular. Motor/Validate bu alanları yok sayar.
- **Controls + MiniMap koyulaştırıldı** (`flowCanvas.css`, MiniMap `bgColor=#0b0e14`).
- **Dev proxy düzeltmesi:** `vite.config.ts` proxy hedefi `localhost` → `127.0.0.1` (Windows'ta
  `localhost` önce IPv6 `::1`'e çözülüp Go backend'in IPv4 bind'ine 502 veriyordu).

Yeni/değişen dosyalar: `flow/NodeShell.tsx`, `flow/flowCanvas.css`, `agents/AgentPicker` & `AgentAvatar`
(yeniden kullanım), `types/flow.ts` (`FlowGraph.edgeStyle`/`animated`), `vite.config.ts`.

## Canlı koşu

`runFlowStreamStandalone` SSE `onNode`: `start` → node `data.status='running'` (parlama),
`done` → `status='done'` (yeşil ring), `error` → kırmızı ring + canlı listede `⚠️` satırı.
Kalıcı trace yine altta node-node liste olarak gösterilir (mevcut davranış korunur).

## Doğrulama (2026-06-18)

- `go build`/`go vet` yeşil; `tsc -b` + `vite build` yeşil (bundle ~988KB, React Flow nedeniyle;
  yalnız chunk-size uyarısı).
- **Playwright canlı test:** "Artı-Eksi Analizi" (paralel fan-out + join) akışı canvas'ta
  doğru render (⚡ paralel node + başlangıç rozeti, fan-out + `join` etiketli kenar, inspector
  seçili node'u düzenliyor), Controls + MiniMap çalışıyor. Gerçek claude-cli ile uçtan uca
  çalıştırma → **success**; trace `[parallel] Tartışma` (Lehte+Aleyhte) → `[agent] Sonuç`.
- **Pozisyon kalıcılığı:** API'den doğrulandı — akış canvas ile kaydedilince 4/4 node `x/y`
  taşıyor; kaydedilmemiş akışlar `x/y`'siz (auto-layout devrede).

## Doğrulama (UX cilası, 2026-06-18)

- `go build` + `tsc -b` yeşil. **Playwright canlı test:** edge stili "Basamak" + "Animasyon"
  açık kaydedildi → API'de `edgeStyle=step, animated=true` (yalnız o akışta; diğerleri boş),
  DOM'da edge sınıfı `react-flow__edge-step ... animated`, handle `title="Giriş"`, handle
  boyutu 12px. Başka akışa geçip dönünce UI graf'tan `step`+animasyonu yeniden yükledi
  (akış-bazlı kalıcılık uçtan uca).

## Code-split + Şablon galerisi (2026-06-18)

- **Code-split:** `FlowsPanel` artık `App.tsx`'te `React.lazy(() => import(...))` + `<Suspense>`
  ile tembel yüklenir. React Flow yalnızca **Akışlar** görünümü açılınca iner. Sonuç:
  ana bundle `~988KB → ~811KB`, ayrı `FlowsPanel-*.js` (~202KB) + `FlowsPanel-*.css` (~16KB) chunk'ı.
- **Salt-okunur şablon galerisi:** `features/flows/flowTemplates.ts` agent-bağımsız şablonlar
  (Sıralı Hat, Duygu Yönlendirici, Artı-Eksi, Eleştir-Düzelt, Planla-Uygula, Çoklu Uzman+Sentez,
  Generator↔Evaluator (GAN)). **Koordinatör deseni karşılıkları (M5, 2026-07-15):**
  Sınıflandır & Yönlendir (`classify-act`, 3-yollu branch), Üret & Süz (`generate-filter`,
  paralel üretim→süzme), Turnuva (`tournament`, 4 aday→yarı-final→final; parallel→parallel).
  `FlowsPanel` sol kolonunda **Akışlarım / Şablonlar** sekme geçişi; şablon seçilince
  `flow/TemplatePreview.tsx` salt-okunur canvas (`FlowCanvas readOnly` — `nodesDraggable`/
  `nodesConnectable`/`elementsSelectable` kapalı, `onConnect` yok) yapıyı önizler.
- **Şablondan oluşturma:** "Bu şablondan akış oluştur" → `instantiateTemplate` graf'ı kopyalar,
  **agent node'lara varsayılan ajanı (ilk ajan) atar** (backend `Validate` boş `agentId`'yi
  reddeder), `api.createFlow(name, desc, graph)` ile yeni akış kurar, "Akışlarım"a geçip açar.
  Şablonun `edgeStyle`/`animated` sunumu da kopyalanır.
- **Doğrulama (Playwright):** 6 şablon listelenir; read-only önizlemede node'lar `draggable`
  sınıfı taşımaz (5 node/4 edge render); instantiate sonrası API'de 5 node + `animated=true` +
  agent node'lar atanmış. (Not: backend değişmedi — yalnız frontend; `tsc -b` + `vite build` yeşil.)

## Canvas etkileşim özellikleri (2026-06-18)

- **Ok uçları:** tüm kenarlara `markerEnd: ArrowClosed` (yön netliği) — `FlowCanvas.styledEdges`
  + `defaultEdgeOptions`.
- **Paralel kenar renkleri:** `FlowCanvas.edgeColor(sourceHandle)` ile paralel node'un çıkışları
  ayırt edilir — **fan** kenarları (eşzamanlı çocuklar) sky `#0ea5e9`, **join** kenarı violet
  `#7c3aed` (paralel node aksanı + join tutamağıyla aynı). Kenar çizgisi **ve** ok ucu aynı renkte;
  `ParallelNode` fan tutamağı sky, join tutamağı violet. Yüklemede ve canlı bağlamada
  `sourceHandle`'a göre tek noktadan uygulanır. (agent/branch kenarları varsayılan renkte.)
- **Canvas araç çubuğu** (`FlowCanvas` içinde `<Panel position="top-right">` → `CanvasTools`,
  `useReactFlow` ile): **▦ Otomatik diz** (parent `onAutoLayout` → `autoArrange`:
  `reactFlowToGraph`→`autoLayout`→`setNodes`, sonra `fitView` ile ortalar). Ortala/zoom zaten
  sol-alt `Controls`'ta (Fit View + +/−) olduğundan ayrı "Ortala" butonu yok. Read-only
  önizlemede panel hiç render edilmez (`onAutoLayout` yok).
- **NodeToolbar** (seçili node üstünde, `NodeShell` + `NodeActionsContext`): **▶ Başlangıç yap**
  (başlangıç değilse), **⧉ Çoğalt** (yeni id + offset, bağlantısız kopya), **✕ Sil**. Aksiyonlar
  id-bazlı (`makeStartNode`/`duplicateNode`/`deleteNode`), `FlowsPanel`'den
  `nodeActions` prop'u ile sağlanır; inspector aynı çekirdeği kullanır. Read-only'de provider null
  → toolbar yok.
- **Doğrulama (Playwright):** edge `marker-end=url(#…arrowclosed…)`; panelde "▦ Otomatik
  diz"; node seçilince toolbar "▶ Başlangıç ⧉ Çoğalt ✕ Sil"; Çoğalt 4→5 node; Otomatik diz
  pozisyonları ızgaraya dizdi (0,0 → 280,0 → 280,140). (Ortala butonu sonradan kaldırıldı —
  Controls'ta zaten Fit View var.)

## Koşular sekmesi — salt-okunur koşu izleme (2026-06-18)

- `FlowsPanel` sol kolonuna **Koşular** (3. sekme) eklendi: tüm flow koşuları (devam eden + bitmiş)
  yeni→eski listelenir. `api.listAllFlowRuns()` (`GET /api/flow-runs`, flowId'siz — **backend
  değişmedi**) ile yüklenir ve sekme açıkken **3sn'de bir poll** edilir (devam eden koşular canlı
  ilerler; sekmeden çıkınca interval temizlenir).
  **(2026-07-28)** Liste varsayılan olarak yalnız **kök** koşuları gösterir
  (`listAllFlowRuns(true)` → `?rootOnly=true`); composed bir akışın subflow/spawn çocukları
  "alt koşuları göster" onay kutusuyla açılır. Seçilen koşu artık `RunTreeView` ile sarılıyor:
  solda **koşu ağacı** paneli (tek koşuluk ağaçta gizli), subflow/spawn node'unda çocuğun o anki
  adımını gösteren **rozet**, node'a **çift tıkla** alt koşuya iniş + breadcrumb.
  Ayrıntı: `_Docs/62-BIRLESIK-RUN-AWAIT.md`.
- Liste öğesi: durum rozeti (▶ devam ediyor / ✓ başarılı / ✕ hata) + akış adı (flowId→`flows`
  map; silinmişse "（silinmiş akış）") + zaman.
- Seçilince **`flow/RunView.tsx`** (salt-okunur): başlık (ad + durum + girdi + `run.error` ⚠️),
  **aşama göstergeli canvas** (`FlowCanvas readOnly` + `graphToReactFlow`; node `data.status`
  `nodeStatuses(run,state)` ile türetilir: `trace`'tekiler `done`, `state.current` çalışırken
  `running` / hata ise `error`), ve **adım izi** listesi (node çıktıları `Markdown`, branch düz
  metin). Seçili koşu `runs` listesinden türetildiği için poll ile canlı tazelenir.
- **Canlı node ilerlemesi (2026-07-09):** RunView artık 3sn poll'a ek olarak **global bus
  üzerinden per-node canlı olay** alır. `driveFlow` observer'ı her zaman sarmalanır →
  her `orchestration.NodeEvent` `emitFlowNode(runID, flowID, ev)` ile bus'a yayılır
  (`flow_node` tipi, `Target.flowRunId`; API SSE `flownode` kanalı — notify/badge yolunu
  kirletmez). Frontend: `flowNodeBus.ts` (runId-keyed) ← `useAppEvents.onFlowNode` ←
  `system.ts` üçüncü SSE callback'i; RunView `run.id`'ye abone → canvas node durumu **asla
  geri sarmadan** (pending→running→done/error) + biten node çıktıları "Adım izi"nde **anında**
  + çalışan node için "…çalışıyor" satırı. Poll caught-up olunca canlı ekler `nodeId` ile
  dedupe olur. **Tüm** koşular (UI/otonom/scheduled) yayar — eskiden node olayları yalnız
  koşuyu başlatan HTTP istemcisine gidiyordu (`obs=nil` otonom koşuda hiç canlı yoktu).
  **Not:** akış node'u session'sız `complete()` çağrısıdır → `session_step`/tool-adımı
  **canlı yaymaz**; bu yüzden canlılık **node-seviyesindedir** (chat/Aktivite'deki adım-seviyesi değil).
- **Koşu canvas'ı: seçilebilir + sürüklenebilir (2026-07-27):** `RunView` artık `FlowCanvas`'ı
  `readOnly` yerine yeni **`runMode`** ile çağırır. Eski `readOnly` node seçimini kapatıyordu
  (`elementsSelectable={false}`), bu yüzden node-tıkla inspector paneli koda rağmen açılamıyordu —
  düzeltildi. `runMode`: node **sürüklenebilir** (`nodesDraggable`) + **seçilebilir**
  (`elementsSelectable`) ama bağlantı/palet-drop/node-config kapalı (`editable = !readOnly && !runMode`).
  Sürükleme bitince (`onNodeDragStop`) `RunView.persistNodePosition` sadece taşınan node'un `x/y`'sini
  flow tanımına `updateFlow` ile yazar (koşu canlı flow graph'ını render eder, snapshot değil → düzen
  editörle paylaşılır). ~3s status/output poll'ünde node yeniden-inşa effect'i mevcut canvas
  pozisyonlarını korur → taşınan node geri sıçramaz.
- **Node-tıkla chat görünümü (2026-07-27):** `RunView`'de canvas'ta bir node'a tıklayınca alt
  panel düz "Adım izi" listesinden **`RunNodeInspector`** görünümüne geçer: agent node için
  **girdi mesaj balonu** (çözülmüş prompt) + **tool/düşünce adımları** (chat'in `TurnSteps`
  bileşeni) + **çıktı balonu** (assistant), chat mekanizmasının kendi bileşenleriyle
  (`UserBubble`/`TurnSteps`/`Markdown`). Adımlar node çalışırken **çöpe atılmıyor** artık:
  agent node yolu (`executor.go` `complete`/`completeThread`) `CompleteWithToolsTraced` ile
  `[]TurnStep` yakalar → **sidecar** dosyaya yazar (`<store>/flow_runs/<runID>/steps-<nodeID>.json`;
  state şişmesin diye State içinde DEĞİL — bkz. WS5/TSK64). `TraceEntry.Input` çözülmüş prompt'u
  taşır. API: `GET /api/flow-runs/{id}/nodes/{nodeId}/steps`. Node id ctx'e
  `orchestration.WithNodeID`, run id `withFlowRunID` ile taşınır. Non-agent node'lar (branch/delay/…)
  chat değil basit çıktı kartı gösterir. Çıktı balonunda hover'da **kopyala** butonu. Not:
  `NodeInspector` = flow **editörünün** node-config paneli; `RunNodeInspector` = koşu görüntüleyici.
  - **Canlı intra-node step streaming (Faz 3, yapıldı):** agent node yolu `CompleteWithToolsStream`
    ile her adımı anında yayar → `flow_node_step` olayı (SSE adı **`flownodestep`**,
    `Target{flowRunId,nodeId}`, `Step`=marshalled `TurnStep`) → `flowNodeStepBus` (runId-keyed) →
    `RunNodeInspector` çalışan node için canlı adım ekler. Node bitince sidecar refetch'i (running→done
    geçişinde) tam izi geri doldurur (inspector geç açıldıysa kaçan erken adımlar dahil). Emit `ctx`'ten
    runID+nodeID okur → flow-dışı yolda no-op.
  - **Paralel-çocuk trace'i (yapıldı):** `runParallel` artık her çocuk için `TraceEntry` (input+output)
    döndürür, `Run` bunları parent fold kaydından önce `st.Trace`'e ekler → paralel node'un
    çocuklarına da tıklanıp chat görünümü açılır. Çocuk step sidecar'ları zaten yazılıyordu
    (`runAgentNodeSafe` ctx'e child.ID koyar). Eski "paralel çocuklar trace'te yok" notu artık geçersiz.
  - **Node-tipine-özel inspector (yapıldı):** `RunNodeInspector` node tipine göre dallanır —
    **agent**: chat görünümü (+ accumulate modda **önceki bağlam** açılır bölümü, aşağıda); **branch**:
    **karar kartı** (değerlendirilen değer + eşleşme modu + tüm dallar, eşleşen ✓ yeşil, hedef node;
    `TraceEntry.Input`=`st.Last`, matched arm output label'ından türetilir); **parallel**: **fan-out**
    (çocuk listesi, tıkla→çocuğun kendi görünümü, `traceByNode` + `onSelectNode`) + **eşzamanlılık
    timeline'ı** (her çocuk göreli çubuk; kritik yol=en geç biten kırmızı; `TraceEntry.StartMs`/`EndMs`
    unix-ms `runParallel`'de goroutine içinde ölçülür — yalnız paralel çocuklarda dolar); diğerleri: basit kart.
  - **Accumulate-thread gösterimi (yapıldı, indeks yöntemi):** her agent node çalışmadan önce
    gördüğü thread uzunluğu `TraceEntry.ThreadLen` olarak kaydedilir (snapshot YOK — thread zaten
    `State.Thread`'de). Inspector `State.Thread[:ThreadLen]`'i **"Önceki bağlam (N mesaj)"** açılır
    bölümünde user/assistant balonlarıyla gösterir → accumulate node'un gerçekte gördüğü bağlam net.
    `FlowState.thread` + `FlowTraceEntry.threadLen` frontend tiplerine eklendi.
- **Tekrar çalıştır (2026-06-23):** RunView başlığında **"↺ Tekrar çalıştır"** butonu — koşunun
  akışını **aynı girdiyle** (`run.input`) yeniden koşar (`runFlowStreamStandalone(run.flowId, …)`;
  güncel akış tanımıyla). Akış silinmişse veya koşu hâlâ `running` ise buton pasif. Stream
  ilerledikçe sol liste tazelenir; biten yeni koşu listeye anında upsert edilip seçilir
  (`FlowsPanel.rerunRun`).
- **Doğrulama (Playwright):** geçmiş koşular listelendi (✓/✕ rozet); hata koşusu → ⚠️
  `node "classify" … anthropic provider not configured` + boş trace + classify `error` ring;
  başarılı koşu → çalışan node'larda `done` ring + trace çıktıları (paralel çocuklar trace'te
  olmadığından ring'siz — beklenen); yeni başlatılan koşu poll ile listeye otomatik düştü.
  readOnly canvas'ta "Otomatik diz" gizli, "⊕ Ortala" var.

## Masaüstü bildirimleri — flow sonuçları (2026-06-18)

- Her **kaydedilen** flow koşusu (`RunFlowRecorded`) tamamlanınca `emitFlowDelivery`
  bir `flow` event'i yayınlar (`Type:"flow"`, `Target:{view:"executions", sessionId}`).
  Önceden yalnız **otonom** (zamanlanmış / `run_flow` aracı) koşular bildiriyordu; artık
  **interaktif** (UI'dan başlatılan) koşular da bildiriyor. Spam riski yok: istemcideki
  `notify()` yalnızca pencere **arka plandayken** toast gösterir — canlı stream'i izleyen
  kullanıcı rahatsız edilmez.
- Bildirime tıklayınca App.tsx global handler'ı `routeFromEvent` → `executions` view'ına
  deep-link yapar; flow koşusunun **salt-okunur transkripti (RunView)** açılır.
- **Ayar:** `flow` artık `NOTIFY_TYPES`'ta ("Akışlar") → Settings'ten tip bazında
  susturulabilir (varsayılan açık; master toggle `desktopNotifications` hepsini kapsar).
- **Sohbet sonuçları** zaten bildiriyor (`useChatStream.onReply/onError` → `notify`,
  tıklayınca ilgili sohbet oturumu açılır); chat tipi tasarımca `NOTIFY_TYPES` dışında
  (yanıt oturum içinde de görünür) ve her zaman açık.

## Yeni mantıksal node tipleri (2026-06-18)

Motora iki yeni node tipi eklendi (delay + transform) ve **branch genişletildi**
(önceki agent/branch/parallel'e ek):

- **branch + `matchMode`** (birleşik): branch artık `Node.MatchMode` ile üç eşleşme modu
  destekler — **contains** (varsayılan, case-insensitive substring), **equals** (case-insensitive
  trim tam eşleşme), **regex** (Go regexp, ham çıktı). Boş Contains = varsayılan arm (konumdan
  bağımsız, en son değerlendirilir). Motor `evalBranch` + `branchArmMatches`. _(Not: önce ayrı
  bir `switch` node'u eklenmişti; "equals branch ile aynı" geri bildirimi üzerine **branch'e
  matchMode olarak birleştirildi**; switch tipi kaldırıldı.)_ Inspector'da "Eşleşme" dropdown'u.
- **delay** (`⏱️`, LLM'siz): `DelayMs` kadar bekler, sonra `next`. `sleepCtx` ctx-iptaline saygı
  duyar, en çok 5 dk (`maxDelayMs`). Çıktıyı (`Last`) değiştirmez, trace'e "waited Nms" yazar.
- **transform** (UI'da **Birleştir** `🧩`, LLM'siz; tip kimliği `transform` kalır): `Template`'i (`{{input}}/{{last}}/{{node.<id>}}`) render edip
  **çıktı** olarak yayar (`Last` + `Outputs[id]`), sonra `next`. Token harcamadan birleştirme/
  biçimlendirme. Motor: mevcut `render` kullanılır.

**Backend:** `orchestration` `NodeSwitch/NodeDelay/NodeTransform` consts + `Node.DelayMs`/`Template`
(switch `Branches`'i paylaşır), `engine.go` Run case'leri + `evalSwitch`/`sleepCtx`, `Validate`.
**Frontend:** `types/flow.ts` union + alanlar; `flowGraph.ts` adapter (switch=branch-gibi `b<i>`
kenarlar, delay/transform=agent-gibi tek `next`); `nodeStyles.chromeFor` (switch sarı/delay
cyan/transform yeşil); `flow/{SwitchNode,DelayNode,TransformNode}.tsx`; `FlowCanvas` nodeTypes;
`NodeInspector` (switch case editörü "eşittir", delay saniye input, transform şablon textarea);
`FlowsPanel` palet 6 tip.

**Doğrulama:** `go build/vet` + `tsc` yeşil. **API E2E** (LLM'siz): branch `matchMode` —
contains: "now is good"+"no"→MATCHED; equals: "now is good"+"no"→DEFAULT, "no"+"no"→MATCHED;
regex: "error 503"+"^err"→MATCHED, "all ok"→DEFAULT. (Ayrıca transform `{{input}}` render +
delay wait doğrulandı.) **Playwright:** palet 5 tip (Switch yok); branch inspector "Eşleşme"
dropdown'u (İçerir/Eşittir/Regex); transform/delay node'ları canvas'ta doğru render.

## Generator↔Evaluator (GAN-benzeri) döngü şablonu (2026-06-25)

Anthropic *harness design* makalesindeki **self-evaluation problemi**ne (ajan kendi işini
körü körüne över) çözüm: işi yapan **generator** ile yargılayan **ayrı, şüpheci evaluator**
ajanını birbirinden ayıran sözleşmeli iterasyon döngüsü. **Yeni kod yok** — mevcut motorun
node tipleriyle kuruldu; tek "yenilik" döngünün (cycle) bilinçli kullanımı.

**Döngü engine'de İZİNLİDİR (acyclic değil).** `orchestration.Validate()` acyclicity kontrol
**etmez**; `engine.go` döngüye izin verip `maxSteps` (50) ile sınırlar (*"a cyclic graph
(loops are allowed)"*). Frontend flow editöründe de cycle reddi yok (`TaskDetailPanel`'deki
`hasCycle` yalnız kanban görev bağımlılıkları içindir, flow'a değmez). Bu yüzden
`evaluate → decide → generate` **geri-kenarı** doğrudan kurulabilir. _(Düzeltme: `tionharness-flows`
skill'i eskiden "graph must be acyclic" diyordu — yanlıştı, düzeltildi.)_

**Graf:** `contract(gen) → review-contract(eval) → generate(gen) → evaluate(eval) →
decide(branch)`; `decide` → `VERDICT: SHIP` ise `finalize(transform)`, `VERDICT: PIVOT` ise
`pivot(gen) → generate`, aksi halde (default = REFINE) → `generate`.

**Motor kısıtı → tasarım kararları:**
- `branch` yalnız string eşler (sayısal eşik yok) → skor→pivot kararı **keyword verdict**
  (`VERDICT: SHIP|REFINE|PIVOT`) olarak kodlanır; `decide` `matchMode:regex` ile
  `(?m)^VERDICT:\s*SHIP` desenini son satıra demirler.
- Döngüde `Outputs[id]` üzerine yazılır → skor **trend'i** graf state'inde tutulamaz →
  evaluator trend'i kalıcı bir dosyaya/artifact'e yazar ve oradan okur. Sözleşme de aynı
  şekilde bir dosyada yaşar. _(Önceki `core:sprint-scorelog`/`core:sprint-contract` çekirdek
  bellek blokları, memory alt sistemiyle birlikte 2026-07-05'te kaldırıldı.)_
- Loop sayacı branch'e açık değil → döngü evaluator SHIP dediğinde biter; `maxSteps=50`
  (~15 iterasyon) sert backstop.

**Dağıtım:** gömülü gallery şablonu `gan-loop` ("Generator↔Evaluator (GAN)",
`lib/flowTemplates.ts`) + market paketleri (`flow.gan-generator-evaluator`,
`agent.skeptical-evaluator`, `mcp.playwright`) + yeni default skill `tionharness-gan-loop`.
Şablon agent-bağımsız; kurulumdan sonra **iki ayrı ajan** atanır (generator vs evaluator) —
aynı ajanı iki role atamak deseni bozar. UI/E2E hedeflerinde evaluator `mcp.playwright` ile
gerçek tarayıcı testi yapar; saf metin/kod hedeflerinde kod okuma + `Bash`'e düşer (opsiyonel).

**Doğrulama:** `engine_test.go` cyclic-GAN testleri — `Validate` cyclic grafı kabul eder;
loop 2× REFINE → SHIP ile `finalize`'a varır; hiç ship etmeyen evaluator `step cap` hatasıyla
durur. `go build`/`vet` + `tsc -b`/`vite build` yeşil.

## Notlar / sıradaki adımlar

- Şablonları **kategorilere** ayırma / arama eklenebilir.
- Koşu **silme / temizleme (cap)** ve koşudan **yeniden çalıştır** ileride eklenebilir.
- Paperclip-tarzı statik "ajan ilişki haritası" (run_subagent kenarları)
  ayrı bir ekran olarak değerlendirilebilir.
- MiniMap arka planı sabit `#0b0e14` (temaya duyarlı değil) — istenirse tema değişkenine bağlanır.
- Şablon `instantiateTemplate` varsayılan olarak ilk ajanı atıyor; ileride "ajan eşleme" adımı
  (her şablon node'u için ajan seçtirme) eklenebilir.

## Akış emojisi + tek renkli (monokrom) node ikonları (2026-07-04)

İki görsel iyileştirme:

### 1. Akış emojisi (`Flow.emoji`)

Her akışa opsiyonel bir emoji seçilebilir; akışın listelendiği/seçildiği her yerde gösterilir.

- **Model:** `db.Flow.Emoji` (`json:"emoji,omitempty"`). **Bağımsız** kalıcı — `SetFlowEmoji`
  (mirror `SetFlowTags`); `UpdateFlow` emojiye dokunmaz, böylece ad/graph kaydı emojiyi silmez.
- **API:** `PUT /api/flows/{id}/emoji` (`handleSetFlowEmoji`). Create isteği de `emoji` alanını kabul eder.
- **Araçlar:** `create_flow`/`update_flow` `emoji` alanı (update'te boş string temizler),
  `get_flow`/`list_flows` çıktısına `emoji` eklendi. Provenance yine geçerli.
- **UI:** `FlowsPanel` başlık çubuğunda ortak `common/EmojiField` (picker), seçim anında
  `api.setFlowEmoji` ile kalıcı olur (Kaydet butonundan bağımsız — etiketler gibi). Emoji
  görünen yerler: flow listesi, Koşular listesi + başlığı, `RunView` başlığı, `FlowPicker`
  (Zamanlama/Otomasyon dropdown'ları), zamanlama/otomasyon satır rozetleri (emoji varsa
  `Workflow` ikonu yerine gösterilir; metinde `🔀` yerine emoji), TaskBoard kartı + `TaskFormModal`.
  Tüm gösterimler mojibake'e karşı `normalizeAvatar` ile geçirilir.

### 2. Monokrom node ikonları

Node tür ikonları çok renkli emojilerden (🤖🔀⚡⏱️🧩) **temaya uygun tek renkli lucide
ikonlarına** geçti: `Bot / Split / Zap / Timer / Puzzle` (`nodeStyles.NODE_ICONS`).

- `NodeChrome.icon: string` → `Icon: LucideIcon`. `NodeShell` başlıkta `<chrome.Icon size={13}/>`
  render eder (accent zeminde `currentColor` = beyaz). Bilinmeyen tip → `Circle`.
- **Node ağacı (palet):** `FlowsPanel` "Node ekle" listesi aynı `NODE_ICONS`'u
  `text-[var(--color-text-dim)]` ile render eder — canvas başlıkları ile tutarlı.
- Yeni/değişen: `flow/nodeStyles.ts`, `flow/NodeShell.tsx`, `panels/FlowsPanel.tsx`.

## Node türü sabit + değişken info butonu + flow tag görünürlüğü (2026-07-04)

Üç küçük düzenleme (hepsi frontend):

- **Node türü artık sabit.** `NodeInspector`'daki **"Tür" `<select>` kaldırıldı** — bir node'un
  türü yalnız palet'ten oluşturulurken belirlenir, sonradan değiştirilemez. Yerine **salt-okunur
  tür başlığı**: monokrom tür ikonu (accent zeminde) + tür adı + "(tür sabit)". (`patchSelected`'in
  tür-değişim/kenar-budama dalı artık tetiklenmez ama savunma amaçlı duruyor.)
- **Node'lar arası değişken info butonu.** `NodeInspector`'da agent **Prompt** ve transform
  **Şablon** alanlarının yanına ℹ️ popover (`FlowVarsButton`) eklendi — Otomasyonlardaki
  prompt-değişken yardımcısını taklit eder. Flow motoru (`orchestration.render`, engine.go)
  yalnız şunları destekler: `{{input}}` (akış girdisi), `{{last}}` (en son node çıktısı),
  `{{node.<id>}}` (belirli node çıktısı). Popover statik iki girdi + akıştaki **diğer** her node
  için bir `{{node.<id>}}` satırı (node başlığıyla) listeler; tıklayınca alana ekler. Not:
  otomasyonların `{{date}}/{{time}}/{{agent}}` gibi değişkenleri flow motorunda **yok**.
- **Flow tag görünürlüğü.** Flow etiketleri zaten atanabiliyordu (sol **Görünüm > Etiket**
  `TagEditor` → `setFlowTags`); artık **sol flow listesi satırlarında chip** olarak da görünür
  (ilk 4 + "+N"). Görünüm editörünün `onChange`'i `flows` dizisini de senkronlar → chip'ler canlı yenilenir.


## Flow tarih/saat değişkenleri + tag filtreleme (2026-07-04)

- **Yeni flow değişkenleri.** `orchestration.render` (engine.go) artık `{{date}}` (`2006-01-02`),
  `{{time}}` (`15:04`), `{{datetime}}` (`2006-01-02 15:04`) placeholder'larını da çözer —
  otomasyon `turnVars` formatıyla birebir. Render anındaki duvar-saati kullanılır (resume'da
  resume anı; bu "şimdi" değerleri için kabul edilebilir). Node inspector'ın ℹ️ popover'ına
  eklendi. Test: `TestRenderVars`. (Otomasyona özgü `{{result}}/{{tag}}/{{iteration}}` gibi
  oturum-bağlamlı değişkenler flow motorunda **yok** — flow'un oturum/tetik bağlamı yoktur.)
- **Flow tag filtreleme.** `FlowsPanel` "Akışlarım" sekmesinde arama kutusunun altında
  **etiket chip'leri** (yalnız en az bir flow etiketliyse). Chip'e tıkla → o etikete göre süz
  (**ANY** eşleşme: seçili etiketlerden birini taşıyan flow'lar), "temizle" ile sıfırla. Ad
  araması (`q`) ile **AND**'lenir. Boş sonuç → "Eşleşen akış yok."

## Değişken popover'ı satır-içi (kırpılma fix) + run input info butonu (2026-07-04)

- **Kırpılma fix.** `FlowVarsButton` ayrı bir bileşene taşındı (`flow/FlowVarsButton.tsx`) ve
  **absolute popover yerine satır-içi genişleyen panel** olur: `w-full basis-full`, ebeveyn
  `flex flex-wrap` satırında kendi satırına sarar. Böylece node-editör modal'ının
  `overflow-y-auto` gövdesi artık paneli **kesmiyor** (eski `absolute bottom-full` üstten/yandan
  kırpılıyordu). Panel içeriği büyüdükçe modal gövdesi kaydırılır.
- **Run input info butonu.** Flow'u başlatan girdi alanına ("Girdi") da ℹ️ butonu eklendi
  (`context="seed"`). Seed, node'larda `{{input}}` olur; bu yüzden yalnız `{{date}}/{{time}}/
  {{datetime}}` önerilir (bunlar motorun `render` sıralı ikamesiyle — önce `{{input}}` sonra
  `{{date}}` — bir node `{{input}}` kullandığında gerçekten çözülür). `{{last}}/{{node.<id>}}`
  seed anında önceki çıktı olmadığından listelenmez; panelde "Bu metin akışta {{input}} olur" notu var.

## Daraltılabilir "Node ekle" paleti + yukarı büyüyen run input (2026-07-04)

- **Daraltılabilir palet.** Sol paletteki "Node ekle" başlığı artık chevron'lu bir **toggle**
  (`paletteOpen`, `localStorage: tionharness.flowPaletteOpen`). Daraltınca ipucu + node tip butonları
  gizlenir; "Görünüm" bölümü hep görünür kalır.
- **Run input yukarı büyür.** Çalıştır girdisi `rows={1}` sabit yükseklikten **auto-grow**'a geçti
  (`runInputRef` + effect: `height=auto` → `min(scrollHeight,160)`; `resize-none max-h-40`). Run
  paneli `flex-1` canvas'ın altında bottom-anchored olduğundan textarea büyüdükçe panelin üst kenarı
  yukarı kayar → girdinin **alt kenarı sabit kalır, üst yukarı genişler**. Satır `items-end` ile
  ℹ️ + Çalıştır son satıra hizalı.

## Değişken info butonu → baloncuk (portal) + run input satırı ortalı (2026-07-04)

- **Baloncuk.** `FlowVarsButton` artık içeriği **floating baloncukta** gösterir: `createPortal`
  ile `<body>`'ye fixed-positioned popover. Buton viewport'un alt yarısındaysa **üstte**, değilse
  altta açılır (`getBoundingClientRect`; scroll/resize'da yeniden konumlanır, Escape/backdrop kapatır).
  Portal olduğu için hiçbir `overflow` ata (node-editör modalı dahil) baloncuğu **kesmez** — önceki
  satır-içi `basis-full` panel yaklaşımının yerini aldı.
- **Run input satırı** tekrar `flex items-center gap-2` (ortalı `[ℹ️][input][Çalıştır]`). Baloncuk
  artık satırı büyütmüyor. Textarea auto-grow + run panelinin bottom-anchored olması sayesinde
  input **ve çevresindeki alan** yukarı doğru genişler.

## Run paneli fazla alt boşluk fix (2026-07-04)

Çalıştır panelindeki `pb-24 md:pb-4` **kaldırıldı** → `p-4`. MobileNavBar boşluğu zaten global
olarak `<main>`'in `max-md:pb-[calc(3.25rem+env(safe-area-inset-bottom))]`'i (App.tsx) ile
sağlanıyor; panele ayrıca eklenen `pb-24` girdi altında ölü boşluk + gereksiz scroll yaratıyordu.

## Dar ekranda MiniMap default kapalı + sol panel gizle/göster (2026-07-05)

- **MiniMap dar ekranda kapalı.** `FlowCanvas` `showMinimap` başlangıcı artık ekran genişliğine
  bağlı: `window.innerWidth >= 768` (yani `< md`'de **kapalı**, md+'da açık). Canvas toolbar'daki
  "🗺 harita" toggle'ı hâlâ elle aç/kapa yapıyor.
- **Sol palet gizle/göster.** Node ekle + Görünüm kolonu **canvas'ın üzerinde sol üstte** yüzen
  (`absolute left-2 top-2 z-10`) `PanelLeftClose`/`PanelLeftOpen` butonuyla tümüyle gizlenip
  açılabilir (`paletteVisible`, `localStorage: tionharness.flowPaletteVisible`). Canvas sarmalayıcı
  `relative`; React Flow toolbar'ı top-right, Controls bottom-left olduğundan sol üst boş.
  Gizliyken canvas tam genişlik. (Palet içi "Node ekle" bölüm-daraltma `paletteOpen`'dan ayrıdır.)

## Koşular: "Adım izi" alttan açılıp kapanabilir + dikey yükseklik fix (2026-07-05)

- **Adım izi toggle.** `RunView`'deki "Adım izi" (step trace) bölümü artık **alttan açılıp
  kapanabilen** bir panel: başlık satırı chevron'lu toggle (`ChevronUp` kapalı → yukarı aç,
  `ChevronDown` açık) + "N adım" sayacı. `traceOpen` `localStorage: tionharness.flowTraceOpen`'da
  kalıcı; **default dar ekranda (`< md`) kapalı**, md+'da açık.
- **Dikey yükseklik fix.** `RunView` kökü `min-h-0` aldı (flex çocukları düzgün küçülsün diye);
  trace listesi `max-h-[40%]` → `max-h-[40vh]` (kesin-yükseklik gerektirmeyen, daha kararlı).
  Kapalıyken canvas tüm yüksekliği alır — dar ekranda "yükseklik bozulması" giderildi.

## Varsayılan flow tohumlama (per-workspace, 2026-07-25)

Her workspace store'una otomatik bir **varsayılan flow** ("Yanıtla & Doğrula" — bir ajan
isteği yanıtlar, ikinci ajan hataları/eksikleri bulup düzeltilmiş nihai sürümü üretir)
tohumlanır. Silinebilir; silinen tohum **geri gelmez**.

- **Kaynak:** `internal/agent/flow_defaults.go` — `defaultFlows` tek doğruluk kaynağı
  (`Seed` stabil kimlik + graph). `EnsureDefaultFlows(ctx, db, storeDir)` seed eder.
- **Tetikleme:** `workspace.Manager.open()` her açılışta çağırır → yeni workspace'ler kurulumda,
  **eski workspace'ler bir sonraki başlangıçta** otomatik backfill.
- **İdempotens + silme kalıcılığı:** DB'de aynı `Seed`'li flow varsa atlar; yoksa store
  kökündeki `.seeded-flows.json` ledger'ında kayıtlıysa (= kullanıcı silmiş) **yeniden
  oluşturmaz**. `db.Flow.Seed` alanı shipped default'ı işaretler.
- **Ajan ataması:** agent node'lara workspace'in ilk ajanı atanır (ajan yoksa boş `agentId` —
  `CreateFlow` doğrulamaz, flow yine görünür, kullanıcı ajan atar).
- **Şablon galerisi:** aynı graph `flowTemplates.ts`'te `default-starter` olarak da listelenir
  (iki graph senkron tutulmalı).
- **Test:** `internal/agent/flow_defaults_test.go` — seed/idempotens, silme kalıcılığı,
  ilk-ajan ataması, ajansız seed.

## Accumulate (cache'li bağlam) modu + Döngü node + Paralel fold (2026-07-25)

Üç bağlı özellik; hepsi geriye tam uyumlu (yeni alanlar `omitempty`, yeni runner arayüzü opsiyonel).

### Accumulate modu (`Graph.Accumulate`)
Açıkken (flow düzenleyicide **Görünüm ▸ "Bağlamı biriktir (cache)"**) ardışık agent node'ları
**büyüyen tek bir konuşma thread'ini** (`State.Thread []Msg`) paylaşır: her node'un render'lanmış
prompt'u `{user}`, cevabı `{assistant}` olarak eklenir ve sonraki node bu thread'le çağrılır.
Böylece ajanın statik system + büyüyen mesaj prefix'i provider prompt-cache'inde yeniden kullanılır
(düğümler arası cache). Kapalı (varsayılan) = eski **stateless** yol (node başına tek-mesajlık taze
çağrı). Bir agent node **`Fresh`** ile devre dışı kalır (birikmiş bağlamı görmez/büyütmez).
Runner köprüsü: `orchestration.ThreadAgentRunner` → `flowRunner.RunAgentNodeThread` →
`completeThread` (provider zaten mesaj slice'ı alıyor; **provider tarafında değişiklik yok**).

### Paralel fork/fold
Accumulate altında paralel node **copy-on-fork**: her çocuk aynı birikmiş prefix'i (`st.Thread`)
**okur** (aynı ajanlı dallar cache prefix'ini paylaşır) ama ebeveyn thread'i tek başına büyütmez.
Join'de birleşik çıktı ebeveyn thread'e **tek sentetik `[user-marker, assistant-combined]` çifti**
olarak katlanır (`parallelFoldMarker`) → thread lineer, alternasyonlu ve **assistant ile biter**
(sonraki agent'ın user turn'ü alternasyonu bozmaz). Transform/branch/delay LLM'siz → thread'e dokunmaz.

### Döngü node (`NodeLoop`)
Alanlar: `Body` (yinelenen alt-zincirin giriş id'si), `LoopNext` (çıkışta gidilecek node),
`MaxIters` (sert cap), `Until`+`UntilMode` (çıkış koşulu, branch eşleşme modlarını paylaşır).
Motor `runLoop`: her iterasyonda `st.Current=Body` ile **motorun kendi `Run`'ını özyinelemeli**
çağırır → gövde her node tipini (iç içe parallel/loop dahil) içerebilir; `Run`'daki global
`maxSteps` tüm iterasyonlar toplamını sınırlar. Gövde node'ları **`{{iteration}}`** (0-tabanlı,
`render`'a eklendi) okuyabilir. Çıkış: iterasyon `MaxIters`'a ulaşır **veya** `Until` son çıktıya
uyar. `Validate`: `Body` zorunlu + (`MaxIters>0` **veya** boş-olmayan `Until`) — sonsuz döngü
imkânsız. **Resume sınırı:** döngü kontrolü çağrı yığınında (State'te değil) → iterasyon-ortası
çökme mevcut gövde geçişini tamamlayıp çıkar (dokümante). Frontend: palet 7. tip "Döngü"
(`LoopNode`, gövde=pembe `body` + çıkış=mavi `loop` tutamağı), inspector maxIters/until/mode.

**Kapsam dışı (opsiyonel, sonraki):** iç içe paralel / dal-başına alt-hat (paralel çocuğun
alt-graf olması — `runLoop`'un kullandığı özyinelemeli `Run` bunu ileride bedavaya yakın açar);
loop için `CompactEachIter` (her iterasyonda thread katlama — cache'i kırar, varsayılan kapalı).

**Test:** `internal/orchestration/accumulate_test.go` (thread büyüme, Fresh opt-out, stateless
fallback, parallel fold) + `loop_test.go` (maxIters/until çıkış, Validate bound/body). Backend
243 test yeşil; `tsc -b` + `vite build` yeşil.

### Session → Flow köprüsü ("Akış" inline görünüm, 2026-07-25)
Chat header'ındaki **"Akış" toggle'ı** (`AppHeader`, Debug'ın yanında; aktifken accent) mevcut
oturumu **tamamlanmış bir flow KOŞUSU** olarak sohbet alanında **inline** gösterir (popup değil;
"Sohbete dön" ile geri). Kilit karar: transkript bir flow *tanımı* değil, *koşusu* olarak üretilir →
`sessionToFlowRun` (`features/flows/sessionToFlow.ts`, saf/backend'siz) hem grafiği hem **sentetik
`{flow, run}`**'ı kurar; `FlowState.trace` her node'un **çıktısını = asistan cevabını** taşır.
Böylece `RunView` yeniden kullanılır (`SessionFlowInline`): canvas'ta node = user prompt'u (başlık),
**"Adım izi"nde agent cevabı** görünür — önceki "yalnız bizim mesajlarımız görünüyordu" sorunu çözülür.
Her assistant turn'ü bir agent node; `next` ile lineer; `accumulate:true` (sohbet tek büyüyen konuşma).
(2026-07-27: reify edilen graf zorunlu **Start node** ile başlar — `sessionToFlowRun` bir `start`
node'u prepend eder, trace'te done görünür; adım sayacı start'ı saymaz.)
Dal/paralel yapısı düz transkriptten çıkarılamaz → reify sonucu daima lineer. Reset: aktif oturum
değişince inline görünüm kapanır. (Ters yön — flow koşusunu çok-turlu session olarak render — mevcut
per-node step-kartı kaydıyla zaten karşılanıyor.) (2026-07-27: **"Flow olarak kaydet" butonu ve
özelliği kaldırıldı** — `SessionFlowInline` artık yalnız görüntüler, `api.createFlow` çağırmaz;
`onError` prop'u da söküldü.)

**Gerçek flow oturumu → gerçek graf (2026-07-27):** Bir flow koşusunun transkript oturumunu
"Akış olarak gör" ile açınca artık transkript **reify edilmez** — koşunun **gerçek grafiği/düzeni**
gösterilir (Koşular tab'ıyla birebir aynı düzen; "dizilim farklı" sorunu çözüldü). Mekanizma:
`db.FlowRun`'a **`SessionID`** alanı eklendi; `RunFlowRecorded` nihai `sessionID`'yi
`db.SetFlowRunSession` ile koşuya damgalar (yeni koşular için kesin bağ). `SessionFlowInline` artık
`sessionKind`+`sourceId`+`sessionCreatedAt` alır: `kind==='flow'` ise **`api.listFlows()` +
`api.listFlowRuns(sourceId)`** ile flow'u ve koşuyu bulur → gerçek `{flow, run}`'ı `RunView`'e verir
(görüntüleme-only; kaydet yok). Koşu eşleştirme **iki aşamalı**: önce `run.sessionId === sessionId`
(yeni koşular), yoksa **en yakın `createdAt`** (eski koşular — per-run oturum koşuyla ~aynı saniyede
yaratılır; 5 sn tolerans, aşılırsa reify). Bu sayede **backend restart gerekmeden** eski koşular da
(örn. SES200 → RUN11) gerçek grafiği gösterir. Eşleşme yoksa (flow silinmiş / koşu silinmiş) veya
oturum flow-kaynaklı değilse reify. (`GET /api/flows/{id}` / `handleGetFlow` de eklendi ama zorunlu
değil — frontend `listFlows` kullanır.)

**Node inline çıktı önizlemesi:** `FlowRFNode.data.output` (koşu görünümlerinde `RunView` node data'sına
canlı/trace'ten geçirilir); `AgentNode` node `done` olduğunda cevabı yeşil kenarlı `line-clamp-3`
kutuda gösterir (`data-testid="flow-node-output"`) → canvas'ta prompt **ve** çıktı birlikte görünür.

**E2E doğrulaması (Playwright, headless chromium, canlı dev :5173):** 6/6 kontrol geçti — header
"Akış" toggle → inline canvas render, **9 node çıktı önizlemesi** (agent cevapları), "Sohbete dön"
geri butonu; flow editör paletinde **Döngü** + Görünüm'de **Bağlamı biriktir (cache)** toggle'ı.

### İyileştirmeler (2026-07-25, ikinci tur)
- **Dikey auto-layout:** `autoLayout` (flowGraph.ts) artık BFS derinliğini **y** (yukarı→aşağı),
  kardeş sırasını **x** (sola→sağa) yapar → "Oto diz" dikey dizer. (2026-07-27: aralıklar
  sıkılaştırıldı — `COL_W` **280→240**, `ROW_H` **180→150** → oto-dizilen graf daha kompakt,
  çıktı önizlemeli node'ları hâlâ çakışmadan geçirecek kadar yüksek.)
- **Accumulate default AÇIK:** `Graph.Accumulate` JSON tag'inden **`omitempty` kaldırıldı**
  (`false` verbatim persist olur → save/reload round-trip'i bozulmaz). Frontend `FlowsPanel`
  toggle'ı `useState(true)`; `selectFlow` `g.accumulate === undefined ? true : !!g.accumulate`
  (yalnız açıkça `false` olan akış kapalı; alan yoksa/eski akış → açık).
- **Flow-run oturumu → çok-node açılımı (bugfix):** bir flow koşusu oturuma **tek assistant turn**
  olarak, node'lar o turn'ün `steps`'ine gömülü kaydedilir (`flowStateToSteps`, her node bir text
  step `**title**\n\n<çıktı>`). `sessionToFlowRun` artık bu turn'ü açar: `steps` **≥2 ve hepsi text**
  ise her step bir node olur (başlık bold header'dan, çıktı gövdeden) → reify edilen akış orijinal
  grafiği yansıtır (önceden tek node'a çöküyordu). Normal sohbet turn'ü (thinking/tool step'li) tek
  node kalır. Doğrulandı: gerçek `SES194` (FLW5 "Yanıtla & Doğrula") → 2 node (Yanıtla, Doğrula).

### Flows tab'ları deep-link + Koşular'da Girdi → Adım izi (2026-07-27)
- **3 tab için ayrı URL:** FlowsPanel sol-kolon tab'ı (`flows`|`templates`|`runs`) artık URL'de:
  **`#/w/{ws}/flows/{tab}`** (varsayılan `flows` segment taşımaz → temiz `#/w/{ws}/flows`).
  `useSessionState('flows.tab')` kaldırıldı; tab state App'e taşındı (`useDeepLinks.flowsTab`),
  `useAppNavigation` (`routeIdForView`/`applyRoute` `flows` case'i) URL↔state senkronu yapar,
  FlowsPanel controlled `tab`/`onTabChange` prop'larını alır (`setTab` = `Dispatch<SetStateAction>`
  imzasını korur ama sonucu parent'a yazar). Reload/`geri`/`ileri` doğru tab'a düşer, link paylaşılır.
- **Koşular'da Girdi Adım izi'nde:** Koşular tab'ındaki `RunView` de `inputInTrace` alır → üstteki
  "Girdi:" satırı yerine alt Adım izi panelinin ilk öğesi (sohbet flow görünümüyle aynı davranış).

### Editörden çalıştır → Koşular tab'ına yönlendir (2026-07-25)
Editörden "Çalıştır" artık koşuyu **editör canvas'ına boyamaz** (eski `setNodeStatus`/`liveNodes`
kaldırıldı) → editör ekranı değişmeden kalır. Bunun yerine `doRun` (flowActions.ts) `setTab('runs')`
ile **Koşular** tab'ına geçer ve taze koşuyu otomatik seçer: koşu-öncesi bu flow'un koşu id'lerini
(`priorIds`) alır, stream'in `onNode`'unda `listAllFlowRuns` yenileyip `priorIds`'te olmayan **yeni**
koşuyu `setSelectedRunId` ile seçer (`RunView` flow-node bus'tan canlı akıtır); `onReply`'de biten
koşuyu upsert+seçer. Deps değişti: `setNodes/setRun/setLiveNodes` yerine `runs/setRuns/setSelectedRunId`.
E2E: Çalıştır → Koşular tab aktif + RunView + 2 node çıktısı.

### Per-run flow oturumu (2026-07-25)
Eskiden bir flow'un tüm koşuları **tek** transcript oturumunda birikiyordu
(`GetOrCreateSourceSession("flow", flow.ID)`, flow başına bir session) → "Akış olarak gör"
N koşuyu tek zincire karıştırıyordu. Artık **her koşu kendi oturumunu** alır: `RunFlowRecorded`
`db.CreateSession` ile (kind `flow`, **`SourceID = flow.ID`** korunur — Ağ grafiği + Aktivite feed
flow'a bu alanla bağlanır; graph.go/executions.go bozulmaz) **yeni** bir session yaratır, id'yi
`recordFlowSessionTurn`'e geçirir (çift-oluşturmayı önler; boşsa fallback create). Böylece bir koşunun
transkripti — ve reify'ı — tam olarak **tek koşu** gösterir. `RunFlow` (kayıtsız, `handleSessionRunFlow`)
oturuma dokunmaz → etkilenmez. Test: `flow_session_test.go` (iki koşu → iki ayrı session, kind/sourceID,
2 mesaj). **Bilinen kozmetik sınır:** `executions.go lastStatusFor` flow session'ı için flow'un **en
yeni** koşusunun statüsünü döndürür → eski bir koşu-oturumu daha yeni bir koşu oluşunca statü çipinde
onu gösterir (oluşturma anında doğru). Tam eşleme FlowRun↔session linkage'i gerektirir (sonraki).
**Not (2026-08-16):** "en yeni" artık deterministik — `ListFlowRuns` yalnız saniye-granülerlikli
`CreatedAt` ile sıralıyordu, aynı saniyedeki iki koşunun sırası map iterasyonuna kalıyordu; şimdi
`flowRunBefore` (id sayacı tie-break) kullanılıyor. Ayrıca statü artık poll başına tek taramayla
kurulan `newestFlowRunStatus` indeksinden okunuyor (oturum başına `ListFlowRuns` değil). **Not:** backend değişikliği; canlı görmek için backend yeniden derlenip
başlatılmalı (Go hot-reload olmaz).

### Flow oturumu: anında sidebar + boş-ekran fix (2026-07-27)
Eskiden `RunFlowRecorded` session'ı up-front oluşturuyor ama **user + assistant mesajlarının ikisini de
run bittikten sonra** ekliyordu (`recordFlowSessionTurn`) ve `db.CreateSession` bir `session` SSE olayı
yaymıyordu. Sonuç: (1) koşu sohbet listesinde ancak manuel yenilemeyle çıkıyordu, (2) çıksa bile koşu
sürerken tıklayınca **boş, mesajsız ekran** görünüyordu. Düzeltme:
- Session oluşunca **user turn hemen** yazılır (`recordFlowInput` + `flowInputText` helper'ı) ve bir
  `session`/`op:create` olayı yayınlanır → sidebar canlı yenilenir (`useAppEvents` → `refreshSessions`),
  tıklayınca en azından **input balonu** görünür.
- Assistant yanıtı sonda eklenince `session`/`op:message_added` olayı yayınlanır → açık transcript
  (`listMessages`) yeniden yüklenir, yanıt manuel yenileme olmadan belirir.
- `recordFlowSessionTurn`'e **`inputRecorded bool`** parametresi eklendi: up-front yol `true` geçer
  (input çift eklenmez); fallback (sessionID boş) yol `false` geçer ve user mesajını kendisi yazar.
  Test çağrıları `false` ile güncellendi (`flow_session_test.go`, hâlâ 2 mesaj). Backend değişikliği →
  yeniden derle/başlat.

## Start / End node'ları (zorunlu giriş + opsiyonel çıktı sözleşmesi, 2026-07-27)

Flow'lara ilk-sınıf **Start** ve **End** node tipleri eklendi (temiz kurulum — geri uyumluluk
gözetilmedi, eski flow'lar migrate edildi).

- **`NodeStart` ("start"):** **zorunlu** giriş markeri; LLM'siz pass-through (`st.Current=Next`).
  `Validate` **tam bir** start node ister ve `Graph.Start` ona eşit olmalı → ayrı "başlangıç
  işaretle" kalktı (per-node `onMakeStart` + `▶ Başlangıç yap` kaldırıldı). `graphToReactFlow`
  `isStart = type==='start'`.
- **`NodeEnd` ("end"):** **opsiyonel** terminal (giden kenarı yok). `Template` nihai çıktıyı
  şekillendirir; `OutputSchema` (JSON Schema) verilirse nihai çıktı geçerli JSON değilse koşu
  **`failure`** — flow'un **çıktı sözleşmesi**. Birden çok dal tek End'e yakınsayabilir. Empty-Next
  yine terminal (End şart değil).
- **Migration:** `orchestration.MigrateAddStart` (idempotent) — start node'u olmayan grafa bir tane
  prepend eder (`Next`=eski giriş). `agent.MigrateFlowsStartEnd` her workspace açılışında tüm
  flow'ları migrate eder (`manager.open`); default flow (`flow_defaults.go`) + gallery templates
  (`flowTemplates.ts` `default-starter`) + harnesspack template builder (`resolveTemplateFlowGraph`) +
  frontend `ensureStartNode` (instantiate/preview) yeni formatta. Yeni flow oluşturma start node
  ile tohumlanır.
- **UI:** palet'te "Başlangıç" (yeşil `Play`) + "Bitiş" (mavi `Square`); `StartNode`/`EndNode`
  bileşenleri; inspector'da End için şablon + JSON-Schema alanları.
- **Test:** `startend_test.go` (pass-through, End template/schema, tek-start Validate, MigrateAddStart);
  fixture'lar + template pack test'i güncellendi. Backend 1004 test yeşil; `tsc`+`vite build` yeşil;
  canlı: start→agent→end koşusu (`[final] DONE_OK`) + eski FLW15 migrate.

## Flow başlatma öncesi doğrulama (semantic precheck, 2026-07-13)

**Sorun.** `orchestration.Graph.Validate()` yalnız **yapısal** doğrulama yapar: start node var
mı, id'ler tekil mi, `Next`/`Branches`/`Parallel`/`JoinNext` referansları graf içinde çözülüyor
mu, agent node'un `agentId` alanı **dolu** mu. Ama o `agentId`'nin DB'de gerçekten **var olup
olmadığını** ya da ajanın sağlayıcısının yapılandırılmış olup olmadığını bilemez —
`orchestration` paketi bilerek `db`/`providers`'a bağımlı değildir.

Sonuç: silinmiş bir ajana ya da API anahtarı olmayan bir sağlayıcıya referans veren akış,
yapısal doğrulamayı geçip **`FlowRun` kaydı oluşturuyor**, sonra motor ilk agent node'a
geldiğinde patlıyordu. Kullanıcı, aslında hiç başlayamayacak bir akış için başarısız bir koşu
kaydıyla karşılaşıyordu.

**Çözüm.** `RunFlow` içinde, `g.Validate()` **sonrası** ve `CreateFlowRun` **öncesi** DB+registry
destekli ikinci bir katman: `internal/agent/flow_precheck.go` →
`(*Runtime).validateFlowPreconditions(ctx, g)`.

Kontroller:
- **Agent node** → `db.GetAgent(agentID)`: ajan silinmişse hata. Ajan varsa
  `providers.Registry.Get(agent.Provider)`: sağlayıcı bilinmiyorsa veya yapılandırılmamışsa
  (ör. anahtarsız `anthropic`) hata. Ajan id'leri **distinct** olarak bir kez sorgulanır — aynı
  ajana çok node'dan referans veren fan-out graflarında tekrar okuma yok.
- **Transform node** → `Template` boşsa (yalnız boşluk dahil) hata: boş template sessizce boş
  çıktı üretip `{{last}}` ile aşağı taşınır; bu bir yapılandırma hatasıdır, geçerli no-op değil.
- **Delay node** → `DelayMs` negatifse hata.

Hatalar `errors.Join` ile **tek seferde** birleştirilir — kullanıcı her denemede bir sonraki
hatayı keşfetmek yerine akışı tek geçişte düzeltir.

**Hata yolu.** `RunFlow`'un tüm çağıranları (`RunFlowRecorded`, `handleSessionRunFlow`,
`handleSessionRunFlowStream`) dönen `error`'u zaten kullanıcıya iletiyordu → ek UI değişikliği
gerekmedi. Precheck başarısız olduğunda **hiç `FlowRun` kaydı oluşmaz**.

**Geriye dönük uyumluluk riski.** Bugüne kadar çalışmayı deneyip ilk node'da patlayan akışlar
artık **baştan** reddedilir. Davranış farkı: başarısız bir koşu kaydı yerine anında hata mesajı.

**Doğrulama:** `internal/agent/flow_precheck_test.go` — geçerli graf regresyon testi (precheck'i
geçer), eksik ajan, yapılandırılmamış sağlayıcı, bozuk transform/delay parametreleri, tüm
hataların birlikte raporlanması ve `RunFlow`'un **`FlowRun` kaydı oluşturmadan** reddettiği.
`go build ./...` + `go vet ./...` + `go test ./internal/...` yeşil.

## Palet node butonlarında (ⓘ) bilgi balonu (2026-07-27)

Sol paletteki "Node ekle" listesinde her tipin adı vardı ama **ne işe yaradığı**
hiçbir yerde yazmıyordu; kullanıcı ancak node'u ekleyip inspector'ı açarak
anlayabiliyordu. Artık her palet satırı **buton + (ⓘ)** ikilisi:

- Metinler `frontend/src/features/flows/nodeTypeHelp.ts` → `NODE_TYPE_HELP:
  Record<FlowNodeType, string>`. Sözlü açıklamalar `internal/orchestration/model.go`
  içindeki node-tipi yorumlarıyla hizalı — motor davranışı değişirse ikisi birlikte
  güncellenmeli.
- Render: mevcut paylaşılan `shared/components/InfoPopover` yeniden kullanılır.
  Palet kolonu `overflow-y-auto` olduğu için mutlak konumlu balon **kırpılırdı** →
  `InfoPopover`'a opsiyonel **`fixed`** modu eklendi: açılışta butonun
  `getBoundingClientRect()`'i ölçülür ve balon viewport koordinatlarında
  (`position: fixed`, sağ/alt kenara clamp'li) çizilir. Varsayılan `fixed=false`,
  yani diğer kullanım yerlerinin davranışı değişmez.
- Palet satırı `flex items-center gap-1`; buton `min-w-0 flex-1` + etiket `truncate`
  (dar `w-32` mobil palette taşma yok). Sürükle-bırak (`FLOW_NODE_DND_MIME`) ve tıkla-ekle
  davranışı aynen korunur — (ⓘ) butonu draggable değildir, tıklaması node eklemez.

## `coordinator` node tipi — dinamik worker fan-out (2026-07-28)

### Neden

`parallel` ve `spawn`/`join` fan-out **genişliği tasarım anında sabittir**: akışı
çizerken kaç kol olacağını bilmek gerekir. "Bulunan her bulgu için bir worker",
"bu klasördeki her modül için bir inceleme" gibi **sayısı çalışma anında belli
olan** işlerin karşılığı yoktu. Koordinatör/worker mekanizması (`_Docs/47`) tam
bunu çözüyordu ama yalnız kullanıcı sohbetinde erişilebiliyordu.

`coordinator` node'u iki mekanizmayı birleştirir: graf **deterministik motorda**
kalır, tek bir düğüm alt-hedefi canlı bir koordinatör oturumuna devreder.

### Sözleşme

Motor açısından düğüm **atomiktir**: koordinatör "susana" kadar bloklar, sonra tek
çıktı üretir. Yarıda kalan bir koşu resume'da düğümü **baştan** (yeni koordinatör
oturumuyla) çalıştırır — restart-safe state modeli bozulmaz.

| Alan | Anlamı |
|------|--------|
| `agentId` | Koordinatör olarak koşacak ajan (zorunlu; precheck agent node'la aynı) |
| `prompt` | Devredilen hedef; `{{input}}`/`{{last}}`/`{{node.<id>}}` render edilir |
| `workflow` | **Koordinasyon türü** — kayıtlı reçete slug'ı (`kind: coordinator-workflow` skill'i). "" = serbest |
| `maxTurns` | Bu düğüme özel koordinatör auto-tur (notify-loop) tavanı (0 = reçetenin kendi `max_turns`'ü, o da yoksa workspace varsayılanı) |
| `timeoutSec` | Yerleşme (settle) süresi tavanı, 0 = 30 dk varsayılan |
| `next` | Sonraki düğüm |

Çıktı = koordinatörün **son assistant mesajı**. Yanıt boşsa düğüm hata verir
(sessizce boş `{{last}}` taşımaz).

### Yürütme (`internal/agent/flow_coordinator.go`)

0. `workflow` doluysa `skills.ResolveCoordinatorWorkflow` ile **doğrulanır** — bilinmeyen
   slug / coordinator-workflow olmayan skill / geçersiz pattern düğümü hata verdirir
   (sessizce serbest koordinasyona düşmez; yazar reçeteyi bilerek seçmiştir). Aynı geçit
   `validateFlowPreconditions`'ta da koşar → koşu başlamadan yakalanır.
1. `Kind="flow-coordinator"` + `Role="coordinator"` bir oturum açılır
   (`SourceID` = flow run id → Aktivite akışı koşuya geri çözer). Seçilen slug
   `Session.CoordinatorWorkflow`'a yazılır — reçetenin gövdesini prompt'a enjekte eden
   `coordinatorRecipeBlock` zaten oturumdan okuduğu için ek kablolama gerekmez.
2. Prompt user turu olarak yazılır, `enqueueCoordinatorTurn` çağrılır. Bu çağrı
   slot'u **senkron** olarak `running=true` yapar → aşağıdaki bekleme asla erken
   "boşta" göremez.
3. Ajan `spawn_worker`/`send_to_worker` ile workerlarını kendisi açar; normal
   `<task-notification>` döngüsü işler (koordinatör prompt'u + canlı worker-state
   bloğu `Role`'den gelir, ek kablolama yok).
4. `waitCoordinatorIdle` 500 ms'de bir yerleşme kontrolü yapar:
   `!running && !pending && workers==0`.
5. Zaman aşımında çalışan workerlar `StopWorker` ile durdurulur ve düğüm hata verir
   (arkada yetim worker turu kalmaz).

**Yarış yok:** `runWorker`, worker sayacını azaltan `defer`'inden **önce**
`NotifyCoordinator`'ı çağırır; yani son worker sayımdan düşerken koordinatör turu
zaten claim edilmiş olur.

**Accumulate modu:** koordinatör kendi oturumunda koştuğu için akış thread'ini ne
görür ne büyütür. Sonuç, `parallel` fold'u gibi **tek sentetik user/assistant
çifti** olarak thread'e katlanır → thread sıralı, alternatif ve assistant-sonlu kalır.

**Crash kurtarma:** `RecoverOrphanedTurns` `flow-coordinator` oturumlarını
**atlar** (koordinatör branch'i) ve bunlara bağlı yetim worker'lar için
`NotifyCoordinator` yapmaz — aksi halde `ResumeRunningFlows`'un düğümü yeniden
çalıştırmasıyla yarışır ve iş iki kez yapılırdı. Worker yine "interrupted" yanıtla
kapatılır.

### UI

`CoordinatorNode.tsx` (pusula ikonu, turuncu aksan `#ea580c`), palet + (ⓘ) yardımı,
`NodeInspector`'da ajan/görev/**workflow**/maxTurns/timeoutSec formu. Workflow seçici
**Oturum Bilgisi panelindekiyle aynı bileşendir** (`shared/components/CoordinatorWorkflowPicker`,
per-reçete (ⓘ) + 📖 skill linki) — `CoordinatorSection` de bu ortak bileşene taşındı, böylece
iki liste tek kaynaktan gelir ve ayrışamaz. Koşu görüntüleyicide düğüme
tıklayınca devredilen hedef + son rapor gösterilir; worker adımları koordinatörün
kendi oturumunda (Oturumlar ▸ "Akış Koordinatörü" kind'ı).

### Doğrulama

`internal/orchestration/coordinator_test.go` (render/knob geçişi, trace, accumulate
fold, kablosuz runner, panic, hata propagasyonu, validate) +
`internal/agent/flow_coordinator_test.go` (uçtan uca oturum/kind/maxTurns, boş yanıt,
eksik ajan, settle timeout, yerleşme koşulu, crash-kurtarma atlaması, **workflow
kalıcılığı + reçete `max_turns` fallback'i + düğüm tavanının reçeteyi ezmesi + bilinmeyen
reçetenin oturum açmadan reddi**).

Not: reçete doğrulama mantığı `internal/api`'den leaf `internal/skills` paketine taşındı
(`ResolveCoordinatorWorkflow`) — `internal/agent` `internal/api`'yi import edemez (döngü).
`api.ResolveCoordinatorRecipe` artık ince bir alias.
