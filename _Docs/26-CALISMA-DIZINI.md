# Çalışma Dizini (Working Directory) — Oturum-Başına cwd

> Eklendi: **2026-06-22**. the external agent project (external-agent-oss) "working directory"
> mekaniğinin TionHarness'e uyarlaması.

## Amaç

Her sohbet oturumunun, ajanın dosya/kabuk araçlarının çalışacağı bir **çalışma
dizini (cwd)** olabilir — tıpkı terminalde `cd /path/to/proje` yapmak gibi.
Böylece bir oturumu doğrudan bir proje klasörüne bağlayıp ajana o repoda kod
yazdırabilir, test çalıştırabilir, git kullandırabilirsin.

Bu özellik, daha önce kaldırılan **fs/shell sandbox kilidinin** (bkz.
`06-WORKSPACES.md`) doğal devamıdır: araçlar artık her yere erişebildiği için,
"nerede çalışacağını" oturum başına seçmek anlamlı hale geldi.

## Katmanlar

```mermaid
graph LR
    A[Composer klasör rozeti] -->|PUT /workdir| B[Session.WorkingDir]
    B --> C[effectiveWorkDir ctx]
    C --> D[fs/shell sandbox kökü]
    C --> E[claude-cli req.WorkDir]
    B --> F[Context bloğu:<br/>cwd + git branch + CLAUDE.md]
```

### 1. Veri
- `db.Session.WorkingDir` (`workingDir` JSON) — oturuma özel cwd. Boş = workspace
  varsayılanı (`Runtime.workDir`). `SetSessionWorkingDir` ile yazılır.

### 2. Çözümleme (`internal/agent/workdir_ctx.go`)
- `Runtime.effectiveWorkDir(ctx)` — ctx'teki session id'den oturumu okur;
  `WorkingDir` set ve gerçek bir dizinse onu, değilse `r.workDir`'i döndürür.
- `completeTraced` her turda çözer → `req.WorkDir` (CLI cwd) + ctx'e koyar
  (`withResolvedWorkDir`). `buildRegistry` bu ctx'ten sandbox kökünü alır.
- Chat yolları artık `agent.WithSessionID` ile session id'yi ctx'e basıyor
  (`chat_stream.go`, `chat.go`); otonom yollar (scheduler/spawn) zaten basıyordu.

### 3. Bağlam enjeksiyonu (`internal/api/workdir_context.go`)
- `workdirContextBlock(dir)` — sistem promptunun dinamik (cache-dışı) kısmına
  "Working directory" bloğu ekler: cwd yolu + git branch (`git rev-parse
  --abbrev-ref HEAD`) + `CLAUDE.md` varsa onu okuma hatırlatması.
- `composeTurnRequest` (chat_turn.go) bu bloğu goal'dan hemen sonra ekler.

### 4. UX (`frontend`)
- `components/chat/WorkDirBadge.tsx` — Composer'da klasör rozeti (Thinking/Permission
  rozetlerinin yanında). Klasör ikonu + dizin adı + git branch gösterir; tıklayınca
  dizin gezgini popover'ı açılır (alt klasörlere in/çık, "Bu klasörü kullan",
  "Sıfırla"). `sessionId` ile kendi kendine yeter.
- API: `GET/PUT /api/sessions/{id}/workdir`, `GET /api/fs/browse?path=`
  (`internal/api/workdir.go`). Browse boş path'te kökleri döndürür (Windows'ta
  sürücü harfleri, POSIX'te `/`).

## Otonomi frenleri (güvenlik)

Araçlar kilitsiz olduğundan, **insan döngüde olmayan** (scheduler/spawn/flow) turlar
için bir fren var (Ayarlar ▸ MCP & Araçlar):

| Ayar | Varsayılan | Etki |
|---|---|---|
| `autonomousConfine` | **açık** | Otonom turda **fs araçları** çalışma dizinine **kilitlenir** (mutlak yol + `..` reddi) ve shell'de `git push` engellenir. **Shell yolu kısıtlanmaz** (aşağıya bak). İnteraktif sohbet etkilenmez. |

