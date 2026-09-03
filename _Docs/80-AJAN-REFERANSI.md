# 80 — Ajan Referansı (CLAUDE.md'den taşınan ayrıntılar)

> **Özet (2026-09-03):** Kök `CLAUDE.md` her Claude Code oturumuna ~10k token yüklediği
> için (ölçüm: `_Docs/17` "claude-cli prefix anatomisi") yalnız her turda geçerli kurallar
> orada kaldı; nadiren gereken referans bilgisi buraya taşındı. Bir ajan bu dosyaya yalnız
> ilgili bölüm gerektiğinde bakmalı: `internal/db` dosya haritası, codebase-memory-mcp
> `project` argümanı, ertelenmiş (deferred) araç yükleme, Playwright MCP çıktı kökü, kısmi
> commit reçetesi, Go araç zinciri keşfi ve `website/` notları.

## 1. codebase-memory-mcp — `project` argümanı

`mcp__codebase-memory-mcp__*` sorgu araçları (`search_code`, `search_graph`,
`query_graph`, `trace_path`, `get_code_snippet`, `get_architecture`, `index_status`)
**`project`** argümanı ister — bu `repo_path` değil, indeksleme çıktısındaki yol-tabanlı
kimliktir (format: `C-Users-user-Desktop-<repo-yolu>`). Bu depo için:
`C-Users-user-Desktop-Projects-TionHarness`.

- İlk çağrıdan önce bir kez `list_projects` çalıştır; `project` değerini listeden birebir
  kopyala.
- `project` hiç gönderilmezse sunucu yanlış kimlik gönderilmiş gibi
  `"project not found or not indexed"` döndürür — bu tek başına "indeksli değil" demek
  değildir. Hata `available_projects` ile gelirse aynı çağrıyı tekrarlama, doğru kimliği
  kopyala. Repo listede yoksa bu MCP'yi kullanma; `Glob`/`Grep`'e düş.
- TionHarness'in kendi ajan döngüsü eksik `project`'i oturumun cwd'sinden doldurur ve
  düzeltilebilir kimlik hatasını onarır (`internal/agent/mcpargs.go`, `mcprepair.go`).
  **claude-cli sağlayıcısında bu koruma yoktur**; orada kuralları elle uygula.

## 2. Ertelenmiş (deferred) araçlar

TionHarness bazı araçları şema yüklemeden sunar. Doğrudan çağrı `InputValidationError`
verir — araç yok değildir, şeması yüklenmemiştir.

- TionHarness araçları: `mcp__tionharness_interaction__activate_tools`; harici
  `mcp__<server>__*` araçları: `ToolSearch` ile `select:<ad>`.
- Şema yüklendikten sonra aracı namespaced tam adıyla çağır. Bir turda gereken tüm araçları
  tek çağrıda yükle (`select:a,b,c`).
- `activate_tools` bundle anahtarı da kabul eder (`group:<kategori>`, `mcp:<sunucu>`)
  ama demet açmak şema yüklemez, yalnız üyeleri listeler; ikinci çağrıda adları ver.
  Detay: `19-LAZY-TOOL-LOADING.md`, `52-MCP-GATEWAY.md` §13.

## 3. Playwright MCP

`browser_take_screenshot` / PDF çıktıları yalnız MCP'nin izinli kökü altına yazılabilir.
Runtime bu kökü her istekte aktif oturumun scratchpad'ine ayarlar
(`internal/agent/mcp_playwright.go`): **dosya adı ver, mutlak yol verme**; gerekirse
`Read` ile geri oku.

## 4. Go araç zinciri keşfi

Ortamı tahmin etme: `printf 'shell=%s\n' "$SHELL"`, `command -v go`, `command -v gofmt`,
`where.exe go`. `command -v` sonuç verirse o çağrıyı kullan. Yalnız `where.exe` Windows
yolu bulursa `/c/...` biçiminin çalışacağını varsayma; `command -v powershell.exe` ile
köprüyü doğrula ve gereken alt komutu Bash içinden ver:

```bash
powershell.exe -NoProfile -Command '& (Get-Command go).Source version'
```

