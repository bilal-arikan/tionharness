# Kaynak kullanımı incelemesi — 28 Eylül 2026

Uygulama sonrası sonuçlar: [Kaynak optimizasyonları ve doğrulama](KAYNAK-OPTIMIZASYONU-2026-09-28.md). Bu belge ilk ölçümün tarihsel kaydıdır.

> **Özet:** Öncelik arayüz hızı ve bellek tüketimidir. En güçlü fırsatlar sohbet geçmişini sayfalama, yalnız görünür mesajları bileşenleştirme, eski transkriptleri ihtiyaç halinde yükleme ve tamamlanmış işlemlerin olay belleğini küçültmedir. İnceleme sırasında ürün kodu değiştirilmedi; ücretli model çağrısı yapılmadı. Gerçek veriler salt okunur envanterlendi, depolama deneyi ayrı kopyalar üzerinde yürütüldü.

## Ölçüm kapsamı ve sınırlar

- Windows 11, Intel Core Ultra 9 275HX, 24 mantıksal işlemci, yaklaşık 31,4 GiB fiziksel bellek.
- Go 1.26.4 `windows/amd64`, Node 24.18.1, üretim frontend derlemesi; tarayıcı Playwright 1.62.1 / Chromium 151.0.7922.34, headless, 1440×900.
- Kaydedilen Git HEAD: `46228b8b4cb4e5c49c693785966c6b632ed9714b`. Çalışma alanı kullanıcıyla paylaşıldığı için bu, ölçüm anındaki dosyaların değişmez bir Git anlık görüntüsü olduğu anlamına gelmez.
- Başlangıçta çalışan bir TionHarness süreci bulunamadı. Uygulama ayrı veri dizininde, yalnız loopback üzerinde çalıştırıldı. Mevcut Windows uygulamalarının tüketimi TionHarness'e yazılmadı.
- Sentetik veri: 50, 500 ve 2.000 mesajlık üç sohbet; ayrıca 40 arşiv sohbetinde toplam 2.000 mesaj. Genel toplam 43 sohbet / 4.550 mesaj. Asistan cevapları Markdown, tablo, kod bloğu ve yaklaşık 8,4 KB araç çıktısı içerir.
- 2.000 mesajlık tek sohbet büyüme/stres senaryosudur. Gerçek veri kopyasında 330 oturuma dağılmış 2.357 mesaj vardır; sentetik senaryoyu bugünkü tipik kullanım diye yorumlamamak gerekir.
- Tarayıcı soğuk açılışı üç ayrı önbelleksiz bağlamda tekrarlandı. API karşılaştırmasında iki ısınma ve 11 ölçüm yapıldı. Küçük örneklem yüzünden p95 değerleri istatistiksel kapasite tahmini değildir.
- Tarayıcı bellek sayıları zorlanmış GC sonrasındaki **JavaScript heap** değerleridir; tüm Chromium/WebView2 sürecinin RAM'i değildir. Go heap, Windows working set ve private bytes birbirinden farklı ölçülerdir.
- Kaydırma testi 2,4 saniyede tüm transkript boyunca programatik `requestAnimationFrame` taramasıdır. Normal fare kullanımı veya ekranın fiziksel FPS ölçümü değildir; büyük geçmişlerdeki ölçeklenme baskısını gösterir.
- GPU, WebView2'nin gerçek masaüstü süreç ağacı, canlı model akışı, çoklu gerçek ajan yükü ve token/fiyat maliyetleri ölçülmedi. Bunlar için sonuç uydurulmadı.

Ham sayısal sonuçlar: [ölçüm dosyası](olcumler/kaynak-kullanimi-2026-09-28.json). Ayrıntılı profiller ve tekrar üretim araçları yerel `.scratch/resource-audit-20260928/` dizinindedir.

## Ölçülen mevcut durum

### Gerçek verinin depolama ve bellek ayak izi

Kayıtlı beş workspace dizini toplam **8.397 dosya / 351,1 MiB** içeriyor. Bunun **57,0 MiB** kısmı 327 `messages.jsonl` dosyasıdır. Bu toplam bütün bilgisayarı veya global yedek/sağlayıcı dizinlerini kapsamaz.

Gerçek store'ların ayrı kopyaları tek Go sürecinde açıldığında:

| Ölçü | Sonuç |
|---|---:|
| Oturum / transkripti yüklenen oturum | 330 / 327 |
| Belleğe alınan mesaj | 2.357 |
| Store'un yaklaşık mesaj belleği hesabı | 54,0 MiB |
| Beş store açıkken GC sonrası toplam Go heap | 64,3 MiB |
| Beş `db.Open` süresinin toplamı | 5,66 sn |

64,3 MiB yalnız bu inceleme programının heap'idir; tam uygulamanın toplam RAM'i değildir. 54,0 MiB ise `Stats().MessageBytes` tahminidir, allocator ölçümü değildir. Açılış süreleri yeni kopyalanmış dosyalardan alınmıştır; yeniden başlatılmış bilgisayardaki soğuk disk ölçümü olarak kullanılamaz.

| Workspace | Oturum | Mesaj | Mesaj belleği tahmini | Store açılışı |
|---|---:|---:|---:|---:|
| WS1 | 101 | 621 | 5,2 MiB | 1.382 ms |
| WS5 | 56 | 668 | 9,1 MiB | 2.706 ms |
| WS10 | 80 | 551 | 15,9 MiB | 727 ms |
| WS14 | 6 | 22 | 0,6 MiB | 187 ms |
| WS15 | 87 | 495 | 23,3 MiB | 655 ms |

### Sentetik uygulamanın boşta tüketimi

30,88 saniyelik, tarayıcı bağlı olmayan örnekte CPU süresi **0,0156 saniye**, tek çekirdeğe göre ortalama kullanım **%0,051** oldu. Windows working set yaklaşık **52,9 MiB**, private bytes **88,1 MiB** seviyesindeydi. Bu kısa pencere uzun süreli bellek sızıntısını dışlamaz; yalnız boşta tüketimin bu örnekte ana problem olmadığını gösterir.

GC sonrası pprof örneğinde canlı heap **27,1 MiB**; `decodeMessages` çağrı ağacının payı **18,6 MiB / %68,5**. Boot boyunca toplam tahsis yaklaşık **104,4 MiB**. Toplam tahsis, aynı anda kullanılan bellek değildir.

### Sohbetin tarayıcı maliyeti

Aşağıdaki sayılar üç tekrarın medyanıdır:

| Mesaj | Sohbet hazır ölçüm süresi | JS heap | Tarayıcı DOM düğümü | Kaydırma kare aralığı p95 |
|---|---:|---:|---:|---:|
| 50 | 1,88 sn | 9,7 MiB | 6.129 | 4,3 ms |
| 500 | 2,19 sn | 27,0 MiB | 47.529 | 79,2 ms |
| 2.000 | 3,47 sn | 81,9 MiB | 185.713 | 325,0 ms |

“Sohbet hazır” süresi gezinmenin başlangıcından tüm mesajların DOM'da görünmesine kadar ölçülür ve **300 ms ölçüm yerleşme beklemesini içerir**. Yeni tarayıcı bağlamı uygulamanın ilk kurulum davranışını da kullanır; kodda 1.100 ms minimum splash vardır. Bu süreler günlük kullanımda sıcak sohbet değiştirmenin birebir karşılığı değildir. İlk içerik boyaması medyanı üç senaryoda da yaklaşık 52–56 ms'dir; bu çoğunlukla açılış kabuğunun görünmesidir, sohbetin hazır olması değildir.

### API: tüm geçmiş ve son 50 mesaj

| Oturum | Tüm geçmiş gövdesi | Tüm geçmiş medyanı | Son 50 gövdesi | Son 50 medyanı |
|---|---:|---:|---:|---:|
| 50 mesaj | 80.432 B | 16,29 ms | 80.498 B | 15,61 ms |
| 500 mesaj | 806.282 B | 29,27 ms | 80.719 B | 15,07 ms |
| 2.000 mesaj | 3.231.782 B | 84,46 ms | 80.921 B | 14,72 ms |

2.000 mesajda son 50'yi istemek gövdeyi **%97,5 küçültüyor**, bu yerel ölçümde uçtan uca isteği **5,7 kat hızlandırıyor**. Bu, mevcut iki API yolunun karşılaştırmasıdır; henüz değiştirilmiş bir arayüzün kazanımı değildir.

## Optimizasyon listesi

Öncelikler kullanıcıya hissedilen etki, ölçüm gücü ve uygulama riskine göre sıralanmıştır.

