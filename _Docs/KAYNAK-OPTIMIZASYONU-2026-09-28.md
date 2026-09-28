# Kaynak optimizasyonları — 28 Eylül 2026

> **Özet:** Önceki incelemedeki on fırsat uygulandı. Öncelik arayüz hızı ve bellekti: geçmiş sayfalara bölündü, yalnız görünür mesajlar oluşturuluyor, eski transkriptler gerektiğinde yükleniyor ve önbellekler sınırlandırılıyor. Çalışma sırasında sayfalama, kaydırma, kayıt hataları ve eşzamanlı yenileme sorunları da düzeltildi. Bu belge uygulama ve doğrulama kaydıdır; [ilk inceleme](KAYNAK-KULLANIMI-2026-09-28.md) değişiklik öncesindeki ölçümleri korur.

## Ölçülen değişim

Windows üzerinde aynı sentetik veri, üretim derlemesi ve Chromium/1440×900 penceresi kullanıldı. Tarayıcı değerleri üç tekrarın medyanıdır. Bellek, zorlanmış GC sonrasındaki JavaScript heap'idir; tüm tarayıcı veya masaüstü uygulamasının RAM'i değildir.

| Ölçü | Önce | Sonra |
|---|---:|---:|
| 2.000 mesajı açarken JS heap | 81,87 MiB | 7,20 MiB |
| Aynı ekranda DOM düğümü | 185.713 | 2.073 |
| İlk açılışta taşınan veri | 5,72 MiB | 0,89 MiB |
| Sohbet hazır ölçüm süresi | 3,47 sn | 2,15 sn |
| Kaydırma kare aralığı p95 | 325 ms | 20,8 ms |
| Gerçek beş store kopyası açıkken Go heap | 64,25 MiB | 8,82 MiB |
| Store açılışında belleğe alınan transkript mesajı | 2.357 | 0 |
| 5.000 olay commit edildikten sonra hub'ın ek heap'i | 11,37 MiB | 1,17 MiB |

Tablonun tarayıcı sonuçları kaydırma düzeltmeleri tamamlanmış nihai sürüme aittir. İlk optimize edilmiş sürümde 50 / 500 / 2.000 mesajlık sohbetlerde JS heap sırasıyla **7,19 / 7,43 / 7,33 MiB** olmuştu; nihai 2.000 mesaj tekrarı **7,20 MiB** doğruladı. İlk istek her boyutta 50 mesaj taşıdı. Ekrana başlangıçta yaklaşık beş mesaj bileşeni bağlandı; kalanlar kaydırma sırasında oluşturuluyor. Hazır ölçümü 300 ms bekleme içerir; uygulamanın yaklaşık 1,1 saniyelik açılış ekranı da sürenin içindedir.

Kaydırma deneyi 2,4 saniyelik programatik taramadır; fiziksel ekran FPS'i veya normal fare kullanımının birebir karşılığı değildir. Önceden 2.000 mesajın tamamı, sonrasında 150 mesajlık sınırlı pencere taranmıştır. Nihai üç denemede 50 ms'yi aşan kare veya uzun görev görülmedi; gözlenen en büyük kare aralığı **41,6 ms**, üç koşunun p95 medyanı **20,8 ms** idi. Önceki ara sürümdeki 4,3 ms değeri tarihsel ölçüm dosyasında tutulur; nihai başarı değeri olarak kullanılmaz. Bu karşılaştırma yeni gösterim yaklaşımının etkisini ölçer.

Sohbet → ajanlar → aynı sohbet şeklindeki on aç/kapa döngüsünde, test aracında mesaj düğümü referansı tutulmadan ölçülen açık ekran heap'i **7,17 → 9,53 MiB** oldu. DOM **2.073 → 2.078** düzeyinde kaldı. Son iki döngünün toplam artışı yaklaşık **64,7 KiB**; her açılışta tüm geçmiş büyüklüğünde birikim görülmedi. Yine de 2,35 MiB toplam artış gizlenmemiştir; bu kısa deney her tür uzun süreli sızıntının olmadığını kanıtlamaz.

API'de 2.000 mesajın tamamını isteyen eski uç noktanın medyanı **84,46 → 28,00 ms** oldu. Yeni 50 mesajlık sayfanın medyanı **14,52 ms**, gövdesi yaklaşık **79 KiB**; eski tam gövde **3,08 MiB** idi. Bu API serisi, konuşma özeti alanlarının sonradan eklendiği nihai sürümden önce alınmıştır; nihai sayfa birkaç küçük özet alanı daha taşır. Aynı 14,5 saniyelik seri yükte tam geçmiş istek sayısı **78 → 692** oldu. Bunlar yerel, ısınmış ve küçük örneklemli ölçümlerdir; üretim kapasitesi taahhüdü değildir.

## On fırsatın uygulaması

