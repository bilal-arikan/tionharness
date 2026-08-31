# TionHarness — Ajan Rehberi

Bu dosya, TionHarness deposunda çalışan ajanlar için tekrar eden friction'dan
türetilmiş kısa kurallar içerir. Terminal: zorunlu Git Bash.

## Playwright MCP

`browser_take_screenshot` / PDF çıktıları yalnızca MCP'nin izinli kökü altına
yazılabilir. Runtime bu kökü her istekte aktif oturumun scratchpad'ine ayarlar
(`internal/agent/mcp_playwright.go`), yani **dosya adı ver, mutlak yol verme** —
çıktı doğru yere düşer, gerekirse `Read` ile geri oku.

## Edit aracı — eşleşme

- **Edit'ten önce hedef bölgeyi `Read` ile oku.** `old_string`'i çıktının birebir
  kopyası olarak al: yalnız satır-no + tab ön ekini temizle, **içeriği normalize etme**
  — unicode (« » ✅ ⏳), emoji ve hizalama/boşluk karakterlerini olduğu gibi bırak.
  Hafızadan/özetten yeniden yazılan `old_string` çoğu kez birebir eşleşmez.
- Eşleşme tutmazsa **kısa, BENZERSIZ bir ASCII parça** hedefle (unicode noktalama ve
  satır-sonu boşluk en sık suçlulardır). Edit boşluk/hizalama farkını toleranslı
  eşleştirmeyle telafi eder ama yalnız tekil konumda; başarısızlıkta en yakın satırı ve
  ilk farklılaşan sütunu gösteren tanılayıcı hata döner (bkz. `computeEdit`,
  `_Docs\56-SELF-HEALING.md`).
## codebase-memory-mcp kullanımı (project argümanı)

`mcp__codebase-memory-mcp__*` sorgu araçları (`search_code`, `search_graph`,
`query_graph`, `trace_path`, `get_code_snippet`, `get_architecture`,
`index_status`) **`project`** argümanı ister — bu, `repo_path` değil, indeksleme
çıktısındaki yol-tabanlı kimliktir (format: `C-Users-user-Desktop-<repo-yolu>`).

- **İlk çağrıdan ÖNCE bir kez `list_projects` çalıştır.** `project` argümanını
  dönen listeden **birebir kopyala** — kimliği elle uydurma.
- `project`'i **hiç göndermezsen** sunucu, yanlış kimlik göndermişsin gibi aynı
  `"project not found or not indexed"` mesajını döndürür. Bu mesaj tek başına
  "repo indeksli değil" demek DEĞİLDİR — önce argümanı gerçekten yolladığını
  doğrula.
- Yanlış/indekslenmemiş bir `project` verirsen sunucu aynı hatayı +
  `available_projects` döndürür. Bu hata geldiğinde **aynı çağrıyı tekrarlama**;
  `available_projects`'ten doğru kimliği kopyala.
- Hedef repo listede **yoksa** bu MCP'yi kullanma; o repo için `Glob`/`Grep`'e düş.
- TionHarness'in **kendi** ajan döngüsü bunları büyük ölçüde otomatik halleder:
  eksik `project` gönderilmeden önce oturumun working directory'sinden doldurulur,
  düzeltilebilir bir kimlik hatası çağrı tekrar koşturularak onarılır, oturumun
  reposu indeksli değilse arka planda indeksleme tetiklenir
  (`internal/agent/mcpargs.go`, `mcprepair.go`). **claude-cli sağlayıcısında bu
  koruma yoktur** — araç döngüsünü CLI kendi koşturur, çağrılar TionHarness'ten
  geçmez; orada yukarıdaki kuralları elle uygula.
- Bu depoda sorgular için `project` = `C-Users-user-Desktop-Projects-TionHarness`.

## Deferred (ertelenmiş) araçlar

