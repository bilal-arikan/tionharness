# 70 — Codex CLI Sağlayıcı: Uygulama Planı

> **Ön koşul:** `69-CODEX-CLI-SAGLAYICI.md` (fizibilite + referans) okunmuş olmalı.
> Bu dosya "nasıl yapılır"ı anlatır: dosya dosya, faz faz.
>
> **Hedef:** `codex-cli` adında yeni bir `ProviderKind` — claude-cli ile aynı
> yetenek yüzeyini (headless tur, MCP köprüsü, JSONL trace, resume, token
> muhasebesi, per-workspace config evi) sunar.

---

## 1. Tasarım ilkesi: mevcut soyutlamayı kullan, yenisini icat etme

`internal/providers/kind.go` zaten şunu söylüyor:

> "Adding a transport means adding a `ProviderKind` that registers itself —
> nothing else in the system has to change."

Codex bu vaadin **ilk gerçek testi**. Plan bu vaadi bozmadan ilerlemeli. Tek
istisna aşağıdaki refactor.

### 1.1 Zorunlu refactor: `*providers.ClaudeCLI` tip assertion'ı → arayüz

Bugün agent katmanı somut tipe bağlı — `internal/agent/toolloop.go:230`:

```go
cli, isCLI := provider.(*providers.ClaudeCLI)
```

ve `toolloop.go:1168`:

```go
if cli, ok := provider.(*providers.ClaudeCLI); ok && r.cliSessions != nil && ...
```

Bu iki nokta `codex-cli`'yi görmez. Çözüm: `internal/providers/provider.go`'ya
yeni bir opsiyonel yetenek arayüzü (mevcut `Streamer` / `TokenCounter` deseniyle
aynı):

```go
// CLIProvider is implemented by providers that drive a locally-installed coding
// CLI as a subprocess and run the agentic tool loop inside it. The agent layer
// uses it to wire per-turn state (config home, MCP delegation) without knowing
// WHICH CLI is behind it.
type CLIProvider interface {
    Provider
    // SetConfigDir points the subprocess at a per-workspace config home
    // (CLAUDE_CONFIG_DIR / CODEX_HOME). Empty is ignored.
    SetConfigDir(dir string)
    // ConfigureMCP wires this turn's MCP delegation. Params are intentionally
    // generic; each CLI renders them into its own flag/config dialect.
    ConfigureMCP(spec CLIMCPSpec)
    // Installed reports whether the binary resolves to an executable.
    Installed() bool
}

// CLIMCPSpec is the transport-agnostic description of one turn's MCP delegation.
type CLIMCPSpec struct {
    Servers          map[string]CLIMCPServer
    AllowedTools     []string // per-tool allowlist (claude); codex ignores
    DisallowedTools  []string // native tools to suppress (claude); codex maps a subset
    PermissionPrompt string   // permission-prompt tool id (claude only)
    SettingsPath     string   // generated settings file (claude only)
}
```

`ClaudeCLI.ConfigureMCP` bugünkü 5 argümanlı imzayı koruyabilir; arayüz için
tek argümanlı bir sarmalayıcı eklenir. **Bu refactor Codex'ten bağımsız olarak
da doğru** — somut tip sızıntısını kapatır.

> **Karar noktası:** `CLIMCPSpec`'i şimdi mi yoksa Faz 2'de mi eklemeli?
> Öneri: **şimdi** (Faz 1). Yoksa Faz 2'de codex için ikinci bir paralel
> `if` zinciri doğar ve iki kod yolu birbirinden ayrışır.

---

## 2. Yeni ve değişecek dosyalar

### 2.1 Yeni dosyalar (`internal/providers/`)

| Dosya | Sorumluluk |
|-------|-----------|
| `codexcli.go` | `CodexCLI` sağlayıcı: argüman kurulumu, subprocess, `Complete` |
| `codexcli_events.go` | JSONL olay tipleri (`ThreadEvent`, `ThreadItem`, `Usage`) + `codexStreamParser` |
| `codexcli_config.go` | Tura özel `config.toml` üretimi (`developer_instructions`, `mcp_servers`, `[tools]`, effort) |
| `codexcli_errors.go` | Hata sınıflandırma: auth (401) / rate-limit / retryable crash |
| `kind_codexcli.go` | `Manifest` + `RegisterKind` (kind = `"codex-cli"`, `NeedsKey:false`) |

