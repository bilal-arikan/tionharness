# 24 — Self-Management + Ayarlar Alt Sistemi

Bir ajanın **SwarmGo'nun kendisini** araçlarla yönetmesini sağlayan tool ailesi ve
uygulama-geneli ayarların canlı okunup yazıldığı ayarlar köprüsü.

> İlgili: araç-use döngüsü için `_Docs/01-MIMARI.md`, lazy yükleme için
> `_Docs/19-LAZY-TOOL-LOADING.md`, provenance (`created_by`) konvansiyonu için
> `_Docs/02-VERI-MODELI.md`, hook'lar için `_Docs/18-HOOKS.md`,
> spawn için `_Docs/22-SPAWN-SESSION.md`.

## Gating + lazy yükleme

- **Tek anahtar:** tüm öz-yönetim araçları yalnız `SelfManageEnabled()` true ise
  kaydedilir (env `SWARMGO_ENABLE_SELFMANAGE=1`). Kapalıyken hiç eklenmez (~+2000
  tok/tur tasarrufu).
- **Hepsi lazy:** aile geniş ve turların azında kullanıldığı için şemalar her tura
  basılmaz; ajan "Available Tools (load on demand)" listesinden gerekeni
  `activate_tools` / `tool_search` ile **kendisi yükler** (`toolsetup.go`'da
  `selfManageStart`'tan sonrası `MarkLazy`). MCP araçları da `AttachMCP` içinde lazy.
- **Koşullu alt-aileler:** secret yazımı yalnız vault varsa; skill yazımı yalnız
  skill store varsa; ayar araçları yalnız `settingsBridge` bağlıysa eklenir.

Öğretici default skill: **`swarmgo-self-management`** (`access: shared`,
`swarmgo-guide`'a subskill) — aileyi kataloglar ve self-aktivasyon akışını öğretir.

**claude-cli yolu (CLI-3):** CLI'nin native `activate_tools` döngüsü olmadığından
bu lazy aile, Interaction MCP **köprüsü** ile CLI ajanlarına önden advertise edilip
native registry üzerinden dispatch edilir (`Runtime.BridgeTools` →
`Registry.BridgeableDefs`; backend `Tools(token)` + `Call` default). Per-ajan
`toolFilter` ve self-manage gate'i CLI'de de aynen geçerli. Detay:
`_Docs/11-INTERACTION-MCP.md`.

## Provenance guard'ı (`created_by`)

Ajan **yalnızca kendi oluşturduğu** (`CreatedBy` dolu) entity'leri silebilir;
kullanıcı varlıklarına (`CreatedBy == ""`) dokunamaz. Kapsam: agents, tasks,
schedules, flows, hooks, mcp_servers, **workspaces** (`Meta.CreatedBy`). Detay:
`_Docs/02-VERI-MODELI.md`.

> Workspace silmede iki ek guard daha var: ajan **mevcut çalıştığı** workspace'i
> silemez (kendini çalıştığı zeminden çıkaramaz) ve manager **son kalan**
> workspace'i silmeyi reddeder (her zaman ≥1 workspace olmalı).

## Araç ailesi

| Alan | Araçlar | Notlar |
|------|---------|--------|
| **Agents** | `create_agent` / `update_agent` / `delete_agent` / `list_agents` | create canlı `Start`, delete `Stop`; delete provenance-guard'lı. **Cascade:** delete ajanın session'larını + bağlı schedule'larını (`AgentID`) + sahip olduğu task'ları (`OwnerAgentID`) ve run'larını da siler, ardından `reloadSchedules` ile cron registry'sini tazeler |
| **Ajan delegasyonu** | `run_subagent` | izole işçi başlat (built-in profil: `explore`/`coder`/`reviewer`; ya da mevcut ajan adı/id). `wait:"sync"` (varsayılan, cevabı bekle) \| `"async"` (detached arka plan). Tek turda birden çok çağrı paralel koşar. `enableDelegation` ile gated. Bkz. `25-SUBAGENT-ISOLATION.md` |
| **Flows** | `create_flow` / `update_flow` / `delete_flow` / `list_flows` / `get_flow` / `run_flow` | `run_flow` otonom, bütçe-gated, executions feed'ine kaydeder |
| **Schedules** | `create_schedule` / `update_schedule` / `delete_schedule` / `list_schedules` | her yazım `reloadSchedules` ile scheduler'ı tazeler |
| **Tasks (kanban)** | `list_tasks` / `create_task` / `update_task` / `move_task` / `delete_task` | her görevde okuma/edit/move; **delete yalnız ajan-oluşturduğu**. Pano pasif — run yok |
| **Hooks** | `list_hooks` / `create_hook` / `delete_hook` | delete provenance-guard'lı. Bkz. `18-HOOKS.md` |
| **MCP sunucuları** | `list_mcp_servers` / `create_mcp_server` / `toggle_mcp_server` / `delete_mcp_server` | yeni/etkin sunucunun araçları **sonraki turda** görünür; delete provenance-guard'lı, toggle her sunucuda |
| **Workspaces** | `list_workspaces` / `create_workspace` / `rename_workspace` / `delete_workspace` | **çapraz-workspace**: `WorkspaceBridge` köprüsünden (manager). list/create/rename her workspace'te; **delete yalnız ajan-oluşturduğu** (`Meta.CreatedBy`), **mevcut çalıştığı** workspace'i ve **son kalan** workspace'i silemez. create blank şablonla tohumlanır; değişiklik UI'a `workspaces` SSE event'i yayar |
| **Artifacts** | `delete_artifact` / `list_artifacts` | create/update zaten tur-başı sink ile sağlanır |
| **Memory** | `memory_add` | recall her zaman açık (`memory_recall`) |
| **Loglar** | `read_logs` | ring-buffer log okuma |
| **Secret kasası** | `secret_set` / `secret_delete` | yazım gated; okuma (`secret_list` / `secret_get`) **her zaman açık** (`builtin_secret.go`), per-workspace AES-GCM vault |
| **Skills** | `create_skill` / `delete_skill` | yalnız **workspace-tier**; global/bundled skill store tarafından korumalı. `use_skill` ile sonraki turdan yüklenebilir. **Cascade:** delete sonrası `RemoveSkillFromAgents` slug'ı tüm ajanların `Skills` listesinden düşürür (dangling referans kalmaz) |
| **App-ayarları** | `get_settings` / `update_settings` | aşağıdaki ayarlar köprüsü; secret alanları maskeli |

> Kaynaklar: `internal/tools/builtin_{agentmgmt,flowmgmt,schedulemgmt,taskmgmt,hookmgmt,mcpmgmt,workspacemgmt,artifactmgmt,memory_add,logs,secretmgmt,skillmgmt,settings}.go`,
> `internal/tools/subagent.go` (`run_subagent` tool def), `internal/agent/subagent.go` (çekirdek). Workspace köprüsü: `internal/api/workspace_bridge.go`.
> Guard testleri: `builtin_controlgaps_test.go`, `builtin_taskmgmt_test.go`,
> `builtin_workspacemgmt_test.go`, `builtin_settings_test.go`.

### Workspace köprüsü (bridge) wiring'i

Workspace araçları **çapraz-workspace** olduğundan (mevcut DB'nin dışına çıkar)
ayarlar gibi bir köprüden geçer:

```
tools.WorkspaceBridge (arayüz)
  → api.workspaceBridge (workspace.Manager + seedTemplate + publishWorkspacesChanged'i sarar)
  → Server.WorkspaceBridge()
  → Manager.SetWorkspaceBridge (mevcut + sonraki tüm workspace Runtime'larına dağıtır)
  → Runtime.workspaceBridge (+ SetWorkspaceBridge)
```

`main.go` server kurulumundan sonra `SetSettingsBridge`'in hemen yanında bağlar.
Köprü nil iken (server'dan önce) araçlar hiç eklenmez. Araç katmanı provenance +
"mevcut/son silinemez" guard'larını taşır; manager `Delete` son-workspace guard'ını.

## Ayarlar alt sistemi (settings)

Uygulama-geneli ayarlar `settings.json` belgesinde tutulur (şifreli secret
alanlarıyla). Ajan bunu canlı okuyup yazabilir; değişiklik **restart olmadan**
çalışan runtime'a uygulanır.

### Araçlar

- **`get_settings`** → `settings.json` dosya yolunu (`Store.Path()`) + güncel
  ayarları **maskeli** JSON döner (secret key'ler yalnız "set mi" olarak görünür).
  Alan adları `update_settings` patch'inin anahtarlarıdır.
- **`update_settings`** → yalnız değişen alanları içeren bir `patch` alır, diske
  yazar **ve canlı uygular**. Sayısal alanlar güvenli aralığa **clamp**'lenir;
  write-only secret alanları (`anthropicKey`/`minimaxKey`, "" = temizle) kabul edilir.

Alanların tam referansı: default skill **`swarmgo-settings`** (`access: shared`,
`swarmgo-guide`'a subskill).

### Validation + normalize (`internal/settings/validate.go`)

- `Validate(Patch)` enum/format alanlarını denetler (theme, language,
  defaultProvider, defaultPermissionMode, logLevel, accent hex) ve geçersizleri
  **açık hata mesajıyla reddeder**. `Apply` en başta çağırır → hatalı değişiklik
  canlı alt sistemlere hiç ulaşmaz.
- Sayısal alanlar reddedilmez, `normalize` ile clamp'lenir.
- `normalize` güvenlik ağı: geçersiz `accent` hex → varsayılan; bilinmeyen
  `defaultPermissionMode` → `auto`. Elle bozulmuş `settings.json` bile uygulamayı
  çökertmez.
- HTTP `PUT /api/settings` validation hatasında **400**, encryption hatasında 500.

### Köprü (bridge) wiring'i

```
tools.SettingsBridge (arayüz)
  → api.settingsBridge (settings store + applySettings hook'unu sarar)
  → Server.SettingsBridge()
  → Manager.SetSettingsBridge (mevcut + sonraki tüm workspace Runtime'larına dağıtır)
  → Runtime.settingsBridge (+ SetSettingsBridge)
```

`main.go` server kurulumundan sonra bağlar. Dosyalar: `internal/api/settings_bridge.go`,
`internal/settings/store.go` (`Path()`, `Apply` validation çağrısı).

### Canlı yenileme + çok-pencere senkronu

- Bir ajan (veya UI) ayar değiştirince `Apply` `/api/events` üzerinden bir
  **`settings`** SSE event'i yayınlar (app-global → workspace rozeti/toast yok).
- UI yolu (`PUT /api/settings`) ve bridge ortak `Server.publishSettingsChanged`
  helper'ını çağırır (tek kaynak, tekrar yok).
- `App.tsx onEvent`: client-side prefs'i (tema/accent/bildirim) **canlı uygular**
  (`applyClientPrefs`) + `settingsNonce`'u artırır. `SettingsPanel` `reloadNonce`
  prop'u ile — **yalnız dirty değilse** formu yeniden yükler (eşzamanlı değişiklik
  kullanıcının yazdığını ezmez). Sonuç: bir pencerede/ajanda yapılan değişiklik
  diğer tüm açık pencerelerde canlı yansır.

## Durum

✅ `go build/vet/test ./...` + `tsc -b`/`vite build` yeşil. Guard + validation
testleri mevcut (`builtin_controlgaps_test.go`, `validate_test.go`,
`builtin_settings_test.go`).
