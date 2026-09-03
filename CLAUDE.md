# TionHarness — Ajan Rehberi

Yalnız **her turda geçerli** kurallar buradadır. Ayrıntılı referans (internal/db
haritası, codebase-memory `project` argümanı, deferred araçlar, Playwright, kısmi commit,
`website/`) → `_Docs/80-AJAN-REFERANSI.md`. Doküman indeksi → `_Docs/00-GENEL-BAKIS.md`
(her dokümanın başında 3–5 satırlık **Özet** bloğu var; önce onu oku).

## Kabuk ve araç zinciri

- Zorunlu kabuk **Git Bash**; POSIX sözdizimi, Windows yolları `/c/...`. PowerShell'i
  varsayılan yapma; Windows-native bir işlem gerçekten gerekirse
  `powershell.exe -NoProfile -Command "..."` çağır.
- Araçların yerini tahmin etme: `command -v go`, `command -v gofmt`. Go dosyası
  değişince `gofmt -w <dosya>`; araç yoksa formatlama/test yapılmış gibi raporlama.
- `rtk`/`sqz` token optimizasyonu hook olarak kuruludur; komutun başına elle `rtk`
  ekleme.
- Kod arama sırası: `codebase-memory-mcp` (`project` =
  `C-Users-user-Desktop-Projects-TionHarness`) → `Glob`/`Grep` → ham shell grep.
- `Grep` aracı **ripgrep** sözdizimidir: literal `{}` `()` `[]` kaçırılır
  (`interface\{\}`), alternation düz `|` (`a\|b` değil).

## Düzenleme

- Edit'ten önce hedef bölgeyi `Read` ile oku; `old_string` çıktının birebir kopyası olsun
  (unicode/emoji/boşluğu normalize etme). Eşleşmezse kısa, benzersiz bir ASCII parça hedefle.
- Kod, yorum ve metin İngilizce; dokümanlar Türkçe. Yeni kodu olabildiğince ayrı dosyalara böl.
- `.go` dosyalarına BOM yazma; `//go:embed` varlıkları LF kalmalı.

## Git

- Ana çalışma ağacı kullanıcıyla paylaşılır: `git reset --hard`, `git checkout -- .`,
  `git stash`, `git clean` **yasak**. Kendi dosyanı geri almak için yolu adıyla ver.
  İzole iş için `git worktree add`; `git checkout -b` "already exists" derse durdur
  (`git worktree list`).
- Satır sonları: depo yapılandırması `core.autocrlf=false` + `core.eol=lf`; çalışma
  ağacı LF'tir. Dosya yazarken LF kullan; `gofmt -l` bir dosya listeliyorsa gerçek hata.
- Pre-commit hook (`git config core.hooksPath .githooks`) stage'lenmiş dosyaları
  prettier/gofmt ile formatlar ve BOM soyar. Kısmi commit reçetesi → `_Docs/80` §5.
- Frontend `frontend/.prettierrc.json` ile formatlanır; config'siz prettier koşma.
  `cd frontend && npm run format:check` temizse `npm run format` koşma.
- Teslimden önce depo kökünde `git diff --check`; çıktı varsa teslim FAIL.
- Commit yalnız istenince; committen önce `git diff --exit-code` ile boş yama kontrolü.

## Test

```bash
scripts/test.sh fast      # yalnız değişen Go paketleri + frontend vitest (dakikalar değil saniyeler)
scripts/test.sh full      # go test ./... -count=1 + vitest (~3-4 dk; teslimden önce zorunlu)
```

- `TIONHARNESS_ENABLE_SHELL=1` script tarafından ayarlanır; elle koşarken sen ayarla.
- Harici araç isteyen testler `t.Skip` ile geçitlenir; goroutine başlatan test sonunda
  `drainSpawns(t, rt)` çağırır (ayrıntı `_Docs/80` §6).
- Paket alt-kümesi geçidi kurma; `full` teslim kapısıdır.

## Delegasyon

- `spawn_worker` / `run_subagent` için ajan adını önce `list_agents` ile doğrula.
- Yerleşik `Agent` aracının `subagent_type` listesi TionHarness ajan listesinden ayrıdır;
  ikisini karıştırma.

## Proje skill'leri (`.claude/skills/`)

- `/test` — yukarıdaki test script'ini doğru modda koşturur ve sonucu yorumlar.
- `/run-app` — uygulamayı `.claude/launch.json` ile başlatıp tarayıcıda açar.
- `/docs-update` — bir değişiklikten sonra `_Docs/05-ILERLEME.md` girişi + ilgili
  dokümanın Özet bloğunu güncelleme kalıbı.
