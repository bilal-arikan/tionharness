# TionHarness performans incelemesi — 29 Eylül 2026

> **Özet:** Mevcut veriyle Go API tarafında genel bir darboğaz görülmedi. En somut hafifletme fırsatları günlük kullanımda Vite'ı kaldırmak, önerilerdeki harici araç sürüm sorgularını önbelleğe almak ve ilk açılış JavaScript yükünü azaltmak. Yinelenen/gizli pencere sorguları küçük ama kolay kazanımlar; tüm geçmişi tarayan çalışma durumu sorgusu ise büyüme riski. Bu çalışma ürün kodunu değiştirmez.

## Kapsam ve yöntem

- Windows 11 üzerinde mevcut çalışma ağacından Go sunucusu derlendi; `127.0.0.1:8090` üzerinde başlatıldı. Vite `127.0.0.1:5173` üzerinde çalıştırıldı.
- İnceleme başlangıcında var olan Araçlar/MCP değişiklikleri korundu. Ölçümler bu çalışma ağacını kapsar; temiz HEAD ölçümü değildir.
- Üç mevcut workspace kullanıldı: WS1, WS5, WS30. Oturum listeleme API'sinin bildirdiği toplamlar sırasıyla 92, 44 ve 3.
- Sekiz okuma uç noktası, her workspace için altışar kez çağrıldı: toplam 144 istek. Tablodaki medyan ilk istek dışındaki beş sıcak istektir. Bu küçük seri bir gecikme örneklemesidir; yük testi veya p95 ölçümü değildir.
- Süreler aynı Windows makinesinde HttpClient ile, yanıt gövdesinin okunması dahil ölçüldü. İlk istekte bağlantı/istemci ısınma maliyeti bulunabilir.
- Go CPU profili 30 saniye alındı; pencere tarayıcı incelemesiyle çakıştığı için saf boşta ölçüm değildir. 360 ms CPU örneği toplandı; bu makinenin toplam CPU yüzdesi değildir.
- Go heap örneklemesi yaklaşık 24,4 MiB canlı tahsis gösterdi. Heap, süreç çalışma kümesi ve özel bellek farklı metriklerdir.
- Üretim frontend derlemesi başarılı. Tarayıcı kontrol aracı ağ ve Performance API ölçümlerini açmadığından tarayıcı yükleme süresi, aktarılan veri ve uzun görev süreleri ölçülmüş gibi sunulmadı.
- Kod grafiği ilk kontrolde `2026-09-29T00:13:34Z`, son MCP kontrolünde `2026-09-29T18:11:44Z` kuşağındaydı; incelenen dosyaların kapsama kontrolü `metadata_changed` döndürdü. Maddi bulgular güncel kaynak parçalarıyla doğrulandı; bu inceleme yeniden indeksleme başlatmadı.

## Ölçümler

| Ölçüm | Sonuç |
|---|---:|
| Go sunucusu çalışma kümesi, iki örnek | 70,9 → 74,5 MiB |
| Vite çalışma kümesi, iki örnek | 611,4 → 758,4 MiB |
| Vite özel bellek, iki örnek | 2,04 → 2,19 GiB |
| Go özel bellek, iki örnek | 98,4 → 99,7 MiB |
| Activity API sıcak medyanları | 0,19–0,40 ms |
| Workspace activity sıcak medyanları | 0,29–0,42 ms |
| Execution runtime sıcak medyanları | 0,23–0,41 ms |
| Oturum listesi sıcak medyanları | 0,25–1,19 ms |
| Dashboard sıcak medyanları | 0,51–2,85 ms |
| WS5 dashboard ilk örneği | 37,31 ms |
| Usage sıcak medyanları | 0,41–0,73 ms |
| Process listesi sıcak medyanları | 0,24–0,89 ms |
| MCP pool istatistiği sıcak medyanları | 0,17–0,43 ms |

