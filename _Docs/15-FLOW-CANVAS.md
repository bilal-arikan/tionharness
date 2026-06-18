# 15 — Görsel Flow Builder (React Flow Canvas)

> Faz 7 orchestration akışlarının düzenleyicisi, form/liste editöründen **sürükle-bırak
> node-graph canvas**'a yükseltildi. Referans: SwarmClaw protokol builder'ı (React Flow),
> ComfyUI/LiteGraph bağlantı UX'i. Bkz. `_Docs/14-SWARMCLAW-PROVIDER-INCELEME.md` çizgisi.

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
- `internal/orchestration/model.go` — `Node` += `X,Y` (kozmetik).

## Canlı koşu

`runFlowStreamStandalone` SSE `onNode`: `start` → node `data.status='running'` (parlama),
`done` → `status='done'` (yeşil ring). Kalıcı trace yine altta node-node liste olarak gösterilir
(mevcut davranış korunur).

## Doğrulama (2026-06-18)

- `go build`/`go vet` yeşil; `tsc -b` + `vite build` yeşil (bundle ~988KB, React Flow nedeniyle;
  yalnız chunk-size uyarısı).
- **Playwright canlı test:** "Artı-Eksi Analizi" (paralel fan-out + join) akışı canvas'ta
  doğru render (⚡ paralel node + başlangıç rozeti, fan-out + `join` etiketli kenar, inspector
  seçili node'u düzenliyor), Controls + MiniMap çalışıyor. Gerçek claude-cli ile uçtan uca
  çalıştırma → **success**; trace `[parallel] Tartışma` (Lehte+Aleyhte) → `[agent] Sonuç`.
- **Pozisyon kalıcılığı:** API'den doğrulandı — akış canvas ile kaydedilince 4/4 node `x/y`
  taşıyor; kaydedilmemiş akışlar `x/y`'siz (auto-layout devrede).

## Notlar / sıradaki adımlar

- Bundle büyüdü; ileride React Flow'u dinamik `import()` ile code-split etmek düşünülebilir.
- SwarmClaw'daki salt-okunur "şablon görüntüleyici" modu ileride eklenebilir.
- Paperclip-tarzı statik "ajan ilişki haritası" (call_agent/send_agent_message kenarları)
  ayrı bir ekran olarak değerlendirilebilir.
