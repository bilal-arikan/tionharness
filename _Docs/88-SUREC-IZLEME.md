# 88 — Süreç izleme (process ledger)

> **Özet (2026-09-23, güncelleme 2026-11-17):** TionHarness'in ajanlar adına başlattığı **yerel işletim
> sistemi süreçleri** artık tek bir defterde toplanıyor: `internal/procwatch`.
> `run_code`/`transform_data`
> yorumlayıcıları, ajansal CLI taşıyıcıları (claude-cli/codex-cli), stdio MCP
> sunucuları, hook komutları ve harici araç koşuları (ripgrep, indeks derlemeleri,
> araç güncellemeleri) her biri bir kayıt açar; kayıt komut satırını, PID'i, sahibini
> (workspace + oturum + ajan), durumunu, çıkış kodunu ve kısa bir çıktı kuyruğunu
> taşır. Okuma yüzeyleri: `GET /api/workspace/processes`, `list_processes` ajan
> aracı ve Workspace ekranının **İşlemler** sekmesi (canlı, SSE ile tazelenir).
> Durdurma yalnızca **kullanıcı** eylemidir (panelden, onaylı); ajan aracı salt
> okunurdur. Durum: **uygulandı (TSK1040)**. Bir ajan için: yeni bir yerde süreç
> başlatıyorsan enstrümantasyonu buradan oku — defterde görünmeyen süreç, bu
> özelliğin engellemek için var olduğu şeyin ta kendisidir.
> **2026-11-17:** TionHarness'in kendi kabuğu kaldırıldı; `shell` /
> `shell_background` türleri ve `shell_manage` gitti. Ajanın kabuk komutları artık
> CLI'nin native kabuğunda, CLI taşıyıcısının (`provider`) **alt süreçleri** olarak
> koşar ve defterde ayrı satır açmaz.

## 1. Neden var

Bir tur `go test`'te asılı kaldığında, bir codex-cli oturumunu geride bıraktığında
ya da bir MCP sunucusu döngüde yeniden doğduğunda, kullanıcının makinesinde
penceresi olmayan, log satırı üretmeyen ve Görev Yöneticisi dışında
durdurulamayan gerçek bir süreç kalır. Loglar "ne yaptı"yı anlatır, defter
"**şu anda ne çalışıyor**"u anlatır.

## 2. Defterin modeli

`internal/procwatch` yalnız **kaydeder**, süreç başlatmaz. Çağıran kendi
`exec.Cmd`'sini kurup çalıştırmaya devam eder; sadece koşuyu `Begin`/`Finish`
arasına alır:

```go
h := procwatch.Default().Begin(runCtx, procwatch.Meta{
    Kind: procwatch.KindCode, Label: "run_code", Command: interp + " " + scriptPath, Dir: sb.Root, Stop: cancel,
})
runErr := runTrackedCmd(cmd, h) // Start + h.Started(cmd) + Wait
h.AppendOutput(out)
h.Finish(runErr)
```

- **`Registry`** — canlı süreçlerin haritası + sınırlı bir geçmiş halkası
  (`DefaultHistory = 300`). Çalışan kayıtlar **asla** düşürülmez; yalnız bitmişler
  budanır.
- **`Handle`** — tek sürecin canlı tarafı. `nil` alıcıyla da güvenlidir, böylece
  defteri olmayan bir bağlamda (test, önizleme) çağrı yeri değişmeden çalışır.
- **`Default()`** — süreç-genelinde tek defter. Bilinçli bir taviz: spawn noktaları
  birbirine kablolanamayan katmanlara dağılmış (stdio MCP istemcisi, sağlayıcı
  taşıyıcısı, kod yorumlayıcısı, hook çalıştırıcısı); loglama da aynı problemi aynı şekilde çözer.
- **`Meta.Command`** 2000 rune'da kırpılır — komut satırı ajan girdisidir ve defter
  bellekte tutulup SSE'ye yayılır.

### Durumlar

