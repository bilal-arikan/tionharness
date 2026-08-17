# TionSwarm — Profilleme (pprof) Rehberi

> Performans darboğazlarını **tahmin etmeden** bulmak için. Önce profil, sonra optimize.

## Felsefe

pprof **varsayılan olarak kapalıdır** — üretim binary'si hiçbir profilleme yüzeyi
açmaz. Yalnız geliştirme/teşhis sırasında bir ortam değişkeniyle açılır ve **yalnız
loopback** (127.0.0.1) dinler, böylece ağa açılmaz ve Windows Firewall sormaz.

Uçlar `http.DefaultServeMux` üzerinde yayınlanır; ana API sunucusu kendi mux'ını
(`server.Routes()`) kullandığından profiler uygulama rotalarından **tamamen izoledir**.

## Açma

PowerShell:

```powershell
$env:TIONSWARM_PPROF = "1"          # profiler'ı aç (varsayılan kapalı)
# opsiyonel: bind adresini değiştir (varsayılan 127.0.0.1:6060)
$env:TIONSWARM_PPROF_ADDR = "127.0.0.1:6060"
.\tionswarm.exe
```

Açıldığında loglarda şu satır görünür:

```
level=WARN msg="pprof profiling server enabled" addr=http://127.0.0.1:6060/debug/pprof/
```

Tarayıcıdan kontrol: <http://127.0.0.1:6060/debug/pprof/>

## Profil toplama

Önce uygulamayı **gerçek bir iş yükü altına** sok (birkaç ajan + bir flow koşusu +
birkaç zamanlanmış çağrı), sonra profilleri al.

### Heap / allocation (bellek)

```powershell
# Anlık kullanımda olan bellek (sızıntı/retention buradan görülür)
go tool pprof http://127.0.0.1:6060/debug/pprof/heap

# Süreç ömrü boyunca toplam allocation (allocation-heavy yolları bulur)
go tool pprof -alloc_space http://127.0.0.1:6060/debug/pprof/heap
```

### CPU (30 saniyelik örnek)

```powershell
go tool pprof "http://127.0.0.1:6060/debug/pprof/profile?seconds=30"
```

### Goroutine (sızıntı tespiti — per-agent goroutine modeli için kritik)

```powershell
go tool pprof http://127.0.0.1:6060/debug/pprof/goroutine
# veya tam yığın dökümü:
curl http://127.0.0.1:6060/debug/pprof/goroutine?debug=2
```

### Mutex / block (kilit çekişmesi)

`TIONSWARM_PPROF=1` ayrıca `runtime.SetMutexProfileFraction(5)` +
`runtime.SetBlockProfileRate(1ms)` çağırır. **Bunlar olmadan uçlar var ama profil
boş döner** — örnekleme kapalıdır. Kapı kapalıyken hiçbir maliyet oluşmaz.

```powershell
curl -o mutex.txt "http://127.0.0.1:6060/debug/pprof/mutex?debug=1"
curl -o block.txt "http://127.0.0.1:6060/debug/pprof/block?debug=1"
```

`db.(*DB).mu` bu profillerde üst sıradaysa: store'un tek RWMutex'i altında bir
tam-tarama okuma yolu var demektir. `appendMessageLocked` bu kilidi **senkron
dosya yazımı boyunca** tutar ve Go'da bekleyen yazar yeni okurları bloklar —
yani bir poll ile canlı tur birbirini serileştirir.

## pprof içinde gezinme

`go tool pprof` etkileşimli kabuğunda en sık komutlar:

| Komut | İşlev |
|-------|-------|
| `top` | En çok kaynak tüketen 10 fonksiyon |
| `top -cum` | Kümülatif (çağrı zinciri dahil) |
| `list <fonksiyon>` | Fonksiyonun satır-satır maliyeti |
| `web` | SVG çağrı grafiği (Graphviz gerekir) |
| `peek <fonksiyon>` | Çağıran/çağrılanlar |

Web arayüzü (tarayıcıda interaktif flame graph):

```powershell
go tool pprof -http=127.0.0.1:8000 http://127.0.0.1:6060/debug/pprof/heap
```

## TionSwarm'da öncelikli bakılacak sıcak yollar

Profil alırken şu adaylara dikkat et (mimariden türetilmiş hipotezler):

1. **JSON marshal/unmarshal** — dosya-tabanlı store her yazımda entity'yi JSON'a
   çevirir; recall vektörleri (`unmarshalVector`) her recall'da parse edilir.
2. **Memory recall** — `Recall` → `cosineNorm`/`norm`/`buildVector`; her sohbet turu +
   her görevde çalışır (`ContextBlock` enjeksiyonu).
3. **Trace serileştirme** — `marshalSteps` / flow transcript render (büyük turlarda).
4. **Token tahmini** — `EstimateTokens` her compaction kararında tüm mesajları gezer.
5. **Goroutine sızıntısı** — workspace silme / ajan disable yollarında ticker `Stop()`
   ve kanal kapanışı (Go 1.26 `goroutineleak` profili de kullanılabilir).

