# SwarmGo — Profilleme (pprof) Rehberi

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
$env:SWARMGO_PPROF = "1"          # profiler'ı aç (varsayılan kapalı)
# opsiyonel: bind adresini değiştir (varsayılan 127.0.0.1:6060)
$env:SWARMGO_PPROF_ADDR = "127.0.0.1:6060"
.\swarmgo.exe
```

Açıldığında loglarda şu satır görünür:

```
level=WARN msg="pprof profiling server enabled" addr=http://127.0.0.1:6060/debug/pprof/
```

Tarayıcıdan kontrol: <http://127.0.0.1:6060/debug/pprof/>

## Profil toplama

Önce uygulamayı **gerçek bir iş yükü altına** sok (birkaç ajan + bir flow koşusu +
heartbeat birkaç dakika), sonra profilleri al.

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

## SwarmGo'da öncelikli bakılacak sıcak yollar

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
go test -run=^$ -bench=BenchmarkRecall -benchmem ./internal/memory/
# allocation profili de üret:
go test -bench=. -benchmem -memprofile=mem.prof ./internal/memory/
go tool pprof mem.prof
```

## Notlar

- Profiler dinleyicisi graceful shutdown'a bağlı değildir; süreç bitince kapanır
  (loopback-only ve teşhis amaçlı olduğundan kabul edilebilir).
- Wiring: `cmd/swarmgo/pprof.go` (`startPprof`), `cmd/swarmgo/main.go` (config sonrası çağrı).
