# 51 — Per-Workspace Claude Config Home (Birleşik Config)

> **Amaç:** TionHarness'in workspace klasör yapısı ile claude-cli'nin `CLAUDE_CONFIG_DIR`
> config evini **tek bir per-workspace dizinde** birleştirmek. Böylece hem TionHarness
> hem de driver ettiği `claude` CLI **aynı skill/settings/login** setini kullanır.

> **Güncel durum (2026-08-19):** Bu belge per-workspace modelinin tarihsel
> tasarımını anlatır. Sağlayıcı örnekleriyle birlikte yeni `claude-cli`
> örneklerinin login/config evi artık **örnek başına**dır. Boş `configDir` ile
> oluşturulan örneğe backend `<dataDir>/provider-homes/<instance-id>` yolunu
> atar; bu uygulama-geneli örneği seçen bütün ajanlar aynı kimliği kullanır.
> Boş `configDir` fallback'i ve eski `/api/workspace-settings/claude-auth...`
> rotaları artık uygulama-geneli `<dataDir>/claude-home` kullanır. Açık
> `configDir` verilen örneklerin izole kimlikleri değişmez. Eski workspace home'u
> yalnız tek login bulunduğunda global home'a bir kez kopyalanır; silinmez.
> Güncel sözleşme: `71-SAGLAYICI-ORNEKLERI-PLANI.md` §4.4 ve §5.1.

> **Config evi taşınmasının bedeli — `--resume` (2026-08-19):** claude-cli
> konuşma transkriptleri config evinin **içinde** durur
> (`<home>/projects/<cwd-slug>/<id>.jsonl`). Ev taşınınca oturumlardaki kayıtlı
> `cliSessionId` eski evi işaret etmeye devam etti; CLI bunu
> `No conversation found with session ID: …` + exit 1 ile reddetti, hata
> "yeniden denenebilir" göründüğü için aynı ölü id ile 3 tur harcandı ve oturum
> `stuck` etiketlendi. İki taraflı çözüm:
>
> - **Kod:** `ClaudeCLI.CanResume(id)` yeni evde transkript var mı diye bakar;
>   `planClaudeResume` yoksa **soğuk** başlar (tam transkript korunur, delta
>   gönderilmez). Yarış hâlinde CLI yine reddederse hata artık
>   NON-retryable ve id + ev adını söyler.
> - **Veri:** `cmd/repair-provider-migration` eski workspace evinde hâlâ bulunan
>   transkriptleri yeni eve **taşır**, bulunamayanların resume kaydını temizler
>   (geçmiş kaybolmaz — TionHarness mesajları kendi store'unda tutar, CLI kopyası
>   yalnız önbellektir) ve bu kesintinin bıraktığı `stuckTurns`/`stuck` izini siler.

## Sorun

Önceden `claude-cli`'nin config evi **global tek bir ayardı**:

- `settings.json` → `claudeConfigDir`, varsayılan `~/.tionharness/claude-home`
  (`settings.defaultClaudeConfigDir`)
- Değer paylaşılan `providers.Registry` üzerinde tutuluyordu (`SetClaudeConfigDir`);
  tüm workspace'ler **aynı** `claude-home`'u kullanıyordu.
- Sonuç: CLI'nin native `Skill` aracı yalnızca `<CLAUDE_CONFIG_DIR>/skills`'i okuduğu
  için TionHarness'in workspace skill'lerini **hiç göremiyordu** → native `Skill`
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
  hemen sonrası `cli.SetConfigDir(r.claudeHomeDir())`. Tüm CLI **turları** (chat +
  otonom, tüm çağrı noktaları) bu tek seam'den geçtiği için tur yolunda başka yeri
  değiştirmeye gerek yok. Aux çağrılar (title/summary/lesson) `guardedComplete`'te
  `PinClaudeHome` ile pinlenir.
- **Tek giriş noktası `Runtime.PinCLIHome(provider)` (2026-08-22):** hem
  claude-cli hem codex-cli evini pinler; diğer transport için no-op. Doğrudan
  `provider.Complete` çağıran her yer (`guardedComplete`, `handoff.go`,
  `api/summary.go` `/compact`, `api/chat_stream.go` ön-compaction) artık bunu
  çağırır. Önceden bu yerler yalnız `PinClaudeHome` çağırıyordu → codex-cli
  ajanında `CODEX_HOME` hiç export edilmiyor, alt süreç ambient `~/.codex`'i
  okuyup çoğu kez `refresh token was revoked` veriyordu.
- `PinClaudeHome` artık `(home string, err error)` döner ve pinlediği yerde
  `MkdirAll` + credential heal'i de yapar (eskiden bunlar yalnız `toolloop.go`
  içindeydi, yani döngü dışı çağrılar korumasızdı).
- **İstisna — fold yolları (compaction/handoff):** `conversation.summarizeRendered`
  ve `BuildHandoff` tur döngüsünün ve `guardedComplete`'in DIŞINDA doğrudan
  `provider.Complete` çağırır → toolloop seam'inden geçmez. Pinlenmezse **global**
  claude-home'a düşüp login olsa bile `authentication_failed` verirdi (2026-08-04'te
  `compaction_failed` olarak görüldü; otomatik rolling-compaction, `/btw` ve wake
  turu pinlenmiyordu). **Kalıcı çözüm:** fold çekirdeği artık ctx'ten **self-pin**
  yapıyor — `conversation.WithClaudeHome(ctx, Runtime.ClaudeHomeDir())` → çekirdekteki
  `pinClaudeHome`. Her fold giriş noktası (`chat_stream`, `chat_btw`, `wake_turn`,
  `summary`, `agent.handoff`) home'u ctx'e koyar; böylece yeni bir fold call-site
  eklendiğinde pinlemeyi unutsa bile çekirdek korur.