TionHarness bazı araçları **şema yüklemeden** sunar (oturum açılışındaki
"deferred tools" listesi). Böyle bir aracı doğrudan çağırmak `InputValidationError`
ile başarısız olur — araç yok değildir, şeması henüz yüklenmemiştir.

- TionHarness araçları için `mcp__tionharness_interaction__activate_tools` çağır;
  harici `mcp__<server>__*` araçları için `ToolSearch` ile `select:<ad>` sorgusu
  kullan (ör. `select:mcp__codebase-memory-mcp__search_graph`).
- Şema yüklendikten sonra aracı **namespaced tam adıyla** çağır; kısa/çıplak ad
  kullanma.
- Bir turda ihtiyaç duyacağın **tüm araçları tek çağrıda** yükle
  (`select:a,b,c`) — araç başına ayrı tur tüketme.
- `activate_tools` araç adının yanında **bundle anahtarı** da kabul eder:
  `group:<kategori>` (built-in kategorisi) ve `mcp:<sunucu>`. Ama **demet açmak
  şema YÜKLEMEZ** — yalnız o demetin üyelerini `ad — özet` satırlarıyla listeler
  (en fazla 40 üye), hiçbir araç aktif sete girmez.
- Yani akış iki adımdır: demeti aç (isimleri gör) → istediğin **adı/adları** ikinci
  bir `activate_tools` çağrısına ver (şema ancak o zaman gelir). Detay:
  `_Docs\19-LAZY-TOOL-LOADING.md`, `_Docs\52-MCP-GATEWAY.md` §13.

## Ajan delegasyonu

- `spawn_worker` / `run_subagent` çağrısında bir ajan adı vermeden önce o ajanın
  workspace'te gerçekten tanımlı olduğunu `list_agents` ile doğrula. Uydurulmuş
  ad, iş başlamadan hataya düşer.
- Yerleşik `Agent`/`Task` aracının `subagent_type` listesi (ör. `general-purpose`,
  `Explore`, `Plan`) TionHarness ajan listesinden **tamamen ayrıdır**. İki listeyi
  karıştırma: TionHarness ajan adını `subagent_type`'a, `subagent_type` değerini
  TionHarness delegasyon araçlarına geçirme.

## Ana çalışma ağacında yıkıcı git komutları

Ana çalışma ağacı (`C:\Users\user\Desktop\Projects\TionHarness`) kullanıcıyla
paylaşılır ve **commitlenmemiş değişiklik içerebilir**. Orada `git reset --hard`,
`git checkout -- .`, `git stash` ve `git clean` **yasaktır** — stage'lenmemiş bir
düzenlemenin nesne veritabanında blob'u yoktur, `--hard` onu kalıcı olarak siler
(reflog, `fsck`, editör local-history hiçbiri geri getirmez; bu bir kez yaşandı ve
iki dosyalık iş kayboldu).

- İzole çalışma gerektiğinde `git worktree add` ile ayrı bir ağaç aç; ana ağaçta
  branch değiştirme.
- `git checkout -b <ad>` "branch already exists" ile düşerse bu **bloklayıcıdır**:
  branch büyük olasılıkla başka bir worktree'de çekilidir (`git worktree list` ile
  doğrula). Ana ağaçta çalışmaya devam etme.
- Kendi değişikliklerini geri almak gerekiyorsa yalnız kendi dokunduğun yolları
  hedefle (`git checkout -- <dosya>`), asla `.` verme.

## Shell ve Go araç zinciri

- Bu Windows çalışma alanının zorunlu komut kabuğu **Git Bash**'tir. Komutları POSIX
  sözdizimiyle yaz; Windows yollarını Bash'te `/c/...` biçiminde kullan. PowerShell'i
  varsayılan yapma. Yalnız registry/cmdlet gibi Windows-native bir işlem gerçekten
  gerekirse Bash içinden `powershell -NoProfile -Command "..."` çağır.
