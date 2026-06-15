# SwarmGo — Genel Bakış

> **SwarmGo**, [SwarmClaw](https://github.com/swarmclawai/swarmclaw) projesinin Go diliyle, kendi UI/UX tasarımımızla, sıfırdan yeniden yazımıdır.

## Amaç

Açık kaynaklı, kendi sunucunda barındırılan (self-hosted) bir **çoklu-ajan (multi-agent) AI çalışma ortamı (runtime)** ve **kontrol düzlemi (control plane)** inşa etmek. Birden fazla otonom AI ajanını yöneten, görev dağıtan, hafıza tutan, zamanlanmış işler çalıştıran ve harici platformlara (Discord, Slack vb.) bağlanan bir sistem.

## Neden Go?

| Kriter | Kazanım |
|--------|---------|
| **Eşzamanlılık (concurrency)** | Goroutine + channel modeli, çoklu-ajan orkestrasyonu için doğal |
| **Tek binary** | Bağımlılıksız, kolay dağıtım |
| **Düşük RAM / yüksek performans** | SwarmClaw'un Electron/Node yükü olmadan |
| **Çapraz derleme** | Tek komutla Windows / macOS / Linux |

## Orijinal (SwarmClaw) vs SwarmGo

| Bileşen | SwarmClaw | SwarmGo |
|---------|-----------|---------|
| Dil | TypeScript / Node.js 22 | Go 1.26+ |
| Masaüstü kabuk | Electron | Wails v2 |
| Web framework | Next.js | Bağımsız frontend + Go API |
| Veritabanı | better-sqlite3 | modernc.org/sqlite (saf Go, CGO yok) |
| Orkestrasyon | LangGraph | Kendi state-machine + goroutine/channel |
| Boyut | ~100-150 MB | ~10-20 MB hedef |

## Temel Kavramlar

- **Agent (Ajan):** Kalıcı kimlik, hafıza ve araç erişimi olan otonom AI varlığı. Bir LLM sağlayıcı/modeline bağlanır.
- **Swarm (Sürü):** Delegasyon ve paylaşımlı hafıza ile işbirliği yapan ajan toplulukları.
- **Session (Oturum):** Mesaj geçmişini ve bağlamı koruyan konuşma dizisi.
- **Memory (Hafıza):** Hibrit hatırlama — dokümanlar, günlük (journal), yansıtma (reflection) notları.
- **Task (Görev):** Yürütme politikaları, retry mantığı ve bağımlılıkları olan pano-tabanlı iş kuyruğu.
- **Connector (Bağlayıcı):** Discord, Slack, Telegram gibi harici platform köprüleri.
- **Provider (Sağlayıcı):** LLM uç noktası soyutlaması (Anthropic, OpenAI, Ollama...).

## Doküman Dizini

| Doküman | İçerik |
|---------|--------|
| [00-GENEL-BAKIS.md](00-GENEL-BAKIS.md) | Bu dosya — projenin amacı ve özeti |
| [01-MIMARI.md](01-MIMARI.md) | Sistem mimarisi, katmanlar, modüller |
| [02-VERI-MODELI.md](02-VERI-MODELI.md) | Veritabanı tabloları ve veri modeli |
| [03-YOL-HARITASI.md](03-YOL-HARITASI.md) | Aşama aşama (faz) geliştirme planı |
| [04-TEKNOLOJI-SECIMLERI.md](04-TEKNOLOJI-SECIMLERI.md) | Kütüphane seçimleri ve gerekçeleri |
| [05-ILERLEME.md](05-ILERLEME.md) | Yapılanlar / sıradaki adımlar takibi |

## Kurulu Ortam (2026-06-15 itibarıyla)

- ✅ Go 1.26.4
- ✅ Node.js v24 + npm 11
- ⏳ Wails v2 (kurulacak)
