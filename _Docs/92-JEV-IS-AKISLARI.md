# JEV iş akışları

**Özet — Uygulandı, 2026-10-02.** Mevcut karar Hub'ına altı merci ve oturum karar
kayıtları bağlandı. JEV tipli seçim yapar; metin üretimi, özetleme ve worker
sonuçlarının sentezi çalıştırma modelinin işidir.
İlgili oturum bağlamı karar girdilerine eklenir; Bütçe ekranındaki ayrı bölüm
JEV/OpenRouter karar tüketimini kalıcı günlük toplamlarla gösterir.

## Kullanım

1. Ayarlar → Karar Sağlayıcıları'ndan kullanılabilir bir sağlayıcı yapılandır.
2. Karar Mercileri'nde ana anahtarı aç, soldan bir iş akışı seç.
3. Kapalı, Gölge (gözlem) veya Açık (aktif) modunu seçip kaydet. Yeni iş akışlarının varsayılanı
   Kapalı'dır; mevcut oturumların davranışı güncelleme sırasında kendiliğinden değişmez.
4. Oturum ayrıntılarındaki **Oturum kararları** bölümünde öneri, uygulama, model,
   skill/araçlar ve korunan bağlamı incele. Modeli veya kayıtlı bağlamı sabitle;
   karara Faydalı / Düzeltilmeli geri bildirimi ver.
5. Debug'daki merci, oturum ve iz kimliklerini kullanarak sağlayıcı denemelerine,
   yedek modele, olasılıklara, süreye ve maliyete git.

Oturumdaki Debug aç düğmesi **Karar kayıtları** sekmesini açar. Burada oturum ve
workspace filtresi sabittir; aynı SES kimliğini kullanan farklı workspace'ler
karışmaz. Genel Karar Mercileri debug görünümü bütün workspace'leri inceleyebilir.
Yeni workspace alanı olmayan eski debug kayıtları yalnız genel görünümde kalır.

## Gerçek çalışma noktaları

| Merci | Tetikleyici | Aktif modda uygulanan sonuç |
|---|---|---|
| session-setup | Oturumun ilk uygun çalıştırma turu | İzinli skill ve araçlar aynı çok-sorulu çağrıda seçilir. Bütçeye sığan tam skill talimatları sabit bir oturum kopyası olarak yüklenir. Native döngüde araç şemaları, Claude CLI'de interaction gateway araçları etkinleştirilir. |
| model-router | İlk tur, provider/prompt hazırlığından önce | Yapılandırılmış ve kullanılabilir model adaylarından güven eşiğini geçen seçim oturuma kaydedilir. Sonraki turlar rotayı kullanır; kullanıcı pini önceliklidir. |
| clarification | ask_user / ask_user_async | Açıkça isteğe bağlı belirtilmiş ve cevabı konuşmada bulunan soruda devam önerilir. Zorunlu bilgi ve eylem onayları kullanıcıya gider. |
| compact-retention | Yönetilen rolling, manual veya reactive compact | Eski parçalar koru/özetle/çıkar olarak sınıflanır. Çıkar seçimi yalnız özetleme girdisinden çıkarır; kanonik transkript silinmez. Korunan parçalar özete eklenir, pinler sonraki turlara taşınır. |
| context-reminder | Kayıtlı parçanın tutulması veya son hatırlatılması üzerinden N başarılı compact sonrası | Mevcut göreve gerekli parçalar seçilip yeni tur bağlamına eklenir. Başarılı CLI compact işlemleri de sayılır. |
| worker-review | Terminal worker sonucunun koordinatöre teslimi | Doğrulama, alternatif karşılaştırma, varsayım sorgulama, çelişki çözme ve sonraki adımlar içinden en fazla üç bakış koordinatör notuna eklenir. Sonuç teslimi karar modelinin başarısına bağlı değildir. |

## Modlar ve sınırlar

- **Kapalı:** karar modeline çağrı yapılmaz. Kullanıcının elle sabitlediği bağlam
  ve model tercihleri korunur.
- **Gölge / gözlem:** çağrı arka plandadır; mevcut akışı değiştirmez. Öneri ve önceki
  davranış ayrı kaydedilir. Gözlemden Aktif'e geçiş gerçek başlangıç hazırlığını
  engellemez.
- **Açık / aktif:** yalnız izin verilen adaylar, güven eşiği ve yükleme bütçesi içinde
  uygulanır. Karar çağrısı en fazla altı saniye bekletir; sağlayıcı hatasında
  mevcut davranış devam eder.
- Aday sınırı 4–48, seçim sınırı 1–16, hatırlatma aralığı 1–20 compact ve bağlam
  bütçesi 1–32 KiB arasında ayarlanır. Worker incelemesi en fazla üç bakış yükler.
- Hatırlatma sayacı yalnız gerçekten yeni tur bağlamına eklenen parçalar için
  ilerler. Kaynağı bulunmayan veya bütçeye sığmayan seçim uygulama sayılmaz.
