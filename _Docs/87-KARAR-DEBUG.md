# Karar Mercileri — İnceleme ve Debug

> **Özet (2026-10-01):** Jev bağlantısı canlı olarak doğrulandı; mevcut yerel
> karar defteri faydayı kanıtlamaya yetmiyor. Yeni debug mekanizması bir kararın
> model denemelerini, HTTP tekrarlarını, eşiklerini, olasılıklarını ve uygulanmış
> sonucunu aynı çağrı kimliğinde toplar. Arayüz: **Ayarlar → Karar Mercileri →
> Karar debug**. API: `GET /api/decider/debug`. Ajan aracı: `read_decider_debug`.
> Ham konuşma, komut ve soru metni saklanmaz. Yeni görünüm, yeni sürüm çalıştırıldıktan
> sonraki kararlar için veri toplar; geçmişte kaydedilmemiş ayrıntıları üretemez.

## İnceleme sonucu

`C:\Users\Bilal\.tionharness` altındaki kalıcı defterde inceleme anında dört
kayıt vardı. Çalışan uygulama aynı kayıtları `http://127.0.0.1:8090/api/decider`
üzerinden sundu. Ana anahtar açık, DM1 hazır; sağlayıcı OpenRouter, model
`typesafe/jev-1.13`, deneme başına zaman aşımı 3000 ms.

| Gözlem | Sonuç |
|---|---|
| Saklanan geçmiş | 22 Eylül–1 Ekim 2026, dört `stall-judge` gölge çağrısı |
| Başarılı / hatalı | 3 / 1; hata yalnız `error` sınıfıyla kaydedilmiş |
| Karşılaştırma | Başarılı üç cevap da eski yargıçla `ok` sonucunda aynı |
| Gecikme | Başarılı çağrılar 709, 829 ve 6823 ms; medyan 829 ms |
| Gözlenen maliyet | Dört kaydın bildirilen toplamı $0.00010794 |
| Uygulanmış karar | Bu kayıtların hiçbirinde davranış değiştirilmemiş |
| Diğer merciler | İncelenen defterde tool-risk, flow-judge ve phase-gate kaydı yok |
| Son yedi günlük görünüm | Üç çağrı: iki karşılaştırma, bir hata, p95 6823 ms |

**Sonuç:** Bağlantı ve karar sözleşmesi çalışıyor. Mevcut gözlemler “işe yaradığı
kanıtlandı” sonucunu desteklemiyor. Üç `ok` karşılaştırması olumlu/olumsuz örnek
dengesini, zararlı eylemleri yakalamayı, takılmayı doğru bulmayı veya yanlış
alarm oranını göstermiyor. Defterde başka merci kaydı bulunmaması o yolların
hiç çalışmadığını kanıtlamaz.

Çalışan uygulamanın sabit sağlayıcı testi de başarılı oldu: Jev somut sürümü
`typesafe/jev-1.13-20260917`, 727 ms, 388 girdi token, $0.000016296; force-push
örneği için P(onay gerekir)=0,98 ve risk=2/2. Komut yürütülmedi; yalnız sabit
test metni değerlendirme servisine gönderildi. Bu test yeni debug arayüzünün
dağıtıldığını veya dört merciin gerçek trafikte doğru çalıştığını göstermez.

## JEV araştırmasından çıkan ayrımlar

