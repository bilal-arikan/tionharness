# TionHarness–Codex CLI karşılaştırmalı deney

CLI: `codex-cli 0.157.1`. Düşünme seviyesi: `high`.

Bu sonuçlar üç küçük Python göreviyle yapılan taşıyıcı deneyidir. Codex Desktop
veya TionHarness'in tam uygulaması ölçülmedi. Her görev/model/taşıyıcı yalnız
bir kez çalıştırıldı; istatistiksel üstünlük veya genel başarı oranı çıkarılamaz.

| Model | Yol | Başarılı görev | Ortalama süre | Ortanca süre | Toplam girdi¹ | Cache okuma² | Çıktı |
|---|---|---:|---:|---:|---:|---:|---:|
| gpt-6-astra | tionharness | 3/3 | 49.07 sn | 47.36 sn | 157,777 | 116,992 | 3,374 |
| gpt-6-astra | codex-cli | 3/3 | 47.70 sn | 47.06 sn | 183,174 | 141,952 | 3,211 |
| gpt-6-sol | tionharness | 3/3 | 43.79 sn | 42.40 sn | 177,183 | 113,664 | 4,568 |
| gpt-6-sol | codex-cli | 3/3 | 40.22 sn | 40.92 sn | 165,632 | 91,264 | 4,075 |

¹ Çağrı içindeki tüm model adımlarının toplamıdır; tek bağlam büyüklüğü değildir.
² Cache okuma toplam girdinin alt kümesidir; ikinci kez eklenmez. Çıktı düşünmeyi
zaten içerir. Bunlar abonelik kotası veya gerçek fatura ölçümü değildir.

## Tekil denemeler

| Görev | Model | Yol | Süre | Araç adımı³ | Test sonucu |
|---|---|---|---:|---:|---|
| intervals | gpt-6-astra | tionharness | 37.09 sn | 4 | Geçti |
| intervals | gpt-6-astra | codex-cli | 40.47 sn | 4 | Geçti |
| intervals | gpt-6-sol | codex-cli | 32.36 sn | 3 | Geçti |
| intervals | gpt-6-sol | tionharness | 38.36 sn | 4 | Geçti |
| ttl_cache | gpt-6-astra | codex-cli | 55.57 sn | 4 | Geçti |
| ttl_cache | gpt-6-astra | tionharness | 62.75 sn | 4 | Geçti |
| ttl_cache | gpt-6-sol | tionharness | 50.62 sn | 4 | Geçti |
| ttl_cache | gpt-6-sol | codex-cli | 47.38 sn | 4 | Geçti |
| dependency_layers | gpt-6-astra | tionharness | 47.36 sn | 4 | Geçti |
| dependency_layers | gpt-6-astra | codex-cli | 47.06 sn | 4 | Geçti |
| dependency_layers | gpt-6-sol | codex-cli | 40.92 sn | 4 | Geçti |
| dependency_layers | gpt-6-sol | tionharness | 42.40 sn | 4 | Geçti |

³ Tamamlanan araç olaylarıdır; sağlayıcı izlerinin sınıflandırması farklı olabilir.

## Yöntem ve sınırlar

Her görev sekiz bağımsız unittest yöntemiyle değerlendirildi. Değerlendirici
model tamamlandıktan sonra eklendi. Temiz klasörler, aynı hesap, CLI, görev
talimatları ve düşünme seviyesi kullanıldı. Sıra dönüşümlüydü; sunucu cache'i
sıfırlanmadı. Başarısız görevlere dışarıdan düzeltme veya tekrar hakkı verilmedi.

Ölçüm native dosya/kabuk araçlarını içerir; MCP, plugin, worker, çok turlu
oturum, compaction ve Desktop araç ekosistemini içermez. Süre CLI başlatma,
model ve araç döngüsünü kapsar, bağımsız değerlendiriciyi kapsamaz. Cache ve
araç stratejisi değişebildiğinden süre/token farkı yalnız taşıyıcı maliyeti
olarak yorumlanamaz. Uzun veya zor depo görevlerinde sonuç değişebilir.

Tekrar üretme: [deney aracı](../scripts/codexbench/README.md).
Ham ölçümler: [JSON sonuçları](69-CODEX-BENCHMARK-2026-09-28.json).