| No | Değişiklik | Sınır / davranış |
|---|---|---|
| 1 | Geçmiş API'si ve arayüz sayfalaması | Varsayılan 50, API üst sınırı 200, arayüz penceresi en çok 150 geçmiş mesajı; kimlik tabanlı önce/sonra/hedef imleçleri |
| 2 | Değişken yükseklikli mesaj sanallaştırması | Görünür alan + tampon oluşturulur; araç adımları, boyut değişimi, arama hedefi ve önceki sayfa konumu ölçülür |
| 3 | İhtiyaç halinde transkript yükleme | Store başına yaklaşık 16 MiB tutulma bütçesi; kullanımda olan kayıt geçici olarak bütçeyi aşabilir |
| 4 | Tamamlanan hub olaylarını bırakma | Commit sonrasında halka sınırına iner; atılan gövdeler ve büyük arka diziler serbest bırakılır; commit edilmemiş olaylar korunur |
| 5 | Kısaltılmış araç izi önbelleği | İçerik hash'i anahtarı, yaklaşık 4 MiB toplam bütçe; tam ham iz saklanmaz; workspace kimliği çakışması eski içerik döndürmez |
| 6 | Geçmiş yenilemesinde tek sahip | 75 ms birleştirme; devam eden istek sırasında gelen yeni tamamlanma için ek yenileme; eski workspace/oturum yanıtlarını iptal ve sürüm kontrolü |
| 7 | Ertelenmiş ekranlar ve statik dosya sunumu | Büyük ikincil ekranlar gerektiğinde yüklenir; derleme sırasında gzip; hash'li dosyalara immutable cache; ETag, HEAD ve 304 desteği |
| 8 | Küçük kullanım kayıtlarının açılış maliyeti | Dosya boyutu ve değişiklik zamanıyla doğrulanan, atılabilir toplu önbellek; kaynak JSON kayıtları ve maliyet geçmişi korunur |
| 9 | JSONL geçici tahsisleri ve sayaçlar | Satırları ara `[][]byte` kopyası olmadan decode etme; doğrulanmış açılış checkpoint'i; canlı append sırasında yalnız yeni mesajın araç adımlarını sayma |
| 10 | Kullanılmayan eski workspace indeksleri | Kullanım kontrollerinden sonra SHA-256 doğrulamalı ZIP arşivi; toplam **192,33 MiB** net disk kazancı |

Bellek sınırı toplam uygulama belleği sınırı değildir. Aktif bir büyük transkript ilk erişimde hâlâ tamamen decode edilir; işlem sırasında ve çağıranın aldığı mesaj kopyalarında geçici bellek vardır. Arama soğuk oturumları sırayla yükler; tüm arşivi sürekli RAM'de tutmaz. Çok büyük tek oturumlarda diskten gerçek satır aralığı okuma, gelecekte ayrıca ölçülebilecek bir adımdır.

## Çalışırken düzeltilen hatalar

- İlk açılışta mesaj yazma kutusunun yüksekliği ölçülmeyebiliyor, en yeni mesaja dönüş düğmesini örtebiliyordu. Ölçüm, başlangıç yüklemesi ve salt okunur durum değişince de bağlanıyor.
- Önceki sayfa yüklenince okunan mesajın dikey konumu kayabiliyordu. Mesaj kimliği ve gerçek ekran konumu korunuyor; kullanıcı kaydırmaya başladığında sabitleme bırakılıyor.
- Sanal görünür aralık değişince eski sabit soru başlığı kalabiliyordu. Güncel aralıkla yeniden hesaplanıyor ve en üstte temizleniyor.
- Eski geçmiş açıkken yeniden deneme / son turu çalıştırma yanlış görünür pencereye dayanabiliyordu. Gereken kullanıcı mesajı önceki sayfalarda aranıyor; son tur gerçek kuyruktan bulunuyor.
- Geri sarma sayıları, yapılacaklar, katılımcılar ve son yanıt bilgisi yalnız görünür sayfadan türetildiğinde yanlış oluyordu. Konuşma genelindeki küçük metadata ile korunuyor.
- Geç gelen HTTP yanıtı canlı cevapları veya eşzamanlı düzenlemeleri ezebiliyordu. İstek başından sonra gelen canlı değişiklikler birleştiriliyor; silinen mesaj diriltilmiyor.
- Silme / geri sarma / puanlama yazma hatasında bellek diskteki kayıttan ayrışabiliyordu. Başarısız yazma eski bellek ve başlığı geri bırakıyor; arayüz de sunucu başarısından önce geçmişi silmiyor.
- Silinen son mesajın arka dizisi ve geri sarılan mesajların gövdeleri tutulabiliyordu. Kalan mesajlar bağımsız dilime alınır; araç sayacı ve geçersiz özet de düzeltilir.
- Önbellek girişi pin edilmeden arada çıkarılabiliyordu. Giriş kontrolü ve pin aynı kilit kapsamındadır; metadata güncellemesi oturum kilidi bırakılmadan yapılır.
- Crash kurtarma eki, mesaja dönüştürme başarısız olsa bile kaldırılabiliyordu. Başarısız yazmada kurtarma dosyası korunur.
- Ek dosya referansları eksik okunmuşsa eski yüklemelerin temizlenmesi durdurulur.
- Hızlı workspace değişiminde eski artifact yanıtının yeni ekranı güncellemesi engellenir.

## Açılış ve uyumluluk

