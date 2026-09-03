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
| 1 | ✅ **YAPILDI** — `claudePersistentSession` artık varsayılan **açık** (`internal/settings/settings.go` `Default()`): oturum+ajan başına tek sıcak claude süreci stdin üzerinden beslenir, `--resume`'u geçersiz kılar | `internal/settings/settings.go:501`; `internal/providers/claudecli.go` | Oturumun ~%36'sı ($0.93); tur-2 sınıfı miss'ler biter | Küçük |
| 2 | Kodlama ajanları için playwright + tarayıcı MCP sunucu satırlarını kapat; `writeCLIMCPConfig`'i per-tool visibility/DisabledTools'a saygılı hale getir | `internal/agent/climcp.go:76-135`; `internal/agent/toolsetup.go:573-599` | ~45-55 şema = 10-15k token/çağrı; 3 turluk oturum ~$0.60-0.75'e iner | Orta |
| 3 | ✅ **YAPILDI** — `ExtendedPromptCache` varsayılan **açık** (`internal/settings/settings.go:449`); timestamp artık cache'li statik prefix'te değil, breakpoint sonrası `SystemDynamic` suffix'inde üretiliyor (`internal/api/chat_turn.go:56,158` `dateTimeContextBlock`) | `internal/settings/settings.go:106,293-425`; `internal/providers/anthropic.go:499-517`; `internal/api/chat_turn.go:85,182-185` | Taze kurulumda sıcak çağrılar 1× tam fiyattan 0.1× cache-read'e (prefix'te ~%85-90) | Küçük |
| 4 | Lazy-tool aktivasyonunu **oturum bazında** kalıcı yap (append-only), tur-ortası Prune'u kaldır; uzun vadede API-native Tool Search (`tool_search_tool_regex_20251119` + `defer_loading:true`) | `internal/agent/toolloop.go:279,292,347-348,592`; `internal/tools/registry.go:289-313` | Araç aktivasyonu sonrası her turda ~65k token 2× fiyatlı yeniden yazımı önler | Orta |
| 5 | TTL'i yapılandırılabilir yap, varsayılan **5m** (beta header'sız `ephemeral`); 1h yalnızca uzun düşünme aralıklı otonom oturumlara opt-in | `internal/providers/anthropic.go:17,44,567-577,604-620` | Yazım primi 2×→1.25× (soğuk yazımda ~%37) | Küçük |
| 6 | Yan işleri ucuz modele yönlendir. **Çözüm global `miniModel` ayarı değil, sistem ajanları oldu** (2026-08-26): başlık, özet, compaction, ders çıkarma ve içgörü işlerinin her biri kendi modeli/promptu olan ayrı bir sistem ajanıdır; ucuzlatma, ilgili sistem ajanının modelini değiştirmekle yapılır. Detay: [74-SISTEM-AJANLARI.md](74-SISTEM-AJANLARI.md) | `internal/agent/systemagents.go`; `internal/agent/titler.go`, `summarizer.go` | Yan çağrılarda ~%80-95 | Küçük |
| 7 | Büyük araç sonuçlarını sınırla: ~12k token üstünü dosyaya yaz, Haiku özetini + dosya yolunu geçmişe koy | `internal/agent/toolloop.go:416-420,586-589` | Tarayıcı/MCP ağırlıklı oturumlarda on binlerce token'ın her turda yeniden faturalanması biter | Orta |
| 8 | Cache-kırılma dedektörünü düzelt: messages-prefix hash'i, tool-listesi deltası logu, "hiç ısınmayan oturum" bayrağı | `internal/providers/anthropic.go` (usage/probe mantığı) | Doğrudan kazanç yok; gelecekteki sessiz $1+/tur regresyonları görünür kılar | Küçük |

Ayrıca: dahili fiyat tablosundaki opus-4-8 satırını sağlayıcının güncel liste fiyatına çek — raporlanan maliyetler şu an 3× şişkin. ✅ **YAPILDI** (2026-07-05): `internal/providers/pricing.go` içinde `anthropic/claude-opus-4-8` ve `openrouter/anthropic/claude-opus-4.8` satırları güncel liste fiyatına göre düzeltildi; `pricing_test.go` + `billing_test.go` beklentileri düzeltildi (tümü geçiyor). Kaynak: sağlayıcının yayınlanmış liste fiyatı.

## the external agent project'tan Kopyalanacak Desenler

- **Claude Agent SDK** (`query()` + resume) tüm istek döngüsünü yönetiyor → cache breakpoint'leri, geçmiş kalıcılığı, auto-compaction otomatik (`claude-agent.ts:1332-1345`).
- **Volatile/stable ayrımı**: tarih-saat (dakika hassasiyeti), oturum durumu vb. kullanıcı mesajına eklenir; sistem promptu ilk `chat()` çağrısında **pinlenir**, oturum boyunca bayt-sabit (`prompt-builder.ts:85-133`, `claude-agent.ts:817-839`).
- **Yalnızca bağlı-aktif kaynakların araçları** eklenir; pasif kaynak tek satır metin + hata güdümlü lazy aktivasyon (`claude-agent.ts:432-466, 1503-1506`).
- **Haiku yönlendirmesi** (başlık/özet/doğrulama) ve **12k+ araç sonuçlarını diske yazma**.

## 2026-09-03 güncellemesi — 30 günlük veri ve üç yeni kaldıraç

Son 30 gün (`session-usage`, claude-cli): 1.429 oturum, 4.807 tur, 443,6M cache-write
(tur başına ~92k), 7,34B cache-read, 32,3M output → harcamanın ~%90'ı yeniden gönderilen
prefix. Transkriptlerde her oturumun ilk çağrısı 50–59k `cache_creation`.

Ölçüm (claude-cli 2.1.259, `_Docs/17` "claude-cli prefix anatomisi"): taban 36,4k;
`--disallowedTools` küçültmüyor (38,8k); `--tools` allowlist 10,6–12,2k; `--tools ""` 7,3k;
MCP ertelemesi zaten çalışıyor (+460); proje CLAUDE.md +10k (kullanıcı dosyası).

| # | Ne | Durum |
|---|-----|-------|
| 9 | ✅ claude-cli yerleşik araç **allowlist'i** (`claudeCliToolAllowlist`, `--tools`) — köprülü turda taban 36k → ~11k | `internal/agent/climcp.go cliNativeToolAllowlist`, `providers/claudecli.go nativeToolArgs` |
| 10 | ✅ Yardımcı çağrılar (title/summary/compaction/reflect/btw) **köprüsüz + `--tools ""`** ve persistent havuz dışı | `internal/agent/toolloop_phases.go` (`aux`), `callkind.go isAuxiliaryKind` |
| 11 | ✅ Yardımcı çağrıları **native Anthropic API'ye yönlendirme** (`auxNativeRouting`; compaction katlaması dahil, `FoldContext`) — ~36k → ~1–2k/çağrı, API anahtarından faturalanır | `internal/agent/systemagent_route.go`, `fold_target.go`, `conversation.WithFoldTarget` |
| 12 | ✅ Stall yargıcı `stall-judge` sistem ajanı + metin-hash memo (aynı mesaj tekrar yargılanmaz) | `internal/agent/coordination_stall.go` |
| — | ✅ Kullanıcı kurulumunda `claudePersistentSession` eski `false` → `true` | `~/.tionharness/settings.json` |

Açık kalanlar: 2 (MCP sunucu satırlarını ajan bazında kapatma), 5 (TTL 1h → 5m seçeneği),
7 (büyük araç sonuçlarını dosyaya taşıma), 8 (cache-break dedektörü iyileştirmeleri).