- Ortamı tahmin etme: önce `printf 'shell=%s\n' "$SHELL"`, ardından
  `command -v go`, `command -v gofmt`, `where.exe go` ve `where.exe gofmt`
  çalıştır. `command -v` sonuç verirse bulunan Bash çağrısını kullan. Yalnız
  `where.exe` Windows yolu bulursa bu yolu `/c/...` biçimine çevirip çalışacağını
  varsayma; etkin Bash bunu desteklemeyebilir. `command -v powershell.exe` ile
  Windows köprüsünü doğrula, sonra gereken alt komutu Bash içinden açıkça ver:

  ```bash
  powershell.exe -NoProfile -Command '& (Get-Command go).Source version'
  powershell.exe -NoProfile -Command '& (Get-Command gofmt).Source -h' >/dev/null
  ```

  `Get-Command` da aracı bulamazsa bilinen Windows kurulumlarını salt-okunur kontrol
  et ve yalnız gerçekten çalışan çağrıyı kullan. Var olmayan veya bu kabukta
  çalışmayan sabit yolu fallback diye belgeleme. Araç yoksa biçimlendirme/test
  yapılmış gibi raporlama.
- `GOROOT` değerini sabit yazma ve `GOROOT`'u komut gibi çağırma. Etkin Go komutunu
  seçtikten sonra `go env GOROOT` eşdeğerini çalıştır; gerekiyorsa çıktıyı
  `powershell.exe -NoProfile -Command '& (Get-Command go).Source env GOROOT'`
  eşdeğerini kullan. PATH'te doğrudan Go bulunduysa `go env GOROOT` çağır.
- Go dosyası değişince hedef dosyalarda `gofmt -w` çalıştır; bu kullanılmayan
  importları da görünür kılar. Ardından ilgili testleri ve Windows'a özel kod için
  `GOOS=windows powershell.exe -NoProfile -Command '& (Get-Command go).Source test ./...'`
  doğrulamasını çalıştır (PATH'te doğrudan Go bulunduysa `GOOS=windows go test ./...`
  kullan). Git Bash'te ortam
  atamasını komutun önüne koy; PowerShell `$env:` sözdizimi kullanma.

## Grep/ripgrep kullanımı

Buradaki yerleşik `Grep` aracı **ripgrep** sözdizimi kullanır (POSIX `grep`
değil). Gerçek yaşanan hatalardan çıkarılmış kurallar:

- **Literal süslü/parantez karakterlerini kaçır.** Regex meta karakteri olan
  `{}` `()` `[]` işaretlerini düz metin ararken ters bölü ile kaçır.
  Örnek: bir Go interface aramak için `interface\{\}` yaz, `interface{}` değil.
  Bilinçli regex'te açılan her `(`, `[` ve `{` grubunu kapat; uzun deseni geniş
  aramaya vermeden önce küçük bir `rg -- "$pattern" CLAUDE.md` çağrısıyla doğrula.

- **Alternation için düz `|` kullan.** Shell alışkanlığıyla `a\|b` yazma —
  ripgrep bunu "regex parse error" olarak reddeder. Grep aracına `pattern`
  değerini `error|warning|fatal` biçiminde, kaçırılmamış boru ile ver.

- Shell'de dosya listesi için önce `rg --files` kullan. Literal kullanıcı girdisini
  `rg -F -- "$text"`, bilinçli regex'i `rg -- "$pattern"` ile ara. Tireyle
  başlayabilecek desenlerde `--` ayırıcısını koru; boş değişken veya geniş,
  doğrulanmamış glob ile arama başlatma.

## Dosya okuma aracı tercihi

- Kod arama ve gezinme için sıra: önce `codebase-memory-mcp` araçları
  (`search_code`, `search_graph` + `get_code_snippet`, `trace_path`,
  `query_graph`, `get_architecture`), sonra `Glob`/`Grep`. Ham shell grep
  (`rg`, `findstr`, `Select-String`) **son çare** — yalnız indeks gerçekten
  cevap veremediğinde.