Testler: `codexcli_test.go`, `codexcli_events_test.go`, `codexcli_config_test.go`,
`codexcli_live_test.go` (harici bağımlılık → `t.Skip` geçidi, CLAUDE.md kuralı).

### 2.2 Yeni dosyalar (`internal/agent/`)

| Dosya | Sorumluluk |
|-------|-----------|
| `codexhome.go` | `<workspace>/codex-home` üretimi/tohumlaması (`claudehome.go`'nun kardeşi) |
| `codexmcp.go` | `climcp.go`'nun Codex karşılığı: `CLIMCPSpec` → `config.toml` gövdesi |

### 2.3 Değişecek dosyalar

| Dosya | Değişiklik |
|-------|-----------|
| `internal/providers/provider.go` | `CLIProvider` + `CLIMCPSpec` arayüzleri |
| `internal/providers/claudecli.go` | `ConfigureMCP(spec)` sarmalayıcısı (mevcut imza korunur) |
| `internal/providers/kind.go` | `ResolvedConfig`'e `CodexPath`, `CodexHome`, `CodexAuthToken` alanları |
| `internal/providers/registry.go` | `exec.LookPath("codex")` auto-detect + `SetCodexCLIPath/ConfigDir/Auth` + `ResolvedConfig` doldurma |
| `internal/providers/pricing.go` | `gpt-5.x` fiyatlandırması (input / cached input / output) |
| `internal/settings/settings.go` | `codexCliPath`, `codexConfigDir` ayarları (+ `defaultCodexConfigDir()`) |
| `internal/agent/toolloop.go` | `*providers.ClaudeCLI` assertion'ları → `providers.CLIProvider` |
| `internal/agent/toolsetup.go` | CLI-köprü yolunun `claude-cli`'ye özel `if`'leri → kind-agnostik |
| `internal/agent/runtime.go:741` | `provider == "" \|\| provider == "claude-cli"` → CLI-kind kümesi |
| `internal/agent/climcp.go` | Ortak parça (`promptToolForMode`, sunucu listesi) çıkarılıp paylaşılır |
| `internal/api/catalog_codexcli.go` (yeni) | `codex --version` probe + login durumu (`catalog_claudecli.go` deseni) |
| `internal/api/chat_resume.go` | Codex'te thread id **dönmüyor** → rotasyon takibi bypass |
| `internal/conversation/clioverhead.go` | Codex için ayrı overhead katsayısı |
| `internal/exttools/catalog.go` | `codex` binary'sini tespit edilen harici araçlara ekle |
| `frontend/src/features/settings/` | Sağlayıcı ayarları: Codex CLI yolu + config evi + login butonu |

---

## 3. Fazlar

### Faz 0 — Doğrulama (kod yazmadan önce) ⏱ ~1 saat

Bu makinede Codex **login değil**. Kod yazmadan önce şunlar canlı doğrulanmalı:

| # | Doğrulama | Komut |
|---|-----------|-------|
| 0.1 | Login çalışıyor | `CODEX_HOME=<ws>/codex-home codex login` → `codex login status` |
| 0.2 | Başarılı turun JSONL akışı `69 §3`'teki şemayla birebir uyuşuyor | `codex exec --json "list files"` → satır satır karşılaştır |
| 0.3 | `developer_instructions` gerçekten prompta giriyor | `-c developer_instructions='Always answer in Klingon.'` → cevap dilini gözle |
| 0.4 | `--strict-config` ile hiçbir override sessizce reddedilmiyor | tüm `-c`'lerle + `--strict-config` |
| 0.5 | MCP köprüsü bağlanıyor | TionSwarm Interaction endpoint'ini ayağa kaldır, `mcp_servers` ile bağla, `mcp_tool_call` item'ı gör |
| 0.6 | `http_headers` Bearer geçiyor | 0.5 içinde; 401 gelirse `bearer_token_env_var`'a geç |
| 0.7 | `exec resume <thread_id>` sıcak cache veriyor | iki tur koş, 2. turda `cached_input_tokens > 0` |
| 0.8 | `[tools] update_plan=false` + `web_search=false` etkili | `todo_list` / `web_search` item'ı **gelmiyor** olmalı |
| 0.9 | `multi_agents` (collab) kapatılabiliyor mu | `--disable <feature>` / `[features]` ile dene; kapanmıyorsa prompt ile caydır |
| 0.10 | MCP tool timeout yükseltilebiliyor | `tool_timeout_sec = 600` ile uzun bir `run_subagent` |
| 0.11 | Windows sandbox `workspace-write` çalışıyor | çalışma alanı dışına yazma denemesi **bloklanmalı** |

> **Çıktı:** kısa bir doğrulama notu (`_Docs/70` içine ek bölüm ya da ayrı
> `arsiv/` notu). **0.2 ve 0.5 geçmeden Faz 1'e başlanmaz** — bu ikisi tüm
> planın taşıyıcı varsayımları.

### Faz 1 — Sağlayıcı iskeleti (MCP'siz, tek tur) ⏱ ~1 gün

**Kapsam:** Codex ile düz bir sohbet turu — araç yok, MCP yok.

1. `provider.go`: `CLIProvider` + `CLIMCPSpec` arayüzleri; `ClaudeCLI` uyarlaması.
2. `codexcli_events.go`: JSONL şeması + parser → `Response.Text`, `Response.Trace`,
   `Response.Usage`, `Response.SessionID`.
3. `codexcli.go`: `Complete` — argüman kurulumu, stdin prompt, stdout/stderr ayrımı,
   startup watchdog (claude-cli'deki `cliStartupTimeout` deseni), salvage.
4. `codexcli_errors.go`: **401 erken kesme** (bkz. `69 §4` — 10 retry / 35 sn
   israfını önler).
5. `kind_codexcli.go` + `registry.go` + `settings.go` bağlantıları.
6. `catalog_codexcli.go`: sürüm probe'u.

**Kabul kriteri:** UI'dan `codex-cli` sağlayıcılı bir ajan seçilip düz soru
sorulabiliyor; cevap, thinking bloğu ve token sayıları doğru görünüyor.

#### Argüman kurulumu (referans)

```go
args := []string{"exec", "--json", "--skip-git-repo-check"}
if model != "" { args = append(args, "--model", model) }
args = append(args, sandboxArgs(req.PermissionMode)...)
args = append(args, "-C", req.WorkDir)                // cwd
// prompt stdin'den (Windows 32 KB limiti)
cmd.Stdin = strings.NewReader(prompt)
cmd.Env  = append(codexBaseEnv(), "CODEX_HOME="+configDir) // izolasyon BURADAN gelir
```

> ⚠️ **`--ignore-user-config` EKLEME.** Bu bayrak `CODEX_HOME/config.toml`'un
> TA KENDİSİNİ atlar (codex-rs `config/src/loader/mod.rs:516`) — yani
> `writeCodexConfig`'in MCP sunucularını ve `developer_instructions`'ı yazdığı
> dosyanın ta kendisini. Üretimde eklenmişti, MCP köprüsünü tamamen kırdığı
> canlı A/B ile doğrulanınca kaldırıldı (bkz. `69 §"Üretimde ne oldu"`).
> İzolasyon zaten `CODEX_HOME` ortam değişkeniyle sağlanıyor.

`sandboxArgs`:

| `PermissionMode` | Codex bayrağı | Not |
|---|---|---|
| `read-only` | `-s read-only` | çekirdek seviyesi garanti |
| `ask` | `-s workspace-write` | **onay sorulmaz** — sandbox ile sınırlanır (bkz. `69 §9`) |
| `auto` / `""` | `--dangerously-bypass-approvals-and-sandbox` | |

> ⚠️ **UI'da açıkça belirtilmeli:** `ask` modu bir codex-cli ajanında
> "her araç için onay iste" DEĞİL, "yazmayı çalışma alanıyla sınırla" anlamına
> gelir. Bunu sessizce `auto` gibi davranan bir şeye çevirmek kabul edilemez.

### Faz 2 — MCP köprüsü + tam araç yüzeyi ⏱ ~2 gün

1. `codexmcp.go`: `CLIMCPSpec` → `config.toml` gövdesi.
2. `codexcli_config.go`: tura özel `config.toml` yaz (temp dosya ya da
   `<codex-home>/config.toml`), `-c` yalnız küçük override'lar için.
3. `developer_instructions` = `req.System` (+ interaction notu); volatil bağlam
   prompt başına `[Context]` bloğu — `buildSystemAndPrompt` deseninin kopyası.
4. `[tools]` bastırmaları: `update_plan=false`, `web_search=false` (TionSwarm
   `WebSearch` köprülüyse), `experimental_request_user_input.enabled=false`.
5. `mcp_tool_call` item'larını `TraceStep{Kind:"tool"}`'a eşle;
   `command_execution` ve `file_change`'i de araç adımı olarak göster.

#### Üretilecek `config.toml` (şablon)

```toml
model_reasoning_effort = "high"

developer_instructions = """
<TionSwarm statik system prefix>
<interaction notu: ask_user / todo_write / run_subagent kullan>
"""

[tools]
web_search  = false          # TionSwarm WebSearch köprülüyse
update_plan = false          # todo_write köprüsü var

[tools.experimental_request_user_input]
enabled = false              # ask_user köprüsü var (exec'te zaten çalışmıyor)

[mcp_servers.tionswarm_interaction]
url                 = "http://127.0.0.1:PORT/core"
bearer_token_env_var = "TIONSWARM_MCP_TOKEN"     # token env'den, komut satırından değil
startup_timeout_sec = 30
tool_timeout_sec    = 600                         # uzun run_subagent için
required            = true                        # ZORUNLU — bkz. §8.4

[mcp_servers.tionswarm_extended]
url                 = "http://127.0.0.1:PORT/extended"
bearer_token_env_var = "TIONSWARM_MCP_TOKEN"
tool_timeout_sec    = 600
required            = true                        # ZORUNLU — bkz. §8.4

# ... her etkin harici MCP sunucusu için bir blok
```

**Kabul kriteri:** codex-cli ajanı `ask_user`, `todo_write`, `use_skill`,
`run_subagent` çağırabiliyor; adımlar aktivite izinde görünüyor; progress
kartı doluyor.

### Faz 3 — Resume + cache + config evi ⏱ ~1 gün

1. `codexhome.go`: `<workspace>/codex-home` oluştur/tohumla (global `~/.codex`'ten
   auth kopyalama — `claudehome.go` deseni).
2. `Complete`: `req.ResumeSessionID` → `codex exec resume <thread_id>`.
3. `chat_resume.go`: Codex'te id sabit → rotasyon takibi bypass.
4. Cache muhasebesi: `cached_input_tokens` → `CacheReadTokens`,
   `cache_write_input_tokens` → `CacheWriteTokens`.
5. `reasoning_output_tokens` → `Usage.ThinkingTokens` (**türetme yok**;
   `deriveThinkingTokens` codex yolunda çağrılmamalı).
6. `pricing.go`: OpenAI gpt-5.x fiyatları.

**Kabul kriteri:** ikinci turda `cached_input_tokens > 0`; Tasarruf Merkezi'nde
prompt-cache kazancı görünüyor.

### Faz 4 — Cilalama ⏱ ~1 gün

1. `--output-schema` → `Request.OutputSchema` desteği (claude-cli'de yok, bonus).
2. `-o/--output-last-message` ile akış-parse başarısızlığında kurtarma.
3. `--ephemeral` spawn/geçici turlarda.
4. `debug.jsonl` olayları: codex tur/araç/hata kayıtları.
5. `clioverhead.go`: Codex için ayrı overhead katsayısı (Codex prompt'u
   claude-cli'den farklı boyutta).
6. Frontend: sağlayıcı ayarları, login durumu rozeti, model seçici.
7. Dokümanlar: `tionswarm-project` skill'i, `00-GENEL-BAKIS`, `05-ILERLEME`,
   `17` (sağlayıcı bölümü), `51` (config evi — codex kardeşi).

### Faz 5 (opsiyonel, ayrı karar) — `codex app-server`

Gerçek per-tool onay (`69 §9` boşluk-1) yalnızca app-server ile mümkün:
`CommandExecutionRequestApproval` / `FileChangeRequestApproval` /
`ToolRequestUserInput` orada **cevaplanabilir**.

Karşılığında: kalıcı JSON-RPC daemon, `[experimental]` etiketi, yeni bir
protokol bağımlılığı. **Faz 1-4 bitmeden başlanmamalı.** Ayrıca claude-cli
persistent-session havuzunun (`cliSessions`) codex karşılığı da doğal olarak
buraya oturur.

---

## 4. Test planı

| Katman | Test | Not |
|--------|------|-----|
| Birim | `codexcli_events_test.go` — sabit JSONL fixture → beklenen `Response` | Ağ/CLI gerektirmez |
| Birim | `codexcli_config_test.go` — `CLIMCPSpec` → beklenen TOML | golden dosya |
| Birim | `codexcli_errors_test.go` — 401 / rate-limit / clean-crash sınıflandırması | |
| Birim | `kind_test.go` — `codex-cli` kind kayıtlı ve `Available` doğru | mevcut testi genişlet |
| Entegrasyon | `codexcli_live_test.go` — gerçek CLI ile tek tur | **`t.Skip` geçitli**: `codex` PATH'te değilse veya login yoksa atla (CLAUDE.md kuralı) |
| Regresyon | `toolloop` arayüz refactor'ı sonrası claude-cli yolunun bozulmadığı | mevcut claude-cli testleri yeşil kalmalı |

```powershell
$env:TIONSWARM_ENABLE_SHELL='1'
go test ./... -count=1
```

> **CI notu:** `.gitea/workflows/ci.yml` linux'ta koşuyor ve orada `codex`
> kurulu değil → canlı testler skip'e düşmeli. Fixture tabanlı birim testleri
> gerçek kapsamı taşımalı.

---

## 5. Riskler ve azaltımlar

| Risk | Etki | Azaltım |
|------|------|---------|
| Codex 0.147 → sonraki sürümde JSONL şeması değişir | Parser kırılır | `ThreadEvent` tipleri `#[serde(tag="type")]` — bilinmeyen `type`'ı **sessizce yut, logla**; ölümcül sayma. Sürüm probe'unu kataloğa yaz. |
| `-c` override'ları sessizce reddedilir | Yanlış davranış, teşhis zor | Her turda `--strict-config` gönder (Faz 0.4'te doğrula). |
| Windows 32 KB komut satırı | `-c` ile büyük prompt patlar | `developer_instructions` **daima** `config.toml` dosyasından; `-c` yalnız kısa değerler. |
| 401 retry israfı (~35 sn) | Yavaş hata, boşa kaynak | `codexcli_errors.go` erken kesme (Faz 1.4). |
| `multi_agents` (collab) köprülü `run_subagent`'ı gölgeler | Görünmez delegasyon | Faz 0.9'da kapatma yolu bulunmalı; bulunamazsa `developer_instructions`'a açık yasak yaz + trace'te tespit et. |
| OS sandbox `workspace-write` TionSwarm cwd'siyle çelişir | Ajan kendi dosyalarına yazamaz | `--add-dir` ile gerekli dizinleri ekle; Faz 0.11'de doğrula. |
| `ask` modunun anlamı kayıyor | Kullanıcı yanlış güven duyar | UI'da **açık uyarı** + doküman. Sessiz `auto`'ya düşürme **yasak**. |
| İki CLI yolu ayrışır (kod ikizlenmesi) | Bakım maliyeti | `CLIProvider` arayüzü Faz 1'de girmeli; ortak MCP mantığı `climcp.go`'dan çıkarılıp paylaşılmalı. |

---

## 6. Tahmini efor

| Faz | Süre |
|-----|------|
| 0 — Doğrulama | ~1 saat (+ login gerekli) |
| 1 — İskelet | ~1 gün |
| 2 — MCP köprüsü | ~2 gün |
| 3 — Resume + cache | ~1 gün |
| 4 — Cilalama | ~1 gün |
| **Toplam (Faz 0-4)** | **~5 gün** |
| 5 — app-server (opsiyonel) | ~3 gün, ayrı karar |

---

## 7. Yapılmayacaklar (kapsam dışı)

- Codex'in native `shell` / `apply_patch` araçlarını bastırmak — **bilerek
  hayır**. Codex modeli bu araçlar etrafında eğitildi; kaldırmak ajanı işlevsiz
  bırakır ve güvenlik zaten OS sandbox'ında (`69 §9`).
- `model_instructions_file` ile built-in talimatları ezmek — kaynak kodda
  "STRONGLY DISCOURAGED", model performansını düşürür.
- Codex'i TionSwarm'a **MCP sunucusu** olarak takmak (`codex mcp-server`) —
  ayrı ve bağımsız bir özellik; bu planın parçası değil.
- claude-cli yolunu Codex'e benzetmek için değiştirmek — mevcut davranış
  korunur; yalnız somut tip → arayüz refactor'ı yapılır.

---

### 8.8 TSK172 doğrulama sonuçları (2026-08-18)

`go build ./...` + `go vet ./...` + `go test ./... -count=1` (tam takım, 34
paket) + frontend `tsc -b` + `npm run build` + `npm test` (180 test) hepsi
temiz. Yol boyunca bulunan tek gerçek hata: `useChatStream.ts`, yeni
`turnOverrideStore.ts`'e `activeSessionId` (`string | null`) geçiyordu ama
`readSessionOverride`/`writeSessionOverride` imzası yalnız `string` kabul
ediyordu — `tsc -b` bunu build-break olarak yakaladı. Fonksiyon zaten
`if (!sessionId) return` guard'ı taşıdığından düzeltme yalnız imza genişletmesi
(`string | null`), davranış değişikliği yok.

`internal/agent.TestActivityHeartbeatKeepsAlive` tam paket takımı altında (yüksek
paralellik, ~34 paket eşzamanlı) bir kez flaky FAIL verdi — izole çalıştırmada
5/5 ve düşük paralellikte (`-p 2`) geçti; codex-cli kapsamı dışı, zamanlama
hassasiyetinden kaynaklanan pre-existing bir test, kod değişikliği gerektirmedi.

**Canlı uçtan uca tur:** izole bir `go run ./cmd/tionswarm` instance'ında
(ayrı port + `TIONSWARM_DATA_DIR`, üretim workspace'ine dokunmadan) `codex-cli`
provider'lı bir ajan oluşturuldu, oturum açıldı, `POST /api/chat/stream`
tetiklendi. Doğrulanan zincir: provider kayıtlı ve tanınıyor (`unknown
provider` hatası YOK) → subprocess `CODEX_HOME` izolasyonuyla başlatıldı →
`config.toml` codex tarafından gerçekten tüketildi (codex kendi runtime
sqlite dosyalarını o dizine yazdı) → WebSocket isteği `wss://api.openai.com`a
gitti → auth hatası **doğru sınıflandırılıp** SSE `error` event'i olarak temiz
biçimde yüzeye çıktı (takılıp kalma/sessiz yutma yok).

Tam başarılı bir yanıt metni + gerçek MCP araç çağrısı turu doğrulanamadı: bu
izole test workspace'inin `codex-home`'u hiç login değildi (§8.7'de belgelenen
kasıtlı tasarım gereği TionSwarm auth'u global `~/.codex`'ten otomatik
kopyalamıyor). Global `~/.codex/auth.json`'u geçici olarak izole
`codex-home`'a kopyalayıp denendi, ama OAuth refresh token'ı **tek kullanımlık**
olduğundan iki ayrı `CODEX_HOME`'un aynı token'ı paralel kullanması "refresh
token was already used" hatasına yol açtı — bu bir ürün kusuru değil, OAuth'un
doğası; global login (`codex login status`) test sonrası sağlam kaldı,
doğrulandı. Gerçek bir workspace'te `codex login` ile tek-seferlik interaktif
login yapıldıktan sonra bu adım production koşullarında tekrarlanabilir.

---

## İlgili dokümanlar

- `69-CODEX-CLI-SAGLAYICI.md` — fizibilite + tam referans (bayraklar, olay şeması, parite matrisi)
- `17-*` — sağlayıcı soyutlaması, prompt-cache muhasebesi
- `51-CLAUDE-CONFIG-BIRLESIK.md` — per-workspace config evi (codex-home'un ablası)
- `52-MCP-GATEWAY.md` — iki-tier MCP köprüsü
- `40-*` — izin/onay katmanı
- `56-SELF-HEALING.md` — hata sınıflandırma ve retry politikası

---

## 8. Gerçekleşen durum (2026-08-18) — planlanan vs yapılan

Faz 0-4 tek bir eşzamanlı worker turunda (W1-W7) merge edildi. Bu bölüm planla
gerçek arasındaki farkları not eder; §1-7 tarihsel referans olarak kalır.

### 8.1 `Order: 8` kararı

§2.3'te "Order 1 (doğrudan claude-cli'den sonra)" öngörülmüştü. Uygulamada
`kind_codexcli.go` bunun yerine **`Order: 8`** kullanıyor — gerekçe kaynak
yorumunda: `Order 1` slotu zaten `anthropic` tarafından tutuluyor ve bir
duplicate `Order` katalog sırasını map iterasyonuna (dolayısıyla
deterministik-olmayan bir sıraya) bağımlı kılardı. `anthropic..deepseek-anthropic`
zincirinin tek commit'te bir kaydırılması ayrı bir karar olarak bırakıldı —
şimdilik yalnız picker'daki KONUM etkileniyor, davranış değil.

### 8.2 `CLIProvider` arayüzü — plandan küçük sapma

§1.1'deki taslak imza `ConfigureMCP(spec)` idi; gerçekleşen arayüz adı
`ConfigureCLIMCP(spec)` oldu (bkz. `internal/providers/provider.go`) — claude
tarafında zaten var olan 5 argümanlı `ConfigureMCP`'nin adıyla çakışmaması için.
`ClaudeCLI.ConfigureCLIMCP` bu 5 argümanlı imzaya ince bir sarmalayıcıdır; claude
yolunda davranış değişikliği yok.

### 8.3 🔴 `default_tools_approval_mode = "approve"` blocker'ı — çözüldü, koda gömülü

§5.1'de tespit edilen kritik blocker (`"auto"` YETMEZ, yalnız `"approve"` koşulsuz
geçer) `codexcli_config.go`'nun ürettiği HER `[mcp_servers.*]` bloğuna sabit
yazılıyor; kod içi yorum bu satırın neden var olduğunu ve neden silinmemesi
gerektiğini taşıyor (§5.1'deki kaynak alıntısına referansla).

### 8.4 "AuthProber" arayüzü — plana girmedi, ayrı bir yol seçildi

Plan bir `AuthProber` soyutlaması öngörmüyordu ve uygulamada da öyle bir arayüz
**yok**. Login/subscription durumu iki taraf için de farklı somut fonksiyonlarla
okunuyor:

- claude-cli → `internal/claudeauth` paketi, `<claude-home>/…` içindeki OAuth
  credential'ını ayrıştırır, plan adını (`max`/`pro`) döner
  (`catalog_claudecli.go:claudeSubscriptionTier`).
- codex-cli → `catalog_codexcli.go:codexSubscriptionTier`, yalnızca
  `<codex-home>/auth.json`'un **var olup okunabilir olduğunu** kontrol eder ve
  sabit `"chatgpt"` döner. Codex'in `auth.json`'u Claude Code'un credential
  dosyasının aksine plan adını (Plus/Pro/Team) stabil bir alanda **açığa
  çıkarmıyor** — bu yüzden zengin bir tier ismi yerine yalnız "login var/yok"
  raporlanıyor. Bu kasıtlı bir asimetri; `resolveRuntimeBadge` (frontend)
  ikisini de aynı "CLI vX.Y.Z · Tier" biçimine indirger, ama codex için Tier
  her zaman "Chatgpt" ya boş.

### 8.5 Fiyatlandırma — eklenen ve BİLEREK atlanan modeller

`internal/providers/pricing.go`, yeni bir `"openai"` tablosu ekledi (yalnız
`EstimateFor`'un `codex-cli` case'ini beslemek için — `priceTable`'da
`"codex-cli"` anahtarı YOK, claude-cli ile aynı "abonelik = fiyatsız" mantığı).
İki bağımsız kaynaktan (2026-08-18) doğrulanan modeller:

| Model | Input/1M | Cached/1M | Output/1M |
|---|---|---|---|
| gpt-5.6-sol | $5.00 | $0.50 | $30.00 |
| gpt-5.6-terra | $2.00 | $0.20 | $12.00 |
| gpt-5.6-luna | $0.20 | $0.02 | $1.20 |
| gpt-5.5 | $5.00 | $0.50 | $30.00 |
| gpt-5.4-mini | $0.75 | $0.075 | $4.50 |

**Bilerek ATLANANLAR:** `gpt-5.4` (fiyatı var ama katalog notu "ChatGPT
hesabıyla kullanılamaz" diyor — bu transport'un çalıştıramayacağı bir modeli
fiyatlandırmanın anlamı yok) ve `gpt-5.2` (denenen iki kaynakta da yayınlanmış
per-token fiyat YOK — tahmin etmek yerine tablodan tamamen çıkarıldı;
`PriceFor`/`EstimateFor` bu modeli doğru şekilde "fiyatsız" raporlar).

### 8.6 Katalog sürüm/login probe'u

`internal/api/catalog_codexcli.go` — `catalog_claudecli.go`'nun birebir aynası
(10 dk TTL başarı / 1 dk TTL başarısızlık önbelleği, `codex --version` probe'u).
`internal/exttools/catalog.go`'ya `CodexToolName = "codex"` girişi eklendi
(Harici Araçlar panelinde claude'un kardeşi). `server.go:applySettings` artık
`exttools.SetPathOverride(exttools.CodexToolName, cur.CodexCLIPath)` çağırıyor
— panelin, sağlayıcının gerçekte çalıştırdığı ikiliyi göstermesi için (claude
tarafındaki aynı desenin eksik kalan tek satırıydı).

### 8.7 Frontend — codex kartı, `ask` modu onay butonu YOK

`ProvidersPanel.tsx`'e claude-cli kartının hemen altına ikinci bir kart eklendi:
CLI yolu alanı, workspace'e özel salt-okunur config dizini
(`GET /api/workspace-settings` → yeni `codexHomeDir` alanı, `ClaudeHomeDir`'in
kardeşi) ve bir **login butonu değil**, `CODEX_HOME=<dir> codex login` komutunu
gösteren yardım metni. Gerekçe: claude-cli'nin `ClaudeAuthDialog`'u
`checkWorkspaceClaudeAuth` gibi backend endpoint'lerine dayanıyor; codex'in
env-var kimlik kanalı (`ANTHROPIC_API_KEY` eşdeğeri) yok ve bu turda backend
login endpoint'i yazılmadı — sessizce hiçbir şey yapmayan bir buton koymak
yerine gerçek komutu gösteren metin tercih edildi.

### 8.8 🔴 `required = true` blocker'ı — optional sunucu, 1sn grace, sessiz araç kaybı

§5.1'deki onay blocker'ından **ayrı ve sonraki bir sessiz kayıp noktası**
bulundu (kaynak: `codex-rs/codex-mcp/src/connection_manager/tool_catalog.rs:35`
ve `capture_binding_with_metadata`, satır 171-224; alan tanımı
`config/src/mcp_types.rs:198,341`). Bir `[mcp_servers.*]` bloğunda
`required = true` yoksa sunucu **optional** sayılır; codex, optional sunucu
için handshake + `tools/list`'in tamamlanmasını beklemeden yalnızca
`OPTIONAL_MCP_STARTUP_GRACE = 1 saniye` bekler. `codex exec` her turda taze
süreç başlattığından ilk turda cache her zaman boştur — bu grace aşılırsa
sunucunun araçları o tur için **hiç** sunulmaz (hata/log yok). Çözüm:
`codexcli_config.go`'nun ürettiği HER `[mcp_servers.*]` bloğuna
`required = true` sabit yazılıyor (stdio ve remote/http fark etmeksizin);
kod içi yorum kaynak referansını taşıyor. Regresyon testi:
`TestRenderCodexConfigMarksServersRequired`
(`internal/providers/codexcli_config_test.go`).
