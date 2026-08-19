# 69 — Codex CLI Sağlayıcı: Fizibilite ve Referans

> **Soru:** TionSwarm bugün `claude-cli`'yi arka planda sürerek çalışıyor. Aynı
> yaklaşımı OpenAI **Codex CLI** için de kurabilir miyiz?
>
> **Cevap: Evet — ana akış (headless tur + MCP köprüsü + JSONL trace + resume +
> token muhasebesi) birebir kurulabilir.** İki mekanizmada gerçek boşluk var
> (per-tool onay ve native araç bastırma); ikisinin de telafisi var ama parite
> %100 değil. Detay aşağıda.
>
> Bu dosya **referans + fizibilite**dir. Uygulama adımları için → `70-CODEX-CLI-UYGULAMA-PLANI.md`.

---

## 0. Doğrulama ortamı

Bu dokümandaki her iddia, aşağıdaki iki kaynaktan **doğrulanarak** yazıldı; tahmin
yok:

| Kaynak | Konum |
|--------|-------|
| Codex CLI (kurulu) | `codex-cli 0.147.0` — `npm i -g @openai/codex` |
| Codex kaynak kodu | `C:\Users\user\Desktop\Progs\codex-src` (`openai/codex`, `main`, sığ klon) |

Canlı doğrulanan komutlar: `codex exec --help`, `codex login --help`,
`codex mcp --help`, `codex mcp-server --help`, `codex app-server --help` ve
gerçek bir `codex exec --json` koşusu (auth'suz bir `CODEX_HOME` ile — hata
sınıflandırmasını görmek için).

**Faz 0 durumu: TAMAMLANDI (2026-08-18).** `~/.codex` ChatGPT hesabıyla login
çıktı; izole bir `CODEX_HOME`'a `auth.json` kopyalanarak **başarılı turlar canlı
koşuldu**. JSONL şeması, `developer_instructions`, usage alanları, resume ve MCP
köprüsü gerçek çağrılarla doğrulandı. Sonuçlar → §7. Bu sırada **plana girmemiş
kritik bir blocker** bulundu → §5.1.

---

## 1. Codex CLI nedir, claude-cli'den farkı ne?

| | claude-cli (Claude Code) | codex-cli |
|---|---|---|
| Dil / dağıtım | Node/TS, `npm i -g @anthropic-ai/claude-code` | **Rust**, `npm i -g @openai/codex` (native binary sarmalayıcı) |
| Headless komut | `claude -p` | `codex exec` |
| Akış formatı | `--output-format stream-json --verbose` | `--json` (JSONL) |
| Prompt kanalı | stdin | argüman **veya** stdin (`-` ya da boru) |
| Config evi | `CLAUDE_CONFIG_DIR` | **`CODEX_HOME`** (birebir aynı rol) |
| Config dosyası | `settings.json` (JSON) | `config.toml` (**TOML**) |
| Config override | dosya bayrakları (`--settings`, `--mcp-config`) | **`-c dotted.key=<TOML>`** (tekrarlanabilir) |
| Oturum sürdürme | `--resume <id>` (**id her turda döner**) | `codex exec resume <thread_id>` (**id sabit**) |
| Dosya araçları | native `Read`/`Write`/`Edit`/`Glob`/`Grep` | **yok** — her şey `shell` + `apply_patch` üzerinden |
| Sandbox | yok (izin modu ile yönetilir) | **var** — OS düzeyi (`read-only` / `workspace-write` / `danger-full-access`) |
| Ayrıca | — | Codex kendisi **MCP sunucusu** olabilir (`codex mcp-server`) ve **app-server** (JSON-RPC daemon) sunar |

En önemli kavramsal fark: **Codex'te dosya okuma/yazma ayrı araç değil.** Model
`shell` ile okur, `apply_patch` ile yazar. TionSwarm'ın aktivite izinde
`Read`/`Edit` kartları yerine `command_execution` + `file_change` kartları
görülecek. Bu bir kayıp değil, **eşleme farkı** — UI tarafında ayrı map gerekir.

---

## 2. `codex exec` yüzeyi (0.147.0 — birebir `--help` çıktısından)

```
codex exec [OPTIONS] [PROMPT]
codex exec [OPTIONS] <COMMAND> [ARGS]

Commands:  resume | review | help
```