## Profilleri saklama / karşılaştırma

```powershell
# Profili dosyaya kaydet
curl -o heap_before.prof http://127.0.0.1:6060/debug/pprof/heap
# ... bir optimizasyon uygula ...
curl -o heap_after.prof http://127.0.0.1:6060/debug/pprof/heap
# Farkı incele (before → after)
go tool pprof -base heap_before.prof heap_after.prof
```

## Benchmark ile mikro-ölçüm

Tek bir fonksiyonu izole ölçmek için pprof yerine Go benchmark + `-benchmem`:

```powershell
go test -run=^$ -bench=. -benchmem ./internal/conversation/
# allocation profili de üret:
go test -bench=. -benchmem -memprofile=mem.prof ./internal/db/
go tool pprof mem.prof
```

## Vaka: boşta CPU — activity polling (2026-08-16)

**Belirti:** boşta duran backend'de 249 s CPU / 116 dk uptime → **~%3.6** (tek
çekirdek). 10 workspace açık, 5 pencere.

**Kök neden:** `GET /api/workspaces/activity`, `workspaceRunning()`'i **her
workspace için** çağırıyordu; o da `ListRunningRuns` + `ListRunningFlowRuns` ile
`d.runs` ve `d.flowRuns` map'lerini **tamamen tarıyor**, sonucu sıralıyor ve
`d.mu.RLock()` altında yapıyordu. 5 pencere × 4 sn × 10 workspace ≈ **saniyede
~25 tam-store taraması**, hiçbir iş yokken.

**Ölçüm (benchmark, 2000 flow-run'lı store):**

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| Önce (tam tarama) | 272.932 | 385.074 | 3 |
| Sonra (O(1) sayaç) | 27,8 | 0 | 0 |

~9.800× hızlanma. 125 çağrı/s × 273 µs ≈ **saniyede 34 ms CPU = %3,4** — ölçülen
%3,6 ile birebir örtüşüyor. Ayrıca çağrı başına 385 KB çöp → saniyede ~48 MB GC
baskısı ortadan kalktı.

**Çözüm:** `internal/db`'de `runningFlowRuns` `atomic.Int64` sayacı (`db.go`),
durum-geçiş yollarında güncellenir; okuma `d.mu`'ya **hiç dokunmaz**. Drift
koruması `ReconcileRunCounters` (`store_runcount.go`), 10 dakikada bir mevcut flow
sweeper'ından çağrılır ve sapma bulursa `Error` seviyesinde loglar. Ayrıca frontend
poll'ları gevşetildi + `document.visibilityState` kapısı eklendi (`useAsync`
`pauseWhenHidden`, `useVisiblePoll`).

Taramanın **diğer yarısı** (`d.runs` / `ListRunningRuns`) ise optimize edilmedi,
**silindi**: `Run` entity'sini hiçbir kod yolu üretmiyordu (bkz. aşağıdaki bölüm).

Tekrar ölçüm için:

```powershell
go test ./internal/api -run '^$' -bench WorkspaceRunning -benchmem
```

Süreç düzeyinde önce/sonra:

```powershell
$p = Get-Process tionswarm-dev
$t0 = $p.TotalProcessorTime; Start-Sleep -Seconds 300; $p.Refresh()
$sec = ($p.TotalProcessorTime - $t0).TotalSeconds
"CPU: {0:N2} s / 300 s = %{1:N2} (tek çekirdek)" -f $sec, ($sec/300*100)
```

## Store bellek ayak izi (mesaj lazy-load ölçümü)

Tüm store `db.Open`'da RAM'e yüklenir ve baskın kalem transkriptlerdir: her
oturumun `messages.jsonl`'i `[]Message`'a parse edilip süreç ömrü boyunca tutulur.
Bunu ölçmek için pprof'a gerek yok, iki hazır kaynak var:

**1) Boot log'u** — her workspace açılışında bir satır (`internal/workspace/manager.go`):

```
level=INFO msg="store opened" workspace=WS17 ms=412 sessions=84 messages=9130 message_mb=118
```

`ms` o workspace'in store açılış süresi, `message_mb` mesajların yaklaşık heap
maliyeti. Yavaş bir açılışta hangi workspace'in sorumlu olduğu doğrudan görünür.

**2) `GET /api/debug/store-stats`** — workspace başına + toplam anlık ayak izi
(profiler kapalıyken de çalışır, workspace-scoped değildir):

```powershell
curl.exe http://127.0.0.1:8080/api/debug/store-stats | ConvertFrom-Json | Select -Expand totals
```

Dönen alanlar: `sessions`, `loadedSessions`, `messages`, `messageBytes` +
entity sayıları. `loadedSessions` bugün `sessions`'a eşittir (yükleme eager);
mesaj lazy-load'u geldiğinde **hareket edecek metrik budur** — aynı uç,
sözleşmesi değişmeden "ne kadar fayda etti?" sorusunu yanıtlar.