- **Token optimizasyonunu elle çağırma.** `rtk` (komut katmanı) ve `sqz` (çıktı
  katmanı) hook olarak kuruludur; shell komutları çalıştırılmadan önce runtime
  tarafından otomatik yeniden yazılır ve çıktı otomatik sıkıştırılır
  (`internal/agent/capabilities_tokenopt.go`). Komutun başına elle `rtk` ekleme —
  ihtiyacın olan komutu düz yaz. Byte-exact çıktı gerekiyorsa shell aracına
  `no_compress: true` geç.

## Kod formatı (otomatik)

Frontend **Prettier** ile formatlanır — ayarlar `frontend/.prettierrc.json`
(tek tırnak, noktalı virgülsüz, `printWidth: 100`, `endOfLine: auto`). Elle
"prettier'i varsayılan ayarlarla çalıştırmak" YASAK: config'siz koşarsan dosyayı
çift tırnak + noktalı virgüle çevirir ve devasa sahte diff üretir.

```bash
cd frontend
npm run format         # yaz (src/**/*.{ts,tsx,css})
npm run format:check   # sadece kontrol
```

**Pre-commit hook** (`.githooks/pre-commit`) yalnız **stage'lenmiş** dosyaları
formatlar (frontend → prettier, `*.go` → gofmt) ve yeniden stage'ler. Klon başına
bir kez: `git config core.hooksPath .githooks`. Tek seferlik atlamak için
`git commit --no-verify`.

## Boş patch / gereksiz format koşusu

- `git apply` veya `apply_patch` çağırmadan önce yamanın **gerçekten değişiklik
  içerdiğini** doğrula: `git diff --exit-code` (0 = değişiklik yok). Boş yama
  `git apply` tarafından reddedilir; bilinçli olarak boş yama uygulanacaksa
  `--allow-empty` ver.
- Pre-commit hook stage'lenmiş dosyaları zaten formatlar; tekil düzenlemede elle
  format koşusu gereksizdir. Büyük bir elle-düzenlenmiş dosya grubu (ör. dışarıdan
  import edilen kod) eklendiyse önce `cd frontend && npm run format:check` ile ölç;
  temizse `npm run format` **koşma** — gereksiz koşu sahte diff üretir.

## Kısmi commit (`git add -p`) ve pre-commit

`.githooks/pre-commit` stage'lenmiş her `.go` dosyasını `gofmt` ile biçimlendirip
`git add <dosya>` ile **dosyanın tamamını** yeniden stage'ler (BOM soyma adımı da
aynısını yapar). Bu yüzden `git add -p` ile seçilen kısmi hunk'lar commit anında
geçersizleşir: dosyanın working tree'deki tüm değişiklikleri — o commit'e girmesini
istemediklerin dahil — commit'e sızar.

Bir dosyanın yalnız bir kısmını commit'lemek gerektiğinde:

```bash
git add -p -- <dosya>              # istenen hunk'ları stage'le
cp <dosya> /tmp/dosya.full         # tam sürümü sakla
git checkout-index -f -- <dosya>   # working tree = yalnız stage'li sürüm
git commit -m "..."                # hook artık aynı içeriği yeniden stage'ler
cp /tmp/dosya.full <dosya>         # tam sürümü geri yaz
```

`--no-verify` ile hook'u atlamak çözüm **değildir**: hook aynı zamanda `gofmt` ve
UTF-8 BOM soyma görevini yapar; atlarsan biçimsiz veya BOM'lu dosya commit'lenir.

## Teslim whitespace kapısı

- Markdown (`*.md`) ve YAML (`*.yml`, `*.yaml`) satırlarında trailing whitespace
  yasaktır. Üretilen veya düzenlenen dosyaları teslimden önce kontrol et; boşluk
  hatasını sonraki ajana bırakma.
- Her değişiklikten sonra depo kökünde `git diff --check` çalıştır. Çıktı varsa
  teslim **FAIL**'dir; tüm `trailing whitespace` ve `space before tab` hatalarını
  düzeltip komutu sıfır çıkış koduyla yeniden çalıştır.