- **Migration/kopyalama:** `agent.EnsureWorkspaceClaudeHome(wsRoot)` — workspace
  açılışında (`workspace/manager.go open()`) çağrılır. İlk açılışta
  `~/.tionharness/claude-home` (global) içeriğini per-workspace eve **tohumlar**
  (login dahil; `projects/`, `sessions/`, `cache/` gibi çalışma-anı/büyük dizinler
  atlanır). Idempotent. **Skill taşımaz** — claude-home yalnız login/settings tutar.
- **Credential self-heal (`ensureClaudeHomeCredential`):** her açılışta **ve her CLI
  turunda** (`toolloop.go` per-turn dikişi) çalışır. Global tohum home'un
  `.credentials.json`'ı **boş/token'sız** olabilir (accessToken="", refreshToken="",
  expiresAt=0 scaffold) — bu durumda tohumlanan her yeni workspace CLI login popup'ı
  verirdi. Guard: adaylar (**global `~/.tionharness/claude-home` + gerçek `~/.claude`**)
  `credentialRank` ile **sıralanır** ve en iyisi kopyalanır; workspace'in kendi
  credential'ı daha iyi sıralanıyorsa asla ezilmez.

  **Sıralama ölçütü: önce CANLI access token** (`credentialLiveness`). Bu, 2026-08-01'de
  canlı bir 3-seviyeli koordinatör denemesinde bulunan hatanın düzeltmesidir
  (`_Docs/47` §14.8): global home 19 gün önce ölmüş bir credential tutuyordu, eski
  guard yalnız "token boş mu" baktığı için onu geçerli sayıp kullanıcının canlı
  `~/.claude`'unun önüne koyuyordu. CLI ölü refresh token ile `invalid_grant` alıp
  credential'ı **temizliyor**, self-heal de aynı ölü dosyayı geri kopyalıyordu — ve
  eski hâliyle yalnız açılışta koştuğu için o workspace restart'a kadar ölüydü.
  Yalnız expiry damgalarına bakmak yetmez: bayat home'un `refreshTokenExpiresAt`'i
  **gelecekte** olabilir (diskte canlı görünür) ve bir refresh token'ın harcanmış
  olduğu diskten **hiç anlaşılmaz**. Süresi dolmamış access token ise o home'un son
  bir saat içinde başarıyla kimlik doğruladığının kanıtıdır ve taklit edilemez.

- **Refresh serileştirme (`claudeauth/refreshgate.go`):** OAuth refresh token'ları
  **tek kullanımlıktır**; TionHarness ise tek bir claude-home'a karşı çok sayıda
  eşzamanlı CLI süreci koşturur (koordinatör ağacı bunu tasarım gereği yapar).
  Hepsi aynı anda yenilemeye kalkarsa kaybedenler `invalid_grant` alır ve CLI
  credential'ı siler. `SerializeRefresh` yalnız **yenilemenin gerçekten gerekli
  olduğu** pencerede tek süreç kabul eder, refresh dosyaya düşünce kapıyı açar
  (30 sn tavan). Token sağlıklıyken kilit yoktur → paralel fan-out etkilenmez.

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
| Global | `~/.tionharness/skills` | ✅ | ❌ (kapalı) |

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
TionHarness'in AES-GCM sır kasasında tutup her tur env ile enjekte etmek. Daha sıkı ama
`claudeCliAuthKind` set olmasını zorunlu kılar.

### Sürüm + plan rozeti (2026-08-01)

`GET /api/catalog`'un `claude-cli` girdisi iki ek alan taşır: `cliVersion` (yerel
`claude --version`, `exttools.LocalVersion` ile probe; başarıda 10 dk / hatada 1 dk
TTL'li memo) ve `subscription` (`claudeauth.ReadCredentials(<workspace>/claude-home)`
→ `claudeAiOauth.subscriptionType`, yani `max`/`pro`). Model seçicide ve Sağlayıcılar
kartında `Claude Code v2.1.220 · Max` rozeti olarak gösterilir — claude-cli modelleri
çıplak takma ad olduğu için "hangi kurulum, hangi plan" başka yerde görünmüyordu.
Login per-workspace olduğundan rozet de per-workspace. `claudeCliAuthKind == "apikey"`
iken plan yazılmaz (abonelik değil); okunamayan bilgi tahmin edilmez, parça düşer.

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
- `internal/api/catalog_claudecli.go` (yeni) + `catalog.go` — sürüm probe'u + plan okuma
- `internal/claudeauth/read.go` (yeni) — `ReadCredentials` (WriteCredentials'ın salt-okunur eşi)

## İlgili

`17-TOKEN-OPTIMIZASYON.md` (claude-cli config evi + auth), `50-CLAUDE-CODE-CACHE-PARITE.md`,
mimari karar #3 (claude-cli) ve #5 (workspace izolasyonu).