| Durum | Anlamı |
|-------|--------|
| `running` | Başladı, henüz reap edilmedi |
| `succeeded` | 0 ile çıktı |
| `failed` | Sıfırdan farklı çıkış (kod kayıtta) ya da hiç başlayamadı (`exitCode -1`) |
| `killed` | İstek üzerine durduruldu (panel, havuz teardown'ı) |
| `timed_out` | Koşu bağlamı süresini doldurdu |

Sınıflandırma sırası `Handle.Finish` içinde sabittir: durdurma isteği → süresi
dolmuş bağlam → `ExitError` → diğer hata → başarı. Bir sitenin kendi teardown yolu
varsa (kalıcı CLI'yi öldüren havuz) önce `MarkStopping()` çağırır; yoksa kullanıcının
bastığı "Durdur" bir başarısızlık gibi görünürdü.

## 3. Sahiplik (owner) nasıl bulunuyor

Kimlik bağlamla taşınır: `procwatch.WithOwner(ctx, Owner{...})`. Damga tek yerde
basılır — `internal/agent/toolloop_phases.go` içindeki tur hazırlığı — çünkü o
fazda kurulan ctx, turun başlattığı her şeye ulaşır: kod
yorumlayıcıları, CLI taşıyıcısının kendisi, hook'lar. Daha derindeki bir damga
yalnız kendi alanlarını ezer (`mergeOwner`), böylece bir worker turu workspace'i
kaybetmeden oturumu daraltabilir.

Oturum dışında doğan süreçler (workspace havuzunun MCP sunucusu, ayarlar
ekranından tetiklenen harici araç güncellemesi ya da `rtk gain` raporu)
sahipsizdir ve **bilerek** öyle listelenir: API bunları her workspace'e
gösterir, çünkü gösterilemeyen süreç tam da bu özelliğin önlemek istediği şeydir.

**İkinci damga yeri — Interaction MCP köprüsü.** Ajansal bir CLI (claude-cli,
codex-cli) araçlarını turun ctx'i üzerinden değil, **HTTP üzerinden** çağırır; o
istek turun damgasını taşımaz. Bu yüzden `internal/api/mcp_interaction.go`
içindeki `stampRunSession` — zaten her köprülenmiş araç çağrısının tek damga
noktası — owner'ı da basar. Koşu yalnız kendi oturumunu bilir, ajan alanları boş
kalır; onları **okuma tarafında** `handleListProcesses` çözer (oturum → ajan,
istek başına her ayrı oturum için bir kez). Oturumu silinmiş bir süreç ajansız
listelenir, listeden düşmez.

## 4. Enstrümante edilen yerler

| Kind | Yer | Durdurulabilir mi |
|------|-----|-------------------|
| `code` | `builtin_runcode.go`, `builtin_transform_data.go` | evet |
| `provider` | `providers/procwatch.go` `watchCLI` (claude-cli tek atış, codex-cli, opencode-cli), `claudecli_session.go` (kalıcı), `cli_preflight.go`, `codexcli_plugins.go` | evet — kullanıcı durdurmasıyla **aynı** yol (ctx gölgelenir) |
| `mcp` | `internal/mcp/client.go` (`DialStdio`) | evet — `Close` üzerinden |
| `hook` | `internal/agent/hooks.go` | evet |
| `external` | `grep_rg.go`, `builtin_codebase_search.go`, `capabilities.go`, `zvecgrep_index.go`, `exttools/update.go`, `api/external_tools_maint.go` | hayır (kısa yoklamalar) |

> **Kaldırılan türler (2026-11-17):** `shell` (köprülü ön plan Bash/PowerShell) ve
> `shell_background` (`run_in_background` + `shell_manage`) TionHarness'in kendi
> kabuğuyla birlikte silindi. CLI'nin native kabuğunda koşan komutlar, CLI
> taşıyıcısının alt süreçleridir: taşıyıcı satırı (`provider`) durdurulunca onlar da
> gider, ama komut başına satır yoktur. Ayrıca CLI kabuk komutları için TionHarness
> tarafında süreç-job kapsaması (Windows job object) da yoktur.

**Enstrümante edilmeyenler** (bilinçli): CLI'nin native kabuğunda koşan komutlar
(yukarı bak) ve onlara hook olarak takılan `rtk`/`sqz` — bunlar CLI'nin kendi hook
koşucusundan çıkar, TionHarness'in `execHook`'undan değil —,
`exttools.LocalVersion` sürüm yoklamaları (`--version`; her araç kartı açılışında
koşan, milisaniyelik ve yan etkisiz bir okuma) ve API'nin kendi git plumbing
okumaları (`git rev-parse`, commit aktivitesi). Defterde
görünmemek "süreç yok" demek **değildir**.

## 5. Okuma ve durdurma yüzeyleri

- `GET /api/workspace/processes?status=&kind=&session=&agent=&limit=` — `status`
  ve `kind` virgüllü ya da tekrarlı verilebilir; aktif workspace'e (artı sahipsiz
  kayıtlara) daraltılır.
- `POST /api/workspace/processes/{id}/stop` — `{id, stopped, reason?}`. Bitmiş bir
  kaydı durdurmak 200 + `stopped:false` + gerekçedir, hata değil; bilinmeyen id
  404'tür. Durdurma yalnız **sinyaldir**: kayıt, süreci başlatan yer onu reap
  ettiğinde terminal duruma geçer.
- `list_processes` ajan aracı (`CategoryDiagnostics`) — salt okunur. Kasıtlı:
  süreç öldürebilen bir ajan, içinde koştuğu turun CLI taşıyıcısını öldürebilirdi.
  Kendi arka plan komutlarını ajan CLI'nin native araçlarıyla yönetir (ör.
  claude-cli `Bash` `run_in_background` + `BashOutput`/`KillShell`).