- Pre-commit hook yalnız stage'lenmiş dosyalara baktığından commit yapılmayan
  görevlerde whitespace kanıtı yerine geçmez. Bu görevlerde de `git diff --check`
  zorunludur.

## website/ — tanıtım sitesi (frontend/ ile karıştırma)

Depoda **iki ayrı npm projesi** vardır. `frontend/` uygulamanın arayüzüdür ve binary'ye
gömülür; `website/` ise statik tanıtım sitesidir (Astro), Go modülünün dışındadır ve
hiçbir şeye gömülmez. Kendi `package.json`/`node_modules`'ü vardır — komutları
`cd website` içinden koştur. `frontend/.prettierrc.json` ve pre-commit prettier adımı
**yalnız `frontend/`** içindir; `website/` dosyalarını oraya sokma.

- Doğrulama: `cd website; npm run build` + `npm run check` (0 hata beklenir).
- **Placeholder kuralı:** projede henüz olmayan her şey (repo/release/docs/lisans/sürüm)
  `website/src/site.config.ts`'te `null` olarak durur ve bileşenler bunu ölü link yerine
  "Coming soon" olarak render eder. Yeni bir "henüz yok" alanı **bileşenin içine değil**
  bu dosyaya eklenir.
- **İki dosya uygulamadan elle senkronlanır**, uygulamada tema değişirse ikisi de
  güncellenmelidir: `website/src/styles/theme.css` ← `frontend/src/index.css`,
  `website/src/content/themes.ts` ← `frontend/src/shared/lib/themePresets.ts`.
- Site metni için kök `README.md` kullanılabilir (artık `_Docs` ile hizalı), ama
  ayrıntı ve güncel durum kaynağı `_Docs\00-GENEL-BAKIS.md` +
  `_Docs\05-ILERLEME.md`'dir; üçü çelişirse `_Docs` kazanır.

Detay: `_Docs\72-TANITIM-SITESI.md`.

## Test koşturma

```bash
export TIONHARNESS_ENABLE_SHELL=1  # yoksa shell aracı testleri skip'e düşer
go test ./... -count=1             # tüm backend (~90sn)
cd frontend && npm test            # vitest (pure-logic modüller)
```

CI iki yerdedir: `.github/workflows/` (yayın hattının sahibi — release + Pages) ve
`.gitea/workflows/ci.yml` + `release.yml` (yalnız doğrulama, hiçbir şey yayınlamaz).
Hangi remote'ların bağlı olduğunu `git remote -v` ile doğrula. `ci.yml` tam olarak
yukarıdaki test setini koşar (`-race` ile). VPS deploy hattı kaldırılmıştır.

**Paket alt-kümesi geçidi kurma** — daha önce 4 pakete daralmış ve tam da en çok değişen
paketleri (`agent`/`api`/`tools`) kapsamaz hale gelmişti.

- **`.go` dosyalarına BOM yazma.** `go build`/`go vet` tolere eder ama cover instrumentation
  dosyayı yeniden yazınca BOM ortada kalır ve paket `invalid BOM in the middle of the file`
  ile derlenmez → `go test -cover` o pakette tamamen çöker. Pre-commit hook artık stage'lenmiş
  her `.go` dosyasından BOM'u soyar ve `pre-commit: stripped UTF-8 BOM from <dosya>` diye
  bildirir, ama bu yalnız hook kuruluysa (`git config core.hooksPath .githooks`) ve
  `--no-verify` kullanılmadıysa korur — hâlâ BOM'suz yazmak esas kuraldır.
