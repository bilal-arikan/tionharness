# TionHarness — Ajan Rehberi

Bu dosya, TionHarness deposunda çalışan ajanlar için tekrar eden friction'dan
türetilmiş kısa kurallar içerir. Terminal: zorunlu Git Bash.

## Playwright MCP

`browser_take_screenshot` / PDF çıktıları **yalnızca MCP'nin izinli kökü**
altına yazılabilir. TionHarness bu kökü otomatik olarak **aktif oturumun
scratchpad'ine** (`<store>/sessions/<SID>/scratchpad`) ayarlar; oturum
scratchpad'ine **doğrudan mutlak yol vererek yazmaya çalışma** — dosyayı izinli
köke (varsayılan olarak orası) kaydet, gerekiyorsa `Read` ile geri oku. Kök
her istekte aktif oturumdan yeniden çözülür, dolayısıyla bayat oturum yolu
kullanma.
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
çıktısındaki yol-tabanlı kimliktir (format: `C-Users-user-Desktop-<repo>`).

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

- **`output_mode` yalnızca şu üç değeri kabul eder:** `content` |
  `files_with_matches` | `count`. Başka bir değer (ör. `text`, `lines`) geçersiz
  ve reddedilir.

- **Sabit dosya yolu vermeden önce Glob ile doğrula.** Grep'e var olmayan bir
  `path` verirsen sonuç boş/yanıltıcı döner ve hata da vermez. Önce
  `Glob` ile (ör. `internal/db/store_*.go`) dosyanın gerçekten var olduğunu
  teyit et, sonra o yolu Grep'e geç.

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

`format:check` şu an (2026-08-18 itibarıyla) **temiz** — depo genelinde uyarı yok.
Pre-commit hook stage'lenmiş dosyayı otomatik dönüştürüyor, bu yüzden tekil
düzenlemeler için elle `npm run format` koşmaya gerek yok. Yine de büyük bir
elle-düzenlenmiş dosya grubu eklenirse (ör. dışarıdan import edilen kod) kontrol
etmeden varsayma — `cd frontend && npm run format:check` ile ölç.

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
- Site metni yazarken **kök `README.md`'yi kaynak alma** — bayat. Kaynak:
  `_Docs\00-GENEL-BAKIS.md` + `_Docs\05-ILERLEME.md`.

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
- **Harici araç isteyen testler `t.Skip` ile geçitlenir** (rg, python, node, claude CLI, ağ).
  Yeni bir testin böyle bir bağımlılığı varsa aynı deseni izle, yoksa CI kırılır.
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
