# 85 — Monitor sözleşmesi

> **Özet (2026-09-22):** `monitor` aracı bir kaynağı bir regex'e karşı izler ve
> eşleşme olduğunda ajanı **uyandırır** — böylece uzun süren bir işin beklenmesi
> hiç tur harcamaz. Dört kaynak vardır: arka plan kabuğu (`shell_id`), dosya
> kuyruğu (`path`), periyodik URL yoklaması ve WebSocket aboneliği (ikisi de
> `url`, şemadan yönlendirilir). Kaynak seçimi tek bir alanla değil, üç alandan
> **tam birini** vermekle yapılır. Dışarı çıkan kaynaklar WebFetch ile **aynı**
> SSRF korumalı taşıyıcıyı (`egress_guard.go`) kullanır; dosya kaynağı sandbox
> sınırına uyar.
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
olayın eşleştiğine monitör katmanı karar verir. Yeni bir kaynak eklemek yalnız bu
arayüzü uygulamak demektir; `MonitorManager` değişmez — aşağıdaki dört kaynak da
yöneticiye tek satır dokunmadan eklendi.

### Kaynak seçimi (`monitor_source_select.go`)

Araç `shell_id`, `path` ve `url` alanlarından **tam birini** ister. Hiçbiri
verilmezse izlenecek bir şey yoktur; birden fazlası verilirse hangisinin
izleneceği belirsizdir ve yanlış şeyi sessizce izlemektense ikisi de hata olur.
`url`, şemasına göre yönlendirilir: `http(s)` yoklama kaynağına, `ws(s)` soket
kaynağına. Böylece ajan bir arka uç seçmek zorunda kalmaz.

### Kaynak 1: kabuk çıktısı (`monitor_source_shell.go`)

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

### Kaynak 2: dosya kuyruğu (`monitor_source_file.go`)

`fileSource`, bir dosyaya eklenen satırları izler. Yol, aracın **sandbox**'ından
(`Sandbox.Resolve`) geçirilir; confined bir sandbox'ta kök dışına çıkan bir yol
reddedilir — monitör, fs araçlarının sınırını dolaşmanın yolu olamaz.

`fsnotify` yerine `os.Stat` yoklaması seçildi, üç gerekçeyle (ağırlık sırasıyla):

1. Yönetici zaten her kaynağı süren 1 sn'lik **tek** bir tik sahibi. İzleyici,
   ajanın algılayabileceği bir gecikme kazandırmadan ikinci bir olay yolu ekler —
   uyandırma zaten `monitorMinCooldown` (5 sn) ile kapılı.
2. Eklenen baytlar, değişikliğin nasıl fark edildiğinden bağımsız olarak yine bir
   boyut imleciyle okunup satırlara bölünmek zorunda; izleyici bu kodun hiçbirini
   ortadan kaldırmaz.
3. Windows'ta `ReadDirectoryChangesW` olayları birleştirir ve başka bir sürecin
   yazdığı dosyalarda zaman zaman olay düşürür; boyut yoklamasının böyle bir kör
   noktası yok. Bedeli monitör başına saniyede bir `Stat`.

Diğer kurallar:

- İmleç, monitör kurulduğu andaki **dosya boyutundan** başlar; birikmiş içerik
  tekrar oynatılmaz.
- Boyut geriye giderse (döndürülmüş/`truncate` edilmiş log) imleç sıfırlanır ve
  yeni baştan okunur — aksi halde imleç sonsuza dek EOF'un ötesinde kalırdı.
- Satır sonu olmayan kuyruk parçası **bir sonraki yoklamaya** saklanır, böylece
  iki yoklamaya bölünen bir satır tek parça olarak bildirilir.
- Var olmayan yol ya da dizin **kurulum anında** hatadır.
- Dosyanın silinmesi terminal durumdur ve **bir kez** bildirilir.

### Kaynak 3: URL yoklaması (`monitor_source_url.go`)

`urlSource` bir `http(s)` adresini periyodik çeker ve gövde **değiştiğinde** olay
üretir (olayın yükü yeni gövdedir, regex ona uygulanır). İlk çekim sessiz bir
**taban çizgisi**dir: monitör bundan sonra olanı bildirir.

- Yoklama aralığı en az **30 sn**'dir (`urlSourceMinInterval`) ve yöneticinin 1
  sn'lik tikinden bağımsızdır: cooldown uyandırmayı sınırlar, bu ise **isteği**.
- Gövde 256KB ile sınırlıdır; ajana giden yük `monitorPayloadBytes` ile çok daha
  aşağıda kapanır.
- Durum kodu, hash'lenen yükün parçasıdır — 200'den 500'e düşmek de bir
  değişikliktir, hata değil.
- Üst üste **5** başarısız çekim terminal durumdur; tek bir kesinti monitörü
  öldürmez, gerçekten yok olan bir uç nokta da oturum boyunca yoklanmaz.

### Kaynak 4: WebSocket aboneliği (`monitor_source_ws.go`)

`wsSource` bir `ws(s)` adresine bağlanır ve her gelen mesaj bir olaydır. Diğer
iki kaynaktan farkı **push** olmasıdır: mesajlar yönetici tik attığında değil,
sunucu gönderdiğinde gelir. Bu yüzden kaynak, bağlantıyı sınırlı bir kuyruğa
boşaltan bir okuyucu goroutine'i sahiplenir; `Poll` yalnız biriken sonuçları
devreder. Uyandırma zamanını yine yöneticinin tiki belirler.