- **`//go:embed`'lenen varlıklar LF kalmalı.** Windows'ta `core.autocrlf=true`, kökteki
  `* text=auto` kuralıyla birleşince gömülü dosyaların çalışma kopyasını CRLF'e çevirir;
  Go kaynağındaki `\n` tabanlı sabitlerle bayt-bayt karşılaştırılan testler (ör.
  `TestResolveLessonConfigDefaultPathUnchanged`) her taze klonda düşer. `.gitattributes`
  bunu `internal/*/defaults/** text eol=lf` ile sabitler — depodaki her `//go:embed` kökü
  bu kalıbın altındadır (`internal/web/dist` hariç: takip edilmeyen derleme çıktısı).
  Yeni bir gömülü varlık dizini eklersen kalıbın kapsadığından emin ol; mevcut bir çalışma
  kopyasını düzeltmek için `git add --renormalize .` yetmez, dosyaları silip
  `git checkout --` ile geri almak gerekir.
- **Harici araç isteyen testler `t.Skip` ile geçitlenir** (rg, python, node, claude CLI, ağ).
  Yeni bir testin böyle bir bağımlılığı varsa aynı deseni izle, yoksa CI kırılır.
- **Arka plan goroutine'i başlatan test, dönmeden önce `drainSpawns(t, rt)` çağırmalı**
  (`internal/agent/spawn_test.go`). Windows açık dosyayı silmeyi reddeder, bu yüzden
  `t.TempDir()` temizliği hâlâ yazan bir goroutine'e denk gelirse test
  `TempDir RemoveAll cleanup: ... Dizin boş değil` ile **rastgele** düşer (POSIX'te
  görünmez). `spawnActive`'in sıfıra inmesi tek başına yetmez: bir worker'ın son işi
  koordinatörünü bilgilendirmektir ve `NotifyCoordinator` → `drainCoordinator`'ı
  **ayrı bir goroutine'de** başlatır; o sayaçta yer almaz. `drainSpawns` artık
  `backgroundWorkPending()` ile koordinatör drain'ini ve oturum tur slotlarını da
  bekler — `SpawnSession`, `ReportToCoordinator` veya `NotifyCoordinator` çağıran
  her testin sonuna koy.
- `-race` bu makinede CGO kapalı olduğu için koşmaz; CI (linux) koşar.

## internal/db dizin haritası

`internal/db` paketi tek bir dosya-tabanlı store'dur (`DB` tipi, `store.go` ve
`db.go`). Her varlık ayrı bir `store_*.go` dosyasında sahiplenilir. Aşağıdaki
liste yalnızca depoda **gerçekten var olan** dosyaları içerir.

