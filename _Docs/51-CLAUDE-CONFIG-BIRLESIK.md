# 51 — Per-Workspace Claude Config Home (Birleşik Config)

> **Amaç:** TionSwarm'ın workspace klasör yapısı ile claude-cli'nin `CLAUDE_CONFIG_DIR`
> config evini **tek bir per-workspace dizinde** birleştirmek. Böylece hem TionSwarm
> hem de driver ettiği `claude` CLI **aynı skill/settings/login** setini kullanır.

## Sorun

Önceden `claude-cli`'nin config evi **global tek bir ayardı**:

- `settings.json` → `claudeConfigDir`, varsayılan `~/.tionswarm/claude-home`
  (`settings.defaultClaudeConfigDir`)
- Değer paylaşılan `providers.Registry` üzerinde tutuluyordu (`SetClaudeConfigDir`);
  tüm workspace'ler **aynı** `claude-home`'u kullanıyordu.
- Sonuç: CLI'nin native `Skill` aracı yalnızca `<CLAUDE_CONFIG_DIR>/skills`'i okuduğu
  için TionSwarm'ın workspace skill'lerini **hiç göremiyordu** → native `Skill`
  aracı `climcp.go`'da devre dışı bırakılmıştı ("Unknown skill" hatası).

## Çözüm: config evi = `<workspace>/claude-home`

Her workspace kendi izole config evini alır (mimari karar #5 "workspace izolasyonu"
ile tutarlı):

```
<workspace>/
├── claude-home/          ← CLAUDE_CONFIG_DIR (per-workspace) — YALNIZ login/settings
│   ├── settings.json     ← CLI-managed (global home'dan tohumlanır)
│   ├── .claude.json      ← login/oturum durumu
│   └── ...
├── skills/               ← workspace skill tier (use_skill köprüsü; native Skill KAPALI)
├── store/                ← DB (MCP/hooks/agents — DB-güdümlü, bayrakla enjekte)
├── config/               ← düzenlenebilir instructions.md
├── workspace/            ← fs/shell sandbox (cwd)
└── ws-settings.json
```

## Mekanik

### Faz 1 — Config dizinini per-workspace yap ✅ (uygulandı)
- `providers.ClaudeCLI.SetConfigDir(dir)` (yeni): construct-time config dir'i
  turluk override eder. Boş değer yok sayılır (global varsayılan korunur).
