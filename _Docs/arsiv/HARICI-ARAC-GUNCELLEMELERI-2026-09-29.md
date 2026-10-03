# Kişisel araç güncellemesinin tarihsel kaydı

> **Özet (2026-10-03):** 28–29 Eylül 2026 kişisel Windows araç bakımının raporudur. Aşağıdaki kurulum durumları, bekleyen yönetici adımları, yerel yedekler ve Temp yolları o tarihin kaydı olarak korunur; tamamlanmamış adımların sonradan tamamlandığı iddia edilmez. Bu yollar taşınabilir proje kaynakları değildir. Projeye ait Go uyumluluk bulgusu [teknoloji rehberinde](../04-TEKNOLOJI-SECIMLERI.md) ayrıca açıklanır; bu belge bir çalıştırma talimatı veya yeni işlem yetkisi değildir.

# Harici araç güncellemeleri — 29 Eylül 2026

> **Özet:** TionHarness'in 16 araçlık kataloğu ile Go ve ripgrep kontrol edildi.
> 14 aracın/araç zincirinin kullanıcı kurulumları güncellendi; üç araç zaten
> günceldi. Git ve bazı sistem kopyaları tamamlanamadı. İşlem 28 Eylül'de başladı.

## Sürümler

| Araç | Önce | Sonuç |
|---|---|---|
| Claude Code | 2.1.278 | 2.1.284 |
| Codex CLI | 0.157.1 | 0.158.0; eski ve yeni nvm kurulumları |
| RTK | 0.45.0 | 0.50.0 |
| sqz | 1.6.1 | 1.9.0 |
| Mermaid CLI | 11.16.0 | 12.0.0; Puppeteer tarayıcısı da kuruldu |
| Git for Windows | 2.55.0.windows.3 | Değişmedi; açık Git Bash süreçleri 2.56.0 kurulumunu engelledi |
| Node.js | 24.18.1 | nvm: 24.21.0 LTS; sistem kopyası: 24.18.1 |
| Bun | 1.3.1 | 1.4.2; çalışan bunx süreci sonlandırılmadı |
| npm | nvm: 11.19.0; sistem: 12.0.2 | nvm: 12.1.0; sistem için aşağıdaki önemli notu okuyun |
| Python | 3.13.7 | 3.13.15; mevcut 3.13 hattı korundu |
| codebase-memory-mcp | 0.11.0 | Zaten güncel |
| zvec-grep | 0.2.2 | Zaten güncel; yeni nvm ortamına taşındı |
| OpenPencil | 0.8.4 | op-real.exe zaten güncel; özel op.exe başlatıcısı korundu |
| Piper | 1.7.0 | 1.8.0; mevcut sanal ortamda |
| whisper.cpp | 1.9.2 | 1.9.4; resmî sürümün işaret ettiği b5130 Windows paketi |
| FFmpeg | 8.1.1 | 9.0.2 |
| ripgrep | 15.1.0 | 15.2.0 |
| Go | 1.26.4 | Kullanıcı araç zinciri: 1.27.1; sistem başlatıcısı: 1.26.4 |

Go'nun resmî otomatik araç zinciri mekanizması kullanıldı:
GOTOOLCHAIN=go1.27.1+auto. Normal go version çıktısı artık 1.27.1.
Projenin minimum Go gereksinimi değiştirilmedi; önceki kullanıcı ayarı auto idi.

## Tamamlanması gerekenler

**Önemli:** Sistem Node.js yükleyicisi yönetici yetkisi olmadığı için başarısız
oldu. Yükleyicinin geri alma işlemi, C:/Program Files/nodejs altındaki npm'i
12.0.2'den 11.16.0'a döndürdü. Bu kopyayı npm ile 12.1.0'a yükseltme denemesi de
Windows'un EPERM hatasıyla reddedildi. nvm altındaki npm 12.1.0 çalışıyor;
sistem npm kopyasının onarımı henüz tamamlanmış değildir.

Git 2.56 yükleyicisi açık bash/grep süreçlerinin kapatılmasını istiyor.
Bu süreçler zorla sonlandırılmadı. Go'nun sistem MSI kurulumu yönetici
yetkisi istedi; kullanıcı araç zinciri güncellemesi başarıyla tamamlandı.

