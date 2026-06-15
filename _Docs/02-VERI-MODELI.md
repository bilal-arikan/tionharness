# SwarmGo — Veri Modeli

SwarmClaw'un SQLite şemasını temel alır. Veritabanı: **modernc.org/sqlite** (saf Go, CGO gerektirmez).

## Tablolar (ER Diyagramı)

```mermaid
erDiagram
    agents ||--o{ sessions : "sahip"
    sessions ||--o{ session_messages : "icerir"
    agents ||--o{ tasks : "sahip/atanan"
    agents ||--o{ schedules : "tetikler"
    tasks ||--o{ runs : "uretir"
    schedules ||--o{ runs : "tetikler"
    agents ||--o{ knowledge_sources : "hafiza"
    connectors ||--o{ sessions : "kaynak"

    agents {
        text id PK
        text name
        text soul
        text identity
        text provider
        text model
        text capabilities
        text planning_mode
        text dream_config
        int  created_at
    }
    sessions {
        text id PK
        text agent_id FK
        text kind
        int  message_count
        text state
        int  created_at
    }
    session_messages {
        text id PK
        text session_id FK
        text role
        text text
        text tool_calls
        text reasoning_content
        int  created_at
    }
    tasks {
        text id PK
        text title
        text owner_agent_id FK
        text board_state
        text execution_policy
        text execution_policy_state
        text workspace_path
        text dependencies
        int  created_at
    }
    schedules {
        text id PK
        text agent_id FK
        text cron_expr
        int  next_run_at
        text last_delivery_status
        text last_delivery_error
    }
    runs {
        text id PK
        text task_id FK
        text status
        text error
        text usage
        text run_state
        int  created_at
    }
    connectors {
        text id PK
        text platform
        text encrypted_key
        text routing_policy
        text health
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
| `agents` | Ajan tanımı: isim, ruh (soul), kimlik, sağlayıcı, model, yetenekler, planlama modu |
| `sessions` | Oturum: ajan/chatroom ilişkisi, mesaj sayısı, durum |
| `session_messages` | Tur geçmişi: rol, metin, araç çağrıları, akıl yürütme içeriği |
| `tasks` | Pano durumu, sahiplik, yürütme politikası, workspace yolu, bağımlılıklar |
| `schedules` | Cron/interval zamanlama, sonraki çalışma, son teslim durumu |
| `runs` | Yürütme kaydı: durum, hata, kullanım (token), restart için run state |
| `connectors` | Platform, şifreli kimlik, yönlendirme politikası, sağlık |
| `knowledge_sources` | Doküman, journal, reflection notları + embedding |
| `skills` | İsim, özet, etiket, canlı/taslak, kapsam |
| `mcp_servers` | İsim, taşıma (stdio/SSE/HTTP), env config |
| `provider_configs` | Ajan başına LLM override: model, endpoint, anahtar |

## Güvenlik / Şifreleme

- `encrypted_key` alanları **AES-GCM** ile şifrelenir.
- Şifre anahtarı çözümü: `CREDENTIAL_SECRET` env → `DATA_DIR/credential-secret` dosyası → otomatik üretim.
- Sırlar asla düz metin loglanmaz veya workspace manifestlerine yazılmaz.

## Ortam Değişkenleri

| Değişken | Açıklama |
|----------|----------|
| `SWARMGO_DATA_DIR` | Kalıcı durum dizini (varsayılan `~/.swarmgo`) |
| `SWARMGO_WORKSPACE_DIR` | Görev workspace kökü |
| `CREDENTIAL_SECRET` | Şifreleme anahtarı |
| `ACCESS_KEY` | Dashboard auth token (hosted dağıtım) |
| `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` ... | Sağlayıcı anahtarları |
| `DISCORD_BOT_TOKEN` / `SLACK_BOT_TOKEN` ... | Connector anahtarları |

## Migration Stratejisi

- Sürümlü SQL migration dosyaları: `internal/db/migrations/0001_init.sql`, ...
- Uygulama açılışında otomatik uygulanır (golang-migrate veya basit kendi runner).
- Şema değişiklikleri geriye dönük uyumlu tutulur.
