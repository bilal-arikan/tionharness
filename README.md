# SwarmGo

> [SwarmClaw](https://github.com/swarmclawai/swarmclaw) projesinin **Go** ile, kendi UI/UX tasarımıyla yeniden yazımı.

Açık kaynaklı, kendi sunucunda barındırılan **çoklu-ajan (multi-agent) AI çalışma ortamı** ve kontrol düzlemi.

## Hızlı Başlangıç

```powershell
# Sunucuyu çalıştır
go run ./cmd/swarmgo

# Sağlık kontrolü (başka bir terminalde)
curl http://localhost:8080/health
```

## Dokümantasyon

Tüm plan ve tasarım dokümanları [`_Docs/`](_Docs/) klasöründedir:

- [Genel Bakış](_Docs/00-GENEL-BAKIS.md)
- [Mimari](_Docs/01-MIMARI.md)
- [Veri Modeli](_Docs/02-VERI-MODELI.md)
- [Yol Haritası](_Docs/03-YOL-HARITASI.md)
- [Teknoloji Seçimleri](_Docs/04-TEKNOLOJI-SECIMLERI.md)
- [İlerleme Takibi](_Docs/05-ILERLEME.md)

## Proje Yapısı

```
SwarmGo/
├── _Docs/              # Plan ve tasarım dokümanları (Türkçe)
├── cmd/swarmgo/        # Giriş noktası
├── internal/           # Uygulama kodu (agent, providers, db, api...)
├── frontend/           # Web UI (React + Tailwind)
└── go.mod
```

## Geliştirme İlkeleri

- Kod ve yorumlar **İngilizce**, dokümanlar **Türkçe**.
- CGO yok → kolay çapraz derleme (Windows / macOS / Linux).
- Modüler: her sorumluluk ayrı pakette.

## Durum

🚧 **Faz 0 → Faz 1** — geliştirme erken aşamada. Bkz. [İlerleme](_Docs/05-ILERLEME.md).