Tüm 144 ölçüm HTTP 200 döndü. Bu örneklerde aktif ajan çalıştırılmadı; yoğun üretim, büyük akış grafikleri ve çok pencereli uzun oturum yükü test edilmedi. Vite belleğindeki iki örnek arasındaki artış, ziyaret edilen modüllerin derlenmesiyle birlikte gerçekleşti; tek başına bellek sızıntısı kanıtı değildir. İlgisiz Node süreçleri uygulamanın maliyetine eklenmedi.

## Öncelikli hafifletme listesi

### 1. Günlük kullanımda geliştirme sunucusunu kaldır — yüksek, doğrudan ölçülen kazanç

Vite tek başına 611–758 MiB çalışma kümesi tüketti. Go sunucusu gömülü üretim arayüzünü zaten sunabiliyor; günlük kullanımda ikinci bir Node/Vite süreci gerekli değil. Geliştirme sırasında sıcak yenileme için Vite yararlı.

**Öneri:** Normal kullanım için derlenmiş tek uygulama; geliştirme modu ayrı giriş olarak kalsın. Bu ölçümde kaldırılabilecek süreç çalışma kümesi 611–758 MiB. Bu, işletim sisteminin hemen aynı miktarda boş RAM raporlayacağı veya tarayıcının bu kadar küçüleceği anlamına gelmez.

**Kanıt:** `frontend/vite.config.ts:83` üretim çıktısını Go'nun gömdüğü dizine yönlendiriyor; `README.md:24` tek uygulama derlemesini açıklıyor. Süreç örnekleri `_devlogs` altındaki ölçüm çıktılarında.

### 2. Öneriler için tekrarlanan harici araç sürüm sorguları — yüksek öncelik

Başlangıç/gözlem penceresinde `/api/external-tools/check-updates` sunucu kayıtları **4,428 s**, **2,802 s**, **440 ms** gösterdi. Bunlar tüm sayfanın yüklenme süresi değildir.

`fetchRecommendationData` yedi isteği `Promise.all` ile bekliyor; dolayısıyla yavaş güncelleme kontrolü öneri sonuçlarının tamamını bekletiyor. Sunucu GitHub sürüm bilgisini önbellekten alabilse de her çağrıda yerel araçlar için yeniden `LocalVersion` çalıştırıyor. Bu fonksiyon alt süreç başlatıyor; araç başına üç saniyelik zaman aşımı var. Paralel çağrılar için ortak devam eden işlem veya yerel sürüm önbelleği bu yolda yok.

**Öneri:** Yerel sürümü araç yolu/değişiklik zamanı ve süre sınırıyla önbellekle; güncelleme/kurulumdan sonra geçersiz kıl. Eşzamanlı istekleri tek sorguda birleştir. Yerel önerileri önce göster, güncelleme önerisini sonra ekle. Ağ bağımlı kontrolün tüm önerileri bekletmesini kaldır.

**Kanıt:** `frontend/src/features/workspace/recommendations.ts:394`, `internal/api/external_tools.go:96`, `internal/exttools/version.go:41`, `_devlogs/perf-audit-backend.log:19`.

### 3. İlk açılış JavaScript paketi hâlâ büyük — orta/yüksek öncelik

Üretim çıktısında ana giriş **707.044 bayt**. HTML'deki ana betik ve statik modulepreload listesi toplam **1.566.126 bayt JavaScript**; aynı dosyaların yerel gzip hesabı yaklaşık **488.580 bayt**. Bu sayı CSS, font, API yanıtları ve sonradan açılan sayfa parçalarını içermez; tarayıcıda ölçülmüş ağ aktarımı değildir. Brotli kullanılırsa farklı olur.

Sohbet bileşeni, oturum detayları ve bazı modallar App içinde doğrudan içe aktarılıyor. Markdown ve sözdizimi renklendirme paketleri başka sayfaya doğrudan girişte de ön yükleniyor. Üretim derlemesi 600 kB üstü parçalar için uyarı verdi.

