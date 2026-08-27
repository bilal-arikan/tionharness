# TionHarness Maliyet Düşürme Planı

Tarih: 2026-07-05 · Kaynak: 21 ajanlık ultracode incelemesi (kod + oturum logu + Anthropic dokümanları), tüm bulgular koda karşı adversarial olarak doğrulandı.

## Ölçülen Sorun

Test oturumu `260705-tall-lake` (SES91, 3 basit sohbet çağrısı, neredeyse hiç araç kullanımı yok): **$2.59** —
aynı iş için beklenenin bir kaç katı.

| Tur | input | output | cacheWrite | cacheRead |
|-----|-------|--------|-----------|-----------|
| 1 | 334 | 165 | 51.920 | 0 (soğuk, normal) |
| 2 | 945 | 995 | **65.686** | **0 (tam miss!)** |
| 3 | 945 | 1.324 | 3.794 | 63.742 |

- Tur 1→2 arası sadece **18 saniye** → TTL süresi dolması elendi; miss'in tek açıklaması **prefix'in bayt düzeyinde değişmesi**.
- Tur 2'nin yazdığı prefix tur 1'inkinden **~13.8k token daha büyük** (yalnızca ~630 token yeni konuşma varken) → istek gövdesi çağrılar arasında mutasyona uğruyor.
- Cache yazımları oturum maliyetinin **~%88'i**. Mükemmel cache ile aynı oturum ~$1.38–1.66 olurdu.
- Her çağrıda 140 araç tanımı = **31.331 token** (context'in %73,7'si) + 9.793 token sistem promptu.

## Kök Nedenler (kod kanıtlı)

1. **Oturum claude-cli sarmalayıcı yolunda koştu** (provider=`claude-cli`, model `claude-cli|claude-opus-4-8`). `toolloop.go:193-270` her turda **tek seferlik yeni bir `claude -p` alt süreci** başlatıyor ve `--mcp-config`'i yeniden üretiyor (`claudePersistentSession=false`) → prefix her turda değişiyor, cache kırılıyor.
2. **Araç şişkinliği CLI yolunda**: 140 aracın çoğu Claude Code yerleşikleri + playwright (~21) + bir tarayıcı MCP sunucusu (~25-30). `climcp.go:85-103` harici MCP sunucularını **toptan** geçiriyor; kullanıcının 23 playwright aracına koyduğu "hidden" işaretleri ve `DisabledTools` listesi bu yolda **hiç okunmuyor**. TionHarness'in kendi 24 çekirdek aracı sadece ~6k token.
3. **Native (anthropic) yolda gizli cache hataları** (ajan bu yola geçince patlayacak):
   - Lazy-tool aktivasyon seti **her turda sıfırlanıyor** ve tur ortasında budanıyor (`toolloop.go:279/347-348/592`) → tools bloğu değişince Anthropic'in tools→system→messages hiyerarşisi gereği TÜM prefix geçersiz.
   - `ExtendedPromptCache` varsayılan **kapalı** (`settings.go` `Default()` içinde yok) → taze kurulum hiç `cache_control` göndermiyor VE sistem promptuna **saniye hassasiyetli timestamp** ekleniyor (`chat_turn.go:85,182-185`).
   - TTL sabit **1 saat** (`anthropic.go:17,44`) → yazım primi 2× (5 dk'lık 1.25× yerine); ölçülen aralıklar 18 sn iken 1h hiçbir şey kazandırmadı.
4. **Fiyat tablosu ve kör noktalar**: Dahili fiyat tablosu opus-4-8'i sağlayıcının güncel liste fiyatının 3 katı sayıyor → maliyetler şişkin tahmin ediliyor. Yan işler (başlık/özet) için ucuz model yönlendirmesi yok; büyük araç sonuçları sınırlanmıyor; cache-kırılma dedektörü tam bu senaryoyu (ikinci çağrıda %100 miss) yakalayamıyor.

## Plan (kazanca göre sıralı)

| # | Ne | Nerede | Beklenen kazanç | Efor |
|---|-----|--------|-----------------|------|
| 1 | `claudePersistentSession=true` yap (tek uzun ömürlü claude süreci) **veya** Coder ajanını (AGT1) native `anthropic` provider'a geçir | `~\.tionharness\settings.json`; `internal/providers/claudecli.go:392-400` | Oturumun ~%36'sı ($0.93); tur-2 sınıfı miss'ler biter | Küçük |
| 2 | Kodlama ajanları için playwright + tarayıcı MCP sunucu satırlarını kapat; `writeCLIMCPConfig`'i per-tool visibility/DisabledTools'a saygılı hale getir | `internal/agent/climcp.go:76-135`; `internal/agent/toolsetup.go:573-599` | ~45-55 şema = 10-15k token/çağrı; 3 turluk oturum ~$0.60-0.75'e iner | Orta |
| 3 | `ExtendedPromptCache`'i varsayılan **açık** yap; timestamp'i sistem promptundan çıkarıp rolling breakpoint sonrası kullanıcı-mesajı kuyruğuna taşı | `internal/settings/settings.go:106,293-425`; `internal/providers/anthropic.go:499-517`; `internal/api/chat_turn.go:85,182-185` | Taze kurulumda sıcak çağrılar 1× tam fiyattan 0.1× cache-read'e (prefix'te ~%85-90) | Küçük |
| 4 | Lazy-tool aktivasyonunu **oturum bazında** kalıcı yap (append-only), tur-ortası Prune'u kaldır; uzun vadede API-native Tool Search (`tool_search_tool_regex_20251119` + `defer_loading:true`) | `internal/agent/toolloop.go:279,292,347-348,592`; `internal/tools/registry.go:289-313` | Araç aktivasyonu sonrası her turda ~65k token 2× fiyatlı yeniden yazımı önler | Orta |
| 5 | TTL'i yapılandırılabilir yap, varsayılan **5m** (beta header'sız `ephemeral`); 1h yalnızca uzun düşünme aralıklı otonom oturumlara opt-in | `internal/providers/anthropic.go:17,44,567-577,604-620` | Yazım primi 2×→1.25× (soğuk yazımda ~%37) | Küçük |
| 6 | Yan işleri Haiku'ya yönlendir: başlık üretimi, özet/compaction (`miniModel` ayarı) | `internal/providers/summary.go:34-43`; `internal/api` başlık çağrıları | Yan çağrılarda ~%80-95 | Küçük |
| 7 | Büyük araç sonuçlarını sınırla: ~12k token üstünü dosyaya yaz, Haiku özetini + dosya yolunu geçmişe koy | `internal/agent/toolloop.go:416-420,586-589` | Tarayıcı/MCP ağırlıklı oturumlarda on binlerce token'ın her turda yeniden faturalanması biter | Orta |
| 8 | Cache-kırılma dedektörünü düzelt: messages-prefix hash'i, tool-listesi deltası logu, "hiç ısınmayan oturum" bayrağı | `internal/providers/anthropic.go` (usage/probe mantığı) | Doğrudan kazanç yok; gelecekteki sessiz $1+/tur regresyonları görünür kılar | Küçük |

