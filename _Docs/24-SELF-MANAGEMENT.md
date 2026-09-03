# 24 — Self-Management + Ayarlar Alt Sistemi

> **Özet (2026-09-03):** Bir ajanın TionHarness'in kendisini (agents, flows,
> schedules, hooks, MCP sunucuları, workspaces, tasks, skills, app-ayarları)
> araçlarla yönetmesini sağlayan geniş tool ailesi + canlı ayarlar köprüsü. Durum:
> uygulanmış ve olgun. En önemli kararlar: provenance guard'ı **workspace hariç**
> tüm entity türlerinden kaldırıldı (ajan artık kullanıcı-oluşturduğu varlıkları da
> düzenleyip silebilir; workspace silme yıkıcılığı yüzünden istisna kaldı), tüm
> self-management araçları tek `SelfManageEnabled()` bayrağıyla kapatılıp lazy
> yüklenir, `update_settings` değişikliği restart olmadan canlı uygular. Dayandığı
> dosyalar: `internal/tools/builtin_{agentmgmt,flowmgmt,schedulemgmt,taskmgmt,
> hookmgmt,mcpmgmt,workspacemgmt,artifactmgmt,skillmgmt,settings}.go`,
> `internal/settings/validate.go`.

Bir ajanın **TionHarness'in kendisini** araçlarla yönetmesini sağlayan tool ailesi ve
uygulama-geneli ayarların canlı okunup yazıldığı ayarlar köprüsü.

> İlgili: araç-use döngüsü için `_Docs/01-MIMARI.md`, lazy yükleme için
> `_Docs/19-LAZY-TOOL-LOADING.md`, provenance (`created_by`) konvansiyonu için
> `_Docs/02-VERI-MODELI.md`, hook'lar için `_Docs/18-HOOKS.md`,
> spawn için `_Docs/22-SPAWN-SESSION.md`.

## Gating + lazy yükleme

- **Tek anahtar:** tüm öz-yönetim araçları yalnız `SelfManageEnabled()` true ise
  kaydedilir (env `TIONHARNESS_ENABLE_SELFMANAGE=1`). Kapalıyken hiç eklenmez (~+2000
  tok/tur tasarrufu).
