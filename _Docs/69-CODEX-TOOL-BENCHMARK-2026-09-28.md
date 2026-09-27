# Ayrıntılı araç kullanımı karşılaştırması

CLI: `codex-cli 0.157.1`; düşünme: `high`. Önceki deneyden ayrı koşudur.

Aynı üç görev, iki model ve iki taşıyıcı: 12 çağrı. Her görev sonunda sekiz
bağımsız test çalıştırılır. Aşağıdaki amaç etiketleri kaydedilmiş komut ve
araç olayları incelenerek eklenir; modelin kendi beyanı değildir.

| Model | Yol | Başarı | Araç adımı | Araç hatası | Keşif | Okuma | Düzenleme | Doğrulama | Temizlik | Ortalama tur |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| gpt-6-astra | tionharness | 3/3 | 12 | 0 | 3 | 3 | 3 | 3 | 0 | 48.89 sn |
| gpt-6-astra | codex-cli | 3/3 | 12 | 0 | 3 | 3 | 3 | 3 | 0 | 46.86 sn |
| gpt-6-sol | tionharness | 3/3 | 11 | 0 | 3 | 3 | 3 | 3 | 0 | 43.32 sn |
| gpt-6-sol | codex-cli | 3/3 | 13 | 0 | 5 | 3 | 3 | 3 | 1 | 50.19 sn |

Amaç sütunları örtüşebilir: tek kabuk çağrısı hem listeleme hem dosya okuma yapabilir.

## Bulguların yorumu

- 12 deneme ve 96 bağımsız test geçti. 48 tamamlanan araç olayında hata ve aynı
  araç/girdi çiftinin birebir tekrarı yok. Bu, gözlenen izlerdeki durumdur;
  servis içindeki görünmeyen retry'ları ölçmez.
