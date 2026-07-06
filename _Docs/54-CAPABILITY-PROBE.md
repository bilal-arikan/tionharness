# 54 — Capability Probe → Context Genişletme (+ per-workspace codebase-memory store)

> Generic bir katman: cihazda/işlemde bir **opsiyonel harici yetenek** mevcutsa,
> ajanın **cachelenebilir statik** sistem-prompt prefix'ine kısa bir bilgi bloğu
> enjekte edilir. İlk müşteri: `codebase-memory-mcp` (kod bilgi-grafiği).

## Amaç

- Ajan, workspace'te hangi opsiyonel araçların **var olduğunu** promptundan bilsin
  ve onları kullansın (grep yerine kod bilgi-grafiği gibi).
- Tespit + genişletme **generic** olsun: yeni bir tool eklemek = tek bir `Capability`
  kaydı. Assembler kodu değişmez.
- codebase-memory için: her workspace **izole** bir indeks store kullansın (store =
  workspace sınırı), cwd repo'su ilk kullanımda otomatik indekslensin.

## Bileşenler

### 1. Generic katman — `internal/agent/capabilities.go`

```go
type Capability struct {
    ID      string
    Detect  func(ctx, r *Runtime) bool                 // ucuz probe
    Context func(ctx, r *Runtime, cwd string) string   // enjekte edilecek blok
}
var capabilities = []Capability{ codebaseMemoryCapability }
func (r *Runtime) CapabilityContext(ctx, cwd) string   // mevcut olanların bloklarını birleştirir
```

- `Detect` **herhangi bir sinyal** olabilir: MCP-sunucu varlığı, `exec.LookPath`
  (on-PATH binary), settings-flag. Yeni tool = slice'a bir `Capability`.
- `CapabilityContext` "" dönerse assembler için güvenli no-op. Sonuç sabit →
  prompt-cache bozulmaz.

### 2. codebase-memory capability

- **Tespit:** `r.db.ListEnabledMCPServers` içinde `Transport=stdio` + `Command`'da
  `codebase-memory-mcp` işareti (`codebaseMemoryCommand`). MCP modelinde slug yok →
  Command üzerinden.
- **Blok:** kısa; araç-tercihi yönlendirmesi (`search_code`/`search_graph`/
  `get_code_snippet`/`query_graph`/`trace_path`/`get_architecture`) + izole store notu
  + cwd'den türetilen `project` id.
- **`projectIDForPath`:** path→id kuralını codebase-memory ile birebir taklit eder
  (ayraç+`:` → `-`, `[A-Za-z0-9-]` dışı düşer). İki gerçek örnekle test edildi; ıskalarsa
  blok ajanı `list_projects`'e yönlendirir (hedge).

### 3. Enjeksiyon — iki senkron assembler (mevcut desen)

| Yol | Yer | İçerik |
|-----|-----|--------|
| Chat | `api.composeTurnRequest` (statik `system` prefix) | cwd/project id'li tam blok |
| Headless | `agent.autonomousSystemPrompt` | cwd-siz farkındalık (assembler per-agent) |

`GoalUsageHint` / `EnvironmentContextBlock` ile aynı "tek-kaynak paylaşılan blok"
desenini izler. Statik prefix'te çünkü **varlık oturum boyunca sabit** →
cache-dostu.

### 4. Per-workspace store (§C)

- **`(*Runtime).CBMStoreDir()`** = `filepath.Join(filepath.Dir(workDir), "cbm-store")`
  — `skills/`, `hook-scripts/` ile aynı kardeş-dizin deseni. `workDir` boşsa "" →
  sunucunun default store'una düşülür.
- **`toolsetup.go`:** `cfgs` kurulurken codebase-memory sunucusunun stdio env'ine
  `CBM_CACHE_DIR = CBMStoreDir()` enjekte edilir (kullanıcı elle set etmişse o kazanır).
  `mcp.ServerConfig.Env` → `DialStdio` env'i → sunucu yalnız bu store'u görür.

### 5. Auto-index (§C)

- **`(*Runtime).EnsureCodebaseIndexed(ctx, cwd)`** — best-effort, arka planda
  `command cli index_repository {repo_path}` + `CBM_CACHE_DIR`. `(cwd,store)` başına
  **süreç-içi tek sefer** (`cbmIndexed sync.Map`). Chat turunda `session.WorkingDir`
  ile tetiklenir.
- **Hata yutulmaz:** başarısızlık `logger.Warn` ile loglanır ve guard silinir →
  sonraki tur retry edebilir.

## Cache güvenliği

- Bloklar statik prefix'te ama **sunucu yokken "" **→ mevcut promptlar hiç değişmez
  (cachebreak testleri yeşil kalır).
- Presence + cwd oturum içinde stabil → blok string'i sabit → prompt-cache READ olarak
  kalır. cwd nadiren değişirse cache bir kez tazelenir (kabul edilebilir).

## Genişletme (ileride)

Yeni bir harici tool için `capabilities.go`'ya tek bir `Capability` eklemek yeter;
`Detect` `exec.LookPath("...")` veya settings-flag olabilir. Assembler/enjeksiyon
noktaları değişmez.

## Dosyalar

- `internal/agent/capabilities.go` (yeni) · `capabilities_test.go` (yeni)
- `internal/agent/runtime.go` (`cbmIndexed` alanı + headless enjeksiyon)
- `internal/agent/toolsetup.go` (`CBM_CACHE_DIR` env enjeksiyonu)
- `internal/api/chat_turn.go` (chat enjeksiyon + `EnsureCodebaseIndexed`)

## Sıradaki adımlar (opsiyonel)

- Headless yola da cwd/project id (ctx'ten session çözerek `autonomousSystemPrompt`'a
  cwd threading).
- `search_code` project-scoped → workspace-geneli metin araması için fan-out köprü tool.
- Mevcut default-store'daki eski TionSwarm kopyasının temizliği (çok-workspace geçişte).