- **Hepsi lazy:** aile geniş ve turların azında kullanıldığı için şemalar her tura
  basılmaz; ajan "Available Tools (load on demand)" listesinden gerekeni
  `activate_tools` / `tool_search` ile **kendisi yükler** (`toolsetup.go`'da
  `selfManageStart`'tan sonrası `MarkLazy`). MCP araçları da `AttachMCP` içinde lazy.
- **Koşullu alt-aileler:** secret yazımı yalnız vault varsa; skill yazımı yalnız
  skill store varsa; ayar araçları yalnız `settingsBridge` bağlıysa eklenir.

Öğretici default skill: **`tionharness-self-management`** (`access: shared`,
`tionharness-guide`'a subskill) — aileyi kataloglar ve self-aktivasyon akışını öğretir.

**claude-cli yolu (CLI-3):** CLI'nin native `activate_tools` döngüsü olmadığından
bu lazy aile, Interaction MCP **köprüsü** ile CLI ajanlarına önden advertise edilip
native registry üzerinden dispatch edilir (`Runtime.BridgeTools` →
`Registry.BridgeableDefs`; backend `Tools(token)` + `Call` default). Per-ajan
`toolFilter` ve self-manage gate'i CLI'de de aynen geçerli. Detay:
`_Docs/11-INTERACTION-MCP.md`.

## Provenance guard'ı KALDIRILDI — workspace HARİÇ (2026-08-05)

Eskiden ajan **yalnızca kendi oluşturduğu** (`CreatedBy` dolu) entity'leri
düzenleyip silebilir, kullanıcı varlıklarına (`CreatedBy == ""`) dokunamazdı.
**Bu engel kaldırıldı:** ajan artık agents, flows, schedules, automations,
hooks, mcp_servers, artifacts ve tasks dahil **her varlığı** (kullanıcı- veya
ajan-oluşturduğu) düzenleyip silebilir. `CreatedBy`/`AgentID` bu türlerde yalnız
köken/görüntüleme için tutulur — silme/düzenleme kapısını artık kapatmaz.

> **Tek istisna — workspaces:** Workspace silme benzersiz derecede yıkıcıdır
> (bir workspace'in TÜM agent/session/secret/dosyalarını geri dönüşsüz siler),
> bu yüzden köken guard'ı **workspace silmede korunur**: ajan yalnız
> **kendi oluşturduğu** (`Meta.CreatedBy` dolu) workspace'i silebilir, kullanıcı
> workspace'ine dokunamaz.

> Workspace silmede ayrıca iki operasyonel guard daha var: ajan **mevcut
> çalıştığı** workspace'i silemez ve manager **son kalan** workspace'i silmeyi
> reddeder. Ayrıca ajan **kendini** silemez (`delete_agent`).

## Araç ailesi

| Alan | Araçlar | Notlar |
|------|---------|--------|
| **Agents** | `create_agent` / `update_agent` / `delete_agent` / `list_agents` | create canlı `Start`, delete `Stop`; edit/delete köken filtresi YOK (ajan kendini silemez). **Provider mirası (2026-08-06):** `create_agent` provider'ı boşsa **oluşturan ajanın** provider+model'i eşleşik çift olarak devralınır (koordinatör deepseek'te → alt-ajanlar da deepseek), yoksa `claude-cli`'ye düşer; model yalnız hem provider hem model boşken miras alınır (kaldırılan soyut workspace-default yerine agent-bazlı yaklaşım). **Cascade:** delete ajanın session'larını + bağlı schedule'larını (`AgentID`) + sahip olduğu task'ları (`OwnerAgentID`) ve run'larını da siler, ardından `reloadSchedules` ile cron registry'sini tazeler |
| **Ajan delegasyonu** | `run_subagent` | izole işçi başlat (built-in profil: `explore`/`coder`/`reviewer`; ya da mevcut ajan adı/id). **Daima senkrondur:** çağrı alt-ajan bitene kadar bekler ve final cevabını döndürür; detached/arka plan modu yoktur (arka plan işi için görevi birkaç küçük çağrıya böl ya da koordinatör modu + `spawn_worker`). Şemadaki `wait` alanı tek sürümlük geçiş için hâlâ kabul edilir ama kullanımdan kaldırılmış no-op'tur: `wait:"sync"`/verilmemesi çalışır, `wait:"async"` hata döndürür. Koşan bir alt-ajanı durduran araç yoktur; koordinatörün `stop_worker`'ı **ayrı bir araçtır** ve etkilenmedi. Tek turda birden çok çağrı paralel koşar. **Yapılandırılmış görev sözleşmesi (2026-06-25):** opsiyonel `objective`/`output_format`/`boundaries` alanları subagent system-prompt'una "Task contract" bloğu olarak enjekte edilir (iş tekrarı/boşluğu önler; verilmezse eski düz-`task` davranışı). Daima kurulu (2026-07-02'den beri gate yok; görünürlük araç-bazlı Araçlar ekranından). Bkz. `25-SUBAGENT-ISOLATION.md` |
| **Peer mesajlaşma** | `send_message` | başka ajana **adresli DM** (`{to, message, summary?}`); alıcının kalıcı **inbox** oturumuna `<agent_message from="…">` etiketiyle düşer, alıcı arka planda geçmiş-duyarlı turla işler (fire-and-forget, `SpawnMaxConcurrent` guard, kendine-mesaj reddi). run_subagent (sonuç-odaklı) yanında "süregelen işbirliği" yolu. Bkz. `28-PEER-MESAJLASMA-PLANI.md` |
| **Flows** | `create_flow` / `update_flow` / `delete_flow` / `list_flows` / `get_flow` / `run_flow` | `run_flow` otonom, bütçe-gated, executions feed'ine kaydeder. `update_flow` `tags` alanı da alır (2026-07-04'te `set_flow_tags` bununla birleşti; ayrı `SetFlowTags` ile persist) |
| **Schedules** | `create_schedule` / `update_schedule` / `delete_schedule` / `list_schedules` / `run_schedule` | her yazım `reloadSchedules` ile scheduler'ı tazeler; `run_schedule` bir zamanlamayı **şimdi** elle tetikler (`runScheduleNow`). `update_schedule` `tags` alanı da alır (2026-07-04'te `set_schedule_tags` bununla birleşti) |
| **Tasks (kanban)** | `list_tasks` / `get_task` / `create_task` / `update_task` / `move_task` / `set_archived_task` / `delete_task` | her görevde okuma/edit/move/arşiv/delete (köken filtresi yok). Pano pasif — run yok. `list_tasks` aktif ve arşiv kartlarını karıştırmaz (`archived:true` yalnız arşiv); `get_task` tam kartı döndürür. `artifactIds` list/create/update tarafından desteklenir. |
| **Hooks** | `list_hooks` / `create_hook` / `update_hook` / `delete_hook` | update/delete köken filtresi YOK (her hook değiştirilebilir/silinebilir). Bkz. `18-HOOKS.md` |
| **MCP sunucuları** | `list_mcp_servers` / `create_mcp_server` / `toggle_mcp_server` / `delete_mcp_server` | yeni/etkin sunucunun araçları **sonraki turda** görünür; delete/toggle her sunucuda (köken filtresi yok) |
| **Workspaces** | `list_workspaces` / `create_workspace` / `rename_workspace` / `delete_workspace` | **çapraz-workspace**: `WorkspaceBridge` köprüsünden (manager). list/create/rename her workspace'te; **delete yalnız ajan-oluşturduğu** (`Meta.CreatedBy`) — köken guard'ı **burada korunur** (benzersiz yıkıcı) —, **mevcut çalıştığı** workspace'i ve **son kalan** workspace'i silemez. create blank şablonla tohumlanır; değişiklik UI'a `workspaces` SSE event'i yayar |
| **Artifacts** | `delete_artifact` / `list_artifacts` / `read_artifact` | create/update zaten tur-başı sink ile sağlanır; `list_artifacts` artık `contentFile` yolunu da döndürür; `read_artifact` ID ile içeriği döndürür (dosya yolu tahmin etmeye gerek yok) |
| ~~**Memory**~~ | ~~`memory_add` / `memory_recall`~~ | **KALDIRILDI (2026-07-05)** — memory alt sistemi tamamen çıkarıldı |
| **Oturum / handoff** | `handoff_session` | bağlam sınırına yaklaşan oturumu **temiz pencerede** sürdürür: handoff artifact yazıp child oturum açar (name-only tier'a terfi etti). Bkz. `35-CONTEXT-RESET-HANDOFF.md` |
| **Oturum yönetimi (workspace-scoped)** | `list_sessions` / `update_session` / `archive_sessions` | `update_session` yalnız **bu** oturumu düzenler (title/working_dir/tags/**archive**). `archive_sessions` **başka** oturumları toplu arşivler (soft, geri alınabilir): **`kinds`** (oturum tipi) + `idle_days` (N günden eski) + `title_contains` filtreleri, `dry_run` önizleme, `exclude` listesi; **daima `r.db`'ye bağlı → fiziksel olarak bu workspace'e scope'lu** ve **mevcut oturumu varsayılan olarak hariç tutar** (registry-build anında `SessionIDFrom(ctx)`); `include_current:true` ile ajan **kendi oturumunu da** arşivleyebilir (soft/geri alınabilir, çalışan tur durmaz). **`kinds` (2026-07-13):** geçerli tipler `chat`, `spawned`, `worker`, `flow`, `task`, `schedule`, `inbox` — ya da hepsi için `["*"]`. **Verilmezse varsayılan `["chat"]`** (geri uyumluluk: araç eskiden tipi sabit `chat` olarak filtreliyordu, dolayısıyla mevcut "eski oturumları temizle" çağrıları birebir aynı davranır). Bilinmeyen bir tip **sessizce yutulmaz, hata döner**. Otonom çalıştırmaların (spawn/flow/task/schedule/inbox) bıraktığı oturumlar eskiden **hiçbir toplu araçla** arşivlenemiyordu — `kinds` bu boşluğu kapatır. ⚠️ `schedule` ve `inbox` oturumları uzun ömürlü/sistemseldir; `["*"]` bunları da süpürür → önce `dry_run` ile önizle (`dry_run` çıktısı her satırda oturum tipini `[flow]` gibi gösterir). Tip mantığı ayrı dosyada: `builtin_sessionkinds.go`. Bu, ajanın "eski oturumları temizle" isteğinde ham REST'e (`Invoke-RestMethod /api/sessions/{id}/state`) düşüp workspace scope'unu kaybetmesini önler — bir kez yanlış (default) workspace'i arşivleyen tam da o footgun'du. Kaynak: `builtin_sessionarchive.go`, `builtin_sessionupdate.go`, `builtin_sessions.go`. **Her zaman aktif** (cross-session farkındalığı toggle'ı kaldırıldı) |
| **Loglar** | `read_logs` | ring-buffer log okuma |
| **Secret kasası** | `secret` (`action: list\|get\|set\|delete`) | tek araç (2026-07-04'te `secret_list`/`_get`/`_set`/`_delete` birleşti, `builtin_secret.go`); vault varken kayıtlı, hidden görünürlük, per-workspace AES-GCM vault |
| **Skills** | `create_skill` / `update_skill` / `delete_skill` / `import_skill` | `import_skill` bir **Claude Code skill'ini** `source:"local"` (klasör yolu) veya `source:"github"` (github.com folder URL) içe aktarır — CC frontmatter eşler (allowed-tools→always_allow, paths, version/license/source), bundled dosya kopyalar, uyumsuz CC özelliklerini (context:fork/hooks/slash-arg) uyarıyla ayıklar (`internal/skills/import.go`+`github.go`, `POST /api/skills/import`). yalnız **workspace-tier**; global/bundled skill store tarafından korumalı. `use_skill` ile yüklenir, `skill_search` ile bulunur (koşullu/on-demand skill'ler katalogda olmasa da). `update_skill` slug ile in-place + partial (yalnız verilen alan değişir). Frontmatter: `paths:` (koşullu, SK-2), `always_allow:` (yüklerken oturuma auto-grant, SK-3), `${SKILL_DIR}`+ek dosyalar (SK-1), `version`/`source_url`/`license` (SK-4). **Değişiklik sinyali:** store, çözümlenmiş katalog fingerprint'i gerçekten değiştiğinde workspace SSE akışına `skills` kontrol olayı yollar; açık Skills paneli liste+detayı tazeler, kapalıysa navbar butonu okunmamış noktasını gösterir. İlk/no-op reload olay üretmez; kontrol olayı OS bildirimi değildir. **Cascade:** delete sonrası `RemoveSkillFromAgents` slug'ı tüm ajanların `Skills` listesinden düşürür (dangling referans kalmaz) |
| **App-ayarları** | `get_settings` / `update_settings` | aşağıdaki ayarlar köprüsü; secret alanları maskeli |
| **Provider'lar** | `list_providers` (**yalnız okuma**) | yapılandırılmış provider **instance**'larını listeler: `id`, `kindId`, `label`, `enabled`, `defaultModel`, `models`, `available`. API anahtarı **hiç** dönmez (`Registry.ListInstances` yalnız kimlik-bilgisiz özet üretir; devre dışı instance'lar da listelenir). Dönen `id`, `create_agent`/`update_agent` içindeki `provider` alanına verilecek instance kimliğidir. **Oluşturma/düzenleme/silme aracı YOK** — bunlar yalnız Ayarlar ekranı + `PUT`/`DELETE /api/providers` üzerinden yapılır (`internal/tools/builtin_providermgmt.go`) |

> Kaynaklar: `internal/tools/builtin_{agentmgmt,flowmgmt,schedulemgmt,taskmgmt,hookmgmt,mcpmgmt,workspacemgmt,artifactmgmt,logs,secret,skillmgmt,settings}.go`,
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
Köprü nil iken (server'dan önce) araçlar hiç eklenmez. Araç katmanı workspace
köken guard'ını + "mevcut/son silinemez" guard'larını taşır (köken filtresi yalnız
**workspace** için korundu, diğer self-management türlerinde kaldırıldı); manager
`Delete` son-workspace guard'ını.

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

Alanların tam referansı: default skill **`tionharness-settings`** (`access: shared`,
`tionharness-guide`'a subskill).

### Validation + normalize (`internal/settings/validate.go`)

- `Validate(Patch)` enum/format alanlarını denetler (theme, language,
  defaultPermissionMode, logLevel, accent hex) ve geçersizleri
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
