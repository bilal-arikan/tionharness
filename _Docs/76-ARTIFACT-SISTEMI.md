# Artifact Sistemi: Görsel Üzerine Çizim

Bu belge artifact önizlemesinden açılan görsel işaretleme akışını ve türetilmiş
artifact kalıcılık sözleşmesini tanımlar. Özellik TSK476 (`32ed34a3`) ile
tamamlandı; yaşam döngüsü, erişilebilirlik ve büyük görsel korumaları TSK477
(`cafbb3bc`) ile sertleştirildi.

## Kullanıcı akışı

1. Kullanıcı PNG, JPEG veya WebP türündeki bir artifact'ın önizlemesini açar.
2. **Üzerine çiz** düğmesi kaynak blob'u indirir, sınırlarını doğrular ve tarayıcıda
   çizilebilir kaynağa dönüştürür. Metin artifact'larında bu eylem gösterilmez.
3. Editör açıldığında canvas klavye odağını alır. Kalem rengi ve boyutu seçilebilir;
   pointer ile çizim yapılabilir. **Geri al**, **Yinele** ve **Temizle** komutları
   aktif stroke sürerken devre dışıdır.
4. **Kaydet**, kaynak görseli ve stroke'ları özgün çözünürlükte birleştirir. Opak
   kaynak önce WebP olarak kodlanır; desteklenmeyen veya başarısız WebP kodlaması
   tek bir PNG denemesine düşer. Saydam kaynak PNG kalır.
5. Kodlanan dosya önce upload staging alanına yazılır. Ardından
   `POST /api/artifacts`, `derivedFromArtifactId` ile yeni bir image artifact
   oluşturur. Özgün artifact değişmez; önizleme yeni türetilmiş artifact'a geçer.

## Canvas modeli ve sınırlar

- Stroke, kaynak görsel koordinatlarında nokta, basınç, renk ve kalem genişliği
  taşır. Görünüm ölçeklense bile export özgün genişlik ve yükseklikte üretilir.
- Undo/redo geçmişi stroke değişikliklerini saklar. Model stroke başına 20.000
  noktayı ve geçmiş için bellek sınırını uygular; sınıra ulaşınca kararlı bir uyarı
  verir.
- Kabul edilen kaynaklar PNG, JPEG ve WebP'dir. Kaynak üst sınırları 20 MiB,
  boyut başına 8192 piksel ve toplam 40 milyon pikseldir. Dosya başlığı ile decode
  sonucu birlikte doğrulanır.
- Büyük görsellerde ekran canvas'ının backing store'u en fazla 8 milyon piksele
  ölçeklenir. Bu yalnız etkileşimli önizlemeyi sınırlar; export canvas'ı özgün
  çözünürlüğü korur ve encode tamamlanınca backing store hemen serbest bırakılır.
- Kalem varsayılanı kaynak genişliğine göre ölçeklenir. Bu sayede küçük ve büyük
  kaynaklarda çizginin görsel oranı tutarlı kalır.

## Türetilmiş artifact ve parent session sözleşmesi

İstemci create isteğinde açık artifact'ın `sessionId` değerini gönderir. Sunucu
buna güvenmek yerine `derivedFromArtifactId` ile parent kaydını kilit altında
çözer ve türetilmiş kaydın `SessionID` değerini parent artifact'tan alır. Dosya
`artifacts/<parent-session>/<derived-id>.<ext>` yoluna taşınır. Böylece farklı bir
session kimliğiyle gönderilen istek bile parent soyunu ve depolama dizinini
değiştiremez.

`DB.CreateDerivedArtifact` yeni kimliği ayırır, parent'ı doğrular, dosyayı atomik
olarak yerine koyan callback'i çalıştırır ve metadata JSON'u başarıyla yazıldıktan
sonra bellek içi indeksi günceller. Eşzamanlı türetmeler ayrı kimlikler alır ve
özgün dosyaya yazmaz.

## Hata ve cleanup davranışı

- Kaynak indirme, dosya başlığı, decode, pointer capture, export, upload ve artifact
  oluşturma hataları kullanıcıya çevrilmiş hata metniyle gösterilir.
- Artifact oluşturma staging upload'dan sonra başarısız olursa istemci staged
  dosyayı siler. Oluşturma ve cleanup birlikte başarısız olursa iki neden
  `AggregateError` içinde korunur.
- İstemci editör kapanırken devam eden kaynak indirmesini abort eder. Geç gelen
  decode/save sonuçları generation kontrolüyle UI durumunu değiştiremez; artifact
  create başlamadan kapanmışsa staged upload silinir.
- Sunucu staged dosyanın sahipliğini ancak atomik taşıma başarıyla tamamlanınca
  devralır. Okuma, doğrulama veya taşıma öncesi hata staging dosyasını temizler.
  Metadata kalıcılığı başarısız olursa callback hedef dosyayı siler ve bellek içi
  artifact kaydı oluşmaz.
