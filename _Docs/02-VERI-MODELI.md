# SwarmGo — Veri Modeli

> ⚠️ **GÜNCEL (2026-06-15):** Depolama SQLite'tan **dosya sistemine** taşındı. Aşağıdaki
> entity'ler ve ilişkiler **kavramsal olarak geçerli**, ancak artık SQL tabloları değil
> entity-başına **JSON dosyaları** (oturumlar `session.jsonl`) olarak saklanıyor. Diske
> yazım biçimi, dizin yapısı ve eşzamanlılık için: **`_Docs/08-DEPOLAMA.md`**.

Entity modeli SwarmClaw'un şemasından türetilmiştir (başta SQLite tabloları olarak tasarlandı, sonra dosya-store'a taşındı).

## Tablolar (ER Diyagramı)

```mermaid
erDiagram
    agents ||--o{ sessions : "sahip"
    sessions ||--o{ session_messages : "icerir"
    agents ||--o{ tasks : "sahip/atanan"
    agents ||--o{ schedules : "tetikler"
    tasks ||--o{ runs : "uretir"
    agents ||--o{ knowledge_sources : "hafiza"
    agents ||--o{ agent_usage : "kullanim"
    flows ||--o{ flow_runs : "uretir"

    agents {
        text id PK
        text name
        text soul
        text identity
        text provider
        text model
        text capabilities
        text planning_mode
        text avatar
        text color
        int  heartbeat_enabled
        int  heartbeat_interval_sec
        text heartbeat_prompt
        int  daily_call_limit
        int  daily_token_limit
        int  mcp_enabled
        text allowed_tools
        int  created_at
        int  updated_at
    }
    sessions {
        text id PK
        text agent_id FK
        text kind
        text title
        int  message_count
        text state
        text summary
        int  summary_msg_count
        int  created_at
        int  updated_at
    }
    agent_usage {
        text agent_id FK
        text day
        int  calls
        int  input_tokens
        int  output_tokens
    }
    session_messages {
        text id PK
        text session_id FK
        text agent_id FK
        text role
        text text
        text tool_calls
        text reasoning_content
        text steps
        int  created_at
    }
    tasks {
        text id PK
        text title
        text description
        text prompt
        text owner_agent_id FK
        text flow_id FK
        text board_state
        text dependencies
        text last_run_id
        text last_run_status
        int  last_run_at
        text created_by
        int  created_at
        int  updated_at
    }
    schedules {
        text id PK
        text agent_id FK
        text cron_expr
        text prompt
        int  next_run_at
        int  last_run_at
        text last_delivery_status
        text last_delivery_error
        int  enabled
        int  created_at
    }
    runs {
        text id PK
        text task_id FK
        text agent_id FK
        text status
        text trigger
        text output
        text error
        int  created_at
        int  updated_at
    }
    knowledge_sources {
        text id PK
        text agent_id FK
        text kind
        text content
        blob embedding
        int  created_at
    }
    skills {
        text id PK
        text name
        text summary
        text tags
        int  is_live
        text scope
    }
    mcp_servers {
        text id PK
        text name
        text transport
        text env_config
        text command
        text args
        text url
        int  enabled
        text scope
    }
    flows {
        text id PK
        text name
        text graph
        int  created_at
        int  updated_at
    }
    flow_runs {
        text id PK
        text flow_id FK
        text status
        text input
        text output
        text state
        text error
        int  created_at
        int  updated_at
    }
    provider_configs {
        text id PK
        text agent_id FK
        text model
        text endpoint
        text encrypted_key
    }
```

## Tablo Açıklamaları