- **UI:** Workspace → **İşlemler** (`frontend/src/features/workspace/ProcessPanel.tsx`).
  Duruma/türe göre filtre, komut+etiket+ajan+oturum üzerinde metin araması,
  çalışan satırlar için saniyede bir işleyen süre, satır detayında çıktı kuyruğu ve
  hata, `stoppable` satırlarda iki adımlı onaylı **Durdur**.
- **Sahip sütunundaki oturum bağlantısı.** Oturum kimliği (ve varsa `üst:` ile
  ebeveyn oturumu) sohbet ekranına götüren bir bağlantıdır. Panel hangi
  oturumların hâlâ var olduğunu `GET /api/sessions?ids=…` ile doğrular: silinmiş
  bir oturumun kimliği düz metin kalır — satır listeden düşmez, bağlantı ölü olmaz.

## 6. Canlılık (SSE)

Defter her durum değişiminde (`start`, durdurma isteği, `finish`) olay yayınlar:
`internal/app/app.go` içindeki `SetNotify`, `events.TypeProcess` ("process")
frame'ini ortak `/api/events` akışına basar. Frame **kaydı taşımaz**: tek içeriği
`target: {processId, status}` — yani "hangi kayıt kımıldadı" ipucu — ve panel bunu
bilerek okumaz, gördüğünde listeyi baştan çeker (300 ms debounce). Kodda geçen
"payload-free" nitelemesi (`internal/events/types.go`, `frontend/src/api/system.ts`)
bunu kasteder: birleştirilebilir bir **kayıt gövdesi** yoktur, `target` alanı
vardır. On kod/hook süreci başlatan bir tur panele on birleştirme
değil, bir okuma maliyeti çıkarır. Toast üretmez: bir yerel süreç kimsenin
bildirim olarak görmek isteyeceği haber değildir.

## 7. Sınırlar

- Defter **yalnız bellektedir**: TionHarness yeniden başlarsa geçmiş gider ve
  yeniden başlatmadan önce çalışan süreçler (kalıcı CLI, MCP sunucusu) yeni
  defterde görünmez.
- Geçmiş sınırlıdır (300 kayıt); uzun bir oturumun eski kayıtları düşer.
- Çıktı kuyruğu 4 KB ile sınırlıdır ve yalnız zaten çıktı tamponlayan siteler
  doldurur — tanılamadır, log değil.
