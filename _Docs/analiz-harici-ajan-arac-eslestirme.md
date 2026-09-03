# Analiz: External Agent `session-tools-core` Araç Envanteri ve TionHarness Eşleştirmesi

> **Özet (2026-09-03):** Salt-okuma karşılaştırma analizidir (kod değiştirilmedi) — External Agent OSS'in 25 `session-tools-core` aracını TionHarness'in ~80 builtin aracıyla eşleştirir. Sonuç: iki ürün farklı felsefede (External Agent oturum-yardımcısı, TionHarness multi-agent platform yönetimi), örtüşme `spawn_session`/`list_sessions`/mesajlaşmada yoğunlaşıyor. En değerli boşluklar `config_validate`, `skill_validate`, `mermaid_validate` doğrulama araçları ile `transform_data`/`render_template`/ucuz `call_llm` katmanı olarak belirlenmiş; bunların aksiyon karşılığı ayrı `41-ARAC-BOSLUKLARI-YAPILACAKLAR.md` dosyasındadır ve bazıları (config_validate, skill_validate, mermaid_validate) bu analizden sonra zaten eklenmiştir.

> Kaynaklar: External Agent OSS (`external-agent-project/external-agent-oss`, `main`) — tek kaynak dosyası
> `packages/session-tools-core/src/tool-defs.ts` (`SESSION_TOOL_DEFS`). TionHarness (yerel) —
> `internal/tools/builtin_*.go` ve interaction köprüsü (`internal/api/mcp_interaction.go`).
> Analiz salt-okuma yapılmıştır; TionHarness kodu değiştirilmemiştir.
>
> **GÜNCELLEME:** Aksiyon (yapılacaklar) karşılığı artık ayrı dosyada:
> [`41-ARAC-BOSLUKLARI-YAPILACAKLAR.md`](./41-ARAC-BOSLUKLARI-YAPILACAKLAR.md).
> Bu analizden sonra `config_validate`, `skill_validate`, `mermaid_validate` araçları **eklendi**
> (artık TionHarness'te mevcut) → §4a'daki ilgili maddeler **KAPANDI**; güncel açık boşluk listesi için
> 41 numaralı dokümana bakın.

## Özet

- **External Agent session araçları:** **25 araç** (`SESSION_TOOL_DEFS` dizisi, `getToolDefsAsJsonSchema()`
  ile MCP/Codex tarafına da aynen sunuluyor — `session-mcp-server` ile envanter birebir doğrulandı).
- **TionHarness araçları:** **~80 builtin araç** (`builtin_*.go` içindeki `Def()` kayıtları) + interaction
  köprüsünden gelen `run_subagent`. Bu doküman özellikle self-management + session/spawn/subagent
  ailesine odaklanır, ancak tüm builtin envanteri de gruplanmıştır.

İki ürün farklı felsefelere oturuyor:

- **External Agent**, oturum-kapsamlı yardımcı araçlar (kaynak/credential yönetimi, config doğrulama,
  şablon/veri dönüştürme, oturumlar-arası mesajlaşma) üzerine kurulu; çekirdek dosya/şüt araçlarını
  (Read/Write/Bash...) Claude/Codex SDK'sından **natif** alır, bu yüzden `session-tools-core` içinde
  yer almazlar.
- **TionHarness**, bir multi-agent platformudur: ajan/flow/task/schedule/hook/workspace CRUD'u,
  secret vault, artifact ve skill yönetimi gibi **platform yönetim
  araçlarını** kendi builtin'leri olarak taşır. Dosya/şal araçları da builtin'dir.

---

## 1. External Agent Araç Envanteri (`session-tools-core`)

`safeMode` = Explore/Safe modda izinli mi (`allow`/`block`). `exec` = registry (yerel handler) veya
backend (Pi/Claude/Electron adaptörü).

| # | Araç | Açıklama (özet) | Temel parametreler (zod) | exec / safe |
|---|------|-----------------|---------------------------|-------------|
| 1 | `SubmitPlan` | Yazılmış plan markdown'ını kullanıcı onayına sunar; çağrı sonrası yürütme duraklar | `planPath: string` | registry / allow |
| 2 | `config_validate` | External Agent config dosyalarını doğrular | `target: enum(config\|sources\|statuses\|preferences\|permissions\|automations\|tool-icons\|all)`, `sourceSlug?: string` | registry / allow (readOnly) |
| 3 | `skill_validate` | Bir skill'in `SKILL.md` dosyasını doğrular (slug, frontmatter, içerik) | `skillSlug: string` | registry / allow (readOnly) |
| 4 | `mermaid_validate` | Mermaid diyagram sözdizimini (opsiyonel render ile) doğrular | `code: string`, `render?: boolean` | registry / allow (readOnly) |
| 5 | `source_test` | Bir source config'ini doğrular, bağlantıyı test eder ve varsayılan olarak aktive eder | `sourceSlug: string`, `autoEnable?: boolean` | registry / allow |
| 6 | `source_oauth_trigger` | MCP source için OAuth 2.0 + PKCE akışını başlatır (yürütme duraklar) | `sourceSlug: string` | registry / block |
| 7 | `source_google_oauth_trigger` | Google API source için OAuth (Gmail, Calendar, Drive...) | `sourceSlug: string` | registry / block |
| 8 | `source_slack_oauth_trigger` | Slack API source için OAuth | `sourceSlug: string` | registry / block |
| 9 | `source_microsoft_oauth_trigger` | Microsoft API source için OAuth (Outlook, Teams...) | `sourceSlug: string` | registry / block |
| 10 | `source_credential_prompt` | Kullanıcıya OAuth-dışı credential giriş UI'ı açar | `sourceSlug: string`, `mode: enum(bearer\|basic\|header\|query\|multi-header)`, `labels?`, `description?`, `hint?`, `headerNames?: string[]`, `passwordRequired?: boolean` | registry / block |
| 11 | `update_user_preferences` | Kalıcı kullanıcı tercihlerini günceller | `name?`, `timezone?`, `city?`, `region?`, `country?`, `notes?`, `includeCoAuthoredBy?: boolean` | registry / block |
| 12 | `transform_data` | İzole subprocess'te script çalıştırıp datatable/spreadsheet/preview için yapısal çıktı üretir | `language: enum(python3\|node\|bun)`, `script: string`, `inputFiles: string[]`, `outputFile: string` | registry / allow |
| 13 | `script_sandbox` | Ağ/FS izolasyonlu sandbox'ta kısa tanılama script'i çalıştırır | `language: enum(python3\|node\|bun)`, `script: string`, `inputFiles?: string[]`, `stdin?: string`, `timeoutMs?: number(1..15000)` | registry / allow |
| 14 | `render_template` | Bir source'un Mustache HTML şablonunu veri ile render eder | `source: string`, `template: string`, `data: record<string,unknown>` | registry / allow |
| 15 | `send_developer_feedback` | External Agent geliştirici ekibine serbest-metin geri bildirim yollar | `message: string` | registry / allow |
| 16 | `call_llm` | İkincil LLM'i odaklı alt görev için çağırır (özet/sınıflandırma/yapısal çıktı) | `prompt: string`, `attachments?`, `model?`, `systemPrompt?`, `maxTokens?`, `temperature?`, `thinking?`, `thinkingBudget?`, `outputFormat?`, `outputSchema?` | backend / allow (readOnly) |
| 17 | `spawn_session` | Kendi prompt/bağlantı/model/source'larıyla bağımsız yeni oturum başlatır | `help?`, `prompt?`, `name?`, `llmConnection?`, `model?`, `enabledSourceSlugs?`, `permissionMode?`, `thinkingLevel?`, `labels?`, `workingDirectory?`, `attachments?` | backend / block |
| 18 | `browser_tool` | CLI-benzeri tek komutla tüm tarayıcı eylemleri (navigate/click/snapshot/evaluate...) | `command: string \| string[]` | backend / allow |
| 19 | `set_session_labels` | Oturumun etiketlerini (toplu) ayarlar; otomasyon tetikler | `sessionId?: string`, `labels: string[]` | registry / block |
| 20 | `set_session_status` | Oturum durumunu ayarlar (todo/in_progress/done); otomasyon tetikler | `sessionId?: string`, `status: string` | registry / block |
| 21 | `get_session_info` | Oturum metadata'sını döner (etiket, durum, ad, izin modu) | `sessionId?: string` | registry / allow (readOnly) |
| 22 | `list_sessions` | Workspace oturumlarını filtre+sayfalama ile listeler | `status?`, `label?`, `search?`, `sortBy?`, `limit?`, `offset?` | registry / allow (readOnly) |
| 23 | `send_agent_message` | Başka bir oturuma gönderen-zarfı ile mesaj yollar | `sessionId: string`, `message: string`, `attachments?` | registry / block |
| 24 | `list_messaging_channels` | Oturuma bağlı Telegram/WhatsApp kanallarını listeler | `sessionId?: string` | registry / allow (readOnly) |
| 25 | `unbind_messaging_channel` | Bir mesajlaşma kanalını oturumdan ayırır | `platform?: enum(telegram\|whatsapp)` | registry / block |

> Not: Çekirdek dosya/şal araçları (Read, Write, Edit, Bash, Glob, Grep, WebFetch vb.) External Agent'ta
> **Claude/Codex SDK'sından natif** gelir; `session-tools-core` envanterine dahil değildir. Bu yüzden
> yukarıdaki 25'lik listede yer almazlar — ama TionHarness karşılaştırması için aşağıda dikkate alınmıştır.

---

## 2. TionHarness Araç Envanteri (gruplanmış)

`builtin_*.go` içindeki `Def()` kayıtları + interaction köprüsü. Toplam ~80 builtin + 1 köprülü araç.

**Ajan yönetimi:** `create_agent`, `update_agent`, `delete_agent`, `list_agents`
**Flow orkestrasyonu:** `create_flow`, `update_flow`, `delete_flow`, `list_flows`, `get_flow`, `run_flow`
**Task / kanban:** `create_task`, `update_task`, `move_task`, `delete_task`, `list_tasks`
**Zamanlama (schedule/routine):** `create_schedule`, `update_schedule`, `delete_schedule`, `list_schedules`, `run_schedule`, `schedule_wake`
**Hook'lar:** `create_hook`, `update_hook`, `delete_hook`, `list_hooks`
**MCP sunucu yönetimi:** `create_mcp_server`, `toggle_mcp_server`, `delete_mcp_server`, `list_mcp_servers`
**Skill yönetimi:** `create_skill`, `update_skill`, `delete_skill`, `use_skill`, `import_skill`, `skill_search`
**Workspace yönetimi:** `create_workspace`, `rename_workspace`, `delete_workspace`, `list_workspaces`
**Secret vault:** `secret_set`, `secret_get`, `secret_list`, `secret_delete`
**Ayarlar:** `get_settings`, `update_settings`
_(**Bellek:** `memory_add`/`memory_recall`/`core_memory_*` — memory alt sistemiyle birlikte 2026-07-05'te KALDIRILDI.)_
**Artifact:** `create_artifact`, `update_artifact`, `read_artifact`, `delete_artifact`, `list_artifacts`
**Oturum kendi-yönetimi:** `set_session_goal`, `complete_goal`, `set_session_title`, `set_working_dir`, `archive_session`, `handoff_session`, `list_sessions`
**Kullanıcı etkileşimi:** `ask_user`, `request_confirmation`, `notify`, `focus_view`, `todo_write`
**Delegasyon / mesajlaşma:** `spawn_session`, `send_message`, `run_subagent` *(köprülü; delegasyon açıkken)*
**Lazy araç yükleme:** `tool_search`, `activate_tools`, `deactivate_tools`
**Gözlemlenebilirlik:** `read_logs`, `read_session_debug`, `conversation_search`
**Config dosyaları:** `read_config`, `write_config`, `list_config`
**Çekirdek dosya/şal:** `Read`, `Write`, `Edit`, `LS`, `Glob`, `Grep`, `Bash`, `WebFetch`

---

## 3. Eşleştirme Tablosu (External Agent → TionHarness)

Durum: `birebir` / `kısmi` / `TionHarness'te yok`.

| External Agent aracı | TionHarness karşılığı | Durum | Not |
|-------------------|--------------------|-------|-----|
| `SubmitPlan` | claude-cli plan modu (`ExitPlanMode` köprüsü) | kısmi | TionHarness'te bağımsız bir builtin plan aracı yok; plan onayı CLI'nin `ExitPlanMode`'una bağlanmış (son commit: "claude-cli plan modu"). |
| `config_validate` | `read_config` / `write_config` / `list_config` | kısmi | Config dosyalarını okuma/yazma var; **doğrulama (schema validation) yok**. |
| `skill_validate` | `create_skill` / `update_skill` | kısmi | Skill CRUD var ama ayrı `validate` adımı yok. |
| `mermaid_validate` | — | TionHarness'te yok | Diyagram doğrulama aracı yok. |
| `source_test` | `create_mcp_server` / `toggle_mcp_server` / `list_mcp_servers` | kısmi | MCP sunucu yönetimi var; "doğrula + bağlantı testi + otomatik aktive et" tek-adımı yok. TionHarness source ≈ MCP sunucu kavramı. |
| `source_oauth_trigger` | — | TionHarness'te yok | OAuth-tabanlı source akışı yok (kimlik bilgileri vault'tan). |
| `source_google_oauth_trigger` | — | TionHarness'te yok | — |
| `source_slack_oauth_trigger` | — | TionHarness'te yok | — |
| `source_microsoft_oauth_trigger` | — | TionHarness'te yok | — |
| `source_credential_prompt` | `secret_set` (+ `ask_user`) | kısmi | Secret vault var ama "güvenli credential giriş UI'ı" yok; ajan secret'ı kendisi yazar. |
| `update_user_preferences` | `update_user_preferences` (builtin) | kısmi | Yapısal tercih alanları var; önceki serbest-metin core-memory yolu (memory alt sistemi) 2026-07-05'te kaldırıldı. |
| `transform_data` | `Bash`/`Shell` + `Write` | kısmi | İzole subprocess + yapısal çıktı sözleşmesi yok; aynı sonuç shell ile elde edilebilir. |
| `script_sandbox` | `Bash` (sandbox'lı PowerShell shell) | kısmi | Sandbox'lı shell var; ağ-izolasyonlu satır-içi script tanılama aracı ayrı değil. |
| `render_template` | — | TionHarness'te yok | Mustache/HTML şablon render aracı yok. |
| `send_developer_feedback` | — | TionHarness'te yok | Geliştiriciye geri bildirim kanalı yok. |
| `call_llm` | `run_subagent` (sync, izole) | kısmi | `run_subagent` araçlı tam bir ajan (daha ağır); `call_llm` tek-completion/ucuz. En yakın karşılık. |
| `spawn_session` | `spawn_session` | birebir | Her ikisi de bağımsız yeni oturum başlatır (fire-and-forget). |
| `browser_tool` | — (MCP: playwright vb.) | TionHarness'te yok | Natif tarayıcı aracı yok; tarayıcı MCP sunucuları üzerinden kullanılır. |
| `set_session_labels` | — (`move_task` kanban kolonları) | TionHarness'te yok | Oturum-seviyesi etiket kavramı yok; benzer "durum" mantığı kanban task'larında. |
| `set_session_status` | `archive_session` / `complete_goal` / `move_task` | kısmi | Oturum için done≈`archive_session`; durum-makinesi task board'unda (`move_task`). |
| `get_session_info` | `list_sessions` | kısmi | Tekil oturum metadata'sını dönen ayrı araç yok; `list_sessions` durumsal farkındalık verir. |
| `list_sessions` | `list_sessions` | birebir | Her ikisi de workspace oturumlarını listeler. |
| `send_agent_message` | `send_message` | kısmi | TionHarness başka **ajana** DM yollar; Craft başka **oturuma**. Amaç aynı (sürmekte olan koordinasyon). |
| `list_messaging_channels` | — | TionHarness'te yok | Telegram/WhatsApp gateway entegrasyonu yok. |
| `unbind_messaging_channel` | — | TionHarness'te yok | — |

> Çekirdek araç eşi (envanter-dışı, her iki tarafta var): `Read/Write/Edit/Bash/Glob/Grep/WebFetch`
> TionHarness'te builtin, External Agent'ta SDK-natif. `mermaid_validate` hariç bu çekirdek küme örtüşür.

---

## 4. Boşluk Analizi

### 4a. External Agent'ta var, TionHarness'te yok (eklenmesi mantıklı olanlar)

| Craft aracı | Öneri (1 cümle) |
|-------------|------------------|
| `config_validate` | TionHarness config/settings dosyaları için bir `config_validate` builtin'i, `write_config`/`update_settings` öncesi schema doğrulaması yaparak hatalı yapılandırmaları erken yakalar. |
| `skill_validate` | `create_skill`/`update_skill` akışına bir `skill_validate` aracı eklemek, frontmatter/slug/gövde tutarlılığını yayınlamadan önce garanti eder. |
| `transform_data` | İzole subprocess'te yapısal JSON/datatable üreten bir `transform_data` builtin'i, büyük veri setlerini token-verimli işlemeyi standardize eder (şu an ad-hoc shell). |
| `render_template` | Source/artifact verisini Mustache HTML şablonuyla render eden bir araç, rapor/önizleme çıktılarını tutarlı ve markalı hale getirir. |
| `mermaid_validate` | Diyagram üretimi yapan ajanlar için bir `mermaid_validate`, bozuk diyagramların kullanıcıya gitmesini önler. |
| `get_session_info` | Tekil oturum metadata'sını (etiket/durum/cwd/goal) dönen bir araç, `list_sessions`'a göre daha ucuz "kendini incele" yolu sağlar. |
| `source_credential_prompt` | Kullanıcıya güvenli credential giriş UI'ı açan bir araç, ajanların secret'ı kendilerinin yazmasına (veya tahmin etmesine) gerek bırakmaz. |
| `call_llm` | `run_subagent`'ten ayrı, araçsız tek-completion bir `call_llm`, ucuz özet/sınıflandırma alt görevleri için ağır subagent yerine doğru maliyet katmanını verir. |

### 4b. TionHarness'te var, External Agent'ta yok (TionHarness'in platform üstünlüğü)

Bunlar TionHarness'in multi-agent platform doğasından gelir ve External Agent'ın oturum-kapsamlı modelinde
karşılığı yoktur (eksiklik değil, kapsam farkı):

- **Tam CRUD aileleri:** ajan (`create/update/delete/list_agent`), flow (`create/.../run_flow`),
  task/kanban (`create/move/.../list_tasks`), schedule (`create/.../run_schedule`),
  hook (`create/delete/list_hooks`), MCP sunucu, workspace.
- **Secret vault:** `secret_set/get/list/delete` — Craft'ta source-OAuth/credential-prompt modeli var.
- **Artifact yönetimi:** `create/update/read/delete/list_artifact` — sürümlü içerik saklama.
- **Gözlemlenebilirlik:** `read_logs`, `read_session_debug`, `conversation_search`.
- **Lazy araç yükleme:** `tool_search`/`activate_tools`/`deactivate_tools` (CLI'nin ToolSearch'üne benzer).
- **Goal yönetimi:** `set_session_goal`/`complete_goal` (oturumun "north star"ı).
- **`run_subagent`:** aynı turda sonuç dönen izole alt-ajan (Craft'ta `call_llm` + `spawn_session`
  bu ihtiyacı bölüştürür).

### 4c. Karşılıklı denk olanlar

`spawn_session` (birebir) ve `list_sessions` (birebir) iki tarafta da neredeyse aynı; `send_agent_message`
↔ `send_message` ve `set_session_status` ↔ `archive_session/move_task` kavramsal olarak örtüşür.

---

## Sonuç

İki araç seti, **oturum-yardımcısı** (External Agent) ve **multi-agent platform yönetimi** (TionHarness) olarak
ayrışır. Örtüşme delegasyon (`spawn_session`), oturum farkındalığı (`list_sessions`) ve oturumlar-arası
mesajlaşmada yoğunlaşır. TionHarness'e en yüksek değerli adaylar: doğrulama araçları
(`config_validate`, `skill_validate`, `mermaid_validate`), `transform_data`/`render_template` çıktı
katmanı ve ucuz `call_llm` katmanı.