- Astra iki yolda da her görevde keşif → okuma → tek yama → yerel doğrulama
  uyguladı (toplam 12'şer adım). Bu görevlerde yeniden düzenleme döngüsü oluşmadı.
- Sol'un önbellek görevi TionHarness'te **3**, CLI'da **6** adım: iki yol da ilk
  çağrıda keşif+okumayı birleştirdi. CLI, başarılı kontrolden sonra klasörü ve
  `__pycache__` içeriğini listeledi, ardından üretilen cache dosyasını/klasörünü
  temizledi. Üç ek kabuk adımının ölçülen süresi toplam **1,278 sn**; tur farkı
  **18,883 sn**. Süre farkının tamamı kabuk çalışmasına atfedilemez.
- Sol bağımlılık görevinde CLI keşif+okumayı birleştirip 3 adım, TionHarness
  ayrı çağrılarla 4 adım kullandı. Üç görev toplamında fark bu nedenle 11 / 13
  adımdır (TionHarness / CLI). Birleşik çağrı sayısı ve temizlik tercihleri önemlidir.
- **Doğrulama derinliği:** Astra iki yolda da tek Python çağrısında üç düğümlü
  512 yönlü grafiğin tamamını basit bir referans hesaplamayla karşılaştırdı.
  Komut içeriği ve başarılı çıktı bunu doğrular. Sol seçili uç durumları sınadı.
  Aynı sayıda doğrulama çağrısı, aynı kontrol kapsamı anlamına gelmez.
- Bu farkı çözüm kalitesiyle karıştırmamak için kaydedilen **dört bağımlılık
  çözümü ayrıca aynı bağımsız 512 grafikle sınandı: 4/4 başarılı, toplam 2048
  grafik kontrolü geçti.** Bunlar model tamamlandıktan sonra yapılan ek kontrollerdir;
  yukarıdaki araç, süre ve token sayaçlarına dahil değildir.
- Önceki koşuda Sol'un CLI yolu daha hızlıydı; bu koşuda TionHarness daha hızlı.
  Bu iki küçük koşu kalıcı bir hız üstünlüğünü kanıtlamaz. Bu örneklemde TionHarness
  araç kullanımında doğruluk kaybı veya sistematik çağrı şişmesi gözlenmedi.

## Kabuk araç süreleri

| Model | Yol | Süresi ölçülen kabuk çağrısı | Toplam | Ortanca | En uzun |
|---|---|---:|---:|---:|---:|
| gpt-6-astra | tionharness | 9/9 | 3.482 sn | 376 ms | 525 ms |
| gpt-6-astra | codex-cli | 9/9 | 3.371 sn | 369 ms | 485 ms |
| gpt-6-sol | tionharness | 8/8 | 4.289 sn | 499 ms | 1089 ms |
| gpt-6-sol | codex-cli | 10/10 | 4.997 sn | 465 ms | 923 ms |

## Çağrı sıraları

Süreler araç başlangıç/bitiş olaylarının istemciye gelişinden ölçülür; saf CPU
süresi değildir. `?` başlangıç ölçümünün bulunmadığını veya sağlayıcıda sıfır
ile ayırt edilemediğini gösterir. Sıfır maliyet olarak yorumlanmaz. Araçların
toplam süresini turdan çıkarmak saf model düşünme süresini vermez.

### intervals — gpt-6-astra — tionharness

Tur: 37.87 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 9.35 sn | 512 ms | Hayır |
| 2 | shell | solution.py okuma | 13.39 sn | 285 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 33.53 sn | ? | Hayır |
| 4 | shell | Yerel Python kontrolleri | 34.43 sn | 350 ms | Hayır |

### intervals — gpt-6-astra — codex-cli

Tur: 36.53 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 7.25 sn | 286 ms | Hayır |
| 2 | shell | solution.py okuma | 10.62 sn | 376 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 31.87 sn | 2 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 32.63 sn | 360 ms | Hayır |

### intervals — gpt-6-sol — codex-cli

Tur: 34.48 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 11.48 sn | 923 ms | Hayır |
| 2 | shell | solution.py okuma | 14.83 sn | 315 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 21.99 sn | 3 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 30.68 sn | 300 ms | Hayır |

### intervals — gpt-6-sol — tionharness

Tur: 31.61 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 10.64 sn | 554 ms | Hayır |
| 2 | shell | solution.py okuma | 14.00 sn | 323 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 20.30 sn | 1 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 28.07 sn | 329 ms | Hayır |

### ttl_cache — gpt-6-astra — codex-cli

Tur: 57.66 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 9.48 sn | 327 ms | Hayır |
| 2 | shell | solution.py okuma | 12.60 sn | 286 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 50.70 sn | 3 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 51.68 sn | 432 ms | Hayır |

### ttl_cache — gpt-6-astra — tionharness

Tur: 59.46 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 7.94 sn | 313 ms | Hayır |
| 2 | shell | solution.py okuma | 11.10 sn | 396 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 29.74 sn | 2 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 55.29 sn | 443 ms | Hayır |

### ttl_cache — gpt-6-sol — tionharness

Tur: 59.03 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme + solution.py okuma | 11.33 sn | 503 ms | Hayır |
| 2 | apply_patch | Dosya yaması | 36.48 sn | 1 ms | Hayır |
| 3 | shell | Yerel Python kontrolleri | 49.62 sn | 537 ms | Hayır |

### ttl_cache — gpt-6-sol — codex-cli

Tur: 77.91 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme + solution.py okuma | 19.59 sn | 493 ms | Hayır |
| 2 | apply_patch | Dosya yaması | 45.54 sn | 7 ms | Hayır |
| 3 | shell | Yerel Python kontrolleri | 58.63 sn | 756 ms | Hayır |
| 4 | shell | Klasör/dosya listeleme | 63.96 sn | 432 ms | Hayır |
| 5 | shell | Klasör/dosya listeleme | 69.31 sn | 347 ms | Hayır |
| 6 | shell | Üretilen Python cache dosyalarını temizleme | 73.30 sn | 499 ms | Hayır |

### dependency_layers — gpt-6-astra — tionharness

Tur: 49.34 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 8.65 sn | 525 ms | Hayır |
| 2 | shell | solution.py okuma | 12.13 sn | 282 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 25.41 sn | 1 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 44.63 sn | 376 ms | Hayır |

### dependency_layers — gpt-6-astra — codex-cli

Tur: 46.40 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 8.23 sn | 485 ms | Hayır |
| 2 | shell | solution.py okuma | 11.35 sn | 369 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 25.20 sn | 1 ms | Hayır |
| 4 | shell | Yerel Python kontrolleri | 42.11 sn | 450 ms | Hayır |

### dependency_layers — gpt-6-sol — codex-cli

Tur: 38.17 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme + solution.py okuma | 10.97 sn | 495 ms | Hayır |
| 2 | apply_patch | Dosya yaması | 21.42 sn | 2 ms | Hayır |
| 3 | shell | Yerel Python kontrolleri | 30.41 sn | 437 ms | Hayır |

### dependency_layers — gpt-6-sol — tionharness

Tur: 39.33 sn; değerlendirme: geçti.

| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |
|---|---|---|---:|---:|---|
| 1 | shell | Klasör/dosya listeleme | 9.79 sn | 1089 ms | Hayır |
| 2 | shell | solution.py okuma | 12.95 sn | 459 ms | Hayır |
| 3 | apply_patch | Dosya yaması | 24.16 sn | ? | Hayır |
| 4 | shell | Yerel Python kontrolleri | 34.70 sn | 495 ms | Hayır |

¹ Tur başlangıcından itibaren; araçlar bitiş sırasıyla gösterilir.

## Kapsam ve ham kanıt

Bu bir native dosya/kabuk deneyi; MCP, tarayıcı, worker, uzun oturum ve Codex
Desktop karşılaştırması değildir. Her görev/model/yol bir kez çalıştırıldı.
Cache, sunucu yükü ve farklı araç stratejileri gecikmeyi etkileyebilir.

Yama olayları tam yama metnini vermeyebilir; boş alan eksik ölçümdür.
Nihai solution.py içeriği ayrıca saklanır; bu, ara yamaların kaydı değildir.

TionHarness çıktısı sağlayıcının normalleştirdiği ve boyutunu sınırladığı
izdir; doğrudan CLI çıktısı native olay alanlarından alınır. Özellikle yama
girdi/çıktı biçimleri aynı olmadığından karakter sayıları token maliyeti gibi
karşılaştırılmaz. Araç başına token ölçümü yoktur; token bilgisi tur düzeyindedir.

Aynı okuma komutunun düzenlemeden sonra tekrarlanması tek başına israf değildir.
Hata sayacı başarısız durum/çıkış kodlarını ölçer; başarılı test sonucu araç
hatasını silmez. Testler küçük örneklemi kapsar, genel kalite garantisi vermez.

[Girdiler, çıktılar, nihai kod ve ham ölçümler](69-CODEX-TOOL-BENCHMARK-2026-09-28.json)
· [Deney aracı](../scripts/codexbench/README.md)