- Skill yükleme yeni araç izni vermez. Araç/skill izinleri her tur yeniden kontrol
  edilir. Gizli araçlar başlangıç seçim havuzuna girmez.
- Router native sağlayıcılar arasında aynı aktarım türünü korur. CLI'de aynı
  sağlayıcı örneğinin modelleri arasında seçim yapar; mevcut resume kimliğiyle
  başka sağlayıcı örneğine geçmez. İlk turdan sonraki rota değişimi otomatik değildir.
- Claude CLI'nin köprü dışı MCP araçları kendi keşif mekanizmasını kullanır.
  Codex CLI mevcut tam araç kataloğunu kullanır; başlangıç seçimi bu katalog için
  ek bir şema yükleme gerektirmez.
- CLI'nin kendi iç özetleme girdisi dışarıdan yeniden yazılmaz; compact-retention
  yönetilen özetleyiciye bağlıdır. CLI compact sonrası hatırlatmalar sonraki
  TionHarness turunda uygulanır.

## Kayıt ve API

### Karar girdisindeki ilgili bağlam

2026-10-02: skill/araç seçimi, model yönlendirme, soru inceleme, hatırlatma ve
worker inceleme kararlarına oturum amacı, mevcut özet, son kullanıcı/assistant
konuşması ve sabitlenen bağlam eklenir. İlk kullanıcı isteği 2 KiB, özet 4 KiB,
son 16 mesajdan konuşma metni toplam 6 KiB ve sabitlenen kaynak metinleri toplam
2 KiB ile sınırlıdır. Son konuşma kronolojik verilir; yeni kullanıcı düzeltmeleri
önceliklidir. Araç çıktıları bu konuşma kanıtına alınmaz.

Hatırlatmalarda yalnız kısa etiket yerine kanonik mesaj kimliğinden çözülen asıl
metin de verilir (toplam 12 KiB). Kaybolan bir kanonik kaynağın eski kopyası
kullanılmaz. Compact kararında az sayıda parçaya 4 KiB'ye kadar metin verilebilir;
tüm parça metinleri toplam 24 KiB ile sınırlandırılır. Mevcut özet 6 KiB'ye kadar
verilir; zorunlu/sabitlenmiş işaretleri ayrı alanlardır.