**Öneri:** Önce seyrek açılan sohbet modallarını/detaylarını talep anında yükle. Ardından sohbet dışı girişlerde sohbet/Markdown/renklendirme bağımlılıklarını ayırmayı ölç. Kazanç değerlendirmesinde yalnız dosya sayısını değil ilk açılışta zorunlu yüklenen toplamı kullan.

**Kanıt:** `frontend/src/app/App.tsx:55`, `frontend/src/app/App.tsx:61`, `internal/web/dist/index.html:56`, `frontend/src/shared/components/markdown/CodeBlock.tsx:2`.

### 4. Activity sorguları ilk açılışta iki kez çalışıyor — düşük maliyetli düzeltme

`useAsync` kendi mount etkisinde `run()` çağırıyor. `useActivity` ve `useWorkspaceActivity` ayrıca sinyal etkisinde koşulsuz `refresh()` çağırıyor; ilk mount/workspace değişiminde aynı veri tekrar isteniyor. Bu mekanizma üretim kodunda da var; geliştirme StrictMode etkileri ek tekrar oluşturabilir.

Ortak `useAsync`, eski yanıtın ekrana yazılmasını engelliyor fakat ağ isteğini iptal etmiyor ve eşzamanlı aynı isteği birleştirmiyor. Activity uç noktaları mevcut veride çok ucuz olduğu için bu bulguyu büyük CPU darboğazı olarak değerlendirmemek gerekir.

**Öneri:** İlk yükü tek etkiye bırak; sinyal yenilemesini gerçek sinyal değişimine bağla. Gerekli yerlerde istek birleştirme/iptal desteği ekle. Sinyal değeri workspace değişiminde sıfır olmayabileceğinden yalnız `tick === 0` kontrolüne güvenme.

**Kanıt:** `frontend/src/shared/hooks/useAsync.ts:83`, `frontend/src/app/useActivity.ts:34`, `frontend/src/app/useWorkspaceActivity.ts:44`.

### 5. Gizli pencerede kalan yenilemeler — küçük/orta, kullanım koşuluna bağlı

Ortak hook zamanlayıcıyı gizli pencerede durduruyor, ancak dışarıya verdiği `refresh` doğrudan `run`; SSE sinyallerinden gelen yenileme görünürlük kontrolünü atlıyor. Ayrıca LogsPanel takip modunda 30 saniyelik, çalışan/sıcak CLI oturumu detayında 3 saniyelik, etkin İçgörü taramasında 4 saniyelik döngüler doğrudan `setInterval` kullanıyor.

Bu sayfalar kapatıldığında etkileri temizleniyor. Sorun her sayfanın her zaman çalışması değil, ilgili sayfa açıkken tarayıcı penceresinin gizlenmesi. Tarayıcı zamanlayıcı yavaşlatması kesin durdurma garantisi değildir.

**Öneri:** Gizliyken sinyali kirli durum olarak biriktir; görünür olunca tek yenileme yap. Ekrana özel veri döngülerinde mevcut `useVisiblePoll` davranışını kullan. Çalışan işin sunucu tarafındaki takibini kaldırma.

**Kanıt:** `frontend/src/shared/hooks/useAsync.ts:122`, `frontend/src/features/logs/LogsPanel.tsx:205`, `frontend/src/features/sessions/SessionDetailPanel.tsx:160`, `frontend/src/features/insight/InsightPanel.tsx:105`.

### 6. Kompakt çalışma durumu yanıtı arka tarafta tüm geçmişi tarıyor — büyüme riski

`/api/executions/runtime` her çağrıda tüm oturumları alıyor; akış oturumu varsa tüm flow run geçmişini listeliyor ve durum haritaları kuruyor. `ListFlowRuns` sıralamayı genel store okuma kilidi altında yapıyor. Yanıtın küçük olması hesaplamanın yalnız aktif işler kadar olduğu anlamına gelmiyor. App bu veriyi sohbet dışı sayfalarda da 20 saniyede bir istiyor.

