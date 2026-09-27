# Codex taşıyıcı karşılaştırması

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
