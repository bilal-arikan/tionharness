# TionHarness — Ajan Rehberi

Yalnız **her turda geçerli** kurallar buradadır. Ayrıntılı referans (internal/db
haritası, deferred araçlar, Playwright, kısmi commit, BOM/LF, `website/`) →
`_Docs/80-AJAN-REFERANSI.md`. Doküman indeksi → `_Docs/00-GENEL-BAKIS.md`
(dokümanların başındaki **Özet** bloğunu önce oku).

## Araç zinciri

- Go araçlarının yerini tahmin etme (`command -v go`, `command -v gofmt`); Go dosyası
  değişince `gofmt -w <dosya>`. Ayrıntı `_Docs/80` §4.
- Arama: niyet → `zvec_grep_search`, sembol ilişkisi → `codebase-memory-mcp`, birebir
  string → `Grep`/`Glob`. codebase-memory `project` kimliği **makineye göre değişir**
  (depo yolunun ayraçları `-` yapılmış hali): Windows `C-Users-Bilal-Desktop-Projects-TionHarness`,
  bu Mac `Users-monster-Desktop-tionharness`. Emin değilsen `list_projects` ile bak.
- Edit `old_string` eşleşmezse kısa, benzersiz bir ASCII parça hedefle.

## Git

- Ana çalışma ağacı kullanıcıyla paylaşılır: `git reset --hard`, `git checkout -- .`,
  `git stash`, `git clean` **yasak**. Kendi dosyanı geri almak için yolu adıyla ver.
  İzole iş için `git worktree add`; `git checkout -b` "already exists" derse durdur
  (`git worktree list`).
- Çalışma ağacı LF'tir; `gofmt -l` bir dosya listeliyorsa gerçek hatadır.
- Kısmi commit reçetesi → `_Docs/80` §5. Commit yalnız istenince; committen önce
  `git diff --exit-code` ile boş yama kontrolü.

## Test

```bash
scripts/test.sh fast      # yalnız değişen Go paketleri + frontend vitest (saniyeler)
scripts/test.sh full      # go test ./... -count=1 + vitest + depcheck + git diff --check (~3-4 dk; teslimden önce zorunlu)
```

- Script dışında elle koşarken `TIONHARNESS_ENABLE_SHELL=1` ayarla.
- Goroutine başlatan test sonunda `drainSpawns(t, rt)` çağırır (ayrıntı `_Docs/80` §6).
- Paket alt-kümesi geçidi kurma; `full` teslim kapısıdır. Import yönü kuralı →
  `scripts/depcheck.sh`.

## Delegasyon

- `spawn_worker` / `run_subagent` için ajan adını önce `list_agents` ile doğrula.
- Yerleşik `Agent` aracının `subagent_type` listesi TionHarness ajan listesinden ayrıdır;
  ikisini karıştırma.
