# SwarmGo — Yol Haritası (Aşama Aşama)

> İlke: **MVP ile başla, katman katman büyüt.** Her faz çalışan ve test edilebilir bir çıktı verir.

```mermaid
graph LR
    F0[Faz 0<br/>Iskelet + Ortam] --> F1[Faz 1<br/>DB + Config]
    F1 --> F2[Faz 2<br/>Provider + Chat MVP]
    F2 --> F3[Faz 3<br/>Web UI]
    F3 --> F4[Faz 4<br/>Agent Runtime]
    F4 --> F5[Faz 5<br/>Tasks + Schedules]
    F5 --> F6[Faz 6<br/>Memory]
    F6 --> F7[Faz 7<br/>Orchestration]
    F7 --> F8[Faz 8<br/>MCP]
    F8 --> F9[Faz 9<br/>Wails Paketleme]
```

---

> **Durum (2026-06-16):** Faz 0–8 (Faz 8, Faz 7'den önce) + Workspace İzolasyonu + Faz 6.5 + SDK paritesi P1/P2 tamamlandı; ✅ işaretleri canlı durumu yansıtır. **Sıradaki:** Yapılacaklar / Backlog. Plan-dışı ara özellikler ve günlük: [05-ILERLEME.md](05-ILERLEME.md).

## Faz 0 — İskelet ve Ortam ✅
- [x] Go kurulumu (1.26.4)
- [x] Proje klasör yapısı + `go mod init`
- [x] `_Docs` plan dokümanları
- [x] `main.go` temel HTTP sunucu (health check)
- [~] Wails v2 → **Faz 9'a ertelendi**
- **Çıktı:** `go run` ile ayağa kalkan, `/health` dönen sunucu. ✅

## Faz 1 — Kalıcılık ve Config ✅
- [x] `internal/db`: (başlangıçta SQLite + migration runner; **2026-06-15'te dosya-tabanlı store'a taşındı** — JSON/JSONL, DB yok, bkz. `08-DEPOLAMA.md`)
- [x] `internal/config`: env + AES-GCM credential secret
- **Çıktı:** Açılışta veriyi diskten yükleyen uygulama. ✅

> › **Güncelleme (2026-06-15):** SQLite (`modernc.org/sqlite`), migration runner ve `migrations/*.sql` kaldırıldı; kalıcılık bellek-içi maps + atomik diske yazma ile dosya sistemine taşındı.

## Faz 2 — Provider Katmanı + Chat MVP ✅
- [x] `internal/providers`: `Provider` arayüzü
- [x] **anthropic** (SDK yerine ince HTTP istemci) **+ claude-cli** (anahtarsız, OAuth/abonelik) **+ minimax** (OpenAI-uyumlu)
- [x] Streaming → **SSE** ile geldi (`POST /api/chat/stream`, Chat UX fazında — bkz. [07-CHAT-UX.md](07-CHAT-UX.md))
- [x] `/api/chat`: mesaj → LLM → DB
- **Çıktı:** Çok-turlu sohbet, hafıza korunuyor. ✅

## Faz 3 — Web UI ✅
- [x] `frontend/`: React + Vite + TS + **Tailwind v4** (shadcn/ui yerine kendi bileşenlerimiz)
- [x] Chat ekranı (mesaj listesi + composer, optimistic UI)
- [x] Canlı streaming → **SSE** (WebSocket değil) ile sonradan eklendi — bkz. [07-CHAT-UX.md](07-CHAT-UX.md)
- [x] Ajan listesi / oluşturma
- **Çıktı:** Tarayıcıdan kullanılabilir koyu temalı arayüz. ✅

## Faz 4 — Agent Runtime ✅
- [x] `internal/agent`: goroutine tabanlı ajan döngüsü (worker.go)
- [x] Wake signal (heartbeat `time.Ticker` + manuel wake kanalı)
- [~] Tool loop → bu fazda yoktu; **Faz 8'de eklendi** (`agent/toolloop.go`)
- [x] Outcome classification + exponential backoff (10 hata → devre dışı)
- **Çıktı:** Otonom çalışan, kendi kendine uyanan ajan. ✅

## Ara Faz — Workspace İzolasyonu ✅ (plan dışı, kullanıcı talebiyle)
- [x] Her workspace = ayrı `store/` dizini + ayrı Runtime + ayrı Scheduler (fiziksel ayrım)
- [x] `internal/workspace/manager.go`, `withWorkspace` middleware, switcher UI
- **Çıktı:** Sızıntısız çoklu workspace. Detay: [06-WORKSPACES.md](06-WORKSPACES.md). ✅

## Faz 5 — Tasks + Schedules ✅
- [x] Görev panosu (kanban), `tasks`/`runs` CRUD, `RunTask` executor ("şimdi çalıştır" + run geçmişi)
- [~] Delegasyon (alt-ajan) → **Faz 7'ye taşındı**
- [x] `internal/agent/scheduler.go`: `robfig/cron` (workspace başına)
- [x] UI: Task board + Schedules ekranı
- **Çıktı:** Zamanlanmış görev/prompt teslimi. ✅
- **Kanban iyileştirme turu (2026-06-17):** ajan avatarları, sağ detay/düzenleme paneli
  (sürükle-genişlet), ajan görev tool ailesi (`builtin_taskmgmt.go`). Detay: `05-ILERLEME.md`.
- **⚠️ Pivot — Pasif durum panosu (2026-06-18):** pano artık **iş çalıştırmaz**, sadece
  durum yansıtır; iş flow/schedule/agent oturumunda yapılır. Karttan çalıştırma, cron bağlama,
  geçmiş, prompt kaldırıldı; oluşturma **açıklama** ile (başlık otomatik), Ajan/Flow opsiyonel
  bilgi etiketi. Backend temizliği: `run_task` agent tool + schedule↔task (karttan-cron) bağı
  **silindi**; ajan görev tool ailesi 5 araca indi (list/create/update/move/delete). Yetim run yolu
  da temizlendi: `RunTask`/`RunTaskStream`/`runTaskFlow` + `/run`·`/run-stream`·`/runs` uçları +
  ölü DB metotları silindi (`db.Run` + `ListRunningRuns` korundu — activity/executions feed). Detay: `05-ILERLEME.md`.
- [~] **Görev dispatcher'ı:** pivot sonrası **kapsam dışı** — pano pasif olduğundan
  "ajan todo'yu otomatik koşar" akışı artık hedef değil. Otomasyon flow/schedule katmanında.

## Faz 6 — Memory ✅
- [x] `internal/memory`: doküman + journal + reflection
- [x] Recall: **embedding yerine saf Go lexical cosine** (anahtarsız/çevrimdışı; embedding ileride takılabilir)
- [x] Dream cycle (`Reflect`) — journal'ı provider'a özetletip reflection üret
- **Çıktı:** Hatırlayan, yansıtan ajanlar; sohbet+göreve otomatik enjeksiyon. ✅

## Faz 6.5 — Sağlamlaştırma ✅ (plan dışı, SwarmClaw kıyas açığı)
- [x] `internal/conversation`: token-bütçeli **compaction** (rolling summary)
- [x] Otonom döngü **bütçe guardrail'i** (`guardedComplete` + `agent_usage`, ajan başına günlük limit)
- [x] UI: context + bütçe meter; 3 kolonlu yerleşim (NavRail)
- **Çıktı:** Uzun oturumlarda taşma yok, otonom maliyet sınırlı. ✅

## Faz 7 — Orchestration ✅ (Faz 8'den sonra yapıldı)
- [x] `internal/orchestration`: graf motoru (`model.go`/`engine.go`), yapılandırılmış akışlar
- [x] Branch / parallel join (agent/branch/parallel node; `maxSteps` döngü guard)
- [x] Şablon sistemi (`{{input}}`/`{{last}}`/`{{node.<id>}}`) + restart-safe run state (`flow_runs`, her node sonrası persist + boot'ta resume)
- [x] UI: görsel protokol builder (`FlowsPanel.tsx`, "🔀 Akışlar")
- [~] Loop node → **henüz yok** (branch + maxSteps ile döngüler dolaylı kurulabilir)
- **Çıktı:** Çok adımlı, dallanan iş akışları. ✅

## Faz 8 — Tool-use + MCP Entegrasyonu ✅ (Faz 7'den önce yapıldı)
- [x] `internal/mcp`: **SDK yerine elle JSON-RPC 2.0** istemci (mark3labs/mcp-go değil — bağımlılıksız felsefe)
- [~] Transport: **yalnızca stdio**; SSE / HTTP → henüz yok (net "desteklenmiyor")
- [x] Native tool-use protokolü (`providers` ToolDef/ToolCall/ToolResult + anthropic content-block + `agent/toolloop.go`) **ve** anahtarsız claude-cli MCP delegasyonu (`--mcp-config`)
- [x] `internal/tools`: built-in (get_current_time/http_get/memory_recall) + MCP birleşik registry; ajan başına `mcp_enabled` + allowlist
- [x] UI: "🔌 Araçlar" paneli (`ToolsPanel.tsx`) + "🔀 Akışlar"
- **Çıktı:** Harici MCP sunucularına bağlanan, araç kullanan ajanlar. ✅

## Faz 9 — Wails Paketleme + Çoklu Platform
- [ ] Wails ile native pencere entegrasyonu
- [ ] `wails build` → Windows `.exe`
- [ ] GitHub Actions: Win/macOS/Linux otomatik derleme
- **Çıktı:** Dağıtıma hazır masaüstü uygulaması.

> **Fikir (2026-06-22) — Sistem tepsisi (system tray) entegrasyonu.** SwarmGo arka planda
> çalışan otonom-ajanlı bir runtime; masaüstü dağıtımında **tray'e küçülme + durum göstergesi**
> (kaç ajan aktif/çalışıyor) + hızlı menü (workspace aç/durdur, çıkış) + mevcut tür-bazlı
> masaüstü bildirimleriyle bütünleşme doğal bir tamamlayıcı.
> - **Önce Wails'in yerleşik tray API'sine bak** — varsa ayrı bağımlılığa gerek yok.
> - **B planı:** [`gogpu/systray`](https://github.com/gogpu/systray) — **saf Go, CGO'suz** tray
>   kütüphanesi (Win `Shell_NotifyIconW` · macOS `NSStatusBar` · Linux D-Bus
>   StatusNotifierItem). SwarmGo'nun "tek binary, çapraz-derleme, minimal bağımlılık"
>   felsefesiyle birebir uyumlu. **Uyarılar:** (1) v0.1.0 — çok genç, üretime erken; (2) Wails'in
>   kendi event loop'u ile systray message-pump'ı çakışabilir (özellikle macOS main-thread →
>   deadlock riski), entegrasyonda test şart. Yalnız native masaüstü modunda anlamlı; web
>   dağıtımında işlevsiz.

> **Not (2026-06-16):** **Connectors fazı (Discord/Slack/Telegram köprüleri) kapsamdan çıkarıldı.** İhtiyaç olursa ayrı bir faz olarak yeniden değerlendirilebilir.

---

## Yapılacaklar / Backlog (canlı liste)

> SDK paritesi (P1–P4) + `observed-behavior` mimari incelemesinden çıkan işler. Kavramsal detay: [10-KAVRAMSAL-TASARIM-NOTLARI.md](10-KAVRAMSAL-TASARIM-NOTLARI.md). Her madde bittiğinde işaretle ve [05-ILERLEME.md](05-ILERLEME.md)'ye günlük gir.

### Tamamlananlar ✅
- [x] **Faz P2** — Built-in dosya/shell araçları (sandbox'lı `read/write/edit/list/glob/grep` + gate'li `shell`)
- [x] **D2 (kısmi)** — Provider native token streaming (`Streamer`: anthropic+minimax) + claude-cli stream-json → SSE
- [x] **Faz P1** — Etkileşim araçları: `todo_write` (checklist) + `ask_user` (SSE-blok suspend/resume)
- [x] **E3** — Trace `StepKind` genişletme: `ask`/`todo`/`recovery`/`error`/`steer` + `tool_delta`/`tombstone` (akan shell üreticili) + **Ayarlar ▸ Adım Türleri** referans ekranı
- [x] **Faz A1** — Artifact sistemi: sürümlü içerik (doküman/kod/HTML/SVG/Mermaid), `create/update_artifact` araçları, Artifactlar ekranı + sohbet kartı
- [x] **İki-seviyeli araç yönetimi** — workspace-geneli aktivasyon (denylist `tools-config.json`) + ajan-bazlı seçim (allowlist); `WorkspaceToolCatalog`/`ActiveToolCatalog`/`ToolCatalog` + `GET/PUT /api/workspace-tools`
- [x] **Talep-üzerine özetler** — "/" komut paleti: hafıza/görev panosu/akışlar (ucuz model) + araç listesi (deterministik); `Runtime.Summarize` + `POST /api/sessions/{id}/summary`
- [x] **D2** — Provider **retry middleware**: `transport.go` `doWithRetry` (üstel backoff + jitter, `Retry-After` saygılı, 429/5xx/529 + ağ hatası); `postJSON`/`postSSE` sarıldı (+ token streaming `Streamer`)
- [x] **C1** — Sistem-prompt **cache sınırı**: `Request.System` (statik: persona+profil) / `Request.SystemDynamic` (dinamik: bellek+özet); Anthropic cache breakpoint yalnız statik blokta → araç+statik prefix cache'lenir, dinamik suffix cache'i bozmaz
- [x] **Ara özellikler** — otonom olay akışı (`/api/events`), workspace switcher + çapraz-ws rozet, tıklanabilir bildirimler, sessions-only sidebar + okundu/okunmadı, tema presetleri
- [x] **A1 (loop recovery)** — Agent loop **recovery + `continuationReason`**: saf karar katmanı (`agent/recovery.go`: `loopState`+`decideRecovery`), max-token resume (guard'lı + partial-stitch + withhold), reaktif compaction (`conversation/reactive.go`, assistant-sınır fold), minimax `length`→`max_tokens` map; `recovery_test.go`+`reactive_test.go`. **Kalan:** max-token escalation merdiveni (8k→64k) *(A3 iptal sentetiği ✅ 2026-06-19)*
- [x] **Self-management genişlemesi** ✅ 2026-06-19 — öz-yönetim araç ailesine **hooks/MCP/secret/skill/settings** eklendi (`builtin_{hookmgmt,mcpmgmt,secretmgmt,skillmgmt,settings}.go`); provenance guard'ı (`created_by`); öğretici default skill `swarmgo-self-management` + ayar referansı `swarmgo-settings`. Bkz. `_Docs/24-SELF-MANAGEMENT.md`
- [x] **Ayarlar canlı-uygulama + validation** ✅ 2026-06-19 — `get_settings`/`update_settings` tool'ları + `settings.Validate` (enum reddi/clamp) + bridge wiring + `settings` SSE event'i ile çok-pencere senkronu. Bkz. `_Docs/24-SELF-MANAGEMENT.md`
- [x] **İlişki Grafiği** ✅ 2026-06-19 — Workspace Ağı (NavRail) + Hafıza Bilgi Grafiği (salt-okunur React Flow ağları, Fizik/Küme yerleşim). Bkz. `_Docs/23-ILISKI-GRAFIGI.md`
- [x] **CLI araç köprüsü (CLI-1/2/3)** ✅ 2026-06-19 — claude-cli ajanları Interaction MCP üzerinden: `use_skill` skill-gövde yükleme (CLI-1), advertise+allowlist tek-kaynak (`InteractionEndpoint.ToolNames`, CLI-2), lazy self-management ailesi köprüsü (`BridgeTools`/`Tools(token)`, CLI-3). Bkz. `_Docs/11-INTERACTION-MCP.md`. Kalan: claude-cli ile canlı uçtan-uca doğrulama.
- [x] **Lazy araç yükleme** ✅ — Self-management + MCP araç şemaları tura girmez; sistem promptunda özet katalog yayımlanır, `tool_search` ile keşfedilir, `activate_tools`/`deactivate_tools` ile istenince tam şema aktive edilir (`internal/tools/activetools.go`, `builtin_activate.go`). Bkz. `_Docs/19-LAZY-TOOL-LOADING.md`.
- [x] **Prefix'li insan-okunabilir ID'ler** ✅ — Workspace `WS<n>`, ajan `AGT<n>`, oturum `SES<n>` biçiminde monoton sayaç ID'leri; `internal/workspace/id.go` + `ws-counter.json` ile yeniden başlamada sayaç korunur; tek seferlik migrasyon: `cmd/migrate-ids/`.
- [x] **Tek-binary web dağıtımı** ✅ — `frontend/dist/` `go:embed all:dist` ile derleme anında binary'ye gömülür (`internal/web/embed.go`); ayrı statik sunum gerekmez.

### Mimari sıçrama
- [x] **A2** ✅ (2026-06-19) — **Subagent / Task izolasyonu**: `AgentContext` + `runAgent` çekirdeği; `run_subagent` aracı (profil: `explore`/`coder`/`reviewer`); paralel fan-out; `subagent` StepKind + `SubagentStep.tsx`. `call_agent`/`send_agent_message` kaldırıldı; `spawn_session` native tool'dan kaldırıldı (`run_subagent` async moduna taşındı). **Detay:** `25-SUBAGENT-ISOLATION.md`.
- [x] **A3** — İptal hiyerarşisi: tur-içi iptalde (`toolloop.go`) yarım kalan tool_call'lara sentetik `cancelled` tool_result (`fillCancelledResults`) → dangling tool_use yok; `cancel_test.go` (2026-06-19)

### Araç & yetki katmanı
- [ ] **B1** — Tool sözleşmesi v2: `ReadOnly()`/`ConcurrencySafe()`/`ValidateInput()` + `BaseTool` varsayılanları
- [ ] **B4** — Paralel tool yürütme (read-only'leri `errgroup`) + büyük çıktı için disk-spill + referans
- [x] **Faz P4** — Hooks (`PreToolUse`/`PostToolUse`, subprocess JSON I/O) ✅ 2026-06-18 — `internal/agent/hooks.go`, Ayarlar → Hooks; bkz. `_Docs/18-HOOKS.md`

### Bağlam, bellek, trace
- [ ] **C5** — **Recency + importance ağırlıklı recall** (Generative Agents, Park et al. 2023): mevcut
  `memory.Recall` saf cosine (yalnız *relevance*). Üzerine iki sinyal eklenir →
  `score = α·relevance + β·recency + γ·importance`. **recency** = son erişimden bu yana üstel sönüm
  (`exp(-λ·Δt)`, erişimde `LastAccess` tazelenir); **importance** = belleğe yazılırken 1–10 arası bir önem
  skoru (ucuz LLM ya da heuristik; `Memory.Importance` alanı). `minScore` eşiği ağırlıklı skora uygulanır.
  Geri-uyumlu: β=γ=0 → bugünkü davranış. İlham: `didiforgithub/SwarmAgent`'ın taklit ettiği orijinal
  Generative Agents "memory stream" deseni (o repo'da kod stub; fikir makaleden alındı). İlişkili: **C3**.
- [ ] **C6** — **MemGPT/Letta tarzı self-editing bellek + memory-pressure sinyali** (`letta-ai/letta`,
  "LLM as OS" deseni): compaction'ı ajandan gizli tutmak yerine belleği **ajanın açık kontrolüne** ver.
  İki parça → (a) **memory-pressure sinyali**: `conversation.Manager` bağlam bütçesine yaklaşınca ajana
  sistem-uyarısı enjekte eder ("bağlam doluyor, önemliyi belleğe yaz") + ajan `memory_write` (C3) ile neyin
  kalıcı olacağına karar verir (sessiz oto-katlamadan önce); (b) **self-editing core memory bloğu**:
  `SystemDynamic` içinde ajanın `core_memory_append/replace` araçlarıyla güncelleyebildiği küçük kalıcı
  "çalışma belleği" bloğu (Letta'nın human/persona memory-block'larına karşılık). SwarmGo'nun iki-parçalı
  sistem promptu + Faz 6 recall + C3 bunun altyapısı; eksik olan **ajana açık araç yüzeyi + pressure
  sinyali**. İlişkili: **C2** (compaction), **C3** (memory_write), **HA-1** (kullanıcı modelleme).
  **Detaylı uygulama planı:** [`26-MEMGPT-CORE-MEMORY.md`](26-MEMGPT-CORE-MEMORY.md) (Mod C — neden doğrudan
  Letta değil + 3 parça gerçek dosya temas noktalarıyla, 2026-06-22).
- [ ] **C3** — memdir benzeri bellek **yazma/indeksleme** (`memory_write`, frontmatter türleri) — şu an sadece recall
- [x] **C4** — Maliyet takibi: `cache_creation` vs `cache_read` ayrımı (uçtan uca) + oturumlar arası kümülatif toplam & `cacheHitRate` (caching ROI) — Bütçe ekranı pencere-kümülatif kartları + trend maliyet/tasarruf (2026-06-19)
- [~] **E3 kalan** — `subagent` StepKind ✅ (A2 ile tamamlandı); `tombstone`/`tool_delta` (canlı adım güncelleme altyapısı) — kalan
- [ ] **C2** — Compaction emniyet katmanı (`snip`) — 1M tampon var, düşük öncelik

### MCP & dağıtım
- [ ] **D1** — MCP çoklu-transport (stdio + SSE/HTTP factory) + config kapsam-zinciri (local<user<project)
- [ ] **Faz 9** — Wails paketleme (yukarıdaki Faz 9 bloğu)
- [ ] **D3** — Server/Remote/Bridge (kapsam dışı, not olarak saklanır)

---

## external-agent-oss İncelemesinden (2026-06-17)

> Kaynak: [external-agent-project/external-agent-oss](https://github.com/external-agent-project/external-agent-oss) v0.2.19→v0.10.3 (71 release) analizi. Tam gerekçe + kod-doğrulama (EXISTS/MISSING) + sürüm-sürüm liste: [13-CRAFT-AGENTS-INCELEME.md](13-CRAFT-AGENTS-INCELEME.md). Maddeler SwarmGo koduna karşı doğrulandı.

### 🔴 P0 — Doğrulanmış boşluklar (yüksek etki)
- [x] **CG-1 — Tool çıktısı boyut sınırı** ✅ **YAPILDI** (commit `69601ab`, 2026-06-17): `tools/registry.go` `capToolOutput()` (100K bayt, UTF-8 sınırında trunc + `…[truncated N bytes]`) `Registry.Call`/`CallStream`'de built-in + MCP tüm başarılı çıktılara uygulanır. `registry_cap_test.go`. *(craft v0.4.4)*
- [x] **CG-2 — http_get SSRF koruması** ✅ **YAPILDI** (commit `69601ab`, 2026-06-17): `builtin_http.go` özel `net.Dialer.Control` guard'ı çözülen IP'yi her dial'da denetler (loopback/unspecified/link-local/private/ULA/CGNAT + 169.254.169.254 metadata reddedilir; redirect/DNS-rebind kapsanır) + http/https şema kontrolü. `builtin_http_test.go`. *(craft v0.3.2, v0.5.0)*
- [x] **CG-3 — Thinking resolver model-sınıf farkındalığı** ✅ **YAPILDI** (commit `69601ab`, 2026-06-17): `providers.RequiresAdaptiveThinking(model)` + `agent.resolveThinkingBudget`; Fable/Mythos 5'te "off"/"low" → `MinAdaptiveThinkingBudget=1024`, Opus/Sonnet/Haiku değişmez. `thinking_test.go`. *(craft v0.10.3)*
- [x] **CG-4 — MCP şema normalizasyonu** ✅ **YAPILDI** (commit `69601ab`, 2026-06-17): `mcp.NormalizeSchema()` `Registry.Defs`'te her MCP şemasına uygulanır — `$schema`/`$id`/`$ref`/`$defs`/`definitions` recursive strip, kök object garanti; `additionalProperties`/`required`/`oneOf`/`anyOf`/`allOf` korunur. `normalize_test.go`. *(craft v0.7.3, v0.7.5, v0.7.12)*

### 🟠 P1 — Çok-ajan mimarisine uyan
- [x] **CG-5 — Ajanlar-arası mesajlaşma** (`send_agent_message`) ✅ **YAPILDI** (commit `69601ab`, 2026-06-17): `tools/builtin_agentmsg.go` self-manage gate'li — hedef ajanın `agent-inbox` oturumuna user-mesaj append + `Runtime.Wake` (fire-and-forget; `call_agent` senkron delegasyonun async tamamlayıcısı). `agentmsg_test.go`. Native yolunda. *(craft v0.8.8)* — **Sonraki durum (A2, 2026-06-19):** `send_agent_message` ve `call_agent` kaldırıldı; `run_subagent` ile birleştirildi.
- [~] **CG-6 — Oturum öz-yönetim araçları** *(kısmî)*: **`list_sessions`** built-in tool'u **var** (`tools/builtin_sessions.go`, cross-session farkındalık, commit `54ab736`). **Kalan:** `set_session_labels`/`set_session_status`/`get_session_info` → kendini-kapatan otomasyon (görev bitince status=done → trigger). *(craft v0.8.3)*
- [~] **CG-7 — Hooks + koşullu otomasyon + webhook** *(kısmî)*: (a) command hook'ları (olay→shell, timeout + fail-open) **✅ YAPILDI** — Faz P4 (`internal/agent/hooks.go`, bkz. `_Docs/18-HOOKS.md`); ajan da `create_hook`/`delete_hook` ile yönetebilir (`_Docs/24-SELF-MANAGEMENT.md`). **Kalan:** (b) otomasyon koşulları (time/state/label gate); (c) webhook action (exp. backoff retry); prompt-hook'ları + rate limiter. `events` bus + `scheduler` ile örtüşür. *(craft v0.4.3, v0.7.5, v0.7.7)*
- [ ] **CG-8 — Otomasyon/flow geçmişi cap + compaction**: `flow_runs` sınırla (örn. 20/flow, 1000 global) + periyodik compaction. *(craft v0.7.8)*

### 🟡 P2 — Sağlamlaştırma (file-based depolama)
- [ ] **CG-9 — Density-aware token tahmini**: sabit `chars/4` (`tokens.go:8`) base64/yoğun içerikte ~%25 eksik sayıyor → `chars/1.5` dal + tool-result eşiğini context window'a göre dinamik (floor 2K/ceil 15K). *(craft v0.9.1, v0.9.3)*
- [ ] **CG-10 — Config oto-onarım**: başlangıçta bozuk config/`~/.claude.json` (boş/BOM/invalid) tespit+onarım+backup; Windows file-lock retry. *(craft v0.2.33)*
- [ ] **CG-11 — Volatile-context kuralını koru**: tarih/saat/session-state eklenirse **yalnız `SystemDynamic`'e veya user-mesaj kuyruğuna** (statik cache prefix'e değil). Mevcut iki-parçalı tasarım zaten doğru — regression'a karşı not. *(craft v0.10.2)* ✅ tasarım uyumlu

### 🟢 P3 — Güvenlik (web UI / remote açılırsa)
- [ ] **CG-12 — URL şema blocklist** *(düşük risk — kontrol edildi)*: `Markdown.tsx` `isExternal` yalnız `http(s)` eşliyor; `mailto:`/`#`/`http(s)` → `<a href>`, **diğer tüm şemalar** (`javascript:`/`file:`/`data:`/`vscode:`…) `onOpenFile` (yerel dosya) dalına düşüyor → doğrudan `href` XSS yok, ama keyfi şema string'i callback'e gidiyor. Açık allowlist/blocklist + `onOpenFile`'da şema doğrulaması ekle. *(craft v0.8.12, v0.9.6)*
- [ ] **CG-13 — Shell sandbox escape**: `find -exec` vb. engelle (shell açıkken). *(craft v0.5.0)* — **Faz P3 ile birlikte**
- [ ] **CG-14 — Remote açılırsa**: WebUI auth (argon2id + JWT + rate limiter) · WS TLS zorunlu (`wss://`) · upload/artifact yollarında path-traversal sanitize doğrula · CI'da `go mod verify`. *(craft v0.8.2, v0.7.0, v0.3.2, v0.8.0)*

### 🔵 P4 — UI/UX & ekosistem (opsiyonel)
- [ ] **CG-15 — Render blokları**: Mermaid native · HTML/PDF/image/markdown preview · datatable/spreadsheet + `transform_data`. Artifact sistemine eklenebilir. *(craft v0.3.0, v0.4.2, v0.4.6, v0.9.6)*
- [ ] **CG-16 — Cross-session full-text arama** (ripgrep/Go). *(craft v0.3.1)*
- [ ] **CG-17 — Mini agents** (hafif prompt + hızlı model profili). *(craft v0.3.1)*
- [ ] **CG-18 — Session labels + auto-label + batch işlemler**. *(craft v0.2.27, v0.4.6)*
- [x] **CG-19 — Generic OpenAI-uyumlu custom endpoint** ✅ (commit `cf7d718` generic `OpenAICompat` tool-use + `35ec373` data-instance özel sağlayıcılar: kullanıcı OpenAI- veya Anthropic-uyumlu herhangi bir ucu — OpenRouter/Gemini/Kimi/Ollama — ekleyip ajan sağlayıcısı seçebiliyor; `<think>` ayıklama `7273a62`). Bedrock/DeepSeek özel-eklenti yolundan karşılanıyor. *(craft v0.7.4, v0.5.0)*
- [ ] **CG-20 — Doküman araçları** (`markitdown`/`pdf-tool`/`xlsx-tool`) attachment işleme için. *(craft v0.6.0)*
- [ ] **CG-21 — Messaging gateway** (Telegram/WhatsApp/Lark): response mode enum + subprocess izolasyon + **erişim kontrol** (güvenlik kritik). Not: Connectors fazı kapsam dışıydı. *(craft v0.8.10, v0.9.1)*

---

## swarmclaw incelemesinden — Provider ekosistemi genişletme (2026-06-18)

> Kaynak + tam analiz: [14-SWARMCLAW-PROVIDER-INCELEME.md](14-SWARMCLAW-PROVIDER-INCELEME.md).
> swarmclaw ~70 provider'ı "metadata'yı protokolden ayır" deseniyle düşük eforla ekliyor;
> SwarmGo zaten aynı mimaride (CG-19). Aşağıdakiler opsiyonel genişletmeler. **Plan — uygulanmadı.**

- [ ] **SC-1 — Built-in API provider preset kataloğu** (CLI değil): `OpenAICompat` handler'ı hazır; DeepSeek/Groq/Together/xAI/Fireworks/Nebius/DeepInfra/OpenRouter/Mistral/Google-compat'i **preset katalog** girişi (yalnız `{id, label, baseURL, defaultModel, models}`) olarak ekle → sıfır yeni protokol kodu, tek-tıkla ekle. **Düşük efor / yüksek değer.**
- [ ] **SC-2 — Generic CLI factory** (CLI ailesi): swarmclaw `streamGenericCliChat` deseni (binary spawn + stdout satır-stream, JSON parse yok) ile yapısal çıktısı olmayan onlarca coding-CLI'yi tek handler + veri listesiyle ekle. Yeni `kind_genericcli.go` + `[]genericCLI{id,label,binary}`. **CLI işi — CLI fazı açılınca, SC-1'den sonra.**

---

## the external agent-Agent incelemesinden — Kendini-geliştiren ajan özellikleri (2026-06-19)

> Kaynak: [nousresearch/external-context-agent](https://github.com/nousresearch/external-context-agent) ("seninle büyüyen ajan")
> ile SwarmGo karşılaştırması. the external agent mesajlaşma-merkezli, kendini-geliştiren bir kişisel asistan;
> SwarmGo web-UI merkezli, tek-binary self-hosted orkestrasyon. İki alanda the external agent açık ara önde ve
> SwarmGo'ya değer katacak. **Plan — uygulanmadı; ileride eklenebilecek featureler.**

- [ ] **HA-1 — Gelişmiş hafıza: tam-metin arama + LLM özet + kullanıcı modelleme** *(yüksek değer)*:
  the external agent hafızası üç katman taşıyor — (a) **FTS5 tam-metin arama** oturumlar üzerinde (SwarmGo'da
  mevcut **CG-16** ile örtüşür; Go tarafında ripgrep veya bleve/saf-Go ters-indeks ile, DB-siz
  felsefeye uygun), (b) **LLM-destekli özetleme** ile çapraz-oturum recall (SwarmGo'da `Reflect`
  dream-cycle + rolling summary kısmen var; oturumlar-arası kalıcı özet indeksine genişletilir),
  (c) **Honcho-benzeri kullanıcı modelleme** — etkileşimlerden kalıcı kullanıcı profili çıkarma
  (tercihler/bağlam/davranış). SwarmGo'nun mevcut lexical-cosine recall'ı (Faz 6) bunun altyapısı;
  üzerine kalıcı kullanıcı-profili entity'si + oto-güncelleme eklenir. İlişkili: **CG-16**, **C3** (memory_write).
- [ ] **HA-2 — Kendini-geliştiren prosedürel skill + skill hub** *(yüksek değer — ayırt edici)*:
  harici ajanin en özgün yanı: ajan zor bir görevi tamamladıktan sonra **kendi prosedürel skill'ini
  otonom yazar** ve tekrar kullanımla **iyileştirir** (procedural memory); skill'ler
  [agentskills.io](https://agentskills.io) merkezi hub'ında paylaşılır. SwarmGo'da skill sistemi
  (dosya-tabanlı, global/workspace tier, `create_skill`/`delete_skill`) + market (SwarmPack v1)
  **zaten var** — eksik olan **otonom skill üretimi** (görev sonrası ajanın deneyimden skill
  damıtması) ve **skill'in zamanla iyileşmesi** (kullanım geri-bildirimiyle revizyon). Mevcut
  self-management skill araçları + `Reflect` döngüsü bunun temelini oluşturuyor; üzerine
  "görev-sonrası skill-damıtma" hook'u + agentskills.io uyumlu içe/dışa aktarım eklenir.
  İlişkili: market (`_Docs/21-MARKET.md`), self-management (`_Docs/24-SELF-MANAGEMENT.md`).

> **Not:** İkisi de SwarmGo'nun mevcut alt sistemlerinin (Faz 6 hafıza, skill sistemi, market,
> `Reflect`) **üzerine** kurulabilir; sıfırdan değil. harici ajanin diğer güçlü yanları (mesajlaşma
> gateway → **CG-21**; çoklu çalıştırma backend'i Docker/SSH/Modal → kapsam dışı/D3) ayrı maddelerde.

---

## Faz R — Çok-ajan yarış & kurtarma guard'ları (oturum analizinden, 2026-06-19)

> Kaynak: bir dev-oturumunun (`260617-gentle-coyote`) analizi. Oturumda agent'ın
> **kendi muhakemesiyle** çözdüğü üç sürtünme (paralel-commit yarışı, port çakışması,
> elle temizlik) ve bir yanlış karar (varlık-kontrolü yapmadan "özellik yok" demek)
> SwarmGo'nun da yaşayacağı gerçek mimari boşluklara birebir oturuyor — çünkü ürün de
> birden çok ajanı **aynı workspace'te** shell/fs/git araçlarıyla eşzamanlı koşturuyor.
> Amaç: bu davranışları muhakemeden **mekanizmaya** taşımak. Önceliklendirme: 1. dalga
> RG-6 + RG-1 + RG-4; 2. dalga RG-2 + RG-3 + RG-5.

- [ ] **RG-1 — Entity versioning + CAS** *(1. dalga, S-M, yüksek değer)*: `db` entity'lerine `Version int`; her `mutate*Locked` bump'lar; yazım araç/API'sinde opsiyonel `If-Match` → bayat sürüm reddedilir. Belgelenmiş **"son yazan kazanır"** veri-kaybını kapatır. Temel: atomik `*.tmp`→`rename` + `store.go mutateSessionLocked` deseni (zaten var).
- [ ] **RG-2 — Workspace git-lock + provenance-scoped staging** *(2. dalga, M, yüksek değer)*: workspace başına tek-yazar git kilidi + ajan yalnız dokunduğu/`created_by` path'leri stage eder (oturumda elle Python ile yapılanın ürünleşmiş hali). Temel: `created_by` provenance (`_Docs/24-SELF-MANAGEMENT.md`) + `builtin_shell.go` git çağrıları.
- [ ] **RG-3 — Kaynak kira (lease) registry** *(2. dalga, M, orta değer)*: ajan port/temp-dir ister, runtime boş olanı verir, `stop`'ta otomatik bırakır. `:8090` çakışması bir daha olmaz; SwarmGo'nun kendi açılış preflight'ı için de kullanılır. Yer: `agent/runtime.go` yaşam döngüsü.
- [ ] **RG-4 — Tur yan-etki defteri → otomatik teardown** *(1. dalga, M, yüksek değer)*: tur başına spawn edilen PID / geçici workspace / temp dosya kaydı; tur biter veya çökerse otomatik teardown. Agent'ın elle yaptığı "test ws sil, sunucu durdur" işini garantiye alır. Temel: `agent/recovery.go` (A1) + `db/inflight.go` sidecar deseni.
- [ ] **RG-5 — Boot orphan reconcile genişletmesi** *(2. dalga, S-M, orta değer)*: açılışta takılı child süreç + sahipsiz temp workspace temizliği. Şu an yalnız tur (`inflight.go`) ve flow (`ResumeRunningFlows`, `flow.go`) resume ediliyor; aynı boot yoluna süreç/kaynak reconcile eklenir.
- [ ] **RG-6 — Implement-öncesi keşif guard'ı** *(1. dalga, XS, yüksek değer — KOD YOK)*: agent'a "uygulamadan önce ilgili dosyaları okuyup özelliğin zaten var olup olmadığını doğrula" adımını zorunlu kıl. Oturumdaki en büyük yanlış kararı (var olan özelliği "yok" sanıp yeniden yazma riski) önler. Yer: `swarmgo-guide` / `swarmgo-self-management` SKILL.md (system-prompt/skill düzeyi).

> **Ürün mü, skill mi?** RG-1/2/3/4/5 = SwarmGo **ürün kodu** (runtime ajanlarına verilen guard'lar); RG-6 = **skill/system-prompt**. İkisi farklı yere yazar.

---

## En Sona Ertelenenler (en düşük öncelik)

> Kullanıcı talebiyle (2026-06-17) bilinçli olarak en sona alındı. Diğer tüm backlog maddelerinden sonra ele alınacak.

- [x] **B2** — İzin modeli (`allow/ask/deny`, **arg-bazlı desen eşleme** `Bash(git *)`): `tools/permpattern.go` (`PermRule`/glob/`RepresentativeArg`/`DeriveGrantRule`) + `grants.go` kural deposu (`Matches`/`GrantRule`); "Her zaman izin ver" exec araçta komut ailesine daraltılır (`shell(git *)`); native + CLI yolu ortak; izin kartı gated komutu gösterir. `permpattern_test.go`+`permission_test.go` (2026-06-19)
- [x] **Faz P3** — Permission/onay modu (`auto`/`ask`/`read-only`) *(Aşama 1-4 ✅ YAPILDI; 1-3: 2026-06-17, 4: 2026-06-19)*: claude-cli mod bayrakları (`permission.go`/`claudecli.go`), native risk-sınıflı gate (`tools/classify.go` read/write/exec + `agent/permission.go permGate` + `AskPrompt` onayı), UI (ajan formu + composer 🛡 Shift+Tab + Settings varsayılanı). **Aşama 4:** claude-cli "ask" → Interaction MCP `permission_prompt` tool'u (`mcp_interaction.go callPermission`, `--permission-prompt-tool`) + native+CLI ortak `StepPermission` kartı (`PermissionPrompt.tsx`) + oturum-ömürlü "Always allow" (`tools/grants.go`, native ile aynı grant seti). `permission_test.go`.

> Not: `PermissionMode` gate'i, B2 arg-bazlı desen eşlemesi ve P3 Aşama 4 (CLI permission-prompt + StepPermission + oturum-ömürlü grant) **tamamlandı** (2026-06-19). Bu bölümde bekleyen izin işi kalmadı.

---

## Önceliklendirme Notu

İlk **görünür sonuç** Faz 3'te (çalışan chat UI). Faz 0-3 projenin "iskelet + nabız" aşamasıdır ve en kritik temeli atar; sonraki fazlar bunun üzerine eklenir.