- Kuyruk **256** mesajla sınırlıdır; taşma, kaybın görünür olması için "düşürüldü"
  notu olarak bildirilir.
- İkili (binary) çerçeveler bir regex'in eşleşebileceği metin taşımaz; baytlar
  metinmiş gibi gösterilmez, yerine boyut notu geçer.
- Soketin kapanması terminal durumdur ve **bir kez** bildirilir; son mesajlar
  kapanışla **birlikte** teslim edilir, böylece son mesaja gelen eşleşme kaybolmaz.
- **Bağımlılık seçimi:** `github.com/coder/websocket` (eski adıyla
  `nhooyr.io/websocket`). Geçişli bağımlılığı **sıfır** olan tek yaygın Go
  istemcisi — üç doğrudan bağımlılığı olan bir ağaca tam bir modül ekler —,
  context tabanlı API'si `Poll` imzasına oturur ve **çağıranın verdiği
  `*http.Client` ile** el sıkışır. Bu sonuncusu belirleyici oldu: WebFetch'in
  SSRF korumalı taşıyıcısı buraya olduğu gibi uygulanabiliyor. `gorilla/websocket`
  kendi dialer'ını kullanır ve koruma yeniden yazılmak zorunda kalırdı.

### Dışarı çıkış sınırı (`egress_guard.go`)

URL ve WebSocket kaynakları, WebFetch'in kullandığı dialer'ın **aynısını**
kullanır: her bağlantının **çözülmüş IP**'si denetlenir, yani loopback, özel,
link-local ve bulut meta-veri adresleri DNS rebinding ya da bir yönlendirme
sıçraması üzerinden de engellenir. Koruma tek bir yerde durur (önceden
`builtin_http.go` içinde gömülüydü, oraya da bu iş sırasında çıkarıldı): ikinci ve
ince farklı bir dialer, bir SSRF açığının sonradan içeri girme biçimidir. Monitör
gözetimsiz ve uzun ömürlü bir çekim döngüsü olduğu için bu sınır burada tek
seferlik bir çekimdekinden daha önemlidir, daha az değil.

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
| `internal/tools/monitor_source_select.go` | Kaynak seçimi: `shell_id`/`path`/`url`'den tam biri, `url` şemadan yönlendirilir |
| `internal/tools/monitor_source_{shell,file,url,ws}.go` | Dört `MonitorSource` uygulaması, kaynak başına bir dosya |
| `internal/tools/egress_guard.go` | WebFetch ile paylaşılan SSRF korumalı dialer/taşıyıcı |
| `internal/agent/toolsetup.go` | `monitorMgrFor` + `shellMgr != nil` bloğunda kayıt; `SessionMonitorManagers` (köprü erişimcisi) |
| `internal/tools/categories.go` | `CategoryFiles` |
| `internal/tools/tierdefaults.go` | `VisibilityNameOnly` — yalnız `run_in_background` sonrası gerekir |
| `internal/tools/builtin_runcode.go` | Kod yürütme kipinden **hariç**: betik koşusunun uyanacağı bir tur yoktur |
| `internal/api/mcp_interaction*.go` | claude-cli köprüsü: ilan, dispatch, `callMonitor` |
| `internal/api/chat_control.go`, `chat_turn_phases.go` | `setMonitor` ile tur başına yöneticilerin takılması |
| `frontend/src/shared/lib/toolIcons.ts` | `Radar` ikonu |

Köprü, kabuk kapısının (`ShellEnabled`) arkasındadır. Bu kapı, dosya/URL/soket
kaynakları eklendikten sonra da olduğu gibi bırakıldı: `SessionMonitorManagers`,
kabuk kapalıyken monitör yöneticisini hiç kurmaz, dolayısıyla araç tümüyle
kapanır. Kabuksuz bir turda yalnız URL izlemeye izin vermek ayrı bir kapı
tasarımı ister (bkz. aşağıdaki açık uç). Yönetici yoksa köprü **açık bir hata**
döndürür ("monitoring is not available in this context") — sessiz bir başarı,
ajanı asla gelmeyecek bir uyandırma için turunu bitirmeye yollardı.

`SessionMonitorManagers` artık üçüncü bir değer olarak **sandbox** döndürür:
izlenen dosya yolu buna göre çözülür. CLI köprüsü bunu `chatRun.monitorSb`
içinde taşır, native yol ise kayıt kurulumundaki `sb`'yi doğrudan geçirir — iki
yol da aynı sınırı kullanır.

## 7. Açık uçlar

- **Kabuk kapısı ile dışarı çıkan kaynaklar birlikte kapanıyor.** URL/WebSocket
  izleme kabuk çalıştırmayı gerektirmez, ama bugün `ShellEnabled` kapalıyken
  onlar da kapalı. Ayrıştırmak, monitör yöneticisinin ömrünü kabuk yöneticisinden
  ayırmayı gerektirir.
- **Monitörler bellekte.** Süreç yeniden başlarsa dosya imleci, URL taban
  çizgisi ve soket aboneliği kaybolur; kalıcılık hâlâ kapsam dışı (bkz. §4).