Bu üst sınırlar her çağrıda tamamen doldurulmaz. Seçili ana, yedek ve karşılaştırma
modellerinin en küçük giriş penceresine göre soru boyutu ve aktarım payı düşülür;
gerekirse yalnız kanıt metni daha da kısaltılır. Aday kimlikleri, roller, soru
şemaları ve karar talimatları korunur. Kısaltma `evidenceTruncated` ile belirtilir;
Hub'ın son boyut ve redaksiyon kontrolü yürürlükte kalır. Gerçek doğruluk artışı
bu eklemeden kendiliğinden çıkarılmaz; oturum geri bildirimi ve etiketli örneklerle
ölçülmelidir. [JEV belgeleri](https://docs.typesafe.ai/model-jaggedness/jev-1.13)
de ilgisiz büyük durumların bağlam kaybına yol açabildiğini belirtir.

### Bütçe ekranındaki karar harcaması

Bütçe → **JEV / karar modeli harcaması**, tüm çalışma alanlarının karar çağrılarını
gösterir. Bugün ve seçili 7/30/90 gün için USD, girdi/çıktı token, çağrı ve
sağlayıcı/model kırılımı vardır. Küçük pozitif tutarlar sekiz ondalığa kadar
gösterilir; daha küçük tutarlar sıfır yerine bir alt sınır işaretiyle gösterilir.
Ana/yedek, karşılaştırma ve sağlayıcı testi çağrıları ayrı sayılır.

OpenRouter'ın döndürdüğü `usage.cost` önceliklidir; açıkça bildirilen sıfır da
korunur. Fiyat verilmezse bilinen modelin token tarifesiyle **tahmini** maliyet
hesaplanır. Fiyatı bilinmeyen çağrı ücretsiz kabul edilmez. Kararı geçersiz olsa
bile sağlayıcı token/ücret döndürdüyse tüketim kaydedilir; geçersiz cevap bir
eylemi onaylamak için kullanılamaz. Bağlantı hatasında sağlayıcının bildirmediği
bir ücreti uygulama belirleyemez. [OpenRouter JEV belgesi](https://openrouter.ai/docs/guides/community/jev)
maliyet alanını ve aynı OpenRouter hesabından faturalandırmayı açıklar.

Günlük toplamlar `decider/spend.json` içinde atomik kaydedilir, 365 gün tutulur;
rapor günleri UTC'ye göredir. Debug/karar geçmişinin rotasyonu maliyet geçmişini
silmez. Ekran takip başlangıcını gösterir; önceki çağrılar için geriye dönük
eksiksiz harcama iddiası yoktur. Okuma/yazma hatasında toplamların eksik olabileceği
belirtilir; bozuk dosya sessizce üzerine yazılmaz. Bu bölüm mevcut workspace/ajan
toplamına yeniden eklenmez; aynı çağrı ajan kullanımında da bulunabilir. Hesap
kredi bakiyesi veya sağlayıcı faturası yerine uygulamanın kaydettiği tüketimdir.

API: `GET /api/usage?days=7|30|90` yanıtındaki `decisionSpend` alanı; mevcut kullanım
toplamlarının sözleşmesi ve hesabı korunur.

Her oturumda atomik decisions.json sidecar'ı bulunur. En fazla 128 karar,
128 geri bildirim ve 32 bağlam parçası tutulur; bellek metni toplam 32 KiB ile
sınırlıdır. DB mesajları kimlikleriyle referans edilir. Geçici reactive parçalar
sınırlı içerikle saklanır. Skill talimatlarının yüklenen kopyaları ayrıca
32 KiB bütçeye tabidir.

- GET /api/sessions/{id}/decisions
- PUT /api/sessions/{id}/decisions/pin — kayıtlı bağlam anahtarı veya model
- POST /api/sessions/{id}/decisions/feedback — karar kimliği, helpful/correction

API workspace kapsamlıdır. Bilinmeyen oturum/parça/karar kabul edilmez; zorunlu
bağlamın sabitlemesi kaldırılamaz. Inspector API'si yüklenen skill gövdelerini ve
geçici bağlamın tam metnini dönmez. Debug/ledger ham karar girdisi tutmaz; içerik
gerekiyorsa kanonik oturum transkripti kullanılır.

Öneri ile uygulama aynı alan değildir. Düşük güven, bütçe ve katalog değişiklikleri
fark yaratabilir. İz kimliği mevcut karar debug kaydına bağlanır. Faydalı/
Düzeltilmeli geri bildirimi kullanıcı değerlendirmesidir; model doğruluğu veya
task başarısı için bağımsız etiketli ölçümün yerine geçmez. Yeni iş akışlarında
yalnız davranış uyumundan otomatik “aktif moda geç” önerisi üretilmez.

## Doğrulama ve sonraki fikirler

Sahte sağlayıcılarla gerçek başlangıç şeması yükleme, shadow izolasyonu,
model/pin/resume, managed compact girdisi/çıktısı, reminder aralığı, clarification
ve worker teslimi test edilir. API pin/feedback ve workspace kapsamı; UI eski
session yanıtları, çift tıklama, yeniden yükleme ve gözlem/uygulama ayrımı
regresyon testleriyle doğrulanır. Teslim kapısı scripts/test.sh full'dur.

2026-10-01 teslim doğrulaması: tam test kapısı geçti; bütün Go paketleri,
180 dosyada 1.231 arayüz testi, bağımlılık yönü ve diff kontrolü başarılı.
Production arayüz derlemesi de geçti. İzole uygulama üzerinde ayarların yeniden
yüklenmesi, model/bağlam pinleri, geri bildirim kalıcılığı, karar sekmeleri,
workspace/oturum filtreli JSON indirme ve 1024/736/360 px görünümler doğrulandı.
Console veya HTTP hatası görülmedi. Tarayıcı kayıtları sentetik örneklerdir;
gerçek JEV trafiğinde görev başarısı ve maliyet kazanımı bu testlerden çıkarılmaz.

2026-10-02 ek doğrulaması: tam test kapısı yeniden geçti; bütün Go paketleri ve
181 dosyada 1.238 arayüz testi başarılı. Derleme, değişen arayüz dosyalarının lint
kontrolü, bağımlılık yönü ve diff kontrolü geçti. Gerçek HTTP decoder/Hub üzerinden
ana, ücretli-geçersiz cevap, yedek, arka plan karşılaştırma ve sağlayıcı testi
muhasebesi doğrulandı. Ek bağlamın UTF-8 sınırları, son düzeltme sırası, kanonik
pin/hatırlatma metni ve küçük model penceresine sığdırma test edildi. İzole bütçe
UI/API'sinde 7/30/90 gün, TR/EN, 1024/360 px, mikro maliyet, yeniden açılış ve bozuk
geçmiş uyarısı doğrulandı; console/HTTP hatası ve taşma görülmedi. Harcama ekranı
kontrolü sentetik sayısal kayıtlarla yapıldı; gerçek sağlayıcı ücreti oluşturulmadı.

Araştırma ve yaklaşık yirmi kullanım fikri [87-JEV-FIKIRLERI.md](87-JEV-FIKIRLERI.md)
içindedir. Bu sürüm kullanıcının yedi çekirdek örneğini altı çalışma noktasıyla
uygular; diğer fikirler bu dosyada uygulanmış özellik olarak sunulmaz.
