# SwarmGo — Depolama Katmanı (Dosya Sistemi)

> Son güncelleme: **2026-06-19**
> SwarmGo'nun kalıcılık katmanı **SQLite'tan tamamen dosya sistemine** taşındı.
> SQLite (`modernc.org/sqlite`), migration runner ve `.sql` dosyaları kaldırıldı.

## Neden dosya sistemi?

External Agents OSS'in "klasör = workspace" yaklaşımı örnek alındı: tüm veri
insan-okunabilir **JSON / JSONL** dosyalarında. Avantajlar:

- **Taşınabilir & git'lenebilir:** workspace klasörünü kopyala/versiyonla.
- **Şeffaf:** her kayıt diskte okunabilir; harici araçlarla incelenebilir.
- **Bağımlılıksız:** `modernc.org/sqlite` + tüm dolaylı bağımlılıklar gitti
  (`go.mod` artık yalnız `google/uuid` + `robfig/cron`).
- **Tek binary, CGO yok** hedefiyle tam uyumlu.

## Mimari: bellek-içi + diske yazma (write-through)

`internal/db` paketi **yerinde** dosya-tabanlıya çevrildi; **tüm metod imzaları,
model struct'ları, sabitler ve `ErrNotFound` aynı kaldı**. Bu sayede onu kullanan
22 dosya (`agent`, `api`, `conversation`, `memory`, `workspace`) **tek satır
değişmeden** çalışmaya devam etti.

- `Open(path)` → tüm entity'leri diskten okuyup belleğe yükler.
- Okumalar bellekten servis edilir (hızlı, sıralı).
- Her mutasyon: belleği günceller **ve** etkilenen dosyayı **atomik** yazar
  (`*.tmp` yaz → `rename`), böylece çökme yarım dosya bırakmaz.
- Tek `sync.RWMutex` eşzamanlılığı korur (ajan-başına goroutine + scheduler +
  flow runner aynı anda yazabilir). SQLite'ın WAL + tx garantilerinin yerini alır.

## Disk yapısı

Her workspace fiziksel olarak izole; kökü `{dataDir}/workspaces/{wsID}/store/`:

```
store/
├── agents/{id}.json
├── sessions/{id}/session.jsonl      # satır 1: oturum header'ı, satır 2+: mesajlar
│   └── inflight.json                 # (geçici) stream'lenen asistan turu — crash kurtarma sidecar'ı
├── tasks/{id}.json
├── runs/{id}.json
├── schedules/{id}.json
├── knowledge/{id}.json              # embedding base64 olarak gömülü
├── mcp-servers/{id}.json
├── flows/{id}.json
├── flow-runs/{id}.json
├── usage/{agentID}__{YYYY-MM-DD}.json
└── counters.json                      # entity-başına insan-okunabilir id sayacı
```

## Kimlik (ID) şeması — insan-okunabilir + tekrar-kullanımsız