- Jev metin üretmez; metin/JSON durum üzerinde `noul`, `choice` ve `score`
  döndürür. Görev yürütme, izin verme ve akış denetimi uygulamanın işidir.
  [System One](https://docs.typesafe.ai/concepts/system-one),
  [API](https://docs.typesafe.ai/api).
- `noul` doğrudan P(evet) döndürür. Choice/Score `confidence` değeri dağılımın
  yoğunluğuyla ilgilidir; tek olayın doğru olma garantisi değildir.
  [Confidence](https://docs.typesafe.ai/confidence).
- Güncel OpenRouter modeli `typesafe/jev-1.13`, fiyat $0.042/milyon girdi token,
  çıktı ücretsiz; model sayfası 32K bağlam listeliyor.
  [Model](https://openrouter.ai/typesafe/jev-1.13),
  [Jev rehberi](https://openrouter.ai/docs/guides/community/jev).
- Doğrudan TypeSafe belgeleri toplam istek için 64K, durum + en uzun soru için
  32K sınırını ayrı verir. Sağlayıcıların sınırları aynı kabul edilmemeli.
  Doğrudan API'nin sabit sürüm adı `jev-1.13.0`; projedeki eski çıplak
  `jev-1.13` şablonunun doğrudan TypeSafe kabulü bu oturumda canlı denenmedi.
  [Models](https://docs.typesafe.ai/models).
- Resmî sınırlar arasında adversarial metin, çok dolaylı muhakeme, ilgisiz uzun
  bağlam ve sayısal/tarihsel karşılaştırmalar bulunuyor. Türkçe iş yükü ayrıca
  etiketlenmeli. [Jev sınırları](https://docs.typesafe.ai/model-jaggedness/jev-1.13).
- Yayınlanan hız/kalite sonuçları sağlayıcının ölçümleridir; yerel doğruluk
  deneyi yerine geçmez. Eşikler etiketli bir örneklemden seçilip ayrı bir
  örneklemde doğrulanmalıdır.
  [Lansman değerlendirmesi](https://typesafe.ai/blog/introducing-system-one-models-and-jev),
  [Eşik ayarı örneği](https://openrouter.ai/blog/tutorials/how-to-use-jev/).

## Yeni debug akışı

Her `Hub.Decide` çağrısı tek bir `traceId` alır. Başlangıç, birincil model,
her HTTP denemesi/tekrar beklemesi, gerekirse yedek model, tamamlanma ve
çağıranın uyguladığı/karşılaştırdığı sonuç aynı kimliğe bağlıdır. Rakip model de
aynı kimliği kullanır; rolü ayrı tutulur. Rakip kapasitesi doluysa atlama
görünür. Sağlayıcı testleri gerçek karar sayısından ayrı sayılır.

Kaydedilen bilgiler:

- Merci, o çağrının modu ve eşiği; ayar ve model yapılandırması izi.
- Oturum, tur ve bağımsız akış/rota referansı. Faz kapısı kök oturuma da bağlıdır.
- Deneme rolü, model örneği, istenen model ve servis edilen somut sürüm.
- Model denemesi ve HTTP süresi; HTTP deneme sırası ve tekrar beklemesi.
- Deneme zaman aşımı, bağlam bütçesi, kırpılmadan önce/sonra redakte girdi
  boyutu, soru türü sayıları ve istek SHA-256 izi.
- Tipli cevaplar ve seçenek dağılımları; yerel logprobs sorularının ayrı kimlikleri.
- Sabit hata sınıfları: HTTP durumları, timeout, network, cancelled,
  invalid_request, invalid_response, no_model, no_endpoint ve backoff.
- Uygulanmış sonuç, karşılaştırılan sonuç, maliyet/token ölçümleri, logprobs
  yokluğu gibi güvenilirlik uyarıları.

`debug.jsonl` ve `debug.1.jsonl` dosyaları `<dataDir>/decider/` altındadır.
Bellek son 5000 **olayı** tutar; bu 5000 karar değildir. Her dosya yaklaşık
4 MiB ile döner. İki dosyanın son kayıtları yeniden açılışta yüklenir.
Yazma hatası kararın çalışmasını kesmez. Windows'ta mevcut rotasyon hedefi
kontrollü olarak kaldırılır; dosya büyümesinin sessizce sürmesi önlenir.

Ham istek, soru/ölçüt metni, konuşma, komut, anahtar, uç nokta veya sağlayıcı
hata gövdesi tutulmaz. Kısa model/cevap kimlikleri sınırlandırılır, şüpheli metin
kimlikleri parmak izine çevrilir. Olasılık uyarılarında sağlayıcının serbest
metni yerine sabit sınıf saklanır. Bu nedenle kayıttan tek başına modelin
muhakeme açıklaması veya özgün girdinin yeniden oynatımı çıkarılamaz.

## Kullanım

Karar Mercileri'ndeki debug bölümünde merci, model, 1/7/30/90 günlük aralık ve
tam oturum/koşu/tur kimliğiyle filtrele. Bir kaydı açınca aynı kararın zaman
çizelgesi görünür. “JSON'u indir” saklanan eşleşen olayları dışa aktarır;
hiçbir model çağrısı yapmaz ve karar ayarlarını değiştirmez.

API örnekleri:

```text
GET /api/decider/debug?days=7&authority=stall-judge&ref=SES7&limit=500
GET /api/decider/debug?traceId=<id>&limit=5000
```

Liste en yeni olaydan başlar. Özet, gösterilen ilk 500 kayıt yerine saklanan
tüm filtre eşleşmelerinden hesaplanır. `retainedEvents`, `matchedEvents`,
`oldestAt` ve `truncated` alanları sınırı görünür kılar. Model filtresi yalnız
o modele ait olayları gösterir; bir yedek çağrının diğer modeldeki hatalarını
görmek için model filtresini kaldırıp çağrı kimliğini kullan.

Ajan aracı örnekleri:

```json
{"summary":true}
{"summary":false,"authority":"stall-judge","limit":100}
{"summary":false,"trace_id":"<id>","all_sessions":true}
```

Varsayılan çağrı mevcut oturumla sınırlıdır. Oturum bağlamı yoksa açık `ref`
veya `all_sessions=true` gerekir. Araç salt okunurdur, lazy/name-only olarak
kayıtlıdır; native araç döngüsü ve genel CLI köprüsü üzerinden keşfedilebilir.

## Bulgulara bağlı düzeltmeler

1. System One cevabındaki aralık dışı olasılıklar önceden 0–1 aralığına
   kırpılıyordu. Artık geçersiz olasılık/güven, tanımsız seçenek, ölçek dışı
   puan ve tanımsız dağılım anahtarı reddedilir; uygun hata/yedek yolu çalışır.
2. Takılma süpürücüsünün `context.Background()` çağrıları oturum kimliğini
   kaybediyordu. Koordinatör kimliği yargıç bağlamına aktarılır; senkron
   takılma kaydı da `Ref` taşır.
3. Faz kapısının rota kimliği korunurken kök oturum ayrıca debug'a bağlanır.
4. “Uyum yüksek, açmaya/devralmaya hazır” metinleri bağımsız etiketli kalite
   doğrulaması gerektiğini söyleyecek şekilde düzeltilir.

## Fayda nasıl ölçülmeli?

50 karşılaştırma ürünün mevcut örneklem eşiğidir; istatistiksel veya semantik
doğruluk garantisi değildir. Debug uyarısı bu sayının altında kanıtın zayıf
olduğunu hatırlatır. Daha büyük örneklem de aynı hatalı baseline ile eşleşerek
yanlış kararları doğru yapmaz.

Her merci için bağımsız etiketli olumlu, olumsuz ve belirsiz örnekler gerekir.
`tool-risk` için riskli komutu kaçırma/gereksiz onay; `stall-judge` için
gerçek takılmayı kaçırma/yanlış nudge; `flow-judge` için doğru dal/emin değil
oranı; `phase-gate` için yanlış geçiş/kanıt eksikliğini yakalama ölçülmeli.
P50/P95, timeout, HTTP tekrarları, yedek/rakip maliyeti ayrıca izlenmeli.

Debug verileri işletim kanıtı sağlar. Kalibrasyon için Brier/ECE gibi ölçüler
ve doğrulanmış etiketler; yeni context/model seçicilerinin gerçek faydası için
aynı görevlerde kontrollü yeniden yürütme gerekir. Bu oturum, yeni mercileri
otomatik açmaz veya mevcut eşikleri değiştirmez.

## Doğrulama (2026-10-01)

- `scripts/test.sh full` son çalıştırmada başarılı: tüm Go paketleri,
  170 dosyada 1194 frontend testi, bağımlılık denetimi ve diff biçim kontrolü.
- Frontend üretim derlemesi ve yeni debug dosyalarının ESLint kontrolü başarılı.
- Debug testleri HTTP tekrarı, yedek/rakip rolü, kapasite atlaması, iptal,
  filtre/saklama sınırları, dosya döndürme/yeniden açma ve hassas metin
  saklanmamasını kapsıyor. Takılma yargıcının arka plan çağrısında oturum
  bağlantısı ve aynı mesaj için memo kullanımı ayrıca doğrulandı.
- Önceki tam çalıştırmada mevcut gerçek indeksleme testi 3 dakika sınırını
  aştı; tek başına tekrarında 16 saniyede, son tam çalıştırmada da geçti.
  Bu testin kodu değiştirilmedi.
- İzole deneme sunucusu derlendi. Ayrı IAB ve Chrome QA sekmelerinde yerel
  adres `net::ERR_BLOCKED_BY_CLIENT` ile engellendiğinden görsel tarayıcı
  kontrolü tamamlanamadı. Arayüzün render testleri başarılı; görsel doğrulama
  yapılmış kabul edilmemeli. Deneme sunucusu sonrasında durduruldu.
- Çalışan uygulamanın sağlayıcı bağlantısı önceki sürümde canlı doğrulandı.
  Yeni backend/debug sürümü çalışan uygulamaya bu oturumda dağıtılmadı.
