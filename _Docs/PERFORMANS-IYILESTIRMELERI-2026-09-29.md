# Performans iyileştirmeleri — 29 Eylül 2026

> **Özet:** Sürüm sorgularına önbellek ve eşzamanlı istek birleştirme eklendi. Sohbetin ağır panelleri ihtiyaç anında yükleniyor. Activity ilk yükündeki çift istek giderildi. Çalışma durumu, kalıcı verinin önbelleklenmiş özeti ile canlı çalışma kayıtları birleştirilerek üretiliyor. MCP devre kesicisinin eşzamanlı katalog çağrılarındaki yarış aralığı kapatıldı.

## Yapılan değişiklikler

### Sürüm sorguları

- Yerel araç sürümü başarılı sonuçları beş dakika saklanıyor. Anahtar araç yolu, dosya boyutu/değişiklik zamanı ve sürüm argümanlarını kapsıyor.
- Aynı araç için eşzamanlı sorgular tek sürüm alt sürecini paylaşıyor. GitHub release sorguları da aynı depo ve sürüm kanalı için birleştiriliyor; mevcut altı saatlik release önbelleği korunuyor.
- Bir HTTP isteğinin iptali diğer bekleyenleri iptal etmiyor. Ortak iş kendi zaman aşımıyla sınırlı; her bekleyen kendi isteğini hemen iptal edebiliyor.
- Başarısız sürüm sonuçları kalıcılaştırılmıyor. Güncelleme girişimi ve açıkça zorlanan yenileme yerel önbelleği temizliyor. Güncellemeden önce başlayan bir sorgu yeni önbelleğin üstüne eski sürümü yazamıyor.
- Sıcak önbellek okuması yeni goroutine veya alt süreç oluşturmuyor.

İlgili dosyalar: `internal/exttools/version_cache.go`, `shared_requests.go`, `version.go`, `release.go`, `update.go`.

### Sohbet ve modallar

`ChatView`, `SessionDetailPanel`, `CoordinatorPanel`, `SessionContextModal`, `SessionDebugModal` ve `ArtifactPreviewModal` ayrı dinamik parçalar olarak yükleniyor. Yan panel/modallara ayrı Suspense sınırları eklendi; bir panelin yüklenmesi tüm uygulama kabuğunu saklamıyor.

| Üretim çıktısı | Önce | Sonra | Azalma |
|---|---:|---:|---:|
| Ana JavaScript parçası | 707.044 B | 215.851 B | %69,5 |
| HTML giriş + statik ön yükleme listesindeki JS | 1.566.126 B | 1.073.123 B | %31,5 |
| Aynı listenin yerel gzip toplamı | 488.580 B | 335.831 B | %31,3 |

Bu karşılaştırma CSS, fontlar ve sonradan açılan sayfa parçalarını içermez; tarayıcı ağ aktarım ölçümü değildir. Sohbet açıldığında kendi parçası ayrıca yüklenir. Markdown/renklendirme ortak bağımlılıklarının tümü ilk girişten kaldırılmadı.

İlgili dosyalar: `frontend/src/app/lazyChatPanels.ts`, `frontend/src/app/App.tsx`.

### Activity çift yükü

İlk yük ve workspace değişimi veri hook'una bırakıldı. Yeni `useSignalRefresh`, yalnız aynı workspace içinde gerçek sinyal değiştiğinde ek yenileme yapıyor. Sinyal sayacının ilk render öncesinde sıfırdan büyük olması ikinci bir isteğe yol açmıyor. Canlı activity/workspace sinyalleri çalışmaya devam ediyor.

İlgili dosyalar: `frontend/src/shared/hooks/useSignalRefresh.ts`, `frontend/src/app/useActivity.ts`, `useWorkspaceActivity.ts`.

### Çalışma durumu sorgusu

- Kalıcı durum/soy bağı bilgisi DB değişiklik kuşağına göre önbellekleniyor; değişmeyen geçmiş her sorguda yeniden dolaşılmıyor.
- Akış oturumunun bağlı olduğu koşu doğrudan kimlikle okunuyor. Yalnız eski veya koşusu silinmiş kayıtlar için en yeni koşu indeksi gerekiyor; burada bütün geçmişin sıralanması kaldırıldı.
- Kalıcı anlamlı durumu olmayan boşta sohbetler özete alınmıyor. Aktif sohbetler canlı kayıtlardan her HTTP isteğinde ekleniyor; yalnız önbelleğe bakıldığı için başlangıç/bitiş rozetleri bayatlamıyor.
- DB değiştiğinde özet yeniden oluşturulur. Bu, tüm yazmalardan bağımsız artımlı bir indeks değildir; kapsamı büyütmeden tekrar eden boşta taramayı kaldırır.