| Tablo | Sorumluluk |
|-------|-----------|
| `agents` | Ajan tanımı: soul, kimlik, sağlayıcı, model, planlama modu; **heartbeat** ayarları; **günlük bütçe limitleri** (`daily_call_limit`/`daily_token_limit`); **araç ayarları** (`mcp_enabled`/`allowed_tools` allowlist) |
| `sessions` | Oturum: ajan ilişkisi, başlık, mesaj sayısı, durum; **compaction** özeti (`summary` + `summary_msg_count`) |
| `session_messages` | Tur geçmişi: rol, metin, araç çağrıları, akıl yürütme içeriği, aktivite izi (`steps`); **`agent_id`** = turu üreten ajan (çok-ajanlı oturumda mesaj başına ajan) |
| `agent_usage` | Ajan başına gün bazlı kullanım sayacı (çağrı + giriş/çıkış token) — bütçe guardrail'i için |
| `tasks` | Pano durumu (`board_state`), sahiplik, ajana verilen `prompt`, son çalışma özeti, bağımlılıklar. **`flow_id`** dolu ise görev "flow-backed" — çalıştırılınca ajana prompt yerine o orchestration akışı koşar. **`created_by`** = görevi oluşturan ajan ("" = kullanıcı; ajan yalnız kendi oluşturduğunu silebilir) |
| `schedules` | Cron zamanlama; ajana doğrudan `prompt` teslimi (panodan bağımsız — görev çalıştırmaz); sonraki/son çalışma + teslim durumu; etkin mi. **`expires_at`** dolu ise (opsiyonel son tarih, unix saniye) o tarihten sonra zamanlama çalışmaz ve otomatik pasifleşir (0 = süresiz) |
| `runs` | Yürütme kaydı: durum, tetikleyici (`trigger`), ajan çıktısı (`output`), hata |
| `knowledge_sources` | Doküman, journal, reflection notları + embedding |
| `mcp_servers` | İsim, taşıma (stdio; SSE/HTTP henüz yok), `command`/`args`/`url`, env config, `enabled`, `scope` (workspace) |
| `flows` | Akış tanımı: `graph` (JSON `orchestration.Graph` — agent/branch/parallel node) |
| `flow_runs` | Akış yürütmesi: durum, girdi/çıktı, **restart-safe** `state` (her node sonrası persist), hata |
| `artifacts` | Ajanın ürettiği kalıcı, sürümlenen içerik (doküman/kod/HTML/metin/SVG/Mermaid). `kind`+`language`, güncel `content`/`version` ve **inline revizyon geçmişi** (`revisions`, eskiden yeniye); köken `session_id`/`agent_id`. Workspace-scoped — SwarmGo'nun Claude.ai artifact karşılığı |

## Güvenlik / Şifreleme

- `encrypted_key` alanları **AES-GCM** ile şifrelenir.
- Şifre anahtarı çözümü: `CREDENTIAL_SECRET` env → `DATA_DIR/credential-secret` dosyası → otomatik üretim.
- Sırlar asla düz metin loglanmaz veya workspace manifestlerine yazılmaz.

## Ortam Değişkenleri

| Değişken | Açıklama |
|----------|----------|
| `SWARMGO_ADDR` | HTTP dinleme adresi (varsayılan loopback `127.0.0.1:8080`; geliştirmede `127.0.0.1:8090`; ağa açmak için `0.0.0.0:8090`). Loopback, Windows Güvenlik Duvarı'nın izin sormasını önler |
| `SWARMGO_DATA_DIR` | Kalıcı durum dizini (varsayılan `~/.swarmgo`) |
| `SWARMGO_WORKSPACE_DIR` | Görev workspace kökü |
| `SWARMGO_MAX_CONTEXT_TOKENS` | Bağlam sıkıştırma eşiği (varsayılan 12000) |
| `SWARMGO_KEEP_RECENT_MSGS` | Sıkıştırmada korunan son mesaj sayısı (varsayılan 8) |
| `CREDENTIAL_SECRET` | Şifreleme anahtarı |
| `ACCESS_KEY` | Dashboard auth token (hosted dağıtım) |
| `ANTHROPIC_API_KEY` | `anthropic` sağlayıcı anahtarı (claude-cli'da gerekmez) |

## Şema Evrimi (Migration yok)

> Dosya-tabanlı depolamada **SQL migration kavramı yoktur** — şema yoktur, her kayıt
> bir JSON dosyasıdır. Yeni alanlar Go model struct'ına eklenir; eski JSON dosyaları
> okunurken eksik alanlar Go'nun sıfır değerleriyle doldurulur (geriye dönük uyumlu).
> Geçmişte (SQLite döneminde) `0001_init` … `0007_message_steps` migration'larıyla
> eklenen alanlar bugün ilgili model struct'larında yaşar:

- **Ana entity'ler** (eski `0001_init`): agents, sessions, session_messages, tasks,
  schedules, runs, knowledge_sources, mcp_servers — `models*.go`.
- **Heartbeat** (eski `0002`): `Agent.Heartbeat*` alanları.
- **Tasks/Schedules** (eski `0003`): `Task.Prompt/LastRun*`, `Schedule.TaskID/Prompt`, `Run.Output/Trigger`.
- **Context/Budget** (eski `0004`): `Session.Summary*`, `Agent.Daily*Limit`, `Usage` (gün-bazlı dosya).
- **MCP/Tools** (eski `0005`): `MCPServer.Command/Args/URL/Enabled/Scope`, `Agent.MCPEnabled/AllowedTools`.
- **Flows** (eski `0006`): `Flow.Graph`, `FlowRun` (restart-safe `State` JSON, status/input/output/error).
- **Message steps** (eski `0007`): `Message.Steps` (JSON `[]TurnStep`: zengin sohbet tur izi — thinking/ara metin/tool çağrıları).

Diske yazım ve dizin yapısı için: **`_Docs/08-DEPOLAMA.md`**.
