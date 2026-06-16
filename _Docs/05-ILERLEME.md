# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-06-16**

## Kararlar (2026-06-15)
- **İlk LLM sağlayıcısı:** Anthropic (Claude) ✅
- **Frontend:** React ✅

## Mevcut Durum: FAZ 7 ORCHESTRATION TAMAMLANDI ✅ (CANLI TEST GEÇTİ) → Faz 9 (Wails)

> Not: Faz 8 (MCP/Tools) kullanıcı talebiyle Faz 7'den önce yapıldı; ardından Faz 7 tamamlandı.
> Kalan sıra: **Faz 9 Wails paketleme.** (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.)

### Model kataloğu + provider/model seçici + MiniMax sağlayıcı ✅ (2026-06-15)

Ajan için provider+model artık **listeden** seçilebiliyor (serbest girişe de izin var) ve yeni bir **MiniMax** sağlayıcı (OpenAI-uyumlu) eklendi.

- [x] **Katalog** (`internal/providers/catalog.go`): provider+model listesi (claude-cli aliasları, Anthropic Claude modelleri, MiniMax M2.1/M2.1-lightning/M2); her provider `allowCustomModel`. `GET /api/catalog` canlı `available` bayrağıyla (claude-cli PATH'te mi, anthropic/minimax anahtarı var mı).
- [x] **MiniMax provider** (`internal/providers/minimax.go`): OpenAI-uyumlu `POST {base}/chat/completions` (Bearer), `base_resp`/`error` zarfı işlenir; varsayılan `https://api.minimax.io/v1`. `registry` → `SetMinimax(key, baseURL)` + Get("minimax"); `settings` → şifreli `MinimaxKeyEnc` + `MinimaxBaseURL`, DTO `minimaxKeySet`/`minimaxBaseUrl`, `applySettings` push.
- [x] **Frontend** `ProviderModelSelect.tsx` (yeniden kullanılabilir, katalogu modül-düzeyinde cache'ler): provider dropdown + model dropdown + "Özel…" serbest giriş. AgentSettingsModal, Sidebar **ajan oluşturma** (artık model de seçiliyor → `onCreateAgent(name,soul,provider,model)`), SettingsPanel varsayılan provider/model'e entegre. SettingsPanel "Sağlayıcılar"a **MiniMax API anahtarı + base URL** alanları.

**CANLI TEST (API + Chrome):**
- [x] `GET /api/catalog`: 3 provider modelleriyle döndü; anahtarsız anthropic/minimax `available=false`.
- [x] MiniMax key PUT → `minimaxKeySet=true`, baseUrl persist; katalogda minimax `available=true` oldu (sonra sıfırlandı).
- [x] Chrome: ajan oluşturma formunda Sağlayıcı dropdown'ı **Claude CLI · Anthropic (anahtar gerek) · MiniMax (anahtar gerek)** seçeneklerini gösterdi (HTML doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: MiniMax (OpenAI-uyumlu istemci) tool-use'suz düz completion yapar; aynı istemci başka OpenAI-uyumlu uçlar için de base URL ile kullanılabilir. Model ID'leri sık değiştiğinden her provider serbest model girişine izin verir.

### Ajan görseli (yuvarlak avatar) + ajan ayar modalı ✅ (2026-06-15)

Ajan listesindeki her ajana **yuvarlak görsel** (avatar) eklendi ve satır kenarındaki **⚙ ayar butonundan** ajan değerleri düzenlenebilir hale geldi.

- [x] **Backend:**
  - `db.Agent`'a `avatar` (emoji/glif) + `color` (hex accent) alanları.
  - `db.UpdateAgent(id, AgentProfilePatch)` — pointer alanlı kısmi güncelleme (nil = dokunma), atomik diske yazar.
  - `PUT /api/agents/{id}` (`handleUpdateAgent`) — name/soul/identity/provider/model/planningMode/avatar/color kısmi patch. `handleCreateAgent` de opsiyonel avatar/color kabul eder.
- [x] **Frontend:**
  - `lib/avatar.ts`: id'den **deterministik renk** türetme (djb2 hash → palet), baş harf çıkarımı, glif/renk paletleri (`AVATAR_COLORS`, `AVATAR_GLYPHS`).
  - `components/AgentAvatar.tsx`: yuvarlak gradient disk — özel emoji ya da baş harf; aktif ajanda halka.
  - `components/AgentSettingsModal.tsx`: canlı avatar önizlemeli editör (ad, emoji seçici, renk paleti, sağlayıcı/model, planlama modu, soul, identity).
  - `Sidebar.tsx`: ajan satırı avatar + hover'da görünen ⚙ butonu (oturum ⟳ deseniyle aynı); modal render.
  - `types.ts` `Agent.avatar/color` + `AgentPatch`; `api.ts` `updateAgent`; `App.tsx` `updateAgent` callback (liste canlı güncellenir).

**CANLI TEST (API + Chrome):**
- [x] `PUT /api/agents/{id}` avatar/color/soul patch'ledi; dosyada `avatar` 🤖 (U+1F916), `color` `#7c3aed` doğru UTF-8 kalıcılaştı.
- [x] Chrome: roster yuvarlak avatarlar (RE/ST deterministik renk); ⚙ → modal açıldı; emoji+renk seçip Kaydet → roster anında 🤖 mor daireye döndü (DOM doğrulandı).
- [x] `go build ./...` + `tsc --noEmit` + `vite build` temiz.

### Workspace geniş düzenleme + paralel çalışma ✅ (2026-06-15)

Workspace düzenleme seçenekleri genişletildi ve paralel çalışma netleştirildi.

- [x] **Görsel kimlik**: `WSSettings`'e `Icon` (emoji) + `Color` (hex). `/api/workspaces` listesi artık icon/color ile zengin; WorkspaceSwitcher + NavRail daraltılmış rozet ikon/renk gösterir. Emoji uçtan uca round-trip doğrulandı (disk codepoint `D83D DE80`).
- [x] **İstatistikler**: `workspace-settings` DTO'ya `createdAt` + `agentCount`/`sessionCount`/`taskCount` (canlı sayım). Ayar ekranında rozet kartları.
- [x] **Silme**: "Bu Workspace" kategorisinde tehlikeli-bölge sil butonu (`onDeleteWorkspace`, App'te confirm + başka workspace'e geçiş; backend son workspace'i silmeyi reddeder).
- [x] **Paralel çalışma (zaten var, doğrulandı)**: `workspace.Manager.open()` boot'ta **her** workspace için `Runtime.StartConfigured` (heartbeat ajanları) + `Scheduler.Start` (cron) + `ResumeRunningFlows` çağırır → tüm workspace'lerin otonom ajanları/zamanlamaları **aynı süreçte eşzamanlı** koşar (UI yalnız aktif olanı gösterir). Per-workspace pause (`Runtime.SetPaused`) ile biri diğerlerini etkilemeden durdurulabilir.

**CANLI TEST (API + Chrome):**
- [x] workspace-settings GET: icon/color + stats (2 ajan/3 oturum/0 görev/createdAt) döndü.
- [x] PUT icon=🚀 color=#22c55e → kaydedildi; disk codepoint D83D DE80 (emoji bozulmadan); `/api/workspaces` zenginleşti.
- [x] Chrome: "Bu Workspace" kategorisi — istatistik kartları, ikon/renk alanları, 🚀 önizleme, sil bölümü render (DOM doğrulandı). Sonra varsayılana sıfırlandı.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Loglar ekranı (uygulama + tüm workspace) ✅ (2026-06-15)

En sol NavRail'e **📜 Loglar** görünümü eklendi; tüm uygulama ve workspace logları tek ekranda.

- [x] `internal/logbuf` (yeni): `Buffer` (sabit kapasiteli ring, 2000) + `slog.Handler` (kayıtları buffer'a yakalar **ve** stdout text handler'a delege eder). `main.go` logger'ı bu handler ile kurar → tüm workspace runtime'ları aynı logger'dan geçtiği için **çapraz-workspace** tüm akış tek buffer'da.
- [x] `api/logs.go` (yeni): `GET /api/logs?limit=&level=&q=` — seviye (min) + substring filtre; uygulama-geneli (workspace-scoped değil). `Server`'a `*logbuf.Buffer` alanı + `NewServer` imzası.
- [x] Frontend: `types.ts`/`api.ts` `LogEntry` + `getLogs`; `NavRail` "📜 Loglar"; `LogsPanel.tsx` (canlı poll 2.5sn "Canlı" toggle, seviye filtre chip'leri, arama, renkli seviye rozeti, zaman+mesaj+alanlar, oto-scroll); `App.tsx` render.

**CANLI TEST (API + Chrome):**
- [x] `/api/logs` boot loglarını döndürdü (config/providers/scheduler/workspace opened + attrs).
- [x] Filtre: `level=error`→0, `q=workspace`→2; yeni workspace oluştur → log anında yakalandı (`name=LogTest WS`).
- [x] Chrome: 📜 Loglar render — zaman/seviye/mesaj/attrs satırları, 2 workspace'in açılış logları görünür (DOM doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Depolama: SQLite → Dosya Sistemi ✅ (2026-06-15)

Kalıcılık katmanı **tamamen dosya-tabanlıya** çevrildi (External Agents OSS tarzı).
SQLite (`modernc.org/sqlite`), migration runner ve `migrations/*.sql` kaldırıldı.
Detay: **`_Docs/08-DEPOLAMA.md`**.

- [x] `internal/db` **yerinde** dosya-store'a çevrildi: bellek-içi maps + diske
  atomik write-through (`*.tmp`→`rename`), tek `sync.RWMutex`. **Tüm metod imzaları,
  model struct'ları, sabitler, `ErrNotFound` korundu** → onu kullanan 22 dosya
  (`agent`/`api`/`conversation`/`memory`/`workspace`) **değişmedi**.
- [x] Disk yapısı: `{wsID}/store/` altında entity-başına JSON + `sessions/{id}/session.jsonl`
  (satır 1 header, satır 2+ mesajlar). Embedding base64 gömülü; usage gün-bazlı dosya.
- [x] `swarmgo.db` yolu → `store/` dizini (`workspace/manager.go`); ölü `Config.DBPath()` silindi.
- [x] `go.mod` temizlendi: yalnız `google/uuid` + `robfig/cron/v3` kaldı (sqlite + ~8 dolaylı dep gitti).
- [x] `internal/db/filestore_test.go` round-trip testi (create→reopen→reload + UTF-8/HTML/embedding/cascade) **PASS**.

**CANLI TEST (API + restart):**
- [x] Backend temiz veri diziniyle ayağa kalktı; `store/` boş oluştu.
- [x] API ile ajan/oturum/görev/MCP oluşturuldu → doğru disk dosyaları yazıldı.
- [x] **Restart** sonrası 2 ajan + 2 oturum + 1 görev + 1 MCP **diskten reload** edildi; Türkçe başlık (`İlk sohbet — ğüşıöç`) round-trip korundu.
- [x] `go build/vet ./...` temiz.

> Not: Otomatik SQLite→dosya migration'ı yok (kullanıcı talebiyle SQLite tamamen kaldırıldı); yeni kurulum `store/`'da sıfırdan başlar.

### Ayarlar ekranı — genişletme: 2 panel + workspace + profil + beta'lar ✅ (2026-06-15)

Ayarlar ekranı **sohbet gibi 2 panele** dönüştürüldü (sol kategori rayı, sağ içerik) ve kapsam genişletildi.

- [x] **2 panel UI** (`SettingsPanel.tsx` yeniden): sol rayda **Uygulama** grubu (Profil · Görünüm · Bildirimler & Ekran · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında) + **Bu Workspace** grubu; kaydedilmemiş kategoride nokta göstergesi; kategoriye-özel Kaydet (app/workspace ayrı scope).
- [x] **Workspace'e özel ayarlar** (`internal/workspace/settings.go`): her workspace'in `ws-settings.json`'u — ad (rename), açıklama, **bu workspace'e özel** varsayılan sağlayıcı/model, **bu workspace'te otonomiyi duraklat**. `Manager.Rename`/`UpdateSettings`; `agent.Runtime` per-workspace `SetPaused/Paused` (guardedComplete'te global pause ile birlikte kontrol); `GET/PUT /api/workspace-settings` (X-Workspace-Id). Ajan oluşturmada öncelik: istek → workspace → uygulama → varsayılan.
- [x] **Kullanıcı profili** (resimdeki gibi): Ad · Saat dilimi · Şehir · Ülke · Notlar — `settings` alanları; chat sistem promptuna `## About the user` bloğu olarak enjekte edilir (`userContextBlock`).
- [x] **Masaüstü bildirimleri** + **Ekranı açık tut**: istemci-tarafı (`lib/clientPrefs.ts` — Notification API + Wake Lock, visibility-change'te yeniden alır); pencere arkadayken yanıt gelince bildirim.
- [x] **Anthropic beta'ları** (yalnız anthropic sağlayıcı): **1M token bağlam** (`context-1m-2025-08-07`) + **uzatılmış prompt cache 1 saat** (`extended-cache-ttl-2025-04-11` + system'e `cache_control` ttl=1h). `anthropic.go` `WithBetas`/`betaHeader`/`systemField`; `registry.SetAnthropicBetas`; `applySettings` push.

**CANLI TEST (API + Chrome):**
- [x] settings yeni alanlar GET/PUT: profil (Bilal/Türkiye/İstanbul), 1M+cache+notif+awake → kaydedildi.
- [x] workspace-settings: rename "Ana Workspace" + açıklama + pause=true persist; `/api/workspaces` rename'i yansıttı; sonra varsayılana sıfırlandı.
- [x] Chrome: ⚙ Ayarlar → **Profil** kategorisi (Ad/Saat dilimi/Şehir/Ülke/Notlar) ve **Bu Workspace > Genel** kategorisi (ad/açıklama/sağlayıcı-model override) DOM ile doğrulandı; 0 konsol hatası.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: 1M context / uzatılmış cache yalnız **anthropic** sağlayıcıda etkilidir (claude-cli yok sayar). Db katmanı bu sırada paralel olarak SQLite'tan dosya-store'a taşındı; eklemeler birleşik derlemede temiz.

---

### Ayarlar ekranı (uygulama geneli) ✅ (2026-06-15)

Uygulama genelinde tek bir ayar belgesi (`settings.json`, data dizininde) + Ayarlar UI eklendi.
Hassas Anthropic API anahtarı AES-GCM ile şifreli saklanır, istemciye asla düz dönmez
(`anthropicKeySet` boolean). Ayarlar canlı olarak alt sistemlere uygulanır.

**Backend**
- [x] `internal/settings/` (yeni): `settings.go` (Settings + DTO maskeli + Patch), `store.go` (thread-safe JSON deposu, atomik yazım, normalize/clamp, `Cipher` arayüzü ile şifreli secret).
- [x] `providers/registry.go`: mutable + `SetAnthropicKey/SetClaudeCLIPath/SetDefaultModel`, RWMutex; `Get` canlı değerleri okur.
- [x] `conversation/manager.go`: `SetLimits` (canlı maxTokens/keepRecent), mutex.
- [x] `agent/tunables.go` (yeni): süreç-geneli `SetAutonomyPaused`/`SetTitleModel`; `budget.go` otonom çağrıda `ErrAutonomyPaused`; `titler.go` başlık-modeli override.
- [x] `api/settings.go` (yeni): `GET/PUT /api/settings`, `POST /api/settings/test-provider` (gerçek minimal completion ile bağlantı testi).
- [x] `api/server.go`: `settings` alanı + `NewServer` imzası + `applySettings()` (boot + her güncellemede push).
- [x] `api/agents.go`: yeni ajanlarda varsayılan provider/model + günlük bütçe ayarlardan.
- [x] `api/chat.go`: auto-title `autoTitleEnabled` ayarına bağlı.
- [x] `cmd/swarmgo/main.go`: settings store + env `ANTHROPIC_API_KEY`'i tek seferlik şifreli store'a migrate.

**Frontend**
- [x] `types.ts`/`api.ts`: `AppSettings`/`SettingsPatch`/`ProviderTestResult` + `getSettings/updateSettings/testProvider`.
- [x] `components/SettingsPanel.tsx` (yeni): 9 bölüm (Görünüm · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında), dirty-takip + tek Kaydet, API key maskeli/sil, provider test rozeti, toggle/sayı/select alanları.
- [x] `components/NavRail.tsx`: en altta sabit ⚙ Ayarlar görünümü.
- [x] `lib/theme.ts` (yeni) + `index.css`: açık tema (`[data-theme=light]`) + accent CSS değişkeni; `App.tsx` boot'ta + kayıtta `applyTheme`.

**CANLI TEST (API + Chrome):**
- [x] GET/PUT settings round-trip; `theme/maxContextTokens/pauseAutonomy` güncellendi.
- [x] Anthropic key: set → `anthropicKeySet=true` (değer dönmüyor), temizle → false.
- [x] test-provider claude-cli → `ok=true model=claude-opus-4-8 sample="OK"`.
- [x] Ajan varsayılanları: `defaultModel=sonnet` + `callLimit=50` ayarıyla yeni ajan o değerlerle oluştu.
- [x] Auto-title kapısı: `autoTitleEnabled=false` iken ilk mesajda başlık üretilmedi.
- [x] Chrome: uygulama 0 konsol hatasıyla yüklendi; ⚙ Ayarlar tıklandı, 9 bölüm render oldu (DOM doğrulandı). Görsel ekran görüntüsü 87 sekmeli ortamda `image readback` ile alınamadı (guide'da bilinen sorun).

### Sohbet: adım-adım akış (SSE streaming) ✅ (2026-06-15)

Sohbet artık tüm tur bitince değil, **her adım bittikçe** UI'a akıtılıyor.
- `providers.Request.OnEvent func(TraceStep)` + `claudecli.go` stream-json'u **satır
  satır** (`bufio` + `cliStreamParser`) okuyup olayları anında yayınlar (thinking
  hemen, ara metin flush'ta, tool adımı sonucu gelince). `agent/toolloop.go`
  `CompleteWithToolsStream(... onStep)` köprüler; native döngü de her adımı yayınlar.
- `internal/api/chat_stream.go` — `POST /api/chat/stream` (SSE): `meta`→`step`*→`done`.
  Tur sonunda mesaj + tam iz kalıcılaştırılır.
- Frontend: `api.streamChat` (`fetch`+`ReadableStream` SSE ayrıştırma), `App.sendMessage`
  canlı asistan balonu büyütür, `MessageList` boş balonda "çalışıyor" noktaları.
- **CANLI TEST:** `curl -N /api/chat/stream` zaman damgaları artımlı geldi
  (`meta` 08.9s → text 14.7s → Bash step-one 37.4s → Bash step-two 47.8s → `done` 56.7s);
  Chrome'da canlı asistan balonu akış sırasında "çalışıyor" göstergesiyle yakalandı.
  `go build ./...` + `tsc --noEmit` temiz.

### Zengin Sohbet Arayüzü (Chat UX) ✅ (2026-06-15)

Sohbet ekranı [external-agent-oss](https://github.com/external-agent-project/external-agent-oss)
referans alınarak External Agent benzeri zengin render katmanına kavuşturuldu: markdown
çıktı, tool kullanım kartları, düşünme adımları, dosya satır değişimi (diff),
tıklanabilir dosya yolları ve inline görsel. Asistan turu artık **aktivite izini**
(thinking + ara metin + tool çağrıları) ve son cevabı zengin biçimde gösterir.
Detaylı doküman: [07-CHAT-UX.md](07-CHAT-UX.md).

**Backend**
- [x] `internal/agent/trace.go` (yeni): `TurnStep` (kind=text|thinking|tool, tool/input/output/isError).
- [x] `internal/agent/toolloop.go`: `CompleteWithToolsTraced` — native agentic döngü ara metni + her `tool_use` çağrısını (girdi+sonuç) sıralı izle kaydeder. Eski `CompleteWithTools` bunu çağırıp izi yutar (imza uyumlu). İz **hem native hem claude-cli** (stream-json) yolunda üretilir — bkz. aşağıdaki "Anahtarsız adım izi" notu.
- [x] `db.Message.Steps` alanı (oturum JSONL'inde JSON `[]TurnStep`) — yeniden yüklemede tur yeniden çizilir; `AddMessage`/`ListMessages` korur. (O dönem SQLite `0007_message_steps.sql` migration'ıydı; depolama dosya-tabanlıya taşınınca alana dönüştü — bkz. `08-DEPOLAMA.md`.)
- [x] `internal/api/chat.go`: yanıt `steps` döndürür ve izi mesaja yazar.
- [x] `internal/api/files.go` (yeni): `GET /api/files?path=` — inline görsel için salt-okunur akış (görsel uzantı allowlist'i).

**Frontend** (yeni bağımlılıklar: react-markdown, remark-gfm, highlight.js)
- [x] `components/markdown/`: `Markdown.tsx` (GFM, özel kod/link/görsel render, `urlTransform` kapalı → yerel `C:\` yolları korunur, Windows ters-bölü normalize), `CodeBlock.tsx` (dil etiketi + kopya + highlight.js, diff→DiffView), `DiffView.tsx` (+N/−M satır renkli).
- [x] `components/chat/`: `TurnSteps.tsx` (iz çizimi + parseSteps), `ThinkingBlock.tsx` (açılır akıl yürütme), `ActivityCard.tsx` (tool kartı: ikon+etiket+niyet, açınca girdi/çıktı, Edit/Write→diff, hata kırmızı), `PathText.tsx` (metindeki yolları tıklanır çip).
- [x] `lib/`: `tools.ts` (tool meta), `paths.ts` (yol tespiti/görsel url), `diff.ts` (diff ayrıştırma).
- [x] `MessageList.tsx`: asistan turu ThinkingBlock → TurnSteps → markdown cevap; `App.tsx` `onOpenFile` (görsel→yeni sekme, diğer→pano); `index.css` `.sg-markdown` tipografisi + github-dark tema.

**CANLI TEST (Chrome DOM + API):**
- [x] Markdown: h2/liste/tablo, `go` kod bloğu highlight.js renkli span'larla, unified diff `+2 / −1` satır renkli — DOM doğrulandı.
- [x] Aktivite izi (DB'ye enjekte edilen örnek tur): düşünme bloğu, ara metin, Read/Edit/Bash tool kartları (tıklanabilir yol + kırmızı "hata" rozeti), Edit çıktısı diff olarak render.
- [x] Inline görsel: markdown `![](C:\…\hero.png)` → `src=/api/files?path=…`; uç **HTTP 200 image/png 13KB** döndürdü.
- [x] `go build ./...` + `tsc --noEmit` temiz.

**Anahtarsız adım izi (güncelleme):** `providers/claudecli.go` `--output-format stream-json --verbose`'a geçirildi; CLI'ın kendi olay akışı (`tool_use`/`tool_result`/`thinking`/`text`) ayrıştırılıp `Response.Trace` → `traceToSteps` ile `[]TurnStep`'e çevrilir. Böylece **API anahtarı olmadan** da tool kartları + ara adımlar görünür. Canlı test: claude-cli ajanı "Bash ile `echo hello-from-swarmgo`" → yanıt `steps`=[text, tool(Bash, output=hello-from-swarmgo)]; Chrome DOM'da ▶️ Bash kartı GIRDI/ÇIKTI ile render oldu. Native (anthropic) yol da kendi izini üretmeye devam eder.

---

### Faz 7 — Orchestration ✅ (2026-06-15)

Yapılandırılmış **çok-ajanlı akışlar**: bir akış = node grafiği (agent / branch / parallel).
Şablon sistemi + **restart-safe run state** (her node sonrası DB'ye yazılır, çökme sonrası kaldığı yerden devam).

**1) Graf modeli + motor (`internal/orchestration`)**
- [x] `model.go`: `Graph`/`Node`/`Branch`; node tipleri `agent`/`branch`/`parallel`; `Validate` (start/uniq/ref/agent kontrolleri)
- [x] `engine.go`: `Engine.Run` — start'tan yürür; `agent` node `AgentRunner` ile çalışır, çıktı saklanır; `branch` son çıktıya göre yönlendirir (case-insensitive substring, boş = varsayılan); `parallel` çocukları goroutine ile eşzamanlı koşar + join birleştirir; `maxSteps=50` döngü guard; her node sonrası `SaveFunc` ile state persist
- [x] Şablon: `{{input}}`, `{{last}}`, `{{node.<id>}}` prompt yer tutucuları

**2) DB (migration `0006_flows.sql`)**
- [x] `flows` (graph JSON) + `flow_runs` (restart-safe `state` JSON, status, input, output, error)
- [x] `models_flow.go` + `store_flow.go`: Flow CRUD + FlowRun (Create/Get/List/SetState/Finish + `ListRunningFlowRuns`)

**3) Agent entegrasyonu (`internal/agent/flow.go`)**
- [x] `flowRunner` → `orchestration.AgentRunner`: her node ajanın tam hattından geçer (memory recall + tools + budget via `complete`)
- [x] `RunFlow` (yeni run aç + motoru sür) + `driveFlow` (per-node persist + terminal status)
- [x] `ResumeRunningFlows` (boot'ta `running` kalan run'ları persist state'ten devam ettirir) — workspace açılışına bağlandı (restart-safe)

**4) API + Frontend**
- [x] `api/flows.go`: GET/POST `/api/flows`, PUT/DELETE `/api/flows/{id}`, POST `/api/flows/{id}/run`, GET `/api/flow-runs`, GET `/api/flow-runs/{id}`
- [x] `components/FlowsPanel.tsx`: görsel protokol builder — akış listesi, node editörü (agent/branch/parallel form kartları, başlangıç seçimi, sonraki/dal/paralel yönlendirme), çalıştır + trace görüntüleyici; NavRail "🔀 Akışlar"

**CANLI TEST (API + Chrome):**
- [x] **Flow A (agent→branch→agent)** "Sentiment Router": pozitif girdi → n1 "POSITIVE" → branch doğru şekilde n3 (Cheerful) → neşeli yanıt; trace state'e kalıcı yazıldı
- [x] **Flow B (parallel→join→agent)** "Pros & Cons": a1+a2 eşzamanlı koştu, çıktılar `[Advantage]…[Disadvantage]…` birleşti, s1 birleşik `{{last}}`'i özetledi
- [x] Chrome (DOM): "🔀 Akışlar" sekmesi; iki akış listelendi; node editörü 4 node'u tip/ajan/yönlendirme ile render etti; çalıştır bölümü göründü

> Bilinen sınır: `branch` substring eşleşmesi LLM çıktısının temizliğine duyarlı — claude-cli gevezelik ekleyince ("POSITIVE? No. NEGATIVE") yanlış dal seçilebilir. Motor doğru; sınıflandırma node'larında prompt katı tutulmalı veya varsayılan dal dikkatli sıralanmalı.

---

## ESKİ: FAZ 8 MCP + ARAÇLAR TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### Faz 8 — Tool-use + MCP ✅ (2026-06-15)

Ajanlara **araç kullanımı** kazandırıldı: hem yerleşik (built-in) hem **MCP sunucu** araçları.
SwarmClaw deseni: built-in + MCP tek katalogda, ajan başına atanır. İki yol birlikte kuruldu.

**1) Provider native tool-use protokolü (gerçek motor)**
- [x] `providers/provider.go`: `ToolDef`/`ToolCall`/`ToolResult`; `Request.Tools`, `Message.ToolCalls`/`ToolResults`, `Response.ToolCalls`+`StopReason`
- [x] `providers/anthropic.go`: content-block modeli (text/tool_use/tool_result); `tools` gönderimi + `tool_use` ayrıştırma
- [x] `agent/toolloop.go`: `CompleteWithTools` — native agentic döngü (provider → tool çalıştır → tool_result → tekrar, `maxToolIters=8`), bütçe guardrail entegre

**2) claude-cli MCP delegasyonu (anahtarsız, canlı test edilen yol)**
- [x] `providers/claudecli.go`: `ConfigureMCP` — `--mcp-config` + `--strict-mcp-config` + `--allowedTools`; CLI tool döngüsünü kendi içinde çalıştırır (API anahtarı gerekmez)
- [x] etkin MCP sunucularından geçici `--mcp-config` JSON üretimi + temizleme

**3) MCP istemcisi (`internal/mcp`) — SDK'sız elle JSON-RPC 2.0**
- [x] `client.go`: stdio; `initialize` → `notifications/initialized` → `tools/list` / `tools/call`
- [x] `manager.go`: `ServerConfig`, namespace (`<server>__<tool>`), `BuildCatalog`, `CallNamespaced`, `ListServerTools`; sse/http → net "henüz desteklenmiyor"

**4) Tool kayıt defteri (`internal/tools`)**
- [x] `registry.go`: `Tool` arayüzü + birleşik `Registry` (built-in + MCP); `Defs(allow)` allowlist, `Call` (hata → IsError)
- [x] Built-in: `get_current_time`, `http_get` (64KB sınır), `memory_recall`

**5) DB (migration `0005_mcp_tools.sql`)**
- [x] `mcp_servers` += `command`/`args`/`url`/`enabled`/`scope`; `agents` += `mcp_enabled`/`allowed_tools`
- [x] `models_mcp.go` + `store_mcp.go`: CRUD + `UpdateAgentTools`

**6) API + Frontend**
- [x] `api/mcp.go`: GET/POST `/api/mcp-servers`, `/toggle`, `/test`, DELETE
- [x] `api/agent_tools.go`: GET/POST `/api/agents/{id}/tools` (mcpEnabled + allowlist + canlı katalog)
- [x] chat/executor/heartbeat → `CompleteWithTools` (usage tek yerde)
- [x] `components/ToolsPanel.tsx` + NavRail "🔌 Araçlar" + types/api

**CANLI TEST (Go test + API + Chrome):**
- [x] `internal/mcp` live test: gerçek `@modelcontextprotocol/server-filesystem` (npx) → 14 araç, `list_directory` → `[FILE] note.txt`
- [x] API: MCP oluştur → `/test` ok=true 14 araç; ajan tools aç → katalog **17 araç** (14 MCP + 3 built-in)
- [x] API: claude-cli delegasyonu — "note.txt oku" → araçla okudu, **"hello from swarmgo"** (BOM + CRLF dahil → gerçekten araçla, tahmin değil)
- [x] Chrome (DOM): "🔌 Araçlar" sekmesi; 17 araç tam açıklamayla; filesystem MCP sunucusu Test/Kapat/Sil; ekleme formu render

> Not: `chrome_screenshot` odaktaki başka sekmeyi yakaladı (bilinen mcp-chrome sorunu); doğrulama DOM (`chrome_get_web_content`) ile yapıldı.

---

## ESKİ: Otomatik başlık üretimi (auto-title) ✅ (2026-06-15)

Görev ve sohbetlere otomatik başlık üreten ortak bir sistem eklendi. Kanban'da artık
yalnızca prompt girilir; başlık prompt'tan üretilir. Sohbette ilk mesaj gönderilince
başlık otomatik oluşur. Her ikisi de istenildiğinde ⟳ ile yeniden üretilebilir.

**Backend**
- [x] `internal/agent/titler.go` (yeni): `GenerateTitle` (ajan provider'ı ile kısa başlık),
  `TitleFor` (tercih edilen ajan yoksa ilk ajana düşer; üretim hatasında prompt'tan
  `FallbackTitle`), `SanitizeTitle` (tek satır, tırnak/noktalama temizliği, 60 karakter sınırı).
  Başlık talimatı hem system hem **user turn** içine gömülü — claude-cli'ın büyük taban
  promptunun appended system'i bastırmasını önler.
- [x] `db/store.go`: `SetSessionTitle`.
- [x] `api/chat.go`: ilk turda (kind=chat, başlık boş, messageCount=0) başlık otomatik üretilir,
  `sessionTitle` alanı ile döner; best-effort (hata yanıtı bozmaz).
- [x] `api/sessions.go`: `POST /api/sessions/{id}/title` — sohbet geçmişinden (veya `source`) yeniden üret.
- [x] `api/tasks.go`: görev oluşturmada `title` opsiyonel (prompt'tan üretilir); `POST /api/tasks/{id}/title` yeniden üret.

**Frontend**
- [x] `types.ts`/`api.ts`: `ChatResponse.sessionTitle`; `generateSessionTitle`, `generateTaskTitle`; `createTask.title` opsiyonel.
- [x] `TaskBoard.tsx`: form'dan başlık alanı kaldırıldı (yalnız prompt + ajan); her kartta ⟳ "başlığı yeniden oluştur".
- [x] `Sidebar.tsx`: her oturumda hover'da ⟳ başlık yenileme.
- [x] `App.tsx`: chat yanıtındaki `sessionTitle` ile oturum başlığı güncellenir; `regenerateSessionTitle`.

**CANLI TEST (API):**
- [x] Görev: yalnız prompt → başlık "Veritabanı yedekleme cron görevi kurulumu" otomatik üretildi.
- [x] Görev yeniden başlık: ⟳ → "Veritabanı yedekleme gece cron ve hata raporu".
- [x] Sohbet: ilk mesaj → `sessionTitle` "Python liste tuple farkı", DB'ye yazıldı.
- [x] Frontend: `tsc -b && vite build` temiz; uygulama tarayıcıda render oldu (DOM doğrulandı).
- Not: Chrome görsel testi 80+ sekmeli ortamda kararsız (image readback) — doğrulama API + DOM metni üzerinden yapıldı.

### UI yeniden düzenleme — 3 kolonlu yerleşim ✅ (2026-06-15)
- [x] `components/NavRail.tsx` (yeni): en solda **daraltılabilir** nav rail — marka + workspace switcher + görünüm geçişleri (💬 Sohbet · 🗂 Görevler · ⏰ Zamanlamalar · ⛁ Hafıza). Daraltınca ikon-only (w-14 ↔ w-52); tercih `localStorage`'da.
- [x] `components/Sidebar.tsx`: artık orta kolon — yalnızca Ajanlar + Oturumlar listesi (logo/workspace NavRail'e taşındı). **Yalnızca ajan-bazlı görünümlerde (Sohbet, Hafıza) gösterilir;** Görevler/Zamanlamalar workspace-bazlı olduğundan orada gizli.
- [x] `App.tsx`: `NavRail → Sidebar → main` 3 kolon; görünüm sekmeleri header'dan NavRail'e taşındı, header artık görünüm başlığı + (chat'te) meter'ları gösterir.
- [x] **Chrome canlı test:** 3 kolon render, daralt/genişlet çalışıyor, meter'lar header'da.



### Faz 6.5 — Sağlamlaştırma ✅ (2026-06-15)

SwarmClaw kıyaslamasında öne çıkan iki kritik açık kapatıldı: **bağlam (context) yönetimi** ve **otonom döngü maliyet guardrail'i**.

**1) Bağlam yönetimi + compaction (`internal/conversation`)**
- [x] `tokens.go`: tokenizer-bağımsız token tahmini (~4 char/token)
- [x] `manager.go`: `Manager.Prepare` — pending geçmiş bütçeyi aşınca eski turları **rolling summary**'ye katlar (compaction), sadece özet + son N tur gönderilir
- [x] Migration `0004`: `sessions.summary` + `summary_msg_count`; `db.SetSessionSummary`
- [x] `api/chat.go`: her turda Prepare çağrılır; özet sistem promptuna enjekte edilir; yanıt `contextTokens` + `compacted` döner
- [x] Env: `SWARMGO_MAX_CONTEXT_TOKENS` (varsayılan 12000), `SWARMGO_KEEP_RECENT_MSGS` (8)
- [x] **Önceki bug:** chat her turda TÜM geçmişi gönderiyordu → uzun oturumda context taşması + artan maliyet. Artık sınırlı.

**2) Maliyet / bütçe guardrail'i (otonom döngü)**
- [x] Migration `0004`: `agents.daily_call_limit` + `daily_token_limit` (0=sınırsız); `agent_usage(agent_id, day, calls, in/out tokens)` tablosu
- [x] `db/store_usage.go`: `AddUsage` (upsert), `GetUsageToday`, `UpdateBudget`
- [x] `agent/budget.go`: `guardedComplete` — **tek provider çağrı hunisi**; otonom çağrıda `ensureBudget` (limit aşılırsa `ErrBudgetExceeded`, provider'a gitmeden), her çağrıda usage kaydı
- [x] Tüm otonom yollar guardedComplete'e bağlandı: heartbeat (otonom), scheduler prompt/task (otonom), reflect (kullanıcı tetikli=false ama usage kaydı var). Manuel chat + run-now budget'a takılmaz.
- [x] API: GET `/api/agents/{id}/usage`, POST `/api/agents/{id}/budget`, GET `/api/sessions/{id}/context`

**Frontend**
- [x] `components/ChatMeters.tsx`: chat başlığında **⛁ context meter** (token + ⧉ özet göstergesi) ve **◷ bütçe meter** (bugünkü çağrı/limit, tıkla→limit ayarla)
- [x] `App.tsx`: chat header'a meter'lar; her turdan sonra `meterRefresh`

**CANLI TEST (API + Chrome):**
- [x] Compaction: maxctx=120 ile 4 mesaj → 4.'te `compacted=true`, summary_msg_count=5, özet kalıcı bilgileri yakaladı (Go, Türkiye, oyun, SQLite)
- [x] Budget: limit=4 (mevcut kullanım) → heartbeat wake → "failure: daily budget exceeded", **token harcanmadı** (provider'a gitmeden bloklandı); manuel chat etkilenmedi
- [x] Chrome: chat başlığında ⛁ 362 ⧉ ve ◷ 4 çağrı göstergeleri render edildi

> Not: claude-cli compaction özetine kendi persona tonunu katıyor (anthropic provider'da daha temiz). İşlevsel olarak doğru.

---

## ESKİ: FAZ 6 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 6 — Memory ✅ (2026-06-15)

Ajan hafızası: belge + günlük (journal) + yansıma (reflection). Recall **saf Go sözcüksel benzerlik** (token-frekans cosine) — harici embedding API yok, CGO yok, çevrimdışı. Tüm anılar workspace-scoped.

**Mimari karar:** Anahtarsız/CGO-free felsefeye uygun olarak gerçek semantik embedding yerine lexical cosine kullanıldı; term-vektörü `knowledge_sources.embedding` BLOB'unda cache'lenir. İleride aynı `memory.Store` arkasına gerçek embedder takılabilir (`0001`'deki şema yeterli, migration gerekmedi).

**Memory paketi (`internal/memory`)**
- [x] `vector.go`: Unicode-aware tokenizasyon (TR+EN stopword), tf-vektör, cosine, JSON marshal/unmarshal
- [x] `memory.go`: `Store` (db sarmalayıcı) — Remember / Recall (top-N, minScore eşiği) / List / Delete / **ContextBlock** (sistem promptuna enjekte edilecek blok)

**DB**
- [x] `models_memory.go`: `KnowledgeSource` + kind sabitleri (document/journal/reflection)
- [x] `store_memory.go`: CreateKnowledge / GetKnowledge / ListKnowledge(kind filtreli) / DeleteKnowledge + `placeholders` helper

**Agent paketi**
- [x] `runtime.go`: Runtime'a `mem *memory.Store` alanı + `Memory()` erişimcisi
- [x] `reflector.go`: `Journal` (aktiviteyi kaydet) + `Reflect` (dream cycle — son ~20 journal'ı provider'a özetletip reflection olarak sakla)
- [x] `executor.go`: `invokeWithMemory` (recall→sistem promptuna enjekte) + başarılı task sonrası journal; `complete` ortak helper

**Enjeksiyon noktaları**
- [x] `api/chat.go`: her turda recall→enjekte + tur sonrası journal
- [x] `agent/executor.go`: task çalıştırmada recall→enjekte + journal

**API uçları**
- [x] GET/POST `/api/agents/{id}/memories` (liste, ?kind= filtreli / belge ekle)
- [x] POST `/api/agents/{id}/reflect` (yansıma üret)
- [x] POST `/api/agents/{id}/recall` (recall önizleme — skorlu, debug)
- [x] DELETE `/api/memories/{id}`

**Frontend**
- [x] `types.ts`/`api.ts`: Memory/RecallHit + uç metodları
- [x] `components/MemoryPanel.tsx`: belge ekle, ✦ Yansıt, tür filtreleri (Tümü/Belgeler/Günlük/Yansımalar), rozetli liste, sil
- [x] `App.tsx`: 4. görünüm "Hafıza" (aktif ajana göre)

**CANLI TEST (API + Chrome):**
- [x] API: 3 belge eklendi; recall "hangi veritabani" → SQLite anısı skor 0.236 ile döndü (diğerleri eşik altı)
- [x] API: chat "hangi veritabani?" → ajan enjekte edilen anıdan **"SQLite"** cevapladı (başka bilme yolu yok); tur journal'landı
- [x] API: reflect → ajan journal üzerine birinci şahıs yansıma yazdı, kullanıcının kısa cevap tercihini bile not etti
- [x] Chrome: Hafıza sekmesinde belge/günlük/yansıma rozetli olarak göründü

---

## ESKİ: FAZ 5 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 5 — Tasks + Schedules ✅ (2026-06-15)

Görev panosu (kanban) + cron zamanlama. Hepsi **workspace-scoped** (her workspace kendi scheduler'ı).

**DB (migration `0003_tasks_schedules.sql`)**
- [x] `tasks`/`schedules`/`runs` zaten 0001'de vardı; Faz 5 kolonları eklendi: tasks → `prompt`, `last_run_id/status/at`; schedules → `task_id`, `prompt`, `last_run_at`; runs → `output`, `trigger` + indeksler
- [x] `internal/db/models_task.go`: Task / Schedule / Run tipleri + board state sabitleri + `ValidBoardState`
- [x] `internal/db/store_task.go`: CreateTask/GetTask/ListTasks/UpdateTask/MoveTask/SetTaskLastRun/DeleteTask + `nullable`/`mustAffect` helper'ları
- [x] `internal/db/store_run.go`: CreateRun/FinishRun/ListRuns/GetRun
- [x] `internal/db/store_schedule.go`: CRUD + ListEnabledSchedules + SetScheduleEnabled/Delivery + GetOrCreateKindSession

**Çalıştırma + zamanlama (agent paketi)**
- [x] `internal/agent/executor.go`: `Runtime.RunTask` — run aç → board `in_progress` → provider çağrısı → çıktı + done/failed; `invoke` helper'ı (tek prompt)
- [x] `internal/agent/scheduler.go`: `robfig/cron/v3` ile workspace başına Scheduler; Start/Reload/Stop; fire → task çalıştır veya prompt teslim et; `nextRunAt` senkronu; geçersiz cron yakalama
- [x] `internal/workspace/manager.go`: Workspace'e `Scheduler` alanı; open'da Start, Delete/Close'da Stop

**API uçları**
- [x] `internal/api/tasks.go`: GET/POST `/api/tasks`, PUT/DELETE `/api/tasks/{id}`, POST `/api/tasks/{id}/run`, GET `/api/tasks/{id}/runs`
- [x] `internal/api/schedules.go`: GET/POST `/api/schedules`, POST `/api/schedules/{id}/toggle`, DELETE `/api/schedules/{id}` (her yazımda Scheduler.Reload)

**Frontend**
- [x] `types.ts`/`api.ts`: Task/Run/Schedule tipleri + tüm uç metodları
- [x] `components/TaskBoard.tsx`: 5 sütunlu kanban, HTML5 sürükle-bırak ile taşıma, görev oluştur, ▶ Çalıştır, run geçmişi, sil
- [x] `components/Schedules.tsx`: cron preset'leri + serbest cron, görev/prompt seçimi, aç-kapa toggle, sil, sonraki/son çalışma
- [x] `App.tsx`: üstte Sohbet / Görevler / Zamanlamalar görünüm değiştirici

**CANLI TEST (API + Chrome):**
- [x] API: ajan→görev→çalıştır (claude-cli "Merhaba!"), board done'a geçti, run geçmişi yazıldı
- [x] API: cron `*/1 * * * *` schedule gerçekten tetiklendi, ajan prompt'a cevap verdi, schedule oturumuna kaydedildi
- [x] Chrome: Görevler sekmesinde form ile görev oluşturuldu → ▶ Çalıştır → claude-cli "4" → kart **Bitti** sütununa otomatik geçti, yeşil `● success` rozeti
- [x] Chrome: Zamanlamalar görünümü cron preset'leriyle render edildi

> Not: Cron 5-alanlı standart format (dk sa gün ay haftagünü). Çalıştırma claude-cli üzerinden anahtarsız.

---

## ESKİ: WORKSPACE İZOLASYONU TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Ara Faz — Workspace İzolasyonu ✅ (kullanıcı talebiyle Faz 5 öncesi eklendi)
Detaylı tasarım: [06-WORKSPACES.md](06-WORKSPACES.md)

- [x] `internal/workspace/manager.go`: her workspace = ayrı DB + ayrı runtime; workspaces.json kayıt defteri; Create/Delete/Get/List/Default/Close
- [x] `internal/api/server.go`: `withWorkspace` middleware (X-Workspace-Id header → context); workspace CRUD uçları
- [x] Tüm handler'lar workspace-scoped (`ws(r).DB`, `ws(r).Runtime`)
- [x] `main.go`: tek db/runtime yerine Manager
- [x] Frontend: `WorkspaceSwitcher.tsx`, api.ts header injection + localStorage, App.tsx workspace state + geçişte tam reset
- [x] **CANLI TEST (API + Chrome):** WS1=Ajan-A, WS2=Ajan-B; her workspace yalnızca kendi verisini görüyor; UI switcher ile geçiş çalışıyor; ayrı .db dosyaları ✅

### Mimari karar
Tek DB + workspace_id kolonu yerine **fiziksel ayrım** (workspace başına ayrı swarmgo.db) → sıfır sızıntı riski.

---

## ESKİ: Faz 4 TAMAMLANDI ✅ (OTONOM HEARTBEAT CANLI TEST GEÇTİ)

### Faz 4 — Agent Runtime ✅
- [x] Migration `0002_heartbeat.sql`: agents'a heartbeat_enabled/interval/prompt sütunları
- [x] `internal/db/store.go`: scanAgent/scanSession refactor, UpdateHeartbeat, GetOrCreateHeartbeatSession
- [x] `internal/agent/worker.go`: per-agent goroutine, heartbeat ticker, wake/stop channel, exponential backoff, 10 hatada otomatik devre dışı
- [x] `internal/agent/runtime.go`: yaşam döngüsü yöneticisi (Start/Stop/Wake/Status/StartConfigured/StopAll) + heartbeat eylemi (LLM çağrısı → heartbeat oturumuna yaz)
- [x] `internal/api/runtime.go`: GET /api/runtime, POST /api/agents/{id}/heartbeat, POST /api/agents/{id}/wake
- [x] `main.go`: boot'ta StartConfigured, shutdown'da StopAll
- [x] **CANLI TEST:** ajan 5sn aralıkla kendi kendine uyandı, Claude'u çağırdı, "Sen yapabilirsin, asla pes etme!" cevabını heartbeat oturumuna yazdı; runtime durumu success ✅

### Yeni API uçları
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/api/runtime` | Tüm worker'ların durumu |
| POST | `/api/agents/{id}/heartbeat` | Heartbeat aç/kapat + ayarla |
| POST | `/api/agents/{id}/wake` | Anlık uyandır |

### Runtime mekanikleri
- Her ajan = 1 goroutine; kontrol kanalları (wake/stop) ile yönetilir
- Outcome classification: success → failures sıfırlanır; failure → artar, backoff uygulanır
- Exponential backoff: `base * 2^failures` (maxBackoffSteps ile sınırlı)
- 10 ardışık hata → ajan otomatik devre dışı (Disabled=true)

### ⏳ Sonraki (Faz 4 artıkları, opsiyonel)
- [ ] Heartbeat durumunu UI'da göster (runtime paneli)
- [ ] claude-cli için araç kısıtlama (`--allowedTools ""`) — saf chat güvenliği

---

## ESKİ: Faz 3 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 3 — React Web UI ✅
- [x] `frontend/`: Vite + React + TypeScript + Tailwind v4 kuruldu
- [x] `vite.config.ts`: Tailwind eklentisi + backend proxy (:8090)
- [x] `src/index.css`: özel koyu tema (@theme değişkenleri)
- [x] `src/types.ts`, `src/api.ts`: tip güvenli API istemcisi
- [x] `src/components/Sidebar.tsx`: ajan listesi + oluşturma formu + oturumlar
- [x] `src/components/MessageList.tsx`: mesaj balonları + typing animasyonu + auto-scroll
- [x] `src/components/Composer.tsx`: mesaj girişi (Enter ile gönder)
- [x] `src/App.tsx`: durum yönetimi, optimistic UI, hata gösterimi
- [x] `npm run build`: TypeScript temiz derlendi
- [x] **CHROME CANLI TEST:** ajan seç → geçmiş yüklendi → yeni mesaj "3+7" → cevap "10." ✅
- [x] Türkçe karakterler UI'da kusursuz (PowerShell mojibake'si sadece konsoldaymış)

### Çalıştırma (geliştirme)
```powershell
# Terminal 1 - backend
$env:SWARMGO_ADDR=":8090"; go run ./cmd/swarmgo
# Terminal 2 - frontend
cd frontend; npm run dev   # http://localhost:5173
```

### ⏳ Sonraki iyileştirmeler
- [ ] Streaming (WebSocket ile token token akış)
- [ ] Frontend build'i Go binary'sine embed (tek dosya dağıtım)

---

## ESKİ: Faz 2 TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### 🎉 Önemli: API anahtarı OLMADAN çalışıyor (claude-cli provider)
SwarmClaw'un "CLI provider" yaklaşımı eklendi. Yerel `claude` (Claude Code) CLI'ı
kullanıcının OAuth/abonelik girişiyle çalışır — **API anahtarı gerekmez.**

- [x] `internal/providers/claudecli.go`: `claude -p --output-format json` ile shell-out
- [x] `internal/providers/registry.go`: claude CLI otomatik tespit (exec.LookPath)
- [x] Yeni ajanların varsayılan provider'ı: `claude-cli`
- [x] **CANLI TEST:** Çok turlu sohbet çalıştı, hafıza korundu, mesajlar DB'ye yazıldı ✅
- [x] Çözülen sorun: `--bare` flag'i keychain okumasını atlayıp "Not logged in" veriyordu → kaldırıldı

### İki provider seçeneği (ajan başına seçilebilir)
| Provider | Auth | Maliyet | Persona kontrolü |
|----------|------|---------|------------------|
| `claude-cli` | OAuth/abonelik (anahtarsız) | Abonelik dahili | Claude Code kimliği taban + append |
| `anthropic` | API anahtarı | Token başına ücret | Tam temiz kontrol |

### ⚠️ Bilinen kısıtlar / sonraki iyileştirmeler
- claude-cli, Claude Code'un taban sistem prompt'unu (≈23k token) yükler → her çağrı bu yükü taşır
- claude-cli print modunda araç (Bash/Edit) erişimi olabilir → saf chat için `--allowedTools ""` ile kısıtlanmalı
- Streaming henüz yok (Faz 3'te WebSocket)

---

## ESKİ: Faz 2 KOD TAMAM

### Faz 2 — Anthropic Provider + Chat MVP ✅ (kod)
- [x] `internal/db/models.go`: Agent, Session, Message tipleri
- [x] `internal/db/store.go`: CRUD (CreateAgent/GetAgent/ListAgents, sessions, messages, transaction'lı AddMessage)
- [x] `internal/providers/provider.go`: ortak `Provider` arayüzü + Message/Request/Response
- [x] `internal/providers/anthropic.go`: ince HTTP istemci (Messages API, SDK'sız)
- [x] `internal/providers/registry.go`: sağlayıcı seçimi (anthropic)
- [x] `internal/api/server.go`: router + CORS + health
- [x] `internal/api/agents.go`, `sessions.go`, `chat.go`: HTTP handler'ları
- [x] `main.go` API sunucusuyla bağlandı
- [x] Test: ajan/oturum/mesaj akışı ✅; chat anahtarsız güvenli hata veriyor ✅

### ⏳ Açık iş
- [ ] **Canlı chat testi:** `ANTHROPIC_API_KEY` verilince gerçek Claude cevabı doğrulanacak
- [ ] Streaming (Faz 3'te WebSocket ile)

### API Uçları (mevcut)
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/health` | Sağlık + db |
| GET/POST | `/api/agents` | Ajan listele/oluştur |
| GET/POST | `/api/sessions` | Oturum listele/oluştur |
| GET | `/api/sessions/{id}/messages` | Mesaj geçmişi |
| POST | `/api/chat` | Sohbet turu (Claude) |

---

## ESKİ: Faz 1 TAMAMLANDI ✅

### Faz 1 — Veritabanı ve Config ✅
- [x] `internal/config/config.go`: env okuma, dizin çözümleme, DBPath
- [x] `internal/config/secret.go`: AES-GCM credential şifreleme (env→dosya→üretim)
- [x] `internal/db/db.go`: SQLite (modernc.org/sqlite, CGO yok), WAL + busy_timeout + FK
- [x] `internal/db/migrate.go`: embed.FS migration runner + schema_migrations takibi
- [x] `internal/db/migrations/0001_init.sql`: 11 tablo (agents, sessions, messages, tasks, schedules, runs, connectors, knowledge_sources, skills, mcp_servers, provider_configs)
- [x] `main.go` config+db ile bağlandı; `/health` artık `db:true` döndürüyor
- [x] `.gitignore` eklendi
- [x] Test: DB oluştu, migrate geçti, health `db:true` ✅

---

## ESKİ: Faz 0 TAMAMLANDI ✅

### Tamamlananlar ✅
- [x] Go 1.26.4 kuruldu ve doğrulandı
- [x] Node.js v24 + npm 11 mevcut (frontend için hazır)
- [x] Proje klasör yapısı oluşturuldu (`C:\Users\user\Desktop\Projects\SwarmGo`)
- [x] `go mod init github.com/bilal/swarmgo`
- [x] `_Docs` plan dokümanları yazıldı (00-05)
- [x] `cmd/swarmgo/main.go`: HTTP sunucu + `/health` ucu (graceful shutdown, slog)
- [x] `go build` + `go vet` temiz
- [x] Çalıştırma testi: `/health` → `{"status":"ok"}` ✅
- [x] Kök `README.md` (Türkçe)

### Sıradaki Adımlar ⏳ (Faz 1)
- [ ] Wails v2 CLI kurulumu (Faz 9'a kadar ertelenebilir)
- [ ] `internal/db`: SQLite (modernc.org/sqlite) bağlantısı
- [ ] Migration runner + `0001_init.sql`
- [ ] `internal/config`: env + credential secret (AES-GCM)

### Sonraki Faz: Faz 1 (Veritabanı ve Config)
Detaylar için bkz. [03-YOL-HARITASI.md](03-YOL-HARITASI.md)

---

## Karar Bekleyen Konular

Kullanıcıyla netleştirilecek:
1. **İlk LLM sağlayıcısı:** Anthropic (Claude) mı, OpenAI mı, yoksa yerel Ollama mı?
2. **Frontend framework:** React (varsayılan) mı, Svelte mi?
3. **MVP kapsamı:** Tek ajan + chat ile mi başlayalım?

---

## Oturum Günlüğü

### 2026-06-16 — Modülerlik refactor'ları (davranış değişmedi)
Dört adet düşük-riskli, davranış-korumalı refactor uygulandı (`go build`/`go vet`/`go test ./...` + canlı `/health` smoke testi yeşil):

1. **`providers/transport.go` (yeni):** Ortak `postJSON` HTTP yardımcısı. `anthropic.go` ve `minimax.go`'daki tekrar eden marshal → request → header → `Do` → `ReadAll` → unmarshal iskeleti tek noktaya alındı.
2. **`api/server.go` → `writeDBError`:** Her handler'da tekrar eden `db.ErrNotFound → 404 / diğer → 500` eşlemesi merkezîleştirildi; `agents/sessions/tasks/schedules/memory/usage` handler'ları sadeleşti. (`handleRunTask` 400 semantiği korunarak hariç tutuldu.)
3. **`agent/tunables.go` global state → `Tunables` struct:** Süreç-geneli paket globalleri kaldırıldı; tek `*Tunables` örneği `main.go`'da oluşturulup `NewManager`/`NewRuntime`/`NewServer` üzerinden enjekte ediliyor. `budget.go`/`titler.go` artık `r.tun` kullanıyor.
4. **`db/store.go` → `mutateAgentLocked`/`mutateSessionLocked`:** Kilitle→bul→değiştir→persist kalıbı generic yardımcılara alındı; `UpdateAgent`, `UpdateHeartbeat`, `UpdateBudget`, `UpdateAgentTools`, `SetSessionSummary`, `SetSessionTitle` sadeleşti.

> Not: `tunables.go` artık `Set*` yerine `*Tunables` metotları sunuyor (önceki satır 133'teki paket-fonksiyon imzaları değişti).

#### Devam refactor'ları (aynı gün)
5. **`agent/climcp.go` (yeni):** claude-cli `--mcp-config` üretimi (`cliMCPConfig`/`cliMCPServer` + `writeCLIMCPConfig`) `toolloop.go`'dan ayrı, kohezyonlu bir dosyaya taşındı. `ToolCatalog` registry'nin yanına (`toolsetup.go`) alındı. `toolloop.go` artık yalnızca completion/agentic-loop mantığına odaklı. (Saf kod taşıma — davranış değişmedi.)
6. **`api/server.go` → `register*Routes`:** Tek `Routes()` bloğu domain-bazlı 13 yardımcıya bölündü (`registerAgentRoutes`, `registerTaskRoutes`, …).
7. **Testler (yeni):** `providers/transport_test.go` (`postJSON`: başlık/gövde, non-200, decode hatası, transport hatası), `providers/minimax_test.go` (httptest ile `Complete` başarı/hata/varsayılan baseURL), `agent/tunables_test.go` (`*Tunables` get/set + eşzamanlı erişim), `api/server_test.go` (rota kaydı panik regresyon koruması).

> Repo hijyeni: `internal/` altında 36 kaynak dosyası henüz versiyon kontrolüne hiç girmemiş durumda (HEAD tek başına derlenmez). Tüm kaynak ağacını tek seferlik "track existing sources" commit'iyle eklemek önerilir.

### 2026-06-15
- Proje başlatıldı.
- SwarmClaw mimarisi analiz edildi, Go karşılıkları belirlendi.
- Klasör yapısı + go.mod + plan dokümanları oluşturuldu.