> 2026-06-19'dan itibaren yeni entity'ler **kısa, prefix'li** kimlik alır
> (örn. `TSK7`, `AGT3`). Eski UUID kimlikler **dokunulmadan** çalışmaya devam
> eder (lookup'lar opak string üzerinden; iki şema yan yana yaşar). Kimlik aynı
> zamanda dosya/klasör adıdır → diskte göz gezdirmek artık çok daha okunabilir.

**Prefix haritası (İngilizce mnemonik, `internal/db/db.go`):**

| Entity | Prefix | Örnek |
|--------|--------|-------|
| Workspace | `WS` | `WS1` |
| Agent | `AGT` | `AGT3` |
| Session | `SES` | `SES42` |
| Task | `TSK` | `TSK17` |
| Flow | `FLW` | `FLW5` |
| Flow Run | `RUN` | `RUN128` |
| Artifact | `ART` | `ART9` |
| Knowledge/Memory | `MEM` | `MEM88` |
| MCP Server | `MCP` | `MCP4` |
| Hook | `HOK` | `HOK2` |
| Schedule | `SCH` | `SCH7` |

> **Mesajlar** hâlâ UUID kullanır (yüksek hacimli hot-path + UI'da görünmez).
> Task `Run` ID'leri de UUID'de kalır (legacy, artık üretilmiyor). Merkez-dışı
> geçici tanımlayıcılar (chat `runID`/`replyID`, spawn `SourceID`, upload dosya
> adı) de UUID'de kalır. Workspace ID'leri ise ayrıca `WS<n>`'e geçti (aşağıda).

**Mekanik (`DB.nextID(prefix)`):**
- Prefix-başına monoton sayaç; `counters.json`'da tutulur (`{"TSK":17,...}`),
  her tahsiste **atomik** (`*.tmp`→`rename`) yazılır.
- **Tekrar kullanım yok:** sayaç yalnız artar; silme bir numarayı serbest
  bırakmaz, restart bir numarayı yeniden vermez (`load()` → `loadCounters()`).
- Eski UUID'ler **taranmaz/dikkate alınmaz**; prefix'li id'ler saf harf+rakam
  olduğundan bir UUID ile asla çakışamaz.
- Kendi mutex'i (`countersMu`) vardır → hem `mu` alınmadan (çoğu `Create*`)
  hem de `mu` tutulurken (örn. `createSessionLocked`) güvenle çağrılabilir.

**Workspace kimlikleri (`WS<n>`):** db dışında, `internal/workspace/manager.go`'da
üretilir. Sayaç `{dataDir}/ws-counter.json`'da tutulur; `NewManager` boot'ta
sayaç dosyası + mevcut `WS<n>` metalarının maks'ını alarak rewind'i önler
(`internal/workspace/id.go`). Workspace ID'si dizin adıdır (`workspaces/<id>/`);
entity dosyalarının **içine gömülü değildir** (workspace'ler izole).

### Eski UUID → yeni şema migration aracı (`cmd/migrate-ids`)

Tek-seferlik, **idempotent** dönüştürücü. Strateji: her UUID global benzersiz bir
token olduğundan, (1) tüm migratable entity'ler için eski→yeni ID haritası kurar,
(2) bu token'ları workspace ağacındaki **tüm metin dosyalarında** birebir değiştirir
— her cross-reference'ı (AgentID, SessionID, OwnerAgentID, `Task.Dependencies`,
`Flow.Graph` düğüm ID'leri, içerik-dosyası yolları…) alan-alan saymadan yakalar —
ve (3) ID ile adlandırılmış dosya/klasörleri yeniden adlandırır (entity JSON,
session klasörleri, artifact içerik dosyaları, upload klasörleri, usage dosyaları).
Sayaç dosyalarını (`counters.json` / `ws-counter.json`) ileri taşır → çalışan
uygulama numaralamaya kaldığı yerden devam eder.

- **Varsayılan dry-run** (hiçbir şey değişmez); `-apply` ile uygulanır ve önce
  tüm `dataDir` zaman damgalı yedeklenir (`-backup=false` ile atlanır).
- **Junction/symlink güvenli:** içerik-rewrite yalnız `store/` üzerinde çalışır
  (tüm ID referansları orada); workspace içerik dizini (artifacts/uploads) bir
  reparse-point (junction) ise — örn. sandbox gerçek bir projeye bağlıyken —
  ona dalınmaz, yedekleme de junction'ları atlar (uyarı basar). Junctioned
  workspace'in `artifacts/uploads` alt-klasörleri yine sığ olarak yeniden adlanır.
- **Uygulama KAPALIYKEN çalıştırın** (write-through dosyalar değişeceğinden).
- Mesaj (`msg`) ve task `Run` ID'leri UUID'de kalır (migrate edilmez); onlara
  yapılan referanslar yine token-replacement ile düzeltilir.
- Kullanım:
  ```powershell
  go run ./cmd/migrate-ids                          # dry-run (~/.swarmgo)
  go run ./cmd/migrate-ids -apply                   # uygula (önce yedek)
  go run ./cmd/migrate-ids -data D:\sg -apply -workspaces=false
  ```

> Eski yol `{wsID}/swarmgo.db` idi; artık `{wsID}/store/` dizini.

## Oturum biçimi (JSONL — Craft tarzı)

`session.jsonl` her oturum için tek dosya:

- **Satır 1** = `Session` header'ı (id, agentId, kind, title, messageCount, state,
  summary, summaryMsgCount, zaman damgaları). **Zenginleştirilmiş alanlar (2026-06-26):**
  `v` (SchemaVersion — header format sürümü, ileri-migration için), `labels[]`
  (serbest etiket, filtre/otomasyon), `status` (serbest **workflow** durumu —
  state=lifecycle'dan ayrı), `pinned` (sidebar'da üste sabitleme; `ListSessions`
  pinned'leri öne alır).
- **Satır 2+** = `Message` kayıtları (role, text, toolCalls, reasoningContent,
  steps, createdAt) — kronolojik. **Asistan turu zenginleştirmesi (2026-06-26):**
  `model` (turu cevaplayan gerçek model), `stopReason` (`end_turn|max_tokens|
  refusal|…` — kesilme/red UI uyarısı), `usage` (`{in,out,cacheRead,cacheWrite}`
  — per-balon maliyet; transkript kendi kendine yeter), `durationMs` (tur süresi),
  `cancelled` (kullanıcı durdurması — crash `interrupted`'tan ayrı), `feedback`
  (`{rating:±1,note,at}` — 👍/👎 kalıcı kalite sinyali, reflektör/eval için).
  Hepsi `omitempty` (eski mesajlar + user/system turları boş).

`AddMessage` mesajı belleğe ekler, oturum sayacını artırır ve **yalnızca yeni
satırı dosyaya ekler** (`O_APPEND`, O(1)) — tüm dosyayı yeniden yazmaz. Eski
davranış her mesajda dosyanın tamamını yeniden yazıyordu (mesaj başına O(n),
oturum başına O(n²)); append-only ile bu O(1)'e indi. Header satırındaki
`messageCount`/`updatedAt` bu yüzden diskte **bayat** kalabilir; bu sayaçlar
boot'ta mesaj satırlarından **yeniden hesaplanır** ve bir sonraki tam yeniden
yazımda (başlık/özet değişimi) tazelenir. Header'ı değiştiren işlemler
(`SetSessionTitle`, `SetSessionSummary`, oturum oluşturma) hâlâ atomik tam
yeniden yazım yapar.

## Tur-içi crash kurtarma (inflight sidecar)

Asistan yanıtı `session.jsonl`'e yalnızca **stream tamamen bitince** eklenir
(O(1) append). Süreç tam o anda ölürse (dev rebuild, OOM, elektrik kesintisi)
stream'lenmiş ama henüz persist edilmemiş yanıt kaybolurdu — yenilemede tur
"buharlaşmış" görünürdü. Bunu önlemek için her stream'lenen tur, oturum dizininde
**`inflight.json`** adlı bir sidecar'a throttle'lı (≤ ~600ms'de bir) **atomik**
(`*.tmp`→`rename`) anlık görüntü yazar: kısmi cevap metni (stream delta'larından
biriktirilir) + o ana kadarki kalıcı iz (`TurnStep[]`).

- **Yazma yolu hot path'i bozmaz:** ayrı dosya, store kilidi gerektirmez (`db.WriteInflight`).
- **Normal her çıkışta silinir** (`db.ClearInflight`): başarı, ele alınan hata,
  istemci kopması — hepsi sidecar'ı kaldırır (`chat_stream.go`'da tur başına
  `defer` + yanıt persist edilince anında). Yalnızca **gerçek süreç ölümü**
  sidecar'ı geride bırakır.
- **Boot'ta kurtarma** (`db.recoverInflight`, `loadSessions`'tan sonra): orphan
  bir `inflight.json` varsa → mesaj zaten persist edilmişse (append ile clear
  arasındaki minik pencerede çökme) dosya düşürülür; değilse kısmi yanıt
  `Interrupted=true` asistan mesajı olarak `session.jsonl`'e eklenir. **Idempotent**:
  sidecar'ın `MessageID`'si nihai mesajla paylaşılır (`AddMessage` boş olmayan
  ID'yi korur), böylece tekrar boot'larda kopya oluşmaz.
- Frontend `Message.interrupted` → asistan balonunda "Bu yanıt yarıda kesildi
  (sunucu yeniden başladı)" uyarı banner'ı.
- Testler: `db/inflight_test.go` (materialize + idempotent + already-persisted skip).

### external-agent-oss ile karşılaştırma (ilham kaynağı)

`external-agent-oss` (Electron + Pi/Claude Agent SDK; runtime sunucu, renderer ince
istemci) aynı sorunu **çok-katmanlı** çözer. İlginç olan, aynı **`session.jsonl`
(header + satırlar) + atomik `tmp→rename`** desenini kullanmasıdır:

| Konu | external-agent-oss | SwarmGo |
|------|------------------|---------|
| Artımlı persist | Her olay sınırında (`text_complete`/`tool_*`/`error`) **tüm oturumu** debounce'lı (500ms) yeniden yazar (`SessionPersistenceQueue`) | Final mesaj O(1) append; tur-içi durum ayrı **sidecar**'a snapshot |
| Stream'lenen kısmi metin | ❌ Yalnız bellekte (`streamingText`), `text_complete`'e dek diske yazılmaz | ✅ `delta`'lar sidecar'a birikir (biraz daha granüler) |
| Kurtarma yeri | Ağırlıklı **istemci** (reconnect replay + stale-watchdog + sunucudan tazele) | **Sunucu boot** (`recoverInflight`) |
| Kullanıcı mesajı | ack öncesi senkron `flushSession` (regression eb81086e) | Stream öncesi senkron append (bu açık SwarmGo'da hiç yoktu) |

Neden farklı: external-agent sunucusu oturumları RAM'de tutar → asıl risk istemci↔sunucu
desenkronu; SwarmGo tek binary → asıl risk sürecin tamamen ölmesi (boot recovery mantıklı).

### Gelecek iş (external-agent'tan devşirilebilecek, henüz YOK)

1. **Stale-session watchdog** — backend ölmeden tek bir SSE olayı düşerse frontend
   "düşünüyor…"da takılabilir. external-agent'taki `useStaleSessionRecovery` gibi
   periyodik "X sn'dir olay yok → sunucudan tazele" güvenlik ağı SwarmGo'da yok.
2. **`preserved_stale_messages` kuralı** — oturum yeniden yüklenirken sunucu listesi
   istemcidekinden kısa olsa bile istemcideki mesajları **silmeme** garantisi.

Boot'ta `loadSessions` her `session.jsonl`'i okuyup header + mesaj satırlarını
ayrıştırır. Append modeli gereği bir çökme **yarım bir son satır** bırakabilir;
`readSessionFile` yalnızca **son** satır ayrıştırılamazsa onu sessizce atar
(daha önceki bir satırdaki bozulma ise ölümcül hatadır). JSON encoder
`SetEscapeHTML(false)` ile yazar; UTF-8 (Türkçe dahil) ve HTML içerik bozulmadan
saklanır.

## Önemli detaylar

- **Zaman damgaları:** unix epoch **saniye** (`int64`), değişmedi.
- **Birincil anahtarlar:** UUID (`google/uuid`), değişmedi.
- **Gömülü JSON alanları:** `dream_config`, `tool_calls`,
  `graph`, `state`, `env_config` vb. yine TEXT-içinde-JSON string olarak tutulur
  (model değişmedi).
- **Memory embedding:** `KnowledgeSource.Embedding []byte` bellek modelinde
  `json:"-"`; diske `knowledgeDisk` sarmalayıcısıyla base64 olarak yazılır, böylece
  cosine vektör cache'i restart'ta korunur (yoksa `memory.Recall` içerikten yeniden
  hesaplar — geriye dönük güvenli).
- **Usage:** gün-bazlı dosya (`{agentID}__{gün}.json`), read-modify-write upsert.
- **Cascade silme:**
  - `DeleteTask` → task'ın run'ları; `DeleteFlow` → flow'un `flow_runs`'ı.
  - `DeleteSession` → oturum mesajları + oturuma ait artifact kayıtları + upload
    klasörü (`workspace/artifacts/<sid>/`).
  - `DeleteAgent` → ajanın sahip olduğu **session'lar** (yukarıdaki kaskadla),
    ajana bağlı **schedule'lar** (`AgentID`), ajanın sahip olduğu **task'lar**
    (`OwnerAgentID`) + bu task'ların **run'ları**. Hook/Flow'da doğrudan `AgentID`
    alanı olmadığından (flow ajanları graph içinde referanslanır) bunlar silinmez.
    DB yalnızca kalıcı satırları temizler; canlı cron registry'si için çağıran
    katman (`api.handleDeleteAgent` / `delete_agent` tool'u) ayrıca
    `Scheduler.Reload` çağırır.
  - `RemoveSkillFromAgents(slug)` → bir skill silindiğinde slug'ı her ajanın
    `Skills` listesinden düşürür (dangling skill referansı kalmaz);
    `api.handleDeleteSkill` ve `delete_skill` tool'u çağırır.
- **Restart-safe flow:** `flow_runs` durumu her node sonrası dosyaya yazılır;
  boot'ta `ListRunningFlowRuns` `running` kalanları diskten devam ettirir.

## Eski veri (SQLite)

Otomatik migration **yok** — kullanıcı talebiyle SQLite tamamen kaldırıldı.
Eski `swarmgo.db` dosyaları okunmaz; yeni kurulum `store/` dizininde sıfırdan
başlar. Gerekirse eski DB'den dışa aktarım ayrı bir tek-seferlik script ile
yapılabilir.

## Test

- `internal/db/filestore_test.go` — round-trip: agent/session/mesaj/task/usage/
  knowledge oluştur → close → reopen → diskten reload + sayaç + UTF-8 + HTML +
  embedding + cascade-delete doğrulaması.
- Canlı API testi: entity oluşturma → doğru disk yapısı → **restart sonrası tüm
  entity'lerin diskten reload'u** + Türkçe başlık round-trip'i doğrulandı.