[Hazır tamamlama betiği](C:/Users/Bilal/AppData/Local/Temp/tionharness-tools-20260928/complete-system-updates.ps1)
Git Bash süreçlerini kapattıktan sonra **yönetici olarak açılmış PowerShell**
içinden çalıştırılabilir. Betik SHA-256 değerlerini yeniden denetler; sistem Go,
Node.js, Git ve npm kurulumlarını sırayla tamamlar. Otomatik yeniden başlatma
veya süreçleri zorla kapatma yapmaz. Sözdizimi doğrulandı; yönetici adımları
bu oturumda çalıştırılmadı.

Bun binary'si güncel olsa da başarısız ilk winget denemesi nedeniyle winget
envanteri eski sürümü gösterebilir. Eski bunx süreci doğal olarak tamamlandıktan
sonra paket yöneticisi kaydı ayrıca eşitlenebilir.

## Doğrulama ve uyumluluk

- Güncellenen binary'lerden sürüm çıktıları alındı.
- Mermaid ile SVG, Piper ile mevcut Türkçe modelden WAV üretildi.
- Piper pip check geçti; Whisper mevcut modelle örnek ses kaydını çözdü.
- Yeni Node ortamında zvec, ONNX Runtime ve sharp yerel modülleri yüklendi.
- İlk tam test turu geçti: tüm Go paketleri, 154 arayüz test dosyası / 1.067 test,
  bağımlılık denetimi ve diff kontrolü.
- Go 1.27 ile bulunan JSON hata mesajı uyumsuzluğu giderildi. İlgili regresyon
  testleri hem Go 1.26 hem Go 1.27 ile geçti.
- Son tam test turu Go 1.27.1 ile geçti: tüm Go paketleri, 155 arayüz test
  dosyası / 1.068 test, bağımlılık denetimi ve diff kontrolü başarılı.

Go 1.27 bazı JSON hata yollarını Graph.nodes.0.branches.0 biçiminde üretiyor.
Önceki Node.nodes.branches biçimiyle birlikte tanınması için
internal/tools/graph_error_path.go ve regresyon testleri eklendi.
İlgisiz alanların yanlış öneriye yönlendirilmediği de sınandı.

Son canlı servis kontrolünde 8090 portu yanıt vermedi. Servis otomatik
başlatılmadı; nedeninin bu güncellemeler olduğu doğrulanmadı. Yukarıdaki
binary denemeleri ve tam test sonuçları canlı servisten bağımsızdır.

## Yedekler ve kanıtlar

- RTK ve sqz: kendi klasörlerinde exe.bak-20260928 dosyaları.
- Whisper: C:/Users/Bilal/Desktop/Progs/whisper/Release.bak-1.9.2-20260928.
- Bun: winget paket klasöründe bun.exe.bak-1.3.1-20260928.
  Çalışan eski süreç bu dosyayı kullanabilir.
- Önceki nvm Node sürümü v24.18.1 korunuyor.
- Go kullanıcı ayarı, go env -w GOTOOLCHAIN=auto ile geri alınabilir.
- Piper'ın önceki sürümü 1.7.0; önceki paket listesi piper-before.txt içinde.
- Ses modelleri ve arama indeksleri korundu; yeni indeks oluşturulmadı.

Paketler, sürüm kayıtları, test günlükleri ve örnek çıktılar:
C:/Users/Bilal/AppData/Local/Temp/tionharness-tools-20260928

Bu geçici klasör yönetici adımı tamamlanana kadar korunmalıdır.
Kullanıcının mevcut proje değişiklikleri geri alınmadı; commit oluşturulmadı.

## Resmî kaynaklar

- [RTK](https://github.com/rtk-ai/rtk/releases/tag/v0.50.0)
- [sqz](https://github.com/ojuschugh1/sqz/releases/tag/v1.9.0)
- [Git](https://github.com/git-for-windows/git/releases/tag/v2.56.0.windows.1)
- [Node.js LTS](https://nodejs.org/en/download)
- [Go sürümleri](https://go.dev/dl/) ve [araç zinciri yönetimi](https://go.dev/doc/toolchain)
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp/releases/tag/v1.9.4)
- [FFmpeg](https://www.gyan.dev/ffmpeg/builds/)
- Diğer paketler resmî GitHub release API, npm registry, PyPI ve winget ile doğrulandı.
