# TionHarness arayüzü

React, TypeScript, Vite ve Tailwind tabanlı web arayüzüdür. API Go sunucusunda
çalışır; üretim derlemesi `internal/web/dist` içine yazılır ve Go binary'sine gömülür.

## Yerel geliştirme

Komutları depo kökünde Git Bash ile çalıştırın. Node.js/npm ve Go kurulu olmalıdır.

```bash
npm --prefix frontend ci
powershell.exe -NoProfile -File scripts/dev.ps1 -Loopback -NoKillPort
```

Geliştirme başlatıcısı API'yi `http://127.0.0.1:8090`, Vite'ı
`http://127.0.0.1:5173` üzerinde çalıştırır. Vite `/api` ve `/health` isteklerini
8090'a yönlendirir. Kullanıcının çalışan sunucusunu kapatmayın; port doluysa
önce hangi sürecin kullandığını kontrol edin.

Backend zaten 8090'da çalışıyorsa yalnız arayüzü başlatabilirsiniz:

```bash
npm --prefix frontend run dev
```

## Derleme ve doğrulama

```bash
npm --prefix frontend run build
npm --prefix frontend test
npm --prefix frontend run lint
npm --prefix frontend run format:check
```

`build`, TypeScript denetiminden sonra gömülecek dosyaları üretir. Derleme ile Go
testlerini aynı anda çalıştırmayın; çıktı klasörünün temizlenmesi `go:embed`
taramasıyla çakışabilir. Değişiklik tesliminde ortak kapı depo kökünden
`scripts/test.sh full` komutudur.

## Dosya düzeni

- `src/features/`: ekranlar, o ekrana ait bileşenler ve testler.
- `src/shared/`: ortak bileşenler, yardımcılar ve biçimlendirme.
- `src/api/`: HTTP ve canlı olay bağlantıları.
- `src/i18n/`: dil altyapısı; görünür metinler ilgili çeviri kataloglarından gelir.
- `vite.config.ts`: geliştirme proxy'si, çıktı yolu ve paket bölme ayarları.

Mimari ve kullanım ayrıntıları: [Sistem mimarisi](../_Docs/01-MIMARI.md),
[Sohbet arayüzü](../_Docs/07-CHAT-UX.md),
[Lokalizasyon](../_Docs/73-LOKALIZASYON.md) ve
[Ajan kuralları](../CLAUDE.md).
