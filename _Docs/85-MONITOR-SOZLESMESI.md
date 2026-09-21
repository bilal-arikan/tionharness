# 85 — Monitor sözleşmesi

> **Özet (2026-09-22):** `monitor` aracı bir arka plan kabuğunun (background
> shell) çıktısını bir regex'e karşı izler ve eşleşme olduğunda ajanı **uyandırır**
> — böylece uzun süren bir işin beklenmesi hiç tur harcamaz. v1 kapsamı yalnız
> kabuk çıktısı kaynağıdır; dosya/URL/WebSocket kaynakları ertelendi (TSK941),
> ama `MonitorSource` arayüzü onları yöneticiye dokunmadan almak üzere şekillendi.
> Monitörler **yalnız bellektedir**: süreç yeniden başlarsa kaybolur ve uyandırma
> **en fazla bir kez** (at-most-once) çalışır. Durum: **uygulandı**. Bir ajan
> için: bu dosya sözleşmeyi (kaynak arayüzü, uyandırma zinciri, kapasiteler,
> teardown) tanımlar — `monitor`'a kaynak eklerken veya uyandırma yolunu
> değiştirirken önce burayı oku.

## 1. Neden var

Bir ajan `run_in_background=true` ile bir dev server ya da uzun test koşusu
başlattığında, tek seçeneği `shell_manage` (action=output) ile yoklamaktı: bir tur
harca, çıktıyı oku, bir şey bulamadıysan `schedule_wake` ile kendine randevu ver,
baştan. Bekleyiş turla ödenir ve her yoklama bağlama çıktı doldurur.

`monitor` bu ilişkiyi tersine çevirir: ajan ne beklediğini **bir kez** söyler
(`shell_id` + regex filtre), turunu bitirir, ve eşleşme olduğunda eşleşen
satırları taşıyan **yeni bir tur** ile uyandırılır. Bekleyiş sıfır tura iner.

## 2. Kaynak arayüzü (`MonitorSource`)

Kaynaklar `internal/tools/monitor.go` içindeki şu arayüzü uygular:

```go
type MonitorEvent struct{ At time.Time; Payload string }

type MonitorSource interface {
    Poll(ctx context.Context) (events []MonitorEvent, done bool, reason string, err error)
    Describe() string
    Close()
}
```

- `done=true` **terminal** durumdur: kaynak bir daha asla üretemez (süreç çıktı,
  dosya silindi). `reason` nedenini anlatır. Monitör bu durumu **tam olarak bir
  kez** raporlar ve kapanır; dönüp durmaz (no spinning).
- `err` ise **geçici**dir: son hata olarak kaydedilir, `list` çıktısında görünür,
  yoklama sürer. Asla kalıcı sayılmaz.
- Terminal durum, son çıktı teslim edildikten **sonra** raporlanır — son satıra
  gelen bir eşleşme kaybolmaz.

**Filtreleme kaynağın işi değildir.** Kaynak gördüğü her şeyi bildirir; hangi
olayın eşleştiğine monitör katmanı karar verir. Yeni bir kaynak (dosya kuyruğu,
URL yoklaması) eklemek yalnız bu arayüzü uygulamak demektir; `MonitorManager`
değişmez.

### v1 kaynağı: kabuk çıktısı (`monitor_source_shell.go`)

`shellSource`, `bgWriter`'ın yeni `drainFrom(cursor)` metoduyla **kendi mutlak
bayt imlecini** tutar. Bu kritik: `shell_manage` (action=output) kendi
`delivered` imlecini kullanır. İki okuyucu tamamen bağımsızdır — monitörün
yoklaması, ajanın henüz okumadığı çıktıyı ne tüketir ne gizler; tersi de geçerli.

Diğer kurallar:

- İmleç, monitör kurulduğu **andaki çıktı sonundan** başlar. Geçmiş (birikmiş)
  çıktı tekrar oynatılmaz — ajan onu zaten görmüştü.
- Olaylar **satır** granülaritesindedir; regex filtre ancak böyle anlamlıdır
  ("ERROR" bir satırla eşleşmeli, tampon sınırıyla değil).
- Bilinmeyen `shell_id` **gerçek bir hatadır**. Sessizce hiçbir şeyi izlemeyen
  bir monitör, ajanı asla gelmeyecek bir uyandırma için beklemeye yollardı.
- Monitörü durdurmak izlediği süreci **durdurmaz**: kabuk `ShellManager`'ındır.

## 3. Uyandırma zinciri — tek yol

Eşleşme `Runtime.WakeNow` ile teslim edilir. `WakeNow`, `ScheduleWake`'ten
çıkarılan ortak `armWake` üzerine kuruludur, yani ikisi de **aynı** zincirden
geçer:

```
armWake -> tek seferlik (one-shot) schedule satırı + scheduler yeniden kurulumu
        -> fireWake -> ConsumeOneShotSchedule -> deliverWake
```

**Paralel bir teslim yolu bilerek yoktur.** Sonuçları:

- Uyandırma **en fazla bir kez** çalışır — satır teslimden *önce* atomik olarak
  tüketilir. Süreç teslim ortasında ölürse o olay **kaybolur**. Monitör bir
  kolaylıktır, dayanıklı bir kuyruk değildir; araç açıklaması bunu söyler.
- `ScheduleWake`'in `MinWakeDelaySec` (5 sn) tabanı olay uyandırmasına
  **uygulanmaz**: bir olaya tepkinin gecikmesi anlamsız olurdu. Gerçek gecikmeye
  scheduler'ın kendi kıskacı karar verir (`armWakeLocked` geçmiş bir `FireAt`'i
  1 sn'ye çeker).

## 4. Kapasiteler ve frenler

| Sınır | Değer | Niçin |
|-------|-------|-------|
| Oturum başına kurulu monitör | 8 (`monitorMaxLive`) | `bgShellMaxLive` (16)'dan bilerek düşük: her monitör ajanı bölebilir, "kaç şey beni bölebilir" freni "kaç süreç koşabilir"den sıkıdır |
| Olay başına yük | 2 KB | Uzun bir satır uyandırma istemini şişirmesin |
| Uyandırma başına olay | 10 | Fazlası düşürülür ve **düşen sayısı raporlanır** (sessizce yutulmaz) |
| Soğuma (cooldown) | en az 5 sn | Gürültülü bir kaynak her tıkta uyandıramaz; penceredeki eşleşmeler **tek** uyandırmada birleşir |
| `max_fires` | isteğe bağlı | Dolunca monitör nedeniyle birlikte kapanır |

Yoklama tek bir goroutine ve 1 sn'lik tek bir `time.Ticker` ile yürür — monitör
başına goroutine yoktur.

## 5. Yaşam döngüsü ve teardown (sızıntı düzeltmesi)

Durumlar: `armed` → (`stopped` | `done`). `stop` **idempotent**tir; zaten bitmiş
bir monitörü durdurmak hata değil, "zaten şu durumdaydı" notudur. Bilinmeyen bir
monitör id'si ise `ShellManager.Kill` gibi **eyleme geçirilebilir bir hatadır**.

`Runtime.ReleaseSessionRuntimeState(sessionID)` oturum silinirken çağrılır
(`internal/api/session_teardown.go`, Faz 7, `CloseSessionMCP` ile yan yana) ve
oturum başına bellekte tutulan durumu bırakır:

- `monitorMgrs` — yönetici **kapatılır**: yoklama goroutine'i durur, her monitör
  sonlanır, kaynaklar `Close()` edilir.
- `shellMgrs`, `readTrackers` — girdiler silinir.

Bu aynı zamanda önceden var olan bir **sızıntının düzeltmesidir**: bu üç
`sync.Map`'ten hiçbir girdi kaldırılmıyordu, yani kabuk çalıştırmış ya da dosya
okumuş her oturum yöneticisini süreç ömrü boyunca sızdırıyordu. Monitörlerle
birlikte bu artık yalnız bellek değil, davranış sorunu da olurdu: yaşayan bir
monitör artık var olmayan bir oturumu uyandırmayı sürdürebilirdi.

## 6. Kayıt noktaları (bir aracı eklerken dokunulan her yer)

| Yer | Ne |
|-----|-----|
| `internal/tools/builtin_monitor.go` | Aracın kendisi (`action=start\|list\|stop`) |
| `internal/agent/toolsetup.go` | `monitorMgrFor` + `shellMgr != nil` bloğunda kayıt; `SessionMonitorManagers` (köprü erişimcisi) |
| `internal/tools/categories.go` | `CategoryFiles` |
| `internal/tools/tierdefaults.go` | `VisibilityNameOnly` — yalnız `run_in_background` sonrası gerekir |
| `internal/tools/builtin_runcode.go` | Kod yürütme kipinden **hariç**: betik koşusunun uyanacağı bir tur yoktur |
| `internal/api/mcp_interaction*.go` | claude-cli köprüsü: ilan, dispatch, `callMonitor` |
| `internal/api/chat_control.go`, `chat_turn_phases.go` | `setMonitor` ile tur başına yöneticilerin takılması |
| `frontend/src/shared/lib/toolIcons.ts` | `Radar` ikonu |

Köprü, kabuk kapısının (`ShellEnabled`) arkasındadır: monitör yalnız arka plan
kabuğu izler, kabuk kapalıyken izleyecek bir şey yoktur. Yönetici yoksa köprü
**açık bir hata** döndürür ("monitoring is not available in this context") —
sessiz bir başarı, ajanı asla gelmeyecek bir uyandırma için turunu bitirmeye
yollardı.

## 7. Ertelenen: diğer kaynaklar (TSK941)

Dosya, URL ve WebSocket kaynakları v1'de yoktur. Arayüz onları taşıyacak
şekilde tasarlandı; eklerken `MonitorSource`'u uygulamak ve `monitor` aracına
kaynak seçici bir alan eklemek yeter — `MonitorManager`'ın yaşam döngüsü,
kapasiteleri ve uyandırma zinciri değişmemeli.
