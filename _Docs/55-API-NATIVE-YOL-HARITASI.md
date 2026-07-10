# 55 — API-Native Özellikler Yol Haritası

> Anthropic Messages API'nin sunucu-tarafı yeteneklerinin TionSwarm'a kademeli entegrasyonu.
> Amaç: istemci tarafında elle kurduğumuz mekanizmaları, model bunlara göre eğitildiği için
> daha iyi çalışan API-native muadilleriyle tamamlamak/değiştirmek. Tümü yalnız **birinci-parti
> anthropic** sağlayıcıyı hedefler (`provider.Name() == "anthropic"` kapısı); claude-cli kendi
> döngüsünü, minimax-anthropic/custom uçlar kendi kısıtlarını korur.

Oluşturma: 2026-07-07 · Durum: **P0–P4, P6, P7 (batch hariç) tamam; yalnız P5 (memory tool) planlı**

> **Uygulama notu (2026-07-07, ikinci tur):** P2 (sunucu web search/fetch — `AnthropicWebTools`
> ayarı; 4.6+ modellerde `_20260209` dinamik-filtreli sürüm, eski modellerde ve PTC açıkken
> temel sürüm — çifte code-execution ortamı engellenir; tur başına max_uses 8/12) ve P3
> (server-side compaction — `AnthropicServerCompaction` ayarı; `compact-2026-01-12` beta +
> `compact_20260112` context_management edit'i; compaction blokları tur içinde RawContent
> verbatim echo ile korunur, turlar-arası transkripti istemci compaction yönetmeye devam eder)
> uygulandı. Her ikisi de Ayarlar → Bağlam → "Anthropic beta" altında ayrı toggle; ek
> sunucu/süreç gerektirmez — ikisi de mevcut /v1/messages çağrısının alanlarıdır.