- Kaydetme sürerken ikinci kaydetme ve kapatma engellenir. Kaydedilmemiş stroke'lar
  varken kapatma kullanıcı onayı ister.

## Erişilebilirlik ve modal yaşam döngüsü

- Editör `role="dialog"`, yerelleştirilmiş erişilebilir ad, klavye yardım metni ve
  kaydetme sırasında `aria-busy` sunar.
- Canvas `tabIndex=0` ile klavyeden odaklanabilir. Tab ve Shift+Tab odağı modal
  içindeki etkin kontroller arasında döndürür; editör kapanınca önceki odak geri
  yüklenir.
- Düğmelerin görünür metinleri ve `aria-label` değerleri yerelleştirilmiştir.
  Kalem boyutu range kontrolü `aria-valuetext` ile piksel değerini bildirir; hata
  alanı `role="alert"` kullanır.
- Editör açıkken dış önizleme modalının Escape/backdrop kapatması devre dışıdır;
  kapanma kararı editörün kaydedilmemiş değişiklik ve kaydetme kurallarından geçer.

## Doğrulama kapsamı

Kalıcı otomatik tarayıcı E2E paketi bulunmuyor; frontend test altyapısı
Vitest/jsdom tabanlıdır. Akış şu katmanlarda doğrulanır:

- `ArtifactPreviewModal.test.tsx`: editörü açma, türetilmiş artifact isteği,
  session/parent alanları, create hatasında staging cleanup, geç async sonuç ve
  modal kapanma davranışı.
- `ImageAnnotator.test.tsx`: çizim kontrolleri, undo/redo/clear, dirty kapanma,
  kaydetme, odak tuzağı/geri yükleme, kaydetme yaşam döngüsü ve büyük canvas oranı.
- `imageAnnotatorModel.test.ts`, `imageAnnotatorExport.test.ts` ve
  `imageAnnotatorLimits.test.ts`: model limitleri, encode fallback/cleanup ve
  kaynak doğrulaması.
- `internal/api/artifacts_test.go` ve `store_artifact_derived_test.go`: parent
  session zorlaması, eşzamanlı türetme, özgünün korunması, staging/hedef cleanup ve
  disk/bellek atomikliği.

## Clipboard görseli composer akışı

Clipboard'dan yapıştırılan desteklenen görsel doğrulandıktan sonra seçim paneli
göstermeden özgün haliyle doğrudan composer ekine yüklenir. Ek küçük görselinin
üzerindeki **Görseli düzenle** düğmesi ortak `ImageAnnotator` panelini açar.
**Kaydet**, çizimli dosyayı yükleyip composer'daki aynı eki ve önizlemeyi değiştirir;
önceki upload dosyası yeni upload başarıyla tamamlandıktan sonra silinir. Dosya
seçiciden eklenen görsellerin otomatik çizim akışı değişmez; ilk kayıt sonrasında
aynı **Görseli düzenle** eylemi ve güvenli upload değiştirme davranışı kullanılabilir.

## Artifacts ekranında yerinde düzenleme (override)

Artifacts ekranındaki detay başlığında, yalnız `kind === 'image'` ve `sourcePath`
dolu olan artifact'larda **Görseli düzenle** düğmesi görünür
(`ImageArtifactEditButton.tsx`). Önizleme modalındaki **Üzerine çiz** akışından
farkı: türev artifact üretmez, **aynı artifact'ı override eder**.

- Kaynak yükleme ve doğrulama ortak `useImageArtifactSource` hook'undadır; bitmap
  sahipliği (kapatma, generation ile geç sonuç iptali) hook'a aittir.
- **Kaydet**: export edilen dosya `POST /api/uploads` ile staging'e yazılır,
  ardından `PUT /api/artifacts/{id}` gövdesinde yalnız `sourcePath` gönderilir.
  Backend (`UpdateArtifactSource`) artifact'ı yeni dosyaya yönlendirir ve eski
  dosyayı — yeni yoldan farklıysa — siler.
- `PUT` başarısız olursa staging dosyası silinir; temizlik de başarısız olursa iki
  hata `AggregateError` ile birleştirilip editörde gösterilir. Hata sessizce
  yutulmaz.
- Yükleme yolu her seferinde benzersiz bir id öneki taşıdığından (`artifacts/<session>/<id>-<ad>`)
  görselin URL'i değişir; ayrıca cache-buster gerekmez.
- Yeni metinler `artifactAnnotation.edit`, `artifactAnnotation.editTitle`,
  `artifactAnnotation.updated` ve `artifactAnnotation.overwriteAndCleanupError`
  anahtarlarındadır (en + tr).

Canlı kabul testi için çalışan uygulamada image artifact önizlemesi açılır; çizim
eklenir; kaydetme sonrası yeni artifact kimliği, `derivedFromArtifactId`, parent
session ve özgün artifact'ın değişmediği API üzerinden kontrol edilir. Ardından
klavye odağı, Escape/backdrop davranışı ve tarayıcı konsolu denetlenir.