Gerçek beş store'un ilk yeni açılış toplamı **5,22 sn**; öncesi **5,66 sn** idi. İlk açılışta checkpoint henüz yoktur ve kaynak dosyalar doğrulanır. WS5 kullanım kayıtları bu ilk çalışmada hâlâ yaklaşık **1,38 sn** sürmüştür; burada büyük bir soğuk açılış kazancı iddia edilmiyor.

Tekrar açılışında beş store toplamı **260 ms**, WS5 kullanım kayıtları **39 ms** ölçüldü. Bu fark hem uygulama önbelleğini hem işletim sistemi / antivirüs dosya önbelleğini içerir; tamamı yeni kodun kazancı olarak yorumlanmamalı.

Kaynak `session.json`, `messages.jsonl` ve kullanım JSON düzenleri değişmedi. Yeni `transcripts-cache.json` ve `session-usage-cache.json` dosyaları atılabilir; eski uygulama sürümleri onları yok sayar. Önbellek bozuksa veya kaynak dosya değişmişse yeniden okunur. Boyut ve değişiklik zamanını aynı bırakan harici düzenlemeler bu doğrulamanın sınırıdır. Legacy birleşik oturum dosyası dönüşümü ve WAL kurtarma yolu korunmuştur.

## Geri alınabilir indeks arşivi

Etkin süreç komut satırları, harici MCP yapılandırmaları, workspace MCP kayıtları ve proje yapılandırmaları kontrol edildi. İki eski `cbm-store` için etkin kullanım referansı bulunmadı; yeniden yönlendirme noktası olmadığı ve dosyaların özel erişimle açılabildiği doğrulandı. Her dosya arşiv içinden SHA-256 ile doğrulandı; kaynaklar kaldırılmadan önce tekrar hash alındı.

- WS10: `C:\Users\Bilal\.tionharness\workspaces\WS10\.legacy-archives\cbm-store-2026-09-28.zip` — 6 dosya.
- WS15: `C:\Users\Bilal\.tionharness\workspaces\WS15\.legacy-archives\cbm-store-2026-09-28.zip` — 2 dosya.

Her ZIP yanında SHA-256 manifesti vardır. Geri alma gerektiğinde ilgili ZIP, aynı workspace'in `cbm-store` dizinine açılır ve manifestle doğrulanır. **232,62 MiB kaynak → 40,28 MiB arşiv**, net **192,33 MiB** tasarruf. Sohbet veya kullanım geçmişi silinmedi.

## Doğrulama kaydı

- Zorunlu `scripts/test.sh full` geçidi başarılı, çıkış kodu **0**: tüm Go paketleri, **147 frontend test dosyası / 1.041 test**, depcheck ve `git diff --check` geçti.
- TypeScript ve üretim frontend derlemesi, ayrıca Windows backend derlemesi başarılı.
- Son tarayıcı işlev kontrolü **9/9 başarılı**, konsol / sayfa hatası **0**. Beş ardışık önceki sayfa yüklemede fiziksel konum sapması **0,15625 px**, en çok **150** geçmiş mesajı ve o görünümde **10** bağlı mesaj bileşeni; yinelenen kimlik yok.
- En yeni mesaja gerçek tıklamayla dönüş, arama hedefi ve vurgusu, sabit soruya dönüş, araç adımı açma/kapama, üç pencere boyutu, hızlı oturum değişimi ve statik dosya önbelleği tekrar kullanımı doğrulandı.
- Yeni testler önbellek tahliyesi, sekiz eşzamanlı okuyucu, kararlı sayfa imleçleri, bozulan checkpoint, yarım son JSONL satırı, yazma hatasında geçmişin korunması, sayfalar arası yeniden deneme ve istek sırasında gelen canlı mesajları kapsıyor. Yerel Go ortamında CGO kapalı / C derleyicisi yok; race detector çalıştırılmadı.

Sayısal sonuçlar ve kontrol kanıtları: [optimizasyon ölçüm dosyası](olcumler/kaynak-optimizasyonu-2026-09-28.json). Önceki başarısız kaydırma denemeleri tanı için yerel scratch dizininde tutulur; nihai başarı kanıtı `after-qa-final3-results.json` dosyasıdır.

GPU, gerçek WebView2 masaüstü süreç ağacı, ücretli model akışı ve saatler süren çoklu ajan yükü ölçülmedi. Canlı akış sıralaması otomatik testlerle sınandı; gerçek sağlayıcıya ücretli çağrı yapılmadı. Kısa bellek tekrarları her olası uzun süreli sızıntının yokluğunu kanıtlamaz.

Kaynak ilişkileri graph Tier 2 ve güncel dosya okumalarıyla doğrulandı. Coverage sonuçlarındaki `metadata_changed` durumları doğrudan kaynakla kontrol edildi; elle indeks oluşturulmadı. Ölçümler ayrı uygulama/veri kopyasında yapıldı.

İnceleme sunucusu durduruldu; sentetik veri, gerçek store kopyaları ve test uygulaması kaldırıldı. Sayısal sonuçlar, profiller ve tekrar üretim araçları korundu.