`GOROOT` değerini sabit yazma; `go env GOROOT` çağır. Araç yoksa biçimlendirme/test
yapılmış gibi raporlama. Windows'a özel kod için
`GOOS=windows go test ./...` (Git Bash'te ortam atamasını komutun önüne koy).

## 5. Kısmi commit (`git add -p`) ve pre-commit

`.githooks/pre-commit` stage'lenmiş her `.go` dosyasını `gofmt` ile biçimlendirip
**dosyanın tamamını** yeniden stage'ler (BOM soyma da aynı). `git add -p` ile seçilen
kısmi hunk'lar commit anında geçersizleşir. Bir dosyanın yalnız bir kısmını commit'lemek
için:

```bash
git add -p -- <dosya>
cp <dosya> /tmp/dosya.full
git checkout-index -f -- <dosya>
git commit -m "..."
cp /tmp/dosya.full <dosya>
```

`--no-verify` çözüm değildir: hook `gofmt` ve BOM soyma görevini de yapar.

## 6. Test koşturma ayrıntıları

- Harici araç isteyen testler `t.Skip` ile geçitlenir (rg, python, node, claude CLI, ağ).
  Yeni test aynı deseni izlemeli.
- Arka plan goroutine'i başlatan test dönmeden önce `drainSpawns(t, rt)` çağırmalı
  (`internal/agent/spawn_test.go`): Windows açık dosyayı silmeyi reddeder ve
  `t.TempDir()` temizliği rastgele düşer. `SpawnSession`, `ReportToCoordinator`,
  `NotifyCoordinator` çağıran her testin sonuna koy; rota grafını okuyan test
  `drainSpawns` veya `waitTrajectory` kullanmalı.
- `.go` dosyalarına BOM yazma: cover instrumentation dosyayı yeniden yazınca
  `invalid BOM in the middle of the file` ile paket derlenmez.
- `//go:embed`'lenen varlıklar LF kalmalı; `.gitattributes`
  `internal/*/defaults/** text eol=lf` bunu sabitler. Yeni gömülü dizin eklersen kalıbın
  kapsadığından emin ol.
- Sandbox grep aracı `--no-ignore-parent` ile koşar: `%TEMP%\.gitignore` gibi üst dizin
  ignore dosyaları sonuçları gizleyemez (2026-09-03, `internal/tools/grep_rg.go`).
- `-race` bu makinede CGO kapalı olduğu için koşmaz; CI (linux) koşar.
- CI: `.github/workflows/` (release + Pages) ve `.gitea/workflows/ci.yml` (yalnız
  doğrulama). Paket alt-kümesi geçidi kurma; en çok değişen paketler dışarıda kalır.

## 7. `website/` — tanıtım sitesi

Depoda iki ayrı npm projesi vardır. `frontend/` uygulamanın arayüzüdür ve binary'ye
gömülür; `website/` statik tanıtım sitesidir (Astro), Go modülünün dışındadır. Komutları
`cd website` içinden koştur; `frontend/.prettierrc.json` ve pre-commit prettier adımı
yalnız `frontend/` içindir.

- Doğrulama: `cd website; npm run build` + `npm run check`.
- Placeholder kuralı: projede henüz olmayan her şey `website/src/site.config.ts`'te
  `null` durur ve bileşenler "Coming soon" render eder.
- Elle senkronlanan iki dosya: `website/src/styles/theme.css` ← `frontend/src/index.css`,
  `website/src/content/themes.ts` ← `frontend/src/shared/lib/themePresets.ts`.
- Site metni için kök `README.md`; güncel durum kaynağı `00-GENEL-BAKIS.md` +
  `05-ILERLEME.md`; çelişirse `_Docs` kazanır. Detay: `72-TANITIM-SITESI.md`.

## 8. internal/db dizin haritası

`internal/db` tek bir dosya-tabanlı store'dur (`DB` tipi, `store.go` ve `db.go`). Her
varlık ayrı bir `store_*.go` dosyasında sahiplenilir. Kesin liste için
`ls internal/db/*.go | grep -v _test.go`.

| Dosya | Sahiplendiği alan |
|-------|-------------------|
| `store.go` | Agents, Sessions, Messages ve boot yüklemesi |
| `db.go` | `DB` tipi, açılış, dizin sabitleri, kilit/persist yardımcıları |
| `store_messages_read.go` | Dar transkript okuyucuları: `ListMessagesTail`, `LastMessage`, `FindMessage`, `StreamMessages` |
| `store_stats.go` | `Stats()`: RAM ayak izi + boot faz süreleri |
| `loadpar.go` | `parallelLoad`: boot dosya okumaları için worker havuzu |
| `store_artifact.go` | Artifacts; oturum başına "onaylanan planlar" rolling artifact |
| `store_task.go` | Kanban görevleri ve board değişiklik olayları |
| `store_flow.go` | Flows ve flow run'ları |
| `store_runcount.go` | Running flow-run sayacının drift koruması |
| `store_flow_gc.go` | Flow koşusu GC: `DeleteFlowRunTree`, `PruneFlowRuns` |
| `store_flow_testhooks.go` | Yalnız test: `SetFlowRunCreatedAtForTest` |
| `store_schedule.go` | Schedules |
| `store_hook.go` | Hook yapılandırmaları |
| `store_mcp.go` | MCP sunucu yapılandırmaları |
| `store_tools.go` | Workspace düzeyi araç etkinleştirme/görünürlük (`tools-config.json`) |
| `store_automation.go` | Otomasyon; etiket normalizasyonu |
| `store_search.go` | Oturumlar arası mesaj araması |
| `store_usage.go` | Ajan/gün bazlı LLM kullanım rollup'ı ve çağrı taksonomisi |
| `store_session_usage.go` | Oturum ömrü boyunca LLM kullanım rollup'ı |
| `store_lessons.go` / `store_lessons_dedupe.go` | Hata→ders JSONL store'u ve tekilleştirme |
| `store_activity.go` | Mesaj ekleme aktivite sinyali ve gözlemci hook'u |
| `store_agent_system.go` | Yerleşik (system) ajan tanımları; silme yerine devre dışı bırakma |
| `store_session_ask.go` | Durable Ask; Rota F5 faz kapıları da burada (`Kind: ask` + payload `gate`) |
| `models_session_origin.go` | `SessionOrigin`, `Lineage()`/`RootSession()` |
| `store_session_hook.go` | `SetSessionHook` — oturum yaşam döngüsü gözlemcisi |
| `sidecar.go` | `Sidecar[T]` — varlığın yanında tipli JSON dosyası, atomik yazım, bozuk dosya karantinası |
| `models_trajectory.go` / `store_trajectory.go` | Rota modeli ve deposu (`sessions/<root>/trajectory.json`, `trajectories/index.json`, `UpdateTrajectory` CAS) |
| `store_curator.go` / `store_optimizer.go` / `store_pin.go` | Küratör raporu, reçete optimizer bookkeeping, küratörden muafiyet bayrağı (Rota F3/F4) |
| `store_hook_testhooks.go` | Yalnız test: `SetHookCreatedAtForTest` |
| `store_model_resolution.go` | İstenen model id → servis edilen model eşlemesi |
| `filestore.go` | Varlıktan bağımsız generic CRUD/persist yapı taşları |
| `board_columns.go` | Kanban kolon anahtarı doğrulama ve yerleşik kolon sabitleri |
| `automation_limits.go` / `automation_trigger.go` | Otomasyon iterasyon limitleri; tetik registry'si (`RegisterTrigger`) |
| `store_automation_fires.go` | Otomasyon ateşleme defteri (`automation-fires/<id>.jsonl`, 500 kayıt) |
| `artifact_content.go` / `artifact_migrate.go` | Metin artifact gövdelerinin dosyada tutulması; ek göçü |
| `inbox.go` / `inflight.go` / `promptepoch.go` | Kuyruklanmış mesaj sidecar'ı; akış anlık görüntüsü; dondurulmuş prompt-prefix |
| `render_cleanup.go` | `render_template` çıktı TTL'i ve açılış temizliği |
| `debug_journal.go` | `debug.jsonl` tanılama olayları, özet ve anomali tespiti |

Model tanımları `models_*.go` dosyalarındadır (`models_task.go`, `models_flow.go`,
`models_hook.go`, `models_mcp.go`, `models_artifact.go`, `models_automation.go`,
`models_attachment.go`); `Usage`/`Session`/`Agent` `models.go` içindedir.

**Mesaj okurken `ListMessages` varsayılan değildir**: oturumun tüm mesaj slice'ını
kopyalar. Son mesaj için `LastMessage`, son N için `ListMessagesTail`, id ile tek mesaj
için `FindMessage`, tarama için `StreamMessages` kullan.

## 9. internal/billing

`billing/billing.go` — fiyat hesaplama (`PriceStat`, `NoCacheCost`).
`internal/billing/model_change_poc_test.go` — model değişiminin geçmiş kayıtları yeniden
fiyatlamadığına dair regresyon testi (TSK67).
