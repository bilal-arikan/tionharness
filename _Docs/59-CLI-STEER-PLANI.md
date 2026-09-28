# TionHarness — Native ve CLI Canlı Yönlendirme

> **Özet (2026-09-28):** Native sağlayıcılarda sonraki model isteğinde; Claude ve
> Codex CLI'de sonraki TionHarness Interaction MCP araç yanıtında canlı yönlendirme
> desteklenir. CLI desteği `auto`, `ask` ve `read-only` izin modlarından bağımsızdır;
> köprü adresi bulunmalıdır. Claude izin onaylarının `additionalContext` yolu da
> korunur. Mesajlar ortak FIFO ile sırayla, bir kez tüketilir. Teslim noktası olmadan
> tur biterse kalanlar sonraki turun başına alınır; açık durdurma yeniden tur başlatmaz.
> Bu, Codex App Server `turn/steer` protokolüne geçiş değildir.

## Güncel kullanım ve teslim (2026-09-28)

- Çalışan oturumda mesajı yazıp **Yönlendir** seçin. **Sıraya** ayrı tur başlatmak
  için mevcut davranışını korur; Enter kısayolunun davranışı değişmedi.
- Kuyruğa gönderilmiş bir mesaj **Şimdi yönlendir** ile mevcut tura aktarılabilir.
  Dönüşüm atomiktir; reddedilirse mesaj kuyrukta kalır.
- Soru kartına verilen asenkron cevaplar da aynı MCP sınırından taşınabilir.
  Genel yönlendirme ve soru cevabı birbirini silmez; soru kimliği korunur.
- CLI'lerde araç çıktısı ilk içerik bloğunda kalır; yönlendirme ayrı metin
  bloklarına eklenir. İzin callback'lerinde yalnız geçerli JSON `additionalContext`
  kullanılır; genel metin bloğu eklenmez.
- Arka arkaya mesajlar birbirinin üzerine yazılmaz. Dolu tampon `503`, finalizasyon
  veya durdurma sonrası gönderim `409` döner; arayüz taslağı korur.
- Native/CLI tur sonu toparlaması kabul kapısını aynı kilitle kapatır. Böylece tam
  tur biterken gelen mesaj ya toparlamaya dahil olur ya açıkça reddedilir.
- Otonom CLI turlarında da köprü üzerinden yönlendirme ve teslim edilemeyen mesajı
  sonraki tura aktarma uygulanır. Bu özellik başsız ajanlara soru sorma izni vermez.

**Sınır:** TionHarness dışındaki araçlara veya uzun süren tek bir CLI/model çağrısına
anında müdahale edilmez. Sonraki TionHarness MCP yanıtı gelmezse yönlendirme ancak
sonraki turda işlenir. `Kes` mevcut turu durdurup yeni mesajı başlatan ayrı eylemdir.
Steering tamponu bellektedir; sunucu çökmesine dayanıklı teslim garantisi vermez.

Kod: `steer_run.go`, `steer_delivery.go`, `steer_recovery.go`, `mcp_interaction.go`,
`mcp_interaction_tools.go`, `inbox_steer.go`. Testler iki CLI'de sırayla/tek tüketim,
soru cevabıyla ortak teslim, kuyruktan dönüşüm, tampon doluluğu, tur sonu yarışı,
durdurma ve arayüzün kuyruk/kesme yerine yönlendirme seçmesini kapsar.
Bu testler TionHarness köprüsünü doğrular; canlı CLI/model uçtan uca deneyi değildir.

Doğrulama: tam test kapısında tüm Go paketleri ve 147 dosyada 1044 frontend testi
geçti. TypeScript, değişen yönlendirme arayüzü dosyalarının ESLint kontrolü ve
depcheck başarılıdır.

Aşağıdaki kayıtlar önceki uygulamanın tarihçesidir. İzin moduna bağlı eski destek
matrisi, 28 Eylül'de eklenen genel MCP teslim yolu için geçerli değildir.

> **Tarihçe (kısa):** İlk uygulama (2026-07-11) steer'i claude-cli'de permission-prompt
> (`callPermission`) yanıtına `additionalContext` enjekte ederek taşıdı (`chatRun.pendingSteer`,
> `setSteer`/`takeSteer` — `chat_control.go`). 2026-07-13'te bu aracın `auto` modda hiç
> bağlanmadığı görüldü; karar (C) "dürüst UX" oldu: teslim edilemeyecek steer için
> backend `"unsupported"` döner, mesaj kuyruğa alınır (`steerableForTurn`,
> `TestSteerableForTurn`). Bu kayıt eski permission-prompt uygulamasını anlatır. `additionalContext`'in
> modele gerçekten bağlam olarak girdiği CLI sürümüne bağlıdır; teslim olmazsa mesaj
> kuyruğa düşer, kaybolmaz.

