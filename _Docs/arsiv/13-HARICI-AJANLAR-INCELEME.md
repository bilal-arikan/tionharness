# external-agent-oss Release İncelemesi → TionSwarm Çıkarımları

> Kaynak: [external-agent-project/external-agent-oss](https://github.com/external-agent-project/external-agent-oss)
> İncelenen sürümler: **v0.2.19 → v0.10.3** (71 release, 2026-01-19 → 2026-06-09)
> Tarih: 2026-06-17 · Yöntem: tüm release gövdeleri çekildi, paralel LLM ile feature/fix çıkarımı, ardından TionSwarm kod tabanına karşı doğrulama.

Bu doküman iki bölümden oluşur:
1. **TionSwarm için önceliklendirilmiş eylem listesi** — kod tabanına karşı doğrulanmış (EXISTS/MISSING).
2. **Sürüm-sürüm tam fix & feature listesi** (referans/appendix).

---

## 1. TionSwarm İçin Önceliklendirilmiş Eylem Listesi

Her madde TionSwarm kaynak koduna karşı kontrol edildi. **Durum** = şu anki TionSwarm durumu.

### 🔴 P0 — Doğrulanmış gerçek boşluklar (yüksek etki)

| # | Konu | external-agent kaynağı | TionSwarm durumu | Öneri |
|---|------|----------------------|----------------|-------|
| 1 | **Fable 5 / adaptive-thinking** | v0.10.3 | ✅ **FIXED (2026-06-17)** — `providers.RequiresAdaptiveThinking(model)` + `agent.resolveThinkingBudget(model,level)`; Fable/Mythos 5'te "off"/"low" → min adaptive bütçe (`MinAdaptiveThinkingBudget=1024`), Opus/Sonnet/Haiku değişmez. `thinking_test.go`+`agentmsg_test.go`. | ~~Model registry'ye `requiresAdaptiveThinking` bayrağı ekle.~~ Tamamlandı. (Native tool-path thinking'i echo edemediğinden hâlâ kapalı — yalnız plain path resolver model-farkında.) |
| 2 | **Tool çıktısı boyut sınırı** | v0.4.4 (100K/200K cap), v0.9.3 | ✅ **FIXED (2026-06-17)** — `tools/registry.go` `capToolOutput()` (100K bayt, UTF-8 sınırında trunc + `…[truncated N bytes]`) `Registry.Call`/`CallStream`'de built-in + MCP tüm başarılı çıktılara uygulanır. `registry_cap_test.go`. | ~~Persistence/gönderim öncesi cap uygula.~~ Tamamlandı. |
| 3 | **http_get SSRF koruması** | v0.3.2, v0.5.0 | ✅ **FIXED (2026-06-17)** — `builtin_http.go` özel `net.Dialer.Control` guard'ı çözülen IP'yi her dial'da denetler (loopback/unspecified/link-local/private/ULA/CGNAT + 169.254.169.254 metadata reddedilir); redirect/DNS-rebind kapsanır + http/https şema kontrolü. `builtin_http_test.go`. | ~~SSRF koruması ekle.~~ Tamamlandı. |
| 4 | **MCP şema sağlamlığı** | v0.7.3, v0.7.5, v0.7.12 | ✅ **FIXED (2026-06-17)** — `mcp.NormalizeSchema()` `Registry.Defs`'te her MCP şemasına uygulanır: `$schema`/`$id`/`$ref`/`$defs`/`definitions` recursive strip, kök object garanti; `additionalProperties`/`required`/`oneOf`/`anyOf`/`allOf` korunur. `normalize_test.go`. | ~~Dış şemaları normalize et.~~ Tamamlandı. |
| 5 | **MCP tool metadata sızıntısı** | v0.7.7, v0.8.13 | ⚠️ Şu an TionSwarm iç metadata enjekte etmiyor (temiz) — ama Craft tool çağrı kuralı `_displayName`/`_intent` zorunlu kılıyor | Eğer ileride UI için tool şemasına meta alan eklenirse, provider'a göndermeden **boundary'de strip et**. Şimdilik not; tasarımı bozma. |

### 🟠 P1 — Çok-ajan mimarisine doğrudan uyan eksikler

| # | Konu | Kaynak | TionSwarm durumu | Öneri |
|---|------|--------|----------------|-------|
| 6 | **Ajanlar-arası mesajlaşma** | v0.8.8 `send_agent_message` | ✅ **FIXED (2026-06-17)** — `tools/builtin_agentmsg.go` `send_agent_message` (self-manage gate'li): hedef ajanın `agent-inbox` oturumuna user-mesaj append + `Runtime.Wake` (fire-and-forget; `call_agent` senkron delegasyonun async tamamlayıcısı). `agent.SendAgentMessage` çözer/teslim eder. `agentmsg_test.go`. | ~~Built-in tool ekle.~~ Tamamlandı. Native (anthropic/minimax) yolunda; claude-cli kendi döngüsünü sürer (SDK-parite). |
| 7 | **Oturum öz-yönetim araçları** | v0.8.3 | ⚠️ **Kısmî** — `list_sessions` built-in tool'u **var** (`tools/builtin_sessions.go`, cross-session farkındalık, commit `54ab736`); `set_session_*`/`get_session_info` yok | Kalan: `set_session_labels` / `set_session_status` / `get_session_info` built-in tool'ları → kendini-kapatan otomasyon akışları (görev bitince status=done → trigger). |
| 8 | **Hooks / koşullu otomasyon** | v0.4.3, v0.7.7, v0.7.5 | ⚠️ `events` bus + `scheduler` var; hook/condition/webhook yok | (a) Command + prompt hook'ları (olay → shell/prompt enjeksiyonu), rate limiter + zorla-sonlandırma. (b) Otomasyon koşulları (time/state/label gate). (c) Webhook action (exponential backoff retry). TionSwarm'nun schedule sistemiyle birebir örtüşür. |
| 9 | **Otomasyon geçmişi cap + compaction** | v0.7.8 | ⚠️ `flow_runs` var, sınır belirsiz | Çalıştırma geçmişini sınırla (örn. 20/otomasyon, 1000 global) + periyodik compaction → disk şişmesini önle. |

### 🟡 P2 — Sağlamlaştırma & doğrulama (file-based depolamaya özgü)

| # | Konu | Kaynak | TionSwarm durumu | Öneri |
|---|------|--------|----------------|-------|
| 10 | **Density-aware token tahmini** | v0.9.3 | ⚠️ Sabit `chars/4` (`tokens.go:8`) | base64/yoğun içerik için `chars/1.5` kullan; aksi halde tool result token sayımı ~%25 eksik → bağlam zehirlenmesi. Tool-result eşiğini context window'a göre dinamik yap (v0.9.1: floor 2K, ceil 15K, `ctx*0.10`). |
| 11 | **Ara mesajları JSONL'e yaz** | v0.8.8 | ✅ Büyük ölçüde var (`Message.Steps` iz tutuyor) — doğrula | Tool result / sistem event'lerinin reload sonrası boşluk bırakmadığını test et. |
| 12 | **Prompt cache: volatile veri** | v0.10.2, v0.2.31 | ✅ **İYİ** — `System` (statik, cache breakpoint) + `SystemDynamic` ayrımı doğru (`chat_turn.go`). Tarih/saat şu an enjekte edilmiyor. | Eğer tarih/session-state eklenirse **mutlaka `SystemDynamic`'e veya user-mesaj kuyruğuna** koy, statik prefix'e değil — yoksa her tur cache busts olur. Mevcut tasarım zaten doğru; bu kuralı koru. |
| 13 | **fs.watch yarış koşulu** | v0.8.3, v0.7.4 | ✅ Şu an `fsnotify` kullanılmıyor (atomik write-through) | Eğer canlı dosya izleme eklenirse: programmatic write'ları ezen watcher yarışına dikkat (ID-dedup + debounce, Windows'ta agresif tetikleme). Şimdilik risk yok. |
| 14 | **Config oto-onarım** | v0.2.33 | ❌ Yok | Başlangıçta bozuk config/`~/.claude.json` (boş/BOM/invalid JSON) tespit + onarım + backup; Windows file-lock için retry. claude-cli yolu için faydalı. |
| 15 | **Provider-aware mini-model** | v0.8.12 | ✅ **İYİ** — titler/summarizer ajanın modelini kullanıyor, hardcode Haiku yok (`titler.go:36`) | Korunması gereken doğru desen. |

### 🟢 P3 — Güvenlik sıkılaştırma (web UI / remote açılırsa)

- **URL şeması blocklist** (v0.8.12, v0.9.6): `Markdown.tsx:58` http/https/mailto/# dışını `<a href>`'e geçiriyor → `javascript:`/`file:`/`data:` engellenmeli (allowlist yerine blocklist + DOM href sanitization, middle/cmd-click kaçışını da kapat). **MISSING.**
- **Shell sandbox escape** (v0.5.0): `find -exec` ve benzeri escape vector'lerini engelle (TionSwarm shell varsayılan kapalı ama açıkken geçerli).
- **Web UI auth** (v0.8.2): headless/remote sunulursa argon2id + JWT (`jose`) + rate limiter.
- **Remote WebSocket TLS zorunlu** (v0.7.0): plaintext `ws://` reddet, `wss://` + self-signed kabul (intranet).
- **Path traversal** (v0.3.2): `sessionId`/`rel` ile attachment/upload yazımında — TionSwarm `sandbox.go` Resolve koruması **VAR**; upload/artifact yollarında da aynı sanitize uygulandığını doğrula.
- **Supply chain** (v0.8.0): CI'da `go mod verify` + `go.sum` doğrulaması.

### 🔵 P4 — UI/UX ve ekosistem (opsiyonel zenginleştirme)

- **Cross-session full-text arama** (v0.3.1): ripgrep/Go ile tüm oturumlarda arama — güçlü UX.
- **Render blokları**: Mermaid native (v0.3.0), HTML/PDF/image/markdown preview (v0.4.6, v0.9.6), datatable/spreadsheet + `transform_data` (v0.4.2). TionSwarm'nun artifact sistemine eklenebilir.
- **Doküman araçları** (v0.6.0): `markitdown`/`pdf-tool`/`xlsx-tool`/`docx-tool` — attachment işleme için.
- **In-app browser tool** (v0.6.0) + yüksek-riskli aksiyon onayı.
- **Session labels + auto-label** (v0.2.27), **batch işlemler** (v0.4.6), **workflow state badge** (v0.2.31).
- **Mini agents** (v0.3.1): hafif prompt + hızlı model profili — basit görevler için.
- **Session branching/fork** (v0.6.0).
- **Messaging gateway** (v0.8.10+): Telegram/WhatsApp/Lark — response mode enum (`progress`/`streaming`/`final_only`) + subprocess izolasyon + **erişim kontrol** (v0.9.1, güvenlik kritik).
- **i18n** (v0.8.5): erken kurulursa migration ucuz.
- **Model çeşitliliği**: OpenAI-uyumlu generic custom endpoint (v0.7.4) — TionSwarm'nun minimax provider'ı genelleştirilebilir; Gemini/Bedrock/DeepSeek/external CLI agent opsiyonel. **Not (2026-06-19):** `openrouter` kind (`internal/providers/kind_openrouter.go`) eklendi; OpenRouter üzerinden yüzlerce modele tek key ile erişim sağlanıyor. Bu, SC-1 önerisinin (preset katalog) ilk somut adımıdır.

---

## 2. Sürüm-Sürüm Tam Fix & Feature Listesi (Referans)

> Kısaltılmış; ✨=feature, 🐛=fix, ♻️=refactor, 🔒=security, ⚡=perf.

### v0.2.x serisi (temel + auth + UI)
- **v0.2.21** 🐛 Claude CLI bağımlılığı kaldırıldı → OAuth 2.0 PKCE.
- **v0.2.22** ✨ Windows code signing (RFC3161) · tıklanabilir context badge → compaction · JSON preview overlay. 🐛 macOS menü başlığı · RenameDialog · Windows icon · boş authScheme→Bearer fix · reauth/close. ♻️ Radix overlay · `usage_update` event.
- **v0.2.23** ✨ macOS Liquid Glass icon · mesaj kopyala · custom tool icon · `/docs/*` · source dropdown. 🐛 source guide oto-güncelleme · MCP URL · session sharing · mention Enter · credential şifreleme · attachment picker. ♻️ `semver.gt()` · plan doğrudan sunum.
- **v0.2.24** ✨ Contextual help. 🐛 dock badge focus · Windows scriptler · overlay scroll · share error.
- **v0.2.25** ✨ "External Agents" rebrand · filtre-farkındalıklı source UI · online source guide. ♻️ tabular numbers · server-filesystem bağımlılığı kaldırıldı.
- **v0.2.26** ✨ Custom model endpoint (OpenRouter/Azure) · **otomatik OAuth token refresh** (Google/Slack/MS, expiry-5dk). ⚡ sistem promptu %70 küçültüldü (doc'lar runtime markdown'a). ♻️ case-insensitive context dosya eşleşmesi. 🐛 OAuth debounce · tool-desteksiz model handling.
- **v0.2.27** ✨ Session labels (`#` etiketleme + auto-label + filtreli view) · ikon/renk sistemi. 🐛 Windows auto-update · API connection test.
- **v0.2.30** ✨ Diff viewer kontrolleri (split/unified, `preferences.json`) · Windows Git Bash tespiti · gelişmiş app menü. ♻️ cross-platform ESLint · **async queue session persistence (generation counter, stale-overwrite önleme)**. 🐛 task result cleanup · persistence race.
- **v0.2.31** ✨ Workflow state badge · inline diff viewer · TurnCard expand kalıcılığı (LRU 100). ♻️ tek-kaynak versiyon yönetimi (`package.json`).
- **v0.2.32** ✨ OAuth-only auth (CLI/Desktop token tespiti kaldırıldı) · token test scripti. 🐛 no-auth source auto-activate · session date grouping (`lastMessageAt` ayrı persist) · workspace icon crash null-handling.
- **v0.2.33** ✨ **Claude config auto-repair** (bozuk `~/.claude.json` onarımı, Windows file-lock retry). ♻️ 2 aşamalı CI release (build→promote).

### v0.3.x (render + arama + skill)
- **v0.3.0** ✨ Native Mermaid (SVG, beautiful-mermaid) · tam-ekran diagram viewer · çok-tip diagram · in-app PDF/image preview · smart file detection · JSON tree. ⚡ 100+ diagram <500ms. ♻️ stateless/ID-bazlı tool eşleştirme.
- **v0.3.1** ✨ **Cross-session full-text arama (ripgrep, Cmd+F)** · **Mini Agents** (hafif prompt, hızlı model) · draggable popover. 🐛 Microsoft OAuth explicit config.
- **v0.3.2** ✨ Focus mode · basic auth password (`passwordRequired`). 🔒 **path traversal fix (STORE_ATTACHMENT sessionId)** · gömülü Google OAuth credential kaldırıldı · OAuth discovery SSRF koruması. 🐛 RFC 8414 progressive OAuth discovery.
- **v0.3.3** ✨ MCP OAuth otomatik token refresh · multi-header auth. 🔒 SSRF sıkılaştırma. ⚡ paralel token check. ♻️ RFC 9728 protected resource metadata. 🐛 token refresh rate limiting.
- **v0.3.4** ✨ `~/.agents/skills/` konvansiyonu (cross-tool). 🐛 config > localStorage önceliği · provider error mesajları · custom model summarization (`resolveModelId`) · @mention alias.
- **v0.3.5** ✨ **Claude Opus 4.6** · otomatik image resize (8000px). 🐛 macOS auto-update progress · skill mention regex (`\s`→literal boşluk).

### v0.4.x (multi-LLM + hooks + dökümanlar)
- **v0.4.0** ✨ **Çoklu LLM bağlantısı** (bağımsız doğrula/yönet) · Codex/OpenAI OAuth · per-workspace default LLM/tema.
- **v0.4.1** ♻️ Connection-merkezli model yönetimi. 🐛 PowerShell validation/sandbox/build (Windows).
- **v0.4.2** ✨ an external CLI agent provider · **native datatable + spreadsheet blok** · **`transform_data` tool** · file-backed tablo (`src`) · Mermaid ELK layout · provider ikonları · multi-header auth. 🐛 session metadata race condition · Codex binary path.
- **v0.4.3** ✨ **Hooks sistemi** (event bus + command hook + prompt hook) · cron scheduler · **event rate limiter + SIGKILL fallback** · session metadata→env var · **`call_llm` tool** (paralel, attachment, structured output, extended thinking). 🐛 Cmd+Enter double-send · `update_user_preferences` append.
- **v0.4.4** 🐛 **OAuth token refresh deadlock** · `expiresAt` eksikse 3600s default · MCP `http_headers`→`headers` token drop · **unbounded tool output OOM → 100K/200K char cap** · provider model fetch 30s timeout.
- **v0.4.5** 🐛 Non-refreshable token (Slack) · refresh cooldown clear · external CLI agent Windows.
- **v0.4.6** ✨ **HTML & PDF inline preview** · batch session işlemleri · dynamic Codex model discovery (30dk cache + fallback) · **Mustache template engine** + source templates (`render_template`). 🐛 skill `requiredSources` frontmatter · per-session env override race (global→session). ♻️ **unified event adapter (BaseEventAdapter + EventQueue, sıralı teslimat)** · TodoState→SessionStatus.
- **v0.4.7** 🐛 Multi-tier skill resolution (global/workspace/project) · skill live reload.
- **v0.4.8** ✨ `call_llm` tüm backend'lerde. 🐛 plugin adı manifest'ten · Codex event queue race (turn/completed öncesi handler bitişi).

### v0.5.x (provider patlaması + otomasyon UI)
- **v0.5.0** ✨ Codex·ChatGPT Plus · external CLI agent device-code · **Google AI Studio/Gemini (Search grounding)** · OpenRouter/Ollama/custom (OpenAI-uyumlu) · searchable model picker (Best/Balanced/Fast) · **MCP source'ları non-Anthropic backend'lere proxy**. 🔒 web-fetch SSRF + unbounded read · Explore mode `find -exec` engeli. ⚠️ Breaking: Pi SDK konsolidasyonu.
- **v0.5.1** ✨ **Automations UI** (enable/disable, history timeline, manuel test) · prompt action'a `llmConnection`+`model` · `automations.json` v2 (legacy `hooks.json` kaldırıldı). 🐛 file mention→absolute path. ⚠️ Breaking: `hooks.json` migration yok.

### v0.6.0 (browser + döküman araçları)
- ✨ **In-app browser** (navigate/click/fill/screenshot/eval) · **privileged execution approval** · `markitdown`/`pdf-tool`/`xlsx-tool`/`docx-tool`/`pptx-tool`/`img-tool`/`doc-diff`/`ical-tool` · **session branching** · multi-panel · Sonnet 4.6. ♻️ Explore mode güvenlik baseline.

### v0.7.x (headless + protokol + thinking + Bedrock)
- **v0.7.0** ✨ **`craft-cli run` (terminal client)** · **headless Bun server** (TLS, env config) · **WebSocket-only RPC** (Electron IPC yerine) · server-core paketi · protokol formalizasyonu (typed DTO + wire-format test) · model fallback chain. 🔒 unencrypted WS engeli (`wss://` zorunlu). 🐛 config.json existence guard.
- **v0.7.1** ✨ Pi engine 0.56.2 (GPT-5.4). 🐛 branch race (15s timeout) · base64 false-positive (roundtrip verify) · Codex token expiry (global refresh mutex).
- **v0.7.2** ✨ **Minimax preset** · Kimi · OpenAI bölgesel preset · app-level default thinking. ♻️ deferred SDK check (boot→session). 🐛 paylaşılan session delete sonrası erişim iptali · @mention subsequence search.
- **v0.7.3** ✨ Background task UI · dil-farkındalıklı başlık. ♻️ **MCP şema oneOf/anyOf/allOf + nested object** · @file mention semantic marker. 🐛 MCP transport race (ardışık query) · "stuck running" activity · background task memory leak.
- **v0.7.4** ✨ Custom OpenAI-uyumlu endpoint (`registerProvider`). 🐛 session branching overhaul (ilk user mesajında fork) · Windows fs.watch duplicate (ID-dedup + 300ms debounce) · portable `{{SESSION_PATH}}` token · boş blok `cache_control` strip.
- **v0.7.5** ✨ **Network proxy** (HTTP/HTTPS + bypass) · **webhook action** (auth, replay, exponential backoff). 🐛 Zod/JSON schema `.passthrough()` + `additionalProperties` koru · self-signed TLS · event payload'a `labels`.
- **v0.7.6** ✨ MCP custom header (`headerNames`, credential store). 🐛 `customEndpoint` persist edilmiyordu · OAuth browser-auth refresh loop.
- **v0.7.7** ✨ **5-seviye thinking** (Off/Low/Med/High/Max, adaptive) · **otomasyon koşulları** (time/state/logic gate) · paralel `call_llm` · server-side directory browsing. 🐛 `_intent`/`_displayName` SDK'ya sızıntısı strip · proxy tüm subprocess'lere · WS client race.
- **v0.7.8** ✨ **Amazon Bedrock** (IAM) · **1M context** (Opus/Sonnet 4.6) · CLI `--base-url`. 🐛 **ara mesajları JSONL'e persist** (tool result/sistem event) · otomasyon history cap (20/1000).
- **v0.7.9** ✨ **Güvenilir WS event teslimatı (sequence-number + buffer + reconnect replay)**. 🐛 external CLI agent model 3-tier fallback · 1M context `[1m]` suffix.
- **v0.7.10** 🐛 Claude OAuth 429 (spoofed UA→doğru client ID).
- **v0.7.11** ✨ per-workspace 1M context toggle. ♻️ custom endpoint `contextWindow` config.
- **v0.7.12** ✨ Bedrock (full, model ID normalize `us.anthropic.*`) · **extended prompt cache (1 saat TTL)** · **Docker headless server**. 🐛 branch fork fallback (mini-model özet) · **MCP `$schema` strip** · **MCP URL'e `/mcp` ekleme bug'ı** · interceptor cache/beta header strip.

### v0.8.x (hibrit transport + i18n + messaging)
- **v0.8.0** ✨ **Hibrit local/remote transport** (resume-first session transfer) · çoklu remote workspace · **browser-erişimli WebUI** (headless aynı portta) · session export/import · **mobil WebUI**. 🔒 supply chain (`trustedDependencies` + `--frozen-lockfile`). 🐛 abort'ta server crash (unhandled rejection).
- **v0.8.1** ✨ remote workspace recovery · Docker deploy (GHCR). 🐛 büyük attachment chunked encoding · subprocess pipe hata flood.
- **v0.8.2** ✨ browser_tool toggle · unified WebUI source OAuth (server-side relay) · **per-session FS caching**. 🔒 **WebUI auth: `jose` JWT + `argon2id` + rate limiter**. 🐛 search reliability (ripgrep fallback).
- **v0.8.3** ✨ **Session self-management tools** (`set_session_labels`/`set_session_status`/`get_session_info`/`list_sessions`) · compact mode. 🐛 **`fs.watch` programmatic write'ı ezme race** · headless'ta automation eager init · `includeCoAuthoredBy:false` saygı.
- **v0.8.4** ✨ Generic OAuth (RFC 9728 auto-discovery) · Send to Workspace. 🐛 session tool Pi yolunda · model tier provider-aware.
- **v0.8.5** ✨ **i18n** (EN/ES/zh/JA, 1050+ string) · canonical locale registry. ⚠️ Breaking: free-text `language` kaldırıldı.
- **v0.8.6** ✨ **chunked session transfer** (base64 + SHA-256 + per-chunk retry) · custom endpoint image input (`supportsImages`). 🐛 sleep/wake sonrası mesaj kaybı (stale WS).
- **v0.8.7** ✨ HU/DE/PL çeviri · API token refresh endpoint (`renewEndpoint`) · raw body (`_rawBody`/`_contentType`). 🐛 Bedrock region-aware · server lock release.
- **v0.8.8** ✨ **Inter-session messaging (`send_agent_message`)**. 🐛 local model setup (placeholder key) · headless duplicate ConfigWatcher hang.
- **v0.8.9** ✨ **Claude Opus 4.7 default** (4.6→4.7 oto-migration). · Claude Agent SDK 0.2.111 (per-tool `permission_policy`, `WarmQuery`, `memory_recall` event).
- **v0.8.10** ✨ **Messaging Gateway: Telegram & WhatsApp** (response mode `progress`/`streaming`/`final_only`, subprocess worker) · `xhigh` thinking · per-automation topic. 🐛 **1M context opt-in (default off, Tier 1-3 400 fix)** · `spawn_session` `~` expand · `set_session_labels` valued label (`id::value`).
- **v0.8.11** 🐛 `call_llm` partial output recovery (maxTurns 10) · follow-up quote truncation kaldırıldı. ♻️ `queryLlm` test edilebilir çıkarım.
- **v0.8.12** ✨ GPT-5.5 default · **DeepSeek** (`deepseek-v4-pro/flash`). 🐛 **source activation mid-turn (turn boundary + resend)** · attachment session leak (hybrid path vs inline bytes) · URL şema **blocklist** (`javascript:`/`data:`/`file:`) · `/compact` GPT timeout · **`call_llm` requested model honor** · **provider-aware mini model** (`pickProviderAppropriateMiniModel`) · MCP tool registration shape contract test. ⚠️ Claude Agent SDK 0.2.111'e pin (0.2.113 native binary breaking).

### v0.8.13 → v0.10.x (provider olgunlaşma + mobil + Fable 5)
- **v0.8.13** ✨ otomasyon thinking-level override. 🐛 **DeepSeek/OpenAI-uyumlu tool-call history corruption** (id dedup, two-phase stream) · OpenAI-compat 400 diagnostics zenginleştirme.
- **v0.9.0** ✨ Claude Agent SDK native binary (per-platform) · **Lark/Feishu adapter** (WS long-connection) · Telegram supergroup topic routing · Group-by-Unread. 🐛 loopback custom endpoint API key strip · Mermaid validation renderer hizalama.
- **v0.9.1** ✨ **Telegram whitelist + access control** (workspace + per-binding) · per-connection mid-stream (Steer vs Queue) · per-model image toggle · **tool-result eşiği context window'a göre ölçekleniyor** (`tokenLimitFor`). 🐛 runtime config canlı subprocess'e yansıma (mutex/gate) · context overflow 5-state recovery machine.
- **v0.9.2** 🐛 OAuth refresh agent-build'den önce · Pi sistem prompt her turn siliniyor (workaround) · streaming response boş completion fallback zinciri · stale cross-machine branch cwd guard.
- **v0.9.3** ✨ **Mobile-first compact mode** (container query, vaul drawer) · Manifest provider preset. 🐛 **oversized tool result session poisoning → density-aware estimator (`chars/1.5`, 12K eşik)** · Telegram polling auto-reconnect (backoff) · channel routing exhaustiveness.
- **v0.9.4** ✨ opt-in **RTK bash token compression**. 🐛 OpenAI/Codex uzun session instability (Pi SDK 0.73.1, WS→SSE fallback, keepalive).
- **v0.9.5** ✨ compact drawer'lar (working-dir, AcceptPlan, model selector). 🐛 latest turn branching son mesaj kaybı · stdio MCP fake timeout (stderr watchdog) · paralel `source_test` orphaned `tool_use` ID · tool-call'la biten turn "Thinking…" takılması · messaging gateway final mesaj teslimatı.
- **v0.9.6** ✨ multi-window başlıkta workspace adı · **`markdown-preview` blok**. 🐛 API credential mid-session refresh (her çağrıda vault'tan oku, statik bağlama değil) · authType→`none` stale credential temizliği · **`cache_control` 1h TTL sırası (tools→system→messages)** · over-broad "tool not supported" sınıflandırma. 🔒 URL şema blocklist + DOM href sanitization.
- **v0.10.0** ✨ **remote `browser_tool` WS bridging** (capability-advertise + per-method authz) · browser tab'ları workspace-izole. 🐛 IPC sınırında native referans → `toSnapshot()` plain DTO · `source_test` basic-auth base64 encode (validator=runtime).
- **v0.10.1** 🐛 session başlık dili (main-process i18n hydration). ⚡ **Claude Opus 4.8 default** (4.6 kaldırıldı, migration). ⚠️ Breaking: macOS Intel build durduruldu · `language`→`uiLanguage`.
- **v0.10.2** ✨ label `link` value type · OAuth bağlantı başına Anthropic hesap/org görünürlüğü (duplicate quota uyarısı) · Stop'ta son mesaj input'a geri. 🐛 **`updateLlmConnection` allowlist veri kaybı** · **Pi prompt-cache her tur bust** (volatile context cache prefix'ten ayrıldı → user mesaj kuyruğuna).
- **v0.10.3** ✨ **Claude Fable 5** (1M context, 7 dil, **adaptive thinking always-on — `disabled` reddediliyor**). ♻️ **thinking resolver model-sınıf mapping** (Fable/Mythos 5 → off/minimize'i low-effort adaptive'e map et; Opus/Sonnet/Haiku değişmez).

---

## Sonuç

external-agent-oss'un yolculuğu TionSwarm için bir **yol haritası önizlemesi**: tek-provider → multi-provider olgunlaşma, Electron IPC → WebSocket RPC + headless server, ve giderek artan **otomasyon/messaging/orkestrasyon** katmanları. TionSwarm'nun mevcut mimarisi (file-based store, iki-parçalı sistem prompt, provider soyutlaması, events bus, scheduler) bu yörüngeyle **uyumlu**; en yüksek getirili adımlar P0–P1 tablolarındaki doğrulanmış boşluklar.
