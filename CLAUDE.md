# TionSwarm — Ajan Rehberi

Bu dosya, TionSwarm deposunda çalışan ajanlar için tekrar eden friction'dan
türetilmiş kısa kurallar içerir. Terminal: PowerShell veya Git-Bash.

## Grep/ripgrep kullanımı

Buradaki yerleşik `Grep` aracı **ripgrep** sözdizimi kullanır (POSIX `grep`
değil). Gerçek yaşanan hatalardan çıkarılmış kurallar:

- **Literal süslü/parantez karakterlerini kaçır.** Regex meta karakteri olan
  `{}` `()` `[]` işaretlerini düz metin ararken ters bölü ile kaçır.
  Örnek: bir Go interface aramak için `interface\{\}` yaz, `interface{}` değil.

- **Alternation için düz `|` kullan.** Shell alışkanlığıyla `a\|b` yazma —
  ripgrep bunu "regex parse error" olarak reddeder. Grep aracına `pattern`
  değerini `error|warning|fatal` biçiminde, kaçırılmamış boru ile ver.

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

```powershell
cd frontend
npm run format         # yaz (src/**/*.{ts,tsx,css})
npm run format:check   # sadece kontrol
```

**Pre-commit hook** (`.githooks/pre-commit`) yalnız **stage'lenmiş** dosyaları
formatlar (frontend → prettier, `*.go` → gofmt) ve yeniden stage'ler. Klon başına
bir kez: `git config core.hooksPath .githooks`. Tek seferlik atlamak için
`git commit --no-verify`.

Depo tarihsel olarak elle formatlanmış: `format:check` şu an ~248 dosyada uyarı
verir. Kasıtlı olarak toplu format ATILMADI — dokunulan dosya hook ile kendiliğinden
dönüşür. Toplu geçiş yapılacaksa **temiz ağaçta, kendi commit'inde** (`npm run format`).

## Test koşturma

```powershell
$env:TIONSWARM_ENABLE_SHELL='1'   # yoksa shell aracı testleri skip'e düşer
go test ./... -count=1            # tüm backend (~90sn)
cd frontend; npm test             # vitest (pure-logic modüller)
```

CI `.gitea/workflows/ci.yml`'dedir — **repo'nun tek remote'u Gitea'dır, GitHub değil**
(`git remote -v`), yani `.github/workflows` altına konan hiçbir şey çalışmaz. `ci.yml`
tam olarak yukarıdakini koşar (`-race` ile) ve `deploy.yml` `needs: test` ile ona bağlıdır.

**Paket alt-kümesi geçidi kurma** — daha önce 4 pakete daralmış ve tam da en çok değişen
paketleri (`agent`/`api`/`tools`) kapsamaz hale gelmişti.

- **`.go` dosyalarına BOM yazma.** `go build`/`go vet` tolere eder ama cover instrumentation
  dosyayı yeniden yazınca BOM ortada kalır ve paket `invalid BOM in the middle of the file`
  ile derlenmez → `go test -cover` o pakette tamamen çöker.
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
| `store_artifact.go` | Artifacts; oturum başına tekil "onaylanan planlar" rolling artifact |
| `store_task.go` | Kanban board görevleri (task kartları) ve board değişiklik olayları |
| `store_flow.go` | Flows ve flow run'ları |
| `store_run.go` | Çalışan run'ların (running) listelenmesi |
| `store_schedule.go` | Schedules (zamanlanmış çalıştırmalar) |
| `store_hook.go` | Hook yapılandırmaları |
| `store_mcp.go` | MCP sunucu yapılandırmaları |
| `store_tools.go` | Workspace düzeyi araç etkinleştirme/görünürlük yapılandırması (`tools-config.json`) |
| `store_automation.go` | Otomasyon; etiket (tags) normalizasyonu yardımcıları |
| `store_search.go` | Oturumlar arası mesaj araması (`SearchHit`, `SearchOpts`) |
| `store_usage.go` | Ajan/gün bazlı LLM kullanım (usage) rollup'ı ve çağrı taksonomisi |
| `store_session_usage.go` | Oturum ömrü boyunca LLM kullanım rollup'ı (`SessionUsage`) |
| `store_lessons.go` | Workspace düzeyi hata→ders (lessons) JSONL store'u (`lessons.jsonl`) |

İlgili model tanımları ayrı `models_*.go` dosyalarındadır (ör.
`models_task.go`, `models_flow.go`, `models_hook.go`, `models_mcp.go`,
`models_artifact.go`, `models_automation.go`, `models_attachment.go`). Ana
`Usage`/`Session`/`Agent` modelleri `models.go` içindedir. `store_*_test.go`
dosyaları ilgili store dosyalarının testleridir.