Ayrıca: dahili fiyat tablosundaki opus-4-8 satırını sağlayıcının güncel liste fiyatına çek — raporlanan maliyetler şu an 3× şişkin. ✅ **YAPILDI** (2026-07-05): `internal/providers/pricing.go` içinde `anthropic/claude-opus-4-8` ve `openrouter/anthropic/claude-opus-4.8` satırları güncel liste fiyatına göre düzeltildi; `pricing_test.go` + `billing_test.go` beklentileri düzeltildi (tümü geçiyor). Kaynak: sağlayıcının yayınlanmış liste fiyatı.

## the external agent project'tan Kopyalanacak Desenler

- **Claude Agent SDK** (`query()` + resume) tüm istek döngüsünü yönetiyor → cache breakpoint'leri, geçmiş kalıcılığı, auto-compaction otomatik (`claude-agent.ts:1332-1345`).
- **Volatile/stable ayrımı**: tarih-saat (dakika hassasiyeti), oturum durumu vb. kullanıcı mesajına eklenir; sistem promptu ilk `chat()` çağrısında **pinlenir**, oturum boyunca bayt-sabit (`prompt-builder.ts:85-133`, `claude-agent.ts:817-839`).
- **Yalnızca bağlı-aktif kaynakların araçları** eklenir; pasif kaynak tek satır metin + hata güdümlü lazy aktivasyon (`claude-agent.ts:432-466, 1503-1506`).
- **Haiku yönlendirmesi** (başlık/özet/doğrulama) ve **12k+ araç sonuçlarını diske yazma**.

## Doğrulama

Her adım sonrası aynı 3-turluk testi tekrar çalıştır (`http://127.0.0.1:8090/api/chat`, AGT1/WS1) ve yeni `sw_turn*.json` + `sw_usage.json` ile karşılaştır. Hedefler:

- Tur-2 `cacheReadTokens` ≥ tur-1 `cacheWriteTokens`'ın ~%95'i (önce: 0 / 51.920)
- Tur-2 ve tur-3 `cacheWriteTokens` < 2.000 (önce: 65.686 / 3.794)
- Toplam `cacheWriteTokens` < 60k (önce: 121.400)
- `costUSD` ≤ ~$1.00 (önce: $2.5915); adım 2 sonrası araç token'ı < 15k, araç sayısı < 90
- `debug.jsonl`'de ilk çağrı sonrası her çağrıda `cache_read > 0`; hâlâ tur-2 yazımı > 5k ise iki ardışık istek gövdesini diff'le