| Öncelik | Alan | Kanıt | Önerilen değişim | Efor / risk |
|---|---|---|---|---|
| P1 | Sohbet geçmişi sayfalama | 2.000 mesaj: 3,23 MB → 81 KB | İlk açılışta son 50–100 mesaj; geçmişe gidildikçe önceki sayfa | Orta |
| P1 | Mesaj listesi ve kaydırma | 185.713 DOM düğümü, 81,9 MiB JS heap | Görünür pencereyi bileşenleştir; sabit başlık ölçümünü daralt | Orta–yüksek |
| P1 | Arka uç transkript belleği | Gerçek veride 327 transkript açılışta RAM'de | Aktif transkriptler için sınırlı önbellek, diğerlerini diskten yükleme | Yüksek; depolama sözleşmesi |
| P1 | Olay geçmişinin bellekte tutulması | 512 sınırına rağmen 5.000 olay commit sonrasında kalıyor | Commit'te sınırı uygula, bırakılan referansları temizle | Küçük–orta |
| P2 | Araç izinin tekrar ayrıştırılması | Stres CPU profilinin %67,7'si bu çağrı ağacında | Sayfalama + sınırlı/değişiklik duyarlı önizleme üretimi | Orta |
| P2 | Tur sonunda tekrarlı tam yenileme | İki bağımsız olay yolu aynı geçmişi tekrar istiyor | Tek yenileme sahibi, istek birleştirme ve iptal | Küçük–orta; kaynak kanıtı |
| P2 | Başlangıç paketi ve HTTP önbelleği | Başlangıç JS/CSS 2,42 MB; sıkıştırma/önbellek başlığı yok | Daha fazla ekranı tembel yükle; hash'li varlıklara önbellek ve önceden sıkıştırma | Orta |
| P2 | Küçük dosyalardan kullanım kaydı yükleme | WS5 açılışının 1,39 sn'si sessionUsage | Kullanım kayıtlarını toplu snapshot/segment olarak yükle | Orta–yüksek |
| P3 | Boot sırasında geçici kopyalar | `readJSONLines` 24,4 MiB, sayaç yeniden taraması 21,7 MiB tahsis | Akış halinde çözümleme, uyumlu sayaç/checkpoint tasarımı | Orta |
| P3 | Eski indeks dizinleri | İki `cbm-store` toplam 232,6 MiB | Kullanılmadığı doğrulanırsa arşivle/temizle | Küçük; manuel doğrulama |

### 1. Arayüzü sayfalı geçmişe geçirmek

`useSessionsController.ts:412`, `api.listMessages` üzerinden bütün geçmişi alıyor. Sunucudaki `handleListMessages` ise zaten `?tail=N` destekliyor (`internal/api/sessions.go:313`).

Dar store okuyucusunda da ölçülebilir fark var: 2.000 mesaj için `ListMessages` **581.656 B/işlem ve 96,4 µs**, `ListMessagesTail(...,50)` **16.408 B/işlem ve 3,2 µs**. Bunlar mesaj struct'larının sığ kopyalarıdır; her string gövdesinin ayrıca kopyalandığı anlamına gelmez. API'nin daha büyük maliyeti JSON işleme ve serileştirmeden geliyor.

Öneri: mevcut tail desteği ilk adım olarak kullanılabilir. Uzun vadede `beforeMessageId`/cursor ile önceki sayfayı almak daha uygundur; `tail` sayısını sürekli büyütmek eski veriyi tekrar taşır. Arama sonucu bağlantıları, mesaj silme, rewind ve canlı mesaja ekleme aynı mesaj kimlikleriyle korunmalı.

### 2. Görünmeyen mesajların bileşen maliyetini kaldırmak

`MessageList.tsx:80` içindeki `contentVisibility: 'auto'` çizim/layout işini azaltıyor, fakat bütün React alt ağaçlarını oluşturup DOM'da tutuyor. Mevcut `React.memo`, sabit callback ve Markdown memo önlemleri değerlidir; bu inceleme bunların eksik olduğunu iddia etmiyor.

Ek aday: `MessageList.tsx:219` içindeki `updateActivePinned`, her kullanıcı mesajında `getBoundingClientRect()` okuyor. `requestAnimationFrame` birleştirmesi zaten var, fakat 2.000 mesajda hâlâ bir güncellemede yaklaşık 1.000 satırın geometrisi okunabiliyor. Ölçüm kaydırma yavaşlamasını gösteriyor; bunun ne kadarının bu fonksiyondan geldiği ayrı fonksiyon düzeyinde ölçülmedi.