## Tarihsel durum — TSK762 araştırması (2026-09-06)

> Bu bölüm 6 Eylül tarihinde doğrulanmış eski davranıştır (kod okuması, `74399ffb`; yol/satır
> referansları o commit'e göredir). İlk plan/tasarım metni
> `arsiv/59-CLI-STEER-PLANI-TASARIM.md`'ye taşındı.

| Taşıyıcı × izin modu | Mid-turn steer | Kanıt |
|---|---|---|
| native (doğrudan API), **araçlı** tur | **DESTEKLİ** — çalışıyor | `drainSteer`, `internal/agent/toolloop_phases.go:609` |
| native, **araçsız** tur | **DESTEKLİ** (TSK899 ile düzeltildi) | `runPlain` da `foldSteer()` çağırıyor: `internal/agent/steer.go:44`, `internal/agent/toolloop_phases.go:294`; sağlayıcı çağrısından sonra gelen mesajı tur sonunda `recoverUndeliveredSteer` kanaldan kurtarıp kuyruğun **başına** koyuyor (`internal/api/steer_recovery.go`) |
| claude-cli + `ask` | **SINIRLI DESTEK** — kod yolu var, etkisi uçtan uca doğrulanmadı | permission-prompt aracı yalnız gated (write/exec) araç sınırında fire eder: `internal/climcp/climcp.go:97-102`, `internal/api/mcp_interaction_tools.go` `callPermission` |
| claude-cli + `read-only` | **DESTEKLENMİYOR** — `steerableForTurn` artık `false` (TSK900) | Permission-prompt aracı bağlı (`climcp.go:97-102`) ama `read-only` ayrıca `--permission-mode plan` koşuyor (`claudecli.go:203-216`): CLI mutasyonları kendi bloklar, köprülenmiş/harici MCP araçları `--allowedTools`'ta (`climcp.go:178,215-229`), dolayısıyla prompt'a ulaşan tek çağrı `ExitPlanMode`. `callPermission` onu `steerContext()`'e uğramadan `callExitPlan`'a sapıtıyor (`mcp_interaction_tools.go:99-101`), steer ise yalnız `callPermission`'ın allow yollarında enjekte ediliyor (`:118,131,133`). Boundary yok ⇒ `inbox.go` `"unsupported"` döndürür, mesaj kuyruğa alınır. |
| claude-cli + `auto` (**VARSAYILAN**) | **YAPISAL OLARAK DESTEKLENMİYOR** — enjeksiyon noktası yok; backend `"unsupported"` dönüp mesajı kuyruğa alıyor | auto modda permission-prompt aracı hiç bağlanmıyor: `internal/climcp/climcp.go:97-102`; varsayılan mod `internal/db/store.go:84`, `internal/settings/settings.go:400` |
| codex-cli (her mod) | **YAPISAL OLARAK DESTEKLENMİYOR** | `exec --json` + `cmd.Stdin = strings.NewReader(prompt)` prompt bitince EOF verir, ikinci mesaj için kanal yok — `internal/providers/codexcli.go:153`, `:512`; gerçek steer için app-server (JSON-RPC) moduna geçiş gerekir |

### Claude CLI kalıcı stdin — mevcut, ama kazanç yok

`--input-format stream-json` entegrasyonda **mevcut ve varsayılan açık**
(`internal/providers/claudecli_session.go:340`,
`internal/settings/settings.go:489`). Tur sürerken stdin'e ikinci bir
`{"type":"user"}` satırı yazmak protokolce kabul ediliyor, ama CLI onu **tur
sonuna erteliyor** ([claude-code#41665](https://github.com/anthropics/claude-code/issues/41665))
— yani bugünkü kuyruk davranışına göre davranışsal bir kazanç yok.

**DOĞRULANMADI:** CLI'nin `system/init` olayındaki `capabilities` alanı bir
`control_request` / `interrupt` kanalını **ima ediyor**, ancak wire formatı ne
resmî dokümanda ne de üçüncü parti protokol dokümanında verilmiş; koştuğumuz CLI
sürümünde deneysel olarak teyit edilmedi. Bu satır kesinmiş gibi bir plan
dayanağı olarak kullanılmamalı.

### Bilinen sınırlar / açık kartlar

- **TSK899 — KAPATILDI (2026-09-06).** İki sessiz kayıp yolu da kapandı;
  ayrıntı aşağıdaki "TSK899 düzeltmesi" notunda.
- **TSK900 — KAPATILDI (2026-09-06).** `read-only` modda `steerableForTurn` artık
  `false` dönüyor; teslim yolu olmayan mod için backend dürüstçe `"unsupported"`
  veriyor. Ayrıntı aşağıdaki "TSK900 düzeltmesi" notunda.
- **TSK901 — KAPATILDI (2026-09-06).** Kuyruktaki mesajı steer'e çevirme ucu
  (`POST /api/sessions/{id}/queue/{msgId}/steer`) ve `PendingTray` düğmesi
  uygulandı; ayrıntı aşağıdaki "TSK901 — kuyruktan steer'e çevirme" notunda.
- **TSK906 — KAPATILDI (2026-09-22).** Backend `steerable` bayrağını `queue_update`
  olayıyla yayınlıyor (`inbox.go` `publishQueue`); `PendingTray`/`Composer`
  desteklenmeyen turda düğmeyi önceden pasifleştiriyor.

### TSK899 düzeltmesi — steer artık sessizce kaybolmuyor (2026-09-06)

Yukarıdaki matriste "native araçsız → HAYIR (bug)" olarak kayıtlı davranış
düzeltildi. Üç değişiklik:

1. **Araçsız tur da drain ediyor.** Drain + enjeksiyon mantığı tek bir
   `foldSteer()` metoduna taşındı (`internal/agent/steer.go`); native araç
   döngüsü her sağlayıcı çağrısından önce (`toolloop_phases.go:614`), araçsız
   yol da tek çağrısından önce (`toolloop_phases.go:294`) onu çağırıyor. Steer
   rolü (`steerRoleFor`) artık `prepare()` içinde çözülüyor, çünkü araçsız yol
   native döngünün kurulumuna hiç uğramıyor.
2. **Tur sonu fallback kanalı da kurtarıyor.** `recoverUndeliveredSteer`
   (`internal/api/steer_recovery.go`) yalnız claude-cli `pendingSteer`
   alanını değil, `run.steer` **kanalını** da boşaltıyor; kurtarılan mesajlar
   kuyruğun **başına** (sonuna değil) konuyor — kullanıcı onları bu turu
   yönlendirmek için yazmıştı, sonradan kuyruğa aldıklarından önce gelmeliler.
3. **Buffer dolu = `503`, sessiz düşürme değil.** `handleSessionControl`
   (`internal/api/inbox.go`) ve chat-control steer ucu (`chat_control.go`)
   artık `run.trySteer` başarısız olduğunda `503` + `steerBufferFullMsg`
   dönüyor. Gerekçe: dolu buffer "tur rehberliği tüketmiyor" demektir; mesajı
   sessizce kuyruğa almak steer'i yeni mesajla karıştırırdı.

Testler: `internal/agent/steer_plain_test.go`
(`TestPlainTurnFoldsPendingSteer`, `TestPlainTurnLeavesLateSteerOnChannel`),
`internal/api/steer_recovery_test.go`
(`TestRecoverUndeliveredSteerRequeuesChannelMessages`,
`TestSessionSteerReportsFullQueue`, `TestSessionSteerAcceptedWhenQueueHasRoom`,
`TestChatControlSteerReportsFullQueue`).

TSK900 ve TSK901 bu düzeltmenin kapsamı dışındaydı; ikisi de aynı gün ayrıca
kapatıldı (aşağıya bakın).

### TSK900 düzeltmesi — `read-only` artık dürüst rapor veriyor (2026-09-06)

Matriste "`steerableForTurn` `true` dönüyor, pratikte DESTEKLENMİYOR" olarak
kayıtlı tutarsızlık kapatıldı. `steerableForTurn` (`internal/api/chat_control.go`)
claude-cli için artık yalnız `mode == "ask"` durumunda `true` dönüyor; `read-only`
ve `auto` için `handleSessionControl` (`internal/api/inbox.go`) `"unsupported"`
verip mesajı kuyruğa alıyor — kullanıcı "steered" yanıtı alıp hiçbir şey olmadığını
görmüyor.

Kartın öncülü kısmen yanlıştı: permission-prompt aracı `read-only` modda da
**bağlı** (`internal/climcp/climcp.go:97-102`). Gerçek sebep `read-only`'nin
ayrıca `--permission-mode plan` koşması: mutasyonları CLI kendi blokluyor,
köprülenmiş MCP araçları `--allowedTools` üzerinden geçiyor, prompt'a ulaşan tek
çağrı `ExitPlanMode` kalıyor ve `callPermission` onu `steerContext()`'e uğramadan
`callExitPlan`'a sapıtıyor (`internal/api/mcp_interaction_tools.go:99-101`).

**Bilinen sınır:** plan onaylanıp CLI plan modundan çıkarsa steer teknik olarak
teslim edilebilir hale gelir, ama `steerable` tur kurulumunda bir kez hesaplanıyor
(`chat_turn_phases.go:196`) ve o an bu bilinemez — yaygın durumda dürüst cevap
`false`.

Testler: `internal/api/steer_cli_test.go` (`TestSteerableForTurn` read-only vakası
`false`'a çevrildi; yeni `TestSteerableForTurnReadOnlyCLI` hem `read-only=false`
hem `ask=true` yönünü pinliyor).

### TSK901 — kuyruktan steer'e çevirme (2026-09-06)

Kuyrukta bekleyen bir mesaj eskiden yalnız tur bitince teslim edilebiliyordu.
Yeni uç `POST /api/sessions/{id}/queue/{msgId}/steer`
(`internal/api/inbox_steer.go` `handleSteerQueued`) o mesajı, ayrı bir steer
metni yazdırmadan, **çalışan tura** canlı yönlendirme olarak taşır.

**Neden ayrı uç.** İstemci bunu "steer + kuyruktan sil" ikilisiyle yapamaz: iki
çağrı arasında seri worker mesajı dispatch edebilir → metin hem yönlendirme hem
kendi turu olarak koşar; ters sırada ise steer reddedilince mesaj kuyruktan
silinmiş olur. İkisi de istemci tarafında telafi edilemez.

**Atomiklik.** Arama ve silme tek bir `withInbox` geri çağrısı içinde yapılır
(`steerQueuedMessage`). Seri worker kuyruk başını aynı kilit altında poplar
(`popInboxHead`), dolayısıyla biz kilidi tutarken mesajı dispatch edemez. Silme
yalnız run metni **kabul ettikten sonra** yapılır; kabul edilmezse kuyruk hiç
değişmez. Geri çağrı bilinçli olarak inbox dışına da uzanır (metni run'a verir),
bu güvenli çünkü `deliverSteer`'in iki yolu da bloklamaz ve yalnız run'ın kendi
mutex'ini alır (kilit sırası inbox.mu → run.mu). Aynı anda biten bir tur da
güvenli: tüketilmemiş yönlendirmeyi tur sonunda `recoverUndeliveredSteer`
kuyruğun **başına** geri koyar.

Ortak teslim yardımcıları `internal/api/steer_delivery.go`'ya alındı
(`steerTargetRun` — yalnız uçuştaki turun sahibi run hedeflenir, superseded run
fenced'dır; `deliverSteer` — native kanal vs. CLI stash ayrımı).
`handleSessionControl` (`inbox.go`) aynı yardımcılara indirgendi, davranışı
birebir korundu.

| Durum | Yanıt | Kuyruk |
|---|---|---|
| Uçuşta tur yok / run bitmiş | `404` | değişmez |
| `msgId` WAITING kuyrukta yok | `404` | değişmez |
| Mesajda ek dosya var (steer metin-only) | `400` | değişmez |
| Mesajın steer edilecek metni yok | `400` | değişmez |
| Tur steerable değil (claude-cli `auto`/`read-only`, codex-cli) | `200 {"result":"unsupported"}` | değişmez |
| Steer buffer dolu | `503 steerBufferFullMsg` | değişmez |
| Başarılı | `200 {"result":"steered"}` | mesaj düşer |

**Frontend.** `PendingTray` kuyruk çipine "şimdi yönlendir" eylemi eklendi
(`onSteerNow` + `canSteer`); `ChatView` onu `chat.steerQueued` /
`chat.activeStreaming` ile bağlar. İstemci **optimistik silme yapmaz** — çip
ancak sunucunun kuyruk güncellemesi mesajın gerçekten düştüğünü söyleyince
kaybolur, yani bir red mesajı UI'dan yok edemez. `"unsupported"` yanıtında
kullanıcıya "mesaj sırada kaldı, canlı yönlendirme için ajanı `ask` moduna al"
ipucu gösterilir.

Testler: `internal/api/inbox_steer_test.go` (6 vaka: dönüşüm, unsupported,
buffer dolu, ek dosya, bilinmeyen id, tur yok), `PendingTray.test.tsx` (3 vaka).

## İlk plan ve tasarım (arşivde)

Arka plan, external-agent referansı, hedef mimari, enjeksiyon kanalı incelikleri,
dosya bazında değişiklikler, fazlar ve riskler → `arsiv/59-CLI-STEER-PLANI-TASARIM.md`.