| Bayrak | Anlamı | TionSwarm karşılığı |
|--------|--------|---------------------|
| `--json` | stdout'a JSONL olay akışı | `--output-format stream-json --verbose` |
| `-o, --output-last-message <FILE>` | son mesajı dosyaya yaz | (claude'da yok — bonus) |
| `--output-schema <FILE>` | yapılandırılmış çıktı (JSON Schema) | `Request.OutputSchema` |
| `-m, --model <MODEL>` | model seçimi | `Request.Model` |
| `-c, --config <key=value>` | **TOML değerli config override, tekrarlanabilir** | `--settings` + `--mcp-config` yerine geçen tek kanal |
| `--ignore-user-config` | `$CODEX_HOME/config.toml`'u **atlar** (auth yine `CODEX_HOME`'dan okunur, ayrı katman) | ⚠️ TionSwarm KULLANMIYOR — bkz. §"Üretimde ne oldu" |
| `-s, --sandbox <MODE>` | `read-only` \| `workspace-write` \| `danger-full-access` | `Request.PermissionMode` |
| `--dangerously-bypass-approvals-and-sandbox` | tam serbest | `--dangerously-skip-permissions` |
| `--approve-for-me` | onayları **otomatik gözden geçirici modele** yönlendir (+ workspace-write) | (yakın karşılık: `--permission-prompt-tool`) |
| `-C, --cd <DIR>` | çalışma kökü | `Request.WorkDir` (claude'da `cmd.Dir`) |
| `--add-dir <DIR>` | ek yazılabilir dizin | — |
| `--skip-git-repo-check` | git deposu dışında çalış | — (claude gerektirmez) |
| `--ephemeral` | oturum dosyası yazma | — |
| `--strict-config` | tanınmayan config alanında hata ver | — |
| `--ignore-rules` | execpolicy `.rules` dosyalarını yükleme | — |
| `-p, --profile <NAME>` | `$CODEX_HOME/<name>.config.toml` katmanla | — |
| `-i, --image <FILE>` | prompta görsel ekle | ek (attachment) yolu |
| `--enable/--disable <FEATURE>` | `-c features.<n>=true/false` kısayolu | — |
| `--color <always\|never\|auto>` | renk | — |

### Prompt geçişi — dikkat

- Pozisyonel argüman **veya** stdin.
- **Hem** pozisyonel prompt **hem** boru ile stdin verilirse, stdin ayrıca
  `<stdin>` bloğu olarak **eklenir** (üzerine yazmaz).
- Windows ~32 KB komut satırı limiti nedeniyle TionSwarm prompt'u **stdin'den**
  geçirmeli (claude-cli'de olduğu gibi). Doğrulandı: stdin yolu çalışıyor.

---

## 3. JSONL olay şeması (kaynak: `codex-rs/exec/src/exec_events.rs`)

`stdout` **saf JSONL**'dir; tracing logları `stderr`'e gider. Canlı doğrulandı
(bkz. §7).

### Üst düzey olaylar — `{"type": ...}`

| `type` | Payload | TionSwarm eşlemesi |
|--------|---------|--------------------|
| `thread.started` | `{thread_id}` | `Response.SessionID` (resume anahtarı) |
| `turn.started` | `{}` | — |
| `turn.completed` | `{usage}` | `Response.Usage` |
| `turn.failed` | `{error:{message}}` | hata sınıflandırması |
| `item.started` | `{item}` | araç adımı başlangıcı |
| `item.updated` | `{item}` | araç adımı güncellemesi |
| `item.completed` | `{item}` | `TraceStep` (tamamlanmış) |
| `error` | `{message}` | ölümcül/ara hata |

### `usage` (turn.completed)

```json
{ "input_tokens": 0, "cached_input_tokens": 0, "cache_write_input_tokens": 0,
  "output_tokens": 0, "reasoning_output_tokens": 0 }
```

`providers.Usage` ile örtüşüyor — **ama input alanı bire bir DEĞİL**:

| Codex | TionSwarm |
|-------|-----------|
| `input_tokens` − `cached_input_tokens` − `cache_write_input_tokens` | `InputTokens` |
| `cached_input_tokens` | `CacheReadTokens` |
| `cache_write_input_tokens` | `CacheWriteTokens` |
| `output_tokens` | `OutputTokens` |
| `reasoning_output_tokens` | `ThinkingTokens` — **üstelik ölçülmüş** (claude-cli'de türetiliyor: `deriveThinkingTokens`) |

> **Çıkarma zorunlu.** Anthropic'te `input_tokens` cache'lenen ön eki **dışlar**;
> codex'te ise `input_tokens` **toplam** prompt'tur ve iki cache sayacı onun
> **alt kümesidir** (codex-rs bu yüzden `non_cached_input()` yardımcısını
> taşır). `providers.Usage` alanları ayrık olduğu için ham `input_tokens`'i
> yazmak cache'lenen ön eki iki kez faturalandırır. Eşleme
> `codexUsage.freshInput()` üzerinden yapılır; negatife düşerse **kırpılmaz** —
> wire semantiği değişmişse bu görünür olsun diye.

> Thinking tarafında claude-cli yolundan **daha iyi**: token tahmin değil, ölçüm.

**Fiyatlama:** OpenAI cache'i otomatiktir, TTL seçimi (Anthropic'in 5m/1h
`cache_control`'ü gibi) **yoktur** ve cache **yazımına prim uygulanmaz** —
yazılan token düz input fiyatındadır. Bu yüzden `pricing.go`'daki `openai`
tablosunun her satırı `CacheWriteMultOverride: 1.0` taşır; boş bırakılırsa
paket varsayılanı `CacheWriteMult` (1.25×) devreye girip olmayan bir prim
uydurur (`EstimateFor("codex-cli", …)` bu Price'ı doğrudan döndürür).

### `item` türleri — `item.details.type`

| `type` | Alanlar | UI karşılığı |
|--------|---------|--------------|
| `agent_message` | `text` | asistan metni / final cevap |
| `reasoning` | `text` | `TraceStep{Kind:"thinking"}` |
| `command_execution` | `command`, `aggregated_output`, `exit_code`, `status` | `TraceStep{Kind:"tool", Tool:"shell"}` |
| `file_change` | `changes:[{path,kind:add\|delete\|update}]`, `status` | diff kartı |
| `mcp_tool_call` | `server`, `tool`, `arguments`, `result`, `error`, `status` | **bridged araç adımı** — TionSwarm araçları buradan gelir |
| `collab_tool_call` | `tool`, `sender_thread_id`, `receiver_thread_ids`, `agents_states`, `status` | Codex'in kendi çok-ajan aracı (bastırılmalı, bkz. §5) |
| `web_search` | `id`, `query`, `action` | arama adımı |
| `todo_list` | `items:[{text,completed}]` | (bastırılmalı — `todo_write` köprüsü var) |
| `error` | `message` | ölümcül olmayan hata |

Durum alanları: `in_progress` → `completed` \| `failed` \| `declined`.

**Sonuç:** `climcp` + `cliStreamParser`'ın ürettiği `TraceStep` modelinin
tamamı Codex akışından üretilebilir. Tek eksik: **`Batch`** (paralel araç
gruplaması) — Codex akışında bir "assistant message id" yok, dolayısıyla
paralel çağrılar gruplanamaz. Kozmetik kayıp.

---

## 4. Kimlik doğrulama ve config izolasyonu

### `CODEX_HOME` = `CLAUDE_CONFIG_DIR`

`codex-rs/utils/home-dir/src/lib.rs`: `CODEX_HOME` set edilmişse **var olan bir
dizin olmalı**; değilse `~/.codex`. TionSwarm bunu sağlayıcı örneği başına izole eder:

```
<dataDir>/provider-homes/<instance-id>/  ← CODEX_HOME
├── config.toml               ← TionSwarm üretir (veya -c ile override)
├── auth.json                 ← login durumu
└── log/
```

Canlı doğrulandı: boş bir `CODEX_HOME` verildiğinde CLI onu kullandı ve
"login yok" hatasına düştü (global `~/.codex`'e sızmadı).

### Login yolları

| Yol | Komut / env |
|-----|-------------|
| ChatGPT aboneliği (OAuth, tarayıcı) | `codex login` |
| ChatGPT aboneliği (başsız/SSH) | `codex login --device-auth` |
| API anahtarı (stdin'den) | `printenv OPENAI_API_KEY \| codex login --with-api-key` |
| Access token (stdin'den) | `codex login --with-access-token` |
| Env | `OPENAI_API_KEY` |
| Durum | `codex login status` |

claude-cli'nin `CLAUDE_CODE_OAUTH_TOKEN` / `ANTHROPIC_API_KEY` env enjeksiyonuna
karşılık, Codex'te **`OPENAI_API_KEY`** aynı işi görür. `--with-api-key` stdin
kanalı, TionSwarm'ın "izole config evine anahtarsız login" akışını (workspace
kurulumunda bir kez) mümkün kılar.

### TionSwarm login akışı: örnek başına ✅

Yeni `codex-cli` örneği boş `configDir` ile oluşturulursa backend
`<dataDir>/provider-homes/<instance-id>` dizinini oluşturup yolu örneğe kaydeder.
Her örnek kendi `auth.json`, `config.toml` ve thread durumunu taşır; login bir
workspace'e değil örneğe aittir. Ajanlar uygulama-geneli örneklerden birini seçer.

UI'daki **Giriş yap / Durum** işlemleri seçili örnek id'siyle çalışır:
`GET /api/providers/{id}/auth`, device-code için `POST .../device/start`,
`GET .../device/status`, `POST .../device/cancel`; API anahtarı için
`POST .../api-key`. Device ve API-key login doğrudan o örneğin `CODEX_HOME`una
yazılır. Eski `/api/workspace-settings/codex-auth...` yolları geriye uyumluluk
için durur; yeni istemciler örnek-bazlı yolları kullanır.

Eski/dışarıdan oluşturulmuş örneğin `configDir` alanı boşsa auth/runtime çözümü
uygulama-geneli `<dataDir>/codex-home` yoluna düşer. Workspace başına home
tohumlama kaldırılmıştır. Tek login bulunan legacy workspace home'u global home'a
bir kez kopyalanır; birden fazla login varsa uyarı loglanır ve seçim yapılmaz.

### Auth hatası imzası (canlı gözlem)

Login'siz koşuda alınan **gerçek** çıktı:

```
{"type":"thread.started","thread_id":"01a01449-392e-7fb2-a685-5b3cc6eaa5cd"}
{"type":"turn.started"}
{"type":"error","message":"Reconnecting... 2/5 (unexpected status 401 Unauthorized: ...)"}
...
{"type":"turn.failed","error":{"message":"unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, ..."}}
```

Çıkış kodu **1**.

⚠️ **Önemli davranış:** Codex 401'i geçici hata sanıp **önce WebSocket transportunda
5 kez, sonra HTTPS transportunda 5 kez** yeniden denedi — toplam ~35 saniye
boşa akış. TionSwarm tarafında `401 Unauthorized` / `Missing bearer` görülür
görülmez **turu erken kesip non-retryable auth hatası** olarak sınıflandırmalıyız
(claude-cli'deki `isAuthErrorText` deseninin Codex karşılığı). Aksi halde her
login lapse'i 35 saniye yiyor.

---

## 5. MCP köprüsü — TionSwarm araçlarını Codex'e taşımak

Bu, entegrasyonun **kalbi**. Cevap: **çalışır.**

### Transport uyumu

Codex `mcp_servers` iki transport destekler (`codex-rs/config/src/mcp_types.rs`):

```toml
# stdio
[mcp_servers.foo]
command = "node"
args    = ["server.js"]
env     = { KEY = "val" }

# Streamable HTTP  ← TionSwarm Interaction bridge bunu kullanır
[mcp_servers.tionswarm_interaction]
url          = "http://127.0.0.1:PORT/core"
http_headers = { Authorization = "Bearer <token>" }
```

TionSwarm'ın in-process Interaction MCP sunucusu **Streamable HTTP + Bearer**
kullanıyor (`climcp.go`). Codex'in `StreamableHttp` varyantı **literal
`http_headers`** kabul ediyor → köprü **olduğu gibi** takılıyor. Ek olarak
`bearer_token_env_var` ile token'ı komut satırından tamamen gizlemek de mümkün
(daha güvenli, tercih edilmeli).

### Config'i tura özel enjekte etme

claude-cli'de `--mcp-config <dosya> --strict-mcp-config` var. Codex'te eşdeğer:

```bash
codex exec --json \
  -c 'mcp_servers.tionswarm_interaction={url="http://127.0.0.1:9x/core", http_headers={Authorization="Bearer ..."}}' \
  -c 'mcp_servers.tionswarm_extended={url="http://127.0.0.1:9x/extended", ...}' \
  ...
```

**Canlı doğrulandı:** `-c 'mcp_servers.demo={command="node",args=["x.js"]}'`
satır içi TOML tablosu **kabul edildi** (config parse hatası yok).

> **Ölçek uyarısı:** çok sayıda MCP sunucusu + uzun Bearer token, Windows'ta
> 32 KB komut satırı limitine yaklaştırabilir. Bu yüzden `70` dosyasındaki plan
> **her tur için `CODEX_HOME/config.toml` yazma**yı birincil yol, `-c`'yi ince
> ayar yolu olarak seçiyor — ve üretimde uygulanan da bu yoldur.

### Üretimde ne oldu — `--ignore-user-config` KULLANILMIYOR 🔴

Keşif aşamasında `--ignore-user-config`'in "yalnız `-c` ile verilen dünya
geçerli olur" şeklinde izolasyon sağlayacağı varsayılmıştı. **Bu yanlıştı.**
Codex kaynağında (`codex-rs/config/src/loader/mod.rs:516`,
`load_user_instance`) bu bayrak `CODEX_HOME/config.toml`'un TA KENDİSİNİ atlar
— ve TionSwarm'ın MCP sunucularını, `developer_instructions`'ı yazdığı dosya
tam olarak budur (`writeCodexConfig`, bkz. `internal/providers/codexcli.go`).
Bayrak açıkken canlı turlarda MCP köprüsü hiç yüklenmiyordu; model araçları
"tool registry'de yok" diye reddediyordu. İzolasyon zaten `CODEX_HOME` ortam
değişkeniyle sağlanıyor (alt süreç kendi home'unu okur, çağıran kullanıcının
`~/.codex`'ini değil) — bayrak hem gereksizdi hem de yıkıcıydı. Düzeltme:
bayrak `buildArgs`'tan tamamen kaldırıldı, `--strict-config` (bağımsız bir
kontrol, katman atlamıyor) korundu.

### 5.1 🔴 KRİTİK — `default_tools_approval_mode = "approve"` zorunlu

**Faz 0'da bulundu; planın ilk halinde yoktu ve tüm entegrasyonu sessizce
öldürüyordu.**

MCP sunucusu bağlanıyor, araçlar keşfediliyor, model doğru argümanlarla çağırıyor
— ama çağrı **çalışmadan** şu hatayla dönüyor:

```json
{"type":"item.completed","item":{"type":"mcp_tool_call","server":"tionprobe",
 "tool":"tion_ping","result":null,
 "error":{"message":"user cancelled MCP tool call"},"status":"failed"}}
```

Sebep: `codex exec` **her onay isteğini reddediyor** (§9 boşluk-1) ve MCP araç
çağrıları varsayılan olarak onaya tabi. Model iki kez denedi, iki kez reddedildi,
sonra pes etti.

Kaynak koddaki kapı (`codex-mcp/src/mcp/mod.rs`):

```rust
pub fn mcp_permission_prompt_is_auto_approved(...) -> bool {
    if context.tool_approval_mode == Some(AppToolApproval::Approve) {
        return true;                       // ← koşulsuz otomatik onay
    }
    if approval_policy != AskForApproval::Never { return false; }
    match permission_profile { ... has_full_disk_write_access() }
}
```

**Çözüm:** her `[mcp_servers.*]` bloğuna

```toml
default_tools_approval_mode = "approve"
```

⚠️ `"auto"` **yetmiyor** — canlı denendi, yine reddedildi. Enum değerleri
`auto` / `prompt` / `writes` / `approve`; koşulsuz geçen tek değer `approve`.

✅ Doğrulandı: `-s read-only` ve `-s workspace-write` **her ikisinde de** araç
çağrısı başarıyla çalıştı ve sonuç modele döndü.

> Bu satır TionSwarm'ın ürettiği her Codex MCP bloğunda bulunmak **zorunda**.
> "Gereksiz görünüyor" diye temizlenirse köprü tamamen ölür — kod içine bu
> gerekçeyi yazan bir yorum konuldu.

### 5.2 🔴 KRİTİK — `required = true` zorunlu (optional sunucu, 1sn grace, sessiz araç kaybı)

**Faz sonrasında bulundu.** `default_tools_approval_mode = "approve"` (§5.1)
çağrının reddedilmesini önler, ama bundan **önceki** bir aşamada — sunucunun
tool listesinin modele hiç sunulmaması — ayrı bir sessiz kayıp noktası var.

Kaynak (`codex-rs/codex-mcp/src/connection_manager/tool_catalog.rs:35` ve
`capture_binding_with_metadata`, satır 171-224; alan tanımı
`config/src/mcp_types.rs:198,341`): bir `[mcp_servers.*]` bloğunda
`required = true` **yoksa** sunucu **optional** sayılır. Optional sunucu için
codex, handshake + `tools/list`'in tamamlanmasını **beklemez** — yalnızca
`OPTIONAL_MCP_STARTUP_GRACE = 1 saniye` bekler. Bu süre içinde başlangıç
tamamlanmazsa ve cache boşsa (ki `codex exec` her turda **taze süreç**
başlattığından ilk turda cache **her zaman** boştur), o sunucunun araçları
o tur için **tamamen** dışlanır — ne doğrudan araç listesinde, ne
`tool_search` üzerinden. Hata da, log de yok; model sunucunun hiç var
olmadığını sanır.

Canlı gözlem bunu doğruladı: MCP süreci spawn oluyor ama
`dynamic_tool_count=0` kalıyor.

**Çözüm:** her `[mcp_servers.*]` bloğuna (stdio ve remote/http fark etmeksizin)

```toml
required = true
```

> Bu satır da `default_tools_approval_mode = "approve"` gibi TionSwarm'ın
> ürettiği her Codex MCP bloğunda bulunmak **zorunda**. Kaldırılırsa köprü
> yavaş başlayan/uzak sunucularda ilk turda sessizce araçsız kalır.
> `internal/providers/codexcli_config.go` → `renderCodexServer` içinde
> kaynak referansıyla birlikte yorum var.

### Araç adlandırma

`codex-rs/core/src/tools/handlers/mcp.rs`: `mcp__<server>__<tool>` — claude-cli
ile **aynı** namespace deseni. TionSwarm'ın mevcut isim ayrıştırması
(`mcp.SplitNamespaced`, trace strip'leme) değişmeden çalışır.

### Sunucu bazlı araç filtresi

`enabled_tools` (izin listesi) ve `disabled_tools` (red listesi) **sunucu
başına** ayarlanabilir. `--allowedTools` kadar esnek değil ama gerekeni verir:

```toml
[mcp_servers.tionswarm_extended]
url = "..."
disabled_tools = ["delete_workspace"]
```

### MCP zaman aşımları

`startup_timeout_sec` ve `tool_timeout_sec` (sunucu başına). claude-cli'nin
`MCP_TIMEOUT` / `MCP_TOOL_TIMEOUT` env'lerinin karşılığı. Uzun `run_subagent`
çağrıları için **mutlaka yükseltilmeli** (varsayılan düşük).

### codebase-memory-mcp prefill guard codex yolunda YOK (Q4 notu)

Q4 doğrulaması (codebase-memory-mcp çağrılarının `project` argümanı eksikken
bile çalışması) **PASS** geçti, ama bunun **neden** çalıştığı önemli: TionSwarm'ın
kendi ajan döngüsü eksik `project` argümanını oturumun working directory'sinden
otomatik dolduruyor ve düzeltilebilir kimlik hatalarını tekrar koşturarak
onarıyor (`internal/agent/mcpargs.go`, `mcprepair.go`). **Bu koruma codex-cli
yolunda devrede DEĞİL** — `internal/providers/codexcli.go` `internal/agent`
paketine hiç referans vermiyor, yani `mcpargs.go`/`mcprepair.go`'daki prefill/
repair mantığı codex'in tool-call döngüsüne hiç bağlanmıyor. Tıpkı
`CLAUDE.md`'nin claude-cli için zaten söylediği gibi ("claude-cli sağlayıcısında
bu koruma yoktur — araç döngüsünü CLI kendi koşturur"), **codex-cli için de
aynı durum geçerli**: araç döngüsünü codex kendi koşturuyor, çağrılar
TionSwarm'ın agent paketinden geçmiyor. Q4'ün PASS çıkması modelin argümanı
doğru vermesinden kaynaklandı, TionSwarm'ın bir güvencesinden değil — bu ayrım
gelecekteki bir regresyonu yanlış tanılamamak için önemli.

---

## 6. Sistem promptu enjeksiyonu

claude-cli: `--append-system-prompt` / `--append-system-prompt-file`.

Codex'te **üç** kanal var (`codex-rs/config/src/config_toml.rs`):

| Kanal | Alan | Davranış | Uygun mu? |
|-------|------|----------|-----------|
| **`developer_instructions`** | config | `developer` rollü mesaj olarak **eklenir** | ✅ **Doğru kanal** — `--append-system-prompt`'un tam karşılığı |
| `instructions` | config | sistem talimatı | kısmen |
| `model_instructions_file` | config (dosya yolu) | built-in talimatları **EZER** | ❌ kaynak kodda "STRONGLY DISCOURAGED" |
| `AGENTS.md` | proje dosyası | cwd hiyerarşisinden toplanır | ikincil (repo bağlamı için doğal) |

Yani TionSwarm'ın statik system prefix'i → `developer_instructions`,
volatil `[Context]` bloğu → prompt'un başına (claude-cli'deki `buildSystemAndPrompt`
ayrımı **aynen** korunur).

`developer_instructions` çok büyüyeceği için (skill kataloğu + araç notları)
**komut satırından `-c` ile değil, `CODEX_HOME/config.toml` içine yazılmalı.**

Ayrıca ilgili anahtarlar: `include_permissions_instructions`,
`include_apps_instructions`, `include_collaboration_mode_instructions`,
`include_environment_context` — Codex'in kendi enjekte ettiği blokları kapatıp
prompt'u sadeleştirmek için.

---

## 7. Faz 0 canlı doğrulama sonuçları (2026-08-18)

Kurulum: `~/.codex/auth.json` izole bir probe home'a kopyalandı
(`codex login status` → `Logged in using ChatGPT`), tüm turlar
`--ignore-user-config --strict-config` ile koşuldu.

> ⚠️ **Bu doğrulama serisi MCP sunucularını `-c` satır-içi override ile
> vermişti**, `CODEX_HOME/config.toml` dosyasına yazarak değil — bu yüzden 0.5/
> 0.6 o zaman geçti. Üretim yolu (`writeCodexConfig`) sunucuları config.toml'a
> yazıyor, ve `--ignore-user-config` TA O DOSYAYI atlıyor. Bayrak sonradan
> kaldırıldı — bkz. §"Üretimde ne oldu" ve §0.4'ün altındaki not.

| # | Doğrulama | Sonuç |
|---|-----------|-------|
| 0.1 | İzole `CODEX_HOME` + auth.json ile login | ✅ `Logged in using ChatGPT` |
| 0.2 | JSONL şeması kaynak koddan çıkarılanla uyuşuyor | ✅ **birebir** |
| 0.3 | `developer_instructions` gerçekten prompta giriyor | ✅ "her mesaja sadece KLINGON yaz" talimatı → cevap `KLINGON` |
| 0.4 | `--strict-config` hiçbir override'ı reddetmiyor | ✅ hepsi geçerli anahtar (not: bu doğrulama `--ignore-user-config`'in üretimde MCP köprüsünü kırdığını YAKALAMADI, çünkü sunucular `-c` ile veriliyordu — bkz. yukarıdaki uyarı) |
| 0.5 | MCP sunucusu bağlanıyor, araçlar keşfediliyor | ✅ (yerel node stdio sunucusu) |
| 0.6 | MCP aracı **çalışıyor** | ⚠️→✅ **önce başarısız** (§5.1), `default_tools_approval_mode="approve"` ile çözüldü |
| 0.7 | `resume` + sıcak cache | ✅ `thread_id` **sabit**; kod kelimesi hatırlandı; `cached_input_tokens=14592` |
| 0.8 | `[tools]` bastırmaları | ✅ `web_search=false`; ⚠️ `update_plan` **bool kabul etmiyor** → `{enabled=false}` |
| — | stdout saf JSONL / stderr ayrı | ✅ |
| — | `command_execution` item'ı | ✅ shell koştu, `exit_code:0`, çıktı `aggregated_output`'ta |
| — | prompt stdin'den | ✅ |

### Doğrulanmış usage çıktısı

```json
{"type":"turn.completed","usage":{"input_tokens":21060,"cached_input_tokens":14592,
 "cache_write_input_tokens":0,"output_tokens":52,"reasoning_output_tokens":33}}
```

Prompt cache **ilk turda bile** çalışıyor (Codex kendi sistem promptunu cache'liyor).

### Faz 0'da çıkan ek sürprizler

1. **`-s` / `-c` bayrakları `resume` alt-komutundan ÖNCE gelmeli.**
   `codex exec resume <id> -s read-only` → `error: unexpected argument '-s' found`.
   Yalnız `--model` ve `--dangerously-bypass-*` gerçek global. Doğru sıra:
   `codex exec <OPTIONS> resume <id> <prompt>`.
2. **`tools.update_plan` bool kabul etmiyor.**
   `invalid type: boolean 'false', expected struct UpdatePlanToolConfig` →
   `{enabled=false}` yazılmalı. `web_search` ise bare bool kabul ediyor.
3. **ChatGPT hesabında model kısıtı var.** `gpt-5.4-mini` çalıştı;
   `gpt-5.4` ve `gpt-5.2` → `"The 'X' model is not supported when using Codex
   with a ChatGPT account."`; varsayılan model o an
   `"Selected model is at capacity."` verdi. Manifest'te `AllowCustomModel:true`
   ve "CLI varsayılanını kullan" (boş model) girişi bu yüzden şart.
4. **401 israfı doğrulandı** — login'siz koşuda WS'te 5 + HTTPS'te 5 retry,
   ~35 sn. Sağlayıcı ilk `error` olayında auth imzasını görüp süreci öldürmeli.

---

## 8. Parite matrisi — TionSwarm'ın claude-cli'de kullandığı **her** mekanizma

| # | TionSwarm mekanizması | claude-cli | codex-cli | Durum |
|---|----------------------|-----------|-----------|-------|
| 1 | Başsız tek-tur | `claude -p` | `codex exec` | ✅ tam |
| 2 | Yapılandırılmış akış | `stream-json --verbose` | `--json` | ✅ tam |
| 3 | Prompt stdin'den (32 KB limiti) | stdin | stdin | ✅ tam |
| 4 | İzole config evi | `CLAUDE_CONFIG_DIR` | `CODEX_HOME` | ✅ tam |
| 5 | Anahtarsız abonelik login | OAuth token | `codex login` (ChatGPT) | ✅ tam |
| 6 | API-key alternatifi | `ANTHROPIC_API_KEY` | `OPENAI_API_KEY` | ✅ tam |
| 7 | MCP delegasyonu | `--mcp-config` | `-c mcp_servers.*` / `config.toml` | ✅ tam |
| 8 | Sıkı MCP izolasyonu | `--strict-mcp-config` | `CODEX_HOME` (ayrı config evi) | ⚠️ kısmi — `--ignore-user-config` DENENDİ, üretimde MCP köprüsünü kırdığı için KALDIRILDI (bkz. §"Üretimde ne oldu"); izolasyon yalnız `CODEX_HOME` ile sağlanıyor |
| 9 | HTTP+Bearer MCP transport | `type:"http"` + headers | `url` + `http_headers` | ✅ tam |
| 10 | Araç namespace'i | `mcp__srv__tool` | `mcp__srv__tool` | ✅ aynı |
| 11 | Sistem prompt ekleme | `--append-system-prompt[-file]` | `developer_instructions` | ✅ tam |
| 12 | Statik/volatil prompt ayrımı (cache) | var | var (aynı desen) | ✅ tam |
| 13 | Oturum sürdürme (sıcak cache) | `--resume <rotating id>` | `exec resume <stable thread_id>` | ✅ **daha iyi** (id dönmüyor) |
| 14 | Token muhasebesi + cache read/write | usage | usage | ✅ tam |
| 15 | Thinking token'ı | türetilmiş tahmin | **ölçülmüş** (`reasoning_output_tokens`) | ✅ **daha iyi** |
| 16 | Reasoning/effort seviyesi | `effortLevel` (low→max) | `model_reasoning_effort` (none/minimal/low/medium/high/xhigh/max/ultra) | ✅ **daha geniş** |
| 17 | Çalışma dizini | `cmd.Dir` | `-C/--cd` | ✅ tam |
| 18 | Aktivite izi (tool/thinking/text) | trace | `item.*` olayları | ✅ tam |
| 19 | Paralel araç gruplama (`Batch`) | message id ile | ✗ id yok | ⚠️ kozmetik kayıp |
| 20 | İzin modu: read-only | `--permission-mode plan` | `-s read-only` (**OS sandbox**) | ✅ **daha güçlü** |
| 21 | İzin modu: auto | `--dangerously-skip-permissions` | `--dangerously-bypass-approvals-and-sandbox` | ✅ tam |
| 22 | İzin modu: **ask** (UI'da per-tool onay) | `--permission-prompt-tool` | ✗ **yok** | ❌ **boşluk-1** |
| 23 | Native araç bastırma (shadowing) | `--disallowedTools` | ✗ genel bastırma yok | ❌ **boşluk-2** |
| 24 | Hook geçişi (Pre/PostToolUse) | `--settings` hooks → CLI'nin kendi tool loop'u tetikler | ❌ **hiç uygulanmıyor** — bkz. §9 Boşluk-3 |
| 25 | Yapılandırılmış çıktı | — | `--output-schema` | ✅ **bonus** |
| 26 | Yanlış-tur kurtarma / salvage | var | aynı desen uygulanabilir | ✅ port edilebilir |
| 27 | Kalıcı (persistent) oturum havuzu | var | `codex app-server` daha uygun | ⚠️ farklı mimari |
| 28 | Lazy tool loading (extended tier gate) | `activate_tools` + `tools/list_changed` re-list | ✗ bildirim loglanıyor, re-list **hiç yapılmıyor** | ❌ **boşluk-4** — bkz. §9 |
| 29 | sqz/rtk token-optimizer çıktı sıkıştırma | `db.HookPostToolUse` komutu (24 numaralı satıra bağımlı) | hook yok → sqz de yok | ❌ 24 numaralı satırın türevi, ayrıca not düşüldü |

---

## 9. Dört gerçek boşluk ve telafileri

> Bu bölüm 2026-08-18'de sekiz sorulu bir canlı entegrasyon doğrulaması (Q1-Q8,
> özet tablo `70-CODEX-CLI-UYGULAMA-PLANI.md` §8'de) ile genişletildi: Boşluk-1/2
> keşif aşamasından; Boşluk-3/4 (hook/sqz, lazy tool loading) o doğrulamadan geldi.

### ❌ Boşluk-1: "ask" modunda per-tool onay yok

`codex-rs/exec/src/lib.rs` **her** onay isteğini reddediyor — kaynaktan birebir:

```rust
ServerRequest::CommandExecutionRequestApproval { .. } =>
    reject_server_request(..., "command execution approval is not supported in exec mode ...")
ServerRequest::FileChangeRequestApproval  { .. } => ... "file change approval is not supported in exec mode"
ServerRequest::ToolRequestUserInput       { .. } => ... "request_user_input is not supported in exec mode"
ServerRequest::McpServerElicitationRequest{ .. } => // otomatik CANCEL
```

Yani `--permission-prompt-tool`'un karşılığı `codex exec`'te **yok**.

**Telafiler (tercih sırasıyla):**

1. **OS sandbox'ı izin modu olarak kullan** — `read-only` → `-s read-only`
   (yazma denemesi çekirdek seviyesinde bloklanır, prompt'a güvenmez);
   `auto` → `--dangerously-bypass-approvals-and-sandbox`;
   `ask` → **`-s workspace-write`** (yazma yalnız çalışma alanına, çalışma alanı
   dışı ve ağ engelli). Bu, "onay sor" değil ama **"zararı sınırla"** — ve
   claude-cli'nin `acceptEdits`'inden daha güvenli.
2. **`--approve-for-me`** — onayları Codex'in kendi guardian modeline yönlendirir.
   İnsan onayı değil, model onayı. Otonom akışlar için makul.
3. **`codex app-server`** (JSON-RPC daemon) — bu ServerRequest'ler orada
   **cevaplanabilir**, yani gerçek per-tool onay UI'ı mümkün. Ama app-server
   `[experimental]` işaretli. → Faz 4 (opsiyonel), `70` dosyasında.

> **Karar (önerilen):** Faz 1'de yol 1 + 2. TionSwarm UI'ında `ask` modunun
> codex-cli ajanlarında **"sandbox ile sınırla"** anlamına geldiği açıkça
> yazılmalı — sessizce `auto` gibi davranmamalı.

> **Dürüstlük notu (Q6, 2026-08-18 doğrulaması):** canlı test yalnızca **PARTIAL**
> geçti. `ask`/`read-only` modda gözlemlenen ret, modelin **kendi policy metnine**
> göre (sistem promptundaki talimata uyarak) çekilmesiydi — gerçek bir **OS-sandbox
> seviyesi** reddi (çekirdek/`-s read-only` bir yazma syscall'ını engellemesi)
> **kanıtlanmadı**. Yani "OS sandbox garanti eder" iddiası (yukarıdaki 1. telafi
> ve §10.1) mimari olarak doğru ama bu doğrulama turunda **gözlemlenmedi** —
> ayrı, düşük maliyetli bir deney (sandbox'ı bilerek ihlal eden bir komut verip
> modelin DEĞİL çekirdeğin reddettiğini görmek) hâlâ yapılmadı.

### ❌ Boşluk-2: native araçları bastıramıyoruz

`climcp.go`'nun en değerli işi: `TodoWrite`, `Skill`, `Task`/`Agent`,
`SendMessage`, `Bash` gibi **köprülenmiş araçları gölgeleyen** CLI-native
araçları `--disallowedTools` ile kapatmak.

Codex'te genel bir `--disallowedTools` **yok**. `ToolsToml` yalnızca üç şeyi
kapatabiliyor:

```toml
[tools]
web_search = false
update_plan = false                       # → todo_list item'ını susturur
experimental_request_user_input = { enabled = false }
```

Codex'in native araç seti (`codex-rs/core/src/tools/handlers/`):
`shell` / `unified_exec`, `apply_patch`, `view_image`, `plan`,
`request_user_input`, `request_permissions`, `tool_search`, `current_time`,
`sleep`, `get_context_remaining`, `new_context_window`, `multi_agents`,
`mcp`, `mcp_resource`, plugin araçları.

**Gölgeleme riski analizi:**

| TionSwarm köprüsü | Codex native gölgesi | Kapatılabilir mi? |
|---|---|---|
| `todo_write` | `update_plan` (`todo_list` item) | ✅ `[tools] update_plan = false` |
| `ask_user` | `request_user_input` | ✅ `experimental_request_user_input.enabled = false` (üstelik exec'te zaten çalışmıyor) |
| `WebSearch` | `web_search` | ✅ `[tools] web_search = false` |
| `run_subagent` / coordinator | `multi_agents` (collab) | ⚠️ `features` bayrağı ile — Faz 0'da doğrulanacak |
| `Bash` / `PowerShell` | `shell`, `unified_exec` | ❌ kapatılamaz — **ama kapatmamalıyız da** |
| `Read`/`Write`/`Edit` | `apply_patch` | ❌ kapatılamaz — kapatmamalıyız |
| `use_skill` | Codex `skills` alt sistemi | ⚠️ `[skills]` config ile sınırlanabilir |

**Kritik gözlem:** Boşluk-2 aslında iyi haber. Codex'te native dosya/shell
araçlarını **bastırmak istemiyoruz** — Codex modeli `shell` + `apply_patch`
etrafında eğitildi, onları alırsak ajan işe yaramaz hale gelir. TionSwarm'ın
claude-cli'de `Bash`'i bastırma gerekçesi ("kendi sandbox'ımızdan geçsin"),
Codex'te **Codex'in kendi OS sandbox'ı** tarafından zaten karşılanıyor.

Yani gerçek etkisi: **`todo_write` / `ask_user` / `WebSearch` köprüleri
korunabiliyor (kapatılabilir), dosya-shell tarafı native kalıyor.**
`climcp.go`'daki `suppressIfBridged` mantığının Codex karşılığı çok daha küçük
bir liste olacak.

### ❌ Boşluk-3: Hook'lar (Pre/PostToolUse) codex turunda HİÇ uygulanmıyor — ve sqz de onunla birlikte devre dışı

**Önceki hal bu bölümde "şema farklı" diyordu — bu yanlıştı.** 2026-08-18
doğrulamasında kanıtlandı: codex turunda hook'lar **hiç çalıştırılmıyor**, şema
sorunu değil, çağrı yeri sorunu.

Kanıt zinciri:

- Hook'lar yalnız **native tool loop**'ta uygulanıyor:
  `internal/agent/hooks.go` → `runPreToolHooks` / `runPostToolHooks`; çağrı
  yerleri `internal/agent/toolloop.go:803` ve `:988`.
- codex `cliMCP` dalı `recordedComplete` çağırıp `internal/agent/toolloop.go:376`'da
  **erken return** ediyor — native tool loop'a hiç girmiyor, dolayısıyla
  `runPreToolHooks`/`runPostToolHooks` çağrılarına hiç ulaşmıyor.
- claude-cli bunu `internal/agent/climcp.go` → `writeCLISettings` ile telafi
  ediyor: workspace hook'larını claude'un kendi `--settings` `hooks` sözleşmesine
  çevirip yazıyor, CLI'nin **kendi** tool loop'u tetikliyor. **codex'te eşdeğer
  bir `writeCodexSettings`/hook-çeviri yolu yok.**
- MCP köprüsünden gelen `mcp__tionswarm_interaction__*` / `mcp__tionswarm_extended__*`
  çağrıları da hook'suz: `internal/api` tarafındaki dispatch (`interactionBackend.Call`,
  `mcp_interaction.go`) `internal/agent/hooks.go`'a hiç referans vermiyor.

**sqz de bunun türevi, ayrı bir kayıp değil:** sqz katmanı `db.HookPostToolUse`
komutu olarak tanımlı — bağımsız bir katman değil. Hook mekanizması codex
turunda hiç tetiklenmediği için, workspace'te sqz kurulu olsa bile codex
turlarında **sqz çıktı sıkıştırması da devre dışı**. Codex'in kendi shell/
`apply_patch` araçlarının çıktısı ham döner.

**Telafi:** Codex 0.147.0'ın kendi `[hooks]` config anahtarı var (§8 satır 24
eski hali bunu işaret ediyordu) ama bu, TionSwarm'ın workspace hook store'unu
(`db.ListEnabledHooksByEvent`) codex'in `config.toml`'una çeviren bir yazıcı
gerektirir — `writeCLISettings`'in codex karşılığı henüz **yazılmadı**. Faz
planına eklenmeli; şu an için codex ajanları workspace hook'larından ve
sqz/rtk optimizasyonundan **tamamen muaf**.

#### UI'da görünürlük (2026-08-18 — uygulandı)

Bu boşluk artık **sessiz değil**, ürün içinde açıkça görünüyor:

- **Sağlayıcı-yeteneği sinyali:** `internal/providers/kind.go`'daki `Manifest`
  yeni bir `AppliesToolHooks bool` alanı taşıyor — her `kind_*.go` kendi
  değerini bildiriyor (`codex-cli` → `false`, geri kalan tüm dahili/özel
  sağlayıcılar → `true`). `internal/providers/catalog.go` bunu `CatalogEntry`
  üzerinden `/api/catalog` yanıtına (`appliesToolHooks` alanı) taşıyor —
  frontend'de codex-cli'yi elle isimle eşleştiren bir kod yok, tek gerçek
  kaynak bu Manifest alanı.
- **Hooks ekranı** (`frontend/src/features/settings/HooksPanel.tsx`): mevcut
  hook açıklaması kutusunun hemen altına, `color-warning` stiliyle
  (`ClaudeAuthGate`/`HooksPanel` yerleşik davranışlar bölümüyle aynı görsel
  dil) sabit bir uyarı eklendi — hook'ların codex-cli turlarında **hiç**
  çalışmadığını, bunun hem codex'in kendi shell/apply_patch araçlarını hem MCP
  köprüsü üzerinden çağrılan TionSwarm araçlarını kapsadığını ve sqz/
  PostToolUse token-optimizer sıkıştırmasının da bu yüzden devre dışı
  olduğunu açıkça belirtiyor.
- **Ajan düzenleyici** (`frontend/src/features/agents/AgentSettingsForm.tsx`):
  `ProviderModelSelect`'in hemen altında, seçili sağlayıcının kataloğundaki
  `appliesToolHooks === false` olduğu durumda aynı stilde bir satır uyarı
  çıkıyor — kullanıcı bir ajanı codex-cli'ye çevirdiği anda, o ajana hiç hook
  uygulanmayacağını orada görüyor.
- **`internal/api/hooks.go`** içindeki "CLI hook passthrough" yerleşik-davranış
  açıklaması artık yalnız claude-cli'den bahsetmiyor; codex-cli'nin hiçbir
  hook'u tetiklemediğini ve bunun `appliesToolHooks` bayrağıyla
  ilişkilendiğini de söylüyor.
- **Test:** `internal/providers/kind_test.go` →
  `TestAppliesToolHooksSignal`, katalogda `codex-cli.AppliesToolHooks=false`,
  `claude-cli.AppliesToolHooks=true` ve `anthropic.AppliesToolHooks=true`
  olduğunu doğruluyor.

Bu değişiklik hook **çalıştırma** davranışına dokunmuyor — yalnızca var olan
sınırı görünür kılıyor.

### ❌ Boşluk-4: Lazy tool loading (extended tier gate) codex'te çalışmıyor

TionSwarm'ın gateway modeli (Doc 52), extended tier'ı boş başlatıp modelin
`activate_tools` çağrısıyla büyütmesine, backend'in `tools/list_changed`
push'lamasına ve CLI'nin bunu görüp `tools/list`'i **yeniden çekmesine**
dayanır. claude-cli bunu 10-16ms içinde yapıyor (canlı ölçüldü,
`probe_relist_test.go`). **codex-cli bu bildirimi asla işlemiyor:**

- Kanıt: `C:\Users\user\Desktop\Progs\codex-src\codex-rs\rmcp-client\src\logging_client_handler.rs:86-88`
  — `on_tool_list_changed` gövdesi tek satır `info!(...)`; hiçbir re-fetch
  tetiklenmiyor.
- "Sonraki turda görünür olur" hipotezi **canlı test edildi ve çürütüldü**: tur 1
  aktive et / tur 2 çağır senaryosunda model tur 2'de aracı asla göremedi,
  `activate_tools`'u 3 kez daha tekrarlayıp pes etti (toplam 7+3 boşuna deneme).

**Sonuç:** codex turunda `activate_tools`/`deactivate_tools`/`active_tools`/
`tool_search` mekanizmasının **hiçbiri işe yaramaz** — sadece model boşuna
dener ve token yakar.

**Çözüm (uygulandı, bu görevin A kısmı):** codex-cli isteği artık interaction
endpoint URL'lerine `?full=1` sorgu parametresi ekliyor
(`internal/agent/codexmcp.go` → `interactionServers`). Backend
(`internal/api/mcp_interaction.go` → `Tools()`) bu işareti gördüğünde:

1. Extended tier'ı **aktivasyon gate'i atlayarak** tam listeyle döndürür (gizli
   katman dahil — hiçbiri artık "aktive edilmemiş" diye gizlenmez).
2. `activate_tools`/`deactivate_tools`/`active_tools`/`tool_search`
   meta-araçlarını **hem core hem extended** yüzeyden çıkarır — çalışmayan bir
   mekanizmayı modele göstermenin tek etkisi döngüye sokmaktı.

claude-cli isteği (query string'siz `/core`, `/extended`) davranışsal olarak
**birebir aynı** kalır — bu, `?full=1` path'i değil query'yi kullandığı ve
`tierFromPath` (path tabanlı ayrıştırma) hiç değişmediği için garanti
edilir. Bkz. `internal/interaction/server.go` → `requestTier`/`fullTierQueryParam`.

**Token maliyeti:** extended tier tam açıkken (self-management suite +
interaction extended tools, meta-araçlar hariç) **71 bridged self-management
tool + 5 interaction-extended tool = 76 araç**, yaklaşık **75-90 KB** ham
şema+açıklama metni — kabaca **~19-23k token** (4 byte/token kaba tahmini;
gerçek tokenizer'a göre değişir). Bu, `19-LAZY-TOOL-LOADING.md`'nin belirttiği
eski ~49 araç / ~10-15k token tahmininden **yüksek** — self-management suite o
zamandan beri büyümüş (create_automation, insight_*, handoff_session gibi
tool'lar eklendi). Codex zaten her turda taze süreç başlattığı ve
lazy-activation'ı kullanamadığı için, bu maliyet codex'in **her turunda**
sabit olarak ödenir (claude-cli'de ise extended tier normalde boş başlar ve
yalnız aktive edilen araçlar kadar büyür).

---

## 10. Codex'e özgü kazançlar (claude-cli'de olmayan)

1. **OS düzeyi sandbox** — Linux'ta seccomp/landlock, macOS'ta Seatbelt,
   Windows'ta `windows-sandbox-rs`. `read-only` modu prompt'a değil çekirdeğe
   dayanıyor. TionSwarm'ın `read-only` izin modu için **gerçek** bir garanti.
2. **`--output-schema`** — `Request.OutputSchema` CLI yolunda da desteklenebilir
   (claude-cli'de yok).
3. **Sabit `thread_id`** — resume anahtarı dönmüyor; `chat_resume.go`'daki
   rotasyon takibi Codex için gereksiz (daha basit).
4. **Ölçülmüş reasoning token** — `deriveThinkingTokens` tahmini yerine gerçek sayı.
5. **`codex mcp-server`** — Codex'in kendisi TionSwarm'a MCP sunucusu olarak
   takılabilir (ajan → ajan delegasyonu için alternatif desen).
6. **`-o/--output-last-message`** — final cevabı dosyaya alma; akış parse
   başarısız olsa bile kurtarma yolu.
7. **`--ephemeral`** — oturum dosyası yazmadan koşma (geçici/spawn turları için).

---

## 11. Karar

| Soru | Cevap |
|------|-------|
| Codex CLI, claude-cli gibi arka planda sürülebilir mi? | **Evet.** |
| Mimari değişiklik gerekir mi? | **Hayır, kırıcı değişiklik yok.** `ProviderKind` soyutlaması (bkz. `kind.go`: "yeni transport = yeni `kind_*.go`") tam da bunun için var. Tek gerçek refactor: `toolloop.go`'daki `provider.(*providers.ClaudeCLI)` tip assertion'ı bir **arayüze** çevirmek. |
| Parite ne kadar? | 27 mekanizmanın **22'si tam**, 2'si Codex lehine daha iyi, 3'ü kısmi/eksik. |
| Ne kaybediyoruz? | (a) UI'da per-tool onay ("ask" modu sandbox'a düşer), (b) paralel araç gruplama rozeti, (c) native araç bastırmanın çoğu — ki bunun büyük kısmı Codex'te **gereksiz**. |
| Ne kazanıyoruz? | OS sandbox, ölçülmüş reasoning token, sabit thread id, output-schema, ikinci bir abonelik havuzu (ChatGPT Plus/Pro), OpenAI model ailesi. |

**Öneri: yapılmalı.** Uygulama planı → `70-CODEX-CLI-UYGULAMA-PLANI.md`.

---

## 12. Model kataloğu (Codex 0.147.0 — `models-manager/models.json`)

| slug | Görünen ad | Bağlam |
|------|-----------|--------|
| `gpt-5.6-sol` | GPT-5.6-Sol | 272.000 |
| `gpt-5.6-terra` | GPT-5.6-Terra | 272.000 |
| `gpt-5.6-luna` | GPT-5.6-Luna | 272.000 |
| `gpt-5.5` | GPT-5.5 (**varsayılan**) | 272.000 |
| `gpt-5.4` | GPT-5.4 | 272.000 |
| `gpt-5.4-mini` | GPT-5.4-Mini | 272.000 |
| `gpt-5.2` | GPT-5.2 | 272.000 |
| `codex-auto-review` | Codex Auto Review | 272.000 |

Reasoning effort değerleri: `none` \| `minimal` \| `low` \| `medium` (varsayılan)
\| `high` \| `xhigh` \| `max` \| `ultra`.

---

## İlgili dokümanlar

- `70-CODEX-CLI-UYGULAMA-PLANI.md` — faz faz uygulama planı
- `17-*` — sağlayıcı soyutlaması, prompt-cache muhasebesi
- `51-CLAUDE-CONFIG-BIRLESIK.md` — per-workspace config evi deseni
- `52-MCP-GATEWAY.md` — iki-tier MCP köprüsü
- `40-*` — izin/onay katmanı