**Mevcut etki:** Üç workspace'te sıcak medyan 0,23–0,41 ms; şimdi büyük bir darboğaz değil. Maliyet oturum/akış geçmişi büyüdükçe ve eşzamanlı yazmalar arttıkça önem kazanabilir. Kilit beklemesi bu incelemede kanıtlanmadı.

**Öneri:** Önce büyük geçmişle gerçekçi örnek ölçümü; ardından durum haritalarını ilgili değişiklik kuşağına göre önbellekle veya görünür oturumlar + aktif işler için daralt. Flow run sıralamasında veri kopyası üzerinde kilit dışında çalışmayı değerlendir. Soy bağı/durum rozetlerinin doğruluğunu koru.

**Kanıt:** `frontend/src/app/App.tsx:320`, `frontend/src/app/useExecutionRuntime.ts:8`, `internal/api/executions_runtime.go:30`, `internal/db/store_flow.go:403`, `internal/db/filestore.go:54`.

### 7. Arızalı MCP bağlantılarının tekrar maliyeti — doğrulanmış hata, kısmen açık neden

Tarayıcı turunda `yargi-mcp` bağlantı zaman aşımı, `codebase-memory-mcp` ise çalışan 0.11.0 sürümü ile uygulamanın istediği 0.10.8 sürümünün çakışması nedeniyle başarısız oldu. Her iki sunucuda da dört katalog hatası kaydı oluştu. Bu bağlantıların çalışmadığı kesin; boşuna yeniden bağlantı kurulmasının süresini ve alt süreç maliyetini azaltmak anlamlı.

**Önemli ayrım:** Sistemde devre kesici zaten var. `FailStreaks.Note` ilk başarısızlıktan sonra 45 saniye bekleme açıyor, `buildRegistry` bunu kontrol ediyor. Dolayısıyla çözüm “devre kesici ekle” kadar basit değil. Bazı hata kayıtları 45 saniyeden yakın; daha önce başlamış/eşzamanlı katalog isteklerinin korumayı geçmesi olası, ancak bu tur istek düzeyinde izlenip kanıtlanmadı. Araç ekranı önce önbelleği gösterse de ardından canlı katalog talep ediyor.

**Öneri:** Önce sürüm uyuşmazlığını ve ulaşılamayan sunucu yapılandırmasını düzelt. Ardından eşzamanlı katalog isteklerinin tek bağlantı denemesini paylaşmasını ve bağlantı kuyruğunda da bekleme durumunun korunmasını sınayan bir tekrar testi yap. Sunucuları veya başka uygulamalara ait MCP süreçlerini bu inceleme kapatmadı.

**Kanıt:** `_devlogs/perf-audit-backend.log:24`, `internal/mcp/repair/streaks.go:79`, `internal/agent/toolsetup.go:601`, `internal/mcp/pool.go:183`, `frontend/src/features/tools/useToolsPanelState.ts:218`.

## Tarayıcı turu

16 görünüm gezildi: Sohbet, Panel, Ajanlar, Rota, Harita, Görevler, Otomasyon, Akışlar, Araçlar/MCP, Bütçe, İçgörü, Workspace, Ayarlar ve Workspace altındaki Loglar, İşlemler, Öneriler. Panel, Sohbet ve Loglar ayrıca doğrudan Go sunucusunun üretim arayüzünde karşılaştırıldı.

