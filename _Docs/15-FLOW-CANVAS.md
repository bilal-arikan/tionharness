# 15 — Görsel Flow Builder (React Flow Canvas)

> Faz 7 orchestration akışlarının düzenleyicisi, form/liste editöründen **sürükle-bırak
> node-graph canvas**'a yükseltildi. Referans: SwarmClaw protokol builder'ı (React Flow),
> ComfyUI/LiteGraph bağlantı UX'i. Bkz. `_Docs/arsiv/14-SWARMCLAW-PROVIDER-INCELEME.md` çizgisi.

## Neden

Eski `FlowsPanel` node'ları üst üste kartlar olarak gösteriyor, bağlantıları `next`
dropdown'larıyla kuruyordu — graf topolojisi görünmüyordu. SwarmClaw'un (yeniden yazdığımız
ana proje) builder'ı React Flow canvas kullanıyor. Aynı kütüphaneyle, kodu kopyalamadan
aynı deneyim kuruldu.

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
- **Düzen:** açıklama üst toolbar'da ad'ın yanında; node'lar **sol palet**ten tıklanarak eklenir
  (ikonlu liste); inspector'da ajan seçimi avatarlı `AgentPicker`, prompt alanı yüksek + monospace.
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

## Notlar / sıradaki adımlar

- SwarmClaw'daki gibi şablonları **kategorilere** ayırma / arama eklenebilir.
- Koşu **silme / temizleme (cap)** ve koşudan **yeniden çalıştır** ileride eklenebilir.
- Paperclip-tarzı statik "ajan ilişki haritası" (run_subagent kenarları)
  ayrı bir ekran olarak değerlendirilebilir.
- MiniMap arka planı sabit `#0b0e14` (temaya duyarlı değil) — istenirse tema değişkenine bağlanır.
- Şablon `instantiateTemplate` varsayılan olarak ilk ajanı atıyor; ileride "ajan eşleme" adımı
  (her şablon node'u için ajan seçtirme) eklenebilir.
