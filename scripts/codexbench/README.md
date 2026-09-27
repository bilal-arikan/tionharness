# Codex taşıyıcı karşılaştırması

## Kotayı kullanmadan araç izinlerini doğrulama

`probe_policy.py`, gerçek Codex çalıştırıcısını yerel sahte model ve MCP
sunucularıyla sınar. Giriş veya model API çağrısı yapmaz. Astra/Sol kimlikleriyle
sekizer senaryoda gerçek araç kataloğunu ve izinli/yasaklı çağrıların dispatch
sonucunu denetler; bu bir model başarı karşılaştırması değildir.

```bash
python scripts/codexbench/probe_policy.py --codex 'C:/path/to/codex.exe' --output .scratch/codex-policy/results.json
```

Test izole geçici ev ve çalışma dizini kullanır. Yalnız zararsız taklit araçlar
çağrılır; yerel model sağlayıcısına geçiş ve CLI araç keşfi gerçek çalıştırıcıda
gerçekleşir. Codex 0.157.1 ile doğrulanmıştır.

## Gerçek model karşılaştırması

Bu isteğe bağlı deney gerçek hesap kotası tüketir. `TionHarness CodexCLI.Complete`
ile doğrudan `codex exec` yolunu aynı CLI, hesap, model ve `high` düşünme seviyesiyle
karşılaştırır. **Codex Desktop veya TionHarness'in tam uygulama testi değildir.**
MCP köprüsü, pano/worker akışları, plugin'ler, uzun oturum, resume ve compaction
ölçülmez. Native dosya/kabuk araçlarıyla gerçek Python dosyaları düzenlenir.

## Deney düzeni

- Astra 6 ve Sol 6 × iki taşıyıcı × üç görev = 12 bağımsız çağrı.
- Her çağrı temiz geçici çalışma dizini ve geçici Codex evi kullanır.
- Her görevde sekiz `unittest` kontrolü vardır; değerlendirici model bittikten
  sonra çalışma dizinine konur. Hatalı sonuçlara düzeltme turu verilmez.
- Görevler: yarı açık aralıkları birleştirme, TTL/LRU önbelleği, topolojik katmanlar.
- Sıra görev/model bazında dönüşümlüdür; bu tam rastgeleleştirme değildir.
- Native web araması ve alt ajanlar kapalıdır; ağ erişimi görev talimatıyla
  yasaklanır (kabuk için teknik ağ izolasyonu kurulmaz). MCP/plugin eklenmez. Ortak görev talimatları
  aynıdır. TionHarness kendi prompt biçimlendirmesini ve ek etkileşim talimatını
  uygular; bunlar ölçülen taşıyıcı farkının parçasıdır.
- İki yol da etkileşimsiz, sandbox/onay bypass modunda çalışır. Yalnız bu küçük,
  denetlenen görevler için kullanın. CLI hesabı gerçek kullanıcı hesabıdır.
- Çağrı başına 180 saniye, değerlendirmeye 15 saniye sınırı uygulanır.
- Süre sağlayıcı/CLI çağrısını ve native araç döngüsünü kapsar; harici
  değerlendirici süresi dahil değildir. TionHarness'in dahili retry davranışı
  varsa bu süreye dahildir; ayrıca otomatik benchmark retry yapılmaz.
- Girdi alanı cache hariç token'dır. Toplam girdi = input + cache read + cache
  write. Çıktı düşünme token'larını zaten içerir. Abonelik kotası/maliyeti bu
  sayaçlardan kesin hesaplanamaz.
- Her hücrede yalnız üç görev vardır. Başarı yüzdesi bu örneklem içindir;
  genel kodlama başarısı veya istatistiksel üstünlük göstermez. Sunucu yükü,
  cache ısınması ve görev sırası gecikmeyi etkiler.

## Çalıştırma

Depo kökünden Git Bash ile, yolları kendi kurulumunuza göre verin:

```bash
TIONHARNESS_ENABLE_SHELL=1 go run ./scripts/codexbench \
  -bin 'C:/path/to/codex.exe' \
  -home 'C:/path/to/authenticated/codex-home' \
  -out 'C:/path/to/results.json'
```

`-out` üst dizini önceden mevcut olmalıdır. Sonuç her çağrıdan sonra yazılır;
kesilen deneyin tamamlanan hücreleri korunur. Başarısız model çağrısı veya test
`passed: false` olarak kaydedilir; programın tamamlanması tüm görevlerin geçtiği
anlamına gelmez. Kimlik bilgileri rapora yazılmaz, geçici evler iş sonunda silinir.

## Ayrıntılı araç izi

Her çağrının `trace` alanı araç adı, mevcut girdi/çıktı, hata bayrağı, turdan
itibaren bitiş zamanı ve ölçülebilen süreyi saklar. Nihai `solution.py` da
sonuca eklenir. TionHarness için üretim `OnEvent` izi, doğrudan CLI için canlı
JSONL olay akışı kullanılır. Tamamlanan adımlar sayılır; ara güncellemeler ikinci
çağrı sayılmaz. Süre testleri başlangıcı olmayan adımları sıfır olarak sunmaz.

- TionHarness'in sıfır `DurMs` değeri gerçek sıfırla eksik başlangıcı ayırt
  ettirmediği için raporda `null` olur. Native yolda başlangıç varsa ölçülen sıfır
  korunur; yoksa `null` yazılır. Bu fark süre kapsam tablosunda görünür.
- `file_change` olayları tam yamayı içermeyebilir. Boş yama girdisi geriye dönük
  uydurulmaz. Nihai dosya, ara düzenleme geçmişinin yerine geçmez.
- Komut izleri yerel yolları ve görev çıktılarını içerir.
- Araç hatası, tur başarısı ve bağımsız değerlendirici sonucu farklı ölçümlerdir.
- `annotate_tools.py` bilinen komutları muhafazakâr biçimde etiketler; tanınmayan
  komut `incelenmedi` kalır. Nihai rapordan önce komutları ve bu etiketleri inceleyin.
  Aynı girdinin yeniden görülmesi otomatik olarak israf veya retry sayılmaz.

```bash
python scripts/codexbench/annotate_tools.py results.json annotated.json
python scripts/codexbench/detailed_report.py annotated.json report.md
```

Ayrıntılı rapordaki sabit bağlantılar 2026-09-28 deney dosyaları içindir; farklı
bir tarih/konuma çıktı alındığında bu bağlantıları uyarlayın. İz ayrıştırıcısının
parçalı JSONL, hata, mükerrer tamamlanma ve eksik süre testleri canlı çağrı yapmaz:
`go test ./scripts/codexbench`.

Kaydedilmiş dört bağımlılık çözümünü bağımsız olarak 512'şer grafikle sınamak için:
`python scripts/codexbench/verify_solutions.py annotated.json`. Bu sonradan yapılan
kontrol modelin araç sayısına, süresine veya token kullanımına eklenmez; sonucu
`supplementalGraphOracle` alanına yazılır. Etiketleme aracını tekrar çalıştırmak
bu ek alanı yeniden üretmez; sıralama etiketleme → ek kontrol → rapordur.