- `Runtime.claudeHomeDir()` = `<workspace>/claude-home` (workDir'in kardeşi).
- **Choke point `toolloop.go`:** `cli, isCLI := provider.(*providers.ClaudeCLI)`
  hemen sonrası `cli.SetConfigDir(r.claudeHomeDir())`. Tüm CLI turları (chat +
  otonom, tüm çağrı noktaları) bu tek seam'den geçtiği için başka yeri değiştirmeye
  gerek yok.
- **Migration/kopyalama:** `agent.EnsureWorkspaceClaudeHome(wsRoot)` — workspace
  açılışında (`workspace/manager.go open()`) çağrılır. İlk açılışta
  `~/.tionswarm/claude-home` (global) içeriğini per-workspace eve **tohumlar**
  (login dahil; `projects/`, `sessions/`, `cache/` gibi çalışma-anı/büyük dizinler
  atlanır). Idempotent. **Skill taşımaz** — claude-home yalnız login/settings tutar.
- **Credential self-heal (`ensureClaudeHomeCredential`):** her açılışta çalışır. Global
  tohum home'un `.credentials.json`'ı **boş/token'sız** olabilir (accessToken="",
  refreshToken="", expiresAt=0 scaffold) — bu durumda tohumlanan her yeni workspace CLI
  login popup'ı verirdi. Guard: workspace home'un kendi credential'ı token taşımıyorsa
  sırayla **global → gerçek `~/.claude`** ilk kullanılabilir credential'ı kopyalar
  (keyless CLI = kullanıcının yerel login'i). Zaten token taşıyan per-workspace login
  asla ezilmez.

### Faz 2 — Skill tier'ı = `<claude-home>/skills` ⟲ (GERİ ALINDI 2026-07-05)
Kısa süre denendi, sonra geri alındı. **Karar:** skill'ler eski yerinde kalsın
(`<workspace>/skills`) ve tek yol `use_skill` köprüsü olsun.
- `workspaceSkillsDir` yeniden `<workspace>/skills` döndürüyor.
- Diskteki taşınmış skill'ler (WS1: 4, WS8: 14) `<ws>/skills`'e geri alındı;
  `claude-home/skills` silindi.
- Migration'dan skills-taşıma bloğu çıkarıldı.

### Faz 3 — Native `Skill` aracını geri aç ⟲ (GERİ ALINDI 2026-07-05)
`climcp.go`'daki `disallowed = append(disallowed, "Skill")` **korunuyor** (native
`Skill` KAPALI). CLI native aracı yalnız `<CLAUDE_CONFIG_DIR>/skills`'i okur; workspace
tier'ı artık orada olmadığı için native yol "Unknown skill" verir → tek doğru yol
`use_skill` köprüsü (hem workspace hem global tier'ı sunar).

## Skill görünürlüğü (özet)

| Tier | Konum | `use_skill` köprüsü | native `Skill` |
|------|-------|:---:|:---:|
| Workspace | `<workspace>/skills` | ✅ | ❌ (kapalı) |
| Global | `~/.tionswarm/skills` | ✅ | ❌ (kapalı) |

Her iki tier de `skills.New(globalSkillsDir, workspaceSkillsDir)` ile yüklenir ve
yalnız `use_skill` üzerinden sunulur. claude-home skill İÇERMEZ.

## UI

Settings → Sağlayıcılar → Anthropic altındaki **"claude config dizini"** alanı artık
**salt-okunur** (`ProvidersPanel.tsx`, `endpoint2ReadOnly`): değer per-workspace
türetildiği için elle değiştirilemez; ipucu metni global alanın yalnızca fallback
olduğunu açıklar. Tip yorumu `types/settings.ts`'te güncellendi.

## Kimlik (auth) & yedekleme

Her workspace kendi `claude-home` login'ini taşır: migration global home'dan
`.claude.json` + `.credentials.json` (OAuth token) dosyalarını da **kopyalar** — böylece
her workspace bağımsız login'lidir. Kopyalar tek makinede/aynı kullanıcı altında
kaldığı için saldırı yüzeyini büyütmez (global `~/.claude` zaten düz metin), **ANCAK**
periyodik yedek (`backup/zipDir`, `34-YEDEKLEME.md`) workspace dizininin tamamını
zip'lediği için credential dosyaları yedek arşivine sızabilirdi.

**Koruma:** `backup/archive.go` → `backupExcludeNames` = `{.credentials.json,
.claude.json}`; bu dosya adları **hiçbir yedek zip'ine yazılmaz**. Credential diskte
workspace içinde kalır (auth çalışmaya devam eder) ama arşive girmez. Restore edilen
bir workspace bu dosyalara sahip olmaz → sonraki açılışta global home'dan yeniden
tohumlanır (claude-home yoksa) **veya** enjekte edilen auth env'ine (`claudeCliAuthKind`)
dayanır. Test: `backup/backup_test.go TestZipExcludesClaudeCredentials`.

Alternatif (uygulanmadı): saf env-enjeksiyon — token'ı hiç kopyalamayıp yalnız
TionSwarm'ın AES-GCM sır kasasında tutup her tur env ile enjekte etmek. Daha sıkı ama
`claudeCliAuthKind` set olmasını zorunlu kılar.

## Sınırda bırakılanlar (bilinçli)

MCP sunucular, hooks, izinler, agent tanımları **DB'de** kalır ve her turda
`--mcp-config` / `--settings` bayraklarıyla enjekte edilmeye devam eder — bu zaten
Claude CLI'nin resmi override mekanizması olduğundan dosya-config'e taşımanın net
faydası yoktur. Yalnızca **statik/dosya-şekilli** config (skills + login/settings)
paylaşılır.

## Dosyalar

- `internal/providers/claudecli.go` — `SetConfigDir`
- `internal/agent/claudehome.go` (yeni) — path helper'ları + `EnsureWorkspaceClaudeHome` + copy
- `internal/agent/runtime.go` — `claudeHomeDir()`, `workspaceSkillsDir` güncellendi
- `internal/agent/toolloop.go` — per-turn `SetConfigDir` seam
- `internal/agent/climcp.go` — native `Skill` disallow kaldırıldı
- `internal/workspace/manager.go` — `open()`'da migration çağrısı

## İlgili

`17-TOKEN-OPTIMIZASYON.md` (claude-cli config evi + auth), `50-CLAUDE-CODE-CACHE-PARITE.md`,
mimari karar #3 (claude-cli) ve #5 (workspace izolasyonu).