> **Uygulama notu (2026-07-07):** P1 (structured outputs — titler + flow `outputSchema`/`jsonField`),
> P6 (mid-conversation system — steer mesajları Opus 4.8'de `role:system`, diğerlerinde
> `<system-reminder>` fold), P7 (strict fs/Grep araçları + `xhigh`/`max` thinking seviyeleri +
> `CountTokens`/`?accurate=1` önizleme sayımı) ve P4 (programmatic tool calling —
> `AnthropicProgrammaticTools` ayarı, `allowed_callers`, container zinciri, saf tool_result
> yanıtları, steer erteleme) uygulandı. Summarizer/System B serbest metin bırakıldı (çıktıları
> zaten metin ürünü — şema parse güvenilirliği kazandırmaz; System B compactor sonradan,
> 2026-07-10'da tamamen kaldırıldı). Flow-builder UI'da
> `outputSchema`/`jsonField` alanları henüz görsel olarak düzenlenemiyor (graf JSON'unda desteklenir).

---

## 0. Mevcut durum (tamamlananlar)

| Özellik | API yüzeyi | TionSwarm'daki hâli | UI karşılığı |
|---|---|---|---|
| Uzatılmış prompt cache (1h) | `cache_control` + `extended-cache-ttl` beta | 3 breakpoint (tools→system→rolling history), dinamik sonek breakpoint arkasında; **varsayılan açık** | Ayarlar → Bağlam toggle; oturum bağlam önizlemesi cache katmanlarını gösterir |
| Adaptive thinking + effort | `thinking:{adaptive}` + `output_config.effort` | Model-sınıf farkındalı `thinkingFor`; ThinkingLevel→effort eşlemesi; `display:summarized` | Ajan kartındaki thinking seviyesi; sohbette düşünme blokları |
| API-native context editing | `clear_tool_uses_20250919` | Opt-in (`AnthropicContextEditing`) | Ayarlar → Bağlam toggle |
| **Task Budgets** | `output_config.task_budget` + `task-budgets-2026-03-13` | Otonom turlara bütçe bildirimi; min 20K; adaptive sınıf | Ayarlar → Bağlam sayı alanı |
| **Native Tool Search** | `tool_search_tool_regex_20251119` + `defer_loading` | `DeferredDefs` tam katalog + raw passthrough + pause_turn devam; opt-in | Ayarlar → Bağlam toggle; keşif adımı sohbette araç kartı; pause_turn kurtarma kartı |
| Tool input examples | `input_schema.examples` | `ToolDef.Examples` → `foldExamples` | — (istek düzeyi) |

**Bilinen UI boşlukları (küçük işler):**
- [ ] Oturum bağlam önizlemesi (`session_context.go`) native-search modunda deferred kataloğu
      yansıtmıyor (ActiveDefs varsayar) — "shipped tools" listesine `deferred` rozeti eklenmeli.
- [ ] Task budget'ın tur-meta'da gösterimi: otonom mesaj balonuna "bütçe: N token" rozeti
      (Request'e yazılan değer TurnMeta'ya kopyalanıp UI'da render edilebilir).

---

## P1 — Structured Outputs (`output_config.format`) · Etki: yüksek · Efor: S

**Ne:** Yardımcı LLM çağrılarının çıktısını JSON şemayla garantiye almak. Serbest metin
parse'ı (kırpma, tırnak temizliği, "VERDICT:" string eşleşmesi) tamamen kalkar.

**Nerede kullanılacak:**
1. **Titler** (`titler.go`): `{"title": string}` şeması — tırnak/uzunluk temizliği kalkar.
2. **Summarizer** (`summarizer.go`): yapılandırılmış özet. (~~System B compactor
   `compactor.go`~~ — 2026-07-10'da built-in araç-çıktısı sıkıştırmasıyla birlikte
   tamamen kaldırıldı, bkz. `17-TOKEN-OPTIMIZASYON.md`.)
3. **Orchestration branch node**: `matchMode:"structured"` — node prompt'una şema eklenir,
   `{"verdict":"SHIP"|"FIX"}` gibi enum'lu karar → regex/contains kırılganlığı biter.

**Teknik:** `anthropicReq.OutputConfig.Format = {type:"json_schema", schema:...}`;
yalnız destekleyen modellerde (Fable 5, Opus 4.8, Sonnet 5, Haiku 4.5 — capability tablosu).
İlk istek şema derleme gecikmesi (24h şema cache'i) — titler gibi sık çağrılarda amorti olur.
Citations ile uyumsuz (şu an kullanılmıyor → risk yok).

**UI:** Branch node editörüne "yapılandırılmış karar" seçeneği (enum değerleri alanı).

---

## P2 — Sunucu-tarafı Web Search + Web Fetch · Etki: yüksek · Efor: M

**Ne:** `web_search_20260209` / `web_fetch_20260209` sunucu araçları (dinamik filtreli sürüm).
Native anthropic ajanları gerçek web araması + sayfa çekme kazanır; sonuçlar alıntılı (citations)
gelir; dinamik filtreleme sonuçları bağlama girmeden kod-tarafında süzer.

**Teknik:**
- `toAnthropicTools`'a koşullu sunucu araç girdileri (native tool search'teki desenle aynı).
- Yanıtta `web_search_tool_result`/`web_fetch_tool_result` blokları → **RawContent passthrough
  altyapısı P0'da kuruldu, yeniden kullanılır**; `pause_turn` işleme de hazır.
- Hata blokları raise etmez (`content` obje ise hata) — parse dalı gerekli.
- Builtin `WebSearch`/`WebFetch` ile örtüşme: native modda builtin'ler anthropic yolunda
  def listesinden çıkarılır (Anthropic "örtüşen araç bırakma" ilkesi).
- Maliyet: arama başına ücret (+token) — ayar başına per-workspace toggle + `max_uses` sınırı.

**UI:** Arama sonucu kartı (kaynak listesi + alıntılar); Ayarlar → Araçlar'da toggle + max_uses.

---

## P3 — Sunucu-tarafı Compaction (`compact-2026-01-12` beta) · Etki: orta-yüksek · Efor: M-L

**Ne:** İstemci-tarafı compaction'ın (manager.go) API-native muadili: sunucu, eşiğe yaklaşınca
geçmişi kendi özetler ve `compaction` blokları döner; sonraki isteklerde bloklar aynen geri
gönderilir (RawContent altyapısı yine yeniden kullanılabilir).

**Neden:** Özet kalitesi model-tarafı; compaction LLM çağrısının maliyeti/karmaşası bizden
çıkar; "monoton büyüyen özet" sorunu sunucu yönetimine geçer.

**Teknik/risk:**
- Mevcut sistemle **birlikte değil, yerine** çalışmalı (çifte özet çakışır) → `CompactionMode:
  "client" | "server"` ayarı; server modunda `Manager.Prepare` fold atlar, yalnız bütçe ölçer.
- `response.content`'in compaction blokları dahil eksiksiz geri gönderilmesi ŞART — turlar
  arası kalıcılık gerekir: compaction blokları db'ye persist edilmeli (yeni mesaj alanı) —
  en büyük iş kalemi bu.
- Beta + yalnız anthropic; diğer sağlayıcılar client modda kalır.
- A/B: `_Docs/17` ölçüm düzeneğiyle iki modun token/kalite karşılaştırması.

**UI:** Oturum bağlam önizlemesinde "sunucu compaction" rozeti + pre_compaction_tokens göstergesi.

---

## P4 — Programmatic Tool Calling (`code_execution_20260120` + `allowed_callers`) · Etki: orta · Efor: L

**Ne:** `run_code`'un API-native muadili: Claude, Anthropic'in sandbox'ında Python yazar ve
TionSwarm araçlarını **kod içinden** çağırır; ara sonuçlar bağlama hiç girmez.

**Mevcut run_code'dan farkı:** yerel Python kurulumu gerekmez (sunucu container'ı);
model bu akışa göre eğitilmiş; tool_use round-trip'leri sunucu içinde döner.

**Teknik:**
- Custom tool def'lerine `allowed_callers: ["code_execution_20260120"]` alanı (ToolDef'e ekleme).
- `code_execution` sunucu aracı + beta; container id yönetimi (tur içi yeniden kullanım).
- Programatik çağrı yanıtlarken user mesajı YALNIZ tool_result blokları içermeli → tool loop'ta
  ayrı dal. `strict:true`, `disable_parallel_tool_use` ve MCP araçlarıyla uyumsuz —
  MCP araçları bu modda dışarıda kalır (builtin'ler girer).
- Yerel `run_code` (codemode) korunur: MCP + yerel dosya sistemi gerektiren işler onda.

**UI:** Kod yürütme kartı (stdout/stderr + içeriden yapılan araç çağrıları alt-adım listesi).

---

## P5 — Memory Tool (`memory_20250818`) · Etki: orta · Efor: M

**Ne:** Claude'un eğitildiği native hafıza protokolü: `view/create/str_replace/insert/delete/
rename` komutlarıyla `/memories` dizini. İstemci-tarafı araç — depoyu TionSwarm sağlar.

**Teknik:**
- Def `{type:"memory_20250818", name:"memory"}` — şemasız Anthropic-tanımlı araç →
  `anthropicTool.Type` alanı P0'da eklendi, yeniden kullanılır.
- Depo: `<workspace>/memory/<agent-id>/` (mevcut fs sandbox + freshness guard yeniden kullanılır).
- Mevcut progress/handoff dosyalarıyla ilişki: memory = uzun-vadeli, progress = görev-içi;
  ikisi ayrı kalır, autonomous-ops skill'ine kullanım rehberi eklenir.
- Güvenlik: path traversal koruması (sandbox Resolve), sır saklamama uyarısı.

**UI:** Ajan detayında "Hafıza" sekmesi (dizin ağacı + dosya önizleme + elle düzenleme).

---

## P6 — Mid-conversation System Messages (Opus 4.8) · Etki: küçük-orta · Efor: S

**Ne:** Tur ortası operatör talimatı (`{"role":"system"}` mesaj-içi) — cache'i bozmadan ve
sahtelenemez kanaldan. Şu an lifecycle-hook bağlamı ve steer mesajları user-turn metni olarak
giriyor.

**Teknik:** `toAnthropicMessages` şu an `RoleSystem`'ı atlıyor → Opus 4.8 + native yolda
system mesajını geçir; diğer modellerde mevcut davranış (user-turn `<system-reminder>` düşüşü).
Konum kuralları: messages[0] olamaz, user'dan sonra gelmeli. Steer akışı (`drainSteer`) ve
lifecycle `passContext` bu kanala taşınabilir (model-farkındalı dallanma).

**UI:** Steer mesajları sohbette zaten görünüyor; değişiklik yok.

---

## P7 — Küçük kalemler

| İş | Not | Efor |
|---|---|---|
| `count_tokens` kalibrasyonu | Heuristik `EstimateTokens`'ı gerçek API sayımıyla periyodik kalibre et (model başına düzeltme katsayısı; compaction tetiği isabetlenir) | S |
| `strict: true` araç girdisi | Eager builtin şemalarında `additionalProperties:false` zaten var → def'e alan eklemek yeterli; PTC ile uyumsuz (P4 modunda kapat) | S |
| Effort'un ayrı ayar olması | ThinkingLevel→effort dolaylı eşleme yerine ajan kartına doğrudan effort seçimi (low/medium/high/xhigh/max; model desteğine göre filtrele) | S |
| Batch API | Gece toplu işleri (özet yenileme, etiket taraması) %50 indirimli batch'e taşımak — bekleyen somut iş yükü yok, ihtiyaç doğunca | M |

---

## Önerilen sıra ve bağımlılıklar

```
P1 Structured Outputs (bağımsız, hızlı kazanç)
P2 Web Search/Fetch (RawContent altyapısını kullanır — P0'a dayanır)
P6 Mid-conversation system (bağımsız, küçük)
P7 count_tokens + strict + effort (bağımsız küçükler)
P3 Server compaction (persist şeması gerektirir — en dikkatli iş)
P5 Memory tool (bağımsız, UI'lı)
P4 PTC (en büyük; P2'nin container deneyiminden sonra)
```

Her faz: opt-in ayar → tek ajanla canlı deneme → debug journal ölçümü → varsayılan kararı.
(Bu, extended-cache/task-budget/tool-search'te izlenen desenle aynı.)
