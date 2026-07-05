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
  (`application/tionswarm-flow-node`); `FlowCanvas` içindeki `CanvasInner` (artık
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
- **Salt-okunur şablon galerisi:** `lib/flowTemplates.ts` 6 agent-bağımsız şablon
  (Sıralı Hat, Duygu Yönlendirici, Artı-Eksi, Eleştir-Düzelt, Planla-Uygula, Çoklu Uzman+Sentez).
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
- Liste öğesi: durum rozeti (▶ devam ediyor / ✓ başarılı / ✕ hata) + akış adı (flowId→`flows`
  map; silinmişse "（silinmiş akış）") + zaman.
- Seçilince **`flow/RunView.tsx`** (salt-okunur): başlık (ad + durum + girdi + `run.error` ⚠️),
  **aşama göstergeli canvas** (`FlowCanvas readOnly` + `graphToReactFlow`; node `data.status`
  `nodeStatuses(run,state)` ile türetilir: `trace`'tekiler `done`, `state.current` çalışırken
  `running` / hata ise `error`), ve **adım izi** listesi (node çıktıları `Markdown`, branch düz
  metin). Seçili koşu `runs` listesinden türetildiği için poll ile canlı tazelenir.
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
`evaluate → decide → generate` **geri-kenarı** doğrudan kurulabilir. _(Düzeltme: `tionswarm-flows`
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
`agent.skeptical-evaluator`, `mcp.playwright`) + yeni default skill `tionswarm-gan-loop`.
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
  (`paletteOpen`, `localStorage: tionswarm.flowPaletteOpen`). Daraltınca ipucu + node tip butonları
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
  açılabilir (`paletteVisible`, `localStorage: tionswarm.flowPaletteVisible`). Canvas sarmalayıcı
  `relative`; React Flow toolbar'ı top-right, Controls bottom-left olduğundan sol üst boş.
  Gizliyken canvas tam genişlik. (Palet içi "Node ekle" bölüm-daraltma `paletteOpen`'dan ayrıdır.)

## Koşular: "Adım izi" alttan açılıp kapanabilir + dikey yükseklik fix (2026-07-05)

- **Adım izi toggle.** `RunView`'deki "Adım izi" (step trace) bölümü artık **alttan açılıp
  kapanabilen** bir panel: başlık satırı chevron'lu toggle (`ChevronUp` kapalı → yukarı aç,
  `ChevronDown` açık) + "N adım" sayacı. `traceOpen` `localStorage: tionswarm.flowTraceOpen`'da
  kalıcı; **default dar ekranda (`< md`) kapalı**, md+'da açık.
- **Dikey yükseklik fix.** `RunView` kökü `min-h-0` aldı (flex çocukları düzgün küçülsün diye);
  trace listesi `max-h-[40%]` → `max-h-[40vh]` (kesin-yükseklik gerektirmeyen, daha kararlı).
  Kapalıyken canvas tüm yüksekliği alır — dar ekranda "yükseklik bozulması" giderildi.