| Dosya | Sahiplendiği alan |
|-------|-------------------|
| `store.go` | Agents, **Sessions**, **Messages** ve önyükleme (boot) yüklemesi — çekirdek store |
| `db.go` | `DB` tipi, açılış, dizin sabitleri, kilit/persist yardımcıları |
| `store_messages_read.go` | Dar transkript okuyucuları: `ListMessagesTail`, `LastMessage`, `FindMessage`, `StreamMessages` — tam kopya çıkarmadan kuyruk/tek-mesaj/tarama |
| `store_stats.go` | `Stats()`: store'un RAM ayak izi (yüklü oturum, mesaj sayısı, yaklaşık bayt) + boot faz süreleri |
| `loadpar.go` | `parallelLoad`: boot'taki dosya okumalarını sınırlı worker havuzuyla eşzamanlı koşturur |
| `store_artifact.go` | Artifacts; oturum başına tekil "onaylanan planlar" rolling artifact |
| `store_task.go` | Kanban board görevleri (task kartları) ve board değişiklik olayları |
| `store_flow.go` | Flows ve flow run'ları |
| `store_runcount.go` | Running flow-run sayacının drift koruması (`ReconcileRunCounters`) |
| `store_schedule.go` | Schedules (zamanlanmış çalıştırmalar) |
| `store_hook.go` | Hook yapılandırmaları |
| `store_mcp.go` | MCP sunucu yapılandırmaları |
| `store_tools.go` | Workspace düzeyi araç etkinleştirme/görünürlük yapılandırması (`tools-config.json`) |
| `store_automation.go` | Otomasyon; etiket (tags) normalizasyonu yardımcıları |
| `store_search.go` | Oturumlar arası mesaj araması (`SearchHit`, `SearchOpts`) |
| `store_usage.go` | Ajan/gün bazlı LLM kullanım (usage) rollup'ı ve çağrı taksonomisi |
| `store_session_usage.go` | Oturum ömrü boyunca LLM kullanım rollup'ı (`SessionUsage`) |
| `store_lessons.go` | Workspace düzeyi hata→ders (lessons) JSONL store'u (`lessons.jsonl`) |
| `store_lessons_dedupe.go` | Lesson tekilleştirme — birebir `Signature` eşleşmesinin ötesinde |
| `store_activity.go` | Mesaj ekleme aktivite sinyali (`ActivitySignal`) ve gözlemci hook'u |
| `store_agent_system.go` | Yerleşik (system) ajan tanımları; silme yerine devre dışı bırakma (`ErrSystemAgentDelete`) |
| `store_session_ask.go` | Durable Ask — oturuma sorulan ve yanıt bekleyen sorular |
| `store_model_resolution.go` | İstenen model id → sağlayıcının gerçekte servis ettiği model eşlemesi |
| `filestore.go` | Varlıktan bağımsız generic CRUD/persist yapı taşları; `store_*.go` bunlara delege eder |
| `board_columns.go` | Kanban kolon anahtarı doğrulama (`IsValidBoardKey`) ve yerleşik kolon sabitleri |
| `automation_limits.go` | Otomasyon iterasyon limitleri (`MaxIterations`) ve doğrulaması |
| `artifact_content.go` | Metin türü artifact gövdelerinin JSON içinde değil `<workspace>/artifacts/` altında dosya olarak tutulması |
| `artifact_migrate.go` | Sohbet eklerinin (attachment) artifact türüne göçü |
| `inbox.go` | Oturum başına kuyruklanmış mesaj sidecar'ı (`inbox.json`), atomik yazım |
| `inflight.go` | Akış hâlindeki yanıtın disk anlık görüntüsü (`inflight.json`) — çökme kurtarma |
| `promptepoch.go` | Oturum başına dondurulmuş prompt-prefix anlık görüntüsü (opak JSON) |
| `render_cleanup.go` | `render_template` çıktı dosyalarının TTL'i ve açılıştaki temizlik taraması |
| `debug_journal.go` | `debug.jsonl` — kullanıcıya görünmeyen tanılama olayları, özet ve anomali tespiti |

Tablo elle tutulur ve **tam olmayabilir**; kesin liste için
`ls internal/db/*.go | grep -v _test.go`.

**Mesaj okurken `ListMessages` varsayılanın DEĞİL.** O, oturumun tüm mesaj
slice'ını her çağrıda kopyalar (uzun bir oturumda megabaytlarca `Steps` JSON'u).
Son mesaj için `LastMessage`, son N için `ListMessagesTail` (ikinci dönüş değeri
kuyruğun başlangıç indeksidir), id ile tek mesaj için `FindMessage`, "hepsini gez
ama neredeyse hiçbir şey tutma" için `StreamMessages` kullan. Tam transkripti
yalnız gerçekten tamamı gerektiğinde (tur bağlamı, compaction) iste.

İlgili model tanımları ayrı `models_*.go` dosyalarındadır (ör.
`models_task.go`, `models_flow.go`, `models_hook.go`, `models_mcp.go`,
`models_artifact.go`, `models_automation.go`, `models_attachment.go`). Ana
`Usage`/`Session`/`Agent` modelleri `models.go` içindedir. `store_*_test.go`
dosyaları ilgili store dosyalarının testleridir.

## internal/billing

`billing/billing.go` — fiyat hesaplama (`PriceStat`, `NoCacheCost`).
`internal/billing/model_change_poc_test.go` — model değişiminin geçmiş kayıtları
yeniden fiyatlamadığına dair regresyon testi (TSK67).
