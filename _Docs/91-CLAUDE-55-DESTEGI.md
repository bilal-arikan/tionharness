# Claude Sonnet 5.5 ve Opus 5.5 desteği

> **Özet (2026-10-01):** İki model Anthropic API, Claude CLI ve OpenRouter
> kataloglarına eklendi. Model seçimi, düşünme ayarları, imzalı konuşma geçmişi
> ve maliyet tahminleri desteklenir. Aktif Claude aboneliği olmadığı için
> doğrulama yerel sunucu ve CLI taklitleriyle yapılır; canlı hesap erişimi
> doğrulanmış değildir.

## Model seçimi

| Sağlayıcı | Sonnet 5.5 | Opus 5.5 |
|---|---|---|
| Anthropic API | `claude-sonnet-5-5` | `claude-opus-5-5` |
| Claude CLI | `claude-sonnet-5-5` | `claude-opus-5-5` |
| OpenRouter | `anthropic/claude-sonnet-5.5` | `anthropic/claude-opus-5.5` |

CLI'de sabit kimlik seçimi `--model` ile aynen aktarılır. Boş model seçimi
CLI oturumunun varsayılanını kullanmaya devam eder. `sonnet` ve `opus`
takma adları API üzerinden çalışan yardımcı ajanlara yönlendirildiğinde
5.5 kimliklerine çevrilir; açıkça seçilen eski model kimlikleri korunur.
Anthropic sağlayıcısının genel varsayılan modeli değiştirilmemiştir.

İki model için bağlam kapasitesi 1 milyon tokendir. Sağlayıcının yayımladığı
azami çıktı kapasitesi 128 bin olsa da uygulamanın mevcut 32.768 tokenlık
istek sınırı korunur.

## Düşünme ve konuşma geçmişi

- Opus 5.5 adaptif düşünmeyi daima açık tutar. Eski kayıtlardaki kapalı ayarı
  API'ye `disabled` olarak gönderilmez; CLI'de de düşünmeyi kapatan ortam
  değişkeni zorla eklenmez. Model seçenekleri kapalı düzeyini sunmaz.
- Sonnet 5.5 kapalı ayarında yalnız `thinking.type=between_tools` gönderir.
  Bu, başlangıçtaki düşünmeyi azaltır; araç çağrıları arasındaki ilerleme
  notlarını tamamen kapatmaz. Diğer düzeyler adaptif düşünme kullanır.
- Adaptif istekler düşünme metninin özetini ister ve imzalı blokları korur.
  Konuşma ön eki değişirse `thinking-binding-controls-2026-08-01` ve
  `drop_block` ilkesi geçersiz imzaların isteği bozmasını önler.
- `between_tools` ek düşünme alanlarını kabul etmez. Uygulama geçmişi
  düzenleyebildiği için bu modda önceki düşünme blokları yeniden gönderilen
  kopyadan çıkarılır. Kaydedilmiş mesajlar, araç çağrıları ve diğer sunucu
  blokları korunur.

Fable/Mythos'a özgü reddedilen istek yedeği ve veri saklama uyarıları
Opus 5.5'e uygulanmaz. Zorlanmış araç seçimi gönderilmez.

## Maliyet ve erişim

Standart API fiyatları, milyon token başına Sonnet için giriş $2 / çıkış $10,
Opus için giriş $4 / çıkış $20 olarak kaydedildi. Önbellek okuma her iki
modelde $0,20'dir. Anthropic'in mevcut bir saatlik önbellek yazma ayarı
giriş ücretinin iki katını; OpenRouter ve CLI eşdeğer tahmini beş dakikalık
yazma için 1,25 katını kullanır.

CLI aboneliği API faturası olarak gösterilmez; ücretler eşdeğer API maliyeti
tahminidir. Gerçek erişim yapılandırılmış hesabın aboneliğine veya API
yetkisine bağlıdır. Yerel Claude Code sürümü 2.1.284 olarak tespit edildi.
Doğrulama sırasında Claude hesabına model isteği gönderilmedi.

## Doğrulama kapsamı

Yerel testler üç sağlayıcının kataloğunu, bağlam ve fiyatları, düşünme
düzeylerinin istek biçimini, normal ve akışlı API yanıtlarını, geçmiş
bloklarının korunmasını, CLI model/ortam aktarımını ve ajan oluşturma/
güncelleme akışını kapsar. Canlı abonelik doğrulaması kapsam dışındadır.

## Kaynaklar

- [Sonnet 5.5 model ve fiyat bilgileri](https://platform.claude.com/docs/en/models/sonnet-5-5/overview)
- [Sonnet 5.5 uyumluluk değişiklikleri](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5)
- [Opus 5.5 model ve fiyat bilgileri](https://platform.claude.com/docs/en/models/opus-5-5/overview)
- [Opus 5.5 uyumluluk değişiklikleri](https://platform.claude.com/docs/en/models/opus-5-5/whats-new-opus-5-5)
