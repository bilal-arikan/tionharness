# SwarmGo — Depolama Katmanı (Dosya Sistemi)

> Son güncelleme: **2026-06-15**
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
├── tasks/{id}.json
├── runs/{id}.json
├── schedules/{id}.json
├── knowledge/{id}.json              # embedding base64 olarak gömülü
├── mcp-servers/{id}.json
├── flows/{id}.json
├── flow-runs/{id}.json
└── usage/{agentID}__{YYYY-MM-DD}.json
```

> Eski yol `{wsID}/swarmgo.db` idi; artık `{wsID}/store/` dizini.

## Oturum biçimi (JSONL — Craft tarzı)

`session.jsonl` her oturum için tek dosya:

- **Satır 1** = `Session` header'ı (id, agentId, kind, title, messageCount, state,
  summary, summaryMsgCount, zaman damgaları).
- **Satır 2+** = `Message` kayıtları (role, text, toolCalls, reasoningContent,
  steps, createdAt) — kronolojik.

`AddMessage` mesajı belleğe ekler, oturum sayacını artırır ve dosyayı yeniden
yazar (atomik). Boot'ta `loadSessions` her `session.jsonl`'i okuyup header + mesaj
satırlarını ayrıştırır. JSON encoder `SetEscapeHTML(false)` ile yazar; UTF-8
(Türkçe dahil) ve HTML içerik bozulmadan saklanır.

## Önemli detaylar

- **Zaman damgaları:** unix epoch **saniye** (`int64`), değişmedi.
- **Birincil anahtarlar:** UUID (`google/uuid`), değişmedi.
- **Gömülü JSON alanları:** `capabilities`, `dream_config`, `tool_calls`,
  `graph`, `state`, `env_config` vb. yine TEXT-içinde-JSON string olarak tutulur
  (model değişmedi).
- **Memory embedding:** `KnowledgeSource.Embedding []byte` bellek modelinde
  `json:"-"`; diske `knowledgeDisk` sarmalayıcısıyla base64 olarak yazılır, böylece
  cosine vektör cache'i restart'ta korunur (yoksa `memory.Recall` içerikten yeniden
  hesaplar — geriye dönük güvenli).
- **Usage:** gün-bazlı dosya (`{agentID}__{gün}.json`), read-modify-write upsert.
- **Cascade silme:** `DeleteTask`/`DeleteFlow` ilgili run dosyalarını da siler.
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