Heap profilinde karşılığı: boot sonrası `-alloc_space`'te `db.decodeMessages` ve
`encoding/json.Unmarshal` üst sıralardaysa maliyet buradadır.

Boot log'undaki `phases_ms` alanı `db.Open`'ın içini yavaştan hızlıya döker
(`counters`, `entities`, `usage`, `sessionUsage`, `toolConfig`,
`modelResolutions`, `sessions`, `recoverInflight`, `migrateUnifiedLayout`,
`cleanupRenders`). Toplam süre tek başına **hangi** loader'ın suçlu olduğunu
söylemez; aşağıdaki vaka tam da bu yüzden yanlış teşhis edilmeye çok yakındı.

## Vaka: 59 sn'lik boot — soğuk dosya açma (2026-08-17)

**Belirti:** 10 workspace'in `db.Open` toplamı **59,1 sn**. İlk hipotez:
"93 MB'lık `messages.jsonl` parse ediliyor" (bkz. mesaj lazy-load planı).

**Ölçüm hipotezi çürüttü.** Maliyet mesaj MB'ıyla değil, **dosya sayısıyla**
ölçekleniyordu:

| Workspace | Süre | Oturum | Mesaj MB | ms/oturum |
|---|---|---|---|---|
| WS1 | 13.152 ms | 104 | **2** | 126 |
| WS15 | 5.537 ms | 71 | **18** | 78 |
| WS17 | 15.936 ms | 90 | 32 | 177 |

WS1 yalnız **2 MB** mesaj taşıyor ama 13 sn sürüyor; WS15 dokuz katı mesajla
oturum başına daha ucuz. Yani parse değil, **dosya açma** baskın.

**Ham ölçüm (WS1 store'u, 720 dosya / 3,9 MB):**

| | Süre | Dosya başına |
|---|---|---|
| Soğuk seri okuma | 10.812 ms | **15,02 ms** |
| Sıcak seri okuma | 101 ms | 0,14 ms |
| Soğuk **paralel** (16 worker, WS17/1315 dosya) | 1.886 ms | **1,43 ms** |

Soğuk/sıcak farkı **107×**. Bu, NTFS'in değil, **dosya-açma başına ~15 ms'lik
bir filtre sürücüsünün** (Windows Defender gerçek-zamanlı tarama) imzasıdır.
Maliyet CPU değil **gecikme** olduğu için worker'lar çekirdek değil, bekleyen
syscall harcar — paralelleştirme doğrudan kazanç verir.

**Kontrollü A/B** (aynı ağacın yarısı seri, yarısı 16 worker ile — iki yarı da
eşit soğuk olduğundan cache durumu aynı):

| Store | Seri | Paralel | Hızlanma |
|---|---|---|---|
| WS5 (701 dosya) | 13,48 ms/dosya | 2,13 ms/dosya | **6,3×** |
| WS16 (265 dosya) | 25,67 ms/dosya | 8,69 ms/dosya | **3,0×** |

Değişkenlik iki yarının dosya boyut dağılımından ve arka plan yükünden geliyor;
gerçekçi bant **3–6×**.

**Sonuçlar:**

1. **Store yüklemesi paralelleştirildi** ✅ (`internal/db/loadpar.go`). Boot'ta
   dosya okuyan her yol sınırlı bir worker havuzundan geçiyor: `loadJSONDir`
   (tüm entity dizinleri), artifact içerik dosyaları, `loadSessions`
   (oturum başına `session.json` + `messages.jsonl`) ve `recoverInflight`
   (oturum başına sidecar sondası). Disk formatı, lazy yükleme veya cache
   politikası gerekmedi.
2. **Defender istisnası** (`~/.tionswarm`) kod dışı ama muhtemelen en büyük tek
   kazanç — 15 ms/dosya doğrudan buradan geliyor. Paralelleştirme bu maliyeti
   gizler, **ortadan kaldırmaz**.
3. **Mesaj lazy-load'u boot için ikincil**: oturum başına ~4 dosyanın yalnız
   birini (`messages.jsonl`) eler. Asıl faydası **RAM**'dir — ölçülen 95 MB
   (`messageBytes`), 300 MB'lık RSS'in ~%32'si.

Ölçümü tekrarlamak için boot log'undaki `ms` + `phases_ms` alanlarına bak.
**Önce/sonra karşılaştırırken cache'e dikkat**: yeniden başlatılan bir uygulama
dosyaları OS cache'inden okur ve her iki sürüm de hızlı görünür. Anlamlı
karşılaştırma ya makine yeniden başlatıldıktan sonra ya da yukarıdaki gibi
**aynı ağacın iki eşit-soğuk yarısı** ile yapılır.

## Notlar

- Profiler dinleyicisi graceful shutdown'a bağlı değildir; süreç bitince kapanır
  (loopback-only ve teşhis amaçlı olduğundan kabul edilebilir).
- Wiring: `cmd/tionswarm/pprof.go` (`startPprof`), `cmd/tionswarm/main.go` (config sonrası çağrı).