| Görünüm | Gözlem | Yorum |
|---|---|---|
| Sohbet | Vite 2.140, üretim 2.130 DOM düğümü; üretimde 201 buton | Yan panelleri ve görünür mesaj içeriğini ayrı profillemek için aday. Mevcut mesaj sanallaştırmasını yok saymamalı. |
| Ajanlar | 1.300 DOM düğümü, 157 buton | Kart ayrıntılarını isteğe bağlı açmak değerlendirilebilir; çizim darboğazı ölçülmedi. |
| Loglar | 47 kayıtta üretimde 1.363 DOM düğümü, yaklaşık 12.500 görünür metin karakteri | Hata yığınlarını varsayılan kapalı tutmak veya satırları pencerelemek değerlendirilebilir. |
| Görevler | 18 görevde 575 DOM düğümü, yaklaşık 19.014 görünür metin karakteri | Uzun açıklamalara kısa önizleme adayı; yüksek karakter sayısı tek başına ağır çizim kanıtı değildir. |
| Panel | Vite 691, üretim 679 DOM düğümü | Genel ağırlık iddiasını destekleyen belirgin bulgu yok. |
| Harita | 448 DOM düğümü | Arayüz 367 düğüm/416 ilişki bildirirken tümünü DOM'a açmıyor; büyük veri sayısı tek başına sorun değil. |

Loglar görünümünde 30 saniyelik beklemede DOM ve görünür metin değişmedi. Bu, ağ isteği yapılmadığının kanıtı değildir. Vite üzerinde hızlı rota değişimlerinde iptal edilen isteklerin `AbortError` kayıtları UI günlüklerine düştü; üretimde karşılaştırılan üç sayfada hata/uyarı görülmedi. Geliştirme modunda beklenen iptalleri hata telemetrisinden ayırmak ikincil temizlik adayıdır. Tarayıcı otomasyon adımlarının bekleme süreleri sayfa açılış performansı olarak raporlanmadı.

## Korunması gereken mevcut optimizasyonlar

- Mesaj listesi görünür pencere dilimini çiziyor: `frontend/src/features/chat/MessageList.tsx:477`. Uzun sohbetin tamamı DOM'a basılıyor varsayımı doğru değil.
- Ağır sayfalar talep anında yükleniyor: `frontend/src/app/lazyPanels.ts`. Mermaid de Vite yapılandırmasında bilerek dinamik bırakılmış; büyük paket gördüğümüz için bu sistemi bütünüyle kaldırmak gerekmez.
- Activity için 15 saniye, workspace activity ve execution runtime için 20 saniye yedek sorgu aralığı var; ortak zamanlayıcı görünürlük kontrolü zaten mevcut.
- Workspace activity sunucuda tüm geçmişi taramak yerine sayaç/aktif kayıt kontrolü kullanıyor: `internal/api/activity.go:91`.
- Oturumların sıralı listesi değişiklik kuşağına göre önbellekleniyor: `internal/db/store_sessions_cache.go:21`.

Bu tur hiçbir alt sistemin bütünüyle gereksiz olduğunu kanıtlamadı. Özellik silme yerine önce yukarıdaki çalışma ve yükleme maliyetlerini azaltmak daha somut bir başlangıç.

## Uygulama sırası ve doğrulama

1. Günlük kullanımda tek Go uygulamasıyla süreç/RAM farkını tekrar ölç.
2. Araç sürümü önbelleği + eşzamanlı istek birleştirme; aynı öneri ekranına tekrar girişte yeni sürüm alt süreci açılmadığını doğrula. Arızalı MCP yapılandırmalarını ve yeniden deneme birleşimini ayrıca ele al.
3. Çift ilk yük ve gizli pencere yenilemelerini gider; görünürlük dönüşünde tek güncel yanıt alındığını doğrula.
4. İlk açılış paketini ayır; aynı giriş sayfasında önce/sonra üretim ağ ve ana iş parçacığı profili al.
5. Gerçekçi büyük oturum/flow geçmişiyle runtime uç noktasını ölç; ihtiyaç varsa veri yapısını değiştir.

Ham API örnekleri: `_devlogs/perf-audit-measurements.json`. Profiller: `_devlogs/perf-audit-cpu.pprof`, `_devlogs/perf-audit-heap.pprof`. Bunlar yerel ve git dışı tanı dosyalarıdır. Ürün koduna dokunulmadığından davranış test paketi çalıştırılmadı; Go derlemesi, üretim frontend derlemesi, API örnekleri ve tarayıcı gezintisi yapıldı.