Öneri: değişken yükseklikli görünür pencere + sınırlı taşma alanı; eski satırların Markdown, kod renklendirme ve araç kartlarını mount etmemek. Sabit başlık için görünür sınırı izleyen gözlemci/indeks kullanılabilir. Arama hedeflerine kaydırma ve pinned soru davranışları bugün gerçek DOM düğümlerine bağlı olduğundan geçiş test edilmelidir.

### 3. Eski transkriptleri ihtiyaç halinde yüklemek

`internal/db/store_load.go:17–70`, her oturumu açarken tüm mesajları okuyor ve `d.messages` içine yerleştiriyor. Arşivdeki oturumlar da yükleniyor. Gerçek veri kopyasında bu nedenle 327 transkript, herhangi bir sohbet görüntülenmeden bellekte.

Öneri: oturum başlıklarını başlangıçta yüklemeye devam et; transkriptlere boyut sınırlı LRU/aktif oturum önbelleği uygula. Yazılan/çalışan oturumlar tutulmalı, diğerleri çıkarılabilmeli. `LastMessage`, `ListMessagesTail`, `FindMessage` ve `StreamMessages` mevcut geçiş noktalarıdır.

54,0 MiB mevcut mesaj tahmini, tamamen kazanılabilecek RAM sözü değildir: aktif oturumlar ve önbellek yine bellek kullanacaktır. Arşivleme, arama, silme, crash recovery ve CLI reply transaction davranışları bu değişimde korunmalı.

### 4. Commit sonrasında olay belleğini gerçekten sınırlamak

`internal/sessionhub/hub.go:152` aktif turun henüz kalıcılaşmamış olaylarını 512 sınırının üzerinde tutuyor; canlı yeniden bağlanma için bu anlaşılır. Ancak `Commit` (`:296`) yalnız sıra numarasını güncelliyor. Halka burada küçülmüyor.

Gerçek `sessionhub` koduyla, her biri 2.048 karakter gövdeli 5.000 olay üretildi. GC sonrası heap tabanı yaklaşık **0,63 MiB** iken commit sonrasında **12,0 MiB** kaldı. `Replay(..., since=1)` hâlâ **4.999 olay** döndürdü. Bir sonraki publish sonrasında da heap yaklaşık 12,0 MiB kaldı; dilim başını ilerletmek eski backing array referanslarını bırakmadı. `Drop` sonrasında heap yaklaşık **0,64 MiB** oldu.

Bu, sınırsız büyümenin her normal sohbette yaşandığının kanıtı değildir; büyük turun tamamlanması/boşta kalması sınırındaki yeniden üretilebilir retention sorunudur. Öneri: commit'te tutulacak aralığı küçült, çıkarılan `Event` referanslarını sıfırla veya korunacak kuyruğu yeni küçük diziye taşı. Bağlantısı olmayan eski oturumlara TTL/byte bütçesi ekleme ayrıca değerlendirilebilir. Henüz kalıcılaşmamış olaylar sessizce atılmamalı; mevcut cursor/reset sözleşmesi korunmalı.

### 5. Araç önizlemesini her okumada yeniden üretmemek

`internal/api/sessions.go:358`, döndürülen her mesaj için `trimStepsJSON` çağırıyor. `internal/api/steps_trim.go:37` büyük iz JSON'unu `[]any` olarak çözüp uzun alanları kesiyor ve yeniden serileştiriyor.

15 saniyelik, 2.000 mesaj uç noktasını tekrarlayan CPU profilinde **%67,66 kümülatif CPU** bu fonksiyonun çağrı ağacında. Öncesi/sonrası allocation profilinde 78 ölçüm/ısınma isteği boyunca yaklaşık **1.110,9 MiB tahsis**, bunun **936,0 MiB / %84,3** kısmı aynı yolda. Bunlar süreç genelindeki sürekli RAM tüketimi veya normal kullanım CPU yüzdesi değildir.

Önce sayfalama, sonra gerekiyorsa mesaj sürümüyle anahtarlanan **sınırlı** önizleme önbelleği veya kalıcı küçük önizleme alanı. Bütün transkriptlerin ikinci kopyasını RAM'e koymak bellek hedefiyle çelişir. Ham araç sonucu model bağlamı/audit için korunmalı; yalnız gösterim projeksiyonu değişmeli.

### 6. Tamamlanma yenilemelerini birleştirmek

`internal/api/chat_turn_phases.go:1030–1049` hem session hub `turn_done` hem genel `chat` olayı yayımlıyor. `chatStreamHub.ts:540` → `useChatStream.ts:416` tam geçmişi tekrar alıyor; `useAppEvents.ts:210` genel chat olayında yine `listMessages` çağırıyor.

