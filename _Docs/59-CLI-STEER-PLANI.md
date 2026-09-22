# TionHarness — claude-cli Canlı Steer (Yönlendirme) Planı

> **Özet (2026-09-06):** Bu doküman claude-cli sağlayıcısında turu durdurmadan yönlendirme (mid-turn steer) yapabilmenin tasarımını ve durumunu anlatır. Uygulandı ama sınırlıyla: steer yalnız "ask" izin modunda çalışır, "read-only" ve "auto" modlarda yapısal olarak desteklenmediği için backend `"unsupported"` döner ve mesaj tur bitince kuyruğa alınır. Ana mekanizma external-agent'tan esinlenerek permission-prompt (`callPermission`) yanıtına `additionalContext` enjekte etmektir; ilgili kod `chat_control.go`, `inbox.go`, `mcp_interaction_tools.go` ve `steer_cli_test.go` dosyalarındadır. TSK762 araştırması (2026-09-06) taşıyıcı × izin modu destek matrisini kod kanıtıyla doğruladı: aşağıdaki **"Doğrulanmış durum"** bölümü gerçek davranıştır; ilk plan/tasarım metni `arsiv/59-CLI-STEER-PLANI-TASARIM.md`'dedir. TSK899 (2026-09-06) native yoldaki iki sessiz kayıp yolunu kapattı: araçsız tur da `foldSteer()` ile steer'i drain ediyor, tur sonu fallback `run.steer` kanalını kurtarıyor ve dolu steer buffer'ı artık sessizce düşürülmek yerine `503` döndürüyor. TSK900 (2026-09-06) `read-only` yanlış raporlamasını kapattı: canlı steer artık yalnız claude-cli + **`ask`** modunda destekli; `read-only` ve `auto` dürüstçe `"unsupported"` döner. TSK901 (2026-09-06) kuyrukta bekleyen bir mesajı çalışan tura canlı yönlendirme olarak taşıyan `POST /api/sessions/{id}/queue/{msgId}/steer` ucunu ekledi: dönüşüm tek `withInbox` kilidi altında atomiktir, red hâlinde mesaj kuyrukta kalır. 2026-09-22'de destek durumu arayüze taşındı: backend `steerable` bayrağını mevcut `queue_update` hub olayıyla yayınlıyor (`inbox.go` `publishQueue`) ve tur sınırında `republishQueue` ile tazeliyor (`chat_turn_phases.go`); `PendingTray` desteklenmeyen turda butonu pasifleştirip sebebini gösteriyor. Aynı tarihte kapsam da gerçeklendi: `steerableForTurn` testi kendi beklentisini üretmek yerine sabit (sağlayıcı, izin modu) tablosuna bakıyor ve `foldSteer()` doğrudan test ediliyor (`internal/agent/steer_fold_test.go` — sıraya ekleme, tek kez drain, `pendingProgrammatic` ertelemesi, `steerRoleFor` kanal seçimi).

> **Tarihçe (kısa):** İlk uygulama (2026-07-11) steer'i claude-cli'de permission-prompt
> (`callPermission`) yanıtına `additionalContext` enjekte ederek taşıdı (`chatRun.pendingSteer`,
> `setSteer`/`takeSteer` — `chat_control.go`). 2026-07-13'te bu aracın `auto` modda hiç
> bağlanmadığı görüldü; karar (C) "dürüst UX" oldu: teslim edilemeyecek steer için
> backend `"unsupported"` döner, mesaj kuyruğa alınır (`steerableForTurn`,
> `TestSteerableForTurn`). Güncel davranış aşağıdaki matristir. `additionalContext`'in
> modele gerçekten bağlam olarak girdiği CLI sürümüne bağlıdır; teslim olmazsa mesaj
> kuyruğa düşer, kaybolmaz.

## Doğrulanmış durum — TSK762 araştırması (2026-09-06)

> Bu bölüm **doğrulanmış** gerçek davranıştır (kod okuması, `74399ffb`; yol/satır
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