- Confine, `Sandbox.Confined` bayrağını yeniden kullanır (`internal/tools/sandbox.go`):
  interaktif = `NewSandbox` (kilitsiz), otonom+confine = `NewConfinedSandbox`.
- `git push` engeli `builtin_shell.go isNetworkMutatingGit` ile (best-effort substring;
  gerçek sınır confine'ın kendisidir).
- **Windows yol yazımları (confined dalda, `rejectWindowsPathTricks`):** kök
  karşılaştırmasından ÖNCE açıkça reddedilenler — NT/device namespace önekleri
  (`\\?\`, `\\.\`, `\??\`, dolayısıyla `\\?\UNC\`), `GLOBALROOT` aygıt yolları ve
  NTFS alternate data stream'leri (`file.txt:stream`). Bunlar `filepath.Clean`'den
  sağ çıktığı için, kök öneki karşılaştırmasının tesadüfen tutmamasına
  güvenilmez: kural, `Sandbox.Root` bir gün uzun-yol için `\\?\` biçimine
  normalize edilse bile ayakta kalır. Sürücü harfindeki `:` meşrudur; eleman
  içindeki `:` reddedilir. Kontroller yalnız Windows'ta çalışır — başka
  platformlarda bu yazımların özel anlamı yoktur ve `:` geçerli bir dosya adı
  karakteridir. Kilitsiz (`NewSandbox`) dal tasarım gereği etkilenmez.
- Kök içi/dışı karşılaştırması (`underRoot`) Windows'ta **case-insensitive**'dir;
  bu, dosya sistemiyle ve `internal/api/files.go` içindeki `underDir` sınırıyla
  aynı davranışı verir (önceden ikisi farklıydı).

### Confine, shell'de path kısıtlamaz (2026-08-28)

`autonomousConfine` adı yanıltıcı olabilir: confine **yalnız fs araçlarını**
kapatır, kabuğu kapatmaz.

| Araç | Confined turda kısıtlı mı? | Mekanizma |
|---|---|---|
| `read_file`, `write_file`, `edit_file`, `apply_patch`, `list_dir`, `glob`, `grep`, `config_validate` | **Evet** | Her yol argümanı `Sandbox.Resolve`'dan geçer; mutlak yol ve `..` kaçışı reddedilir. |
| `shell` (Bash), `powershell` | **Hayır** | `Sandbox.Resolve` bu yolda **hiç çağrılmaz**. Komut, kabuğa opak bir metin olarak verilir. |
| `transform_data` | **Argümanlar evet, script gövdesi hayır** | Aynı kapıdan (`ShellEnabled`) kayıtlı, ana makinede python/node çalıştırır. |

`transform_data` input/output argüman yollarını `Sandbox.Resolve` üzerinden
kısıtlar. Ancak script gövdesi rastgele ana makine kodu çalıştırdığı için kendi
dosya erişimini açabilir ve bu argüman kısıtını aşabilir.

Confined bayrağının shell yolunda yaptığı **tek** iki şey
(`internal/tools/builtin_shell.go`, `builtin_shell_harden.go`):

1. `proc.DisableGitSigningEnv()` env'e eklenir — ajanın koştuğu `git commit`
   GPG pinentry parola istemine takılıp sonsuza kadar beklemesin diye.
2. `isNetworkMutatingGit` ile `git push` / `git remote add` / `git remote set-url`
   engellenir — substring tabanlı, best-effort fren.

Gerçek tek sınır **süreç çalışma dizinidir** (`cmd.Dir = sb.Root`). Yani confined
bir turda `write_file` ile `/c/başka/yer` yazılamaz ama
`sh -c 'echo x > /c/başka/yer/f'` serbesttir — shell, fs araçlarının kısıtını
komple bypass eder.

**Neden statik komut ayrıştırma çözüm değil:** komut metninden yol çıkarıp
reddetmek güvenilir biçimde yapılamaz. `cat $(echo /c/x)`, `p=/c/x; cat "$p"`,
`cd /c/x && cat f`, `python -c "open('/c/x')"` — hepsi her ayrıştırıcıyı deler.
Yarım koruma "kısıtlı" etiketiyle sunulduğunda, hiç koruma olmamasından daha
kötüdür: yanlış güven yaratır. Bu yüzden mevcut davranış **belgelenmiştir,
kısıtlanmamıştır**.

Gerçek sınır için OS-seviyesi izolasyon gerekir (container / Windows job object
ile dosya sistemi kapsamı). Bu ayrı bir karttır ve v0.1.0 sonrasına bırakılmıştır.

> **Not (2026-08-26):** Eski per-session `gitWorktreeIsolation` özelliği kaldırılmış
> kalır. Yerine kart yaşam döngüsünün tek sahibi olan `internal/worktree` geldi:
> `todo` worktree açar, `done` base dala merge eder, `iptal`/`failed` güvenli temizlik
> yapar. Dirty veya base'e ulaşmamış commit varsa silmez; merge çatışmasında worktree'yi
> korur. Base ref ve worktree kökü workspace ayarlarıdır. Aşağıdaki **geliştirici**
> worktree scripti bundan ayrıdır ve durmaktadır.

### Geliştirici worktree'leri (`scripts\worktree.ps1`) — ajan izolasyonundan AYRI

**İnsan geliştirici** için aynı anda birden çok dalda çalışmayı
(stash/checkout gidip-gelmesi olmadan) kolaylaştıran bir yardımcı script vardır:

```powershell
.\scripts\worktree.ps1 add    feat-login                        # yeni dal: feature/feat-login
.\scripts\worktree.ps1 add    hotfix -Branch fix/crash -Existing # mevcut dalı bağla
.\scripts\worktree.ps1 list
.\scripts\worktree.ps1 remove feat-login
.\scripts\worktree.ps1 prune
```

Worktree'ler reponun **kardeşi** olarak açılır (ör. `...\Projects\TionHarness-feat-login`);
tek `.git` deposu paylaşılır, her worktree'nin kendi çalışma dizini + dalı olur.
`add` sırasında `frontend/node_modules` ana repodan **junction** ile bağlanır (sıfırdan
`npm install` beklemezsin); `-NoLink` verirsen bunun yerine `npm install` koşar.

## Akış matrisi

| Tur tipi | fs araçları | shell path | git push |
|---|---|---|---|
| İnteraktif sohbet | Kilitsiz | Kilitsiz | Serbest |
| Otonom + confine açık | Çalışma dizinine kilitli | **Kilitsiz** (yalnız cwd = çalışma dizini) | Engelli |
| Otonom + confine kapalı | Kilitsiz | Kilitsiz | Serbest |

## the external agent project ile fark

- **Eşit:** oturum-başına cwd, klasör rozeti, cwd değiştirme, git branch göstergesi,
  CLAUDE.md farkındalığı, makine geneli erişim.
- **TionHarness'e özgü:** otonom turlar (scheduler/flow/spawn) için confine freni —
  the external agent project interaktif olduğu için buna ihtiyaç duymaz.

## Workspace varsayılan çalışma dizini (2026-06-22)

the external agent project paritesi: workspace başına "varsayılan çalışma dizini".
- `WSSettings.DefaultWorkingDir` (`ws-settings.json`) — Ayarlar ▸ Bu Workspace ▸
  "Varsayılan çalışma dizini" alanından girilir (mutlak yol).
- Runtime'a `SetDefaultWorkDir` ile uygulanır; `Runtime.WorkspaceDefaultDir()`
  geçerli ve var olan bir dizinse onu, değilse fiziksel `workDir`'i döndürür.
- `effectiveWorkDir` çözüm sırası: **oturum `WorkingDir`** → **workspace
  `DefaultWorkingDir`** → fiziksel `workDir`.
- **Yeni oturuma seeding (2026-06-26):** `handleCreateSession` artık yeni oturumun
  `WorkingDir`'ini workspace'in `DefaultWorkingDir`'i ile **tohumlar** (istek kendi
  `workingDir`'ini vermezse). Böylece her yeni oturum o path'e **kilitli başlar** ve
  Composer rozeti yolu `Dir` olarak (yalnız `effective` fallback değil) gösterir.
  Path boşsa eskisi gibi fiziksel `workDir`'e düşer. Not: bu bir kopyadır —
  varsayılanı sonradan değiştirmek, zaten tohumlanmış eski oturumların `WorkingDir`'ini
  geriye dönük değiştirmez (runtime fallback yalnız `WorkingDir` boş kalan oturumlarda
  varsayılanı izlemeye devam eder).
- **Spawn/handoff seeding (2026-06-26):** `SpawnOptions.WorkingDir` eklendi;
  `Runtime.SpawnSession` yeni oturumun `WorkingDir`'ini açık seçenek varsa onunla,
  yoksa workspace'in yapılandırılmış default'u (`defaultWorkDir`) ile tohumlar →
  spawn edilen (ve `spawn_session` aracı/UI ile açılan) oturumlar da Path'e kilitli
  başlar. **Handoff** continuation'ı ayrıca ebeveyn oturumun `WorkingDir`'ini geçirir
  → context-reset sonrası taze oturum **aynı dizinde** devam eder. Default boşsa alan
  boş kalır (runtime fiziksel `workDir`'e düşer).

## Workspace penceresi + Proje sekmesi (2026-06-22)

Workspace + path ayarları artık Ayarlar'dan ayrı, **sol navbar'daki "Workspace"
butonuyla** açılan kendi penceresinde (`components/workspace/WorkspaceView.tsx`).
Sol alt-navbar (Settings/Logs benzeri) üç sekme:
- **Genel** — kimlik, sağlayıcı/model, otonomi, session bağlamı (`WorkspacePanel`).
- **Proje** (`ProjectPanel`) — proje dizini (path) seçici + **git**: depo durumu,
  `git init` (yoksa), klasör hiç yoksa **"Klasörü oluştur + git init"** (istek alanı
  `createDir`; yalnız bu daldan gönderilir ki yazım hatası boş klasör açmasın),
  origin remote URL + `user.name`/`user.email` ayarları.
- **Promptlar & Dosyalar** — düzenlenebilir config dosyaları (`WorkspaceFilesPanel`).

Git endpoint'leri (`internal/api/git.go`): `GET /api/fs/gitinfo?path=`,
`POST /api/git/init`, `POST /api/git/config` (origin remote + commit kimliği).
TionHarness'in kendi başlattığı her depoya (`prepareGitRepo`) gömülü bir **başlangıç
`.gitignore`** yazılır (`internal/api/defaults/gitignore.txt`): sırlar (`.env`),
bağımlılıklar (`node_modules/`, `.venv/`), build çıktısı, loglar, editör/OS artıkları
ve `.tionharness/`. **Var olan `.gitignore` asla ezilmez**; zaten repo olan klasöre
hiç dokunulmaz. Dosya kullanıcının deposunda düz metindir, serbestçe düzenlenebilir.

Yanıttaki `gitInstalled` alanı makinede git binary'si olup olmadığını söyler
(`exec.LookPath`); yoksa UI `git init` butonu yerine "git kurulu değil" der ve
endpoint 400 döner. Aynı kontrol workspace oluşturma modalındaki "Git deposu
başlat" onay kutusunu da yönetir (bkz. `_Docs/06-WORKSPACES.md`).
Secrets de NavRail'den çıkıp **Ayarlar ▸ Sırlar** alt-kategorisine taşındı; UI'da
"Beceri/Beceriler" terimleri **Skill/Skills**'e çevrildi.