Bu iki yenileme yolunun varlığı kaynakta doğrulandı; ücretli/live tur çalıştırılmadığı için gerçek bir model turunda kaç HTTP isteği üretildiği ölçülmedi. Öneri: aktif stream'in `reply` olayını mevcut mesajlara uygula; tamamlanma için tek yenileme sahibi belirle. Yenileme gerekiyorsa workspace/session/sürüm anahtarıyla eşzamanlı istekleri birleştir. Hızlı oturum değişimlerinde önceki fetch'i iptal etmek de gereksiz işi azaltır.

### 7. Başlangıç paketi ve statik dosya sunumu

Üretim derlemesinde başlangıç HTML'inin yüklediği JS/CSS **2.415.710 bayt**; ana `index` chunk'ı **1.609.068 bayt**. Aynı varlıkların çevrimdışı gzip toplamı **679.908 bayt**: yaklaşık **%71,9** daha küçük. Bu sıkıştırma potansiyelidir, uygulanmış hızlanma değildir.

`Accept-Encoding: gzip, br` ile yapılan gerçek isteklerde `Content-Encoding`, `Cache-Control`, `ETag` ve `Last-Modified` bulunmadı. `internal/web/embed.go:28` gömülü dosyaları doğrudan `http.FileServer` ile sunuyor. `App.tsx:45–61` birçok büyük ekranı statik import ediyor; `lazyPanels.ts` yalnız üç ana paneli erteliyor.

Öneri: ilk ekranda gerekmeyen ayarlar/market/bütçe/insight gibi ekranları dinamik yükle. Hash içeren varlıklara uzun ömürlü `immutable` önbellek ver; HTML'in yenilenebilir kalmasını koru. JS/CSS için build sırasında gzip/Brotli üretmek runtime CPU maliyetini de azaltır. SSE'yi körlemesine sıkıştıran genel middleware yerine statik içerikten başlamak daha düşük risklidir. Yerel masaüstünde etkisi ağdan çok parse/compile ve tekrar açılış yüküdür; uzaktan erişimde bant genişliği kazancı büyür.

### 8. Küçük kullanım dosyalarının boot yükü

Gerçek WS5 kopyasında yalnız 56 oturum bulunmasına rağmen `store/session-usage` altında **2.165 dosya / 1,14 MiB** var. `loadSessionUsage` açılışı **1.391 ms**, workspace'in toplam 2.706 ms süresinin yaklaşık **%51'i**. `internal/db/store_session_usage.go:44` bu kayıtların hepsini `loadJSONDir` ile açıyor.

`internal/db/loadpar.go` zaten en fazla 16 worker ile paralel okuyor. Bu nedenle öneri “paralelleştirme ekle” değil; küçük dosya sayısını azaltan, crash-safe toplu snapshot + ekleme günlüğü veya segmentlenmiş arşiv. Eski oturum maliyetleri faturalama geçmişi olabilir; oturum silinmiş diye bu kayıtlar silinmemeli.

### 9. Boot geçici tahsisleri

Sentetik boot profilinde `readJSONLines` yaklaşık **24,4 MiB**, `countToolSteps` çağrı ağacı **21,7 MiB** tahsis etti. `store_load.go:157` bütün satırları önce kopyalıyor; ardından decode ediliyor. `store_activity.go:60` yalnız adım sayısı için büyük JSON'u tekrar tarıyor.

Öneri: satırı akış halinde decode ederek ara `[][]byte` yükünü azaltmak. Sayaç için kalıcı, doğrulanabilir checkpoint ile yalnız kuyruğu uzlaştırma değerlendirilebilir. Mevcut başlık sayaçları append yolunda bilerek güncel tutulmadığından doğrudan başlığa güvenmek hatalı olur. Bozuk son satırı tolere etme ve önceki satır bozukluğunu hata sayma davranışı korunmalı.

### 10. Eski workspace indeksleri

WS10 `cbm-store` **214,1 MiB**, WS15 `cbm-store` **18,5 MiB**: toplam **232,6 MiB**, ölçülen workspace disk alanının yaklaşık **%66,3'ü**. `_Docs/54-CAPABILITY-PROBE.md:196` workspace başına CBM store düzeninin kaldırıldığını kaydediyor; güncel ürün kaynaklarında bu adın kullanımına rastlanmadı.

