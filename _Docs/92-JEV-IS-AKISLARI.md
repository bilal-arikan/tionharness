# JEV iş akışları

**Özet — Uygulandı, 2026-10-01.** Mevcut karar Hub'ına altı merci ve oturum karar
kayıtları bağlandı. JEV tipli seçim yapar; metin üretimi, özetleme ve worker
sonuçlarının sentezi çalıştırma modelinin işidir.

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

Araştırma ve yaklaşık yirmi kullanım fikri [87-JEV-FIKIRLERI.md](87-JEV-FIKIRLERI.md)
içindedir. Bu sürüm kullanıcının yedi çekirdek örneğini altı çalışma noktasıyla
uygular; diğer fikirler bu dosyada uygulanmış özellik olarak sunulmaz.
