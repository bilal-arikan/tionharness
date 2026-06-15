# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-06-15**

## Kararlar (2026-06-15)
- **İlk LLM sağlayıcısı:** Anthropic (Claude) ✅
- **Frontend:** React ✅

## Mevcut Durum: FAZ 6.5 SAĞLAMLAŞTIRMA TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ) → Faz 7 (Orchestration)

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
- [ ] Wails v2 CLI kurulumu (Faz 10'a kadar ertelenebilir)
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

### 2026-06-15
- Proje başlatıldı.
- SwarmClaw mimarisi analiz edildi, Go karşılıkları belirlendi.
- Klasör yapısı + go.mod + plan dokümanları oluşturuldu.