Bunlar güçlü temizlik adaylarıdır, ancak harici MCP yapılandırmalarının bu dizinleri kullanmadığı bu incelemede doğrulanmadı. Otomatik silme yapılmadı. Önce etkin cache yolları/harici süreç kullanımı doğrulanmalı, sonra geri alınabilir arşivleme/temizlik yapılmalı. Bu fırsat disk içindir; doğrudan UI RAM kazancı sayılmamalıdır.

## Önceden mevcut ve korunması gereken iyileştirmeler

- Akış güncellemeleri görünür sohbette 50 ms, farklı ekranda 500 ms birleştiriliyor.
- Mesaj satırlarında memo/sabit callback, Markdown ve adım parse işlemlerinde memo kullanılıyor.
- Ekran dışı satırlarda `content-visibility` ve görünürlük kontrollü polling var.
- Workspace activity kontrolü tam geçmiş taraması yerine O(1) çalışma sayaçları kullanıyor.
- API araç çıktısını alan başına 2.048 bayta kesiyor; tam iz ayrıca getirilebiliyor.
- Boot dosya okumaları zaten sınırlı worker havuzuyla paralelleştirilmiş.

Bu mekanizmalar kaldırılmamalı; önerilen çalışmalar bunların üstündeki kalan maliyeti hedefliyor.

## Doğrulama ve sonraki kabul ölçüleri

Üretim frontend ve Windows backend derlemeleri başarılı. Zorunlu `scripts/test.sh full` geçidi başarılı: Go paketleri, **143 frontend test dosyası / 1.026 test**, depcheck ve `git diff --check` geçti. Ürün kodu değişmediğinden bu, optimizasyon sonrası doğrulama değil mevcut durumun taban çizgisidir.

İlk tarayıcı tekrar-açma deneyinde oluşan artış, ölçüm aracının dispose edilmeyen `ElementHandle` referanslarıyla kirlenmişti. O retention serisi ürün sızıntısı kanıtı olarak kullanılmadı; handle üretmeyen kontrol ölçümü ayrı kaydedildi.

Kontrol deneyinde 10 aç/kapa sonunda 500 mesajlık sohbetin açık durum heap'i **26,5 → 28,6 MiB**, 2.000 mesajınki **83,1 → 85,2 MiB** oldu. 2.000 mesajda açık DOM düğümü ilk ve son ölçümde **185.713**, son kapalı ölçümde **1.645** idi. Böylece ilk serideki her açılışta tüm transkript kadar artış doğrulanmadı. Bir ara örnekte geçici yüksek retention görüldü ve sonraki döngüde geri alındı; bu kısa test her türlü uzun süreli sızıntıyı dışlamaz.

Önerilen uygulama sırası: **sayfalama → görünür mesaj penceresi → olay belleği sınırı → eski transkriptleri diskten yükleme → önizleme/yenileme birleştirme → başlangıç paketi ve dosya sayısı**. İlk üçü arayüz ve bellek hedeflerine doğrudan dokunur; store dönüşümü daha geniş regresyon kapsamı ister.

Kabul ölçüleri:

1. 2.000 mesajlık oturumu ilk açarken yaklaşık 50–100 mesaj taşınmalı ve DOM boyutu toplam geçmişe göre doğrusal büyümemeli.
2. 50/500/2.000 senaryoları aynı veri, pencere boyutu ve build türüyle yeniden ölçülmeli; ilk kurulum splash süresi ayrıca ayrılmalı.
3. Büyük bir tur commit edildikten sonra hub ring'i ayarlanan sınıra inmeli ve atılan gövdeler GC ile geri alınabilmeli.
4. Aktif olmayan/arşiv oturum sayısı artarken boşta transkript RAM'i belirlenmiş bütçeyi aşmamalı.
5. Bir tamamlanma olayı, aktif sohbet için gereksiz ikinci tam-geçmiş isteği üretmemeli.
6. Arama hedefi, rewind, canlı akış, yeniden bağlanma ve bozuk son satır recovery davranışları korunmalı.

Graph sorguları Tier 2 kapsamında sembol/çağrı ilişkilerini bulmak için kullanıldı. Dayanılan dosyalarda coverage kontrolü yapıldı; `metadata_changed` sonuçları nedeniyle ilgili güncel kaynaklar ayrıca doğrudan okundu. Bu çalışma tüm kod yollarının tüketimini eksiksiz kanıtlayan bir denetim değildir; ölçülen senaryolar ve yukarıdaki sınırlar için kanıta dayalı bir öncelik listesidir.