Sıcak okuma mikro ölçümü, tek anlamlı satır içeren iki bellek içi veri setinde:

| Toplam oturum | Süre/işlem | Bellek/işlem |
|---|---:|---:|
| 100 | 32,13 ns | 64 B / 1 tahsis |
| 10.000 | 35,43 ns | 64 B / 1 tahsis |

Bu bir HTTP gecikme ölçümü değildir. Önbellekli veri okumasının maliyetinin toplam boşta geçmişle büyümediğini gösterir; daha çok anlamlı satır döndürüldüğünde kopyalama/JSON maliyeti artar.

İlgili dosyalar: `internal/db/store_runtime_sessions.go`, `internal/api/executions_runtime.go`.

### MCP eşzamanlılığı

Mevcut devre kesici kontrolü havuz bağlantı kilidinin dışındaydı. Birden çok katalog isteği ilk başarısızlık kaydedilmeden kontrolü geçip bağlantı kilidinde sıraya girebiliyordu. Workspace başına, iptal edilebilir bir katalog geçidi kontrol/bağlantı/sonuç kaydını birlikte koruyor. Böylece sıradaki katalog isteği ilk hatadan sonra açılmış bekleme süresini görüyor.

Önbellekli/no-dial görünümler bu geçidi beklemiyor. Açık kullanıcı/ajan talebiyle yapılan MCP bekleme aracının mevcut özel davranışı değiştirilmedi. Workspace'ler birbirlerinin katalog geçidini paylaşmıyor.

Testler: sekiz eşzamanlı başarısız katalog çağrısı tek bağlantı denemesi ve tek hata serisi artışı üretiyor; sekiz başarılı çağrı tek bağlantıyı yeniden kullanıyor. Bekleyen çağrının iptali ve bağlantı sürerken önbellekli önizlemenin açılması ayrıca doğrulandı.

İlgili dosyalar: `internal/agent/mcp_catalog_gate.go`, `runtime.go`, `toolsetup.go`.

## Doğrulama

- Hedefli Go testleri: sürüm paylaşımı, TTL, dosya değişimi, hata sonrası tekrar, iptal, güncelleme sırasında eski sonucun yazılmaması; GitHub sorgu birleştirme; durum/soy bağı eşdeğerliği, silme/güncelleme sonrası geçersiz kılma; canlı durumun store yazılmadan değişmesi; MCP eşzamanlılığı.
- Frontend regresyonları: ilk açılış/workspace değişimi başına bir istek ve gerçek sinyallerde yenileme.
- Üretim frontend ve Windows Go uygulaması derlendi: `bin/tionharness-optimized.exe`.
- Hedefli ESLint: sıfır hata; App'te değiştirilmeyen iki mevcut effect uyarısı var.
- Üretim frontend önizlemesinde altı geç yüklenen bileşen açılıp kapatıldı; konsolda hata/uyarı görülmedi. Bağlam penceresinin kabuğu açıldı, fakat içerik eski çalışan backend üzerinde beklemede kaldı; bu ekranın veri yüklemesi uçtan uca doğrulanmış sayılmıyor.
- Bir tam test turunda değiştirilmeyen `internal/codexauth/TestFlowSuccessRequiresAuthFile` testi zamanlama nedeniyle başarısız oldu; izole on tekrarda geçti. Sahte süreç bekleyicisi dosya yazımıyla tamamlanma işareti üzerinden eşleşmek yerine ayrı zamanlayıcı kullanıyor. Bu kapsamda değiştirilmedi.
- Son tam test geçidi: `GOFLAGS=-p=4` ile `scripts/test.sh full` başarıyla tamamlandı; tüm Go paketleri, 158 dosyada 1.075 frontend testi, bağımlılık kontrolü ve `git diff --check` geçti.

Mevcut Araçlar/MCP çalışma ağacı değişiklikleri korundu; commit oluşturulmadı. Geçici frontend önizleme süreci kapatıldı. Önceden başlatılmış `8090` süreci eski binary ile çalışmaya devam ediyor; backend değişikliklerinin etkinleşmesi için yeni binary ile yeniden başlatılmalı.
